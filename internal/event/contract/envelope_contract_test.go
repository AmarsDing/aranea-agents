package contract

import (
	"testing"
)

// TestEnvelopeContractNoDuplicates verifies that all EnvelopeType constants
// have unique string values — no accidental duplication.
func TestEnvelopeContractNoDuplicates(t *testing.T) {
	allTypes := []EnvelopeType{
		EnvelopeTypeTextDelta,
		EnvelopeTypeTextDone,
		EnvelopeTypeToolCall,
		EnvelopeTypeToolResult,
		EnvelopeTypeStateDelta,
		EnvelopeTypeTransfer,
		EnvelopeTypeRunnerCompletion,
		EnvelopeTypeContextUsage,
		EnvelopeTypeRunStatus,
		EnvelopeTypeError,
		EnvelopeTypeLog,
		EnvelopeTypeFlowLog,
		EnvelopeTypeGraphNodeStart,
		EnvelopeTypeGraphNodeEnd,
		EnvelopeTypeCheckpoint,
		EnvelopeTypeIntentPass,
		EnvelopeTypeMemberMessageStart,
		EnvelopeTypeMemberDelta,
		EnvelopeTypeMemberMessageDone,
		EnvelopeTypeTeamRunStarted,
		EnvelopeTypeTeamRunFinished,
		EnvelopeTypeTeamStepStarted,
		EnvelopeTypeTeamStepFinished,
		EnvelopeTypeTeamRunFailed,
		EnvelopeTypeTeamSummary,
		EnvelopeTypeGraphStep,
		EnvelopeTypeGraphExecutionDone,
		EnvelopeTypeGraphNodeError,
		EnvelopeTypeGraphNodeCustom,
		EnvelopeTypeGraphTaskStatus,
		EnvelopeTypeKnowledgeIngest,
		EnvelopeTypeMCPSessionReconnect,
		EnvelopeTypeMCPHealthAlert,
		EnvelopeTypeAlertNotify,
		EnvelopeTypeOrchestrationAgentStatus,
		EnvelopeTypeUserFeedback,
		EnvelopeTypeSessionStatusChanged,
		EnvelopeTypeSpiritTeamAssembled,
		EnvelopeTypeSpiritTeamCompleted,
		EnvelopeTypeSpiritTeamFailed,
		EnvelopeTypeSpiritTeamCancelled,
		EnvelopeTypeSpiritTeamInterrupted,
		EnvelopeTypeSpiritTeamProgress,
		EnvelopeTypeSpiritTeamsAllCompleted,
		EnvelopeTypeSpiritSynthesisCompleted,
		EnvelopeTypeSpiritPlanCreated,
		EnvelopeTypeSpiritAllocationCreated,
		EnvelopeTypeSpiritOrchestrationStarted,
		EnvelopeTypeSpiritOrchestrationCheckpoint,
		EnvelopeTypeSpiritOrchestrationInterrupted,
		EnvelopeTypePlanningPhaseStart,
		EnvelopeTypePlanningPhaseProgress,
		EnvelopeTypePlanningPhaseDone,
		// Wave 1 batch-registered types (§12.0 preprocessing)
		EnvelopeTypeRunHeartbeat,
		EnvelopeTypeAgentCreated,
		EnvelopeTypeGraphReplanned,
		EnvelopeTypeGraphTopologyEvolved,
		EnvelopeTypeTokenUsage,
		EnvelopeTypeMetricsUpdated,
		EnvelopeTypeButlerOrchestrationStarted,
		EnvelopeTypeButlerOrchestrationCompleted,
		EnvelopeTypeButlerOrchestrationFailed,
		EnvelopeTypeSkillHealthChanged,
		EnvelopeTypeSkillEvolutionProposed,
		EnvelopeTypeOrchestrationEvolutionSuggested,
		EnvelopeTypeOrchestrationCacheHit,
		EnvelopeTypeMonitorAutoHealed,
		EnvelopeTypeMonitorSelfCheckCompleted,
		EnvelopeTypeBorrowApproved,
		EnvelopeTypeBorrowRejected,
		EnvelopeTypeBorrowAutoApproved,
		EnvelopeTypeOrganizationCreated,
		EnvelopeTypeOrganizationUpdated,
		EnvelopeTypeOrganizationDeleted,
		EnvelopeTypeActivityStart,
		EnvelopeTypeActivityDelta,
		EnvelopeTypeActivityDone,
		EnvelopeTypeActivityChildStart,
		EnvelopeTypeExecutionProgress,
	}

	seen := make(map[string]string, len(allTypes))
	for _, et := range allTypes {
		v := string(et)
		if prev, dup := seen[v]; dup {
			t.Errorf("duplicate EnvelopeType value %q: already registered by %s", v, prev)
		}
		seen[v] = string(et)
	}

	expected := 79 // total number of EnvelopeType constants
	if len(allTypes) != expected {
		t.Errorf("expected %d EnvelopeType constants, got %d — update this test when adding new types", expected, len(allTypes))
	}
}

