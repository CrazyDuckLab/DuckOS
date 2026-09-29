// Package doctor owns the structured diagnostic snapshot the V1
// contract requires. The Doctor is the single source of truth for
// "what does this DuckOS look like right now, in a way that is safe
// to share with support and the community". The package is
// self-contained: it never executes commands and never opens Linux
// configuration files.
//
// Design contract:
//
//   - Every block is bounded. There are no unbounded slices, no
//     growing caches, no recent-event tail beyond the configured
//     size. The hard limits are part of the API.
//
//   - Every block is redacted. MAC addresses are exposed at byte
//     granularity only when the subsystem is private (LAN) — the
//     Doctor does not echo public WireGuard keys, vault key ids
//     are exposed but never key material, and the Logger's
//     canonical redaction is the only redaction path.
//
//   - The Doctor is a snapshot, not a stream. A single call returns
//     a deterministic, JSON-serialisable value; long-running
//     observation lives in the Logger and the Speed Lab.
//
//   - The Doctor is provider-agnostic. The platform-specific
//     collectors are split into doctor_linux.go and doctor_other.go
//     so non-Linux builds are honest about their limitations.
package doctor

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const SchemaVersion = "1"

// HardCaps are the policy limits every Doctor call must respect.
// They are deliberately restrictive: a Doctor dump must never
// exceed 256 KiB so the API can stay under its response cap and
// a CLI can print it without paging.
const (
	MaxIssues         = 64
	MaxServices       = 32
	MaxInterfaces     = 32
	MaxRoutes         = 64
	MaxDNSRecords     = 32
	MaxStorage        = 16
	MaxWireGuardPeers = 64
	MaxLogLines       = 64
	MaxFingerprints   = 32
	MaxBytes          = 256 << 10
)

// Snapshot is the canonical diagnostic payload. The schema is
// stable across V1 point releases; the only legitimate change is
// the addition of optional fields, never the removal of existing
// ones.
type Snapshot struct {
	SchemaVersion string      `json:"schemaVersion"`
	GeneratedAt   time.Time   `json:"generatedAt"`
	UptimeSeconds float64     `json:"uptimeSeconds"`
	Environment   string      `json:"environment"`
	BootID        string      `json:"bootId,omitempty"`
	Schema        Schema      `json:"schema"`
	System        System      `json:"system"`
	Resources     Resources   `json:"resources"`
	Network       Network     `json:"network"`
	DNS           DNS         `json:"dns"`
	Firewall      Firewall    `json:"firewall"`
	Vault         Vault       `json:"vault"`
	Identity      Identity    `json:"identity"`
	Devices       Devices     `json:"devices"`
	WireGuard     WireGuard   `json:"wireguard"`
	Xray          Xray        `json:"xray"`
	Logging       Logging     `json:"logging"`
	Subsystem     []Subsystem `json:"subsystems"`
	Issues        []Issue     `json:"issues"`
}

// Schema captures the package versions of every subsystem the
// Doctor observed. A mismatch in this block is the canonical
// "configuration drift" indicator for support and CI.
type Schema struct {
	DuckOS  string `json:"duckos"`
	Schema  string `json:"schema"`
	Network string `json:"network"`
	Vault   string `json:"vault,omitempty"`
	Engine  string `json:"engine,omitempty"`
}

// System captures the small handful of fields that uniquely
// identify the host without leaking secrets.
type System struct {
	Hostname     string `json:"hostname,omitempty"`
	Kernel       string `json:"kernel,omitempty"`
	OS           string `json:"os,omitempty"`
	Architecture string `json:"architecture,omitempty"`
}

// Resources is the system-resource slice of the snapshot. It is
// intentionally narrow: a Doctor is not a metrics scraper.
type Resources struct {
	CPULogicalCores int       `json:"cpuLogicalCores"`
	LoadAverage     []float64 `json:"loadAverage,omitempty"`
	Memory          Memory    `json:"memory"`
	Storage         []Storage `json:"storage"`
	FDUsed          uint64    `json:"fdUsed,omitempty"`
	FDLimit         uint64    `json:"fdLimit,omitempty"`
}

type Memory struct {
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	UsedBytes      uint64 `json:"usedBytes"`
}

type Storage struct {
	Mountpoint     string `json:"mountpoint"`
	Filesystem     string `json:"filesystem"`
	Device         string `json:"device,omitempty"`
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	UsedBytes      uint64 `json:"usedBytes"`
}

