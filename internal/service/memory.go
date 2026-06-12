package service

import (
	"context"
	"encoding/json"
	"strings"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
)

type queueStatsProvider interface {
	QueueLaneStats() (highLen, normalLen, lowLen int, highCap, normalCap, lowCap int, dropped, debounced int64)
}

// MemoryServiceConfig holds all dependencies for MemoryService.
// Using a config struct instead of positional parameters improves readability
// and makes future additions non-breaking.
type MemoryServiceConfig struct {
	Admin             *biz.MemoryAdminUsecase
	Cascade           *biz.L4CascadeUsecase
	SysUC             *biz.SystemSettingUsecase
	DeadLetterRepo    biz.MemoryDeadLetterAdminRepo
	DebugRecaller     biz.MemoryDebugRecaller
	FactIndexCounter  biz.MemoryFactIndexCounter
	DeadLetterEnqueue func(ctx context.Context, id int64) error
	QueueStats        queueStatsProvider
	Logger            loggateway.Logger
}

type MemoryService struct {
	v1.UnimplementedMemoryServiceServer

	admin             *biz.MemoryAdminUsecase
	cascade           *biz.L4CascadeUsecase
	sysUC             *biz.SystemSettingUsecase
	deadLetterRepo    biz.MemoryDeadLetterAdminRepo
	debugRecaller     biz.MemoryDebugRecaller
	factIndexCounter  biz.MemoryFactIndexCounter
	deadLetterEnqueue func(ctx context.Context, id int64) error
	queueStats        queueStatsProvider
	lg                loggateway.Logger
}

func NewMemoryService(cfg MemoryServiceConfig) *MemoryService {
	return &MemoryService{
		admin:             cfg.Admin,
		cascade:           cfg.Cascade,
		sysUC:             cfg.SysUC,
		deadLetterRepo:    cfg.DeadLetterRepo,
		debugRecaller:     cfg.DebugRecaller,
		factIndexCounter:  cfg.FactIndexCounter,
		deadLetterEnqueue: cfg.DeadLetterEnqueue,
		queueStats:        cfg.QueueStats,
		lg:                cfg.Logger,
	}
}

func (s *MemoryService) requireAdmin() error {
	if s.admin == nil {
		return apierror.Internal("MEMORY", "memory admin usecase not wired")
	}
	return nil
}

func (s *MemoryService) ListL0Snapshots(ctx context.Context, req *v1.ListL0SnapshotsRequest) (*v1.ListL0SnapshotsResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	sid := strings.TrimSpace(req.GetSessionId())
	if sid == "" {
		return nil, apierror.BadRequest("MEMORY", "session_id is required")
	}
	rows, err := s.admin.ListL0SnapshotRows(ctx, sid, strings.TrimSpace(req.GetAgentId()), req.GetLimit())
	if err != nil {
		return nil, err
	}
	out := &v1.ListL0SnapshotsResponse{}
	for _, raw := range rows {
		snap, e := pbL0AssemblySnapshot(raw)
		if e != nil {
			s.lg.Warn("ListL0Snapshots: failed to decode snapshot row",
				loggateway.StepID("memory.decode_fail"),
				loggateway.Err(e))
			continue
		}
		out.Items = append(out.Items, snap)
	}
	return out, nil
}

func (s *MemoryService) ListPIIFlaggedFacts(ctx context.Context, req *v1.ListPIIFlaggedFactsRequest) (*v1.ListPIIFlaggedFactsResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	scopeType := strings.TrimSpace(req.GetScopeType())
	if scopeType == "" {
		return nil, apierror.BadRequest("MEMORY", "scope_type is required")
	}
	rows, total, err := s.admin.ListPIIFlaggedFacts(ctx,
		scopeType,
		strings.TrimSpace(req.GetScopeId()),
		req.GetLimit(),
		req.GetOffset(),
	)
	if err != nil {
		return nil, err
	}
	out := &v1.ListPIIFlaggedFactsResponse{Total: total}
	for _, raw := range rows {
		f, e := pbMemoryFact(raw)
		if e != nil {
			s.lg.Warn("ListPIIFlaggedFacts: failed to decode fact row",
				loggateway.StepID("memory.decode_fail"),
				loggateway.Err(e))
			continue
		}
		if f != nil {
			out.Items = append(out.Items, f)
		}
	}
	return out, nil
}

