package logging

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoggerRespectsPerSubsystemLevel(t *testing.T) {
	l := New(DefaultConfig())
	l.SetLevel("wireguard", Errors)
	l.SetLevel("xray", Detailed)
	if l.Level("wireguard") != Errors {
		t.Fatal("wireguard dial was not set")
	}
	if l.Level("xray") != Detailed {
		t.Fatal("xray dial was not set")
	}
	l.Log("wireguard", SevError, "wg failed", nil)
	l.Log("wireguard", SevInfo, "wg info", nil)
	l.Log("xray", SevError, "xray failed", nil)
	l.Log("xray", SevDebug, "xray debug", nil)
	records := l.Snapshot()
	// 3 admitted: wg error, xray error, xray debug. wg info dropped.
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	for _, r := range records {
		if r.Subsystem == "wireguard" && r.Severity != SevError {
			t.Fatal("wireguard info was not filtered")
		}
	}
}

func TestLoggerRedactsSensitiveFields(t *testing.T) {
	l := New(DefaultConfig())
	l.Log("xray", SevInfo, "applied user", map[string]string{
		"uuid":    "01234567-89ab-cdef-0123-456789abcdef",
		"private": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"name":    "alice",
	})
	recs := l.Snapshot()
	if len(recs) != 1 {
		t.Fatal("expected one record, got", len(recs))
	}
	if !strings.HasPrefix(recs[0].Context["private"], "redacted:") {
		t.Fatal("private value was not redacted", recs[0].Context["private"])
	}
	if !strings.HasPrefix(recs[0].Context["uuid"], "redacted:") {
		t.Fatal("uuid was not redacted", recs[0].Context["uuid"])
	}
	if recs[0].Context["name"] != "alice" {
		t.Fatal("non-sensitive value was redacted")
	}
}

func TestLoggerRingIsBounded(t *testing.T) {
	l := New(Config{RingCapacity: 4, Replacer: DefaultReplacer})
	for i := 0; i < 100; i++ {
		l.Log("x", SevInfo, "msg", nil)
	}
	records := l.Snapshot()
	if len(records) != 4 {
		t.Fatal("ring exceeded capacity", len(records))
	}
	emitted, dropped := l.Counts()
	if emitted != 100 {
		t.Fatal("emitted counter did not match", emitted)
	}
	if dropped == 0 {
		t.Fatal("dropped counter did not advance")
	}
}

func TestLoggerRingPreservesNewestRecords(t *testing.T) {
	l := New(Config{RingCapacity: 3, Replacer: DefaultReplacer})
	for i := 0; i < 5; i++ {
		l.Log("x", SevInfo, "msg", map[string]string{"i": string(rune('A' + i))})
	}
	records := l.Snapshot()
	if len(records) != 3 {
		t.Fatal("ring did not cap at 3", len(records))
	}
	// The newest three should be C, D, E.
	if records[0].Context["i"] != "C" || records[2].Context["i"] != "E" {
		t.Fatal("ring order wrong", records)
	}
}

