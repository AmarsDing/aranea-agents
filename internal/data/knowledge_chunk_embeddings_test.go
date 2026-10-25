package data

import (
	"context"
	"sort"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"

	"github.com/lib/pq"
)

// ── G-39 多维度向量共存（ADR-KN-20261021-G，PG 矩阵 ①–⑦）────────────────────
// 侧表 chunk_embeddings 按 (chunk_id, dim) 分列存向量；dense 召回按查询向量
// dim 字面桶定 JOIN——跨维度比较（22P02）结构性不可能；换模型不 ALTER 主表、
// 不作废旧 dim 向量（reconcileEmbeddingDim 08-10 事故 wipe 路径已退役）。
// 覆盖：①主表列脱离写路径+侧表落账 ②多代 dim 共存/桶定 ③批内混维拒绝
// ④pending-reembed 按 dim 口径 ⑤删文档级联 ⑥移库侧表随迁 ⑦Ensure 幂等回填。

func seedG39Collection(t *testing.T, repo *knowledgeRepo, colID string) {
	t.Helper()
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: colID, Name: colID, EmbeddingModel: "m", Dim: 3,
	}); err != nil {
		t.Fatal(err)
	}
}
func seedG39Doc(t *testing.T, repo *knowledgeRepo, colID, docID string) {
	t.Helper()
	if _, err := repo.CreateDocument(context.Background(), biz.KnowledgeDocument{
		ID: docID, CollectionID: colID, Source: docID + ".md",
		Status: "indexing", ContentText: "doc " + docID,
	}); err != nil {
		t.Fatal(err)
	}
}

// ① 新写入只落侧表：主表 legacy embedding 列恒 NULL；侧表 (chunk_id, dim)
// 行带 dim/model/doc_id；dense 召回经 JOIN 命中。
func TestChunkEmbeddingSideTable_InsertWritesSideOnly(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	seedG39Collection(t, repo, "g39-side")
	seedG39Doc(t, repo, "g39-side", "d1")

	chunks := []biz.KnowledgeChunk{
		{ID: "c1", DocID: "d1", CollectionID: "g39-side", Content: "apple 订单流", ChunkIndex: 0, Embedding: []float32{1, 0, 0}},
		{ID: "c2", DocID: "d1", CollectionID: "g39-side", Content: "apple 仓储区", ChunkIndex: 1, Embedding: []float32{0, 1, 0}},
		{ID: "c3-parent", DocID: "d1", CollectionID: "g39-side", Content: "apple 父块", ChunkIndex: 2, IsParent: true},
	}
	if err := repo.InsertChunks(ctx, chunks); err != nil {
		t.Fatal(err)
	}

	var mainVecCount int
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_chunks WHERE doc_id='d1' AND embedding IS NOT NULL`).Scan(&mainVecCount); err != nil {
		t.Fatal(err)
	}
	if mainVecCount != 0 {
		t.Fatalf("legacy main column must stay NULL after G-39, got %d non-null rows", mainVecCount)
	}

	rows, err := repo.data.Postgres().QueryContext(ctx,
		`SELECT chunk_id, dim, doc_id, model FROM chunk_embeddings WHERE doc_id='d1' ORDER BY chunk_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct {
		chunkID      string
		dim          int
		docID, model string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.chunkID, &r.dim, &r.docID, &r.model); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("side rows = %d, want 2 (parent must not carry vector): %+v", len(got), got)
	}
	for i, want := range []row{
		{"c1", 3, "d1", "m"}, {"c2", 3, "d1", "m"},
	} {
		if got[i] != want {
			t.Fatalf("side row %d = %+v, want %+v", i, got[i], want)
		}
	}

	hits, err := repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "g39-side", TopK: 5}, []float32{1, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, hits, "c1", "c2")
}

