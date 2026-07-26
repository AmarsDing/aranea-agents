package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── vault sync 内存 Repo（完整 bizknowledge.Repo 实现，测试专用） ─────────────

type vaultSyncMemRepo struct {
	mu                sync.Mutex
	collections       map[string]bizknowledge.Collection
	documents         map[string]bizknowledge.Document
	chunks            []bizknowledge.Chunk
	links             []bizknowledge.Link
	docEntities       map[string][]bizknowledge.DocEntity
	deleteChunksCalls int
}

func newVaultSyncMemRepo() *vaultSyncMemRepo {
	return &vaultSyncMemRepo{
		collections: make(map[string]bizknowledge.Collection),
		documents:   make(map[string]bizknowledge.Document),
		docEntities: make(map[string][]bizknowledge.DocEntity),
	}
}

func (m *vaultSyncMemRepo) CreateCollection(_ context.Context, c bizknowledge.Collection) (bizknowledge.Collection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.collections[c.ID] = c
	return c, nil
}
func (m *vaultSyncMemRepo) GetCollection(_ context.Context, id string) (bizknowledge.Collection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.collections[id]
	if !ok {
		return bizknowledge.Collection{}, errMemNotFound
	}
	return c, nil
}
func (m *vaultSyncMemRepo) ListCollections(_ context.Context, _ string, _, _ int) ([]bizknowledge.Collection, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]bizknowledge.Collection, 0, len(m.collections))
	for _, c := range m.collections {
		out = append(out, c)
	}
	return out, len(out), nil
}
func (m *vaultSyncMemRepo) DeleteCollection(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.collections, id)
	return nil
}
func (m *vaultSyncMemRepo) UpdateCollectionCounts(_ context.Context, id string, docD, chunkD int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.collections[id]
	c.DocumentCount += docD
	c.ChunkCount += chunkD
	m.collections[id] = c
	return nil
}
func (m *vaultSyncMemRepo) UpdateCollectionSyncState(_ context.Context, id, state string, lastSyncAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.collections[id]
	c.SyncState = state
	if !lastSyncAt.IsZero() {
		c.LastSyncAt = lastSyncAt.UTC().Format(time.RFC3339)
	}
	m.collections[id] = c
	return nil
}
func (m *vaultSyncMemRepo) CreateDocument(_ context.Context, d bizknowledge.Document) (bizknowledge.Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.documents[d.ID] = d
	return d, nil
}
func (m *vaultSyncMemRepo) GetDocument(_ context.Context, id string) (bizknowledge.Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.documents[id]
	if !ok {
		return bizknowledge.Document{}, errMemNotFound
	}
	return d, nil
}
func (m *vaultSyncMemRepo) GetDocumentByRelPath(_ context.Context, collectionID, relPath string) (bizknowledge.Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.documents {
		if d.CollectionID == collectionID && d.RelPath == relPath {
			return d, nil
		}
	}
	return bizknowledge.Document{}, errMemNotFound
}
func (m *vaultSyncMemRepo) UpdateDocumentRelPath(_ context.Context, id, newRelPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.documents[id]
	d.RelPath = newRelPath
	m.documents[id] = d
	return nil
}
func (m *vaultSyncMemRepo) UpdateDocumentSyncMeta(_ context.Context, id string, meta bizknowledge.DocumentSyncMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.documents[id]
	d.ContentHash = meta.ContentHash
	d.Summary = meta.Summary
	d.SummaryHash = meta.SummaryHash
	d.Tags = meta.Tags
	d.DocType = meta.DocType
	m.documents[id] = d
	return nil
}
func (m *vaultSyncMemRepo) UpdateDocumentStatus(_ context.Context, id, status, errMsg string, cc int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.documents[id]
	d.Status = status
	d.ErrorMessage = errMsg
	d.ChunkCount = cc
	m.documents[id] = d
	return nil
}
func (m *vaultSyncMemRepo) UpdateDocumentContent(_ context.Context, id, contentText string, organized bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.documents[id]
	d.ContentText = contentText
	d.Organized = organized
	m.documents[id] = d
	return nil
}
func (m *vaultSyncMemRepo) ListDocuments(_ context.Context, collectionID string, limit, offset int) ([]bizknowledge.Document, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []bizknowledge.Document
	for _, d := range m.documents {
		if d.CollectionID == collectionID {
			all = append(all, d)
		}
	}
	if offset >= len(all) {
		return nil, len(all), nil
	}
	end := offset + limit
	if limit <= 0 || end > len(all) {
		end = len(all)
	}
	return all[offset:end], len(all), nil
}
func (m *vaultSyncMemRepo) DeleteDocument(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.documents[id]
	if !ok {
		return errMemNotFound
	}
	delete(m.documents, id)
	// 模拟生产 repo 的级联计数校正
	c := m.collections[d.CollectionID]
	c.DocumentCount--
	c.ChunkCount -= d.ChunkCount
	m.collections[d.CollectionID] = c
	var kept []bizknowledge.Chunk
	for _, ch := range m.chunks {
		if ch.DocID != id {
			kept = append(kept, ch)
		}
	}
	m.chunks = kept
	return nil
}
func (m *vaultSyncMemRepo) MoveDocument(_ context.Context, id, target string) (bizknowledge.Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.documents[id]
	if !ok {
		return bizknowledge.Document{}, errMemNotFound
	}
	d.CollectionID = target
	m.documents[id] = d
	return d, nil
}
func (m *vaultSyncMemRepo) InsertChunks(_ context.Context, chunks []bizknowledge.Chunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chunks = append(m.chunks, chunks...)
	return nil
}
func (m *vaultSyncMemRepo) DeleteChunksByDocument(_ context.Context, docID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteChunksCalls++
	var kept []bizknowledge.Chunk
	for _, ch := range m.chunks {
		if ch.DocID != docID {
			kept = append(kept, ch)
		}
	}
	m.chunks = kept
	return nil
}
func (m *vaultSyncMemRepo) SearchChunks(_ context.Context, _ bizknowledge.SearchQuery, _ []float32) ([]bizknowledge.Chunk, error) {
	return m.chunks, nil
}

