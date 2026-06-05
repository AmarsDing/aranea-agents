package monitor

import "github.com/google/uuid"

// FailureType classifies the kind of failure for standardized processing.
type FailureType string

const (
	FailureTypeLint      FailureType = "lint_error"
	FailureTypeTest      FailureType = "test_failure"
	FailureTypeBuild     FailureType = "build_failure"
	FailureTypeProtoSync FailureType = "proto_sync"
	FailureTypeRuntime   FailureType = "runtime_error"
)

// FailureReport is a standardized error representation that unifies CI and
// runtime error description formats. Both the CI pipeline and the runtime
// monitor produce FailureReports so that downstream consumers (root cause
// analyzer, auto-fix engine) share a single error model.
type FailureReport struct {
	ID          string            `json:"id"`
	Type        FailureType       `json:"type"`
	Source      string            `json:"source"`       // "ci" or "runtime"
	Job         string            `json:"job"`          // CI job name or runtime component
	File        string            `json:"file"`         // source file path
	Line        int               `json:"line"`         // line number (0 if unknown)
	ErrorCode   string            `json:"error_code"`   // machine-readable error code
	Message     string            `json:"message"`      // human-readable error message
	StackTrace  string            `json:"stack_trace"`  // full stack trace (runtime errors)
	RelatedCode string            `json:"related_code"` // surrounding code snippet
	Metadata    map[string]string `json:"metadata"`     // extra key-value pairs
}

// NewFailureReport creates a FailureReport with a generated ID and initialized Metadata map.
func NewFailureReport() *FailureReport {
	return &FailureReport{
		ID:       uuid.New().String(),
		Metadata: make(map[string]string),
	}
}
