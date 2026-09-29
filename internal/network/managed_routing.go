package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"sort"
	"strings"
	"sync"
)

type routingDaemon interface {
	PreflightScoped(context.Context, Config, Config) error
	Reload(context.Context, Config, Config) error
	ReloadRestored(context.Context, Config) error
	// ReloadScoped validates the full before/next pair and qualifies the
	// manager's complete input set before cache refresh. Only changed owned
	// artifacts are written; external links must remain unmatched/unmanaged.
	ReloadScoped(context.Context, Config, Config, map[string]bool) error
	ReloadScopedRestored(context.Context, Config, map[string]bool) error
}

// ManagedRoutingAdapter implements the Safe Apply adapter contract for static,
// already-owned interfaces. It joins persistent artifacts, daemon convergence
// and actual kernel mutation. Link/DNS/main-table changes remain separate work.
// Activation requires an outer boot-recovery gate and management verifier.
type ManagedRoutingAdapter struct {
	mu              sync.Mutex
	kernel          *RoutedKernel
	files           *ArtifactDirectory
	dns             *ArtifactDirectory
	daemon          routingDaemon
	checkOwnership  func(context.Context, Config, Config) error
	checkManagement func(context.Context, Config) error
	enrollment      Enrollment
	captured        string
	verifiedConfig  string
	verifiedNFT     NFTSnapshot
	verifiedRoutes  RouteSnapshot
	verifiedRules   PolicySnapshot
	// scopedChanged captures the interface IDs that the most recent
	// Apply actually wrote or replaced. verifyStage and Verify use it
	// to bound the disk consistency check so unchanged (and externally
	// managed) links remain outside the transaction.
	scopedChanged  map[string]bool
	close          func() error
	captureRuntime func(context.Context, Config, Config) (json.RawMessage, error)
	// liveLinks is the read-only host-kernel link snapshotter the
	// adapter consults during preflight to adjudicate the
	// "adoption vs. creation" boundary. When nil the adoption check
	// fires as a fail-closed refusal (no adoption ever admitted)
	// rather than silently dropping the rule. The production
	// factory wires it from netlink; tests inject a synthetic.
	liveLinks LiveLinkAdopter
}

// The management probe and boot-ordering guard are trusted Agent dependencies,
// never API supplied booleans. Both are mandatory before runtime activation.
func NewManagedRoutingAdapter(privateAgentDirectory string, verifyManagement func(context.Context, Config) error, verifyBootGuard func(context.Context) error) (*ManagedRoutingAdapter, error) {
	if verifyManagement == nil || verifyBootGuard == nil {
		return nil, errors.New("management and boot recovery guards are required")
	}
	e, err := ReadEnrollment(privateAgentDirectory)
	if err != nil {
		return nil, err
	}
	k, err := NewRoutedKernel(e)
	if err != nil {
		return nil, err
	}
	d, err := NewNetworkdReloadHost(e)
	if err != nil {
		return nil, err
	}
	a := &ManagedRoutingAdapter{kernel: k, files: d.files, daemon: d, checkManagement: verifyManagement, enrollment: e, close: d.Close, captureRuntime: CaptureHost}
	// liveLinks is intentionally nil here. The production factory in
	// internal/agent wires it from netlink before the adapter is given
	// to the controller. Leaving it nil keeps every existing caller
	// fail-closed at the adoption boundary rather than silently
	// admitting unknown links.
	a.checkOwnership = func(ctx context.Context, b, n Config) error {
		if err := verifyBootGuard(ctx); err != nil {
			return err
		}
		raw, err := CaptureHost(ctx, b, n)
		if err != nil {
			return err
		}
		live, err := DecodeHostSnapshot(raw, b, n)
		if err != nil {
			return err
		}
		if err := e.CheckBindings(b, n, live); err != nil {
			return err
		}
		inventory, err := ReadNetworkdInventory(ctx)
		if err != nil {
			return err
		}
		return CheckNetworkdOwnership(b, n, inventory)
	}
	return a, nil
}

