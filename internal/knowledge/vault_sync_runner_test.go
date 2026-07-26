package knowledge

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── stub watcher ─────────────────────────────────────────────────────────────

type stubWatcher struct {
	ch     chan struct{}
	closed atomic.Bool
}

func newStubWatcher() *stubWatcher { return &stubWatcher{ch: make(chan struct{}, 8)} }

func (s *stubWatcher) Changed() <-chan struct{} { return s.ch }
func (s *stubWatcher) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		close(s.ch)
	}
	return nil
}
func (s *stubWatcher) ping() { s.ch <- struct{}{} }

// ── runner 构造辅助 ──────────────────────────────────────────────────────────

func newTestRunner(repo *vaultSyncMemRepo, embedder Embedder) *VaultSyncRunner {
	uc := bizknowledge.NewUsecaseFromRepo(repo)
	applier := NewVaultSyncApplier(uc, bizknowledge.NewVaultFiler(nil), embedder, loggateway.NewNoop())
	return NewVaultSyncRunner(bizknowledge.NewSyncEngine(nil), applier, uc, loggateway.NewNoop())
}

// ── SyncOnce：首轮全量 ──────────────────────────────────────────────────────

func TestVaultSyncRunner_SyncOnce_FirstScan_IndexesAll(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)
	writeVaultFile(t, root, "notes/b.md", "# B\n\nbravo")
	writeVaultFile(t, root, ".hidden/c.md", "# C\n\nignored") // 隐藏目录不索引

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	r := newTestRunner(repo, nil)
	if err := r.SyncOnce(context.Background(), vault); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}

	if len(repo.documents) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(repo.documents))
	}
	col := repo.collections[vault.ID]
	if col.SyncState != "active" {
		t.Errorf("SyncState = %q, want active", col.SyncState)
	}
	if col.LastSyncAt == "" {
		t.Error("LastSyncAt must be set after successful sync")
	}
}

// ── SyncOnce：增量 diff，无变化不产生事件 ──────────────────────────────────

func TestVaultSyncRunner_SyncOnce_SecondScan_NoChanges_NoWork(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	r := newTestRunner(repo, nil)
	if err := r.SyncOnce(context.Background(), vault); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if len(repo.documents) != 1 {
		t.Fatalf("setup: expected 1 doc, got %d", len(repo.documents))
	}
	chunksBefore := len(repo.chunks)
	deletesBefore := repo.deleteChunksCalls

	if err := r.SyncOnce(context.Background(), vault); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(repo.chunks) != chunksBefore {
		t.Errorf("chunks rebuilt on no-change scan: before=%d after=%d", chunksBefore, len(repo.chunks))
	}
	if repo.deleteChunksCalls != deletesBefore {
		t.Errorf("DeleteChunks called on no-change scan: before=%d after=%d", deletesBefore, repo.deleteChunksCalls)
	}
}

// ── SyncOnce：重启后从 DB 重建 prev，检测出启动前被删除的文件 ──────────────

func TestVaultSyncRunner_SyncOnce_Restart_DetectsDeletedFromDB(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "alive.md", "# alive\n\nx")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault
	// 模拟上一轮同步留下的镜像：gone.md 已从磁盘删除，但 DB 镜像还在。
	repo.documents["doc-gone"] = bizknowledge.Document{
		ID: "doc-gone", CollectionID: vault.ID, RelPath: "gone.md",
		Status: "indexed", ChunkCount: 1,
	}
	repo.chunks = append(repo.chunks, bizknowledge.Chunk{ID: "ch-gone", DocID: "doc-gone", CollectionID: vault.ID, Content: "x"})
	col := repo.collections[vault.ID]
	col.DocumentCount = 1
	col.ChunkCount = 1
	repo.collections[vault.ID] = col

	r := newTestRunner(repo, nil) // prev 为空，必须先从 DB 重建
	if err := r.SyncOnce(context.Background(), vault); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	if _, ok := repo.documents["doc-gone"]; ok {
		t.Error("stale mirror for deleted file must be removed after restart scan")
	}
	if _, ok := repo.collections[vault.ID]; !ok {
		t.Fatal("collection must survive sync")
	}
}

// ── SyncOnce：Scan 失败 → sync_state=error，不刷新 lastSyncAt ──────────────

