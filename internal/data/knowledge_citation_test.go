package data

import (
	"context"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── 29-token P2-2: knowledge chunk citation tracking ─────────────────────
// PG 集成测试：reader 走 steps_v2(notice) ⋈ steps_v2(reply) + knowledge_chunks
// 全文批量解析；recorder 走 knowledge_chunk_citations 去重账本 + cited_count
// 首次命中 +1。幂等（重叠窗口重扫不重复计数）是核心契约。

func setupKnowledgeCitationData(t *testing.T) *Data {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	// 同 setupDimReconcileRepo：pgvector 扩展在 public schema，钉单连接补 search_path。
	db.SetMaxOpenConns(1)
	var schema string
	if err := db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO ` + schema + `, public`); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	ctx := context.Background()
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	return &Data{
		entClient:  client,
		readClient: client,
		rawDB:      db,
		readDB:     db,
		pg:         db,
		pgRead:     db,
		rw:         NewReadWriteClient(client, client),
		rwDB:       NewReadWriteDB(db, db),
		lg:         loggateway.NewNoop(),
		dialect:    DialectPostgres,
		txTimeout:  30 * time.Second,
	}
}

func insertCitationTestChunk(t *testing.T, d *Data, id, content string) {
	t.Helper()
	// collection → document → chunk 外键链（CASCADE 引用需真实存在）。
	if _, err := d.rawDB.Exec(`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'vault', 'm') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert collection: %v", err)
	}
	if _, err := d.rawDB.Exec(`INSERT INTO knowledge_documents (id, collection_id, source) VALUES ('d1', 'c1', 'a.md') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert document: %v", err)
	}
	if _, err := d.rawDB.Exec(`INSERT INTO knowledge_chunks (id, doc_id, collection_id, content) VALUES ($1, 'd1', 'c1', $2)`, id, content); err != nil {
		t.Fatalf("insert chunk %s: %v", id, err)
	}
}

func insertCitationTestStep(t *testing.T, d *Data, id, turnID, kind, noticeType, content string, isFinal bool, seq int64, startedAt time.Time) {
	t.Helper()
	if _, err := d.rawDB.Exec(`
		INSERT INTO steps_v2 (id, turn_id, task_id, session_id, spirit_session_id, kind, notice_type, content, is_final, seq, status, started_at)
		VALUES ($1, $2, 'task', 'sess', 'spirit', $3, $4, $5, $6, $7, 'completed', $8)`,
		id, turnID, kind, noticeType, content, isFinal, seq, startedAt); err != nil {
		t.Fatalf("insert step %s: %v", id, err)
	}
}

func citedCountOf(t *testing.T, d *Data, chunkID string) int {
	t.Helper()
	var n int
	if err := d.rawDB.QueryRow(`SELECT cited_count FROM knowledge_chunks WHERE id = $1`, chunkID).Scan(&n); err != nil {
		t.Fatalf("read cited_count %s: %v", chunkID, err)
	}
	return n
}

func TestKnowledgeCitationRecorder_DedupLedger(t *testing.T) {
	d := setupKnowledgeCitationData(t)
	ctx := context.Background()
	recorder := NewKnowledgeChunkCitationRecorder(d)
	if recorder == nil {
		t.Fatal("recorder is nil")
	}
	insertCitationTestChunk(t, d, "k1", "alpha content")

	// 首记：cited_count 0 → 1。
	if err := recorder.RecordChunkCitations(ctx, []bizknowledge.ChunkCitation{{ChunkID: "k1", TurnID: "t1"}}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if got := citedCountOf(t, d, "k1"); got != 1 {
		t.Fatalf("cited_count after first record = %d, want 1", got)
	}

	// 重扫同窗口（同 chunk 同 turn）：去重账本拦截，不重复计数。
	if err := recorder.RecordChunkCitations(ctx, []bizknowledge.ChunkCitation{{ChunkID: "k1", TurnID: "t1"}}); err != nil {
		t.Fatalf("re-record same pair: %v", err)
	}
	if got := citedCountOf(t, d, "k1"); got != 1 {
		t.Fatalf("cited_count after dup record = %d, want 1 (idempotent)", got)
	}

	// 新 turn：是新的引用，计数 +1。
	if err := recorder.RecordChunkCitations(ctx, []bizknowledge.ChunkCitation{{ChunkID: "k1", TurnID: "t2"}}); err != nil {
		t.Fatalf("record second turn: %v", err)
	}
	if got := citedCountOf(t, d, "k1"); got != 2 {
		t.Fatalf("cited_count after second turn = %d, want 2", got)
	}

	// 空字段静默跳过；nil 安全。
	if err := recorder.RecordChunkCitations(ctx, []bizknowledge.ChunkCitation{{ChunkID: "", TurnID: "t3"}, {ChunkID: "k1", TurnID: ""}}); err != nil {
		t.Fatalf("record blank fields: %v", err)
	}
	if got := citedCountOf(t, d, "k1"); got != 2 {
		t.Fatalf("cited_count after blank record = %d, want 2", got)
	}
}

func TestKnowledgeCitationTraceReader_ListCandidates(t *testing.T) {
	d := setupKnowledgeCitationData(t)
	ctx := context.Background()
	reader := NewKnowledgeCitationTraceReader(d)
	if reader == nil {
		t.Fatal("reader is nil")
	}
	insertCitationTestChunk(t, d, "k1", "向量检索的第一段内容")
	insertCitationTestChunk(t, d, "k2", "向量检索的第二段内容")
	now := time.Now()

	// 正常 notice + final reply。
	insertCitationTestStep(t, d, "n1", "turn-1", "notice", "knowledge_recalled",
		`{"chunks":[{"chunk_id":"k1","score":0.9},{"chunk_id":"k2","score":0.8},{"chunk_id":"k1"}]}`, false, 1, now)
	insertCitationTestStep(t, d, "r1", "turn-1", "reply", "", "这是引用了资料的回复", true, 2, now)

	// 无回复的 notice：跳过。
	insertCitationTestStep(t, d, "n2", "turn-2", "notice", "knowledge_recalled",
		`{"chunks":[{"chunk_id":"k1"}]}`, false, 1, now)

	// 坏 JSON notice：跳过。
	insertCitationTestStep(t, d, "n3", "turn-3", "notice", "knowledge_recalled",
		`not-json`, false, 1, now)
	insertCitationTestStep(t, d, "r3", "turn-3", "reply", "", "回复", true, 2, now)

	// 窗口外旧 notice：不可见。
	insertCitationTestStep(t, d, "n4", "turn-4", "notice", "knowledge_recalled",
		`{"chunks":[{"chunk_id":"k1"}]}`, false, 1, now.Add(-2*time.Hour))
	insertCitationTestStep(t, d, "r4", "turn-4", "reply", "", "旧回复", true, 2, now.Add(-2*time.Hour))

	// 其它 notice_type：不可见。
	insertCitationTestStep(t, d, "n5", "turn-5", "notice", "memory_recalled",
		`{"hits":[]}`, false, 1, now)
	insertCitationTestStep(t, d, "r5", "turn-5", "reply", "", "回复", true, 2, now)

	candidates, err := reader.ListKnowledgeCitationCandidates(ctx, now.Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want 1 (only turn-1 qualifies)", len(candidates))
	}
	cand := candidates[0]
	if cand.TurnID != "turn-1" {
		t.Fatalf("candidate turn = %q, want turn-1", cand.TurnID)
	}
	if cand.ReplyText != "这是引用了资料的回复" {
		t.Fatalf("reply = %q", cand.ReplyText)
	}
	// chunk 去重（payload 中 k1 出现两次）+ 全文解析。
	if len(cand.Chunks) != 2 {
		t.Fatalf("chunks = %d, want 2 (deduped)", len(cand.Chunks))
	}
	byID := map[string]string{}
	for _, ch := range cand.Chunks {
		byID[ch.ChunkID] = ch.Content
	}
	if byID["k1"] != "向量检索的第一段内容" || byID["k2"] != "向量检索的第二段内容" {
		t.Fatalf("chunk contents not resolved: %v", byID)
	}
}

// 引用不存在的 chunk（已被删除）：从候选中剔除；全部剔除则候选整体跳过。
func TestKnowledgeCitationTraceReader_DropsMissingChunks(t *testing.T) {
	d := setupKnowledgeCitationData(t)
	ctx := context.Background()
	reader := NewKnowledgeCitationTraceReader(d)
	insertCitationTestChunk(t, d, "k1", "仅存的一段")
	now := time.Now()

	insertCitationTestStep(t, d, "n1", "turn-1", "notice", "knowledge_recalled",
		`{"chunks":[{"chunk_id":"k1"},{"chunk_id":"gone"}]}`, false, 1, now)
	insertCitationTestStep(t, d, "r1", "turn-1", "reply", "", "回复", true, 2, now)
	// turn-2 的 chunk 全部不存在 → 候选跳过。
	insertCitationTestStep(t, d, "n2", "turn-2", "notice", "knowledge_recalled",
		`{"chunks":[{"chunk_id":"gone"}]}`, false, 1, now)
	insertCitationTestStep(t, d, "r2", "turn-2", "reply", "", "回复", true, 2, now)

	candidates, err := reader.ListKnowledgeCitationCandidates(ctx, now.Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(candidates))
	}
	if len(candidates[0].Chunks) != 1 || candidates[0].Chunks[0].ChunkID != "k1" {
		t.Fatalf("missing chunk not dropped: %+v", candidates[0].Chunks)
	}
	if candidates[0].TurnID != "turn-1" {
		t.Fatalf("turn = %q", candidates[0].TurnID)
	}
}
