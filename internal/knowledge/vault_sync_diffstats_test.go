package knowledge

import (
	"context"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── G-32 T5 决策9 vault 通道钉：diff 帧消费钩子 ─────────────────────────────

// 索引成功提交后 diffStatsHook 必须恰好唤一次，collectionID/docID/统计三元组
// 忠实透传（Exact 判定由 service 帧函数兜底，本层不二判）。stats 值=退化
// 内存伪全量（Added=子块数）——钩子照收；帧发布闸在 service 层。
func TestVaultSyncApplier_DiffStatsHook_FiresOnCommit(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "notes/a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "v1", Name: "vault", RootPath: root, VaultBackend: "local"}
	uc := bizknowledge.NewUsecaseFromRepo(repo)
	if _, err := uc.CreateCollection(context.Background(), vault); err != nil {
		t.Fatalf("seed collection: %v", err)
	}
	applier := newTestApplier(repo, nil)

	type hookCall struct {
		collectionID, docID string
		stats               bizknowledge.ChunkDiffStats
	}
	var calls []hookCall
	applier.SetDiffStatsHook(func(collectionID, docID string, stats bizknowledge.ChunkDiffStats) {
		calls = append(calls, hookCall{collectionID, docID, stats})
	})

	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{createdEvent("notes/a.md", testVaultMD)}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("diffStatsHook 唤次 = %d, want 1（索引成功提交后恰好一次）", len(calls))
	}
	c := calls[0]
	if c.collectionID != "v1" {
		t.Fatalf("collectionID = %q, want v1", c.collectionID)
	}
	if c.docID == "" {
		t.Fatal("docID 为空——钩子必须携目标文档标识")
	}
	if c.stats.Added <= 0 {
		t.Fatalf("stats.Added = %d, want >0（伪全量路径 added=子块数）", c.stats.Added)
	}
	if c.stats.Exact {
		t.Fatal("内存退化路径 stats.Exact 必须 false（帧发布闸在 service 层）")
	}
}

// 未接线钩子（生产未接 diff 帧配方）时索引主链行为不得变（无 panic、零调用
// 依赖）。既有 ApplyEvents 全族测试本就在无钩子构型运行，此钉显式锁语义。
func TestVaultSyncApplier_NoDiffStatsHook_NoPanic(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "v2", Name: "vault2", RootPath: root, VaultBackend: "local"}
	uc := bizknowledge.NewUsecaseFromRepo(repo)
	if _, err := uc.CreateCollection(context.Background(), vault); err != nil {
		t.Fatalf("seed collection: %v", err)
	}
	applier := newTestApplier(repo, nil) // 无 SetDiffStatsHook
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{createdEvent("a.md", testVaultMD)}); err != nil {
		t.Fatalf("apply without diff hook: %v", err)
	}
}
