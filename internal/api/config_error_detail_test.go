// api/config_error_detail_test.go: writeEngineError must
// surface the underlying validator/network message in the
// `detail` field of the problem envelope. The default-case
// "invalid_request" used to swallow the real reason — the
// UI/CLI/operator only saw "Configuration request was
// rejected; inspect validated input and current state." This
// test pins the new behavior for every category the helper
// maps.
package api

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/engine"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// runWriteEngineError invokes writeEngineError against a
// recorder and returns the JSON body of the response.
func runWriteEngineError(t *testing.T, err error) (int, string) {
	t.Helper()
	r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/v1/config/apply", nil)
	w := httptest.NewRecorder()
	writeEngineError(w, r, err)
	return w.Code, w.Body.String()
}

// TestWriteEngineErrorSurfacesValidatorDetail ensures the
// default 400 invalid_request carries the underlying
// network.Decode error message verbatim, so operators do not
// have to consult journal-only logs to find out why their
// candidate was refused.
func TestWriteEngineErrorSurfacesValidatorDetail(t *testing.T) {
	const reason = "at least one static management LAN is required"
	code, body := runWriteEngineError(t, errors.New(reason))
	if code != 400 {
		t.Fatalf("status = %d, want 400", code)
	}
	if !strings.Contains(body, `"category":"validation"`) {
		t.Fatalf("body missing category: %s", body)
	}
	if !strings.Contains(body, `"code":"invalid_request"`) {
		t.Fatalf("body missing code: %s", body)
	}
	if !strings.Contains(body, fmt.Sprintf("%q", reason)) {
		t.Fatalf("body missing detail %q: %s", reason, body)
	}
	if !strings.Contains(body, `"requestId":"`) {
		t.Fatalf("body missing requestId: %s", body)
	}
}

// TestWriteEngineErrorRevisionConflict ensures revision
// conflicts keep the legacy 409 mapping and also surface the
// underlying detail string.
func TestWriteEngineErrorRevisionConflict(t *testing.T) {
	code, body := runWriteEngineError(t, fmt.Errorf("wrap: %w", state.ErrConflict))
	if code != 409 {
		t.Fatalf("status = %d, want 409", code)
	}
	if !strings.Contains(body, `"code":"revision_conflict"`) {
		t.Fatalf("body missing code: %s", body)
	}
	if !strings.Contains(body, "configuration revision conflict") {
		t.Fatalf("body missing underlying detail: %s", body)
	}
}

// TestWriteEngineErrorRecovery ensures ErrRecovery keeps the
// 409 recovery_required mapping with its detail preserved.
func TestWriteEngineErrorRecovery(t *testing.T) {
	code, body := runWriteEngineError(t, fmt.Errorf("wrap: %w", state.ErrRecovery))
	if code != 409 {
		t.Fatalf("status = %d, want 409", code)
	}
	if !strings.Contains(body, `"code":"recovery_required"`) {
		t.Fatalf("body missing code: %s", body)
	}
	if !strings.Contains(body, "recovery") {
		t.Fatalf("body missing underlying detail: %s", body)
	}
}

// TestWriteEngineErrorRuntimeUnavailable ensures the
// ErrConfigRuntimeUnavailable path keeps its 503 mapping and
// still carries a detail field for symmetry with the other
// branches.
func TestWriteEngineErrorRuntimeUnavailable(t *testing.T) {
	code, body := runWriteEngineError(t, engine.ErrConfigRuntimeUnavailable)
	if code != 503 {
		t.Fatalf("status = %d, want 503", code)
	}
	if !strings.Contains(body, `"code":"config_runtime_unavailable"`) {
		t.Fatalf("body missing code: %s", body)
	}
	if !strings.Contains(body, `"detail":`) {
		t.Fatalf("body missing detail field: %s", body)
	}
}

// TestWriteEngineErrorPreservesCallerRequestID ensures that
// when the caller supplied an X-Request-Id, the helper
// echoes it back on the response and reuses it in the body.
// This lets correlation tooling keep working without parsing
// the body twice.
func TestWriteEngineErrorPreservesCallerRequestID(t *testing.T) {
	r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/v1/config/apply", nil)
	r.Header.Set("X-Request-Id", "witness-request-id-12345")
	w := httptest.NewRecorder()
	writeEngineError(w, r, errors.New("some validator message"))
	if got := w.Header().Get("X-Request-Id"); got != "witness-request-id-12345" {
		t.Fatalf("response X-Request-Id = %q, want preserved", got)
	}
	if !strings.Contains(w.Body.String(), "witness-request-id-12345") {
		t.Fatalf("body missing requestId: %s", w.Body.String())
	}
}
