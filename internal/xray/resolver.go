// Package xray / resolver.go: bounded vault-aware key resolver.
//
// The Xray runtime renders VLESS / Trojan / SOCKS / etc.
// configs from a typed catalog (internal/xray/server.go +
// render.go). The render emits:
//
//   - TLS:     streamSettings.tlsSettings.{certificateFile,keyFile}
//              pointing at <stage>/<name>.crt and <name>.key.
//              The runtime must materialize those files from the
//              vault references TLSCertRef / TLSKeyRef BEFORE
//              running `xray run -c active.json -test`.
//   - REALITY: streamSettings.realitySettings.{privateKey,shortIds}
//              placeholder strings carrying the vault key id.
//              The runtime must substitute the placeholder with
//              the actual base64(X25519-private-key) / hex(short-id)
//              bytes BEFORE running `xray run -c active.json -test`.
//
// Without this step the rendered config cannot pass `xray -test`
// (TLS: file does not exist; REALITY: invalid JSON). The runtime
// would refuse to bring the daemon up.
//
// The Resolver interface mirrors the wireguard.KeyResolver
// pattern: it is the narrow contract the runtime needs to
// materialise vault-managed secrets. The agent binds a
// vault-backed implementation at startup; the runtime holds
// the resolver and calls it from Apply.
package xray

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolver resolves a typed vault reference into its raw
// secret bytes. The runtime owns the resolver; the catalog
// and the renderer are unaware of the concrete backing
// store (production: vault.VaultLifecycle; tests: stub).
//
// Resolve MUST return an error if ctx is cancelled, if the
// ref is unknown to the resolver, or if the resolved secret
// has been zero-length. Resolve MAY return any other error
// the backing store surfaces; the runtime propagates it
// verbatim through Apply.
type Resolver interface {
	Resolve(ctx context.Context, ref KeyRef) ([]byte, error)
}

