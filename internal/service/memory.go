package service

import (
	"context"
	"encoding/json"
	"strings"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/data/sessionmemory"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// MemoryService implements **memory/v1** against SQLite session-chain tables (**internal/data/sessionmemory**)，与 Ent 共用同一连接。
// **pkg/backend** 独立进程已废弃；不复用 **LEGACY_REST_ORIGIN** HTTP 转发。
// 与 **internal/biz.MemoryUsecase**（Postgres **pgvector**）是另一条「向量记忆」边界。
type MemoryService struct {
	v1.UnimplementedMemoryServiceServer

	store *sessionmemory.Store
}

// NewMemoryService wires L0–L4 + evolution reads from Ent SQLite.
func NewMemoryService(store *sessionmemory.Store) *MemoryService {
	return &MemoryService{store: store}
}

func (s *MemoryService) errStore() error {
	if s.store == nil {
		return kerrors.InternalServer("MEMORY", "session memory store not wired")
	}
	return nil
}

// ListL0Snapshots lists L0 assembly snapshots for a session.
func (s *MemoryService) ListL0Snapshots(ctx context.Context, req *v1.ListL0SnapshotsRequest) (*v1.ListL0SnapshotsResponse, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	sid := strings.TrimSpace(req.GetSessionId())
	if sid == "" {
		return nil, kerrors.BadRequest("MEMORY", "session_id is required")
	}
	rows, err := s.store.ListL0SnapshotRows(ctx, sid, req.GetLimit())
	if err != nil {
		return nil, err
	}
	out := &v1.ListL0SnapshotsResponse{}
	for _, raw := range rows {
		snap, e := pbL0AssemblySnapshot(raw)
		if e != nil {
			continue
		}
		out.Items = append(out.Items, snap)
	}
	return out, nil
}

// ListL1Tasks lists L1 tasks for a session.
func (s *MemoryService) ListL1Tasks(ctx context.Context, req *v1.ListL1TasksRequest) (*v1.ListL1TasksResponse, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	sid := strings.TrimSpace(req.GetSessionId())
	if sid == "" {
		return nil, kerrors.BadRequest("MEMORY", "session_id is required")
	}
	rows, err := s.store.ListL1TaskRows(ctx, sid,
		strings.TrimSpace(req.GetAgentId()),
		strings.TrimSpace(req.GetStatus()),
		strings.TrimSpace(req.GetIncludeEnded()),
	)
	if err != nil {
		return nil, err
	}
	out := &v1.ListL1TasksResponse{}
	for _, raw := range rows {
		if t := pbL1Task(raw); t != nil {
			out.Items = append(out.Items, t)
		}
	}
	return out, nil
}

// ListL1Fields lists fields for a task.
func (s *MemoryService) ListL1Fields(ctx context.Context, req *v1.ListL1FieldsRequest) (*v1.ListL1FieldsResponse, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	tid := strings.TrimSpace(req.GetTaskId())
	if tid == "" {
		return nil, kerrors.BadRequest("MEMORY", "task_id is required")
	}
	includeInternal := strings.TrimSpace(req.GetIncludeInternal()) == "true"
	rows, err := s.store.ListL1FieldRows(ctx, tid, includeInternal)
	if err != nil {
		return nil, err
	}
	out := &v1.ListL1FieldsResponse{}
	for _, raw := range rows {
		m, _ := jsonMap(raw)
		out.Items = append(out.Items, &v1.L1Field{
			Id:            ifaceStr(m, "id"),
			TaskId:        ifaceStr(m, "task_id"),
			SessionId:     ifaceStr(m, "session_id"),
			AgentId:       ifaceStr(m, "agent_id"),
			FieldPath:     ifaceStr(m, "field_path"),
			FieldKind:     ifaceStr(m, "field_kind"),
			Visibility:    ifaceStr(m, "visibility"),
			PinToPrompt:   ifaceBool(m, "pin_to_prompt"),
			IsRequired:    ifaceBool(m, "is_required"),
			ValueText:     ifaceStr(m, "value_text"),
			ValueJson:     ifaceStr(m, "value_json"),
			ValueRef:      ifaceStr(m, "value_ref"),
			Preview:       ifaceStr(m, "preview"),
			TokenEstimate: ifaceI32(m, "token_estimate"),
			Source:        ifaceStr(m, "source"),
			SourceRef:     ifaceStr(m, "source_ref"),
			TtlSeconds:    ifaceI32(m, "ttl_seconds"),
			ExpiresAt:     ifaceStr(m, "expires_at"),
			Revision:      ifaceI32(m, "revision"),
			LastReadAt:    ifaceStr(m, "last_read_at"),
			ReadCount:     ifaceI32(m, "read_count"),
			MetadataJson:  ifaceStr(m, "metadata_json"),
			CreatedAt:     ifaceStr(m, "created_at"),
			UpdatedAt:     ifaceStr(m, "updated_at"),
		})
	}
	return out, nil
}

// ListMemoryFacts lists L3 facts.
func (s *MemoryService) ListMemoryFacts(ctx context.Context, req *v1.ListMemoryFactsRequest) (*v1.ListMemoryFactsResponse, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	rows, total, lim, off, err := s.store.ListFactRows(ctx,
		strings.TrimSpace(req.GetScopeType()),
		strings.TrimSpace(req.GetScopeId()),
		strings.TrimSpace(req.GetKind()),
		strings.TrimSpace(req.GetStatus()),
		strings.TrimSpace(req.GetKeyword()),
		req.GetLimit(),
		req.GetOffset(),
	)
	if err != nil {
		return nil, err
	}
	out := &v1.ListMemoryFactsResponse{Total: total, Limit: lim, Offset: off}
	for _, raw := range rows {
		f, e := pbMemoryFact(raw)
		if e == nil && f != nil {
			out.Items = append(out.Items, f)
		}
	}
	return out, nil
}

// ListMemoryEntities lists L4 entities.
func (s *MemoryService) ListMemoryEntities(ctx context.Context, req *v1.ListMemoryEntitiesRequest) (*v1.ListMemoryEntitiesResponse, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	rows, total, err := s.store.ListEntityRows(ctx,
		strings.TrimSpace(req.GetScopeType()),
		strings.TrimSpace(req.GetScopeId()),
		strings.TrimSpace(req.GetWorkspaceId()),
		strings.TrimSpace(req.GetUserId()),
		strings.TrimSpace(req.GetEntityType()),
		strings.TrimSpace(req.GetStatus()),
		strings.TrimSpace(req.GetKeyword()),
		req.GetLimit(),
		req.GetOffset(),
	)
	if err != nil {
		return nil, err
	}
	out := &v1.ListMemoryEntitiesResponse{Total: total}
	for _, raw := range rows {
		ent, e := pbMemoryEntity(raw)
		if e == nil && ent != nil {
			out.Items = append(out.Items, ent)
		}
	}
	return out, nil
}

// GetMemoryNeighborhood returns a bounded ego-graph around an entity.
func (s *MemoryService) GetMemoryNeighborhood(ctx context.Context, req *v1.GetMemoryNeighborhoodRequest) (*v1.GraphNeighborhood, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	cid := strings.TrimSpace(req.GetCenterId())
	if cid == "" {
		return nil, kerrors.BadRequest("MEMORY", "center_id is required")
	}
	body, err := s.store.NeighborhoodJSON(ctx, cid, req.GetHops(), req.GetMaxNodes())
	if err != nil {
		return nil, err
	}
	var top struct {
		Center    map[string]any   `json:"center"`
		Hops      int32            `json:"hops"`
		Entities  []map[string]any `json:"entities"`
		Relations []map[string]any `json:"relations"`
	}
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}
	out := &v1.GraphNeighborhood{Hops: top.Hops}
	if top.Center != nil {
		raw, _ := json.Marshal(top.Center)
		c, _ := pbMemoryEntity(raw)
		out.Center = c
	}
	for _, row := range top.Entities {
		raw, _ := json.Marshal(row)
		e, _ := pbMemoryEntity(raw)
		if e != nil {
			out.Entities = append(out.Entities, e)
		}
	}
	for _, row := range top.Relations {
		raw, _ := json.Marshal(row)
		r, _ := pbMemoryRelation(raw)
		if r != nil {
			out.Relations = append(out.Relations, r)
		}
	}
	return out, nil
}