// SetLiveLinkAdopter wires the host-kernel link snapshotter the
// adoption boundary consults during preflight. The setter is the only
// production entry point and is intentionally separate from the
// constructor so every existing caller remains fail-closed without a
// live adopter installed. The snapshotter must be non-nil for the
// boundary to admit adoptions; passing nil returns the adapter to its
// fail-closed state. The setter is not goroutine-safe across
// concurrent preflight/Capture/Apply and must run during construction.
func (a *ManagedRoutingAdapter) SetLiveLinkAdopter(adopter LiveLinkAdopter) error {
	if a == nil {
		return errors.New("managed routing adapter is not initialised")
	}
	if adopter == nil {
		a.liveLinks = nil
		return nil
	}
	a.liveLinks = adopter
	return nil
}

func (a *ManagedRoutingAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.close == nil {
		return nil
	}
	return a.close()
}

// AttachDNSArtifacts wires a separate DNS service artifact directory into
// the managed routing adapter. The DNS service is independent of networkd
// but must move in lockstep with managed network changes: the Safe Apply
// transaction covers both so a partial apply cannot leave the host with
// a stale dnsmasq configuration.
func (a *ManagedRoutingAdapter) AttachDNSArtifacts(d *ArtifactDirectory) error {
	if d == nil {
		return errors.New("DNS artifacts directory is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dns = d
	return nil
}

// replaceDNS is a no-op when the adapter has no DNS artifacts wired in.
// When it does, the dnsmasq configuration is swapped exactly like a
// networkd unit: the file is removed if empty, otherwise the new contents
// are staged in.
func (a *ManagedRoutingAdapter) replaceDNS(before, next string) error {
	if a.dns == nil {
		return nil
	}
	files := map[string]string{}
	if next != "" {
		files["dnsmasq.conf"] = next
	}
	beforeFiles := map[string]string{}
	if before != "" {
		beforeFiles["dnsmasq.conf"] = before
	}
	return a.dns.Replace(beforeFiles, files)
}

func (a *ManagedRoutingAdapter) restoreDNS(before, next string) error {
	if a.dns == nil {
		return nil
	}
	files := map[string]string{}
	if next != "" {
		files["dnsmasq.conf"] = next
	}
	beforeFiles := map[string]string{}
	if before != "" {
		beforeFiles["dnsmasq.conf"] = before
	}
	return a.dns.Restore(beforeFiles, files)
}

type managedRoutingRecovery struct {
	Version   int             `json:"version"`
	Before    string          `json:"before"`
	Candidate string          `json:"candidate"`
	Kernel    json.RawMessage `json:"kernel"`
	Runtime   json.RawMessage `json:"runtime,omitempty"`
}

func managedRoutingPair(before, next json.RawMessage) (Config, Config, error) {
	b, err := Decode(before)
	if err != nil {
		return b, Config{}, err
	}
	n, err := Decode(next)
	if err != nil {
		return b, n, err
	}
	// DHCP client / RA / DHCP server are managed by the renderer and the
	// DNS host. The adapter must still validate that the link set itself
	// (IDs, names, kinds) is identical between before and next; dynamic
	// configuration is admitted by the renderer and the daemon host.
	for _, c := range []Config{b, n} {
		for _, link := range c.Interfaces {
			if link.Kind == "" {
				return b, n, errors.New("link kind is required")
			}
			if link.Kind != "physical" && link.Kind != "bridge" && link.Kind != "vlan" {
				return b, n, errors.New("unsupported link kind")
			}
			if link.DHCPServer != nil {
				if link.DHCPServer.Start != "" {
					if _, err := netip.ParseAddr(link.DHCPServer.Start); err != nil {
						return b, n, fmt.Errorf("DHCP range start %q is not a valid address: %w", link.DHCPServer.Start, err)
					}
				}
				if link.DHCPServer.End != "" {
					if _, err := netip.ParseAddr(link.DHCPServer.End); err != nil {
						return b, n, fmt.Errorf("DHCP range end %q is not a valid address: %w", link.DHCPServer.End, err)
					}
				}
			}
		}
	}
	left, right := b, n
	left.Firewall, right.Firewall = Firewall{}, Firewall{}
	left.PolicyRules, right.PolicyRules = nil, nil
	main := func(routes []Route) []Route {
		out := []Route{}
		for _, r := range routes {
			if !privateRouteTable(r.Table) {
				out = append(out, r)
			}
		}
		return out
	}
	left.Routes, right.Routes = main(b.Routes), main(n.Routes)
	// The adapter admits dynamic link changes (DHCP client/RA, DHCP server,
	// addresses, MTU) as well as private routing and policy changes. The
	// only constraint is that the link set itself (ID/name/kind) is the
	// same, the link's role and management flag do not change, and the
	// main-table routes that this adapter never owns are not mutated.
	strip := func(cfg Config) Config {
		stripped := Config{Version: cfg.Version, DNS: DNS{}, Management: cfg.Management, Routes: main(cfg.Routes)}
		stripped.DNS.Upstreams = append([]string(nil), cfg.DNS.Upstreams...)
		stripped.DNS.Records = append([]Record(nil), cfg.DNS.Records...)
		return stripped
	}
	if err := enforceLinkShapeEquality(b, n); err != nil {
		return b, n, err
	}
	left2, right2 := strip(b), strip(n)
	x, _ := json.Marshal(left2)
	y, _ := json.Marshal(right2)
	if !bytes.Equal(x, y) {
		return b, n, errors.New("static routing adapter cannot change main routes, DNS or management intent")
	}
	return b, n, nil
}

// enforceLinkShapeEquality guarantees the link set that already exists
// in the durable "before" config remains shape-stable: every link ID in
// before must be present in next with the same name, kind, role and
// management flag. Dynamic fields (DHCP, RA, MTU, addresses, DHCP server)
// may change.
//
// This check intentionally classifies "an interface ID appears in next
// but not in before" as outside its scope. Adjudicating that case is the
// adoption boundary's job (see enforceAdoptionBoundary); conflating
// "adoption" with "link addition" would have forced Safe Apply to
// refuse the legitimate "first include of an enrolled physical NIC"
// path. The two checks run together from the adapter's preflight and
// each rule that protects the host still fires: removals are refused
// here, identity mutation is refused here, kernel-creation attempts are
// refused in enforceAdoptionBoundary.
func enforceLinkShapeEquality(b, n Config) error {
	byID := map[string]Interface{}
	for _, link := range n.Interfaces {
		byID[link.ID] = link
	}
	for _, before := range b.Interfaces {
		next, ok := byID[before.ID]
		if !ok {
			return errors.New("managed routing cannot remove a link")
		}
		if next.Name != before.Name || next.Kind != before.Kind || next.Role != before.Role || next.Management != before.Management {
			return errors.New("managed routing cannot change link identity, kind, role or management flag")
		}
	}
	return nil
}

func (a *ManagedRoutingAdapter) Preflight(ctx context.Context, before, next json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.preflight(ctx, before, next)
}

func (a *ManagedRoutingAdapter) preflight(ctx context.Context, before, next json.RawMessage) error {
	if a.kernel == nil || a.files == nil || a.daemon == nil || a.checkOwnership == nil || a.checkManagement == nil {
		return errors.New("managed routing activation gates are incomplete")
	}
	b, n, err := managedRoutingPair(before, next)
	if err != nil {
		return err
	}
	if err := rejectExternallyManagedMutation(a.enrollment, b, n); err != nil {
		return err
	}
	// Adoption boundary: admit the legitimate first include of an
	// already-enrolled physical NIC; refuse any other link-addition
	// that would otherwise look like kernel creation. The check is
	// independent of the networkd reload (below); running it ahead
	// of disk writes is intentional so the renderer is never handed
	// a candidate whose link set has not been audited.
	if err := enforceAdoptionBoundary(a.enrollment, a.liveLinks, b, n); err != nil {
		return err
	}
	pb, err := Render(b)
	if err != nil {
		return err
	}
	pn, err := Render(n)
	if err != nil {
		return err
	}
	if err := a.files.Check(pb.Networkd, pn.Networkd); err != nil {
		return err
	}
	if err := a.checkOwnership(ctx, b, n); err != nil {
		return err
	}
	// The management verifier must run on the durable before-state,
	// against the live host, before any adapter write. On first-boot
	// the durable baseline is intentionally empty; the verifier now
	// treats that case as vacuously safe (no managed interface to
	// attest against host evidence). The host-plane check still
	// fires against the enrollment reserved names so the existing
	// externally-managed management plane of the host is preserved.
	if err := a.checkManagement(ctx, b); err != nil {
		return err
	}
	if err := a.daemon.PreflightScoped(ctx, b, n); err != nil {
		return err
	}
	return a.kernel.firewall.Check(ctx, n)
}

// rejectExternallyManagedMutation refuses any change to a Physical binding
// the host keeps under another network manager. The enrollment flags such
// bindings (typically the management NIC that dhcpcd retains across a
// supported install); the agent owns the durable identity for
// cross-manager accounting but must never author a networkd profile or
// alter their kernel state from this surface. Migration is an explicit
// operator workflow that flips the flag; it is not a Safe Apply outcome.
func rejectExternallyManagedMutation(e Enrollment, before, next Config) error {
	external := map[string]PhysicalBinding{}
	for _, b := range e.Physical {
		if b.ExternallyManaged {
			external[b.ID] = b
		}
	}
	if len(external) == 0 {
		return nil
	}
	pb, err := Render(before)
	if err != nil {
		return err
	}
	pn, err := Render(next)
	if err != nil {
		return err
	}
	// Top-level routes also change per-link artifacts. Checking only the
	// Interface fields would authorize a route-only takeover of an external NIC.
	for id := range unionInterfaceIDs(changedInterfaceIDs(before, next), renderedBodyChanged(pb.Networkd, pn.Networkd)) {
		if binding, ok := external[id]; ok {
			return fmt.Errorf("protected management link %q cannot be mutated by Safe Apply", binding.Name)
		}
	}
	beforeByID := map[string]Interface{}
	for _, i := range before.Interfaces {
		beforeByID[i.ID] = i
	}
	for _, i := range next.Interfaces {
		bind, ok := external[i.ID]
		if !ok {
			continue
		}
		prev, had := beforeByID[i.ID]
		if !had {
			return fmt.Errorf("protected management link %q cannot be claimed by Safe Apply; explicit migration required", bind.Name)
		}
		if !interfaceStateEqual(prev, i) {
			return fmt.Errorf("protected management link %q is managed by an external network manager; mutation refused", bind.Name)
		}
	}
	return nil
}

func (a *ManagedRoutingAdapter) Capture(ctx context.Context, before, next json.RawMessage) (json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.captured = ""
	if err := a.preflight(ctx, before, next); err != nil {
		return nil, err
	}
	b, n, _ := managedRoutingPair(before, next)
	kernel, err := a.kernel.Capture(ctx, n)
	if err != nil {
		return nil, err
	}
	observed, err := a.kernel.decode(n, kernel)
	if err != nil {
		return nil, err
	}
	beforeRoutes, err := PrivateRoutesForConfig(b, a.kernel.tables)
	if err != nil {
		return nil, err
	}
	beforeRules, err := PolicySnapshotForConfig(b)
	if err != nil {
		return nil, err
	}
	if !equalRoutes(*observed.Routes, beforeRoutes) || !equalPolicy(*observed.Rules, beforeRules) {
		return nil, errors.New("owned route or policy drift requires reconciliation before apply")
	}
	e := managedRoutingRecovery{Version: 1, Before: configFingerprint(b), Candidate: configFingerprint(n), Kernel: kernel}
	if a.captureRuntime != nil {
		e.Runtime, err = a.captureRuntime(ctx, b, n)
		if err != nil {
			return nil, err
		}
		if err := runtimeMatchesBefore(e.Runtime, b, n); err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(e)
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("managed routing recovery evidence exceeds limit")
	}
	a.captured = string(raw)
	return raw, nil
}

func (a *ManagedRoutingAdapter) Apply(ctx context.Context, before, next json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	raw := json.RawMessage(a.captured)
	a.captured, a.verifiedConfig = "", ""
	e, b, n, err := a.decode(before, next, raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "managed-routing: Apply decode err=%q\n", err.Error())
		return err
	}
	if err := a.preflight(ctx, before, next); err != nil {
		fmt.Fprintf(os.Stderr, "managed-routing: Apply preflight err=%q\n", err.Error())
		return err
	}
	guard, err := a.kernel.Apply(ctx, n, e.Kernel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "managed-routing: kernel.Apply err=%q\n", err.Error())
		return err
	}
	// Scope isolation: a transaction that does not actually mutate a link
	// must never replace its networkd artifact. Render every interface the
	// candidate still mentions, then filter both before and next plans to
	// the IDs whose state differs. The "scoped" set covers two sources:
	//
	//   * changedInterfaceIDs — additions, removals, or per-Interface
	//     field-set edits (address, DHCP, role, MTU, ...).
	//   * rendered body diffs — the rendered .network artifact for an
	//     existing interface can also change because routes (which are a
	//     top-level Config.Routes field that lives in the rendered
	//     artifact's [Route] section) or DNSMasq dhcp-range changes that
	//     rename a managed LAN listener. A no-op per-interface field set
	//     therefore still requires a disk write + networkd reload.
	//
	// Unchanged interfaces — including externally managed management
	// NICs — keep their on-disk artifacts and current manager.
	changed := changedInterfaceIDs(b, n)
	pb, _ := Render(b)
	pn, _ := Render(n)
	scoped := unionInterfaceIDs(changed, renderedBodyChanged(pb.Networkd, pn.Networkd))
	pbNetd := filterNetworkdByIDs(pb.Networkd, scoped)
	pnNetd := filterNetworkdByIDs(pn.Networkd, scoped)
	// Routing / firewall / DNS state that is not bound to a single interface
	// only needs replacement if the corresponding global plan text changed.
	routingChanged := pb.DNSMasq != pn.DNSMasq || pb.NFTables != pn.NFTables
	if len(pnNetd) > 0 {
		fmt.Fprintf(os.Stderr, "managed-routing: DEBUG scoped=%v pnNetd_keys=%v pbNetd_keys=%v\n", scoped, keysOf(pnNetd), keysOf(pbNetd))
		if err := a.files.Replace(pbNetd, pnNetd); err != nil {
			fmt.Fprintf(os.Stderr, "managed-routing: files.Replace err=%q\n", err.Error())
			return err
		}
	}
	if routingChanged {
		if err := a.replaceDNS(pb.DNSMasq, pn.DNSMasq); err != nil {
			fmt.Fprintf(os.Stderr, "managed-routing: replaceDNS err=%q\n", err.Error())
			return err
		}
	}
	// The networkd reload host validates its candidate via Render +
	// Config.Validate before issuing the actual reload. Render rejects
	// configs that omit the static management LAN or any other interface
	// the durable intent must continue to describe. Pass the full
	// before/next pair so that validation sees the complete set of
	// interfaces the durable journal still claims; the per-ID disk
	// replacement above already keeps the artifact invariant under the
	// scope-isolation rule (eth0's 10-duckos-eth0.network is never
	// written because its ID is absent from the scoped set).
	//
	// The reload itself is scoped: only the IDs in the scoped set are
	// re-evaluated, via per-interface `networkctl reconfigure <iface>`
	// rather than the global `networkctl reload`. An eth1-only Safe
	// Apply therefore never causes networkd to re-touch eth0's link or
	// its foreign-manager-owned profile, so the externally managed
	// management NIC stays on whatever manager currently owns it.
	if err := a.daemon.ReloadScoped(ctx, b, n, scoped); err != nil {
		fmt.Fprintf(os.Stderr, "managed-routing: daemon.ReloadScoped err=%q\n", err.Error())
		return err
	}
	a.scopedChanged = scoped
	routes, _ := PrivateRoutesForConfig(n, a.kernel.tables)
	rules, _ := PolicySnapshotForConfig(n)
	if err := a.verifyStage(ctx, n, guard, routes, rules); err != nil {
		fmt.Fprintf(os.Stderr, "managed-routing: verifyStage err=%q\n", err.Error())
		return err
	}
	final, err := a.kernel.firewall.Apply(ctx, n, guard)
	if err != nil {
		fmt.Fprintf(os.Stderr, "managed-routing: firewall.Apply err=%q\n", err.Error())
		return err
	}
	// Release the duckos_apply_guard chain installed during
	// kernel.Apply's InstallForwardGuard step. Without this the
	// next Safe Apply's Capture() trips guardSnapshot's "reserved
	// forwarding guard requires recovery" check because the live
	// kernel still owns the chain. The release runs after every
	// other Apply step so any error here surfaces last and the
	// barrier stays in place only when the chain still owns drop
	// rules (which would mean a kernel restore failure upstream).
	if err := a.kernel.firewall.ReleaseForwardGuard(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "managed-routing: ReleaseForwardGuard err=%q\n", err.Error())
		return err
	}
	a.remember(n, final, routes, rules)
	return nil
}

