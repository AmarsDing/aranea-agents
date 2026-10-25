package data

import (
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// knowledge_g32_t5_e2e_test.go G-32 T5 PG e2e 钉阵（设计文档测试矩阵 ③⑨ 的
// e2e 缺口收口）：③ 缺 dim 条件 upsert 收敛（pending 列入列→同体重嵌补齐→
// 出列→同 dim fast-path）；⑨ 时态生命周期（v1→v2→tombstone 全程 as-of 双腿
// 与 current 对照——as-of 命中编辑前化身、current 零泄漏、created_at 等值归
// 新区间、双腿 top-k 合并、tombstone 后可回放而 current 查无）。
// 提交级 CON-19B 与化身区间内核已分由 knowledge_diff_commit_test.go 与
// knowledge_chunk_archive_test.go 钉住，本文件钉接线后的端到端语义。

// t5Contents 收集化身内容集（断言只关心存在性）。
func t5Contents(items []bizknowledge.KnowledgeChunkIncarnation) map[string]bool {
	out := map[string]bool{}
	for _, it := range items {
		out[it.Content] = true
	}
	return out
}

// t5Parent 父块语境行（IsParent=true，无向量——父行不入 embed/侧表，
// 仅摄取写路径使用，与 ingest.go 混合 chunker 输出同形）。
func t5Parent(docID, content string, idx int) biz.KnowledgeChunk {
	return biz.KnowledgeChunk{DocID: docID, CollectionID: "c1", Content: content, ChunkIndex: idx, IsParent: true}
}

// ── R-2 子块口径钉（wf_f76a9b06-996）：帧 chunk_count 与台账同轨但父行入流 ──
// parent_child 摄取形状（1 父 + 2 子）：stats 双口径必须分叉——Added/Kept
// 全行口径含父（fast-path/版本行语义），ChildAdded/ChildKept 只数子块；帧
// chunk_count=ChildAdded+ChildKept 恰等于 documents.chunk_count 台账值 2。
// 若有人把帧取值改回 Added+Kept，本钉与 service 层分叉钉（meta chunk_count=4
// ≠ 全行 5）双保险立即变红。
func TestG32T5ChunkDiffStatsChildCaliber(t *testing.T) {
	ctx, db, repo := t4Repo(t)
	t4SeedIndexing(t, ctx, db, "d1", "v1 body")

	set := []biz.KnowledgeChunk{
		t5Parent("d1", "parent ctx", 0),
		t4Child("d1", "child a", 1, 3, ""),
		t4Child("d1", "child b", 2, 3, ""),
	}
	stats, err := repo.CommitIndexedDocument(ctx, "c1", "d1", set, 1, nil)
	if err != nil {
		t.Fatalf("commit v1: %v", err)
	}
	if stats.Added != 3 || stats.Deleted != 0 || stats.Kept != 0 {
		t.Fatalf("全行口径 = +%d -%d =%d, want +3 -0 =0（父行亦计入配对计划）",
			stats.Added, stats.Deleted, stats.Kept)
	}
	if stats.ChildAdded != 2 || stats.ChildKept != 0 {
		t.Fatalf("子块口径 = ChildAdded %d ChildKept %d, want 2/0（父行必不入）",
			stats.ChildAdded, stats.ChildKept)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT chunk_count FROM knowledge_documents WHERE id='d1'`); got != 2 {
		t.Fatalf("documents.chunk_count = %d, want 2（台账=子块口径）", got)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM knowledge_chunks WHERE doc_id='d1' AND chunk_role='parent'`); got != 1 {
		t.Fatalf("parent rows = %d, want 1", got)
	}
	// 帧值=台账值：chunk_count=ChildAdded+ChildKept（service 层逐值相等钉已对位）。
	if got := stats.ChildAdded + stats.ChildKept; got != 2 {
		t.Fatalf("帧 chunk_count = %d, want 2（与台账同轨）", got)
	}

	// 同体同 dim 再摄（先按摄取惯例 reopen 回 indexing，与 ③ 测试同形）：
	// fast-path 全保留——父行参与配对（pass 0）但子块口径
	// 归零父：Kept=3 全行、ChildKept=2 子块；台账不动。
	t4Reopen(t, ctx, db, "d1", "")
	stats2, err := repo.CommitIndexedDocument(ctx, "c1", "d1", set, 0, nil)
	if err != nil {
		t.Fatalf("commit v1 again: %v", err)
	}
	if !stats2.FastPath {
		t.Fatal("同体同 dim 再摄须 fast-path（父行不阻断缺 dim 前哨）")
	}
	if stats2.Kept != 3 || stats2.ChildAdded != 0 || stats2.ChildKept != 2 {
		t.Fatalf("fast-path 口径 = Kept %d ChildAdded %d ChildKept %d, want 3/0/2",
			stats2.Kept, stats2.ChildAdded, stats2.ChildKept)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT chunk_count FROM knowledge_documents WHERE id='d1'`); got != 2 {
		t.Fatalf("fast-path 后 documents.chunk_count = %d, want 2", got)
	}
}

// ── ③ 缺 dim 条件 upsert 收敛 e2e（pending→reembed→侧表补齐→出列）──────────
func TestG32T5PendingDimConvergeEndToEnd(t *testing.T) {
	ctx, db, repo := t4Repo(t)
	t4SeedIndexing(t, ctx, db, "d1", "v1 body")

	// v1：dim3 摄取（模拟旧模型世代）。
	if _, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 3, "")}, 1, nil); err != nil {
		t.Fatalf("commit v1: %v", err)
	}
	pendingList := func(dim int) []string {
		t.Helper()
		docs, err := repo.ListDocumentsPendingReembed(ctx, "c1", dim)
		if err != nil {
			t.Fatalf("pending list dim=%d: %v", dim, err)
		}
		ids := []string{}
		for _, d := range docs {
			ids = append(ids, d.ID)
		}
		return ids
	}
	// 入列语义：缺 dim4 在列；dim3 已覆不在列（双向判据防恒真/恒假谓词）。
	if ids := pendingList(4); len(ids) != 1 || ids[0] != "d1" {
		t.Fatalf("pending dim4 = %v, want [d1]（换模型后缺维应在列）", ids)
	}
	if ids := pendingList(3); len(ids) != 0 {
		t.Fatalf("pending dim3 = %v, want []（已覆维度不得误列）", ids)
	}

	// 同体换 dim 重嵌（embedder 已换 dim4）：CON-19B 降级放行，侧表补 dim4 行。
	t4Reopen(t, ctx, db, "d1", "")
	stats, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 4, "")}, 0, nil)
	if err != nil {
		t.Fatalf("dim-fill commit: %v", err)
	}
	if stats.FastPath || stats.Kept != 1 {
		t.Fatalf("dim-fill stats wrong: %+v（缺维补齐不得 fast-path）", stats)
	}
	// 出列收敛：dim4 已补 → 出列；dim5 仍缺 → 在列（列表真实反映而非恒空）。
	if ids := pendingList(4); len(ids) != 0 {
		t.Fatalf("pending dim4 after fill = %v, want []（补齐即出列收敛）", ids)
	}
	if ids := pendingList(5); len(ids) != 1 || ids[0] != "d1" {
		t.Fatalf("pending dim5 = %v, want [d1]（未覆维度仍在列）", ids)
	}
	// 侧表双代共存：dim3 旧行不废、dim4 新行补齐。
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_embeddings WHERE doc_id='d1'`); got != 2 {
		t.Fatalf("side rows = %d, want 2（dim3/dim4 双代共存）", got)
	}

	// 收敛后同 dim 再摄：fast-path 归位（缺维前哨通过，零写）。
	t4Reopen(t, ctx, db, "d1", "")
	stats, err = repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 4, "")}, 0, nil)
	if err != nil {
		t.Fatalf("post-fill commit: %v", err)
	}
	if !stats.FastPath {
		t.Fatalf("post-fill must be fast-path: %+v（dim 补齐后同体重摄零写）", stats)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_embeddings WHERE doc_id='d1'`); got != 2 {
		t.Fatalf("side rows after fast-path = %d, want 2（零写）", got)
	}
}

// ── ⑨ 时态生命周期 e2e（v1→v2→tombstone：as-of 双腿 + current 对照）─────────
func TestG32T5TemporalLifecycleEndToEnd(t *testing.T) {
	ctx, db, repo := t4Repo(t)
	t4SeedIndexing(t, ctx, db, "d1", "v1 body")
	qEmb := t4Child("d1", "alpha", 0, 3, "").Embedding // 稠密查询向量（dim3）

	// v1：alpha + beta 两代际首版。
	if _, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 3, ""), t4Child("d1", "beta", 1, 3, "")}, 1, nil); err != nil {
		t.Fatalf("commit v1: %v", err)
	}
	versionTime := func(ver int) time.Time {
		t.Helper()
		var ts time.Time
		if err := db.QueryRowContext(ctx,
			`SELECT created_at FROM document_versions WHERE doc_id='d1' AND version_no=$1`, ver).Scan(&ts); err != nil {
			t.Fatalf("version %d time: %v", ver, err)
		}
		return ts
	}
	t1 := versionTime(1)
	asOfDense := func(at time.Time, topK int) map[string]bool {
		t.Helper()
		items, err := repo.RecallChunksAsOf(ctx, "c1", qEmb, bizknowledge.TemporalSpan{ValidAt: at}, topK, "")
		if err != nil {
			t.Fatalf("dense as-of %v: %v", at, err)
		}
		return t5Contents(items)
	}
	asOfSparse := func(query string, at time.Time) map[string]bool {
		t.Helper()
		items, err := repo.SparseRecallChunksAsOf(ctx, "c1", query, bizknowledge.TemporalSpan{ValidAt: at}, 10, "")
		if err != nil {
			t.Fatalf("sparse %q as-of %v: %v", query, at, err)
		}
		return t5Contents(items)
	}

	// as-of t1（编辑前时点）：双腿命中 v1 化身（含将死的 beta）。
	if got := asOfDense(t1, 10); !got["alpha"] || !got["beta"] {
		t.Fatalf("dense as-of t1 = %v, want alpha+beta（编辑前化身）", got)
	}
	if got := asOfSparse("beta", t1); !got["beta"] {
		t.Fatalf("sparse as-of t1 = %v, want beta（词法腿同步命中历史化身）", got)
	}

	// v2：混合差分（alpha 保留、beta 孤儿、gamma 新增）。
	time.Sleep(5 * time.Millisecond) // 账本时刻错位护栏（µs 级同 tick 防撞）
	t4Reopen(t, ctx, db, "d1", "v2 body")
	if _, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 3, ""), t4Child("d1", "gamma", 1, 3, "")}, 0, nil); err != nil {
		t.Fatalf("commit v2: %v", err)
	}
	t2 := versionTime(2)
	if !t2.After(t1) {
		t.Fatalf("version times not ordered: t1=%v t2=%v（边界归组前提破）", t1, t2)
	}

	// 回放：as-of t1 仍命中编辑前化身（beta 已死亡化但不灭迹）。
	if got := asOfDense(t1, 10); !got["alpha"] || !got["beta"] || got["gamma"] {
		t.Fatalf("dense replay t1 = %v, want alpha+beta 且无 gamma（未来不得倒流）", got)
	}
	// current 零泄漏：零 span 退化 now——beta 不得出现在当下腿。
	if got := asOfDense(time.Time{}, 10); got["beta"] || !got["alpha"] || !got["gamma"] {
		t.Fatalf("dense current = %v, want alpha+gamma 且零 beta 泄漏", got)
	}
	if got := asOfSparse("beta", time.Time{}); got["beta"] {
		t.Fatalf("sparse current leaked beta: %v（死亡化身不得进当下召回）", got)
	}
	// top-k 合并钉：双腿 RRF/距离裁并后 topK=1 恒一条且为最近邻 alpha。
	if got := asOfDense(t1, 1); len(got) != 1 || !got["alpha"] {
		t.Fatalf("dense topK=1 = %v, want 恰一条且为 alpha（合并裁剪纪律）", got)
	}

	// 区间边界：ValidAt 等值 v2.created_at 归新区间（左闭右开——归档腿
	// [t1,t2) 在等值点闭合、v2 代际当前腿 [t2,∞) 自此存活）。
	if got := asOfDense(t2, 10); got["beta"] || !got["gamma"] {
		t.Fatalf("boundary at v2.created_at = %v, want gamma 存活 beta 闭合（等值归新）", got)
	}

	// tombstone：DeleteDocument 终态——as-of t1 可回放而 current 查无。
	time.Sleep(5 * time.Millisecond)
	if err := repo.DeleteDocument(ctx, "d1"); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	if got := asOfDense(t1, 10); !got["alpha"] || !got["beta"] {
		t.Fatalf("post-tombstone replay t1 = %v, want alpha+beta（tombstone 不灭历史）", got)
	}
	if got := asOfDense(time.Time{}, 10); len(got) != 0 {
		t.Fatalf("post-tombstone current = %v, want 空（tombstone 后 current 查无）", got)
	}
	if got := asOfSparse("beta", time.Time{}); len(got) != 0 {
		t.Fatalf("post-tombstone sparse current = %v, want 空", got)
	}
}
