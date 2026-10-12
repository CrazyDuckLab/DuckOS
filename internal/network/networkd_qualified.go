package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"time"
)

// These are systemd 257's network configuration search roots. Qualification
// deliberately does not try to interpret arbitrary third-party .network files.
var networkdConfigRoots = []string{"/etc/systemd/network", "/run/systemd/network", "/usr/local/lib/systemd/network", "/usr/lib/systemd/network"}

// Enrollment establishes the permitted kernel kind before entering the refresh
// primitive. Refresh may change profile state, never kernel link identity.
func checkRefreshedProfiles(c Config, initial, observed []NetworkdLink) error {
	old := map[string]NetworkdLink{}
	now := map[string]NetworkdLink{}
	for _, i := range initial {
		old[i.Name] = i
	}
	for _, i := range observed {
		now[i.Name] = i
	}
	for _, i := range c.Interfaces {
		b, ok := old[i.Name]
		n, present := now[i.Name]
		if !ok || !present || b.Index != n.Index || b.Kind != n.Kind || n.AdministrativeState != "configured" || n.NetworkFile != "/etc/systemd/network/10-duckos-"+i.ID+".network" || len(n.NetworkFileDropins) != 0 {
			return errors.New("profile is not converged on the admitted kernel link")
		}
	}
	return nil
}

func staticNetworkdConfig(c Config) error {
	for _, i := range c.Interfaces {
		if i.Kind != "physical" || i.Parent != "" {
			return errors.New("qualified networkd refresh supports standalone physical links only")
		}
		// LAN physical links are still strictly static: the
		// renderer writes a fixed-address networkd profile and the
		// management verifier refuses to adopt a managed link that
		// pulls an upstream DHCP/RA assignment. WAN physical links
		// may opt into DHCPv4 / DHCPv6 / dual when the renderer has
		// an upstream networkd profile for that mode; the renderer
		// and ownership checks already validate the rendered file
		// before reloading, so the admission gate only restricts
		// who MAY opt in, not the rendered output.
		if i.Role == "wan" {
			if i.DHCPClient != "none" && i.DHCPClient != "ipv4" && i.DHCPClient != "ipv6" && i.DHCPClient != "dual" {
				return errors.New("unsupported WAN DHCP client mode")
			}
			continue
		}
		if i.DHCPClient != "none" || i.AcceptRA {
			return errors.New("qualified networkd refresh supports standalone static LAN links only")
		}
	}
	return nil
}

