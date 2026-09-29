// Package main is the Duck CLI entry point. It is a thin wrapper over the
// loopback management API and shares the same model, RBAC and audit
// surface as the GUI. Anything that requires a shell is documented but
// not the normal operator UX.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/pkg/brand"
	"github.com/CrazyDuckLab/DuckOS/pkg/buildinfo"
)

const cliUsage = `duck — manage DuckOS from the shell

Subcommands (each is a separate concern, all share the loopback API):
  health                  probe the loopback API health probe
  overview                print the canonical Engine overview
  ready                   check that the privileged Agent is ready
  network status          print the Agent's network service status
  network apply           submit a network candidate for Safe Apply
  network confirm         confirm a transaction after a successful window
  network rollback        roll back a transaction before confirmation
  client export <id>      export a managed client configuration
  identity login          perform a First-Owner password login
  owner reset             offline root-only Owner recovery (stop duck-api first)
  qos status              print the QoS topology, last apply and last error
  qos apply <file>        submit a QoS plan (JSON) for tc apply
  qos rollback            roll back the active QoS plan
  ups status              print the UPS topology, health and last probe
  ups probe               run an on-demand probe of every UPS device
  migrate analyse <file>  analyse a migration source (MikroTik/OpenWrt/UCI/Kerio/Linux interfaces) and print the per-line classification
  sites status            print the sites topology, listener/target and per-site health
  wifi status             print the Wi-Fi topology, host capability and per-AP health
  multiwan                print the multi-WAN plan and selections
  topology                print the live relationship graph
  doctor                  print the bounded diagnostic snapshot
  provenance              print the ownership + provenance record for a single object
  firstrun                print the first-run persistence status
  diagjobs                print the contextual diagnostic job list
  upgrade                 print the upgrade activation status
  backup                  print the backup catalog / restore plan

Use --help on any subcommand for arguments.
`

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, cliUsage) }
	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(2)
	}
	args := os.Args[1:]
	if err := dispatch(args); err != nil {
		fmt.Fprintf(os.Stderr, "duck: %v\n", err)
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	if len(args) == 0 {
		return errors.New("missing subcommand")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "-v", "--version", "version":
		fmt.Println(buildinfo.String())
		return nil
	case "help", "-h", "--help":
		fmt.Print(cliUsage)
		return nil
	case "health":
		return healthCmd(rest)
	case "overview":
		return overviewCmd(rest)
	case "ready":
		return readyCmd(rest)
	case "doctor":
		return doctorCmd(rest)
	case "topology":
		return topologyCmd(rest)
	case "multiwan":
		return multiwanCmd(rest)
	case "qos":
		return qosCmd(rest)
	case "ups":
		return upsCmd(rest)
	case "migrate":
		return migrateCmd(rest)
	case "sites":
		return sitesCmd(rest)
	case "wifi":
		return wifiCmd(rest)
	case "network":
		return networkCmd(rest)
	case "client":
		return clientCmd(rest)
	case "identity":
		return identityCmd(rest)
	case "owner":
		return ownerCmd(rest)
	case "wireguard":
		return wireguardCmd(rest)
	case "xray":
		return xrayCmd(rest)
	case "config":
		return configCmd(rest)
	case "provenance":
		return provenanceCmd(rest)
	case "firstrun":
		return firstrunCmd(rest)
	case "diagjobs":
		return diagjobCmd(rest)
	case "upgrade":
		return upgradeCmd(rest)
	case "backup":
		return backupCmd(rest)
	default:
		return fmt.Errorf("unknown subcommand %q", cmd)
	}
}

// CLI flags are shared across subcommands. Per-command flags live on a
// dedicated FlagSet so they can fail loudly without mutating the global
// flag set.

func newClient(endpoint, tokenPath string, tlsOnly bool) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// newAgentClient returns an HTTP client that talks to the
// privileged Agent over a Unix socket. The client refuses
// redirects and enforces a short timeout so a stalled
// Agent does not hang the CLI.
func newAgentClient(socketPath string) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		ResponseHeaderTimeout: 12 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          2,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type commonFlags struct {
	endpoint    string
	tokenFile   string
	insecure    bool
	output      string
	agentSocket string
}

func addCommonFlags(fs *flag.FlagSet) *commonFlags {
	cf := &commonFlags{}
	fs.StringVar(&cf.endpoint, "endpoint", "http://127.0.0.1:8080", "Loopback management endpoint")
	fs.StringVar(&cf.tokenFile, "token-file", defaultTokenFile(), "Path to the bearer token file")
	fs.BoolVar(&cf.insecure, "allow-insecure", false, "Allow plain HTTP loopback (only for development)")
	fs.StringVar(&cf.output, "output", "json", "Output format: json|table|text")
	fs.StringVar(&cf.agentSocket, "agent-socket", defaultAgentSocket(), "Path to the privileged Agent Unix socket (required for /agent/v1/* operations)")
	return cf
}

func defaultAgentSocket() string {
	if v := os.Getenv("DUCK_AGENT_SOCKET"); v != "" {
		return v
	}
	return "/run/duckos-agent/agent.sock"
}

