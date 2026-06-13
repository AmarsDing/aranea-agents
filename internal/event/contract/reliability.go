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
//   - Important (BlockUpTo + async): see isImportantEvent for the full list
//   - Informational (best-effort): ContextUsage, everything else
//
// This function is the single source of truth for event reliability classification.
// All consumers (Bus delivery policy, EventWAL, persist handler) must use this
// function instead of maintaining their own critical/important type sets.
//
// Stability:stable
func ClassifyEventReliability(t EnvelopeType) EventReliability {
	if isCriticalEvent(t) {
		return ReliabilityCritical
	}
	if isImportantEvent(t) {
		return ReliabilityImportant
	}
	return ReliabilityInformational
}

// isCriticalEvent returns true for events that require WBPF (Write-Before-Publish-Fanout).
// Loss of these events causes observable data corruption.
func isCriticalEvent(t EnvelopeType) bool {
	switch t {
	case EnvelopeTypeToolResult,
		EnvelopeTypeError,
		EnvelopeTypeRunnerCompletion,
		EnvelopeTypeCheckpoint:
		return true
	default:
		return false
	}
}

// isImportantEvent returns true for events that require BlockUpTo delivery
// (never silently dropped) but do not need WBPF.
// Organized by domain:
//   - Runtime: StateDelta, TokenUsage, RunStatus
//   - Session: SessionStatusChanged
//   - Graph: GraphNodeEnd
//   - TeamRun: TeamRunFinished, TeamRunFailed
//   - Spirit: SpiritTeamAssembled, SpiritTeamCompleted, SpiritTeamFailed,
//     SpiritTeamsAllCompleted, SpiritOrchestrationStarted
//   - User: UserFeedback
func isImportantEvent(t EnvelopeType) bool {
	switch t {
	// Runtime
	case EnvelopeTypeStateDelta,
		EnvelopeTypeTokenUsage,
		EnvelopeTypeRunStatus:
		return true
	// Session
	case EnvelopeTypeSessionStatusChanged:
		return true
	// Graph
	case EnvelopeTypeGraphNodeEnd:
		return true
	// TeamRun
	case EnvelopeTypeTeamRunFinished,
		EnvelopeTypeTeamRunFailed:
		return true
	// Spirit — team lifecycle events must not be silently dropped
	case EnvelopeTypeSpiritTeamAssembled,
		EnvelopeTypeSpiritTeamCompleted,
		EnvelopeTypeSpiritTeamFailed,
		EnvelopeTypeSpiritTeamsAllCompleted,
		EnvelopeTypeSpiritOrchestrationStarted:
		return true
	// User
	case EnvelopeTypeUserFeedback:
		return true
	// Activity-First lifecycle — start/done/child_start must not be silently dropped
	case EnvelopeTypeActivityStart,
		EnvelopeTypeActivityDone,
		EnvelopeTypeActivityChildStart:
		return true
	default:
		return false
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