func (s *MemoryService) ListConflictingFacts(ctx context.Context, req *v1.ListConflictingFactsRequest) (*v1.ListConflictingFactsResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	scopeType := strings.TrimSpace(req.GetScopeType())
	if scopeType == "" {
		return nil, apierror.BadRequest("MEMORY", "scope_type is required")
	}
	rows, total, err := s.admin.ListConflictingFacts(ctx,
		scopeType,
		strings.TrimSpace(req.GetScopeId()),
		req.GetLimit(),
		req.GetOffset(),
	)
	if err != nil {
		return nil, err
	}
	out := &v1.ListConflictingFactsResponse{Total: total}
	for _, raw := range rows {
		f, e := pbMemoryFact(raw)
		if e != nil {
			s.lg.Warn("ListConflictingFacts: failed to decode fact row",
				loggateway.StepID("memory.decode_fail"),
				loggateway.Err(e))
			continue
		}
		if f != nil {
			out.Items = append(out.Items, f)
		}
	}
	return out, nil
}

func (s *MemoryService) ReviewPIIFact(ctx context.Context, req *v1.ReviewPIIFactRequest) (*v1.ReviewPIIFactResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	factID := strings.TrimSpace(req.GetFactId())
	if factID == "" {
		return nil, apierror.BadRequest("MEMORY", "fact_id is required")
	}
	action := strings.TrimSpace(req.GetAction())
	switch action {
	case "approve":
		if err := s.admin.ApprovePIIFact(ctx, factID); err != nil {
			return nil, err
		}
	case "reject":
		if err := s.admin.RejectPIIFact(ctx, factID); err != nil {
			return nil, err
		}
	default:
		return nil, apierror.BadRequest("MEMORY", "action must be 'approve' or 'reject'")
	}
	raw, err := s.admin.GetFactByID(ctx, factID)
	if err != nil {
		return nil, err
	}
	fact, err := pbMemoryFact(raw)
	if err != nil || fact == nil {
		return nil, apierror.Internal("MEMORY", "failed to hydrate fact after review")
	}
	return &v1.ReviewPIIFactResponse{Fact: fact}, nil
}

