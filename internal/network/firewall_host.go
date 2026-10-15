package network

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"slices"

	"github.com/CrazyDuckLab/DuckOS/internal/nftlaunch"
)

// FirewallHost performs real, bounded nftables operations. It is Agent-only:
// enrollment must establish exclusive inet/duckos ownership and the coordinator
// must durably capture before-state before calling Apply. It is not an RPC
// surface and never accepts executable names, paths, or shell text.
type FirewallHost struct {
	run func(context.Context, []string, []byte) ([]byte, error)
}

func NewFirewallHost() (*FirewallHost, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil, errors.New("firewall operations require privileged Linux Agent")
	}
	return &FirewallHost{run: runNFT}, nil
}

func (h *FirewallHost) Capture(ctx context.Context) (NFTSnapshot, error) {
	data, err := h.run(ctx, []string{"--json", "list", "tables"}, nil)
	if err != nil {
		return NFTSnapshot{}, err
	}
	var listing struct {
		NFTables *[]map[string]json.RawMessage `json:"nftables"`
	}
	if len(data) > maxNFTSnapshotBytes || json.Unmarshal(data, &listing) != nil || listing.NFTables == nil {
		return NFTSnapshot{}, errors.New("invalid nft table inventory")
	}
	found := false
	for _, entry := range *listing.NFTables {
		if len(entry) != 1 {
			return NFTSnapshot{}, errors.New("ambiguous nft inventory entry")
		}
		if _, ok := entry["metainfo"]; ok {
			continue
		}
		var table struct {
			Family string `json:"family"`
			Name   string `json:"name"`
		}
		if json.Unmarshal(entry["table"], &table) != nil || table.Family == "" || table.Name == "" {
			return NFTSnapshot{}, errors.New("invalid nft inventory entry")
		}
		if table.Family == "inet" && table.Name == "duckos" {
			if found {
				return NFTSnapshot{}, errors.New("duplicate nft inventory entry")
			}
			found = true
		}
	}
	if !found {
		return NFTSnapshot{Objects: []json.RawMessage{}}, nil
	}
	data, err = h.run(ctx, []string{"--json", "list", "table", "inet", "duckos"}, nil)
	if err != nil {
		return NFTSnapshot{}, err
	}
	s, err := DecodeNFTSnapshot(data)
	if err != nil || len(s.Objects) == 0 {
		return NFTSnapshot{}, errors.New("owned nft table changed during capture or is unsupported")
	}
	return s, nil
}

func (h *FirewallHost) Check(ctx context.Context, c Config) error {
	p, err := Render(c)
	if err != nil {
		return err
	}
	_, err = h.run(ctx, []string{"--check", "--file", "-"}, []byte(p.NFTables))
	return err
}

// Apply installs one atomic nft batch. Returned kernel readback is evidence
// for subsequent drift checks, not proof of end-to-end network reachability.
func (h *FirewallHost) Apply(ctx context.Context, c Config, before NFTSnapshot) (NFTSnapshot, error) {
	return h.apply(ctx, c, before, false)
}

func (h *FirewallHost) apply(ctx context.Context, c Config, before NFTSnapshot, guarded bool) (NFTSnapshot, error) {
	want, err := before.Fingerprint()
	if err != nil {
		return NFTSnapshot{}, err
	}
	p, err := Render(c)
	if err != nil {
		return NFTSnapshot{}, err
	}
	if guarded {
		p.NFTables += forwardGuardBatch
	}
	if len(p.NFTables) > maxNFTSnapshotBytes {
		return NFTSnapshot{}, errors.New("firewall batch exceeds capacity")
	}
	if _, err = h.run(ctx, []string{"--check", "--file", "-"}, []byte(p.NFTables)); err != nil {
		return NFTSnapshot{}, err
	}
	live, err := h.Capture(ctx)
	if err != nil {
		return NFTSnapshot{}, err
	}
	got, err := live.Fingerprint()
	if err != nil || got != want {
		return NFTSnapshot{}, errors.New("firewall drift before apply; no mutation performed")
	}
	if _, err = h.run(ctx, []string{"--file", "-"}, []byte(p.NFTables)); err != nil {
		return NFTSnapshot{}, errors.New("firewall apply outcome uncertain; transaction recovery required")
	}
	after, err := h.Capture(ctx)
	if err != nil {
		return NFTSnapshot{}, errors.New("firewall readback failed; transaction recovery required")
	}
	if guarded {
		if err := verifyForwardGuard(after); err != nil {
			return NFTSnapshot{}, errors.New("forwarding barrier readback failed; recovery required")
		}
	} else if (len(c.Interfaces) == 0) != (len(after.Objects) == 0) {
		return NFTSnapshot{}, errors.New("firewall activation state does not match intent")
	}
	return after, nil
}

