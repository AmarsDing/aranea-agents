package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
)

// ── SP1-H H-2：惰性锚点回填执行侧（backfillAnchors） ─────────────────────────
//
// 口径（设计 S2/F-SP1-10）：
//   - team 库 / 非 vault 文档：PG content_text 为真相源，落锚 = UpdateDocumentContent + 重索引；
//   - local 库：文件系统为真相源，落锚 = VaultFiler CAS 写文件 + 镜像正文/hash 同步 + 重索引；
//   - 幂等：已锚块 AppendHeadingAnchor 复查跳过；同一文件多次解析锚点稳定；
//   - 一跳即止：回填自触发重索引 allowBackfill=false，不级联改写第三方文档；
//   - best-effort：单请求失败仅记日志，其余请求照常处理。

// backfillFixture 装配回填执行侧测试环境：可感知状态的文档/集合存根 + 内存块索引。
type backfillFixture struct {
	repo           *mockRepo
	idx            *memBlockIndex
	u              *Usecase
	docs           map[string]Document
	collections    map[string]Collection
	contentUpdates []string           // docID（按调用序）
	syncMetas      []DocumentSyncMeta // 与 contentUpdates 平行记录 hash 同步
	candidatesFn   func() []ResolveDocCandidate
}

func newBackfillFixture() *backfillFixture {
	f := &backfillFixture{
		docs:        map[string]Document{},
		collections: map[string]Collection{},
	}
	repo := noOpMockRepo()
	repo.docGetFn = func(_ context.Context, id string) (Document, error) {
		d, ok := f.docs[id]
		if !ok {
			return Document{}, apierror.NotFound("KNOWLEDGE", "doc not found: %s", id)
		}
		return d, nil
	}
	repo.collGetFn = func(_ context.Context, id string) (Collection, error) {
		c, ok := f.collections[id]
		if !ok {
			return Collection{}, apierror.NotFound("KNOWLEDGE", "collection not found: %s", id)
		}
		return c, nil
	}
	repo.collListFn = func(_ context.Context, _ string, _, _ int) ([]Collection, int, error) {
		out := make([]Collection, 0, len(f.collections))
		for _, c := range f.collections {
			out = append(out, c)
		}
		return out, len(out), nil
	}
	repo.docContentFn = func(_ context.Context, id, contentText string, organized bool) error {
		d := f.docs[id]
		d.ContentText = contentText
		d.Organized = organized
		f.docs[id] = d
		f.contentUpdates = append(f.contentUpdates, id)
		return nil
	}
	repo.docSyncMetaFn = func(_ context.Context, _ string, meta DocumentSyncMeta) error {
		f.syncMetas = append(f.syncMetas, meta)
		return nil
	}
	repo.docListFn = func(_ context.Context, collectionID string, limit, offset int) ([]Document, int, error) {
		var all []Document
		for _, d := range f.docs {
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
	f.repo = repo
	f.idx = newMemBlockIndex(func() []ResolveDocCandidate {
		if f.candidatesFn != nil {
			return f.candidatesFn()
		}
		return nil
	})
	u := NewUsecaseFromRepo(repo)
	u.SetBlockIndexRepos(f.idx, f.idx)
	f.u = u
	return f
}

// anchoredBlocks 返回文档已锚 heading 块的锚文本集。
func anchoredBlocks(idx *memBlockIndex, docID string) []string {
	var out []string
	for _, b := range idx.blocks[docID] {
		if b.Kind == "heading" && b.Anchor != "" {
			out = append(out, b.Anchor)
		}
	}
	return out
}

// TestBackfillAnchors_TeamDoc team 库目标：content_text 行尾落锚 + 目标重索引锚定；
// 二次回填幂等（已锚跳过，不再写内容）。
func TestBackfillAnchors_TeamDoc(t *testing.T) {
	f := newBackfillFixture()
	f.collections["c2"] = Collection{ID: "c2", Workspace: "w", VaultBackend: VaultBackendTeam}
	f.docs["d-t"] = Document{ID: "d-t", CollectionID: "c2", Organized: true,
		ContentText: "# Alpha\n\n正文。\n\n## Beta\n\n子节。\n"}

	req := AnchorBackfillRequest{CollectionID: "c2", DocID: "d-t", HeadingPath: []string{"Alpha", "Beta"}}
	f.u.backfillAnchors(context.Background(), []AnchorBackfillRequest{req})

	if len(f.contentUpdates) != 1 || f.contentUpdates[0] != "d-t" {
		t.Fatalf("contentUpdates = %v, want [d-t]", f.contentUpdates)
	}
	body := f.docs["d-t"].ContentText
	if !strings.Contains(body, "## Beta ^") {
		t.Errorf("落锚后正文缺锚: %q", body)
	}
	anchors := anchoredBlocks(f.idx, "d-t")
	if len(anchors) != 1 {
		t.Fatalf("目标重索引后锚块 = %v, want 1 个", anchors)
	}
	if !strings.Contains(body, "^"+anchors[0]) {
		t.Errorf("正文锚文本与块锚不一致: body=%q anchor=%q", body, anchors[0])
	}
	// 幂等：二次回填同一路径不再写内容（AppendHeadingAnchor 复查跳过）。
	f.u.backfillAnchors(context.Background(), []AnchorBackfillRequest{req})
	if len(f.contentUpdates) != 1 {
		t.Errorf("幂等失败：二次回填仍写内容 %v", f.contentUpdates)
	}
	if got := anchoredBlocks(f.idx, "d-t"); len(got) != 1 || got[0] != anchors[0] {
		t.Errorf("锚点漂移: %v → %v", anchors, got)
	}
}

// TestBackfillAnchors_NoCascade 一跳即止：回填自触发的目标重索引即便检出新的
// 未锚命中，也不得级联回填第三方文档。
func TestBackfillAnchors_NoCascade(t *testing.T) {
	f := newBackfillFixture()
	f.collections["c2"] = Collection{ID: "c2", Workspace: "w", VaultBackend: VaultBackendTeam}
	// d-t 的 Beta 未锚（回填对象）；其正文还引用 d-c 的未锚 Gamma（级联诱饵）。
	f.docs["d-t"] = Document{ID: "d-t", CollectionID: "c2", Organized: true,
		ContentText: "# Alpha\n\n## Beta\n\n见 [[c-doc#Gamma]]\n"}
	f.docs["d-c"] = Document{ID: "d-c", CollectionID: "c2", Organized: true,
		ContentText: "# Gamma\n\n诱饵。\n"}
	f.candidatesFn = func() []ResolveDocCandidate {
		return []ResolveDocCandidate{{DocID: "d-c", CollectionID: "c2", RelPath: "c-doc.md"}}
	}
	// d-c 已被索引（未锚），Resolver 重索引 d-t 时可命中并检出回填请求。
	f.idx.blocks["d-c"] = []KnowledgeBlock{{Ordinal: 0, Kind: "heading", HeadingPath: []string{"Gamma"}}}

	f.u.backfillAnchors(context.Background(), []AnchorBackfillRequest{
		{CollectionID: "c2", DocID: "d-t", HeadingPath: []string{"Alpha", "Beta"}},
	})

	for _, id := range f.contentUpdates {
		if id == "d-c" {
			t.Error("级联回填：第三方文档 d-c 被改写")
		}
	}
	if len(anchoredBlocks(f.idx, "d-t")) != 1 {
		t.Error("d-t 自身回填未生效")
	}
	if len(anchoredBlocks(f.idx, "d-c")) != 0 {
		t.Error("d-c 不应被锚定")
	}
}

// TestBackfillAnchors_BestEffort 单请求失败（目标消失）不阻塞其余请求。
func TestBackfillAnchors_BestEffort(t *testing.T) {
	f := newBackfillFixture()
	f.collections["c2"] = Collection{ID: "c2", Workspace: "w", VaultBackend: VaultBackendTeam}
	f.docs["d-ok"] = Document{ID: "d-ok", CollectionID: "c2", ContentText: "# H\n"}

	f.u.backfillAnchors(context.Background(), []AnchorBackfillRequest{
		{CollectionID: "c2", DocID: "d-gone", HeadingPath: []string{"H"}}, // 不存在 → 失败
		{CollectionID: "c2", DocID: "d-ok", HeadingPath: []string{"H"}},
	})
	if len(f.contentUpdates) != 1 || f.contentUpdates[0] != "d-ok" {
		t.Errorf("失败请求阻塞了后续回填: %v", f.contentUpdates)
	}
}

// TestBackfillAnchors_LocalDoc local 库目标：VaultFiler CAS 写文件落锚 + 镜像
// 正文/hash 同步（hash 同步后下轮轮询幂等短路，不重算 embedding）+ 目标重索引。
func TestBackfillAnchors_LocalDoc(t *testing.T) {
	root := t.TempDir()
	orig := "# Alpha\n\n## Beta\n\n内容。\n"
	if err := os.WriteFile(filepath.Join(root, "b.md"), []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newBackfillFixture()
	f.collections["c1"] = Collection{ID: "c1", Workspace: "w", VaultBackend: VaultBackendLocal, RootPath: root}
	f.docs["d-t"] = Document{ID: "d-t", CollectionID: "c1", RelPath: "b.md", Organized: true, ContentText: orig}
	f.u.SetVaultFiler(NewVaultFiler(nil))

	f.u.backfillAnchors(context.Background(), []AnchorBackfillRequest{
		{CollectionID: "c1", DocID: "d-t", HeadingPath: []string{"Alpha", "Beta"}},
	})

	disk, err := os.ReadFile(filepath.Join(root, "b.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(disk), "## Beta ^") {
		t.Fatalf("vault 文件未落锚: %q", disk)
	}
	if len(f.contentUpdates) != 1 || f.contentUpdates[0] != "d-t" {
		t.Errorf("镜像正文未同步: %v", f.contentUpdates)
	}
	if !strings.Contains(f.docs["d-t"].ContentText, "## Beta ^") {
		t.Errorf("镜像 ContentText 缺锚: %q", f.docs["d-t"].ContentText)
	}
	// 文件 hash 同步：下轮轮询按 hash 幂等短路（chunks/embedding 不重建）。
	if len(f.syncMetas) != 1 || f.syncMetas[0].ContentHash != HashContent(string(disk)) {
		t.Errorf("文件 hash 未同步: %+v, want %q", f.syncMetas, HashContent(string(disk)))
	}
	if len(anchoredBlocks(f.idx, "d-t")) != 1 {
		t.Error("目标块索引未锚定")
	}
}

// TestRebuildBlockIndex_BackfillWritePath 写路径端到端：源文档重建检出
// heading-path 引用命中未锚块 → 目标文档落锚并重索引。
func TestRebuildBlockIndex_BackfillWritePath(t *testing.T) {
	f := newBackfillFixture()
	f.collections["c1"] = Collection{ID: "c1", Workspace: "w", VaultBackend: VaultBackendLocal}
	f.collections["c2"] = Collection{ID: "c2", Workspace: "w", VaultBackend: VaultBackendTeam}
	f.docs["d-t"] = Document{ID: "d-t", CollectionID: "c2", RelPath: "note.md", Organized: true,
		ContentText: "# Alpha\n\n## Beta\n\n子节。\n"}
	f.candidatesFn = func() []ResolveDocCandidate {
		return []ResolveDocCandidate{{DocID: "d-t", CollectionID: "c2", RelPath: "note.md"}}
	}
	// 目标已被索引但未锚（模拟 DB 镜像现状）。
	f.idx.blocks["d-t"] = []KnowledgeBlock{
		{Ordinal: 0, Kind: "heading", HeadingPath: []string{"Alpha"}},
		{Ordinal: 1, Kind: "heading", HeadingPath: []string{"Alpha", "Beta"}},
	}

	if err := f.u.RebuildBlockIndex(context.Background(), "c1", "d-s", "见 [[note#Alpha#Beta]]\n"); err != nil {
		t.Fatalf("RebuildBlockIndex: %v", err)
	}
	// 源边物化：doc 级解析命中。
	refs := f.idx.refs["d-s"]
	if len(refs) != 1 || refs[0].DstDocID != "d-t" {
		t.Fatalf("源边解析 = %+v, want dst d-t", refs)
	}
	// 目标落锚（回填副作用）。
	if !strings.Contains(f.docs["d-t"].ContentText, "## Beta ^") {
		t.Errorf("写路径未触发回填: %q", f.docs["d-t"].ContentText)
	}
	if len(anchoredBlocks(f.idx, "d-t")) != 1 {
		t.Error("目标重索引后未锚定")
	}
}

// TestRebuildCollectionBlockIndex_NoBackfill 全量重建不改源文本（allowBackfill=false）：
// 即便检出未锚命中也不回填。
func TestRebuildCollectionBlockIndex_NoBackfill(t *testing.T) {
	f := newBackfillFixture()
	f.collections["c1"] = Collection{ID: "c1", Workspace: "w", VaultBackend: VaultBackendTeam, SyncState: "active"}
	f.docs["d-s"] = Document{ID: "d-s", CollectionID: "c1", ContentText: "见 [[note#Alpha]]\n"}
	f.docs["d-t"] = Document{ID: "d-t", CollectionID: "c1", RelPath: "note.md", ContentText: "# Alpha\n"}
	f.candidatesFn = func() []ResolveDocCandidate {
		return []ResolveDocCandidate{{DocID: "d-t", CollectionID: "c1", RelPath: "note.md"}}
	}
	f.idx.blocks["d-t"] = []KnowledgeBlock{{Ordinal: 0, Kind: "heading", HeadingPath: []string{"Alpha"}}}

	res, err := f.u.RebuildCollectionBlockIndex(context.Background(), "c1", nil)
	if err != nil {
		t.Fatalf("RebuildCollectionBlockIndex: %v", err)
	}
	if res.Done != 2 || res.Failed != 0 {
		t.Errorf("res = %+v, want done=2 failed=0", res)
	}
	if len(f.contentUpdates) != 0 {
		t.Errorf("全量重建不应改写源文本: %v", f.contentUpdates)
	}
}
