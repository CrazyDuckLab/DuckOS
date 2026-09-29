// safeapply/transaction_test.go pins the V1
// config transaction model extensions: diff,
// checkpoints, lock.
package safeapply

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestComputeDiffAddedRemovedModified(t *testing.T) {
	before := map[string]json.RawMessage{
		"wan/wan-1": json.RawMessage(`{"id":"wan-1","type":"dhcp"}`),
		"vlan/v-10": json.RawMessage(`{"id":"v-10","vlanId":10}`),
		"site/old":  json.RawMessage(`{"id":"old"}`),
	}
	after := map[string]json.RawMessage{
		"wan/wan-1": json.RawMessage(`{"id":"wan-1","type":"pppoe"}`),
		"vlan/v-10": json.RawMessage(`{"id":"v-10","vlanId":10}`),
		"site/blog": json.RawMessage(`{"id":"blog","domain":"blog.example.com"}`),
	}
	d := ComputeDiff(before, after, []string{"firewall"}, true, "moderate")
	if len(d.Objects) != 3 {
		t.Fatalf("expected 3 deltas, got %d: %+v", len(d.Objects), d.Objects)
	}
	ops := map[ChangeKind]int{}
	for _, o := range d.Objects {
		ops[o.Op]++
	}
	if ops[ChangeAdded] != 1 || ops[ChangeRemoved] != 1 || ops[ChangeModified] != 1 {
		t.Fatalf("op distribution: %+v", ops)
	}
	if d.Fingerprint == "" {
		t.Fatal("fingerprint must be set")
	}
	if d.Impact.ManagementRisk != "moderate" {
		t.Fatal("impact not propagated")
	}
}

func TestComputeDiffIdenticalHasNoObjects(t *testing.T) {
	m := map[string]json.RawMessage{"x/y": json.RawMessage(`{"a":1}`)}
	d := ComputeDiff(m, m, nil, false, "none")
	if len(d.Objects) != 0 {
		t.Fatalf("identical inputs must produce no deltas: %+v", d.Objects)
	}
}

func TestCheckpointStorePutGetRemove(t *testing.T) {
	s := NewCheckpointStore(time.Now)
	cp, err := s.Put("before isp migration", "op-1", "rev-1", json.RawMessage(`{"a":1}`), "test")
	if err != nil {
		t.Fatal(err)
	}
	if cp.ID == "" {
		t.Fatal("id is required")
	}
	if _, ok := s.Get(cp.ID); !ok {
		t.Fatal("Get must find the new checkpoint")
	}
	if _, ok := s.GetByLabel("before isp migration"); !ok {
		t.Fatal("GetByLabel must find the new checkpoint")
	}
	// Re-put with the same label is rejected.
	if _, err := s.Put("before isp migration", "op-1", "rev-2", json.RawMessage(`{"a":2}`), ""); err == nil {
		t.Fatal("duplicate label must be rejected")
	}
	if err := s.Remove(cp.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(cp.ID); ok {
		t.Fatal("Get after Remove must miss")
	}
}

func TestCheckpointStoreRefusesBadLabel(t *testing.T) {
	s := NewCheckpointStore(time.Now)
	if _, err := s.Put("../bad", "op-1", "rev-1", json.RawMessage(`{}`), ""); err == nil {
		t.Fatal("non-canonical label must be rejected")
	}
	if _, err := s.Put("ok label", "op-1", "rev-1", json.RawMessage(`not json`), ""); err == nil {
		t.Fatal("non-JSON config must be rejected")
	}
}

func TestCheckpointStoreCapacityBound(t *testing.T) {
	s := NewCheckpointStore(time.Now)
	for i := 0; i < MaxCheckpoints; i++ {
		if _, err := s.Put(labelFor(i), "op-1", "rev-1", json.RawMessage(`{"i":1}`), ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Put("over-cap", "op-1", "rev-1", json.RawMessage(`{"i":1}`), ""); err == nil {
		t.Fatal("Put past MaxCheckpoints must be refused")
	}
}

func TestLockManagerBeginAuthorizeEnd(t *testing.T) {
	m := NewLockManager(time.Now)
	s, err := m.Begin("op-1", "config change for ISP", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Authorize(s.ID); err != nil {
		t.Fatal("owner must be authorized")
	}
	if err := m.Authorize("not-the-id"); err == nil {
		t.Fatal("non-owner must be refused")
	}
	if _, err := m.Begin("op-2", "another change", 5*time.Minute); err == nil {
		t.Fatal("second Begin while active must be refused")
	}
	if err := m.End(s.ID, "op-1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Authorize(s.ID); err != nil {
		t.Fatal("after End, Authorize must succeed")
	}
}

func TestLockManagerAutoExpires(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	m := NewLockManager(func() time.Time { return now })
	s, err := m.Begin("op-1", "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Active(); !ok {
		t.Fatal("session must be active")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := m.Active(); ok {
		t.Fatal("session must be auto-expired")
	}
	if err := m.Authorize(s.ID); err != nil {
		t.Fatal("after expiry, Authorize must succeed")
	}
}

func TestLockManagerRejectsWindowOutOfRange(t *testing.T) {
	m := NewLockManager(time.Now)
	if _, err := m.Begin("op-1", "test", 5*time.Second); err == nil {
		t.Fatal("window below MinSessionWindow must be refused")
	}
	if _, err := m.Begin("op-1", "test", 2*time.Hour); err == nil {
		t.Fatal("window above MaxSessionWindow must be refused")
	}
}

func TestLockManagerConcurrent(t *testing.T) {
	m := NewLockManager(time.Now)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				_, _ = m.Begin("op-x", "concurrent test", 2*time.Minute)
			}
		}()
	}
	wg.Wait()
}

func labelFor(i int) string {
	return "label-" + jsonNum(i)
}

func jsonNum(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
