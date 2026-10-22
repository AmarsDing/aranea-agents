package data

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
)

// knowledge_chunk_archive_test.go G-32 T3 钉阵（ADR-KN-20261022-I 测试矩阵 ④⑤⑨）：
// 归档 writer/向量镜档/tombstone 整档/回落读内核/时态双腿 的全部 PG 行为。
// 时间戳全部显式注入以脱离时钟漂移——版本台账行的 created_at 是化身区间
// 的连接点，夹具必须用字面时刻构造确定性区间。

func g32T3Base(t *testing.T) (context.Context, *sql.DB) {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	return ctx, db
}

func mustExecG32T3(t *testing.T, ctx context.Context, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, q, args...); err != nil {
		t.Fatalf("exec %.120q: %v", q, err)
	}
}

// ── 以下为真实测试体 ─────────────────────────────────────────────────────

const (
	t3T0 = "2026-10-01 00:00:00Z"
	t3T1 = "2026-10-02 00:00:00Z"
	t3T2 = "2026-10-03 00:00:00Z"
)

// seedT3Doc d1：v1 出生（seed，created_at=t3T0），两块 v1 namespace +
// 文本 body；d2 在孤儿镜档专用夹具中另起。
func seedT3Doc(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'c1', 'm0')`)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_documents (id, collection_id, source, content_text, created_at)
		VALUES ('d1', 'c1', 's1', 'body v1', $1)`, t3T0)
	mustExecG32T3(t, ctx, db, `INSERT INTO document_versions (doc_id, version_no, content_hash, change_kind, producer, created_at)
		VALUES ('d1', 1, 'sha1:v1', 'seed', 'g32-seed', $1)`, t3T0)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, chunk_index) VALUES
		('d1-v1-ch-0', 'd1', 'c1', 'alpha', 0),
		('d1-v1-ch-1', 'd1', 'c1', 'beta', 1)`)
}

func TestG32T3VersionWriterIdempotent(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("exec %.120q: %v", q, err)
		}
	}
	must(`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'c1', 'm0')`)
	must(`INSERT INTO knowledge_documents (id, collection_id, source, content_text, created_at)
		VALUES ('d1', 'c1', 's1', 'body', $1)`, t3T0)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 空档 → 1；写入后 → 2（决策 8：COALESCE(MAX,0)+1）。
	v, err := nextDocVersionNoTx(ctx, tx, "d1")
	if err != nil || v != 1 {
		t.Fatalf("next ver on empty ledger: got %d err %v, want 1", v, err)
	}
	if err := insertDocumentVersionTx(ctx, tx, documentVersionRow{
		DocID: "d1", VersionNo: 1, ContentHash: "sha1:h1", ChangeKind: "seed",
		ChunkStats: []byte(`{"chunk_count": 2}`), Producer: "g32-seed",
	}); err != nil {
		t.Fatalf("insert v1: %v", err)
	}
	v, err = nextDocVersionNoTx(ctx, tx, "d1")
	if err != nil || v != 2 {
		t.Fatalf("next ver after v1: got %d err %v, want 2", v, err)
	}
	// 幂等重放：同 (doc,ver) 第二次写入被静默忽略且 FIRST-WINS（producer 不换）。
	if err := insertDocumentVersionTx(ctx, tx, documentVersionRow{
		DocID: "d1", VersionNo: 1, ChangeKind: "edit", Producer: "replay-intruder",
	}); err != nil {
		t.Fatalf("replay insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	var producer, kind, stats string
	if err := db.QueryRowContext(ctx,
		`SELECT producer, change_kind, chunk_stats::text FROM document_versions WHERE doc_id='d1' AND version_no=1`).
		Scan(&producer, &kind, &stats); err != nil {
		t.Fatalf("readback: %v", err)
	}
	if producer != "g32-seed" || kind != "seed" {
		t.Fatalf("replay overwrote row: producer=%q kind=%q", producer, kind)
	}
	var cnt int
	must2 := db.QueryRowContext(ctx, `SELECT count(*) FROM document_versions WHERE doc_id='d1'`)
	if err := must2.Scan(&cnt); err != nil || cnt != 1 {
		t.Fatalf("ledger dup: got %d rows", cnt)
	}
	if stats != `{"chunk_count": 2}` {
		t.Fatalf("chunk_stats drift: %q", stats)
	}
}

// ② 归档 writer：全字段快照（content/cited_count/metadata/chunk_role 随行）、
// 重放幂等；化身视图归档腿区间=[birthVersion.created_at, killVersion.created_at左闭右开)。
func TestG32T3ArchiveWriterSnapshotAndViewInterval(t *testing.T) {
	ctx, db := g32T3Base(t)
	seedT3Doc(t, ctx, db)
	mustExecG32T3(t, ctx, db, `UPDATE knowledge_chunks SET cited_count = 7,
		metadata = '{"p": "x"}'::jsonb WHERE id = 'd1-v1-ch-0'`)
	// v2 版本行（编辑死亡点 t3T1）先入列——视图 valid_to 的连接对象。
	mustExecG32T3(t, ctx, db, `INSERT INTO document_versions (doc_id, version_no, change_kind, producer, created_at)
		VALUES ('d1', 2, 'edit', 'test', $1)`, t3T1)

	tx, _ := db.BeginTx(ctx, nil)
	n, err := archiveChunksForVersionTx(ctx, tx, "d1", 2, []string{"d1-v1-ch-0", "d1-v1-ch-1"})
	if err != nil || n != 2 {
		t.Fatalf("archive run 1: n=%d err=%v want 2", n, err)
	}
	// 重放安全：同版本同集合再归档 = 0 新增。
	n, err = archiveChunksForVersionTx(ctx, tx, "d1", 2, []string{"d1-v1-ch-0", "d1-v1-ch-1"})
	if err != nil || n != 0 {
		t.Fatalf("archive replay: n=%d err=%v want 0", n, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var cited int
	var content, meta string
	if err := db.QueryRowContext(ctx,
		`SELECT content, cited_count, metadata::text FROM chunk_archive WHERE doc_id='d1' AND chunk_id='d1-v1-ch-0' AND revision_no=2`).
		Scan(&content, &cited, &meta); err != nil {
		t.Fatalf("read archive row: %v", err)
	}
	if content != "alpha" || cited != 7 || !strings.Contains(meta, `"p": "x"`) {
		t.Fatalf("snapshot drift: content=%q cited=%d meta=%q", content, cited, meta)
	}
	// 视图归档腿：valid_from=v1(t3T0)，valid_to=v2(t3T1)（左闭右开由谓词承载）。
	var validFrom, validTo time.Time
	var archived bool
	if err := db.QueryRowContext(ctx, `
		SELECT valid_from, valid_to, archived FROM knowledge_chunk_incarnations
		WHERE chunk_id = 'd1-v1-ch-0' AND archived`).Scan(&validFrom, &validTo, &archived); err != nil {
		t.Fatalf("view archive leg: %v", err)
	}
	if !archived || !validFrom.Equal(mustTime(t, t3T0)) || !validTo.Equal(mustTime(t, t3T1)) {
		t.Fatalf("incarnation interval wrong: from=%v to=%v archived=%v", validFrom, validTo, archived)
	}
	// 当前腿仍在（未删行）：同 id 双腿并存于视图。
	var legs int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM knowledge_chunk_incarnations WHERE chunk_id='d1-v1-ch-0'`).Scan(&legs); err != nil {
		t.Fatalf("count legs: %v", err)
	}
	if legs != 2 {
		t.Fatalf("view legs mismatch: got %d want 2", legs)
	}
}

