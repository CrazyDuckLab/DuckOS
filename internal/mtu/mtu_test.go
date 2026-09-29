// mtu/mtu_test.go pins the V1 MTU/PMTUD model.
package mtu

import "testing"

func TestLinkMTUValid(t *testing.T) {
	good := LinkMTU{Link: "eth0", MTU: 1500}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	jumbo := LinkMTU{Link: "eth0", MTU: 9000}
	if err := jumbo.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := LinkMTU{Link: "eth0", MTU: 500}
	if err := bad.Valid(); err == nil {
		t.Fatal("MTU below IPv6 minimum must fail")
	}
}

func TestClampMSS(t *testing.T) {
	if got := ClampMSS(1500, false); got != 1460 {
		t.Fatalf("ClampMSS v4: %d", got)
	}
	if got := ClampMSS(1500, true); got != 1440 {
		t.Fatalf("ClampMSS v6: %d", got)
	}
}

func TestClampMSSForLink(t *testing.T) {
	m := NewManager()
	_ = m.SetLink(LinkMTU{Link: "pppoe-wan", MTU: 1492, Overhead: 8})
	mss, err := m.ClampMSSForLink("pppoe-wan", false)
	if err != nil {
		t.Fatal(err)
	}
	if mss != 1444 {
		t.Fatalf("pppoe MSS: %d", mss)
	}
	if _, err := m.ClampMSSForLink("missing", false); err == nil {
		t.Fatal("missing link must fail")
	}
}

func TestPMTUReport(t *testing.T) {
	good := PMTUReport{Destination: "8.8.8.8", MTU: 1500, Reachable: true}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	blackhole := PMTUReport{Destination: "8.8.8.8", MTU: 1280, Reachable: false, Blackhole: true, Reason: "DF fragments lost"}
	if err := blackhole.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestManagerReports(t *testing.T) {
	m := NewManager()
	_ = m.SetReport(PMTUReport{Destination: "8.8.8.8", MTU: 1500, Reachable: true})
	if got := len(m.Reports()); got != 1 {
		t.Fatalf("Reports count: %d", got)
	}
}