// ② 多代 dim 共存：dim-3 时代摄取的 docA 与 dim-4 时代摄取的 docB 同栈并存；
// dense 查询按查询向量 dim 桶定——桶外文档不进比较（无 22P02、无串桶）。
// T4 代际命名（决策 5⑧）：摄取块一律改名 <docID>-v<ver>-ch-<idx>，断言按新 ID。
func TestChunkEmbeddingSideTable_MultiDimCoexistence(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	seedG39Collection(t, repo, "g39-multi")
	seedG39Doc(t, repo, "g39-multi", "docA")
	seedG39Doc(t, repo, "g39-multi", "docB")

	commit := func(docID, chunkID string, vec []float32) {
		t.Helper()
		if _, err := repo.CommitIndexedDocument(ctx, "g39-multi", docID, []biz.KnowledgeChunk{{
			ID: chunkID, DocID: docID, CollectionID: "g39-multi",
			Content: "apple " + chunkID, ChunkIndex: 0, Embedding: vec,
		}}, 1, nil); err != nil {
			t.Fatal(err)
		}
	}
	commit("docA", "kA", []float32{1, 0, 0})
	commit("docB", "kB", []float32{1, 0, 0, 0})

	assert := func(vec []float32, want ...string) {
		t.Helper()
		hits, err := repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "g39-multi", TopK: 5}, vec)
		if err != nil {
			t.Fatalf("dense dim=%d: %v", len(vec), err)
		}
		assertChunkIDs(t, hits, want...)
	}
	assert([]float32{1, 0, 0}, "docA-v1-ch-0")    // dim-3 查询只中 dim-3 桶
	assert([]float32{1, 0, 0, 0}, "docB-v1-ch-0") // dim-4 查询只中 dim-4 桶（共存无 22P02）
	assert([]float32{1, 0, 0, 0, 0})              // dim-5 查询空结果（无桶、无错误）

	// 同内容重摄取换维（G-39 模型升级走 ⑪ 缺 dim 条件补齐）：kept 身份不动，
	// dim-4 侧行落账，dim-3 旧维保留多代共存；delete→cascade 仅作用于孤儿。
	// 生产重嵌流先把文档状态拨回 indexing 再提交（CommitIndexedDocument CAS
	// 守卫 status='indexing'）——测试镜像该时序。
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents SET status='indexing' WHERE id='docA'`); err != nil {
		t.Fatal(err)
	}
	commit("docA", "kA", []float32{0, 1, 0, 0})
	var dims []int
	rows, err := repo.data.Postgres().QueryContext(ctx,
		`SELECT dim FROM chunk_embeddings WHERE chunk_id='docA-v1-ch-0' ORDER BY dim`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var d int
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		dims = append(dims, d)
	}
	if len(dims) != 2 || dims[0] != 3 || dims[1] != 4 {
		t.Fatalf("after dim-fill side dims for docA-v1-ch-0 = %v, want [3 4] (kept 缺 dim 补齐、旧维保留)", dims)
	}
	// 补齐后 dim-4 查询可命中 docA（同一 chunk 两代同栈），dim-3 查询仍在。
	assert([]float32{0, 1, 0, 0}, "docA-v1-ch-0", "docB-v1-ch-0")
}

// ③ 批内混维拒绝（同一摄取批不可能产两种维度）；统一批任意 dim 合法
// （多代共存由侧表承载）。父块/空向量不参与判定。
func TestChunkEmbeddingSideTable_MixedDimBatchRejected(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	seedG39Collection(t, repo, "g39-mixed")
	seedG39Doc(t, repo, "g39-mixed", "d1")

	err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{
		{ID: "m1", DocID: "d1", CollectionID: "g39-mixed", Content: "x", ChunkIndex: 0, Embedding: []float32{1, 2}},
		{ID: "m2", DocID: "d1", CollectionID: "g39-mixed", Content: "y", ChunkIndex: 1, Embedding: []float32{1, 2, 3}},
	})
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("mixed-dim batch must be rejected, got %v", err)
	}
	var n int
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_chunks WHERE doc_id='d1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rejected batch must not partially commit, got %d chunks", n)
	}

	// 统一批 + 空向量父块：合法，dim=2 侧行落账（非台账 dim——台账仅账本）。
	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{
		{ID: "u0", DocID: "d1", CollectionID: "g39-mixed", Content: "p", ChunkIndex: 0, IsParent: true},
		{ID: "u1", DocID: "d1", CollectionID: "g39-mixed", Content: "x", ChunkIndex: 1, Embedding: []float32{1, 2}},
		{ID: "u2", DocID: "d1", CollectionID: "g39-mixed", Content: "y", ChunkIndex: 2, Embedding: []float32{3, 4}},
	}); err != nil {
		t.Fatalf("uniform batch must pass: %v", err)
	}
	var d2 int
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chunk_embeddings WHERE dim=2 AND doc_id='d1'`).Scan(&d2); err != nil {
		t.Fatal(err)
	}
	if d2 != 2 {
		t.Fatalf("uniform dim-2 side rows = %d, want 2", d2)
	}
}