func (s *MemoryService) ListL1Tasks(ctx context.Context, req *v1.ListL1TasksRequest) (*v1.ListL1TasksResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	sid := strings.TrimSpace(req.GetSessionId())
	if sid == "" {
		return nil, apierror.BadRequest("MEMORY", "session_id is required")
	}
	rows, err := s.admin.ListL1TaskRows(ctx, sid,
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

func (s *MemoryService) ListL1Fields(ctx context.Context, req *v1.ListL1FieldsRequest) (*v1.ListL1FieldsResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	tid := strings.TrimSpace(req.GetTaskId())
	if tid == "" {
		return nil, apierror.BadRequest("MEMORY", "task_id is required")
	}
	includeInternal := strings.TrimSpace(req.GetIncludeInternal()) == "true"
	agentID := strings.TrimSpace(req.GetAgentId())
	rows, err := s.admin.ListL1FieldRows(ctx, tid, includeInternal, agentID)
	if err != nil {
		return nil, err
	}
	out := &v1.ListL1FieldsResponse{}
	for _, raw := range rows {
		m, perr := jsonutil.ParseMap(raw)
		if perr != nil {
			s.lg.Warn("ListL1Fields: failed to parse field row",
				loggateway.StepID("memory.parse_fail"),
				loggateway.Err(perr))
			continue
		}
		out.Items = append(out.Items, &v1.L1Field{
			Id:            jsonutil.IfaceStr(m, "id"),
			TaskId:        jsonutil.IfaceStr(m, "task_id"),
			SessionId:     jsonutil.IfaceStr(m, "session_id"),
			AgentId:       jsonutil.IfaceStr(m, "agent_id"),
			FieldPath:     jsonutil.IfaceStr(m, "field_path"),
			FieldKind:     jsonutil.IfaceStr(m, "field_kind"),
			Visibility:    jsonutil.IfaceStr(m, "visibility"),
			PinToPrompt:   jsonutil.IfaceBool(m, "pin_to_prompt"),
			IsRequired:    jsonutil.IfaceBool(m, "is_required"),
			ValueText:     jsonutil.IfaceStr(m, "value_text"),
			ValueJson:     jsonutil.IfaceStr(m, "value_json"),
			ValueRef:      jsonutil.IfaceStr(m, "value_ref"),
			Preview:       jsonutil.IfaceStr(m, "preview"),
			TokenEstimate: jsonutil.IfaceI32(m, "token_estimate"),
			Source:        jsonutil.IfaceStr(m, "source"),
			SourceRef:     jsonutil.IfaceStr(m, "source_ref"),
			TtlSeconds:    jsonutil.IfaceI32(m, "ttl_seconds"),
			ExpiresAt:     jsonutil.IfaceStr(m, "expires_at"),
			Revision:      jsonutil.IfaceI32(m, "revision"),
			LastReadAt:    jsonutil.IfaceStr(m, "last_read_at"),
			ReadCount:     jsonutil.IfaceI32(m, "read_count"),
			MetadataJson:  jsonutil.IfaceStr(m, "metadata_json"),
			CreatedAt:     jsonutil.IfaceStr(m, "created_at"),
			UpdatedAt:     jsonutil.IfaceStr(m, "updated_at"),
		})
	}
	return out, nil
}

func (s *MemoryService) ListMemoryFacts(ctx context.Context, req *v1.ListMemoryFactsRequest) (*v1.ListMemoryFactsResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	rows, total, lim, off, err := s.admin.ListFactRows(ctx,
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
		if e != nil {
			s.lg.Warn("ListMemoryFacts: failed to decode fact row",
				loggateway.StepID("memory.decode_fail"),
				loggateway.Err(e))
			continue
		}
		if f != nil {
			out.Items = append(out.Items, f)
		}
	}
	return out, nil
}

func (s *MemoryService) ListMemoryEntities(ctx context.Context, req *v1.ListMemoryEntitiesRequest) (*v1.ListMemoryEntitiesResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	rows, total, err := s.admin.ListEntityRows(ctx,
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
		if e != nil {
			s.lg.Warn("ListMemoryEntities: failed to decode entity row",
				loggateway.StepID("memory.decode_fail"),
				loggateway.Err(e))
			continue
		}
		if ent != nil {
			out.Items = append(out.Items, ent)
		}
	}
	return out, nil
}

func (s *MemoryService) GetMemoryNeighborhood(ctx context.Context, req *v1.GetMemoryNeighborhoodRequest) (*v1.GraphNeighborhood, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	cid := strings.TrimSpace(req.GetCenterId())
	if cid == "" {
		return nil, apierror.BadRequest("MEMORY", "center_id is required")
	}
	body, err := s.admin.NeighborhoodJSON(ctx, cid, req.GetHops(), req.GetMaxNodes(), strings.TrimSpace(req.GetQueryAt()))
	if err != nil {
		return nil, err
	}
	top, err := jsonutil.ParseMap(body)
	if err != nil {
		return nil, apierror.Internal("MEMORY", "failed to parse neighborhood: %s", err.Error())
	}
	out := &v1.GraphNeighborhood{
		Hops:    jsonutil.IfaceI32(top, "hops"),
		QueryAt: jsonutil.IfaceStr(top, "query_at"),
	}
	if centerRaw := top["center"]; centerRaw != nil {
		raw, merr := json.Marshal(centerRaw)
		if merr != nil {
			s.lg.Warn("GetMemoryNeighborhood: failed to marshal center",
				loggateway.StepID("memory.marshal_fail"),
				loggateway.Err(merr))
		} else {
			c, _ := pbMemoryEntity(raw)
			out.Center = c
		}
	}
	if entitiesRaw, ok := top["entities"].([]any); ok {
		for _, row := range entitiesRaw {
			raw, merr := json.Marshal(row)
			if merr != nil {
				continue
			}
			e, _ := pbMemoryEntity(raw)
			if e != nil {
				out.Entities = append(out.Entities, e)
			}
		}
	}
	if relationsRaw, ok := top["relations"].([]any); ok {
		for _, row := range relationsRaw {
			raw, merr := json.Marshal(row)
			if merr != nil {
				continue
			}
			r, _ := pbMemoryRelation(raw)
			if r != nil {
				out.Relations = append(out.Relations, r)
			}
		}
	}
	return out, nil
}

func (s *MemoryService) ListCascadeProposals(ctx context.Context, req *v1.ListCascadeProposalsRequest) (*v1.ListCascadeProposalsResponse, error) {
	if s.cascade == nil {
		return nil, apierror.Internal("MEMORY", "cascade store not wired")
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id is required")
	}
	rows, err := s.cascade.ListRows(ctx, aid, strings.TrimSpace(req.GetStatus()), req.GetLimit())
	if err != nil {
		return nil, err
	}
	out := &v1.ListCascadeProposalsResponse{}
	for _, raw := range rows {
		p, e := pbCascadeProposal(raw)
		if e == nil && p != nil {
			out.Items = append(out.Items, p)
		}
	}
	return out, nil
}

func (s *MemoryService) ApproveCascadeProposal(ctx context.Context, req *v1.ApproveCascadeProposalRequest) (*v1.ApproveCascadeProposalResponse, error) {
	if s.cascade == nil {
		return nil, apierror.Internal("MEMORY", "cascade store not wired")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest("MEMORY", "id is required")
	}
	raw, err := s.cascade.Approve(ctx, id, strings.TrimSpace(req.GetReviewer()))
	if err != nil {
		return nil, err
	}
	p, err := pbCascadeProposal(raw)
	if err != nil || p == nil {
		return nil, apierror.Internal("MEMORY", "failed to hydrate cascade proposal")
	}
	return &v1.ApproveCascadeProposalResponse{Proposal: p}, nil
}

func (s *MemoryService) RejectCascadeProposal(ctx context.Context, req *v1.RejectCascadeProposalRequest) (*v1.RejectCascadeProposalResponse, error) {
	if s.cascade == nil {
		return nil, apierror.Internal("MEMORY", "cascade store not wired")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest("MEMORY", "id is required")
	}
	raw, err := s.cascade.Reject(ctx, id, strings.TrimSpace(req.GetReviewer()), strings.TrimSpace(req.GetReason()))
	if err != nil {
		return nil, err
	}
	p, err := pbCascadeProposal(raw)
	if err != nil || p == nil {
		return nil, apierror.Internal("MEMORY", "failed to hydrate cascade proposal")
	}
	return &v1.RejectCascadeProposalResponse{Proposal: p}, nil
}

func (s *MemoryService) PreviewCascadeApprove(ctx context.Context, req *v1.PreviewCascadeApproveRequest) (*v1.PreviewCascadeApproveResponse, error) {
	if s.cascade == nil {
		return nil, apierror.Internal("MEMORY", "cascade store not wired")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest("MEMORY", "id is required")
	}
	preview, err := s.cascade.Preview(ctx, id)
	if err != nil {
		return nil, err
	}
	pb := &v1.CascadePreview{
		AffectedEntitiesCount: int32(preview.AffectedEntitiesCount),
		AffectedFactsCount:    int32(preview.AffectedFactsCount),
	}
	for _, d := range preview.FactDiffs {
		pb.FactDiffs = append(pb.FactDiffs, &v1.CascadeFactDiff{
			FactId:          d.FactID,
			BeforeStatement: d.BeforeStatement,
			AfterStatement:  d.AfterStatement,
			Scope:           d.Scope,
		})
	}
	for _, r := range preview.EntityRenames {
		pb.EntityRenames = append(pb.EntityRenames, &v1.CascadeEntityRename{
			EntityId:   r.EntityID,
			EntityType: r.EntityType,
			OldName:    r.OldName,
			NewName:    r.NewName,
		})
	}
	return &v1.PreviewCascadeApproveResponse{Preview: pb}, nil
}

func (s *MemoryService) GetCascadeSagaSteps(ctx context.Context, req *v1.GetCascadeSagaStepsRequest) (*v1.GetCascadeSagaStepsResponse, error) {
	if s.cascade == nil {
		return nil, apierror.Internal("MEMORY", "cascade store not wired")
	}
	proposalID := strings.TrimSpace(req.GetProposalId())
	if proposalID == "" {
		return nil, apierror.BadRequest("MEMORY", "proposal_id is required")
	}
	steps, err := s.cascade.GetSagaSteps(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	out := &v1.GetCascadeSagaStepsResponse{}
	for _, step := range steps {
		out.Steps = append(out.Steps, &v1.CascadeSagaStep{
			Id:          step.ID,
			ProposalId:  step.ProposalID,
			StepIndex:   int32(step.StepIndex),
			StepName:    step.StepName,
			State:       step.State,
			IsCritical:  step.IsCritical,
			Attempts:    int32(step.Attempts),
			StartedAt:   step.StartedAt,
			FinishedAt:  step.FinishedAt,
			PayloadJson: step.PayloadJSON,
			ResultJson:  step.ResultJSON,
			Error:       step.Error,
		})
	}
	return out, nil
}

func (s *MemoryService) RetryCascadeApprove(ctx context.Context, req *v1.RetryCascadeApproveRequest) (*v1.RetryCascadeApproveResponse, error) {
	if s.cascade == nil {
		return nil, apierror.Internal("MEMORY", "cascade store not wired")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest("MEMORY", "id is required")
	}
	raw, err := s.cascade.Retry(ctx, id, strings.TrimSpace(req.GetReviewer()))
	if err != nil {
		return nil, err
	}
	p, err := pbCascadeProposal(raw)
	if err != nil || p == nil {
		return nil, apierror.Internal("MEMORY", "failed to hydrate cascade proposal")
	}
	return &v1.RetryCascadeApproveResponse{Proposal: p}, nil
}

func (s *MemoryService) CompensateCascadeApprove(ctx context.Context, req *v1.CompensateCascadeApproveRequest) (*v1.CompensateCascadeApproveResponse, error) {
	if s.cascade == nil {
		return nil, apierror.Internal("MEMORY", "cascade store not wired")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest("MEMORY", "id is required")
	}
	raw, err := s.cascade.Compensate(ctx, id, strings.TrimSpace(req.GetReviewer()))
	if err != nil {
		return nil, err
	}
	p, err := pbCascadeProposal(raw)
	if err != nil || p == nil {
		return nil, apierror.Internal("MEMORY", "failed to hydrate cascade proposal")
	}
	return &v1.CompensateCascadeApproveResponse{Proposal: p}, nil
}

func pbCascadeProposal(raw []byte) (*v1.CascadeProposal, error) {
	m, err := jsonutil.ParseMap(raw)
	if err != nil {
		return nil, err
	}
	out := &v1.CascadeProposal{
		Id:                jsonutil.IfaceStr(m, "id"),
		AgentId:           jsonutil.IfaceStr(m, "agent_id"),
		WorkspaceId:       jsonutil.IfaceStr(m, "workspace_id"),
		TriggerEntityId:   jsonutil.IfaceStr(m, "trigger_entity_id"),
		TriggerEntityName: jsonutil.IfaceStr(m, "trigger_entity_name"),
		TriggerAttribute:  jsonutil.IfaceStr(m, "trigger_attribute"),
		OldValue:          jsonutil.IfaceStr(m, "old_value"),
		NewValue:          jsonutil.IfaceStr(m, "new_value"),
		Status:            jsonutil.IfaceStr(m, "status"),
		RiskLevel:         jsonutil.IfaceStr(m, "risk_level"),
		Rationale:         jsonutil.IfaceStr(m, "rationale"),
		ReviewedBy:        jsonutil.IfaceStr(m, "reviewed_by"),
		ReviewedAt:        jsonutil.IfaceStr(m, "reviewed_at"),
		ExpiresAt:         jsonutil.IfaceStr(m, "expires_at"),
		CreatedAt:         jsonutil.IfaceStr(m, "created_at"),
		UpdatedAt:         jsonutil.IfaceStr(m, "updated_at"),
	}
	affectedRaw := jsonutil.IfaceStr(m, "affected_json")
	if affectedRaw != "" {
		var rows []map[string]any
		if err := json.Unmarshal([]byte(affectedRaw), &rows); err == nil {
			for _, row := range rows {
				out.AffectedEntities = append(out.AffectedEntities, &v1.CascadeAffectedEntity{
					EntityId:     jsonutil.IfaceStr(row, "entity_id"),
					EntityName:   jsonutil.IfaceStr(row, "entity_name"),
					EntityType:   jsonutil.IfaceStr(row, "entity_type"),
					RelationType: jsonutil.IfaceStr(row, "relation_type"),
					Hops:         jsonutil.IfaceI32(row, "hops"),
				})
			}
		}
	}
	return out, nil
}

func (s *MemoryService) GetAgentIdentity(ctx context.Context, req *v1.GetAgentIdentityRequest) (*v1.AgentIdentity, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id is required")
	}
	body, err := s.admin.AgentIdentityJSON(ctx, aid)
	if err != nil {
		return nil, err
	}
	m, err := jsonutil.ParseMap(body)
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
		AgentId:          jsonutil.IfaceStr(m, "agent_id"),
		Persona:          jsonutil.IfaceStr(m, "persona"),
		Values:           vals,
		Tone:             jsonutil.IfaceStr(m, "tone"),
		Domains:          doms,
		UserExpectations: jsonutil.IfaceStr(m, "user_expectations"),
		CurrentPhase:     jsonutil.IfaceStr(m, "current_phase"),
		Version:          jsonutil.IfaceI32(m, "version"),
	}, nil
}

func (s *MemoryService) GetAgentStrategy(ctx context.Context, req *v1.GetAgentStrategyRequest) (*v1.AgentStrategyProfile, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id is required")
	}
	body, err := s.admin.AgentStrategyJSON(ctx, aid)
	if err != nil {
		return nil, err
	}
	m, err := jsonutil.ParseMap(body)
	if err != nil {
		return nil, err
	}
	out := &v1.AgentStrategyProfile{
		AgentId:            jsonutil.IfaceStr(m, "agent_id"),
		Exploration:        jsonutil.IfaceF64(m, "exploration"),
		Conciseness:        jsonutil.IfaceF64(m, "conciseness"),
		Caution:            jsonutil.IfaceF64(m, "caution"),
		Delegation:         jsonutil.IfaceF64(m, "delegation"),
		Version:            jsonutil.IfaceI32(m, "version"),
		ToolPreference:     map[string]float64{},
		ProviderPreference: map[string]float64{},
		ModelPreference:    map[string]float64{},
	}
	if raw, ok := m["tool_preference"].(map[string]any); ok {
		out.ToolPreference = jsonutil.MapStringFloat(raw)
	}
	if raw, ok := m["provider_preference"].(map[string]any); ok {
		out.ProviderPreference = jsonutil.MapStringFloat(raw)
	}
	if raw, ok := m["model_preference"].(map[string]any); ok {
		out.ModelPreference = jsonutil.MapStringFloat(raw)
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

func (s *MemoryService) ListEvolutionProposals(ctx context.Context, req *v1.ListEvolutionProposalsRequest) (*v1.ListEvolutionProposalsResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id is required")
	}
	rows, err := s.admin.EvolutionProposalRows(ctx, aid, strings.TrimSpace(req.GetStatus()), req.GetLimit())
	if err != nil {
		return nil, err
	}
	out := &v1.ListEvolutionProposalsResponse{}
	for _, raw := range rows {
		m, perr := jsonutil.ParseMap(raw)
		if perr != nil {
			s.lg.Warn("ListEvolutionProposals: failed to parse proposal row",
				loggateway.StepID("memory.parse_fail"),
				loggateway.Err(perr))
			continue
		}
		out.Items = append(out.Items, &v1.EvolutionProposal{
			Id:             jsonutil.IfaceStr(m, "id"),
			AgentId:        jsonutil.IfaceStr(m, "agent_id"),
			ProposalKind:   jsonutil.IfaceStr(m, "proposal_kind"),
			Kind:           jsonutil.IfaceStr(m, "kind"),
			TargetField:    jsonutil.IfaceStr(m, "target_field"),
			Rationale:      jsonutil.IfaceStr(m, "rationale"),
			ExpectedImpact: jsonutil.IfaceStr(m, "expected_impact"),
			RiskLevel:      jsonutil.IfaceStr(m, "risk_level"),
			Status:         jsonutil.IfaceStr(m, "status"),
			CreatedAt:      jsonutil.IfaceStr(m, "created_at"),
		})
	}
	return out, nil
}

func (s *MemoryService) ListEvolutionEvents(ctx context.Context, req *v1.ListEvolutionEventsRequest) (*v1.ListEvolutionEventsResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id is required")
	}
	rows, err := s.admin.EvolutionEventRows(ctx, aid, req.GetLimit())
	if err != nil {
		return nil, err
	}
	out := &v1.ListEvolutionEventsResponse{}
	for _, raw := range rows {
		m, perr := jsonutil.ParseMap(raw)
		if perr != nil {
			s.lg.Warn("ListEvolutionEvents: failed to parse event row",
				loggateway.StepID("memory.parse_fail"),
				loggateway.Err(perr))
			continue
		}
		out.Items = append(out.Items, &v1.EvolutionEvent{
			Id:          jsonutil.IfaceStr(m, "id"),
			AgentId:     jsonutil.IfaceStr(m, "agent_id"),
			EventKind:   jsonutil.IfaceStr(m, "event_kind"),
			Kind:        jsonutil.IfaceStr(m, "kind"),
			TargetField: jsonutil.IfaceStr(m, "target_field"),
			Reason:      jsonutil.IfaceStr(m, "reason"),
			Reverted:    jsonutil.IfaceBool(m, "reverted"),
			CreatedAt:   jsonutil.IfaceStr(m, "created_at"),
		})
	}
	return out, nil
}

