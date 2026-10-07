//go:build linux

package upnp

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// recordingBackend records every Add/Remove so tests can
// assert the firewall side of the runtime.
type recordingBackend struct {
	mu       sync.Mutex
	added    []Mapping
	removed  []string
	listenOK bool
	trusted  []string
}

func (r *recordingBackend) Add(m Mapping) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.added = append(r.added, m)
	return nil
}

func (r *recordingBackend) TrustIface(iface string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trusted = append(r.trusted, iface)
}

func (r *recordingBackend) UntrustIface(iface string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.trusted[:0]
	for _, t := range r.trusted {
		if t != iface {
			out = append(out, t)
		}
	}
	r.trusted = out
}

func (r *recordingBackend) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removed = append(r.removed, id)
	return nil
}

func (r *recordingBackend) AllowListen(iface string, addr net.Addr) bool {
	return r.listenOK
}

func (r *recordingBackend) addCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.added)
}

func (r *recordingBackend) removeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.removed)
}

// 1. default state is OFF: no networks, no mappings.
func TestRuntime_DefaultOff(t *testing.T) {
	rt := NewRuntime(nil, nil)
	defer rt.Close()
	if got := rt.Networks(); len(got) != 0 {
		t.Fatalf("default networks=%d", len(got))
	}
	if got := rt.Mappings(); len(got) != 0 {
		t.Fatalf("default mappings=%d", len(got))
	}
	if rt.Stats().Mappings != 0 || rt.Stats().Networks != 0 {
		t.Fatalf("stats not zero")
	}
}

// 2. EnableNetwork refuses when backend disallows.
func TestRuntime_EnableNetwork_RefusedByBackend(t *testing.T) {
	be := &recordingBackend{listenOK: false}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	err := rt.EnableNetwork(Network{NetworkRef: "wan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth0")
	if err == nil {
		t.Fatalf("expected refusal")
	}
	if be.addCount() != 0 {
		t.Fatalf("backend.Add called despite refusal")
	}
	if got := rt.Networks(); len(got) != 0 {
		t.Fatalf("networks=%d", len(got))
	}
}

// 3. EnableNetwork accepts when backend allows.
func TestRuntime_EnableNetwork_Accepted(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP, ProtoNATPMP, ProtoPCP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatalf("EnableNetwork: %v", err)
	}
	if got := rt.Networks(); len(got) != 1 {
		t.Fatalf("networks=%d", len(got))
	}
	if !rt.ObservedAt().After(time.Time{}) {
		t.Fatalf("observedAt not set")
	}
}

// 4. AddMapping refuses when network is not enabled.
func TestRuntime_AddMapping_NetworkNotEnabled(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, ""); err == nil {
		t.Fatalf("expected refusal")
	}
	if be.addCount() != 0 {
		t.Fatalf("backend.Add called despite refusal")
	}
}

// 5. AddMapping adds a mapping and emits an event.
func TestRuntime_AddMapping_Happy(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	sub := rt.Subscribe(4)
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, "test"); err != nil {
		t.Fatal(err)
	}
	if be.addCount() != 1 {
		t.Fatalf("backend add=%d", be.addCount())
	}
	select {
	case e := <-sub:
		if e.Kind != "added" || e.Mapping.ID != "m-1" || e.Source != "test" {
			t.Fatalf("event: %+v", e)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("no add event")
	}
}

// 6. AddMapping rejects disabled protocol.
func TestRuntime_AddMapping_ProtocolRejected(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoPCP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, ""); err == nil {
		t.Fatal("expected protocol rejection")
	}
}

// 7. AddMapping sets ExpiresAt when not provided.
func TestRuntime_AddMapping_SetsExpiresAt(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	before := time.Now().UTC()
	if err := rt.AddMapping(m, ""); err != nil {
		t.Fatal(err)
	}
	got := rt.Mappings()
	if len(got) != 1 {
		t.Fatalf("mappings=%d", len(got))
	}
	if got[0].ExpiresAt.Before(before) {
		t.Fatalf("ExpiresAt not set in the future: %s", got[0].ExpiresAt)
	}
}

// 8. RevokeMapping removes and emits.
func TestRuntime_RevokeMapping(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	sub := rt.Subscribe(4)
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, ""); err != nil {
		t.Fatal(err)
	}
	<-sub // drain the add event
	rt.RevokeMapping("m-1")
	if got := rt.Mappings(); len(got) != 0 {
		t.Fatalf("mappings after revoke=%d", len(got))
	}
	if be.removeCount() != 1 {
		t.Fatalf("backend remove=%d", be.removeCount())
	}
	select {
	case e := <-sub:
		if e.Kind != "removed" {
			t.Fatalf("kind=%s", e.Kind)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("no remove event")
	}
}