// Materialize walks the rendered Config and substitutes the
// vault-backed references with their secret bytes:
//
//   - TLS inbound: TLSCertRef -> write <stage>/<name>.crt
//                  TLSKeyRef  -> write <stage>/<name>.key
//                  (the renderer's certificateFile/keyFile
//                  paths already point at these locations.)
//   - REALITY inbound: RealityKeyRef -> base64-encoded
//                  privateKey inline in the JSON.
//                  ShortIDRef -> hex-encoded short id
//                  inline in the JSON.
//
// Materialize is idempotent: if the inbound has no security
// material (security=none) or the vault resolver returns
// zero bytes, the inbound is left unchanged and Apply
// proceeds without that material.
//
// The returned Config is a deep copy of cfg with the
// in-memory parts (REALITY keys) replaced; the TLS cert/key
// files are written to <stageDir>/<name>.{crt,key}. The
// caller MUST guarantee stageDir exists (the runtime
// creates it via os.MkdirAll(stage, 0700) at startup).
//
// A nil resolver is accepted when no inbound requires
// materialisation (security=none only); it is rejected
// with a typed error when any inbound has tls or reality
// security so the runtime fails fast.
func Materialize(ctx context.Context, cfg Config, stageDir string, servers []Server, users []User, resolver Resolver) (Config, error) {
	if stageDir == "" {
		return cfg, errors.New("xray: stage dir is required for TLS materialization")
	}
	// Build server-by-name lookup.
	byName := make(map[string]Server, len(servers))
	for _, s := range servers {
		byName[s.Name] = s
	}
	// Build credential-keyid -> KeyRef lookup so the
	// materialiser can resolve placeholder client.id
	// strings emitted by the renderer when a user has
	// only a Credential vault ref (no inline UUID).
	credByKeyID := make(map[string]KeyRef)
	for _, u := range users {
		if u.Credential == nil {
			continue
		}
		credByKeyID[u.Credential.KeyID] = *u.Credential
	}
	// Decide whether a resolver is required at all.
	requiresResolver := false
	for _, s := range servers {
		if s.Security == SecurityTLS || s.Security == SecurityReality {
			requiresResolver = true
			break
		}
	}
	if requiresResolver && resolver == nil {
		return cfg, errors.New("xray: vault resolver is required to materialize TLS / REALITY")
	}
	if resolver == nil {
		return cfg, nil
	}
	// Walk inbounds and materialise.
	for i := range cfg.Inbounds {
		in := cfg.Inbounds[i]
		var server *Server
		// Render tag is "in-<name>"; strip the prefix.
		if name := strings.TrimPrefix(in.Tag, "in-"); name != in.Tag {
			if s, ok := byName[name]; ok {
				server = &s
			}
		}
		if server == nil {
			continue
		}
		switch server.Security {
		case SecurityTLS:
			if server.TLSCertRef == nil || server.TLSKeyRef == nil {
				continue
			}
			if err := ctx.Err(); err != nil {
				return cfg, err
			}
			cert, err := resolver.Resolve(ctx, *server.TLSCertRef)
			if err != nil {
				return cfg, fmt.Errorf("xray: resolve tlsCertRef: %w", err)
			}
			key, err := resolver.Resolve(ctx, *server.TLSKeyRef)
			if err != nil {
				return cfg, fmt.Errorf("xray: resolve tlsKeyRef: %w", err)
			}
			certPath := filepath.Join(stageDir, server.Name+".crt")
			keyPath := filepath.Join(stageDir, server.Name+".key")
			if err := os.WriteFile(certPath, cert, 0o644); err != nil {
				return cfg, fmt.Errorf("xray: write cert %s: %w", certPath, err)
			}
			if err := os.WriteFile(keyPath, key, 0o600); err != nil {
				return cfg, fmt.Errorf("xray: write key %s: %w", keyPath, err)
			}
		case SecurityReality:
			if server.RealityKeyRef == nil || server.ShortIDRef == nil {
				continue
			}
			if err := ctx.Err(); err != nil {
				return cfg, err
			}
			key, err := resolver.Resolve(ctx, *server.RealityKeyRef)
			if err != nil {
				return cfg, fmt.Errorf("xray: resolve realityKeyRef: %w", err)
			}
			sid, err := resolver.Resolve(ctx, *server.ShortIDRef)
			if err != nil {
				return cfg, fmt.Errorf("xray: resolve shortIdRef: %w", err)
			}
			// Inline the materialised bytes into the
			// rendered StreamSettings via a JSON
			// round-trip: decode, replace placeholders
			// with real bytes, encode. The renderer
			// emits a placeholder string of the form
			// "<resolved at apply time; see vault record
			// <KeyID>>" for both privateKey and each
			// shortIds entry; substitution keys on the
			// KeyID so multiple REALITY inbounds do not
			// collide.
			//
			// Xray accepts the privateKey ONLY as
			// URL-safe base64 without padding; standard
			// base64 or raw hex are rejected with
			// "invalid privateKey". The shortIds are
			// raw hex strings.
			inlined, err := substituteRealityFromValues(&in, server.RealityKeyRef.KeyID, key, server.ShortIDRef.KeyID, sid)
			if err != nil {
				return cfg, fmt.Errorf("xray: substitute reality inbound %s: %w", in.Tag, err)
			}
			cfg.Inbounds[i] = inlined
		}
		// User Credential substitution: a user may carry
		// a Credential vault ref (purpose "xray-credential")
		// instead of an inline UUID. xray(8) refuses to
		// start with an empty settings.clients[].id, so
		// the renderer emits a placeholder string and the
		// materialiser resolves the corresponding KeyRef
		// at apply time and inlines the canonical UUID.
		// The pass runs after the security switch so
		// credentials are substituted even for security=none
		// or security=tls inbounds.
		if hasCredentialPlaceholder(in) {
			if err := substituteCredentialsOnInbound(&cfg.Inbounds[i], credByKeyID, resolver, ctx); err != nil {
				return cfg, err
			}
		}
	}
	return cfg, nil
}

// hasCredentialPlaceholder reports whether the inbound
// contains any settings.clients[].id that still carries
// the credential placeholder string. The placeholder is
// JSON-escaped by the renderer (`<` -> `\u003c`, `>` ->
// `\u003e`); we search for the escaped form so the
// check survives JSON marshalling.
func hasCredentialPlaceholder(in Inbound) bool {
	if len(in.Settings) == 0 {
		return false
	}
	// The rendered placeholder is the literal
	// "<resolved at apply time; see vault record " in
	// the in-memory model, but JSON marshals it to the
	// escaped form. Match the escaped form to avoid a
	// re-marshal of the entire Settings.
	return bytes.Contains(in.Settings, []byte(jsonPlaceholderMarker))
}

