package service

import (
	"context"
	"database/sql"
	"errors"

	v1 "aranea-agents/api/kratos/industry_taxonomy/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type IndustryTaxonomyService struct {
	v1.UnimplementedIndustryTaxonomyServiceServer

	uc *biz.TaxonomyUsecase
}

func NewIndustryTaxonomyService(uc *biz.TaxonomyUsecase) *IndustryTaxonomyService {
	return &IndustryTaxonomyService{uc: uc}
}

func toProtoIndustryTaxonomy(c biz.TaxonomyNode) *v1.IndustryTaxonomy {
	return &v1.IndustryTaxonomy{
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

func fromProtoIndustryTaxonomy(pb *v1.IndustryTaxonomy) biz.TaxonomyNode {
	if pb == nil {
		return biz.TaxonomyNode{}
	}
	return biz.TaxonomyNode{
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

func toIndustryTaxonomyTree(nodes []biz.TaxonomyTreeNode) []*v1.IndustryTaxonomyTreeNode {
	out := make([]*v1.IndustryTaxonomyTreeNode, 0, len(nodes))
	for i := range nodes {
		out = append(out, toIndustryTaxonomyTreeNode(&nodes[i]))
	}
	return out
}

func toIndustryTaxonomyTreeNode(n *biz.TaxonomyTreeNode) *v1.IndustryTaxonomyTreeNode {
	if n == nil {
		return nil
	}
	it := toProtoIndustryTaxonomy(n.Category)
	children := make([]*v1.IndustryTaxonomyTreeNode, 0, len(n.Children))
	for j := range n.Children {
		children = append(children, toIndustryTaxonomyTreeNode(&n.Children[j]))
	}
	return &v1.IndustryTaxonomyTreeNode{
		IndustryTaxonomy: it,
		Children:         children,
	}
}

func (s *IndustryTaxonomyService) ListIndustryTaxonomies(ctx context.Context, _ *emptypb.Empty) (*v1.ListIndustryTaxonomiesResponse, error) {
	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListIndustryTaxonomiesResponse{Items: make([]*v1.IndustryTaxonomy, 0, len(items))}
	for i := range items {
		resp.Items = append(resp.Items, toProtoIndustryTaxonomy(items[i]))
	}
	return resp, nil
}

func (s *IndustryTaxonomyService) ListIndustryTaxonomyTree(ctx context.Context, _ *emptypb.Empty) (*v1.ListIndustryTaxonomyTreeResponse, error) {
	nodes, err := s.uc.Tree(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.ListIndustryTaxonomyTreeResponse{Items: toIndustryTaxonomyTree(nodes)}, nil
}

func (s *IndustryTaxonomyService) CreateIndustryTaxonomy(ctx context.Context, req *v1.CreateIndustryTaxonomyRequest) (*v1.IndustryTaxonomy, error) {
	in := biz.TaxonomyNode{
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
	return toProtoIndustryTaxonomy(created), nil
}

func (s *IndustryTaxonomyService) GetIndustryTaxonomy(ctx context.Context, req *v1.GetIndustryTaxonomyRequest) (*v1.IndustryTaxonomy, error) {
	c, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("INDUSTRY_TAXONOMY", "node not found")
		}
		return nil, err
	}
	return toProtoIndustryTaxonomy(c), nil
}

func (s *IndustryTaxonomyService) UpdateIndustryTaxonomy(ctx context.Context, req *v1.UpdateIndustryTaxonomyRequest) (*v1.IndustryTaxonomy, error) {
	if req.GetIndustryTaxonomy() == nil {
		return nil, kerrors.BadRequest("INDUSTRY_TAXONOMY", "industry_taxonomy body is required")
	}
	patch := fromProtoIndustryTaxonomy(req.GetIndustryTaxonomy())
	out, err := s.uc.Update(ctx, req.GetId(), patch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("INDUSTRY_TAXONOMY", "node not found")
		}
		return nil, err
	}
	return toProtoIndustryTaxonomy(out), nil
}

func (s *IndustryTaxonomyService) DeleteIndustryTaxonomy(ctx context.Context, req *v1.DeleteIndustryTaxonomyRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
