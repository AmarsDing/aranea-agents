// Package event re-exports types from contract and adds implementation-specific helpers.
// New code in biz should import event/contract directly.
package event

import (
	"aranea-agents/internal/event/contract"
)

// Re-export the Envelope-era types that remain in active production use.
// See contract/envelope_types.go for the rationale (ADR-03 Phase 5 Blocker G).
type (
	EnvelopeError      = contract.EnvelopeError
	EnvelopeTokenUsage = contract.EnvelopeTokenUsage
)

// Re-export the tool invocation error_code constants consumed by the agent layer.
const (
	ErrorCodeToolError            = contract.ErrorCodeToolError
	ErrorCodeConfirmationRequired = contract.ErrorCodeConfirmationRequired
	ErrorCodeConfirmationDenied   = contract.ErrorCodeConfirmationDenied
	ErrorCodeConfirmationTimeout  = contract.ErrorCodeConfirmationTimeout
)
