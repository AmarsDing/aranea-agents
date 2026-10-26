package artifactfs_test

// repo_purge_test.go — T-3 附件生命周期（2026-10-26 评审二轮）：
// FSArtifactRepo.PurgeBySession 会话级级联清理钉板。

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"aranea-agents/internal/data/artifactfs"
	"aranea-agents/pkg/loggateway"
)

// ① 删除目标会话全部文件 + 索引；其他会话不受影响；幂等重清成功。
func TestPurgeBySession_RemovesSessionKeepsOthers(t *testing.T) {
	repo := artifactfs.NewFSArtifactRepoAt(t.TempDir(), loggateway.NewNoop())
	ctx := context.Background()

	a1, err := repo.Save(ctx, "sess-a", "f1.txt", "text/plain", []byte("a1"))
	if err != nil {
		t.Fatal(err)
	}
	a2, err := repo.Save(ctx, "sess-a", "f2.txt", "text/plain", []byte("a2"))
	if err != nil {
		t.Fatal(err)
	}
	b1, err := repo.Save(ctx, "sess-b", "g1.txt", "text/plain", []byte("b1"))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.PurgeBySession(ctx, "sess-a"); err != nil {
		t.Fatalf("purge: %v", err)
	}
	// 目标会话：磁盘与索引双清。
	if _, _, err := repo.Load(ctx, a1.ID, 0); err == nil {
		t.Fatal("purged artifact a1 must not load")
	}
	if _, _, err := repo.Load(ctx, a2.ID, 0); err == nil {
		t.Fatal("purged artifact a2 must not load")
	}
	// 其他会话原样可读。
	if _, data, err := repo.Load(ctx, b1.ID, 0); err != nil || string(data) != "b1" {
		t.Fatalf("other-session artifact must survive, err=%v data=%q", err, data)
	}
	// 幂等：重清成功。
	if err := repo.PurgeBySession(ctx, "sess-a"); err != nil {
		t.Fatalf("re-purge must be idempotent, got %v", err)
	}
	// 清后同会话可重新写入（目录重建）。
	if _, err := repo.Save(ctx, "sess-a", "new.txt", "text/plain", []byte("x")); err != nil {
		t.Fatalf("save after purge: %v", err)
	}
}

// ② 目录穿越形态的 sessionID 拒绝（OUT-05/ART-01 防线在 purge 路径同样生效）。
func TestPurgeBySession_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(root, loggateway.NewNoop())
	// 哨兵：root 旁一层的目标文件，穿越成功则会被删。
	sibling := filepath.Join(filepath.Dir(root), "purge_sentinel.txt")
	if err := os.WriteFile(sibling, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(sibling)

	if err := repo.PurgeBySession(context.Background(), "../"+filepath.Base(filepath.Dir(root))); err == nil {
		t.Fatal("traversal sessionID must be rejected")
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("sentinel must survive rejected purge: %v", err)
	}
}
