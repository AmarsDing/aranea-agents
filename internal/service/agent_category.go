package service

import (
	"context"
	"database/sql"
	"errors"

	v1 "aranea-agents/api/kratos/agent_category/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// AgentCategoryService implements kratos agent_category.v1.
type AgentCategoryService struct {
	v1.UnimplementedAgentCategoryServiceServer

	uc *biz.AgentCategoryUsecase
}

// NewAgentCategoryService constructs the service.
func NewAgentCategoryService(uc *biz.AgentCategoryUsecase) *AgentCategoryService {
	return &AgentCategoryService{uc: uc}
}

func toProtoCat(c biz.AgentCategory) *v1.AgentCategory {
	return &v1.AgentCategory{
		Id:           c.ID,
		Key:          c.Key,
		Name:         c.Name,
		Description:  c.Description,
		Status:       c.Status,
		Enabled:      c.Enabled,
		SortOrder:    int32(c.SortOrder),
		ParentId:     c.ParentID,
		Level:        c.Level,
		WorkspaceId:  c.WorkspaceID,
		OwnerUserId:  c.OwnerUserID,
		IsSystem:     c.IsSystem,
		ConfigJson:   c.ConfigJSON,
		MetadataJson: c.MetadataJSON,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
		DeletedAt:    c.DeletedAt,
	}
}

func fromProtoCat(pb *v1.AgentCategory) biz.AgentCategory {
	if pb == nil {
		return biz.AgentCategory{}
	}
	return biz.AgentCategory{
		ID:           pb.GetId(),
		Key:          pb.GetKey(),
		Name:         pb.GetName(),
		Description:  pb.GetDescription(),
		Status:       pb.GetStatus(),
		Enabled:      pb.GetEnabled(),
		SortOrder:    int(pb.GetSortOrder()),
		ParentID:     pb.GetParentId(),
		Level:        pb.GetLevel(),
		WorkspaceID:  pb.GetWorkspaceId(),
		OwnerUserID:  pb.GetOwnerUserId(),
		IsSystem:     pb.GetIsSystem(),
		ConfigJSON:   pb.GetConfigJson(),
		MetadataJSON: pb.GetMetadataJson(),
		CreatedAt:    pb.GetCreatedAt(),
		UpdatedAt:    pb.GetUpdatedAt(),
		DeletedAt:    pb.GetDeletedAt(),
	}
}

func toProtoTree(nodes []biz.AgentCategoryTreeNode) []*v1.AgentCategoryTreeNode {
	out := make([]*v1.AgentCategoryTreeNode, 0, len(nodes))
	for i := range nodes {
		out = append(out, toProtoTreeNode(&nodes[i]))
	}
	return out
}

func toProtoTreeNode(n *biz.AgentCategoryTreeNode) *v1.AgentCategoryTreeNode {
	if n == nil {
		return nil
	}
	cat := toProtoCat(n.Category)
	children := make([]*v1.AgentCategoryTreeNode, 0, len(n.Children))
	for j := range n.Children {
		children = append(children, toProtoTreeNode(&n.Children[j]))
	}
	return &v1.AgentCategoryTreeNode{
		Category: cat,
		Children: children,
	}
}

// ListAgentCategories implements GET /v1/agent-categories.
func (s *AgentCategoryService) ListAgentCategories(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentCategoriesResponse, error) {
	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListAgentCategoriesResponse{Items: make([]*v1.AgentCategory, 0, len(items))}
	for i := range items {
		resp.Items = append(resp.Items, toProtoCat(items[i]))
	}
	return resp, nil
}

// ListAgentCategoryTree implements GET /v1/agent-categories/tree.
func (s *AgentCategoryService) ListAgentCategoryTree(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentCategoryTreeResponse, error) {
	nodes, err := s.uc.Tree(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.ListAgentCategoryTreeResponse{Items: toProtoTree(nodes)}, nil
}

// CreateAgentCategory implements POST /v1/agent-categories.
func (s *AgentCategoryService) CreateAgentCategory(ctx context.Context, req *v1.CreateAgentCategoryRequest) (*v1.AgentCategory, error) {
	in := biz.AgentCategory{
		Key:          req.GetKey(),
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		Status:       req.GetStatus(),
		Enabled:      req.GetEnabled(),
		SortOrder:    int(req.GetSortOrder()),
		ParentID:     req.GetParentId(),
		Level:        req.GetLevel(),
		WorkspaceID:  req.GetWorkspaceId(),
		OwnerUserID:  req.GetOwnerUserId(),
		ConfigJSON:   req.GetConfigJson(),
		MetadataJSON: req.GetMetadataJson(),
	}
	created, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return toProtoCat(created), nil
}

// GetAgentCategory implements GET /v1/agent-categories/{id}.
func (s *AgentCategoryService) GetAgentCategory(ctx context.Context, req *v1.GetAgentCategoryRequest) (*v1.AgentCategory, error) {
	c, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT_CATEGORY", "category not found")
		}
		return nil, err
	}
	return toProtoCat(c), nil
}

// UpdateAgentCategory implements PATCH /v1/agent-categories/{id}.
func (s *AgentCategoryService) UpdateAgentCategory(ctx context.Context, req *v1.UpdateAgentCategoryRequest) (*v1.AgentCategory, error) {
	if req.GetCategory() == nil {
		return nil, kerrors.BadRequest("AGENT_CATEGORY", "category body is required")
	}
	patch := fromProtoCat(req.GetCategory())
	out, err := s.uc.Update(ctx, req.GetId(), patch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("AGENT_CATEGORY", "category not found")
		}
		return nil, err
	}
	return toProtoCat(out), nil
}

// DeleteAgentCategory implements DELETE /v1/agent-categories/{id}.
func (s *AgentCategoryService) DeleteAgentCategory(ctx context.Context, req *v1.DeleteAgentCategoryRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
