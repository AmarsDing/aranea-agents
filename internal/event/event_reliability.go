package event

import "aranea-agents/internal/event/contract"

// EventReliability defines the reliability guarantee for an event type.
// Stability:evolving
type EventReliability int

const (
	// ReliabilityCritical events use WBPF (Write-Before-Publish-Fanout).
	// They are persisted to WAL before being published to the Bus.
	// Loss of these events causes observable data corruption.
	ReliabilityCritical EventReliability = iota

	// ReliabilityImportant events use BlockUpTo + async persistence.
	// They are published immediately but persisted asynchronously.
	ReliabilityImportant

	// ReliabilityInformational events are best-effort with no persistence.
	ReliabilityInformational
)

// ClassifyEventReliability returns the reliability tier for an event type.
// Classification follows AS-EVT-01:
//   - Critical (WBPF): ToolResult, Error, RunnerCompletion, Checkpoint
//   - Important (BlockUpTo + async): StateDelta, TokenUsage, RunStatus,
//     SessionStatusChanged, GraphNodeEnd, TeamRunFinished, UserFeedback
//   - Informational (best-effort): everything else
func ClassifyEventReliability(t contract.EnvelopeType) EventReliability {
	switch t {
	case contract.EnvelopeTypeToolResult,
		contract.EnvelopeTypeError,
		contract.EnvelopeTypeRunnerCompletion,
		contract.EnvelopeTypeCheckpoint:
		return ReliabilityCritical
	case contract.EnvelopeTypeStateDelta,
		contract.EnvelopeTypeTokenUsage,
		contract.EnvelopeTypeRunStatus,
		contract.EnvelopeTypeSessionStatusChanged,
		contract.EnvelopeTypeGraphNodeEnd,
		contract.EnvelopeTypeTeamRunFinished,
		contract.EnvelopeTypeUserFeedback:
		return ReliabilityImportant
	default:
		return ReliabilityInformational
	}
}

// IsCriticalWBPFType returns true if the event type requires WBPF.
func IsCriticalWBPFType(t contract.EnvelopeType) bool {
	return ClassifyEventReliability(t) == ReliabilityCritical
}
