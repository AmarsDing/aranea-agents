package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/learning_loop/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/auth"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type LearningLoopService struct {
	v1.UnimplementedLearningLoopServiceServer
	uc *biz.LearningLoopUsecase
}

func NewLearningLoopService(uc *biz.LearningLoopUsecase) *LearningLoopService {
	return &LearningLoopService{uc: uc}
}

func (s *LearningLoopService) ListProposals(ctx context.Context, req *v1.ListProposalsRequest) (*v1.ListProposalsResponse, error) {
	items, err := s.uc.ListProposals(ctx, req.GetAgentId(), req.GetStatus())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.KnowledgeProposal, 0, len(items))
	for _, p := range items {
		out = append(out, toProtoProposal(p))
	}
	return &v1.ListProposalsResponse{Items: out}, nil
}

func (s *LearningLoopService) ListPatterns(ctx context.Context, req *v1.ListPatternsRequest) (*v1.ListPatternsResponse, error) {
	items, err := s.uc.ListPatterns(ctx, req.GetAgentId(), req.GetStatus())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.Pattern, 0, len(items))
	for _, p := range items {
		out = append(out, toProtoPattern(p))
	}
	return &v1.ListPatternsResponse{Items: out}, nil
}

func (s *LearningLoopService) ListObservations(ctx context.Context, req *v1.ListObservationsRequest) (*v1.ListObservationsResponse, error) {
	items, err := s.uc.ListObservations(ctx, req.GetAgentId(), req.GetSince())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.Observation, 0, len(items))
	for _, o := range items {
		out = append(out, toProtoObservation(o))
	}
	return &v1.ListObservationsResponse{Items: out}, nil
}

func (s *LearningLoopService) ApproveProposal(ctx context.Context, req *v1.ApproveProposalRequest) (*v1.KnowledgeProposal, error) {
	approvedBy := biz.ApprovedBySystem
	if a, ok := auth.FromContext(ctx); ok && a.UserID > 0 {
		approvedBy = biz.FormatApprovedByUser(a.UserID)
	}
	p, err := s.uc.ApproveProposal(ctx, req.GetId(), approvedBy)
	if err != nil {
		return nil, err
	}
	return toProtoProposal(p), nil
}

func (s *LearningLoopService) RejectProposal(ctx context.Context, req *v1.RejectProposalRequest) (*v1.KnowledgeProposal, error) {
	p, err := s.uc.RejectProposal(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoProposal(p), nil
}

func (s *LearningLoopService) RunLoop(ctx context.Context, req *v1.RunLoopRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.uc.RunLoop(ctx, req.GetAgentId())
}

func toProtoObservation(o biz.Observation) *v1.Observation {
	return &v1.Observation{
		Id:         o.ID,
		AgentId:    o.AgentID,
		SessionId:  o.SessionID,
		Kind:       string(o.Kind),
		Content:    o.Content,
		Metadata:   o.Metadata,
		ObservedAt: o.ObservedAt.Format(time.RFC3339),
	}
}

func toProtoPattern(p biz.Pattern) *v1.Pattern {
	return &v1.Pattern{
		Id:          p.ID,
		AgentId:     p.AgentID,
		Kind:        p.Kind,
		Description: p.Description,
		Frequency:   int32(p.Frequency),
		Confidence:  p.Confidence,
		Evidence:    p.Evidence,
		Status:      string(p.Status),
		DetectedAt:  p.DetectedAt.Format(time.RFC3339),
	}
}

func toProtoProposal(p biz.KnowledgeProposal) *v1.KnowledgeProposal {
	out := &v1.KnowledgeProposal{
		Id:         p.ID,
		AgentId:    p.AgentID,
		PatternId:  p.PatternID,
		Title:      p.Title,
		Content:    p.Content,
		Kind:       p.Kind,
		Status:     string(p.Status),
		ApprovedBy: p.ApprovedBy,
		CreatedAt:  p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  p.UpdatedAt.Format(time.RFC3339),
	}
	if p.ValidatedAt != nil {
		out.ValidatedAt = p.ValidatedAt.Format(time.RFC3339)
	}
	return out
}
