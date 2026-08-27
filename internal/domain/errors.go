package domain

import "fmt"

type ErrorCode string

const (
	CodeValidation          ErrorCode = "VALIDATION_ERROR"
	CodeNotFound            ErrorCode = "NOT_FOUND"
	CodeVersionConflict     ErrorCode = "VERSION_CONFLICT"
	CodeStateConflict       ErrorCode = "STATE_CONFLICT"
	CodeDuplicateEvidence   ErrorCode = "DUPLICATE_EVIDENCE"
	CodeIdempotencyConflict ErrorCode = "IDEMPOTENCY_CONFLICT"
	CodeForbidden           ErrorCode = "FORBIDDEN"
	CodeRetryableStorage    ErrorCode = "RETRYABLE_STORAGE_ERROR"
	CodeUnknownRuleSet      ErrorCode = "UNKNOWN_RULE_SET"
	CodePreviewExpired      ErrorCode = "PREVIEW_EXPIRED"
	CodeAuditInconsistent   ErrorCode = "AUDIT_INCONSISTENT"
)

type BusinessError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *BusinessError) Error() string { return e.Message }

func NewError(code ErrorCode, format string, args ...any) error {
	return &BusinessError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func ErrorCodeOf(err error) ErrorCode {
	if business, ok := err.(*BusinessError); ok {
		return business.Code
	}
	return "INTERNAL_ERROR"
}
