package contract

import (
	"trpc.group/trpc-go/trpc-agent-go/event/reliability"
)

// EventReliability defines the reliability guarantee for an event type.
// Delegates to the framework reliability.Tier for the actual tier values.
// Stability:stable
type EventReliability = reliability.Tier

const (
	// ReliabilityCritical events use WBPF (Write-Before-Publish-Fanout).
	// They are persisted to WAL before being published to the Bus.
	// Loss of these events causes observable data corruption.
	ReliabilityCritical EventReliability = reliability.Critical

	// ReliabilityImportant events use BlockUpTo + async persistence.
	// They are published immediately but persisted asynchronously.
	ReliabilityImportant EventReliability = reliability.Important

	// ReliabilityInformational events are best-effort with no persistence.
	ReliabilityInformational EventReliability = reliability.Informational
)

// defaultClassifier is the project-wide event reliability classifier.
// It is initialized with AS-EVT-01 classifications at init time.
var defaultClassifier = reliability.NewClassifier[EnvelopeType]()

func init() {
	// Critical (WBPF): ToolResult, Error, RunnerCompletion, Checkpoint
	defaultClassifier.RegisterBulk(reliability.Critical,
		EnvelopeTypeToolResult,
		EnvelopeTypeError,
		EnvelopeTypeRunnerCompletion,
		EnvelopeTypeCheckpoint,
	)

	// Important (BlockUpTo + async): never silently dropped
	defaultClassifier.RegisterBulk(reliability.Important,
		// Runtime
		EnvelopeTypeStateDelta,
		EnvelopeTypeTokenUsage,
		EnvelopeTypeRunStatus,
		// Session
		EnvelopeTypeSessionStatusChanged,
		// Graph
		EnvelopeTypeGraphNodeEnd,
		// TeamRun
		EnvelopeTypeTeamRunFinished,
		EnvelopeTypeTeamRunFailed,
		// Spirit — team lifecycle events must not be silently dropped
		EnvelopeTypeSpiritTeamAssembled,
		EnvelopeTypeSpiritTeamCompleted,
		EnvelopeTypeSpiritTeamFailed,
		EnvelopeTypeSpiritTeamsAllCompleted,
		EnvelopeTypeSpiritOrchestrationStarted,
		// User
		EnvelopeTypeUserFeedback,
		// Activity-First lifecycle — start/done/child_start must not be silently dropped
		EnvelopeTypeActivityStart,
		EnvelopeTypeActivityDone,
		EnvelopeTypeActivityChildStart,
	)
}

// ClassifyEventReliability returns the reliability tier for an event type.
// Classification follows AS-EVT-01:
//   - Critical (WBPF): ToolResult, Error, RunnerCompletion, Checkpoint
//   - Important (BlockUpTo + async): see init() for the full list
//   - Informational (best-effort): everything else
//
// This function is the single source of truth for event reliability classification.
// All consumers (Bus delivery policy, EventWAL, persist handler) must use this
// function instead of maintaining their own critical/important type sets.
//
// Stability:stable
func ClassifyEventReliability(t EnvelopeType) EventReliability {
	return defaultClassifier.Classify(t)
}

// IsCriticalWBPFType returns true if the event type requires WBPF.
func IsCriticalWBPFType(t EnvelopeType) bool {
	return reliability.IsCriticalWBPF(defaultClassifier.Classify(t))
}

// RequiresBlockUpTo returns true if the event type must use BlockUpTo delivery
// (i.e., it is Critical or Important — never silently dropped).
func RequiresBlockUpTo(t EnvelopeType) bool {
	return reliability.RequiresBlockUpTo(defaultClassifier.Classify(t))
}
