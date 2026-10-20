package service

import (
	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
	"context"
	"strings"
	"testing"
)

// 显式 collection_id 行为不变（单库检索）。
func TestSearch_ExplicitCollection_Unchanged(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)

	if _, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "col-x", Name: "x", EmbeddingModel: "m", Dim: 3,
	}); err != nil {
		t.Fatal(err)
	}
	retriever := knowledge.NewRetriever(us14QueryEmbedder{}, repo, nil, loggateway.NewNoop())
	federated := knowledge.NewFederatedRetrieverWithMeta(nil, retriever, repo, loggateway.NewNoop())
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{
		Retriever: retriever,
		Federated: federated,
	}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	_, err := svc.Search(context.Background(), &v1.SearchRequest{
		CollectionId: "col-x",
		Query:        "test",
	})
	if err != nil {
		t.Fatalf("explicit collection search must not error, got %v", err)
	}
}

func TestSearch_PathPrefixPlumbed(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	if _, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "col-x", Name: "x", EmbeddingModel: "m", Dim: 3,
	}); err != nil {
		t.Fatal(err)
	}
	retriever := knowledge.NewRetriever(us14QueryEmbedder{}, repo, nil, loggateway.NewNoop())
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{
		Retriever: retriever,
	}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	if _, err := svc.Search(context.Background(), &v1.SearchRequest{
		CollectionId: "col-x",
		Query:        "test",
		PathPrefix:   "notes/guides",
	}); err != nil {
		t.Fatalf("search with path_prefix must not error, got %v", err)
	}
	repo.mu.Lock()
	got := repo.lastSearchQ.PathPrefix
	repo.mu.Unlock()
	if got != "notes/guides" {
		t.Errorf("PathPrefix = %q, want %q（proto → biz 映射丢失）", got, "notes/guides")
	}
}

func TestMoveDocument_Success(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	src, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name: "默认知识库", EmbeddingModel: "m", Dim: 1536,
		Workspace: workspace.DefaultWorkspaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	dst, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name: "quant", EmbeddingModel: "m", Dim: 1536,
		Workspace: workspace.DefaultWorkspaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := uc.CreateDocument(context.Background(), biz.KnowledgeDocument{
		CollectionID: src.ID,
		Source:       "report.txt",
		MimeType:     "text/plain",
	})
	if err != nil {
		t.Fatal(err)
	}

	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	moved, err := svc.MoveDocument(context.Background(), &v1.MoveDocumentRequest{
		Id:                 doc.ID,
		TargetCollectionId: dst.ID,
	})
	if err != nil {
		t.Fatalf("MoveDocument failed: %v", err)
	}
	if moved.GetCollectionId() != dst.ID {
		t.Errorf("document collection = %q, want %q", moved.GetCollectionId(), dst.ID)
	}
}

// 目标库 dim 不一致 → CodeConflict（向量维度不兼容，禁止移动）。
func TestMoveDocument_DimMismatch_ReturnsConflict(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	src, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name: "默认知识库", EmbeddingModel: "m", Dim: 1536,
		Workspace: workspace.DefaultWorkspaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	dst, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		Name: "other-dim", EmbeddingModel: "m", Dim: 768,
		Workspace: workspace.DefaultWorkspaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := uc.CreateDocument(context.Background(), biz.KnowledgeDocument{
		CollectionID: src.ID,
		Source:       "report.txt",
		MimeType:     "text/plain",
	})
	if err != nil {
		t.Fatal(err)
	}

	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	_, err = svc.MoveDocument(context.Background(), &v1.MoveDocumentRequest{
		Id:                 doc.ID,
		TargetCollectionId: dst.ID,
	})
	if err == nil {
		t.Fatal("expected dim-mismatch conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "dimension") {
		t.Errorf("error should mention dimension mismatch, got %v", err)
	}
}
