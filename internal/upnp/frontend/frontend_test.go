// Cross-platform security and behaviour tests for the
// protocol frontend handlers: malformed packets, unauthorized
// clients, conflicts, quotas, rate limiting and renewals.
package frontend

import (
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/upnp"
)

// memSink is a minimal in-memory MappingSink.
type memSink struct {
	mu   sync.Mutex
	mgr  *upnp.Manager
	fail bool
}

func newMemSink(t *testing.T) *memSink {
	t.Helper()
	return &memSink{mgr: upnp.NewManager()}
}

func (m *memSink) AddMapping(mp upnp.Mapping, source string) error {
	if m.fail {
		return errSinkDown
	}
	return m.mgr.AddMapping(mp)
}

func (m *memSink) Mappings() []upnp.Mapping { return m.mgr.Mappings() }

var errSinkDown = &sinkError{}

type sinkError struct{}

func (*sinkError) Error() string { return "sink down" }

var lanNet = upnp.Network{
	NetworkRef: "lan-1",
	Protocols:  []upnp.Protocol{upnp.ProtoNATPMP, upnp.ProtoPCP, upnp.ProtoUPnP},
	TTLSec:     3600,
}

func natPMPReq(transport byte, intPort, extPort uint16, lifetime uint32) []byte {
	b := make([]byte, 12)
	b[0] = 0
	b[1] = transport
	b[2] = byte(intPort >> 8)
	b[3] = byte(intPort)
	b[4] = byte(extPort >> 8)
	b[5] = byte(extPort)
	b[6] = byte(lifetime >> 24)
	b[7] = byte(lifetime >> 16)
	b[8] = byte(lifetime >> 8)
	b[9] = byte(lifetime)
	return b
}

func clientIP() netip.Addr { return netip.MustParseAddr("192.168.10.50") }

func newTestServer(t *testing.T, sink MappingSink) (*Server, *memSink) {
	t.Helper()
	if sink == nil {
		sink = newMemSink(t)
	}
	s := NewServer(Limits{}, nil, sink)
	s.SetExternal("203.0.113.7")
	return s, sink.(*memSink)
}

func TestNATPMPMappingAccepted(t *testing.T) {
	s, sink := newTestServer(t, nil)
	res := s.HandleNATPMP(natPMPReq(1, 5000, 0, 1800), clientIP(), lanNet)
	if !res.Accepted || res.Response == nil {
		t.Fatalf("not accepted: %+v", res)
	}
	got := sink.Mappings()
	if len(got) != 1 {
		t.Fatalf("mappings: %+v", got)
	}
	m := got[0]
	if m.Protocol != upnp.ProtoNATPMP || m.Transport != upnp.TransportUDP || m.ExternalPort != 5000 || m.InternalPort != 5000 {
		t.Fatalf("mapping: %+v", m)
	}
	if m.InternalHost != clientIP().String() {
		t.Fatalf("internal host: %q", m.InternalHost)
	}
	// Lifetime is clamped to the effective default, not fabricated.
	if m.ExpiresAt.IsZero() {
		t.Fatal("no expiry")
	}
}

func TestNATPMPRenewalReplacesID(t *testing.T) {
	s, sink := newTestServer(t, nil)
	if !s.HandleNATPMP(natPMPReq(1, 5000, 5000, 600), clientIP(), lanNet).Accepted {
		t.Fatal("first not accepted")
	}
	res := s.HandleNATPMP(natPMPReq(1, 5000, 5000, 600), clientIP(), lanNet)
	if !res.Accepted {
		t.Fatalf("renewal refused: %+v", res)
	}
	if n := len(sink.Mappings()); n != 1 {
		t.Fatalf("renewal duplicated mapping: %d", n)
	}
}

func TestNATPMPConflictRefused(t *testing.T) {
	s, _ := newTestServer(t, nil)
	if !s.HandleNATPMP(natPMPReq(1, 5000, 5000, 600), clientIP(), lanNet).Accepted {
		t.Fatal("first not accepted")
	}
	other := netip.MustParseAddr("192.168.10.51")
	res := s.HandleNATPMP(natPMPReq(1, 8080, 5000, 600), other, lanNet)
	if res.Accepted {
		t.Fatal("conflicting mapping accepted")
	}
	found := false
	for _, r := range s.Journal().Recent(16) {
		if r.Reason == "external port already mapped" {
			found = true
		}
	}
	if !found {
		t.Fatal("conflict not journaled")
	}
}

func TestNATPMPAllowList(t *testing.T) {
	s, _ := newTestServer(t, nil)
	restricted := lanNet
	restricted.AllowedClients = []string{"192.168.10.99"}
	res := s.HandleNATPMP(natPMPReq(1, 5000, 0, 600), clientIP(), restricted)
	if res.Accepted {
		t.Fatal("unauthorized client accepted")
	}
}

