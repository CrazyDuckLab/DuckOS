// G14 bounded logging / trace jobs torture regression.
package logsink

import (
	"sync"
	"testing"
	"time"
)

func sampleTrace(id string) Trace {
	return Trace{
		ID:        id,
		Component: "network.firewall",
		StartedAt: time.Now().UTC(),
		State:     "running",
	}
}

func sampleEntry(traceID string) Entry {
	return Entry{
		TraceID:   traceID,
		Component: "network.firewall",
		Severity:  SeverityInfo,
		Message:   "dropped packet",
		At:        time.Now().UTC(),
	}
}

func TestSeverity_Valid(t *testing.T) {
	for _, s := range []Severity{SeverityTrace, SeverityDebug, SeverityInfo, SeverityWarning, SeverityError, SeverityFatal} {
		if !s.Valid() {
			t.Errorf("%q must be valid", s)
		}
	}
	if Severity("garbage").Valid() {
		t.Errorf("garbage must be invalid")
	}
}

func TestEntry_Validate(t *testing.T) {
	e := sampleEntry("trace-001")
	if err := e.Validate(); err != nil {
		t.Fatalf("valid entry: %v", err)
	}
	// Bad trace id
	bad := sampleEntry("X")
	if err := bad.Validate(); err == nil {
		t.Fatalf("bad trace id must be rejected")
	}
	// Bad component
	bad2 := sampleEntry("trace-002")
	bad2.Component = "Bad"
	if err := bad2.Validate(); err == nil {
		t.Fatalf("bad component must be rejected")
	}
	// Bad severity
	bad3 := sampleEntry("trace-003")
	bad3.Severity = Severity("garbage")
	if err := bad3.Validate(); err == nil {
		t.Fatalf("bad severity must be rejected")
	}
	// Zero at
	bad4 := sampleEntry("trace-004")
	bad4.At = time.Time{}
	if err := bad4.Validate(); err == nil {
		t.Fatalf("zero at must be rejected")
	}
}

func TestTrace_Validate(t *testing.T) {
	tr := sampleTrace("trace-001")
	if err := tr.Validate(); err != nil {
		t.Fatalf("valid trace: %v", err)
	}
	bad := sampleTrace("X")
	if err := bad.Validate(); err == nil {
		t.Fatalf("bad trace id must be rejected")
	}
	bad2 := sampleTrace("trace-002")
	bad2.State = "pending"
	if err := bad2.Validate(); err == nil {
		t.Fatalf("bad state must be rejected")
	}
}

func TestStore_StartAndAppend(t *testing.T) {
	s := NewStore()
	ok, err := s.StartTrace(sampleTrace("trace-001"))
	if !ok || err != nil {
		t.Fatalf("StartTrace: ok=%v err=%v", ok, err)
	}
	if err := s.Append(sampleEntry("trace-001")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	entries := s.Entries("trace-001")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestStore_StartRejectsDuplicate(t *testing.T) {
	s := NewStore()
	if _, err := s.StartTrace(sampleTrace("trace-001")); err != nil {
		t.Fatal(err)
	}
	ok, err := s.StartTrace(sampleTrace("trace-001"))
	if ok {
		t.Fatalf("duplicate must yield ok=false")
	}
	if err != nil {
		t.Fatalf("duplicate must not error: %v", err)
	}
}

func TestStore_AppendRejectsUnknownTrace(t *testing.T) {
	s := NewStore()
	if err := s.Append(sampleEntry("no-such")); err == nil {
		t.Fatalf("unknown trace must be rejected")
	}
}

func TestStore_AppendRejectsAfterEnd(t *testing.T) {
	s := NewStore()
	_, _ = s.StartTrace(sampleTrace("trace-001"))
	if err := s.EndTrace("trace-001", "succeeded"); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(sampleEntry("trace-001")); err == nil {
		t.Fatalf("append to ended trace must be rejected")
	}
}

func TestStore_EndTraceRejectsBadState(t *testing.T) {
	s := NewStore()
	_, _ = s.StartTrace(sampleTrace("trace-001"))
	if err := s.EndTrace("trace-001", "weird"); err == nil {
		t.Fatalf("bad end state must be rejected")
	}
}

func TestStore_EndTraceUnknownID(t *testing.T) {
	s := NewStore()
	if err := s.EndTrace("no-such", "succeeded"); err == nil {
		t.Fatalf("unknown id must surface error")
	}
}

func TestStore_TrimsEntriesToMax(t *testing.T) {
	s := NewStore()
	s.SetLimits(3, 32, time.Hour)
	_, _ = s.StartTrace(sampleTrace("trace-001"))
	base := time.Now().UTC()
	for i := 0; i < 10; i++ {
		e := sampleEntry("trace-001")
		e.At = base.Add(time.Duration(i) * time.Millisecond)
		if err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	entries := s.Entries("trace-001")
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries after trim, got %d", len(entries))
	}
}

func TestStore_TrimsTracesToMax(t *testing.T) {
	s := NewStore()
	s.SetLimits(64, 3, time.Hour)
	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		tr := sampleTrace("trace-" + string(rune('a'+i)))
		tr.StartedAt = base.Add(time.Duration(i) * time.Millisecond)
		if _, err := s.StartTrace(tr); err != nil {
			t.Fatal(err)
		}
	}
	all := s.AllTraces()
	if len(all) != 3 {
		t.Fatalf("expected 3 traces after trim, got %d", len(all))
	}
}

func TestStore_AllTracesNewestFirst(t *testing.T) {
	s := NewStore()
	base := time.Now().UTC()
	ids := []string{"trace-aaa", "trace-bbb", "trace-ccc"}
	for i, id := range ids {
		tr := sampleTrace(id)
		tr.StartedAt = base.Add(time.Duration(i) * time.Second)
		_, _ = s.StartTrace(tr)
	}
	all := s.AllTraces()
	if len(all) != 3 || all[0].ID != "trace-ccc" {
		t.Fatalf("newest first failed: %+v", all)
	}
}

func TestStore_GCExpired(t *testing.T) {
	s := NewStore()
	s.SetLimits(64, 32, 50*time.Millisecond)
	_, _ = s.StartTrace(sampleTrace("trace-001"))
	if err := s.EndTrace("trace-001", "succeeded"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	if removed := s.GCExpired(); removed != 1 {
		t.Fatalf("expected 1 expired trace removed, got %d", removed)
	}
	if _, ok := s.Trace("trace-001"); ok {
		t.Fatalf("expired trace must be removed")
	}
}

func TestStore_GCExpiredKeepsRecent(t *testing.T) {
	s := NewStore()
	_, _ = s.StartTrace(sampleTrace("trace-001"))
	if err := s.EndTrace("trace-001", "succeeded"); err != nil {
		t.Fatal(err)
	}
	if removed := s.GCExpired(); removed != 0 {
		t.Fatalf("recent trace must not be GC'd, got %d", removed)
	}
}

func TestStore_Counters(t *testing.T) {
	s := NewStore()
	_, _ = s.StartTrace(sampleTrace("trace-001"))
	_ = s.Append(sampleEntry("trace-001"))
	c := s.Counters()
	if c.Traces != 1 || c.Running != 1 || c.Entries != 1 {
		t.Fatalf("counter drift: %+v", c)
	}
}

func TestStore_ConcurrentAppend(t *testing.T) {
	s := NewStore()
	_, _ = s.StartTrace(sampleTrace("trace-001"))
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Append(sampleEntry("trace-001"))
		}()
	}
	wg.Wait()
	if c := s.Counters(); c.Entries != 32 {
		t.Fatalf("expected 32 entries, got %d", c.Entries)
	}
}
