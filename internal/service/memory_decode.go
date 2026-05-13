package service

import (
	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/pkg/jsonutil"
)

func pbL0AssemblySnapshot(raw []byte) (*v1.L0AssemblySnapshot, error) {
	m, err := jsonutil.ParseMap(raw)
	if err != nil {
		return nil, err
	}
	return &v1.L0AssemblySnapshot{
		Id:                    jsonutil.IfaceStr(m, "id"),
		SessionId:             jsonutil.IfaceStr(m, "session_id"),
		RunId:                 jsonutil.IfaceStr(m, "run_id"),
		TurnId:                jsonutil.IfaceStr(m, "turn_id"),
		SpanId:                jsonutil.IfaceStr(m, "span_id"),
		AgentId:               jsonutil.IfaceStr(m, "agent_id"),
		TeamId:                jsonutil.IfaceStr(m, "team_id"),
		Provider:              jsonutil.IfaceStr(m, "provider"),
		Model:                 jsonutil.IfaceStr(m, "model"),
		ContextWindowTokens:   jsonutil.IfaceI32(m, "context_window_tokens"),
		BudgetTokens:          jsonutil.IfaceI32(m, "budget_tokens"),
		RecentWindowTurns:     jsonutil.IfaceI32(m, "recent_window_turns"),
		RecentWindowTokens:    jsonutil.IfaceI32(m, "recent_window_tokens"),
		SummaryTokenEstimate:  jsonutil.IfaceI32(m, "summary_token_estimate"),
		L1FieldCount:          jsonutil.IfaceI32(m, "l1_field_count"),
		L1TokenEstimate:       jsonutil.IfaceI32(m, "l1_token_estimate"),
		L3ChunkCount:          jsonutil.IfaceI32(m, "l3_chunk_count"),
		L3TokenEstimate:       jsonutil.IfaceI32(m, "l3_token_estimate"),
		L4PathCount:           jsonutil.IfaceI32(m, "l4_path_count"),
		L4TokenEstimate:       jsonutil.IfaceI32(m, "l4_token_estimate"),
		PromptTokenEstimate:   jsonutil.IfaceI32(m, "prompt_token_estimate"),
		PromptTokenActual:     jsonutil.IfaceI32(m, "prompt_token_actual"),
		UsedRatio:             jsonutil.IfaceF64(m, "used_ratio"),
		TruncateStrategy:      jsonutil.IfaceStr(m, "truncate_strategy"),
		TruncatedMessageCount: jsonutil.IfaceI32(m, "truncated_message_count"),
		SummarizedTurnFrom:    jsonutil.IfaceI32(m, "summarized_turn_from"),
		SummarizedTurnTo:      jsonutil.IfaceI32(m, "summarized_turn_to"),
		SegmentsJson:          jsonutil.IfaceStr(m, "segments_json"),
		WarningCodesJson:      jsonutil.IfaceStr(m, "warning_codes_json"),
		MetadataJson:          jsonutil.IfaceStr(m, "metadata_json"),
		CreatedAt:             jsonutil.IfaceStr(m, "created_at"),
	}, nil
}

func pbL1Task(raw []byte) *v1.L1Task {
	m, err := jsonutil.ParseMap(raw)
	if err != nil {
		return nil
	}
	return &v1.L1Task{
		Id:             jsonutil.IfaceStr(m, "id"),
		SessionId:      jsonutil.IfaceStr(m, "session_id"),
		RunId:          jsonutil.IfaceStr(m, "run_id"),
		TeamId:         jsonutil.IfaceStr(m, "team_id"),
		AgentId:        jsonutil.IfaceStr(m, "agent_id"),
		TaskKey:        jsonutil.IfaceStr(m, "task_key"),
		TaskTitle:      jsonutil.IfaceStr(m, "task_title"),
		TaskGoal:       jsonutil.IfaceStr(m, "task_goal"),
		Status:         jsonutil.IfaceStr(m, "status"),
		SchemaVersion:  jsonutil.IfaceI32(m, "schema_version"),
		BudgetTokens:   jsonutil.IfaceI32(m, "budget_tokens"),
		UsedTokens:     jsonutil.IfaceI32(m, "used_tokens"),
		ParentTaskId:   jsonutil.IfaceStr(m, "parent_task_id"),
		SharedWithJson: jsonutil.IfaceStr(m, "shared_with_json"),
		StartedAt:      jsonutil.IfaceStr(m, "started_at"),
		EndedAt:        jsonutil.IfaceStr(m, "ended_at"),
		ArchivedAt:     jsonutil.IfaceStr(m, "archived_at"),
		MetadataJson:   jsonutil.IfaceStr(m, "metadata_json"),
		CreatedAt:      jsonutil.IfaceStr(m, "created_at"),
		UpdatedAt:      jsonutil.IfaceStr(m, "updated_at"),
	}
}

