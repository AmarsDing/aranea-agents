package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// knowledge_diff_commit_test.go G-32 T4 钉阵（ADR-KN-20261022-I 决策 5 十三步
// diff-commit tx + 决策 7 引用回落接线 + 决策 12 时态读装配守卫）：
// 混合差分全账、fast-path 零写、CON-19 缺 dim 降级放行、kept 实变传播、
// tombstone 入口集成、引用回落集成、时态守卫/RRF 同轨纯函数、as-of ACL
// 复合谓词。档案内核行为由 knowledge_chunk_archive_test.go 覆盖，本文件钉
// 接线后的端到端语义。

func t4Repo(t *testing.T) (context.Context, *sql.DB, *knowledgeRepo) {
	t.Helper()
	ctx, db := g32T3Base(t)
	d := &Data{rawDB: db, readDB: db, pg: db, pgRead: db,
		rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	return ctx, db, &knowledgeRepo{data: d, lg: loggateway.NewNoop()}
}

func t4ScanInt(t *testing.T, ctx context.Context, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		t.Fatalf("scan %.60q: %v", q, err)
	}
	return n
}

// t4SeedIndexing 集合 c1 + 摄取态文档（content_hash=正文 hash；status=indexing）。
func t4SeedIndexing(t *testing.T, ctx context.Context, db *sql.DB, docID, body string) {
	t.Helper()
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'c1', 'm0')`)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_documents
		(id, collection_id, source, content_text, content_hash, status, created_at)
		VALUES ($1, 'c1', 's1', $2, $3, 'indexing', $4)`,
		docID, body, bizknowledge.HashContent(body), t3T0)
}

// t4Reopen 模拟再摄取开端：status 回 indexing；newBody 非空时换正文但
// content_hash 列保持旧值（摄取期列由 syncMeta 在 ⑬ 才更新——与生产形态一致）。
func t4Reopen(t *testing.T, ctx context.Context, db *sql.DB, docID, newBody string) {
	t.Helper()
	if newBody != "" {
		mustExecG32T3(t, ctx, db,
			`UPDATE knowledge_documents SET content_text = $2, status = 'indexing' WHERE id = $1`, docID, newBody)
		return
	}
	mustExecG32T3(t, ctx, db,
		`UPDATE knowledge_documents SET status = 'indexing' WHERE id = $1`, docID)
}

func t4Child(docID, content string, idx, dim int, meta string) biz.KnowledgeChunk {
	emb := make([]float32, dim)
	for i := range emb {
		emb[i] = 0.01*float32(idx+1) + 0.0001*float32(i+1)
	}
	ch := biz.KnowledgeChunk{DocID: docID, CollectionID: "c1", Content: content, ChunkIndex: idx, Embedding: emb}
	if meta != "" {
		ch.MetadataJSON = meta
	}
	return ch
}

