package service_test

import (
	"database/sql"
	"errors"
	"testing"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/apierror"
)

func TestToProtoSession(t *testing.T) {
	tests := []struct {
		name  string
		input biz.Session
		check func(t *testing.T, got *v1.Session)
	}{
		{
			name: "full fields",
			input: biz.Session{
				ID:                         "sess-1",
				WorkspaceID:                "ws-1",
				UserID:                     "user-1",
				OwnerType:                  "agent",
				AgentID:                    "agent-1",
				TeamID:                     "",
				Title:                      "Test Session",
				Summary:                    "A test",
				TagsJSON:                   "[]",
				DialogMode:                 "single",
				DefaultProvider:            "openai",
				DefaultModel:               "gpt-4",
				DefaultContextWindowTokens: 8192,
				LastProvider:               "openai",
				LastModel:                  "gpt-4",
				LastContextWindowTokens:    8192,
				Status:                     "active",
				Visibility:                 "private",
				MessageCount:               10,
				RunCount:                   5,
				ModelCallCount:             5,
				ToolCallCount:              3,
				SkillCallCount:             2,
				MCPCallCount:               1,
				InputTokens:                1000,
				OutputTokens:               500,
				TotalTokens:                1500,
				TotalCostMicroUSD:          2500,
				AvgLatencyMs:               1.5,
				ErrorCount:                 0,
				ContextUsedTokens:          1200,
				ContextUsedRatio:           0.75,
				MaxContextUsedRatio:        0.9,
				ContextStatus:              "ok",
				FirstMessageAt:             "2026-01-01T00:00:00Z",
				LastMessageAt:              "2026-01-02T00:00:00Z",
				LastRunAt:                  "2026-01-02T00:00:00Z",
				CreatedAt:                  "2026-01-01T00:00:00Z",
				UpdatedAt:                  "2026-01-02T00:00:00Z",
				ArchivedAt:                 "",
				DeletedAt:                  "",
				PinnedAt:                   "",
				RunnerSnapshotJSON:         "{}",
				MetadataJSON:               "{}",
				StateJSON:                  "{}",
			},
			check: func(t *testing.T, got *v1.Session) {
				if got.Id != "sess-1" {
					t.Errorf("Id = %q, want %q", got.Id, "sess-1")
				}
				if got.OwnerType != "agent" {
					t.Errorf("OwnerType = %q, want %q", got.OwnerType, "agent")
				}
				if got.AgentId != "agent-1" {
					t.Errorf("AgentId = %q, want %q", got.AgentId, "agent-1")
				}
				if got.Title != "Test Session" {
					t.Errorf("Title = %q, want %q", got.Title, "Test Session")
				}
				if got.MessageCount != 10 {
					t.Errorf("MessageCount = %d, want 10", got.MessageCount)
				}
				if got.TotalCostMicroUsd != 2500 {
					t.Errorf("TotalCostMicroUsd = %d, want 2500", got.TotalCostMicroUsd)
				}
				if got.AvgLatencyMs != 1.5 {
					t.Errorf("AvgLatencyMs = %f, want 1.5", got.AvgLatencyMs)
				}
				if got.ContextUsedRatio != 0.75 {
					t.Errorf("ContextUsedRatio = %f, want 0.75", got.ContextUsedRatio)
				}
				if got.DefaultContextWindowTokens != 8192 {
					t.Errorf("DefaultContextWindowTokens = %d, want 8192", got.DefaultContextWindowTokens)
				}
			},
		},
		{
			name:  "zero value",
			input: biz.Session{},
			check: func(t *testing.T, got *v1.Session) {
				if got.Id != "" {
					t.Errorf("Id = %q, want empty", got.Id)
				}
				if got.MessageCount != 0 {
					t.Errorf("MessageCount = %d, want 0", got.MessageCount)
				}
				if got.AvgLatencyMs != 0 {
					t.Errorf("AvgLatencyMs = %f, want 0", got.AvgLatencyMs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoSession(tt.input, nil)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestToProtoTimeline(t *testing.T) {
	tests := []struct {
		name  string
		input biz.SessionTimeline
		check func(t *testing.T, got *v1.SessionTimeline)
	}{
		{
			name: "with items",
			input: biz.SessionTimeline{
				SessionID: "sess-1",
				Items: []biz.SessionTimelineItem{
					{
						ID:              "item-1",
						Kind:            "message",
						Side:            "left",
						Title:           "User",
						Subtitle:        "user",
						ActorID:         "",
						ActorName:       "User",
						Status:          "ok",
						OccurredAt:      "2026-01-01T00:00:00Z",
						DurationMS:      0,
						ContentMarkdown: "hello",
						Preview:         "hello",
						DetailJSON:      "",
						Tags:            []string{"User"},
					},
				},
				Summary: biz.SessionTimelineSummary{
					Total:        1,
					MessageCount: 1,
					ToolCount:    0,
					SkillCount:   0,
					MCPCount:     0,
				},
			},
			check: func(t *testing.T, got *v1.SessionTimeline) {
				if got.SessionId != "sess-1" {
					t.Errorf("SessionId = %q, want %q", got.SessionId, "sess-1")
				}
				if len(got.Items) != 1 {
					t.Fatalf("Items len = %d, want 1", len(got.Items))
				}
				if got.Items[0].Id != "item-1" {
					t.Errorf("Items[0].Id = %q, want %q", got.Items[0].Id, "item-1")
				}
				if got.Summary == nil {
					t.Fatal("Summary = nil, want non-nil")
				}
				if got.Summary.Total != 1 {
					t.Errorf("Summary.Total = %d, want 1", got.Summary.Total)
				}
			},
		},
		{
			name: "empty items",
			input: biz.SessionTimeline{
				SessionID: "sess-2",
				Items:     nil,
				Summary:   biz.SessionTimelineSummary{},
			},
			check: func(t *testing.T, got *v1.SessionTimeline) {
				if len(got.Items) != 0 {
					t.Errorf("Items len = %d, want 0", len(got.Items))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoTimeline(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestToProtoTimelineItem(t *testing.T) {
	tests := []struct {
		name  string
		input biz.SessionTimelineItem
		check func(t *testing.T, got *v1.SessionTimelineItem)
	}{
		{
			name: "with tags",
			input: biz.SessionTimelineItem{
				ID:              "ti-1",
				Kind:            "tool",
				Side:            "right",
				Title:           "Search",
				Subtitle:        "web_search",
				ActorID:         "agent-1",
				ActorName:       "Agent",
				Status:          "ok",
				OccurredAt:      "2026-01-01T00:00:00Z",
				DurationMS:      500,
				ContentMarkdown: "",
				Preview:         "search result",
				DetailJSON:      "{}",
				Tags:            []string{"Tool", "Web"},
			},
			check: func(t *testing.T, got *v1.SessionTimelineItem) {
				if got.Id != "ti-1" {
					t.Errorf("Id = %q, want %q", got.Id, "ti-1")
				}
				if got.Kind != "tool" {
					t.Errorf("Kind = %q, want %q", got.Kind, "tool")
				}
				if got.DurationMs != 500 {
					t.Errorf("DurationMs = %d, want 500", got.DurationMs)
				}
				if len(got.Tags) != 2 {
					t.Fatalf("Tags len = %d, want 2", len(got.Tags))
				}
				if got.Tags[0] != "Tool" {
					t.Errorf("Tags[0] = %q, want %q", got.Tags[0], "Tool")
				}
			},
		},
		{
			name: "nil tags becomes empty slice",
			input: biz.SessionTimelineItem{
				ID:   "ti-2",
				Tags: nil,
			},
			check: func(t *testing.T, got *v1.SessionTimelineItem) {
				if got.Tags == nil {
					t.Error("Tags = nil, want empty slice")
				}
				if len(got.Tags) != 0 {
					t.Errorf("Tags len = %d, want 0", len(got.Tags))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoTimelineItem(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestToProtoChatMessageRow(t *testing.T) {
	tests := []struct {
		name  string
		input biz.ChatMessage
		check func(t *testing.T, got *v1.ChatMessageRow)
	}{
		{
			name: "full fields",
			input: biz.ChatMessage{
				ID:               "msg-1",
				SessionID:        "sess-1",
				ParentMessageID:  "msg-0",
				TurnID:           "turn-1",
				TurnNumber:       1,
				SeqInTurn:        0,
				Role:             "user",
				ContentMarkdown:  "hello",
				ModelName:        "",
				TokenIn:          10,
				TokenOut:         0,
				LatencyMS:        0,
				Status:           "ok",
				AttachmentsCount: 0,
				OptionsJSON:      "{}",
				ErrorMessage:     "",
				CreatedAt:        "2026-01-01T00:00:00Z",
			},
			check: func(t *testing.T, got *v1.ChatMessageRow) {
				if got.Id != "msg-1" {
					t.Errorf("Id = %q, want %q", got.Id, "msg-1")
				}
				if got.SessionId != "sess-1" {
					t.Errorf("SessionId = %q, want %q", got.SessionId, "sess-1")
				}
				if got.Role != "user" {
					t.Errorf("Role = %q, want %q", got.Role, "user")
				}
				if got.TurnNumber != 1 {
					t.Errorf("TurnNumber = %d, want 1", got.TurnNumber)
				}
				if got.TokenIn != 10 {
					t.Errorf("TokenIn = %d, want 10", got.TokenIn)
				}
			},
		},
		{
			name:  "zero value",
			input: biz.ChatMessage{},
			check: func(t *testing.T, got *v1.ChatMessageRow) {
				if got.Id != "" {
					t.Errorf("Id = %q, want empty", got.Id)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoChatMessageRow(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestToProtoSessionTurn(t *testing.T) {
	tests := []struct {
		name  string
		input biz.SessionTurn
		check func(t *testing.T, got *v1.SessionTurn)
	}{
		{
			name: "full fields",
			input: biz.SessionTurn{
				ID:                  "turn-1",
				SessionID:           "sess-1",
				RunID:               "run-1",
				TurnNumber:          3,
				UserMessageID:       "msg-1",
				AssistantMessageID:  "msg-2",
				OwnerType:           "agent",
				AgentID:             "agent-1",
				TeamID:              "",
				Status:              "completed",
				StartedAt:           "2026-01-01T00:00:00Z",
				EndedAt:             "2026-01-01T00:00:05Z",
				DurationMs:          5000,
				FirstTokenMs:        200,
				ModelCallCount:      1,
				ToolCallCount:       2,
				SkillCallCount:      1,
				MCPCallCount:        0,
				InputTokens:         100,
				OutputTokens:        200,
				TotalTokens:         300,
				TotalCostMicroUSD:   500,
				FinalProvider:       "openai",
				FinalModel:          "gpt-4",
				FinalContentPreview: "hi there",
				ErrorCode:           "",
				ErrorMessage:        "",
				MetadataJSON:        "{}",
				CreatedAt:           "2026-01-01T00:00:00Z",
				UpdatedAt:           "2026-01-01T00:00:05Z",
			},
			check: func(t *testing.T, got *v1.SessionTurn) {
				if got.Id != "turn-1" {
					t.Errorf("Id = %q, want %q", got.Id, "turn-1")
				}
				if got.TurnNumber != 3 {
					t.Errorf("TurnNumber = %d, want 3", got.TurnNumber)
				}
				if got.DurationMs != 5000 {
					t.Errorf("DurationMs = %d, want 5000", got.DurationMs)
				}
				if got.FirstTokenMs != 200 {
					t.Errorf("FirstTokenMs = %d, want 200", got.FirstTokenMs)
				}
				if got.TotalCostMicroUsd != 500 {
					t.Errorf("TotalCostMicroUsd = %d, want 500", got.TotalCostMicroUsd)
				}
				if got.FinalProvider != "openai" {
					t.Errorf("FinalProvider = %q, want %q", got.FinalProvider, "openai")
				}
			},
		},
		{
			name:  "zero value",
			input: biz.SessionTurn{},
			check: func(t *testing.T, got *v1.SessionTurn) {
				if got.Id != "" {
					t.Errorf("Id = %q, want empty", got.Id)
				}
				if got.TurnNumber != 0 {
					t.Errorf("TurnNumber = %d, want 0", got.TurnNumber)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoSessionTurn(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestMapSessionErr(t *testing.T) {
	tests := []struct {
		name         string
		input        error
		wantNil      bool
		wantNotFound bool
	}{
		{
			name:    "nil error",
			input:   nil,
			wantNil: true,
		},
		{
			name:         "sql.ErrNoRows maps to NotFound",
			input:        sql.ErrNoRows,
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
			got := service.MapSessionErr(tt.input)
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
				ae, ok := apierror.From(got)
				if !ok {
					t.Fatalf("got = %v, want apierror.Error", got)
				}
				if ae.Domain != "SESSION" {
					t.Errorf("Domain = %q, want %q", ae.Domain, "SESSION")
				}
			}
		})
	}
}

func TestSearchQueryFromProto(t *testing.T) {
	tests := []struct {
		name  string
		input *v1.SearchSessionsRequest
		check func(t *testing.T, got biz.SessionSearchQuery)
	}{
		{
			name: "full fields",
			input: &v1.SearchSessionsRequest{
				OwnerType:     "agent",
				AgentId:       "agent-1",
				TeamId:        "team-1",
				Status:        "active",
				ContextStatus: "ok",
				Keyword:       "test",
				UserId:        "user-1",
				Limit:         20,
				Offset:        0,
				Page:          1,
				PageSize:      20,
				SortBy:        "created_at",
				SortOrder:     "desc",
			},
			check: func(t *testing.T, got biz.SessionSearchQuery) {
				if got.OwnerType != "agent" {
					t.Errorf("OwnerType = %q, want %q", got.OwnerType, "agent")
				}
				if got.AgentID != "agent-1" {
					t.Errorf("AgentID = %q, want %q", got.AgentID, "agent-1")
				}
				if got.Keyword != "test" {
					t.Errorf("Keyword = %q, want %q", got.Keyword, "test")
				}
				if got.Limit != 20 {
					t.Errorf("Limit = %d, want 20", got.Limit)
				}
				if got.SortBy != "created_at" {
					t.Errorf("SortBy = %q, want %q", got.SortBy, "created_at")
				}
			},
		},
		{
			name:  "nil input",
			input: nil,
			check: func(t *testing.T, got biz.SessionSearchQuery) {
				if got.OwnerType != "" {
					t.Errorf("OwnerType = %q, want empty", got.OwnerType)
				}
				if got.Limit != 0 {
					t.Errorf("Limit = %d, want 0", got.Limit)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.SearchQueryFromProto(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