func TestVaultSyncRunner_SyncOnce_ScanFailure_MarksError(t *testing.T) {
	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: filepath.Join(t.TempDir(), "missing")}
	repo.collections[vault.ID] = vault

	r := newTestRunner(repo, nil)
	err := r.SyncOnce(context.Background(), vault)
	if err == nil {
		t.Fatal("expected scan error for missing root")
	}
	col := repo.collections[vault.ID]
	if col.SyncState != "error" {
		t.Errorf("SyncState = %q, want error", col.SyncState)
	}
	if col.LastSyncAt != "" {
		t.Errorf("LastSyncAt must not be refreshed on failure, got %q", col.LastSyncAt)
	}
}

// ── RunVault：tick 循环捕获变更，ctx 取消退出 ──────────────────────────────

func TestVaultSyncRunner_RunVault_TickCapturesChange(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	r := newTestRunner(repo, nil)
	r.SetInterval(20 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.RunVault(ctx, vault) }()

	// 等首轮完成后修改文件，等下一轮 tick 捕获。
	waitFor(t, 2*time.Second, func() bool { return len(repo.documents) == 1 })
	writeVaultFile(t, root, "b.md", "# B\n\nnew file")
	waitFor(t, 2*time.Second, func() bool { return len(repo.documents) == 2 })

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RunVault returned error on cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunVault did not exit after cancel")
	}
}

// ── RunVault：watcher hint 触发提前扫描，不等 interval ─────────────────────

func TestVaultSyncRunner_RunVault_WatcherHint_EarlyScan(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	w := newStubWatcher()
	defer w.Close()

	r := newTestRunner(repo, nil)
	r.SetInterval(10 * time.Second) // 长到永不触发；只有 watcher 能推进
	r.SetWatcher(w)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.RunVault(ctx, vault) }()

	// 等首轮完成（RunVault 启动时立即扫一轮），再加文件 + ping。
	waitFor(t, 2*time.Second, func() bool { return len(repo.documents) == 1 })
	writeVaultFile(t, root, "b.md", "# B\n\nwatcher-triggered")
	w.ping()
	waitFor(t, 2*time.Second, func() bool { return len(repo.documents) == 2 })

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunVault did not exit after cancel")
	}
}

// ── 可靠性契约：apply 失败后下轮 SyncOnce 自动重试（prev 未推进） ──────────

// failOnceEmbedder 首次 Embed 返回错误，之后正常。
type failOnceEmbedder struct {
	calls atomic.Int32
}

func (f *failOnceEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.calls.Add(1) == 1 {
		return nil, errEmbedOnce
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3}
	}
	return out, nil
}
func (f *failOnceEmbedder) Dim() int { return 3 }

var errEmbedOnce = apierror.Internal("KNOWLEDGE", "transient embed failure")

func TestVaultSyncRunner_SyncOnce_ApplyFailure_RetriedNextRound(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root, EmbeddingModel: "m", Dim: 3}
	repo.collections[vault.ID] = vault

	emb := &failOnceEmbedder{}
	r := newTestRunner(repo, emb)

	// 第一轮：embed 失败 → 整轮 error，文档已建镜像但未索引。
	if err := r.SyncOnce(context.Background(), vault); err == nil {
		t.Fatal("first round must fail")
	}
	col := repo.collections[vault.ID]
	if col.SyncState != "error" {
		t.Errorf("SyncState = %q, want error", col.SyncState)
	}
	var doc bizknowledge.Document
	for _, d := range repo.documents {
		doc = d
	}
	if doc.Status == "indexed" {
		t.Fatal("setup: doc must not be indexed after failed round")
	}

	// 第二轮：文件未变，但 prev 未推进 → 事件重新生成 → 自动重试成功。
	if err := r.SyncOnce(context.Background(), vault); err != nil {
		t.Fatalf("second round must retry and succeed: %v", err)
	}
	doc = repo.documents[doc.ID]
	if doc.Status != "indexed" {
		t.Errorf("doc Status = %q after retry, want indexed（prev 未推进则事件重新生成）", doc.Status)
	}
	if len(repo.chunks) == 0 {
		t.Error("chunks must be built on retry")
	}
	col = repo.collections[vault.ID]
	if col.SyncState != "active" {
		t.Errorf("SyncState = %q, want active after recovery", col.SyncState)
	}
}

// ── 辅助 ────────────────────────────────────────────────────────────────────

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}
