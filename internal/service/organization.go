package service

import (
	"context"

	v1 "aranea-agents/api/kratos/organization/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type OrganizationService struct {
	v1.UnimplementedOrganizationServiceServer

	uc *biz.OrganizationUsecase
}

func NewOrganizationService(uc *biz.OrganizationUsecase) *OrganizationService {
	return &OrganizationService{uc: uc}
}

func toProtoOrganization(c biz.OrganizationNode) *v1.OrganizationNode {
	return &v1.OrganizationNode{
		Id:                 c.ID,
		OrgKey:             c.Key,
		Name:               c.Name,
		Description:        c.Description,
		Status:             c.Status,
		Enabled:            c.Enabled,
		SortOrder:          int32(c.SortOrder),
		ParentId:           c.ParentID,
		Level:              c.Level,
		WorkspaceId:        c.WorkspaceID,
		OwnerUserId:        c.OwnerUserID,
		IsSystem:           c.IsSystem,
		ConfigJson:         c.ConfigJSON,
		MetadataJson:       c.MetadataJSON,
		CreatedAt:          c.CreatedAt,
		UpdatedAt:          c.UpdatedAt,
		DeletedAt:          c.DeletedAt,
		DeptLeadAgentId:    c.DeptLeadAgentID,
		DeptLeadConfigJson: c.DeptLeadConfigJSON,
	}
}

func fromProtoOrganization(pb *v1.OrganizationNode) biz.OrganizationNode {
	if pb == nil {
		return biz.OrganizationNode{}
	}
	return biz.OrganizationNode{
		ID:                 pb.GetId(),
		Key:                pb.GetOrgKey(),
		Name:               pb.GetName(),
		Description:        pb.GetDescription(),
		Status:             pb.GetStatus(),
		Enabled:            pb.GetEnabled(),
		SortOrder:          int(pb.GetSortOrder()),
		ParentID:           pb.GetParentId(),
		Level:              pb.GetLevel(),
		WorkspaceID:        pb.GetWorkspaceId(),
		OwnerUserID:        pb.GetOwnerUserId(),
		IsSystem:           pb.GetIsSystem(),
		ConfigJSON:         pb.GetConfigJson(),
		MetadataJSON:       pb.GetMetadataJson(),
		CreatedAt:          pb.GetCreatedAt(),
		UpdatedAt:          pb.GetUpdatedAt(),
		DeletedAt:          pb.GetDeletedAt(),
		DeptLeadAgentID:    pb.GetDeptLeadAgentId(),
		DeptLeadConfigJSON: pb.GetDeptLeadConfigJson(),
	}
}

func toOrganizationTree(nodes []biz.OrganizationTreeNode) []*v1.OrganizationTreeNode {
	out := make([]*v1.OrganizationTreeNode, 0, len(nodes))
	for i := range nodes {
		out = append(out, toOrganizationTreeNode(&nodes[i]))
	}
	return out
}

func toOrganizationTreeNode(n *biz.OrganizationTreeNode) *v1.OrganizationTreeNode {
	if n == nil {
		return nil
	}
	node := toProtoOrganization(n.Category)
	children := make([]*v1.OrganizationTreeNode, 0, len(n.Children))
	for j := range n.Children {
		children = append(children, toOrganizationTreeNode(&n.Children[j]))
	}
	return &v1.OrganizationTreeNode{
		Node:     node,
		Children: children,
	}
}

func (s *OrganizationService) ListOrganization(ctx context.Context, _ *emptypb.Empty) (*v1.ListOrganizationResponse, error) {
	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListOrganizationResponse{Items: make([]*v1.OrganizationNode, 0, len(items))}
	for i := range items {
		resp.Items = append(resp.Items, toProtoOrganization(items[i]))
	}
	return resp, nil
}

func (s *OrganizationService) ListOrganizationTree(ctx context.Context, _ *emptypb.Empty) (*v1.ListOrganizationTreeResponse, error) {
	nodes, err := s.uc.Tree(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.ListOrganizationTreeResponse{Items: toOrganizationTree(nodes)}, nil
}

func (s *OrganizationService) CreateOrganization(ctx context.Context, req *v1.CreateOrganizationRequest) (*v1.OrganizationNode, error) {
	in := biz.OrganizationNode{
		Key:                req.GetOrgKey(),
		Name:               req.GetName(),
		Description:        req.GetDescription(),
		Status:             req.GetStatus(),
		SortOrder:          int(req.GetSortOrder()),
		ParentID:           req.GetParentId(),
		Level:              req.GetLevel(),
		WorkspaceID:        req.GetWorkspaceId(),
		OwnerUserID:        req.GetOwnerUserId(),
		ConfigJSON:         req.GetConfigJson(),
		MetadataJSON:       req.GetMetadataJson(),
		DeptLeadAgentID:    req.GetDeptLeadAgentId(),
		DeptLeadConfigJSON: req.GetDeptLeadConfigJson(),
	}
	if req.Enabled != nil {
		in.Enabled = req.GetEnabled()
	} else {
		in.Enabled = true
	}
	created, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return toProtoOrganization(created), nil
}

func (s *OrganizationService) GetOrganization(ctx context.Context, req *v1.GetOrganizationRequest) (*v1.OrganizationNode, error) {
	c, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("ORGANIZATION", "node not found")
		}
		return nil, err
	}
	return toProtoOrganization(c), nil
}

func (s *OrganizationService) UpdateOrganization(ctx context.Context, req *v1.UpdateOrganizationRequest) (*v1.OrganizationNode, error) {
	if req.GetNode() == nil {
		return nil, apierror.BadRequest("ORGANIZATION", "node body is required")
	}
	patch := fromProtoOrganization(req.GetNode())
	out, err := s.uc.Update(ctx, req.GetId(), patch)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("ORGANIZATION", "node not found")
		}
		return nil, err
	}
	return toProtoOrganization(out), nil
}

func (s *OrganizationService) DeleteOrganization(ctx context.Context, req *v1.DeleteOrganizationRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *OrganizationService) ReorderOrganization(ctx context.Context, req *v1.ReorderOrganizationRequest) (*v1.ReorderOrganizationResponse, error) {
	if len(req.Ids) == 0 {
		return &v1.ReorderOrganizationResponse{}, nil
	}
	err := s.uc.Reorder(ctx, req.Ids)
	if err != nil {
		return nil, err
	}
	return &v1.ReorderOrganizationResponse{}, nil
}
