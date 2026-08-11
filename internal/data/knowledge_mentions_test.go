package data

import (
	"context"
	"testing"
)

// ── P2-7：DocContentSearcher 候选扫描（ILIKE 预筛 + 通配符转义） ────────────

// TestSearchDocContentMentions ILIKE 大小写不敏感候选 + 目标排除 + 通配符转义。
func TestSearchDocContentMentions(t *testing.T) {
	br := setupKnowledgeBlocksRepo(t)
	r := &knowledgeRepo{data: br.data, lg: br.lg}
	ctx := context.Background()

	seedDoc(t, br, "c1", "d1") // 目标（应被排除）
	seedDoc(t, br, "c1", "d2")
	seedDoc(t, br, "c1", "d3")
	seedDoc(t, br, "c1", "d4")
	setDocContent(t, r, "d2", "这篇提到目标笔记。")
	setDocContent(t, r, "d3", "完全无关的内容。")
	setDocContent(t, r, "d4", "达 50% 增长，不是 500。")

	// 中文名候选：仅 d2 命中（大小写不敏感由 ILIKE 承担）。
	hits, err := r.SearchDocContentMentions(ctx, "c1", "目标笔记", "d1", 10)
	if err != nil {
		t.Fatalf("SearchDocContentMentions: %v", err)
	}
	if len(hits) != 1 || hits[0].DocID != "d2" || hits[0].DocName != "d2.md" || hits[0].Content == "" {
		t.Fatalf("hits = %+v, want 仅 d2", hits)
	}

	// 通配符转义：needle "50%" 必须字面匹配（不命中 "500"）。
	hits, err = r.SearchDocContentMentions(ctx, "c1", "50%", "d1", 10)
	if err != nil {
		t.Fatalf("escape: %v", err)
	}
	if len(hits) != 1 || hits[0].DocID != "d4" {
		t.Fatalf("通配符转义失效: %+v", hits)
	}

	// 无命中 → 空而非错误。
	hits, err = r.SearchDocContentMentions(ctx, "c1", "不存在的词", "d1", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("无命中应返回空: hits=%v err=%v", hits, err)
	}
}

func setDocContent(t *testing.T, r *knowledgeRepo, docID, content string) {
	t.Helper()
	if _, err := r.data.rawDB.ExecContext(context.Background(),
		`UPDATE knowledge_documents SET content_text = $2 WHERE id = $1`, docID, content); err != nil {
		t.Fatalf("set content %s: %v", docID, err)
	}
}
