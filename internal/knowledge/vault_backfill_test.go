package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// ── SP1-H H-2：惰性锚点回填 local 库端到端 ──────────────────────────────────

// TestVaultSync_AnchorBackfill_LocalEndToEnd 写路径端到端（local 库）：
// A 引用 B 的未锚标题 → ApplyOne(A) 触发回填——B 文件落锚（VaultFiler CAS）、
// 镜像正文/hash 同步（下轮轮询幂等短路，chunks/embedding 不重建）、块索引锚定；
// A 下次写路径重解析后，边愈合到锚块 ID（最终一致）。
func TestVaultSync_AnchorBackfill_LocalEndToEnd(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "B.md", "# Sec\n\n内容。\n")
	writeVaultFile(t, root, "A.md", "见 [[B#Sec]]\n")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root, VaultBackend: bizknowledge.VaultBackendLocal, SyncState: "active"}
	repo.collections[vault.ID] = vault

	filer := bizknowledge.NewVaultFiler(nil)
	uc := bizknowledge.NewUsecaseFromRepo(repo)
	uc.SetLinkRepos(repo, repo)
	uc.SetBlockIndexRepos(repo, repo)
	uc.SetVaultFiler(filer) // 回填 local 路径必经（对齐生产 ProvideKnowledgeUsecase 装配）
	applier := NewVaultSyncApplier(uc, filer, nil, loggateway.NewNoop())

	ctx := context.Background()
	if err := applier.ApplyOne(ctx, vault, "B.md"); err != nil {
		t.Fatalf("apply B: %v", err)
	}
	docB, err := uc.GetDocumentByRelPath(ctx, vault.ID, "B.md")
	if err != nil {
		t.Fatalf("doc B: %v", err)
	}
	docAID := func() string {
		d, err := uc.GetDocumentByRelPath(ctx, vault.ID, "A.md")
		if err != nil {
			t.Fatalf("doc A: %v", err)
		}
		return d.ID
	}

	chunksBefore := repo.deleteChunksCalls // B 自己的 chunks 重建计数
	if err := applier.ApplyOne(ctx, vault, "A.md"); err != nil {
		t.Fatalf("apply A: %v", err)
	}

	// ① B 文件落锚（文件系统真相源）。
	disk, err := os.ReadFile(filepath.Join(root, "B.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(disk), "# Sec ^") {
		t.Fatalf("B 文件未落锚: %q", disk)
	}
	// ② 镜像 hash 同步 = 新文件 hash（下轮轮询短路）；chunks 未重建
	// （deleteChunks 仅 +1：A 自己的首次索引）。
	docB2, err := uc.GetDocumentByRelPath(ctx, vault.ID, "B.md")
	if err != nil {
		t.Fatal(err)
	}
	if docB2.ContentHash != bizknowledge.HashContent(string(disk)) {
		t.Errorf("B 镜像 hash = %q, want %q（轮询短路前提）", docB2.ContentHash, bizknowledge.HashContent(string(disk)))
	}
	if repo.deleteChunksCalls != chunksBefore+1 {
		t.Errorf("回填触发 B chunks 重建: deleteChunksCalls %d → %d, want +1（仅 A）", chunksBefore, repo.deleteChunksCalls)
	}
	if !strings.Contains(docB2.ContentText, "# Sec ^") {
		t.Errorf("B 镜像正文未同步: %q", docB2.ContentText)
	}
	// ③ B 块索引锚定，锚文本与文件一致。
	var anchor string
	for _, b := range repo.blocks[docB.ID] {
		if b.Anchor != "" {
			anchor = b.Anchor
		}
	}
	if anchor == "" {
		t.Fatal("B 块索引未锚定")
	}
	if !strings.Contains(string(disk), "^"+anchor) {
		t.Errorf("文件锚文本与块锚不一致: %q vs %q", disk, anchor)
	}
	// ④ A 的边 doc 级已解析（块级愈合留待 A 下次写路径，最终一致）。
	refs := repo.blockRefs[docAID()]
	if len(refs) != 1 || refs[0].DstDocID != docB.ID {
		t.Fatalf("A 边解析 = %+v, want dst doc %s", refs, docB.ID)
	}
	// ⑤ A 内容变更再应用 → Resolver 命中锚块 → 边愈合到锚块 ID。
	writeVaultFile(t, root, "A.md", "见 [[B#Sec]]\n\n补充。\n")
	if err := applier.ApplyOne(ctx, vault, "A.md"); err != nil {
		t.Fatalf("re-apply A: %v", err)
	}
	refs = repo.blockRefs[docAID()]
	if len(refs) != 1 || refs[0].DstBlockID != anchor {
		t.Errorf("边愈合失败: %+v, want dst block %q", refs, anchor)
	}
	// ⑥ 幂等稳定：B 文件不再变化（无二次回填），锚点不漂移。
	disk2, _ := os.ReadFile(filepath.Join(root, "B.md"))
	if string(disk2) != string(disk) {
		t.Errorf("B 文件二次漂移:\n%q\n→\n%q", disk, disk2)
	}
}