func pbMemoryFact(raw []byte) (*v1.MemoryFact, error) {
	m, err := jsonutil.ParseMap(raw)
	if err != nil {
		return nil, err
	}
	return &v1.MemoryFact{
		Id:                       jsonutil.IfaceStr(m, "id"),
		ScopeType:                jsonutil.IfaceStr(m, "scope_type"),
		ScopeId:                  jsonutil.IfaceStr(m, "scope_id"),
		WorkspaceId:              jsonutil.IfaceStr(m, "workspace_id"),
		UserId:                   jsonutil.IfaceStr(m, "user_id"),
		TeamId:                   jsonutil.IfaceStr(m, "team_id"),
		AgentId:                  jsonutil.IfaceStr(m, "agent_id"),
		Statement:                jsonutil.IfaceStr(m, "statement"),
		DetailsMarkdown:          jsonutil.IfaceStr(m, "details_markdown"),
		FactKind:                 jsonutil.IfaceStr(m, "fact_kind"),
		TagsJson:                 jsonutil.IfaceStr(m, "tags_json"),
		Confidence:               jsonutil.IfaceF64(m, "confidence"),
		Importance:               jsonutil.IfaceF64(m, "importance"),
		UseCount:                 jsonutil.IfaceI32(m, "use_count"),
		HitCount:                 jsonutil.IfaceI32(m, "hit_count"),
		PositiveFeedbackCount:    jsonutil.IfaceI32(m, "positive_feedback_count"),
		NegativeFeedbackCount:    jsonutil.IfaceI32(m, "negative_feedback_count"),
		ConflictCount:            jsonutil.IfaceI32(m, "conflict_count"),
		SourceKind:               jsonutil.IfaceStr(m, "source_kind"),
		SourceEpisodeId:          jsonutil.IfaceStr(m, "source_episode_id"),
		SourceSessionId:          jsonutil.IfaceStr(m, "source_session_id"),
		SourceMessageId:          jsonutil.IfaceStr(m, "source_message_id"),
		Version:                  jsonutil.IfaceI32(m, "version"),
		Status:                   jsonutil.IfaceStr(m, "status"),
		PiiFlag:                  jsonutil.IfaceBool(m, "pii_flag"),
		CreatedAt:                jsonutil.IfaceStr(m, "created_at"),
		UpdatedAt:                jsonutil.IfaceStr(m, "updated_at"),
	}, nil
}

func pbMemoryEntity(raw []byte) (*v1.MemoryEntity, error) {
	m, err := jsonutil.ParseMap(raw)
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
		Id:              jsonutil.IfaceStr(m, "id"),
		ScopeType:       jsonutil.IfaceStr(m, "scope_type"),
		ScopeId:         jsonutil.IfaceStr(m, "scope_id"),
		WorkspaceId:     jsonutil.IfaceStr(m, "workspace_id"),
		UserId:          jsonutil.IfaceStr(m, "user_id"),
		EntityType:      jsonutil.IfaceStr(m, "entity_type"),
		Name:            jsonutil.IfaceStr(m, "name"),
		NameNormalized:  jsonutil.IfaceStr(m, "name_normalized"),
		Aliases:         aliases,
		Description:     jsonutil.IfaceStr(m, "description"),
		Importance:      jsonutil.IfaceF64(m, "importance"),
		Confidence:      jsonutil.IfaceF64(m, "confidence"),
		UseCount:        jsonutil.IfaceI32(m, "use_count"),
		SourceKind:      jsonutil.IfaceStr(m, "source_kind"),
		Status:          jsonutil.IfaceStr(m, "status"),
		CreatedAt:       jsonutil.IfaceStr(m, "created_at"),
		UpdatedAt:       jsonutil.IfaceStr(m, "updated_at"),
	}, nil
}

func pbMemoryRelation(raw []byte) (*v1.MemoryRelation, error) {
	m, err := jsonutil.ParseMap(raw)
	if err != nil {
		return nil, err
	}
	return &v1.MemoryRelation{
		Id:           jsonutil.IfaceStr(m, "id"),
		SourceId:     jsonutil.IfaceStr(m, "source_id"),
		TargetId:     jsonutil.IfaceStr(m, "target_id"),
		RelationType: jsonutil.IfaceStr(m, "relation_type"),
		Weight:       jsonutil.IfaceF64(m, "weight"),
		Confidence:   jsonutil.IfaceF64(m, "confidence"),
		Status:       jsonutil.IfaceStr(m, "status"),
	}, nil
}
