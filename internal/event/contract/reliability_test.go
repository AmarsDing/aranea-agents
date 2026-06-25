package contract

import "testing"

func TestRequiresBlockUpTo(t *testing.T) {
	tests := []struct {
		name string
		typ  EnvelopeType
		want bool
	}{
		// Critical events — WBPF + BlockUpTo
		{"Critical: Error", EnvelopeTypeError, true},
		{"Critical: Checkpoint", EnvelopeTypeCheckpoint, true},
		// Important events — BlockUpTo + async persistence
		{"Important: StateDelta", EnvelopeTypeStateDelta, true},
		{"Important: TokenUsage", EnvelopeTypeTokenUsage, true},
		{"Important: RunStatus", EnvelopeTypeRunStatus, true},
		{"Important: SessionStatusChanged", EnvelopeTypeSessionStatusChanged, true},
		{"Important: GraphNodeEnd", EnvelopeTypeGraphNodeEnd, true},
		{"Important: TeamRunFinished", EnvelopeTypeTeamRunFinished, true},
		{"Important: UserFeedback", EnvelopeTypeUserFeedback, true},
		// Informational events — best-effort, no BlockUpTo
		{"Informational: FlowLog", EnvelopeTypeFlowLog, false},
		{"Informational: Log", EnvelopeTypeLog, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresBlockUpTo(tt.typ); got != tt.want {
				t.Errorf("RequiresBlockUpTo(%v) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestRequiresBlockUpTo_ConsistentWithClassify(t *testing.T) {
	// RequiresBlockUpTo must return true for exactly Critical + Important types.
	allTypes := []EnvelopeType{
		EnvelopeTypeError,
		EnvelopeTypeCheckpoint,
		EnvelopeTypeStateDelta,
		EnvelopeTypeTokenUsage,
		EnvelopeTypeRunStatus,
		EnvelopeTypeSessionStatusChanged,
		EnvelopeTypeGraphNodeEnd,
		EnvelopeTypeTeamRunFinished,
		EnvelopeTypeUserFeedback,
		EnvelopeTypeFlowLog,
		EnvelopeTypeLog,
	}
	for _, typ := range allTypes {
		r := ClassifyEventReliability(typ)
		got := RequiresBlockUpTo(typ)
		want := r == ReliabilityCritical || r == ReliabilityImportant
		if got != want {
			t.Errorf("RequiresBlockUpTo(%v) = %v, but ClassifyEventReliability = %v (want %v)", typ, got, r, want)
		}
	}
}
