package service

import (
	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/jsonutil"
	"context"
	"encoding/json"
	"strings"
)

func (s *MemoryService) UpsertMemoryFact(ctx context.Context, req *v1.UpsertMemoryFactRequest) (*v1.UpsertMemoryFactResponse, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	f := req.GetFact()
	if f == nil {
		return nil, apierror.BadRequest(apierror.DomainMemory, "fact is required")
	}
	scopeType := strings.TrimSpace(f.GetScopeType())
	if err := s.assertOptionalAgentMemoryAccess(ctx, f.GetAgentId()); err != nil {
		return nil, err
	}
	if err := s.assertAgentScopeIfAgent(ctx, scopeType, f.GetScopeId()); err != nil {
		return nil, err
	}
	scopeID, err := authorizeMemoryScope(ctx, scopeType, f.GetScopeId(), true)
	if err != nil {
		return nil, err
	}
	wsID, err := authorizeMemoryWorkspaceField(ctx, f.GetWorkspaceId())
	if err != nil {
		return nil, err
	}
	raw, err := s.admin.UpsertFactRow(ctx, biz.FactUpsert{
		ID:                    strings.TrimSpace(f.GetId()),
		ScopeType:             scopeType,
		ScopeID:               scopeID,
		WorkspaceID:           wsID,
		UserID:                strings.TrimSpace(f.GetUserId()),
		TeamID:                strings.TrimSpace(f.GetTeamId()),
		AgentID:               strings.TrimSpace(f.GetAgentId()),
		Statement:             strings.TrimSpace(f.GetStatement()),
		DetailsMarkdown:       f.GetDetailsMarkdown(),
		FactKind:              f.GetFactKind(),
		TagsJSON:              f.GetTagsJson(),
		Confidence:            f.GetConfidence(),
		Importance:            f.GetImportance(),
		UseCount:              f.GetUseCount(),
		HitCount:              f.GetHitCount(),
		PositiveFeedbackCount: f.GetPositiveFeedbackCount(),
		NegativeFeedbackCount: f.GetNegativeFeedbackCount(),
		ConflictCount:         f.GetConflictCount(),
		SourceKind:            f.GetSourceKind(),
		SourceEpisodeID:       f.GetSourceEpisodeId(),
		SourceSessionID:       f.GetSourceSessionId(),
		SourceMessageID:       f.GetSourceMessageId(),
		Version:               f.GetVersion(),
		Status:                f.GetStatus(),
		PIIFlag:               f.GetPiiFlag(),
		PIITypes:              parsePIITypesJSON(f.GetPiiTypesJson()),
		CreatedAt:             strings.TrimSpace(f.GetCreatedAt()),
		UpdatedAt:             strings.TrimSpace(f.GetUpdatedAt()),
	})
	if err != nil {
		return nil, err
	}
	pb, err := pbMemoryFact(raw)
	if err != nil || pb == nil {
		return nil, apierror.Internal(apierror.DomainMemory, "failed to hydrate fact after upsert")
	}
	return &v1.UpsertMemoryFactResponse{Fact: pb}, nil
}

// parsePIITypesJSON decodes a JSON array string (e.g. '["email","phone"]') into
// a []string. Returns nil for empty/invalid input so the biz layer treats it as
// "no PII types specified".
func parsePIITypesJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func (s *MemoryService) AppendEvolutionEvent(ctx context.Context, req *v1.AppendEvolutionEventRequest) (*v1.AppendEvolutionEventResponse, error) {
	if err := s.requireWired(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, apierror.BadRequest(apierror.DomainMemory, "agent_id is required")
	}
	if err := s.assertAgentMemoryAccess(ctx, aid); err != nil {
		return nil, err
	}
	raw, err := s.admin.InsertEvolutionEventRow(ctx, biz.EvolutionEventInsert{
		AgentID:       aid,
		WorkspaceID:   strings.TrimSpace(req.GetWorkspaceId()),
		EventKind:     strings.TrimSpace(req.GetEventKind()),
		Kind:          strings.TrimSpace(req.GetKind()),
		TargetField:   strings.TrimSpace(req.GetTargetField()),
		Reason:        strings.TrimSpace(req.GetReason()),
		TriggerKind:   strings.TrimSpace(req.GetTriggerKind()),
		TriggerSource: strings.TrimSpace(req.GetTriggerSource()),
		MetadataJSON:  strings.TrimSpace(req.GetMetadataJson()),
	})
	if err != nil {
		return nil, err
	}
	m, err := jsonutil.ParseMap(raw)
	if err != nil {
		return nil, err
	}
	out := &v1.AppendEvolutionEventResponse{Event: &v1.EvolutionEvent{
		Id:          jsonutil.IfaceStr(m, "id"),
		AgentId:     jsonutil.IfaceStr(m, "agent_id"),
		EventKind:   jsonutil.IfaceStr(m, "event_kind"),
		Kind:        jsonutil.IfaceStr(m, "kind"),
		TargetField: jsonutil.IfaceStr(m, "target_field"),
		Reason:      jsonutil.IfaceStr(m, "reason"),
		Reverted:    jsonutil.IfaceBool(m, "reverted"),
		CreatedAt:   jsonutil.IfaceStr(m, "created_at"),
	}}
	return out, nil
}