// ── LinkRepo / EntityRepo（P2-4 双轨关联，内存实现） ─────────────────────────

func (m *vaultSyncMemRepo) ReplaceLinks(_ context.Context, collectionID, docID, linkType string, links []bizknowledge.Link) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var kept []bizknowledge.Link
	for _, l := range m.links {
		if l.DocID == docID && l.LinkType == linkType {
			continue
		}
		kept = append(kept, l)
	}
	m.links = append(kept, links...)
	return nil
}
func (m *vaultSyncMemRepo) ListLinks(_ context.Context, collectionID, docID, linkType string) ([]bizknowledge.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []bizknowledge.Link
	for _, l := range m.links {
		if l.CollectionID != collectionID {
			continue
		}
		if l.DocID != docID && l.TargetDocID != docID {
			continue
		}
		if linkType != "" && l.LinkType != linkType {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}
func (m *vaultSyncMemRepo) ReplaceDocEntities(_ context.Context, collectionID, docID string, entities []bizknowledge.DocEntity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docEntities[docID] = entities
	return nil
}
func (m *vaultSyncMemRepo) FindEntityCooccurrences(_ context.Context, collectionID string, entityNames []string, excludeDocID string, maxDocFreq int) ([]bizknowledge.EntityCooccurrence, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[string]bool{}
	for _, n := range entityNames {
		want[n] = true
	}
	// 频次统计（R-3：超 maxDocFreq 的实体视为噪声）
	freq := map[string]int{}
	for docID, ents := range m.docEntities {
		if m.documents[docID].CollectionID != collectionID {
			continue
		}
		seen := map[string]bool{}
		for _, e := range ents {
			if !seen[e.Name] {
				seen[e.Name] = true
				freq[e.Name]++
			}
		}
	}
	shared := map[string][]string{}
	for docID, ents := range m.docEntities {
		if docID == excludeDocID || m.documents[docID].CollectionID != collectionID {
			continue
		}
		for _, e := range ents {
			if !want[e.Name] {
				continue
			}
			if maxDocFreq > 0 && freq[e.Name] > maxDocFreq {
				continue
			}
			shared[docID] = append(shared[docID], e.Name)
		}
	}
	var out []bizknowledge.EntityCooccurrence
	for docID, names := range shared {
		out = append(out, bizknowledge.EntityCooccurrence{DocID: docID, SharedEntities: names})
	}
	return out, nil
}

var errMemNotFound = apierror.NotFound("KNOWLEDGE", "document not found")

// ── stub embedder ─────────────────────────────────────────────────────────────

type vaultSyncStubEmbedder struct {
	dim   int
	calls int
}

func (s *vaultSyncStubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	s.calls++
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, s.dim)
		for j := range v {
			v[j] = 0.1
		}
		out[i] = v
	}
	return out, nil
}
func (s *vaultSyncStubEmbedder) Dim() int { return s.dim }

