package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── 自治理知识图谱 M2 语义关系层 ────────────────────────────────────────────
// fresh/存量两路径收敛：knowledge_relation_vocab（8 core 谓词种子）+
// knowledge_relation_state（抽取幂等）。

func setupRelationRepo(t *testing.T) *knowledgeRepo {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	if err := EnsureKnowledgeSchema(context.Background(), db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	repo, ok := NewKnowledgeRepo(&Data{rawDB: db, pg: db, pgRead: db, dialect: DialectPostgres},
		loggateway.NewNoop()).(*knowledgeRepo)
	if !ok {
		t.Fatal("NewKnowledgeRepo did not return *knowledgeRepo")
	}
	return repo
}

func seedRelationDocs(t *testing.T, db *sql.DB, ids ...string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1','c1','m')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO knowledge_documents (id, collection_id, source) VALUES ($1,'c1','s')`, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnsureKnowledgeSchema_M2RelationFreshShape(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	// 词表 8 core 谓词种子。
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_relation_vocab WHERE tier='core'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(bizknowledge.CoreRelations) {
		t.Errorf("core vocab = %d, want %d", n, len(bizknowledge.CoreRelations))
	}
	// state 表存在。
	var reg *string
	if err := db.QueryRowContext(ctx,
		`SELECT to_regclass('knowledge_relation_state')::text`).Scan(&reg); err != nil {
		t.Fatal(err)
	}
	if reg == nil || *reg == "" {
		t.Error("knowledge_relation_state missing")
	}
	// fresh 建库幂等（重跑不报错、种子不重复）。
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure knowledge schema rerun: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_relation_vocab`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(bizknowledge.CoreRelations) {
		t.Errorf("vocab after rerun = %d, want %d (seed must be idempotent)", n, len(bizknowledge.CoreRelations))
	}
}

func TestMigration20261221_RelationVocab(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	// 存量形态：只有 documents（state 表 FK 依赖）。
	if _, err := db.ExecContext(ctx, `CREATE TABLE knowledge_documents (
		id TEXT PRIMARY KEY, collection_id TEXT NOT NULL, source TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	run := func() {
		if err := executeSQLFileWithDialect(ctx, db,
			"sql/migrations/20261221_knowledge_relation_vocab.sql", DialectPostgres, loggateway.NewNoop()); err != nil {
			t.Fatalf("migration: %v", err)
		}
	}
	run()
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_relation_vocab WHERE tier='core'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(bizknowledge.CoreRelations) {
		t.Errorf("core vocab = %d, want %d", n, len(bizknowledge.CoreRelations))
	}
	run() // 幂等重跑
}

func TestKnowledgeRepo_ReplaceSemanticLinks(t *testing.T) {
	repo := setupRelationRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()
	seedRelationDocs(t, raw, "d1", "d2", "d3")

	links := []bizknowledge.SemanticLink{
		{TargetDocID: "d2", Relation: "depends-on", Confidence: 0.9, Context: "PostgreSQL"},
		{TargetDocID: "d3", Relation: "is-a", Confidence: 0.5, Closed: true, Context: "PG"},
		{TargetDocID: "d1", Relation: "self-loop", Confidence: 0.9}, // 自环跳过
		{TargetDocID: "", Relation: "no-target", Confidence: 0.9},   // 空目标跳过
	}
	if err := repo.ReplaceSemanticLinks(ctx, "c1", "d1", links); err != nil {
		t.Fatalf("ReplaceSemanticLinks: %v", err)
	}
	var open, closed int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FILTER (WHERE valid_to IS NULL), COUNT(*) FILTER (WHERE valid_to IS NOT NULL)
		 FROM knowledge_links WHERE doc_id='d1' AND link_type='semantic'`).Scan(&open, &closed); err != nil {
		t.Fatal(err)
	}
	if open != 1 || closed != 1 {
		t.Errorf("open=%d closed=%d, want 1/1 (self-loop & empty target skipped)", open, closed)
	}
	var conf float64
	var relation string
	if err := raw.QueryRowContext(ctx,
		`SELECT relation, confidence FROM knowledge_links
		 WHERE doc_id='d1' AND target_doc_id='d2' AND link_type='semantic'`).Scan(&relation, &conf); err != nil {
		t.Fatal(err)
	}
	if relation != "depends-on" || conf != 0.9 {
		t.Errorf("relation=%q confidence=%v, want depends-on/0.9", relation, conf)
	}
	// 替换语义：旧边关闭留历史，新边成为唯一 active。
	if err := repo.ReplaceSemanticLinks(ctx, "c1", "d1", []bizknowledge.SemanticLink{
		{TargetDocID: "d3", Relation: "applies-to", Confidence: 0.8},
	}); err != nil {
		t.Fatal(err)
	}
	var total, active int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE valid_to IS NULL)
		 FROM knowledge_links WHERE doc_id='d1' AND link_type='semantic'`).Scan(&total, &active); err != nil {
		t.Fatal(err)
	}
	if total != 3 || active != 1 {
		t.Errorf("after replace total=%d active=%d, want 3/1 (history preserved)", total, active)
	}
	// 同对文档多谓词共存（relation 参与唯一键）。
	if err := repo.ReplaceSemanticLinks(ctx, "c1", "d2", []bizknowledge.SemanticLink{
		{TargetDocID: "d3", Relation: "is-a", Confidence: 0.9},
	}); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_links WHERE doc_id='d2' AND target_doc_id='d3' AND link_type='semantic'`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("d2→d3 edges = %d, want 1", total)
	}
}

func TestKnowledgeRepo_UpsertCandidate(t *testing.T) {
	repo := setupRelationRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()

	if err := repo.UpsertCandidate(ctx, "extends", "llm"); err != nil {
		t.Fatal(err)
	}
	var tier string
	var useCount int
	if err := raw.QueryRowContext(ctx,
		`SELECT tier, use_count FROM knowledge_relation_vocab WHERE relation='extends'`).Scan(&tier, &useCount); err != nil {
		t.Fatal(err)
	}
	if tier != "candidate" || useCount != 0 {
		t.Errorf("new candidate tier=%q use_count=%d, want candidate/0", tier, useCount)
	}
	// 重复出现：use_count 递增。
	if err := repo.UpsertCandidate(ctx, "extends", "llm"); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRowContext(ctx,
		`SELECT use_count FROM knowledge_relation_vocab WHERE relation='extends'`).Scan(&useCount); err != nil {
		t.Fatal(err)
	}
	if useCount != 1 {
		t.Errorf("use_count = %d, want 1", useCount)
	}
	// core 谓词不降级不计数。
	if err := repo.UpsertCandidate(ctx, "is-a", "llm"); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRowContext(ctx,
		`SELECT tier, use_count FROM knowledge_relation_vocab WHERE relation='is-a'`).Scan(&tier, &useCount); err != nil {
		t.Fatal(err)
	}
	if tier != "core" || useCount != 0 {
		t.Errorf("core predicate touched: tier=%q use_count=%d", tier, useCount)
	}
}

func TestKnowledgeRepo_ListHotDocuments(t *testing.T) {
	repo := setupRelationRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()
	seedRelationDocs(t, raw, "hot", "warm", "cold", "old")

	insertHits := func(docID string, n int, age time.Duration) {
		for i := 0; i < n; i++ {
			if _, err := raw.ExecContext(ctx,
				`INSERT INTO knowledge_access_log (collection_id, doc_id, accessed_at)
				 VALUES ('c1', $1, NOW() - $2::interval)`, docID, age.String()); err != nil {
				t.Fatal(err)
			}
		}
	}
	insertHits("hot", 5, time.Hour)       // 窗口内 5 次
	insertHits("warm", 3, 24*time.Hour)   // 窗口内 3 次
	insertHits("cold", 1, time.Hour)      // 窗口内 1 次
	insertHits("old", 9, 90*24*time.Hour) // 90 天前 9 次（出窗）

	docs, err := repo.ListHotDocuments(ctx, "c1", 30, 3, 10)
	if err != nil {
		t.Fatalf("ListHotDocuments: %v", err)
	}
	if len(docs) != 2 || docs[0] != "hot" || docs[1] != "warm" {
		t.Errorf("hot docs = %v, want [hot warm]（cold 频次不足、old 出窗）", docs)
	}
	// limit 截断。
	docs, err = repo.ListHotDocuments(ctx, "c1", 30, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0] != "hot" {
		t.Errorf("limit=1 hot docs = %v, want [hot]", docs)
	}
}

func TestKnowledgeRepo_RelationStateRoundTrip(t *testing.T) {
	repo := setupRelationRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()
	seedRelationDocs(t, raw, "d1")

	// 未抽取：found=false 非错误。
	_, found, err := repo.GetRelationState(ctx, "d1")
	if err != nil || found {
		t.Fatalf("unextracted state found=%v err=%v, want false/nil", found, err)
	}
	now := time.Now()
	if err := repo.UpsertRelationState(ctx, bizknowledge.RelationState{
		DocID: "d1", CollectionID: "c1", ContentHash: "h1", RelationsExtractedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	st, found, err := repo.GetRelationState(ctx, "d1")
	if err != nil || !found {
		t.Fatalf("state found=%v err=%v", found, err)
	}
	if st.ContentHash != "h1" || st.RelationsExtractedAt.IsZero() || !st.EntitiesExtractedAt.IsZero() {
		t.Errorf("state = %+v, want hash h1 / relations set / entities zero", st)
	}
	// 零值时间不覆盖既有列（实体时间单独登记后不被关系时间抹掉）。
	if err := repo.UpsertRelationState(ctx, bizknowledge.RelationState{
		DocID: "d1", CollectionID: "c1", ContentHash: "h1", EntitiesExtractedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	st, _, err = repo.GetRelationState(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if st.EntitiesExtractedAt.IsZero() || st.RelationsExtractedAt.IsZero() {
		t.Errorf("state = %+v, want both timestamps kept (COALESCE merge)", st)
	}
	// 文档删除级联清理。
	if _, err := raw.ExecContext(ctx, `DELETE FROM knowledge_documents WHERE id='d1'`); err != nil {
		t.Fatal(err)
	}
	_, found, err = repo.GetRelationState(ctx, "d1")
	if err != nil || found {
		t.Errorf("after doc delete found=%v err=%v, want cascade gone", found, err)
	}
}

func TestKnowledgeRepo_ResolveEntityIDs(t *testing.T) {
	repo := setupRelationRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()
	seedRelationDocs(t, raw, "d1", "d2")

	// 经写式解析播种字典（含别名：PG → PostgreSQL keeper）。
	if _, err := repo.ReplaceDocEntities(ctx, "c1", "d1", []bizknowledge.DocEntity{
		{Name: "PostgreSQL", EntityType: "tech"}, {Name: "Redis", EntityType: "tech"},
	}); err != nil {
		t.Fatal(err)
	}
	// 合并造别名：PG 并入 PostgreSQL。
	var keeperID, mergeeID int64
	if err := raw.QueryRowContext(ctx,
		`SELECT id FROM knowledge_entities WHERE collection_id='c1' AND name_norm='redis'`).Scan(&mergeeID); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRowContext(ctx,
		`SELECT id FROM knowledge_entities WHERE collection_id='c1' AND name_norm='postgresql'`).Scan(&keeperID); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO knowledge_entity_aliases (collection_id, entity_id, alias_norm) VALUES ('c1', $1, 'pg')`, keeperID); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ResolveEntityIDs(ctx, "c1", []string{"postgresql", "PG", "pg", "Redis", "Unknown"})
	if err != nil {
		t.Fatalf("ResolveEntityIDs: %v", err)
	}
	if got["postgresql"] != keeperID {
		t.Errorf("postgresql → %d, want %d", got["postgresql"], keeperID)
	}
	if got["PG"] != keeperID || got["pg"] != keeperID {
		t.Errorf("alias PG/pg → %v/%v, want keeper %d", got["PG"], got["pg"], keeperID)
	}
	if got["Redis"] != mergeeID {
		t.Errorf("Redis → %d, want %d", got["Redis"], mergeeID)
	}
	if _, ok := got["Unknown"]; ok {
		t.Error("unknown name must be absent (read-only resolve, no dict creation)")
	}
	// 只读契约：不新建字典条目。
	var n int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_entities WHERE collection_id='c1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("entities = %d, want 2 (resolve must not create)", n)
	}
}