// Network captures the live interface and route surface. The
// Doctor only exposes metadata, never packets.
type Network struct {
	Interfaces  []Interface `json:"interfaces"`
	Routes      []Route     `json:"routes"`
	GatewayLoss bool        `json:"gatewayLoss"`
}

type Interface struct {
	Name       string     `json:"name"`
	Index      int        `json:"index"`
	LinkUp     bool       `json:"linkUp"`
	MTU        int        `json:"mtu"`
	MAC        string     `json:"mac,omitempty"`
	Kind       string     `json:"kind,omitempty"`
	Addresses  []string   `json:"addresses,omitempty"`
	Throughput Throughput `json:"throughput,omitempty"`
	Driver     string     `json:"driver,omitempty"`
	OperState  string     `json:"operState,omitempty"`
}

type Throughput struct {
	RxBytes uint64 `json:"rxBytes"`
	TxBytes uint64 `json:"txBytes"`
}

type Route struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway,omitempty"`
	Device      string `json:"device,omitempty"`
	Table       string `json:"table,omitempty"`
	Source      string `json:"source,omitempty"`
	Metric      int    `json:"metric,omitempty"`
}

// DNS captures the live nameserver and search-domain surface. The
// Doctor only exposes metadata.
type DNS struct {
	Nameservers  []string `json:"nameservers"`
	SearchDomain []string `json:"searchDomains"`
	Listener     string   `json:"listener,omitempty"`
	Status       string   `json:"status"`
}

// Firewall captures the nftables state in aggregate. The Doctor
// never lists individual rules.
type Firewall struct {
	Backend string `json:"backend"`
	Status  string `json:"status"`
	Tables  int    `json:"tables"`
	Chains  int    `json:"chains"`
	Rules   int    `json:"rules"`
	Sets    int    `json:"sets,omitempty"`
}

// Vault captures vault health. Key material is never included;
// only the key id (a stable fingerprint).
type Vault struct {
	Available    bool   `json:"available"`
	State        string `json:"state"`
	KeyID        string `json:"keyId,omitempty"`
	Records      int    `json:"records"`
	Tombstones   int    `json:"tombstones"`
	MasterPath   string `json:"masterPath,omitempty"`
	MasterExists bool   `json:"masterExists"`
}

// Identity captures identity health without exposing users,
// sessions or audit content.
type Identity struct {
	Enrolled     bool   `json:"enrolled"`
	UserCount    int    `json:"userCount"`
	SessionCount int    `json:"sessionCount"`
	AuditLines   int    `json:"auditLines"`
	LastLogin    string `json:"lastLogin,omitempty"`
}

// Devices captures the device catalog in aggregate. The Doctor
// never lists individual devices.
type Devices struct {
	Total       int            `json:"total"`
	PerLink     map[string]int `json:"perLink,omitempty"`
	PerRole     map[string]int `json:"perRole,omitempty"`
	Quarantined int            `json:"quarantined"`
}

// WireGuard captures the live WireGuard surface in aggregate.
// Keys and allowed IPs are never included.
type WireGuard struct {
	Servers  int    `json:"servers"`
	Peers    int    `json:"peers"`
	Endpoint string `json:"endpoint,omitempty"`
	Status   string `json:"status"`
}

// Xray captures the live Xray surface in aggregate. UUIDs and
// passwords are never included.
type Xray struct {
	Servers    int    `json:"servers"`
	Users      int    `json:"users"`
	Profiles   int    `json:"profiles"`
	Subscribes int    `json:"subscriptions"`
	Status     string `json:"status"`
}

// Logging captures the logger health. Tail lines are limited to
// MaxLogLines and are already redacted by the Logger.
type Logging struct {
	Subsystems  int      `json:"subsystems"`
	RingSize    int      `json:"ringSize"`
	RingUsed    int      `json:"ringUsed"`
	Subscribers int      `json:"subscribers"`
	Tail        []string `json:"tail,omitempty"`
}

// Subsystem is a per-subsystem health record. The Doctor is the
// canonical place to look for "is everything OK" without scraping
// logs.
type Subsystem struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Since  string `json:"since,omitempty"`
}

