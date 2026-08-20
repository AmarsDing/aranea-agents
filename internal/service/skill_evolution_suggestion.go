package service

import (
	"context"
	"encoding/json"
	"strings"

	v1 "aranea-agents/api/kratos/skill_evolution_suggestion/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	structpb "google.golang.org/protobuf/types/known/structpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

// SkillEvolutionSuggestionService implements the proto-generated
// SkillEvolutionSuggestionServiceServer interface.
type SkillEvolutionSuggestionService struct {
	v1.UnimplementedSkillEvolutionSuggestionServiceServer

	uc      *biz.SkillIntelligenceUsecase
	curator *SkillCuratorService
	sandbox *SandboxRunner
	// skillUC 用于 P0-1c IDOR 断言（读取宿主 skill 归属 workspace）。
	skillUC *biz.SkillUsecase
	// agentUC 用于 ADR-3 agent 维度建议的 IDOR 断言（读取宿主 agent 归属 workspace）。
	agentUC *biz.AgentUsecase
	lg      loggateway.Logger
}

func NewSkillEvolutionSuggestionService(
	uc *biz.SkillIntelligenceUsecase,
	curator *SkillCuratorService,
	sandbox *SandboxRunner,
	skillUC *biz.SkillUsecase,
	agentUC *biz.AgentUsecase,
	lg loggateway.Logger,
) *SkillEvolutionSuggestionService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SkillEvolutionSuggestionService{uc: uc, curator: curator, sandbox: sandbox, skillUC: skillUC, agentUC: agentUC, lg: lg}
}

// assertSkillAccess 校验 caller 是否可访问指定 skill（P0-1c IDOR 防护）。
// 语义与 SkillService.assertSkillAccess 一致：跨租户访问返回 NotFound
// （避免泄露 skill 存在性）；系统 caller（cron/admin）绕过；空 workspace_id
// 的 skill 视为全局共享。
func (s *SkillEvolutionSuggestionService) assertSkillAccess(ctx context.Context, skillID string) error {
	if skillID == "" {
		return nil
	}
	sk, err := s.skillUC.Get(ctx, skillID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound("SKILL", "skill not found")
		}
		return err
	}
	if err := workspace.AssertWorkspaceOrShared(workspace.IDFromContext(ctx), sk.WorkspaceID); err != nil {
		s.lg.Warn("skill evolution suggestion access denied: workspace mismatch",
			loggateway.StepID("skill_evo_suggestion.idor"),
			loggateway.Str("skill_id", skillID),
			loggateway.Str("caller_ws", workspace.IDFromContext(ctx)))
		return apierror.NotFound("SKILL", "skill not found")
	}
	return nil
}

// assertAgentAccess 校验 caller 是否可访问指定 agent（ADR-3 agent 维度 IDOR 防护）。
// 语义与 AgentService.assertAgentAccess 一致：跨租户访问返回 NotFound；系统
// caller 绕过；空 workspace_id 的 agent 视为全局共享。agentUC 未注入时（测试
// 装配）fail-closed 返回 NotFound。
func (s *SkillEvolutionSuggestionService) assertAgentAccess(ctx context.Context, agentID string) error {
	if agentID == "" {
		return nil
	}
	if s.agentUC == nil {
		return apierror.NotFound(apierror.DomainAgent, "agent not found")
	}
	a, err := s.agentUC.Get(ctx, agentID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return err
	}
	if err := workspace.AssertWorkspaceOrShared(workspace.IDFromContext(ctx), a.WorkspaceID); err != nil {
		s.lg.Warn("agent evolution suggestion access denied: workspace mismatch",
			loggateway.StepID("skill_evo_suggestion.idor"),
			loggateway.Str("agent_id", agentID),
			loggateway.Str("caller_ws", workspace.IDFromContext(ctx)))
		return apierror.NotFound(apierror.DomainAgent, "agent not found")
	}
	return nil
}

// assertTargetAccess 按建议的 target 维度分派 IDOR 断言（ADR-3）。
// 空 TargetType（历史行）按 skill 维度处理。
func (s *SkillEvolutionSuggestionService) assertTargetAccess(ctx context.Context, suggestion *biz.SkillEvolutionSuggestion) error {
	if suggestion.TargetType == string(biz.EvolutionTargetAgent) {
		return s.assertAgentAccess(ctx, suggestion.TargetID)
	}
	return s.assertSkillAccess(ctx, suggestion.SkillID)
}

// assertSuggestionAccess 读取建议并断言 caller 可访问其宿主 target。
// 建议不存在或跨租户均返回 NotFound。
func (s *SkillEvolutionSuggestionService) assertSuggestionAccess(ctx context.Context, suggestionID string) (*biz.SkillEvolutionSuggestion, error) {
	suggestion, err := s.uc.GetEvolutionSuggestion(ctx, suggestionID)
	if err != nil {
		return nil, err
	}
	if suggestion == nil {
		return nil, apierror.NotFound("SKILL_EVO_SUGGESTION", "suggestion %s not found", suggestionID)
	}
	if err := s.assertTargetAccess(ctx, suggestion); err != nil {
		return nil, err
	}
	return suggestion, nil
}

