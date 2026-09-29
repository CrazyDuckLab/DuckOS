// G09 topology evidence torture regression.
package topology

import (
	"strings"
	"testing"
	"time"
)

func TestEvidenceSource_Valid(t *testing.T) {
	for _, s := range []EvidenceSource{
		SourceOverview, SourceDoctor, SourceDeviceCatalog,
		SourceNetlink, SourceWireGuard, SourceXray,
		SourceMultiWAN, SourceManual,
	} {
		if !s.Valid() {
			t.Errorf("%q must be valid", s)
		}
	}
	if EvidenceSource("garbage").Valid() {
		t.Errorf("garbage must be invalid")
	}
}

func TestEvidence_Valid(t *testing.T) {
	e := Evidence{Source: SourceNetlink, ObservedAt: time.Now().UTC(), Confidence: 0.9}
	if !e.Valid() {
		t.Fatalf("valid evidence: %+v", e)
	}
	// Unknown source
	if (Evidence{Source: EvidenceSource("x"), ObservedAt: time.Now(), Confidence: 0.5}).Valid() {
		t.Fatalf("unknown source must be invalid")
	}
	// Zero observed at
	if (Evidence{Source: SourceNetlink, Confidence: 0.5}).Valid() {
		t.Fatalf("zero observedAt must be invalid")
	}
	// Out-of-range confidence
	if (Evidence{Source: SourceNetlink, ObservedAt: time.Now(), Confidence: 1.5}).Valid() {
		t.Fatalf("confidence > 1 must be invalid")
	}
	if (Evidence{Source: SourceNetlink, ObservedAt: time.Now(), Confidence: -0.1}).Valid() {
		t.Fatalf("confidence < 0 must be invalid")
	}
}

func TestStore_SetAndGetNode(t *testing.T) {
	s := NewEvidenceStore()
	e := Evidence{Source: SourceNetlink, ObservedAt: time.Now().UTC(), Confidence: 0.8}
	s.SetNode("if-eth0", e)
	got, ok := s.NodeEvidence("if-eth0")
	if !ok || got.Source != SourceNetlink {
		t.Fatalf("drift: %+v", got)
	}
}

func TestStore_SetClampsConfidence(t *testing.T) {
	s := NewEvidenceStore()
	s.SetNode("a", Evidence{Source: SourceNetlink, ObservedAt: time.Now().UTC(), Confidence: 2.0})
	got, _ := s.NodeEvidence("a")
	if got.Confidence != 1.0 {
		t.Fatalf("confidence must be clamped to 1, got %v", got.Confidence)
	}
	s.SetNode("b", Evidence{Source: SourceNetlink, ObservedAt: time.Now().UTC(), Confidence: -1.0})
	got, _ = s.NodeEvidence("b")
	if got.Confidence != 0.0 {
		t.Fatalf("confidence must be clamped to 0, got %v", got.Confidence)
	}
}

func TestStore_SetAndGetEdge(t *testing.T) {
	s := NewEvidenceStore()
	e := Evidence{Source: SourceWireGuard, ObservedAt: time.Now().UTC(), Confidence: 0.7}
	s.SetEdge("a", "b", "tunnel", e)
	got, ok := s.EdgeEvidence("a", "b", "tunnel")
	if !ok || got.Source != SourceWireGuard {
		t.Fatalf("drift: %+v", got)
	}
	// Different kind must miss.
	if _, ok := s.EdgeEvidence("a", "b", "other"); ok {
		t.Fatalf("different kind must miss")
	}
}

func TestStore_RejectsEmptyKeys(t *testing.T) {
	s := NewEvidenceStore()
	s.SetNode("", Evidence{Source: SourceNetlink, ObservedAt: time.Now().UTC(), Confidence: 0.5})
	if _, ok := s.NodeEvidence(""); ok {
		t.Fatalf("empty id must be ignored")
	}
	s.SetEdge("", "b", "tunnel", Evidence{Source: SourceNetlink, ObservedAt: time.Now().UTC(), Confidence: 0.5})
	s.SetEdge("a", "", "tunnel", Evidence{Source: SourceNetlink, ObservedAt: time.Now().UTC(), Confidence: 0.5})
	s.SetEdge("a", "b", "", Evidence{Source: SourceNetlink, ObservedAt: time.Now().UTC(), Confidence: 0.5})
	if len(s.AllEdgeEvidence()) != 0 {
		t.Fatalf("empty edge keys must be ignored")
	}
}

