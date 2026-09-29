package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

type linuxObjectsFakeRunner struct {
	run  map[string][]byte
	file map[string][]byte
	glob []string
}

func (r linuxObjectsFakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	if data, ok := r.run[key]; ok {
		return data, nil
	}
	return nil, errors.New("missing command")
}
func (r linuxObjectsFakeRunner) ReadFile(path string) ([]byte, error) {
	if data, ok := r.file[path]; ok {
		return data, nil
	}
	return nil, errors.New("missing file")
}
func (r linuxObjectsFakeRunner) Glob(string) ([]string, error) { return r.glob, nil }

func TestLinuxObjectsHostListsRealSourceBackedKinds(t *testing.T) {
	host := NewLinuxObjectsHost(linuxObjectsFakeRunner{
		run: map[string][]byte{
			"vtysh -c show running-config": []byte("router bgp 65001\n neighbor 192.0.2.2 remote-as 65002\n route-map DUCK permit 10\n password supersecret\n"),
			"nft -j list ruleset":          []byte(`{"nftables":[{"rule":{"family":"inet","table":"duck","chain":"input","expr":[]}},{"set":{"family":"inet","table":"duck","name":"staff"}}]}`),
		},
		file: map[string][]byte{
			"/var/lib/misc/dnsmasq.leases": []byte("1 aa:bb:cc:dd:ee:ff 10.0.0.42 phone *\n"),
			"/etc/dnsmasq.conf":            []byte("dhcp-range=10.0.0.50,10.0.0.99,12h\naddress=/duck.lan/10.0.0.1\n"),
			"/etc/frr/frr.conf":            []byte("router bgp 65001\n neighbor 192.0.2.2 remote-as 65002\n route-map DUCK permit 10\n password supersecret\n"),
		},
	})
	for _, kind := range []string{"network.firewall.rule", "network.firewall.set", "network.lease", "dns.zone", "dhcp.subnet", "bgp.peer", "route.map"} {
		req := httptest.NewRequest(http.MethodGet, LinuxObjectsPathPrefix+kind, nil)
		rec := httptest.NewRecorder()
		host.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s status %d body %s", kind, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "supersecret") {
			t.Fatalf("%s leaked secret line: %s", kind, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"complete":true`) {
			t.Fatalf("%s did not produce a complete source-backed envelope: %s", kind, rec.Body.String())
		}
	}
}

func TestLinuxObjectsNFTObservationTruthfulness(t *testing.T) {
	for _, kind := range []string{"network.firewall.rule", "network.firewall.set"} {
		for _, tc := range []struct {
			name, data string
			status     int
		}{
			{"empty", `{"nftables":[]}`, 200},
			{"malformed", `{"nftables":[`, 503},
			{"missing", `{}`, 503},
			{"oversized", `{"nftables":[],"padding":"` + strings.Repeat("x", linuxObjectsMaxBytes) + `"}`, 503},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				h := NewLinuxObjectsHost(linuxObjectsFakeRunner{run: map[string][]byte{"nft -j list ruleset": []byte(tc.data)}})
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("GET", LinuxObjectsPathPrefix+kind, nil))
				if w.Code != tc.status {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
				if tc.status == 200 {
					var col object.Collection
					if err := json.Unmarshal(w.Body.Bytes(), &col); err != nil {
						t.Fatal(err)
					}
					if !col.Complete || len(col.Items) != 0 {
						t.Fatalf("not an empty complete observation: %+v", col)
					}
				} else if !strings.Contains(w.Body.String(), "engine_unavailable") {
					t.Fatal("missing typed failure")
				}
			})
		}
	}
}

func TestLinuxObjectsFRRFileCannotImpersonateLiveDaemon(t *testing.T) {
	h := NewLinuxObjectsHost(linuxObjectsFakeRunner{file: map[string][]byte{"/etc/frr/frr.conf": []byte("router bgp 65001\n neighbor 192.0.2.2 remote-as 65002\n")}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", LinuxObjectsPathPrefix+"bgp.peer", nil))
	if w.Code != 503 {
		t.Fatalf("offline daemon reported live state: %d %s", w.Code, w.Body.String())
	}
}

func TestLinuxObjectsHostUnavailableIsTyped(t *testing.T) {
	host := NewLinuxObjectsHost(linuxObjectsFakeRunner{})
	req := httptest.NewRequest(http.MethodGet, LinuxObjectsPathPrefix+"network.firewall.rule", nil)
	rec := httptest.NewRecorder()
	host.ServeHTTP(rec, req)
	if rec.Code != 503 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "engine_unavailable") {
		t.Fatalf("missing typed unavailable: %s", rec.Body.String())
	}
}
