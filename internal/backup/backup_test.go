package backup

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func newEntry(path, kind, body string) Entry {
	return Entry{
		Path:   path,
		Kind:   Kind(kind),
		Size:   int64(len(body)),
		Digest: Digest([]byte(body)),
		Mode:   0o600,
		MTime:  time.Now().Unix(),
		Owner:  "root",
	}
}

func TestEntryValid(t *testing.T) {
	e := newEntry("etc/duckos/network.json", "config", "{}")
	if err := e.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestEntryRejectsBadPath(t *testing.T) {
	e := newEntry("../etc/passwd", "config", "")
	if err := e.Valid(); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestEntryRejectsBadKind(t *testing.T) {
	e := newEntry("a", "weird", "")
	if err := e.Valid(); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestEntryRejectsBadDigest(t *testing.T) {
	e := newEntry("a", "config", "")
	e.Digest = "not-hex"
	if err := e.Valid(); err == nil {
		t.Fatal("bad digest accepted")
	}
}

func TestEntryRejectsBadMode(t *testing.T) {
	e := newEntry("a", "config", "")
	e.Mode = 0o10000
	if err := e.Valid(); err == nil {
		t.Fatal("oversized mode accepted")
	}
}

func TestEntryRejectsOversized(t *testing.T) {
	e := newEntry("a", "config", "")
	e.Size = MaxSingleFileBytes + 1
	if err := e.Valid(); err == nil {
		t.Fatal("oversized size accepted")
	}
}

func TestManifestValid(t *testing.T) {
	m, err := Build([]Entry{
		newEntry("etc/a", "config", "alpha"),
		newEntry("etc/b", "config", "beta"),
	}, "host-1", "duckos-dev", "scheduled", "admin", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.Entries))
	}
}

func TestManifestRejectsEmpty(t *testing.T) {
	if _, err := Build(nil, "host", "duckos", "", "", time.Now().UTC()); err == nil {
		t.Fatal("empty manifest accepted")
	}
}

func TestManifestRejectsDuplicate(t *testing.T) {
	entries := []Entry{
		newEntry("etc/a", "config", "x"),
		newEntry("etc/a", "config", "y"),
	}
	if _, err := Build(entries, "h", "v", "", "", time.Now().UTC()); err == nil {
		t.Fatal("duplicate path accepted")
	}
}

func TestManifestRejectsBadOperator(t *testing.T) {
	entries := []Entry{newEntry("etc/a", "config", "x")}
	if _, err := Build(entries, "h", "v", "", "root; rm -rf /", time.Now().UTC()); err == nil {
		t.Fatal("unsafe operator accepted")
	}
}

func TestManifestRejectsOversizedReason(t *testing.T) {
	entries := []Entry{newEntry("etc/a", "config", "x")}
	if _, err := Build(entries, "h", "v", strings.Repeat("a", 257), "", time.Now().UTC()); err == nil {
		t.Fatal("oversized reason accepted")
	}
}

func TestManifestRejectsTotalOverflow(t *testing.T) {
	entries := make([]Entry, MaxEntries+1)
	for i := range entries {
		entries[i] = newEntry("e/"+itoa(i), "config", "x")
	}
	if _, err := Build(entries, "h", "v", "", "", time.Now().UTC()); err == nil {
		t.Fatal("too many entries accepted")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	m, err := Build([]Entry{
		newEntry("etc/a", "config", "alpha"),
		newEntry("etc/b", "vault", "encrypted"),
	}, "host-1", "duckos", "reason", "admin", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	data, err := EncodeManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxManifestBytes {
		t.Fatalf("manifest exceeded size cap: %d", len(data))
	}
	got, err := DecodeManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("entries lost in roundtrip: %d", len(got.Entries))
	}
}

func TestEncodeRefusesBadManifest(t *testing.T) {
	m := Manifest{SchemaVersion: "wrong", CreatedAt: time.Now().UTC(), Entries: []Entry{newEntry("a", "config", "x")}}
	if _, err := EncodeManifest(m); err == nil {
		t.Fatal("bad manifest accepted")
	}
}

func TestDecodeRefusesOversized(t *testing.T) {
	huge := make([]byte, MaxManifestBytes+1)
	if _, err := DecodeManifest(huge); err == nil {
		t.Fatal("oversized manifest accepted")
	}
}

func TestDecodeRefusesMalformed(t *testing.T) {
	if _, err := DecodeManifest([]byte("not json")); err == nil {
		t.Fatal("malformed manifest accepted")
	}
}

func TestDecodeRefusesUnknownField(t *testing.T) {
	m, _ := Build([]Entry{newEntry("a", "config", "x")}, "h", "v", "", "", time.Now().UTC())
	data, _ := json.Marshal(m)
	tampered := bytes.Replace(data, []byte(`"hostId"`), []byte(`"hostId":"h","extra":"x",`+"\"hostId\":\""), 1)
	if _, err := DecodeManifest(tampered); err == nil {
		t.Fatal("extra field accepted")
	}
}

func TestDigest(t *testing.T) {
	if Digest([]byte("hello")) != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatal("digest mismatch")
	}
}

func TestManifestSortedDeterministic(t *testing.T) {
	fixed := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	a, _ := Build([]Entry{
		newEntry("c", "config", "same"),
		newEntry("a", "config", "same"),
		newEntry("b", "config", "same"),
	}, "h", "v", "", "", fixed)
	b, _ := Build([]Entry{
		newEntry("a", "config", "same"),
		newEntry("b", "config", "same"),
		newEntry("c", "config", "same"),
	}, "h", "v", "", "", fixed)
	if !bytes.Equal(mustEncode(t, a), mustEncode(t, b)) {
		t.Fatal("manifest encoding not deterministic")
	}
}

func mustEncode(t *testing.T, m Manifest) []byte {
	t.Helper()
	data, err := EncodeManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	const digits = "0123456789"
	if i < 0 {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}
