package service

import (
	"context"
	"strings"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/knowledge"
	internalknowledge "aranea-agents/internal/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── G5-F 实体治理：service 层 stub（EntityRepo / Embedder） ─────────────────

type stubGovEntityRepo struct {
	mergeResult knowledge.EntityMergeResult
	mergeErr    error
	entities    []knowledge.Entity
	// 记录调用参数供断言。
	gotKeeper  int64
	gotMergees []int64
}

func (s *stubGovEntityRepo) ReplaceDocEntities(context.Context, string, string, []knowledge.DocEntity) ([]int64, error) {
	return nil, nil
}

func (s *stubGovEntityRepo) FindEntityCooccurrences(context.Context, string, []int64, string, int) ([]knowledge.EntityCooccurrence, error) {
	return nil, nil
}

func (s *stubGovEntityRepo) MergeEntities(_ context.Context, _ string, keeperID int64, mergeeIDs []int64) (knowledge.EntityMergeResult, error) {
	s.gotKeeper, s.gotMergees = keeperID, mergeeIDs
	return s.mergeResult, s.mergeErr
}

func (s *stubGovEntityRepo) ListEntities(context.Context, string) ([]knowledge.Entity, error) {
	return s.entities, nil
}

// stubGovEmbedder 满足 internal/knowledge.Embedder（Embed + Dim）。
type stubGovEmbedder struct {
	calls int
}

func (s *stubGovEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	s.calls++
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0} // 全同向 → 余弦 1.0（auto 对）
	}
	return out, nil
}

func (s *stubGovEmbedder) Dim() int { return 2 }

func newGovernanceService(t *testing.T, entities *stubGovEntityRepo, embedder internalknowledge.Embedder) (*KnowledgeService, *us14MemRepo) {
	t.Helper()
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	uc.SetLinkRepos(nil, entities)
	return NewKnowledgeService(uc, embedder, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop()), repo
}

// ── MergeKnowledgeEntities ──────────────────────────────────────────────────

func TestKnowledgeService_MergeKnowledgeEntities(t *testing.T) {
	entities := &stubGovEntityRepo{mergeResult: knowledge.EntityMergeResult{
		RewrittenMentions: 3, RewrittenLinks: 1, MergedEntities: 2,
	}}
	svc, repo := newGovernanceService(t, entities, nil)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}

	resp, err := svc.MergeKnowledgeEntities(context.Background(), &v1.MergeKnowledgeEntitiesRequest{
		CollectionId: "c1", KeeperId: 7, MergeeIds: []int64{8, 9},
	})
	if err != nil {
		t.Fatalf("MergeKnowledgeEntities: %v", err)
	}
	if resp.GetRewrittenMentions() != 3 || resp.GetRewrittenLinks() != 1 || resp.GetMergedEntities() != 2 {
		t.Errorf("response = %+v, want mentions=3 links=1 merged=2", resp)
	}
	if entities.gotKeeper != 7 || len(entities.gotMergees) != 2 {
		t.Errorf("repo got keeper=%d mergees=%v, want 7/[8 9]", entities.gotKeeper, entities.gotMergees)
	}
}

func TestKnowledgeService_MergeKnowledgeEntities_Validation(t *testing.T) {
	svc, repo := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MergeKnowledgeEntities(context.Background(), &v1.MergeKnowledgeEntitiesRequest{
		CollectionId: "c1", MergeeIds: []int64{8},
	}); err == nil || !strings.Contains(err.Error(), "keeper_id") {
		t.Errorf("missing keeper_id must be BadRequest, got %v", err)
	}
	if _, err := svc.MergeKnowledgeEntities(context.Background(), &v1.MergeKnowledgeEntitiesRequest{
		CollectionId: "c1", KeeperId: 7,
	}); err == nil || !strings.Contains(err.Error(), "mergee_ids") {
		t.Errorf("empty mergee_ids must be BadRequest, got %v", err)
	}
}

