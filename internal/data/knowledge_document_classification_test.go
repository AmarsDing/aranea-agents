package data

// D6 L-3（20261329）：knowledge_documents.classification 列读写往返
//（验收 #7 载体面——embedding 闸按文档密级判定通道，列值必须无损往返；
// 首个写入方为 office 附件 sink 透传）。

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
)

func TestKnowledgeDocument_ClassificationRoundtrip(t *testing.T) {
	repo := setupCurateRepo(t)
	ctx := workspace.WithSystemWorkspace(context.Background())

	// 显式密级写入 → 读回一致。
	doc, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
		ID: "d-l3", CollectionID: "c1", Source: "d-l3.txt", Status: "pending", Classification: 3,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if doc.Classification != 3 {
		t.Fatalf("returned classification = %d, want 3", doc.Classification)
	}
	got, err := repo.GetDocument(ctx, doc.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Classification != 3 {
		t.Fatalf("read-back classification = %d, want 3", got.Classification)
	}

	// 未标（零值）→ 落库默认 0，读回 0。
	doc0, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
		ID: "d-unset", CollectionID: "c1", Source: "d-unset.txt", Status: "pending",
	})
	if err != nil {
		t.Fatalf("create unset: %v", err)
	}
	got0, err := repo.GetDocument(ctx, doc0.ID)
	if err != nil {
		t.Fatalf("get unset: %v", err)
	}
	if got0.Classification != 0 {
		t.Fatalf("unset classification = %d, want 0", got0.Classification)
	}
}