// GetAgentIdentity returns persisted identity JSON for an agent.
func (s *MemoryService) GetAgentIdentity(ctx context.Context, req *v1.GetAgentIdentityRequest) (*v1.AgentIdentity, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, kerrors.BadRequest("MEMORY", "agent_id is required")
	}
	body, err := s.store.AgentIdentityJSON(ctx, aid)
	if err != nil {
		return nil, err
	}
	m, err := jsonMap(body)
	if err != nil {
		return nil, err
	}
	var vals []string
	if rawV, ok := m["values"].([]any); ok {
		for _, v := range rawV {
			if sstr, ok := v.(string); ok {
				vals = append(vals, sstr)
			}
		}
	}
	var doms []string
	if rawD, ok := m["domains"].([]any); ok {
		for _, v := range rawD {
			if sstr, ok := v.(string); ok {
				doms = append(doms, sstr)
			}
		}
	}
	return &v1.AgentIdentity{
		AgentId:          ifaceStr(m, "agent_id"),
		Persona:          ifaceStr(m, "persona"),
		Values:           vals,
		Tone:             ifaceStr(m, "tone"),
		Domains:          doms,
		UserExpectations: ifaceStr(m, "user_expectations"),
		CurrentPhase:     ifaceStr(m, "current_phase"),
		Version:          ifaceI32(m, "version"),
	}, nil
}