// 9. DisableNetwork revokes all mappings on the network.
func TestRuntime_DisableNetwork_RevokesMappings(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		m := Mapping{ID: "m-" + string(rune('a'+i)), NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000 + i, InternalPort: 5000 + i, InternalHost: "192.168.1.10"}
		if err := rt.AddMapping(m, ""); err != nil {
			t.Fatal(err)
		}
	}
	rt.DisableNetwork("lan")
	if got := rt.Mappings(); len(got) != 0 {
		t.Fatalf("mappings after disable=%d", len(got))
	}
	if got := rt.Networks(); len(got) != 0 {
		t.Fatalf("networks after disable=%d", len(got))
	}
	if be.removeCount() != 3 {
		t.Fatalf("backend remove=%d", be.removeCount())
	}
}

// 10. expiry tick removes mappings after TTL elapses.
func TestRuntime_ExpiryTickRemoves(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 1}, "eth1"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)
	sub := rt.Subscribe(4)
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, ""); err != nil {
		t.Fatal(err)
	}
	<-sub // drain add event
	// Wait for TTL to elapse + 1 s tick to fire.
	time.Sleep(2500 * time.Millisecond)
	if got := rt.Mappings(); len(got) != 0 {
		t.Fatalf("mappings=%d after expiry", len(got))
	}
	if be.removeCount() != 1 {
		t.Fatalf("backend remove=%d after expiry", be.removeCount())
	}
}

// 11. Subscribe channels are isolated; one slow consumer
// does not block the runtime or starve the other.
func TestRuntime_SubscribeIsolated(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	full := rt.Subscribe(0) // zero buffer = every send blocks
	other := rt.Subscribe(4)
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-other:
		if e.Kind != "added" {
			t.Fatalf("kind=%s", e.Kind)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("non-blocking channel starved")
	}
	// drain the slow consumer so the runtime can shut down cleanly
	select {
	case <-full:
	default:
	}
}

// 12. backend Add error rolls back the Manager state.
func TestRuntime_BackendAddFailure_RollsBack(t *testing.T) {
	be := &failingBackend{}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, ""); err == nil {
		t.Fatal("expected backend failure")
	}
	if got := rt.Mappings(); len(got) != 0 {
		t.Fatalf("manager not rolled back: %d", len(got))
	}
}

type failingBackend struct{}

func (failingBackend) Add(Mapping) error              { return errBackendNew }
func (failingBackend) Remove(string) error            { return nil }
func (failingBackend) AllowListen(string, net.Addr) bool { return true }
func (failingBackend) TrustIface(string)               {}
func (failingBackend) UntrustIface(string)             {}

var errBackendNew = errBackend("backend refused")

type errBackend string

func (e errBackend) Error() string { return string(e) }

// 13. Stats returns the live counts.
func TestRuntime_Stats(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, ""); err != nil {
		t.Fatal(err)
	}
	st := rt.Stats()
	if st.Networks != 1 || st.Mappings != 1 {
		t.Fatalf("stats=%+v", st)
	}
}

// 14. invalid network reference rejected.
func TestRuntime_InvalidNetworkRef(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	err := rt.EnableNetwork(Network{NetworkRef: "$$$bad", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1")
	if err == nil {
		t.Fatal("expected invalid network ref rejection")
	}
}

// 15. invalid protocol rejected at EnableNetwork.
func TestRuntime_InvalidProtocol(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{"garbage"}, TTLSec: 60}, "eth1")
	if err == nil {
		t.Fatal("expected invalid protocol rejection")
	}
}

// 16. FormatMapping produces a stable summary.
func TestFormatMapping(t *testing.T) {
	m := Mapping{Protocol: ProtoUPnP, NetworkRef: "lan", ExternalPort: 5000, InternalHost: "192.168.1.10", InternalPort: 5000, ExpiresAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	got := FormatMapping(m)
	if !contains(got, "upnp/lan") {
		t.Fatalf("missing protocol/network: %s", got)
	}
	if !contains(got, "ext=5000") {
		t.Fatalf("missing external port: %s", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// 17. close is idempotent.
func TestRuntime_CloseIdempotent(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	rt.Close()
	rt.Close()
}

// 18. Close stops the expiry loop.
func TestRuntime_CloseStopsExpiry(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 1}, "eth1"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)
	rt.Close()
	time.Sleep(1500 * time.Millisecond)
	if !rt.Stats().Stopped {
		t.Fatal("runtime not stopped after Close")
	}
}

// 19. noopBackend is the safe default; with no iface
// trust registered, EnableNetwork refuses any non-empty
// interface so the operator cannot accidentally start
// listening on the WAN. With iface="" the AllowListen
// check is bypassed and the manager store commits the
// network — the nftables backend is what would normally
// install the listener; the runtime never installs anything
// on its own.
func TestRuntime_NoopBackendSafeDefault(t *testing.T) {
	rt := NewRuntime(nil, nil)
	defer rt.Close()

	// iface != "" + noopBackend (no TrustIface registered)
	// → runtime refuses. This is the SAFE default.
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err == nil {
		t.Fatal("noop backend without TrustIface must refuse non-empty iface")
	}

	// iface == "" + noopBackend → check is bypassed.
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, ""); err != nil {
		t.Fatalf("noop backend must accept empty iface (no listener): %v", err)
	}
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, ""); err != nil {
		t.Fatal(err)
	}
}

