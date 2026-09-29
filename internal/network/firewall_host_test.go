package network

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const syntheticNFTTable = `{"nftables":[{"metainfo":{"json_schema_version":1}},{"table":{"family":"inet","name":"duckos","handle":1}},{"chain":{"family":"inet","table":"duckos","name":"input","handle":2,"type":"filter","hook":"input","prio":0,"policy":"drop"}},{"rule":{"family":"inet","table":"duckos","chain":"input","handle":3,"expr":[{"counter":{"packets":9,"bytes":400}},{"accept":null}]}}]}`

func TestNFTFingerprintAndScopedRestore(t *testing.T) {
	s, err := DecodeNFTSnapshot([]byte(syntheticNFTTable))
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.ReplaceAll(syntheticNFTTable, `"handle":3`, `"handle":300`)
	changed = strings.ReplaceAll(changed, `"packets":9,"bytes":400`, `"packets":9000,"bytes":400000`)
	other, err := DecodeNFTSnapshot([]byte(changed))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.Fingerprint()
	b, _ := other.Fingerprint()
	if a != b {
		t.Fatal("dynamic counters or handles changed fingerprint")
	}
	other, err = DecodeNFTSnapshot([]byte(strings.ReplaceAll(changed, `"accept":null`, `"drop":null`)))
	if err != nil {
		t.Fatal(err)
	}
	b, _ = other.Fingerprint()
	if a == b {
		t.Fatal("rule drift hidden")
	}
	batch, err := s.RestoreBatch()
	if err != nil || strings.Contains(string(batch), "handle") || !strings.Contains(string(batch), `"destroy":{"table":{"family":"inet","name":"duckos"}}`) {
		t.Fatal("unsafe recovery batch", err)
	}
	for _, raw := range []string{
		`{}`,
		`{"nftables":null}`,
		strings.ReplaceAll(syntheticNFTTable, `"duckos"`, `"foreign"`),
		strings.ReplaceAll(syntheticNFTTable, `"inet"`, `"ip"`),
		`{"nftables":[{"flush":{"ruleset":null}}]}`,
		`{"nftables":[{"rule":{"family":"inet","table":"duckos"}}]}`,
	} {
		if _, err := DecodeNFTSnapshot([]byte(raw)); err == nil {
			t.Fatal("unowned/command-shaped snapshot accepted")
		}
	}
}

func TestFirewallApplyChecksAndReadsRealOperationBoundary(t *testing.T) {
	var calls []string
	active := false
	h := &FirewallHost{run: func(_ context.Context, args []string, input []byte) ([]byte, error) {
		operation := strings.Join(args, " ")
		calls = append(calls, operation)
		switch operation {
		case "--check --file -":
			if !strings.HasPrefix(string(input), "destroy table inet duckos\n") {
				t.Fatal("not a complete owned batch")
			}
		case "--json list tables":
			if active {
				return []byte(`{"nftables":[{"table":{"family":"inet","name":"duckos"}}]}`), nil
			}
			return []byte(`{"nftables":[]}`), nil
		case "--file -":
			active = true
		case "--json list table inet duckos":
			return []byte(syntheticNFTTable), nil
		default:
			t.Fatal("unexpected operation", operation)
		}
		return nil, nil
	}}
	s, err := h.Apply(context.Background(), fixture(), NFTSnapshot{})
	if err != nil || len(s.Objects) == 0 {
		t.Fatal("apply failed", err)
	}
	if strings.Join(calls, ";") != "--check --file -;--json list tables;--file -;--json list tables;--json list table inet duckos" {
		t.Fatal("unsafe command ordering", calls)
	}
	if err = h.Verify(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	// Once kernel state differs from the supplied baseline, no mutation occurs.
	calls = nil
	if _, err = h.Apply(context.Background(), fixture(), NFTSnapshot{}); err == nil {
		t.Fatal("drift overwritten")
	}
	for _, call := range calls {
		if call == "--file -" {
			t.Fatal("mutated despite drift")
		}
	}
}

func TestFirewallInventoryAndCommandFailureNeverMeanAbsent(t *testing.T) {
	for _, raw := range []string{`{}`, `{"nftables":null}`, `{"nftables":[{}]}`, `{"nftables":[{"rule":{}}]}`, `broken`} {
		h := &FirewallHost{run: func(context.Context, []string, []byte) ([]byte, error) { return []byte(raw), nil }}
		if _, err := h.Capture(context.Background()); err == nil {
			t.Fatal("unreadable state treated as empty", raw)
		}
	}
	h := &FirewallHost{run: func(context.Context, []string, []byte) ([]byte, error) {
		return nil, errors.New("synthetic netlink failure")
	}}
	if _, err := h.Capture(context.Background()); err == nil {
		t.Fatal("failed inventory treated as empty")
	}
}

func TestFirewallRestoreVerifiesKernelReadback(t *testing.T) {
	s, err := DecodeNFTSnapshot([]byte(syntheticNFTTable))
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	h := &FirewallHost{run: func(_ context.Context, args []string, input []byte) ([]byte, error) {
		op := strings.Join(args, " ")
		calls = append(calls, op)
		switch op {
		case "--json --check --file -", "--json --file -":
			if !json.Valid(input) {
				t.Fatal("invalid restore JSON")
			}
		case "--json list tables":
			return []byte(`{"nftables":[{"table":{"family":"inet","name":"duckos"}}]}`), nil
		case "--json list table inet duckos":
			return []byte(syntheticNFTTable), nil
		default:
			t.Fatal("unexpected operation", op)
		}
		return nil, nil
	}}
	if err = h.Restore(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 4 || calls[0] != "--json --check --file -" || calls[1] != "--json --file -" {
		t.Fatal("restore skipped checks", calls)
	}
}