func TestLoggerSubscribersReceiveRecords(t *testing.T) {
	l := New(DefaultConfig())
	ch, err := l.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	l.Log("x", SevError, "boom", nil)
	select {
	case rec := <-ch:
		if rec.Message != "boom" {
			t.Fatal("subscriber got wrong message", rec.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive a record")
	}
	l.Unsubscribe(ch)
}

func TestLoggerSubscriberLimitIsBounded(t *testing.T) {
	l := New(Config{MaxSubscribers: 2, Replacer: DefaultReplacer})
	ch1, err := l.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	ch2, err := l.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Subscribe(); err == nil {
		t.Fatal("subscriber limit was not enforced")
	}
	l.Unsubscribe(ch1)
	l.Unsubscribe(ch2)
}

func TestLoggerContextTruncatesAtLimit(t *testing.T) {
	l := New(Config{MaxContextBytes: 32, Replacer: DefaultReplacer})
	ctx := map[string]string{}
	for i := 0; i < 10; i++ {
		ctx[string(rune('A'+i))] = "value"
	}
	l.Log("x", SevInfo, "m", ctx)
	recs := l.Snapshot()
	if len(recs) != 1 {
		t.Fatal("expected one record")
	}
	// The implementation replaces the value with "context-truncated"
	// when the size limit is exceeded, so at least one entry must
	// carry that marker.
	truncated := false
	for _, v := range recs[0].Context {
		if v == "context-truncated" {
			truncated = true
			break
		}
	}
	if !truncated {
		t.Fatal("context was not truncated", recs[0].Context)
	}
}

func TestLoggerConcurrentEmitDoesNotRace(t *testing.T) {
	l := New(Config{RingCapacity: 32, Replacer: DefaultReplacer})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				l.Log("x", SevInfo, "m", map[string]string{"k": "v"})
			}
		}()
	}
	wg.Wait()
	emitted, _ := l.Counts()
	if emitted != 800 {
		t.Fatal("emitted count is wrong", emitted)
	}
}

func TestParseLevelRoundTrip(t *testing.T) {
	for _, level := range []Level{Off, Errors, Normal, Detailed, Trace} {
		if level, err := ParseLevel(level.String()); err != nil || level.String() != level.String() {
			t.Fatal("level did not round-trip", level, err)
		}
	}
	if _, err := ParseLevel("loud"); err == nil {
		t.Fatal("unknown level was accepted")
	}
}

func TestLevelAdmitsMonotonic(t *testing.T) {
	cases := []struct {
		dial Level
		sev  Severity
		want bool
	}{
		{Off, SevError, false},
		{Off, SevTrace, false},
		{Errors, SevError, true},
		{Errors, SevInfo, false},
		{Normal, SevInfo, true},
		{Normal, SevDebug, false},
		{Detailed, SevDebug, true},
		{Detailed, SevTrace, false},
		{Trace, SevTrace, true},
	}
	for _, c := range cases {
		if got := levelAdmits(c.dial, c.sev); got != c.want {
			t.Fatalf("dial=%v sev=%v got=%v want=%v", c.dial, c.sev, got, c.want)
		}
	}
}

func TestLoggerAfterCloseAcceptsNoMoreRecords(t *testing.T) {
	l := New(DefaultConfig())
	l.Close()
	l.Log("x", SevError, "boom", nil)
	if records := l.Snapshot(); len(records) != 0 {
		t.Fatal("closed logger accepted a record")
	}
}

func TestRedactLikelyWireGuardKey(t *testing.T) {
	// Base64 of 32 bytes is exactly 44 characters.
	key := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqr"
	if len(key) != 44 {
		t.Fatalf("test key has wrong length: %d", len(key))
	}
	if !isLikelyWireGuardKey(key) {
		t.Fatalf("valid WG key was rejected: %s (len=%d)", key, len(key))
	}
	if isLikelyWireGuardKey("not a key") {
		t.Fatal("invalid WG key was accepted")
	}
}

func TestRedactLikelyUUID(t *testing.T) {
	if !isLikelyUUID("01234567-89ab-cdef-0123-456789abcdef") {
		t.Fatal("valid UUID was rejected")
	}
	if isLikelyUUID("01234567-89ab-cdef-0123-456789abcde") {
		t.Fatal("short UUID was accepted")
	}
}

func TestLoggerDefaultReplacerHandlesEmpty(t *testing.T) {
	if DefaultReplacer("k", "") != "" {
		t.Fatal("empty value should be empty")
	}
}

func TestLoggerFingerprintIsStable(t *testing.T) {
	a := fingerprint("hello")
	b := fingerprint("hello")
	if a != b {
		t.Fatal("fingerprint was not stable")
	}
	if fingerprint("hello") == fingerprint("world") {
		t.Fatal("fingerprints collided")
	}
}
