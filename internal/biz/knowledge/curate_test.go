package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
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
	// P1-b：conflict 处置调用记录。
	closedConflictCollection string
	closedConflictDoc        string
	closedConflictTargets    []string
	closeContradictsErr      error
	// P1-c：stale_at 置位调用记录。
	markStaleIDs  []string
	markStaleErr  error
	markStaleHits int
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

// P1-b：提案详情读取（处置前取 kind/payload/collection + pending 守卫）。
func (s *stubCurateRepo) GetGovernanceProposal(_ context.Context, id int64) (GovernanceProposalView, error) {
	for _, v := range s.proposalViews {
		if v.ID == id {
			return v, nil
		}
	}
	return GovernanceProposalView{}, apierror.NotFound(apierror.DomainKnowledge, "proposal not found")
}

// P1-b：conflict 处置——关闭 active contradicts 边（记录调用参数供断言）。
func (s *stubCurateRepo) CloseContradictsEdges(_ context.Context, collectionID, docID string, targetDocIDs []string) (int, error) {
	s.closedConflictCollection = collectionID
	s.closedConflictDoc = docID
	s.closedConflictTargets = append([]string(nil), targetDocIDs...)
	return len(targetDocIDs), s.closeContradictsErr
}

// P1-c：stale_at 置位（记录调用参数供断言）。
func (s *stubCurateRepo) MarkStaleEntries(_ context.Context, docIDs []string) error {
	s.markStaleHits++
	s.markStaleIDs = append([]string(nil), docIDs...)
	return s.markStaleErr
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
	// P1-c：stale 标记落地文档字段（置位成功才落提案）。
	if curate.markStaleHits != 1 || len(curate.markStaleIDs) != 1 || curate.markStaleIDs[0] != "d-stale" {
		t.Fatalf("stale_at mark = hits %d ids %v, want 1/[d-stale]", curate.markStaleHits, curate.markStaleIDs)
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
	// P1-c：dry_run 不置位 stale_at。
	if curate.markStaleHits != 0 {
		t.Fatalf("dry_run must not mark stale_at: hits %d", curate.markStaleHits)
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
	// P1-c：全部被去重时不发起 stale_at 置位。
	if curate.markStaleHits != 0 {
		t.Fatalf("dedup-skipped stale must not mark stale_at: hits %d", curate.markStaleHits)
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
	// 去重状态集语义：conflict/orphan 含 rejected（人工否决即沉默，防周期骚扰）；
	// stale 含 pending+applied（applied 留痕即已标记，不重复）。
	// P1-b（2026-08-16）：conflict/orphan 去重补 applied——resolve(applied) 现已
	// 执行实际处置（orphan 删词条 / conflict 关 contradicts 边），applied 即终态，
	// 不再周期性重提同一已处置事项（moc_emerge 三态全含同口径）。
	assertStatuses(ProposalKindConflict, ProposalStatusPending, ProposalStatusRejected, ProposalStatusApplied)
	assertStatuses(ProposalKindOrphan, ProposalStatusPending, ProposalStatusRejected, ProposalStatusApplied)
	assertStatuses(ProposalKindStale, ProposalStatusPending, ProposalStatusApplied)
}

// P1-c：stale_at 置位失败 → 不落提案/不计数（无 applied 提案去重不拦截，
// 下轮 dream 自然重试）；其余治理任务不受影响。
func TestCurateKnowledge_StaleMarkFailureSkipsProposal(t *testing.T) {
	curate := &stubCurateRepo{
		stales:       []StaleEntryStat{{DocID: "d-stale", RelPath: "entries/旧.md"}},
		orphans:      []OrphanEntryStat{{DocID: "d-orphan"}},
		markStaleErr: errors.New("db down"),
	}
	proposals := &stubProposalRepo{}
	u := newCurateUsecase(curate, proposals)

	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if curate.markStaleHits != 1 {
		t.Fatalf("mark must be attempted once, got %d", curate.markStaleHits)
	}
	if rep.StaleMarked != 0 {
		t.Fatalf("mark failure must not count stale: %+v", rep)
	}
	// orphan 提案照常（stale 失败不波及其他任务）；stale 提案缺席。
	if len(proposals.items) != 1 || proposals.items[0].Kind != ProposalKindOrphan {
		t.Fatalf("proposals = %+v, want orphan only", proposals.items)
	}
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

// 人工二审闭环：rejected 无需处置直接透传 repo；applied 需提案存在且 pending
// （P1-b 新语义）；非法状态拒绝；未接线报错。
func TestResolveGovernanceProposal(t *testing.T) {
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 42, CollectionID: "c1", Kind: ProposalKindMOCEmerge, Status: ProposalStatusPending,
		Payload: map[string]any{"dedup_key": "moc:hub-x"},
	}}}
	u := newCurateUsecase(curate, nil)
	if err := u.ResolveGovernanceProposal(context.Background(), 42, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if curate.resolvedID != 42 || curate.resolvedStatus != ProposalStatusApplied {
		t.Fatalf("resolved = %d/%s", curate.resolvedID, curate.resolvedStatus)
	}
	// rejected 不取提案、不做处置，直接落库。
	if err := u.ResolveGovernanceProposal(context.Background(), 99, ProposalStatusRejected); err != nil {
		t.Fatal(err)
	}
	if curate.resolvedID != 99 || curate.resolvedStatus != ProposalStatusRejected {
		t.Fatalf("resolved = %d/%s", curate.resolvedID, curate.resolvedStatus)
	}
	// applied 目标不存在 → 报错（不透传）。
	if err := u.ResolveGovernanceProposal(context.Background(), 100, ProposalStatusApplied); err == nil {
		t.Fatal("applied on missing proposal must error")
	}
	if err := u.ResolveGovernanceProposal(context.Background(), 42, "bogus"); err == nil {
		t.Fatal("invalid status must error")
	}
	u2 := NewUsecase(nil, nil, nil)
	if err := u2.ResolveGovernanceProposal(context.Background(), 1, ProposalStatusApplied); err == nil {
		t.Fatal("unwired must error")
	}
}

// ── P1-b：resolve(applied) 实际处置（2026-08-16 根治「applied 语义虚假 +
// 已处置事项周期重提」）：orphan 删词条、conflict 关 contradicts 边，
// 处置成功才落 applied；处置失败报错且提案停留 pending 可重审。

// orphan applied → 删除孤儿词条（DeleteDocument 收 payload.doc_id）后落 applied；
// 文档已不存在（并发已删）视为幂等成功。
func TestResolveGovernanceProposal_OrphanAppliedDeletesDoc(t *testing.T) {
	repo := noOpMockRepo()
	var deleted []string
	repo.docDeleteFn = func(_ context.Context, id string) error {
		deleted = append(deleted, id)
		return nil
	}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 7, CollectionID: "c1", Kind: ProposalKindOrphan, Status: ProposalStatusPending,
		Payload: map[string]any{"dedup_key": "orphan:d-or", "doc_id": "d-or", "rel_path": "entries/孤岛.md"},
	}}}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)

	if err := u.ResolveGovernanceProposal(context.Background(), 7, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "d-or" {
		t.Fatalf("orphan disposal must delete payload doc, got %v", deleted)
	}
	if curate.resolvedID != 7 || curate.resolvedStatus != ProposalStatusApplied {
		t.Fatalf("resolved = %d/%s", curate.resolvedID, curate.resolvedStatus)
	}
}

