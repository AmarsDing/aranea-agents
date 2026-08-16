package knowledge

import (
	"context"
	"errors"
	"testing"
)

// ── 自治理知识图谱 M4 自治理层（词条治理编排） ─────────────────────────────
// 契约：低风险自动应用（decay/promote/stale-applied），高风险仅 pending 提案；
// dry_run 只探测不写入；同 dedup_key 提案去重防周期风暴；
// 提案 repo 未接线降级只报计数；curate repo 未接线显式报不可用。

type stubCurateRepo struct {
	decayed, closed int
	promotable      []string
	promoted        []string
	stales          []StaleEntryStat
	orphans         []OrphanEntryStat
	conflicts       []ContradictsEdgeStat
	hubs            []HubClusterStat
	edgesWithin     int
	hasProposal     bool
	dryRunSeen      bool
	resolvedID      int64
	resolvedStatus  string
	proposalViews   []GovernanceProposalView
	// dedupStatuses 记录每次 HasProposal 调用的 kind→statuses（拒绝即沉默语义断言）。
	dedupStatuses map[string][]string
}

func (s *stubCurateRepo) DecayCoActivatedEdges(_ context.Context, _ string, _, _ float64, dryRun bool) (int, int, error) {
	s.dryRunSeen = dryRun
	return s.decayed, s.closed, nil
}

func (s *stubCurateRepo) ListPromotableRelations(context.Context, int) ([]string, error) {
	return s.promotable, nil
}

func (s *stubCurateRepo) PromoteRelation(_ context.Context, relation string) error {
	s.promoted = append(s.promoted, relation)
	return nil
}

func (s *stubCurateRepo) ListStaleEntries(context.Context, string, int, int) ([]StaleEntryStat, error) {
	return s.stales, nil
}

func (s *stubCurateRepo) ListOrphanEntries(context.Context, string, int, int) ([]OrphanEntryStat, error) {
	return s.orphans, nil
}

func (s *stubCurateRepo) ListContradictsEdges(context.Context, string, int) ([]ContradictsEdgeStat, error) {
	return s.conflicts, nil
}

func (s *stubCurateRepo) ListHubClusters(context.Context, string, int, int) ([]HubClusterStat, error) {
	return s.hubs, nil
}

func (s *stubCurateRepo) CountActiveEdgesWithin(context.Context, string, []string) (int, error) {
	return s.edgesWithin, nil
}

func (s *stubCurateRepo) HasProposal(_ context.Context, _, kind, _ string, statuses []string) (bool, error) {
	if s.dedupStatuses == nil {
		s.dedupStatuses = map[string][]string{}
	}
	s.dedupStatuses[kind] = statuses
	return s.hasProposal, nil
}

func (s *stubCurateRepo) ResolveGovernanceProposal(_ context.Context, id int64, status string) error {
	s.resolvedID, s.resolvedStatus = id, status
	return nil
}

func (s *stubCurateRepo) ListGovernanceProposals(context.Context, string, string, int) ([]GovernanceProposalView, error) {
	return s.proposalViews, nil
}

func newCurateUsecase(curate KnowledgeCurateRepo, proposals GovernanceProposalRepo) *Usecase {
	u := NewUsecase(nil, nil, nil)
	u.SetCurateRepo(curate)
	if proposals != nil {
		u.SetEvolutionRepos(nil, proposals)
	}
	return u
}

// 未接线 curate repo → 显式报不可用（非静默 no-op）。
func TestCurateKnowledge_RepoNotWired(t *testing.T) {
	u := NewUsecase(nil, nil, nil)
	if _, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"}); err == nil {
		t.Fatal("unwired curate repo must error")
	}
}