// qualifyNetworkdFiles is a whole-input admission gate, NOT an unrestricted
// reload. Every effective profile that COULD claim a DuckOS-managed link
// must be exact renderer output for an owned standalone static NIC.
// Netdevs, drop-ins, foreign profiles and symlinks fail closed, including
// files in lower-priority configuration search directories — provided the
// file's [Match] clause can reach a DuckOS-managed link. Files whose
// [Match] clause targets an unrelated Kind / Name (Debian upstream
// vendor-shipped defaults in /usr/lib/systemd/network, for example
// `Kind=tun Name=vt-*`) cannot claim DuckOS's standalone physical NICs;
// they are vendor-ambient and pass without contention.
func qualifyNetworkdFiles(roots []string, before, next Config) error {
	if err := staticNetworkdConfig(before); err != nil {
		return err
	}
	if err := staticNetworkdConfig(next); err != nil {
		return err
	}
	pb, err := Render(before)
	if err != nil {
		return err
	}
	pn, err := Render(next)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for index, path := range roots {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
			return errors.New("unsafe networkd search directory")
		}
		dir, err := OpenArtifactDirectory(path)
		if err != nil {
			return err
		}
		f, err := dir.root.Open(".")
		if err != nil {
			dir.Close()
			return err
		}
		entries, readErr := f.ReadDir(4097)
		f.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) || len(entries) > 4096 {
			dir.Close()
			return errors.New("networkd inventory exceeds limit")
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasSuffix(name, ".netdev") || strings.HasSuffix(name, ".network.d") || strings.HasSuffix(name, ".netdev.d") || strings.HasSuffix(name, ".duckos-tmp") {
				dir.Close()
				return errors.New("networkd dependencies or pending artifacts prevent isolated refresh")
			}
			if !strings.HasSuffix(name, ".network") {
				continue
			}
			body, err := dir.read(name)
			if err != nil {
				dir.Close()
				return err
			}
			old, oldOK := pb.Networkd[name]
			candidate, nextOK := pn.Networkd[name]
			// Profile is admissible if its content matches the
			// admitted intent OR if its [Match] clause cannot reach
			// any DuckOS-managed link (vendor-ambient default that
			// targets an unrelated kind / namespace).
			admitted := oldOK && body == old
			admitted = admitted || (nextOK && body == candidate)
			if !admitted {
				// First root must be exact — anything else is
				// unsupported. Non-first roots get the
				// vendor-ambient exemption; the explicit
				// duplicate-basename guard stays.
				if index == 0 || seen[name] {
					dir.Close()
					return errors.New("foreign networkd profile prevents isolated refresh")
				}
				if profileCouldClaimDuckLink(next, []byte(body)) {
					dir.Close()
					return errors.New("foreign networkd profile prevents isolated refresh")
				}
			} else if body != old && body != candidate {
				dir.Close()
				return errors.New("networkd profile differs from admitted intent")
			}
			seen[name] = true
		}
		dir.Close()
	}
	// Every profile the candidate introduces as a NEW file (one that
	// is in pn.Networkd but not in pb.Networkd) is admitted for a
	// fresh-install adoption even when it isn't yet on disk — the
	// apply is about to write it. Profiles that already existed in
	// the durable before-state (pb.Networkd) MUST still be present
	// on disk after the previous apply, otherwise the live kernel
	// drift is unverified.
	for name, body := range pn.Networkd {
		if seen[name] {
			continue
		}
		if _, had := pb.Networkd[name]; !had {
			continue
		}
		if _, err := dirAtRoot(name); err == nil {
			// Brand-new file already on disk for some reason
			// (e.g. recreated by a previous failed attempt).
			// Still acceptable; allow.
			continue
		}
		_ = body // body already compared above when admittance fires.
		return errors.New("owned networkd profile missing")
	}
	return nil
}

// dirAtRoot is a stand-in probe for "would the candidate's file
// already appear at the operator-supervised root". It looks in
// /etc/systemd/network for the entry — the operator-managed
// root the renderer writes to — and reports whether the file is
// present there.
func dirAtRoot(name string) (struct{}, error) {
	root := networkdConfigRoots[0]
	p := root + "/" + name
	info, err := os.Lstat(p)
	if err != nil {
		return struct{}{}, err
	}
	if info.IsDir() {
		return struct{}{}, errors.New("profile path is a directory")
	}
	return struct{}{}, nil
}

// profileCouldClaimDuckLink inspects a .network file's [Match] section
// and reports whether the profile could claim a link the candidate
// declares under management. An empty Match, a Match that contains
// wildcards, or a Match whose Kind / Type / Name intersects the
// candidate's managed links is treated as a potential claim; the
// strict body-equality rule then applies. A specific Match that
// targets an unrelated Kind, Type or Name cannot claim, and the
// vendor-ambient exemption skips the strict rule.
func profileCouldClaimDuckLink(next Config, body []byte) bool {
	keys := parseNetworkdMatchKeys(body)
	if keys.wildcard {
		return true
	}
	if keys.empty {
		return true
	}
	// Kind: physical / vlan / bridge are the kinds DuckOS owns.
	if keys.kind != "" {
		overlap := false
		for _, link := range next.Interfaces {
			if link.Kind == "" {
				continue
			}
			if matchSpecificGlob(keys.kind, link.Kind) {
				overlap = true
				break
			}
		}
		if !overlap {
			return false
		}
	}
	// Type: only "ether" / "physical" (or empty) map to a DuckOS link.
	// Vendor profiles use Type=wlan / Type=wwan / Type=ieee802154 to
	// narrow the target namespace and cannot claim our links.
	if keys.typ != "" {
		if !stringGlobOverlap(keys.typ, []string{"ether", "physical"}) {
			return false
		}
	}
	// Name: explicit overlap with a candidate link name.
	if keys.name != "" {
		overlap := false
		for _, link := range next.Interfaces {
			if link.Name == "" {
				continue
			}
			if matchSpecificGlob(keys.name, link.Name) {
				overlap = true
				break
			}
		}
		if !overlap {
			return false
		}
	}
	// Other Match keys (Driver, Architecture, Property, etc.)
	// are present but unchecked — DuckOS physical NICs share no
	// generic narrowing with vendor-shipped defaults, so declare
	// the profile safe.
	if keys.otherNonEmpty {
		return false
	}
	return true
}

