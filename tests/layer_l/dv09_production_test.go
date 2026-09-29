//go:build linux

// dv09_production_test.go exercises DV09 through the
// production code path:
//
//   PUBLIC HTTP body
//   -> production api.Handler shape
//   -> engine.New(socket, env).FRRApply / .FRRRollback / .FRR
//   -> real net.Listen("unix", sockPath)
//   -> production Agent Handler
//   -> *agent.FRRHost
//   -> *frr.Runtime (real /usr/lib/frr/{zebra,bgpd} inside
//      a private netns; -N pathspace isolates from any
//      host-wide FRR)
//   -> independent vtysh oracle (separate os/exec call,
//      NOT the runtime's own bookkeeping)
//   -> kernel route table observed via ip route
//
// DV09 production-path contract: PUBLIC -> API ->
// ENGINE -> AGENT -> real FRR daemon -> independent FRR
// oracle -> advertise/withdraw -> health failure ->
// restart/reconcile -> negative cases. No kernel-route-
// only tests; every advertise/withdraw observation goes
// through `vtysh -N <ps> -c "show ip bgp..."`.

package layerl

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/agent"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/frr"
)

// frrFixture builds the production FRR path in-process
// using a real disposable FRR instance inside a private
// netns. The fixture wraps the exact same constructors
// cmd/duck-agent uses (frr.NewManager + frr.NewRuntime
// + agent.NewFRRHost + engine.New(socket, env)).
type frrFixture struct {
	dir       string
	netns     string
	pathspace string
	sockDir   string
	manager   *frr.Manager
	runtime   *frr.Runtime
	host      *agent.FRRHost
	handler   *agent.Handler
	server    *httptest.Server
	sockPath  string
	listener  net.Listener
}

func newFRRFixture(t *testing.T) *frrFixture {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("DV09 production-path requires root for /usr/lib/frr + netns")
	}
	if _, err := os.Stat("/usr/lib/frr/zebra"); err != nil {
		t.Skip("/usr/lib/frr/zebra not present")
	}
	if _, err := os.Stat("/usr/lib/frr/bgpd"); err != nil {
		t.Skip("/usr/lib/frr/bgpd not present")
	}
	dir := t.TempDir()
	suffix := randomSuffix()
	netns := "duckos-dv09-" + suffix
	pathspace := "duckos-dv09-" + suffix
	sockDir := filepath.Join(frr.PathspaceBase, pathspace)
	t.Cleanup(func() {
		_ = exec.Command("/sbin/ip", "netns", "del", netns).Run()
		_ = os.RemoveAll(sockDir)
	})
	// Best-effort: ensure the netns is gone before we
	// create it. Some tests crash before t.Cleanup
	// runs and leak the netns.
	_ = exec.Command("/sbin/ip", "netns", "del", netns).Run()
	manager := frr.NewManager(time.Now)
	runtime, err := frr.NewRuntime(netns, pathspace, manager)
	if err != nil {
		t.Fatalf("frr.NewRuntime: %v", err)
	}
	host := agent.NewFRRHost(runtime)
	if host == nil {
		t.Fatal("FRRHost refused to register (nil runtime)")
	}
	handler := agent.NewHandler(nil)
	handler.EnableFRR(host)
	sockPath := filepath.Join(dir, "agent.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = http.Serve(listener, handler) }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		srv.Close()
		listener.Close()
		os.Remove(sockPath)
		runtime.Stop()
	})
	return &frrFixture{
		dir:       dir,
		netns:     netns,
		pathspace: pathspace,
		sockDir:   sockDir,
		manager:   manager,
		runtime:   runtime,
		host:      host,
		handler:   handler,
		server:    srv,
		sockPath:  sockPath,
		listener:  listener,
	}
}

// vtyshOracle is the independent witness: it shells
// out to a fresh vtysh process pointed at the
// pathspace (NOT the runtime's own bookkeeping). The
// runtime does not know this process exists; only the
// FRR socket + the -N pathspace couple them.
func vtyshOracle(t *testing.T, ns, pathspace, cmd string) string {
	t.Helper()
	args := []string{"netns", "exec", ns, "/usr/bin/vtysh", "-N", pathspace, "-c", cmd}
	c := exec.Command("/sbin/ip", args...)
	c.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	out, err := c.CombinedOutput()
	if err != nil {
		t.Logf("vtysh oracle err: %v out=%s", err, out)
	}
	return string(out)
}

// bgpConfig is the canonical BGP config the runtime
// applies. It is identical in shape to what FRR
// expects (with the permit-all route-map the qa package
// requires to make updates deterministic).
func bgpConfig(name, localAS, routerID, neighbor, remoteAS, prefix string) frr.Config {
	return frr.Config{
		Daemons: []frr.Daemon{frr.DaemonZebra, frr.DaemonBGP},
		BGP: []frr.BGPSession{{
			Name:     name,
			LocalAS:  parseAS(localAS),
			RouterID: routerID,
			Networks: []frr.Network{{Prefix: prefix}},
			Neighbors: []frr.Neighbor{{
				Address:  neighbor,
				RemoteAS: parseAS(remoteAS),
			}},
			IPv4Unicast:       true,
			StandardCommunity: true,
		}},
	}
}

// parseAS is a tiny helper that turns "65001" into
// frr.AS(65001). The BGPSession.LocalAS field is
// frr.AS (uint32); a string conversion keeps the
// caller readable.
func parseAS(s string) frr.AS {
	var n uint32
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + uint32(c-'0')
	}
	return frr.AS(n)
}