func TestNATPMPQuotaAndRateLimit(t *testing.T) {
	s, _ := newTestServer(t, nil)
	limits := s.Limits()
	other := netip.MustParseAddr("192.168.10.77")
	var accepted int
	for i := 1; i <= limits.MaxMappingsPerClient+1; i++ {
		port := uint16(20000 + i)
		if s.HandleNATPMP(natPMPReq(1, port, 0, 600), other, lanNet).Accepted {
			accepted++
		}
	}
	if accepted != limits.MaxMappingsPerClient {
		t.Fatalf("quota: accepted %d, want %d", accepted, limits.MaxMappingsPerClient)
	}
	if n := len(s.Journal().Recent(64)); n == 0 {
		t.Fatal("quota denial not journaled")
	}
	// Rate limit: flood from a fresh address.
	fresh := netip.MustParseAddr("192.168.10.78")
	denied := 0
	for i := 0; i < limits.RequestsPerMinute+10; i++ {
		if !s.HandleNATPMP(natPMPReq(1, 30000, 0, 0), fresh, lanNet).Accepted {
			denied++
		}
	}
	if denied != 10 {
		t.Fatalf("rate limit: denied %d, want 10", denied)
	}
}

func TestNATPMPGlobalLimitAndSinkFailure(t *testing.T) {
	sink := newMemSink(t)
	s := NewServer(Limits{MaxMappingsTotal: 2}, nil, sink)
	s.SetExternal("203.0.113.7")
	if !s.HandleNATPMP(natPMPReq(1, 1001, 0, 60), clientIP(), lanNet).Accepted {
		t.Fatal("first refused")
	}
	second := netip.MustParseAddr("192.168.10.60")
	if !s.HandleNATPMP(natPMPReq(1, 1002, 0, 60), second, lanNet).Accepted {
		t.Fatal("second refused")
	}
	third := netip.MustParseAddr("192.168.10.61")
	if s.HandleNATPMP(natPMPReq(1, 1003, 0, 60), third, lanNet).Accepted {
		t.Fatal("global limit not enforced")
	}
	// Sink failure must be refused, never silently dropped.
	sink.fail = true
	if s.HandleNATPMP(natPMPReq(1, 1010, 0, 60), clientIP(), lanNet).Accepted {
		t.Fatal("sink failure accepted")
	}
}

func TestNATPMPMalformedAndPublicAddress(t *testing.T) {
	s, _ := newTestServer(t, nil)
	// Malformed junk still gets a best-effort refusal, not a panic.
	res := s.HandleNATPMP([]byte{0, 1, 9}, clientIP(), lanNet)
	if res.Accepted || res.Response == nil {
		t.Fatalf("malformed handling: %+v", res)
	}
	// Public address returns the observed external address.
	res = s.HandleNATPMP([]byte{0, 128}, clientIP(), lanNet)
	if res.Accepted || len(res.Response) != 12 {
		t.Fatalf("public address: %+v", res)
	}
	if ext := res.Response[8:12]; ext[0] != 203 || ext[1] != 0 || ext[2] != 113 || ext[3] != 7 {
		t.Fatalf("external ip bytes: %v", ext)
	}
	// Unobserved external address = honest network failure.
	s2 := NewServer(Limits{}, nil, newMemSink(t))
	res = s2.HandleNATPMP([]byte{0, 128}, clientIP(), lanNet)
	if res.Response[2] != 0 || res.Response[3] != 3 {
		t.Fatalf("expected network-failure code, got %v", res.Response[2:4])
	}
}

func pcpMapReq(protoNum byte, intPort, extPort uint16, extIP netip.Addr, headerIP netip.Addr, lifetime uint32, opts ...[]byte) []byte {
	buf := make([]byte, 64)
	buf[0] = 2
	buf[1] = 2
	buf[4] = byte(lifetime >> 24)
	buf[5] = byte(lifetime >> 16)
	buf[6] = byte(lifetime >> 8)
	buf[7] = byte(lifetime)
	h := headerIP.As16()
	copy(buf[8:24], h[:])
	p := buf[24:]
	p[12] = protoNum
	p[16] = byte(intPort >> 8)
	p[17] = byte(intPort)
	p[18] = byte(extPort >> 8)
	p[19] = byte(extPort)
	if extIP.IsValid() {
		e := extIP.As16()
		copy(p[20:36], e[:])
	}
	p[36] = byte(lifetime >> 24)
	p[37] = byte(lifetime >> 16)
	p[38] = byte(lifetime >> 8)
	p[39] = byte(lifetime)
	for _, o := range opts {
		buf = append(buf, o...)
	}
	return buf
}

