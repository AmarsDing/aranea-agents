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
