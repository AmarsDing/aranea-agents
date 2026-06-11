package service

import (
	"context"
	"database/sql"
	"errors"

	v1 "aranea-agents/api/kratos/taxonomy/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type TaxonomyService struct {
	v1.UnimplementedTaxonomyServiceServer

	uc *biz.OrganizationUsecase
}

func NewTaxonomyService(uc *biz.OrganizationUsecase) *TaxonomyService {
	return &TaxonomyService{uc: uc}
}

func toProtoTaxonomy(c biz.OrganizationNode) *v1.TaxonomyNode {
	return &v1.TaxonomyNode{
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

func fromProtoTaxonomy(pb *v1.TaxonomyNode) biz.OrganizationNode {
	if pb == nil {
		return biz.OrganizationNode{}
	}
	return biz.OrganizationNode{
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

func toTaxonomyTree(nodes []biz.OrganizationTreeNode) []*v1.TaxonomyTreeNode {
	out := make([]*v1.TaxonomyTreeNode, 0, len(nodes))
	for i := range nodes {
		out = append(out, toTaxonomyTreeNode(&nodes[i]))
	}
	return out
}

func toTaxonomyTreeNode(n *biz.OrganizationTreeNode) *v1.TaxonomyTreeNode {
	if n == nil {
		return nil
	}
	cat := toProtoTaxonomy(n.Category)
	children := make([]*v1.TaxonomyTreeNode, 0, len(n.Children))
	for j := range n.Children {
		children = append(children, toTaxonomyTreeNode(&n.Children[j]))
	}
	return &v1.TaxonomyTreeNode{
		Node:     cat,
		Children: children,
	}
}

func (s *TaxonomyService) ListTaxonomy(ctx context.Context, _ *emptypb.Empty) (*v1.ListTaxonomyResponse, error) {
	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListTaxonomyResponse{Items: make([]*v1.TaxonomyNode, 0, len(items))}
	for i := range items {
		resp.Items = append(resp.Items, toProtoTaxonomy(items[i]))
	}
	return resp, nil
}

func (s *TaxonomyService) ListTaxonomyTree(ctx context.Context, _ *emptypb.Empty) (*v1.ListTaxonomyTreeResponse, error) {
	nodes, err := s.uc.Tree(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.ListTaxonomyTreeResponse{Items: toTaxonomyTree(nodes)}, nil
}

func (s *TaxonomyService) CreateTaxonomy(ctx context.Context, req *v1.CreateTaxonomyRequest) (*v1.TaxonomyNode, error) {
	in := biz.OrganizationNode{
		Key:          req.GetKey(),
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		Status:       req.GetStatus(),
		SortOrder:    int(req.GetSortOrder()),
		ParentID:     req.GetParentId(),
		Level:        req.GetLevel(),
		WorkspaceID:  req.GetWorkspaceId(),
		OwnerUserID:  req.GetOwnerUserId(),
		ConfigJSON:   req.GetConfigJson(),
		MetadataJSON: req.GetMetadataJson(),
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
	return toProtoTaxonomy(created), nil
}

func (s *TaxonomyService) GetTaxonomy(ctx context.Context, req *v1.GetTaxonomyRequest) (*v1.TaxonomyNode, error) {
	c, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apierror.NotFound("TAXONOMY", "node not found")
		}
		return nil, err
	}
	return toProtoTaxonomy(c), nil
}

func (s *TaxonomyService) UpdateTaxonomy(ctx context.Context, req *v1.UpdateTaxonomyRequest) (*v1.TaxonomyNode, error) {
	if req.GetNode() == nil {
		return nil, apierror.BadRequest("TAXONOMY", "node body is required")
	}
	patch := fromProtoTaxonomy(req.GetNode())
	out, err := s.uc.Update(ctx, req.GetId(), patch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apierror.NotFound("TAXONOMY", "node not found")
		}
		return nil, err
	}
	return toProtoTaxonomy(out), nil
}

func (s *TaxonomyService) DeleteTaxonomy(ctx context.Context, req *v1.DeleteTaxonomyRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *TaxonomyService) ReorderTaxonomy(ctx context.Context, req *v1.ReorderTaxonomyRequest) (*v1.ReorderTaxonomyResponse, error) {
	if len(req.Ids) == 0 {
		return &v1.ReorderTaxonomyResponse{}, nil
	}
	err := s.uc.Reorder(ctx, req.Ids)
	if err != nil {
		return nil, err
	}
	return &v1.ReorderTaxonomyResponse{}, nil
}
