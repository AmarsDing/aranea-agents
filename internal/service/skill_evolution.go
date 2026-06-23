package service

import (
	"context"

	v1 "aranea-agents/api/kratos/skill_evolution/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type SkillEvolutionService struct {
	v1.UnimplementedSkillEvolutionServiceServer

	uc *biz.SkillEvolutionUsecase
	lg loggateway.Logger
}

func NewSkillEvolutionService(uc *biz.SkillEvolutionUsecase, lg loggateway.Logger) *SkillEvolutionService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SkillEvolutionService{uc: uc, lg: lg}
}

func (s *SkillEvolutionService) ListSkillProposals(ctx context.Context, req *v1.ListSkillProposalsRequest) (*v1.ListSkillProposalsResponse, error) {
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	proposals, err := s.uc.ListProposals(ctx, req.GetAgentId(), req.GetStatus(), limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := s.uc.CountProposals(ctx, req.GetAgentId(), req.GetStatus())
	if err != nil {
		return nil, err
	}
	resp := &v1.ListSkillProposalsResponse{
		Total:    int32(total),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range proposals {
		resp.Items = append(resp.Items, toProtoSkillProposal(proposals[i]))
	}
	return resp, nil
}

func (s *SkillEvolutionService) GetSkillProposal(ctx context.Context, req *v1.GetSkillProposalRequest) (*v1.SkillProposal, error) {
	p, err := s.uc.GetProposal(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoSkillProposal(p), nil
}

func (s *SkillEvolutionService) ApproveSkillProposal(ctx context.Context, req *v1.ApproveSkillProposalRequest) (*v1.SkillProposal, error) {
	p, err := s.uc.ApproveProposal(ctx, req.GetId(), req.GetApprovedBy())
	if err != nil {
		return nil, err
	}
	return toProtoSkillProposal(p), nil
}

func (s *SkillEvolutionService) RejectSkillProposal(ctx context.Context, req *v1.RejectSkillProposalRequest) (*v1.SkillProposal, error) {
	p, err := s.uc.RejectProposal(ctx, req.GetId(), req.GetRejectedBy())
	if err != nil {
		return nil, err
	}
	return toProtoSkillProposal(p), nil
}

func (s *SkillEvolutionService) RegisterSkillProposal(ctx context.Context, req *v1.RegisterSkillProposalRequest) (*v1.SkillProposal, error) {
	p, err := s.uc.RegisterApproved(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoSkillProposal(p), nil
}

func (s *SkillEvolutionService) TriggerSkillDetection(ctx context.Context, req *v1.TriggerSkillDetectionRequest) (*v1.TriggerSkillDetectionResponse, error) {
	proposals, err := s.uc.DetectAndPropose(ctx, req.GetAgentId())
	if err != nil {
		return nil, err
	}
	resp := &v1.TriggerSkillDetectionResponse{}
	for i := range proposals {
		resp.Proposals = append(resp.Proposals, toProtoSkillProposal(proposals[i]))
	}
	return resp, nil
}

func toProtoSkillProposal(p biz.SkillProposal) *v1.SkillProposal {
	pb := &v1.SkillProposal{
		Id:          p.ID,
		AgentId:     p.AgentID,
		PatternHash: p.PatternHash,
		PatternDesc: p.PatternDesc,
		SkillName:   p.SkillName,
		SkillMd:     p.SkillMD,
		Status:      string(p.Status),
		ApprovedBy:  p.ApprovedBy,
		RejectedBy:  p.RejectedBy,
		CreatedAt:   p.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if p.ApprovedAt != nil {
		pb.ApprovedAt = p.ApprovedAt.Format("2006-01-02T15:04:05Z")
	}
	return pb
}
