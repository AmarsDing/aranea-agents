package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/runtime_profile/v1"
	"aranea-agents/internal/biz"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

type RuntimeProfileService struct {
	v1.UnimplementedRuntimeProfileServiceServer
	uc *biz.RuntimeProfileUsecase
}

func NewRuntimeProfileService(uc *biz.RuntimeProfileUsecase) *RuntimeProfileService {
	return &RuntimeProfileService{uc: uc}
}

func (s *RuntimeProfileService) ListRuntimeProfiles(ctx context.Context, req *v1.ListRuntimeProfilesRequest) (*v1.ListRuntimeProfilesResponse, error) {
	items, err := s.uc.List(ctx, req.GetAgentId(), req.GetActiveOnly())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.RuntimeProfile, 0, len(items))
	for _, p := range items {
		out = append(out, toProtoRuntimeProfile(p))
	}
	return &v1.ListRuntimeProfilesResponse{Items: out}, nil
}

func (s *RuntimeProfileService) GetRuntimeProfile(ctx context.Context, req *v1.GetRuntimeProfileRequest) (*v1.RuntimeProfile, error) {
	p, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoRuntimeProfile(p), nil
}

func (s *RuntimeProfileService) CreateRuntimeProfile(ctx context.Context, req *v1.CreateRuntimeProfileRequest) (*v1.RuntimeProfile, error) {
	p, err := s.uc.Create(ctx, fromCreateRequest(req))
	if err != nil {
		return nil, err
	}
	return toProtoRuntimeProfile(p), nil
}

func (s *RuntimeProfileService) UpdateRuntimeProfile(ctx context.Context, req *v1.UpdateRuntimeProfileRequest) (*v1.RuntimeProfile, error) {
	p, err := s.uc.Update(ctx, fromUpdateRequest(req))
	if err != nil {
		return nil, err
	}
	return toProtoRuntimeProfile(p), nil
}

func (s *RuntimeProfileService) DeleteRuntimeProfile(ctx context.Context, req *v1.DeleteRuntimeProfileRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *RuntimeProfileService) SetActive(ctx context.Context, req *v1.SetActiveRequest) (*v1.RuntimeProfile, error) {
	p, err := s.uc.SetActive(ctx, req.GetId(), req.GetActive())
	if err != nil {
		return nil, err
	}
	return toProtoRuntimeProfile(p), nil
}

func fromCreateRequest(req *v1.CreateRuntimeProfileRequest) biz.RuntimeProfile {
	p := biz.RuntimeProfile{
		AgentID:     req.GetAgentId(),
		Name:        req.GetName(),
		Description: req.GetDescription(),
		Version:     req.GetVersion(),
		IsActive:    req.GetIsActive(),
		Priority:    int(req.GetPriority()),
	}
	if req.GetPromptConfig() != nil {
		p.PromptConfig = fromProtoPromptConfig(req.GetPromptConfig())
	}
	if req.GetToolPolicy() != nil {
		p.ToolPolicy = fromProtoToolPolicy(req.GetToolPolicy())
	}
	if req.GetSkillPolicy() != nil {
		p.SkillPolicy = fromProtoSkillPolicy(req.GetSkillPolicy())
	}
	if req.GetKnowledgePolicy() != nil {
		p.KnowledgePolicy = fromProtoKnowledgePolicy(req.GetKnowledgePolicy())
	}
	if req.GetWorkspacePolicy() != nil {
		p.WorkspacePolicy = fromProtoWorkspacePolicy(req.GetWorkspacePolicy())
	}
	if req.GetCredentialPolicy() != nil {
		p.CredentialPolicy = fromProtoCredentialPolicy(req.GetCredentialPolicy())
	}
	if req.GetIsolationPolicy() != nil {
		p.IsolationPolicy = fromProtoIsolationPolicy(req.GetIsolationPolicy())
	}
	if req.GetExtraModelConfig() != nil {
		p.ExtraModelConfig = req.GetExtraModelConfig().AsMap()
	}
	return p
}

func fromUpdateRequest(req *v1.UpdateRuntimeProfileRequest) biz.RuntimeProfile {
	p := biz.RuntimeProfile{
		ID:          req.GetId(),
		AgentID:     req.GetAgentId(),
		Name:        req.GetName(),
		Description: req.GetDescription(),
		Version:     req.GetVersion(),
		IsActive:    req.GetIsActive(),
		Priority:    int(req.GetPriority()),
	}
	if req.GetPromptConfig() != nil {
		p.PromptConfig = fromProtoPromptConfig(req.GetPromptConfig())
	}
	if req.GetToolPolicy() != nil {
		p.ToolPolicy = fromProtoToolPolicy(req.GetToolPolicy())
	}
	if req.GetSkillPolicy() != nil {
		p.SkillPolicy = fromProtoSkillPolicy(req.GetSkillPolicy())
	}
	if req.GetKnowledgePolicy() != nil {
		p.KnowledgePolicy = fromProtoKnowledgePolicy(req.GetKnowledgePolicy())
	}
	if req.GetWorkspacePolicy() != nil {
		p.WorkspacePolicy = fromProtoWorkspacePolicy(req.GetWorkspacePolicy())
	}
	if req.GetCredentialPolicy() != nil {
		p.CredentialPolicy = fromProtoCredentialPolicy(req.GetCredentialPolicy())
	}
	if req.GetIsolationPolicy() != nil {
		p.IsolationPolicy = fromProtoIsolationPolicy(req.GetIsolationPolicy())
	}
	if req.GetExtraModelConfig() != nil {
		p.ExtraModelConfig = req.GetExtraModelConfig().AsMap()
	}
	return p
}