// jsonPlaceholderMarker is the JSON-escaped form of
// realityMarker, used to scan the marshalled Settings
// JSON for placeholder-bearing client.id values.
const jsonPlaceholderMarker = "\\u003cresolved at apply time; see vault record "

// substituteCredentialsOnInbound walks the inbound's
// settings.clients[] and substitutes every
// placeholder client.id (form: "<resolved at apply
// time; see vault record <credKeyID>>") with the
// resolved UUID bytes for the matching credential
// KeyRef. The placeholder's last token is the vault
// KeyID so the right KeyRef is selected even when
// multiple users carry distinct credentials.
func substituteCredentialsOnInbound(in *Inbound, credByKeyID map[string]KeyRef, resolver Resolver, ctx context.Context) error {
	if len(in.Settings) == 0 {
		return nil
	}
	var settings map[string]any
	if err := json.Unmarshal(in.Settings, &settings); err != nil {
		return fmt.Errorf("decode settings: %w", err)
	}
	clientsRaw, ok := settings["clients"]
	if !ok {
		return nil
	}
	clients, ok := clientsRaw.([]any)
	if !ok {
		return nil
	}
	changed := false
	for i, e := range clients {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		v, ok := entry["id"].(string)
		if !ok {
			continue
		}
		credKeyID, isPlaceholder := parseCredentialPlaceholder(v)
		if !isPlaceholder {
			continue
		}
		cred, ok := credByKeyID[credKeyID]
		if !ok {
			return fmt.Errorf("xray: substitute credentials: no user with credential KeyID %s", credKeyID)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		uuid, err := resolver.Resolve(ctx, cred)
		if err != nil {
			return fmt.Errorf("xray: resolve credential: %w", err)
		}
		entry["id"] = strings.TrimSpace(string(uuid))
		clients[i] = entry
		changed = true
	}
	if !changed {
		return nil
	}
	settings["clients"] = clients
	encoded, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	in.Settings = encoded
	return nil
}

// parseCredentialPlaceholder extracts the vault KeyID
// from a settings.clients[].id placeholder string and
// reports whether the string was a placeholder. The
// format mirrors substituteCredentialFromValues: the
// string starts with realityMarker and ends with ">".
func parseCredentialPlaceholder(s string) (string, bool) {
	return parsePlaceholder(s)
}

func parsePlaceholder(s string) (string, bool) {
	const trailing = ">"
	if !strings.HasPrefix(s, realityMarker) {
		return "", false
	}
	if !strings.HasSuffix(s, trailing) {
		return "", false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(s, realityMarker), trailing)
	return body, true
}

// realityMarker is the substring the renderer emits as
// a placeholder inside StreamSettings.realitySettings;
// both the privateKey field and each shortIds entry are
// emitted as a marker whose last token is the vault KeyID.
// substituteRealityFromValues walks the decoded
// streamSettings JSON tree and replaces any string whose
// raw form starts with realityMarker and whose suffix
// matches the given key id with the actual bytes.
const realityMarker = "<resolved at apply time; see vault record "

// CredentialPlaceholder returns the placeholder string the
// renderer emits in settings.clients[].id when a user has
// a Credential vault ref but no inline UUID. The
// materialiser walks the inbound's settings.clients tree
// and replaces the matching placeholder with the resolved
// UUID bytes (a UTF-8 string of canonical
// 8-4-4-4-12 hex form). The placeholder's last token is
// the vault KeyID of the user's Credential ref so that two
// users with two distinct xray-credential records do not
// collide at substitution time.
func CredentialPlaceholder(credKeyID string) string {
	return realityMarker + credKeyID + ">"
}

// substituteRealityFromValues walks an inbound's
// streamSettings.realitySettings and replaces the
// placeholder strings carrying privKeyID / sidKeyID with
// the actual URL-safe-base64(privateKey) and hex(shortID)
// bytes.
//
// The function operates on the decoded JSON tree
// (map[string]any) so it is robust to JSON-level
// character escaping (`<` becomes `\u003c` inside a
// string). The caller receives a fully-substituted
// Inbound value that no longer references the vault.
func substituteRealityFromValues(in *Inbound, privKeyID string, privKey []byte, sidKeyID string, sid []byte) (Inbound, error) {
	privB64 := base64.RawURLEncoding.EncodeToString(privKey)
	sidHex := hex.EncodeToString(sid)
	// Decode streamSettings into a generic tree so we can
	// navigate the realitySettings sub-tree.
	var stream map[string]any
	if len(in.StreamSettings) == 0 {
		return Inbound{}, errors.New("xray: inbound has no streamSettings")
	}
	if err := json.Unmarshal(in.StreamSettings, &stream); err != nil {
		return Inbound{}, fmt.Errorf("decode streamSettings: %w", err)
	}
	rsRaw, ok := stream["realitySettings"]
	if !ok {
		return Inbound{}, errors.New("xray: inbound has no realitySettings")
	}
	rs, ok := rsRaw.(map[string]any)
	if !ok {
		return Inbound{}, errors.New("xray: realitySettings is not an object")
	}
	// privateKey: substitute when the existing value is
	// "<resolved at apply time; see vault record <privKeyID>>".
	if v, ok := rs["privateKey"].(string); ok && realityMatches(v, privKeyID) {
		rs["privateKey"] = privB64
	}
	// shortIds: substitute every entry whose value is
	// "<resolved at apply time; see vault record <sidKeyID>>".
	if arr, ok := rs["shortIds"].([]any); ok {
		for i, e := range arr {
			if s, ok := e.(string); ok && realityMatches(s, sidKeyID) {
				arr[i] = sidHex
			}
		}
		rs["shortIds"] = arr
	}
	// Re-marshal streamSettings.
	encoded, err := json.Marshal(stream)
	if err != nil {
		return Inbound{}, fmt.Errorf("encode streamSettings: %w", err)
	}
	in.StreamSettings = encoded
	return *in, nil
}

// realityMatches reports whether the placeholder string
// emitted by the renderer carries the expected vault
// KeyID at its end. The renderer uses the format:
//
//   "<resolved at apply time; see vault record <KeyID>>"
//
// (note the single trailing ">" — the placeholder is
// a single opening bracket, the marker body, the key
// id, and a single closing bracket). The matcher splits
// on the literal ">" terminator.
func realityMatches(s, keyID string) bool {
	const trailing = ">"
	if !strings.HasPrefix(s, realityMarker) {
		return false
	}
	if !strings.HasSuffix(s, trailing) {
		return false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(s, realityMarker), trailing)
	return body == keyID
}

// substituteCredentialFromValues walks the inbound's
// settings.clients tree and replaces any settings.clients[i].id
// whose value is the placeholder emitted by the renderer
// for a user with a Credential vault ref (no inline UUID).
// The placeholder carries the vault KeyID of the user's
// Credential ref; the resolved value is the inlined
// canonical UUID string the materialiser fetched from
// the vault at apply time.
//
// xray(8) refuses to start with an empty
// settings.clients[].id, so the substitution is the
// gate between "user has only a credential ref" and
// "xray accepts the rendered config".
func substituteCredentialFromValues(in *Inbound, credKeyID string, uuid []byte) (Inbound, error) {
	if len(in.Settings) == 0 {
		return *in, nil
	}
	var settings map[string]any
	if err := json.Unmarshal(in.Settings, &settings); err != nil {
		return Inbound{}, fmt.Errorf("decode settings: %w", err)
	}
	clientsRaw, ok := settings["clients"]
	if !ok {
		return *in, nil
	}
	clients, ok := clientsRaw.([]any)
	if !ok {
		return *in, nil
	}
	uuidStr := strings.TrimSpace(string(uuid))
	for i, e := range clients {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		v, ok := entry["id"].(string)
		if !ok {
			continue
		}
		if realityMatches(v, credKeyID) {
			entry["id"] = uuidStr
			clients[i] = entry
		}
	}
	settings["clients"] = clients
	encoded, err := json.Marshal(settings)
	if err != nil {
		return Inbound{}, fmt.Errorf("encode settings: %w", err)
	}
	in.Settings = encoded
	return *in, nil
}