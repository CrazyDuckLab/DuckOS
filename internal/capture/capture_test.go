package capture

import (
	"strings"
	"testing"
	"time"
)

func newRequest(t *testing.T) Request {
	t.Helper()
	return Request{
		ID:        "cap-1",
		Link:      "lan0",
		Filter:    "tcp port 80",
		Duration:  2 * time.Second,
		MaxBytes:  1 << 20,
		Operator:  "admin",
		CreatedAt: time.Now().UTC(),
	}
}

func TestRequestValid(t *testing.T) {
	r := newRequest(t)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestRejectsBadID(t *testing.T) {
	r := newRequest(t)
	r.ID = "id with space"
	if err := r.Validate(); err == nil {
		t.Fatal("invalid id accepted")
	}
}

func TestRequestRejectsBadLink(t *testing.T) {
	r := newRequest(t)
	r.Link = "x; rm -rf /"
	if err := r.Validate(); err == nil {
		t.Fatal("unsafe link accepted")
	}
}

func TestRequestRejectsOversizedLink(t *testing.T) {
	r := newRequest(t)
	r.Link = strings.Repeat("a", MaxInterfaceLen+1)
	if err := r.Validate(); err == nil {
		t.Fatal("oversized link accepted")
	}
}

func TestRequestRejectsShellMetacharacterInFilter(t *testing.T) {
	r := newRequest(t)
	r.Filter = "tcp port 80'; rm -rf /"
	if err := r.Validate(); err == nil {
		t.Fatal("unsafe filter accepted")
	}
}

func TestRequestRejectsOversizedFilter(t *testing.T) {
	r := newRequest(t)
	r.Filter = strings.Repeat("a", MaxFilterBytes+1)
	if err := r.Validate(); err == nil {
		t.Fatal("oversized filter accepted")
	}
}

func TestRequestRejectsZeroDuration(t *testing.T) {
	r := newRequest(t)
	r.Duration = 0
	if err := r.Validate(); err == nil {
		t.Fatal("zero duration accepted")
	}
}

func TestRequestRejectsLongDuration(t *testing.T) {
	r := newRequest(t)
	r.Duration = MaxDuration + time.Second
	if err := r.Validate(); err == nil {
		t.Fatal("oversized duration accepted")
	}
}

func TestRequestRejectsShortDuration(t *testing.T) {
	r := newRequest(t)
	r.Duration = 500 * time.Millisecond
	if err := r.Validate(); err == nil {
		t.Fatal("under-minimum duration accepted")
	}
}

func TestRequestRejectsBadMaxBytes(t *testing.T) {
	r := newRequest(t)
	r.MaxBytes = MaxBytes + 1
	if err := r.Validate(); err == nil {
		t.Fatal("oversized max bytes accepted")
	}
	r.MaxBytes = 0
	if err := r.Validate(); err == nil {
		t.Fatal("zero max bytes accepted")
	}
}

func TestRequestRejectsBadOperator(t *testing.T) {
	r := newRequest(t)
	r.Operator = "root; rm -rf /"
	if err := r.Validate(); err == nil {
		t.Fatal("unsafe operator accepted")
	}
}

func TestRequestRejectsZeroCreatedAt(t *testing.T) {
	r := newRequest(t)
	r.CreatedAt = time.Time{}
	if err := r.Validate(); err == nil {
		t.Fatal("zero createdAt accepted")
	}
}

func TestRequestAcceptsCommonFilters(t *testing.T) {
	cases := []string{
		"tcp port 80",
		"host 192.0.2.1 and tcp port 443",
		"ether proto 0x888e",
		"!arp",
		"vlan 100 and (tcp or udp)",
		"greater 1500",
	}
	for _, f := range cases {
		r := newRequest(t)
		r.Filter = f
		if err := r.Validate(); err != nil {
			t.Fatalf("valid filter %q rejected: %v", f, err)
		}
	}
}

func TestManagerStarts(t *testing.T) {
	m := NewManager(&fakeHost{})
	r := newRequest(t)
	ref, err := m.Start(r)
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID != r.ID {
		t.Fatalf("id mismatch: %q vs %q", ref.ID, r.ID)
	}
}

func TestManagerRejectsInvalid(t *testing.T) {
	m := NewManager(&fakeHost{})
	r := newRequest(t)
	r.Link = "bad link"
	if _, err := m.Start(r); err == nil {
		t.Fatal("invalid request accepted")
	}
}

func TestManagerEnforcesCap(t *testing.T) {
	m := NewManager(&fakeHost{})
	for i := 0; i < MaxConcurrent; i++ {
		r := newRequest(t)
		r.ID = "cap-" + itoa(i)
		if _, err := m.Start(r); err != nil {
			t.Fatal(err)
		}
	}
	r := newRequest(t)
	r.ID = "cap-overflow"
	if _, err := m.Start(r); err != ErrBusy {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
}

func TestManagerList(t *testing.T) {
	m := NewManager(&fakeHost{})
	for i := 0; i < MaxConcurrent; i++ {
		r := newRequest(t)
		r.ID = "cap-" + itoa(i)
		if _, err := m.Start(r); err != nil {
			t.Fatal(err)
		}
	}
	list := m.List()
	if len(list) != MaxConcurrent {
		t.Fatalf("expected %d captures, got %d", MaxConcurrent, len(list))
	}
}

func TestManagerCancel(t *testing.T) {
	m := NewManager(&fakeHost{})
	r := newRequest(t)
	if _, err := m.Start(r); err != nil {
		t.Fatal(err)
	}
	if err := m.Cancel(r.ID); err != nil {
		t.Fatal(err)
	}
	if list := m.List(); len(list) != 0 {
		t.Fatalf("expected empty after cancel, got %d", len(list))
	}
}

func TestUnsupportedHost(t *testing.T) {
	m := NewManager(nil)
	r := newRequest(t)
	if _, err := m.Start(r); err != ErrUnsupported {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestManagerStatusUnknown(t *testing.T) {
	m := NewManager(&fakeHost{})
	if _, _, err := m.Status("missing"); err != nil {
		t.Fatal(err)
	}
}

type fakeHost struct{}

func (f *fakeHost) Start(r Request) (ActiveRef, error) {
	return ActiveRef{ID: r.ID, State: StateRunning, StartedAt: time.Now().UTC()}, nil
}
func (f *fakeHost) Status(id string) (State, Result, error) {
	return StateDone, Result{ID: id, Status: string(StateDone)}, nil
}
func (f *fakeHost) Cancel(string) error { return nil }
func (f *fakeHost) List() []ActiveRef   { return nil }

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
