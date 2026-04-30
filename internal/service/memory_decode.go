package service

import (
	"encoding/json"
	"strconv"

	v1 "aranea-agents/api/kratos/memory/v1"
)

// legacy JSON decoding helpers (upstream **`/api/v1/...`** responses use snake_case).
func ifaceStr(m map[string]any, k string) string {
	v, ok := m[k]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case bool:
		return strconv.FormatBool(t)
	default:
		return ""
	}
}

func ifaceBool(m map[string]any, k string) bool {
	v, ok := m[k]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	default:
		return false
	}
}

func ifaceF64(m map[string]any, k string) float64 {
	v, ok := m[k]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	default:
		return 0
	}
}

func ifaceI32(m map[string]any, k string) int32 {
	v, ok := m[k]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int32(t)
	case int:
		return int32(t)
	default:
		return 0
	}
}

func jsonMap(bs []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(bs, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return map[string]any{}, nil
	}
	return m, nil
}

func pbL0AssemblySnapshot(raw []byte) (*v1.L0AssemblySnapshot, error) {
	m, err := jsonMap(raw)
	if err != nil {
		return nil, err
	}
	return &v1.L0AssemblySnapshot{
		Id:                    ifaceStr(m, "id"),
		SessionId:             ifaceStr(m, "session_id"),
		RunId:                 ifaceStr(m, "run_id"),
		TurnId:                ifaceStr(m, "turn_id"),
		SpanId:                ifaceStr(m, "span_id"),
		AgentId:               ifaceStr(m, "agent_id"),
		TeamId:                ifaceStr(m, "team_id"),
		Provider:              ifaceStr(m, "provider"),
		Model:                 ifaceStr(m, "model"),
		ContextWindowTokens:   ifaceI32(m, "context_window_tokens"),
		BudgetTokens:          ifaceI32(m, "budget_tokens"),
		RecentWindowTurns:     ifaceI32(m, "recent_window_turns"),
		RecentWindowTokens:    ifaceI32(m, "recent_window_tokens"),
		SummaryTokenEstimate:  ifaceI32(m, "summary_token_estimate"),
		L1FieldCount:          ifaceI32(m, "l1_field_count"),
		L1TokenEstimate:       ifaceI32(m, "l1_token_estimate"),
		L3ChunkCount:          ifaceI32(m, "l3_chunk_count"),
		L3TokenEstimate:       ifaceI32(m, "l3_token_estimate"),
		L4PathCount:           ifaceI32(m, "l4_path_count"),
		L4TokenEstimate:       ifaceI32(m, "l4_token_estimate"),
		PromptTokenEstimate:   ifaceI32(m, "prompt_token_estimate"),
		PromptTokenActual:     ifaceI32(m, "prompt_token_actual"),
		UsedRatio:             ifaceF64(m, "used_ratio"),
		TruncateStrategy:      ifaceStr(m, "truncate_strategy"),
		TruncatedMessageCount: ifaceI32(m, "truncated_message_count"),
		SummarizedTurnFrom:    ifaceI32(m, "summarized_turn_from"),
		SummarizedTurnTo:      ifaceI32(m, "summarized_turn_to"),
		SegmentsJson:          ifaceStr(m, "segments_json"),
		WarningCodesJson:      ifaceStr(m, "warning_codes_json"),
		MetadataJson:          ifaceStr(m, "metadata_json"),
		CreatedAt:             ifaceStr(m, "created_at"),
	}, nil
}

func pbL1Task(raw []byte) *v1.L1Task {
	m, err := jsonMap(raw)
	if err != nil {
		return nil
	}
	return &v1.L1Task{
		Id:             ifaceStr(m, "id"),
		SessionId:      ifaceStr(m, "session_id"),
		RunId:          ifaceStr(m, "run_id"),
		TeamId:         ifaceStr(m, "team_id"),
		AgentId:        ifaceStr(m, "agent_id"),
		TaskKey:        ifaceStr(m, "task_key"),
		TaskTitle:      ifaceStr(m, "task_title"),
		TaskGoal:       ifaceStr(m, "task_goal"),
		Status:         ifaceStr(m, "status"),
		SchemaVersion:  ifaceI32(m, "schema_version"),
		BudgetTokens:   ifaceI32(m, "budget_tokens"),
		UsedTokens:     ifaceI32(m, "used_tokens"),
		ParentTaskId:   ifaceStr(m, "parent_task_id"),
		SharedWithJson: ifaceStr(m, "shared_with_json"),
		StartedAt:      ifaceStr(m, "started_at"),
		EndedAt:        ifaceStr(m, "ended_at"),
		ArchivedAt:     ifaceStr(m, "archived_at"),
		MetadataJson:   ifaceStr(m, "metadata_json"),
		CreatedAt:      ifaceStr(m, "created_at"),
		UpdatedAt:      ifaceStr(m, "updated_at"),
	}
}

