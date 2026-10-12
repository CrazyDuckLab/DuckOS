package wake

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"
)

func TestMagicPacketLayout(t *testing.T) {
	mac := "AA:BB:CC:DD:EE:FF"
	pkt, err := magicPacket(mac)
	if err != nil {
		t.Fatalf("magicPacket: %v", err)
	}
	if len(pkt) != 102 {
		t.Fatalf("len=%d", len(pkt))
	}
	// First 6 bytes: 0xFF
	for i := 0; i < 6; i++ {
		if pkt[i] != 0xFF {
			t.Fatalf("byte %d: %x", i, pkt[i])
		}
	}
	// Then 16 repetitions of the MAC.
	hw, _ := net.ParseMAC(mac)
	for i := 6; i < 102; i += 6 {
		if !bytes.Equal(pkt[i:i+6], hw) {
			t.Fatalf("repeat %d: %v", i/6, pkt[i:i+6])
		}
	}
}

func TestMagicPacketRejectsShort(t *testing.T) {
	if _, err := magicPacket("AA:BB:CC:DD:EE"); err == nil {
		t.Fatal("expected short mac rejection")
	}
}

func TestValidate_Good(t *testing.T) {
	o := SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 3}
	if err := o.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_BadMAC(t *testing.T) {
	o := SendOpts{Target: "not-a-mac", Port: 9, Packets: 3}
	if err := o.Validate(); err == nil {
		t.Fatal("expected mac rejection")
	}
}

func TestValidate_BadPort(t *testing.T) {
	o := SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 22, Packets: 3}
	if err := o.Validate(); err == nil {
		t.Fatal("expected port rejection")
	}
}

func TestValidate_BadPackets(t *testing.T) {
	o := SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 99}
	if err := o.Validate(); err == nil {
		t.Fatal("expected packets rejection")
	}
}

func TestValidate_ReasonTooLong(t *testing.T) {
	o := SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 3, Reason: strings.Repeat("x", 1024)}
	if err := o.Validate(); err == nil {
		t.Fatal("expected reason rejection")
	}
}

func TestManager_RateLimit(t *testing.T) {
	m := NewManager()
	// Pre-seed the current 1-second window to the refusal
	// threshold: firing five real bursts and requiring all of
	// them to land inside one unix-second makes the test
	// flaky on a loaded host (a boundary crossing legitimately
	// resets the window mid-test).
	m.mu.Lock()
	m.lastMin = time.Now().UTC().Unix()
	m.count = 5
	m.mu.Unlock()
	if _, err := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 1}); err == nil {
		t.Fatal("expected rate limit refusal")
	}
}

func TestManager_DefaultBroadcastIs255(t *testing.T) {
	m := NewManager()
	res, err := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Broadcast != "255.255.255.255:9" {
		t.Fatalf("broadcast: %s", res.Broadcast)
	}
}

func TestManager_DefaultPacketsIs3(t *testing.T) {
	m := NewManager()
	res, _ := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9})
	if res.Packets != 3 {
		t.Fatalf("packets: %d", res.Packets)
	}
}

func TestManager_Port7(t *testing.T) {
	m := NewManager()
	res, err := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 7, Packets: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Port != 7 {
		t.Fatalf("port: %d", res.Port)
	}
}

func TestManager_ObservationPacketSent(t *testing.T) {
	m := NewManager()
	res, err := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Observation != ObservationPacketSent {
		t.Fatalf("observation: %s", res.Observation)
	}
	if res.Status != StatusSent {
		t.Fatalf("status: %s", res.Status)
	}
}

func TestManager_DirectedUDP(t *testing.T) {
	m := NewManager()
	raddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.10"), Port: 9}
	res, err := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 1, Broadcast: raddr})
	if err != nil {
		t.Fatal(err)
	}
	if res.Broadcast != "192.168.1.10:9" {
		t.Fatalf("broadcast: %s", res.Broadcast)
	}
}

func TestManager_LowercaseMAC(t *testing.T) {
	m := NewManager()
	if _, err := m.Send(SendOpts{Target: "aa:bb:cc:dd:ee:ff", Port: 9, Packets: 1}); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestManager_SentAtPopulated(t *testing.T) {
	m := NewManager()
	before := time.Now().UTC()
	res, _ := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 1})
	if res.SentAt.Before(before) {
		t.Fatalf("SentAt not populated")
	}
}

func TestManager_DurationPopulated(t *testing.T) {
	m := NewManager()
	res, _ := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 1})
	if res.Duration == "" {
		t.Fatal("duration empty")
	}
}