func TestStore_StaleNodes(t *testing.T) {
	s := NewEvidenceStore()
	s.SetStale(50 * time.Millisecond)
	now := time.Now().UTC()
	// fresh: observed at "now" (auto-stamped), stale: observed 1 second ago
	s.SetNode("fresh", Evidence{Source: SourceNetlink, ObservedAt: now, Confidence: 1})
	s.SetNode("stale", Evidence{Source: SourceNetlink, ObservedAt: now.Add(-time.Second), Confidence: 1})
	// Wait past threshold so "fresh" becomes stale too.
	time.Sleep(60 * time.Millisecond)
	// Re-stamp fresh so it is not stale.
	s.SetNode("fresh", Evidence{Source: SourceNetlink, ObservedAt: time.Now().UTC(), Confidence: 1})
	stale := s.StaleNodes()
	if len(stale) != 1 || stale[0] != "stale" {
		t.Fatalf("StaleNodes drift: %v", stale)
	}
}

func TestStore_Counters(t *testing.T) {
	s := NewEvidenceStore()
	now := time.Now().UTC()
	s.SetNode("a", Evidence{Source: SourceNetlink, ObservedAt: now, Confidence: 1})
	s.SetNode("b", Evidence{Source: SourceDoctor, ObservedAt: now, Confidence: 1})
	s.SetEdge("a", "b", "link", Evidence{Source: SourceNetlink, ObservedAt: now, Confidence: 1})
	c := s.Counters()
	if c.Nodes != 2 || c.Edges != 1 || c.Sources != 2 {
		t.Fatalf("counter drift: %+v", c)
	}
}

func TestStore_AllNodeEvidenceIsSorted(t *testing.T) {
	s := NewEvidenceStore()
	now := time.Now().UTC()
	s.SetNode("zeta", Evidence{Source: SourceNetlink, ObservedAt: now, Confidence: 1})
	s.SetNode("alpha", Evidence{Source: SourceNetlink, ObservedAt: now, Confidence: 1})
	all := s.AllNodeEvidence()
	if len(all) != 2 || all[0].ID != "alpha" {
		t.Fatalf("sort drift: %+v", all)
	}
}

func TestStore_AllEdgeEvidenceIsSorted(t *testing.T) {
	s := NewEvidenceStore()
	now := time.Now().UTC()
	s.SetEdge("z", "a", "k1", Evidence{Source: SourceNetlink, ObservedAt: now, Confidence: 1})
	s.SetEdge("a", "b", "k2", Evidence{Source: SourceNetlink, ObservedAt: now, Confidence: 1})
	s.SetEdge("a", "b", "k1", Evidence{Source: SourceNetlink, ObservedAt: now, Confidence: 1})
	all := s.AllEdgeEvidence()
	if len(all) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(all))
	}
	if all[0].From != "a" || all[0].To != "b" || all[0].Kind != "k1" {
		t.Fatalf("first must be (a,b,k1), got %+v", all[0])
	}
}

func TestStore_AutoStampsObservedAt(t *testing.T) {
	s := NewEvidenceStore()
	s.SetNode("a", Evidence{Source: SourceNetlink, Confidence: 0.5})
	got, ok := s.NodeEvidence("a")
	if !ok || got.ObservedAt.IsZero() {
		t.Fatalf("ObservedAt must be auto-stamped")
	}
}

func TestStore_SetStaleResetsToDefault(t *testing.T) {
	s := NewEvidenceStore()
	s.SetStale(0)
	if s.StaleAfter() != StaleThreshold {
		t.Fatalf("SetStale(0) must reset to default")
	}
	s.SetStale(5 * time.Second)
	if s.StaleAfter() != 5*time.Second {
		t.Fatalf("SetStale(5s) must stick")
	}
}

// silence unused import warnings for strings.
var _ = strings.TrimSpace
