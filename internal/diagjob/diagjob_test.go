// G19 diagnostic job service torture regression.
package diagjob

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func sampleJob(id string) Job {
	return Job{
		ID:          id,
		Kind:        KindDNSTrace,
		Status:      StatusSucceeded,
		Subject:     "host.example.com",
		RequestedBy: "operator",
		StartedAt:   time.Unix(1, 0).UTC(),
		EndedAt:     time.Unix(2, 0).UTC(),
		OutputLines: []string{"resolved A 198.51.100.10", "resolved AAAA 2001:db8::10"},
	}
}

func TestJobValidate_RejectsBadID(t *testing.T) {
	for _, id := range []string{"", "Bad ID", "abc", "abcd!", "ABCD", "1234", "ab cd"} {
		j := sampleJob(id)
		if err := j.Validate(); err == nil {
			t.Errorf("id=%q must be rejected", id)
		}
	}
}

func TestJobValidate_AcceptsCanonicalID(t *testing.T) {
	for _, id := range []string{"dns-1", "wan-probe-a", "audit_42", "j.0a"} {
		j := sampleJob(id)
		if err := j.Validate(); err != nil {
			t.Errorf("id=%q must be valid, got %v", id, err)
		}
	}
}

func TestJobValidate_RejectsBadKind(t *testing.T) {
	j := sampleJob("dns-1")
	j.Kind = JobKind("garbage")
	if err := j.Validate(); err == nil {
		t.Fatalf("bad kind must be rejected")
	}
}

func TestJobValidate_RejectsBadStatus(t *testing.T) {
	j := sampleJob("dns-1")
	j.Status = JobStatus("garbage")
	if err := j.Validate(); err == nil {
		t.Fatalf("bad status must be rejected")
	}
}

func TestJobValidate_RejectsLongSubject(t *testing.T) {
	j := sampleJob("dns-1")
	j.Subject = strings.Repeat("x", 257)
	if err := j.Validate(); err == nil {
		t.Fatalf("long subject must be rejected")
	}
}

func TestJobValidate_RejectsLongLine(t *testing.T) {
	j := sampleJob("dns-1")
	j.OutputLines = []string{strings.Repeat("x", 4097)}
	if err := j.Validate(); err == nil {
		t.Fatalf("long line must be rejected")
	}
}

func TestStore_AppendAndGet(t *testing.T) {
	s := NewStore()
	j := sampleJob("dns-1")
	if err := s.Append(j); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, ok := s.Get("dns-1")
	if !ok {
		t.Fatalf("Get must return the job")
	}
	if got.Subject != j.Subject || len(got.OutputLines) != 2 {
		t.Fatalf("drift: %+v", got)
	}
}

func TestStore_ListIsNewestFirst(t *testing.T) {
	s := NewStore()
	for _, id := range []string{"dns-a", "dns-b", "dns-c"} {
		j := sampleJob(id)
		j.StartedAt = time.Now().Add(time.Duration(len(id)) * time.Second).UTC()
		if err := s.Append(j); err != nil {
			t.Fatal(err)
		}
	}
	list := s.List()
	if len(list) != 3 || list[0].ID != "dns-c" {
		t.Fatalf("newest first failed: %v", list)
	}
}

func TestStore_ListByKind(t *testing.T) {
	s := NewStore()
	_ = s.Append(sampleJob("dns-a"))
	j := sampleJob("wan-b")
	j.Kind = KindWANProbe
	_ = s.Append(j)
	_ = s.Append(sampleJob("dns-c"))
	got := s.ListByKind(KindDNSTrace)
	if len(got) != 2 {
		t.Fatalf("expected 2 dns-trace jobs, got %d", len(got))
	}
}

func TestStore_TrimsOutputToMaxBytes(t *testing.T) {
	s := NewStore()
	s.SetMax(64, 32)
	j := sampleJob("dns-1")
	j.OutputLines = []string{"abcdefghij", "klmnopqrst", "uvwxyzabcd", "0123456789"}
	if err := s.Append(j); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get("dns-1")
	if !got.Truncated {
		t.Fatalf("truncated flag must be set")
	}
	if len(got.OutputLines) >= 4 {
		t.Fatalf("output must be trimmed, got %d lines", len(got.OutputLines))
	}
}

func TestStore_TrimsToMaxJobs(t *testing.T) {
	s := NewStore()
	s.SetMax(3, 256<<10)
	for i := 0; i < 5; i++ {
		_ = s.Append(sampleJob("job-" + itoa(i)))
	}
	if len(s.List()) != 3 {
		t.Fatalf("expected 3 jobs after trim, got %d", len(s.List()))
	}
	if _, ok := s.Get("job-0"); ok {
		t.Fatalf("oldest job must be evicted")
	}
}

func TestStore_Purge(t *testing.T) {
	s := NewStore()
	_ = s.Append(sampleJob("dns-a"))
	s.Purge("dns-a")
	if _, ok := s.Get("dns-a"); ok {
		t.Fatalf("purged job must not survive")
	}
	s.Purge("does-not-exist")
}

func TestStore_UpdatePreservesOrder(t *testing.T) {
	s := NewStore()
	_ = s.Append(sampleJob("dns-a"))
	_ = s.Append(sampleJob("dns-b"))
	_ = s.Append(sampleJob("dns-c"))
	// Update dns-a, list must still be dns-c, dns-b, dns-a.
	if err := s.Append(sampleJob("dns-a")); err != nil {
		t.Fatal(err)
	}
	list := s.List()
	if len(list) != 3 || list[0].ID != "dns-c" || list[2].ID != "dns-a" {
		t.Fatalf("update must not reorder: %v", list)
	}
}

func TestStore_ConcurrentAppend(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.Append(sampleJob("job-" + itoa(i)))
		}(i)
	}
	wg.Wait()
	if len(s.List()) != 32 {
		t.Fatalf("expected 32 jobs, got %d", len(s.List()))
	}
}

func TestAllKinds(t *testing.T) {
	for _, k := range AllKinds() {
		if !k.Valid() {
			t.Errorf("AllKinds returned invalid kind %q", k)
		}
	}
}

func TestJobDuration(t *testing.T) {
	j := sampleJob("a")
	if d := j.Duration(); d != time.Second {
		t.Fatalf("duration must be 1s, got %v", d)
	}
	j2 := sampleJob("b")
	j2.EndedAt = time.Time{}
	if d := j2.Duration(); d != 0 {
		t.Fatalf("running job must have 0 duration, got %v", d)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