// ③b 带外构造防线（G-39 对抗评审 F2-B4/F2-B5）：父块带向量不落侧表（与
// 「父块行不落侧表」契约一致，防脏 dim 污染 per-dim 索引注册）；批内混
// 集合时侧行 model 各取各自集合台账（不再以 chunks[0] 单点溯源）。
func TestChunkEmbeddingSideTable_ParentGateAndPerCollectionModel(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	seedG39Collection(t, repo, "g39-gateA")
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{
		ID: "g39-gateB", Name: "g39-gateB", EmbeddingModel: "other-model", Dim: 2,
	}); err != nil {
		t.Fatal(err)
	}
	seedG39Doc(t, repo, "g39-gateA", "dA")
	seedG39Doc(t, repo, "g39-gateB", "dB")

	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{
		// 父块带向量（带外构造）：主表行落、侧表行禁落。
		{ID: "pg0", DocID: "dA", CollectionID: "g39-gateA", Content: "p", ChunkIndex: 0, IsParent: true, Embedding: []float32{9, 9, 9}},
		{ID: "pgA", DocID: "dA", CollectionID: "g39-gateA", Content: "a", ChunkIndex: 1, Embedding: []float32{1, 0, 0}},
		{ID: "pgB", DocID: "dB", CollectionID: "g39-gateB", Content: "b", ChunkIndex: 0, Embedding: []float32{0, 1, 0}},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.data.Postgres().QueryContext(ctx,
		`SELECT chunk_id, model FROM chunk_embeddings ORDER BY chunk_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var id, model string
		if err := rows.Scan(&id, &model); err != nil {
			t.Fatal(err)
		}
		got[id] = model
	}
	if _, ok := got["pg0"]; ok {
		t.Fatalf("parent chunk with vector must NOT land a side row: %+v", got)
	}
	if got["pgA"] != "m" || got["pgB"] != "other-model" {
		t.Fatalf("side models must trace each chunk's own collection ledger: %+v", got)
	}
}

// ④ pending-reembed 按当前 dim 判定：欠 dim-4 向量的文档在 dim=4 口径 pending、
// dim=3 口径健康；词法库（embedding_model=空串）与 dim<=0 口径永不入缺维腿。
func TestChunkEmbeddingSideTable_PendingReembedByDim(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	seedG39Collection(t, repo, "g39-pending")
	seedG39Doc(t, repo, "g39-pending", "dp")
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents SET status='indexed' WHERE id='dp'`); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
		ID: "kp", DocID: "dp", CollectionID: "g39-pending", Content: "apple",
		ChunkIndex: 0, Embedding: []float32{1, 0, 0},
	}}); err != nil {
		t.Fatal(err)
	}

	pendingIDs := func(dim int) []string {
		t.Helper()
		docs, err := repo.ListDocumentsPendingReembed(ctx, "g39-pending", dim)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(docs))
		for _, d := range docs {
			ids = append(ids, d.ID)
		}
		sort.Strings(ids)
		return ids
	}
	if ids := pendingIDs(3); len(ids) != 0 {
		t.Fatalf("dim=3 must be complete: %+v", ids)
	}
	if ids := pendingIDs(4); len(ids) != 1 || ids[0] != "dp" {
		t.Fatalf("dim=4 must mark doc pending (side dim-4 rows missing): %+v", ids)
	}
	if ids := pendingIDs(0); len(ids) != 0 {
		t.Fatalf("dim<=0 must skip missing-dim leg: %+v", ids)
	}

	// 词法库恒健康（embedding_model='' 闸），无论 dim。
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "g39-lex", Name: "g39-lex"}); err != nil {
		t.Fatal(err)
	}
	seedG39Doc(t, repo, "g39-lex", "dl")
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents SET status='indexed' WHERE id='dl'`); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
		ID: "kl", DocID: "dl", CollectionID: "g39-lex", Content: "apple lexical", ChunkIndex: 0,
	}}); err != nil {
		t.Fatal(err)
	}
	docs, err := repo.ListDocumentsPendingReembed(ctx, "g39-lex", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("lexical collection must never enter missing-dim pending: %+v", docs)
	}
}

// ⑤ 删文档级联侧表行；⑥ 移库侧表 collection_id 随迁。
func TestChunkEmbeddingSideTable_DeleteCascadeAndMove(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	seedG39Collection(t, repo, "g39-src")
	seedG39Collection(t, repo, "g39-dst")
	seedG39Doc(t, repo, "g39-src", "dm")
	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
		ID: "km", DocID: "dm", CollectionID: "g39-src", Content: "apple move",
		ChunkIndex: 0, Embedding: []float32{1, 0, 0},
	}}); err != nil {
		t.Fatal(err)
	}

	count := func(col string) int {
		t.Helper()
		var n int
		if err := repo.data.Postgres().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM chunk_embeddings WHERE collection_id=$1`, col).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("g39-src") != 1 {
		t.Fatalf("seed side row missing")
	}
	// 移库：侧表 collection_id 与主表随迁。
	if _, err := repo.MoveDocument(ctx, "dm", "g39-dst"); err != nil {
		t.Fatal(err)
	}
	if count("g39-src") != 0 || count("g39-dst") != 1 {
		t.Fatalf("move must relocate side rows: src=%d dst=%d", count("g39-src"), count("g39-dst"))
	}
	// 删 chunks：FK 级联回收侧行。
	if err := repo.DeleteChunksByDocument(ctx, "dm"); err != nil {
		t.Fatal(err)
	}
	if count("g39-dst") != 0 {
		t.Fatalf("delete must cascade side rows, got %d", count("g39-dst"))
	}
}