// conflict applied → 关闭 payload 指明的 contradicts 边（doc_id ↔ target_doc_id）后落 applied。
func TestResolveGovernanceProposal_ConflictAppliedClosesEdges(t *testing.T) {
	repo := noOpMockRepo()
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 9, CollectionID: "c1", Kind: ProposalKindConflict, Status: ProposalStatusPending,
		Payload: map[string]any{
			"dedup_key": "conflict:d1→d2", "doc_id": "d1", "target_doc_id": "d2",
			"context": "发布策略", "confidence": 0.9,
		},
	}}}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)

	if err := u.ResolveGovernanceProposal(context.Background(), 9, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if curate.closedConflictCollection != "c1" || curate.closedConflictDoc != "d1" {
		t.Fatalf("close edges target = %s/%s", curate.closedConflictCollection, curate.closedConflictDoc)
	}
	if len(curate.closedConflictTargets) != 1 || curate.closedConflictTargets[0] != "d2" {
		t.Fatalf("close edge targets = %v", curate.closedConflictTargets)
	}
	if curate.resolvedID != 9 || curate.resolvedStatus != ProposalStatusApplied {
		t.Fatalf("resolved = %d/%s", curate.resolvedID, curate.resolvedStatus)
	}
}

func TestResolveGovernanceProposal_FactConflictCannotSilentlyApply(t *testing.T) {
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 10, CollectionID: "c1", Kind: ProposalKindConflict, Status: ProposalStatusPending,
		Payload: map[string]any{
			"dedup_key": "conflict:fact:d1:fid-old:fid-new",
			"doc_id":    "d1", "target_fact_id": "fid-old", "new_fact_id": "fid-new",
		},
	}}}
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetCurateRepo(curate)

	err := u.ResolveGovernanceProposal(context.Background(), 10, ProposalStatusApplied)
	if err == nil || !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("fact-level conflict must require an explicit resolution action, got %v", err)
	}
	if curate.resolvedID != 0 {
		t.Fatalf("unsupported fact conflict must stay pending, resolvedID = %d", curate.resolvedID)
	}
}

