//go:build linux

package restore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type transaction struct {
	Request Request          `json:"request"`
	Outcome string           `json:"outcome"`
	Before  map[string]image `json:"before,omitempty"`
}
type journal struct {
	Current   *transaction  `json:"current,omitempty"`
	Completed []transaction `json:"completed,omitempty"`
}
type Service struct {
	mu                  sync.Mutex
	source, live, state *root
	lock                *os.File
	journal             journal
	poisoned            bool
	// Fault injection is package-private and never exposed through IPC/config.
	afterWrite    func(int) error
	beforePersist func() error
}

func Open(source, live, state string) (*Service, error) {
	roots := []string{source, live, state}
	for i, a := range roots {
		if !filepath.IsAbs(a) || filepath.Clean(a) != a || a == "/" {
			return nil, errors.New("unsafe restore root configuration")
		}
		for j, b := range roots {
			if i != j && (a == b || strings.HasPrefix(a, b+"/")) {
				return nil, errors.New("overlapping restore roots")
			}
		}
	}
	s := &Service{}
	var err error
	defer func() {
		if err != nil {
			s.Close()
		}
	}()
	if s.source, err = openRoot(source); err != nil {
		return nil, err
	}
	if s.live, err = openRoot(live); err != nil {
		return nil, err
	}
	if s.state, err = openRoot(state); err != nil {
		return nil, err
	}
	s.state.limit = 96 << 20
	fd, e := unix.Openat(int(s.state.f.Fd()), "writer.lock", unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if e != nil {
		err = e
		return nil, err
	}
	s.lock = os.NewFile(uintptr(fd), "writer.lock")
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, err
	}
	f, e := s.state.open("journal.json", unix.O_RDONLY|unix.O_NONBLOCK)
	if e == nil {
		var b []byte
		b, err = io.ReadAll(io.LimitReader(f, 96<<20))
		f.Close()
		if err != nil {
			return nil, err
		}
		var envelope struct {
			Payload json.RawMessage `json:"payload"`
			Digest  string          `json:"digest"`
		}
		if err = json.Unmarshal(b, &envelope); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(envelope.Payload)
		if hex.EncodeToString(sum[:]) != envelope.Digest {
			err = errors.New("restore journal corrupt")
			return nil, err
		}
		if err = json.Unmarshal(envelope.Payload, &s.journal); err != nil {
			return nil, err
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		err = e
		return nil, err
	}
	if t := s.journal.Current; t != nil {
		if err = t.Request.Validate(); err != nil {
			return nil, err
		}
		if len(t.Before) != len(t.Request.Keys) {
			err = errors.New("incomplete durable undo")
			return nil, err
		}
		if err = s.undo(t); err != nil {
			return nil, err
		}
		if err = s.finish(t, "rolled-back"); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Service) Close() error {
	for _, r := range []*root{s.source, s.live, s.state} {
		if r != nil {
			_ = r.f.Close()
		}
	}
	if s.lock != nil {
		return s.lock.Close()
	}
	return nil
}

func (s *Service) persist() error {
	if s.beforePersist != nil {
		if err := s.beforePersist(); err != nil {
			s.poisoned = true
			return err
		}
	}
	b, err := json.Marshal(s.journal)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	envelope, err := json.Marshal(struct {
		Payload json.RawMessage `json:"payload"`
		Digest  string          `json:"digest"`
	}{b, hex.EncodeToString(sum[:])})
	if err != nil {
		return err
	}
	// Journal may contain up to 64 bounded before-images. It is private and
	// never exposed by status. Terminal receipts contain no before-images.
	err = s.state.write("journal.json", image{true, envelope, 0600, os.Getegid()})
	if err != nil {
		s.poisoned = true
	}
	return err
}
func (s *Service) finish(t *transaction, outcome string) error {
	receipt := transaction{Request: t.Request, Outcome: outcome}
	s.journal.Completed = append(s.journal.Completed, receipt)
	// Refuse new operations at the receipt bound rather than forget replay IDs.
	s.journal.Current = nil
	return s.persist()
}
func (s *Service) undo(t *transaction) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, k := range t.Request.Keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		v, ok := t.Before[k]
		if !ok {
			return errors.New("missing undo image")
		}
		if err := s.live.write(k, v); err != nil {
			return err
		}
		got, err := s.live.read(k)
		if err != nil || got.Exists != v.Exists || !bytes.Equal(got.Data, v.Data) || (v.Exists && (got.Mode != v.Mode || got.GID != v.GID)) {
			return errors.New("undo verification failed")
		}
	}
	return nil
}

