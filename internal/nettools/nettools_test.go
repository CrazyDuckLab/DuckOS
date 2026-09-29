// nettools/nettools_test.go pins the V1
// contextual network-tools surface.
package nettools

import (
	"testing"
	"time"
)

func TestNetworkContextValid(t *testing.T) {
	if err := (NetworkContext{}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (NetworkContext{Table: -1}).Valid(); err == nil {
		t.Fatal("negative table must fail")
	}
	if err := (NetworkContext{Mark: -1}).Valid(); err == nil {
		t.Fatal("negative mark must fail")
	}
	if err := (NetworkContext{Timeout: -time.Second}).Valid(); err == nil {
		t.Fatal("negative timeout must fail")
	}
}

func TestPingRequestValid(t *testing.T) {
	good := PingRequest{Target: "1.1.1.1", Count: 4, Payload: 56, Context: NetworkContext{VRF: "mgmt"}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := PingRequest{Target: "1.1.1.1", Count: 999}
	if err := bad.Valid(); err == nil {
		t.Fatal("count too high must fail")
	}
	bad2 := PingRequest{Target: "!!bad"}
	if err := bad2.Valid(); err == nil {
		t.Fatal("non-canonical target must fail")
	}
}

func TestTracerouteRequestValid(t *testing.T) {
	good := TracerouteRequest{Target: "8.8.8.8", MaxHops: 30}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (TracerouteRequest{Target: "8.8.8.8", MaxHops: 999}).Valid(); err == nil {
		t.Fatal("max hops too high must fail")
	}
}

func TestTCPProbeValid(t *testing.T) {
	good := TCPProbeRequest{Host: "example.com", Port: 443}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (TCPProbeRequest{Host: "example.com", Port: 99999}).Valid(); err == nil {
		t.Fatal("port out of range must fail")
	}
	if err := (TCPProbeRequest{Host: "!!bad", Port: 80}).Valid(); err == nil {
		t.Fatal("non-canonical host must fail")
	}
}

func TestDNSQueryValid(t *testing.T) {
	if err := (DNSQueryRequest{Name: "example.com", Type: "A"}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (DNSQueryRequest{Name: "example.com", Type: "garbage"}).Valid(); err == nil {
		t.Fatal("unknown type must fail")
	}
	if err := (DNSQueryRequest{Name: "", Type: "A"}).Valid(); err == nil {
		t.Fatal("missing name must fail")
	}
}

func TestRouteLookupValid(t *testing.T) {
	if err := (RouteLookupRequest{Dst: "1.1.1.1", VRF: "mgmt", Mark: 0x1}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (RouteLookupRequest{Dst: "", Mark: -1}).Valid(); err == nil {
		t.Fatal("missing dst must fail")
	}
}

func TestLiveCaptureValid(t *testing.T) {
	if err := (LiveCaptureRequest{Interface: "eth0", Duration: 5 * time.Second}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (LiveCaptureRequest{Interface: "eth0", Duration: 10 * time.Minute}).Valid(); err == nil {
		t.Fatal("duration too long must fail")
	}
	if err := (LiveCaptureRequest{Interface: "eth0", Duration: time.Second, Bytes: 1 << 30}).Valid(); err == nil {
		t.Fatal("bytes too high must fail")
	}
	if err := (LiveCaptureRequest{Duration: time.Second}).Valid(); err == nil {
		t.Fatal("missing interface must fail")
	}
}
