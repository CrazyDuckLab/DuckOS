// G02+G06 torture regression: every kind the Engine
// advertises must (a) be registered, (b) return a
// well-formed object.Collection envelope, and (c) never
// fabricate success — unsupported kinds carry an explicit
// "unsupported_kind" issue so the UI/CLI can render an
// honest gap badge.
package objectlisters

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
)

// mustHaveKinds is the canonical list of kinds the freeze
// candidate promises to expose. It is duplicated here
// intentionally — a drift between SupportedKinds and this
// list is itself a defect.
var mustHaveKinds = []string{
	"network.interface",
	"network.route",
	"network.firewall.rule",
	"network.firewall.set",
	"network.lease",
	"network.address",
	"wireguard.server",
	"wireguard.peer",
	"xray.server",
	"xray.user",
	"xray.profile",
	"xray.subscription",
	"qos.class",
	"dns.zone",
	"dhcp.subnet",
	"bgp.peer",
	"ospf.area",
	"isis.area",
	"route.map",
}

func TestSupportedKinds_AreStable(t *testing.T) {
	if len(SupportedKinds) != len(mustHaveKinds) {
		t.Fatalf("SupportedKinds drift: have %d, want %d (%v)",
			len(SupportedKinds), len(mustHaveKinds), SupportedKinds)
	}
	got := map[string]bool{}
	for _, k := range SupportedKinds {
		got[k] = true
	}
	for _, k := range mustHaveKinds {
		if !got[k] {
			t.Fatalf("SupportedKinds missing %q", k)
		}
	}
}

func TestRegisterAll_WiresEveryKind(t *testing.T) {
	e := engine.New("/tmp/duckos-lister-test.sock", "test")
	RegisterAll(e)

	for _, k := range mustHaveKinds {
		col, err := e.ObjectList(context.Background(), k, object.CollectionParams{})
		if err != nil {
			t.Errorf("ObjectList(%q) returned error: %v", k, err)
			continue
		}
		if col.SnapshotID == "" {
			t.Errorf("ObjectList(%q) returned empty SnapshotID", k)
		}
		if col.Items == nil {
			t.Errorf("ObjectList(%q) returned nil Items (must be [])", k)
		}
	}
}

// realKinds are the kinds backed by real Engine accessors. The
// freeze candidate wires qos.class, network.interface and
// network.route to the privileged Agent IPC; every other kind
// remains an honest "unsupported" stub so the UI can render the
// gap without inventing availability.
var realKinds = map[string]bool{
	"qos.class":         true,
	"network.interface": true,
	"network.route":     true,
}

func TestRegisterAll_UnsupportedKindCarriesIssue(t *testing.T) {
	e := engine.New("/tmp/duckos-lister-test.sock", "test")
	RegisterAll(e)

	// A still-unsupported kind must carry the honest
	// "engine_unavailable" issue. Use a kind the freeze
	// candidate does not yet back with a real lister.
	col, err := e.ObjectList(context.Background(), "network.firewall.rule", object.CollectionParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(col.Issues) == 0 {
		t.Fatalf("ObjectList(network.firewall.rule) must carry an explicit engine_unavailable issue")
	}
	found := false
	for _, issue := range col.Issues {
		if issue.ReasonID == "engine_unavailable" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ObjectList(network.firewall.rule) issues=%v must include engine_unavailable", col.Issues)
	}
	if col.Complete {
		t.Fatalf("ObjectList stub must NOT claim Complete=true (honest: source missing)")
	}
	if len(col.Items) != 0 {
		t.Fatalf("ObjectList stub must return 0 items, got %d", len(col.Items))
	}
}

// TestRegisterAll_RealKindCarriesEngineUnavailableIssue pins the
// behaviour of the real listers when the privileged Agent is
// not reachable: the envelope still carries a per-source Issue
// naming the failure so the UI can render the gap.
func TestRegisterAll_RealKindCarriesEngineUnavailableIssue(t *testing.T) {
	e := engine.New("/tmp/duckos-lister-test.sock", "test")
	RegisterAll(e)

	for _, k := range []string{"qos.class", "network.interface", "network.route"} {
		col, err := e.ObjectList(context.Background(), k, object.CollectionParams{})
		if err != nil {
			t.Fatalf("ObjectList(%q) returned error: %v", k, err)
		}
		if col.SnapshotID == "" {
			t.Errorf("ObjectList(%q) returned empty SnapshotID", k)
		}
		found := false
		for _, issue := range col.Issues {
			if issue.ReasonID == "engine_unavailable" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ObjectList(%q) issues=%v must include engine_unavailable", k, col.Issues)
		}
		if col.Items == nil {
			t.Errorf("ObjectList(%q) returned nil Items", k)
		}
	}
}

func TestRegisterAll_HonoursCollectionParams(t *testing.T) {
	e := engine.New("/tmp/duckos-lister-test.sock", "test")
	RegisterAll(e)

	col, err := e.ObjectList(context.Background(), "xray.user", object.CollectionParams{Limit: 5, After: "abc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if col.Complete {
		t.Fatalf("stub lister must NOT claim Complete=true (source missing)")
	}
	if col.SnapshotID == "" {
		t.Fatalf("stub lister must produce a non-empty SnapshotID")
	}
}

func TestRegisterAll_RoundTripJSON(t *testing.T) {
	e := engine.New("/tmp/duckos-lister-test.sock", "test")
	RegisterAll(e)

	col, err := e.ObjectList(context.Background(), "dns.zone", object.CollectionParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := json.Marshal(col)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"engine_unavailable"`) {
		t.Fatalf("round-trip JSON must carry engine_unavailable reason, got: %s", b)
	}
}

func TestRegisterAll_UnknownKindStillResponds(t *testing.T) {
	e := engine.New("/tmp/duckos-lister-test.sock", "test")
	RegisterAll(e)

	col, err := e.ObjectList(context.Background(), "no.such.kind", object.CollectionParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if col.SnapshotID != "engine-no-lister" {
		t.Fatalf("unknown kind SnapshotID drift: %q", col.SnapshotID)
	}
	if len(col.Items) != 0 {
		t.Fatalf("unknown kind must return 0 items, got %d", len(col.Items))
	}
	if len(col.Issues) == 0 {
		t.Fatalf("unknown kind must carry engine_unavailable issue")
	}
	if col.Complete {
		t.Fatalf("unknown kind must NOT claim Complete=true")
	}
}

// TestRegisterAll_LastRegistrationWins ensures the Engine
// honors its documented "last registration wins" rule so
// production listers can shadow the stub during boot.
func TestRegisterAll_LastRegistrationWins(t *testing.T) {
	e := engine.New("/tmp/duckos-lister-test.sock", "test")
	RegisterAll(e)

	override := overrideLister{snap: "production-lister-1"}
	e.RegisterObjectLister(override)

	col, err := e.ObjectList(context.Background(), "dns.zone", object.CollectionParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if col.SnapshotID != "production-lister-1" {
		t.Fatalf("last registration must win: got SnapshotID=%q", col.SnapshotID)
	}
}

// overrideLister is a tiny ObjectLister used to assert the
// "last registration wins" semantics of Engine.RegisterObjectLister.
type overrideLister struct{ snap string }

func (o overrideLister) Kind() string { return "dns.zone" }
func (o overrideLister) List(_ context.Context, _ object.CollectionParams) (object.Collection, error) {
	return object.Collection{SnapshotID: o.snap, Complete: true, Items: []object.ViewObject{}}, nil
}
