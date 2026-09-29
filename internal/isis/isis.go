// isis/isis.go: V1 IS-IS first-class
// routing surface. The package owns the
// typed contracts for IS-IS instances,
// levels, neighbors, metrics,
// authentication, redistribution, and
// the segment-routing extension surface.
//
// The runtime is a thin mapping to the
// existing internal/frr package (isisd).
package isis

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Level is the IS-IS level.
type Level string

const (
	Level1  Level = "L1"
	Level2  Level = "L2"
	Level12 Level = "L1L2"
)

// Valid reports whether l is known.
func (l Level) Valid() bool { return l == Level1 || l == Level2 || l == Level12 }

// Instance is an IS-IS instance.
type Instance struct {
	Name   string `json:"name"`
	NET    string `json:"net"` // Network Entity Title, e.g. 49.0001.1920.6800.1001.00
	Level  Level  `json:"level"`
	AreaID string `json:"areaId,omitempty"`
	Metric int    `json:"metric,omitempty"`
	// Passive interfaces do not send IIH
	// but advertise the prefix.
	Passive []string `json:"passive,omitempty"`
	// Redistribute enables route
	// redistribution from connected /
	// static.
	Redistribute []string `json:"redistribute,omitempty"`
	// SRGB is the segment-routing global
	// block.
	SRGBStart int `json:"srgbStart,omitempty"`
	SRGBEnd   int `json:"srgbEnd,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
var netPattern = regexp.MustCompile(`^[0-9a-fA-F]{2,4}(\.[0-9a-fA-F]{4}){3,9}\.[0-9a-fA-F]{2}$`)

// Valid reports whether i is well-formed.
func (i Instance) Valid() error {
	if !namePattern.MatchString(i.Name) {
		return errors.New("isis: name is not canonical")
	}
	if !netPattern.MatchString(i.NET) {
		return errors.New("isis: NET is not canonical: " + i.NET)
	}
	if !i.Level.Valid() {
		return errors.New("isis: level is invalid")
	}
	if i.Metric < 0 || i.Metric > 16777215 {
		return errors.New("isis: metric out of range")
	}
	if i.SRGBStart != 0 && (i.SRGBStart < 0 || i.SRGBStart > 1048575) {
		return errors.New("isis: srgbStart out of range")
	}
	if i.SRGBEnd != 0 && (i.SRGBEnd < i.SRGBStart || i.SRGBEnd > 1048575) {
		return errors.New("isis: srgbEnd must be >= srgbStart")
	}
	return nil
}

// Neighbor is an IS-IS neighbor.
type Neighbor struct {
	SystemID string `json:"systemId"`
	// Level pin the neighbor adjacency to
	// one level.
	Level Level `json:"level"`
	// Metric override.
	Metric int `json:"metric,omitempty"`
}

// Valid reports whether n is well-formed.
func (n Neighbor) Valid() error {
	if len(n.SystemID) < 6 {
		return errors.New("isis: systemId is too short")
	}
	if !n.Level.Valid() {
		return errors.New("isis: neighbor level is invalid")
	}
	return nil
}

// Config is the IS-IS configuration.
type Config struct {
	Instances []Instance `json:"instances"`
}

// Valid reports whether c is well-formed.
func (c Config) Valid() error {
	if len(c.Instances) == 0 {
		return errors.New("isis: at least one instance is required")
	}
	for _, i := range c.Instances {
		if err := i.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Render produces the frr-compatible
// isisd.conf text. The output is
// deterministic.
func Render(c Config) (string, error) {
	if err := c.Valid(); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("! DuckOS-generated isisd config\n")
	insts := append([]Instance(nil), c.Instances...)
	sort.Slice(insts, func(i, k int) bool { return insts[i].Name < insts[k].Name })
	for _, inst := range insts {
		fmt.Fprintf(&sb, "router isis %s\n", inst.Name)
		fmt.Fprintf(&sb, " net %s\n", inst.NET)
		fmt.Fprintf(&sb, " is-type %s\n", levelCode(inst.Level))
		if inst.Metric > 0 {
			fmt.Fprintf(&sb, " metric %d\n", inst.Metric)
		}
		if inst.SRGBStart > 0 && inst.SRGBEnd > 0 {
			fmt.Fprintf(&sb, " segment-routing global-block %d %d\n", inst.SRGBStart, inst.SRGBEnd)
		}
		passive := append([]string(nil), inst.Passive...)
		sort.Strings(passive)
		for _, p := range passive {
			fmt.Fprintf(&sb, " passive-interface %s\n", p)
		}
		redist := append([]string(nil), inst.Redistribute...)
		sort.Strings(redist)
		for _, r := range redist {
			fmt.Fprintf(&sb, " redistribute %s\n", r)
		}
		sb.WriteString("!\n")
	}
	return sb.String(), nil
}

func levelCode(l Level) string {
	switch l {
	case Level1:
		return "level-1"
	case Level2:
		return "level-2"
	}
	return "level-1-2"
}

// Manager is the IS-IS lifecycle store.
// Manager is goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	config   Config
	previous Config
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{} }

// Set replaces the config.
func (m *Manager) Set(c Config) error {
	if err := c.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.previous = m.config
	m.config = c
	return nil
}

// Render returns the rendered config.
func (m *Manager) Render() (string, error) {
	m.mu.Lock()
	c := m.config
	m.mu.Unlock()
	return Render(c)
}

// Rollback restores the previous config.
func (m *Manager) Rollback() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.previous.Instances) == 0 {
		return errors.New("isis: no previous config")
	}
	m.config = m.previous
	m.previous = Config{}
	return nil
}
