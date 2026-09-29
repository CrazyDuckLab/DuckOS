package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/CrazyDuckLab/DuckOS/internal/clientexport"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

type clientExporter interface {
	ExportClient(context.Context, string, string, string, string) (*clientexport.Document, error)
}

func (h *Handler) serveClientExport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Pragma", "no-cache")
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		problem(w, r, 400, errorCategoryFromCode("invalid_request"), "invalid_request", "Export parameters must not appear in URLs.")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		problem(w, r, 405, errorCategoryFromCode("method_not_allowed"), "method_not_allowed", "Client exports require POST.")
		return
	}
	if h.identity == nil {
		identityError(w, r, identity.ErrUnauthorized)
		return
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || len(r.Header.Values("Authorization")) != 1 {
		identityError(w, r, identity.ErrUnauthorized)
		return
	}
	if _, err := h.identity.Authorize(token, identity.ExportClients); err != nil {
		identityError(w, r, err)
		return
	}
	if r.Header.Get("Content-Encoding") != "" {
		problem(w, r, 415, errorCategoryFromCode("invalid_content_type"), "invalid_content_type", "Use uncompressed application/json.")
		return
	}
	var input struct {
		ID     string `json:"id"`
		Kind   string `json:"kind"`
		Action string `json:"action"`
	}
	if !identityBody(w, r, &input) {
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		problem(w, r, 503, errorCategoryFromCode("export_unavailable"), "export_unavailable", "Client export is unavailable.")
		return
	}
	exporter, ok := h.engine.(clientExporter)
	if !ok {
		problem(w, r, 503, errorCategoryFromCode("export_unavailable"), "export_unavailable", "Client export is unavailable.")
		return
	}
	doc, err := exporter.ExportClient(r.Context(), token, input.Kind, input.ID, input.Action)
	if err != nil || doc == nil {
		// Provider errors may include credentials. Never serialize their messages.
		problem(w, r, 403, errorCategoryFromCode("export_unavailable"), "export_unavailable", "Client export is unavailable or access has changed.")
		return
	}
	if r.Context().Err() != nil {
		return
	}
	// Explicit secret serialization is confined to this authenticated no-store
	// response. The browser uses this single payload for QR, copy and download.
	writeJSON(w, 200, struct {
		ID       string `json:"id"`
		Revision string `json:"revision"`
		Kind     string `json:"kind"`
		Filename string `json:"filename"`
		Payload  string `json:"payload"`
	}{doc.ID(), doc.Revision(), doc.Kind(), doc.Filename(), doc.Reveal()})
}
