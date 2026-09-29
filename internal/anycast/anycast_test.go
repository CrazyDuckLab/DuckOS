// Anycast evidence contract torture regression.
package anycast

import (
	"sort"
	"testing"
	"time"
)

func sampleVIP(name string) VIPIdentifier {
	return VIPIdentifier{Name: name, IP: "192.0.2.10"}
}

func sampleDesired(name string) DesiredRecord {
	return DesiredRecord{
		VIP:               sampleVIP(name),
		State:             HysteresisAnnounce,
		HysteresisSeconds: 30,
		UpdatedAt:         time.Now().UTC(),
	}
}

func sampleHealth(name string, at time.Time) HealthRecord {
	return HealthRecord{
		VIP:        sampleVIP(name),
		Outcome:    ProbeSucceeded,
		ObservedAt: at,
		Probe:      "dns-resolver",
	}
}

func sampleAdvertisement(name string, active bool) AdvertisementRecord {
	return AdvertisementRecord{
		VIP:        sampleVIP(name),
		Active:     active,
		ObservedAt: time.Now().UTC(),
		Peers:      []string{"peer-1", "peer-2"},
	}
}

func TestHysteresisState_Valid(t *testing.T) {
	for _, s := range []HysteresisState{HysteresisAnnounce, HysteresisWithdrawn, HysteresisHoldDown} {
		if !s.Valid() {
			t.Errorf("%q must be valid", s)
		}
	}
	if HysteresisState("garbage").Valid() {
		t.Errorf("garbage must be invalid")
	}
}

func TestProbeOutcome_Valid(t *testing.T) {
	for _, o := range []ProbeOutcome{ProbeSucceeded, ProbeFailed, ProbeUnknown} {
		if !o.Valid() {
			t.Errorf("%q must be valid", o)
		}
	}
	if ProbeOutcome("garbage").Valid() {
		t.Errorf("garbage must be invalid")
	}
}

func TestVIPIdentifier_Valid(t *testing.T) {
	for _, v := range []VIPIdentifier{
		{Name: "vip-dns", IP: "192.0.2.10"},
		{Name: "vip-ipv6", IP: "2001:db8::1"},
	} {
		if !v.Valid() {
			t.Errorf("%v must be valid", v)
		}
	}
	for _, v := range []VIPIdentifier{
		{Name: "X", IP: "192.0.2.10"},
		{Name: "vip-bad-ip", IP: "not-an-ip"},
		{Name: "", IP: "192.0.2.10"},
	} {
		if v.Valid() {
			t.Errorf("%v must be invalid", v)
		}
	}
}

func TestDesiredRecord_Validate(t *testing.T) {
	d := sampleDesired("vip-dns")
	if err := d.Validate(); err != nil {
		t.Fatalf("valid desired: %v", err)
	}
	// Bad state
	bad := sampleDesired("vip-dns")
	bad.State = HysteresisState("garbage")
	if err := bad.Validate(); err == nil {
		t.Fatalf("bad state must be rejected")
	}
	// Hysteresis out of range
	bad2 := sampleDesired("vip-dns")
	bad2.HysteresisSeconds = -1
	if err := bad2.Validate(); err == nil {
		t.Fatalf("negative hysteresis must be rejected")
	}
	bad3 := sampleDesired("vip-dns")
	bad3.HysteresisSeconds = 7200
	if err := bad3.Validate(); err == nil {
		t.Fatalf("excessive hysteresis must be rejected")
	}
	// Zero updatedAt
	bad4 := sampleDesired("vip-dns")
	bad4.UpdatedAt = time.Time{}
	if err := bad4.Validate(); err == nil {
		t.Fatalf("zero updatedAt must be rejected")
	}
}

func TestHealthRecord_Validate(t *testing.T) {
	h := sampleHealth("vip-dns", time.Now().UTC())
	if err := h.Validate(); err != nil {
		t.Fatalf("valid health: %v", err)
	}
	// Bad outcome
	bad := sampleHealth("vip-dns", time.Now().UTC())
	bad.Outcome = ProbeOutcome("garbage")
	if err := bad.Validate(); err == nil {
		t.Fatalf("bad outcome must be rejected")
	}
}

func TestAdvertisementRecord_Validate(t *testing.T) {
	a := sampleAdvertisement("vip-dns", true)
	if err := a.Validate(); err != nil {
		t.Fatalf("valid ad: %v", err)
	}
	bad := sampleAdvertisement("vip-dns", true)
	bad.ObservedAt = time.Time{}
	if err := bad.Validate(); err == nil {
		t.Fatalf("zero observedAt must be rejected")
	}
}

func TestStore_SetAndGetDesired(t *testing.T) {
	s := NewStore()
	if err := s.SetDesired(sampleDesired("vip-dns")); err != nil {
		t.Fatal(err)
	}
	ev := s.Evidence(sampleVIP("vip-dns"))
	if ev.Desired == nil || ev.Desired.State != HysteresisAnnounce {
		t.Fatalf("drift: %+v", ev)
	}
}

