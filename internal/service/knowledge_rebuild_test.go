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

// ── SP1-H：RebuildKnowledgeIndex service 接线（设计 S9 / US-29） ──────────────

// rebuildBlockIndexStub 记录 ReplaceDocBlocks 调用；started/release 非 nil 时
// 首次调用阻塞至 release 关闭（冲突门并发测试用）。
type rebuildBlockIndexStub struct {
	mu      sync.Mutex
	docs    []string
	started chan struct{}
	release chan struct{}
}

func (s *rebuildBlockIndexStub) ReplaceDocBlocks(_ context.Context, _, docID string, _ []bizknowledge.KnowledgeBlock, _ []bizknowledge.KnowledgeBlockRefInput) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	s.mu.Lock()
	s.docs = append(s.docs, docID)
	first := len(s.docs) == 1
	started, release := s.started, s.release
	s.mu.Unlock()
	if first && started != nil {
		close(started)
		<-release
	}
	return nil, nil
}

func (s *rebuildBlockIndexStub) ListDocBlocks(context.Context, string) ([]bizknowledge.KnowledgeBlock, error) {
	return nil, nil
}

func (s *rebuildBlockIndexStub) UpdateDocLinkKeys(context.Context, string, string, []string) error {
	return nil
}

func (s *rebuildBlockIndexStub) ListDocsMissingBlockIndex(context.Context, string, int) ([]string, error) {
	return nil, nil
}

func (s *rebuildBlockIndexStub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.docs)
}

// rebuildEventBus mutex 守护的 SystemNoticeEvent 捕获（后台 goroutine 发布 +
// 测试 goroutine 轮询读取，-race 安全）。
type rebuildEventBus struct {
	mu      sync.Mutex
	notices []*biz.SystemNoticeEvent
}

func (b *rebuildEventBus) Publish(_ context.Context, e biz.Event) {
	if ne, ok := e.(*biz.SystemNoticeEvent); ok {
		b.mu.Lock()
		b.notices = append(b.notices, ne)
		b.mu.Unlock()
	}
}

func (b *rebuildEventBus) Subscribe(biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return nil, func() {}
}

func (b *rebuildEventBus) snapshot() []*biz.SystemNoticeEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*biz.SystemNoticeEvent(nil), b.notices...)
}

// newRebuildService 装配 c1 库（local，两文档）+ 块索引桩 + 事件总线。
func newRebuildService(t *testing.T, idx bizknowledge.BlockIndexRepo, bus biz.EventBus) (*KnowledgeService, *us14MemRepo) {
	t.Helper()
	repo := newUS14MemRepo()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{
		ID: "c1", Name: "vault", VaultBackend: bizknowledge.VaultBackendLocal,
		Workspace: workspace.DefaultWorkspaceID, SyncState: "active",
	}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []biz.KnowledgeDocument{
		{ID: "d1", CollectionID: "c1", ContentText: "# A\n\n正文。\n"},
		{ID: "d2", CollectionID: "c1", ContentText: "# B\n\n正文。\n"},
	} {
		if _, err := repo.CreateDocument(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	uc.SetBlockIndexRepos(idx, nil)
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, bus, nil, loggateway.NewNoop())
	return svc, repo
}

// waitRebuildFinished 等待后台重建完成：先等 stub 看到 want 篇文档（此时必已
// 进入 rebuilding），再等 sync_state 恢复非 rebuilding。
func waitRebuildFinished(t *testing.T, stub *rebuildBlockIndexStub, repo *us14MemRepo, want int) biz.KnowledgeCollection {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if stub.count() >= want {
			col, err := repo.GetCollection(context.Background(), "c1")
			if err != nil {
				t.Fatal(err)
			}
			if col.SyncState != bizknowledge.SyncStateRebuilding {
				return col
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("rebuild did not finish in time (stub docs = %d)", stub.count())
	return biz.KnowledgeCollection{}
}

// TestRebuildKnowledgeIndex_AsyncCompletes RPC 立即返回 rebuilding；后台流式
// 重建两文档；sync_state rebuilding→active 恢复；WS 终态事件 done=2/total=2。
func TestRebuildKnowledgeIndex_AsyncCompletes(t *testing.T) {
	stub := &rebuildBlockIndexStub{}
	bus := &rebuildEventBus{}
	svc, repo := newRebuildService(t, stub, bus)

	resp, err := svc.RebuildKnowledgeIndex(context.Background(), &v1.RebuildKnowledgeIndexRequest{Id: "c1"})
	if err != nil {
		t.Fatalf("RebuildKnowledgeIndex: %v", err)
	}
	if resp.GetStatus() != bizknowledge.SyncStateRebuilding {
		t.Errorf("status = %q, want rebuilding", resp.GetStatus())
	}

	col := waitRebuildFinished(t, stub, repo, 2)
	if col.SyncState != "active" {
		t.Errorf("sync_state = %q, want active（重建后恢复原态）", col.SyncState)
	}

	// 终态 WS 事件（EP-KN-02 模式）：status=done、计数齐备。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range bus.snapshot() {
			if n.NoticeType != "knowledge_rebuild" {
				continue
			}
			if n.Meta["event_type"] != "knowledge_rebuild_index" || n.Meta["status"] != "done" {
				continue
			}
			if n.Meta["collection_id"] != "c1" || n.Meta["done"] != 2 || n.Meta["total"] != 2 || n.Meta["failed"] != 0 {
				t.Errorf("终态事件 meta = %+v", n.Meta)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("未收到 knowledge_rebuild_index done 终态事件")
}

// TestRebuildKnowledgeIndex_ConflictWhileRunning 同库重建在途时第二个请求
// 409 Conflict；在途任务完成后门放开（可再次发起，幂等重入）。
func TestRebuildKnowledgeIndex_ConflictWhileRunning(t *testing.T) {
	stub := &rebuildBlockIndexStub{started: make(chan struct{}), release: make(chan struct{})}
	svc, repo := newRebuildService(t, stub, nil)

	if _, err := svc.RebuildKnowledgeIndex(context.Background(), &v1.RebuildKnowledgeIndexRequest{Id: "c1"}); err != nil {
		t.Fatalf("首次重建应受理: %v", err)
	}
	select {
	case <-stub.started:
	case <-time.After(5 * time.Second):
		t.Fatal("后台重建未启动")
	}

	_, err := svc.RebuildKnowledgeIndex(context.Background(), &v1.RebuildKnowledgeIndexRequest{Id: "c1"})
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Errorf("在途重建冲突 = %v, want CodeConflict", err)
	}

	close(stub.release)
	waitRebuildFinished(t, stub, repo, 2)

	// 门已放开：重跑受理（幂等）。
	if _, err := svc.RebuildKnowledgeIndex(context.Background(), &v1.RebuildKnowledgeIndexRequest{Id: "c1"}); err != nil {
		t.Fatalf("完成后重跑应受理: %v", err)
	}
	waitRebuildFinished(t, stub, repo, 4)
}

// TestRebuildKnowledgeIndex_NotFound 集合不存在透传 NotFound。
func TestRebuildKnowledgeIndex_NotFound(t *testing.T) {
	svc, _ := newRebuildService(t, &rebuildBlockIndexStub{}, nil)
	_, err := svc.RebuildKnowledgeIndex(context.Background(), &v1.RebuildKnowledgeIndexRequest{Id: "ghost"})
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Errorf("err = %v, want CodeNotFound", err)
	}
}
