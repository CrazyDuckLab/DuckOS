package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestQualifiedRefreshRejectsForeignInputsBeforeCommand(t *testing.T) {
	for _, mode := range []string{"valid", "foreign", "netdev", "dropin", "changed", "symlink", "dynamic", "lower-priority"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := staticTrafficFixture()
			p, err := Render(c)
			if err != nil {
				t.Fatal(err)
			}
			root := PrivateTempDir(t)
			vendor := PrivateTempDir(t)
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			for name, body := range p.Networkd {
				if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "foreign":
				must(os.WriteFile(filepath.Join(root, "20-foreign.network"), []byte("[Match]\nName=eth0\n"), 0600))
			case "netdev":
				must(os.WriteFile(filepath.Join(root, "foo.netdev"), []byte("[NetDev]\nName=eth0\n"), 0600))
			case "dropin":
				must(os.Mkdir(filepath.Join(root, "10-duckos-inside.network.d"), 0700))
			case "changed":
				must(os.WriteFile(filepath.Join(root, "10-duckos-inside.network"), []byte("foreign"), 0600))
			case "symlink":
				must(os.Remove(filepath.Join(root, "10-duckos-inside.network")))
				must(os.Symlink(filepath.Join(root, "10-duckos-uplink.network"), filepath.Join(root, "10-duckos-inside.network")))
			case "dynamic":
				c.Interfaces[0].DHCPClient = "ipv4"
			case "lower-priority":
				must(os.WriteFile(filepath.Join(vendor, "20-foreign.network"), []byte("[Match]\nName=*\n"), 0600))
			}
			err = qualifyNetworkdFiles([]string{root, vendor}, c, c)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("qualification %s: %v", mode, err)
			}
		})
	}
}

