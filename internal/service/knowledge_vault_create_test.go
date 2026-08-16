package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
)

// US-15：CreateCollection API 即创建 Vault——root_path 必填、embedding_model 可选。
func TestKnowledgeService_CreateCollection_Vault(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, nil)
	root := t.TempDir()

	t.Run("root_path 必填", func(t *testing.T) {
		_, err := svc.CreateCollection(context.Background(), &v1.CreateCollectionRequest{Name: "x"})
		if err == nil || !strings.Contains(err.Error(), "root_path") {
			t.Fatalf("err = %v, want root_path required", err)
		}
	})

	t.Run("embedding_model 可选", func(t *testing.T) {
		c, err := svc.CreateCollection(context.Background(), &v1.CreateCollectionRequest{
			Name:     "无向量库",
			RootPath: root,
		})
		if err != nil {
			t.Fatalf("CreateCollection: %v", err)
		}
		if c.GetRootPath() != filepath.Clean(root) {
			t.Fatalf("root_path = %q, want %q", c.GetRootPath(), filepath.Clean(root))
		}
		if c.GetSyncState() != "active" {
			t.Fatalf("sync_state = %q, want active", c.GetSyncState())
		}
	})

	t.Run("root_path 不存在", func(t *testing.T) {
		_, err := svc.CreateCollection(context.Background(), &v1.CreateCollectionRequest{
			Name:     "x",
			RootPath: filepath.Join(root, "no-such-dir"),
		})
		if err == nil || !strings.Contains(err.Error(), "root_path") {
			t.Fatalf("err = %v, want root_path not found", err)
		}
	})
}

// SP1-F：vault_backend=team 团队库——root_path 必须为空、不拉起同步循环（PG 即真相源）。
func TestKnowledgeService_CreateCollection_TeamBackend(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, nil)
	sync := &stubVaultSync{}
	svc.SetVaultSyncController(sync)

	t.Run("team：root_path 为空成功且不启动同步", func(t *testing.T) {
		c, err := svc.CreateCollection(context.Background(), &v1.CreateCollectionRequest{
			Name:         "团队库",
			VaultBackend: "team",
		})
		if err != nil {
			t.Fatalf("CreateCollection: %v", err)
		}
		if c.GetVaultBackend() != "team" {
			t.Fatalf("vault_backend = %q, want team", c.GetVaultBackend())
		}
		if c.GetRootPath() != "" {
			t.Fatalf("root_path = %q, want empty", c.GetRootPath())
		}
		if sync.started != 0 {
			t.Fatalf("team vault 不应启动同步循环, started = %d", sync.started)
		}
	})

	t.Run("team：设置 root_path 报错", func(t *testing.T) {
		_, err := svc.CreateCollection(context.Background(), &v1.CreateCollectionRequest{
			Name:         "x",
			VaultBackend: "team",
			RootPath:     t.TempDir(),
		})
		if err == nil || !strings.Contains(err.Error(), "root_path") {
			t.Fatalf("err = %v, want team root_path forbidden", err)
		}
	})

	t.Run("local：启动同步循环", func(t *testing.T) {
		_, err := svc.CreateCollection(context.Background(), &v1.CreateCollectionRequest{
			Name:     "本地库",
			RootPath: t.TempDir(),
		})
		if err != nil {
			t.Fatalf("CreateCollection: %v", err)
		}
		if sync.started != 1 {
			t.Fatalf("local vault 应启动同步循环, started = %d", sync.started)
		}
	})

	t.Run("未知 backend 报错", func(t *testing.T) {
		_, err := svc.CreateCollection(context.Background(), &v1.CreateCollectionRequest{
			Name:         "x",
			VaultBackend: "s3",
		})
		if err == nil || !strings.Contains(err.Error(), "vault_backend") {
			t.Fatalf("err = %v, want invalid vault_backend", err)
		}
	})
}

type stubVaultSync struct {
	started int
}

func (s *stubVaultSync) StartVault(_ biz.KnowledgeCollection) { s.started++ }
func (s *stubVaultSync) StopVault(_ string)                   {}
