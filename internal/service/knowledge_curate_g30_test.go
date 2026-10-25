package service

import (
	"context"
	"sync"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── G-30：CurateKnowledgeNow 手动治理 RPC（治理仪表盘触发入口） ───────────────

// curateNowStubRepo 实现 KnowledgeCurateRepo 中 CurateKnowledge 实际触达的最小
// 方法集（治理各任务均空返回）；其余方法经 nil 嵌入——本轮数据全空时不应被调用。
// started/release 非 nil 时首次 DecayCoActivatedEdges 阻塞至 release 关闭（在途
// 冲突门并发测试用）。
type curateNowStubRepo struct {
	bizknowledge.KnowledgeCurateRepo
	mu        sync.Mutex
	calls     int
	err       error
	gotDryRun bool
	started   chan struct{}
	release   chan struct{}
}

func (s *curateNowStubRepo) DecayCoActivatedEdges(_ context.Context, _ string, _, _ float64, dryRun bool) (int, int, error) {
	s.mu.Lock()
	s.calls++
	s.gotDryRun = dryRun
	first := s.calls == 1
	err := s.err
	started, release := s.started, s.release
	s.mu.Unlock()
	if first && started != nil {
		close(started)
		<-release
	}
	return 0, 0, err
}

func (s *curateNowStubRepo) ListPromotableRelations(context.Context, int) ([]string, error) {
	return nil, nil
}

func (s *curateNowStubRepo) ListStaleEntries(context.Context, string, int, int) ([]bizknowledge.StaleEntryStat, error) {
	return nil, nil
}

func (s *curateNowStubRepo) ListContradictsEdges(context.Context, string, int) ([]bizknowledge.ContradictsEdgeStat, error) {
	return nil, nil
}

func (s *curateNowStubRepo) ListOrphanEntries(context.Context, string, int, int) ([]bizknowledge.OrphanEntryStat, error) {
	return nil, nil
}

func (s *curateNowStubRepo) ListHubClusters(context.Context, string, int, int) ([]bizknowledge.HubClusterStat, error) {
	return nil, nil
}

func (s *curateNowStubRepo) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *curateNowStubRepo) dryRunSeen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gotDryRun
}

func newCurateNowService(t *testing.T, curate bizknowledge.KnowledgeCurateRepo, bus *rebuildEventBus) *KnowledgeService {
	t.Helper()
	if bus == nil {
		bus = &rebuildEventBus{} // 防 typed-nil：publishKnowledgeCurate 只判接口 nil
	}
	repo := newUS14MemRepo()
	if _, err := repo.CreateCollection(governanceTestContext(), biz.KnowledgeCollection{
		ID: "c1", Name: "vault", Workspace: workspace.DefaultWorkspaceID,
	}); err != nil {
		t.Fatal(err)
	}
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	if curate != nil {
		uc.SetCurateRepo(curate)
	}
	return NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, bus, nil, nil, loggateway.NewNoop())
}

