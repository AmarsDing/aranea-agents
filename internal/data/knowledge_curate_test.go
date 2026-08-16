package data

import (
	"context"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── 自治理知识图谱 M4 自治理层（PG 集成） ──────────────────────────────────
// 契约：decay 衰减+弱边关闭（dry_run 只预估）；candidate 谓词超阈值提升 promoted；
// 孤儿/陈旧/contradicts 候选检出语义正确；提案去重探测与人工二审闭环。

func setupCurateRepo(t *testing.T) *knowledgeRepo {
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

func seedCurateDoc(t *testing.T, repo *knowledgeRepo, id, relPath string) {
	t.Helper()
	ctx := context.Background()
	raw := repo.data.Postgres()
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1','c1','m')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO knowledge_documents (id, collection_id, source, rel_path) VALUES ($1,'c1','s',$2)`, id, relPath); err != nil {
		t.Fatal(err)
	}
}

func seedCurateLink(t *testing.T, repo *knowledgeRepo, docID, targetID, linkType, relation string, weight float64, closed bool) {
	t.Helper()
	ctx := context.Background()
	q := `INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type, relation, weight_f, valid_to)
	      VALUES ('c1', $1, $2, $3, $4, $5, CASE WHEN $6 THEN NOW() ELSE NULL END)`
	if _, err := repo.data.Postgres().ExecContext(ctx, q, docID, targetID, linkType, relation, weight, closed); err != nil {
		t.Fatal(err)
	}
}

func TestKnowledgeRepo_DecayCoActivatedEdges(t *testing.T) {
	repo := setupCurateRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()
	for _, id := range []string{"d1", "d2", "d3", "d4"} {
		seedCurateDoc(t, repo, id, "notes/"+id+".md")
	}
	// 两条活跃 co_activated 边：1.0（衰而不关）与 0.055（0.055*0.9=0.0495 <0.05 关闭）。
	seedCurateLink(t, repo, "d1", "d2", "co_activated", "", 1.0, false)
	seedCurateLink(t, repo, "d2", "d3", "co_activated", "", 0.055, false)
	// 一条 semantic 边对照（不衰减）。
	seedCurateLink(t, repo, "d1", "d4", "semantic", "depends-on", 0.8, false)

	// dry_run：预估不写库。
	decayed, closed, err := repo.DecayCoActivatedEdges(ctx, "c1", 0.9, 0.05, true)
	if err != nil {
		t.Fatal(err)
	}
	if decayed != 2 || closed != 1 {
		t.Fatalf("dry-run estimate = %d/%d, want 2/1", decayed, closed)
	}
	var w float64
	if err := raw.QueryRowContext(ctx,
		`SELECT weight_f FROM knowledge_links WHERE doc_id='d2' AND link_type='co_activated'`).Scan(&w); err != nil {
		t.Fatal(err)
	}
	if w != 0.055 {
		t.Fatalf("dry-run must not update weight, got %v", w)
	}

	// 实执：权重衰减、弱边关闭、semantic 边不动。
	decayed, closed, err = repo.DecayCoActivatedEdges(ctx, "c1", 0.9, 0.05, false)
	if err != nil {
		t.Fatal(err)
	}
	if decayed != 2 || closed != 1 {
		t.Fatalf("decay = %d/%d, want 2/1", decayed, closed)
	}
	if err := raw.QueryRowContext(ctx,
		`SELECT weight_f FROM knowledge_links WHERE doc_id='d1' AND link_type='co_activated'`).Scan(&w); err != nil {
		t.Fatal(err)
	}
	if w < 0.89 || w > 0.91 {
		t.Fatalf("weight after decay = %v, want ~0.9", w)
	}
	var closedCount, semanticActive int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FILTER (WHERE valid_to IS NOT NULL AND link_type='co_activated'),
		        COUNT(*) FILTER (WHERE valid_to IS NULL AND link_type='semantic')
		 FROM knowledge_links`).Scan(&closedCount, &semanticActive); err != nil {
		t.Fatal(err)
	}
	if closedCount != 1 || semanticActive != 1 {
		t.Fatalf("closed=%d semanticActive=%d, want 1/1", closedCount, semanticActive)
	}
}

func TestKnowledgeRepo_PromoteRelation(t *testing.T) {
	repo := setupCurateRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO knowledge_relation_vocab (relation, tier, proposed_by, use_count) VALUES
		 ('owned-by', 'candidate', 'llm', 5),
		 ('part-of-x', 'candidate', 'llm', 1)
		 ON CONFLICT (relation) DO NOTHING`); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListPromotableRelations(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "owned-by" {
		t.Fatalf("promotable = %+v, want [owned-by]", got)
	}
	if err := repo.PromoteRelation(ctx, "owned-by"); err != nil {
		t.Fatal(err)
	}
	var tier string
	if err := raw.QueryRowContext(ctx,
		`SELECT tier FROM knowledge_relation_vocab WHERE relation='owned-by'`).Scan(&tier); err != nil {
		t.Fatal(err)
	}
	if tier != "promoted" {
		t.Fatalf("tier = %q, want promoted", tier)
	}
	// 低 use_count 不提升；core 种子不受影响。
	var candidateLeft, coreCount int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FILTER (WHERE tier='candidate'), COUNT(*) FILTER (WHERE tier='core')
		 FROM knowledge_relation_vocab`).Scan(&candidateLeft, &coreCount); err != nil {
		t.Fatal(err)
	}
	if candidateLeft != 1 || coreCount != len(bizknowledge.CoreRelations) {
		t.Fatalf("candidate=%d core=%d", candidateLeft, coreCount)
	}
}

func TestKnowledgeRepo_ListOrphanEntries(t *testing.T) {
	repo := setupCurateRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()
	seedCurateDoc(t, repo, "o1", "entries/孤儿旧.md")
	seedCurateDoc(t, repo, "o2", "entries/孤儿新访问.md")
	seedCurateDoc(t, repo, "o3", "entries/有边.md")
	seedCurateDoc(t, repo, "o4", "notes/非词条.md") // 非 entries/ 前缀不算词条
	seedCurateDoc(t, repo, "o5", "entries/边目标.md")
	seedCurateLink(t, repo, "o3", "o5", "semantic", "is-a", 0.9, false)
	// o2 最近被检索（排除）；o1 40 天前检索（命中）；o3 无边但非词条路径不算孤儿（有 active 边也排除）。
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO knowledge_access_log (collection_id, doc_id, accessed_at) VALUES
		 ('c1','o2', NOW()),
		 ('c1','o1', NOW() - interval '40 days')`); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListOrphanEntries(ctx, "c1", 30, 50)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, o := range got {
		ids[o.DocID] = true
	}
	if !ids["o1"] {
		t.Fatalf("stale orphan o1 must be listed: %+v", got)
	}
	if ids["o2"] || ids["o3"] || ids["o4"] || ids["o5"] {
		t.Fatalf("recent/linked/non-entry docs must be excluded: %+v", got)
	}
}

