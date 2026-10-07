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
func Materialize(ctx context.Context, cfg Config, stageDir string, servers []Server, resolver Resolver) (Config, error) {
	if stageDir == "" {
		return cfg, errors.New("xray: stage dir is required for TLS materialization")
	}
	// Build server-by-name lookup.
	byName := make(map[string]Server, len(servers))
	for _, s := range servers {
		byName[s.Name] = s
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
	}
	return cfg, nil
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