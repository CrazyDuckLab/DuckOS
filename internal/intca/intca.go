// intca/intca.go: V1 internal CA / PKI +
// service accounts / API tokens +
// provenance / templates / ZTP surface.
package intca

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// CA is an internal CA.
type CA struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"` // "root" | "intermediate"
	Parent      string    `json:"parent,omitempty"`
	CertPath    string    `json:"certPath"`
	KeyVaultRef string    `json:"keyVaultRef"`
	CRLPath     string    `json:"crlPath,omitempty"`
	ValidFrom   time.Time `json:"validFrom"`
	ValidUntil  time.Time `json:"validUntil"`
	Usages      []string  `json:"usages,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether c is well-formed.
func (c CA) Valid() error {
	if !idPattern.MatchString(c.Name) {
		return errors.New("intca: CA name is not canonical")
	}
	if c.Type != "root" && c.Type != "intermediate" {
		return errors.New("intca: CA type is invalid")
	}
	if c.KeyVaultRef == "" {
		return errors.New("intca: keyVaultRef is required")
	}
	if c.ValidUntil.Before(c.ValidFrom) {
		return errors.New("intca: validUntil must be after validFrom")
	}
	return nil
}

// IssuedCert is a certificate issued by
// a CA.
type IssuedCert struct {
	Serial    string    `json:"serial"`
	Subject   string    `json:"subject"`
	CAName    string    `json:"caName"`
	NotBefore time.Time `json:"notBefore"`
	NotAfter  time.Time `json:"notAfter"`
	Revoked   bool      `json:"revoked"`
	RevokedAt time.Time `json:"revokedAt,omitempty"`
	// Usages describes the bound
	// services.
	Usages []string `json:"usages,omitempty"`
}

// CAStore is the CA store.
type CAStore struct {
	mu    sync.Mutex
	certs map[string]CA
	iss   map[string]IssuedCert
}

// NewCAStore returns an empty store.
func NewCAStore() *CAStore {
	return &CAStore{certs: map[string]CA{}, iss: map[string]IssuedCert{}}
}

// AddCA inserts a CA.
func (s *CAStore) AddCA(c CA) error {
	if err := c.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.certs[c.Name] = c
	return nil
}

// Issue records an issued certificate.
func (s *CAStore) Issue(ic IssuedCert) error {
	if ic.Serial == "" {
		return errors.New("intca: serial is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.iss[ic.Serial] = ic
	return nil
}

// Revoke marks a certificate revoked.
func (s *CAStore) Revoke(serial string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ic, ok := s.iss[serial]
	if !ok {
		return errors.New("intca: unknown serial")
	}
	ic.Revoked = true
	ic.RevokedAt = time.Now().UTC()
	s.iss[serial] = ic
	return nil
}

// CAs returns every CA sorted.
func (s *CAStore) CAs() []CA {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CA, 0, len(s.certs))
	for _, c := range s.certs {
		out = append(out, c)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// Issued returns the issued certificate
// list.
func (s *CAStore) Issued() []IssuedCert {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]IssuedCert, 0, len(s.iss))
	for _, c := range s.iss {
		out = append(out, c)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Serial < out[k].Serial })
	return out
}

// APIToken is a service-account API
// token.
type APIToken struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Scopes     []string  `json:"scopes,omitempty"`
	AllowedIPs []string  `json:"allowedIps,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	LastUsed   time.Time `json:"lastUsed,omitempty"`
	Revoked    bool      `json:"revoked"`
}

// Valid reports whether t is well-formed.
func (t APIToken) Valid() error {
	if !idPattern.MatchString(t.ID) {
		return errors.New("intca: token id is not canonical")
	}
	if t.Name == "" {
		return errors.New("intca: token name is required")
	}
	if t.ExpiresAt.IsZero() || !t.ExpiresAt.After(t.CreatedAt) {
		return errors.New("intca: token expiry is invalid")
	}
	return nil
}

// APITokenStore is the API token store.
type APITokenStore struct {
	mu     sync.Mutex
	tokens map[string]APIToken
}

// NewAPITokenStore returns an empty
// store.
func NewAPITokenStore() *APITokenStore {
	return &APITokenStore{tokens: map[string]APIToken{}}
}

// Add inserts a token.
func (s *APITokenStore) Add(t APIToken) error {
	if err := t.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[t.ID] = t
	return nil
}

// Revoke marks a token revoked.
func (s *APITokenStore) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok {
		return errors.New("intca: unknown token")
	}
	t.Revoked = true
	s.tokens[id] = t
	return nil
}

// Touch records the last-used time.
func (s *APITokenStore) Touch(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok {
		return
	}
	t.LastUsed = time.Now().UTC()
	s.tokens[id] = t
}

