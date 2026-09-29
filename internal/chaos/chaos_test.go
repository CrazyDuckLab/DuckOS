package chaos

import (
	"testing"
	"time"
)

func TestReadiness(t *testing.T) {
	s := NewReadinessStore()
	s.Set(FeatureReadiness{Feature: "frr-bgp", State: ReadySupported})
	s.Set(FeatureReadiness{Feature: "hostapd", State: ReadyExperimental})
	if r, ok := s.Get("frr-bgp"); !ok || r.State != ReadySupported {
		t.Fatal("get")
	}
	if len(s.All()) != 2 {
		t.Fatal("all")
	}
}

func TestFault(t *testing.T) {
	m := NewFaultManager()
	if err := m.Add(Fault{ID: "f1", Kind: "link-down", Target: "wan1", Isolated: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(Fault{ID: "f2", Kind: "link-down", Target: "wan1", Isolated: false}); err == nil {
		t.Fatal("non-isolated must fail")
	}
	if len(m.All()) != 1 {
		t.Fatal("all")
	}
}

func TestTelemetry(t *testing.T) {
	s := NewTelemetryStore()
	if err := s.Set(TelemetryPref{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(TelemetryPref{Enabled: true, UserAgent: "with space"}); err == nil {
		t.Fatal("space in userAgent must fail")
	}
}

func TestCrashStore(t *testing.T) {
	s := NewCrashStore()
	s.Record(CrashEvidence{ID: "c1", Component: "xray", Version: "1.0", Reason: "panic"})
	if len(s.All()) != 1 {
		t.Fatal("all")
	}
}

func TestPerf(t *testing.T) {
	s := NewPerfStore()
	s.Set(IRQCPUPerf{IRQAffinity: map[string]string{"eth0": "0-3"}})
	if !s.Get().Reversible {
		t.Fatal("reversible must default true")
	}
}

func TestNTS(t *testing.T) {
	s := NewNTSStore()
	s.Set(NTSState{Enabled: true, Server: "ts.example.com", Authenticated: true, LastSync: nowTest()})
	if !s.Get().Authenticated {
		t.Fatal("auth")
	}
}

func TestUSP(t *testing.T) {
	s := NewUSPStore()
	if err := s.Set(USPState{Enabled: true, Endpoint: "https://usp.example.com", AuthMethod: "mtls", LocalFirst: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(USPState{Enabled: true, AuthMethod: "garbage"}); err == nil {
		t.Fatal("bad auth must fail")
	}
	if err := s.Set(USPState{Enabled: true, LocalFirst: false}); err == nil {
		t.Fatal("non-local-first must fail")
	}
}

func TestSSHKeys(t *testing.T) {
	s := NewSSHKeyStore()
	s.Set(SSHHostKey{Type: "ed25519", Fingerprint: "SHA256:abc"})
	if _, ok := s.Get("ed25519"); !ok {
		t.Fatal("get")
	}
}

func TestClone(t *testing.T) {
	s := NewCloneStore()
	s.Set(CloneProtection{Class: "machine-id", RegenerateOnClone: true, Description: "x"})
	if _, ok := s.Get("machine-id"); !ok {
		t.Fatal("get")
	}
}

func TestConsole(t *testing.T) {
	c := NewConsoleCatalog()
	c.Register(ConsoleCommand{Name: "show-health", Description: "show health", Mutating: false, Allowed: true})
	c.Register(ConsoleCommand{Name: "restore-previous", Description: "x", Mutating: true, Allowed: true})
	if len(c.All()) != 2 {
		t.Fatal("all")
	}
	if err := c.Register(ConsoleCommand{Name: "!!bad"}); err == nil {
		t.Fatal("bad name must fail")
	}
}

func nowTest() time.Time { return time.Unix(1700000000, 0).UTC() }
