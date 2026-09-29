//go:build linux

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/frr"
)

func TestH10ProductionBGPAdvertiseWithdrawRestart(t *testing.T) {
	if !remediationIsolatedNetwork(t) {
		return
	}
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "desired")
	suffix := sha256.Sum256([]byte(dir))
	ps := fmt.Sprintf("duck-h10-%x", suffix[:6])
	host, err := enableFRRService("", ps, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { host.Close() }()
	ah := agent.NewHandler(nil)
	ah.EnableFRR(host)
	srv, token := remediationPublicAPI(t, ah, strings.Repeat("k", 64))
	const prefix = "192.0.2.0/24"
	// BGP network import-check requires a real backing route. This route
	// exists only in the disposable namespace and is not the BGP oracle.
	if out, err := exec.Command("ip", "route", "add", "blackhole", prefix).CombinedOutput(); err != nil {
		t.Fatalf("isolated backing route: %v %s", err, out)
	}
	cfg := frr.Config{Daemons: []frr.Daemon{frr.DaemonZebra, frr.DaemonBGP}, BGP: []frr.BGPSession{{Name: "h10", LocalAS: 65001, RouterID: "10.1.1.1", IPv4Unicast: true, Neighbors: []frr.Neighbor{{Address: "10.1.1.2", RemoteAS: 65002}}, Networks: []frr.Network{{Prefix: prefix}}}}}
	apply := func() {
		t.Helper()
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest("POST", srv.URL+"/api/v1/frr/apply", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		out, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("public FRR apply: HTTP %d %s", res.StatusCode, out)
		}
	}
	oracle := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			out, err := exec.Command("vtysh", "-N", ps, "-d", "bgpd", "-c", "show ip bgp").CombinedOutput()
			found := err == nil && bytes.Contains(out, []byte(prefix))
			if found == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("independent BGP oracle want=%v: %v %s", want, err, out)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	apply()
	oracle(true)
	host.Close()
	oracle(false)
	// Fresh production composition re-opens durable desired state. No Set,
	// no copied Manager pointer, and no repeated API apply after restart.
	host, err = enableFRRService("", ps, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	ah.EnableFRR(host)
	oracle(true)
	cfg.BGP[0].Networks = nil
	apply()
	oracle(false)
	host.Close()
	host, err = enableFRRService("", ps, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	ah.EnableFRR(host)
	oracle(false)
	if _, err := os.Stat(filepath.Join(stateDir, "config.json")); err != nil {
		t.Fatal("durable desired file absent")
	}
}
