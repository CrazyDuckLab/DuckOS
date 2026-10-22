// Package certs / inventory.go: certificate inventory, expiration
// monitoring and renewal policy on top of the validation
// primitives in certs.go.
//
// The inventory stores records, not secrets: the private key lives
// in the vault and the record carries only its reference plus the
// parsed public metadata (fingerprint, SANs, validity window).
// Renewal state is an explicit state machine — pending, issued,
// failed — so a failed renewal is diagnosable instead of silent.
//
// ACME note: the V1 manager's acquisition hooks (kind
// acme-dns-01 / http-01 / tls-alpn-01) model the upstream ACME
// client boundary; the upstream issuance loop itself requires a
// network-facing challenge path and is qualified separately. The
// inventory, validation, expiration monitoring and renewal-failure
// diagnostics here are fully functional against imported and
// ACME-issued records alike.
package certs

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Record is one inventory entry.
type Record struct {
	ID string `json:"id"`
	// Kind: imported or an ACME challenge kind.
	Kind Kind `json:"kind"`
	// Domains is the SAN list the record must serve.
	Domains []string `json:"domains"`
	// NotBefore / NotAfter come from the parsed leaf, not from the
	// operator: the inventory never trusts configured validity.
	NotBefore time.Time `json:"notBefore"`
	NotAfter  time.Time `json:"notAfter"`
	// Fingerprint is the stable sha256-truncated fingerprint.
	Fingerprint string `json:"fingerprint"`
	// KeyVaultRef points at the private key in the vault. The key
	// itself never enters the record or any API read.
	KeyVaultRef string `json:"keyVaultRef,omitempty"`
	// DNSProvider names the DNS-01 provider for ACME records.
	DNSProvider string `json:"dnsProvider,omitempty"`
	// BoundServer is the xray server id using this record, if any.
	BoundServer string `json:"boundServer,omitempty"`
	// Renewal is the renewal state machine for ACME records.
	Renewal   RenewalState `json:"renewal"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

// RenewalState tracks ACME renewal.
type RenewalState struct {
	// Mode: "" (manual), "acme".
	Mode string `json:"mode,omitempty"`
	// Status: "" | pending | issued | failed.
	Status    string    `json:"status,omitempty"`
	LastTry   time.Time `json:"lastTry,omitempty"`
	LastError string    `json:"lastError,omitempty"`
	// RenewBeforeDays triggers proactive renewal when the record
	// is inside the window (default 30).
	RenewBeforeDays int `json:"renewBeforeDays,omitempty"`
}

// Health classifies a record's validity at time now.
type Health string

const (
	HealthValid    Health = "valid"
	HealthExpiring Health = "expiring"
	HealthExpired  Health = "expired"
)

// HealthAt derives the record's validity class. expiringWindow
// defaults to 30 days.
func (r Record) HealthAt(now time.Time, expiringWindow time.Duration) Health {
	if expiringWindow <= 0 {
		expiringWindow = 30 * 24 * time.Hour
	}
	switch {
	case now.After(r.NotAfter):
		return HealthExpired
	case now.Add(expiringWindow).After(r.NotAfter):
		return HealthExpiring
	default:
		return HealthValid
	}
}

// NeedsRenewal reports whether the renewal policy fires for this
// record at now.
func (r Record) NeedsRenewal(now time.Time) bool {
	if r.Renewal.Mode != "acme" || r.Renewal.Status == "pending" {
		return false
	}
	days := r.Renewal.RenewBeforeDays
	if days <= 0 {
		days = 30
	}
	return now.Add(time.Duration(days) * 24 * time.Hour).After(r.NotAfter)
}

// Inventory is a bounded in-memory record table with a parse-gated
// insert path: a record can only enter through Import (validated
// against real PEM) or RegisterACME (validated domains).
type Inventory struct {
	mu      sync.RWMutex
	records map[string]Record
	clock   func() time.Time
	idgen   func() string
}

const MaxRecords = 128

func NewInventory() *Inventory {
	counter := 0
	return &Inventory{
		records: map[string]Record{},
		clock:   time.Now,
		idgen: func() string {
			counter++
			return fmt.Sprintf("cert-%04d", counter)
		},
	}
}

// SetClock overrides the wall clock (testing).
func (inv *Inventory) SetClock(fn func() time.Time) { inv.clock = fn }

// Import validates a PEM certificate (+ key match) via the V1
// primitives and adds the parsed metadata as a record. The private
// key bytes are used for verification only and are never stored.
func (inv *Inventory) Import(id string, src Source, keyVaultRef string) (Record, error) {
	parsed, err := ParseImported(src)
	if err != nil {
		return Record{}, err
	}
	return inv.put(Record{
		ID:          orID(id, inv.idgen()),
		Kind:        KindImported,
		Domains:     parsed.Leaf.DNSNames,
		NotBefore:   parsed.Leaf.NotBefore,
		NotAfter:    parsed.Leaf.NotAfter,
		Fingerprint: Fingerprint(parsed.Leaf),
		KeyVaultRef: keyVaultRef,
	})
}

// RegisterACME registers a future ACME certificate for domains. The
// record starts in renewal status pending; issuance flips it to
// issued with the parsed leaf metadata.
func (inv *Inventory) RegisterACME(id string, kind Kind, domains []string, dnsProvider string) (Record, error) {
	if kind != KindACMEDNS01 && kind != KindACMEHTTP01 && kind != KindACMETLSALPN01 {
		return Record{}, errors.New("certs: RegisterACME requires an ACME kind")
	}
	src := Source{Kind: kind, Domains: domains, DNSProvider: dnsProvider}
	if err := src.Validate(); err != nil {
		return Record{}, err
	}
	return inv.put(Record{
		ID:          orID(id, inv.idgen()),
		Kind:        kind,
		Domains:     domains,
		DNSProvider: dnsProvider,
		Renewal:     RenewalState{Mode: "acme", Status: "pending", RenewBeforeDays: 30},
	})
}

// IssueACME completes the pending ACME record with parsed leaf
// metadata (the caller ran the challenge through the upstream ACME
// client and validated the result via ParseImported-style parsing).
func (inv *Inventory) IssueACME(id string, leaf *x509.Certificate) error {
	if leaf == nil {
		return errors.New("certs: issued leaf is nil")
	}
	inv.mu.Lock()
	defer inv.mu.Unlock()
	rec, ok := inv.records[id]
	if !ok {
		return errors.New("certs: record not found")
	}
	if rec.Renewal.Mode != "acme" {
		return errors.New("certs: record is not an ACME record")
	}
	rec.NotBefore, rec.NotAfter = leaf.NotBefore, leaf.NotAfter
	rec.Fingerprint = Fingerprint(leaf)
	rec.Domains = leaf.DNSNames
	rec.Renewal.Status = "issued"
	rec.Renewal.LastTry = inv.clock().UTC()
	rec.Renewal.LastError = ""
	rec.UpdatedAt = rec.Renewal.LastTry
	inv.records[id] = rec
	return nil
}

// MarkRenewalFailed records a renewal failure with a bounded
// diagnostic message.
func (inv *Inventory) MarkRenewalFailed(id string, err error) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	rec, ok := inv.records[id]
	if !ok {
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
		if len(msg) > 256 {
			msg = msg[:256]
		}
	}
	rec.Renewal.Status = "failed"
	rec.Renewal.LastTry = inv.clock().UTC()
	rec.Renewal.LastError = msg
	rec.UpdatedAt = rec.Renewal.LastTry
	inv.records[id] = rec
}

// RenewDue returns ACME records whose renewal policy fires at now,
// sorted by NotAfter (most urgent first).
func (inv *Inventory) RenewDue(now time.Time) []Record {
	inv.mu.RLock()
	var out []Record
	for _, rec := range inv.records {
		if rec.NeedsRenewal(now) {
			out = append(out, rec)
		}
	}
	inv.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].NotAfter.Before(out[j].NotAfter) })
	return out
}

// List returns records sorted by expiry (most urgent first), each
// annotated with the derived health class.
func (inv *Inventory) List(expiringWindow time.Duration) []AnnotatedRecord {
	inv.mu.RLock()
	now := inv.clock()
	out := make([]AnnotatedRecord, 0, len(inv.records))
	for _, rec := range inv.records {
		out = append(out, AnnotatedRecord{Record: rec, Health: rec.HealthAt(now, expiringWindow)})
	}
	inv.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].NotAfter.Before(out[j].NotAfter) })
	return out
}

// AnnotatedRecord pairs a record with its derived health class.
type AnnotatedRecord struct {
	Record
	Health Health `json:"health"`
}

// Bind sets the BoundServer of a record (domain association).
func (inv *Inventory) Bind(id, serverID string) error {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	rec, ok := inv.records[id]
	if !ok {
		return errors.New("certs: record not found")
	}
	rec.BoundServer = serverID
	rec.UpdatedAt = inv.clock().UTC()
	inv.records[id] = rec
	return nil
}

// Remove deletes a record. A bound record must be unbound first so
// a certificate cannot disappear out from under a live server
// binding unnoticed.
func (inv *Inventory) Remove(id string) error {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	rec, ok := inv.records[id]
	if !ok {
		return errors.New("certs: record not found")
	}
	if rec.BoundServer != "" {
		return fmt.Errorf("certs: record %s is bound to server %s; unbind first", id, rec.BoundServer)
	}
	delete(inv.records, id)
	return nil
}

func (inv *Inventory) put(rec Record) (Record, error) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	if len(inv.records) >= MaxRecords {
		return Record{}, errors.New("certs: inventory capacity exceeded")
	}
	now := inv.clock().UTC()
	rec.CreatedAt = now
	rec.UpdatedAt = now
	inv.records[rec.ID] = rec
	return rec, nil
}

// ParseLeafPEM is a helper for the ACME path: parse the first
// CERTIFICATE block of a PEM document.
func ParseLeafPEM(pemChain string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemChain))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("certs: no CERTIFICATE PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

func orID(id, fallback string) string {
	if id != "" {
		return id
	}
	return fallback
}