func TestKnowledgeService_MergeKnowledgeEntities_CrossTenantDenied(t *testing.T) {
	svc, repo := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", Workspace: "ws-other"}); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithContext(context.Background(), "ws-mine")
	_, err := svc.MergeKnowledgeEntities(ctx, &v1.MergeKnowledgeEntitiesRequest{
		CollectionId: "c1", KeeperId: 7, MergeeIds: []int64{8},
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("cross-tenant merge must be NotFound, got %v", err)
	}
}

// ── ListEntityMergeSuggestions ──────────────────────────────────────────────

func TestKnowledgeService_ListEntityMergeSuggestions(t *testing.T) {
	entities := &stubGovEntityRepo{entities: []knowledge.Entity{
		{ID: 1, Name: "RAG", NameNorm: "rag"},
		{ID: 2, Name: "rag", NameNorm: "rag"}, // norm 冲突组
		{ID: 3, Name: "财报", NameNorm: "财报"},
	}}
	embedder := &stubGovEmbedder{}
	svc, repo := newGovernanceService(t, entities, embedder)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}

	resp, err := svc.ListEntityMergeSuggestions(context.Background(), &v1.ListEntityMergeSuggestionsRequest{CollectionId: "c1"})
	if err != nil {
		t.Fatalf("ListEntityMergeSuggestions: %v", err)
	}
	if embedder.calls != 1 {
		t.Errorf("embedder calls = %d, want 1", embedder.calls)
	}
	// norm 组 1 条 + embedding 高相似对（1↔3、2↔3 余弦 1.0，1↔2 已被 norm 覆盖）。
	if len(resp.GetItems()) != 3 {
		t.Fatalf("items = %+v, want 3", resp.GetItems())
	}
	first := resp.GetItems()[0]
	if first.GetSource() != "norm" || first.GetKeeperId() != 1 || first.GetMergeeId() != 2 ||
		first.GetTier() != "auto" || first.GetSimilarity() != 1.0 ||
		first.GetKeeperName() != "RAG" || first.GetMergeeName() != "rag" {
		t.Errorf("norm suggestion mapping wrong: %+v", first)
	}
	for _, it := range resp.GetItems()[1:] {
		if it.GetSource() != "embedding" || it.GetTier() != "auto" {
			t.Errorf("embedding suggestion wrong: %+v", it)
		}
	}
}

// embedder 未配置（nil）时降级为仅 norm 组（NFR-15），不报错。
func TestKnowledgeService_ListEntityMergeSuggestions_NilEmbedder(t *testing.T) {
	entities := &stubGovEntityRepo{entities: []knowledge.Entity{
		{ID: 1, Name: "RAG", NameNorm: "rag"},
		{ID: 2, Name: "rag", NameNorm: "rag"},
	}}
	svc, repo := newGovernanceService(t, entities, nil)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.ListEntityMergeSuggestions(context.Background(), &v1.ListEntityMergeSuggestionsRequest{CollectionId: "c1"})
	if err != nil {
		t.Fatalf("nil embedder must degrade, got %v", err)
	}
	if len(resp.GetItems()) != 1 || resp.GetItems()[0].GetSource() != "norm" {
		t.Errorf("items = %+v, want norm-only 1", resp.GetItems())
	}
}

func TestKnowledgeService_ListEntityMergeSuggestions_CrossTenantDenied(t *testing.T) {
	svc, repo := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", Workspace: "ws-other"}); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithContext(context.Background(), "ws-mine")
	_, err := svc.ListEntityMergeSuggestions(ctx, &v1.ListEntityMergeSuggestionsRequest{CollectionId: "c1"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("cross-tenant suggestions must be NotFound, got %v", err)
	}
}

// ── M4 治理提案人工二审 RPC ────────────────────────────────────────────────

// stubGovCurateRepo 最小 KnowledgeCurateRepo（仅提案列表/解析两方法有实装）。
type stubGovCurateRepo struct {
	knowledge.KnowledgeCurateRepo // 嵌入 nil 接口，未覆盖方法不应被调到
	views                         []knowledge.GovernanceProposalView
	resolvedID                    int64
	resolvedStatus                string
}

func (s *stubGovCurateRepo) ListGovernanceProposals(context.Context, string, string, int) ([]knowledge.GovernanceProposalView, error) {
	return s.views, nil
}

func (s *stubGovCurateRepo) ResolveGovernanceProposal(_ context.Context, id int64, status string) error {
	s.resolvedID, s.resolvedStatus = id, status
	return nil
}

// P1-b：applied 前置读取 kind/payload + pending 守卫。
func (s *stubGovCurateRepo) GetGovernanceProposal(_ context.Context, id int64) (knowledge.GovernanceProposalView, error) {
	for _, v := range s.views {
		if v.ID == id {
			return v, nil
		}
	}
	return knowledge.GovernanceProposalView{}, apierror.NotFound(apierror.DomainKnowledge, "proposal not found")
}

func TestKnowledgeService_ListGovernanceProposals(t *testing.T) {
	curate := &stubGovCurateRepo{views: []knowledge.GovernanceProposalView{
		{ID: 7, CollectionID: "c1", Kind: knowledge.ProposalKindConflict, Risk: knowledge.ProposalRiskHigh,
			Status: knowledge.ProposalStatusPending, Payload: map[string]any{"dedup_key": "conflict:a→b"},
			CreatedAt: time.Date(2026, 8, 16, 1, 0, 0, 0, time.UTC)},
		{ID: 3, CollectionID: "c1", Kind: knowledge.ProposalKindStale, Risk: knowledge.ProposalRiskLow,
			Status: knowledge.ProposalStatusApplied, Payload: map[string]any{"dedup_key": "stale:d2"},
			CreatedAt:  time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC),
			ResolvedAt: time.Date(2026, 8, 15, 2, 0, 0, 0, time.UTC)},
	}}
	svc, repo := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	svc.uc.SetCurateRepo(curate)
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}

	// 指定 collection_id（走访问校验）+ 全量（空 collection_id）两路径。
	for _, colID := range []string{"c1", ""} {
		resp, err := svc.ListGovernanceProposals(context.Background(), &v1.ListGovernanceProposalsRequest{CollectionId: colID})
		if err != nil {
			t.Fatalf("ListGovernanceProposals(%q): %v", colID, err)
		}
		if len(resp.GetItems()) != 2 {
			t.Fatalf("items = %+v, want 2", resp.GetItems())
		}
		pending := resp.GetItems()[0]
		if pending.GetId() != 7 || pending.GetKind() != "conflict" || pending.GetRisk() != "high" ||
			pending.GetStatus() != "pending" || pending.GetResolvedAt() != "" ||
			!strings.Contains(pending.GetPayloadJson(), "conflict:a→b") {
			t.Errorf("pending projection wrong: %+v", pending)
		}
		if resp.GetItems()[1].GetResolvedAt() == "" {
			t.Errorf("resolved proposal must carry resolved_at: %+v", resp.GetItems()[1])
		}
	}
}

