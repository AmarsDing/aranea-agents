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

// ── G-11：ReindexVault service 接线（SOTA 报告 G-11） ────────────────────────

// reindexVaultStub 记录 ReindexVault 调用；started/release 非 nil 时首次调用
// 阻塞至 release 关闭（冲突门并发测试用）。
type reindexVaultStub struct {
	mu      sync.Mutex
	calls   int
	err     error
	started chan struct{}
	release chan struct{}
}

func (s *reindexVaultStub) StartVault(biz.KnowledgeCollection) {}
func (s *reindexVaultStub) StopVault(string)                   {}

func (s *reindexVaultStub) ReindexVault(_ context.Context, _ biz.KnowledgeCollection) error {
	s.mu.Lock()
	s.calls++
	first := s.calls == 1
	err := s.err
	started, release := s.started, s.release
	s.mu.Unlock()
	if first && started != nil {
		close(started)
		<-release
	}
	return err
}

func (s *reindexVaultStub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// bus fake 复用 knowledge_rebuild_test.go 的 rebuildEventBus。

// TestReindexVault_AsyncCompletesAndPublishes RPC 立即返回 reindexing；后台调
// runner 全量重建；WS 广播知识 reindex 终态 done。
func TestReindexVault_AsyncCompletesAndPublishes(t *testing.T) {
	repo := newUS14MemRepo()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{
		ID: "c1", Name: "vault", VaultBackend: bizknowledge.VaultBackendLocal,
		Workspace: workspace.DefaultWorkspaceID, RootPath: "C:/vault", SyncState: "active",
	}); err != nil {
		t.Fatal(err)
	}
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	bus := &rebuildEventBus{}
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, bus, nil, nil, loggateway.NewNoop())
	stub := &reindexVaultStub{}
	svc.SetVaultSyncController(stub)

	resp, err := svc.ReindexVault(ctx, &v1.ReindexVaultRequest{Id: "c1"})
	if err != nil {
		t.Fatalf("ReindexVault: %v", err)
	}
	if resp.GetStatus() != "reindexing" {
		t.Errorf("status = %q, want reindexing", resp.GetStatus())
	}

	// 后台完成 + 终态 WS 事件（EP-KN-02 模式）。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if stub.count() >= 1 {
			for _, n := range bus.snapshot() {
				if n.NoticeType != "knowledge_reindex" {
					continue
				}
				if n.Meta["event_type"] != "knowledge_vault_reindex" || n.Meta["status"] != "done" {
					continue
				}
				if n.Meta["collection_id"] != "c1" {
					t.Errorf("终态事件 collection_id = %+v, want c1", n.Meta)
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("未收到 knowledge_vault_reindex done 终态事件（stub calls = %d）", stub.count())
}

// TestReindexVault_ConflictWhileRunning 同库重索引在途时第二个请求 409；
// 在途任务完成后门放开（幂等重入）。
func TestReindexVault_ConflictWhileRunning(t *testing.T) {
	repo := newUS14MemRepo()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{
		ID: "c1", Name: "vault", VaultBackend: bizknowledge.VaultBackendLocal,
		Workspace: workspace.DefaultWorkspaceID, RootPath: "C:/vault", SyncState: "active",
	}); err != nil {
		t.Fatal(err)
	}
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	stub := &reindexVaultStub{started: make(chan struct{}), release: make(chan struct{})}
	svc.SetVaultSyncController(stub)

	if _, err := svc.ReindexVault(ctx, &v1.ReindexVaultRequest{Id: "c1"}); err != nil {
		t.Fatalf("首次重索引应受理: %v", err)
	}
	select {
	case <-stub.started:
	case <-time.After(5 * time.Second):
		t.Fatal("后台重索引未启动")
	}

	_, err := svc.ReindexVault(ctx, &v1.ReindexVaultRequest{Id: "c1"})
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Errorf("在途重索引冲突 = %v, want CodeConflict", err)
	}

	close(stub.release)
	deadline := time.Now().Add(2 * time.Second)
	for stub.count() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("重跑未受理")
		}
		if _, err := svc.ReindexVault(ctx, &v1.ReindexVaultRequest{Id: "c1"}); err == nil {
			t.Log("重跑受理前 stub count", stub.count())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestReindexVault_RejectedInputs：team 库 / 无 root_path / 未装配控制器 / 集合不存在。
func TestReindexVault_RejectedInputs(t *testing.T) {
	repo := newUS14MemRepo()
	ctx := context.Background()
	for _, col := range []biz.KnowledgeCollection{
		{ID: "team1", Name: "team", VaultBackend: bizknowledge.VaultBackendTeam, Workspace: workspace.DefaultWorkspaceID},
		{ID: "legacy1", Name: "legacy", Workspace: workspace.DefaultWorkspaceID}, // 无 RootPath
		{ID: "local1", Name: "local", VaultBackend: bizknowledge.VaultBackendLocal, Workspace: workspace.DefaultWorkspaceID, RootPath: "C:/vault"},
	} {
		if _, err := repo.CreateCollection(ctx, col); err != nil {
			t.Fatal(err)
		}
	}
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)

	t.Run("team 库拒绝", func(t *testing.T) {
		svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
		svc.SetVaultSyncController(&reindexVaultStub{})
		_, err := svc.ReindexVault(ctx, &v1.ReindexVaultRequest{Id: "team1"})
		if !apierror.IsCode(err, apierror.CodeBadRequest) {
			t.Errorf("err = %v, want CodeBadRequest", err)
		}
	})

	t.Run("无 root_path 拒绝", func(t *testing.T) {
		svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
		svc.SetVaultSyncController(&reindexVaultStub{})
		_, err := svc.ReindexVault(ctx, &v1.ReindexVaultRequest{Id: "legacy1"})
		if !apierror.IsCode(err, apierror.CodeBadRequest) {
			t.Errorf("err = %v, want CodeBadRequest", err)
		}
	})

	t.Run("控制层未装配 Unavailable", func(t *testing.T) {
		svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
		_, err := svc.ReindexVault(ctx, &v1.ReindexVaultRequest{Id: "local1"})
		if !apierror.IsCode(err, apierror.CodeUnavailable) {
			t.Errorf("err = %v, want CodeUnavailable", err)
		}
	})

	t.Run("集合不存在 NotFound", func(t *testing.T) {
		svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
		svc.SetVaultSyncController(&reindexVaultStub{})
		_, err := svc.ReindexVault(ctx, &v1.ReindexVaultRequest{Id: "ghost"})
		if !apierror.IsCode(err, apierror.CodeNotFound) {
			t.Errorf("err = %v, want CodeNotFound", err)
		}
	})
}

// TestReindexVault_RunnerFailurePublishesError runner 返回错误 → WS error 终态
// 且任务门放开（可重试）。
func TestReindexVault_RunnerFailurePublishesError(t *testing.T) {
	repo := newUS14MemRepo()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{
		ID: "c1", Name: "vault", VaultBackend: bizknowledge.VaultBackendLocal,
		Workspace: workspace.DefaultWorkspaceID, RootPath: "C:/vault", SyncState: "active",
	}); err != nil {
		t.Fatal(err)
	}
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	bus := &rebuildEventBus{}
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, bus, nil, nil, loggateway.NewNoop())
	stub := &reindexVaultStub{err: apierror.BadRequest("KNOWLEDGE", "scan failed")}
	svc.SetVaultSyncController(stub)

	if _, err := svc.ReindexVault(ctx, &v1.ReindexVaultRequest{Id: "c1"}); err != nil {
		t.Fatalf("ReindexVault: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range bus.snapshot() {
			if n.NoticeType == "knowledge_reindex" && n.Meta["status"] == "error" {
				if n.Meta["error_message"] == "" {
					t.Error("error 终态应携带 error_message")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("未收到 knowledge_vault_reindex error 终态事件")
}
