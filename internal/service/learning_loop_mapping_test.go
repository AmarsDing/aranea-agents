package service

import (
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/learning_loop/v1"
	"aranea-agents/internal/biz"
)

func TestToProtoObservation(t *testing.T) {
	ts := time.Date(2025, 6, 1, 12, 30, 45, 0, time.UTC)

	cases := []struct {
		name string
		in   biz.Observation
		want *v1.Observation
	}{
		{
			name: "full_fields",
			in: biz.Observation{
				ID:         "obs-1",
				AgentID:    "agent-1",
				SessionID:  "sess-1",
				Kind:       biz.ObservationKindToolCall,
				Content:    "called web_search",
				Metadata:   `{"query":"golang testing"}`,
				ObservedAt: ts,
			},
			want: &v1.Observation{
				Id:         "obs-1",
				AgentId:    "agent-1",
				SessionId:  "sess-1",
				Kind:       "tool_call",
				Content:    "called web_search",
				Metadata:   `{"query":"golang testing"}`,
				ObservedAt: ts.Format(time.RFC3339),
			},
		},
		{
			name: "zero_value",
			in:   biz.Observation{},
			want: &v1.Observation{
				Kind:       "",
				ObservedAt: time.Time{}.Format(time.RFC3339),
			},
		},
		{
			name: "kind_feedback",
			in: biz.Observation{
				ID:   "obs-2",
				Kind: biz.ObservationKindFeedback,
			},
			want: &v1.Observation{
				Id:         "obs-2",
				Kind:       "feedback",
				ObservedAt: time.Time{}.Format(time.RFC3339),
			},
		},
		{
			name: "kind_memory_hit",
			in: biz.Observation{
				ID:   "obs-3",
				Kind: biz.ObservationKindMemoryHit,
			},
			want: &v1.Observation{
				Id:         "obs-3",
				Kind:       "memory_hit",
				ObservedAt: time.Time{}.Format(time.RFC3339),
			},
		},
		{
			name: "kind_memory_miss",
			in: biz.Observation{
				ID:   "obs-4",
				Kind: biz.ObservationKindMemoryMiss,
			},
			want: &v1.Observation{
				Id:         "obs-4",
				Kind:       "memory_miss",
				ObservedAt: time.Time{}.Format(time.RFC3339),
			},
		},
		{
			name: "time_format_rfc3339",
			in: biz.Observation{
				ID:         "obs-5",
				ObservedAt: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
			},
			want: &v1.Observation{
				Id:         "obs-5",
				ObservedAt: "2025-12-31T23:59:59Z",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoObservation(tc.in)
			if got.Id != tc.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tc.want.Id)
			}
			if got.AgentId != tc.want.AgentId {
				t.Errorf("AgentId = %q, want %q", got.AgentId, tc.want.AgentId)
			}
			if got.SessionId != tc.want.SessionId {
				t.Errorf("SessionId = %q, want %q", got.SessionId, tc.want.SessionId)
			}
			if got.Kind != tc.want.Kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tc.want.Kind)
			}
			if got.Content != tc.want.Content {
				t.Errorf("Content = %q, want %q", got.Content, tc.want.Content)
			}
			if got.Metadata != tc.want.Metadata {
				t.Errorf("Metadata = %q, want %q", got.Metadata, tc.want.Metadata)
			}
			if got.ObservedAt != tc.want.ObservedAt {
				t.Errorf("ObservedAt = %q, want %q", got.ObservedAt, tc.want.ObservedAt)
			}
		})
	}
}

