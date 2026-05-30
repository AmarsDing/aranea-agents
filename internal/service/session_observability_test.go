package service

import (
	"testing"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
)

func TestToProtoSessionRunRecord(t *testing.T) {
	cases := []struct {
		name string
		run  biz.SessionRun
		want *v1.SessionRunRecord
	}{
		{
			name: "full_fields",
			run: biz.SessionRun{
				ID:             "run-1",
				SessionID:      "sess-1",
				TurnID:         "turn-1",
				RuntimeRunID:   "rr-1",
				Source:         "web",
				Phase:          "interactive",
				SoftBudgetSec:  180,
				HardBudgetSec:  900,
				CheckpointID:   "cp-1",
				WorkflowJobID:  "wj-1",
				AgentID:        "agent-1",
				ErrorMessage:   "something failed",
				StartedAt:      "2025-01-01T00:00:00Z",
				PhaseChangedAt: "2025-01-01T00:01:00Z",
				FinishedAt:     "2025-01-01T00:05:00Z",
				CreatedAt:      "2025-01-01T00:00:00Z",
				UpdatedAt:      "2025-01-01T00:05:00Z",
			},
			want: &v1.SessionRunRecord{
				Id:             "run-1",
				SessionId:      "sess-1",
				TurnId:         "turn-1",
				RuntimeRunId:   "rr-1",
				Source:         "web",
				Phase:          "interactive",
				SoftBudgetSec:  180,
				HardBudgetSec:  900,
				CheckpointId:   "cp-1",
				WorkflowJobId:  "wj-1",
				AgentId:        "agent-1",
				ErrorMessage:   "something failed",
				StartedAt:      "2025-01-01T00:00:00Z",
				PhaseChangedAt: "2025-01-01T00:01:00Z",
				FinishedAt:     "2025-01-01T00:05:00Z",
				CreatedAt:      "2025-01-01T00:00:00Z",
				UpdatedAt:      "2025-01-01T00:05:00Z",
			},
		},
		{
			name: "zero_values",
			run:  biz.SessionRun{},
			want: &v1.SessionRunRecord{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoSessionRunRecord(tc.run)
			if got.Id != tc.want.Id ||
				got.SessionId != tc.want.SessionId ||
				got.TurnId != tc.want.TurnId ||
				got.RuntimeRunId != tc.want.RuntimeRunId ||
				got.Source != tc.want.Source ||
				got.Phase != tc.want.Phase ||
				got.SoftBudgetSec != tc.want.SoftBudgetSec ||
				got.HardBudgetSec != tc.want.HardBudgetSec ||
				got.CheckpointId != tc.want.CheckpointId ||
				got.WorkflowJobId != tc.want.WorkflowJobId ||
				got.AgentId != tc.want.AgentId ||
				got.ErrorMessage != tc.want.ErrorMessage ||
				got.StartedAt != tc.want.StartedAt ||
				got.PhaseChangedAt != tc.want.PhaseChangedAt ||
				got.FinishedAt != tc.want.FinishedAt ||
				got.CreatedAt != tc.want.CreatedAt ||
				got.UpdatedAt != tc.want.UpdatedAt {
				t.Fatalf("got=%+v, want=%+v", got, tc.want)
			}
		})
	}
}

func TestToProtoSessionParticipant(t *testing.T) {
	cases := []struct {
		name string
		row  biz.SessionParticipant
		want *v1.SessionParticipant
	}{
		{
			name: "full_fields",
			row: biz.SessionParticipant{
				ID:               "p-1",
				SessionID:        "sess-1",
				ParticipantType:  "agent",
				ParticipantID:    "agent-1",
				DisplayName:      "GPT-4o",
				RoleInSession:    "primary",
				Status:           "active",
				FirstActiveAt:    "2025-01-01T00:00:00Z",
				LastActiveAt:     "2025-01-01T01:00:00Z",
				MessageCount:     10,
				RunStepCount:     5,
				InputTokens:      1000,
				OutputTokens:     500,
				ContextUsedRatio: 0.75,
				MetadataJSON:     `{"key":"val"}`,
				CreatedAt:        "2025-01-01T00:00:00Z",
				UpdatedAt:        "2025-01-01T01:00:00Z",
			},
			want: &v1.SessionParticipant{
				Id:               "p-1",
				SessionId:        "sess-1",
				ParticipantType:  "agent",
				ParticipantId:    "agent-1",
				DisplayName:      "GPT-4o",
				RoleInSession:    "primary",
				Status:           "active",
				FirstActiveAt:    "2025-01-01T00:00:00Z",
				LastActiveAt:     "2025-01-01T01:00:00Z",
				MessageCount:     10,
				RunStepCount:     5,
				InputTokens:      1000,
				OutputTokens:     500,
				ContextUsedRatio: 0.75,
				MetadataJson:     `{"key":"val"}`,
				CreatedAt:        "2025-01-01T00:00:00Z",
				UpdatedAt:        "2025-01-01T01:00:00Z",
			},
		},
		{
			name: "zero_values",
			row:  biz.SessionParticipant{},
			want: &v1.SessionParticipant{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoSessionParticipant(tc.row)
			if got.Id != tc.want.Id ||
				got.SessionId != tc.want.SessionId ||
				got.ParticipantType != tc.want.ParticipantType ||
				got.ParticipantId != tc.want.ParticipantId ||
				got.DisplayName != tc.want.DisplayName ||
				got.RoleInSession != tc.want.RoleInSession ||
				got.Status != tc.want.Status ||
				got.FirstActiveAt != tc.want.FirstActiveAt ||
				got.LastActiveAt != tc.want.LastActiveAt ||
				got.MessageCount != tc.want.MessageCount ||
				got.RunStepCount != tc.want.RunStepCount ||
				got.InputTokens != tc.want.InputTokens ||
				got.OutputTokens != tc.want.OutputTokens ||
				got.ContextUsedRatio != tc.want.ContextUsedRatio ||
				got.MetadataJson != tc.want.MetadataJson ||
				got.CreatedAt != tc.want.CreatedAt ||
				got.UpdatedAt != tc.want.UpdatedAt {
				t.Fatalf("got=%+v, want=%+v", got, tc.want)
			}
		})
	}
}
