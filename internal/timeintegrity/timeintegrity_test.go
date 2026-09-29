// timeintegrity/timeintegrity_test.go pins the
// V1 wall-clock trust model.
package timeintegrity

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewTrackerStartsUntrusted(t *testing.T) {
	tr := NewTracker(time.Now)
	if tr.State() != StateUntrusted {
		t.Fatalf("State()=%q want %q", tr.State(), StateUntrusted)
	}
}

func TestStateValid(t *testing.T) {
	if !StateUntrusted.Valid() || !StateTrusted.Valid() {
		t.Fatal("StateUntrusted and StateTrusted must be valid")
	}
	if State("garbage").Valid() {
		t.Fatal("garbage state must be invalid")
	}
}

func TestObserveNTPWithinTolerancePromotes(t *testing.T) {
	tr := NewTracker(time.Now)
	rep := tr.Observe(Source{Name: "ntp-1", Stratum: 2, LastSync: time.Now().UTC(), DriftMS: 50})
	if !rep.OK {
		t.Fatalf("expected OK, got %+v", rep)
	}
	if tr.State() != StateTrusted {
		t.Fatalf("State=%q", tr.State())
	}
}

func TestObserveStratumTooHighRefusedWithoutOperator(t *testing.T) {
	tr := NewTracker(time.Now)
	rep := tr.Observe(Source{Name: "ntp-1", Stratum: 5, LastSync: time.Now().UTC(), DriftMS: 50})
	if rep.OK {
		t.Fatal("stratum 5 without operator must be refused")
	}
	if tr.State() != StateUntrusted {
		t.Fatalf("State=%q", tr.State())
	}
}

func TestObserveOperatorAnchorPromotes(t *testing.T) {
	tr := NewTracker(time.Now)
	rep := tr.AnchorManual(time.Now().UTC(), 100)
	if !rep.OK {
		t.Fatal(rep)
	}
	if tr.State() != StateTrusted {
		t.Fatalf("State=%q", tr.State())
	}
}

func TestAnchorManualRefusesExcessiveDrift(t *testing.T) {
	tr := NewTracker(time.Now)
	rep := tr.AnchorManual(time.Now().UTC(), 600_000) // 10 min
	if rep.OK {
		t.Fatal("operator drift beyond 5min must be refused")
	}
}

func TestDriftExceedingFiveSecondsRefused(t *testing.T) {
	tr := NewTracker(time.Now)
	rep := tr.Observe(Source{Name: "ntp-1", Stratum: 1, LastSync: time.Now().UTC(), DriftMS: 6000})
	if rep.OK {
		t.Fatal("drift > 5s must be refused")
	}
}

func TestResetReturnsToUntrusted(t *testing.T) {
	tr := NewTracker(time.Now)
	tr.Observe(Source{Name: "ntp-1", Stratum: 2, LastSync: time.Now().UTC(), DriftMS: 50})
	if tr.State() != StateTrusted {
		t.Fatal("setup")
	}
	tr.Reset()
	if tr.State() != StateUntrusted {
		t.Fatalf("after Reset, State=%q", tr.State())
	}
	if tr.HaveAnchor() {
		t.Fatal("after Reset, HaveAnchor must be false")
	}
}

func TestRequireTrustedReturnsErrorWhenUntrusted(t *testing.T) {
	tr := NewTracker(time.Now)
	if err := tr.RequireTrusted(); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("RequireTrusted=%v, want ErrUntrusted", err)
	}
	tr.Observe(Source{Name: "ntp-1", Stratum: 2, LastSync: time.Now().UTC(), DriftMS: 50})
	if err := tr.RequireTrusted(); err != nil {
		t.Fatalf("after Observe, RequireTrusted=%v", err)
	}
}

func TestStatusSnapshotRoundTrips(t *testing.T) {
	tr := NewTracker(time.Now)
	tr.Observe(Source{Name: "ntp-1", Stratum: 2, LastSync: time.Now().UTC(), DriftMS: 50})
	st := tr.StatusSnapshot()
	if st.State != StateTrusted {
		t.Fatalf("State=%q", st.State)
	}
	if st.DriftMS != 50 {
		t.Fatalf("DriftMS=%d", st.DriftMS)
	}
}

func TestTrackerGoroutineSafe(t *testing.T) {
	tr := NewTracker(time.Now)
	var done int32
	for i := 0; i < 16; i++ {
		go func() {
			for j := 0; j < 64; j++ {
				tr.Observe(Source{Name: "ntp", Stratum: 2, LastSync: time.Now().UTC(), DriftMS: 50})
				_ = tr.State()
				_ = tr.DriftMS()
			}
			atomic.AddInt32(&done, 1)
		}()
	}
	for atomic.LoadInt32(&done) < 16 {
		time.Sleep(2 * time.Millisecond)
	}
	if tr.State() != StateTrusted {
		t.Fatalf("after concurrent Observe, State=%q", tr.State())
	}
}