func defaultTokenFile() string {
	if v := os.Getenv("DUCK_TOKEN_FILE"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".duck", "token")
}

func loadToken(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("token file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("token file is not a regular file")
	}
	if info.Mode().Perm()&0o037 != 0 {
		return "", errors.New("token file is world-accessible")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || len(token) > 128 {
		return "", errors.New("token length out of bounds")
	}
	return token, nil
}

type apiCall struct {
	method  string
	path    string
	body    io.Reader
	headers http.Header
}

func (c *cliState) call(ctx context.Context, call apiCall, out any) (int, error) {
	// CLI commands that target /agent/v1/* are private Agent IPC
	// routes, not public API routes. Forwarding them through
	// the public HTTP endpoint silently produces 404s that look
	// like transient errors. Fail closed:
	// require --agent-socket and route through the Unix
	// socket transport.
	if strings.HasPrefix(call.path, "/agent/") {
		if c.agentSocket == "" {
			return 0, errors.New("agent-socket required for /agent/v1/* operations; pass --agent-socket or set DUCK_AGENT_SOCKET")
		}
		return c.callAgent(ctx, call, out)
	}
	req, err := http.NewRequestWithContext(ctx, call.method, c.endpoint+call.path, call.body)
	if err != nil {
		return 0, err
	}
	for k, v := range call.headers {
		req.Header[k] = v
	}
	if call.body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return resp.StatusCode, err
	}
	if len(data) > 4<<20 {
		return resp.StatusCode, errors.New("API response exceeds size limit")
	}
	if out == nil {
		return resp.StatusCode, nil
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

// callAgent dispatches the supplied apiCall over the
// privileged Agent Unix socket. The HTTP client is built
// once per CLI invocation; per-call latency is dominated by
// the dial, not the client setup.
func (c *cliState) callAgent(ctx context.Context, call apiCall, out any) (int, error) {
	if c.agentClient == nil {
		c.agentClient = newAgentClient(c.agentSocket)
	}
	url := "http://agent" + call.path
	req, err := http.NewRequestWithContext(ctx, call.method, url, call.body)
	if err != nil {
		return 0, err
	}
	for k, v := range call.headers {
		req.Header[k] = v
	}
	if call.body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.agentClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return resp.StatusCode, err
	}
	if len(data) > 4<<20 {
		return resp.StatusCode, errors.New("agent response exceeds size limit")
	}
	if resp.StatusCode/100 != 2 {
		return resp.StatusCode, fmt.Errorf("agent: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return resp.StatusCode, nil
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
			return resp.StatusCode, fmt.Errorf("agent: unexpected content-type %q", ct)
		}
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("agent: decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

type cliState struct {
	endpoint    string
	token       string
	client      *http.Client
	agentClient *http.Client
	agentSocket string
	cf          *commonFlags
}

func newState(cf *commonFlags) (*cliState, error) {
	u, err := url.Parse(cf.endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("endpoint must be an HTTP(S) origin without credentials, path, query or fragment")
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if !cf.insecure || ip == nil || !ip.IsLoopback() {
			return nil, errors.New("plain HTTP requires --allow-insecure and a literal loopback address")
		}
	}
	if cf.output != "" && cf.output != "json" && cf.output != "text" && cf.output != "table" {
		return nil, errors.New("unsupported output format")
	}
	if !cf.insecure && !strings.HasPrefix(cf.endpoint, "https://") {
		return nil, errors.New("plain HTTP is only allowed with --allow-insecure on loopback")
	}
	if strings.HasPrefix(cf.endpoint, "http://") && !cf.insecure {
		return nil, errors.New("plain HTTP requires --allow-insecure on loopback")
	}
	if !strings.HasPrefix(cf.endpoint, "http://") && !strings.HasPrefix(cf.endpoint, "https://") {
		return nil, errors.New("endpoint must use http:// or https://")
	}
	token, err := loadToken(cf.tokenFile)
	if err != nil {
		return nil, err
	}
	state := &cliState{
		endpoint:    strings.TrimSuffix(cf.endpoint, "/"),
		token:       token,
		client:      newClient(cf.endpoint, cf.tokenFile, !cf.insecure),
		agentSocket: cf.agentSocket,
		cf:          cf,
	}
	if cf.agentSocket != "" {
		// Probe the socket lazily: only build the client when
		// the caller actually routes through /agent/.
		state.agentClient = newAgentClient(cf.agentSocket)
	}
	return state, nil
}

func healthCmd(args []string) error {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/health"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("health probe returned %d", code)
	}
	return render(cf.output, out)
}

func overviewCmd(args []string) error {
	fs := flag.NewFlagSet("overview", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/overview"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("overview returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

func readyCmd(args []string) error {
	fs := flag.NewFlagSet("ready", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/ready"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("ready returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

// doctorCmd fetches the structured diagnostic snapshot. The CLI is
// a thin wrapper: the API enforces auth and the Agent enforces the
// redacted, bounded contract. The CLI just prints.
func doctorCmd(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	subsystemOnly := fs.Bool("subsystems", false, "Print only the per-subsystem health table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/doctor"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("doctor returned %d: %s", code, errorMessage(out))
	}
	if *subsystemOnly {
		subs, _ := out["subsystems"].([]any)
		fmt.Printf("%-20s %-12s %s\n", "SUBSYSTEM", "STATUS", "DETAIL")
		for _, s := range subs {
			m, _ := s.(map[string]any)
			if m == nil {
				continue
			}
			name, _ := m["name"].(string)
			status, _ := m["status"].(string)
			detail, _ := m["detail"].(string)
			fmt.Printf("%-20s %-12s %s\n", name, status, detail)
		}
		return nil
	}
	return render(cf.output, out)
}

// topologyCmd fetches the live relationship graph and prints it.
// The CLI is a thin wrapper: the API enforces auth and the
// Engine enforces the bounded, validated contract. The CLI just
// formats the result for a human operator.
func topologyCmd(args []string) error {
	fs := flag.NewFlagSet("topology", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	nodesOnly := fs.Bool("nodes", false, "Print only the node list, not the edges")
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/topology"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("topology returned %d: %s", code, errorMessage(out))
	}
	if *nodesOnly {
		nodes, _ := out["nodes"].([]any)
		fmt.Printf("%-32s %-12s %s\n", "ID", "KIND", "LABEL")
		for _, n := range nodes {
			m, _ := n.(map[string]any)
			if m == nil {
				continue
			}
			id, _ := m["id"].(string)
			kind, _ := m["kind"].(string)
			label, _ := m["label"].(string)
			fmt.Printf("%-32s %-12s %s\n", id, kind, label)
		}
		return nil
	}
	return render(cf.output, out)
}

// multiwanCmd fetches the multi-WAN plan, the most recent
// per-group selection and the per-member health. The CLI is a
// thin wrapper: the API enforces auth and the Engine enforces
// the bounded, validated contract. The CLI just formats the
// result for a human operator.
func multiwanCmd(args []string) error {
	fs := flag.NewFlagSet("multiwan", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	selectionsOnly := fs.Bool("selections", false, "Print only the per-group selection table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/multiwan"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("multiwan returned %d: %s", code, errorMessage(out))
	}
	if *selectionsOnly {
		selections, _ := out["selections"].(map[string]any)
		fmt.Printf("%-16s %-12s %-16s %-10s %s\n", "GROUP", "MEMBER", "INTERFACE", "HEALTH", "REASON")
		for gid, s := range selections {
			m, _ := s.(map[string]any)
			if m == nil {
				continue
			}
			mid, _ := m["memberId"].(string)
			iface, _ := m["interface"].(string)
			health, _ := m["health"].(string)
			reason, _ := m["reason"].(string)
			fmt.Printf("%-16s %-12s %-16s %-10s %s\n", gid, mid, iface, health, reason)
		}
		return nil
	}
	return render(cf.output, out)
}

func networkCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("network: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "status":
		return networkStatus(rest)
	case "apply":
		return networkApply(rest)
	case "confirm":
		return networkConfirm(rest)
	case "rollback":
		return networkRollback(rest)
	default:
		return fmt.Errorf("network: unknown operation %q", op)
	}
}

func networkStatus(args []string) error {
	fs := flag.NewFlagSet("network status", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	// CLI/Engine parity (H10): the CLI routes through the
	// public /api/v1/config/status endpoint so auth, RBAC,
	// revision conflict and Safe Apply semantics stay
	// identical to the dashboard. Direct /agent/v1 calls
	// are reserved for recovery/internal operations.
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/config/status"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("network status returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

func networkApply(args []string) error {
	fs := flag.NewFlagSet("network apply", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	file := fs.String("candidate", "", "Path to a candidate network JSON file")
	requestID := fs.String("request-id", "", "Stable request id (32 hex chars)")
	expected := fs.String("expected-revision", "", "Expected current revision id")
	actor := fs.String("actor", "duck-cli", "Operator identity")
	reason := fs.String("reason", "duck-cli apply", "Human-readable reason")
	confirm := fs.Int("confirm-seconds", 300, "Confirmation window in seconds (120..900)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" || *requestID == "" || *expected == "" {
		return errors.New("network apply: --candidate, --request-id, and --expected-revision are required")
	}
	if len(*requestID) != 32 {
		return errors.New("network apply: --request-id must be exactly 32 hex characters")
	}
	if *confirm < 120 || *confirm > 900 {
		return errors.New("network apply: --confirm-seconds must be in [120, 900]")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("read candidate: %w", err)
	}
	var anyMap map[string]any
	if err := json.Unmarshal(raw, &anyMap); err != nil {
		return fmt.Errorf("candidate is not valid JSON: %w", err)
	}
	body, _ := json.Marshal(map[string]any{
		"requestId":     *requestID,
		"expected":      *expected,
		"candidate":     json.RawMessage(raw),
		"actor":         *actor,
		"reason":        *reason,
		"windowSeconds": *confirm,
	})
	state, err := newState(cf)
	if err != nil {
		return err
	}
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/json")
	// CLI/Engine parity (H10): route through the public
	// /api/v1/config/apply so auth, RBAC, revision
	// conflict and Safe Apply semantics match the
	// dashboard. Direct /agent/v1 calls are reserved for
	// recovery/internal operations.
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/config/apply", body: bytes.NewReader(body), headers: hdr}, &out)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return fmt.Errorf("network apply returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

func networkConfirm(args []string) error {
	fs := flag.NewFlagSet("network confirm", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	id := fs.String("transaction-id", "", "Transaction id to confirm")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("network confirm: --transaction-id is required")
	}
	body, _ := json.Marshal(map[string]any{"transactionId": *id})
	return callNetworkControl(cf, "confirm", body)
}

func networkRollback(args []string) error {
	fs := flag.NewFlagSet("network rollback", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	id := fs.String("transaction-id", "", "Transaction id to roll back")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("network rollback: --transaction-id is required")
	}
	body, _ := json.Marshal(map[string]any{"transactionId": *id})
	return callNetworkControl(cf, "rollback", body)
}

func callNetworkControl(cf *commonFlags, op string, body []byte) error {
	state, err := newState(cf)
	if err != nil {
		return err
	}
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/json")
	// CLI/Engine parity (H10): route confirm/rollback
	// through the public /api/v1/config/{confirm,rollback}
	// endpoints so auth, RBAC, revision and Safe Apply
	// semantics match the dashboard. Direct /agent/v1
	// calls are reserved for recovery/internal operations.
	var env struct {
		TransactionID string `json:"transactionId"`
	}
	_ = json.Unmarshal(body, &env)
	path := "/api/v1/config/" + op + "/" + env.TransactionID
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: path, headers: hdr}, &out)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return fmt.Errorf("network %s returned %d: %s", op, code, errorMessage(out))
	}
	return render(cf.output, out)
}

func clientCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("client: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "export":
		return clientExport(rest)
	default:
		return fmt.Errorf("client: unknown operation %q", op)
	}
}

func clientExport(args []string) error {
	fs := flag.NewFlagSet("client export", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	id := fs.String("client-id", "", "Client id to export")
	format := fs.String("format", "uri", "Export format: uri|qr|config|file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("client export: --client-id is required")
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/clients/export?clientId=" + urlQueryEscape(*id) + "&format=" + urlQueryEscape(*format)}, &out)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return fmt.Errorf("client export returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

func identityCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("identity: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "login":
		return identityLogin(rest)
	case "enroll":
		return identityEnroll(rest)
	case "change-password":
		return identityChangePassword(rest)
	case "users":
		return identityUsers(rest)
	case "audit":
		return identityAudit(rest)
	case "sessions":
		return identitySessions(rest)
	case "revoke":
		return identityRevoke(rest)
	default:
		return fmt.Errorf("identity: unknown operation %q", op)
	}
}

func identityLogin(args []string) error {
	fs := flag.NewFlagSet("identity login", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	username := fs.String("username", "", "Username")
	password := fs.String("password", "", "Password (or use --password-file)")
	fromFile := fs.String("password-file", "", "Path to a file containing the password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" {
		return errors.New("identity login: --username is required")
	}
	pw := *password
	if *fromFile != "" {
		data, err := os.ReadFile(*fromFile)
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		pw = strings.TrimSpace(string(data))
	}
	if pw == "" {
		return errors.New("identity login: --password or --password-file is required")
	}
	body, _ := json.Marshal(map[string]any{"username": *username, "password": pw})
	state, err := newState(cf)
	if err != nil {
		return err
	}
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/json")
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/auth/login", body: bytes.NewReader(body), headers: hdr}, &out)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return fmt.Errorf("identity login returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

// identityEnroll is the First Flight path: it uses the install bootstrap
// token (from --token-file) to create the first Owner. After enrollment
// the bootstrap token remains valid for additional Owner/NetworkAdmin
// provisioning but is no longer required for the Owner login.
func identityEnroll(args []string) error {
	fs := flag.NewFlagSet("identity enroll", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	username := fs.String("username", "", "First-Owner username (1-48 chars, [a-z][a-z0-9._-]*)")
	password := fs.String("password", "", "First-Owner password (15-128 chars, or use --password-file)")
	fromFile := fs.String("password-file", "", "Path to a file containing the password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" || *password == "" && *fromFile == "" {
		return errors.New("identity enroll: --username and --password are required")
	}
	pw := *password
	if *fromFile != "" {
		data, err := os.ReadFile(*fromFile)
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		pw = strings.TrimSpace(string(data))
	}
	if len(pw) < 15 {
		return errors.New("identity enroll: password must be at least 15 characters")
	}
	body, _ := json.Marshal(map[string]any{"username": *username, "password": pw})
	state, err := newState(cf)
	if err != nil {
		return err
	}
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/json")
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/auth/enroll", body: bytes.NewReader(body), headers: hdr}, &out)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return fmt.Errorf("identity enroll returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

func identityChangePassword(args []string) error {
	fs := flag.NewFlagSet("identity change-password", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	old := fs.String("old-password", "", "Existing password (or use --old-password-file)")
	oldFile := fs.String("old-password-file", "", "Path to existing password file")
	new := fs.String("new-password", "", "New password (or use --new-password-file)")
	newFile := fs.String("new-password-file", "", "Path to new password file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	oldPw := *old
	if *oldFile != "" {
		data, err := os.ReadFile(*oldFile)
		if err != nil {
			return fmt.Errorf("read old password: %w", err)
		}
		oldPw = strings.TrimSpace(string(data))
	}
	newPw := *new
	if *newFile != "" {
		data, err := os.ReadFile(*newFile)
		if err != nil {
			return fmt.Errorf("read new password: %w", err)
		}
		newPw = strings.TrimSpace(string(data))
	}
	if oldPw == "" || newPw == "" {
		return errors.New("identity change-password: old and new passwords are required")
	}
	if len(newPw) < 15 {
		return errors.New("identity change-password: new password must be at least 15 characters")
	}
	body, _ := json.Marshal(map[string]any{"oldPassword": oldPw, "newPassword": newPw})
	return authedCall(cf, http.MethodPost, "/api/v1/auth/password", body, "identity change-password")
}

func identityUsers(args []string) error {
	fs := flag.NewFlagSet("identity users", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return authedRead(cf, "/api/v1/auth/users", "identity users")
}

func identityAudit(args []string) error {
	fs := flag.NewFlagSet("identity audit", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return authedRead(cf, "/api/v1/auth/audit", "identity audit")
}

func identitySessions(args []string) error {
	fs := flag.NewFlagSet("identity sessions", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return authedRead(cf, "/api/v1/auth/sessions", "identity sessions")
}

func identityRevoke(args []string) error {
	fs := flag.NewFlagSet("identity revoke", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	id := fs.String("session-id", "", "Session id to revoke")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("identity revoke: --session-id is required")
	}
	body, _ := json.Marshal(map[string]any{"id": *id})
	return authedCall(cf, http.MethodPost, "/api/v1/auth/revoke", body, "identity revoke")
}

func wireguardCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("wireguard: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "servers", "server":
		return wireguardServers(rest)
	case "peers", "peer":
		return wireguardPeers(rest)
	default:
		return fmt.Errorf("wireguard: unknown operation %q", op)
	}
}

func wireguardServers(args []string) error {
	if len(args) == 0 {
		return errors.New("wireguard servers: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "list":
		return wireguardServersList(rest)
	case "create":
		return wireguardServersCreate(rest)
	case "delete":
		return wireguardServersDelete(rest)
	default:
		return fmt.Errorf("wireguard servers: unknown operation %q", op)
	}
}

func wireguardServersList(args []string) error {
	fs := flag.NewFlagSet("wireguard servers list", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return authedRead(cf, "/api/v1/wireguard/servers", "wireguard servers list")
}

func wireguardServersCreate(args []string) error {
	fs := flag.NewFlagSet("wireguard servers create", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	file := fs.String("server", "", "Path to a server JSON file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("wireguard servers create: --server is required")
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("read server file: %w", err)
	}
	return authedCall(cf, http.MethodPost, "/api/v1/wireguard/servers", data, "wireguard servers create")
}

func wireguardServersDelete(args []string) error {
	fs := flag.NewFlagSet("wireguard servers delete", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	id := fs.String("server-id", "", "Server id to delete")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("wireguard servers delete: --server-id is required")
	}
	return authedCall(cf, http.MethodDelete, "/api/v1/wireguard/servers/"+*id, nil, "wireguard servers delete")
}

func wireguardPeers(args []string) error {
	if len(args) == 0 {
		return errors.New("wireguard peers: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "add":
		return wireguardPeersAdd(rest)
	case "delete":
		return wireguardPeersDelete(rest)
	default:
		return fmt.Errorf("wireguard peers: unknown operation %q", op)
	}
}

func wireguardPeersAdd(args []string) error {
	fs := flag.NewFlagSet("wireguard peers add", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	file := fs.String("peer", "", "Path to a peer JSON file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("wireguard peers add: --peer is required")
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("read peer file: %w", err)
	}
	return authedCall(cf, http.MethodPost, "/api/v1/wireguard/peers", data, "wireguard peers add")
}

func wireguardPeersDelete(args []string) error {
	fs := flag.NewFlagSet("wireguard peers delete", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	id := fs.String("peer-id", "", "Peer id to delete")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("wireguard peers delete: --peer-id is required")
	}
	return authedCall(cf, http.MethodDelete, "/api/v1/wireguard/peers/"+*id, nil, "wireguard peers delete")
}

func xrayCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("xray: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "servers", "users", "profiles", "subscriptions":
		return xrayCRUD(op, rest)
	default:
		return fmt.Errorf("xray: unknown operation %q", op)
	}
}

func xrayCRUD(kind string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("xray %s: missing operation", kind)
	}
	op, rest := args[0], args[1:]
	path := "/api/v1/xray/" + kind
	switch op {
	case "create":
		fs := flag.NewFlagSet("xray "+kind+" create", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		file := fs.String("file", "", "Path to a "+kind+" JSON file")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *file == "" {
			return fmt.Errorf("xray %s create: --file is required", kind)
		}
		data, err := os.ReadFile(*file)
		if err != nil {
			return fmt.Errorf("read %s file: %w", kind, err)
		}
		return authedCall(cf, http.MethodPost, path, data, "xray "+kind+" create")
	case "delete":
		fs := flag.NewFlagSet("xray "+kind+" delete", flag.ContinueOnError)
		cf := addCommonFlags(fs)
		id := fs.String("id", "", "Object id to delete")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *id == "" {
			return fmt.Errorf("xray %s delete: --id is required", kind)
		}
		return authedCall(cf, http.MethodDelete, path+"/"+*id, nil, "xray "+kind+" delete")
	default:
		return fmt.Errorf("xray %s: unknown operation %q", kind, op)
	}
}

func authedCall(cf *commonFlags, method, path string, body []byte, op string) error {
	state, err := newState(cf)
	if err != nil {
		return err
	}
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/json")
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: method, path: path, body: bytes.NewReader(body), headers: hdr}, &out)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return fmt.Errorf("%s returned %d: %s", op, code, errorMessage(out))
	}
	return render(cf.output, out)
}

func authedRead(cf *commonFlags, path, op string) error {
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: path}, &out)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		problem, _ := out.(map[string]any)
		return fmt.Errorf("%s returned %d: %s", op, code, errorMessage(problem))
	}
	return render(cf.output, out)
}

// render emits a value as JSON, a tabular representation, or plain text.
func render(format string, value any) error {
	switch format {
	case "json", "":
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "text":
		fmt.Println(flatText(value))
	case "table":
		fmt.Println(flatTable(value))
	default:
		return fmt.Errorf("unknown --output format %q", format)
	}
	return nil
}

// qosCmd is the Duck CLI entry point for the QoS
// surface. The CLI is a thin wrapper: the API
// enforces auth and the Engine enforces the bounded,
// validated contract. The CLI just formats the
// result for a human operator.
func qosCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("qos: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "status":
		return qosStatus(rest)
	case "apply":
		return qosApply(rest)
	case "rollback":
		return qosRollback(rest)
	default:
		return fmt.Errorf("qos: unknown operation %q", op)
	}
}

// qosStatus fetches the QoS state. The CLI prints
// the topology, the last apply timestamp and the
// last error. The Engine is the only side that
// validates the topology.
func qosStatus(args []string) error {
	fs := flag.NewFlagSet("qos status", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	summary := fs.Bool("summary", false, "Print only the per-shaper summary table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/qos"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("qos status returned %d: %s", code, errorMessage(out))
	}
	if *summary {
		topo, _ := out["topology"].(map[string]any)
		shapers, _ := topo["shapers"].([]any)
		fmt.Printf("%-16s %-16s %-10s %-12s %-10s\n", "SHAPER", "LINK", "DIRECTION", "ALGORITHM", "RATE-KBPS")
		for _, s := range shapers {
			m, _ := s.(map[string]any)
			if m == nil {
				continue
			}
			id, _ := m["id"].(string)
			link, _ := m["link"].(string)
			dir, _ := m["direction"].(string)
			algo, _ := m["algorithm"].(string)
			rate, _ := m["rateKbps"].(float64)
			fmt.Printf("%-16s %-16s %-10s %-12s %-10d\n", id, link, dir, algo, int(rate))
		}
		return nil
	}
	return render(cf.output, out)
}

// qosApply submits a plan file to the Agent. The
// file is read into a single buffer; the API
// enforces the body cap. The plan must be a JSON
// document produced by the renderer.
func qosApply(args []string) error {
	fs := flag.NewFlagSet("qos apply", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("qos apply: expected a single plan file path")
	}
	body, err := os.ReadFile(rest[0])
	if err != nil {
		return fmt.Errorf("qos apply: read plan: %w", err)
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	headers := http.Header{"Content-Type": []string{"application/json"}}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/qos/apply", body: bytes.NewReader(body), headers: headers}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("qos apply returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

// qosRollback asks the Agent to roll back the
// active plan. The Engine surfaces the Agent's
// outcome; the CLI just formats it.
func qosRollback(args []string) error {
	fs := flag.NewFlagSet("qos rollback", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/qos/rollback"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("qos rollback returned %d: %s", code, errorMessage(out))
	}
	return render(cf.output, out)
}

// upsCmd is the Duck CLI entry point for the UPS
// surface. The CLI is a thin wrapper: the API
// enforces auth and the Engine enforces the
// bounded, validated contract. The CLI just
// formats the result for a human operator.
func upsCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("ups: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "status":
		return upsStatus(rest)
	case "probe":
		return upsProbe(rest)
	default:
		return fmt.Errorf("ups: unknown operation %q", op)
	}
}

// upsStatus fetches the UPS state. The CLI
// prints the topology, the per-device health
// and the last probe timestamp. The Engine is
// the only side that validates the topology.
func upsStatus(args []string) error {
	fs := flag.NewFlagSet("ups status", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	summary := fs.Bool("summary", false, "Print only the per-device summary table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/ups"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("ups status returned %d: %s", code, errorMessage(out))
	}
	if *summary {
		health, _ := out["health"].(map[string]any)
		topo, _ := out["topology"].(map[string]any)
		devices, _ := topo["devices"].([]any)
		fmt.Printf("%-16s %-24s %-16s %-16s %s\n", "DEVICE", "NAME", "DRIVER", "HEALTH", "CRITICAL")
		for _, d := range devices {
			m, _ := d.(map[string]any)
			if m == nil {
				continue
			}
			id, _ := m["id"].(string)
			name, _ := m["name"].(string)
			driver, _ := m["driver"].(string)
			h, _ := health[id].(string)
			critical := ""
			switch h {
			case "low-battery", "replace-battery", "lost", "degraded":
				critical = "yes"
			}
			fmt.Printf("%-16s %-24s %-16s %-16s %s\n", id, name, driver, h, critical)
		}
		return nil
	}
	return render(cf.output, out)
}

// upsProbe asks the Agent to run an on-demand
// probe of every UPS device. The CLI does not
// receive a body; the Engine surfaces the
// per-device health through the next status
// call.
func upsProbe(args []string) error {
	fs := flag.NewFlagSet("ups probe", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/ups/probe"}, &out)
	if err != nil {
		return err
	}
	if code != 204 && code != 200 {
		return fmt.Errorf("ups probe returned %d: %s", code, errorMessage(out))
	}
	fmt.Println("ups probe: ok")
	return nil
}

// migrateCmd is the Duck CLI entry point for
// the migration surface. The CLI is a thin
// wrapper: the API enforces auth and the
// Engine enforces the bounded, validated
// contract. The CLI just formats the result
// for a human operator.
func migrateCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("migrate: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "analyse":
		return migrateAnalyse(rest)
	default:
		return fmt.Errorf("migrate: unknown operation %q", op)
	}
}

// migrateAnalyse submits a source file to the
// Agent. The CLI reads the file into a single
// buffer; the API enforces the body cap. The
// returned Plan is rendered as a per-line
// classification table.
func migrateAnalyse(args []string) error {
	fs := flag.NewFlagSet("migrate analyse", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	formatHint := fs.String("format", "", "Source format hint: mikrotik|openwrt-uci|linux-interfaces|kerio")
	summary := fs.Bool("summary", false, "Print only the per-status summary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("migrate analyse: expected a single source file path")
	}
	body, err := os.ReadFile(rest[0])
	if err != nil {
		return fmt.Errorf("migrate analyse: read source: %w", err)
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	headers := http.Header{}
	headers.Set("X-DuckOS-Migrate-Filename", rest[0])
	if *formatHint != "" {
		headers.Set("X-DuckOS-Migrate-Format", *formatHint)
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodPost, path: "/api/v1/migrate", body: bytes.NewReader(body), headers: headers}, &out)
	if err != nil {
		return err
	}
	if code == 422 {
		return fmt.Errorf("migrate analyse returned %d: %s", code, errorMessage(out))
	}
	if code != 200 {
		return fmt.Errorf("migrate analyse returned %d: %s", code, errorMessage(out))
	}
	if *summary {
		fmt.Printf("%-16s %-12s %-8s\n", "FORMAT", "CONFIDENCE", "TOTAL")
		fmt.Printf("%-16s %-12s %-8d\n", stringOf(out["format"]), stringOf(out["confidence"]), intOf(out["items"]))
		items, _ := out["items"].([]any)
		var rec, conv, att, uns int
		for _, raw := range items {
			m, _ := raw.(map[string]any)
			if m == nil {
				continue
			}
			switch stringOf(m["status"]) {
			case "recognized":
				rec++
			case "convertible":
				conv++
			case "attention":
				att++
			case "unsupported":
				uns++
			}
		}
		fmt.Printf("  recognized:  %d\n", rec)
		fmt.Printf("  convertible: %d\n", conv)
		fmt.Printf("  attention:   %d\n", att)
		fmt.Printf("  unsupported: %d\n", uns)
		return nil
	}
	return render(cf.output, out)
}

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

func intOf(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return 0
}

// sitesCmd is the Duck CLI entry point for the
// sites surface. The CLI is a thin wrapper: the
// API enforces auth and the Engine enforces
// the bounded, validated contract. The CLI
// just formats the result for a human operator.
func sitesCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("sites: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "status":
		return sitesStatus(rest)
	default:
		return fmt.Errorf("sites: unknown operation %q", op)
	}
}

// sitesStatus fetches the sites state. The CLI
// prints the topology, the per-site health and
// the last probe timestamp. The Engine is the
// only side that validates the topology.
func sitesStatus(args []string) error {
	fs := flag.NewFlagSet("sites status", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	summary := fs.Bool("summary", false, "Print only the per-site summary table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/sites"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("sites status returned %d: %s", code, errorMessage(out))
	}
	if *summary {
		health, _ := out["health"].(map[string]any)
		topo, _ := out["topology"].(map[string]any)
		sites2, _ := topo["sites"].([]any)
		fmt.Printf("%-16s %-32s %-16s %-16s %-8s %-8s\n", "SITE", "DOMAIN", "LISTENER", "TARGET", "TLS", "DDNS")
		for _, s := range sites2 {
			m, _ := s.(map[string]any)
			if m == nil {
				continue
			}
			id, _ := m["id"].(string)
			domain, _ := m["domain"].(string)
			listener, _ := m["listener"].(map[string]any)
			target, _ := m["target"].(map[string]any)
			port, _ := listener["port"].(float64)
			proto, _ := listener["protocol"].(string)
			ifaceID, _ := listener["interfaceId"].(string)
			listenerStr := proto
			if ifaceID != "" {
				listenerStr = ifaceID + ":" + proto
			}
			listenerStr = fmt.Sprintf("%s/%d", listenerStr, int(port))
			tHost, _ := target["host"].(string)
			tPort, _ := target["port"].(float64)
			tProto, _ := target["protocol"].(string)
			tls, _ := m["tls"].(map[string]any)
			ddns, _ := m["ddns"].(map[string]any)
			tlsFlag := "no"
			if e, _ := tls["enabled"].(bool); e {
				tlsFlag = "yes"
			}
			ddnsFlag := "no"
			if p, _ := ddns["provider"].(string); p != "" {
				ddnsFlag = "yes"
			}
			_ = health
			fmt.Printf("%-16s %-32s %-16s %-16s %-8s %-8s\n", id, domain, listenerStr, fmt.Sprintf("%s:%s/%d", tHost, tProto, int(tPort)), tlsFlag, ddnsFlag)
		}
		return nil
	}
	return render(cf.output, out)
}

// wifiCmd is the Duck CLI entry point for the
// Wi-Fi surface. The CLI is a thin wrapper: the
// API enforces auth and the Engine enforces
// the bounded, validated contract. The CLI
// just formats the result for a human operator.
func wifiCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("wifi: missing operation")
	}
	op, rest := args[0], args[1:]
	switch op {
	case "status":
		return wifiStatus(rest)
	default:
		return fmt.Errorf("wifi: unknown operation %q", op)
	}
}

// wifiStatus fetches the Wi-Fi state. The CLI
// prints the topology, the host capability, the
// per-AP health and the last probe timestamp.
func wifiStatus(args []string) error {
	fs := flag.NewFlagSet("wifi status", flag.ContinueOnError)
	cf := addCommonFlags(fs)
	summary := fs.Bool("summary", false, "Print only the per-AP summary table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	state, err := newState(cf)
	if err != nil {
		return err
	}
	var out map[string]any
	code, err := state.call(context.Background(), apiCall{method: http.MethodGet, path: "/api/v1/wifi"}, &out)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("wifi status returned %d: %s", code, errorMessage(out))
	}
	if *summary {
		cap, _ := out["capability"].(map[string]any)
		supported, _ := cap["supported"].(bool)
		driver, _ := cap["driver"].(string)
		iface, _ := cap["interface"].(string)
		reason, _ := cap["reason"].(string)
		fmt.Printf("Wi-Fi capability: supported=%v", supported)
		if driver != "" {
			fmt.Printf(" driver=%s", driver)
		}
		if iface != "" {
			fmt.Printf(" interface=%s", iface)
		}
		if reason != "" {
			fmt.Printf(" reason=%q", reason)
		}
		fmt.Println()
		health, _ := out["health"].(map[string]any)
		stations, _ := out["stations"].(map[string]any)
		topo, _ := out["topology"].(map[string]any)
		aps, _ := topo["aps"].([]any)
		fmt.Printf("%-16s %-32s %-4s %-4s %-12s %-12s %s\n", "AP", "SSID", "BAND", "CH", "SECURITY", "HEALTH", "STATIONS")
		for _, a := range aps {
			m, _ := a.(map[string]any)
			if m == nil {
				continue
			}
			id, _ := m["id"].(string)
			ssid, _ := m["ssid"].(string)
			band, _ := m["band"].(string)
			ch, _ := m["channel"].(float64)
			sec, _ := m["security"].(string)
			h, _ := health[id].(string)
			if h == "" {
				h = "unknown"
			}
			count := 0
			if v, ok := stations[id].(float64); ok {
				count = int(v)
			}
			fmt.Printf("%-16s %-32s %-4s %-4d %-12s %-12s %d\n", id, ssid, band, int(ch), sec, h, count)
		}
		return nil
	}
	return render(cf.output, out)
}

func errorMessage(out map[string]any) string {
	if out == nil {
		return "no body"
	}
	if errObj, ok := out["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok {
			return msg
		}
	}
	return truncate(strings.TrimSpace(fmt.Sprintf("%v", out)), 200)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func flatText(v any) string {
	switch x := v.(type) {
	case map[string]any:
		var b strings.Builder
		for k, val := range x {
			fmt.Fprintf(&b, "%-24s %v\n", k+":", val)
		}
		return b.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func flatTable(v any) string {
	// Tables are a future enhancement; the CLI uses a key/value flat
	// representation today and the user can pipe through `jq` for richer
	// transforms.
	return flatText(v)
}

func urlQueryEscape(s string) string {
	// Local escape helper to keep the imports list short.
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-' || r == '_' || r == '.' || r == '~':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "%%%02X", r)
		}
	}
	return b.String()
}

var _ = strconv.Itoa
var _ = brand.Mark
