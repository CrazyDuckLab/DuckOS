package identity

import (
	"encoding/hex"
	"errors"
)

// RecordClientExport rechecks the live session before durable audit admission.
// Inputs are bounded enums and an opaque object ID, never labels or payloads.
func (s *Service) RecordClientExport(token, kind, id, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.session(token)
	if err != nil {
		return err
	}
	if !Allowed(session.User.Role, ExportClients) {
		return ErrForbidden
	}
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != id ||
		(kind != "wireguard" && kind != "subscription") ||
		(action != "qr" && action != "copy" && action != "download") {
		return errors.New("invalid client export audit metadata")
	}
	return s.save(s.db, session.User.ID, "client-export."+kind+"."+action, id)
}
