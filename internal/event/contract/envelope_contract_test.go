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
		EnvelopeTypeSpiritTeamInterrupted,
		EnvelopeTypeSpiritTeamProgress,
		EnvelopeTypeSpiritTeamsAllCompleted,
		EnvelopeTypeSpiritSynthesisCompleted,
		EnvelopeTypeSpiritPlanCreated,
		EnvelopeTypeSpiritAllocationCreated,
		EnvelopeTypeSpiritOrchestrationStarted,
		EnvelopeTypeSpiritOrchestrationCheckpoint,
		EnvelopeTypeSpiritOrchestrationInterrupted,
		EnvelopeTypeTokenUsage,
		EnvelopeTypeMetricsUpdated,
		EnvelopeTypeButlerOrchestrationStarted,
		EnvelopeTypeButlerOrchestrationCompleted,
		EnvelopeTypeButlerOrchestrationFailed,
		EnvelopeTypeSkillHealthChanged,
		EnvelopeTypeSkillEvolutionProposed,
		EnvelopeTypeMonitorAutoHealed,
		EnvelopeTypeMonitorSelfCheckCompleted,
	}

	seen := make(map[string]string, len(allTypes))
	for _, et := range allTypes {
		v := string(et)
		if prev, dup := seen[v]; dup {
			t.Errorf("duplicate EnvelopeType value %q: already registered by %s", v, prev)
		}
		seen[v] = string(et)
	}

	expected := 58 // total number of EnvelopeType constants
	if len(allTypes) != expected {
		t.Errorf("expected %d EnvelopeType constants, got %d — update this test when adding new types", expected, len(allTypes))
	}
}