// ── 测试辅助 ──────────────────────────────────────────────────────────────────

func writeVaultFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func createdEvent(rel, content string) bizknowledge.ChangeEvent {
	return bizknowledge.ChangeEvent{
		Type:    bizknowledge.ChangeCreated,
		RelPath: rel,
		Snapshot: bizknowledge.FileSnapshot{
			RelPath: rel,
			Size:    int64(len(content)),
			Hash:    bizknowledge.HashContent(content),
		},
	}
}

func newTestApplier(repo *vaultSyncMemRepo, embedder Embedder) *VaultSyncApplier {
	uc := bizknowledge.NewUsecaseFromRepo(repo)
	uc.SetLinkRepos(repo, repo) // P2-4：接线双轨关联持久化
	return NewVaultSyncApplier(uc, bizknowledge.NewVaultFiler(nil), embedder, loggateway.NewNoop())
}

const testVaultMD = `---
summary: 量化策略研究笔记
tags: [quant, momentum]
type: note
---

# 动量策略

正文内容：双均线动量回测结论。
`

// ── Created ──────────────────────────────────────────────────────────────────

func TestVaultSyncApplier_Created_NoEmbedder_LexicalOnly(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "notes/a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root, SyncState: "active"}
	repo.collections[vault.ID] = vault

	applier := newTestApplier(repo, nil)
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{createdEvent("notes/a.md", testVaultMD)}); err != nil {
		t.Fatalf("apply created: %v", err)
	}

	if len(repo.documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(repo.documents))
	}
	var doc bizknowledge.Document
	for _, d := range repo.documents {
		doc = d
	}
	if doc.RelPath != "notes/a.md" {
		t.Errorf("RelPath = %q", doc.RelPath)
	}
	if doc.ContentHash != bizknowledge.HashContent(testVaultMD) {
		t.Errorf("ContentHash = %q, want file hash", doc.ContentHash)
	}
	if doc.Summary != "量化策略研究笔记" {
		t.Errorf("Summary = %q, want frontmatter summary", doc.Summary)
	}
	if len(doc.Tags) != 2 || doc.Tags[0] != "quant" {
		t.Errorf("Tags = %v", doc.Tags)
	}
	if doc.DocType != "note" {
		t.Errorf("DocType = %q", doc.DocType)
	}
	if doc.Status != "indexed" {
		t.Errorf("Status = %q, want indexed", doc.Status)
	}
	if doc.ContentText == "" || doc.ContentText == testVaultMD {
		t.Errorf("ContentText should be markdown body without frontmatter, got %q", doc.ContentText)
	}
	if doc.ChunkCount == 0 {
		t.Error("expected chunks to be built")
	}
	if len(repo.chunks) != doc.ChunkCount {
		t.Errorf("stored chunks = %d, doc.ChunkCount = %d", len(repo.chunks), doc.ChunkCount)
	}
	for i, ch := range repo.chunks {
		if len(ch.Embedding) != 0 {
			t.Errorf("chunk %d: embedding must be empty without semantic layer, got %d dims", i, len(ch.Embedding))
		}
	}
	col := repo.collections[vault.ID]
	if col.DocumentCount != 1 || col.ChunkCount != doc.ChunkCount {
		t.Errorf("collection counts = (%d,%d), want (1,%d)", col.DocumentCount, col.ChunkCount, doc.ChunkCount)
	}
}