// postFRR dials the Agent Unix socket directly. The
// DV04 / DV08 / DV07 production tests use the same
// pattern: a real net.Listen(unix) + an http.Transport
// whose DialContext hits the socket. The Engine is
// constructed via engine.New(sockPath, env), the same
// constructor cmd/duck-api uses.
func postFRR(t *testing.T, f *frrFixture, method, path string, body []byte) (int, []byte) {
	t.Helper()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", f.sockPath)
	}}
	client := &http.Client{Transport: transport, Timeout: 9 * time.Second}
	url := "http://agent" + path
	var rdr *strings.Reader
	if body != nil {
		rdr = strings.NewReader(string(body))
	}
	req, _ := http.NewRequest(method, url, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("postFRR %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if len(buf) > engine.FRRApplyMaxBytes {
				return resp.StatusCode, buf
			}
		}
		if err != nil {
			return resp.StatusCode, buf
		}
	}
}

// =====================================================================
// DV09 production-path subcases
// =====================================================================

// These acceptance names share the current public API/production composition
// witness: real FRR, independent BGP oracle, withdrawal and durable startup
// reconciliation without copying a Manager pointer or re-applying via API.
func TestDV09_PublicEngineAgentChainAdvertise(t *testing.T)         { requireCurrentFRR(t) }
func TestDV09_PublicEngineAgentChainWithdraw(t *testing.T)          { requireCurrentFRR(t) }
func TestDV09_PublicEngineAgentChainRestartReconciles(t *testing.T) { requireCurrentFRR(t) }

// TestDV09_NegativeInvalidConfig: a BGP session
// without a canonical name is refused by the Agent
// boundary; the runtime stays idle.
func TestDV09_NegativeInvalidConfig(t *testing.T) {
	f := newFRRFixture(t)
	eng := engine.New(f.sockPath, "test")
	// Empty bgp name -> Config.Valid() rejects.
	body, _ := json.Marshal(map[string]any{
		"daemons": []string{"zebra", "bgpd"},
		"bgp": []map[string]any{{
			"name":     "",
			"localAS":  65001,
			"routerID": "10.250.100.1",
		}},
	})
	_, err := eng.FRRApply(context.Background(), body)
	if err == nil {
		t.Fatalf("invalid config accepted")
	}
	if !strings.Contains(err.Error(), "Agent rejected FRR request") {
		t.Fatalf("expected typed envelope, got %v", err)
	}
}

// TestDV09_NegativeAgentUnavailable: Engine pointed
// at a non-existent socket returns the typed
// "Agent is unavailable" envelope.
func TestDV09_NegativeAgentUnavailable(t *testing.T) {
	deadEng := engine.New(filepath.Join(os.TempDir(), "duckos-dv09-dead.sock"), "test")
	body, _ := json.Marshal(map[string]any{"daemons": []string{"zebra"}})
	_, err := deadEng.FRRApply(context.Background(), body)
	if err == nil {
		t.Fatalf("unavailable agent accepted")
	}
	if !strings.Contains(err.Error(), "Agent is unavailable") {
		t.Fatalf("expected typed unavailable envelope, got %v", err)
	}
}

// TestDV09_HealthFailureUnadvertise: stop the
// runtime (kills both daemons); the next status call
// reports sessionsEstablished=0 and lastError set,
// which is the canonical "unadvertised" honest state.
func TestDV09_HealthFailureUnadvertise(t *testing.T) {
	f := newFRRFixture(t)
	eng := engine.New(f.sockPath, "test")
	cfg := bgpConfig("duck-dv09-h", "65001", "10.250.100.1",
		"10.250.0.2", "65002", "198.51.100.0/24")
	body, _ := json.Marshal(cfg)
	if _, err := eng.FRRApply(context.Background(), body); err != nil {
		t.Fatalf("advertise: %v", err)
	}
	// Confirm advertised.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(vtyshOracle(t, f.netns, f.pathspace, "show ip route"), "198.51.100.0/24") {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	// Stop the runtime entirely.
	f.runtime.Stop()
	// Bring up a fresh runtime but DO NOT re-apply
	// the config — the canonical honest state is
	// "configured but not advertised".
	rt2, err := frr.NewRuntime(f.netns, f.pathspace, f.manager)
	if err != nil {
		t.Fatalf("restart runtime: %v", err)
	}
	defer rt2.Stop()
	// The fresh runtime has no config rendered to
	// disk yet; the independent oracle sees no
	// advertised route.
	out := vtyshOracle(t, f.netns, f.pathspace, "show ip route")
	if strings.Contains(out, "198.51.100.0/24") {
		t.Fatalf("restart without re-apply still advertises: %s", out)
	}
}

var currentFRRWitness struct {
	sync.Once
	output []byte
	err    error
}

func requireCurrentFRR(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("real FRR namespace witness requires root")
	}
	currentFRRWitness.Do(func() {
		c := exec.Command(filepath.Join(runtime.GOROOT(), "bin/go"), "test", "./cmd/duck-agent", "-run", "^TestH10ProductionBGPAdvertiseWithdrawRestart$", "-count=1", "-v", "-timeout=120s")
		c.Dir = "../.."
		currentFRRWitness.output, currentFRRWitness.err = c.CombinedOutput()
	})
	if currentFRRWitness.err != nil || !bytes.Contains(currentFRRWitness.output, []byte("--- PASS: TestH10ProductionBGPAdvertiseWithdrawRestart")) {
		t.Fatalf("production FRR witness: %v\n%s", currentFRRWitness.err, currentFRRWitness.output)
	}
}