// ⑦ Ensure 幂等 + 存量回填：legacy 主表向量行（历史 typmod 形态直插）经
// Ensure 双行进侧表（dim 取行级 vector_dims 真值，model 取集合台账）；
// 重跑零增量（NOT EXISTS + ON CONFLICT 幂等）；卡片档 dim 同步回填 +
// 裸型化（重复 ALTER 无害）；迁移 .sql 在 Ensure 后执行全量空转。
func TestChunkEmbeddings_EnsureIdempotentBackfill(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	seedG39Collection(t, repo, "g39-legacy")
	seedG39Doc(t, repo, "g39-legacy", "dleg")

	// 历史形态：向量只落主表（侧表无行）。typmod 列接受字面 vector。
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, embedding, chunk_index)
		 VALUES ('kleg', 'dleg', 'g39-legacy', 'apple legacy', $1::vector, 0)`, "[1,0,0]"); err != nil {
		t.Fatal(err)
	}
	// 历史形态卡片：dim=0 默认 + 向量。补宿主实体/文档行让读口径可命中。
	var entID int64
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`INSERT INTO knowledge_entities (collection_id, name, entity_type, name_norm)
		 VALUES ('g39-legacy', 'E', 'concept', 'e') RETURNING id`).Scan(&entID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`INSERT INTO knowledge_doc_entities (collection_id, doc_id, entity_id, mentions)
		 VALUES ('g39-legacy', 'dleg', $1, 1)`, entID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`INSERT INTO knowledge_graph_embeddings (kind, ref_id, collection_id, text, source_hash, embedding)
		 VALUES ('entity', $1, 'g39-legacy', 'E 词条', 'h', $2::vector)`, entID, "[1,0,0]"); err != nil {
		t.Fatal(err)
	}

	// 重跑 Ensure（首次由夹具跑过）→ 存量回填 + 卡片 dim 回填；再跑一次零增量。
	for i := 0; i < 2; i++ {
		if err := EnsureKnowledgeSchema(ctx, repo.data.Postgres(), 3); err != nil {
			t.Fatalf("ensure re-run %d: %v", i, err)
		}
	}
	var sideDim int
	var sideModel string
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`SELECT dim, model FROM chunk_embeddings WHERE chunk_id='kleg'`).Scan(&sideDim, &sideModel); err != nil {
		t.Fatalf("legacy row must be backfilled into side table: %v", err)
	}
	if sideDim != 3 || sideModel != "m" {
		t.Fatalf("backfilled side row = (dim=%d, model=%q), want (3, \"m\")", sideDim, sideModel)
	}
	var sideN int
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chunk_embeddings WHERE chunk_id='kleg'`).Scan(&sideN); err != nil {
		t.Fatal(err)
	}
	if sideN != 1 {
		t.Fatalf("idempotent backfill must keep exactly 1 row, got %d", sideN)
	}
	var cardDim int
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`SELECT dim FROM knowledge_graph_embeddings WHERE kind='entity' AND ref_id=$1`, entID).Scan(&cardDim); err != nil {
		t.Fatal(err)
	}
	if cardDim != 3 {
		t.Fatalf("card dim backfill = %d, want 3", cardDim)
	}

	// 迁移 .sql 在 Ensure 后执行必须全量空转（双轨收敛证明）；裸 ADD COLUMN
	// 的 duplicate column（42701）属 DB-N6 白名单。
	sqlBytes, err := migrationSQLFS.ReadFile("sql/migrations/20261290_chunk_embeddings.sql")
	if err != nil {
		t.Fatalf("read migration file: %v", err)
	}
	for _, stmt := range splitDDLStatements(strings.TrimSpace(string(sqlBytes))) {
		if _, err := repo.data.Postgres().ExecContext(ctx, stmt); err != nil {
			if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "42701" {
				continue
			}
			t.Fatalf("migration re-apply: %v\nstmt: %s", err, stmt)
		}
	}

	// 卡片读口径：dim=3 查询命中回填卡；dim=4 查询空结果（共存桶定）。
	repo.data.rwDB = NewReadWriteDB(repo.data.Postgres(), repo.data.Postgres())
	hits, err := repo.SearchGraphCards(ctx, "g39-legacy", bizknowledge.GraphCardKindEntity, []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("card hit at dim 3 = %d, want 1: %+v", len(hits), hits)
	}
	hits, err = repo.SearchGraphCards(ctx, "g39-legacy", bizknowledge.GraphCardKindEntity, []float32{1, 0, 0, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("dim-4 query must hit nothing (bucket isolation): %+v", hits)
	}
}