// Issue is a structured warning, the same shape used by the
// overview surface. New issues never shadow existing ones; the
// Doctor just orders by severity.
type Issue struct {
	Component string `json:"component"`
	Code      string `json:"code"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
}

// Source is a function that contributes a section of the
// snapshot. The Doctor never depends on a source being present;
// missing sources are reported as Subsystem entries with
// status="unavailable".
type Source interface {
	Name() string
	Contribute(*Builder)
}

// Builder accumulates contributions from each Source. Sources
// never read each other's blocks; the Builder enforces the hard
// caps so a single bad source cannot poison the whole snapshot.
type Builder struct {
	mu          sync.Mutex
	schema      Schema
	system      System
	resources   Resources
	network     Network
	dns         DNS
	firewall    Firewall
	vault       Vault
	identity    Identity
	devices     Devices
	wireguard   WireGuard
	xray        Xray
	logging     Logging
	subsystems  []Subsystem
	issues      []Issue
	environment string
	bootID      string
	uptime      float64
	generated   time.Time
	duckos      string
}

func NewBuilder(environment, duckos, bootID string, generated time.Time, uptime float64) *Builder {
	return &Builder{
		environment: environment,
		bootID:      bootID,
		generated:   generated,
		uptime:      uptime,
		schema: Schema{
			DuckOS: duckos,
			Schema: SchemaVersion,
		},
		resources: Resources{Memory: Memory{}},
		devices:   Devices{PerLink: map[string]int{}, PerRole: map[string]int{}},
	}
}

// GeneratedAt is the snapshot timestamp every Source may stamp
// into the result. The Builder is the single owner.
func (b *Builder) GeneratedAt() time.Time { return b.generated }

// Uptime is the snapshot uptime the System/Resources blocks
// share. The Builder is the single owner.
func (b *Builder) Uptime() float64 { return b.uptime }

// Environment is the production/test/dev tag the snapshot carries.
func (b *Builder) Environment() string { return b.environment }

// SetSchema records a subsystem-specific schema version.
func (b *Builder) SetSchema(network, vault, engine string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.schema.Network = network
	b.schema.Vault = vault
	b.schema.Engine = engine
}

// SetSystem records the host identification block.
func (b *Builder) SetSystem(s System) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.system = normaliseSystem(s)
}

// SetResources records the resource block. The Builder enforces
// the storage cap.
func (b *Builder) SetResources(r Resources) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resources = normaliseResources(r)
}

// SetNetwork records the network block. The Builder enforces
// interface and route caps.
func (b *Builder) SetNetwork(n Network) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.network = normaliseNetwork(n)
}

// SetDNS records the DNS block. The Builder enforces the nameserver
// cap and redaction policy.
func (b *Builder) SetDNS(d DNS) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dns = normaliseDNS(d)
}

// SetFirewall records the firewall block.
func (b *Builder) SetFirewall(f Firewall) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.firewall = normaliseFirewall(f)
}

// SetVault records the vault block. The key id is retained, the
// key material is not.
func (b *Builder) SetVault(v Vault) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.vault = normaliseVault(v)
}

// SetIdentity records the identity block.
func (b *Builder) SetIdentity(i Identity) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.identity = normaliseIdentity(i)
}

// SetDevices records the device aggregate.
func (b *Builder) SetDevices(d Devices) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.devices = normaliseDevices(d)
}

// SetWireGuard records the wireguard aggregate.
func (b *Builder) SetWireGuard(w WireGuard) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.wireguard = normaliseWireGuard(w)
}

// SetXray records the xray aggregate.
func (b *Builder) SetXray(x Xray) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.xray = normaliseXray(x)
}

// SetLogging records the logger block. The Tail slice is
// truncated to MaxLogLines.
func (b *Builder) SetLogging(l Logging) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logging = normaliseLogging(l)
}

// AddSubsystem records a per-subsystem health line.
func (b *Builder) AddSubsystem(s Subsystem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s.Name == "" {
		return
	}
	if !validSubsystemStatus(s.Status) {
		s.Status = "unknown"
	}
	b.subsystems = append(b.subsystems, s)
	if len(b.subsystems) > 64 {
		b.subsystems = b.subsystems[len(b.subsystems)-64:]
	}
}

// AddIssue records an issue. The Builder enforces the cap and
// de-duplicates by (component, code).
func (b *Builder) AddIssue(iss Issue) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if iss.Component == "" || iss.Code == "" {
		return
	}
	if !validSeverity(iss.Severity) {
		iss.Severity = "info"
	}
	for _, existing := range b.issues {
		if existing.Component == iss.Component && existing.Code == iss.Code {
			return
		}
	}
	b.issues = append(b.issues, iss)
	if len(b.issues) > MaxIssues {
		b.issues = b.issues[len(b.issues)-MaxIssues:]
	}
}

// Snapshot finalises the Builder into a stable, redacted,
// bounded payload. The order of fields is deterministic.
func (b *Builder) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := make([]Subsystem, len(b.subsystems))
	copy(subs, b.subsystems)
	sort.Slice(subs, func(i, j int) bool { return subs[i].Name < subs[j].Name })
	issues := make([]Issue, len(b.issues))
	copy(issues, b.issues)
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return severityOrder(issues[i].Severity) < severityOrder(issues[j].Severity)
		}
		if issues[i].Component != issues[j].Component {
			return issues[i].Component < issues[j].Component
		}
		return issues[i].Code < issues[j].Code
	})
	return Snapshot{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   b.generated,
		UptimeSeconds: b.uptime,
		Environment:   b.environment,
		BootID:        b.bootID,
		Schema:        b.schema,
		System:        b.system,
		Resources:     b.resources,
		Network:       b.network,
		DNS:           b.dns,
		Firewall:      b.firewall,
		Vault:         b.vault,
		Identity:      b.identity,
		Devices:       b.devices,
		WireGuard:     b.wireguard,
		Xray:          b.xray,
		Logging:       b.logging,
		Subsystem:     subs,
		Issues:        issues,
	}
}

// Valid checks the snapshot is internally consistent.
func (s Snapshot) Valid() error {
	if s.SchemaVersion != SchemaVersion {
		return errors.New("doctor schema version mismatch")
	}
	if s.GeneratedAt.IsZero() {
		return errors.New("doctor generatedAt is required")
	}
	if len(s.Network.Interfaces) > MaxInterfaces {
		return errors.New("doctor interfaces exceed limit")
	}
	if len(s.Network.Routes) > MaxRoutes {
		return errors.New("doctor routes exceed limit")
	}
	if len(s.Resources.Storage) > MaxStorage {
		return errors.New("doctor storage entries exceed limit")
	}
	if len(s.Logging.Tail) > MaxLogLines {
		return errors.New("doctor log tail exceeds limit")
	}
	if len(s.Subsystem) > 64 {
		return errors.New("doctor subsystem entries exceed limit")
	}
	if len(s.Issues) > MaxIssues {
		return errors.New("doctor issues exceed limit")
	}
	return nil
}

func severityOrder(sev string) int {
	switch sev {
	case "critical":
		return 0
	case "error":
		return 1
	case "warning":
		return 2
	}
	return 3
}

func validSeverity(s string) bool {
	switch s {
	case "info", "warning", "error", "critical":
		return true
	}
	return false
}

func validSubsystemStatus(s string) bool {
	switch s {
	case "ok", "degraded", "missing", "unavailable", "starting", "recovery_required":
		return true
	}
	return false
}

func normaliseSystem(s System) System {
	s.Hostname = truncate(s.Hostname, 64)
	s.Kernel = truncate(s.Kernel, 96)
	s.OS = truncate(s.OS, 96)
	s.Architecture = truncate(s.Architecture, 32)
	return s
}

func normaliseResources(r Resources) Resources {
	if r.CPULogicalCores < 0 {
		r.CPULogicalCores = 0
	}
	if len(r.LoadAverage) > 3 {
		r.LoadAverage = r.LoadAverage[:3]
	}
	if r.Memory.TotalBytes < r.Memory.AvailableBytes {
		r.Memory.AvailableBytes = r.Memory.TotalBytes
	}
	if r.Memory.TotalBytes < r.Memory.UsedBytes {
		r.Memory.UsedBytes = r.Memory.TotalBytes
	}
	if len(r.Storage) > MaxStorage {
		r.Storage = r.Storage[:MaxStorage]
	}
	for i, s := range r.Storage {
		r.Storage[i].Mountpoint = truncate(s.Mountpoint, 64)
		r.Storage[i].Filesystem = truncate(s.Filesystem, 32)
		r.Storage[i].Device = truncate(s.Device, 128)
	}
	return r
}

func normaliseNetwork(n Network) Network {
	if len(n.Interfaces) > MaxInterfaces {
		n.Interfaces = n.Interfaces[:MaxInterfaces]
	}
	if len(n.Routes) > MaxRoutes {
		n.Routes = n.Routes[:MaxRoutes]
	}
	for i, in := range n.Interfaces {
		n.Interfaces[i].Name = truncate(in.Name, 32)
		n.Interfaces[i].MAC = normaliseMAC(in.MAC)
		n.Interfaces[i].Driver = truncate(in.Driver, 32)
		n.Interfaces[i].Kind = truncate(in.Kind, 16)
		n.Interfaces[i].OperState = truncate(in.OperState, 16)
		if len(n.Interfaces[i].Addresses) > 16 {
			n.Interfaces[i].Addresses = n.Interfaces[i].Addresses[:16]
		}
		for j, a := range n.Interfaces[i].Addresses {
			n.Interfaces[i].Addresses[j] = truncate(a, 64)
		}
	}
	for i, rt := range n.Routes {
		n.Routes[i].Destination = truncate(rt.Destination, 64)
		n.Routes[i].Gateway = truncate(rt.Gateway, 64)
		n.Routes[i].Device = truncate(rt.Device, 32)
		n.Routes[i].Table = truncate(rt.Table, 32)
		n.Routes[i].Source = truncate(rt.Source, 64)
	}
	return n
}

func normaliseDNS(d DNS) DNS {
	if len(d.Nameservers) > MaxDNSRecords {
		d.Nameservers = d.Nameservers[:MaxDNSRecords]
	}
	if len(d.SearchDomain) > MaxDNSRecords {
		d.SearchDomain = d.SearchDomain[:MaxDNSRecords]
	}
	for i, ns := range d.Nameservers {
		d.Nameservers[i] = truncate(ns, 64)
	}
	for i, sd := range d.SearchDomain {
		d.SearchDomain[i] = truncate(sd, 128)
	}
	d.Listener = truncate(d.Listener, 128)
	if d.Status == "" {
		d.Status = "unavailable"
	}
	return d
}

func normaliseFirewall(f Firewall) Firewall {
	if f.Backend == "" {
		f.Backend = "nftables"
	}
	if f.Status == "" {
		f.Status = "unavailable"
	}
	if f.Tables < 0 {
		f.Tables = 0
	}
	if f.Chains < 0 {
		f.Chains = 0
	}
	if f.Rules < 0 {
		f.Rules = 0
	}
	if f.Sets < 0 {
		f.Sets = 0
	}
	return f
}

func normaliseVault(v Vault) Vault {
	v.KeyID = truncate(v.KeyID, 16)
	v.MasterPath = truncate(v.MasterPath, 256)
	if v.Records < 0 {
		v.Records = 0
	}
	if v.Tombstones < 0 {
		v.Tombstones = 0
	}
	if v.State == "" {
		v.State = "unknown"
	}
	return v
}

func normaliseIdentity(i Identity) Identity {
	if i.UserCount < 0 {
		i.UserCount = 0
	}
	if i.SessionCount < 0 {
		i.SessionCount = 0
	}
	if i.AuditLines < 0 {
		i.AuditLines = 0
	}
	return i
}

func normaliseDevices(d Devices) Devices {
	if d.Total < 0 {
		d.Total = 0
	}
	if d.Quarantined < 0 {
		d.Quarantined = 0
	}
	if d.PerLink == nil {
		d.PerLink = map[string]int{}
	}
	if d.PerRole == nil {
		d.PerRole = map[string]int{}
	}
	for k, v := range d.PerLink {
		if v < 0 {
			d.PerLink[k] = 0
		}
	}
	for k, v := range d.PerRole {
		if v < 0 {
			d.PerRole[k] = 0
		}
	}
	return d
}

func normaliseWireGuard(w WireGuard) WireGuard {
	if w.Servers < 0 {
		w.Servers = 0
	}
	if w.Peers < 0 {
		w.Peers = 0
	}
	if w.Status == "" {
		w.Status = "unavailable"
	}
	w.Endpoint = truncate(w.Endpoint, 64)
	return w
}

func normaliseXray(x Xray) Xray {
	if x.Servers < 0 {
		x.Servers = 0
	}
	if x.Users < 0 {
		x.Users = 0
	}
	if x.Profiles < 0 {
		x.Profiles = 0
	}
	if x.Subscribes < 0 {
		x.Subscribes = 0
	}
	if x.Status == "" {
		x.Status = "unavailable"
	}
	return x
}

func normaliseLogging(l Logging) Logging {
	if l.Subsystems < 0 {
		l.Subsystems = 0
	}
	if l.RingSize < 0 {
		l.RingSize = 0
	}
	if l.RingUsed < 0 {
		l.RingUsed = 0
	}
	if l.Subscribers < 0 {
		l.Subscribers = 0
	}
	if len(l.Tail) > MaxLogLines {
		l.Tail = l.Tail[len(l.Tail)-MaxLogLines:]
	}
	for i, t := range l.Tail {
		l.Tail[i] = truncate(t, 1024)
	}
	return l
}

var macPattern = regexp.MustCompile(`^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}$`)

func normaliseMAC(s string) string {
	s = strings.TrimSpace(s)
	if !macPattern.MatchString(s) {
		return ""
	}
	return strings.ToLower(s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
