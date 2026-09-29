package observe

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestResourceParsers(t *testing.T) {
	u, err := parseUptime([]byte("123.50 999.0\n"))
	if err != nil || u != 123.5 {
		t.Fatalf("uptime %v %v", u, err)
	}
	for _, s := range []string{"", "NaN", "+Inf", "-1", "broken"} {
		if _, err := parseUptime([]byte(s)); err == nil {
			t.Errorf("accepted uptime %q", s)
		}
	}
	total, idle, err := parseCPU([]byte("cpu 100 0 50 500 20 10 5 15 9999 9999\n"))
	if err != nil || total != 700 || idle != 520 {
		t.Fatalf("CPU %d %d %v", total, idle, err)
	}
	if p := cpuUsage(100, 50, 200, 125); p == nil || *p != 25 {
		t.Fatalf("CPU delta %v", p)
	}
	for _, v := range [][4]uint64{{1, 1, 1, 1}, {2, 1, 1, 0}, {100, 50, 200, 40}, {100, 50, 200, 200}} {
		if cpuUsage(v[0], v[1], v[2], v[3]) != nil {
			t.Errorf("invalid delta accepted %v", v)
		}
	}
	m, err := parseMemory([]byte("MemTotal: 2048 kB\nMemFree: 64 kB\nMemAvailable: 1024 kB\n"))
	if err != nil || m.TotalBytes != 2097152 || m.UsedBytes != 1048576 {
		t.Fatalf("memory %+v %v", m, err)
	}
	for _, s := range []string{"", "MemTotal: 1 kB\nMemAvailable: 2 kB", "MemTotal: 1 MB\nMemAvailable: 0 kB", "MemTotal: 18446744073709551615 kB\nMemAvailable: 0 kB"} {
		if _, e := parseMemory([]byte(s)); e == nil {
			t.Errorf("accepted memory %q", s)
		}
	}
	if _, e := parseLoad([]byte("0.1 0.2 0.3 1/20")); e != nil {
		t.Fatal(e)
	}
	if _, e := parseLoad([]byte("NaN 0 0")); e == nil {
		t.Fatal("accepted NaN load")
	}
}

func TestDNSAndRoutesPreserveIPv6(t *testing.T) {
	d, e := parseDNS([]byte("# TEST fixture\nnameserver 192.0.2.53\nnameserver 2001:db8::53\nsearch example.test branch.test # ignored\n"))
	if e != nil || len(d.Nameservers) != 2 || len(d.SearchDomains) != 2 {
		t.Fatalf("DNS %+v %v", d, e)
	}
	if _, e := parseDNS([]byte("nameserver not-an-ip")); e == nil {
		t.Fatal("invalid DNS accepted")
	}
	r, e := parseRoutes([]byte(`[{"dst":"default","gateway":"2001:db8::1","dev":"test0","protocol":4,"table":100,"metric":42}]`), "ipv6")
	if e != nil || len(r) != 1 || r[0].Family != "ipv6" || r[0].Table != "100" || r[0].Protocol != "4" {
		t.Fatalf("routes %+v %v", r, e)
	}
	if _, e := parseRoutes([]byte(`[{"protocol":{}}]`), "ipv4"); e == nil {
		t.Fatal("route object accepted")
	}
	if _, e := parseRoutes([]byte(`not json`), "ipv4"); e == nil {
		t.Fatal("bad route JSON accepted")
	}
}

func TestServicesAndNFTHonestState(t *testing.T) {
	var b strings.Builder
	for _, name := range ServiceNames {
		fmt.Fprintf(&b, "Id=%s\nLoadState=not-found\nActiveState=inactive\nSubState=dead\n\n", name)
	}
	s, e := parseServices([]byte(b.String()))
	if e != nil || len(s) != len(ServiceNames) || s[0].LoadState != "not-found" {
		t.Fatalf("services %+v %v", s, e)
	}
	if _, e := parseServices([]byte("Id=caller-selected.service\nLoadState=loaded\nActiveState=active\nSubState=running")); e == nil {
		t.Fatal("unlisted service accepted")
	}
	n, e := parseNFT([]byte(`{"nftables":[{"metainfo":{}},{"table":{}},{"chain":{}},{"rule":{"expr":[{"comment":"synthetic secret excluded"}]}}]}`))
	if e != nil || n.Tables != 1 || n.Chains != 1 || n.Rules != 1 || n.Status != "available" {
		t.Fatalf("nft %+v %v", n, e)
	}
	if _, e := parseNFT([]byte(`{}`)); e == nil {
		t.Fatal("missing ruleset accepted")
	}
	n, e = parseNFT([]byte(`{"nftables":[]}`))
	if e != nil || n.Rules != 0 || n.Status != "available" {
		t.Fatal("empty ruleset must be available, not secure")
	}
}

func TestReadAndOutputBounds(t *testing.T) {
	f := t.TempDir() + "/large"
	if e := os.WriteFile(f, make([]byte, maxFileBytes+1), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := readBounded(f); e == nil {
		t.Fatal("unbounded file accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := limitedOutput{limit: 3, cancel: cancel}
	if _, e := w.Write([]byte("abcd")); e == nil || ctx.Err() == nil {
		t.Fatal("output overflow did not cancel")
	}
}

func FuzzObservationParsers(f *testing.F) {
	f.Add([]byte("cpu 1 2 3 4 5 6 7 8\n"))
	f.Add([]byte(`{"nftables":[]}`))
	f.Add([]byte("nameserver 2001:db8::53"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxFileBytes {
			return
		}
		_, _ = parseUptime(data)
		_, _, _ = parseCPU(data)
		_, _ = parseLoad(data)
		_, _ = parseMemory(data)
		_, _ = parseDNS(data)
		_, _ = parseRoutes(data, "ipv6")
		_, _ = parseNFT(data)
		_, _ = parseServices(data)
	})
}
