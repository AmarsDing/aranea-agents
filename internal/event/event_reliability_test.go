package event

import (
	"testing"

	"aranea-agents/internal/event/contract"
)

func TestClassifyEventReliability_Critical(t *testing.T) {
	criticalTypes := []contract.EnvelopeType{
		contract.EnvelopeTypeError,
		contract.EnvelopeTypeCheckpoint,
	}
	for _, typ := range criticalTypes {
		got := ClassifyEventReliability(typ)
		if got != ReliabilityCritical {
			t.Errorf("ClassifyEventReliability(%q) = %v, want ReliabilityCritical", typ, got)
		}
	}
}

func TestClassifyEventReliability_Important(t *testing.T) {
	importantTypes := []contract.EnvelopeType{
		contract.EnvelopeTypeStateDelta,
		contract.EnvelopeTypeTokenUsage,
		contract.EnvelopeTypeRunStatus,
		contract.EnvelopeTypeSessionStatusChanged,
		contract.EnvelopeTypeGraphNodeEnd,
		contract.EnvelopeTypeTeamRunFinished,
		contract.EnvelopeTypeUserFeedback,
	}
	for _, typ := range importantTypes {
		got := ClassifyEventReliability(typ)
		if got != ReliabilityImportant {
			t.Errorf("ClassifyEventReliability(%q) = %v, want ReliabilityImportant", typ, got)
		}
	}
}

func TestClassifyEventReliability_Informational(t *testing.T) {
	infoTypes := []contract.EnvelopeType{
		contract.EnvelopeTypeFlowLog,
		contract.EnvelopeTypeLog,
	}
	for _, typ := range infoTypes {
		got := ClassifyEventReliability(typ)
		if got != ReliabilityInformational {
			t.Errorf("ClassifyEventReliability(%q) = %v, want ReliabilityInformational", typ, got)
		}
	}
}

func TestIsCriticalWBPFType(t *testing.T) {
	tests := []struct {
		typ  contract.EnvelopeType
		want bool
	}{
		{contract.EnvelopeTypeError, true},
		{contract.EnvelopeTypeCheckpoint, true},
		{contract.EnvelopeTypeStateDelta, false},
		{contract.EnvelopeTypeTokenUsage, false},
		{contract.EnvelopeTypeRunStatus, false},
		{contract.EnvelopeTypeLog, false},
		{contract.EnvelopeTypeFlowLog, false},
	}
	for _, tt := range tests {
		got := IsCriticalWBPFType(tt.typ)
		if got != tt.want {
			t.Errorf("IsCriticalWBPFType(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}
