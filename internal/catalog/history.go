package catalog

import (
	"context"
	"reflect"
	"time"
)

// Changes describes canonical intent, never connectivity or packet-path proof.
// Values are fixed event kinds, safe to correlate without copying names/secrets.
func Changes(before, after Client) ([]string, error) {
	if before.Validate() != nil || after.Validate() != nil || before.ID != after.ID || before.Kind != after.Kind {
		return nil, ErrInput
	}
	events := []string{}
	if before.Name != after.Name {
		events = append(events, "client.name.changed")
	}
	if before.Enabled != after.Enabled {
		events = append(events, "client.enablement.changed")
	}
	if before.ProfileID != after.ProfileID {
		events = append(events, "client.profile.changed")
	}
	if before.ExpiresAt != after.ExpiresAt {
		events = append(events, "client.expiry.changed")
	}
	if !reflect.DeepEqual(before.Credential, after.Credential) {
		events = append(events, "client.credential-reference.changed")
	}
	return events, nil
}

func (s *Store) History(ctx context.Context, id string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return nil, ErrUnavailable
	}
	if !identifier.MatchString(id) {
		return nil, ErrInput
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := s.conn.QueryContext(ctx, "SELECT revision,document FROM client_history WHERE client_id=? ORDER BY revision DESC LIMIT 32", id)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	result := []Entry{}
	for rows.Next() {
		var revision int64
		var raw string
		if rows.Scan(&revision, &raw) != nil {
			return nil, ErrUnavailable
		}
		client, err := decodeClient(raw)
		if err != nil || client.ID != id {
			return nil, ErrUnavailable
		}
		result = append(result, Entry{Revision: revision, Client: client})
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}
