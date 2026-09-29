// bgpadv/bgpadv_test.go pins the V1
// advanced BGP surface.
package bgpadv

import (
	"testing"
	"time"
)

func TestLargeCommunityString(t *testing.T) {
	l := LargeCommunity{ASN: 65000, Local1: 100, Local2: 200}
	if l.String() != "65000:100:200" {
		t.Fatalf("string: %s", l.String())
	}
}

func TestCommunityValid(t *testing.T) {
	for _, c := range []Community{CommNoExport, CommNoAdvertise, CommNoExportSubconf, CommGracefulShutdown, CommBlackhole} {
		if !c.Valid() {
			t.Fatalf("%s must be valid", c)
		}
	}
}

func TestRPKIValidate(t *testing.T) {
	m := NewRPKIManager()
	if err := m.SetROAs([]ROA{{ASN: 65000, Prefix: "10.0.0.0/8", MaxLength: 24}}); err != nil {
		t.Fatal(err)
	}
	r := m.Validate("10.0.0.0/8", 65000)
	if r.State != RPKIValid {
		t.Fatalf("expected valid, got %s", r.State)
	}
	r = m.Validate("10.0.0.0/8", 65001)
	if r.State != RPKIInvalid {
		t.Fatalf("expected invalid, got %s", r.State)
	}
	r = m.Validate("192.168.0.0/16", 65000)
	if r.State != RPKINotFound {
		t.Fatalf("expected not-found, got %s", r.State)
	}
}

func TestBMPTargetValid(t *testing.T) {
	good := BMPTarget{Address: "10.0.0.1", Port: 5000, Enabled: true}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := BMPTarget{Address: "", Port: 5000}
	if err := bad.Valid(); err == nil {
		t.Fatal("missing address must fail")
	}
	badPort := BMPTarget{Address: "x", Port: 99999}
	if err := badPort.Valid(); err == nil {
		t.Fatal("bad port must fail")
	}
}

func TestFlowSpecAddRevokeSweep(t *testing.T) {
	m := NewFlowSpecManager(time.Now)
	if err := m.Add(FlowSpec{ID: "f1", Action: FlowSpecAction{Kind: FSActionDiscard}, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	// Inject an already-expired rule for
	// the sweep test (bypassing the public
	// Add validator).
	m.mu.Lock()
	m.rules["f2"] = FlowSpec{ID: "f2", Action: FlowSpecAction{Kind: FSActionRateLimit, RateKbps: 1000}, ExpiresAt: time.Now().Add(-time.Hour)}
	m.mu.Unlock()
	expired := m.SweepExpired()
	if len(expired) != 1 || expired[0] != "f2" {
		t.Fatalf("expired: %v", expired)
	}
	if err := m.Revoke("f1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Revoke("missing"); err == nil {
		t.Fatal("missing revoke must fail")
	}
}

func TestFlowSpecOriginAllowed(t *testing.T) {
	m := NewFlowSpecManager(time.Now)
	m.Add(FlowSpec{ID: "f1", Action: FlowSpecAction{Kind: FSActionDiscard}, SourceASNs: []uint32{65000, 65001}})
	if !m.OriginAllowed("f1", 65000) {
		t.Fatal("65000 must be allowed")
	}
	if m.OriginAllowed("f1", 65002) {
		t.Fatal("65002 must be denied")
	}
	if m.OriginAllowed("missing", 65000) {
		t.Fatal("missing rule must be denied")
	}
}

func TestPrefixLen(t *testing.T) {
	if PrefixLen("10.0.0.0/8") != 8 {
		t.Fatal("bad prefix len")
	}
	if PrefixLen("10.0.0.0") != -1 {
		t.Fatal("missing prefix len")
	}
}