// GetAgentStrategy returns persisted strategy profile JSON for an agent.
func (s *MemoryService) GetAgentStrategy(ctx context.Context, req *v1.GetAgentStrategyRequest) (*v1.AgentStrategyProfile, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, kerrors.BadRequest("MEMORY", "agent_id is required")
	}
	body, err := s.store.AgentStrategyJSON(ctx, aid)
	if err != nil {
		return nil, err
	}
	m, err := jsonMap(body)
	if err != nil {
		return nil, err
	}
	out := &v1.AgentStrategyProfile{
		AgentId:            ifaceStr(m, "agent_id"),
		Exploration:        ifaceF64(m, "exploration"),
		Conciseness:        ifaceF64(m, "conciseness"),
		Caution:            ifaceF64(m, "caution"),
		Delegation:         ifaceF64(m, "delegation"),
		Version:            ifaceI32(m, "version"),
		ToolPreference:     map[string]float64{},
		ProviderPreference: map[string]float64{},
		ModelPreference:    map[string]float64{},
	}
	if raw, ok := m["tool_preference"].(map[string]any); ok {
		out.ToolPreference = mapStringFloat(raw)
	}
	if raw, ok := m["provider_preference"].(map[string]any); ok {
		out.ProviderPreference = mapStringFloat(raw)
	}
	if raw, ok := m["model_preference"].(map[string]any); ok {
		out.ModelPreference = mapStringFloat(raw)
	}
	if raw, ok := m["tool_blacklist"].([]any); ok {
		for _, v := range raw {
			if sstr, ok := v.(string); ok {
				out.ToolBlacklist = append(out.ToolBlacklist, sstr)
			}
		}
	}
	return out, nil
}

func mapStringFloat(in map[string]any) map[string]float64 {
	out := make(map[string]float64)
	for k, v := range in {
		switch t := v.(type) {
		case float64:
			out[k] = t
		case int:
			out[k] = float64(t)
		}
	}
	return out
}

