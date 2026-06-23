package service

import (
	"context"
	"encoding/json"

	v1 "aranea-agents/api/kratos/skill_evolution_suggestion/v1"
	"aranea-agents/internal/biz"
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
	lg      loggateway.Logger
}

func NewSkillEvolutionSuggestionService(
	uc *biz.SkillIntelligenceUsecase,
	curator *SkillCuratorService,
	sandbox *SandboxRunner,
	lg loggateway.Logger,
) *SkillEvolutionSuggestionService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SkillEvolutionSuggestionService{uc: uc, curator: curator, sandbox: sandbox, lg: lg}
}

func (s *SkillEvolutionSuggestionService) ListSkillEvolutionSuggestions(ctx context.Context, req *v1.ListSkillEvolutionSuggestionsRequest) (*v1.ListSkillEvolutionSuggestionsResponse, error) {
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	status := biz.EvolutionSuggestionStatus(req.GetStatus())
	suggestions, err := s.uc.ListEvolutionSuggestions(ctx, req.GetSkillId(), status, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := s.uc.CountEvolutionSuggestions(ctx, req.GetSkillId(), status)
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
	suggestion, err := s.uc.GetEvolutionSuggestion(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if suggestion == nil {
		return nil, apierror.NotFound("SKILL_EVO_SUGGESTION", "suggestion %s not found", req.GetId())
	}
	return &v1.GetSkillEvolutionSuggestionResponse{
		Suggestion: toProtoEvolutionSuggestion(*suggestion, s.lg),
	}, nil
}

func (s *SkillEvolutionSuggestionService) ApproveSkillEvolutionSuggestion(ctx context.Context, req *v1.ApproveSkillEvolutionSuggestionRequest) (*v1.ApproveSkillEvolutionSuggestionResponse, error) {
	if err := s.uc.ApproveSuggestion(ctx, req.GetId(), req.GetApprovedBy()); err != nil {
		return nil, err
	}

	// B-14 fix: actually run draft generation and sandbox validation
	// asynchronously (previously the comment claimed async but the code was
	// synchronous, blocking the approval response). Use context.WithoutCancel
	// so the background work survives HTTP response delivery.
	suggestionID := req.GetId()
	if s.curator != nil || s.sandbox != nil {
		safego.Go(context.WithoutCancel(ctx), "skill_evo_suggestion.approve.async", func() {
			bgCtx := context.Background()
			if s.curator != nil {
				if _, err := s.curator.GenerateDraft(bgCtx, suggestionID); err != nil {
					s.lg.Warn("ApproveSkillEvolutionSuggestion: GenerateDraft failed",
						loggateway.StepID("skill_evo_suggestion.approve"),
						loggateway.Str("suggestion_id", suggestionID),
						loggateway.Err(err))
				}
			}
			if s.sandbox != nil {
				if _, _, err := s.sandbox.ValidateSuggestion(bgCtx, suggestionID); err != nil {
					s.lg.Warn("ApproveSkillEvolutionSuggestion: ValidateSuggestion failed",
						loggateway.StepID("skill_evo_suggestion.approve"),
						loggateway.Str("suggestion_id", suggestionID),
						loggateway.Err(err))
				}
			}
		})
	}

	return &v1.ApproveSkillEvolutionSuggestionResponse{}, nil
}

func (s *SkillEvolutionSuggestionService) RejectSkillEvolutionSuggestion(ctx context.Context, req *v1.RejectSkillEvolutionSuggestionRequest) (*v1.RejectSkillEvolutionSuggestionResponse, error) {
	if err := s.uc.RejectSuggestion(ctx, req.GetId(), req.GetRejectedBy(), req.GetRejectionReason()); err != nil {
		return nil, err
	}
	return &v1.RejectSkillEvolutionSuggestionResponse{}, nil
}

// TriggerCuratorFlow runs the full Curator Agent semi-automatic evolution
// pipeline for a skill: trigger detection → draft generation → sandbox verification.
func (s *SkillEvolutionSuggestionService) TriggerCuratorFlow(ctx context.Context, req *v1.TriggerCuratorFlowRequest) (*v1.TriggerCuratorFlowResponse, error) {
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
