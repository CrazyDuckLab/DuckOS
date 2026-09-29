package observe

import "testing"

func TestSpecialRoutesDoNotBecomeInventedOnLinkRoutes(t *testing.T) {
	for _, extra := range []string{`{"type":"blackhole","dst":"192.0.2.0/24"}`, `{"dst":"default","nexthops":[{"gateway":"192.0.2.1","dev":"test0"}]}`, `{"dst":"default","nhid":7}`} {
		data := []byte(`[{"dst":"198.51.100.0/24","dev":"test0"},` + extra + `]`)
		rows, err := parseRoutes(data, "ipv4")
		if err == nil || len(rows) != 1 || rows[0].Destination != "198.51.100.0/24" {
			t.Fatalf("special route must produce partial observation with issue: %+v %v", rows, err)
		}
	}
}
