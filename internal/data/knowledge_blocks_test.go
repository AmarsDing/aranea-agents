package data

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── SP1-B：knowledge_blocks / knowledge_block_refs 物化（整文档删了重插） ─────

// setupKnowledgeBlocksRepo 建测试库：knowledge 基础表 + 20261203 块索引迁移。
func setupKnowledgeBlocksRepo(t *testing.T) *knowledgeBlockRepo {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	// pgvector 扩展装在 public schema；钉单连接后把 public 追加进 search_path
	//（与 setupKnowledgeSearchRepo 同款处理）。
	db.SetMaxOpenConns(1)
	var schema string
	if err := db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if _, err := db.Exec(`SET search_path TO ` + schema + `, public`); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, pg: db, pgRead: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	ctx := context.Background()
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	sqlBytes, err := migrationSQLFS.ReadFile("sql/migrations/20261203_knowledge_blocks.sql")
	if err != nil {
		t.Fatalf("read blocks migration: %v", err)
	}
	for _, stmt := range splitDDLStatements(strings.TrimSpace(string(sqlBytes))) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("apply blocks migration: %v\nstmt: %s", err, stmt)
		}
	}
	return &knowledgeBlockRepo{data: d, lg: loggateway.NewNoop()}
}

// seedDoc 造 collection + document 行（块表 FK 依赖）。
func seedDoc(t *testing.T, r *knowledgeBlockRepo, collID, docID string) {
	t.Helper()
	ctx := context.Background()
	kr := &knowledgeRepo{data: r.data, lg: loggateway.NewNoop()}
	if _, err := kr.CreateCollection(ctx, biz.KnowledgeCollection{ID: collID, Name: collID}); err != nil && !isPgUniqueViolation(err) {
		t.Fatalf("create collection: %v", err)
	}
	if _, err := kr.CreateDocument(ctx, biz.KnowledgeDocument{
		ID: docID, CollectionID: collID, Source: docID + ".md", Status: "indexed",
	}); err != nil {
		t.Fatalf("create document: %v", err)
	}
}

func blockRows(ids ...string) []bizknowledge.KnowledgeBlock {
	out := make([]bizknowledge.KnowledgeBlock, 0, len(ids))
	for i, id := range ids {
		b := bizknowledge.KnowledgeBlock{
			Ordinal: i, Kind: "paragraph",
			ContentHash: "h-" + id, TextExcerpt: "excerpt " + id,
		}
		if strings.HasPrefix(id, "^") {
			b.Anchor = strings.TrimPrefix(id, "^")
		}
		out = append(out, b)
	}
	return out
}

func countRows(t *testing.T, r *knowledgeBlockRepo, table, where string, args ...any) int {
	t.Helper()
	var n int
	if err := r.data.rawDB.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestKnowledgeBlocksReplace_Basic 基本写入：块落库 + 锚块 ID=anchor + refs 按 ordinal 映射。
func TestKnowledgeBlocksReplace_Basic(t *testing.T) {
	r := setupKnowledgeBlocksRepo(t)
	seedDoc(t, r, "c1", "d1")
	ctx := context.Background()

	blocks := blockRows("p0", "^a1", "p2")
	blocks[1].HeadingPath = []string{"H1", "H2"}
	refs := []bizknowledge.KnowledgeBlockRefInput{
		{SrcOrdinal: 0, RawTarget: "Note", EdgeType: "ref", Context: "见 Note。"},
		{SrcOrdinal: 2, RawTarget: "img.png", Alias: "300", EdgeType: "embed"},
	}
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "d1", blocks, refs); err != nil {
		t.Fatalf("ReplaceDocBlocks: %v", err)
	}

	got, err := r.ListDocBlocks(ctx, "d1")
	if err != nil {
		t.Fatalf("ListDocBlocks: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("blocks = %d, want 3", len(got))
	}
	if got[0].Ordinal != 0 || got[1].Ordinal != 1 || got[2].Ordinal != 2 {
		t.Fatalf("ordinals 乱序: %+v", got)
	}
	// 锚块 ID = 文本中 ^anchor（设计 S2）；未锚块生成非空 ID。
	if got[1].ID != "a1" {
		t.Errorf("anchored block ID = %q, want a1", got[1].ID)
	}
	if got[0].ID == "" || got[2].ID == "" {
		t.Errorf("unanchored block ID 为空: %+v", got)
	}
	if len(got[1].HeadingPath) != 2 || got[1].HeadingPath[1] != "H2" {
		t.Errorf("HeadingPath = %v", got[1].HeadingPath)
	}
	if got[1].Anchor != "a1" || got[0].Anchor != "" {
		t.Errorf("Anchor 字段错误: %q / %q", got[1].Anchor, got[0].Anchor)
	}

	// refs：src_block_id 已映射到块 ID。
	var rawTarget, srcID string
	var dstDoc, dstBlock *string
	if err := r.data.rawDB.QueryRow(
		`SELECT raw_target, src_block_id, dst_doc_id, dst_block_id FROM knowledge_block_refs WHERE collection_id='c1' AND raw_target='Note'`,
	).Scan(&rawTarget, &srcID, &dstDoc, &dstBlock); err != nil {
		t.Fatalf("query ref: %v", err)
	}
	if srcID != got[0].ID {
		t.Errorf("src_block_id = %q, want %q", srcID, got[0].ID)
	}
	if dstDoc != nil || dstBlock != nil {
		t.Errorf("未解析引用应 dst 全 NULL（dangling）: %v/%v", dstDoc, dstBlock)
	}
}

// TestKnowledgeBlocksReplace_Rebuild 重建幂等：整文档删了重插，旧块旧边清零，无孤儿。
func TestKnowledgeBlocksReplace_Rebuild(t *testing.T) {
	r := setupKnowledgeBlocksRepo(t)
	seedDoc(t, r, "c1", "d1")
	ctx := context.Background()

	if _, err := r.ReplaceDocBlocks(ctx, "c1", "d1", blockRows("p0", "p1"), []bizknowledge.KnowledgeBlockRefInput{
		{SrcOrdinal: 0, RawTarget: "Old", EdgeType: "ref"},
	}); err != nil {
		t.Fatal(err)
	}
	// 同内容重建：行数不变，无重复。
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "d1", blockRows("p0", "p1"), []bizknowledge.KnowledgeBlockRefInput{
		{SrcOrdinal: 0, RawTarget: "Old", EdgeType: "ref"},
	}); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, r, "knowledge_blocks", "doc_id='d1'"); n != 2 {
		t.Errorf("重建后 blocks = %d, want 2", n)
	}
	if n := countRows(t, r, "knowledge_block_refs", "collection_id='c1'"); n != 1 {
		t.Errorf("重建后 refs = %d, want 1（refs 重插不累积）", n)
	}

	// 内容变化重建：新块替换旧块。
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "d1", blockRows("q0"), nil); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, r, "knowledge_blocks", "doc_id='d1'"); n != 1 {
		t.Errorf("变更后 blocks = %d, want 1", n)
	}
	if n := countRows(t, r, "knowledge_block_refs", "collection_id='c1'"); n != 0 {
		t.Errorf("变更后 refs = %d, want 0（旧 src 边级联清除）", n)
	}
}