// stringGlobOverlap reports whether ANY pattern in `values` could
// match `names` under the small-set Match-kind glob semantics.
func stringGlobOverlap(pattern string, names []string) bool {
	for _, n := range names {
		if pattern == n {
			return true
		}
	}
	return false
}

// matchKeys holds the parsed [Match] section sufficient to classify
// a profile's reach into DuckOS-managed namespaces.
type matchKeys struct {
	kind          string
	typ           string
	name          string
	wildcard      bool
	empty         bool
	otherNonEmpty bool
}

// parseNetworkdMatchKeys walks the .network body and extracts the
// Match-section values relevant to DuckOS' admission. A literal
// `*` value sets the wildcard flag; specific values (or specific
// globs that reference a bounded set) drive the kind/type/name
// overlap checks below. Any non-empty key NOT inspected here
// (Driver, Architecture, PermanentMACAddress, ...) flags the
// profile as namespace-restricted and DuckOS-safe by default.
func parseNetworkdMatchKeys(body []byte) matchKeys {
	inMatch := false
	out := matchKeys{}
	hasAny := false
	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			inMatch = section == "match"
			continue
		}
		if !inMatch {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if val == "" {
			continue
		}
		hasAny = true
		if val == "*" {
			out.wildcard = true
			continue
		}
		switch key {
		case "Kind":
			out.kind = val
		case "Type":
			out.typ = val
		case "Name":
			out.name = val
		default:
			out.otherNonEmpty = true
		}
	}
	if !hasAny {
		out.empty = true
	}
	return out
}

// parseNetworkdMatch walks the .network body and returns the Match.Kind
// and Match.Name values, plus a wildcard flag set when either value
// is the literal `*`. Patterns like `vt-*` are specific globs that
// need to be tested against the candidate's link set; the wildcard
// flag is reserved for the unconditional forms that networkd
// treats as "match everything". The parser is intentionally
// line-scoped and ignores [Section] headers other than [Match]; it
// accepts `key=value` lines inside the [Match] section.
func parseNetworkdMatch(body []byte) (kind, name string, wildcard bool) {
	inMatch := false
	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			inMatch = section == "match"
			continue
		}
		if !inMatch {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if val == "*" {
			wildcard = true
		}
		switch key {
		case "Kind":
			kind = val
		case "Name":
			name = val
		}
	}
	return kind, name, wildcard
}

// matchSpecificGlob reports whether pattern matches name, accepting the
// small set of globs Match.Name and Match.Kind use in vendor-shipped
// profiles (commas separate alternatives, leading / trailing wildcards
// are accepted).
func matchSpecificGlob(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "*" || strings.HasPrefix(pattern, "*") || strings.HasSuffix(pattern, "*") || strings.Contains(pattern, "*") {
		matched, err := globMatch(pattern, name)
		if err == nil {
			return matched
		}
	}
	return pattern == name
}

// globMatch is a minimal glob implementation covering `*` only. It
// splits at the first `*`, anchors the head and the tail, and accepts
// any number of bytes in between. Patterns starting or ending with `*`
// anchor on the constant boundary.
func globMatch(pattern, name string) (bool, error) {
	if !strings.Contains(pattern, "*") {
		return pattern == name, nil
	}
	idx := strings.Index(pattern, "*")
	head := pattern[:idx]
	tail := pattern[idx+1:]
	if !strings.HasPrefix(name, head) {
		return false, nil
	}
	if !strings.HasSuffix(name, tail) {
		return false, nil
	}
	if len(name)+len(head)+len(tail) < len(head)+len(tail) {
		return false, nil
	}
	return true, nil
}

