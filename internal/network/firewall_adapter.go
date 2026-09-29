package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

type firewallKernel interface {
	Check(context.Context, Config) error
	Capture(context.Context) (NFTSnapshot, error)
	Apply(context.Context, Config, NFTSnapshot) (NFTSnapshot, error)
	Verify(context.Context, NFTSnapshot) error
	Restore(context.Context, NFTSnapshot) error
	Release(context.Context, NFTSnapshot) error
}

// FirewallAdapter connects real nft operations to the durable Safe Apply
// controller. Enrollment must already grant exclusive inet/duckos ownership.
// It deliberately refuses changes to links, addresses, routes or DNS: those
// require the composite network adapter and separate reachability evidence.
// Kernel equality alone is not management-path or packet-filter qualification.
type FirewallAdapter struct {
	mu             sync.Mutex
	host           firewallKernel
	captured       *firewallRecovery
	verifiedConfig string
	verified       NFTSnapshot
}

type firewallRecovery struct {
	Version   int          `json:"version"`
	Before    string       `json:"before"`
	Candidate string       `json:"candidate"`
	Kernel    *NFTSnapshot `json:"kernel"`
}

func NewFirewallAdapter() (*FirewallAdapter, error) {
	h, err := NewFirewallHost()
	if err != nil {
		return nil, err
	}
	return &FirewallAdapter{host: h}, nil
}

func firewallPair(before, next json.RawMessage) (Config, Config, error) {
	b, err := Decode(before)
	if err != nil {
		return b, Config{}, err
	}
	n, err := Decode(next)
	if err != nil {
		return b, n, err
	}
	left, right := b, n
	left.Firewall, right.Firewall = Firewall{}, Firewall{}
	x, _ := json.Marshal(left)
	y, _ := json.Marshal(right)
	if !bytes.Equal(x, y) {
		return b, n, errors.New("firewall adapter cannot apply non-firewall changes")
	}
	return b, n, nil
}

func configFingerprint(c Config) string {
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (a *FirewallAdapter) Preflight(ctx context.Context, before, next json.RawMessage) error {
	_, n, err := firewallPair(before, next)
	if err != nil {
		return err
	}
	return a.host.Check(ctx, n)
}

func (a *FirewallAdapter) Capture(ctx context.Context, before, next json.RawMessage) (json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, n, err := firewallPair(before, next)
	if err != nil {
		return nil, err
	}
	kernel, err := a.host.Capture(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := kernel.Fingerprint(); err != nil {
		return nil, err
	}
	evidence := firewallRecovery{Version: 1, Before: configFingerprint(b), Candidate: configFingerprint(n), Kernel: &kernel}
	raw, err := json.Marshal(evidence)
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("firewall recovery evidence exceeds limit")
	}
	a.captured = &evidence
	return raw, nil
}

func (a *FirewallAdapter) Apply(ctx context.Context, before, next json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, n, err := firewallPair(before, next)
	if err != nil {
		return err
	}
	if a.captured == nil || a.captured.Before != configFingerprint(b) || a.captured.Candidate != configFingerprint(n) {
		return errors.New("firewall apply lacks matching captured evidence")
	}
	a.verifiedConfig = ""
	kernel := *a.captured.Kernel
	a.captured = nil // A capture authorizes only one attempt, never blind replay.
	after, err := a.host.Apply(ctx, n, kernel)
	if err != nil {
		return err
	}
	a.verified, a.verifiedConfig = after, configFingerprint(n)
	return nil
}

func (a *FirewallAdapter) Verify(ctx context.Context, config json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, err := Decode(config)
	if err != nil {
		return err
	}
	if a.verifiedConfig == "" || configFingerprint(c) != a.verifiedConfig {
		return errors.New("firewall verification has no matching kernel readback")
	}
	return a.host.Verify(ctx, a.verified)
}

func (a *FirewallAdapter) Restore(context.Context, json.RawMessage, json.RawMessage) error {
	return errors.New("firewall recovery requires durable captured kernel evidence")
}

func (a *FirewallAdapter) RestoreCaptured(ctx context.Context, before, next, raw json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, n, err := firewallPair(before, next)
	if err != nil {
		return err
	}
	var evidence firewallRecovery
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 1<<20 || d.Decode(&evidence) != nil || d.Decode(new(any)) != io.EOF || evidence.Version != 1 || evidence.Kernel == nil || evidence.Before != configFingerprint(b) || evidence.Candidate != configFingerprint(n) {
		return errors.New("firewall recovery evidence does not match transaction")
	}
	if _, err := evidence.Kernel.Fingerprint(); err != nil {
		return err
	}
	a.verifiedConfig = ""
	if err := a.host.Restore(ctx, *evidence.Kernel); err != nil {
		return err
	}
	a.verified, a.verifiedConfig = *evidence.Kernel, configFingerprint(b)
	a.captured = nil
	return nil
}