// All returns every token sorted.
func (s *APITokenStore) All() []APIToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]APIToken, 0, len(s.tokens))
	for _, t := range s.tokens {
		out = append(out, t)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// Provenance is the per-object
// provenance / ownership record.
type Provenance struct {
	Key       string    `json:"key"`
	Owner     string    `json:"owner"` // "manual" | "easy-mode" | "full-mode" | "template" | "import" | "migration" | "automation" | "api" | "scheduler" | "external-managed" | "generated-dependency"
	Actor     string    `json:"actor,omitempty"`
	Locked    bool      `json:"locked,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Valid reports whether p is well-formed.
func (p Provenance) Valid() error {
	if p.Key == "" {
		return errors.New("intca: key is required")
	}
	if p.Owner == "" {
		return errors.New("intca: owner is required")
	}
	return nil
}

// ProvenanceStore is the provenance store.
type ProvenanceStore struct {
	mu sync.Mutex
	p  map[string]Provenance
}

// NewProvenanceStore returns an empty
// store.
func NewProvenanceStore() *ProvenanceStore {
	return &ProvenanceStore{p: map[string]Provenance{}}
}

// Set records the provenance.
func (s *ProvenanceStore) Set(p Provenance) error {
	if err := p.Valid(); err != nil {
		return err
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	p.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p[p.Key] = p
	return nil
}

// TakeOverControl marks the object as
// manually-controlled.
func (s *ProvenanceStore) TakeOverControl(key, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.p[key]
	if !ok {
		return errors.New("intca: unknown key")
	}
	p.Owner = "manual"
	p.Actor = actor
	p.UpdatedAt = time.Now().UTC()
	s.p[key] = p
	return nil
}

// ReleaseControl marks the object as
// external-managed.
func (s *ProvenanceStore) ReleaseControl(key, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.p[key]
	if !ok {
		return errors.New("intca: unknown key")
	}
	p.Owner = "external-managed"
	p.Actor = actor
	p.UpdatedAt = time.Now().UTC()
	s.p[key] = p
	return nil
}

// Get returns a provenance.
func (s *ProvenanceStore) Get(key string) (Provenance, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.p[key]
	return p, ok
}

// Template is a parameterised template.
type Template struct {
	Name  string                 `json:"name"`
	Group string                 `json:"group,omitempty"`
	Role  string                 `json:"role,omitempty"`
	Body  map[string]interface{} `json:"body"`
	// Inheritance lists parent templates
	// for chain.
	Inheritance []string `json:"inheritance,omitempty"`
}

// Valid reports whether t is well-formed.
func (t Template) Valid() error {
	if !idPattern.MatchString(t.Name) {
		return errors.New("intca: template name is not canonical")
	}
	if len(t.Body) == 0 {
		return errors.New("intca: template body is required")
	}
	return nil
}

// TemplateStore is the template store.
type TemplateStore struct {
	mu        sync.Mutex
	templates map[string]Template
}

// NewTemplateStore returns an empty
// store.
func NewTemplateStore() *TemplateStore {
	return &TemplateStore{templates: map[string]Template{}}
}

// Add inserts a template.
func (s *TemplateStore) Add(t Template) error {
	if err := t.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.templates[t.Name] = t
	return nil
}

// Get returns a template.
func (s *TemplateStore) Get(name string) (Template, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.templates[name]
	return t, ok
}

// All returns every template sorted.
func (s *TemplateStore) All() []Template {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Template, 0, len(s.templates))
	for _, t := range s.templates {
		out = append(out, t)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// ZTPCredential is a one-time enrolment
// credential.
type ZTPCredential struct {
	ID         string    `json:"id"`
	Token      string    `json:"token"`
	Scope      string    `json:"scope"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Consumed   bool      `json:"consumed"`
	ConsumedAt time.Time `json:"consumedAt,omitempty"`
	// NodeHint binds the credential to a
	// serial / machine id.
	NodeHint string `json:"nodeHint,omitempty"`
}

// ZTPStore is the ZTP store.
type ZTPStore struct {
	mu    sync.Mutex
	creds map[string]ZTPCredential
}

// NewZTPStore returns an empty store.
func NewZTPStore() *ZTPStore {
	return &ZTPStore{creds: map[string]ZTPCredential{}}
}

// Issue creates a one-time credential.
func (s *ZTPStore) Issue(c ZTPCredential) error {
	if c.ID == "" || c.Token == "" {
		return errors.New("intca: ztp id+token required")
	}
	if c.ExpiresAt.IsZero() {
		return errors.New("intca: ztp expiry required")
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds[c.ID] = c
	return nil
}

// Consume marks the credential as used.
func (s *ZTPStore) Consume(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.creds[id]
	if !ok {
		return errors.New("intca: unknown ztp id")
	}
	if c.Consumed {
		return errors.New("intca: ztp already consumed")
	}
	if !c.ExpiresAt.After(time.Now()) {
		return errors.New("intca: ztp expired")
	}
	c.Consumed = true
	c.ConsumedAt = time.Now().UTC()
	s.creds[c.ID] = c
	return nil
}

// Active returns the active (un-consumed,
// un-expired) credentials.
func (s *ZTPStore) Active() []ZTPCredential {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []ZTPCredential{}
	now := time.Now()
	for _, c := range s.creds {
		if !c.Consumed && c.ExpiresAt.After(now) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}