func TestStore_SetDesiredRejectsInvalid(t *testing.T) {
	s := NewStore()
	bad := sampleDesired("vip-dns")
	bad.State = HysteresisState("garbage")
	if err := s.SetDesired(bad); err == nil {
		t.Fatalf("invalid state must be rejected")
	}
}

func TestStore_AppendHealth(t *testing.T) {
	s := NewStore()
	_ = s.SetDesired(sampleDesired("vip-dns"))
	if err := s.AppendHealth(sampleHealth("vip-dns", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	ev := s.Evidence(sampleVIP("vip-dns"))
	if ev.LatestHealth == nil || ev.LatestHealth.Outcome != ProbeSucceeded {
		t.Fatalf("drift: %+v", ev)
	}
}

func TestStore_AppendAdvertisement(t *testing.T) {
	s := NewStore()
	_ = s.SetDesired(sampleDesired("vip-dns"))
	if err := s.AppendAdvertisement(sampleAdvertisement("vip-dns", true)); err != nil {
		t.Fatal(err)
	}
	ev := s.Evidence(sampleVIP("vip-dns"))
	if ev.Advertisement == nil || !ev.Advertisement.Active {
		t.Fatalf("drift: %+v", ev)
	}
}

func TestStore_TrimsRings(t *testing.T) {
	s := NewStore()
	s.SetLimits(32, 3, time.Hour)
	_ = s.SetDesired(sampleDesired("vip-dns"))
	base := time.Now().UTC()
	for i := 0; i < 10; i++ {
		_ = s.AppendHealth(sampleHealth("vip-dns", base.Add(time.Duration(i)*time.Millisecond)))
	}
	all := s.AllEvidence()
	if len(all) != 1 {
		t.Fatalf("expected 1 VIP, got %d", len(all))
	}
	// Counters tracks total evidence.
	c := s.Counters()
	if c.Health != 3 {
		t.Fatalf("expected 3 health records after trim, got %d", c.Health)
	}
}

func TestStore_TrimsToMaxVIPs(t *testing.T) {
	s := NewStore()
	s.SetLimits(3, 64, time.Hour)
	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		d := sampleDesired("vip-" + string(rune('a'+i)))
		d.UpdatedAt = base.Add(time.Duration(i) * time.Millisecond)
		_ = s.SetDesired(d)
	}
	if got := len(s.AllVIPs()); got != 3 {
		t.Fatalf("expected 3 VIPs after trim, got %d", got)
	}
}

func TestStore_AllVIPsSorted(t *testing.T) {
	s := NewStore()
	if err := s.SetDesired(sampleDesired("vip-zeta")); err != nil {
		t.Fatalf("SetDesired(vip-zeta): %v", err)
	}
	if err := s.SetDesired(sampleDesired("vip-alpha")); err != nil {
		t.Fatalf("SetDesired(vip-alpha): %v", err)
	}
	if err := s.SetDesired(sampleDesired("vip-mu")); err != nil {
		t.Fatalf("SetDesired(vip-mu): %v", err)
	}
	got := s.AllVIPs()
	if len(got) != 3 {
		t.Fatalf("expected 3 VIPs, got %d", len(got))
	}
	if !sort.StringsAreSorted([]string{got[0].Name, got[1].Name, got[2].Name}) {
		t.Fatalf("not sorted: %+v", got)
	}
}

func TestStore_GCExpired(t *testing.T) {
	s := NewStore()
	s.SetLimits(32, 64, 50*time.Millisecond)
	_ = s.SetDesired(sampleDesired("vip-dns"))
	_ = s.AppendHealth(sampleHealth("vip-dns", time.Now().Add(-time.Second)))
	_ = s.AppendHealth(sampleHealth("vip-dns", time.Now().UTC()))
	_ = s.AppendAdvertisement(sampleAdvertisement("vip-dns", true))
	removed := s.GCExpired()
	if removed != 1 {
		t.Fatalf("expected 1 expired evidence, got %d", removed)
	}
	if c := s.Counters(); c.Health != 1 || c.Advertise != 1 {
		t.Fatalf("counters drift: %+v", c)
	}
}

func TestStore_Evidence_OnlyDesiredWhenIPMatches(t *testing.T) {
	s := NewStore()
	_ = s.SetDesired(sampleDesired("vip-dns"))
	// Different IP must miss.
	ev := s.Evidence(VIPIdentifier{Name: "vip-dns", IP: "198.51.100.10"})
	if ev.Desired != nil {
		t.Fatalf("different IP must yield nil Desired")
	}
}

func TestStore_Evidence_MissingVIPReturnsEmpty(t *testing.T) {
	s := NewStore()
	ev := s.Evidence(sampleVIP("missing"))
	if ev.Desired != nil || ev.LatestHealth != nil || ev.Advertisement != nil {
		t.Fatalf("missing VIP must yield empty evidence")
	}
}