// 全自动路径：五类任务全产出——decay 计数、promote 提升、stale applied 提案、
// conflict/orphan pending 提案；dry_run=false 落库。
func TestCurateKnowledge_FullRound(t *testing.T) {
	curate := &stubCurateRepo{
		decayed: 7, closed: 2,
		promotable: []string{"owned-by"},
		stales:     []StaleEntryStat{{DocID: "d-stale", RelPath: "entries/旧词条.md", LastAccessDays: 45, ClosedRatio: 0.8}},
		orphans:    []OrphanEntryStat{{DocID: "d-orphan", RelPath: "entries/孤岛.md", LastAccessDays: 60}},
		conflicts:  []ContradictsEdgeStat{{DocID: "d1", TargetDocID: "d2", Context: "发布策略", Confidence: 0.9}},
	}
	proposals := &stubProposalRepo{}
	u := newCurateUsecase(curate, proposals)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DecayedEdges != 7 || rep.ClosedEdges != 2 {
		t.Fatalf("decay = %d/%d", rep.DecayedEdges, rep.ClosedEdges)
	}
	if curate.dryRunSeen {
		t.Fatal("non-dry-run must pass dryRun=false to decay")
	}
	if len(rep.PromotedRelations) != 1 || rep.PromotedRelations[0] != "owned-by" {
		t.Fatalf("promoted = %+v", rep.PromotedRelations)
	}
	if len(curate.promoted) != 1 {
		t.Fatalf("PromoteRelation must be called: %+v", curate.promoted)
	}
	if rep.StaleMarked != 1 || rep.ProposalsPending != 2 {
		t.Fatalf("stale=%d pending=%d, want 1/2", rep.StaleMarked, rep.ProposalsPending)
	}
	// 提案落库：stale applied 低风险 + conflict/orphan pending 高风险。
	if len(proposals.items) != 3 {
		t.Fatalf("proposals = %+v, want 3", proposals.items)
	}
	byKind := map[string]GovernanceProposal{}
	for _, p := range proposals.items {
		byKind[p.Kind] = p
	}
	stale := byKind[ProposalKindStale]
	if stale.Risk != ProposalRiskLow || stale.Status != ProposalStatusApplied {
		t.Fatalf("stale proposal = %+v, want low/applied", stale)
	}
	if stale.Payload["doc_id"] != "d-stale" || stale.Payload["dedup_key"] != "stale:d-stale" {
		t.Fatalf("stale payload = %+v", stale.Payload)
	}
	conflict := byKind[ProposalKindConflict]
	if conflict.Risk != ProposalRiskHigh || conflict.Status != ProposalStatusPending {
		t.Fatalf("conflict proposal = %+v, want high/pending", conflict)
	}
	orphan := byKind[ProposalKindOrphan]
	if orphan.Risk != ProposalRiskHigh || orphan.Status != ProposalStatusPending {
		t.Fatalf("orphan proposal = %+v, want high/pending", orphan)
	}
}

// dry_run：计数照报，但不 promote、不落提案。
func TestCurateKnowledge_DryRunWritesNothing(t *testing.T) {
	curate := &stubCurateRepo{
		decayed: 5, closed: 1,
		promotable: []string{"owned-by"},
		stales:     []StaleEntryStat{{DocID: "d-stale"}},
		orphans:    []OrphanEntryStat{{DocID: "d-orphan"}},
	}
	proposals := &stubProposalRepo{}
	u := newCurateUsecase(curate, proposals)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !curate.dryRunSeen {
		t.Fatal("dry_run must propagate to decay estimate")
	}
	if rep.DecayedEdges != 5 || rep.StaleMarked != 1 || rep.ProposalsPending != 1 {
		t.Fatalf("dry-run report = %+v", rep)
	}
	if len(curate.promoted) != 0 {
		t.Fatalf("dry_run must not promote: %+v", curate.promoted)
	}
	if len(proposals.items) != 0 {
		t.Fatalf("dry_run must not insert proposals: %+v", proposals.items)
	}
	// 预估清单仍透出（供报告）。
	if len(rep.PromotedRelations) != 1 || len(rep.Proposals) != 2 {
		t.Fatalf("dry-run preview lists = %+v", rep)
	}
}

