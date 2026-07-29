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
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil)
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
