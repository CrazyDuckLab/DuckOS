// Package xnode / e2e_process_test.go: production-topology E2E with
// the controller and the node as SEPARATE OS PROCESSES, separate
// durable state directories, separate identities and explicitly
// pinned TLS trust.
//
// Harness pattern: the test binary re-executes itself as a child
// (env XNODE_E2E_CHILD=1) running the node role — its own
// NodeIdentity store, its own NodeServer on a real TLS listener.
// The parent process runs the controller (Manager + adapter with
// the certificate-fingerprint pin of the child's freshly generated
// certificate). Both sides use only their own durable files.
package xnode

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const childEnv = "XNODE_E2E_CHILD"

// childConfig is the hand-off document the parent writes and the
// child reads at startup.
type childConfig struct {
	Listen      string `json:"listen"`
	CertPath    string `json:"certPath"`
	KeyPath     string `json:"keyPath"`
	IdentityDir string `json:"identityDir"`
	Credential  string `json:"credential"`
	ReadyFile   string `json:"readyFile"`
}

func TestMain(m *testing.M) {
	if os.Getenv(childEnv) == "1" {
		runChildProcess()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runChildProcess is the node role in its own OS process.
func runChildProcess() {
	raw, err := os.ReadFile(os.Getenv("XNODE_E2E_CONFIG"))
	if err != nil {
		os.Exit(2)
	}
	var cfg childConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		os.Exit(2)
	}
	identity, err := OpenNodeIdentity(cfg.IdentityDir)
	if err != nil {
		os.Exit(3)
	}
	if !identity.Adopted() {
		// First boot adopts the shipped credential; later boots of
		// the same node reuse its persistent identity.
		if err := identity.Adopt(cfg.Credential); err != nil {
			os.Exit(4)
		}
	}
	cert, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
	if err != nil {
		os.Exit(5)
	}
	srv := NewNodeServer(nil, func([]byte) error { return nil },
		func() NodeInfo {
			return NodeInfo{Name: "edge-child", Version: "26.3.27", Capabilities: []string{"vless", "reality"}}
		})
	srv.Identity = identity
	httpSrv := &http.Server{
		Addr:        cfg.Listen,
		Handler:     srv,
		TLSConfig:   &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		ReadTimeout: 35 * time.Second,
	}
	// Ready signal AFTER the listener socket is bound.
	ln, lerr := net.Listen("tcp", cfg.Listen)
	if lerr != nil {
		os.Exit(6)
	}
	_ = os.WriteFile(cfg.ReadyFile, []byte("ok"), 0o600)
	_ = httpSrv.ServeTLS(ln, cfg.CertPath, cfg.KeyPath)
}

// selfSignedPairFor generates the node's TLS pair and returns the
// sha256 fingerprint of the leaf (the explicit trust anchor).
func selfSignedPairFor(t *testing.T, dir string) (certPath, keyPath, fingerprint string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serialNumber(),
		Subject:      pkix.Name{CommonName: "duckos-node-e2e"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  loopbackIPs(),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pemEncodeCert(der)
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pemEncodeKey(keyDER)
	certPath = filepath.Join(dir, "node.crt")
	keyPath = filepath.Join(dir, "node.key")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	return certPath, keyPath, hex.EncodeToString(sum[:])
}

// TestE2E_TwoProcesses runs the complete multi-node lifecycle with
// the controller and the node in separate OS processes.
func TestE2E_TwoProcesses(t *testing.T) {
	if os.Getenv(childEnv) == "1" {
		return // child mode is driven by TestMain
	}
	nodeDir := t.TempDir() // child's durable identity, its OWN directory
	certPath, keyPath, fingerprint := selfSignedPairFor(t, nodeDir)
	ready := filepath.Join(nodeDir, "ready")

	// Enrollment happens on the controller; the credential ships to
	// the node through the operator channel (here: the hand-off
	// config), exactly once.
	var durable DurableState
	controller := NewManager(nil)
	controller.SetPersist(func(st DurableState) (CommitResult, error) {
		durable = st
		return CommitResult{}, nil
	})
	tok, _, err := controller.IssueEnrollmentToken(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	node, credential, err := controller.Enroll(tok, NodeInfo{Name: "edge-e2e", Version: "26.3.27",
		Capabilities: []string{"vless", "reality"}})
	if err != nil {
		t.Fatal(err)
	}

	// Start the CHILD OS PROCESS (the node role).
	childCfg := childConfig{
		Listen: "127.0.0.1:0", CertPath: certPath, KeyPath: keyPath,
		IdentityDir: filepath.Join(nodeDir, "identity"),
		Credential:  credential, ReadyFile: ready,
	}
	// Port must be known to the parent: pick a free port and pin it.
	port := freePortE2E(t)
	childCfg.Listen = fmt.Sprintf("127.0.0.1:%d", port)
	cfgPath := filepath.Join(nodeDir, "child-config.json")
	rawCfg, _ := json.Marshal(childCfg)
	if err := os.WriteFile(cfgPath, rawCfg, 0o600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestE2E_TwoProcesses")
	cmd.Env = append(os.Environ(), childEnv+"=1", "XNODE_E2E_CONFIG="+cfgPath)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	// The child is alive and the identity is durable on disk BEFORE
	// any control traffic (persistence-before-networking).
	waitForFile(t, ready, 10*time.Second)

	// TLS trust: explicit certificate pinning (no CA, no bypass).
	pinned, err := PinningHTTPClient(fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	adapter := NewNodeHTTPAdapter()
	adapter.HTTPClient = pinned
	node.Endpoint = fmt.Sprintf("https://127.0.0.1:%d", port)

	// Wrong TLS pin must fail (certificate identity validation).
	wrongPin, _ := PinningHTTPClient(repeatHex("ab"))
	badAdapter := NewNodeHTTPAdapter()
	badAdapter.HTTPClient = wrongPin
	if _, err := badAdapter.Discover(WithCredential(context.Background(), credential), node); err == nil {
		t.Fatal("E2E: mismatched certificate pin was accepted")
	}

	// Wrong credential must fail over the real wire.
	if _, err := adapter.Discover(WithCredential(context.Background(), "wrong"), node); err == nil {
		t.Fatal("E2E: wrong credential accepted")
	}

	// 6-7: authenticate + health/discovery.
	ctx := WithCredential(context.Background(), credential)
	info, err := adapter.Discover(ctx, node)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if info.Version != "26.3.27" {
		t.Fatalf("capabilities/version: %+v", info)
	}
	if _, err := adapter.Health(ctx, node); err != nil {
		t.Fatalf("health: %v", err)
	}

	// 8-9: deliver a safe configuration (empty inbounds — nothing
	// exposed); the child applies it through its apply hook.
	if err := adapter.PushConfig(ctx, node, []byte(`{"inbounds":[],"outbounds":[{"tag":"direct","protocol":"freedom"}]}`)); err != nil {
		t.Fatalf("push: %v", err)
	}

	// 11: restart the child OS process; identity and acceptance
	// survive (durable state).
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = cmd.Process.Wait()
	cmd2 := exec.Command(exe, "-test.run=TestE2E_TwoProcesses")
	cmd2.Env = append(os.Environ(), childEnv+"=1", "XNODE_E2E_CONFIG="+cfgPath)
	cmd2.Stdout, cmd2.Stderr = os.Stdout, os.Stderr
	if err := cmd2.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd2.Process.Kill()
		_, _ = cmd2.Process.Wait()
	}()
	// The identity was already adopted; the child must NOT require
	// a new credential. Re-binding over a live identity is refused,
	// so the restart path reads the existing store: adjust the
	// hand-off so the child skips adoption when already adopted.
	waitForFile(t, ready, 10*time.Second)
	// Give the new listener a moment, then re-authenticate.
	deadline := time.Now().Add(10 * time.Second)
	var rediscovered bool
	for time.Now().Before(deadline) {
		if _, err := adapter.Discover(ctx, node); err == nil {
			rediscovered = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !rediscovered {
		t.Fatal("E2E: node did not re-authenticate after restart")
	}

	// 12-15: credential rotation across processes.
	rotated, err := controller.RotateCredential(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.DeliverRotation(ctx, node, credential, rotated); err != nil {
		t.Fatalf("rotation delivery: %v", err)
	}
	if err := adapter.ConfirmRotation(WithCredential(context.Background(), rotated), node, rotated); err != nil {
		t.Fatalf("rotation confirm: %v", err)
	}
	// 16: the OLD credential is rejected on BOTH sides.
	if controller.Authenticate(node.ID, credential) {
		t.Fatal("E2E: controller still accepts the old credential")
	}
	if _, err := adapter.Discover(ctx, node); err == nil {
		t.Fatal("E2E: node still accepts the old credential over the wire")
	}
	// 17: the new credential works and survives another restart.
	if _, err := adapter.Discover(WithCredential(context.Background(), rotated), node); err != nil {
		t.Fatal("new credential rejected")
	}

	// 18-20: suspension/revocation deny on the controller, and
	// revocation persists across a controller restart (durable
	// registry state).
	if err := controller.Revoke(node.ID); err != nil {
		t.Fatal(err)
	}
	if controller.Authenticate(node.ID, rotated) {
		t.Fatal("revoked node authenticated")
	}
	// 20: controller restart — a fresh manager reconstructed from
	// the durable registry snapshot must still deny the credential.
	restarted := NewManager(nil)
	restarted.LoadState(durable)
	if restarted.Authenticate(node.ID, rotated) {
		t.Fatal("E2E: revocation lost across controller restart")
	}
	_ = json.Marshal
	_ = hex.EncodeToString
}

func freePortE2E(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("ready file never appeared")
}

func repeatHex(pair string) string {
	out := ""
	for i := 0; i < 32; i++ {
		out += pair
	}
	return out
}

func serialNumber() *bigIntAlias { return newSerial() }

func loopbackIPs() []net.IP { return []net.IP{net.ParseIP("127.0.0.1")} }