func (h *FirewallHost) Verify(ctx context.Context, expected NFTSnapshot) error {
	want, err := expected.Fingerprint()
	if err != nil {
		return err
	}
	got, err := h.Capture(ctx)
	if err != nil {
		return err
	}
	actual, err := got.Fingerprint()
	if err != nil || actual != want {
		return errors.New("managed firewall differs from recorded kernel state")
	}
	return nil
}

// Restore replays exclusively owned kernel state, not the entire host ruleset.
// A lost apply acknowledgement still requires this restoration, never a retry
// of the candidate. Other administrators must not edit the enrolled table.
func (h *FirewallHost) Restore(ctx context.Context, before NFTSnapshot) error {
	batch, err := before.RestoreBatch()
	if err != nil {
		return err
	}
	if _, err = h.run(ctx, []string{"--json", "--check", "--file", "-"}, batch); err != nil {
		return err
	}
	if _, err = h.run(ctx, []string{"--json", "--file", "-"}, batch); err != nil {
		return errors.New("firewall restore outcome uncertain")
	}
	return h.Verify(ctx, before)
}

// Release deletes a bounded set of objects from the live ruleset without
// touching the surrounding table. The Safe Apply rollback path uses
// this to drop the duckos_apply_guard chain it installed during
// Capture so the next Apply's Capture stage does not trip "reserved
// forwarding guard requires recovery". The function is idempotent: a
// no-op when the target object is absent, so the controller can call
// it from both the rollback path and a future confirm-side release
// without an extra "did we install it?" probe.
func (h *FirewallHost) Release(ctx context.Context, target NFTSnapshot) error {
	if len(target.Objects) == 0 {
		return nil
	}
	batch, err := target.DeleteBatch()
	if err != nil {
		return err
	}
	if _, err = h.run(ctx, []string{"--json", "--file", "-"}, batch); err != nil {
		return errors.New("firewall release outcome uncertain")
	}
	return nil
}

// runNFT routes one bounded nftables operation through the privileged
// duck-nft-launcher over its Unix socket. BETA.3 remediation: the
// agent holds zero capabilities, so direct nft exec fails with
// "Operation not permitted"; the launcher (own systemd unit,
// CAP_NET_ADMIN only) performs the netlink work after enforcing the
// DuckOS table-ownership allowlist. The args/input split of the old
// direct-exec seam is preserved so every caller (Capture / Check /
// Apply / Restore / Release / forward guard) keeps its exact
// semantics; only the transport changed. Non-Linux builds keep the
// historical "unavailable" behaviour.
func runNFT(parent context.Context, args []string, input []byte) ([]byte, error) {
	if runtime.GOOS != "linux" || len(input) > 1<<20 {
		return nil, errors.New("nft operation unavailable")
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	client := newLauncherClient()
	switch {
	case len(args) >= 3 && args[1] == "list" && args[2] == "tables":
		return client.ListTablesJSON()
	case len(args) >= 5 && args[1] == "list" && args[2] == "table":
		return client.ListTableJSON(args[3], args[4])
	case slices.Contains(args, "--check"):
		var err error
		if slices.Contains(args, "--json") {
			err = client.CheckJSON(input)
		} else {
			err = client.Check(input)
		}
		if err != nil {
			return nil, err
		}
		return nil, nil
	case slices.Contains(args, "--file"):
		var err error
		if slices.Contains(args, "--json") {
			err = client.ApplyJSON(input)
		} else {
			err = client.Apply(input)
		}
		if err != nil {
			return nil, err
		}
		return nil, nil
	default:
		return nil, errors.New("nft operation unavailable")
	}
}

// newLauncherClient returns the launcher transport. Package-level var
// so tests can inject a socket-backed fake.
var newLauncherClient = func() launcherTransport { return nftlaunch.NewClient() }

// launcherTransport is the seam the firewall host drives; it mirrors
// the nftlaunch.Client surface the runNFT dispatch uses.
type launcherTransport interface {
	ListTablesJSON() ([]byte, error)
	ListTableJSON(family, table string) ([]byte, error)
	Check(batch []byte) error
	CheckJSON(batch []byte) error
	Apply(batch []byte) error
	ApplyJSON(batch []byte) error
}
