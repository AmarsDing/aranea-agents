package contract

// EventReliability defines the reliability guarantee for an event type.
// Stability:stable
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
//     SessionStatusChanged, GraphNodeEnd, TeamRunFinished, TeamRunFailed, UserFeedback
//   - Informational (best-effort): ContextUsage, everything else
//
// This function is the single source of truth for event reliability classification.
// All consumers (Bus delivery policy, EventWAL, persist handler) must use this
// function instead of maintaining their own critical/important type sets.
//
// Stability:stable
func ClassifyEventReliability(t EnvelopeType) EventReliability {
	switch t {
	case EnvelopeTypeToolResult,
		EnvelopeTypeError,
		EnvelopeTypeRunnerCompletion,
		EnvelopeTypeCheckpoint:
		return ReliabilityCritical
	case EnvelopeTypeStateDelta,
		EnvelopeTypeTokenUsage,
		EnvelopeTypeRunStatus,
		EnvelopeTypeSessionStatusChanged,
		EnvelopeTypeGraphNodeEnd,
		EnvelopeTypeTeamRunFinished,
		EnvelopeTypeTeamRunFailed,
		EnvelopeTypeUserFeedback:
		return ReliabilityImportant
	case EnvelopeTypeContextUsage:
		return ReliabilityInformational
	default:
		return ReliabilityInformational
	}
}

// IsCriticalWBPFType returns true if the event type requires WBPF.
func IsCriticalWBPFType(t EnvelopeType) bool {
	return ClassifyEventReliability(t) == ReliabilityCritical
}

// RequiresBlockUpTo returns true if the event type must use BlockUpTo delivery
// (i.e., it is Critical or Important — never silently dropped).
func RequiresBlockUpTo(t EnvelopeType) bool {
	r := ClassifyEventReliability(t)
	return r == ReliabilityCritical || r == ReliabilityImportant
}
