package knowledge

import (
	"context"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// ── ReindexVault：强制重建派生索引（P1-4）─────────────────────────────────
//
// 契约：文件系统为唯一真相源。reindex 后派生索引（chunks/向量）与磁盘完全一致：
//  1. hash 匹配的文档也强制重建 chunks（绕过幂等短路——用于索引损坏/模型升级场景）
//  2. DB 有镜像但磁盘无文件 → 删除镜像与 chunks
//  3. 磁盘有文件但 DB 无镜像 → 创建并索引
//  4. sync_state=active + last_sync_at 刷新

func TestVaultReindex_RebuildsChunks_DespiteHashMatch(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root, EmbeddingModel: "m", Dim: 3}
	repo.collections[vault.ID] = vault

	emb := &vaultSyncStubEmbedder{dim: 3}
	r := newTestRunner(repo, emb)

	// 首轮正常索引。
	if err := r.SyncOnce(context.Background(), vault); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if len(repo.chunks) == 0 {
		t.Fatal("setup: expected chunks after initial sync")
	}
	embedCallsAfterSync := emb.calls
	deletesAfterSync := repo.deleteChunksCalls

	// reindex：文件未变，但必须强制重建（短路被绕过）。
	if err := r.ReindexVault(context.Background(), vault); err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if repo.deleteChunksCalls == deletesAfterSync {
		t.Error("reindex must force chunk rebuild even when content hash matches")
	}
	if emb.calls == embedCallsAfterSync {
		t.Error("reindex must re-embed chunks even when content hash matches")
	}
	var doc bizknowledge.Document
	for _, d := range repo.documents {
		doc = d
	}
	if doc.Status != "indexed" {
		t.Errorf("Status = %q, want indexed", doc.Status)
	}
	col := repo.collections[vault.ID]
	if col.SyncState != "active" {
		t.Errorf("SyncState = %q, want active", col.SyncState)
	}
	if col.LastSyncAt == "" {
		t.Error("LastSyncAt must be refreshed after reindex")
	}
}

func TestVaultReindex_RemovesStaleMirror_AndAddsNewFile(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "new.md", "# New\n\nfresh on disk")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault
	// stale：DB 有镜像但磁盘已删。
	repo.documents["doc-stale"] = bizknowledge.Document{
		ID: "doc-stale", CollectionID: vault.ID, RelPath: "stale.md",
		Status: "indexed", ChunkCount: 2,
	}
	repo.chunks = append(repo.chunks,
		bizknowledge.Chunk{ID: "c1", DocID: "doc-stale", CollectionID: vault.ID, Content: "x"},
		bizknowledge.Chunk{ID: "c2", DocID: "doc-stale", CollectionID: vault.ID, Content: "y"},
	)

	r := newTestRunner(repo, nil)
	if err := r.ReindexVault(context.Background(), vault); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	if _, ok := repo.documents["doc-stale"]; ok {
		t.Error("stale mirror must be removed by reindex")
	}
	for _, ch := range repo.chunks {
		if ch.DocID == "doc-stale" {
			t.Error("chunks of stale doc must be removed")
		}
	}
	found := false
	for _, d := range repo.documents {
		if d.RelPath == "new.md" {
			found = true
			if d.Status != "indexed" {
				t.Errorf("new.md Status = %q, want indexed", d.Status)
			}
		}
	}
	if !found {
		t.Error("new.md on disk must be mirrored and indexed")
	}
}

// reindex 幂等：连续两次结果一致（第二次仍强制重建，但终态相同）。
func TestVaultReindex_Idempotent(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	r := newTestRunner(repo, nil)
	if err := r.ReindexVault(context.Background(), vault); err != nil {
		t.Fatalf("first reindex: %v", err)
	}
	docsAfterFirst := len(repo.documents)
	if err := r.ReindexVault(context.Background(), vault); err != nil {
		t.Fatalf("second reindex: %v", err)
	}
	if len(repo.documents) != docsAfterFirst {
		t.Errorf("doc count drifted: first=%d second=%d", docsAfterFirst, len(repo.documents))
	}
	var doc bizknowledge.Document
	for _, d := range repo.documents {
		doc = d
	}
	col := repo.collections[vault.ID]
	if col.DocumentCount != 1 || col.ChunkCount != doc.ChunkCount {
		t.Errorf("collection counts = (%d,%d), want (1,%d)", col.DocumentCount, col.ChunkCount, doc.ChunkCount)
	}
}

// Scan 失败（root 不存在）→ error + sync_state=error，不破坏现有镜像。
func TestVaultReindex_ScanFailure_KeepsMirror(t *testing.T) {
	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: "no/such/dir"}
	repo.collections[vault.ID] = vault
	repo.documents["doc1"] = bizknowledge.Document{
		ID: "doc1", CollectionID: vault.ID, RelPath: "a.md", Status: "indexed",
	}

	r := newTestRunner(repo, nil)
	err := r.ReindexVault(context.Background(), vault)
	if err == nil {
		t.Fatal("expected scan error")
	}
	if _, ok := repo.documents["doc1"]; !ok {
		t.Error("existing mirror must survive failed reindex")
	}
	col := repo.collections[vault.ID]
	if col.SyncState != "error" {
		t.Errorf("SyncState = %q, want error", col.SyncState)
	}
}
