package service

import (
	"context"
	"encoding/json"

	v1 "aranea-agents/api/kratos/skill_intelligence/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	structpb "google.golang.org/protobuf/types/known/structpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

type SkillIntelligenceService struct {
	v1.UnimplementedSkillIntelligenceServiceServer

	uc *biz.SkillIntelligenceUsecase
	lg loggateway.Logger
}

func NewSkillIntelligenceService(uc *biz.SkillIntelligenceUsecase, lg loggateway.Logger) *SkillIntelligenceService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SkillIntelligenceService{uc: uc, lg: lg}
}

func (s *SkillIntelligenceService) ListExperienceReports(ctx context.Context, req *v1.ListExperienceReportsRequest) (*v1.ListExperienceReportsResponse, error) {
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	reports, err := s.uc.GetExperienceReports(ctx, req.GetSkillId(), limit, offset)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListExperienceReportsResponse{
		Total:    int64(len(reports)),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range reports {
		resp.Items = append(resp.Items, toProtoExperienceReport(reports[i]))
	}
	return resp, nil
}

func (s *SkillIntelligenceService) GetExperienceReport(ctx context.Context, req *v1.GetExperienceReportRequest) (*v1.GetExperienceReportResponse, error) {
	r, err := s.uc.GetExperienceReport(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &v1.GetExperienceReportResponse{
		Report: toProtoExperienceReport(*r),
	}, nil
}

func toProtoExperienceReport(r biz.ExperienceReport) *v1.ExperienceReport {
	pb := &v1.ExperienceReport{
		Id:                 r.ID,
		TenantId:           r.TenantID,
		SessionId:          r.SessionID,
		InvocationId:       r.InvocationID,
		SkillId:            r.SkillID,
		IsSuccess:          r.IsSuccess,
		Score:              int32(r.Score),
		FailureTags:        r.FailureTags,
		FlowSummary:        r.FlowSummary,
		OptimizationAdvice: r.OptimizationAdvice,
		RootCauseAnalysis:  r.RootCauseAnalysis,
		SuggestedFix:       r.SuggestedFix,
		CreatedAt:          timestamppb.New(r.CreatedAt),
	}
	if r.SelectionSnapshot != nil {
		var m map[string]interface{}
		if err := json.Unmarshal(r.SelectionSnapshot, &m); err == nil {
			if s, err := structpb.NewStruct(m); err == nil {
				pb.SelectionSnapshot = s
			}
		}
	}
	if r.GeneratedSuggestionID != nil {
		pb.GeneratedSuggestionId = *r.GeneratedSuggestionID
	}
	return pb
}