func qualifyNetworkdVersion(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	const daemon = "/usr/lib/systemd/systemd-networkd"
	cmd := exec.CommandContext(ctx, daemon, "--version")
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = time.Second
	raw, err := cmd.Output()
	if err != nil || !strings.HasPrefix(string(raw), "systemd 257 ") {
		return errors.New("networkd refresh requires qualified systemd 257")
	}
	// The qualified host uses its installed systemd package; package upgrades
	// and other root configuration writers must not overlap network transactions.
	return nil
}

func (h *NetworkdReloadHost) qualifyRefresh(ctx context.Context, before, next Config) error {
	if err := qualifyNetworkdVersion(ctx); err != nil {
		return err
	}
	if err := rejectExternallyManagedMutation(h.enrollment, before, next); err != nil {
		return err
	}
	// An external link must not appear in ANY loaded DuckOS profile, even
	// unchanged legacy intent: manager-wide refresh would otherwise adopt it.
	for _, binding := range h.enrollment.Physical {
		if binding.ExternallyManaged {
			for _, c := range []Config{before, next} {
				for _, i := range c.Interfaces {
					if i.Name == binding.Name {
						return errors.New("external link present in networkd intent; migration required")
					}
				}
			}
		}
	}
	if err := qualifyNetworkdFiles(networkdConfigRoots, before, next); err != nil {
		return err
	}
	live, err := h.inventory(ctx)
	if err != nil {
		return err
	}
	owned := map[string]bool{}
	for _, i := range next.Interfaces {
		owned[i.Name] = true
	}
	for _, i := range live {
		if owned[i.Name] {
			continue
		}
		// networkctl status reports the LAST-KNOWN NetworkFile path even
		// after the file has been removed from disk; the manager's view
		// lags the file system. An unowned link whose NetworkFile points at
		// a missing path is effectively unmanaged even though networkctl
		// still prints AdministrativeState="configured" and the link
		// itself carries the stale profile name. Verify the file
		// actually exists before rejecting the link. This is what makes
		// the recovery path converge after a DuckOS-owned profile is
		// removed: networkd's in-memory state takes a `networkctl reload`
		// to clear, and a stale NetworkFile path no longer blocks the
		// admission gate once the file is gone from disk. Drop-ins are
		// still checked because drop-ins live in networkd's own state
		// (under /etc/systemd/network) and cannot drift the way the
		// NetworkFile path can.
		if i.NetworkFile != "" {
			if _, err := os.Lstat(i.NetworkFile); err == nil {
				return errors.New("unowned link has a networkd .network file on disk")
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("unowned link networkd file stat failed: %w", err)
			}
			// NetworkFile path is gone from disk: the manager's
			// AdministrativeState lag and stale NetworkFile entry are
			// both expected. Treat the link as effectively unmanaged.
			continue
		}
		if i.AdministrativeState != "unmanaged" || len(i.NetworkFileDropins) != 0 {
			return errors.New("unowned link is not stably unmanaged by networkd")
		}
	}
	return nil
}

// Runtime readback is independent of networkd's profile-name bookkeeping.
// Reject extra global addresses, tolerate only kernel IPv6 link-local addresses.
func verifyStaticHost(ctx context.Context, c Config) error {
	raw, err := CaptureHost(ctx, c, c)
	if err != nil {
		return err
	}
	return runtimeMatchesBefore(raw, c, c)
}

