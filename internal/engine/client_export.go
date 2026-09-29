package engine

import (
	"context"
	"errors"

	"github.com/CrazyDuckLab/DuckOS/internal/clientexport"
)

// EnableClientExports binds canonical providers at startup, before serving HTTP.
// Providers and identity retain ownership of their storage and lifecycle.
func (e *Engine) EnableClientExports(s *clientexport.Service) { e.exports = s }

func (e *Engine) ExportClient(ctx context.Context, token, kind, id, action string) (*clientexport.Document, error) {
	if e.exports == nil {
		return nil, errors.New("client exports are not configured")
	}
	return e.exports.Export(ctx, token, kind, id, action)
}