func TestResolveGovernanceProposal_KeepOldRemovesNewFact(t *testing.T) {
	body := "# 灰度发布\n\n## constraint\n\n生产环境禁止自动发布。\n\n- fact_id: `fid-old`\n\n## decision\n\n生产环境允许自动发布\n\n- fact_id: `fid-new`\n"
	repo := noOpMockRepo()
	var updated string
	repo.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "c1", ContentText: body}, nil
	}
	repo.docContentFn = func(_ context.Context, _ string, contentText string, _ bool) error {
		updated = contentText
		return nil
	}
	versions := &stubFactVersionRepo{}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 10, CollectionID: "c1", Kind: ProposalKindConflict, Status: ProposalStatusPending,
		Payload: map[string]any{
			"dedup_key": "conflict:fact:d1:fid-old:fid-new",
			"doc_id":    "d1", "target_fact_id": "fid-old", "new_fact_id": "fid-new",
		},
	}}}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)
	u.SetEvolutionRepos(versions, nil)

	if err := u.ResolveGovernanceProposal(context.Background(), 10, ProposalDecisionKeepOld); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(updated, "允许自动发布") || strings.Contains(updated, "fid-new") {
		t.Fatalf("keep_old must drop the new section: %q", updated)
	}
	if !strings.Contains(updated, "禁止自动发布") || !strings.Contains(updated, "fid-old") {
		t.Fatalf("keep_old must retain the old section: %q", updated)
	}
	if curate.resolvedStatus != ProposalStatusApplied {
		t.Fatalf("keep_old must persist applied, got %s", curate.resolvedStatus)
	}
	if len(versions.items) != 1 || versions.items[0].FactID != "fid-old" {
		t.Fatalf("keep_old must record version chain: %+v", versions.items)
	}
}

