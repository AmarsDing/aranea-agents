package service

import (
	"context"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"google.golang.org/protobuf/types/known/emptypb"
)

// P3 资源管理器（Vault explorer）RPC：懒加载文件夹树 + 文档关联（来源标注 R-3）。
// 数据流：proto → biz Usecase（ListVaultTree / ListDocumentResolvedLinks）→ proto。

// ListVaultTree returns the direct children of a vault folder (lazy loading).
func (s *KnowledgeService) ListVaultTree(ctx context.Context, req *v1.ListVaultTreeRequest) (*v1.ListVaultTreeResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	nodes, err := s.uc.ListVaultTree(ctx, req.GetCollectionId(), req.GetPrefix())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.VaultTreeNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, toProtoVaultTreeNode(n))
	}
	return &v1.ListVaultTreeResponse{Items: out}, nil
}

// CreateVaultDir creates a (nested) directory inside the vault (G1-B2; idempotent).
func (s *KnowledgeService) CreateVaultDir(ctx context.Context, req *v1.CreateVaultDirRequest) (*emptypb.Empty, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	if err := s.uc.CreateVaultDir(ctx, req.GetCollectionId(), req.GetDirPath()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// CreateVaultDocument writes a template .md into the vault and indexes it
// immediately (G1-B2). Existing path returns CodeConflict.
func (s *KnowledgeService) CreateVaultDocument(ctx context.Context, req *v1.CreateVaultDocumentRequest) (*v1.KnowledgeDocument, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	doc, err := s.uc.CreateVaultDocument(ctx, req.GetCollectionId(), req.GetRelPath())
	if err != nil {
		return nil, err
	}
	return toProtoDocument(doc), nil
}

// ListDocumentLinks returns resolved relations of one document with source-type
// annotation (explicit / entity / semantic), both directions.
func (s *KnowledgeService) ListDocumentLinks(ctx context.Context, req *v1.ListDocumentLinksRequest) (*v1.ListDocumentLinksResponse, error) {
	doc, err := s.uc.GetDocument(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	col, err := s.uc.GetCollection(ctx, doc.CollectionID)
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	links, err := s.uc.ListDocumentResolvedLinks(ctx, doc.CollectionID, doc.ID, req.GetLinkType())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.KnowledgeLink, 0, len(links))
	for _, l := range links {
		out = append(out, toProtoKnowledgeLink(l))
	}
	return &v1.ListDocumentLinksResponse{Items: out}, nil
}

// ListCollectionGraph returns the full link graph of one collection (G4-B8 3D 图谱).
// 一次性全量（<2k 节点），不做分页；未接线关联读取时降级为仅节点无边。
func (s *KnowledgeService) ListCollectionGraph(ctx context.Context, req *v1.ListCollectionGraphRequest) (*v1.ListCollectionGraphResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	g, err := s.uc.ListCollectionGraph(ctx, req.GetCollectionId(), req.GetLinkTypes(), req.GetPathPrefix())
	if err != nil {
		return nil, err
	}
	out := &v1.ListCollectionGraphResponse{
		Nodes: make([]*v1.CollectionGraphNode, 0, len(g.Nodes)),
		Edges: make([]*v1.CollectionGraphEdge, 0, len(g.Edges)),
	}
	for _, n := range g.Nodes {
		out.Nodes = append(out.Nodes, &v1.CollectionGraphNode{
			DocId:   n.DocID,
			Name:    n.Name,
			RelPath: n.RelPath,
			DocType: n.DocType,
			Degree:  int32(n.Degree),
		})
	}
	for _, e := range g.Edges {
		out.Edges = append(out.Edges, &v1.CollectionGraphEdge{
			Source: e.Source,
			Target: e.Target,
			Type:   e.Type,
		})
	}
	return out, nil
}

// --- proto conversion helpers ---

func toProtoVaultTreeNode(n biz.KnowledgeVaultTreeNode) *v1.VaultTreeNode {
	return &v1.VaultTreeNode{
		Name:         n.Name,
		Path:         n.Path,
		Kind:         n.Kind,
		DocId:        n.DocID,
		Summary:      n.Summary,
		Tags:         n.Tags,
		DocType:      n.DocType,
		Status:       n.Status,
		SizeBytes:    n.SizeBytes,
		UpdatedAt:    n.UpdatedAt,
		ErrorMessage: n.ErrorMessage,
	}
}

func toProtoKnowledgeLink(l biz.KnowledgeResolvedLink) *v1.KnowledgeLink {
	return &v1.KnowledgeLink{
		TargetDocId:   l.TargetDocID,
		TargetSource:  l.TargetSource,
		TargetRelPath: l.TargetRelPath,
		LinkType:      l.LinkType,
		Context:       l.Context,
		Direction:     l.Direction,
	}
}
