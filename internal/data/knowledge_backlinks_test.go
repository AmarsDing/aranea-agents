package data

import (
	"context"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── SP1-E：BlockLinkReader 落库兜底 + DocNameReader 批量名解析 ───────────────

// seedBacklinkGraph 造两文档三块图：d1(块 A/B) 引用 d2 块 C + d2 文档级 + dangling。
// 返回 d2 目标块 C 的 ID。
func seedBacklinkGraph(t *testing.T, r *knowledgeBlockRepo) string {
	t.Helper()
	ctx := context.Background()
	seedDoc(t, r, "c1", "d1")
	seedDoc(t, r, "c1", "d2")

	// d2：目标文档（块 C 为锚块）。
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "d2", blockRows("^c"), nil); err != nil {
		t.Fatalf("replace d2: %v", err)
	}
	var dstBlockID string
	if err := r.data.rawDB.QueryRow(`SELECT id FROM knowledge_blocks WHERE doc_id='d2'`).Scan(&dstBlockID); err != nil {
		t.Fatalf("query dst block: %v", err)
	}
	// d1：块级边 → d2#C；文档级边 → d2；dangling → Ghost。
	refs := []bizknowledge.KnowledgeBlockRefInput{
		{SrcOrdinal: 0, RawTarget: "D2#^c", EdgeType: "ref", Context: "见 D2。", DstDocID: "d2", DstBlockID: dstBlockID},
		{SrcOrdinal: 1, RawTarget: "D2", EdgeType: "ref", Context: "整体参考", DstDocID: "d2"},
		{SrcOrdinal: 1, RawTarget: "Ghost", EdgeType: "ref", Context: "未创建"},
	}
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "d1", blockRows("p0", "p1"), refs); err != nil {
		t.Fatalf("replace d1: %v", err)
	}
	return dstBlockID
}

// TestBlockLinkReader_Backlinks 落库兜底三查询：块级 / 文档级 / dangling。
func TestBlockLinkReader_Backlinks(t *testing.T) {
	r := setupKnowledgeBlocksRepo(t)
	dstBlockID := seedBacklinkGraph(t, r)
	ctx := context.Background()

	byBlock, err := r.ListBacklinksByBlock(ctx, dstBlockID)
	if err != nil {
		t.Fatalf("ListBacklinksByBlock: %v", err)
	}
	if len(byBlock) != 1 || byBlock[0].RawTarget != "D2#^c" || byBlock[0].SrcDocID != "d1" {
		t.Fatalf("块级反链错误: %+v", byBlock)
	}
	if byBlock[0].DstCollectionID != "c1" {
		t.Errorf("DstCollectionID = %q, want c1（JOIN 推导）", byBlock[0].DstCollectionID)
	}

	byDoc, err := r.ListBacklinksByDoc(ctx, "d2")
	if err != nil {
		t.Fatalf("ListBacklinksByDoc: %v", err)
	}
	if len(byDoc) != 2 {
		t.Fatalf("文档级反链 = %d, want 2（块级 + 文档级）", len(byDoc))
	}

	dangling, err := r.ListDanglingEdges(ctx, "c1")
	if err != nil {
		t.Fatalf("ListDanglingEdges: %v", err)
	}
	if len(dangling) != 1 || dangling[0].RawTarget != "Ghost" || dangling[0].DstDocID != "" {
		t.Fatalf("dangling 错误: %+v", dangling)
	}
}

// TestBlockLinkReader_Empty 无匹配返回空而非错误。
func TestBlockLinkReader_Empty(t *testing.T) {
	r := setupKnowledgeBlocksRepo(t)
	seedBacklinkGraph(t, r)
	ctx := context.Background()

	edges, err := r.ListBacklinksByDoc(ctx, "d1")
	if err != nil || len(edges) != 0 {
		t.Fatalf("无人引用 d1 = %v/%v, want 空/nil", edges, err)
	}
	dangling, err := r.ListDanglingEdges(ctx, "c-nonexist")
	if err != nil || len(dangling) != 0 {
		t.Fatalf("异集合 dangling = %v/%v, want 空/nil", dangling, err)
	}
}

// TestListDocumentNames 批量名解析：rel_path 优先，空 rel_path 回 source，
// 未知 id 缺席；空入参短路空 map。
func TestListDocumentNames(t *testing.T) {
	r := setupKnowledgeBlocksRepo(t)
	ctx := context.Background()
	kr := &knowledgeRepo{data: r.data}
	seedDoc(t, r, "c1", "d1") // rel_path 空 → source 兜底
	seedDoc(t, r, "c1", "d2")
	if err := kr.UpdateDocumentRelPath(ctx, "d2", "notes/b.md"); err != nil {
		t.Fatalf("update rel_path: %v", err)
	}

	names, err := kr.ListDocumentNames(ctx, []string{"d1", "d2", "d-ghost"})
	if err != nil {
		t.Fatalf("ListDocumentNames: %v", err)
	}
	if names["d1"] != "d1.md" {
		t.Errorf("d1 名 = %q, want source 兜底 d1.md", names["d1"])
	}
	if names["d2"] != "notes/b.md" {
		t.Errorf("d2 名 = %q, want rel_path 优先", names["d2"])
	}
	if _, ok := names["d-ghost"]; ok {
		t.Error("未知 id 应缺席")
	}

	empty, err := kr.ListDocumentNames(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("空入参 = %v/%v, want 空/nil", empty, err)
	}
}