func TestResolveGovernanceProposal_KeepNewPreservesLineage(t *testing.T) {
	body := "# 灰度发布\n\n## constraint\n\n生产环境禁止自动发布。\n\n- fact_id: `fid-old`\n\n## decision\n\n生产环境允许自动发布\n\n- fact_id: `fid-new`\n"
	repo := noOpMockRepo()
	var updated string
	repo.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, CollectionID: "c1", ContentText: body}, nil
	}
	repo.docContentFn = func(_ context.Context, _ string, contentText string, _ bool) error {
		updated = contentText
		return nil
	}
	versions := &stubFactVersionRepo{}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 11, CollectionID: "c1", Kind: ProposalKindConflict, Status: ProposalStatusPending,
		Payload: map[string]any{
			"doc_id": "d1", "target_fact_id": "fid-old", "new_fact_id": "fid-new",
		},
	}}}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)
	u.SetEvolutionRepos(versions, nil)

	if err := u.ResolveGovernanceProposal(context.Background(), 11, ProposalDecisionKeepNew); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(updated, "禁止自动发布") {
		t.Fatalf("keep_new must drop the old section: %q", updated)
	}
	if !strings.Contains(updated, "允许自动发布") {
		t.Fatalf("keep_new must retain the new statement: %q", updated)
	}
	if !strings.Contains(updated, "fact_id: `fid-old`") {
		t.Fatalf("keep_new must keep the old fact_id as lineage: %q", updated)
	}
	if !strings.Contains(updated, "source_id: `fid-new`") {
		t.Fatalf("keep_new must keep incoming provenance: %q", updated)
	}
	if curate.resolvedStatus != ProposalStatusApplied {
		t.Fatalf("keep_new must persist applied, got %s", curate.resolvedStatus)
	}
}

func TestResolveGovernanceProposal_KeepOldOnDocConflictRejected(t *testing.T) {
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 12, CollectionID: "c1", Kind: ProposalKindConflict, Status: ProposalStatusPending,
		Payload: map[string]any{"doc_id": "d1", "target_doc_id": "d2"},
	}}}
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetCurateRepo(curate)
	err := u.ResolveGovernanceProposal(context.Background(), 12, ProposalDecisionKeepOld)
	if err == nil || !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("document-level conflict must not accept keep_old, got %v", err)
	}
	if curate.resolvedID != 0 {
		t.Fatalf("invalid keep_old must stay pending, resolvedID = %d", curate.resolvedID)
	}
}

// 处置失败 → 返回错误且提案不落 applied（停留 pending 可重审）；repo resolve 不得调用。
func TestResolveGovernanceProposal_DisposalFailureStaysPending(t *testing.T) {
	repo := noOpMockRepo()
	curate := &stubCurateRepo{
		closeContradictsErr: errors.New("pg down"),
		proposalViews: []GovernanceProposalView{{
			ID: 11, CollectionID: "c1", Kind: ProposalKindConflict, Status: ProposalStatusPending,
			Payload: map[string]any{"dedup_key": "conflict:d1→d2", "doc_id": "d1", "target_doc_id": "d2"},
		}},
	}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)

	if err := u.ResolveGovernanceProposal(context.Background(), 11, ProposalStatusApplied); err == nil {
		t.Fatal("disposal failure must error")
	}
	if curate.resolvedID != 0 {
		t.Fatalf("failed disposal must not resolve proposal, resolvedID = %d", curate.resolvedID)
	}
}