// TestCurateKnowledgeNow_AsyncCompletesAndPublishes RPC 立即返回 curating；后台
// 治理完成并广播 knowledge_curate done 终态；dry_run 透传至治理选项。
func TestCurateKnowledgeNow_AsyncCompletesAndPublishes(t *testing.T) {
	stub := &curateNowStubRepo{}
	bus := &rebuildEventBus{}
	svc := newCurateNowService(t, stub, bus)
	svc.SetJobLocker(NewMemKnowledgeJobLocker())

	resp, err := svc.CurateKnowledgeNow(governanceTestContext(), &v1.CurateKnowledgeNowRequest{
		CollectionId: "c1", DryRun: true,
	})
	if err != nil {
		t.Fatalf("CurateKnowledgeNow: %v", err)
	}
	if resp.GetStatus() != "curating" {
		t.Errorf("status = %q, want curating", resp.GetStatus())
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if stub.count() >= 1 {
			for _, n := range bus.snapshot() {
				if n.NoticeType != "knowledge_curate" || n.Meta["status"] != "done" {
					continue
				}
				if n.Meta["collection_id"] != "c1" {
					t.Errorf("终态事件 collection_id = %+v, want c1", n.Meta)
				}
				if !stub.dryRunSeen() {
					t.Error("dry_run 未透传至治理选项")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("未收到 knowledge_curate done 终态事件（stub calls = %d）", stub.count())
}

// TestCurateKnowledgeNow_ConflictWhileRunning 同库在途治理时第二个请求 409；
// 在途任务完成后门放开（幂等重入，不叠加双倍提案）。
func TestCurateKnowledgeNow_ConflictWhileRunning(t *testing.T) {
	stub := &curateNowStubRepo{started: make(chan struct{}), release: make(chan struct{})}
	svc := newCurateNowService(t, stub, nil)
	svc.SetJobLocker(NewMemKnowledgeJobLocker())

	if _, err := svc.CurateKnowledgeNow(governanceTestContext(), &v1.CurateKnowledgeNowRequest{CollectionId: "c1"}); err != nil {
		t.Fatalf("首次治理应受理: %v", err)
	}
	select {
	case <-stub.started:
	case <-time.After(5 * time.Second):
		t.Fatal("后台治理未启动")
	}

	_, err := svc.CurateKnowledgeNow(governanceTestContext(), &v1.CurateKnowledgeNowRequest{CollectionId: "c1"})
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Errorf("在途治理冲突 = %v, want CodeConflict", err)
	}

	close(stub.release)
	deadline := time.Now().Add(2 * time.Second)
	for stub.count() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("重跑未受理")
		}
		_, _ = svc.CurateKnowledgeNow(governanceTestContext(), &v1.CurateKnowledgeNowRequest{CollectionId: "c1"})
		time.Sleep(10 * time.Millisecond)
	}
}

// TestCurateKnowledgeNow_RejectedInputs 集合不存在 NotFound；跨租户拒写。
func TestCurateKnowledgeNow_RejectedInputs(t *testing.T) {
	stub := &curateNowStubRepo{}
	svc := newCurateNowService(t, stub, nil)
	svc.SetJobLocker(NewMemKnowledgeJobLocker())

	if _, err := svc.CurateKnowledgeNow(governanceTestContext(), &v1.CurateKnowledgeNowRequest{CollectionId: "ghost"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Errorf("集合不存在 err = %v, want CodeNotFound", err)
	}
	repo := newUS14MemRepo()
	if _, err := repo.CreateCollection(governanceTestContext(), biz.KnowledgeCollection{
		ID: "c2", Name: "vault", Workspace: "ws-other",
	}); err != nil {
		t.Fatal(err)
	}
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	uc.SetCurateRepo(&curateNowStubRepo{})
	svc2 := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	svc2.SetJobLocker(NewMemKnowledgeJobLocker())
	ctx := workspace.WithContext(governanceTestContext(), "ws-mine")
	if _, err := svc2.CurateKnowledgeNow(ctx, &v1.CurateKnowledgeNowRequest{CollectionId: "c2"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Errorf("跨租户治理 err = %v, want CodeNotFound", err)
	}
}

// TestCurateKnowledgeNow_UnwiredCuratePublishesError 治理仓储未接线时后台治理
// 失败 → WS error 终态且任务门放开（可重试）。
func TestCurateKnowledgeNow_UnwiredCuratePublishesError(t *testing.T) {
	bus := &rebuildEventBus{}
	svc := newCurateNowService(t, nil, bus)
	svc.SetJobLocker(NewMemKnowledgeJobLocker())

	if _, err := svc.CurateKnowledgeNow(governanceTestContext(), &v1.CurateKnowledgeNowRequest{CollectionId: "c1"}); err != nil {
		t.Fatalf("CurateKnowledgeNow: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range bus.snapshot() {
			if n.NoticeType == "knowledge_curate" && n.Meta["status"] == "error" {
				if n.Meta["error_message"] == "" {
					t.Error("error 终态应携带 error_message")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("未收到 knowledge_curate error 终态事件")
}