func TestToProtoPattern(t *testing.T) {
	ts := time.Date(2025, 6, 15, 8, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		in   biz.Pattern
		want *v1.Pattern
	}{
		{
			name: "full_fields",
			in: biz.Pattern{
				ID:          "pat-1",
				AgentID:     "agent-1",
				Kind:        "recurring_tool_call",
				Description: "Agent repeatedly calls web_search for same query",
				Frequency:   15,
				Confidence:  0.92,
				Evidence:    "obs-1,obs-2,obs-3",
				Status:      biz.PatternStatusConfirmed,
				DetectedAt:  ts,
			},
			want: &v1.Pattern{
				Id:          "pat-1",
				AgentId:     "agent-1",
				Kind:        "recurring_tool_call",
				Description: "Agent repeatedly calls web_search for same query",
				Frequency:   15,
				Confidence:  0.92,
				Evidence:    "obs-1,obs-2,obs-3",
				Status:      "confirmed",
				DetectedAt:  ts.Format(time.RFC3339),
			},
		},
		{
			name: "zero_value",
			in:   biz.Pattern{},
			want: &v1.Pattern{
				Status:     "",
				DetectedAt: time.Time{}.Format(time.RFC3339),
			},
		},
		{
			name: "int32_cast_frequency",
			in: biz.Pattern{
				ID:        "pat-2",
				Frequency: 1000,
			},
			want: &v1.Pattern{
				Id:         "pat-2",
				Frequency:  1000,
				DetectedAt: time.Time{}.Format(time.RFC3339),
			},
		},
		{
			name: "status_detected",
			in: biz.Pattern{
				ID:     "pat-3",
				Status: biz.PatternStatusDetected,
			},
			want: &v1.Pattern{
				Id:         "pat-3",
				Status:     "detected",
				DetectedAt: time.Time{}.Format(time.RFC3339),
			},
		},
		{
			name: "status_dismissed",
			in: biz.Pattern{
				ID:     "pat-4",
				Status: biz.PatternStatusDismissed,
			},
			want: &v1.Pattern{
				Id:         "pat-4",
				Status:     "dismissed",
				DetectedAt: time.Time{}.Format(time.RFC3339),
			},
		},
		{
			name: "confidence_precision",
			in: biz.Pattern{
				ID:         "pat-5",
				Confidence: 0.123456789,
			},
			want: &v1.Pattern{
				Id:         "pat-5",
				Confidence: 0.123456789,
				DetectedAt: time.Time{}.Format(time.RFC3339),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoPattern(tc.in)
			if got.Id != tc.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tc.want.Id)
			}
			if got.AgentId != tc.want.AgentId {
				t.Errorf("AgentId = %q, want %q", got.AgentId, tc.want.AgentId)
			}
			if got.Kind != tc.want.Kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tc.want.Kind)
			}
			if got.Description != tc.want.Description {
				t.Errorf("Description = %q, want %q", got.Description, tc.want.Description)
			}
			if got.Frequency != tc.want.Frequency {
				t.Errorf("Frequency = %d, want %d", got.Frequency, tc.want.Frequency)
			}
			if got.Confidence != tc.want.Confidence {
				t.Errorf("Confidence = %v, want %v", got.Confidence, tc.want.Confidence)
			}
			if got.Evidence != tc.want.Evidence {
				t.Errorf("Evidence = %q, want %q", got.Evidence, tc.want.Evidence)
			}
			if got.Status != tc.want.Status {
				t.Errorf("Status = %q, want %q", got.Status, tc.want.Status)
			}
			if got.DetectedAt != tc.want.DetectedAt {
				t.Errorf("DetectedAt = %q, want %q", got.DetectedAt, tc.want.DetectedAt)
			}
		})
	}
}