func TestKnowledgeRepo_ListStaleEntries(t *testing.T) {
	repo := setupCurateRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()
	seedCurateDoc(t, repo, "s1", "entries/陈旧.md")
	seedCurateDoc(t, repo, "s2", "entries/新鲜.md")
	seedCurateDoc(t, repo, "s3", "entries/无关闭边.md")
	// s1：两条出向 semantic 边一关一开（关闭比例 0.5）+ 40 天未检索 → 陈旧。
	seedCurateDoc(t, repo, "t1", "entries/目标1.md")
	seedCurateDoc(t, repo, "t2", "entries/目标2.md")
	seedCurateLink(t, repo, "s1", "t1", "semantic", "is-a", 0.9, true)
	seedCurateLink(t, repo, "s1", "t2", "semantic", "part-of", 0.9, false)
	// s3：两边全活跃 → 不陈旧。
	seedCurateLink(t, repo, "s3", "t1", "semantic", "is-a", 0.9, false)
	seedCurateLink(t, repo, "s3", "t2", "semantic", "part-of", 0.9, false)
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO knowledge_access_log (collection_id, doc_id, accessed_at) VALUES
		 ('c1','s1', NOW() - interval '40 days'),
		 ('c1','s2', NOW())`); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListStaleEntries(ctx, "c1", 30, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].DocID != "s1" {
		t.Fatalf("stale = %+v, want only s1", got)
	}
	if got[0].ClosedRatio < 0.5 || got[0].LastAccessDays < 39 {
		t.Fatalf("stale stat = %+v", got[0])
	}
}

func TestKnowledgeRepo_ListContradictsEdges(t *testing.T) {
	repo := setupCurateRepo(t)
	ctx := context.Background()
	seedCurateDoc(t, repo, "c1d", "entries/甲.md")
	seedCurateDoc(t, repo, "c2d", "entries/乙.md")
	seedCurateDoc(t, repo, "c3d", "entries/丙.md")
	seedCurateLink(t, repo, "c1d", "c2d", "semantic", "contradicts", 0.9, false) // active → 命中
	seedCurateLink(t, repo, "c1d", "c3d", "semantic", "contradicts", 0.8, true)  // closed → 排除
	seedCurateLink(t, repo, "c2d", "c3d", "semantic", "depends-on", 0.9, false)  // 非矛盾 → 排除

	got, err := repo.ListContradictsEdges(ctx, "c1", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].DocID != "c1d" || got[0].TargetDocID != "c2d" {
		t.Fatalf("contradicts = %+v", got)
	}
}

func TestKnowledgeRepo_HasProposalAndResolve(t *testing.T) {
	repo := setupCurateRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()

	if err := repo.InsertProposal(ctx, bizknowledge.GovernanceProposal{
		CollectionID: "c1", Kind: bizknowledge.ProposalKindOrphan, Risk: bizknowledge.ProposalRiskHigh,
		Status:  bizknowledge.ProposalStatusPending,
		Payload: map[string]any{"dedup_key": "orphan:d1", "doc_id": "d1"},
	}); err != nil {
		t.Fatal(err)
	}

	// 去重探测：同 key pending 命中；不同 key 不命中。
	exists, err := repo.HasProposal(ctx, "c1", bizknowledge.ProposalKindOrphan, "orphan:d1", []string{bizknowledge.ProposalStatusPending})
	if err != nil || !exists {
		t.Fatalf("has proposal = %v, %v", exists, err)
	}
	exists, err = repo.HasProposal(ctx, "c1", bizknowledge.ProposalKindOrphan, "orphan:d2", []string{bizknowledge.ProposalStatusPending})
	if err != nil || exists {
		t.Fatalf("different dedup key must miss: %v", exists)
	}

	// 人工二审：pending → applied，resolved_at 落时间；二审后 pending 探测不再命中。
	var id int64
	if err := raw.QueryRowContext(ctx,
		`SELECT id FROM knowledge_governance_proposal WHERE kind='orphan'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := repo.ResolveGovernanceProposal(ctx, id, bizknowledge.ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	var status string
	var resolvedAt *string
	if err := raw.QueryRowContext(ctx,
		`SELECT status, resolved_at::text FROM knowledge_governance_proposal WHERE id=$1`, id).Scan(&status, &resolvedAt); err != nil {
		t.Fatal(err)
	}
	if status != "applied" || resolvedAt == nil {
		t.Fatalf("resolved = %s/%v", status, resolvedAt)
	}
	exists, err = repo.HasProposal(ctx, "c1", bizknowledge.ProposalKindOrphan, "orphan:d1", []string{bizknowledge.ProposalStatusPending})
	if err != nil || exists {
		t.Fatalf("applied proposal must leave pending dedup: %v", exists)
	}
	// applied 状态仍可用于 stale 类去重（pending+applied 双态）。
	exists, err = repo.HasProposal(ctx, "c1", bizknowledge.ProposalKindOrphan, "orphan:d1",
		[]string{bizknowledge.ProposalStatusPending, bizknowledge.ProposalStatusApplied})
	if err != nil || !exists {
		t.Fatalf("applied status dedup must hit: %v", exists)
	}
	// 非 pending 提案不能重复二审。
	if err := repo.ResolveGovernanceProposal(ctx, id, bizknowledge.ProposalStatusRejected); err == nil {
		t.Fatal("re-resolve must error")
	}
}

func TestKnowledgeRepo_ListGovernanceProposals(t *testing.T) {
	repo := setupCurateRepo(t)
	ctx := context.Background()

	for _, p := range []bizknowledge.GovernanceProposal{
		{CollectionID: "c1", Kind: bizknowledge.ProposalKindOrphan, Risk: bizknowledge.ProposalRiskHigh,
			Status: bizknowledge.ProposalStatusPending, Payload: map[string]any{"dedup_key": "orphan:d1", "doc_id": "d1"}},
		{CollectionID: "c1", Kind: bizknowledge.ProposalKindStale, Risk: bizknowledge.ProposalRiskLow,
			Status: bizknowledge.ProposalStatusApplied, Payload: map[string]any{"dedup_key": "stale:d2", "doc_id": "d2"}},
		{CollectionID: "c2", Kind: bizknowledge.ProposalKindConflict, Risk: bizknowledge.ProposalRiskHigh,
			Status: bizknowledge.ProposalStatusPending, Payload: map[string]any{"dedup_key": "conflict:a→b"}},
	} {
		if err := repo.InsertProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	// 全量：3 条，id DESC（最新 id 最大在前）。
	all, err := repo.ListGovernanceProposals(ctx, "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].ID < all[2].ID {
		t.Fatalf("all = %+v", all)
	}
	if all[0].CreatedAt.IsZero() || all[0].Payload["dedup_key"] == nil {
		t.Fatalf("view projection broken: %+v", all[0])
	}
	// status 过滤：pending 2 条（c1 orphan + c2 conflict）。
	pending, err := repo.ListGovernanceProposals(ctx, "", bizknowledge.ProposalStatusPending, 50)
	if err != nil || len(pending) != 2 {
		t.Fatalf("pending = %d, %v", len(pending), err)
	}
	// collection 过滤：c2 1 条。
	c2, err := repo.ListGovernanceProposals(ctx, "c2", "", 50)
	if err != nil || len(c2) != 1 || c2[0].Kind != bizknowledge.ProposalKindConflict {
		t.Fatalf("c2 = %+v, %v", c2, err)
	}
	// resolved_at 投影：applied 行经二审落时间后非零。
	var id int64
	if err := repo.data.Postgres().QueryRowContext(ctx,
		`SELECT id FROM knowledge_governance_proposal WHERE kind='orphan'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := repo.ResolveGovernanceProposal(ctx, id, bizknowledge.ProposalStatusRejected); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListGovernanceProposals(ctx, "c1", bizknowledge.ProposalStatusRejected, 50)
	if err != nil || len(got) != 1 || got[0].ResolvedAt.IsZero() {
		t.Fatalf("resolved view = %+v, %v", got, err)
	}
}

// ListHubClusters 契约：只统计 entries↔entries 的 active 边（inbox 流水与 closed 边排除）；
// 度数 = 无向端点计数，>= minDegree 入选，按 degree DESC, doc_id 排序，limit 截断；
// 邻居为 1 跳 DISTINCT entries 对端。
func TestKnowledgeRepo_ListHubClusters(t *testing.T) {
	repo := setupCurateRepo(t)
	ctx := context.Background()

	for _, id := range []string{"h", "a", "b", "c", "d", "e", "f", "c2"} {
		seedCurateDoc(t, repo, id, "entries/"+id+".md")
	}
	seedCurateDoc(t, repo, "inbox-note", "inbox/writeback-x.md")

	// 星型簇：h-a/h-b/h-c + a-b（h 度 3，a/b 度 2，c 度 1）；d-e 对照（各度 1）。
	seedCurateLink(t, repo, "h", "a", "semantic", "depends-on", 0.9, false)
	seedCurateLink(t, repo, "h", "b", "co_activated", "", 0.6, false)
	seedCurateLink(t, repo, "h", "c", "semantic", "related-to", 0.7, false)
	seedCurateLink(t, repo, "a", "b", "semantic", "related-to", 0.8, false)
	seedCurateLink(t, repo, "d", "e", "semantic", "related-to", 0.8, false)
	// inbox 流水边：两端非 entries/*，不得进入度数与邻居。
	seedCurateLink(t, repo, "h", "inbox-note", "co_activated", "", 1.0, false)
	// closed 边：valid_to 置位，不计入。
	seedCurateLink(t, repo, "h", "c2", "semantic", "related-to", 0.9, true)

	hubs, err := repo.ListHubClusters(ctx, "c1", 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hubs) != 3 {
		t.Fatalf("hubs = %+v, want 3", hubs)
	}
	// 排序：degree DESC, doc_id → h(3), a(2), b(2)。
	if hubs[0].HubDocID != "h" || hubs[0].Degree != 3 || hubs[0].HubRelPath != "entries/h.md" {
		t.Fatalf("hubs[0] = %+v", hubs[0])
	}
	if hubs[1].HubDocID != "a" || hubs[1].Degree != 2 || hubs[2].HubDocID != "b" || hubs[2].Degree != 2 {
		t.Fatalf("hubs[1..2] = %+v / %+v", hubs[1], hubs[2])
	}
	// h 的邻居：a/b/c 三个 entries 对端；inbox-note 与 closed 的 c2 不得混入。
	neighbors := map[string]bool{}
	for _, m := range hubs[0].Neighbors {
		neighbors[m.DocID] = true
	}
	if len(hubs[0].Neighbors) != 3 || !neighbors["a"] || !neighbors["b"] || !neighbors["c"] {
		t.Fatalf("h neighbors = %+v", hubs[0].Neighbors)
	}
	if neighbors["inbox-note"] || neighbors["c2"] {
		t.Fatalf("inbox/closed leaked into neighbors: %+v", hubs[0].Neighbors)
	}
	// a 的邻居：h, b。
	if len(hubs[1].Neighbors) != 2 {
		t.Fatalf("a neighbors = %+v", hubs[1].Neighbors)
	}

	// minDegree=3：仅 h 入选。
	top, err := repo.ListHubClusters(ctx, "c1", 3, 10)
	if err != nil || len(top) != 1 || top[0].HubDocID != "h" {
		t.Fatalf("minDegree=3 hubs = %+v, %v", top, err)
	}
	// limit=2 截断：[h, a]。
	cut, err := repo.ListHubClusters(ctx, "c1", 2, 2)
	if err != nil || len(cut) != 2 || cut[0].HubDocID != "h" || cut[1].HubDocID != "a" {
		t.Fatalf("limit=2 hubs = %+v, %v", cut, err)
	}
}

// CountActiveEdgesWithin 契约：两端均在集合内的 active 有向边计数；
// A→B 与 B→A 并存计两条；closed 边不计；空集恒 0。
func TestKnowledgeRepo_CountActiveEdgesWithin(t *testing.T) {
	repo := setupCurateRepo(t)
	ctx := context.Background()

	for _, id := range []string{"h", "a", "b", "c2", "d", "e"} {
		seedCurateDoc(t, repo, id, "entries/"+id+".md")
	}
	seedCurateLink(t, repo, "h", "a", "semantic", "depends-on", 0.9, false)
	seedCurateLink(t, repo, "h", "b", "co_activated", "", 0.6, false)
	seedCurateLink(t, repo, "a", "b", "semantic", "related-to", 0.8, false)
	seedCurateLink(t, repo, "b", "h", "semantic", "related-to", 0.7, false) // 反向并存
	seedCurateLink(t, repo, "h", "c2", "semantic", "related-to", 0.9, true) // closed 不计
	seedCurateLink(t, repo, "d", "e", "semantic", "related-to", 0.8, false)

	cases := []struct {
		name   string
		docIDs []string
		want   int
	}{
		{"tri+reverse", []string{"h", "a", "b"}, 4}, // h→a, h→b, a→b, b→h
		{"pair", []string{"h", "a"}, 1},             // 仅 h→a（b→h 的 b 不在集）
		{"closed excluded", []string{"h", "c2"}, 0},
		{"singleton", []string{"d", "e"}, 1},
		{"empty", nil, 0},
	}
	for _, tc := range cases {
		got, err := repo.CountActiveEdgesWithin(ctx, "c1", tc.docIDs)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
