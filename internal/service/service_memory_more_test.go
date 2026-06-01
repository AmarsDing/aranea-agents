package service_test

import (
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestBizDeadLetterEntryToProto(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	failedAt := time.Date(2025, 6, 1, 12, 5, 0, 0, time.UTC)

	tests := []struct {
		name string
		e    biz.MemoryDeadLetterEntry
		want *v1.MemoryDeadLetterEntry
	}{
		{
			name: "full_entry",
			e: biz.MemoryDeadLetterEntry{
				ID:         42,
				EnqueuedAt: now,
				FailedAt:   failedAt,
				SessionID:  "sess-abc",
				AppName:    "memory-worker",
				DropReason: "llm_timeout",
				Priority:   3,
				Attempts:   2,
				State:      "pending",
				LastError:  "context deadline exceeded",
			},
			want: &v1.MemoryDeadLetterEntry{
				Id:         42,
				EnqueuedAt: now.Format(time.RFC3339),
				FailedAt:   failedAt.Format(time.RFC3339),
				SessionId:  "sess-abc",
				AppName:    "memory-worker",
				DropReason: "llm_timeout",
				Priority:   3,
				Attempts:   2,
				State:      "pending",
				LastError:  "context deadline exceeded",
			},
		},
		{
			name: "zero_entry",
			e:    biz.MemoryDeadLetterEntry{},
			want: &v1.MemoryDeadLetterEntry{
				EnqueuedAt: time.Time{}.Format(time.RFC3339),
				FailedAt:   time.Time{}.Format(time.RFC3339),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.BizDeadLetterEntryToProto(tt.e)
			if got.Id != tt.want.Id {
				t.Errorf("Id = %d, want %d", got.Id, tt.want.Id)
			}
			if got.EnqueuedAt != tt.want.EnqueuedAt {
				t.Errorf("EnqueuedAt = %q, want %q", got.EnqueuedAt, tt.want.EnqueuedAt)
			}
			if got.FailedAt != tt.want.FailedAt {
				t.Errorf("FailedAt = %q, want %q", got.FailedAt, tt.want.FailedAt)
			}
			if got.SessionId != tt.want.SessionId {
				t.Errorf("SessionId = %q, want %q", got.SessionId, tt.want.SessionId)
			}
			if got.AppName != tt.want.AppName {
				t.Errorf("AppName = %q, want %q", got.AppName, tt.want.AppName)
			}
			if got.DropReason != tt.want.DropReason {
				t.Errorf("DropReason = %q, want %q", got.DropReason, tt.want.DropReason)
			}
			if got.Priority != tt.want.Priority {
				t.Errorf("Priority = %d, want %d", got.Priority, tt.want.Priority)
			}
			if got.Attempts != tt.want.Attempts {
				t.Errorf("Attempts = %d, want %d", got.Attempts, tt.want.Attempts)
			}
			if got.State != tt.want.State {
				t.Errorf("State = %q, want %q", got.State, tt.want.State)
			}
			if got.LastError != tt.want.LastError {
				t.Errorf("LastError = %q, want %q", got.LastError, tt.want.LastError)
			}
		})
	}
}

func TestPbRecallHit(t *testing.T) {
	tests := []struct {
		name string
		row  biz.RecallDebugRow
	}{
		{
			name: "full_row",
			row: biz.RecallDebugRow{
				Layer:     "L2",
				ID:        "ep-123",
				Title:     "Meeting Notes",
				Summary:   "Discussed Q3 targets",
				Statement: "The team agreed on a 20% growth target",
				Scores: biz.RecallScoreBreakdown{
					Keyword:      0.8,
					Vector:       0.9,
					Importance:   0.7,
					Recency:      0.6,
					CrossEncoder: 0.85,
					Total:        0.78,
				},
			},
		},
		{
			name: "minimal_row",
			row: biz.RecallDebugRow{
				Layer: "L3",
				ID:    "fact-456",
			},
		},
		{
			name: "zero_scores",
			row: biz.RecallDebugRow{
				Layer:  "L2",
				ID:     "ep-789",
				Scores: biz.RecallScoreBreakdown{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.PbRecallHit(tt.row)
			if got.Layer != tt.row.Layer {
				t.Errorf("Layer = %q, want %q", got.Layer, tt.row.Layer)
			}
			if got.Id != tt.row.ID {
				t.Errorf("Id = %q, want %q", got.Id, tt.row.ID)
			}
			if got.Title != tt.row.Title {
				t.Errorf("Title = %q, want %q", got.Title, tt.row.Title)
			}
			if got.Summary != tt.row.Summary {
				t.Errorf("Summary = %q, want %q", got.Summary, tt.row.Summary)
			}
			if got.Statement != tt.row.Statement {
				t.Errorf("Statement = %q, want %q", got.Statement, tt.row.Statement)
			}
			if got.Scores == nil {
				t.Fatal("Scores is nil")
			}
			if got.Scores.Keyword != tt.row.Scores.Keyword {
				t.Errorf("Scores.Keyword = %v, want %v", got.Scores.Keyword, tt.row.Scores.Keyword)
			}
			if got.Scores.Vector != tt.row.Scores.Vector {
				t.Errorf("Scores.Vector = %v, want %v", got.Scores.Vector, tt.row.Scores.Vector)
			}
			if got.Scores.Importance != tt.row.Scores.Importance {
				t.Errorf("Scores.Importance = %v, want %v", got.Scores.Importance, tt.row.Scores.Importance)
			}
			if got.Scores.Recency != tt.row.Scores.Recency {
				t.Errorf("Scores.Recency = %v, want %v", got.Scores.Recency, tt.row.Scores.Recency)
			}
			if got.Scores.CrossEncoder != tt.row.Scores.CrossEncoder {
				t.Errorf("Scores.CrossEncoder = %v, want %v", got.Scores.CrossEncoder, tt.row.Scores.CrossEncoder)
			}
			if got.Scores.Total != tt.row.Scores.Total {
				t.Errorf("Scores.Total = %v, want %v", got.Scores.Total, tt.row.Scores.Total)
			}
		})
	}
}