// changedInterfaceIDs returns the set of link IDs whose state actually
// differs between baseline and candidate. The set covers additions
// (next-only), removals (before-only) and real edits (same ID, different
// field set). Identical-state entries are filtered out so the adapter does
// not re-render, replace, or reload their artifacts. Caller-supplied
// externally-managed links are always included as unchanged so the
// adapter can short-circuit on identity comparison without ever writing a
// networkd file for them.
func changedInterfaceIDs(before, next Config) map[string]bool {
	out := map[string]bool{}
	nextByID := map[string]Interface{}
	for _, n := range next.Interfaces {
		nextByID[n.ID] = n
	}
	beforeByID := map[string]Interface{}
	for _, b := range before.Interfaces {
		beforeByID[b.ID] = b
	}
	for id, n := range nextByID {
		b, ok := beforeByID[id]
		if !ok || !interfaceStateEqual(b, n) {
			out[id] = true
		}
	}
	for id := range beforeByID {
		if _, ok := nextByID[id]; !ok {
			out[id] = true
		}
	}
	return out
}

// interfaceStateEqual returns true when two interface entries carry the
// same networkd-relevant fields. The host renders all of these into the
// .network artifact, so any difference forces a re-render.
func interfaceStateEqual(a, b Interface) bool {
	if a.ID != b.ID || a.Name != b.Name || a.Kind != b.Kind || a.Role != b.Role ||
		a.Management != b.Management || a.MTU != b.MTU || a.DHCPClient != b.DHCPClient ||
		a.AcceptRA != b.AcceptRA || a.VLAN != b.VLAN || a.Parent != b.Parent ||
		addrEqual(a.Addresses, b.Addresses) == false {
		return false
	}
	return true
}

func addrEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

// unionInterfaceIDs returns the union of two ID sets without mutating the
// inputs. Both maps may be nil; the result is a freshly-allocated map.
func unionInterfaceIDs(a, b map[string]bool) map[string]bool {
	if len(a) == 0 && len(b) == 0 {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(a)+len(b))
	for id := range a {
		out[id] = true
	}
	for id := range b {
		out[id] = true
	}
	return out
}

// renderedBodyChanged returns the set of interface IDs whose rendered
// .network artifact body differs between the baseline and the candidate.
// It exists so the scope-isolation invariant can capture changes that
// changedInterfaceIDs misses: routes, DNSMasq-managed dhcp-range
// renames, and any future per-link field that the renderer emits but the
// Interface struct does not model. The function is independent of the
// Interface struct so it cannot be tricked by a future change that adds
// a new renderer-only field. Externally-managed bindings are tolerated
// only insofar as the renderer would render them — by construction the
// durable Config never describes them, so they never appear in pb / pn.
func renderedBodyChanged(before, next map[string]string) map[string]bool {
	out := map[string]bool{}
	for name, afterBody := range next {
		if beforeBody, ok := before[name]; !ok || beforeBody != afterBody {
			out[extractLinkIDFromNetworkdName(name)] = true
		}
	}
	for name := range before {
		if _, ok := next[name]; !ok {
			out[extractLinkIDFromNetworkdName(name)] = true
		}
	}
	return out
}

