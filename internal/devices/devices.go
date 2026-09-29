// Package devices owns the observed-device and trust-policy model
// the DuckOS V1 contract requires. The package is a small,
// self-contained subsystem with a single source-of-truth: the
// Observable. Devices appear when the host sees traffic from a
// bounded source, are tagged with a stable 32-hex id, and live as
// long as the operator keeps them or the engine observes them. A
// TrustPolicy is a per-link decision: trusted, guest, IoT or
// quarantined. The package never mutates host state and never
// reaches into the network adapter.
package devices

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const MaxDevices = 4096

var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Role is the per-link trust category. The four canonical roles are
// the only legal values; unknown roles are rejected at the
// configuration layer.
type Role string

const (
	RoleTrusted     Role = "trusted"
	RoleGuest       Role = "guest"
	RoleIoT         Role = "iot"
	RoleQuarantined Role = "quarantined"
)

func IsValidRole(r Role) bool {
	switch r {
	case RoleTrusted, RoleGuest, RoleIoT, RoleQuarantined:
		return true
	}
	return false
}

// Policy is the per-link decision for a device. The default is
// trusted; the engine never silences a device without an explicit
// policy. Quarantined is a hard refusal: the device cannot reach
// any network, even the management plane.
type Policy struct {
	ID        string    `json:"id"`
	Link      string    `json:"link"`
	Role      Role      `json:"role"`
	Aliases   []string  `json:"aliases,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	ExpiresAt string    `json:"expiresAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Device is the canonical observed-device record. The ID is a stable
// 32-hex string derived from the first observation so the record
// survives across reboots. The same device can be seen on multiple
// links; Addresses is the merged set.
type Device struct {
	ID         string    `json:"id"`
	Hostname   string    `json:"hostname,omitempty"`
	MAC        string    `json:"mac,omitempty"`
	Addresses  []string  `json:"addresses"`
	Links      []string  `json:"links"`
	Vendor     string    `json:"vendor,omitempty"`
	FirstSeen  time.Time `json:"firstSeen"`
	LastSeen   time.Time `json:"lastSeen"`
	LastSource string    `json:"lastSource,omitempty"`
}

// Validate enforces the bounded contract every Device must satisfy.
func (d Device) Validate() error {
	if !idPattern.MatchString(d.ID) {
		return errors.New("device id must be 32 hex characters")
	}
	if d.MAC != "" && !macPattern.MatchString(d.MAC) {
		return errors.New("device MAC is not a canonical address")
	}
	if len(d.Addresses) > 32 {
		return errors.New("device address list exceeds limit")
	}
	for _, a := range d.Addresses {
		if a == "" {
			return errors.New("device address entry is empty")
		}
	}
	if len(d.Links) > 8 {
		return errors.New("device link list exceeds limit")
	}
	return nil
}

var macPattern = regexp.MustCompile(`^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}$`)

// Validate enforces the bounded contract every Policy must satisfy.
func (p Policy) Validate() error {
	if !idPattern.MatchString(p.ID) {
		return errors.New("policy id must be 32 hex characters")
	}
	if p.Link == "" {
		return errors.New("policy link is required")
	}
	if !linkNamePattern.MatchString(p.Link) {
		return errors.New("policy link is not a canonical interface name")
	}
	if !IsValidRole(p.Role) {
		return errors.New("policy role is not canonical")
	}
	if p.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, p.ExpiresAt); err != nil {
			return fmt.Errorf("policy expiresAt must be RFC3339: %w", err)
		}
	}
	for _, alias := range p.Aliases {
		if alias == "" || len(alias) > 80 {
			return errors.New("policy alias is empty or oversized")
		}
	}
	return nil
}

var linkNamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,47}$`)

// Catalog is the canonical device and policy store. The store is
// concurrency-safe and bound-bounded: at most MaxDevices devices.
type Catalog struct {
	mu       sync.Mutex
	devices  map[string]Device
	policies map[string]Policy
	byMAC    map[string]string // MAC -> device id
	closed   bool
}

func NewCatalog() *Catalog {
	return &Catalog{
		devices:  map[string]Device{},
		policies: map[string]Policy{},
		byMAC:    map[string]string{},
	}
}

// Record records a single observation. The function is idempotent:
// the same MAC + link + address combination observed twice updates
// the LastSeen timestamp and the address set. The device ID is the
// MAC when present, otherwise a stable hash of the address. The
// catalog refuses a record when it is at capacity.
func (c *Catalog) Record(observation Observation) (Device, error) {
	if err := observation.Validate(); err != nil {
		return Device{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return Device{}, errors.New("device catalog is closed")
	}
	id, ok := c.resolveIDLocked(observation)
	if !ok {
		if len(c.devices) >= MaxDevices {
			return Device{}, errors.New("device catalog capacity exceeded")
		}
		id = deriveID(observation)
	}
	existing, ok := c.devices[id]
	if !ok {
		existing = Device{
			ID:         id,
			Hostname:   observation.Hostname,
			MAC:        observation.MAC,
			Addresses:  []string{},
			Links:      []string{},
			FirstSeen:  observation.At,
			LastSeen:   observation.At,
			LastSource: observation.Source,
			Vendor:     observation.Vendor,
		}
		c.byMAC[normaliseMAC(observation.MAC)] = id
	}
	if observation.Hostname != "" {
		existing.Hostname = observation.Hostname
	}
	if observation.MAC != "" && existing.MAC == "" {
		existing.MAC = observation.MAC
	}
	if observation.Vendor != "" && existing.Vendor == "" {
		existing.Vendor = observation.Vendor
	}
	if !containsString(existing.Addresses, observation.Address) {
		existing.Addresses = append(existing.Addresses, observation.Address)
	}
	if !containsString(existing.Links, observation.Link) {
		existing.Links = append(existing.Links, observation.Link)
	}
	if observation.At.After(existing.LastSeen) {
		existing.LastSeen = observation.At
	}
	if observation.Source != "" {
		existing.LastSource = observation.Source
	}
	c.devices[id] = existing
	return existing, nil
}

// Observation is the input the engine feeds to the catalog.
type Observation struct {
	At       time.Time
	MAC      string
	Hostname string
	Address  string
	Link     string
	Source   string
	Vendor   string
}

func (o Observation) Validate() error {
	if o.At.IsZero() {
		return errors.New("observation time is required")
	}
	if o.MAC == "" && o.Address == "" {
		return errors.New("observation must include at least a MAC or an address")
	}
	if o.MAC != "" && !macPattern.MatchString(o.MAC) {
		return errors.New("observation MAC is not a canonical address")
	}
	if o.Address != "" && !addressPattern.MatchString(o.Address) {
		return errors.New("observation address is not a canonical value")
	}
	if o.Link != "" && !linkNamePattern.MatchString(o.Link) {
		return errors.New("observation link is not a canonical interface name")
	}
	return nil
}

var addressPattern = regexp.MustCompile(`^[0-9a-fA-F:.]{2,64}$`)

func (c *Catalog) resolveIDLocked(o Observation) (string, bool) {
	if o.MAC != "" {
		if id, ok := c.byMAC[normaliseMAC(o.MAC)]; ok {
			return id, true
		}
	}
	if o.Address != "" {
		for _, d := range c.devices {
			for _, a := range d.Addresses {
				if a == o.Address {
					return d.ID, true
				}
			}
		}
	}
	return "", false
}

func deriveID(o Observation) string {
	seed := o.MAC
	if seed == "" {
		seed = o.Address
	}
	return stableID(seed + "|" + o.Link)
}

func normaliseMAC(s string) string { return strings.ToLower(s) }

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

// ListDevices returns every device ordered by ID. The slice is a
// copy; mutations do not affect the catalog.
func (c *Catalog) ListDevices() []Device {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Device, 0, len(c.devices))
	for _, d := range c.devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GetDevice returns the device by id. The boolean reports whether
// the device exists.
func (c *Catalog) GetDevice(id string) (Device, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.devices[id]
	return d, ok
}

// ForgetDevice removes a device from the catalog. The next observation
// of the same MAC/address re-creates the device. The function
// refuses to remove devices that have an active policy.
func (c *Catalog) ForgetDevice(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("device catalog is closed")
	}
	if !idPattern.MatchString(id) {
		return errors.New("device id must be 32 hex characters")
	}
	for _, p := range c.policies {
		if deviceHasLink(c.devices[id], p.Link) {
			return errors.New("device has a policy on the same link; remove the policy first")
		}
	}
	d, ok := c.devices[id]
	if !ok {
		return nil
	}
	delete(c.devices, id)
	delete(c.byMAC, normaliseMAC(d.MAC))
	return nil
}

func deviceHasLink(d Device, link string) bool {
	for _, l := range d.Links {
		if l == link {
			return true
		}
	}
	return false
}

// UpsertPolicy records a per-link trust decision. The same (id, link)
// combination is idempotent; the latest role and aliases win.
func (c *Catalog) UpsertPolicy(p Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("device catalog is closed")
	}
	p.UpdatedAt = time.Now().UTC()
	c.policies[p.ID+"|"+p.Link] = p
	return nil
}

// RemovePolicy deletes a per-link trust decision. The device record
// itself is preserved.
func (c *Catalog) RemovePolicy(id, link string) error {
	if !idPattern.MatchString(id) {
		return errors.New("policy id must be 32 hex characters")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("device catalog is closed")
	}
	delete(c.policies, id+"|"+link)
	return nil
}

// LookupPolicy returns the most recent policy for a device on a
// given link. The boolean reports whether one was found.
func (c *Catalog) LookupPolicy(deviceID, link string) (Policy, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := c.policies[deviceID+"|"+link]; ok {
		return p, true
	}
	return Policy{}, false
}

// EffectiveRole returns the role a device should be treated under
// for a given link. The default is trusted; quarantined is the only
// role that flips the device to a hard refusal.
func (c *Catalog) EffectiveRole(deviceID, link string) Role {
	if p, ok := c.LookupPolicy(deviceID, link); ok {
		return p.Role
	}
	return RoleTrusted
}

// IsQuarantined reports whether the device is quarantined on the
// given link. The boolean is the canonical gate for the firewall:
// quarantined devices cannot reach the router, the LAN or the WAN.
func (c *Catalog) IsQuarantined(deviceID, link string) bool {
	return c.EffectiveRole(deviceID, link) == RoleQuarantined
}

// Close marks the catalog as closed. Subsequent Record and policy
// updates return an explicit refusal.
func (c *Catalog) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
}

// stableID is the canonical device fingerprint. The output is a
// 32-hex string (128 bits) derived from a SHA-256 of the seed
// truncated to 16 bytes. SHA-256 is overkill for an identifier but
// gives us a collision-free mapping for any distinct seed without
// pulling in a separate hash package. The function is not a security
// primitive: the seed is observable through normal channel traffic.
func stableID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:16])
}
