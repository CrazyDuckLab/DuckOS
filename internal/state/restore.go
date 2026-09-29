package state

import "encoding/json"

// RestoreAdmission is a durable Engine-owned reservation. It is not supplied by
// a public caller and blocks ordinary configuration writes until a terminal
// receipt from the confined restore service resolves it.
type RestoreAdmission struct {
	ID       string          `json:"id"`
	Expected string          `json:"expected"`
	Digest   string          `json:"digest"`
	Request  json.RawMessage `json:"request"`
}

func (s *Store) BeginRestore(id, expected, digest string, request json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrRecovery
	}
	if !ValidTransactionID(id) || !validHex(expected, 32) || !validHex(digest, 32) || !json.Valid(request) || len(request) > 65536 {
		return ErrConflict
	}
	if s.snapshot.Pending != nil {
		return ErrPending
	}
	if old := s.snapshot.Restore; old != nil {
		if old.ID == id && old.Expected == expected && old.Digest == digest {
			return nil
		}
		return ErrPending
	}
	if s.snapshot.Current.ID != expected {
		return ErrConflict
	}
	next := clone(s.snapshot)
	next.Restore = &RestoreAdmission{id, expected, digest, append(json.RawMessage(nil), request...)}
	return s.persist(next)
}
func (s *Store) FinishRestore(id, digest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrRecovery
	}
	if s.snapshot.Restore == nil || s.snapshot.Restore.ID != id || s.snapshot.Restore.Digest != digest {
		return ErrConflict
	}
	next := clone(s.snapshot)
	next.Restore = nil
	return s.persist(next)
}
