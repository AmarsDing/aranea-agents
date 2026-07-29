package service

import (
	"context"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
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