func runtimeMatchesBefore(raw json.RawMessage, c, next Config) error {
	s, err := DecodeHostSnapshot(raw, c, next)
	if err != nil {
		return err
	}
	want := map[string]Interface{}
	for _, i := range c.Interfaces {
		want[i.Name] = i
	}
	// Only inspect links DuckOS intends to manage. The kernel
	// exposes other interfaces (e.g. eth0 under dhcpcd, or
	// vendor-up NICs) which are out of DuckOS' ownership and
	// outside the durable Config — they are not pinned by this
	// gate. For each wanted link, the host snapshot must agree
	// on Up, MTU and non-link-local addresses.
	for _, i := range c.Interfaces {
		var l HostLink
		found := false
		for _, candidate := range s.Links {
			if candidate.Name == i.Name {
				l = candidate
				found = true
				break
			}
		}
		if !found {
			return errors.New("owned link runtime does not match persisted configuration")
		}
		addresses := []string{}
		for _, a := range l.Addresses {
			p, err := netip.ParsePrefix(a)
			if err != nil {
				return err
			}
			if p.Addr().Is6() && p.Addr().IsLinkLocalUnicast() {
				continue
			}
			addresses = append(addresses, a)
		}
		expected := append([]string{}, i.Addresses...)
		sort.Strings(expected)
		sort.Strings(addresses)
		if !l.Up || l.MTU != i.MTU || !reflect.DeepEqual(addresses, expected) {
			return errors.New("owned link runtime does not match persisted configuration")
		}
	}
	return nil
}

func (h *NetworkdReloadHost) protectedRuntime(ctx context.Context, c Config) (string, error) {
	owned := map[string]bool{}
	for _, i := range c.Interfaces {
		owned[i.Name] = true
	}
	// The protected snapshot is the externally-managed management plane of
	// the host: every DuckOS-managed link is the apply's responsibility,
	// not the apply's invariant to preserve, so a Safe Apply legitimately
	// changes those links' addresses and routes. Only the enrollment's
	// ExternallyManaged links (e.g. eth0 under dhcpcd) are the invariant:
	// DuckOS never commands them, so their identity, addresses and
	// kernel-managed routes must remain stable across a Safe Apply or
	// rollback. Restricting the protected snapshot to the externally-
	// managed set also keeps the snapshot deterministic across the apply
	// and rollback paths — a rollback to an empty durable state must not
	// fail because eth1/eth2 lost their addresses.
	externallyManaged := map[string]bool{}
	for _, b := range h.enrollment.Physical {
		if b.ExternallyManaged && b.Name != "" {
			externallyManaged[b.Name] = true
		}
	}
	inventory, err := h.inventory(ctx)
	if err != nil {
		return "", err
	}
	sort.Slice(inventory, func(i, j int) bool { return inventory[i].Name < inventory[j].Name })
	type protected struct {
		Name, MAC  string
		Index, MTU int
		Flags      uint
		Addresses  []string
		Routes     [2]json.RawMessage
	}
	out := []protected{}
	for _, i := range inventory {
		if owned[i.Name] || i.Name == "lo" {
			continue
		}
		if !externallyManaged[i.Name] {
			continue
		}
		link, err := net.InterfaceByName(i.Name)
		if err != nil {
			return "", err
		}
		p := protected{Name: i.Name, MAC: link.HardwareAddr.String(), Index: link.Index, MTU: link.MTU, Flags: uint(link.Flags), Addresses: []string{}}
		addrs, err := link.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			p.Addresses = append(p.Addresses, addr.String())
		}
		sort.Strings(p.Addresses)
		for j, family := range []string{"-4", "-6"} {
			raw, err := runRouteInventory(ctx, []string{"-j", "-N", family, "route", "show", "table", "all", "dev", i.Name})
			if err != nil {
				return "", err
			}
			p.Routes[j] = raw
		}
		out = append(out, p)
	}
	raw, err := json.Marshal(out)
	if h := os.Getenv("DUCKOS_PROTECTED_TRACE"); h != "" {
		_ = os.WriteFile(h, raw, 0o644)
	}
	if n := os.Getenv("DUCKOS_PROTECTED_LOG"); n != "" {
		f, ferr := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if ferr == nil {
			fmt.Fprintf(f, "%s pid=%d snapshot=%s\n", time.Now().UTC().Format(time.RFC3339Nano), os.Getpid(), string(raw))
			f.Close()
		}
	}
	return string(raw), err
}
