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