func pbMemoryFact(raw []byte) (*v1.MemoryFact, error) {
	m, err := jsonMap(raw)
	if err != nil {
		return nil, err
	}
	return &v1.MemoryFact{
		Id:                       ifaceStr(m, "id"),
		ScopeType:                ifaceStr(m, "scope_type"),
		ScopeId:                  ifaceStr(m, "scope_id"),
		WorkspaceId:              ifaceStr(m, "workspace_id"),
		UserId:                   ifaceStr(m, "user_id"),
		TeamId:                   ifaceStr(m, "team_id"),
		AgentId:                  ifaceStr(m, "agent_id"),
		Statement:                ifaceStr(m, "statement"),
		DetailsMarkdown:          ifaceStr(m, "details_markdown"),
		FactKind:                 ifaceStr(m, "fact_kind"),
		TagsJson:                 ifaceStr(m, "tags_json"),
		Confidence:               ifaceF64(m, "confidence"),
		Importance:               ifaceF64(m, "importance"),
		UseCount:                 ifaceI32(m, "use_count"),
		HitCount:                 ifaceI32(m, "hit_count"),
		PositiveFeedbackCount:    ifaceI32(m, "positive_feedback_count"),
		NegativeFeedbackCount:    ifaceI32(m, "negative_feedback_count"),
		ConflictCount:            ifaceI32(m, "conflict_count"),
		SourceKind:               ifaceStr(m, "source_kind"),
		SourceEpisodeId:          ifaceStr(m, "source_episode_id"),
		SourceSessionId:          ifaceStr(m, "source_session_id"),
		SourceMessageId:          ifaceStr(m, "source_message_id"),
		Version:                  ifaceI32(m, "version"),
		Status:                   ifaceStr(m, "status"),
		PiiFlag:                  ifaceBool(m, "pii_flag"),
		CreatedAt:                ifaceStr(m, "created_at"),
		UpdatedAt:                ifaceStr(m, "updated_at"),
	}, nil
}

func pbMemoryEntity(raw []byte) (*v1.MemoryEntity, error) {
	m, err := jsonMap(raw)
	if err != nil {
		return nil, err
	}
	var aliases []string
	if rawA, ok := m["aliases"].([]any); ok {
		for _, a := range rawA {
			if s, ok := a.(string); ok {
				aliases = append(aliases, s)
			}
		}
	}
	return &v1.MemoryEntity{
		Id:              ifaceStr(m, "id"),
		ScopeType:       ifaceStr(m, "scope_type"),
		ScopeId:         ifaceStr(m, "scope_id"),
		WorkspaceId:     ifaceStr(m, "workspace_id"),
		UserId:          ifaceStr(m, "user_id"),
		EntityType:      ifaceStr(m, "entity_type"),
		Name:            ifaceStr(m, "name"),
		NameNormalized:  ifaceStr(m, "name_normalized"),
		Aliases:         aliases,
		Description:     ifaceStr(m, "description"),
		Importance:      ifaceF64(m, "importance"),
		Confidence:      ifaceF64(m, "confidence"),
		UseCount:        ifaceI32(m, "use_count"),
		SourceKind:      ifaceStr(m, "source_kind"),
		Status:          ifaceStr(m, "status"),
		CreatedAt:       ifaceStr(m, "created_at"),
		UpdatedAt:       ifaceStr(m, "updated_at"),
	}, nil
}

func pbMemoryRelation(raw []byte) (*v1.MemoryRelation, error) {
	m, err := jsonMap(raw)
	if err != nil {
		return nil, err
	}
	return &v1.MemoryRelation{
		Id:           ifaceStr(m, "id"),
		SourceId:     ifaceStr(m, "source_id"),
		TargetId:     ifaceStr(m, "target_id"),
		RelationType: ifaceStr(m, "relation_type"),
		Weight:       ifaceF64(m, "weight"),
		Confidence:   ifaceF64(m, "confidence"),
		Status:       ifaceStr(m, "status"),
	}, nil
}
