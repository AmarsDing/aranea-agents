package knowledge

import (
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"context"
	"testing"
)

// 块索引静默缺失（rebuildBlockIndex 失败仅 Warn 降级，content_hash 已落库下轮不再
// 重试）→ 收敛扫描检出漂移并自动重建块/refs。
func TestVaultSyncApplier_ConvergeMissingBlockIndex_RebuildsDrift(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)
	writeVaultFile(t, root, "b.md", "# 目标笔记\n\n被引用内容。\n")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	applier := newTestApplier(repo, nil)
	linkBody := "# 动量策略\n\n详见 [[b.md]] 的说明。\n"
	writeVaultFile(t, root, "a.md", linkBody)
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{
		createdEvent("a.md", linkBody),
		createdEvent("b.md", "# 目标笔记\n\n被引用内容。\n"),
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var docA string
	for id, d := range repo.documents {
		if d.RelPath == "a.md" {
			docA = id
		}
	}
	if docA == "" {
		t.Fatal("doc a.md 未落库")
	}
	if len(repo.blocks[docA]) == 0 || len(repo.blockRefs[docA]) == 0 {
		t.Fatalf("前置：a.md 应有块和 refs, blocks=%d refs=%d", len(repo.blocks[docA]), len(repo.blockRefs[docA]))
	}

	repo.blocks[docA] = nil
	repo.blockRefs[docA] = nil

	if err := applier.ConvergeMissingBlockIndex(context.Background(), vault); err != nil {
		t.Fatalf("converge: %v", err)
	}
	if len(repo.blocks[docA]) == 0 {
		t.Error("收敛后 a.md 块索引必须重建")
	}
	if len(repo.blockRefs[docA]) == 0 {
		t.Error("收敛后 a.md refs 必须重建（含 [[b.md]] 解析边）")
	}
}

// 无漂移 → 不重建（块索引保持原值，零写放大）；空内容文档不误报（合法 0 块）。
func TestVaultSyncApplier_ConvergeMissingBlockIndex_NoDriftNoOp(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)
	writeVaultFile(t, root, "empty.md", "")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	applier := newTestApplier(repo, nil)
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{
		createdEvent("a.md", testVaultMD),
		createdEvent("empty.md", ""),
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	before := repo.blocks
	if err := applier.ConvergeMissingBlockIndex(context.Background(), vault); err != nil {
		t.Fatalf("converge: %v", err)
	}
	for id, blocks := range repo.blocks {
		if len(before[id]) > 0 && len(blocks) > 0 && &before[id][0] != &blocks[0] {
			t.Errorf("doc %s: 无漂移不得重建块索引", id)
		}
	}

	if err := applier.ConvergeMissingBlockIndex(context.Background(), vault); err != nil {
		t.Fatalf("converge again: %v", err)
	}
}
