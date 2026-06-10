package service

import (
	"context"
	"fmt"

	v1 "aranea-agents/api/kratos/skill_dedup/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type SkillDedupService struct {
	v1.UnimplementedSkillDedupServiceServer

	uc    *biz.SkillDedupUsecase
	merge *biz.SkillMergeUsecase
	lg    loggateway.Logger
}

func NewSkillDedupService(uc *biz.SkillDedupUsecase, merge *biz.SkillMergeUsecase, lg loggateway.Logger) *SkillDedupService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SkillDedupService{uc: uc, merge: merge, lg: lg}
}

func (s *SkillDedupService) ListSkillDuplicateGroups(ctx context.Context, req *v1.ListSkillDuplicateGroupsRequest) (*v1.ListSkillDuplicateGroupsResponse, error) {
	groups, err := s.uc.DetectDuplicateGroups(ctx)
	if err != nil {
		return nil, err
	}

	// Apply pagination.
	total := int64(len(groups))
	page := req.GetPage()
	pageSize := req.GetPageSize()
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	start := int((page - 1) * pageSize)
	end := int(page * pageSize)
	if start > len(groups) {
		start = len(groups)
	}
	if end > len(groups) {
		end = len(groups)
	}

	resp := &v1.ListSkillDuplicateGroupsResponse{
		Total: total,
	}
	for _, g := range groups[start:end] {
		resp.Groups = append(resp.Groups, toProtoSkillDuplicateGroup(g))
	}
	return resp, nil
}

func (s *SkillDedupService) MergeSkills(ctx context.Context, req *v1.MergeSkillsRequest) (*v1.MergeSkillsResponse, error) {
	if s.merge == nil {
		return nil, kerrors.ServiceUnavailable("SKILL_DEDUP", "skill merge usecase not available: transactional merge is required")
	}
	result, err := s.merge.Merge(ctx, biz.SkillMergeRequest{
		SourceID: req.GetSourceSkillId(),
		TargetID: req.GetTargetSkillId(),
		Strategy: biz.MergeStrategyAppend,
	})
	if err != nil {
		return nil, err
	}
	return &v1.MergeSkillsResponse{
		Message: fmt.Sprintf("skills merged successfully (new version: %s, transferred: %d)", result.NewVersionID, result.TransferredCount),
	}, nil
}

func toProtoSkillDuplicateGroup(g biz.SkillDuplicateGroup) *v1.SkillDuplicateGroup {
	pb := &v1.SkillDuplicateGroup{
		GroupId:       g.GroupID,
		OverlapType:   g.OverlapType,
		OverlapScore:  float32(g.OverlapScore),
		ConflictRisk:  g.ConflictRisk,
		Recommendation: g.Recommendation,
	}
	for _, s := range g.Skills {
		pb.Skills = append(pb.Skills, toProtoSkillSummary(s))
	}
	for _, d := range g.Dimensions {
		pb.Dimensions = append(pb.Dimensions, &v1.DimensionScore{
			Dimension: string(d.Dimension),
			Score:     d.Score,
			Method:    d.Method,
		})
	}
	return pb
}

func toProtoSkillSummary(s biz.SkillSummary) *v1.SkillSummary {
	return &v1.SkillSummary{
		Id:          s.ID,
		Name:        s.Name,
		Slug:        s.Slug,
		Description: s.Description,
		BodyPreview: s.BodyPreview,
		Tags:        s.Tags,
	}
}
