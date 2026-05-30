package service_test

import (
	"database/sql"
	"errors"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestToProtoTeam(t *testing.T) {
	tests := []struct {
		name  string
		input biz.Team
		check func(t *testing.T, got *v1.Team)
	}{
		{
			name: "full fields",
			input: biz.Team{
				ID:             "team-1",
				TeamKey:        "my-team",
				DisplayName:    "My Team",
				Status:         "active",
				IsDefault:      true,
				DefinitionJSON: `{"version":2,"mode":"sequential"}`,
				ADKAppName:     "my-team",
				CreatedAt:      "2026-01-01T00:00:00Z",
				UpdatedAt:      "2026-01-02T00:00:00Z",
				DeletedAt:      "",
			},
			check: func(t *testing.T, got *v1.Team) {
				if got.Id != "team-1" {
					t.Errorf("Id = %q, want %q", got.Id, "team-1")
				}
				if got.TeamKey != "my-team" {
					t.Errorf("TeamKey = %q, want %q", got.TeamKey, "my-team")
				}
				if got.DisplayName != "My Team" {
					t.Errorf("DisplayName = %q, want %q", got.DisplayName, "My Team")
				}
				if got.Status != "active" {
					t.Errorf("Status = %q, want %q", got.Status, "active")
				}
				if !got.IsDefault {
					t.Errorf("IsDefault = %v, want true", got.IsDefault)
				}
				if got.DefinitionJson != `{"version":2,"mode":"sequential"}` {
					t.Errorf("DefinitionJson = %q, unexpected", got.DefinitionJson)
				}
				if got.AdkAppName != "my-team" {
					t.Errorf("AdkAppName = %q, want %q", got.AdkAppName, "my-team")
				}
			},
		},
		{
			name:  "zero value team",
			input: biz.Team{},
			check: func(t *testing.T, got *v1.Team) {
				if got.Id != "" {
					t.Errorf("Id = %q, want empty", got.Id)
				}
				if got.IsDefault {
					t.Errorf("IsDefault = true, want false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoTeam(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestToProtoTeamRun(t *testing.T) {
	tests := []struct {
		name  string
		input biz.TeamRun
		check func(t *testing.T, got *v1.TeamRun)
	}{
		{
			name: "full fields",
			input: biz.TeamRun{
				ID:                     "run-1",
				TeamID:                 "team-1",
				SessionID:              "sess-1",
				MessageID:              "msg-1",
				Mode:                   "sequential",
				Status:                 "success",
				InputPreview:           "hello",
				OutputPreview:          "hi there",
				TokenIn:                100,
				TokenOut:               200,
				CostMicroUSD:           5000,
				DurationMS:             3000,
				ErrorMessage:           "",
				TopologyJSON:           "{}",
				GraphExecutionID:       "gexec-1",
				DefinitionSnapshotJSON: "{}",
				TraceID:                "trace-1",
				StartedAt:              "2026-01-01T00:00:00Z",
				FinishedAt:             "2026-01-01T00:00:03Z",
				CreatedAt:              "2026-01-01T00:00:00Z",
				UpdatedAt:              "2026-01-01T00:00:03Z",
			},
			check: func(t *testing.T, got *v1.TeamRun) {
				if got.Id != "run-1" {
					t.Errorf("Id = %q, want %q", got.Id, "run-1")
				}
				if got.TeamId != "team-1" {
					t.Errorf("TeamId = %q, want %q", got.TeamId, "team-1")
				}
				if got.TokenIn != 100 {
					t.Errorf("TokenIn = %d, want %d", got.TokenIn, 100)
				}
				if got.CostMicroUsd != 5000 {
					t.Errorf("CostMicroUsd = %d, want %d", got.CostMicroUsd, 5000)
				}
				if got.DurationMs != 3000 {
					t.Errorf("DurationMs = %d, want %d", got.DurationMs, 3000)
				}
			},
		},
		{
			name:  "zero value",
			input: biz.TeamRun{},
			check: func(t *testing.T, got *v1.TeamRun) {
				if got.Id != "" {
					t.Errorf("Id = %q, want empty", got.Id)
				}
				if got.TokenIn != 0 {
					t.Errorf("TokenIn = %d, want 0", got.TokenIn)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoTeamRun(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestToProtoTeamRunSummary(t *testing.T) {
	tests := []struct {
		name  string
		input biz.TeamRunSummaryData
		check func(t *testing.T, got *v1.TeamRunSummary)
	}{
		{
			name: "with members",
			input: biz.TeamRunSummaryData{
				RunID:         "run-1",
				TeamID:        "team-1",
				SessionID:     "sess-1",
				Mode:          "sequential",
				Status:        "success",
				DurationMS:    5000,
				TokenIn:       100,
				TokenOut:      200,
				CostMicroUSD:  3000,
				MemberCount:   2,
				ToolCallCount: 5,
				OutputPreview: "result",
				ErrorMessage:  "",
				Members: []biz.TeamRunMemberSummaryData{
					{
						AgentID:       "a1",
						AgentKey:      "worker-1",
						AgentName:     "Worker One",
						Role:          "worker",
						SortOrder:     1,
						Status:        "ok",
						TokenIn:       50,
						TokenOut:      100,
						DurationMS:    2000,
						CostMicroUSD:  1500,
						ToolCallCount: 3,
						OutputPreview: "out1",
					},
					{
						AgentID:       "a2",
						AgentKey:      "worker-2",
						AgentName:     "Worker Two",
						Role:          "worker",
						SortOrder:     2,
						Status:        "ok",
						TokenIn:       50,
						TokenOut:      100,
						DurationMS:    3000,
						CostMicroUSD:  1500,
						ToolCallCount: 2,
						OutputPreview: "out2",
					},
				},
			},
			check: func(t *testing.T, got *v1.TeamRunSummary) {
				if got.RunId != "run-1" {
					t.Errorf("RunId = %q, want %q", got.RunId, "run-1")
				}
				if got.MemberCount != 2 {
					t.Errorf("MemberCount = %d, want 2", got.MemberCount)
				}
				if got.ToolCallCount != 5 {
					t.Errorf("ToolCallCount = %d, want 5", got.ToolCallCount)
				}
				if len(got.Members) != 2 {
					t.Fatalf("Members len = %d, want 2", len(got.Members))
				}
				if got.Members[0].AgentId != "a1" {
					t.Errorf("Members[0].AgentId = %q, want %q", got.Members[0].AgentId, "a1")
				}
				if got.Members[0].ToolCallCount != 3 {
					t.Errorf("Members[0].ToolCallCount = %d, want 3", got.Members[0].ToolCallCount)
				}
				if got.Members[1].SortOrder != 2 {
					t.Errorf("Members[1].SortOrder = %d, want 2", got.Members[1].SortOrder)
				}
			},
		},
		{
			name: "no members",
			input: biz.TeamRunSummaryData{
				RunID:  "run-2",
				Status: "pending",
			},
			check: func(t *testing.T, got *v1.TeamRunSummary) {
				if got.RunId != "run-2" {
					t.Errorf("RunId = %q, want %q", got.RunId, "run-2")
				}
				if len(got.Members) != 0 {
					t.Errorf("Members len = %d, want 0", len(got.Members))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoTeamRunSummary(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestToProtoTeamRunStep(t *testing.T) {
	tests := []struct {
		name  string
		input biz.TeamRunStep
		check func(t *testing.T, got *v1.TeamRunStep)
	}{
		{
			name: "full fields",
			input: biz.TeamRunStep{
				ID:            "step-1",
				RunID:         "run-1",
				TeamID:        "team-1",
				AgentID:       "agent-1",
				AgentKey:      "worker-1",
				AgentName:     "Worker One",
				Role:          "worker",
				SortOrder:     1,
				Status:        "ok",
				InputPreview:  "input",
				OutputPreview: "output",
				TokenIn:       50,
				TokenOut:      100,
				CostMicroUSD:  1500,
				DurationMS:    2000,
				ErrorMessage:  "",
				StartedAt:     "2026-01-01T00:00:00Z",
				FinishedAt:    "2026-01-01T00:00:02Z",
				CreatedAt:     "2026-01-01T00:00:00Z",
				ToolCallCount: 3,
			},
			check: func(t *testing.T, got *v1.TeamRunStep) {
				if got.Id != "step-1" {
					t.Errorf("Id = %q, want %q", got.Id, "step-1")
				}
				if got.AgentKey != "worker-1" {
					t.Errorf("AgentKey = %q, want %q", got.AgentKey, "worker-1")
				}
				if got.SortOrder != 1 {
					t.Errorf("SortOrder = %d, want 1", got.SortOrder)
				}
				if got.ToolCallCount != 3 {
					t.Errorf("ToolCallCount = %d, want 3", got.ToolCallCount)
				}
				if got.CostMicroUsd != 1500 {
					t.Errorf("CostMicroUsd = %d, want 1500", got.CostMicroUsd)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoTeamRunStep(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestTeamFromProto(t *testing.T) {
	tests := []struct {
		name  string
		input *v1.Team
		check func(t *testing.T, got biz.Team)
	}{
		{
			name: "full fields",
			input: &v1.Team{
				Id:             "team-1",
				TeamKey:        "my-team",
				DisplayName:    "My Team",
				Status:         "active",
				IsDefault:      true,
				DefinitionJson: `{"version":2}`,
				AdkAppName:     "my-team",
				CreatedAt:      "2026-01-01T00:00:00Z",
				UpdatedAt:      "2026-01-02T00:00:00Z",
				DeletedAt:      "",
			},
			check: func(t *testing.T, got biz.Team) {
				if got.ID != "team-1" {
					t.Errorf("ID = %q, want %q", got.ID, "team-1")
				}
				if got.TeamKey != "my-team" {
					t.Errorf("TeamKey = %q, want %q", got.TeamKey, "my-team")
				}
				if got.DisplayName != "My Team" {
					t.Errorf("DisplayName = %q, want %q", got.DisplayName, "My Team")
				}
				if !got.IsDefault {
					t.Errorf("IsDefault = %v, want true", got.IsDefault)
				}
				if got.DefinitionJSON != `{"version":2}` {
					t.Errorf("DefinitionJSON = %q, unexpected", got.DefinitionJSON)
				}
			},
		},
		{
			name:  "nil input",
			input: nil,
			check: func(t *testing.T, got biz.Team) {
				if got.ID != "" {
					t.Errorf("ID = %q, want empty", got.ID)
				}
				if got.IsDefault {
					t.Errorf("IsDefault = true, want false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.TeamFromProto(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestMapTeamErr(t *testing.T) {
	tests := []struct {
		name       string
		input      error
		wantNil    bool
		wantNotFound bool
	}{
		{
			name:    "nil error",
			input:   nil,
			wantNil: true,
		},
		{
			name:       "sql.ErrNoRows maps to NotFound",
			input:      sql.ErrNoRows,
			wantNotFound: true,
		},
		{
			name:    "other error passes through",
			input:   errors.New("some error"),
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.MapTeamErr(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("got = %v, want nil", got)
				}
				return
			}
			if tt.wantNotFound {
				if got == nil {
					t.Fatal("got nil, want NotFound error")
				}
				var ke *kerrors.Error
				if !errors.As(got, &ke) {
					t.Fatalf("got = %v, want kerrors.Error", got)
				}
				if ke.Reason != "TEAM" {
					t.Errorf("Reason = %q, want %q", ke.Reason, "TEAM")
				}
			}
		})
	}
}
