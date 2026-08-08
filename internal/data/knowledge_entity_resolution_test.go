package data

// ── G5-F（B9/B12）：ReplaceDocEntities 归一化/别名解析 + FindEntityCooccurrences
// 按 entity_id 关联。解析管线：归一化 → 精确 name_norm → 别名命中 keeper → 新建。

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

func seedEntityDoc(t *testing.T, repo *knowledgeRepo, collectionID, docID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{
		ID: docID, CollectionID: collectionID, RelPath: docID + ".md", Source: docID + ".md", Status: "indexed",
	}); err != nil {
		t.Fatal(err)
	}
}

func setupEntityResolutionRepo(t *testing.T) *knowledgeRepo {
	t.Helper()
	repo := setupKnowledgeSearchRepo(t)
	if _, err := repo.CreateCollection(context.Background(),
		biz.KnowledgeCollection{ID: "c1", Name: "vault", RootPath: "/vault"}); err != nil {
		t.Fatal(err)
	}
	return repo
}

// 验收核心："AI"/"ai"/"ＡＩ" 聚合为同一实体；展示名保留首见写法。
func TestKnowledgeRepo_ReplaceDocEntities_NormAggregation(t *testing.T) {
	repo := setupEntityResolutionRepo(t)
	ctx := context.Background()
	seedEntityDoc(t, repo, "c1", "d1")
	seedEntityDoc(t, repo, "c1", "d2")

	ids1, err := repo.ReplaceDocEntities(ctx, "c1", "d1", []bizknowledge.DocEntity{
		{Name: "AI", EntityType: "tech", Mentions: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids1) != 1 {
		t.Fatalf("ids1 = %v, want 1 id", ids1)
	}

	// 大小写 + 全角写法命中同一实体；返回相同 ID。
	ids2, err := repo.ReplaceDocEntities(ctx, "c1", "d2", []bizknowledge.DocEntity{
		{Name: "ai", EntityType: "tech", Mentions: 3},
		{Name: "ＡＩ", EntityType: "concept", Mentions: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids2) != 1 || ids2[0] != ids1[0] {
		t.Fatalf("ids2 = %v, want [%d]（同 doc 内归一化撞车去重）", ids2, ids1[0])
	}

	// 全库仅 1 个实体，展示名 = 首见写法 "AI"，norm = "ai"。
	var count int
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_entities WHERE collection_id='c1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("entities = %d, want 1（AI/ai/ＡＩ 聚合）", count)
	}
	var name, norm string
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT name, name_norm FROM knowledge_entities WHERE id=$1`, ids1[0]).Scan(&name, &norm); err != nil {
		t.Fatal(err)
	}
	if name != "AI" || norm != "ai" {
		t.Errorf("entity = (%q,%q), want (AI,ai)：展示名保留首见写法", name, norm)
	}

	// 同 doc 撞车 mentions 求和：d2 = 3+1。
	var mentions int
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT mentions FROM knowledge_doc_entities WHERE doc_id='d2' AND entity_id=$1`, ids1[0]).Scan(&mentions); err != nil {
		t.Fatal(err)
	}
	if mentions != 4 {
		t.Errorf("d2 mentions = %d, want 4（撞车求和）", mentions)
	}

	// 命中时非空 entity_type 刷新（tech → concept 以最近一次为准是历史语义，此处验证非空更新生效）。
	ids3, err := repo.ReplaceDocEntities(ctx, "c1", "d1", []bizknowledge.DocEntity{
		{Name: "AI", EntityType: "concept", Mentions: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids3) != 1 || ids3[0] != ids1[0] {
		t.Fatalf("ids3 = %v, want [%d]", ids3, ids1[0])
	}
	var entityType string
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT entity_type FROM knowledge_entities WHERE id=$1`, ids1[0]).Scan(&entityType); err != nil {
		t.Fatal(err)
	}
	if entityType != "concept" {
		t.Errorf("entity_type = %q, want concept（命中刷新）", entityType)
	}
}

// B12：合并后 mergee 的 name_norm 落 keeper 别名；后续抽取别名命中跨同步持久路由到 keeper。
func TestKnowledgeRepo_ReplaceDocEntities_AliasRoutesToKeeper(t *testing.T) {
	repo := setupEntityResolutionRepo(t)
	ctx := context.Background()
	seedEntityDoc(t, repo, "c1", "d1")
	seedEntityDoc(t, repo, "c1", "d2")

	keeperIDs, err := repo.ReplaceDocEntities(ctx, "c1", "d1", []bizknowledge.DocEntity{
		{Name: "Claude", EntityType: "tech", Mentions: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 模拟合并善后状态：mergee norm "claude 3" 落 keeper 别名（B12 契约）。
	if _, err := repo.data.rawDB.ExecContext(ctx,
		`INSERT INTO knowledge_entity_aliases (collection_id, entity_id, alias_norm) VALUES ('c1', $1, 'claude 3')`,
		keeperIDs[0]); err != nil {
		t.Fatal(err)
	}

	ids, err := repo.ReplaceDocEntities(ctx, "c1", "d2", []bizknowledge.DocEntity{
		{Name: "Claude 3", EntityType: "tech", Mentions: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != keeperIDs[0] {
		t.Fatalf("ids = %v, want [%d]（别名命中 keeper）", ids, keeperIDs[0])
	}
	var count int
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_entities WHERE collection_id='c1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("entities = %d, want 1（别名路由不新建条目）", count)
	}
}

// 共现查询改按 entity_id 关联；SharedEntities 返回 keeper 展示名；R-3 频次过滤保留。
func TestKnowledgeRepo_FindEntityCooccurrences_ByEntityID(t *testing.T) {
	repo := setupEntityResolutionRepo(t)
	ctx := context.Background()
	for _, d := range []string{"d1", "d2", "d3", "d4"} {
		seedEntityDoc(t, repo, "c1", d)
	}

	// d1: AI + RAG；d2: ai（同 AI id）+ 高频词；d3: ＡＩ（同 AI id）+ 高频词；d4: 高频词。
	idsD1, err := repo.ReplaceDocEntities(ctx, "c1", "d1", []bizknowledge.DocEntity{
		{Name: "AI", EntityType: "tech", Mentions: 1},
		{Name: "RAG", EntityType: "tech", Mentions: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReplaceDocEntities(ctx, "c1", "d2", []bizknowledge.DocEntity{
		{Name: "ai", EntityType: "tech", Mentions: 1},
		{Name: "高频词", EntityType: "topic", Mentions: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReplaceDocEntities(ctx, "c1", "d3", []bizknowledge.DocEntity{
		{Name: "ＡＩ", EntityType: "tech", Mentions: 1},
		{Name: "高频词", EntityType: "topic", Mentions: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReplaceDocEntities(ctx, "c1", "d4", []bizknowledge.DocEntity{
		{Name: "高频词", EntityType: "topic", Mentions: 1},
	}); err != nil {
		t.Fatal(err)
	}

	// d1 的 AI（idsD1[0]）共现：d2/d3 共享，展示名 = keeper "AI"。
	coocs, err := repo.FindEntityCooccurrences(ctx, "c1", idsD1[:1], "d1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(coocs) != 2 {
		t.Fatalf("coocs = %v, want 2 docs (d2,d3)", coocs)
	}
	byDoc := map[string][]string{}
	for _, c := range coocs {
		byDoc[c.DocID] = c.SharedEntities
	}
	if len(byDoc["d2"]) != 1 || byDoc["d2"][0] != "AI" {
		t.Errorf("d2 shared = %v, want [AI]（keeper 展示名）", byDoc["d2"])
	}
	if len(byDoc["d3"]) != 1 || byDoc["d3"][0] != "AI" {
		t.Errorf("d3 shared = %v, want [AI]", byDoc["d3"])
	}

	// R-3 频次过滤：高频词出现在 3 个文档，maxDocFreq=2 → 噪声不返回。
	var hotID int64
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT id FROM knowledge_entities WHERE collection_id='c1' AND name_norm='高频词'`).Scan(&hotID); err != nil {
		t.Fatal(err)
	}
	coocs, err = repo.FindEntityCooccurrences(ctx, "c1", []int64{hotID}, "d2", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(coocs) != 0 {
		t.Errorf("hot entity coocs = %v, want empty（超频次上限过滤）", coocs)
	}

	// 空 ID 集短路。
	coocs, err = repo.FindEntityCooccurrences(ctx, "c1", nil, "d1", 0)
	if err != nil || len(coocs) != 0 {
		t.Errorf("empty ids: coocs=%v err=%v, want nil,nil", coocs, err)
	}
}

// B10 合并事务全契约：提及重写（冲突求和）+ 链接 context 重写 + 别名落库 +
// mergee 删除 + 重写条数返回；幂等重跑为零；合并后别名命中跨同步持久（B12）。
func TestKnowledgeRepo_MergeEntities(t *testing.T) {
	repo := setupEntityResolutionRepo(t)
	ctx := context.Background()
	for _, d := range []string{"d1", "d2", "d3"} {
		seedEntityDoc(t, repo, "c1", d)
	}

	idsD1, err := repo.ReplaceDocEntities(ctx, "c1", "d1", []bizknowledge.DocEntity{
		{Name: "AI", EntityType: "tech", Mentions: 2},
		{Name: "RAG", EntityType: "tech", Mentions: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	aiID, ragID := idsD1[0], idsD1[1]
	claudeIDs, err := repo.ReplaceDocEntities(ctx, "c1", "d3", []bizknowledge.DocEntity{
		{Name: "Claude", EntityType: "tech", Mentions: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	claudeID := claudeIDs[0]
	if _, err := repo.ReplaceDocEntities(ctx, "c1", "d2", []bizknowledge.DocEntity{
		{Name: "rag", EntityType: "tech", Mentions: 3}, // 归一化命中 RAG 同实体
	}); err != nil {
		t.Fatal(err)
	}
	// entity 轨链接 context 含 mergee 展示名。
	if err := repo.ReplaceLinks(ctx, "c1", "d1", bizknowledge.LinkTypeEntity, []bizknowledge.Link{
		{CollectionID: "c1", DocID: "d1", TargetDocID: "d2", LinkType: bizknowledge.LinkTypeEntity, Context: "RAG"},
	}); err != nil {
		t.Fatal(err)
	}

	// 合并 Claude → AI。
	res, err := repo.MergeEntities(ctx, "c1", aiID, []int64{claudeID})
	if err != nil {
		t.Fatalf("merge claude→AI: %v", err)
	}
	if res.MergedEntities != 1 || res.RewrittenMentions != 1 {
		t.Errorf("result = %+v, want merged=1 mentions=1", res)
	}
	// 别名落库 + mergee 删除。
	var aliasTarget int64
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT entity_id FROM knowledge_entity_aliases WHERE collection_id='c1' AND alias_norm='claude'`).Scan(&aliasTarget); err != nil {
		t.Fatalf("alias claude missing: %v", err)
	}
	if aliasTarget != aiID {
		t.Errorf("alias claude → %d, want keeper %d", aliasTarget, aiID)
	}
	var count int
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_entities WHERE id=$1`, claudeID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("mergee row not deleted")
	}
	// B12 验收：合并后新抽取 "Claude" 别名命中 keeper（跨同步持久），不新建条目。
	ids, err := repo.ReplaceDocEntities(ctx, "c1", "d3", []bizknowledge.DocEntity{
		{Name: "Claude", EntityType: "tech", Mentions: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != aiID {
		t.Fatalf("re-extract ids = %v, want [%d]（别名路由）", ids, aiID)
	}

	// 合并 RAG → AI：(d1,AI) 冲突 mentions 求和 2+5=7；链接 context 重写。
	res, err = repo.MergeEntities(ctx, "c1", aiID, []int64{ragID})
	if err != nil {
		t.Fatalf("merge rag→AI: %v", err)
	}
	if res.MergedEntities != 1 || res.RewrittenMentions != 2 || res.RewrittenLinks != 1 {
		t.Errorf("result = %+v, want merged=1 mentions=2 links=1", res)
	}
	var mentions int
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT mentions FROM knowledge_doc_entities WHERE doc_id='d1' AND entity_id=$1`, aiID).Scan(&mentions); err != nil {
		t.Fatal(err)
	}
	if mentions != 7 {
		t.Errorf("d1 AI mentions = %d, want 7（冲突求和）", mentions)
	}
	var linkCtx string
	if err := repo.data.rawDB.QueryRowContext(ctx,
		`SELECT context FROM knowledge_links WHERE doc_id='d1' AND link_type='entity'`).Scan(&linkCtx); err != nil {
		t.Fatal(err)
	}
	if linkCtx != "AI" {
		t.Errorf("link context = %q, want AI（keeper 展示名重写）", linkCtx)
	}

	// 幂等重跑：mergee 已不存在 → 零重写，不报错。
	res, err = repo.MergeEntities(ctx, "c1", aiID, []int64{ragID})
	if err != nil {
		t.Fatalf("idempotent re-merge: %v", err)
	}
	if res.MergedEntities != 0 || res.RewrittenMentions != 0 || res.RewrittenLinks != 0 {
		t.Errorf("re-merge = %+v, want zeros", res)
	}
	// keeper 不存在 → 错误。
	if _, err = repo.MergeEntities(ctx, "c1", 99999, []int64{aiID}); err == nil {
		t.Error("missing keeper must error")
	}
	// keeper 出现在 mergee_ids → 防御性剔除，不自并。
	res, err = repo.MergeEntities(ctx, "c1", aiID, []int64{aiID})
	if err != nil {
		t.Fatalf("self-merge guard: %v", err)
	}
	if res.MergedEntities != 0 {
		t.Errorf("self-merge merged = %d, want 0", res.MergedEntities)
	}
}