func TestVaultSyncApplier_Created_WithEmbedder_EmbedsChunks(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root, EmbeddingModel: "text-embedding-3-small", Dim: 3}
	repo.collections[vault.ID] = vault

	emb := &vaultSyncStubEmbedder{dim: 3}
	applier := newTestApplier(repo, emb)
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{createdEvent("a.md", testVaultMD)}); err != nil {
		t.Fatalf("apply created: %v", err)
	}
	if emb.calls == 0 {
		t.Fatal("embedder must be called when collection has semantic layer")
	}
	for i, ch := range repo.chunks {
		if len(ch.Embedding) != 3 {
			t.Errorf("chunk %d: embedding dims = %d, want 3", i, len(ch.Embedding))
		}
	}
}

// 无语义层 collection 即使配了 embedder 也不调用（R-4 降级）。
func TestVaultSyncApplier_Created_NoSemanticLayer_SkipsEmbedder(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root} // EmbeddingModel 空
	repo.collections[vault.ID] = vault

	emb := &vaultSyncStubEmbedder{dim: 3}
	applier := newTestApplier(repo, emb)
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{createdEvent("a.md", testVaultMD)}); err != nil {
		t.Fatalf("apply created: %v", err)
	}
	if emb.calls != 0 {
		t.Errorf("embedder must not be called for vault without embedding_model, calls=%d", emb.calls)
	}
}

// ── Modified ─────────────────────────────────────────────────────────────────

func TestVaultSyncApplier_Modified_RebuildsChunksAndHash(t *testing.T) {
	root := t.TempDir()
	const oldContent = "# 旧版本\n\n旧内容。"
	const newContent = "---\ntype: report\n---\n\n# 新版本\n\n全新内容，更长一些。"
	writeVaultFile(t, root, "a.md", newContent)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault
	existing := bizknowledge.Document{
		ID: "doc1", CollectionID: vault.ID, RelPath: "a.md", Source: "a.md",
		ContentHash: bizknowledge.HashContent(oldContent), ContentText: "旧内容。",
		Status: "indexed", ChunkCount: 1,
	}
	repo.documents[existing.ID] = existing
	repo.chunks = append(repo.chunks, bizknowledge.Chunk{ID: "old-ch", DocID: "doc1", CollectionID: vault.ID, Content: "旧内容。", ChunkIndex: 0})
	col := repo.collections[vault.ID]
	col.DocumentCount = 1
	col.ChunkCount = 1
	repo.collections[vault.ID] = col

	applier := newTestApplier(repo, nil)
	ev := bizknowledge.ChangeEvent{
		Type:    bizknowledge.ChangeModified,
		RelPath: "a.md",
		Snapshot: bizknowledge.FileSnapshot{
			RelPath: "a.md", Size: int64(len(newContent)), Hash: bizknowledge.HashContent(newContent),
		},
	}
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{ev}); err != nil {
		t.Fatalf("apply modified: %v", err)
	}

	doc := repo.documents["doc1"]
	if doc.ContentHash != bizknowledge.HashContent(newContent) {
		t.Errorf("ContentHash not updated: %q", doc.ContentHash)
	}
	if doc.DocType != "report" {
		t.Errorf("DocType = %q, want report (from new frontmatter)", doc.DocType)
	}
	if repo.deleteChunksCalls != 1 {
		t.Errorf("old chunks must be deleted once, calls=%d", repo.deleteChunksCalls)
	}
	for _, ch := range repo.chunks {
		if ch.ID == "old-ch" {
			t.Error("old chunk must be replaced")
		}
	}
	newCol := repo.collections[vault.ID]
	if newCol.DocumentCount != 1 {
		t.Errorf("DocumentCount drifted: %d", newCol.DocumentCount)
	}
	if newCol.ChunkCount != doc.ChunkCount {
		t.Errorf("ChunkCount = %d, want %d", newCol.ChunkCount, doc.ChunkCount)
	}
}

// ── Deleted ──────────────────────────────────────────────────────────────────