// TestKnowledgeBlocksReplace_DstDangling 目标块被重建删除：dst 置 NULL 保 raw_target（复活线索）。
func TestKnowledgeBlocksReplace_DstDangling(t *testing.T) {
	r := setupKnowledgeBlocksRepo(t)
	seedDoc(t, r, "c1", "dA")
	seedDoc(t, r, "c1", "dB")
	ctx := context.Background()

	// dB 有一块 ^b1；dA 引用它（已解析）。
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "dB", blockRows("^b1"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "dA", blockRows("p0"), []bizknowledge.KnowledgeBlockRefInput{
		{SrcOrdinal: 0, RawTarget: "dB#^b1", EdgeType: "ref", DstDocID: "dB", DstBlockID: "b1"},
	}); err != nil {
		t.Fatal(err)
	}

	// dB 重建（^b1 消失）→ dA 的边 dst 转 dangling，raw_target 保留。
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "dB", blockRows("p9"), nil); err != nil {
		t.Fatal(err)
	}
	var rawTarget string
	var dstDoc, dstBlock *string
	if err := r.data.rawDB.QueryRow(
		`SELECT raw_target, dst_doc_id, dst_block_id FROM knowledge_block_refs WHERE src_block_id IN (SELECT id FROM knowledge_blocks WHERE doc_id='dA')`,
	).Scan(&rawTarget, &dstDoc, &dstBlock); err != nil {
		t.Fatalf("query ref: %v", err)
	}
	if dstBlock != nil {
		t.Errorf("dst_block_id 应为 NULL（dangling），got %v", *dstBlock)
	}
	if rawTarget != "dB#^b1" {
		t.Errorf("raw_target = %q（复活线索必须保留）", rawTarget)
	}
}

// TestKnowledgeBlocks_AnchorUniquePerCollection 锚点库级唯一（部分唯一索引）。
func TestKnowledgeBlocks_AnchorUniquePerCollection(t *testing.T) {
	r := setupKnowledgeBlocksRepo(t)
	seedDoc(t, r, "c1", "d1")
	seedDoc(t, r, "c1", "d2")
	ctx := context.Background()

	if _, err := r.ReplaceDocBlocks(ctx, "c1", "d1", blockRows("^dup"), nil); err != nil {
		t.Fatal(err)
	}
	_, err := r.ReplaceDocBlocks(ctx, "c1", "d2", blockRows("^dup"), nil)
	if err == nil {
		t.Fatal("同 collection 锚冲突应报错")
	}
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Errorf("锚冲突应映射 CodeConflict, got %v", err)
	}
	// 无锚块不受唯一约束影响。
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "d2", blockRows("p0", "p1"), nil); err != nil {
		t.Errorf("无锚块写入不应受唯一索引影响: %v", err)
	}
}

