package service

import (
	"testing"

	v1 "aranea-agents/api/kratos/memory/v1"
)

func TestPbL0AssemblySnapshot(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		check   func(t *testing.T, got *v1.L0AssemblySnapshot)
	}{
		{
			name:    "invalid_json",
			raw:     `{bad`,
			wantErr: true,
		},
		{
			name: "empty_object",
			raw:  `{}`,
			check: func(t *testing.T, got *v1.L0AssemblySnapshot) {
				if got.Id != "" {
					t.Errorf("Id=%q want empty", got.Id)
				}
				if got.SessionId != "" {
					t.Errorf("SessionId=%q want empty", got.SessionId)
				}
				if got.ContextWindowTokens != 0 {
					t.Errorf("ContextWindowTokens=%d want 0", got.ContextWindowTokens)
				}
				if got.UsedRatio != 0 {
					t.Errorf("UsedRatio=%f want 0", got.UsedRatio)
				}
			},
		},
		{
			name: "full_fields",
			raw: `{
				"id": "snap-1",
				"session_id": "sess-1",
				"run_id": "run-1",
				"turn_id": "turn-1",
				"span_id": "span-1",
				"agent_id": "agent-1",
				"team_id": "team-1",
				"provider": "openai",
				"model": "gpt-4o",
				"context_window_tokens": 128000,
				"budget_tokens": 64000,
				"recent_window_turns": 10,
				"recent_window_tokens": 32000,
				"summary_token_estimate": 500,
				"l1_field_count": 3,
				"l1_token_estimate": 200,
				"l3_chunk_count": 5,
				"l3_token_estimate": 800,
				"l4_path_count": 2,
				"l4_token_estimate": 100,
				"prompt_token_estimate": 15000,
				"prompt_token_actual": 15200,
				"used_ratio": 0.75,
				"truncate_strategy": "sliding",
				"truncated_message_count": 4,
				"summarized_turn_from": 1,
				"summarized_turn_to": 5,
				"segments_json": "[{\"key\":\"val\"}]",
				"warning_codes_json": "[\"w1\"]",
				"metadata_json": "{}",
				"created_at": "2025-01-01T00:00:00Z"
			}`,
			check: func(t *testing.T, got *v1.L0AssemblySnapshot) {
				if got.Id != "snap-1" {
					t.Errorf("Id=%q want snap-1", got.Id)
				}
				if got.SessionId != "sess-1" {
					t.Errorf("SessionId=%q want sess-1", got.SessionId)
				}
				if got.RunId != "run-1" {
					t.Errorf("RunId=%q want run-1", got.RunId)
				}
				if got.AgentId != "agent-1" {
					t.Errorf("AgentId=%q want agent-1", got.AgentId)
				}
				if got.Provider != "openai" {
					t.Errorf("Provider=%q want openai", got.Provider)
				}
				if got.Model != "gpt-4o" {
					t.Errorf("Model=%q want gpt-4o", got.Model)
				}
				if got.ContextWindowTokens != 128000 {
					t.Errorf("ContextWindowTokens=%d want 128000", got.ContextWindowTokens)
				}
				if got.BudgetTokens != 64000 {
					t.Errorf("BudgetTokens=%d want 64000", got.BudgetTokens)
				}
				if got.UsedRatio != 0.75 {
					t.Errorf("UsedRatio=%f want 0.75", got.UsedRatio)
				}
				if got.TruncateStrategy != "sliding" {
					t.Errorf("TruncateStrategy=%q want sliding", got.TruncateStrategy)
				}
				if got.TruncatedMessageCount != 4 {
					t.Errorf("TruncatedMessageCount=%d want 4", got.TruncatedMessageCount)
				}
				if got.CreatedAt != "2025-01-01T00:00:00Z" {
					t.Errorf("CreatedAt=%q want 2025-01-01T00:00:00Z", got.CreatedAt)
				}
			},
		},
		{
			name: "partial_fields",
			raw:  `{"id": "snap-2", "provider": "anthropic"}`,
			check: func(t *testing.T, got *v1.L0AssemblySnapshot) {
				if got.Id != "snap-2" {
					t.Errorf("Id=%q want snap-2", got.Id)
				}
				if got.Provider != "anthropic" {
					t.Errorf("Provider=%q want anthropic", got.Provider)
				}
				if got.SessionId != "" {
					t.Errorf("SessionId=%q want empty", got.SessionId)
				}
				if got.ContextWindowTokens != 0 {
					t.Errorf("ContextWindowTokens=%d want 0", got.ContextWindowTokens)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pbL0AssemblySnapshot([]byte(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Errorf("pbL0AssemblySnapshot() error=%v wantErr=%v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestPbL1Task(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		check func(t *testing.T, got *v1.L1Task)
	}{
		{
			name: "invalid_json",
			raw:  `{broken`,
			check: func(t *testing.T, got *v1.L1Task) {
				if got != nil {
					t.Errorf("got=%v want nil", got)
				}
			},
		},
		{
			name: "empty_object",
			raw:  `{}`,
			check: func(t *testing.T, got *v1.L1Task) {
				if got == nil {
					t.Fatal("got nil want non-nil")
				}
				if got.Id != "" {
					t.Errorf("Id=%q want empty", got.Id)
				}
				if got.Status != "" {
					t.Errorf("Status=%q want empty", got.Status)
				}
			},
		},
		{
			name: "full_fields",
			raw: `{
				"id": "task-1",
				"session_id": "sess-1",
				"run_id": "run-1",
				"team_id": "team-1",
				"agent_id": "agent-1",
				"task_key": "research",
				"task_title": "Research Task",
				"task_goal": "Find info",
				"status": "pending",
				"schema_version": 2,
				"budget_tokens": 10000,
				"used_tokens": 3000,
				"parent_task_id": "parent-1",
				"shared_with_json": "[\"a\",\"b\"]",
				"started_at": "2025-01-01T00:00:00Z",
				"ended_at": "2025-01-01T01:00:00Z",
				"archived_at": "",
				"metadata_json": "{}",
				"created_at": "2025-01-01T00:00:00Z",
				"updated_at": "2025-01-01T01:00:00Z"
			}`,
			check: func(t *testing.T, got *v1.L1Task) {
				if got.Id != "task-1" {
					t.Errorf("Id=%q want task-1", got.Id)
				}
				if got.SessionId != "sess-1" {
					t.Errorf("SessionId=%q want sess-1", got.SessionId)
				}
				if got.TaskKey != "research" {
					t.Errorf("TaskKey=%q want research", got.TaskKey)
				}
				if got.Status != "pending" {
					t.Errorf("Status=%q want pending", got.Status)
				}
				if got.BudgetTokens != 10000 {
					t.Errorf("BudgetTokens=%d want 10000", got.BudgetTokens)
				}
				if got.UsedTokens != 3000 {
					t.Errorf("UsedTokens=%d want 3000", got.UsedTokens)
				}
				if got.ParentTaskId != "parent-1" {
					t.Errorf("ParentTaskId=%q want parent-1", got.ParentTaskId)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pbL1Task([]byte(tt.raw))
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestPbMemoryFact(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		check   func(t *testing.T, got *v1.MemoryFact)
	}{
		{
			name:    "invalid_json",
			raw:     `not json`,
			wantErr: true,
		},
		{
			name: "empty_object",
			raw:  `{}`,
			check: func(t *testing.T, got *v1.MemoryFact) {
				if got.Id != "" {
					t.Errorf("Id=%q want empty", got.Id)
				}
				if got.PiiFlag {
					t.Errorf("PiiFlag=%v want false", got.PiiFlag)
				}
				if got.Confidence != 0 {
					t.Errorf("Confidence=%f want 0", got.Confidence)
				}
			},
		},
		{
			name: "full_fields",
			raw: `{
				"id": "fact-1",
				"scope_type": "agent",
				"scope_id": "agent-1",
				"workspace_id": "ws-1",
				"user_id": "user-1",
				"team_id": "team-1",
				"agent_id": "agent-1",
				"statement": "User prefers dark mode",
				"details_markdown": "**bold**",
				"fact_kind": "preference",
				"tags_json": "[\"ui\"]",
				"confidence": 0.95,
				"importance": 0.8,
				"use_count": 5,
				"hit_count": 12,
				"positive_feedback_count": 3,
				"negative_feedback_count": 1,
				"conflict_count": 0,
				"source_kind": "episode",
				"source_episode_id": "ep-1",
				"source_session_id": "sess-1",
				"source_message_id": "msg-1",
				"version": 3,
				"status": "active",
				"pii_flag": true,
				"created_at": "2025-01-01T00:00:00Z",
				"updated_at": "2025-01-02T00:00:00Z"
			}`,
			check: func(t *testing.T, got *v1.MemoryFact) {
				if got.Id != "fact-1" {
					t.Errorf("Id=%q want fact-1", got.Id)
				}
				if got.ScopeType != "agent" {
					t.Errorf("ScopeType=%q want agent", got.ScopeType)
				}
				if got.Statement != "User prefers dark mode" {
					t.Errorf("Statement=%q want 'User prefers dark mode'", got.Statement)
				}
				if got.FactKind != "preference" {
					t.Errorf("FactKind=%q want preference", got.FactKind)
				}
				if got.Confidence != 0.95 {
					t.Errorf("Confidence=%f want 0.95", got.Confidence)
				}
				if got.Importance != 0.8 {
					t.Errorf("Importance=%f want 0.8", got.Importance)
				}
				if got.UseCount != 5 {
					t.Errorf("UseCount=%d want 5", got.UseCount)
				}
				if got.Version != 3 {
					t.Errorf("Version=%d want 3", got.Version)
				}
				if !got.PiiFlag {
					t.Errorf("PiiFlag=%v want true", got.PiiFlag)
				}
				if got.SourceKind != "episode" {
					t.Errorf("SourceKind=%q want episode", got.SourceKind)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pbMemoryFact([]byte(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Errorf("pbMemoryFact() error=%v wantErr=%v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestPbMemoryEntity(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		check   func(t *testing.T, got *v1.MemoryEntity)
	}{
		{
			name:    "invalid_json",
			raw:     `{invalid`,
			wantErr: true,
		},
		{
			name: "empty_object",
			raw:  `{}`,
			check: func(t *testing.T, got *v1.MemoryEntity) {
				if got.Id != "" {
					t.Errorf("Id=%q want empty", got.Id)
				}
				if len(got.Aliases) != 0 {
					t.Errorf("Aliases=%v want empty", got.Aliases)
				}
			},
		},
		{
			name: "full_fields_with_aliases",
			raw: `{
				"id": "ent-1",
				"scope_type": "agent",
				"scope_id": "agent-1",
				"workspace_id": "ws-1",
				"user_id": "user-1",
				"entity_type": "person",
				"name": "Alice",
				"name_normalized": "alice",
				"aliases": ["Al", "Alicia"],
				"description": "A developer",
				"importance": 0.9,
				"confidence": 0.85,
				"use_count": 7,
				"source_kind": "episode",
				"status": "active",
				"created_at": "2025-01-01T00:00:00Z",
				"updated_at": "2025-01-02T00:00:00Z"
			}`,
			check: func(t *testing.T, got *v1.MemoryEntity) {
				if got.Id != "ent-1" {
					t.Errorf("Id=%q want ent-1", got.Id)
				}
				if got.EntityType != "person" {
					t.Errorf("EntityType=%q want person", got.EntityType)
				}
				if got.Name != "Alice" {
					t.Errorf("Name=%q want Alice", got.Name)
				}
				if got.NameNormalized != "alice" {
					t.Errorf("NameNormalized=%q want alice", got.NameNormalized)
				}
				if len(got.Aliases) != 2 {
					t.Fatalf("Aliases len=%d want 2", len(got.Aliases))
				}
				if got.Aliases[0] != "Al" {
					t.Errorf("Aliases[0]=%q want Al", got.Aliases[0])
				}
				if got.Aliases[1] != "Alicia" {
					t.Errorf("Aliases[1]=%q want Alicia", got.Aliases[1])
				}
				if got.Importance != 0.9 {
					t.Errorf("Importance=%f want 0.9", got.Importance)
				}
			},
		},
		{
			name: "aliases_with_non_string_items",
			raw:  `{"id": "ent-2", "aliases": ["valid", 123, true, "also_valid"]}`,
			check: func(t *testing.T, got *v1.MemoryEntity) {
				if len(got.Aliases) != 2 {
					t.Fatalf("Aliases len=%d want 2", len(got.Aliases))
				}
				if got.Aliases[0] != "valid" {
					t.Errorf("Aliases[0]=%q want valid", got.Aliases[0])
				}
				if got.Aliases[1] != "also_valid" {
					t.Errorf("Aliases[1]=%q want also_valid", got.Aliases[1])
				}
			},
		},
		{
			name: "no_aliases_key",
			raw:  `{"id": "ent-3", "name": "Bob"}`,
			check: func(t *testing.T, got *v1.MemoryEntity) {
				if len(got.Aliases) != 0 {
					t.Errorf("Aliases=%v want empty", got.Aliases)
				}
				if got.Name != "Bob" {
					t.Errorf("Name=%q want Bob", got.Name)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pbMemoryEntity([]byte(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Errorf("pbMemoryEntity() error=%v wantErr=%v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestPbMemoryRelation(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		check   func(t *testing.T, got *v1.MemoryRelation)
	}{
		{
			name:    "invalid_json",
			raw:     `bad`,
			wantErr: true,
		},
		{
			name: "empty_object",
			raw:  `{}`,
			check: func(t *testing.T, got *v1.MemoryRelation) {
				if got.Id != "" {
					t.Errorf("Id=%q want empty", got.Id)
				}
				if got.Weight != 0 {
					t.Errorf("Weight=%f want 0", got.Weight)
				}
			},
		},
		{
			name: "full_fields",
			raw: `{
				"id": "rel-1",
				"source_id": "ent-1",
				"target_id": "ent-2",
				"relation_type": "knows",
				"weight": 0.7,
				"confidence": 0.9,
				"status": "active",
				"valid_from": "2025-01-01T00:00:00Z",
				"valid_to": "2025-12-31T23:59:59Z"
			}`,
			check: func(t *testing.T, got *v1.MemoryRelation) {
				if got.Id != "rel-1" {
					t.Errorf("Id=%q want rel-1", got.Id)
				}
				if got.SourceId != "ent-1" {
					t.Errorf("SourceId=%q want ent-1", got.SourceId)
				}
				if got.TargetId != "ent-2" {
					t.Errorf("TargetId=%q want ent-2", got.TargetId)
				}
				if got.RelationType != "knows" {
					t.Errorf("RelationType=%q want knows", got.RelationType)
				}
				if got.Weight != 0.7 {
					t.Errorf("Weight=%f want 0.7", got.Weight)
				}
				if got.Confidence != 0.9 {
					t.Errorf("Confidence=%f want 0.9", got.Confidence)
				}
				if got.Status != "active" {
					t.Errorf("Status=%q want active", got.Status)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pbMemoryRelation([]byte(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Errorf("pbMemoryRelation() error=%v wantErr=%v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
