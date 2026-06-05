package service

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "aranea-agents/api/kratos/skill_evolution_suggestion/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	structpb "google.golang.org/protobuf/types/known/structpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

// SkillEvolutionSuggestionService implements the proto-generated
// SkillEvolutionSuggestionServiceServer interface.
type SkillEvolutionSuggestionService struct {
	v1.UnimplementedSkillEvolutionSuggestionServiceServer

	uc       *biz.SkillIntelligenceUsecase
	curator  *SkillCuratorService
	sandbox  *SandboxRunner
	lg       loggateway.Logger
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
	suggestions, err := s.uc.ListEvolutionSuggestions(ctx, req.GetSkillId(), biz.EvolutionSuggestionStatus(req.GetStatus()), limit, offset)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListSkillEvolutionSuggestionsResponse{
		Total:    int64(len(suggestions)),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range suggestions {
		resp.Items = append(resp.Items, toProtoEvolutionSuggestion(suggestions[i]))
	}
	return resp, nil
}

func (s *SkillEvolutionSuggestionService) GetSkillEvolutionSuggestion(ctx context.Context, req *v1.GetSkillEvolutionSuggestionRequest) (*v1.GetSkillEvolutionSuggestionResponse, error) {
	suggestion, err := s.uc.GetEvolutionSuggestion(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if suggestion == nil {
		return nil, kerrors.NotFound("SKILL_EVO_SUGGESTION", fmt.Sprintf("suggestion %s not found", req.GetId()))
	}
	return &v1.GetSkillEvolutionSuggestionResponse{
		Suggestion: toProtoEvolutionSuggestion(*suggestion),
	}, nil
}

func (s *SkillEvolutionSuggestionService) ApproveSkillEvolutionSuggestion(ctx context.Context, req *v1.ApproveSkillEvolutionSuggestionRequest) (*v1.ApproveSkillEvolutionSuggestionResponse, error) {
	if err := s.uc.ApproveSuggestion(ctx, req.GetId(), req.GetApprovedBy()); err != nil {
		return nil, err
	}

	// After approval, generate draft and run sandbox validation asynchronously.
	// Errors are logged but not blocking the approval response.
	if s.curator != nil {
		if _, err := s.curator.GenerateDraft(ctx, req.GetId()); err != nil {
			s.lg.Warn("ApproveSkillEvolutionSuggestion: GenerateDraft failed",
				loggateway.StepID("skill_evo_suggestion.approve"),
				loggateway.Str("suggestion_id", req.GetId()),
				loggateway.Err(err))
		}
	}
	if s.sandbox != nil {
		if _, _, err := s.sandbox.ValidateSuggestion(ctx, req.GetId()); err != nil {
			s.lg.Warn("ApproveSkillEvolutionSuggestion: ValidateSuggestion failed",
				loggateway.StepID("skill_evo_suggestion.approve"),
				loggateway.Str("suggestion_id", req.GetId()),
				loggateway.Err(err))
		}
	}

	return &v1.ApproveSkillEvolutionSuggestionResponse{}, nil
}

func (s *SkillEvolutionSuggestionService) RejectSkillEvolutionSuggestion(ctx context.Context, req *v1.RejectSkillEvolutionSuggestionRequest) (*v1.RejectSkillEvolutionSuggestionResponse, error) {
	if err := s.uc.RejectSuggestion(ctx, req.GetId(), req.GetRejectedBy(), req.GetRejectionReason()); err != nil {
		return nil, err
	}
	return &v1.RejectSkillEvolutionSuggestionResponse{}, nil
}

// ── Proto conversion helpers ──────────────────────────────────────────────────

func toProtoEvolutionSuggestion(s biz.SkillEvolutionSuggestion) *v1.SkillEvolutionSuggestionMsg {
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
	}
	if s.SandboxResult != nil {
		var m map[string]interface{}
		if err := json.Unmarshal(s.SandboxResult, &m); err == nil {
			if st, err := structpb.NewStruct(m); err == nil {
				pb.SandboxResult = st
			}
		}
	}
	if s.PreVerifyResult != nil {
		var m map[string]interface{}
		if err := json.Unmarshal(s.PreVerifyResult, &m); err == nil {
			if st, err := structpb.NewStruct(m); err == nil {
				pb.PreVerifyResult = st
			}
		}
	}
	if s.ResolvedAt != nil {
		pb.ResolvedAt = timestamppb.New(*s.ResolvedAt)
	}
	return pb
}
