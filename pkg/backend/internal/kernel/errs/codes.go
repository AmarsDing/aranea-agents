// Code and stable string identifiers. See also error.go for sentinel errors.
package errs

// Code is a stable string identifier for logging, API envelopes, and future
// httpx translation (0 main design.md §7.3).
type Code string

const (
	CodeGenericValidation   Code = "GENERIC_VALIDATION"
	CodeGenericNotFound     Code = "GENERIC_NOT_FOUND"
	CodeGenericConflict     Code = "GENERIC_CONFLICT"
	CodeGenericUnauthorized Code = "GENERIC_UNAUTHORIZED"
	CodeGenericInternal     Code = "GENERIC_INTERNAL"

	// Memory L1 (working memory) — see docs/13 memory-L1-working.md.
	CodeMemoryL1Overflow         Code = "MEMORY_L1_OVERFLOW"
	CodeMemoryL1FieldTooLarge    Code = "MEMORY_L1_FIELD_TOO_LARGE"
	CodeMemoryL1RevisionConflict Code = "MEMORY_L1_REVISION_CONFLICT"
	CodeMemoryL1TaskNotWritable  Code = "MEMORY_L1_TASK_NOT_WRITABLE"
	CodeMemoryL1InvalidFieldPath Code = "MEMORY_L1_INVALID_FIELD_PATH"
	CodeMemoryL1InvalidFieldValue Code = "MEMORY_L1_INVALID_FIELD_VALUE"
)
