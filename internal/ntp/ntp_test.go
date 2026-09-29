// ntp/ntp_test.go pins the V1 NTP/PTP
// surface.
package ntp

import "testing"

func TestSourceValid(t *testing.T) {
	good := Source{Name: "ntp1", Kind: SourceNTP, Address: "10.0.0.1", Stratum: 2, MinStratum: 1, MaxDriftMs: 100, Enabled: true}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noAddr := Source{Name: "x", Kind: SourceNTP}
	if err := noAddr.Valid(); err == nil {
		t.Fatal("NTP without address must fail")
	}
	badKind := Source{Name: "x", Kind: "garbage"}
	if err := badKind.Valid(); err == nil {
		t.Fatal("unknown kind must fail")
	}
	badName := Source{Name: "!bad", Kind: SourceManual}
	if err := badName.Valid(); err == nil {
		t.Fatal("non-canonical name must fail")
	}
	badStratum := Source{Name: "x", Kind: SourceManual, Stratum: 99}
	if err := badStratum.Valid(); err == nil {
		t.Fatal("stratum out of range must fail")
	}
}

func TestPTPConfigValid(t *testing.T) {
	good := PTPConfig{Interface: "eth0", Domain: 0, Transport: "udp4", Mode: "slave"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noIface := PTPConfig{Mode: "slave"}
	if err := noIface.Valid(); err == nil {
		t.Fatal("missing interface must fail")
	}
	badTransport := PTPConfig{Interface: "eth0", Transport: "garbage"}
	if err := badTransport.Valid(); err == nil {
		t.Fatal("unknown transport must fail")
	}
	badMode := PTPConfig{Interface: "eth0", Mode: "garbage"}
	if err := badMode.Valid(); err == nil {
		t.Fatal("unknown mode must fail")
	}
}

func TestManagerSourceCRUD(t *testing.T) {
	m := NewManager(nil)
	if err := m.SetSource(Source{Name: "ntp1", Kind: SourceNTP, Address: "10.0.0.1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetSource(Source{Name: "ptp1", Kind: SourcePTP, Address: "/dev/ptp0", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if len(m.Sources()) != 2 {
		t.Fatalf("sources: %d", len(m.Sources()))
	}
	if err := m.RemoveSource("ntp1"); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveSource("ntp1"); err == nil {
		t.Fatal("remove of unknown must fail")
	}
}

func TestManagerPTP(t *testing.T) {
	m := NewManager(nil)
	if err := m.SetPTP([]PTPConfig{{Interface: "eth0", Mode: "slave"}}); err != nil {
		t.Fatal(err)
	}
	if len(m.PTP()) != 1 {
		t.Fatalf("ptp: %d", len(m.PTP()))
	}
}

func TestManagerServer(t *testing.T) {
	m := NewManager(nil)
	if err := m.SetServer(ServerConfig{Stratum: 3}); err != nil {
		t.Fatal(err)
	}
	if m.Server().Stratum != 3 {
		t.Fatal("server not stored")
	}
}

func TestManagerEvaluateTrust(t *testing.T) {
	m := NewManager(nil)
	if err := m.SetSource(Source{Name: "ntp1", Kind: SourceNTP, Address: "10.0.0.1", MinStratum: 3, MaxDriftMs: 100, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetSource(Source{Name: "ntp2", Kind: SourceNTP, Address: "10.0.0.2", MinStratum: 3, MaxDriftMs: 100, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// No status yet: no trust.
	dec := m.EvaluateTrust()
	if dec.Trusted {
		t.Fatal("no status must not be trusted")
	}
	// ntp1 reachable, ntp2 unreachable
	m.SetStatus(Status{Name: "ntp1", Reachable: true, Stratum: 2, DriftMs: 5})
	m.SetStatus(Status{Name: "ntp2", Reachable: false, Stratum: 2, DriftMs: 5})
	dec = m.EvaluateTrust()
	if !dec.Trusted {
		t.Fatalf("expected trusted, got %+v", dec)
	}
	if dec.BestSource != "ntp1" {
		t.Fatalf("best: %q", dec.BestSource)
	}
	if len(dec.FailingSource) != 1 || dec.FailingSource[0] != "ntp2" {
		t.Fatalf("failing: %v", dec.FailingSource)
	}
	// ntp1 now too high stratum
	m.SetStatus(Status{Name: "ntp1", Reachable: true, Stratum: 5, DriftMs: 5})
	dec = m.EvaluateTrust()
	if dec.Trusted {
		t.Fatal("high stratum must fail")
	}
	// ntp1 too much drift
	m.SetStatus(Status{Name: "ntp1", Reachable: true, Stratum: 2, DriftMs: 500})
	dec = m.EvaluateTrust()
	if dec.Trusted {
		t.Fatal("excessive drift must fail")
	}
}