// 20. Subscribe + Close: the runtime must not deadlock.
func TestRuntime_SubscribeCloseDoesNotDeadlock(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	sub := rt.Subscribe(4)
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	_ = rt.AddMapping(m, "")
	done := make(chan struct{})
	go func() {
		rt.Close()
		close(done)
	}()
	select {
	case <-done:
		_ = sub
	case <-time.After(2 * time.Second):
		t.Fatal("Close deadlocked")
	}
}

// 21. noopBackend AllowListen returns false; production
// wiring must explicitly allow.
func TestNoopBackend_RefusesListen(t *testing.T) {
	var b noopBackend
	if b.AllowListen("eth0", nil) {
		t.Fatal("noopBackend must refuse every listen by default")
	}
}

// 22. Subscribe with positive buffer is non-blocking.
func TestRuntime_SubscribePositiveBuffer(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	sub := rt.Subscribe(8)
	for i := 0; i < 5; i++ {
		m := Mapping{ID: "m-x", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000 + i, InternalPort: 5000 + i, InternalHost: "192.168.1.10"}
		if err := rt.AddMapping(m, ""); err != nil {
			t.Fatal(err)
		}
	}
	count := 0
loop:
	for {
		select {
		case <-sub:
			count++
		case <-time.After(100 * time.Millisecond):
			break loop
		}
	}
	if count != 5 {
		t.Fatalf("got %d events", count)
	}
}

// 23. multiple subscribers receive the same event.
func TestRuntime_MultipleSubscribers(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	s1 := rt.Subscribe(4)
	s2 := rt.Subscribe(4)
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, ""); err != nil {
		t.Fatal(err)
	}
	for _, ch := range []<-chan MappingEvent{s1, s2} {
		select {
		case e := <-ch:
			if e.Mapping.ID != "m-1" {
				t.Fatalf("event=%+v", e)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("subscriber starved")
		}
	}
}

// 24. enable then disable then enable again re-registers.
func TestRuntime_EnableDisableEnable(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	rt.DisableNetwork("lan")
	if got := rt.Networks(); len(got) != 0 {
		t.Fatalf("networks after disable=%d", len(got))
	}
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	if got := rt.Networks(); len(got) != 1 {
		t.Fatalf("networks after re-enable=%d", len(got))
	}
}

// 25. observedAt advances on every state mutation.
func TestRuntime_ObservedAtAdvances(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	prev := rt.ObservedAt()
	time.Sleep(2 * time.Millisecond)
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	if !rt.ObservedAt().After(prev) {
		t.Fatalf("observedAt not advanced")
	}
}

// 26. WanRefused sentinel is exported.
func TestWanRefused_Exported(t *testing.T) {
	if WanRefused == nil {
		t.Fatal("WanRefused must be exported")
	}
	if WanRefused.Error() == "" {
		t.Fatal("WanRefused must have a non-empty message")
	}
}

// 27. runtime Stats with nil Manager.
//
// Manager.Mappings() is documented to return a non-nil
// empty slice (the sorted-by-id invariant requires a
// stable length-0 value), so this assertion checks length
// instead of nil-comparison.
func TestRuntime_NilManagerSafe(t *testing.T) {
	rt := NewRuntime(nil, nil)
	defer rt.Close()
	if got := rt.Mappings(); len(got) != 0 {
		t.Fatalf("got=%v want empty", got)
	}
}

// 28. concurrent AddMapping are goroutine-safe.
func TestRuntime_ConcurrentAddMapping(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 60}, "eth1"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := Mapping{ID: "m-" + string(rune('a'+i)), NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000 + i, InternalPort: 5000 + i, InternalHost: "192.168.1.10"}
			_ = rt.AddMapping(m, "")
		}(i)
	}
	wg.Wait()
}

// 29. expiry event is distinct from removed event.
func TestRuntime_ExpiryEventDistinct(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	if err := rt.EnableNetwork(Network{NetworkRef: "lan", Protocols: []Protocol{ProtoUPnP}, TTLSec: 1}, "eth1"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)
	sub := rt.Subscribe(8)
	m := Mapping{ID: "m-1", NetworkRef: "lan", Protocol: ProtoUPnP, ClientIP: "192.168.1.10", ExternalPort: 5000, InternalPort: 5000, InternalHost: "192.168.1.10"}
	if err := rt.AddMapping(m, ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2500 * time.Millisecond)
	saw := map[string]int{}
loop:
	for {
		select {
		case e := <-sub:
			saw[e.Kind]++
		case <-time.After(500 * time.Millisecond):
			break loop
		}
	}
	if saw["added"] != 1 || saw["expired"] != 1 {
		t.Fatalf("events=%v", saw)
	}
}

// 30. Subscribe returns a usable channel even before any
// mutation.
func TestRuntime_SubscribeFreshChannel(t *testing.T) {
	be := &recordingBackend{listenOK: true}
	rt := NewRuntime(nil, be)
	defer rt.Close()
	ch := rt.Subscribe(1)
	select {
	case <-ch:
		t.Fatal("fresh channel must not have events")
	case <-time.After(50 * time.Millisecond):
	}
}