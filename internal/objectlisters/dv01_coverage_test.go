// dv01_coverage_test.go pins DV01 list_all_pages: every
// required core V1 kind returns a typed envelope (Items /
// Issues / SnapshotID / Total / Complete) backed by a real
// source when the production catalog is available, and the
// per-kind DV01 honest-gap policy when it is not.
//
// The DV01 contract says: "an unsupported placeholder for a
// core kind fails the test". The WireGuard / Xray CRUD
// kinds (6 of the 19 required) are therefore FAIL when the
// catalog is missing — not PASS — until the production
// FileCatalog is wired. The test populates synthetic
// catalogs so every one of the 19 kinds can be exercised
// against a real source.
package objectlisters

import (
	"context"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/doctor"
	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/object"
	"github.com/CrazyDuckLab/DuckOS/internal/qos"
	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// dv01Engine is the minimal ListerSource the DV01 coverage
// test drives the engine-backed listers with. It is purely
// a fake for the engine IPC surface; the wireguard / xray
// listers never touch it.
type dv01Engine struct{}

func (dv01Engine) Doctor(ctx context.Context) (any, error)                 { return nil, nil }
func (dv01Engine) DoctorCtx(ctx context.Context) (any, error)             { return nil, nil }

// The engine-backed listers in this package use the
// real_listers.go ListerSource contract (Doctor +
// qos.Status). To avoid pulling the real Engine here the
// test drives the catalog-backed kinds directly through
// RegisterAllWith and asserts the registry contains a
// lister for every required kind; the per-lister logic
// has its own targeted tests in real_listers_test.go and
// wireguard_xray_listers_test.go.

// TestDV01EveryKindRegisteredWithRealLister proves every
// required core V1 kind has a source-aware lister
// registered by RegisterAllWithRealSources. The DV01
// contract is precise: "unsupported placeholder для
// core kind проваливает test". A placeholder is a
// lister that has no source-aware code and returns
// Complete=true on a missing source. The listers in this
// package are NOT placeholders: every one of them either
// delegates to a real ListerSource / FileCatalog /
// LinuxSource or, when the source is genuinely absent,
// reports Complete=false with the engine_unavailable
// Issue. Both behaviors are DV01-PASS per the contract's
// "runtime-unavailable subcase" requirement.
func TestDV01EveryKindRegisteredWithRealLister(t *testing.T) {
	required := []string{
		"qos.class",
		"network.interface",
		"network.route",
		"network.address",
		"network.firewall.rule",
		"network.firewall.set",
		"network.lease",
		"dns.zone",
		"dhcp.subnet",
		"bgp.peer",
		"ospf.area",
		"isis.area",
		"route.map",
		"wireguard.server",
		"wireguard.peer",
		"xray.server",
		"xray.user",
		"xray.profile",
		"xray.subscription",
	}
	e := engine.New("/tmp/duckos-dv01-nonexistent.sock", "test")
	wgDir := t.TempDir()
	xcDir := t.TempDir()
	wgCatalog, err := wireguard.OpenFileCatalog(wgDir)
	if err != nil {
		t.Fatalf("wireguard catalog: %v", err)
	}
	xcCatalog, err := xray.OpenFileCatalog(xcDir)
	if err != nil {
		t.Fatalf("xray catalog: %v", err)
	}
	seedWireguard(t, wgCatalog)
	seedXray(t, xcCatalog)
	// Drive the engine-backed listers (qos.class +
	// network.*) with a fake Doctor + QoS source so the
	// real listers project live rows.
	src := newDV01Source()
	// Drive the Linux-runtime listers (firewall / dhcp /
	// dns / anycast) with a fake LinuxSource that
	// returns synthetic rows; production wiring supplies
	// the real accessor on Linux.
	linuxSrc := newDV01LinuxSource()
	RegisterAllWithRealSourcesAndLinuxSource(e, wgCatalog, xcCatalog, src, linuxSrc)
	for _, kind := range required {
		col, err := e.ObjectList(context.Background(), kind, object.CollectionParams{})
		if err != nil {
			t.Fatalf("%s: ObjectList error: %v", kind, err)
		}
		if col.SnapshotID == "" {
			t.Fatalf("%s: SnapshotID must be set", kind)
		}
		if !col.Complete {
			t.Fatalf("%s: Complete=false on a real source-backed lister with a wired LinuxSource is a contract violation", kind)
		}
		if len(col.Items) == 0 {
			t.Fatalf("%s: real source-backed lister must project at least one item", kind)
		}
	}
}

// TestDV01RuntimeUnavailableSubcase exercises the DV01
// runtime-unavailable subcase: when the runtime source is
// genuinely absent (nil LinuxSource on a macOS runner),
// every Linux-runtime lister must report Complete=false
// with the engine_unavailable Issue — NEVER a fake-empty
// Complete=true.
func TestDV01RuntimeUnavailableSubcase(t *testing.T) {
	e := engine.New("/tmp/duckos-dv01-nonexistent.sock", "test")
	RegisterAllWithRealSourcesAndLinuxSource(e, nil, nil, dv01ListerSource{}, nil)
	linuxKinds := []string{
		"network.firewall.rule",
		"network.firewall.set",
		"network.lease",
		"dns.zone",
		"dhcp.subnet",
		"bgp.peer",
		"ospf.area",
		"isis.area",
		"route.map",
	}
	for _, kind := range linuxKinds {
		col, _ := e.ObjectList(context.Background(), kind, object.CollectionParams{})
		if col.Complete {
			t.Fatalf("%s: Complete=true on missing source is a contract violation (placeholder проваливает test)", kind)
		}
		if col.SnapshotID == "" {
			t.Fatalf("%s: SnapshotID must be set on unavailable envelope", kind)
		}
		found := false
		for _, iss := range col.Issues {
			if iss.ReasonID == "engine_unavailable" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s: must carry engine_unavailable Issue when LinuxSource is nil", kind)
		}
	}
}

// dv01LinuxSource is a fake LinuxSource the DV01 test
// drives the 9 Linux-runtime listers with. Each method
// returns a deterministic synthetic row set so the test
// can assert Complete=true envelopes with real items.
type dv01LinuxSource struct{}

func newDV01LinuxSource() LinuxSource { return dv01LinuxSource{} }

func (dv01LinuxSource) FirewallRules(_ context.Context) ([]object.ViewObject, error) {
	return []object.ViewObject{{
		Ref:          object.ObjectRef{Kind: "network.firewall.rule", ID: "synthetic-rule-1"},
		DisplayName:  "synthetic-rule-1",
		Desired:      map[string]any{"family": "inet", "table": "filter", "chain": "input"},
		Availability: object.Available,
	}}, nil
}

func (dv01LinuxSource) FirewallSets(_ context.Context) ([]object.ViewObject, error) {
	return []object.ViewObject{{
		Ref:          object.ObjectRef{Kind: "network.firewall.set", ID: "synthetic-set-1"},
		DisplayName:  "synthetic-set-1",
		Desired:      map[string]any{"family": "inet", "table": "filter", "name": "allowed"},
		Availability: object.Available,
	}}, nil
}

func (dv01LinuxSource) DHCPLeases(_ context.Context) ([]object.ViewObject, error) {
	return []object.ViewObject{{
		Ref:          object.ObjectRef{Kind: "network.lease", ID: "synthetic-lease-1"},
		DisplayName:  "10.0.0.42",
		Desired:      map[string]any{"address": "10.0.0.42", "mac": "aa:bb:cc:dd:ee:ff"},
		Availability: object.Available,
	}}, nil
}

func (dv01LinuxSource) DNSZones(_ context.Context) ([]object.ViewObject, error) {
	return []object.ViewObject{{
		Ref:          object.ObjectRef{Kind: "dns.zone", ID: "synthetic-zone-1"},
		DisplayName:  "duckos.local",
		Desired:      map[string]any{"name": "duckos.local", "kind": "forward"},
		Availability: object.Available,
	}}, nil
}

func (dv01LinuxSource) DHCPSubnets(_ context.Context) ([]object.ViewObject, error) {
	return []object.ViewObject{{
		Ref:          object.ObjectRef{Kind: "dhcp.subnet", ID: "synthetic-subnet-1"},
		DisplayName:  "10.0.0.0/24",
		Desired:      map[string]any{"cidr": "10.0.0.0/24", "router": "10.0.0.1"},
		Availability: object.Available,
	}}, nil
}

func (dv01LinuxSource) AnycastBGPPeers(_ context.Context) ([]object.ViewObject, error) {
	return []object.ViewObject{{
		Ref:          object.ObjectRef{Kind: "bgp.peer", ID: "synthetic-bgp-1"},
		DisplayName:  "synthetic-bgp-1",
		Desired:      map[string]any{"addressFamily": "ipv4"},
		Availability: object.Available,
	}}, nil
}

func (dv01LinuxSource) AnycastOSPFAreas(_ context.Context) ([]object.ViewObject, error) {
	return []object.ViewObject{{
		Ref:          object.ObjectRef{Kind: "ospf.area", ID: "synthetic-ospf-1"},
		DisplayName:  "0.0.0.1",
		Desired:      map[string]any{"areaId": "0.0.0.1"},
		Availability: object.Available,
	}}, nil
}

func (dv01LinuxSource) AnycastISISAreas(_ context.Context) ([]object.ViewObject, error) {
	return []object.ViewObject{{
		Ref:          object.ObjectRef{Kind: "isis.area", ID: "synthetic-isis-1"},
		DisplayName:  "49.0001",
		Desired:      map[string]any{"areaId": "49.0001"},
		Availability: object.Available,
	}}, nil
}

func (dv01LinuxSource) AnycastRouteMaps(_ context.Context) ([]object.ViewObject, error) {
	return []object.ViewObject{{
		Ref:          object.ObjectRef{Kind: "route.map", ID: "synthetic-rmap-1"},
		DisplayName:  "synthetic-rmap-1",
		Desired:      map[string]any{"action": "permit"},
		Availability: object.Available,
	}}, nil
}

// TestDV01WireguardCatalogRealLister pins the 6
// WireGuard / Xray CRUD kinds: a real FileCatalog must
// produce Complete=true with the projected rows; a nil
// catalog must report honest engine_unavailable — NEVER
// a fake-empty Complete=true.
func TestDV01WireguardCatalogRealLister(t *testing.T) {
	e := engine.New("/tmp/duckos-dv01-nonexistent.sock", "test")

	// Nil-catalog case: every kind must report
	// engine_unavailable, never Complete=true.
	RegisterAllWith(e, nil, nil)
	wgKinds := []string{"wireguard.server", "wireguard.peer"}
	xrayKinds := []string{"xray.server", "xray.user", "xray.profile", "xray.subscription"}
	for _, kind := range append(append([]string{}, wgKinds...), xrayKinds...) {
		col, _ := e.ObjectList(context.Background(), kind, object.CollectionParams{})
		if col.Complete {
			t.Fatalf("%s with nil catalog must NOT claim Complete=true (contract: placeholder проваливает test)", kind)
		}
		found := false
		for _, iss := range col.Issues {
			if iss.ReasonID == "engine_unavailable" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s: must carry engine_unavailable Issue when catalog is missing", kind)
		}
	}

	// Real-catalog case: populate synthetic data and
	// assert every kind reflects it as Complete=true with
	// real rows.
	wgDir := t.TempDir()
	xcDir := t.TempDir()
	wgCatalog, err := wireguard.OpenFileCatalog(wgDir)
	if err != nil {
		t.Fatalf("wireguard catalog: %v", err)
	}
	xcCatalog, err := xray.OpenFileCatalog(xcDir)
	if err != nil {
		t.Fatalf("xray catalog: %v", err)
	}
	seedWireguard(t, wgCatalog)
	seedXray(t, xcCatalog)
	RegisterAllWith(e, wgCatalog, xcCatalog)

	for _, kind := range wgKinds {
		col, err := e.ObjectList(context.Background(), kind, object.CollectionParams{})
		if err != nil {
			t.Fatalf("%s: ObjectList error: %v", kind, err)
		}
		if !col.Complete {
			t.Fatalf("%s with real catalog must report Complete=true", kind)
		}
		if len(col.Items) == 0 {
			t.Fatalf("%s with real catalog must project at least one item", kind)
		}
	}
	for _, kind := range xrayKinds {
		col, err := e.ObjectList(context.Background(), kind, object.CollectionParams{})
		if err != nil {
			t.Fatalf("%s: ObjectList error: %v", kind, err)
		}
		if !col.Complete {
			t.Fatalf("%s with real catalog must report Complete=true", kind)
		}
		if len(col.Items) == 0 {
			t.Fatalf("%s with real catalog must project at least one item", kind)
		}
	}
}

// dv01Source is the minimal ListerSource used to drive
// the engine-backed listers (qos.class, network.interface,
// network.route, network.address). The fakeDoctor/fakeQoS
// responses are deterministic; the test asserts every
// engine-backed kind projects real rows.
type dv01Source struct{}

func newDV01Source() ListerSource { return dv01ListerSource{} }

type dv01ListerSource struct{}

func (dv01ListerSource) Doctor(_ context.Context) (doctor.Snapshot, error) {
	return doctor.Snapshot{
		Network: doctor.Network{
			Interfaces: []doctor.Interface{
				{Name: "eth0", Index: 1, LinkUp: true, MTU: 1500, Addresses: []string{"10.0.0.1/24"}, OperState: "up"},
			},
			Routes: []doctor.Route{
				{Destination: "default", Gateway: "10.0.0.254", Device: "eth0"},
			},
		},
	}, nil
}

func (dv01ListerSource) QoS(_ context.Context) (qos.Status, error) {
	return qos.Status{
		Topology: qos.Topology{
			SchemaVersion: qos.SchemaVersion,
			Classes:       []qos.Class{{ID: "voice", ShaperID: "wan", Name: "Voice"}},
		},
	}, nil
}

func seedWireguard(t *testing.T, c *wireguard.FileCatalog) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	srv := wireguard.Server{
		ID:         "wg-srv-1",
		Name:       "primary",
		Interface:  "wg0",
		ListenPort: 51820,
		Addresses:  []string{"10.66.66.1/24"},
		PublicKey:  "PUBKEY-SYNTHETIC-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := c.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("seed wg server: %v", err)
	}
	if err := c.UpsertPeer(ctx, wireguard.Peer{
		ID:         "wg-peer-1",
		ServerID:   srv.ID,
		Name:       "alice",
		PublicKey:  "PUBKEY-SYNTHETIC-BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
		AllowedIPs: []string{"10.66.66.2/32"},
		Intent:     wireguard.IntentFullTunnel,
		Enabled:    true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("seed wg peer: %v", err)
	}
}

func seedXray(t *testing.T, c *xray.FileCatalog) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	srv := xray.Server{
		ID:        "xray-srv-1",
		Name:      "vless-reality",
		Protocol:  xray.ProtocolVLESS,
		Transport: xray.TransportTCP,
		Security:  xray.SecurityReality,
		Listen:    "0.0.0.0",
		Port:      443,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := c.UpsertServer(ctx, srv); err != nil {
		t.Fatalf("seed xray server: %v", err)
	}
	if err := c.UpsertUser(ctx, xray.User{
		ID:        "xray-user-1",
		ServerID:  srv.ID,
		Name:      "alice",
		UUID:      "synthetic-uuid-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed xray user: %v", err)
	}
	if err := c.UpsertProfile(ctx, xray.Profile{
		ID:        "xray-profile-1",
		Name:      "default",
		UserID:    "xray-user-1",
		Intent:    xray.ProfileIntentFullTunnel,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed xray profile: %v", err)
	}
	if err := c.UpsertSubscription(ctx, xray.Subscription{
		ID:         "xray-sub-1",
		Name:       "main",
		ProfileIDs: []string{"xray-profile-1"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("seed xray subscription: %v", err)
	}
}
