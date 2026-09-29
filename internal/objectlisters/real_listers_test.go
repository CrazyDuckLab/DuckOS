// real_listers_test.go pins the production wiring of the real
// listers (G06): qos.class, network.interface and network.route
// MUST project live Engine data into the object.Collection
// envelope. The tests use a stub ListerSource implementation
// that returns canned Doctor and QoS payloads; the lister is
// the real production code path.
package objectlisters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
)

// fakeListerSource is a stub implementation of ListerSource
// that returns canned Doctor and QoS data so the real listers
// can be exercised without booting a real Agent IPC channel.
type fakeListerSource struct {
	doctor doctor.Snapshot
	qos    qos.Status
	err    error
}

func (f *fakeListerSource) Doctor(context.Context) (doctor.Snapshot, error) {
	if f.err != nil {
		return doctor.Snapshot{}, f.err
	}
	return f.doctor, nil
}

func (f *fakeListerSource) QoS(context.Context) (qos.Status, error) {
	if f.err != nil {
		return qos.Status{}, f.err
	}
	return f.qos, nil
}

// objectListKinds returns the canonical list of kinds the real
// listers cover. The test exercises the production projection
// code path for each.
func objectListKinds() []string {
	return []string{
		"qos.class",
		"network.interface",
		"network.route",
	}
}

// TestRealListers_ProjectQoSClasses proves the qos.class lister
// delegates to the Engine and produces a ViewObject per
// canonical qos.Class.
func TestRealListers_ProjectQoSClasses(t *testing.T) {
	src := &fakeListerSource{
		qos: qos.Status{
			Topology: qos.Topology{
				SchemaVersion: qos.SchemaVersion,
				Classes: []qos.Class{
					{ID: "voice", ShaperID: "wan", Name: "Voice traffic", MarkBits: 7, RateShare: 30},
					{ID: "bulk", ShaperID: "wan", Name: "Bulk transfer", MarkBits: 1, RateShare: 5},
				},
				GeneratedAt: time.Now().Unix(),
			},
			LastApply:  time.Now().Unix(),
			HasShapers: 1,
		},
	}
	lister := NewQoSClassLister(src)
	col, err := lister.List(context.Background(), object.CollectionParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(col.Items) != 2 {
		t.Fatalf("Items: %d, want 2", len(col.Items))
	}
	if col.Items[0].Ref.Kind != "qos.class" || col.Items[0].Ref.ID != "voice" {
		t.Fatalf("first item ref: %+v", col.Items[0].Ref)
	}
	if col.Items[0].Availability != object.Available {
		t.Fatalf("availability: %q, want available", col.Items[0].Availability)
	}
	if !col.Items[0].Permissions.Observe {
		t.Fatal("observe permission must be true for a real lister")
	}
}

// TestRealListers_ProjectNetworkInterfaces proves the
// network.interface lister delegates to the Engine and produces
// a ViewObject per live interface.
func TestRealListers_ProjectNetworkInterfaces(t *testing.T) {
	src := &fakeListerSource{
		doctor: doctor.Snapshot{
			GeneratedAt: time.Now().UTC(),
			Network: doctor.Network{
				Interfaces: []doctor.Interface{
					{Name: "eth0", Index: 1, LinkUp: true, MTU: 1500, Addresses: []string{"10.0.0.1/24"}, OperState: "up"},
					{Name: "eth1", Index: 2, LinkUp: false, MTU: 1500, Addresses: []string{}, OperState: "down"},
				},
			},
		},
	}
	lister := NewNetworkInterfaceLister(src)
	col, err := lister.List(context.Background(), object.CollectionParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(col.Items) != 2 {
		t.Fatalf("Items: %d, want 2", len(col.Items))
	}
	if col.Items[0].Ref.ID != "eth0" {
		t.Fatalf("first item id: %q", col.Items[0].Ref.ID)
	}
	if col.Items[1].Tags[0] != "down" {
		t.Fatalf("second item tag: %v, want down", col.Items[1].Tags)
	}
}

// TestRealListers_ProjectNetworkRoutes proves the network.route
// lister delegates to the Engine and produces a ViewObject per
// live route.
func TestRealListers_ProjectNetworkRoutes(t *testing.T) {
	src := &fakeListerSource{
		doctor: doctor.Snapshot{
			GeneratedAt: time.Now().UTC(),
			Network: doctor.Network{
				Routes: []doctor.Route{
					{Destination: "default", Gateway: "10.0.0.254", Device: "eth0", Metric: 100},
					{Destination: "10.0.0.0/24", Gateway: "0.0.0.0", Device: "eth0", Metric: 0},
				},
			},
		},
	}
	lister := NewNetworkRouteLister(src)
	col, err := lister.List(context.Background(), object.CollectionParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(col.Items) != 2 {
		t.Fatalf("Items: %d, want 2", len(col.Items))
	}
	if col.Items[0].Ref.ID != "default" {
		t.Fatalf("first item id: %q", col.Items[0].Ref.ID)
	}
}

// TestRealListers_EngineErrorSurfacesAsIssue proves the lister
// surfaces a structured Issue when the source call fails, so
// the UI can render the gap honestly.
func TestRealListers_EngineErrorSurfacesAsIssue(t *testing.T) {
	src := &fakeListerSource{err: errors.New("agent unreachable")}
	for _, kind := range objectListKinds() {
		var lister interface {
			List(context.Context, object.CollectionParams) (object.Collection, error)
		}
		switch kind {
		case "qos.class":
			lister = NewQoSClassLister(src)
		case "network.interface":
			lister = NewNetworkInterfaceLister(src)
		case "network.route":
			lister = NewNetworkRouteLister(src)
		}
		col, err := lister.List(context.Background(), object.CollectionParams{})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if len(col.Items) != 0 {
			t.Fatalf("%s: Items: %d, want 0", kind, len(col.Items))
		}
		found := false
		for _, issue := range col.Issues {
			if issue.ReasonID == "engine_unavailable" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s: issues=%v must include engine_unavailable", kind, col.Issues)
		}
	}
}

// TestRealListers_LimitAndCursor prove the pagination
// envelope honours the caller's Limit and the opaque After
// cursor. The real production code path clamps to
// object.MaxCollectionItems.
func TestRealListers_LimitAndCursor(t *testing.T) {
	src := &fakeListerSource{
		doctor: doctor.Snapshot{
			GeneratedAt: time.Now().UTC(),
			Network: doctor.Network{
				Routes: []doctor.Route{
					{Destination: "default", Gateway: "10.0.0.254"},
					{Destination: "10.0.0.0/24", Gateway: "0.0.0.0"},
					{Destination: "192.168.1.0/24", Gateway: "0.0.0.0"},
				},
			},
		},
	}
	lister := NewNetworkRouteLister(src)
	col, err := lister.List(context.Background(), object.CollectionParams{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(col.Items) != 1 {
		t.Fatalf("Limit=1 must clamp to 1 item, got %d", len(col.Items))
	}
	if col.Total == nil || *col.Total != 3 {
		t.Fatalf("Total: %v, want 3", col.Total)
	}
	// Cursor: the second page must contain the next item.
	col2, err := lister.List(context.Background(), object.CollectionParams{Limit: 1, After: encodeCursor(1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(col2.Items) != 1 {
		t.Fatalf("After=1 page: %d, want 1", len(col2.Items))
	}
	if col2.Items[0].Ref.ID != "10.0.0.0/24" {
		t.Fatalf("After=1 first item: %q, want 10.0.0.0/24", col2.Items[0].Ref.ID)
	}
}