func TestKnowledgeService_ListGovernanceProposals_InvalidStatus(t *testing.T) {
	svc, _ := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	svc.uc.SetCurateRepo(&stubGovCurateRepo{})
	_, err := svc.ListGovernanceProposals(context.Background(), &v1.ListGovernanceProposalsRequest{Status: "bogus"})
	if err == nil {
		t.Fatal("invalid status must error (biz 层收口)")
	}
}

func TestKnowledgeService_ListGovernanceProposals_CrossTenantDenied(t *testing.T) {
	svc, repo := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	svc.uc.SetCurateRepo(&stubGovCurateRepo{})
	if _, err := repo.CreateCollection(context.Background(), biz.KnowledgeCollection{ID: "c1", Name: "vault", Workspace: "ws-other"}); err != nil {
		t.Fatal(err)
	}
	ctx := workspace.WithContext(context.Background(), "ws-mine")
	_, err := svc.ListGovernanceProposals(ctx, &v1.ListGovernanceProposalsRequest{CollectionId: "c1"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("cross-tenant list must be NotFound, got %v", err)
	}
}

func TestKnowledgeService_ResolveGovernanceProposal(t *testing.T) {
	curate := &stubGovCurateRepo{views: []knowledge.GovernanceProposalView{{
		ID: 42, CollectionID: "c1", Kind: knowledge.ProposalKindMOCEmerge,
		Status: knowledge.ProposalStatusPending, Payload: map[string]any{"dedup_key": "moc:hub-x"},
	}}}
	svc, _ := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	svc.uc.SetCurateRepo(curate)

	resp, err := svc.ResolveGovernanceProposal(context.Background(), &v1.ResolveGovernanceProposalRequest{Id: 42, Decision: "applied"})
	if err != nil {
		t.Fatalf("ResolveGovernanceProposal: %v", err)
	}
	if resp.GetId() != 42 || resp.GetStatus() != "applied" {
		t.Errorf("response = %+v, want id=42 status=applied", resp)
	}
	if curate.resolvedID != 42 || curate.resolvedStatus != "applied" {
		t.Errorf("repo got id=%d status=%s, want 42/applied", curate.resolvedID, curate.resolvedStatus)
	}
}

func TestKnowledgeService_ResolveGovernanceProposal_Validation(t *testing.T) {
	svc, _ := newGovernanceService(t, &stubGovEntityRepo{}, nil)
	svc.uc.SetCurateRepo(&stubGovCurateRepo{})
	for _, req := range []*v1.ResolveGovernanceProposalRequest{
		{Id: 0, Decision: "applied"},
		{Id: -1, Decision: "applied"},
		{Id: 1, Decision: "bogus"},
		{Id: 1, Decision: ""},
	} {
		if _, err := svc.ResolveGovernanceProposal(context.Background(), req); err == nil {
			t.Errorf("%+v must be BadRequest", req)
		}
	}
}
