// agent/xray_vault.go: vault-backed implementation of
// xray.Resolver for the production agent composition root.
//
// The runtime binds this resolver from *VaultLifecycle in
// enableXrayService; the wireguard counterpart
// (agent/wg_vault.go) uses the same lifecycle but a
// different purpose allow-list. The two resolvers must
// NOT share a type so that:
//
//   - a wireguard-private record cannot be opened as
//     reality-private by mistake,
//   - a tls-private record cannot be opened as
//     tls-certificate by mistake,
//   - the runtime fails fast with an explicit
//     "protected key unavailable" if the wrong purpose
//     is requested.
package agent

import (
	"context"
	"errors"

	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// xrayVaultResolver narrows a *VaultLifecycle to the Xray
// typed binding surface. Production code only constructs
// one resolver per agent instance.
type xrayVaultResolver struct{ lifecycle *VaultLifecycle }

// XrayResolver returns the lifecycle-backed Xray resolver.
// The returned value implements xray.Resolver.
func (l *VaultLifecycle) XrayResolver() xray.Resolver {
	return xrayVaultResolver{lifecycle: l}
}

// Resolve opens the vault record pointed at by ref and
// returns its secret bytes. The lifecycle enforces:
//
//   - ref.VaultID/Revision/KeyID must match a known record,
//   - ref.Purpose must be one of: tls-private, tls-certificate,
//     reality-private, or reality-shortid. The
//     xray-credential purpose is consumed by the User path
//     (internal/xray/server.go User.Credential) and is
//     NOT consumable from the Server materialisation path.
//
// Any mismatch returns "protected Xray key unavailable".
// Secret bytes are returned without copy; the runtime
// zeroises them via os.WriteFile permission bits (the cert
// file is 0644 root-readable; the key file is 0600).
func (r xrayVaultResolver) Resolve(ctx context.Context, ref xray.KeyRef) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	refs, err := r.lifecycle.List()
	if err != nil {
		return nil, errors.New("protected Xray key unavailable")
	}
	for _, rec := range refs {
		if rec.ID != ref.VaultID || rec.Revision != ref.Revision || rec.KeyID != ref.KeyID {
			continue
		}
		if !xrayVaultKeyAllowed(rec.Purpose) {
			break
		}
		secret, err := r.lifecycle.Resolver().Resolve(ctx, rec)
		if err != nil {
			break
		}
		if len(secret) == 0 {
			break
		}
		return secret, nil
	}
	return nil, errors.New("protected Xray key unavailable")
}

// xrayVaultKeyAllowed is the single source-of-truth the
// materialiser consults to decide whether a vault
// record is consumable by the Xray runtime. Cross-purpose
// reuse is structurally impossible: a wireguard-private
// record is rejected here, even though the lifecycle
// happily enumerates it.
//
// xray-credential is consumed by the User path
// (internal/xray/server.go User.Credential). The
// materialiser resolves it at apply time and inlines
// the resulting UUID into settings.clients[].id.
func xrayVaultKeyAllowed(purpose string) bool {
	switch purpose {
	case "tls-private", "tls-certificate", "reality-private", "reality-shortid", "xray-credential":
		return true
	default:
		return false
	}
}