func fromProtoPromptConfig(p *v1.PromptConfig) biz.PromptConfig {
	return biz.PromptConfig{
		Instruction:  p.GetInstruction(),
		SystemPrompt: p.GetSystemPrompt(),
	}
}

func fromProtoToolPolicy(p *v1.ToolPolicy) biz.ToolPolicy {
	return biz.ToolPolicy{
		Include:          p.GetInclude(),
		Exclude:          p.GetExclude(),
		ExecutionInclude: p.GetExecutionInclude(),
		ExecutionExclude: p.GetExecutionExclude(),
		ToolSets:         p.GetToolsets(),
		CredentialRefs:   p.GetCredentialRefs(),
	}
}

func fromProtoSkillPolicy(p *v1.SkillPolicy) biz.SkillPolicy {
	return biz.SkillPolicy{
		Include: p.GetInclude(),
		Exclude: p.GetExclude(),
		Roots:   p.GetRoots(),
	}
}

func fromProtoKnowledgePolicy(p *v1.KnowledgePolicy) biz.KnowledgePolicy {
	out := biz.KnowledgePolicy{
		Indexes: p.GetIndexes(),
	}
	if p.GetFilter() != nil {
		out.Filter = p.GetFilter().AsMap()
	}
	return out
}

func fromProtoWorkspacePolicy(p *v1.WorkspacePolicy) biz.WorkspacePolicy {
	return biz.WorkspacePolicy{
		Workdir:      p.GetWorkdir(),
		AllowedRoots: p.GetAllowedRoots(),
	}
}

func fromProtoCredentialPolicy(p *v1.CredentialPolicy) biz.CredentialPolicy {
	return biz.CredentialPolicy{
		AllowedRefs: p.GetAllowedRefs(),
	}
}

func fromProtoIsolationPolicy(p *v1.IsolationPolicy) biz.IsolationPolicy {
	return biz.IsolationPolicy{
		Mode:         p.GetMode(),
		AgentCache:   p.GetAgentCache(),
		ToolSetCache: p.GetToolsetCache(),
		ServiceMode:  p.GetServiceMode(),
	}
}

func toProtoRuntimeProfile(p biz.RuntimeProfile) *v1.RuntimeProfile {
	out := &v1.RuntimeProfile{
		Id:               p.ID,
		AgentId:          p.AgentID,
		Name:             p.Name,
		Description:      p.Description,
		Version:          p.Version,
		IsActive:         p.IsActive,
		Priority:         int32(p.Priority),
		PromptConfig:     toProtoPromptConfig(p.PromptConfig),
		ToolPolicy:       toProtoToolPolicy(p.ToolPolicy),
		SkillPolicy:      toProtoSkillPolicy(p.SkillPolicy),
		KnowledgePolicy:  toProtoKnowledgePolicy(p.KnowledgePolicy),
		WorkspacePolicy:  toProtoWorkspacePolicy(p.WorkspacePolicy),
		CredentialPolicy: toProtoCredentialPolicy(p.CredentialPolicy),
		IsolationPolicy:  toProtoIsolationPolicy(p.IsolationPolicy),
		CreatedAt:        p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        p.UpdatedAt.Format(time.RFC3339),
	}
	if p.ExtraModelConfig != nil {
		if st, err := structpb.NewStruct(p.ExtraModelConfig); err == nil {
			out.ExtraModelConfig = st
		}
	}
	return out
}

func toProtoPromptConfig(p biz.PromptConfig) *v1.PromptConfig {
	return &v1.PromptConfig{
		Instruction:  p.Instruction,
		SystemPrompt: p.SystemPrompt,
	}
}

func toProtoToolPolicy(p biz.ToolPolicy) *v1.ToolPolicy {
	return &v1.ToolPolicy{
		Include:          p.Include,
		Exclude:          p.Exclude,
		ExecutionInclude: p.ExecutionInclude,
		ExecutionExclude: p.ExecutionExclude,
		Toolsets:         p.ToolSets,
		CredentialRefs:   p.CredentialRefs,
	}
}

func toProtoSkillPolicy(p biz.SkillPolicy) *v1.SkillPolicy {
	return &v1.SkillPolicy{
		Include: p.Include,
		Exclude: p.Exclude,
		Roots:   p.Roots,
	}
}

func toProtoKnowledgePolicy(p biz.KnowledgePolicy) *v1.KnowledgePolicy {
	out := &v1.KnowledgePolicy{
		Indexes: p.Indexes,
	}
	if p.Filter != nil {
		if st, err := structpb.NewStruct(p.Filter); err == nil {
			out.Filter = st
		}
	}
	return out
}

func toProtoWorkspacePolicy(p biz.WorkspacePolicy) *v1.WorkspacePolicy {
	return &v1.WorkspacePolicy{
		Workdir:      p.Workdir,
		AllowedRoots: p.AllowedRoots,
	}
}

func toProtoCredentialPolicy(p biz.CredentialPolicy) *v1.CredentialPolicy {
	return &v1.CredentialPolicy{
		AllowedRefs: p.AllowedRefs,
	}
}

func toProtoIsolationPolicy(p biz.IsolationPolicy) *v1.IsolationPolicy {
	return &v1.IsolationPolicy{
		Mode:         p.Mode,
		AgentCache:   p.AgentCache,
		ToolsetCache: p.ToolSetCache,
		ServiceMode:  p.ServiceMode,
	}
}