func TestManager_NonUDPAddrRejected(t *testing.T) {
	m := NewManager()
	bad := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 9}
	if _, err := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 1, Broadcast: bad}); err == nil {
		t.Fatal("expected non-UDP broadcast rejection")
	}
}

func TestManager_BytesLength(t *testing.T) {
	m := NewManager()
	res, _ := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 3})
	if res.Bytes != 102*3 {
		t.Fatalf("bytes: %d", res.Bytes)
	}
}

func TestManager_RejectedCallDoesNotIncrement(t *testing.T) {
	m := NewManager()
	for i := 0; i < 4; i++ {
		_, _ = m.Send(SendOpts{Target: "bad", Port: 9, Packets: 1})
	}
	if _, err := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 1}); err != nil {
		t.Fatalf("valid send after rejected calls: %v", err)
	}
}

func TestManager_BurstCap(t *testing.T) {
	if MaxPacketsPerBurst < 1 {
		t.Fatal("burst cap must be positive")
	}
	o := SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: MaxPacketsPerBurst}
	if err := o.Validate(); err != nil {
		t.Fatalf("burst at cap: %v", err)
	}
}

func TestManager_BurstAboveCapRejected(t *testing.T) {
	o := SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: MaxPacketsPerBurst + 1}
	if err := o.Validate(); err == nil {
		t.Fatal("expected above-cap rejection")
	}
}

func TestManager_NegativePacketsRejected(t *testing.T) {
	o := SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: -1}
	if err := o.Validate(); err == nil {
		t.Fatal("expected negative packets rejection")
	}
}

func TestManager_ZeroPortMapsToNine(t *testing.T) {
	if portOrDefault(0) != 9 {
		t.Fatal("zero port should default to 9")
	}
}

func TestManager_NonZeroPortPassesThrough(t *testing.T) {
	if portOrDefault(7) != 7 {
		t.Fatal("non-zero port should pass through")
	}
}

func TestValidate_DashesInsteadOfColons(t *testing.T) {
	o := SendOpts{Target: "AA-BB-CC-DD-EE-FF", Port: 9, Packets: 3}
	if err := o.Validate(); err == nil {
		t.Fatal("expected dashed MAC rejection")
	}
}

func TestValidate_LongMAC(t *testing.T) {
	o := SendOpts{Target: "AA:BB:CC:DD:EE:FF:11", Port: 9, Packets: 3}
	if err := o.Validate(); err == nil {
		t.Fatal("expected long MAC rejection")
	}
}

func TestValidate_ShortMAC(t *testing.T) {
	o := SendOpts{Target: "AA:BB:CC:DD:EE", Port: 9, Packets: 3}
	if err := o.Validate(); err == nil {
		t.Fatal("expected short MAC rejection")
	}
}

func TestManager_RateWindowResets(t *testing.T) {
	m := NewManager()
	// Pre-seed the current 1-second window to the refusal
	// threshold instead of firing six real bursts: on a
	// loaded host six rapid Sends can straddle a
	// unix-second boundary, which legitimately resets the
	// window mid-test and makes the refusal flaky.
	m.mu.Lock()
	m.lastMin = time.Now().UTC().Unix()
	m.count = 5
	m.mu.Unlock()
	if _, err := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 1}); err == nil {
		t.Fatal("expected rate limit refusal")
	}
	// Advance the observed window by one second: the next
	// Send must reset the counter and go through.
	m.mu.Lock()
	m.lastMin = time.Now().UTC().Unix() - 1
	m.count = 5
	m.mu.Unlock()
	if _, err := m.Send(SendOpts{Target: "AA:BB:CC:DD:EE:FF", Port: 9, Packets: 1}); err != nil {
		t.Fatalf("after reset: %v", err)
	}
}

func TestMagicPacketDifferentMAC(t *testing.T) {
	pkt, err := magicPacket("12:34:56:78:9A:BC")
	if err != nil {
		t.Fatal(err)
	}
	hw, _ := net.ParseMAC("12:34:56:78:9A:BC")
	for i := 6; i < 102; i += 6 {
		if !bytes.Equal(pkt[i:i+6], hw) {
			t.Fatalf("repeat %d", i/6)
		}
	}
}

func TestMagicPacketEmptyMAC(t *testing.T) {
	if _, err := magicPacket(""); err == nil {
		t.Fatal("expected empty MAC rejection")
	}
}

func TestMagicPacketInvalidHex(t *testing.T) {
	if _, err := magicPacket("ZZ:BB:CC:DD:EE:FF"); err == nil {
		t.Fatal("expected invalid hex rejection")
	}
}

func TestManager_FailureObservation(t *testing.T) {
	if ObservationFailed == "" || StatusFailed == "" {
		t.Fatal("failure constants must be defined")
	}
}