// 提案去重：同 dedup_key 已有 pending/applied → 跳过（防周期风暴）。
func TestCurateKnowledge_ProposalDedup(t *testing.T) {
	curate := &stubCurateRepo{
		hasProposal: true,
		stales:      []StaleEntryStat{{DocID: "d-stale"}},
		orphans:     []OrphanEntryStat{{DocID: "d-orphan"}},
		conflicts:   []ContradictsEdgeStat{{DocID: "d1", TargetDocID: "d2"}},
	}
	proposals := &stubProposalRepo{}
	u := newCurateUsecase(curate, proposals)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.StaleMarked != 0 || rep.ProposalsPending != 0 {
		t.Fatalf("dedup must skip all proposals: %+v", rep)
	}
	if len(proposals.items) != 0 {
		t.Fatalf("dedup must not insert: %+v", proposals.items)
	}
	// 去重状态集语义：conflict/orphan 含 rejected（人工否决即沉默，防周期骚扰）；
	// stale 含 pending+applied（applied 留痕即已标记，不重复）。
	assertStatuses := func(kind string, want ...string) {
		t.Helper()
		got := curate.dedupStatuses[kind]
		if len(got) != len(want) {
			t.Fatalf("%s dedup statuses = %v, want %v", kind, got, want)
		}
		seen := map[string]bool{}
		for _, s := range got {
			seen[s] = true
		}
		for _, w := range want {
			if !seen[w] {
				t.Fatalf("%s dedup statuses = %v, missing %q", kind, got, w)
			}
		}
	}
	assertStatuses(ProposalKindConflict, ProposalStatusPending, ProposalStatusRejected)
	assertStatuses(ProposalKindOrphan, ProposalStatusPending, ProposalStatusRejected)
	assertStatuses(ProposalKindStale, ProposalStatusPending, ProposalStatusApplied)
}

// 提案 repo 未接线 → 计数照报、不 panic（降级安全）。
func TestCurateKnowledge_ProposalRepoNotWired(t *testing.T) {
	curate := &stubCurateRepo{
		orphans: []OrphanEntryStat{{DocID: "d-orphan"}},
	}
	u := newCurateUsecase(curate, nil)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.ProposalsPending != 1 {
		t.Fatalf("report = %+v", rep)
	}
}

// 空集合 ID → 经 LookupWriteBackHome 解析写回落点：无 team 库报错；有 team 库自动选中。
func TestCurateKnowledge_ResolveHomeFailed(t *testing.T) {
	repo := noOpMockRepo()
	repo.collListFn = func(context.Context, string, int, int) ([]Collection, int, error) {
		return nil, 0, nil
	}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(&stubCurateRepo{})
	if _, err := u.CurateKnowledge(context.Background(), CurateOptions{}); err == nil {
		t.Fatal("no team collection must error")
	}
}

func TestCurateKnowledge_ResolveHomePicksTeamCollection(t *testing.T) {
	repo := noOpMockRepo()
	repo.collListFn = func(context.Context, string, int, int) ([]Collection, int, error) {
		return []Collection{{ID: "team-1", Name: WriteBackCollectionName, VaultBackend: VaultBackendTeam}}, 1, nil
	}
	curate := &stubCurateRepo{decayed: 3}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)
	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.CollectionID != "team-1" {
		t.Fatalf("collection = %q, want team-1", rep.CollectionID)
	}
	if rep.DecayedEdges != 3 {
		t.Fatalf("decayed = %d", rep.DecayedEdges)
	}
}

// 人工二审闭环：合法状态透传 repo；非法状态拒绝；未接线报错。
func TestResolveGovernanceProposal(t *testing.T) {
	curate := &stubCurateRepo{}
	u := newCurateUsecase(curate, nil)
	if err := u.ResolveGovernanceProposal(context.Background(), 42, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if curate.resolvedID != 42 || curate.resolvedStatus != ProposalStatusApplied {
		t.Fatalf("resolved = %d/%s", curate.resolvedID, curate.resolvedStatus)
	}
	if err := u.ResolveGovernanceProposal(context.Background(), 42, "bogus"); err == nil {
		t.Fatal("invalid status must error")
	}
	u2 := NewUsecase(nil, nil, nil)
	if err := u2.ResolveGovernanceProposal(context.Background(), 1, ProposalStatusApplied); err == nil {
		t.Fatal("unwired must error")
	}
}

// 二审列表出口：状态白名单校验；未接线报错；合法调用透传。
func TestListGovernanceProposals(t *testing.T) {
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{{ID: 1, Kind: ProposalKindOrphan}}}
	u := newCurateUsecase(curate, nil)
	got, err := u.ListGovernanceProposals(context.Background(), "", ProposalStatusPending, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("views = %+v", got)
	}
	if _, err := u.ListGovernanceProposals(context.Background(), "", "bogus", 0); err == nil {
		t.Fatal("invalid status must error")
	}
	u2 := NewUsecase(nil, nil, nil)
	if _, err := u2.ListGovernanceProposals(context.Background(), "", "", 0); err == nil {
		t.Fatal("unwired must error")
	}
}

// 单任务失败降级：decay 报错不中断后续 promote/orphan 任务。
type failDecayCurateRepo struct{ stubCurateRepo }

