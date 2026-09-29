package agent

import (
	"context"
	"errors"

	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
)

type wireguardVaultResolver struct{ lifecycle *VaultLifecycle }

func (l *VaultLifecycle) WireguardResolver() wireguard.KeyResolver {
	return wireguardVaultResolver{lifecycle: l}
}

func (r wireguardVaultResolver) Resolve(ctx context.Context, key wireguard.KeyRef) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	refs, err := r.lifecycle.List()
	if err != nil {
		return nil, errors.New("protected WireGuard key unavailable")
	}
	for _, ref := range refs {
		if ref.ID != key.VaultID || ref.Revision != key.Revision || ref.KeyID != key.KeyID {
			continue
		}
		if ref.Purpose != "wireguard-private" && ref.Purpose != "wireguard-preshared" {
			break
		}
		secret, err := r.lifecycle.Resolver().Resolve(ctx, ref)
		if err != nil {
			break
		}
		if len(secret) != 32 {
			clear(secret)
			break
		}
		return secret, nil
	}
	return nil, errors.New("protected WireGuard key unavailable")
}