func (s *MemoryService) GetEvolutionMetrics(ctx context.Context, req *v1.GetEvolutionMetricsRequest) (*v1.EvolutionMetricsReport, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id is required")
	}
	timeRange := strings.TrimSpace(req.GetRange())

	body, err := s.admin.EvolutionMetricsJSON(ctx, aid, timeRange)
	if err != nil {
		return nil, err
	}
	m, err := jsonutil.ParseMap(body)
	if err != nil {
		return nil, err
	}
	out := &v1.EvolutionMetricsReport{
		EventsTotal:       jsonutil.IfaceI32(m, "events_total"),
		EventsReverted:    jsonutil.IfaceI32(m, "events_reverted"),
		ProposalsTotal:    jsonutil.IfaceI32(m, "proposals_total"),
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
				AgentId:         jsonutil.IfaceStr(rm, "agent_id"),
				ToolKey:         jsonutil.IfaceStr(rm, "tool_key"),
				Invocations:     jsonutil.IfaceI32(rm, "invocations"),
				Successes:       jsonutil.IfaceI32(rm, "successes"),
				Failures:        jsonutil.IfaceI32(rm, "failures"),
				PreferenceScore: jsonutil.IfaceF64(rm, "preference_score"),
				LastUsedAt:      jsonutil.IfaceStr(rm, "last_used_at"),
			})
		}
	}
	return out, nil
}

