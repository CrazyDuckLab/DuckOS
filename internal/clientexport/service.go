package clientexport

import (
	"context"
	"errors"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

type Access interface {
	Authorize(string, identity.Permission) (identity.User, error)
	RecordClientExport(token, kind, id, action string) error
}

// Provider resolves current canonical client credentials and revision on every
// request. Implementations must honor cancellation and never return cached old
// credentials after rotation, disablement or expiry.
type Provider interface {
	Export(context.Context, string) (*Document, error)
}

type Service struct {
	access    Access
	providers map[string]Provider
	slots     chan struct{}
}

func NewService(access Access, providers map[string]Provider) (*Service, error) {
	if access == nil {
		return nil, errors.New("client export requires identity enforcement")
	}
	copy := map[string]Provider{}
	for kind, provider := range providers {
		if (kind != "wireguard" && kind != "subscription") || provider == nil {
			return nil, errors.New("invalid client export provider")
		}
		copy[kind] = provider
	}
	return &Service{access: access, providers: copy, slots: make(chan struct{}, 2)}, nil
}

func (s *Service) Export(ctx context.Context, token, kind, id, action string) (*Document, error) {
	if _, err := s.access.Authorize(token, identity.ExportClients); err != nil {
		return nil, err
	}
	if !objectID.MatchString(id) || (action != "qr" && action != "copy" && action != "download") {
		return nil, errors.New("invalid client export request")
	}
	provider := s.providers[kind]
	if provider == nil {
		return nil, errors.New("client export provider unavailable")
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return nil, errors.New("client export capacity exceeded")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	doc, err := provider.Export(ctx, id)
	if err != nil || ctx.Err() != nil || doc == nil || doc.id != id || doc.kind != kind {
		return nil, errors.New("client export unavailable")
	}
	// Durable audit failure denies delivery. A revoked session is checked again
	// after resolution, before any secret is released to the calling transport.
	if err := s.access.RecordClientExport(token, kind, id, action); err != nil {
		return nil, err
	}
	return doc, nil
}
