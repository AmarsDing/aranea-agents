package service

import (
	"context"

	v1 "aranea-agents/api/kratos/taxonomy/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
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

// assertTaxAccess verifies the caller can access the given taxonomy node.
// System caller bypasses. IsSystem nodes are shared (visible to all callers).
// Tenant-owned nodes require workspace match.
//
// TECH-DEBT: TaxonomyService has no lg field, so we cannot log IDOR
// attempts. Future refactor should inject loggateway.Logger via constructor.
func (s *TaxonomyService) assertTaxAccess(ctx context.Context, node biz.OrganizationNode) error {
	if workspace.IsSystem(ctx) {
		return nil
	}
	if node.IsSystem {
		return nil // shared system node
	}
	callerWS := workspace.IDFromContext(ctx)
	if err := workspace.AssertWorkspace(callerWS, node.WorkspaceID); err != nil {
		return apierror.NotFound("TAXONOMY", "node not found")
	}
	return nil
}

// filterTaxNodes returns only nodes visible to the caller: system nodes and
// nodes whose workspace matches the caller's workspace.
func (s *TaxonomyService) filterTaxNodes(ctx context.Context, nodes []biz.OrganizationNode) []biz.OrganizationNode {
	if workspace.IsSystem(ctx) {
		return nodes
	}
	callerWS := workspace.IDFromContext(ctx)
	out := make([]biz.OrganizationNode, 0, len(nodes))
	for _, n := range nodes {
		if n.IsSystem {
			out = append(out, n)
			continue
		}
		if workspace.AssertWorkspace(callerWS, n.WorkspaceID) == nil {
			out = append(out, n)
		}
	}
	return out
}

// filterTaxTree recursively filters taxonomy tree nodes.
func (s *TaxonomyService) filterTaxTree(ctx context.Context, nodes []biz.OrganizationTreeNode) []biz.OrganizationTreeNode {
	if workspace.IsSystem(ctx) {
		return nodes
	}
	out := make([]biz.OrganizationTreeNode, 0, len(nodes))
	for _, n := range nodes {
		if err := s.assertTaxAccess(ctx, n.Category); err != nil {
			continue
		}
		filtered := n
		filtered.Children = s.filterTaxTree(ctx, n.Children)
		out = append(out, filtered)
	}
	return out
}

func (s *TaxonomyService) ListTaxonomy(ctx context.Context, _ *emptypb.Empty) (*v1.ListTaxonomyResponse, error) {
	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	items = s.filterTaxNodes(ctx, items)
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
	nodes = s.filterTaxTree(ctx, nodes)
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
	// P2-C: workspace forgery guard. Non-system callers cannot set
	// WorkspaceID to anything other than their ctx workspace.
	if !workspace.IsSystem(ctx) {
		in.WorkspaceID = workspace.IDFromContext(ctx)
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
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TAXONOMY", "node not found")
		}
		return nil, err
	}
	if err := s.assertTaxAccess(ctx, c); err != nil {
		return nil, err
	}
	return toProtoTaxonomy(c), nil
}

func (s *TaxonomyService) UpdateTaxonomy(ctx context.Context, req *v1.UpdateTaxonomyRequest) (*v1.TaxonomyNode, error) {
	if req.GetNode() == nil {
		return nil, apierror.BadRequest("TAXONOMY", "node body is required")
	}
	// P2-C: IDOR guard — verify access before update.
	current, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TAXONOMY", "node not found")
		}
		return nil, err
	}
	if err := s.assertTaxAccess(ctx, current); err != nil {
		return nil, err
	}
	patch := fromProtoTaxonomy(req.GetNode())
	out, err := s.uc.Update(ctx, req.GetId(), patch)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TAXONOMY", "node not found")
		}
		return nil, err
	}
	return toProtoTaxonomy(out), nil
}

func (s *TaxonomyService) DeleteTaxonomy(ctx context.Context, req *v1.DeleteTaxonomyRequest) (*emptypb.Empty, error) {
	// P2-C: IDOR guard — verify access before delete.
	current, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TAXONOMY", "node not found")
		}
		return nil, err
	}
	if err := s.assertTaxAccess(ctx, current); err != nil {
		return nil, err
	}
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *TaxonomyService) ReorderTaxonomy(ctx context.Context, req *v1.ReorderTaxonomyRequest) (*v1.ReorderTaxonomyResponse, error) {
	if len(req.Ids) == 0 {
		return &v1.ReorderTaxonomyResponse{}, nil
	}
	// P2-C: IDOR guard — verify access for all ids before reorder.
	for _, id := range req.Ids {
		node, err := s.uc.Get(ctx, id)
		if err != nil {
			if apierror.IsCode(err, apierror.CodeNotFound) {
				return nil, apierror.NotFound("TAXONOMY", "node not found")
			}
			return nil, err
		}
		if err := s.assertTaxAccess(ctx, node); err != nil {
			return nil, err
		}
	}
	if err := s.uc.Reorder(ctx, req.Ids); err != nil {
		return nil, err
	}
	return &v1.ReorderTaxonomyResponse{}, nil
}
