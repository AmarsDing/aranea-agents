package knowledge

import (
	"context"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// ── supervisor 构造辅助 ─────────────────────────────────────────────────────

func newTestSupervisor(repo *vaultSyncMemRepo, interval time.Duration) *VaultSyncSupervisor {
	r := newTestRunner(repo, nil)
	r.SetInterval(interval)
	return NewVaultSyncSupervisor(r, bizknowledge.NewUsecaseFromRepo(repo), loggateway.NewNoop())
}

func eventually(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v: %s", d, msg)
}

// ── StartAll：仅拉起 root_path 非空的 vault，首轮扫描即入库 ────────────────

func TestVaultSyncSupervisor_StartAll_StartsVaultsWithRootPath(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	repo.collections["col1"] = bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections["col2"] = bizknowledge.Collection{ID: "col2"} // 历史 collection，无 root_path

	s := newTestSupervisor(repo, time.Hour) // 长间隔：只验证启动即扫的一轮
	defer s.Stop()
	s.StartAll(context.Background())

	if got := s.RunningCount(); got != 1 {
		t.Fatalf("RunningCount = %d, want 1（无 root_path 的 collection 必须跳过）", got)
	}
	eventually(t, 2*time.Second, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.documents) == 1
	}, "col1 首轮扫描应索引 a.md")
}

// ── StartVault：幂等，重复调用不重复拉起 ──────────────────────────────────

func TestVaultSyncSupervisor_StartVault_Idempotent(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	s := newTestSupervisor(repo, time.Hour)
	defer s.Stop()
	s.StartVault(vault)
	s.StartVault(vault) // 重复调用
	s.StartVault(vault)

	if got := s.RunningCount(); got != 1 {
		t.Fatalf("RunningCount = %d, want 1（StartVault 必须幂等）", got)
	}
}

// ── StopVault：停止后 tick 不再同步新文件 ─────────────────────────────────

func TestVaultSyncSupervisor_StopVault_StopsSync(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	s := newTestSupervisor(repo, 10*time.Millisecond)
	defer s.Stop()
	s.StartVault(vault)
	eventually(t, 2*time.Second, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return len(repo.documents) == 1
	}, "首轮应索引 a.md")

	s.StopVault(vault.ID)
	if got := s.RunningCount(); got != 0 {
		t.Fatalf("RunningCount = %d, want 0 after StopVault", got)
	}

	// 停止后新增文件：等待若干 tick，不得入库。
	writeVaultFile(t, root, "b.md", "# B\n\nbravo")
	time.Sleep(100 * time.Millisecond)
	repo.mu.Lock()
	n := len(repo.documents)
	repo.mu.Unlock()
	if n != 1 {
		t.Fatalf("documents = %d after StopVault, want 1（停止后不得再同步）", n)
	}
}

// ── Stop：停止全部 vault ──────────────────────────────────────────────────

func TestVaultSyncSupervisor_Stop_StopsAll(t *testing.T) {
	root1 := t.TempDir()
	root2 := t.TempDir()
	writeVaultFile(t, root1, "a.md", testVaultMD)
	writeVaultFile(t, root2, "b.md", "# B\n\nbravo")

	repo := newVaultSyncMemRepo()
	repo.collections["col1"] = bizknowledge.Collection{ID: "col1", RootPath: root1}
	repo.collections["col2"] = bizknowledge.Collection{ID: "col2", RootPath: root2}

	s := newTestSupervisor(repo, time.Hour)
	s.StartAll(context.Background())
	if got := s.RunningCount(); got != 2 {
		t.Fatalf("RunningCount = %d, want 2", got)
	}
	s.Stop()
	if got := s.RunningCount(); got != 0 {
		t.Fatalf("RunningCount = %d after Stop, want 0", got)
	}
	// Stop 幂等。
	s.Stop()
}