func (s *SkillEvolutionSuggestionService) ListSkillEvolutionSuggestions(ctx context.Context, req *v1.ListSkillEvolutionSuggestionsRequest) (*v1.ListSkillEvolutionSuggestionsResponse, error) {
	// ADR-3: target_type/target_id 为泛化入口；skill_id 等价于
	// target_type=skill + target_id=skill_id（向后兼容）。
	targetType := req.GetTargetType()
	targetID := req.GetTargetId()
	if targetType == "" {
		targetType = "skill"
		if req.GetSkillId() != "" && targetID == "" {
			targetID = req.GetSkillId()
		}
	}
	if targetType != "skill" && targetType != "agent" {
		return nil, apierror.BadRequest("SKILL_EVO_SUGGESTION", "invalid target_type: %s", targetType)
	}
	// IDOR：指定具体 target 时断言其归属；空 targetID（列全部）由 biz 的
	// workspace 过滤兜底（evolutionCallerWorkspace）。
	if targetType == "agent" {
		if err := s.assertAgentAccess(ctx, targetID); err != nil {
			return nil, err
		}
	} else if err := s.assertSkillAccess(ctx, targetID); err != nil {
		return nil, err
	}
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	status := biz.EvolutionSuggestionStatus(req.GetStatus())
	suggestions, err := s.uc.ListEvolutionSuggestions(ctx, targetType, targetID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := s.uc.CountEvolutionSuggestions(ctx, targetType, targetID, status)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListSkillEvolutionSuggestionsResponse{
		Total:    int64(total),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range suggestions {
		resp.Items = append(resp.Items, toProtoEvolutionSuggestion(suggestions[i], s.lg))
	}
	return resp, nil
}

func (s *SkillEvolutionSuggestionService) GetSkillEvolutionSuggestion(ctx context.Context, req *v1.GetSkillEvolutionSuggestionRequest) (*v1.GetSkillEvolutionSuggestionResponse, error) {
	suggestion, err := s.assertSuggestionAccess(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &v1.GetSkillEvolutionSuggestionResponse{
		Suggestion: toProtoEvolutionSuggestion(*suggestion, s.lg),
	}, nil
}

func (s *SkillEvolutionSuggestionService) ApproveSkillEvolutionSuggestion(ctx context.Context, req *v1.ApproveSkillEvolutionSuggestionRequest) (*v1.ApproveSkillEvolutionSuggestionResponse, error) {
	if _, err := s.assertSuggestionAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	if err := s.uc.ApproveSuggestion(ctx, req.GetId(), req.GetApprovedBy()); err != nil {
		return nil, err
	}

	// B-14 fix: actually run draft generation and sandbox validation
	// asynchronously (previously the comment claimed async but the code was
	// synchronous, blocking the approval response). Use context.WithoutCancel
	// so the background work survives HTTP response delivery.
	suggestionID := req.GetId()
	if s.curator != nil || s.sandbox != nil {
		bgCtx := context.WithoutCancel(ctx)
		safego.Go(bgCtx, "skill_evo_suggestion.approve.async", func() {
			s.runPostApproveAsync(bgCtx, suggestionID)
		})
	}

	return &v1.ApproveSkillEvolutionSuggestionResponse{}, nil
}

// runPostApproveAsync runs the post-approval pipeline: draft generation (only
// when the suggestion has no draft yet) → sandbox validation → apply.
// F6 (P-evo-2): an existing draft is the one the approver reviewed — it must
// be frozen, never regenerated and overwritten by a second LLM pass.
func (s *SkillEvolutionSuggestionService) runPostApproveAsync(ctx context.Context, suggestionID string) {
	if s.curator != nil && !s.suggestionHasDraft(ctx, suggestionID) {
		if _, err := s.curator.GenerateDraft(ctx, suggestionID); err != nil {
			s.lg.Warn("ApproveSkillEvolutionSuggestion: GenerateDraft failed",
				loggateway.StepID("skill_evo_suggestion.approve"),
				loggateway.Str("suggestion_id", suggestionID),
				loggateway.Err(err))
		}
	}
	if s.sandbox != nil {
		if _, _, err := s.sandbox.ValidateSuggestion(ctx, suggestionID); err != nil {
			s.lg.Warn("ApproveSkillEvolutionSuggestion: ValidateSuggestion failed",
				loggateway.StepID("skill_evo_suggestion.approve"),
				loggateway.Str("suggestion_id", suggestionID),
				loggateway.Err(err))
		}
	}
	// P0 Reload stage: guarded no-op unless the suggestion is
	// approved + lifecycle=ready + sandbox_passed with a draft.
	if err := s.uc.ApplyApprovedSuggestion(ctx, suggestionID); err != nil {
		s.lg.Warn("ApproveSkillEvolutionSuggestion: ApplyApprovedSuggestion failed",
			loggateway.StepID("skill_evo_suggestion.approve"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
	}
}

// suggestionHasDraft reports whether the suggestion currently carries a
// non-empty draft body. Read failures degrade to false (generate as before).
func (s *SkillEvolutionSuggestionService) suggestionHasDraft(ctx context.Context, suggestionID string) bool {
	sug, err := s.uc.GetEvolutionSuggestion(ctx, suggestionID)
	if err != nil {
		s.lg.Warn("ApproveSkillEvolutionSuggestion: GetEvolutionSuggestion failed",
			loggateway.StepID("skill_evo_suggestion.approve"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
		return false
	}
	return sug != nil && strings.TrimSpace(sug.DraftSkillBody) != ""
}

func (s *SkillEvolutionSuggestionService) RejectSkillEvolutionSuggestion(ctx context.Context, req *v1.RejectSkillEvolutionSuggestionRequest) (*v1.RejectSkillEvolutionSuggestionResponse, error) {
	if _, err := s.assertSuggestionAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	if err := s.uc.RejectSuggestion(ctx, req.GetId(), req.GetRejectedBy(), req.GetRejectionReason()); err != nil {
		return nil, err
	}
	return &v1.RejectSkillEvolutionSuggestionResponse{}, nil
}

// TriggerCuratorFlow runs the full Curator Agent semi-automatic evolution
// pipeline for a skill: trigger detection → draft generation → sandbox verification.
func (s *SkillEvolutionSuggestionService) TriggerCuratorFlow(ctx context.Context, req *v1.TriggerCuratorFlowRequest) (*v1.TriggerCuratorFlowResponse, error) {
	if err := s.assertSkillAccess(ctx, req.GetSkillId()); err != nil {
		return nil, err
	}
	if s.curator == nil {
		return nil, apierror.Unavailable("SKILL_EVO_SUGGESTION", "curator service not available")
	}

	suggestion, err := s.curator.RunCuratorFlow(ctx, req.GetSkillId())
	if err != nil {
		return nil, err
	}
	if suggestion == nil {
		return &v1.TriggerCuratorFlowResponse{}, nil
	}
	return &v1.TriggerCuratorFlowResponse{
		Suggestion: toProtoEvolutionSuggestion(*suggestion, s.lg),
	}, nil
}

// ── Proto conversion helpers ──────────────────────────────────────────────────

func toProtoEvolutionSuggestion(s biz.SkillEvolutionSuggestion, lg loggateway.Logger) *v1.SkillEvolutionSuggestionMsg {
	pb := &v1.SkillEvolutionSuggestionMsg{
		Id:              s.ID,
		SkillId:         s.SkillID,
		Type:            string(s.Type),
		Status:          string(s.Status),
		SourceReportIds: s.SourceReportIDs,
		TriggerReason:   s.TriggerReason,
		DraftSkillBody:  s.DraftSkillBody,
		DraftVersionId:  s.DraftVersionID,
		SandboxPassed:   s.SandboxPassed,
		ApprovedBy:      s.ApprovedBy,
		RejectedBy:      s.RejectedBy,
		RejectionReason: s.RejectionReason,
		CreatedAt:       timestamppb.New(s.CreatedAt),
		ParentVersionId: s.ParentVersionID,
		EvolutionReason: s.EvolutionReason,
		LifecycleStatus: string(s.LifecycleStatus),
		DraftOrigin:     s.DraftOrigin,
		TargetType:      s.TargetType,
		TargetId:        s.TargetID,
		DraftName:       s.DraftName,
	}
	if s.SandboxResult != nil {
		var m map[string]interface{}
		if err := json.Unmarshal(s.SandboxResult, &m); err != nil {
			lg.Warn("toProtoEvolutionSuggestion: unmarshal sandbox_result failed",
				loggateway.StepID("skill_evo_suggestion"),
				loggateway.Str("suggestion_id", s.ID),
				loggateway.Err(err))
		} else if st, err := structpb.NewStruct(m); err != nil {
			lg.Warn("toProtoEvolutionSuggestion: structpb.NewStruct sandbox_result failed",
				loggateway.StepID("skill_evo_suggestion"),
				loggateway.Str("suggestion_id", s.ID),
				loggateway.Err(err))
		} else {
			pb.SandboxResult = st
		}
	}
	if s.PreVerifyResult != nil {
		var m map[string]interface{}
		if err := json.Unmarshal(s.PreVerifyResult, &m); err != nil {
			lg.Warn("toProtoEvolutionSuggestion: unmarshal pre_verify_result failed",
				loggateway.StepID("skill_evo_suggestion"),
				loggateway.Str("suggestion_id", s.ID),
				loggateway.Err(err))
		} else if st, err := structpb.NewStruct(m); err != nil {
			lg.Warn("toProtoEvolutionSuggestion: structpb.NewStruct pre_verify_result failed",
				loggateway.StepID("skill_evo_suggestion"),
				loggateway.Str("suggestion_id", s.ID),
				loggateway.Err(err))
		} else {
			pb.PreVerifyResult = st
		}
	}
	if s.ResolvedAt != nil {
		pb.ResolvedAt = timestamppb.New(*s.ResolvedAt)
	}
	return pb
}