func (s *Service) Execute(ctx context.Context, r Request) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := Result{ID: r.ID, Outcome: "rejected"}
	if err := r.Validate(); err != nil {
		return result, err
	}
	if s.poisoned || s.journal.Current != nil {
		return Result{r.ID, "recovery-required"}, errors.New("restore recovery required")
	}
	for _, t := range s.journal.Completed {
		if t.Request.ID == r.ID {
			if t.Request.Digest() != r.Digest() {
				return result, errors.New("restore identity conflict")
			}
			return Result{r.ID, t.Outcome}, nil
		}
	}
	if len(s.journal.Completed) >= 1024 {
		return result, errors.New("restore receipt capacity reached")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	before := map[string]image{}
	after := map[string]image{}
	total := 0
	for _, k := range r.Keys {
		parent, err := s.live.parent(k)
		if err != nil {
			return result, err
		}
		parent.Close()
		v, err := s.live.read(k)
		if err != nil {
			return result, err
		}
		before[k] = v
		v, err = s.source.read(r.Backup + "/" + k)
		if err != nil {
			return result, err
		}
		if !v.Exists {
			return result, errors.New("backup target missing")
		}
		after[k] = v
		total += len(before[k].Data) + len(v.Data)
		if total > 32<<20 {
			return result, errors.New("restore selection exceeds bounded transaction size")
		}
	}
	t := &transaction{Request: r, Outcome: "applying", Before: before}
	s.journal.Current = t
	if err := s.persist(); err != nil {
		return Result{r.ID, "recovery-required"}, err
	}
	applyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var applyErr error
	for i, k := range r.Keys {
		if applyErr = applyCtx.Err(); applyErr != nil {
			break
		}
		if applyErr = s.live.write(k, after[k]); applyErr != nil {
			break
		}
		got, err := s.live.read(k)
		if err != nil || !got.Exists || !bytes.Equal(got.Data, after[k].Data) {
			applyErr = errors.New("live restore verification failed")
			break
		}
		if s.afterWrite != nil {
			if applyErr = s.afterWrite(i); applyErr != nil {
				break
			}
		}
	}
	if applyErr == nil {
		applyErr = applyCtx.Err()
	}
	if applyErr != nil {
		t.Outcome = "rolling-back"
		// The earlier applying record already contains complete undo; attempt to
		// persist recovery intent without making undo depend on a writable journal.
		phaseErr := s.persist()
		if err := s.undo(t); err != nil {
			s.poisoned = true
			return Result{r.ID, "recovery-required"}, errors.Join(applyErr, err)
		}
		if err := s.finish(t, "rolled-back"); err != nil {
			return Result{r.ID, "recovery-required"}, err
		}
		if phaseErr != nil {
			return Result{r.ID, "recovery-required"}, phaseErr
		}
		return Result{r.ID, "rolled-back"}, nil
	}
	if err := s.finish(t, "applied"); err != nil {
		// An uncertain rename/fsync leaves either applying or a complete terminal
		// record. Startup resolves that durable record; do not undo here because
		// the terminal record may already be visible. Never report success now.
		return Result{r.ID, "recovery-required"}, err
	}
	return Result{r.ID, "applied"}, nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" || (r.URL.Path != "/v1/restore" && r.URL.Path != "/v1/reconcile") || r.URL.RawQuery != "" || r.URL.EscapedPath() != r.URL.Path {
		http.Error(w, `{"error":"invalid_restore_operation"}`, 400)
		return
	}
	var req Request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF {
		http.Error(w, `{"error":"invalid_restore_request"}`, 400)
		return
	}
	var result Result
	var err error
	if r.URL.Path == "/v1/reconcile" {
		result, err = s.Reconcile(req)
	} else {
		result, err = s.Execute(r.Context(), req)
	}
	if err != nil {
		w.WriteHeader(422)
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

// Reconcile never starts a new restore. Unknown admitted requests receive a
// durable rollback tombstone before Engine releases its reservation. A delayed
// pre-crash request can therefore not mutate after reservation release.
func (s *Service) Reconcile(r Request) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := Result{r.ID, "recovery-required"}
	if err := r.Validate(); err != nil {
		return result, err
	}
	if s.poisoned || s.journal.Current != nil {
		return result, errors.New("startup recovery required")
	}
	for _, t := range s.journal.Completed {
		if t.Request.ID == r.ID {
			if t.Request.Digest() != r.Digest() {
				return result, errors.New("identity conflict")
			}
			return Result{r.ID, t.Outcome}, nil
		}
	}
	if len(s.journal.Completed) >= 1024 {
		return result, errors.New("receipt capacity reached")
	}
	if err := s.finish(&transaction{Request: r}, "rolled-back"); err != nil {
		return result, err
	}
	return Result{r.ID, "rolled-back"}, nil
}