// ③ 向量镜档：embedding 原文走档往返保真、dim/model/doc_id 随行；
// 选择性镜档（未列 id 不入档）。
func TestG32T3MirrorVectorRoundtrip(t *testing.T) {
	ctx, db := g32T3Base(t)
	seedT3Doc(t, ctx, db)
	mustExecG32T3(t, ctx, db, `INSERT INTO chunk_embeddings (chunk_id, dim, collection_id, doc_id, model, embedding) VALUES
		('d1-v1-ch-0', 3, 'c1', 'd1', 'm0', '[0.1,0.2,0.3]'::vector),
		('d1-v1-ch-1', 3, 'c1', 'd1', 'm0', '[0.4,0.5,0.6]'::vector)`)

	tx, _ := db.BeginTx(ctx, nil)
	n, err := mirrorChunkEmbeddingsArchiveTx(ctx, tx, []string{"d1-v1-ch-0"})
	if err != nil || n != 1 {
		t.Fatalf("mirror: n=%d err=%v want 1", n, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	var dim, total int
	var docID, model, emb string
	if err := db.QueryRowContext(ctx,
		`SELECT dim, doc_id, model, embedding::text FROM chunk_embeddings_archive WHERE chunk_id='d1-v1-ch-0'`).
		Scan(&dim, &docID, &model, &emb); err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	if dim != 3 || docID != "d1" || model != "m0" || !strings.HasPrefix(emb, "[0.1") {
		t.Fatalf("mirror row drift: dim=%d doc=%q model=%q emb=%q", dim, docID, model, emb)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM chunk_embeddings_archive`).Scan(&total); err != nil {
		t.Fatalf("count mirror: %v", err)
	}
	if total != 1 {
		t.Fatalf("unlisted id mirrored: total=%d want 1", total)
	}
}

// ④ tombstone 整档：文档行删除后——版本台账/归档行/向量镜像全部幸存
// （document_versions 无 FK 级联钉；决策12「tombstone 可回放而 current 查无」）；
// 视图闭区间：valid_to = tombstone 版本行 created_at。
func TestG32T3TombstoneSurvivesDocDelete(t *testing.T) {
	ctx, db := g32T3Base(t)
	seedT3Doc(t, ctx, db)
	mustExecG32T3(t, ctx, db, `INSERT INTO chunk_embeddings (chunk_id, dim, collection_id, doc_id, model, embedding) VALUES
		('d1-v1-ch-0', 3, 'c1', 'd1', 'm0', '[0.1,0.2,0.3]'::vector)`)

	tx, _ := db.BeginTx(ctx, nil)
	ver, err := tombstoneDocumentTx(ctx, tx, "d1", "test-tombstone")
	if err != nil || ver != 2 {
		t.Fatalf("tombstone: ver=%d err=%v want 2", ver, err)
	}
	// 调用方责任（T4 接线 DeleteDocument）：同事务删除 documents 行。
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_documents WHERE id = 'd1'`); err != nil {
		t.Fatalf("delete doc: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var versions, archiveRows, mirrorRows, currentChunks int
	scan1 := func(q string, dst *int) {
		t.Helper()
		if err := db.QueryRowContext(ctx, q).Scan(dst); err != nil {
			t.Fatalf("scan %.60q: %v", q, err)
		}
	}
	scan1(`SELECT count(*) FROM document_versions WHERE doc_id='d1'`, &versions)
	scan1(`SELECT count(*) FROM chunk_archive WHERE doc_id='d1'`, &archiveRows)
	scan1(`SELECT count(*) FROM chunk_embeddings_archive WHERE doc_id='d1'`, &mirrorRows)
	scan1(`SELECT count(*) FROM knowledge_chunks WHERE doc_id='d1'`, &currentChunks)
	if versions != 2 { // seed v1 + tombstone v2 双行幸存
		t.Fatalf("ledger cascaded away: %d rows, want 2", versions)
	}
	if archiveRows != 2 || mirrorRows != 1 || currentChunks != 0 {
		t.Fatalf("tombstone archive set wrong: archive=%d mirror=%d current=%d", archiveRows, mirrorRows, currentChunks)
	}
	var kind string
	var tombAt time.Time
	var stats string
	if err := db.QueryRowContext(ctx,
		`SELECT change_kind, created_at, chunk_stats::text FROM document_versions WHERE doc_id='d1' AND version_no=2`).
		Scan(&kind, &tombAt, &stats); err != nil {
		t.Fatalf("tombstone row: %v", err)
	}
	if kind != "tombstone" || !strings.Contains(stats, `"archived": 2`) {
		t.Fatalf("tombstone row wrong: kind=%q stats=%q", kind, stats)
	}
	// 视图闭区间钉：tombstone 行未显式造 created_at（now()），闭端=该行时刻；
	// 闭端之前存活得见、之后查无（回放语义由 as-of 谓词承载，此处钉区间闭端本身）。
	var validTo time.Time
	var archived bool
	if err := db.QueryRowContext(ctx, `
		SELECT valid_to, archived FROM knowledge_chunk_incarnations
		WHERE chunk_id='d1-v1-ch-0' AND archived`).Scan(&validTo, &archived); err != nil {
		t.Fatalf("closed interval: %v", err)
	}
	if !archived || !validTo.Equal(tombAt) {
		t.Fatalf("tombstone interval endpoint: valid_to=%v want tombstone_at=%v", validTo, tombAt)
	}
	// 零泄漏钉：tombstone 之后任取时点，view 不产出该 doc 的存活化身。
	var alive int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM knowledge_chunk_incarnations
		WHERE doc_id='d1' AND valid_from <= now() AND now() < valid_to`).Scan(&alive); err != nil {
		t.Fatalf("post-tombstone alive: %v", err)
	}
	if alive != 0 {
		t.Fatalf("current-leak after tombstone: %d alive incarnations", alive)
	}
}

// ⑤ 按 chunk_id 精确回落：多版死亡取 MAX(revision_no)，无档 id 静默缺席。
func TestG32T3ArchiveLatestContents(t *testing.T) {
	ctx, db := g32T3Base(t)
	seedT3Doc(t, ctx, db)
	mustExecG32T3(t, ctx, db, `INSERT INTO chunk_archive (doc_id, chunk_id, revision_no, collection_id, content) VALUES
		('d1', 'd1-v9-ch-7', 2, 'c1', 'old body'),
		('d1', 'd1-v9-ch-7', 5, 'c1', 'newest body')`)
	got, err := queryChunkArchiveLatestContents(ctx, db, []string{"d1-v9-ch-7", "never-existed"})
	if err != nil {
		t.Fatalf("latest contents: %v", err)
	}
	if got["d1-v9-ch-7"] != "newest body" {
		t.Fatalf("latest revision loses: %q", got["d1-v9-ch-7"])
	}
	if _, ok := got["never-existed"]; ok {
		t.Fatalf("phantom id present: %v", got)
	}
	if len(got) != 1 {
		t.Fatalf("result size: got %d want 1", len(got))
	}
}

// ⑥ 配对投影↔纯函数跨层契约钉：loadChunksForPairingTx 的字段映射逐列对齐，
// 行序 = (chunk_index, id)，经 pairingOldRows 投出直接可喂 T2 PairChunksForDiff；
// 决策 5③ 全列投影钉：cited_count/metadata 随行（DIS-2——compare-and-skip 的
// 实变判定与 DiffStats 的供血列，缺列=契约断裂）。
func TestG32T3PairingLoaderEquivalence(t *testing.T) {
	ctx, db := g32T3Base(t)
	seedT3Doc(t, ctx, db)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, chunk_index, chunk_role, parent_chunk_id, cited_count, metadata) VALUES
		('d1-v1-p-0', 'd1', 'c1', 'PARENT', -1, 'parent', '', 0, '{}'),
		('d1-v1-ch-9', 'd1', 'c1', 'tail', 9, 'child', 'd1-v1-p-0', 3, '{"k":"v"}')`)

	tx, _ := db.BeginTx(ctx, nil)
	defer func() { _ = tx.Rollback() }()
	rows, err := loadChunksForPairingTx(ctx, tx, "d1")
	if err != nil {
		t.Fatalf("load pairing: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("row count: got %d want 4", len(rows))
	}
	// 排序钉：parent(-1) 最先，tail(9) 最后。
	if rows[0].ID != "d1-v1-p-0" || rows[0].Role != "parent" {
		t.Fatalf("order/role: first=%+v", rows[0])
	}
	if rows[3].ID != "d1-v1-ch-9" || rows[3].ParentChunkID != "d1-v1-p-0" {
		t.Fatalf("parent link drift: last=%+v", rows[3])
	}
	// 全列投影钉：末行的 cited_count/metadata 必须随行现载（compare-and-skip
	// 与 DiffStats 的供血列）。jsonb::text 读出原文。
	if rows[3].CitedCount != 3 || !strings.Contains(string(rows[3].Metadata), `"k": "v"`) {
		t.Fatalf("decision-5③ projection missing columns: %+v", rows[3])
	}
	// 经投影直接喂纯函数（cross-layer 编译级契约）：全同 → 全保留。
	newSet := []bizknowledge.PairingNewChunk{
		{Content: "PARENT", Role: "parent", ParentIdx: -1},
		{Content: "alpha", Role: "child", ParentIdx: -1},
		{Content: "beta", Role: "child", ParentIdx: -1},
		{Content: "tail", Role: "child", ParentIdx: 0},
	}
	plan := bizknowledge.PairChunksForDiff(pairingOldRows(rows), newSet)
	if len(plan.Kept) != 4 || len(plan.Added) != 0 || len(plan.Deleted) != 0 {
		t.Fatalf("loader feeds pairing badly: %+v", plan)
	}
}

// ⑦ listAsOf 时点截取：边界等值归新区间 + 两腿并存 + current 零泄漏
// （at=now 与裸表查询字节语义一致）。
func TestG32T3ListChunksAsOfBoundaryAndZeroLeak(t *testing.T) {
	ctx, db := g32T3Base(t)
	seedT3Doc(t, ctx, db)
	// v2：ch-0 死亡（归档 revision_no=2）、ch-1 保留、ch-2 新生。
	mustExecG32T3(t, ctx, db, `INSERT INTO document_versions (doc_id, version_no, change_kind, producer, created_at)
		VALUES ('d1', 2, 'edit', 'test', $1)`, t3T1)
	mustExecG32T3(t, ctx, db, `INSERT INTO chunk_archive (doc_id, chunk_id, revision_no, collection_id, content, chunk_index)
		VALUES ('d1', 'd1-v1-ch-0', 2, 'c1', 'alpha', 0)`)
	mustExecG32T3(t, ctx, db, `DELETE FROM knowledge_chunks WHERE id = 'd1-v1-ch-0'`)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, chunk_index) VALUES
		('d1-v2-ch-2', 'd1', 'c1', 'gamma', 2)`)

	// t3T0+ε（v1 世界）：ch-0/archived=false、ch-1 存活；ch-2 未出生。
	between, _ := time.Parse(time.RFC3339, "2026-10-01T12:00:00Z")
	items, err := listChunksAsOfCore(ctx, db, "c1", between, "")
	if err != nil {
		t.Fatalf("asof v1-world: %v", err)
	}
	ids := map[string]bool{}
	for _, it := range items {
		ids[it.ChunkID] = true
	}
	if len(items) != 2 || !ids["d1-v1-ch-0"] || !ids["d1-v1-ch-1"] {
		t.Fatalf("v1-world wrong: %v", ids)
	}

	// 恰界 t3T1（v2 出生时刻）：等值归新区间——ch-0 归档腿 closed(right-open)，
	// ch-1 跨越存活，ch-2 新生存活。
	atBoundary := mustTime(t, t3T1)
	items, err = listChunksAsOfCore(ctx, db, "c1", atBoundary, "")
	if err != nil {
		t.Fatalf("asof boundary: %v", err)
	}
	ids = map[string]bool{}
	for _, it := range items {
		ids[it.ChunkID] = true
	}
	if len(items) != 2 || ids["d1-v1-ch-0"] || !ids["d1-v1-ch-1"] || !ids["d1-v2-ch-2"] {
		t.Fatalf("boundary world wrong (等值必须归新区间): %v", ids)
	}

	// 零泄漏钉：at=now 的存活集 与 裸表 child 行集 逐一相等。
	nowItems, err := listChunksAsOfCore(ctx, db, "c1", time.Now().Add(time.Hour), "")
	if err != nil {
		t.Fatalf("asof now: %v", err)
	}
	bareRows, err := db.QueryContext(ctx,
		`SELECT id FROM knowledge_chunks WHERE collection_id='c1' AND chunk_role='child' ORDER BY id`)
	if err != nil {
		t.Fatalf("bare select: %v", err)
	}
	defer bareRows.Close()
	bare := map[string]bool{}
	for bareRows.Next() {
		var id string
		if err := bareRows.Scan(&id); err != nil {
			t.Fatalf("scan bare: %v", err)
		}
		bare[id] = true
	}
	if len(nowItems) != len(bare) {
		t.Fatalf("zero-leak size: asof=%d bare=%d", len(nowItems), len(bare))
	}
	for _, it := range nowItems {
		if !bare[it.ChunkID] || it.Archived {
			t.Fatalf("zero-leak violation: %s archived=%v not in bare set %v", it.ChunkID, it.Archived, bare)
		}
	}
}

// ⑧ ANN 双腿合并（决策12）：归档腿经 chunk_embeddings_archive 命中、
// 当前腿经 chunk_embeddings 命中，top-k 按距离排序，doc 收窄生效。
func TestG32T3ANNAsOfTwoLegs(t *testing.T) {
	ctx, db := g32T3Base(t)
	seedT3Doc(t, ctx, db)
	mustExecG32T3(t, ctx, db, `INSERT INTO document_versions (doc_id, version_no, change_kind, producer, created_at)
		VALUES ('d1', 2, 'edit', 'test', $1)`, t3T1)
	// ch-0 死（镜像档 [1,0,0]）、ch-2 新生（[0,1,0]）、ch-1 跨越（[0,0,1]）。
	mustExecG32T3(t, ctx, db, `INSERT INTO chunk_embeddings (chunk_id, dim, collection_id, doc_id, model, embedding) VALUES
		('d1-v1-ch-0', 3, 'c1', 'd1', 'm0', '[1,0,0]'::vector),
		('d1-v1-ch-1', 3, 'c1', 'd1', 'm0', '[0,0,1]'::vector)`)
	tx, _ := db.BeginTx(ctx, nil)
	if _, err := archiveChunksForVersionTx(ctx, tx, "d1", 2, []string{"d1-v1-ch-0"}); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := mirrorChunkEmbeddingsArchiveTx(ctx, tx, []string{"d1-v1-ch-0"}); err != nil {
		t.Fatalf("mirror: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_chunks WHERE id = 'd1-v1-ch-0'`); err != nil {
		t.Fatalf("kill: %v", err)
	}
	mustExec2(t, ctx, tx, `INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, chunk_index) VALUES
		('d1-v2-ch-2', 'd1', 'c1', 'gamma', 2)`)
	mustExec2(t, ctx, tx, `INSERT INTO chunk_embeddings (chunk_id, dim, collection_id, doc_id, model, embedding) VALUES
		('d1-v2-ch-2', 3, 'c1', 'd1', 'm0', '[0,1,0]'::vector)`)
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// v1 世界（两版本间隙）：双腿并集——ch-0 归档腿（已死化身）+ ch-1 当时存活
	// 腿（valid_from=v1seed, valid_to=inf）；ch-2 未出生。决策12「as-of 命中
	// 编辑前化身」核心钉 + 双腿合并钉：距离排序 ch-0 分=1 居首、ch-1 尾随。
	between, _ := time.Parse(time.RFC3339, "2026-10-01T12:00:00Z")
	hits, err := queryChunksAsOfANN(ctx, db, "c1", []float32{1, 0, 0}, between, 5, "")
	if err != nil {
		t.Fatalf("ann v1-world: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("v1-world hit count: got %d want 2: %+v", len(hits), hits)
	}
	if hits[0].ChunkID != "d1-v1-ch-0" || !hits[0].Archived || hits[0].Score < 0.99 {
		t.Fatalf("archive leg miss/rank wrong: %+v", hits[0])
	}
	if hits[1].ChunkID != "d1-v1-ch-1" || hits[1].Archived {
		t.Fatalf("historical live leg missing: %+v", hits[1])
	}
	// v2 世界（现下）：双腿并集 ch-0(archived,不到现下不得见) + ch-1 + ch-2；
	// top-2 按 [1,0,0] 距离，ch-0 归档腿与当前无涉——现下不应含 ch-0。
	now := mustTime(t, t3T2)
	hits, err = queryChunksAsOfANN(ctx, db, "c1", []float32{1, 0, 0}, now, 5, "")
	if err != nil {
		t.Fatalf("ann current: %v", err)
	}
	ids := map[string]bool{}
	for _, h := range hits {
		ids[h.ChunkID] = true
	}
	if ids["d1-v1-ch-0"] || !ids["d1-v1-ch-1"] || !ids["d1-v2-ch-2"] || len(hits) != 2 {
		t.Fatalf("current world wrong: %v", ids)
	}
	// doc 收窄：doc_id='d1' 与 空串 结果一致（doc 过滤链条能动即可证）。
	hits, err = queryChunksAsOfANN(ctx, db, "c1", []float32{0, 1, 0}, now, 1, "d1")
	if err != nil {
		t.Fatalf("ann doc-narrow: %v", err)
	}
	if len(hits) != 1 || hits[0].ChunkID != "d1-v2-ch-2" {
		t.Fatalf("doc-narrow/top-k wrong: %+v", hits)
	}
}

// ⑨ CON-12 镜像 latest-wins 钉：同 (chunk_id,dim) 二次镜档必须用【新化身】
// 向量覆盖旧档——杜绝「chunk_archive 新内容 × chunk_embeddings_archive 旧向量」
// 的串化身读腿分叉（旧二进制回滚窗口 id 复活二次死亡的防御面）。
func TestG32T3MirrorLatestWins(t *testing.T) {
	ctx, db := g32T3Base(t)
	seedT3Doc(t, ctx, db)
	mustExecG32T3(t, ctx, db, `INSERT INTO chunk_embeddings (chunk_id, dim, collection_id, doc_id, model, embedding) VALUES
		('d1-v1-ch-0', 3, 'c1', 'd1', 'm0', '[0.1,0.2,0.3]'::vector)`)

	tx, _ := db.BeginTx(ctx, nil)
	if n, err := mirrorChunkEmbeddingsArchiveTx(ctx, tx, []string{"d1-v1-ch-0"}); err != nil || n != 1 {
		t.Fatalf("mirror1: n=%d err=%v want 1", n, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit1: %v", err)
	}
	// 化身二次死亡前的形态：侧表同 id 换成新向量（复活窗口的形态等效）。
	mustExecG32T3(t, ctx, db, `UPDATE chunk_embeddings SET embedding='[0.9,0.8,0.7]'::vector, model='m1' WHERE chunk_id='d1-v1-ch-0'`)
	tx2, _ := db.BeginTx(ctx, nil)
	if n, err := mirrorChunkEmbeddingsArchiveTx(ctx, tx2, []string{"d1-v1-ch-0"}); err != nil || n != 1 {
		t.Fatalf("mirror2: n=%d err=%v want 1 (DO UPDATE)", n, err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("commit2: %v", err)
	}
	var emb, model string
	var total int
	if err := db.QueryRowContext(ctx,
		`SELECT embedding::text, model, (SELECT count(*) FROM chunk_embeddings_archive) FROM chunk_embeddings_archive WHERE chunk_id='d1-v1-ch-0'`).
		Scan(&emb, &model, &total); err != nil {
		t.Fatalf("readback: %v", err)
	}
	if total != 1 {
		t.Fatalf("dup archive rows: %d", total)
	}
	if !strings.HasPrefix(emb, "[0.9") || model != "m1" {
		t.Fatalf("latest-wins violated: emb=%q model=%q", emb, model)
	}
}

// ⑩ DIS-5 零账本 mint 钉：无任何版本行的 doc 直接 tombstone 时，
// 内核必须先以 doc.created_at mint seed v1 再落 tombstone v2——归档化身区间
// =[doc 出生点, tombstone 时刻) 真实跨度，而非坍缩 [t,t) 任何时点查无。
func TestG32T3TombstoneZeroLedgerMintsSeed(t *testing.T) {
	ctx, db := g32T3Base(t)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'c1', 'm0')`)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_documents (id, collection_id, source, content_text, created_at)
		VALUES ('d1', 'c1', 's1', 'old body', $1)`, t3T0)
	mustExecG32T3(t, ctx, db, `INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, chunk_index) VALUES
		('d1-v1-ch-0', 'd1', 'c1', 'alpha', 0)`)

	tx, _ := db.BeginTx(ctx, nil)
	ver, err := tombstoneDocumentTx(ctx, tx, "d1", "test-tombstone")
	if err != nil || ver != 2 {
		t.Fatalf("tombstone with mint: ver=%d err=%v want 2", ver, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_documents WHERE id = 'd1'`); err != nil {
		t.Fatalf("delete doc: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var seedKind, seedProducer string
	var seedAt, validFrom, validTo time.Time
	if err := db.QueryRowContext(ctx,
		`SELECT change_kind, producer, created_at FROM document_versions WHERE doc_id='d1' AND version_no=1`).
		Scan(&seedKind, &seedProducer, &seedAt); err != nil {
		t.Fatalf("minted seed row: %v", err)
	}
	if seedKind != "seed" || seedProducer != "g32-tombstone-mint" || !seedAt.Equal(mustTime(t, t3T0)) {
		t.Fatalf("mint row wrong: kind=%q producer=%q at=%v want t3T0", seedKind, seedProducer, seedAt)
	}
	// 归档化身区间真实跨度钉：valid_from=doc 出生点（而非 tombstone 时刻）。
	if err := db.QueryRowContext(ctx, `
		SELECT valid_from, valid_to FROM knowledge_chunk_incarnations
		WHERE chunk_id='d1-v1-ch-0' AND archived`).Scan(&validFrom, &validTo); err != nil {
		t.Fatalf("archived interval: %v", err)
	}
	if !validFrom.Equal(mustTime(t, t3T0)) || !validTo.After(validFrom) {
		t.Fatalf("collapsed interval: [%v, %v)", validFrom, validTo)
	}
	// as-of 双时点钉：v1 世界时刻查得见，tombstone 之后查无。
	between, _ := time.Parse(time.RFC3339, "2026-10-01T12:00:00Z")
	items, err := listChunksAsOfCore(ctx, db, "c1", between, "")
	if err != nil || len(items) != 1 || items[0].ChunkID != "d1-v1-ch-0" {
		t.Fatalf("pre-tombstone asof miss: items=%+v err=%v", items, err)
	}
	items, err = listChunksAsOfCore(ctx, db, "c1", time.Now().Add(time.Hour), "")
	if err != nil || len(items) != 0 {
		t.Fatalf("post-tombstone leak: items=%+v err=%v", items, err)
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02 15:04:05Z", s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

func mustExec2(t *testing.T, ctx context.Context, tx *sql.Tx, q string, args ...any) {
	t.Helper()
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		t.Fatalf("tx exec %.120q: %v", q, err)
	}
}