func TestVaultSyncApplier_Deleted_RemovesMirror_Idempotent(t *testing.T) {
	root := t.TempDir()
	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault
	repo.documents["doc1"] = bizknowledge.Document{ID: "doc1", CollectionID: vault.ID, RelPath: "gone.md", Status: "indexed", ChunkCount: 2}
	col := repo.collections[vault.ID]
	col.DocumentCount = 1
	col.ChunkCount = 2
	repo.collections[vault.ID] = col

	applier := newTestApplier(repo, nil)
	ev := bizknowledge.ChangeEvent{Type: bizknowledge.ChangeDeleted, RelPath: "gone.md"}
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{ev}); err != nil {
		t.Fatalf("apply deleted: %v", err)
	}
	if len(repo.documents) != 0 {
		t.Errorf("document mirror must be removed, got %d", len(repo.documents))
	}
	// 幂等：再次删除同一 relPath 不报错
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{ev}); err != nil {
		t.Fatalf("delete must be idempotent: %v", err)
	}
}

func TestVaultSyncApplier_Deleted_RescuesMirrorToTrash(t *testing.T) {
	root := t.TempDir()
	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault
	repo.documents["doc1"] = bizknowledge.Document{
		ID: "doc1", CollectionID: vault.ID, RelPath: "notes/gone.md",
		Status: "indexed", ContentText: "被外部删除的正文",
		Summary: "镜像摘要", Tags: []string{"t1"}, DocType: "note",
	}

	applier := newTestApplier(repo, nil)
	ev := bizknowledge.ChangeEvent{Type: bizknowledge.ChangeDeleted, RelPath: "notes/gone.md"}
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{ev}); err != nil {
		t.Fatalf("apply deleted: %v", err)
	}

	// 镜像删除 + 内容抢救进 trash（R-2：外部删除不丢数据）
	if len(repo.documents) != 0 {
		t.Fatalf("document mirror must be removed, got %d", len(repo.documents))
	}
	data, err := os.ReadFile(filepath.Join(root, ".aranea", "trash", "notes", "gone.md"))
	if err != nil {
		t.Fatalf("mirror content must be rescued to trash: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "被外部删除的正文") {
		t.Errorf("trash copy must contain mirror body, got:\n%s", content)
	}
	if !strings.Contains(content, "镜像摘要") {
		t.Errorf("trash copy must contain mirror summary, got:\n%s", content)
	}
	// 原路径不得复活（尊重用户删除意图）
	if _, err := os.Stat(filepath.Join(root, "notes", "gone.md")); !os.IsNotExist(err) {
		t.Errorf("original path must not be resurrected, stat err=%v", err)
	}
}

// ── Moved ────────────────────────────────────────────────────────────────────

func TestVaultSyncApplier_Moved_KeepsIdentityAndChunks(t *testing.T) {
	root := t.TempDir()
	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault
	repo.documents["doc1"] = bizknowledge.Document{ID: "doc1", CollectionID: vault.ID, RelPath: "old.md", Status: "indexed", ChunkCount: 1}
	repo.chunks = append(repo.chunks, bizknowledge.Chunk{ID: "ch1", DocID: "doc1", CollectionID: vault.ID, Content: "x"})

	applier := newTestApplier(repo, nil)
	ev := bizknowledge.ChangeEvent{Type: bizknowledge.ChangeMoved, RelPath: "dir/new.md", OldRelPath: "old.md"}
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{ev}); err != nil {
		t.Fatalf("apply moved: %v", err)
	}
	doc := repo.documents["doc1"]
	if doc.RelPath != "dir/new.md" {
		t.Errorf("RelPath = %q, want dir/new.md", doc.RelPath)
	}
	if repo.deleteChunksCalls != 0 {
		t.Errorf("chunks must survive a move, deleteChunksCalls=%d", repo.deleteChunksCalls)
	}
	if len(repo.chunks) != 1 {
		t.Errorf("chunks must be kept, got %d", len(repo.chunks))
	}
}

// ── Summary Hook（P2-2）──────────────────────────────────────────────────────