// TestQualifiedRefreshAllowsVendorAmbientProfiles pins the production
// correction: a non-first-root .network file whose [Match] clause
// targets an unrelated Kind / Name cannot claim a DuckOS-managed
// link. The strict body-equality rule stays in force for profiles
// that COULD claim; only vendor-ambient defaults that target nothing
// DuckOS owns pass through unchanged. This is the path Debian and
// other distributions use to ship wired defaults for tun, ve-, sit,
// wifi-* and similar namespaces without contending with the
// standalone physical NICs that DuckOS manages.
func TestQualifiedRefreshAllowsVendorAmbientProfiles(t *testing.T) {
	c, _ := staticTrafficFixture()
	p, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	root := PrivateTempDir(t)
	vendor := PrivateTempDir(t)
	for name, body := range p.Networkd {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	vendorProfiles := map[string]string{
		"80-vm-vt.network":            "[Match]\nKind=tun\nName=vt-*\n",
		"80-container-host0.network": "[Match]\nKind=veth\nName=host0\nVirtualization=container\n",
		"80-6rd-tunnel.network":      "[Match]\nKind=sit\nName=6rd-*\n",
		"80-wifi-adhoc.network":      "[Match]\nKind=wlan\nName=adhoc-*\n",
	}
	for name, body := range vendorProfiles {
		if err := os.WriteFile(filepath.Join(vendor, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := qualifyNetworkdFiles([]string{root, vendor}, c, c); err != nil {
		t.Fatalf("vendor-ambient profiles in non-first root must not be refused: %v", err)
	}
}

// TestQualifiedRefreshRejectsForeignClaimInVendorRoot proves the
// strict rule still catches a non-first-root profile whose [Match]
// DOES name a DuckOS-managed kind / namespace. The vendor-ambient
// exemption is narrow: a profile that could claim a DuckOS-managed
// link still must be exact renderer output.
func TestQualifiedRefreshRejectsForeignClaimInVendorRoot(t *testing.T) {
	c, _ := staticTrafficFixture()
	p, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	root := PrivateTempDir(t)
	vendor := PrivateTempDir(t)
	for name, body := range p.Networkd {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(vendor, "20-foreign.network"),
		[]byte("[Match]\nKind=physical\nName=testlan0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := qualifyNetworkdFiles([]string{root, vendor}, c, c); err == nil {
		t.Fatal("foreign claim in non-first root was admitted")
	}
}

// TestQualifiedRefreshAcceptsNewCandidateProfileNotYetOnDisk pins the
// fresh-install adoption path: when the durable before-config is
// empty and the candidate introduces a new managed link, the
// candidate's profile is about to be written. The qualifier must not
// refuse the apply because the profile isn't yet on disk. The same
// profile would be checked again post-apply to ensure it landed;
// that is the verifier's job, not this preflight gate.
func TestQualifiedRefreshAcceptsNewCandidateProfileNotYetOnDisk(t *testing.T) {
	empty := Config{Version: 1}
	next := Config{Version: 1, Interfaces: []Interface{
		{ID: "inside", Name: "testlan0", Kind: "physical", Role: "lan", MTU: 1500, DHCPClient: "none", Management: true, Addresses: []string{"198.51.100.1/24"}},
	}}
	root := PrivateTempDir(t)
	if err := qualifyNetworkdFiles([]string{root}, empty, next); err != nil {
		t.Fatalf("new adoption must not be refused because the file isn't yet on disk: %v", err)
	}
	// Sanity: full path with vendor root also passes — neither
	// file nor claim check produces a refusal.
	vendor := PrivateTempDir(t)
	if err := os.WriteFile(filepath.Join(vendor, "80-vm-vt.network"),
		[]byte("[Match]\nKind=tun\nName=vt-*\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := qualifyNetworkdFiles([]string{root, vendor}, empty, next); err != nil {
		t.Fatalf("vendor-ambient profiles must not block fresh-install adoption: %v", err)
	}
}

func TestQualifiedRefreshCannotFallBackAfterRefusal(t *testing.T) {
	h, c, calls := reloadFixture(t)
	h.qualify = func(context.Context, Config, Config) error { return errors.New("foreign profile") }
	if err := h.ReloadScoped(context.Background(), c, c, map[string]bool{"inside": true}); err == nil || *calls != 0 {
		t.Fatal("unqualified reload executed", err, *calls)
	}
}

func TestQualifiedRefreshNeedsKernelConvergence(t *testing.T) {
	h, c, calls := reloadFixture(t)
	h.verifyRuntime = func(context.Context, Config) error { return errors.New("old addresses still live") }
	h.wait = func(context.Context) error { return errors.New("bounded stop") }
	if err := h.ReloadScoped(context.Background(), c, c, map[string]bool{"inside": true}); err == nil || *calls != 1 {
		t.Fatal("profile-name readback hid stale kernel state", err, *calls)
	}
}

func TestQualifiedRefreshRejectsProtectedStateDrift(t *testing.T) {
	h, c, calls := reloadFixture(t)
	// The protected check is performed exactly twice in this path:
	// once to capture the baseline signature before any reload, and
	// once after convergence succeeds. Returning "changed" on the
	// post-convergence check is what must abort the apply. Returning
	// "before" on the baseline keeps the apply moving forward; a
	// brief transient "changed" mid-loop must NOT abort the apply,
	// because networkd's internal re-evaluation of eth0 during a
	// reload can briefly perturb DHCP lease timers or other transient
	// state that the protected snapshot captures. The invariant
	// under test is end-state equality, not per-iteration equality.
	h.protected = func(context.Context, Config) (string, error) {
		return "before", nil
	}
	if err := h.ReloadScoped(context.Background(), c, c, map[string]bool{"inside": true}); err != nil {
		t.Fatal("baseline-equal protected state should converge:", err)
	}
	// Now flip the protected signature to "changed" and verify the
	// post-convergence check refuses the apply.
	h, c, calls = reloadFixture(t)
	reads := 0
	h.protected = func(context.Context, Config) (string, error) {
		reads++
		if reads == 1 {
			return "before", nil
		}
		return "changed", nil
	}
	if err := h.ReloadScoped(context.Background(), c, c, map[string]bool{"inside": true}); err == nil || *calls != 1 {
		t.Fatal("post-convergence protected drift accepted", err, *calls)
	}
}

func TestQualifiedRefreshLegacyEntriesCannotBypassQualification(t *testing.T) {
	for _, restore := range []bool{false, true} {
		h, c, calls := reloadFixture(t)
		h.qualify = func(context.Context, Config, Config) error { return errors.New("not admitted") }
		var err error
		if restore {
			err = h.ReloadRestored(context.Background(), c)
		} else {
			err = h.Reload(context.Background(), c, c)
		}
		if err == nil || *calls != 0 {
			t.Fatal("legacy entry bypassed qualification", err, *calls)
		}
	}
}

func TestQualifiedRefreshPreservesKernelIdentity(t *testing.T) {
	c := fixture()
	initial := syntheticNetworkdInventory()
	for _, mode := range []string{"index", "kind", "profile", "dropin", "failed"} {
		t.Run(mode, func(t *testing.T) {
			observed := append([]NetworkdLink(nil), initial...)
			switch mode {
			case "index":
				observed[0].Index++
			case "kind":
				observed[0].Kind = "dummy"
			case "profile":
				observed[0].NetworkFile = "foreign"
			case "dropin":
				observed[0].NetworkFileDropins = []string{"override.conf"}
			case "failed":
				observed[0].AdministrativeState = "failed"
			}
			if err := checkRefreshedProfiles(c, initial, observed); err == nil {
				t.Fatal("lost link identity or profile admitted")
			}
		})
	}
}

func TestManagedRoutingRuntimeEvidenceSurvivesDecodeAndRejectsDrift(t *testing.T) {
	a, b, n, _ := managedRoutingFixture(t)
	before, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	s := HostSnapshot{Version: 1, CapturedAt: time.Now()}
	for index, i := range before.Interfaces {
		s.Links = append(s.Links, HostLink{Name: i.Name, Index: index + 2, MAC: fmt.Sprintf("02:00:00:00:00:%02x", index+1), MTU: i.MTU, Up: true, Addresses: append([]string(nil), i.Addresses...)})
	}
	a.captureRuntime = func(context.Context, Config, Config) (json.RawMessage, error) { return json.Marshal(s) }
	raw, err := a.Capture(context.Background(), b, n)
	if err != nil {
		t.Fatal(err)
	}
	e, _, _, err := a.decode(b, n, raw)
	if err != nil || len(e.Runtime) == 0 {
		t.Fatal("runtime evidence not durable", err)
	}
	s.Links[0].Addresses = []string{"203.0.113.99/24"}
	e.Runtime, err = json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := a.decode(b, n, bad); err == nil {
		t.Fatal("recovery evidence drift accepted")
	}
	if _, err := a.Capture(context.Background(), b, n); err == nil {
		t.Fatal("runtime drift admitted before writes")
	}
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// TestQualifiedRefreshAcceptsUnownedLinkWithStaleNetworkFile pins the
// recovery-path convergence rule: networkctl status reports the
// LAST-KNOWN NetworkFile path even after the file has been removed from
// disk; the manager's view lags the file system. After a Safe Apply
// rolls back, the on-disk DuckOS .network files are gone but networkd's
// in-memory state still points at the deleted paths and reports
// AdministrativeState="configured". The qualified-refresh admission gate
// must treat such a link as effectively unmanaged (and let the recovery
// converge) when the NetworkFile path is missing from disk. A link that
// still has an EXISTING .network file on disk is the dangerous case and
// must still be rejected.
//
// The qualified-refresh admission depends on a Linux networkd version
// check (`/usr/lib/systemd/systemd-networkd --version`); outside Linux
// the early version gate fails before the on-disk check can run, so the
// regression test only exercises the gate on Linux.
func TestQualifiedRefreshAcceptsUnownedLinkWithStaleNetworkFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("qualified networkd refresh runs only on Linux")
	}
	h, _, _ := reloadFixture(t)
	e := syntheticEnrollment()
	e.NetworkdReload = true
	e.Physical = []PhysicalBinding{
		{ID: "eth0", Name: "eth0", MAC: "00:15:5d:04:07:07", ExternallyManaged: true},
		{ID: "eth1", Name: "eth1", MAC: "00:15:5d:04:07:10"},
		{ID: "eth2", Name: "eth2", MAC: "00:15:5d:04:07:11"},
	}
	h.enrollment = e
	// The stale-but-missing path must be a path the test owns, never a
	// real path under /etc/systemd/network. Using a tmpdir path keeps
	// the test deterministic on hosts where real DuckOS .network files
	// exist on disk from a prior Safe Apply (the file is missing from
	// this tmpdir because we never wrote it).
	staleRoot := PrivateTempDir(t)
	stalePath := filepath.Join(staleRoot, "10-duckos-stale-eth1.network")
	h.inventory = func(context.Context) ([]NetworkdLink, error) {
		return []NetworkdLink{
			{Name: "eth0", AdministrativeState: "unmanaged"},
			{
				Name:                "eth1",
				AdministrativeState: "configured",
				NetworkFile:         stalePath,
			},
		}, nil
	}
	h.protected = func(context.Context, Config) (string, error) { return "unchanged", nil }
	// Override networkdConfigRoots so qualifyRefresh reads from isolated
	// tmpdirs and not from a host's real /etc/systemd/network that may
	// already contain legitimate DuckOS profiles. The package var is
	// restored at test end.
	prevRoots := networkdConfigRoots
	networkdConfigRoots = []string{staleRoot}
	t.Cleanup(func() { networkdConfigRoots = prevRoots })
	// Drop the qualify stub so the real on-disk-existence gate runs.
	h.qualify = h.qualifyRefresh
	if err := h.ReloadScopedRestored(context.Background(), Config{Version: 1}, map[string]bool{"eth1": true}); err != nil {
		t.Fatalf("recovery rejected stale-but-missing NetworkFile: %v", err)
	}
	// The dangerous case: a real .network file exists on disk and
	// points at an unowned link. The check must still fail closed.
	tmp := PrivateTempDir(t)
	realPath := filepath.Join(tmp, "10-duckos-rogue.network")
	if err := os.WriteFile(realPath, []byte("[Match]\nName=eth1\n\n[Network]\nAddress=10.0.0.1/24\n"), 0600); err != nil {
		t.Fatal(err)
	}
	h2, _, _ := reloadFixture(t)
	h2.enrollment = e
	h2.inventory = func(context.Context) ([]NetworkdLink, error) {
		return []NetworkdLink{
			{Name: "eth0", AdministrativeState: "unmanaged"},
			{
				Name:                "eth1",
				AdministrativeState: "configured",
				NetworkFile:         realPath,
			},
		}, nil
	}
	h2.protected = func(context.Context, Config) (string, error) { return "unchanged", nil }
	h2.qualify = h2.qualifyRefresh
	if err := h2.ReloadScopedRestored(context.Background(), Config{Version: 1}, map[string]bool{"eth1": true}); err == nil {
		t.Fatalf("recovery accepted unowned link with a real .network file on disk: realPath=%q exists=%v", realPath, fileExists(realPath))
	}
}
