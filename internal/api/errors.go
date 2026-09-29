// Package api structured errors (G15): language-neutral
// category + stable code + safe human-readable message + a
// per-call request id. UI and CLI localise by category; the
// category itself is never translated.
package api

// ErrorCategory is the language-neutral classification of
// every public API error. It is what UI and CLI use to look
// up localized messages.
type ErrorCategory string

const (
	CatValidation         ErrorCategory = "validation"
	CatRevisionConflict   ErrorCategory = "revision_conflict"
	CatPermissionDenied   ErrorCategory = "permission_denied"
	CatDependencyConflict ErrorCategory = "dependency_conflict"
	CatUnsupported        ErrorCategory = "unsupported"
	CatHardwarePending    ErrorCategory = "hardware_pending"
	CatCredentialPending  ErrorCategory = "credential_pending"
	CatDegraded           ErrorCategory = "degraded"
	CatTimeout            ErrorCategory = "timeout"
	CatRuntimeFailure     ErrorCategory = "runtime_failure"
	CatRollback           ErrorCategory = "rollback"
	CatRecoveryRequired   ErrorCategory = "recovery_required"
	CatUnknownOutcome     ErrorCategory = "unknown_outcome"
	CatInternal           ErrorCategory = "internal"
)

// errorCategoryFromCode maps every known API error code to
// its canonical category. Unknown codes fall back to
// "internal".
func errorCategoryFromCode(code string) ErrorCategory {
	switch code {
	case "invalid_request", "invalid_body", "invalid_plan",
		"invalid_content_type", "trailing_data":
		return CatValidation
	case "tls_required":
		return CatValidation
	case "unauthorized":
		return CatPermissionDenied
	case "forbidden":
		return CatPermissionDenied
	case "method_not_allowed":
		return CatValidation
	case "not_found":
		return CatUnsupported
	case "payload_too_large":
		return CatValidation
	case "enrollment_required":
		return CatPermissionDenied
	case "identity_change_rejected":
		return CatRecoveryRequired
	case "auth_busy", "rate_limited":
		return CatTimeout
	case "agent_unavailable", "planner_unavailable",
		"qos_apply_failed", "qos_rollback_failed",
		"ups_probe_failed", "export_unavailable":
		return CatRuntimeFailure
	case "api_busy":
		return CatTimeout
	case "untrusted_host", "untrusted_origin":
		return CatPermissionDenied
	default:
		return CatInternal
	}
}