func (s *failDecayCurateRepo) DecayCoActivatedEdges(context.Context, string, float64, float64, bool) (int, int, error) {
	return 0, 0, errors.New("pg down")
}

func TestCurateKnowledge_TaskFailureDegrades(t *testing.T) {
	curate := &failDecayCurateRepo{stubCurateRepo{
		orphans: []OrphanEntryStat{{DocID: "d-orphan"}},
	}}
	proposals := &stubProposalRepo{}
	u := newCurateUsecase(curate, proposals)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.ProposalsPending != 1 {
		t.Fatalf("decay failure must not break orphan scan: %+v", rep)
	}
	if len(proposals.items) != 1 {
		t.Fatalf("proposals = %+v", proposals.items)
	}
}

// ── M4 distill：高频词条反向蒸馏 memory_fact ─────────────────────────────

type stubHotDocs struct{ ids []string }

func (s *stubHotDocs) ListHotDocuments(context.Context, string, int, int, int) ([]string, error) {
	return s.ids, nil
}

type stubDistillWriter struct{ facts []DistilledFact }

func (s *stubDistillWriter) UpsertDistilledFact(_ context.Context, in DistilledFact) error {
	s.facts = append(s.facts, in)
	return nil
}

// newDistillUsecase 装配 distill 全端口的最小 Usecase：team 集合（workspace=ws-1）
// + 词条文档表 + 治理端口 + distill 端口。
func newDistillUsecase(curate KnowledgeCurateRepo, proposals GovernanceProposalRepo, docs map[string]Document, writer DistillFactWriter) *Usecase {
	repo := noOpMockRepo()
	repo.collGetFn = func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, VaultBackend: VaultBackendTeam, Workspace: "ws-1"}, nil
	}
	repo.docGetFn = func(_ context.Context, id string) (Document, error) {
		d, ok := docs[id]
		if !ok {
			return Document{}, errors.New("doc not found")
		}
		return d, nil
	}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)
	if proposals != nil {
		u.SetEvolutionRepos(nil, proposals)
	}
	u.SetDistillRepos(&stubHotDocs{ids: keysOfDocs(docs)}, writer)
	return u
}

func keysOfDocs(docs map[string]Document) []string {
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	return ids
}

// distill 全路径：entries 词条蒸馏进 workspace scope 事实；非 entries / 空摘要跳过；
// applied 提案留痕；fingerprint 幂等键携带 docID。
func TestCurateKnowledge_Distill(t *testing.T) {
	docs := map[string]Document{
		"d-hot":  {ID: "d-hot", RelPath: "entries/发布策略.md", Summary: "发布策略：灰度先行", SummaryHash: "abc123def456", Tags: []string{"发布"}},
		"d-diary": {ID: "d-diary", RelPath: "inbox/writeback-2026.md", Summary: "日记不进蒸馏"},
		"d-nosum": {ID: "d-nosum", RelPath: "entries/无摘要.md"},
	}
	curate := &stubCurateRepo{}
	proposals := &stubProposalRepo{}
	writer := &stubDistillWriter{}
	u := newDistillUsecase(curate, proposals, docs, writer)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "team-1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DistilledFacts != 1 {
		t.Fatalf("distilled = %d, want 1 (only entries with summary)", rep.DistilledFacts)
	}
	if len(writer.facts) != 1 {
		t.Fatalf("facts = %+v", writer.facts)
	}
	f := writer.facts[0]
	if f.ScopeType != "workspace" || f.ScopeID != "ws-1" {
		t.Fatalf("fact scope = %s/%s, want workspace/ws-1", f.ScopeType, f.ScopeID)
	}
	if f.Statement != "发布策略：灰度先行" || f.Fingerprint != "kdistill:d-hot" {
		t.Fatalf("fact = %+v", f)
	}
	if f.TagsJSON != `["发布"]` || f.SourceDocID != "d-hot" || f.SourcePath != "entries/发布策略.md" {
		t.Fatalf("fact provenance = %+v", f)
	}
	// applied 提案留痕（低风险自动应用），dedup_key 带摘要 hash 前缀。
	if len(proposals.items) != 1 {
		t.Fatalf("proposals = %+v", proposals.items)
	}
	p := proposals.items[0]
	if p.Kind != ProposalKindDistill || p.Risk != ProposalRiskLow || p.Status != ProposalStatusApplied {
		t.Fatalf("distill proposal = %+v", p)
	}
	if p.Payload["dedup_key"] != "distill:d-hot:abc123def456" {
		t.Fatalf("dedup_key = %v", p.Payload["dedup_key"])
	}
	// 去重状态集语义：distill 只看 applied（摘要变更走新 dedup_key 自然重蒸馏）。
	got := curate.dedupStatuses[ProposalKindDistill]
	if len(got) != 1 || got[0] != ProposalStatusApplied {
		t.Fatalf("distill dedup statuses = %v, want [applied]", got)
	}
}

