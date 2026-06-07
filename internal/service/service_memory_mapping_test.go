package service_test

import (
	"testing"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/jsonutil"
)

func TestPbL0AssemblySnapshot(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, got *v1.L0AssemblySnapshot)
	}{
		{
			name:  "full fields",
			input: `{"id":"snap-1","session_id":"sess-1","run_id":"run-1","turn_id":"turn-1","span_id":"span-1","agent_id":"agent-1","team_id":"team-1","provider":"openai","model":"gpt-4","context_window_tokens":8192,"budget_tokens":4096,"recent_window_turns":5,"recent_window_tokens":1000,"summary_token_estimate":200,"l1_field_count":3,"l1_token_estimate":150,"l3_chunk_count":10,"l3_token_estimate":500,"l4_path_count":2,"l4_token_estimate":80,"prompt_token_estimate":3000,"prompt_token_actual":2950,"used_ratio":0.85,"truncate_strategy":"sliding","truncated_message_count":2,"summarized_turn_from":1,"summarized_turn_to":3,"segments_json":"{}","warning_codes_json":"[]","metadata_json":"{}","created_at":"2026-01-01T00:00:00Z"}`,
			check: func(t *testing.T, got *v1.L0AssemblySnapshot) {
				if got.Id != "snap-1" {
					t.Errorf("Id = %q, want %q", got.Id, "snap-1")
				}
				if got.SessionId != "sess-1" {
					t.Errorf("SessionId = %q, want %q", got.SessionId, "sess-1")
				}
				if got.AgentId != "agent-1" {
					t.Errorf("AgentId = %q, want %q", got.AgentId, "agent-1")
				}
				if got.Provider != "openai" {
					t.Errorf("Provider = %q, want %q", got.Provider, "openai")
				}
				if got.ContextWindowTokens != 8192 {
					t.Errorf("ContextWindowTokens = %d, want %d", got.ContextWindowTokens, 8192)
				}
				if got.UsedRatio != 0.85 {
					t.Errorf("UsedRatio = %f, want %f", got.UsedRatio, 0.85)
				}
				if got.TruncateStrategy != "sliding" {
					t.Errorf("TruncateStrategy = %q, want %q", got.TruncateStrategy, "sliding")
				}
			},
		},
		{
			name:  "empty json object",
			input: `{}`,
			check: func(t *testing.T, got *v1.L0AssemblySnapshot) {
				if got.Id != "" {
					t.Errorf("Id = %q, want empty", got.Id)
				}
				if got.ContextWindowTokens != 0 {
					t.Errorf("ContextWindowTokens = %d, want 0", got.ContextWindowTokens)
				}
			},
		},
		{
			name:    "invalid json",
			input:   `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.PbL0AssemblySnapshot([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil && got != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestPbL1Task(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, got *v1.L1Task)
	}{
		{
			name:  "full fields",
			input: `{"id":"task-1","session_id":"sess-1","run_id":"run-1","team_id":"team-1","agent_id":"agent-1","task_key":"research","task_title":"Research Task","task_goal":"Find info","status":"running","schema_version":2,"budget_tokens":2000,"used_tokens":500,"parent_task_id":"","shared_with_json":"[]","started_at":"2026-01-01T00:00:00Z","ended_at":"","archived_at":"","metadata_json":"{}","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			check: func(t *testing.T, got *v1.L1Task) {
				if got.Id != "task-1" {
					t.Errorf("Id = %q, want %q", got.Id, "task-1")
				}
				if got.TaskKey != "research" {
					t.Errorf("TaskKey = %q, want %q", got.TaskKey, "research")
				}
				if got.Status != "running" {
					t.Errorf("Status = %q, want %q", got.Status, "running")
				}
				if got.BudgetTokens != 2000 {
					t.Errorf("BudgetTokens = %d, want %d", got.BudgetTokens, 2000)
				}
			},
		},
		{
			name:  "empty json object",
			input: `{}`,
			check: func(t *testing.T, got *v1.L1Task) {
				if got.Id != "" {
					t.Errorf("Id = %q, want empty", got.Id)
				}
			},
		},
		{
			name:  "invalid json returns nil",
			input: `{invalid`,
			check: func(t *testing.T, got *v1.L1Task) {
				if got != nil {
					t.Errorf("got = %v, want nil", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.PbL1Task([]byte(tt.input))
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestPbMemoryFact(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, got *v1.MemoryFact)
	}{
		{
			name:  "full fields",
			input: `{"id":"fact-1","scope_type":"agent","scope_id":"agent-1","workspace_id":"ws-1","user_id":"u-1","team_id":"t-1","agent_id":"a-1","statement":"user prefers dark mode","details_markdown":"details","fact_kind":"preference","tags_json":"[]","confidence":0.95,"importance":0.8,"use_count":5,"hit_count":10,"positive_feedback_count":3,"negative_feedback_count":1,"conflict_count":0,"source_kind":"conversation","source_episode_id":"ep-1","source_session_id":"sess-1","source_message_id":"msg-1","version":2,"status":"active","pii_flag":false,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			check: func(t *testing.T, got *v1.MemoryFact) {
				if got.Id != "fact-1" {
					t.Errorf("Id = %q, want %q", got.Id, "fact-1")
				}
				if got.ScopeType != "agent" {
					t.Errorf("ScopeType = %q, want %q", got.ScopeType, "agent")
				}
				if got.Statement != "user prefers dark mode" {
					t.Errorf("Statement = %q, want %q", got.Statement, "user prefers dark mode")
				}
				if got.Confidence != 0.95 {
					t.Errorf("Confidence = %f, want %f", got.Confidence, 0.95)
				}
				if got.UseCount != 5 {
					t.Errorf("UseCount = %d, want %d", got.UseCount, 5)
				}
				if got.PiiFlag != false {
					t.Errorf("PiiFlag = %v, want false", got.PiiFlag)
				}
			},
		},
		{
			name:  "empty json object",
			input: `{}`,
			check: func(t *testing.T, got *v1.MemoryFact) {
				if got.Id != "" {
					t.Errorf("Id = %q, want empty", got.Id)
				}
			},
		},
		{
			name:    "invalid json",
			input:   `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.PbMemoryFact([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil && got != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestPbMemoryEntity(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, got *v1.MemoryEntity)
	}{
		{
			name:  "with aliases",
			input: `{"id":"ent-1","scope_type":"agent","scope_id":"agent-1","workspace_id":"ws-1","user_id":"u-1","entity_type":"person","name":"Alice","name_normalized":"alice","aliases":["Al","Alicia"],"description":"A user","importance":0.9,"confidence":0.8,"use_count":3,"source_kind":"conversation","status":"active","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			check: func(t *testing.T, got *v1.MemoryEntity) {
				if got.Id != "ent-1" {
					t.Errorf("Id = %q, want %q", got.Id, "ent-1")
				}
				if got.EntityType != "person" {
					t.Errorf("EntityType = %q, want %q", got.EntityType, "person")
				}
				if got.Name != "Alice" {
					t.Errorf("Name = %q, want %q", got.Name, "Alice")
				}
				if len(got.Aliases) != 2 {
					t.Fatalf("Aliases len = %d, want 2", len(got.Aliases))
				}
				if got.Aliases[0] != "Al" || got.Aliases[1] != "Alicia" {
					t.Errorf("Aliases = %v, want [Al Alicia]", got.Aliases)
				}
				if got.Importance != 0.9 {
					t.Errorf("Importance = %f, want %f", got.Importance, 0.9)
				}
			},
		},
		{
			name:  "no aliases",
			input: `{"id":"ent-2","entity_type":"concept","name":"Go"}`,
			check: func(t *testing.T, got *v1.MemoryEntity) {
				if len(got.Aliases) != 0 {
					t.Errorf("Aliases = %v, want empty", got.Aliases)
				}
			},
		},
		{
			name:    "invalid json",
			input:   `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.PbMemoryEntity([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil && got != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestPbMemoryRelation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, got *v1.MemoryRelation)
	}{
		{
			name:  "full fields",
			input: `{"id":"rel-1","source_id":"ent-1","target_id":"ent-2","relation_type":"knows","weight":0.7,"confidence":0.9,"status":"active","valid_from":"2026-01-01T00:00:00Z","valid_to":"2026-12-31T23:59:59Z"}`,
			check: func(t *testing.T, got *v1.MemoryRelation) {
				if got.Id != "rel-1" {
					t.Errorf("Id = %q, want %q", got.Id, "rel-1")
				}
				if got.SourceId != "ent-1" {
					t.Errorf("SourceId = %q, want %q", got.SourceId, "ent-1")
				}
				if got.RelationType != "knows" {
					t.Errorf("RelationType = %q, want %q", got.RelationType, "knows")
				}
				if got.Weight != 0.7 {
					t.Errorf("Weight = %f, want %f", got.Weight, 0.7)
				}
			},
		},
		{
			name:    "invalid json",
			input:   `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.PbMemoryRelation([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil && got != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestPbCascadeProposal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, got *v1.CascadeProposal)
	}{
		{
			name:  "full fields without affected",
			input: `{"id":"cp-1","agent_id":"agent-1","workspace_id":"ws-1","trigger_entity_id":"ent-1","trigger_entity_name":"Alice","trigger_attribute":"name","old_value":"Al","new_value":"Alice","status":"pending","risk_level":"low","rationale":"user correction","reviewed_by":"","reviewed_at":"","expires_at":"","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			check: func(t *testing.T, got *v1.CascadeProposal) {
				if got.Id != "cp-1" {
					t.Errorf("Id = %q, want %q", got.Id, "cp-1")
				}
				if got.TriggerEntityName != "Alice" {
					t.Errorf("TriggerEntityName = %q, want %q", got.TriggerEntityName, "Alice")
				}
				if got.RiskLevel != "low" {
					t.Errorf("RiskLevel = %q, want %q", got.RiskLevel, "low")
				}
				if len(got.AffectedEntities) != 0 {
					t.Errorf("AffectedEntities = %v, want empty", got.AffectedEntities)
				}
			},
		},
		{
			name:  "with affected entities",
			input: `{"id":"cp-2","agent_id":"agent-1","workspace_id":"ws-1","trigger_entity_id":"ent-1","trigger_entity_name":"Bob","trigger_attribute":"name","old_value":"B","new_value":"Bob","status":"pending","risk_level":"medium","affected_json":"[{\"entity_id\":\"ent-2\",\"entity_name\":\"Related\",\"entity_type\":\"person\",\"relation_type\":\"colleague\",\"hops\":1}]"}`,
			check: func(t *testing.T, got *v1.CascadeProposal) {
				if len(got.AffectedEntities) != 1 {
					t.Fatalf("AffectedEntities len = %d, want 1", len(got.AffectedEntities))
				}
				ae := got.AffectedEntities[0]
				if ae.EntityId != "ent-2" {
					t.Errorf("EntityId = %q, want %q", ae.EntityId, "ent-2")
				}
				if ae.Hops != 1 {
					t.Errorf("Hops = %d, want 1", ae.Hops)
				}
			},
		},
		{
			name:    "invalid json",
			input:   `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.PbCascadeProposal([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil && got != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestMapStringFloat(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  map[string]float64
	}{
		{
			name:  "nil input",
			input: nil,
			want:  map[string]float64{},
		},
		{
			name:  "empty input",
			input: map[string]any{},
			want:  map[string]float64{},
		},
		{
			name:  "float64 values",
			input: map[string]any{"a": 0.5, "b": 1.0},
			want:  map[string]float64{"a": 0.5, "b": 1.0},
		},
		{
			name:  "int values",
			input: map[string]any{"a": 1, "b": 2},
			want:  map[string]float64{"a": 1.0, "b": 2.0},
		},
		{
			name:  "mixed types skips string",
			input: map[string]any{"a": 0.5, "b": "not a number", "c": 3},
			want:  map[string]float64{"a": 0.5, "c": 3.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonutil.MapStringFloat(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("got[%q] = %f, want %f", k, got[k], v)
				}
			}
		})
	}
}