// TestReliabilityClassification verifies that AS-EVT-01 reliability tiers
// declared in envelope.go comments match the actual classification returned
// by ClassifyEventReliability. Prevents silent drift between documentation
// and runtime behavior (see Wave 1 review finding B1).
func TestReliabilityClassification(t *testing.T) {
	t.Parallel()

	criticalCases := []EnvelopeType{
		EnvelopeTypeToolResult,
		EnvelopeTypeError,
		EnvelopeTypeRunnerCompletion,
		EnvelopeTypeCheckpoint,
	}
	for _, et := range criticalCases {
		if got := ClassifyEventReliability(et); got != ReliabilityCritical {
			t.Errorf("ClassifyEventReliability(%s) = %v, want Critical", et, got)
		}
		if !IsCriticalWBPFType(et) {
			t.Errorf("IsCriticalWBPFType(%s) = false, want true", et)
		}
	}

	importantCases := []EnvelopeType{
		// Runtime
		EnvelopeTypeStateDelta,
		EnvelopeTypeTokenUsage,
		EnvelopeTypeRunStatus,
		// Session
		EnvelopeTypeSessionStatusChanged,
		// Graph — Wave 1 additions must be Important (topology drift prevention)
		EnvelopeTypeGraphNodeEnd,
		EnvelopeTypeGraphReplanned,
		EnvelopeTypeGraphTopologyEvolved,
		// TeamRun
		EnvelopeTypeTeamRunFinished,
		EnvelopeTypeTeamRunFailed,
		// Spirit
		EnvelopeTypeSpiritTeamAssembled,
		EnvelopeTypeSpiritTeamCompleted,
		EnvelopeTypeSpiritTeamFailed,
		EnvelopeTypeSpiritTeamsAllCompleted,
		EnvelopeTypeSpiritOrchestrationStarted,
		// User
		EnvelopeTypeUserFeedback,
		// Activity-First
		EnvelopeTypeActivityStart,
		EnvelopeTypeActivityDone,
		EnvelopeTypeActivityChildStart,
	}
	for _, et := range importantCases {
		if got := ClassifyEventReliability(et); got != ReliabilityImportant {
			t.Errorf("ClassifyEventReliability(%s) = %v, want Important", et, got)
		}
		if !RequiresBlockUpTo(et) {
			t.Errorf("RequiresBlockUpTo(%s) = false, want true", et)
		}
	}

	// Wave 1 Informational types — best-effort, no persistence, may drop under backpressure
	informationalCases := []EnvelopeType{
		EnvelopeTypeRunHeartbeat, // loss only degrades progress visibility
		EnvelopeTypeAgentCreated, // Agent already persisted to DB; event drives UI only
		EnvelopeTypeTextDelta,
		EnvelopeTypeLog,
		EnvelopeTypeFlowLog,
	}
	for _, et := range informationalCases {
		if got := ClassifyEventReliability(et); got != ReliabilityInformational {
			t.Errorf("ClassifyEventReliability(%s) = %v, want Informational", et, got)
		}
		if IsCriticalWBPFType(et) {
			t.Errorf("IsCriticalWBPFType(%s) = true, want false", et)
		}
		if RequiresBlockUpTo(et) {
			t.Errorf("RequiresBlockUpTo(%s) = true, want false", et)
		}
	}
}
