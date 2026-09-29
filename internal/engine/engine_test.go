package engine

import (
	"context"
	"encoding/json"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"github.com/CrazyDuckLab/DuckOS/internal/testfixture"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestEngineCoalescesAndDoesNotCacheFailure(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	e := New("/synthetic-test-only.sock", "test")
	e.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		data, _ := json.Marshal(testfixture.Overview())
		status := 200
		if fail.Load() {
			status = 503
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.Overview(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("coalescing failed calls=%d", calls.Load())
	}
	data, _ := e.Overview(context.Background())
	data[0] = '!'
	again, _ := e.Overview(context.Background())
	if again[0] != '{' {
		t.Fatal("caller mutated shared cache")
	}
	fail.Store(true)
	if e.Ready(context.Background()) == nil {
		t.Fatal("ready must bypass cache")
	}
	if _, err := e.Overview(context.Background()); err == nil {
		t.Fatal("failed Agent returned stale healthy cache")
	}
}
func TestEngineRejectsOversizedAndInvalidIPC(t *testing.T) {
	for _, data := range []string{`{}`, strings.Repeat(" ", model.MaxResponseBytes+1)} {
		e := New("/test.sock", "test")
		e.client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(data))}, nil
		})}
		if _, err := e.Overview(context.Background()); err == nil {
			t.Fatal("invalid IPC cached")
		}
	}
}
func TestWaitingCallerCancellation(t *testing.T) {
	e := New("/test.sock", "test")
	e.inflight = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := e.Overview(ctx); err == nil {
		t.Fatal("waiting call did not cancel")
	}
}
