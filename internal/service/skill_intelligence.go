package service

import (
	"context"
	"encoding/json"
	"time"

	v1 "aranea-agents/api/kratos/skill_intelligence/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
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

	var startTime, endTime *time.Time
	if ts := req.GetStartTime(); ts != nil {
		t := ts.AsTime()
		startTime = &t
	}
	if ts := req.GetEndTime(); ts != nil {
		t := ts.AsTime()
		endTime = &t
	}

	result, err := s.uc.GetExperienceReportsFiltered(ctx, req.GetSkillId(), startTime, endTime, limit, offset)
	if err != nil {
		if _, ok := apierror.From(err); ok {
			return nil, err // Already a properly typed domain error
		}
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}

	resp := &v1.ListExperienceReportsResponse{
		Total:    int64(result.TotalCount),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range result.Reports {
		resp.Items = append(resp.Items, toProtoExperienceReport(result.Reports[i]))
	}
	for _, fc := range result.FailureTagCounts {
		resp.FailureTagCounts = append(resp.FailureTagCounts, toProtoFailureTagCount(fc))
	}
	for i := range result.RootCauseReports {
		resp.RootCauseReports = append(resp.RootCauseReports, toProtoExperienceReport(result.RootCauseReports[i]))
	}
	return resp, nil
}

func (s *SkillIntelligenceService) GetExperienceReport(ctx context.Context, req *v1.GetExperienceReportRequest) (*v1.GetExperienceReportResponse, error) {
	r, err := s.uc.GetExperienceReport(ctx, req.GetId())
	if err != nil {
		if _, ok := apierror.From(err); ok {
			return nil, err // Already a properly typed domain error
		}
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainSkill)
	}
	// B-11 fix: nil check before dereference to prevent panic if usecase
	// returns (nil, nil) — defensive against contract violations.
	if r == nil {
		return nil, apierror.NotFound(apierror.DomainSkill, "experience report not found")
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
		SkillName:          r.SkillName,
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

// toProtoFailureTagCount converts a biz FailureTagCount to proto.
func toProtoFailureTagCount(fc biz.FailureTagCount) *v1.FailureTagCount {
	return &v1.FailureTagCount{
		Tag:   fc.Tag,
		Count: int32(fc.Count),
	}
}