func TestVaultSyncApplier_SummaryHookTriggeredOnStale(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", "# 无摘要文档\n\n正文。\n")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	applier := newTestApplier(repo, nil)
	var hooked []string
	applier.SetSummaryHook(func(_, relPath string) { hooked = append(hooked, relPath) })

	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{
		createdEvent("a.md", "# 无摘要文档\n\n正文。\n"),
	}); err != nil {
		t.Fatalf("apply created: %v", err)
	}
	if len(hooked) != 1 || hooked[0] != "a.md" {
		t.Errorf("hooked = %v, want [a.md]（无 summary_hash 视为 stale 必须触发）", hooked)
	}
}

func TestVaultSyncApplier_SummaryHookSkippedWhenFresh(t *testing.T) {
	body := "# 有摘要文档\n\n正文。\n"
	content := "---\nsummary: 已有摘要\nsummary_hash: " + bizknowledge.HashContent(body) + "\n---\n\n" + body

	root := t.TempDir()
	writeVaultFile(t, root, "a.md", content)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	applier := newTestApplier(repo, nil)
	hookCalls := 0
	applier.SetSummaryHook(func(_, _ string) { hookCalls++ })

	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{
		createdEvent("a.md", content),
	}); err != nil {
		t.Fatalf("apply created: %v", err)
	}
	if hookCalls != 0 {
		t.Errorf("hookCalls = %d, want 0（summary_hash 与 body 匹配不触发）", hookCalls)
	}
}

func TestVaultSyncApplier_SummaryHookSkippedWhenUnset(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", "# 无摘要文档\n\n正文。\n")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	// 未设置 hook：stale 检测不得 panic，索引正常完成
	applier := newTestApplier(repo, nil)
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{
		createdEvent("a.md", "# 无摘要文档\n\n正文。\n"),
	}); err != nil {
		t.Fatalf("apply created without hook: %v", err)
	}
	if len(repo.documents) != 1 {
		t.Errorf("expected 1 document, got %d", len(repo.documents))
	}
}

// Moved 但 DB 无原路径镜像 → 兜底按 Created 处理（索引自愈）。
func TestVaultSyncApplier_Moved_NotFound_FallsBackToCreated(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "new.md", testVaultMD)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	applier := newTestApplier(repo, nil)
	ev := bizknowledge.ChangeEvent{
		Type: bizknowledge.ChangeMoved, RelPath: "new.md", OldRelPath: "missing.md",
		Snapshot: bizknowledge.FileSnapshot{RelPath: "new.md", Size: int64(len(testVaultMD)), Hash: bizknowledge.HashContent(testVaultMD)},
	}
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{ev}); err != nil {
		t.Fatalf("apply moved-fallback: %v", err)
	}
	if len(repo.documents) != 1 {
		t.Fatalf("fallback must create mirror, got %d docs", len(repo.documents))
	}
}

// ── 空文件 ───────────────────────────────────────────────────────────────────

func TestVaultSyncApplier_EmptyFile_IndexedWithZeroChunks(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "empty.md", "")

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	applier := newTestApplier(repo, nil)
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{createdEvent("empty.md", "")}); err != nil {
		t.Fatalf("apply created empty: %v", err)
	}
	if len(repo.documents) != 1 {
		t.Fatalf("empty file must still be mirrored, got %d", len(repo.documents))
	}
	for _, d := range repo.documents {
		if d.Status != "indexed" {
			t.Errorf("Status = %q, want indexed", d.Status)
		}
		if d.ChunkCount != 0 {
			t.Errorf("ChunkCount = %d, want 0", d.ChunkCount)
		}
	}
	if len(repo.chunks) != 0 {
		t.Errorf("no chunks expected for empty file, got %d", len(repo.chunks))
	}
}

// ── Explicit 双链（P2-4） ────────────────────────────────────────────────────