// ── ① 混合差分全账（③④⑤⑥⑦⑧⑨⑩⑫⑬ 一次走全）──────────────────────────────
func TestG32T4DiffCommitMixed(t *testing.T) {
	ctx, db, repo := t4Repo(t)
	t4SeedIndexing(t, ctx, db, "d1", "v1 body")
	h1 := bizknowledge.HashContent("v1 body")

	stats, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 3, ""), t4Child("d1", "beta", 1, 3, "")}, 1, nil)
	if err != nil {
		t.Fatalf("commit v1: %v", err)
	}
	if stats.FastPath || stats.Added != 2 || stats.Deleted != 0 || stats.Kept != 0 || stats.VersionNo != 1 {
		t.Fatalf("v1 stats wrong: %+v", stats)
	}
	// ⑩ 代际命名 + ⑬ guard/状态 + 集合计数。
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM knowledge_documents WHERE id='d1'`).Scan(&status); err != nil {
		t.Fatalf("doc status: %v", err)
	}
	if status != "indexed" {
		t.Fatalf("guard CAS: status=%q want indexed", status)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT document_count FROM knowledge_collections WHERE id='c1'`); got != 1 {
		t.Fatalf("document_count=%d want 1", got)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT chunk_count FROM knowledge_collections WHERE id='c1'`); got != 2 {
		t.Fatalf("chunk_count=%d want 2", got)
	}

	// 第二轮：混合差分（beta 保留、alpha 孤儿、gamma 新增）。
	t4Reopen(t, ctx, db, "d1", "v2 body")
	stats, err = repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "beta", 0, 3, ""), t4Child("d1", "gamma", 1, 3, "")}, 0, nil)
	if err != nil {
		t.Fatalf("commit v2: %v", err)
	}
	if stats.FastPath || stats.Added != 1 || stats.Deleted != 1 || stats.Kept != 1 || stats.VersionNo != 2 {
		t.Fatalf("v2 stats wrong: %+v", stats)
	}
	// ⑨ kept 行 ID 不换、chunk_index 重排；⑩ added 代际命名。
	var keptID, addedID string
	var keptIdx int
	if err := db.QueryRowContext(ctx,
		`SELECT id, chunk_index FROM knowledge_chunks WHERE doc_id='d1' AND content='beta'`).Scan(&keptID, &keptIdx); err != nil {
		t.Fatalf("kept row: %v", err)
	}
	if keptID != "d1-v1-ch-1" || keptIdx != 0 {
		t.Fatalf("kept identity/index drift: id=%q idx=%d", keptID, keptIdx)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM knowledge_chunks WHERE doc_id='d1' AND content='gamma'`).Scan(&addedID); err != nil {
		t.Fatalf("added row: %v", err)
	}
	if addedID != "d1-v2-ch-1" {
		t.Fatalf("generational name: id=%q want d1-v2-ch-1", addedID)
	}
	// ⑦ 孤儿整档入 chunk_archive；⑧ 当前表孤儿已删。
	if got := t4ScanInt(t, ctx, db,
		`SELECT count(*) FROM chunk_archive WHERE doc_id='d1' AND chunk_id='d1-v1-ch-0' AND revision_no=2`); got != 1 {
		t.Fatalf("archive kill row=%d want 1", got)
	}
	var archContent string
	if err := db.QueryRowContext(ctx,
		`SELECT content FROM chunk_archive WHERE chunk_id='d1-v1-ch-0' AND revision_no=2`).Scan(&archContent); err != nil {
		t.Fatalf("archive snapshot: %v", err)
	}
	if archContent != "alpha" {
		t.Fatalf("archive content=%q want alpha", archContent)
	}
	if got := t4ScanInt(t, ctx, db,
		`SELECT count(*) FROM knowledge_chunk_incarnations WHERE chunk_id='d1-v1-ch-0' AND archived`); got != 1 {
		t.Fatalf("archived incarnation=%d want 1", got)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM knowledge_chunks WHERE doc_id='d1'`); got != 2 {
		t.Fatalf("current chunks=%d want 2", got)
	}
	// 侧表：孤儿级联清、kept 保留、added 新生 → 净 2 行。
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_embeddings WHERE doc_id='d1'`); got != 2 {
		t.Fatalf("side rows=%d want 2", got)
	}
	// ⑫ 版本双行：v1（无 head 触发）+ v2（head hash 不等/混合触发）；
	// AssetHash 镜像 ① 锁时列值（本测试 syncMeta 空 → 恒 h1）。
	var producer, kind, aHash string
	var stAdded, stDeleted, stKept int
	if err := db.QueryRowContext(ctx, `SELECT producer, change_kind, asset_hash,
		(chunk_stats->>'added')::int, (chunk_stats->>'deleted')::int, (chunk_stats->>'kept')::int
		FROM document_versions WHERE doc_id='d1' AND version_no=2`).
		Scan(&producer, &kind, &aHash, &stAdded, &stDeleted, &stKept); err != nil {
		t.Fatalf("v2 ledger row: %v", err)
	}
	if producer != "ingest" || kind != "edit" {
		t.Fatalf("v2 producer/kind wrong: %q/%q", producer, kind)
	}
	if stAdded != 1 || stDeleted != 1 || stKept != 1 {
		t.Fatalf("v2 chunk_stats=(%d,%d,%d) want (1,1,1)", stAdded, stDeleted, stKept)
	}
	if aHash != h1 {
		t.Fatalf("v2 asset_hash=%q want lock-time column %q", aHash, h1)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT chunk_count FROM knowledge_collections WHERE id='c1'`); got != 2 {
		t.Fatalf("post-diff chunk_count=%d want 2", got)
	}
}

// ── ② fast-path 零写（多重集恒等+列 hash 相等）────────────────────────────
func TestG32T4FastPathZeroWrites(t *testing.T) {
	ctx, db, repo := t4Repo(t)
	t4SeedIndexing(t, ctx, db, "d1", "v1 body")
	if _, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 3, ""), t4Child("d1", "beta", 1, 3, "")}, 1, nil); err != nil {
		t.Fatalf("commit v1: %v", err)
	}

	t4Reopen(t, ctx, db, "d1", "")
	stats, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 3, ""), t4Child("d1", "beta", 1, 3, "")}, 0, nil)
	if err != nil {
		t.Fatalf("fast-path commit: %v", err)
	}
	if !stats.FastPath || stats.Added != 0 || stats.Deleted != 0 || stats.Kept != 2 {
		t.Fatalf("fast-path stats wrong: %+v", stats)
	}
	if stats.VersionNo != 1 { // 透出当前 head（零写不留新号）。
		t.Fatalf("fast-path VersionNo=%d want head 1", stats.VersionNo)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM document_versions WHERE doc_id='d1'`); got != 1 {
		t.Fatalf("ledger rows=%d want 1", got)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_archive WHERE doc_id='d1'`); got != 0 {
		t.Fatalf("archive rows=%d want 0", got)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_embeddings WHERE doc_id='d1'`); got != 2 {
		t.Fatalf("side rows=%d want 2", got)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM knowledge_documents WHERE id='d1'`).Scan(&status); err != nil {
		t.Fatalf("doc status: %v", err)
	}
	if status != "indexed" { // ⑬ guard CAS 在 fast-path 同样原序执行。
		t.Fatalf("status=%q want indexed", status)
	}
}

// ── CON-19 修正钉：同 body 换 dim 重嵌必须降级放行 ⑪ ────────────────────────
// 底层病灶：fast-path 判定（同 body + 多重集恒等）曾让 ⑪ 旁路——新维向量静默
// 丢失、pending-reembed 死循环。本项目在 T4 钉阵期修正：fast-path 前哨
// pendingDimFillTx 现检缺 dim，缺则降级走全链（⑫ 双条件天然抑制伪版本行）。
func TestG32T4FastPathYieldsToPendingDimFill(t *testing.T) {
	ctx, db, repo := t4Repo(t)
	t4SeedIndexing(t, ctx, db, "d1", "v1 body")
	if _, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 3, "")}, 1, nil); err != nil {
		t.Fatalf("commit v1: %v", err)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_embeddings WHERE doc_id='d1'`); got != 1 {
		t.Fatalf("side rows after v1=%d want 1", got)
	}

	// 同 body、同内容、仅向量维度 3→4（G-39 换模型重嵌形态）：不得 fast-path。
	t4Reopen(t, ctx, db, "d1", "")
	stats, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 4, "")}, 0, nil)
	if err != nil {
		t.Fatalf("dim-fill commit: %v", err)
	}
	if stats.FastPath {
		t.Fatalf("CON-19 regression: pending dim fill was fast-pathed (%+v)", stats)
	}
	if stats.Added != 0 || stats.Deleted != 0 || stats.Kept != 1 {
		t.Fatalf("dim-fill stats wrong: %+v", stats)
	}
	// ⑫ 双条件抑制伪版本：head hash 相等且 added/deleted 双零 → 账本恒 1 行。
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM document_versions WHERE doc_id='d1'`); got != 1 {
		t.Fatalf("phantom version written: ledger rows=%d want 1", got)
	}
	// ⑪ 新维落行、旧维保留（多代共存）。
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_embeddings WHERE doc_id='d1'`); got != 2 {
		t.Fatalf("side rows after dim fill=%d want 2", got)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_embeddings WHERE doc_id='d1' AND dim=4`); got != 1 {
		t.Fatalf("dim-4 side row=%d want 1", got)
	}

	// 补齐后同 dim 再摄：真正的 fast-path（存在谓词通过）。
	t4Reopen(t, ctx, db, "d1", "")
	stats, err = repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 4, "")}, 0, nil)
	if err != nil {
		t.Fatalf("post-fill commit: %v", err)
	}
	if !stats.FastPath || stats.VersionNo != 1 {
		t.Fatalf("post-fill must be fast-path at head 1: %+v", stats)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_embeddings WHERE doc_id='d1'`); got != 2 {
		t.Fatalf("side rows after real fast-path=%d want 2", got)
	}
}

