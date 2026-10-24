// Package xnode / final4_test.go: regressions for the final
// independent audit (2026-10-24) — F-01 fail-closed identity reload,
// F-03 NOT_COMMITTED tombstone preservation, and concurrent
// authentication behavior during rotations and storage faults.
package xnode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

// F-01: a read/decode failure of the authoritative identity document
// must fail the authentication boundary closed — the stale in-memory
// credential is never accepted after another process completed the
// rotation. Recovery is automatic once the document is readable.
func TestFinalIdentityReloadErrorsFailClosed(t *testing.T) {
	for _, fault := range []string{"missing", "invalid-json", "directory", "permission-denied"} {
		t.Run(fault, func(t *testing.T) {
			dir := t.TempDir()
			live, err := OpenNodeIdentity(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err = live.Adopt("synthetic-old"); err != nil {
				t.Fatal(err)
			}
			s := NewNodeServer(nil, func([]byte) error { return nil },
				func() NodeInfo { return NodeInfo{Name: "audit"} })
			s.Identity = live
			tls := httptest.NewTLSServer(s)
			defer tls.Close()
			client := NewNodeHTTPAdapter()
			client.HTTPClient = tls.Client()
			node := Node{ID: "audit", Endpoint: tls.URL}
			writer, err := OpenNodeIdentity(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err = writer.BeginTransition("synthetic-new"); err != nil {
				t.Fatal(err)
			}
			if err = writer.Commit("synthetic-new"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "node-identity.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(path); os.WriteFile(path, raw, 0o600) })
			switch fault {
			case "missing":
				err = os.Remove(path)
			case "invalid-json":
				err = os.WriteFile(path, []byte("{"), 0o600)
			case "directory":
				if err = os.Remove(path); err == nil {
					err = os.Mkdir(path, 0o700)
				}
			case "permission-denied":
				err = os.Chmod(path, 0o000)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.Discover(WithCredential(context.Background(), "synthetic-old"), node); err == nil {
				t.Fatalf("revoked OLD credential accepted after durable NEW commit and identity read fault=%s", fault)
			}
			// Recovery: once the authoritative document is readable
			// again, authentication resumes against it (new accepted,
			// old still rejected).
			os.RemoveAll(path)
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err = client.Discover(WithCredential(context.Background(), "synthetic-new"), node); err != nil {
				t.Fatalf("no automatic recovery after the storage fault cleared: %v", err)
			}
			if _, err = client.Discover(WithCredential(context.Background(), "synthetic-old"), node); err == nil {
				t.Fatal("old credential accepted after recovery")
			}
		})
	}
}

// F-01: concurrent authentication while rotations commit from another
// process instance, plus replay protection on the control channel.
func TestFinalConcurrentRotationAuthenticationAndReplay(t *testing.T) {
	dir := t.TempDir()
	live, err := OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = live.Adopt("synthetic-start"); err != nil {
		t.Fatal(err)
	}
	writer, err := OpenNodeIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	server := NewNodeServer(nil, func([]byte) error { return nil },
		func() NodeInfo { return NodeInfo{Name: "concurrent"} })
	server.Identity = live
	call := func(cred, nonce string, at time.Time) int {
		body, _ := json.Marshal(controlRequest{Op: "health", Nonce: nonce, Time: at})
		r := httptest.NewRequest("POST", TransportPath, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+cred)
		r.RemoteAddr = "192.0.2.20:10000"
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w.Code
	}
	old := "synthetic-start"
	for i := 0; i < 12; i++ {
		next := fmt.Sprintf("synthetic-next-%d", i)
		if err = writer.BeginTransition(next); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for j := 0; j < 4; j++ {
			wg.Add(1)
			go func(j int, old, next string) {
				defer wg.Done()
				_ = call(old, fmt.Sprintf("during-old-%d-%d", i, j), time.Now())
				_ = call(next, fmt.Sprintf("during-new-%d-%d", i, j), time.Now())
			}(j, old, next)
		}
		if err = writer.Commit(next); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
		if code := call(old, fmt.Sprintf("after-old-%d", i), time.Now()); code != 401 {
			t.Fatalf("old accepted after completed concurrent rotation: %d", code)
		}
		if code := call(next, fmt.Sprintf("after-new-%d", i), time.Now()); code != 200 {
			t.Fatalf("new rejected: %d", code)
		}
		old = next
	}
	// Reset only the test limiter before the replay checks.
	server.mu.Lock()
	server.hits = map[string]int{}
	server.mu.Unlock()
	now := time.Now()
	if code := call(old, "fixed-replay", now); code != 200 {
		t.Fatal(code)
	}
	if code := call(old, "fixed-replay", now); code != http.StatusUnauthorized {
		t.Fatal("same nonce replay accepted")
	}
	if code := call(old, "stale-replay", now.Add(-10*time.Minute)); code != http.StatusUnauthorized {
		t.Fatal("stale request accepted")
	}
}

// F-03: a definitive NOT_COMMITTED failure of a SECOND rotation must
// restore the previous successful confirmation tombstone together
// with the previous credential — in memory and in the physical
// durable store.
func TestFinalNotCommittedPreservesPreviousCompletion(t *testing.T) {
	dir := t.TempDir()
	disk, err := persist.Open[DurableState](filepath.Join(dir, "registry"), "state.json")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(nil)
	m.SetPersist(func(st DurableState) (CommitResult, error) {
		out, err := disk.Update(func(p *DurableState) { *p = st })
		return CommitResult{Uncertain: out == persist.CommitUncertain}, err
	})
	id, _, first := b4rEnrollAndStage(t, m)
	if _, err := m.ConfirmStagedRotation(id, first); err != nil {
		t.Fatal(err)
	}
	second, _, err := m.StageRotation(id)
	if err != nil {
		t.Fatal(err)
	}
	// Definitive pre-rename failure of the second confirmation.
	m.SetPersist(func(DurableState) (CommitResult, error) {
		return CommitResult{}, errors.New("synthetic ENOSPC before rename")
	})
	if _, err = m.ConfirmStagedRotation(id, second); err == nil {
		t.Fatal("failed persistence acknowledged")
	}
	if !m.Authenticate(id, first) || m.Authenticate(id, second) {
		t.Fatal("credential rollback unsafe")
	}
	// The previous tombstone survives the failed follow-up rotation
	// in the live manager.
	if _, err = m.ConfirmStagedRotation(id, first); err != nil {
		t.Fatalf("NOT_COMMITTED erased the previous successful confirmation tombstone: %v", err)
	}
	// Physical reopen: the authoritative durable snapshot (which never
	// contained the failed second rotation) reproduces the same
	// idempotent duplicate confirmation and the same credential state.
	reopened, err := persist.Open[DurableState](filepath.Join(dir, "registry"), "state.json")
	if err != nil {
		t.Fatal(err)
	}
	fresh := NewManager(nil)
	fresh.LoadState(reopened.Get())
	if !fresh.Authenticate(id, first) || fresh.Authenticate(id, second) {
		t.Fatal("durable registry mismatch after physical reopen")
	}
	if st, err := fresh.ConfirmStagedRotation(id, first); err != nil || st != RotationConfirmed {
		t.Fatalf("duplicate confirmation after physical reopen: %s %v", st, err)
	}
}
