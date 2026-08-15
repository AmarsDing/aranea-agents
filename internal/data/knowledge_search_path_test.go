package data

import (
	"context"
	"sort"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── G3-B7：SearchQuery.PathPrefix 搜索范围过滤（data 层 SQL） ────────────────

func setupKnowledgeSearchRepo(t *testing.T) *knowledgeRepo {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	// pgvector 扩展装在 public schema（库级唯一）；测试 schema 的 search_path 不含
	// public，vector 类型解析失败。钉单连接后把 public 追加进 search_path。
	db.SetMaxOpenConns(1)
	var schema string
	if err := db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO ` + schema + `, public`); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, pg: db, pgRead: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	if err := EnsureKnowledgeSchema(context.Background(), db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	return &knowledgeRepo{data: d, lg: loggateway.NewNoop()}
}

// 造数：4 个 vault 文档各 1 个 chunk，rel_path 覆盖 目录/边界/通配符形似 三类。
func seedPathPrefixChunks(t *testing.T, repo *knowledgeRepo) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: "/vault"}); err != nil {
		t.Fatal(err)
	}
	docs := []struct {
		id, relPath, chunkID, content string
		emb                           []float32
	}{
		{"d1", "notes/a.md", "k1", "apple banana", []float32{1, 0, 0}},
		{"d2", "archive/b.md", "k2", "apple cherry", []float32{0.9, 0.1, 0}},
		// `_` 是 LIKE 通配符：myXdir 必须不能被前缀 my_dir 命中（转义验证）。
		{"d3", "myXdir/g.md", "k3", "apple grape", []float32{0.8, 0.2, 0}},
		{"d4", "my_dir/f.md", "k4", "apple melon", []float32{0.7, 0.3, 0}},
	}
	for _, d := range docs {
		if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
			ID: d.id, CollectionID: "c1", RelPath: d.relPath, Source: d.relPath, Status: "indexed",
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
			ID: d.chunkID, DocID: d.id, CollectionID: "c1", Content: d.content, Embedding: d.emb,
		}}); err != nil {
			t.Fatal(err)
		}
	}
}

func chunkIDs(chunks []biz.KnowledgeChunk) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, c.ID)
	}
	sort.Strings(out)
	return out
}

func assertChunkIDs(t *testing.T, chunks []biz.KnowledgeChunk, want ...string) {
	t.Helper()
	got := chunkIDs(chunks)
	if len(got) != len(want) {
		t.Fatalf("chunk ids = %v, want %v", got, want)
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("chunk ids = %v, want %v", got, want)
		}
	}
}

func TestKnowledgeRepo_SearchChunks_PathPrefix(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	seedPathPrefixChunks(t, repo)
	ctx := context.Background()
	vec := []float32{1, 0, 0}

	// 目录前缀：只中 notes/ 下的 chunk。
	got, err := repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 10, PathPrefix: "notes"}, vec)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k1")

	// 空前缀 = 全库。
	got, err = repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 10}, vec)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k1", "k2", "k3", "k4")

	// 尾部斜杠容忍（前端树选中目录常带 /）。
	got, err = repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 10, PathPrefix: "notes/"}, vec)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k1")

	// 目录边界：note 不得误中 notes/（设计 §V12.3 B7 语义为目录范围）。
	got, err = repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 10, PathPrefix: "note"}, vec)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got)

	// LIKE 转义：`_` 按字面值，myXdir 不得被 my_dir 命中。
	got, err = repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 10, PathPrefix: "my_dir"}, vec)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k4")
}

func TestKnowledgeRepo_SearchChunksBM25_PathPrefix(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	seedPathPrefixChunks(t, repo)
	ctx := context.Background()

	got, err := repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "apple", TopK: 10, PathPrefix: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k1")

	got, err = repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "apple", TopK: 10, PathPrefix: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k2")

	// 空前缀 = 全库（tsvector/trgm 双路合并）。
	got, err = repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "apple", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k1", "k2", "k3", "k4")

	// LIKE 转义同样适用于 BM25 路径。
	got, err = repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "apple", TopK: 10, PathPrefix: "my_dir"})
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k4")
}

// ── P0 词条优先（2026-08-15 评审修订）：写回日记流水默认不进检索 ────────────
// ExcludePathPrefixes 按 rel_path 字面前缀排除整篇文档的全部 chunks：
// 词条页（entries/）照常可检索，inbox/writeback-* 流水只留 provenance。

func seedExcludePathChunks(t *testing.T, repo *knowledgeRepo) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "team"}); err != nil {
		t.Fatal(err)
	}
	docs := []struct {
		id, relPath, chunkID, content string
		emb                           []float32
	}{
		{"e1", "entries/灰度发布.md", "k1", "apple 灰度词条", []float32{1, 0, 0}},
		{"w1", "inbox/writeback-2026-08-15.md", "k2", "apple 日记流水一", []float32{0.9, 0.1, 0}},
		{"w2", "inbox/writeback-2026-08-14.md", "k3", "apple 日记流水二", []float32{0.8, 0.2, 0}},
		{"n1", "notes/其他笔记.md", "k4", "apple 普通笔记", []float32{0.7, 0.3, 0}},
	}
	for _, d := range docs {
		if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
			ID: d.id, CollectionID: "c1", RelPath: d.relPath, Source: d.relPath, Status: "indexed",
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
			ID: d.chunkID, DocID: d.id, CollectionID: "c1", Content: d.content, Embedding: d.emb,
		}}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestKnowledgeRepo_SearchChunks_ExcludePathPrefixes(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	seedExcludePathChunks(t, repo)
	ctx := context.Background()
	vec := []float32{1, 0, 0}
	excl := []string{"inbox/writeback-"}

	// dense：日记流水被排除，词条与普通笔记保留。
	got, err := repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{
		CollectionID: "c1", TopK: 10, ExcludePathPrefixes: excl,
	}, vec)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k1", "k4")

	// BM25 同语义。
	got, err = repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{
		CollectionID: "c1", Query: "apple", TopK: 10, ExcludePathPrefixes: excl,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k1", "k4")

	// 空排除列表 = 全库（回归保护）。
	got, err = repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 10}, vec)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k1", "k2", "k3", "k4")

	// 与 PathPrefix 组合：entries 范围内再排除流水（流水本不在范围内，结果不变）。
	got, err = repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{
		CollectionID: "c1", TopK: 10, PathPrefix: "entries", ExcludePathPrefixes: excl,
	}, vec)
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got, "k1")
}

// ── P1 recency 轻衰减（2026-08-15 评审修订）：score × exp(-λ·age)，
// 时间源 = knowledge_documents.updated_at（不用 chunks.created_at） ──────────

func TestKnowledgeRepo_SearchChunks_RecencyDecay(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "team"}); err != nil {
		t.Fatal(err)
	}
	// 两个 chunk 内容/向量完全相同，只有所属文档的 updated_at 不同。
	for _, d := range []struct{ id, relPath, chunkID string }{
		{"d-fresh", "entries/新词条.md", "k-fresh"},
		{"d-stale", "entries/旧词条.md", "k-stale"},
	} {
		if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
			ID: d.id, CollectionID: "c1", RelPath: d.relPath, Source: d.relPath, Status: "indexed",
		}); err != nil {
			t.Fatal(err)
		}
		if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
			ID: d.chunkID, DocID: d.id, CollectionID: "c1", Content: "apple 值班制度", Embedding: []float32{1, 0, 0},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	// 旧文档 updated_at 拨回 3 年前（衰减后 ≈ ×0.52）。
	if _, err := repo.data.Postgres().ExecContext(ctx,
		`UPDATE knowledge_documents SET updated_at = now() - interval '3 years' WHERE id = 'd-stale'`); err != nil {
		t.Fatal(err)
	}

	// dense：同语义分下新文档排前，旧文档仍返回（轻衰减非过滤）。
	got, err := repo.SearchChunks(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", TopK: 10}, []float32{1, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2", len(got))
	}
	if got[0].ID != "k-fresh" {
		t.Fatalf("fresh doc must rank first: %+v", got)
	}
	var freshScore, staleScore float32
	for _, ch := range got {
		if ch.ID == "k-fresh" {
			freshScore = ch.Score
		} else {
			staleScore = ch.Score
		}
	}
	if staleScore >= freshScore {
		t.Fatalf("decay not applied: fresh=%v stale=%v", freshScore, staleScore)
	}
	// 3 年 ≈ 0.52 倍，容差带 0.4~0.7 防时钟漂移。
	if r := staleScore / freshScore; r < 0.4 || r > 0.7 {
		t.Fatalf("decay ratio = %v, want ~0.52", r)
	}

	// BM25 同语义。
	got, err = repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{CollectionID: "c1", Query: "值班制度", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "k-fresh" {
		t.Fatalf("BM25 fresh must rank first: %+v", got)
	}
}

// ── 2026-08-10 e2e 事故根修：中文短查询词法降级失效 ─────────────────────────
// similarity(q, content) 的分母是双方 trigram 总数：中文 2-4 字短查询对长文档
// 相似度被稀释到 0.3 阈值以下（实测 "斑马线"=0.064），`%` 永不命中，无语义层
// 集合的词法降级路径对中文实质不可用。
// 修复：换 `word_similarity`（查询 vs 文本连续区间最大相似度，操作符 `%>`），
// 中文子串处相似度 ≥ 0.6 阈值命中。
func TestKnowledgeRepo_SearchChunksBM25_ChineseShortQuery(t *testing.T) {
	repo := setupKnowledgeSearchRepo(t)
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "czh", Name: "zh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
		ID: "dz1", CollectionID: "czh", Source: "zh.md", Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}
	// 无语义层集合的 chunk 无向量（R-4），词法路径是唯一检索手段。
	if err := repo.InsertChunks(ctx, []biz.KnowledgeChunk{{
		ID: "kzh1", DocID: "dz1", CollectionID: "czh",
		Content: "# 词法降级验证\n\nObsidian 双链语法与回链面板。BM25 词法检索专用文档，斑马线和卷帘门。",
	}}); err != nil {
		t.Fatal(err)
	}

	for _, q := range []string{"斑马线", "词法", "Obsidian"} {
		got, err := repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{CollectionID: "czh", Query: q, TopK: 5})
		if err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		assertChunkIDs(t, got, "kzh1")
	}
	// 不存在的词不得误中。
	got, err := repo.SearchChunksBM25(ctx, biz.KnowledgeSearchQuery{CollectionID: "czh", Query: "不存在的词语xx", TopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	assertChunkIDs(t, got)
}