func TestPCPMapAcceptedAndAddressMismatch(t *testing.T) {
	s, sink := newTestServer(t, nil)
	res := s.HandlePCP(pcpMapReq(17, 5000, 0, netip.Addr{}, clientIP(), 1800), clientIP(), lanNet)
	if !res.Accepted {
		t.Fatalf("pcp map refused: %+v", res)
	}
	m := sink.Mappings()[0]
	if m.Protocol != upnp.ProtoPCP || m.Transport != upnp.TransportUDP {
		t.Fatalf("mapping: %+v", m)
	}
	// Address mismatch: header claims a neighbour address.
	res = s.HandlePCP(pcpMapReq(17, 6000, 0, netip.Addr{}, netip.MustParseAddr("192.168.10.51"), 1800), clientIP(), lanNet)
	if res.Accepted {
		t.Fatal("address mismatch accepted")
	}
}

func TestPCPUnsupportedOptionAndProtocol(t *testing.T) {
	s, _ := newTestServer(t, nil)
	thirdParty := []byte{1, 0, 0, 0, 0, 0}
	res := s.HandlePCP(pcpMapReq(17, 5000, 0, netip.Addr{}, clientIP(), 600, thirdParty), clientIP(), lanNet)
	if res.Accepted {
		t.Fatal("THIRD_PARTY accepted")
	}
	res = s.HandlePCP(pcpMapReq(132, 5000, 0, netip.Addr{}, clientIP(), 600), clientIP(), lanNet)
	if res.Accepted {
		t.Fatal("SCTP protocol accepted")
	}
	// PEER opcode refused.
	peer := pcpMapReq(17, 5000, 0, netip.Addr{}, clientIP(), 600)
	peer[1] = 3
	res = s.HandlePCP(peer, clientIP(), lanNet)
	if res.Accepted || res.Response == nil {
		t.Fatalf("peer handling: %+v", res)
	}
}

func TestPCPExternalAddressConflict(t *testing.T) {
	s, _ := newTestServer(t, nil)
	res := s.HandlePCP(pcpMapReq(6, 5000, 0, netip.MustParseAddr("198.51.100.9"), clientIP(), 600), clientIP(), lanNet)
	if res.Accepted {
		t.Fatal("wrong suggested external accepted")
	}
	// Matching observed external address succeeds.
	res = s.HandlePCP(pcpMapReq(6, 5001, 0, netip.MustParseAddr("203.0.113.7"), clientIP(), 600), clientIP(), lanNet)
	if !res.Accepted {
		t.Fatalf("matching external refused: %+v", res)
	}
}

func TestJournalRing(t *testing.T) {
	j := NewRejectionJournal(16)
	for i := 0; i < 20; i++ {
		j.Add(Rejection{At: time.Unix(int64(i), 0), Protocol: "x", Reason: string(rune('a' + i))})
	}
	if j.Count() != 16 {
		t.Fatalf("count: %d", j.Count())
	}
	rec := j.Recent(16)
	// Oldest surviving is i=4; order must be oldest-first.
	if rec[0].Reason != string(rune('a'+4)) || rec[15].Reason != string(rune('a'+19)) {
		t.Fatalf("ring order: %q..%q", rec[0].Reason, rec[15].Reason)
	}
	// Recent(4) returns only the newest four, oldest-first.
	rec = j.Recent(4)
	if rec[0].Reason != string(rune('a'+16)) || rec[3].Reason != string(rune('a'+19)) {
		t.Fatalf("recent window: %+v", rec)
	}
}

func TestRateBudgetWindowResets(t *testing.T) {
	now := time.Unix(0, 0)
	s := NewServer(Limits{RequestsPerMinute: 2}, nil, newMemSink(t))
	s.core.nowFn = func() time.Time { return now }
	ip := clientIP()
	// 2 allowed, 3rd denied in the same window.
	s.HandleNATPMP(natPMPReq(1, 1000, 0, 60), ip, lanNet)
	s.HandleNATPMP(natPMPReq(1, 1001, 0, 60), ip, lanNet)
	if r := s.HandleNATPMP(natPMPReq(1, 1002, 0, 60), ip, lanNet); r.Accepted {
		t.Fatal("budget not enforced")
	}
	// Advance past the window: budget resets.
	now = now.Add(time.Minute + time.Second)
	if r := s.HandleNATPMP(natPMPReq(1, 1003, 0, 60), ip, lanNet); !r.Accepted {
		t.Fatal("window did not reset")
	}
}