// TestKnowledgeBlocksReplace_SelfReference 自文档引用（SP1-C）：Resolver 只给目标
// 块 ordinal（新块未落库无 ID），存储层按本次插入的 ordinal→ID 映射回填 dst_block_id；
// ordinal 越界属契约违例 → CodeBadRequest 且整个事务回滚（块也不残留）。
func TestKnowledgeBlocksReplace_SelfReference(t *testing.T) {
	r := setupKnowledgeBlocksRepo(t)
	seedDoc(t, r, "c1", "d1")
	ctx := context.Background()

	blocks := blockRows("h0", "^a1", "p2")
	blocks[0].Kind = "heading"
	blocks[0].HeadingPath = []string{"H1"}
	ordAnchor, ordHeading := 1, 0
	refs := []bizknowledge.KnowledgeBlockRefInput{
		// 自引用锚块：[[#^a1]]（Resolver 产物：DstDocID=源文档，DstSelfOrdinal=目标 ordinal）。
		{SrcOrdinal: 2, RawTarget: "#^a1", EdgeType: "ref", DstDocID: "d1", DstSelfOrdinal: &ordAnchor},
		// 自引用标题块：[[#H1]]。
		{SrcOrdinal: 2, RawTarget: "#H1", EdgeType: "ref", DstDocID: "d1", DstSelfOrdinal: &ordHeading},
	}
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "d1", blocks, refs); err != nil {
		t.Fatalf("ReplaceDocBlocks: %v", err)
	}

	got, err := r.ListDocBlocks(ctx, "d1")
	if err != nil {
		t.Fatalf("ListDocBlocks: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("blocks = %d, want 3", len(got))
	}
	// 回填值必须指向本次插入的块 ID（锚块 = anchor 文本；标题块 = 生成 ID）。
	wantDst := map[string]string{"#^a1": "a1", "#H1": got[0].ID}
	rows, err := r.data.rawDB.QueryContext(ctx,
		`SELECT raw_target, dst_block_id FROM knowledge_block_refs WHERE collection_id='c1' ORDER BY raw_target`)
	if err != nil {
		t.Fatalf("query refs: %v", err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[string]string{}
	for rows.Next() {
		var raw string
		var dst *string
		if err := rows.Scan(&raw, &dst); err != nil {
			t.Fatalf("scan ref: %v", err)
		}
		if dst == nil {
			t.Errorf("自引用 dst_block_id 应已回填, raw=%q 为 NULL", raw)
			continue
		}
		seen[raw] = *dst
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	for raw, want := range wantDst {
		if seen[raw] != want {
			t.Errorf("raw=%q dst_block_id = %q, want %q", raw, seen[raw], want)
		}
	}

	// 契约违例：DstSelfOrdinal 指向不存在的块 → CodeBadRequest + 事务回滚零残留。
	bad := 99
	_, err = r.ReplaceDocBlocks(ctx, "c1", "d1", blockRows("x0"), []bizknowledge.KnowledgeBlockRefInput{
		{SrcOrdinal: 0, RawTarget: "#^ghost", EdgeType: "ref", DstDocID: "d1", DstSelfOrdinal: &bad},
	})
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("越界 DstSelfOrdinal 应 CodeBadRequest, got %v", err)
	}
	if n := countRows(t, r, "knowledge_blocks", "doc_id='d1'"); n != 3 {
		t.Errorf("回滚后块应保持 3（事务不得残留半提交状态）, got %d", n)
	}
}

// TestKnowledgeBlocks_DocDeleteCascade 文档删除级联：块与 src 边清除，入向边转 dangling。
func TestKnowledgeBlocks_DocDeleteCascade(t *testing.T) {
	r := setupKnowledgeBlocksRepo(t)
	seedDoc(t, r, "c1", "dA")
	seedDoc(t, r, "c1", "dB")
	ctx := context.Background()

	if _, err := r.ReplaceDocBlocks(ctx, "c1", "dB", blockRows("^b1"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReplaceDocBlocks(ctx, "c1", "dA", blockRows("p0"), []bizknowledge.KnowledgeBlockRefInput{
		{SrcOrdinal: 0, RawTarget: "dB#^b1", EdgeType: "ref", DstDocID: "dB", DstBlockID: "b1"},
	}); err != nil {
		t.Fatal(err)
	}

	// 删除 dB 文档行（FK 级联删块；dA 入向边 dst_doc_id/dst_block_id 置 NULL）。
	if _, err := r.data.rawDB.ExecContext(ctx, `DELETE FROM knowledge_documents WHERE id='dB'`); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, r, "knowledge_blocks", "doc_id='dB'"); n != 0 {
		t.Errorf("文档删除后块应级联清除, got %d", n)
	}
	var dstDoc, dstBlock *string
	if err := r.data.rawDB.QueryRow(
		`SELECT dst_doc_id, dst_block_id FROM knowledge_block_refs WHERE collection_id='c1'`,
	).Scan(&dstDoc, &dstBlock); err != nil {
		t.Fatal(err)
	}
	if dstDoc != nil || dstBlock != nil {
		t.Errorf("目标文档删除后 dst 应全 NULL（dangling）: %v/%v", dstDoc, dstBlock)
	}
}
