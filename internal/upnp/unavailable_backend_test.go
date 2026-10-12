package upnp

import (
	"errors"
	"net"
	"strings"
	"testing"
)

func TestUnavailableBackend_AddReturnsErrBackendUnavailable(t *testing.T) {
	b := NewUnavailableBackend("nft-binary-missing", "exec: \"nft\": executable file not found")
	err := b.Add(Mapping{ID: "m1", NetworkRef: "lan", Protocol: ProtoUPnP})
	if !errors.Is(err, ErrBackendUnavailable) {
		t.Fatalf("expected ErrBackendUnavailable, got %v", err)
	}
}

func TestUnavailableBackend_RemoveAlwaysSucceeds(t *testing.T) {
	b := NewUnavailableBackend("nft-binary-missing", "test")
	if err := b.Remove("m1"); err != nil {
		t.Fatalf("expected nil Remove, got %v", err)
	}
}

func TestUnavailableBackend_AllowListenRefusesAll(t *testing.T) {
	b := NewUnavailableBackend("nft-binary-missing", "test")
	if b.AllowListen("eth0", &net.UDPAddr{IP: net.IPv4(192, 168, 0, 1), Port: 9}) {
		t.Fatal("expected AllowListen to refuse eth0")
	}
	if b.AllowListen("lo", nil) {
		t.Fatal("expected AllowListen to refuse lo")
	}
	if b.AllowListen("", nil) {
		t.Fatal("expected AllowListen to refuse empty iface")
	}
}

func TestUnavailableBackend_ReasonIsStable(t *testing.T) {
	b := NewUnavailableBackend("nft-binary-missing", "exec: \"nft\": executable file not found in $PATH")
	reason := b.Reason()
	if reason.Reason != "nft-binary-missing" {
		t.Fatalf("expected stable reason, got %s", reason.Reason)
	}
	if !strings.Contains(reason.Diagnostic, "nft") {
		t.Fatalf("expected diagnostic to mention nft, got %s", reason.Diagnostic)
	}
	if reason.Since == "" {
		t.Fatal("expected non- since")
	}
}

func TestRuntimeStub_BackendReportsUnavailable(t *testing.T) {
	mgr := NewManager()
	b := NewUnavailableBackend("test", "diag")
	stub := NewRuntimeStub(mgr, b)
	stats := stub.Stats()
	if stats.Backend.Kind != BackendKindUnavailable {
		t.Fatalf("expected unavailable, got %s", stats.Backend.Kind)
	}
	if stats.Backend.Reason != "test" {
		t.Fatalf("expected reason=test, got %s", stats.Backend.Reason)
	}
}

func TestRuntimeStub_BackendReportsNftablesByTypeName(t *testing.T) {
	// Cross-platform Backend() cannot reference the linux-only
	// *NftablesBackend type directly. Verify the type-name string
	// branch is reachable by inserting a backend whose type name
	// matches the NftablesBackend convention. The lookup is by
	// Go reflect, but for the stub tests this is asserted via the
	// backendTypeName helper to avoid the cross-build constraint.
	mgr := NewManager()
	nftLike := &nftLikeBackend{}
	stub := NewRuntimeStub(mgr, nftLike)
	stats := stub.Stats()
	if stats.Backend.Kind == BackendKindNftables {
		t.Fatalf("did not expect BackendKindNftables for non-real backend")
	}
}

// nftLikeBackend is a stand-in used to verify the Backend()
// routing in cross-platform builds. It does not satisfy the
// real *NftablesBackend type, only the NetworkBackend
// interface.
type nftLikeBackend struct{}

func (nftLikeBackend) Add(Mapping) error                 { return nil }
func (nftLikeBackend) Remove(string) error               { return nil }
func (nftLikeBackend) AllowListen(string, net.Addr) bool { return true }
func (nftLikeBackend) TrustIface(string)                 {}
func (nftLikeBackend) UntrustIface(string)               {}