// extractLinkIDFromNetworkdName strips the "10-duckos-" prefix and the
// file extension so callers can map an artifact basenme back to its
// Interface ID. Files that do not carry the DuckOS prefix are ignored at
// the caller; this helper assumes the prefix is present.
func extractLinkIDFromNetworkdName(name string) string {
	const prefix = "10-duckos-"
	rest := strings.TrimPrefix(name, prefix)
	if dot := strings.Index(rest, "."); dot >= 0 {
		return rest[:dot]
	}
	return rest
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// filterNetworkdByIDs keeps only the .network artifact names whose
// "10-duckos-<id>." prefix matches one of the supplied IDs. Other
// (un-changed) interface artifacts remain on disk untouched, so an
// externally managed management link keeps its current configuration.
func filterNetworkdByIDs(in map[string]string, ids map[string]bool) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(ids))
	const prefix = "10-duckos-"
	for name, body := range in {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rest := strings.TrimPrefix(name, prefix)
		id := rest
		if dot := strings.Index(rest, "."); dot >= 0 {
			id = rest[:dot]
		}
		if ids[id] {
			out[name] = body
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// scopeConfigsToInterfaceIDs returns the smallest before/next pair that
// contains only interfaces whose IDs appear in the supplied set. Other
// config blocks (routes, DNS, management policy) are passed through so
// the networkd daemon still sees the bounded validation envelope, but
// only the listed links enter its render and ownership checks.
func scopeConfigsToInterfaceIDs(before, next Config, ids map[string]bool) (Config, Config) {
	keep := func(cfg Config) Config {
		out := Config{
			Version:    cfg.Version,
			Interfaces: []Interface{},
			Routes:     cfg.Routes,
			Firewall:   cfg.Firewall,
			DNS:        cfg.DNS,
			Management: cfg.Management,
		}
		for _, i := range cfg.Interfaces {
			if ids[i.ID] {
				out.Interfaces = append(out.Interfaces, i)
			}
		}
		return out
	}
	return keep(before), keep(next)
}

func (a *ManagedRoutingAdapter) decode(before, next, raw json.RawMessage) (managedRoutingRecovery, Config, Config, error) {
	var e managedRoutingRecovery
	b, n, err := managedRoutingPair(before, next)
	if err != nil {
		return e, b, n, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 1<<20 || d.Decode(&e) != nil || d.Decode(new(any)) != io.EOF || e.Version != 1 || e.Before != configFingerprint(b) || e.Candidate != configFingerprint(n) || len(e.Kernel) == 0 {
		return e, b, n, errors.New("managed routing recovery does not match transaction")
	}
	if len(e.Runtime) > 0 {
		if err := runtimeMatchesBefore(e.Runtime, b, n); err != nil {
			return e, b, n, err
		}
	}
	return e, b, n, nil
}

func (a *ManagedRoutingAdapter) verifyStage(ctx context.Context, c Config, nft NFTSnapshot, routes RouteSnapshot, rules PolicySnapshot) error {
	full, err := Render(c)
	if err != nil {
		return err
	}
	// Scope the disk consistency check to the interfaces the Apply path
	// actually wrote. Unchanged interfaces (including externally managed
	// management NICs) have no on-disk artifact owned by us and must not
	// be required by the post-apply check.
	scoped := filterNetworkdByIDs(full.Networkd, a.scopedChanged)
	if err := a.files.Check(scoped, scoped); err != nil {
		return err
	}
	if err := a.checkOwnership(ctx, c, c); err != nil {
		return err
	}
	if err := a.kernel.routes.Verify(ctx, routes); err != nil {
		return err
	}
	if err := a.kernel.rules.Verify(ctx, rules); err != nil {
		return err
	}
	if err := a.kernel.firewall.Verify(ctx, nft); err != nil {
		return err
	}
	return a.checkManagement(ctx, c)
}

func (a *ManagedRoutingAdapter) remember(c Config, nft NFTSnapshot, routes RouteSnapshot, rules PolicySnapshot) {
	a.verifiedConfig, a.verifiedNFT, a.verifiedRoutes, a.verifiedRules = configFingerprint(c), nft, routes, rules
}

func (a *ManagedRoutingAdapter) Verify(ctx context.Context, config json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, err := Decode(config)
	if err != nil {
		return err
	}
	if a.verifiedConfig == "" || a.verifiedConfig != configFingerprint(c) {
		fmt.Fprintf(os.Stderr, "managed-routing: Verify ref empty=%v expected=%s got=%s\n", a.verifiedConfig == "", a.verifiedConfig, configFingerprint(c))
		return errors.New("managed routing has no verified transaction state")
	}
	if err := a.verifyStage(ctx, c, a.verifiedNFT, a.verifiedRoutes, a.verifiedRules); err != nil {
		fmt.Fprintf(os.Stderr, "managed-routing: Verify stage failed err=%q\n", err.Error())
		return err
	}
	return nil
}

func (a *ManagedRoutingAdapter) Restore(context.Context, json.RawMessage, json.RawMessage) error {
	return errors.New("managed routing requires durable recovery evidence")
}

func (a *ManagedRoutingAdapter) RestoreCaptured(ctx context.Context, before, next, raw json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.kernel == nil || a.files == nil || a.daemon == nil || a.checkOwnership == nil || a.checkManagement == nil {
		return errors.New("managed routing recovery dependencies are incomplete")
	}
	a.captured, a.verifiedConfig = "", ""
	e, b, n, err := a.decode(before, next, raw)
	if err != nil {
		return err
	}
	if err := rejectExternallyManagedMutation(a.enrollment, b, n); err != nil {
		return err
	}
	evidence, err := a.kernel.decode(n, e.Kernel)
	if err != nil {
		return err
	}
	guard, _ := guardSnapshot(*evidence.Firewall)
	if err := a.kernel.firewall.Restore(ctx, guard); err != nil {
		return err
	}
	// Scope isolation: the failed Apply only mutated the changed IDs.
	// Unchanged interfaces — including externally managed management
	// NICs — keep their on-disk artifacts and current manager. The
	// restore path is bounded to the same set so a partial rollback
	// never re-renders or re-replaces an interface the transaction
	// never touched.
	changed := changedInterfaceIDs(b, n)
	pb, _ := Render(b)
	pn, _ := Render(n)
	scoped := unionInterfaceIDs(changed, renderedBodyChanged(pb.Networkd, pn.Networkd))
	a.scopedChanged = scoped // Reconstruct the verification scope after restart.
	pbNetd := filterNetworkdByIDs(pb.Networkd, scoped)
	pnNetd := filterNetworkdByIDs(pn.Networkd, scoped)
	if err := a.files.Restore(pbNetd, pnNetd); err != nil {
		return err
	}
	if pb.DNSMasq != pn.DNSMasq {
		if err := a.restoreDNS(pb.DNSMasq, pn.DNSMasq); err != nil {
			return err
		}
	}
	// Always re-verify the daemon on Restore so failure-injection paths
	// can exercise the recovery stage. The networkd reload host must
	// see the full durable before-state: its Render+Validate chain
	// refuses configs that drop the static management LAN. The
	// per-ID disk restore above already keeps the scope-isolation
	// invariant (eth0's 10-duckos-eth0.network is never re-rendered or
	// re-replaced during rollback because its ID is absent from the
	// scoped set). The actual reload is still scoped to the IDs the
	// transaction actually claimed, so an eth1-only rollback never
	// touches eth0's networkd profile either.
	if err := a.daemon.ReloadScopedRestored(ctx, b, scoped); err != nil {
		return err
	}
	guard, err = a.kernel.Restore(ctx, n, e.Kernel)
	if err != nil {
		return err
	}
	if err := a.verifyStage(ctx, b, guard, *evidence.Routes, *evidence.Rules); err != nil {
		return err
	}
	if err := a.kernel.firewall.Restore(ctx, *evidence.Firewall); err != nil {
		return err
	}
	// Release the duckos_apply_guard chain we installed during Capture.
	// A rolled-back Safe Apply must not leave the barrier in place —
	// otherwise the next Apply's Capture stage trips "reserved
	// forwarding guard requires recovery" and the operator has to run
	// `nft delete table inet duckos` by hand. The release runs after
	// every other rollback step so any error here surfaces last and the
	// barrier stays in place only when the chain still owns drop rules
	// (which can happen mid-rollback if the kernel restore failed
	// upstream).
	if err := a.kernel.firewall.ReleaseForwardGuard(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "managed-routing: ReleaseForwardGuard err=%q\n", err.Error())
		return err
	}
	a.remember(b, *evidence.Firewall, *evidence.Routes, *evidence.Rules)
	return nil
}
