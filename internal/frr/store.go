package frr

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

type durableConfig struct {
	Version  int    `json:"version"`
	Current  Config `json:"current"`
	Previous Config `json:"previous"`
}

// OpenManager loads the committed desired configuration independently of
// volatile daemon sockets/configuration files in /run/frr.
func OpenManager(dir string, clock func() time.Time) (*Manager, error) {
	if dir == "" {
		return nil, errors.New("frr: durable state directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	m := NewManager(clock)
	m.path = filepath.Join(dir, "config.json")
	m.config = Config{Daemons: []Daemon{DaemonZebra, DaemonBGP}}
	f, err := os.Open(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("frr: invalid persisted config")
	}
	var stored durableConfig
	if json.Unmarshal(raw, &stored) != nil || stored.Version != 1 || stored.Current.Valid() != nil {
		return nil, errors.New("frr: corrupt persisted config")
	}
	if stored.Previous.Daemons != nil && stored.Previous.Valid() != nil {
		return nil, errors.New("frr: corrupt previous config")
	}
	m.config = stored.Current
	m.previous = stored.Previous
	return m, nil
}

func (m *Manager) persist(current, previous Config) error {
	if m.path == "" {
		return nil
	}
	raw, err := json.Marshal(durableConfig{Version: 1, Current: current, Previous: previous})
	if err != nil || len(raw) > 1<<20 {
		return errors.New("frr: persisted config exceeds limit")
	}
	dir := filepath.Dir(m.path)
	f, err := os.CreateTemp(dir, ".config-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), m.path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