func (s *MemoryService) UpsertMemoryFact(ctx context.Context, req *v1.UpsertMemoryFactRequest) (*v1.UpsertMemoryFactResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	f := req.GetFact()
	if f == nil {
		return nil, apierror.BadRequest("MEMORY", "fact is required")
	}
	raw, err := s.admin.UpsertFactRow(ctx, biz.FactUpsert{
		ID:                    strings.TrimSpace(f.GetId()),
		ScopeType:             strings.TrimSpace(f.GetScopeType()),
		ScopeID:               strings.TrimSpace(f.GetScopeId()),
		WorkspaceID:           strings.TrimSpace(f.GetWorkspaceId()),
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
		CreatedAt:             strings.TrimSpace(f.GetCreatedAt()),
		UpdatedAt:             strings.TrimSpace(f.GetUpdatedAt()),
	})
	if err != nil {
		return nil, err
	}
	pb, err := pbMemoryFact(raw)
	if err != nil || pb == nil {
		return nil, apierror.Internal("MEMORY", "failed to hydrate fact after upsert")
	}
	return &v1.UpsertMemoryFactResponse{Fact: pb}, nil
}

func (s *MemoryService) AppendEvolutionEvent(ctx context.Context, req *v1.AppendEvolutionEventRequest) (*v1.AppendEvolutionEventResponse, error) {
	if err := s.requireAdmin(); err != nil {
		return nil, err
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, apierror.BadRequest("MEMORY", "agent_id is required")
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