// distill 去重：同 docID+摘要 hash 已有 applied 提案 → 跳过（周期幂等）。
func TestCurateKnowledge_DistillDedup(t *testing.T) {
	docs := map[string]Document{
		"d-hot": {ID: "d-hot", RelPath: "entries/发布策略.md", Summary: "s", SummaryHash: "h1"},
	}
	curate := &stubCurateRepo{hasProposal: true}
	proposals := &stubProposalRepo{}
	writer := &stubDistillWriter{}
	u := newDistillUsecase(curate, proposals, docs, writer)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "team-1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DistilledFacts != 0 || len(writer.facts) != 0 || len(proposals.items) != 0 {
		t.Fatalf("dedup must skip: rep=%+v facts=%+v proposals=%+v", rep, writer.facts, proposals.items)
	}
}

// distill dry_run：计数照报，不写事实、不落提案。
func TestCurateKnowledge_DistillDryRun(t *testing.T) {
	docs := map[string]Document{
		"d-hot": {ID: "d-hot", RelPath: "entries/发布策略.md", Summary: "s", SummaryHash: "h1"},
	}
	curate := &stubCurateRepo{}
	proposals := &stubProposalRepo{}
	writer := &stubDistillWriter{}
	u := newDistillUsecase(curate, proposals, docs, writer)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "team-1", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DistilledFacts != 1 {
		t.Fatalf("dry-run preview distilled = %d, want 1", rep.DistilledFacts)
	}
	if len(writer.facts) != 0 || len(proposals.items) != 0 {
		t.Fatalf("dry_run must not write: facts=%+v proposals=%+v", writer.facts, proposals.items)
	}
}

// distill 端口未接线 → 静默跳过（其余治理任务照常）。
func TestCurateKnowledge_DistillUnwired(t *testing.T) {
	curate := &stubCurateRepo{decayed: 2}
	u := newCurateUsecase(curate, nil)
	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DistilledFacts != 0 {
		t.Fatalf("unwired distill must skip: %+v", rep)
	}
	if rep.DecayedEdges != 2 {
		t.Fatalf("other tasks must run: %+v", rep)
	}
}

// 非团队库（local vault）→ distill 跳过（事实注入链不覆盖个人库）。
func TestCurateKnowledge_DistillSkipsLocalVault(t *testing.T) {
	docs := map[string]Document{
		"d-hot": {ID: "d-hot", RelPath: "entries/x.md", Summary: "s", SummaryHash: "h1"},
	}
	repo := noOpMockRepo()
	repo.collGetFn = func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, VaultBackend: VaultBackendLocal, Workspace: "ws-1"}, nil
	}
	repo.docGetFn = func(_ context.Context, id string) (Document, error) { return docs[id], nil }
	writer := &stubDistillWriter{}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(&stubCurateRepo{})
	u.SetDistillRepos(&stubHotDocs{ids: []string{"d-hot"}}, writer)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "local-1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DistilledFacts != 0 || len(writer.facts) != 0 {
		t.Fatalf("local vault must not distill: %+v", rep)
	}
}

// ── M4 moc_emerge：hub 簇蒸馏 MOC 提案 ──────────────────────────────────

