package errs

import "errors"

// Sentinel errors shared across bounded contexts. Prefer errors.Is against
// these values; wrap with fmt.Errorf("%w", err) to preserve identity.
//
// L1 working-memory errors support HTTP mapping (422/409) via
// internal/transport/response.go until httpx.MapErr lands.
var (
	ErrValidation   = errors.New("validation error")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInternal     = errors.New("internal error")

	// L1 工作记忆专用错误。包装上述通用错误，使 HTTP 传输层可映射到 422 / 409
	// 而无需单独分支，同时业务代码仍可区分各原因。
	// 见 aranea/docs/13 memory-L1-working.md §5.2 与 §9。
	ErrL1Overflow        = errors.New("l1 overflow")
	ErrFieldTooLarge     = errors.New("l1 field too large")
	ErrRevisionConflict  = errors.New("l1 revision conflict")
	ErrTaskNotWritable   = errors.New("l1 task not writable")
	ErrInvalidFieldPath  = errors.New("l1 invalid field path")
	ErrInvalidFieldValue = errors.New("l1 invalid field value")
)