// 非 pending 提案（已审）拒绝重复处置；rejected 不触发任何处置；
// moc_emerge applied 无自动处置（人工建 MOC，设计如此）。
func TestResolveGovernanceProposal_Guards(t *testing.T) {
	repo := noOpMockRepo()
	deleteCalls := 0
	repo.docDeleteFn = func(_ context.Context, _ string) error { deleteCalls++; return nil }
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{
		{ID: 21, CollectionID: "c1", Kind: ProposalKindOrphan, Status: ProposalStatusApplied,
			Payload: map[string]any{"doc_id": "d-old"}},
		{ID: 22, CollectionID: "c1", Kind: ProposalKindOrphan, Status: ProposalStatusPending,
			Payload: map[string]any{"doc_id": "d-rej"}},
		{ID: 23, CollectionID: "c1", Kind: ProposalKindMOCEmerge, Status: ProposalStatusPending,
			Payload: map[string]any{"hub_doc_id": "d-hub"}},
	}}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)

	if err := u.ResolveGovernanceProposal(context.Background(), 21, ProposalStatusApplied); err == nil {
		t.Fatal("non-pending proposal must error (no re-disposal)")
	}
	if deleteCalls != 0 {
		t.Fatalf("non-pending must not dispose, deletes = %d", deleteCalls)
	}
	if err := u.ResolveGovernanceProposal(context.Background(), 22, ProposalStatusRejected); err != nil {
		t.Fatal(err)
	}
	if deleteCalls != 0 {
		t.Fatalf("rejected must not dispose, deletes = %d", deleteCalls)
	}
	if err := u.ResolveGovernanceProposal(context.Background(), 23, ProposalStatusApplied); err != nil {
		t.Fatal(err)
	}
	if deleteCalls != 0 || curate.closedConflictDoc != "" {
		t.Fatal("moc_emerge applied must not auto-dispose")
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
		"d-hot":   {ID: "d-hot", RelPath: "entries/发布策略.md", Summary: "发布策略：灰度先行", SummaryHash: "abc123def456", Tags: []string{"发布"}},
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

// ── M4 补丁（第三轮）：CurateAllTeamKnowledge 多库枚举治理 ─────────────────
// 契约：枚举全部团队库逐库治理（local vault 排除）；无团队库 NotFound；
// 指定 CollectionID 退化单库且不查集合列表；curate 未接线报 Unavailable。

func TestCurateAllTeamKnowledge_MultiCollection(t *testing.T) {
	repo := noOpMockRepo()
	repo.collListFn = func(context.Context, string, int, int) ([]Collection, int, error) {
		return []Collection{
			{ID: "team-1", VaultBackend: VaultBackendTeam},
			{ID: "local-1", VaultBackend: VaultBackendLocal},
			{ID: "team-2", VaultBackend: VaultBackendTeam},
		}, 3, nil
	}
	curate := &stubCurateRepo{decayed: 2}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)

	reps, err := u.CurateAllTeamKnowledge(context.Background(), CurateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reps) != 2 {
		t.Fatalf("reports = %+v, want 2 team collections", reps)
	}
	if reps[0].CollectionID != "team-1" || reps[1].CollectionID != "team-2" {
		t.Fatalf("collection order = %q/%q", reps[0].CollectionID, reps[1].CollectionID)
	}
	for _, rep := range reps {
		if rep.DecayedEdges != 2 {
			t.Fatalf("per-collection decay not applied: %+v", rep)
		}
	}
}

func TestCurateAllTeamKnowledge_NoTeamCollection(t *testing.T) {
	repo := noOpMockRepo()
	repo.collListFn = func(context.Context, string, int, int) ([]Collection, int, error) {
		return []Collection{{ID: "local-1", VaultBackend: VaultBackendLocal}}, 1, nil
	}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(&stubCurateRepo{})

	if _, err := u.CurateAllTeamKnowledge(context.Background(), CurateOptions{}); err == nil {
		t.Fatal("no team collection must error NotFound")
	}
}

func TestCurateAllTeamKnowledge_SpecifiedCollectionDegradesToSingle(t *testing.T) {
	repo := noOpMockRepo()
	repo.collListFn = func(context.Context, string, int, int) ([]Collection, int, error) {
		t.Error("ListCollections must not be called when CollectionID specified")
		return nil, 0, nil
	}
	curate := &stubCurateRepo{decayed: 1}
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)

	reps, err := u.CurateAllTeamKnowledge(context.Background(), CurateOptions{CollectionID: "c-x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(reps) != 1 || reps[0].CollectionID != "c-x" {
		t.Fatalf("single degrade = %+v", reps)
	}
}

func TestCurateAllTeamKnowledge_Unwired(t *testing.T) {
	u := NewUsecase(nil, nil, nil)
	if _, err := u.CurateAllTeamKnowledge(context.Background(), CurateOptions{}); err == nil {
		t.Fatal("unwired curate repo must error")
	}
}