// ListEvolutionProposals lists evolution proposals for an agent.
func (s *MemoryService) ListEvolutionProposals(ctx context.Context, req *v1.ListEvolutionProposalsRequest) (*v1.ListEvolutionProposalsResponse, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, kerrors.BadRequest("MEMORY", "agent_id is required")
	}
	rows, err := s.store.EvolutionProposalRows(ctx, aid, strings.TrimSpace(req.GetStatus()), req.GetLimit())
	if err != nil {
		return nil, err
	}
	out := &v1.ListEvolutionProposalsResponse{}
	for _, raw := range rows {
		m, _ := jsonMap(raw)
		out.Items = append(out.Items, &v1.EvolutionProposal{
			Id:             ifaceStr(m, "id"),
			AgentId:        ifaceStr(m, "agent_id"),
			ProposalKind:   ifaceStr(m, "proposal_kind"),
			Kind:           ifaceStr(m, "kind"),
			TargetField:    ifaceStr(m, "target_field"),
			Rationale:      ifaceStr(m, "rationale"),
			ExpectedImpact: ifaceStr(m, "expected_impact"),
			RiskLevel:      ifaceStr(m, "risk_level"),
			Status:         ifaceStr(m, "status"),
			CreatedAt:      ifaceStr(m, "created_at"),
		})
	}
	return out, nil
}

// ListEvolutionEvents lists evolution events for an agent.
func (s *MemoryService) ListEvolutionEvents(ctx context.Context, req *v1.ListEvolutionEventsRequest) (*v1.ListEvolutionEventsResponse, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, kerrors.BadRequest("MEMORY", "agent_id is required")
	}
	rows, err := s.store.EvolutionEventRows(ctx, aid, req.GetLimit())
	if err != nil {
		return nil, err
	}
	out := &v1.ListEvolutionEventsResponse{}
	for _, raw := range rows {
		m, _ := jsonMap(raw)
		out.Items = append(out.Items, &v1.EvolutionEvent{
			Id:          ifaceStr(m, "id"),
			AgentId:     ifaceStr(m, "agent_id"),
			EventKind:   ifaceStr(m, "event_kind"),
			Kind:        ifaceStr(m, "kind"),
			TargetField: ifaceStr(m, "target_field"),
			Reason:      ifaceStr(m, "reason"),
			Reverted:    ifaceBool(m, "reverted"),
			CreatedAt:   ifaceStr(m, "created_at"),
		})
	}
	return out, nil
}

// GetEvolutionMetrics aggregates evolution counters from SQLite.
func (s *MemoryService) GetEvolutionMetrics(ctx context.Context, req *v1.GetEvolutionMetricsRequest) (*v1.EvolutionMetricsReport, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, kerrors.BadRequest("MEMORY", "agent_id is required")
	}
	_ = strings.TrimSpace(req.GetRange()) // reserved; native store aggregates all-time window for parity with legacy MVP

	body, err := s.store.EvolutionMetricsJSON(ctx, aid)
	if err != nil {
		return nil, err
	}
	m, err := jsonMap(body)
	if err != nil {
		return nil, err
	}
	out := &v1.EvolutionMetricsReport{
		EventsTotal:       ifaceI32(m, "events_total"),
		EventsReverted:    ifaceI32(m, "events_reverted"),
		ProposalsTotal:    ifaceI32(m, "proposals_total"),
		ProposalsByStatus: map[string]int32{},
	}
	if raw, ok := m["proposals_by_status"].(map[string]any); ok {
		for k, v := range raw {
			switch t := v.(type) {
			case float64:
				out.ProposalsByStatus[k] = int32(t)
			case int:
				out.ProposalsByStatus[k] = int32(t)
			}
		}
	}
	if rawS, ok := m["skill_stats"].([]any); ok {
		for _, row := range rawS {
			rm, ok := row.(map[string]any)
			if !ok {
				continue
			}
			out.SkillStats = append(out.SkillStats, &v1.AgentSkillStat{
				AgentId:         ifaceStr(rm, "agent_id"),
				ToolKey:         ifaceStr(rm, "tool_key"),
				Invocations:     ifaceI32(rm, "invocations"),
				Successes:       ifaceI32(rm, "successes"),
				Failures:        ifaceI32(rm, "failures"),
				PreferenceScore: ifaceF64(rm, "preference_score"),
				LastUsedAt:      ifaceStr(rm, "last_used_at"),
			})
		}
	}
	return out, nil
}