// ── ⑨ kept 实变传播：index 重排 + metadata 更新；⑫ head-hash 条件单独触发 ─
// 正文已变但块多重集恒等 → added/deleted 双零，版本行由 head hash 不等单独
// 触发写入（消歧：不依赖「混合差分才写账」的那条腿）。
func TestG32T4KeptPropagatesMetadataAndIndex(t *testing.T) {
	ctx, db, repo := t4Repo(t)
	t4SeedIndexing(t, ctx, db, "d1", "v1 body")
	if _, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "alpha", 0, 3, `{"k":"v1"}`), t4Child("d1", "beta", 1, 3, `{"k":"v1"}`)}, 1, nil); err != nil {
		t.Fatalf("commit v1: %v", err)
	}

	// 正文变化 + 同块换序 + metadata 换代。
	t4Reopen(t, ctx, db, "d1", "v2 body")
	stats, err := repo.CommitIndexedDocument(ctx, "c1", "d1",
		[]biz.KnowledgeChunk{t4Child("d1", "beta", 0, 3, `{"k":"v2"}`), t4Child("d1", "alpha", 1, 3, `{"k":"v2"}`)}, 0, nil)
	if err != nil {
		t.Fatalf("commit v2: %v", err)
	}
	if stats.FastPath || stats.Added != 0 || stats.Deleted != 0 || stats.Kept != 2 || stats.VersionNo != 2 {
		t.Fatalf("stats wrong: %+v", stats)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, chunk_index, metadata->>'k' FROM knowledge_chunks WHERE doc_id='d1' ORDER BY chunk_index`)
	if err != nil {
		t.Fatalf("read kept rows: %v", err)
	}
	defer rows.Close()
	type keptRow struct {
		id  string
		idx int
		k   string
	}
	got := []keptRow{}
	for rows.Next() {
		var r keptRow
		if err := rows.Scan(&r.id, &r.idx, &r.k); err != nil {
			t.Fatalf("scan kept: %v", err)
		}
		got = append(got, r)
	}
	want := []keptRow{{"d1-v1-ch-1", 0, "v2"}, {"d1-v1-ch-0", 1, "v2"}}
	if len(got) != len(want) {
		t.Fatalf("kept rows=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kept[%d]=%+v want %+v(identity kept, index/meta propagated)", i, got[i], want[i])
		}
	}
	// ⑫ head-hash 腿：added/deleted 双零下版本行仍入列。
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM document_versions WHERE doc_id='d1'`); got != 2 {
		t.Fatalf("ledger rows=%d want 2 (head-hash leg fired)", got)
	}
	var stAdded, stDeleted, stKept int
	if err := db.QueryRowContext(ctx,
		`SELECT (chunk_stats->>'added')::int, (chunk_stats->>'deleted')::int, (chunk_stats->>'kept')::int
		 FROM document_versions WHERE doc_id='d1' AND version_no=2`).Scan(&stAdded, &stDeleted, &stKept); err != nil {
		t.Fatalf("v2 row: %v", err)
	}
	if stAdded != 0 || stDeleted != 0 || stKept != 2 {
		t.Fatalf("v2 chunk_stats=(%d,%d,%d) want (0,0,2)", stAdded, stDeleted, stKept)
	}
	if got := t4ScanInt(t, ctx, db, `SELECT count(*) FROM chunk_archive WHERE doc_id='d1'`); got != 0 {
		t.Fatalf("archive rows=%d want 0", got)
	}
}