// moc_emerge 全路径：规模+密度双达阈值 → 高风险 pending 提案，payload 含成员
// 清单与建议路径；密度不足/规模不足不出提案。
func TestCurateKnowledge_MOCEmerge(t *testing.T) {
	hub := HubClusterStat{
		HubDocID: "d-hub", HubRelPath: "entries/发布体系.md", Degree: 5,
		Neighbors: []HubMember{
			{DocID: "d1", RelPath: "entries/灰度.md"},
			{DocID: "d2", RelPath: "entries/回滚.md"},
			{DocID: "d3", RelPath: "entries/监控.md"},
			{DocID: "d4", RelPath: "entries/告警.md"},
		},
	}
	curate := &stubCurateRepo{hubs: []HubClusterStat{hub}, edgesWithin: 7}
	proposals := &stubProposalRepo{}
	u := newCurateUsecase(curate, proposals)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	// 5 节点完全图 10 边，实际 7 → density 0.7 ≥ 0.3。
	if rep.HubsScanned != 1 || rep.ProposalsPending != 1 {
		t.Fatalf("rep = %+v", rep)
	}
	if len(proposals.items) != 1 {
		t.Fatalf("proposals = %+v", proposals.items)
	}
	p := proposals.items[0]
	if p.Kind != ProposalKindMOCEmerge || p.Risk != ProposalRiskHigh || p.Status != ProposalStatusPending {
		t.Fatalf("moc proposal = %+v", p)
	}
	if p.Payload["dedup_key"] != "moc:d-hub" || p.Payload["suggested_title"] != "发布体系" {
		t.Fatalf("payload = %+v", p.Payload)
	}
	if p.Payload["suggested_path"] != "moc/发布体系.md" {
		t.Fatalf("suggested_path = %v", p.Payload["suggested_path"])
	}
	if d, ok := p.Payload["density"].(float64); !ok || d < 0.69 || d > 0.71 {
		t.Fatalf("density = %v", p.Payload["density"])
	}
	members, ok := p.Payload["members"].([]string)
	if !ok || len(members) != 4 {
		t.Fatalf("members = %v", p.Payload["members"])
	}
	// 去重状态集语义：三态全含（已建 MOC 或人工否决的同 hub 不再提案）。
	got := curate.dedupStatuses[ProposalKindMOCEmerge]
	seen := map[string]bool{}
	for _, s := range got {
		seen[s] = true
	}
	for _, w := range []string{ProposalStatusPending, ProposalStatusApplied, ProposalStatusRejected} {
		if !seen[w] {
			t.Fatalf("moc dedup statuses = %v, missing %q", got, w)
		}
	}
}

// moc_emerge 门槛：密度不足与规模不足均不出提案。
func TestCurateKnowledge_MOCEmergeThresholds(t *testing.T) {
	sparse := HubClusterStat{
		HubDocID: "d-sparse", HubRelPath: "entries/稀疏.md", Degree: 5,
		Neighbors: []HubMember{
			{DocID: "d1", RelPath: "entries/a.md"},
			{DocID: "d2", RelPath: "entries/b.md"},
			{DocID: "d3", RelPath: "entries/c.md"},
			{DocID: "d4", RelPath: "entries/d.md"},
		},
	}
	small := HubClusterStat{
		HubDocID: "d-small", HubRelPath: "entries/小簇.md", Degree: 5,
		Neighbors: []HubMember{
			{DocID: "d1", RelPath: "entries/a.md"},
			{DocID: "d2", RelPath: "entries/b.md"},
		},
	}
	// sparse：5 节点 2 边 → density 0.2 < 0.3；small：3 节点 < 最小规模 4。
	curate := &stubCurateRepo{hubs: []HubClusterStat{sparse, small}, edgesWithin: 2}
	proposals := &stubProposalRepo{}
	u := newCurateUsecase(curate, proposals)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.HubsScanned != 2 || rep.ProposalsPending != 0 || len(proposals.items) != 0 {
		t.Fatalf("thresholds must block: rep=%+v proposals=%+v", rep, proposals.items)
	}
}

// moc_emerge 去重：同 hub 已有提案（任一状态）→ 跳过。
func TestCurateKnowledge_MOCDedup(t *testing.T) {
	hub := HubClusterStat{
		HubDocID: "d-hub", HubRelPath: "entries/发布体系.md", Degree: 5,
		Neighbors: []HubMember{
			{DocID: "d1", RelPath: "entries/a.md"},
			{DocID: "d2", RelPath: "entries/b.md"},
			{DocID: "d3", RelPath: "entries/c.md"},
		},
	}
	curate := &stubCurateRepo{hubs: []HubClusterStat{hub}, edgesWithin: 6, hasProposal: true}
	proposals := &stubProposalRepo{}
	u := newCurateUsecase(curate, proposals)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.ProposalsPending != 0 || len(proposals.items) != 0 {
		t.Fatalf("dedup must skip: rep=%+v proposals=%+v", rep, proposals.items)
	}
}