func TestToProtoProposal(t *testing.T) {
	createdAt := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2025, 6, 2, 14, 30, 0, 0, time.UTC)
	validatedAt := time.Date(2025, 6, 2, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		in   biz.KnowledgeProposal
		want *v1.KnowledgeProposal
	}{
		{
			name: "full_fields_with_validated_at",
			in: biz.KnowledgeProposal{
				ID:          "prop-1",
				AgentID:     "agent-1",
				PatternID:   "pat-1",
				Title:       "Add caching for repeated queries",
				Content:     "Cache web_search results for 5 minutes",
				Kind:        "optimization",
				Status:      biz.ProposalStatusValidated,
				ValidatedAt: &validatedAt,
				ApprovedBy:  "user:42",
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
			},
			want: &v1.KnowledgeProposal{
				Id:          "prop-1",
				AgentId:     "agent-1",
				PatternId:   "pat-1",
				Title:       "Add caching for repeated queries",
				Content:     "Cache web_search results for 5 minutes",
				Kind:        "optimization",
				Status:      "validated",
				ValidatedAt: validatedAt.Format(time.RFC3339),
				ApprovedBy:  "user:42",
				CreatedAt:   createdAt.Format(time.RFC3339),
				UpdatedAt:   updatedAt.Format(time.RFC3339),
			},
		},
		{
			name: "nil_validated_at",
			in: biz.KnowledgeProposal{
				ID:          "prop-2",
				AgentID:     "agent-1",
				PatternID:   "pat-2",
				Title:       "Draft proposal",
				Content:     "Still under review",
				Kind:        "safety",
				Status:      biz.ProposalStatusDraft,
				ValidatedAt: nil,
				ApprovedBy:  "",
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
			},
			want: &v1.KnowledgeProposal{
				Id:          "prop-2",
				AgentId:     "agent-1",
				PatternId:   "pat-2",
				Title:       "Draft proposal",
				Content:     "Still under review",
				Kind:        "safety",
				Status:      "draft",
				ValidatedAt: "",
				ApprovedBy:  "",
				CreatedAt:   createdAt.Format(time.RFC3339),
				UpdatedAt:   updatedAt.Format(time.RFC3339),
			},
		},
		{
			name: "zero_value",
			in:   biz.KnowledgeProposal{},
			want: &v1.KnowledgeProposal{
				CreatedAt: time.Time{}.Format(time.RFC3339),
				UpdatedAt: time.Time{}.Format(time.RFC3339),
			},
		},
		{
			name: "status_approved",
			in: biz.KnowledgeProposal{
				ID:         "prop-3",
				Status:     biz.ProposalStatusApproved,
				ApprovedBy: "system",
				CreatedAt:  createdAt,
				UpdatedAt:  updatedAt,
			},
			want: &v1.KnowledgeProposal{
				Id:         "prop-3",
				Status:     "approved",
				ApprovedBy: "system",
				CreatedAt:  createdAt.Format(time.RFC3339),
				UpdatedAt:  updatedAt.Format(time.RFC3339),
			},
		},
		{
			name: "status_rejected",
			in: biz.KnowledgeProposal{
				ID:        "prop-4",
				Status:    biz.ProposalStatusRejected,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
			want: &v1.KnowledgeProposal{
				Id:        "prop-4",
				Status:    "rejected",
				CreatedAt: createdAt.Format(time.RFC3339),
				UpdatedAt: updatedAt.Format(time.RFC3339),
			},
		},
		{
			name: "status_applied",
			in: biz.KnowledgeProposal{
				ID:        "prop-5",
				Status:    biz.ProposalStatusApplied,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
			want: &v1.KnowledgeProposal{
				Id:        "prop-5",
				Status:    "applied",
				CreatedAt: createdAt.Format(time.RFC3339),
				UpdatedAt: updatedAt.Format(time.RFC3339),
			},
		},
		{
			name: "status_conflict",
			in: biz.KnowledgeProposal{
				ID:        "prop-6",
				Status:    biz.ProposalStatusConflict,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
			want: &v1.KnowledgeProposal{
				Id:        "prop-6",
				Status:    "conflict",
				CreatedAt: createdAt.Format(time.RFC3339),
				UpdatedAt: updatedAt.Format(time.RFC3339),
			},
		},
		{
			name: "status_expired",
			in: biz.KnowledgeProposal{
				ID:        "prop-7",
				Status:    biz.ProposalStatusExpired,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
			want: &v1.KnowledgeProposal{
				Id:        "prop-7",
				Status:    "expired",
				CreatedAt: createdAt.Format(time.RFC3339),
				UpdatedAt: updatedAt.Format(time.RFC3339),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoProposal(tc.in)
			if got.Id != tc.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tc.want.Id)
			}
			if got.AgentId != tc.want.AgentId {
				t.Errorf("AgentId = %q, want %q", got.AgentId, tc.want.AgentId)
			}
			if got.PatternId != tc.want.PatternId {
				t.Errorf("PatternId = %q, want %q", got.PatternId, tc.want.PatternId)
			}
			if got.Title != tc.want.Title {
				t.Errorf("Title = %q, want %q", got.Title, tc.want.Title)
			}
			if got.Content != tc.want.Content {
				t.Errorf("Content = %q, want %q", got.Content, tc.want.Content)
			}
			if got.Kind != tc.want.Kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tc.want.Kind)
			}
			if got.Status != tc.want.Status {
				t.Errorf("Status = %q, want %q", got.Status, tc.want.Status)
			}
			if got.ValidatedAt != tc.want.ValidatedAt {
				t.Errorf("ValidatedAt = %q, want %q", got.ValidatedAt, tc.want.ValidatedAt)
			}
			if got.ApprovedBy != tc.want.ApprovedBy {
				t.Errorf("ApprovedBy = %q, want %q", got.ApprovedBy, tc.want.ApprovedBy)
			}
			if got.CreatedAt != tc.want.CreatedAt {
				t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, tc.want.CreatedAt)
			}
			if got.UpdatedAt != tc.want.UpdatedAt {
				t.Errorf("UpdatedAt = %q, want %q", got.UpdatedAt, tc.want.UpdatedAt)
			}
		})
	}
}
