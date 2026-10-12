// Package wake: bounded Wake-on-LAN action.
//
// Send a standard WOL magic packet (UDP port 9, the
// "discard" port) to a target MAC. The package never:
//
//   - accepts an arbitrary shell string
//   - claims the device woke successfully
//   - returns the host's ARP / ND state
//
// The output of every send is:
//
//	PACKET_SENT     at time, length, target MAC, broadcast addr
//
// Wake-on-LAN is one-shot: the package cannot observe the
// device's powered state without a separate discovery layer
// (which is what P065's UPnP side eventually owns).
package wake

import (
	"errors"
	"net"
	"regexp"
	"sync"
	"time"
)

// MaxPacketsPerBurst caps a single Send call so a runaway
// caller cannot flood the LAN with WOL packets.
const MaxPacketsPerBurst = 8

// MaxBroadcast is the destination address cap. WOL is
// typically broadcast, but operators may want a directed
// UDP send to a known host.
var (
	macPattern = regexp.MustCompile(`^(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)
	bcastV4    = net.IPv4(255, 255, 255, 255)
)

// SendOpts is the operator-facing input.
type SendOpts struct {
	// Target is the canonical MAC of the device to wake.
	Target string
	// Source is a free-form caller identifier (audit).
	Source string
	// Reason is a free-form audit reason (capped).
	Reason string
	// Broadcast is the destination address. Empty means
	// 255.255.255.255.
	Broadcast net.Addr
	// Port is the UDP port. 0 or 9 means the WOL discard
	// port. WOL clients commonly use 7 or 9; we accept
	// either as long as the port is reserved.
	Port int
	// Packets is the burst size. 1 is the WOL default.
	// Capped at MaxPacketsPerBurst.
	Packets int
}

// Result is the redacted result of a send.
type Result struct {
	Target      string    `json:"target"`
	Broadcast   string    `json:"broadcast"`
	Port        int       `json:"port"`
	Packets     int       `json:"packets"`
	Bytes       int       `json:"bytes"`
	SentAt      time.Time `json:"sentAt"`
	Duration    string    `json:"duration"`
	Status      string    `json:"status"`
	Observation string    `json:"observation"`
}

// Observation is the operator-facing caveat: PACKET_SENT,
// not DEVICE_ONLINE. The Agent renders this verbatim.
const (
	ObservationPacketSent = "PACKET_SENT"
	ObservationFailed     = "PACKET_FAILED"
	StatusSent            = "sent"
	StatusFailed          = "failed"
)

// Manager owns the per-second rate limiter. Wake-on-LAN is
// cheap but not free: a 1 s cap prevents accidental flood.
type Manager struct {
	mu      sync.Mutex
	lastMin int64 // unix-second of last send
	count   int   // sends in the current 1 s window
}

// NewManager returns a fresh Manager.
func NewManager() *Manager { return &Manager{} }

// Validate enforces the bounded input.
func (o SendOpts) Validate() error {
	if !macPattern.MatchString(o.Target) {
		return errors.New("wake: target MAC is not canonical (expect XX:XX:XX:XX:XX:XX)")
	}
	if o.Port != 0 && o.Port != 7 && o.Port != 9 {
		return errors.New("wake: port must be 0, 7 or 9")
	}
	if o.Packets < 0 || o.Packets > MaxPacketsPerBurst {
		return errors.New("wake: packets out of bounds")
	}
	if len(o.Reason) > 256 {
		return errors.New("wake: reason exceeds size cap")
	}
	return nil
}

// Send dispatches a WOL packet burst. The Manager's
// rate limiter caps at 5 sends per second; bursts above the
// cap are refused loudly.
func (m *Manager) Send(o SendOpts) (Result, error) {
	if err := o.Validate(); err != nil {
		return Result{}, err
	}
	m.mu.Lock()
	now := time.Now().UTC()
	curMin := now.Unix()
	if curMin != m.lastMin {
		m.lastMin = curMin
		m.count = 0
	}
	if m.count >= 5 {
		m.mu.Unlock()
		return Result{}, errors.New("wake: rate limit exceeded (5 per second)")
	}
	m.count++
	m.mu.Unlock()
	bcast := o.Broadcast
	if bcast == nil {
		bcast = &net.UDPAddr{IP: bcastV4, Port: portOrDefault(o.Port)}
	}
	pkts := o.Packets
	if pkts == 0 {
		pkts = 3 // WOL convention: 3 packets
	}
	payload, err := magicPacket(o.Target)
	if err != nil {
		return Result{}, err
	}
	raddr, ok := bcast.(*net.UDPAddr)
	if !ok {
		return Result{}, errors.New("wake: broadcast must be a UDP address")
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()
	start := time.Now().UTC()
	for i := 0; i < pkts; i++ {
		if _, err := conn.Write(payload); err != nil {
			return Result{
				Target:      o.Target,
				Broadcast:   raddr.String(),
				Port:        raddr.Port,
				Packets:     i,
				Bytes:       len(payload),
				SentAt:      start,
				Duration:    time.Since(start).String(),
				Status:      StatusFailed,
				Observation: ObservationFailed,
			}, err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return Result{
		Target:      o.Target,
		Broadcast:   raddr.String(),
		Port:        raddr.Port,
		Packets:     pkts,
		Bytes:       len(payload) * pkts,
		SentAt:      start,
		Duration:    time.Since(start).String(),
		Status:      StatusSent,
		Observation: ObservationPacketSent,
	}, nil
}

func portOrDefault(p int) int {
	if p == 0 {
		return 9
	}
	return p
}

// magicPacket builds the canonical 102-byte WOL magic
// packet: 6 bytes of 0xFF followed by the target MAC
// repeated 16 times.
func magicPacket(mac string) ([]byte, error) {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return nil, err
	}
	if len(hw) != 6 {
		return nil, errors.New("wake: target MAC must be 6 bytes")
	}
	out := make([]byte, 102)
	for i := 0; i < 6; i++ {
		out[i] = 0xFF
	}
	for i := 6; i < 102; i += 6 {
		copy(out[i:i+6], hw)
	}
	return out, nil
}