// UpsertMemoryFact creates or merges an L3 fact row (SQLite).
func (s *MemoryService) UpsertMemoryFact(ctx context.Context, req *v1.UpsertMemoryFactRequest) (*v1.UpsertMemoryFactResponse, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	f := req.GetFact()
	if f == nil {
		return nil, kerrors.BadRequest("MEMORY", "fact is required")
	}
	raw, err := s.store.UpsertFactRow(ctx, sessionmemory.MemoryFactUpsert{
		ID:                       strings.TrimSpace(f.GetId()),
		ScopeType:                strings.TrimSpace(f.GetScopeType()),
		ScopeID:                  strings.TrimSpace(f.GetScopeId()),
		WorkspaceID:              strings.TrimSpace(f.GetWorkspaceId()),
		UserID:                   strings.TrimSpace(f.GetUserId()),
		TeamID:                   strings.TrimSpace(f.GetTeamId()),
		AgentID:                  strings.TrimSpace(f.GetAgentId()),
		Statement:                strings.TrimSpace(f.GetStatement()),
		DetailsMarkdown:          f.GetDetailsMarkdown(),
		FactKind:                 f.GetFactKind(),
		TagsJSON:                 f.GetTagsJson(),
		Confidence:               f.GetConfidence(),
		Importance:               f.GetImportance(),
		UseCount:                 f.GetUseCount(),
		HitCount:                 f.GetHitCount(),
		PositiveFeedbackCount:    f.GetPositiveFeedbackCount(),
		NegativeFeedbackCount:    f.GetNegativeFeedbackCount(),
		ConflictCount:            f.GetConflictCount(),
		SourceKind:               f.GetSourceKind(),
		SourceEpisodeID:          f.GetSourceEpisodeId(),
		SourceSessionID:          f.GetSourceSessionId(),
		SourceMessageID:          f.GetSourceMessageId(),
		Version:                  f.GetVersion(),
		Status:                   f.GetStatus(),
		PIIFlag:                  f.GetPiiFlag(),
		CreatedAt:                strings.TrimSpace(f.GetCreatedAt()),
		UpdatedAt:                strings.TrimSpace(f.GetUpdatedAt()),
	})
	if err != nil {
		return nil, err
	}
	pb, err := pbMemoryFact(raw)
	if err != nil || pb == nil {
		return nil, kerrors.InternalServer("MEMORY", "failed to hydrate fact after upsert")
	}
	return &v1.UpsertMemoryFactResponse{Fact: pb}, nil
}

// AppendEvolutionEvent inserts a timeline row under agent_evolution_events.
func (s *MemoryService) AppendEvolutionEvent(ctx context.Context, req *v1.AppendEvolutionEventRequest) (*v1.AppendEvolutionEventResponse, error) {
	if err := s.errStore(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, kerrors.BadRequest("MEMORY", "agent_id is required")
	}
	raw, err := s.store.InsertEvolutionEventRow(ctx, sessionmemory.EvolutionEventInsert{
		AgentID:       aid,
		WorkspaceID: strings.TrimSpace(req.GetWorkspaceId()),
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
	m, err := jsonMap(raw)
	if err != nil {
		return nil, err
	}
	out := &v1.AppendEvolutionEventResponse{Event: &v1.EvolutionEvent{
		Id:          ifaceStr(m, "id"),
		AgentId:     ifaceStr(m, "agent_id"),
		EventKind:   ifaceStr(m, "event_kind"),
		Kind:        ifaceStr(m, "kind"),
		TargetField: ifaceStr(m, "target_field"),
		Reason:      ifaceStr(m, "reason"),
		Reverted:    ifaceBool(m, "reverted"),
		CreatedAt:   ifaceStr(m, "created_at"),
	}}
	return out, nil
}