// 目标文档先索引（引用解析候选来自 DB），引用方后索引时建 explicit 链。
func TestVaultSyncApplier_ExplicitLinkBuilt(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "target.md", "# 目标文档\n\n被动量策略引用。\n")
	body := "# 动量笔记\n\n详见 [[target]] 与 [[不存在]]。\n"
	writeVaultFile(t, root, "src.md", body)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	applier := newTestApplier(repo, nil)
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{
		createdEvent("target.md", "# 目标文档\n\n被动量策略引用。\n"),
		createdEvent("src.md", body),
	}); err != nil {
		t.Fatalf("apply created: %v", err)
	}

	var srcID string
	for _, d := range repo.documents {
		if d.RelPath == "src.md" {
			srcID = d.ID
		}
	}
	if srcID == "" {
		t.Fatal("src.md must be mirrored")
	}
	links, err := repo.ListLinks(context.Background(), vault.ID, srcID, bizknowledge.LinkTypeExplicit)
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 explicit link (悬空引用不建链), got %d: %+v", len(links), links)
	}
	l := links[0]
	if l.DocID != srcID || l.LinkType != bizknowledge.LinkTypeExplicit {
		t.Errorf("link doc/type mismatch: %+v", l)
	}
	if l.Context != "target" {
		t.Errorf("Context = %q, want 原始 ref 'target'", l.Context)
	}
	targetID := ""
	for _, d := range repo.documents {
		if d.RelPath == "target.md" {
			targetID = d.ID
		}
	}
	if l.TargetDocID != targetID {
		t.Errorf("TargetDocID = %q, want %q", l.TargetDocID, targetID)
	}
	// 双向可见：从 target 侧也能查到
	backlinks, err := repo.ListLinks(context.Background(), vault.ID, targetID, "")
	if err != nil {
		t.Fatalf("list backlinks: %v", err)
	}
	if len(backlinks) != 1 {
		t.Errorf("target must see 1 backlink, got %d", len(backlinks))
	}
}

// 引用变更后 explicit 链重建（删旧插新）；引用全部移除后链接清空。
func TestVaultSyncApplier_ExplicitLinkRebuiltOnModify(t *testing.T) {
	root := t.TempDir()
	writeVaultFile(t, root, "a.md", "# A\n")
	bodyV1 := "# 笔记\n\n见 [[a]]。\n"
	writeVaultFile(t, root, "src.md", bodyV1)

	repo := newVaultSyncMemRepo()
	vault := bizknowledge.Collection{ID: "col1", RootPath: root}
	repo.collections[vault.ID] = vault

	applier := newTestApplier(repo, nil)
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{
		createdEvent("a.md", "# A\n"),
		createdEvent("src.md", bodyV1),
	}); err != nil {
		t.Fatalf("apply created: %v", err)
	}
	var srcID string
	for _, d := range repo.documents {
		if d.RelPath == "src.md" {
			srcID = d.ID
		}
	}
	links, _ := repo.ListLinks(context.Background(), vault.ID, srcID, "")
	if len(links) != 1 {
		t.Fatalf("v1: expected 1 link, got %d", len(links))
	}

	// 修改：移除引用
	bodyV2 := "# 笔记\n\n不再引用任何文档。\n"
	writeVaultFile(t, root, "src.md", bodyV2)
	if err := applier.ApplyEvents(context.Background(), vault, []bizknowledge.ChangeEvent{
		{Type: bizknowledge.ChangeModified, RelPath: "src.md", Snapshot: bizknowledge.FileSnapshot{
			RelPath: "src.md", Size: int64(len(bodyV2)), Hash: bizknowledge.HashContent(bodyV2),
		}},
	}); err != nil {
		t.Fatalf("apply modified: %v", err)
	}
	links, _ = repo.ListLinks(context.Background(), vault.ID, srcID, "")
	if len(links) != 0 {
		t.Errorf("v2: 引用移除后链接必须清空, got %d", len(links))
	}
}

// 未接线 LinkRepo 时关联方法降级 no-op（不影响索引主流程）。
func TestVaultUsecase_LinksDegradeWhenUnset(t *testing.T) {
	repo := newVaultSyncMemRepo()
	uc := bizknowledge.NewUsecaseFromRepo(repo) // 不调 SetLinkRepos
	if err := uc.ReplaceExplicitLinks(context.Background(), "c1", "d1", []bizknowledge.Link{{DocID: "d1", TargetDocID: "d2", LinkType: "explicit"}}); err != nil {
		t.Fatalf("unset links must degrade to nil error, got %v", err)
	}
	links, err := uc.ListDocumentLinks(context.Background(), "c1", "d1", "")
	if err != nil || len(links) != 0 {
		t.Errorf("unset links: ListDocumentLinks = (%v, %v), want (nil, empty)", links, err)
	}
}
