package knowledge

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
)

// rebuildFixture 装配含 docs 的集合 + 记录型块索引。返回 rebuild 调用所需句柄。
// 解析候选由 docs 派生（rel_path = 文档 ID），模拟 DB 镜像最终一致视图。
// 投影分层与真实 repo 对齐（2026-08-09 事故后修正）：docListFn 返回摘要
// （剥掉 ContentText），docGetFn 才返回含正文的完整文档。
func rebuildFixture(docs ...Document) (*Usecase, *mockRepo, *memBlockIndex) {
	repo := noOpMockRepo()
	repo.collGetFn = func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, VaultBackend: VaultBackendLocal, SyncState: "active"}, nil
	}
	repo.collListFn = func(_ context.Context, _ string, _, _ int) ([]Collection, int, error) {
		return []Collection{{ID: "c1", VaultBackend: VaultBackendLocal}}, 1, nil
	}
	repo.docListFn = func(_ context.Context, _ string, limit, offset int) ([]Document, int, error) {
		if offset >= len(docs) {
			return nil, len(docs), nil
		}
		end := offset + limit
		if end > len(docs) {
			end = len(docs)
		}
		// 摘要投影：真实 ListDocuments 不 SELECT content_text。
		page := make([]Document, 0, end-offset)
		for _, d := range docs[offset:end] {
			d.ContentText = ""
			page = append(page, d)
		}
		return page, len(docs), nil
	}
	repo.docGetFn = func(_ context.Context, id string) (Document, error) {
		for _, d := range docs {
			if d.ID == id {
				return d, nil
			}
		}
		return Document{}, apierror.NotFound("KNOWLEDGE", "doc not found: %s", id)
	}
	idx := newMemBlockIndex(func() []ResolveDocCandidate {
		out := make([]ResolveDocCandidate, 0, len(docs))
		for _, d := range docs {
			out = append(out, ResolveDocCandidate{DocID: d.ID, CollectionID: d.CollectionID, RelPath: d.ID})
		}
		return out
	})
	u := NewUsecaseFromRepo(repo)
	u.SetBlockIndexRepos(idx, idx)
	return u, repo, idx
}

// TestRebuildCollectionBlockIndex_AllDocs 全部文档逐批重建（content_text →
// 解析 → 物化）；sync_state rebuilding 进入、完成后恢复 active；进度回调单调。
func TestRebuildCollectionBlockIndex_AllDocs(t *testing.T) {
	docs := []Document{
		{ID: "d1", CollectionID: "c1", ContentText: "---\ntitle: A\n---\n\n# A\n\n[[d2]]\n"},
		{ID: "d2", CollectionID: "c1", ContentText: "# B\n\n正文。\n"},
		{ID: "d3", CollectionID: "c1", ContentText: "---\ntitle: C\n---\n\n# C\n\n[[d1#A]]\n"},
	}
	u, repo, idx := rebuildFixture(docs...)
	var states []string
	repo.collSyncFn = func(_ context.Context, _, state string, _ time.Time) error {
		states = append(states, state)
		return nil
	}
	type prog struct{ done, total, failed int }
	var progress []prog
	res, err := u.RebuildCollectionBlockIndex(context.Background(), "c1", func(done, total, failed int) {
		progress = append(progress, prog{done, total, failed})
	})
	if err != nil {
		t.Fatalf("RebuildCollectionBlockIndex: %v", err)
	}
	if res.Total != 3 || res.Done != 3 || res.Failed != 0 {
		t.Errorf("res = %+v, want total=3 done=3 failed=0", res)
	}
	for _, d := range docs {
		if len(idx.blocks[d.ID]) == 0 {
			t.Errorf("doc %s 块索引未重建", d.ID)
		}
	}
	// 解析键物化（可见性候选应一次性解析，非逐文档重查）。
	if idx.titles["d1"] != "A" || idx.titles["d3"] != "C" {
		t.Errorf("解析键未物化: %v", idx.titles)
	}
	// sync_state：rebuilding → active（恢复）。
	if len(states) != 2 || states[0] != SyncStateRebuilding || states[1] != "active" {
		t.Errorf("sync states = %v, want [rebuilding active]", states)
	}
	// 进度：每次回调 total=3，done 递增。
	if len(progress) != 3 || progress[2].done != 3 || progress[0].total != 3 {
		t.Errorf("progress = %v", progress)
	}
}

// TestRebuildCollectionBlockIndex_Idempotent 幂等可重入：重跑结果一致
// （整文档删了重插语义，refs 无孤儿边）。
func TestRebuildCollectionBlockIndex_Idempotent(t *testing.T) {
	docs := []Document{
		{ID: "d1", CollectionID: "c1", ContentText: "# A\n\n[[d2]]\n"},
		{ID: "d2", CollectionID: "c1", ContentText: "# B\n\n[[d1#A]]\n"},
	}
	u, _, idx := rebuildFixture(docs...)
	if _, err := u.RebuildCollectionBlockIndex(context.Background(), "c1", nil); err != nil {
		t.Fatal(err)
	}
	firstBlocks := idx.blocks["d2"]
	firstRefs := idx.refs["d2"]
	if _, err := u.RebuildCollectionBlockIndex(context.Background(), "c1", nil); err != nil {
		t.Fatal(err)
	}
	if len(idx.blocks["d2"]) != len(firstBlocks) || len(idx.refs["d2"]) != len(firstRefs) {
		t.Error("重跑后块/边数量漂移")
	}
	// 重跑后边解析结果稳定（d2 → d1#A 文档级+块级解析一致）。
	if len(idx.refs["d2"]) != 1 || idx.refs["d2"][0].DstDocID != "d1" {
		t.Errorf("refs[d2] = %+v", idx.refs["d2"])
	}
}

// TestRebuildCollectionBlockIndex_DocFailureContinues 单文档失败不阻塞整批：
// 计数 failed，其余文档完成；sync_state 仍恢复；返回 nil（降级语义同 vault sync）。
func TestRebuildCollectionBlockIndex_DocFailureContinues(t *testing.T) {
	docs := []Document{
		{ID: "d1", CollectionID: "c1", ContentText: "# A\n"},
		{ID: "d2", CollectionID: "c1", ContentText: "# B\n"},
	}
	u, repo, idx := rebuildFixture(docs...)
	var states []string
	repo.collSyncFn = func(_ context.Context, _, state string, _ time.Time) error {
		states = append(states, state)
		return nil
	}
	failIdx := &failOnceBlockIndex{memBlockIndex: idx, failDoc: "d1"}
	u.SetBlockIndexRepos(failIdx, idx)
	res, err := u.RebuildCollectionBlockIndex(context.Background(), "c1", nil)
	if err != nil {
		t.Fatalf("单文档失败不应上抛: %v", err)
	}
	if res.Done != 1 || res.Failed != 1 || res.Total != 2 {
		t.Errorf("res = %+v, want done=1 failed=1 total=2", res)
	}
	if len(idx.blocks["d2"]) == 0 {
		t.Error("d2 应完成重建")
	}
	if len(states) != 2 || states[1] != "active" {
		t.Errorf("sync states = %v（失败也应恢复）", states)
	}
}

// TestRebuildCollectionBlockIndex_NotFound / 未接线降级。
func TestRebuildCollectionBlockIndex_NotFound(t *testing.T) {
	u, repo, _ := rebuildFixture()
	repo.collGetFn = func(_ context.Context, _ string) (Collection, error) {
		return Collection{}, apierror.NotFound("KNOWLEDGE", "collection not found")
	}
	if _, err := u.RebuildCollectionBlockIndex(context.Background(), "ghost", nil); err == nil {
		t.Error("集合不存在应报错")
	}
	u2 := NewUsecaseFromRepo(noOpMockRepo()) // 未接线块索引
	if _, err := u2.RebuildCollectionBlockIndex(context.Background(), "c1", nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("未接线 = %v, want ErrUnavailable", err)
	}
}

// TestRebuildCollectionBlockIndex_VisibleHoisted 可见集合集整批只解析一次
// （N 文档不放大为 N 次 workspace 扫描）。
func TestRebuildCollectionBlockIndex_VisibleHoisted(t *testing.T) {
	docs := []Document{
		{ID: "d1", CollectionID: "c1", ContentText: "[[x]]\n"},
		{ID: "d2", CollectionID: "c1", ContentText: "[[y]]\n"},
		{ID: "d3", CollectionID: "c1", ContentText: "[[z]]\n"},
	}
	u, repo, _ := rebuildFixture(docs...)
	listCalls := 0
	repo.collListFn = func(_ context.Context, _ string, _, _ int) ([]Collection, int, error) {
		listCalls++
		return []Collection{{ID: "c1"}}, 1, nil
	}
	if _, err := u.RebuildCollectionBlockIndex(context.Background(), "c1", nil); err != nil {
		t.Fatal(err)
	}
	if listCalls != 1 {
		t.Errorf("ListCollections 调用 %d 次, want 1（可见集整批提升）", listCalls)
	}
}

// TestRebuildCollectionBlockIndex_LoadsContentViaGetDocument 回归（2026-08-09
// 运行时事故）：真实 ListDocuments 是摘要投影（SELECT 不含 content_text），
// 重建若直接用摘要的 ContentText（恒空）会把全库索引"删了重插成空"。
// 契约：重建必须逐文档 GetDocument 取回正文。
func TestRebuildCollectionBlockIndex_LoadsContentViaGetDocument(t *testing.T) {
	summaries := []Document{
		{ID: "d1", CollectionID: "c1"}, // 摘要投影：无 ContentText（模拟真实 repo）
		{ID: "d2", CollectionID: "c1"},
	}
	u, repo, idx := rebuildFixture(summaries...)
	full := map[string]string{
		"d1": "# A\n\n[[d2]]\n",
		"d2": "# B\n\n[[d1#A]]\n",
	}
	repo.docGetFn = func(_ context.Context, id string) (Document, error) {
		body, ok := full[id]
		if !ok {
			return Document{}, apierror.NotFound("KNOWLEDGE", "doc not found: %s", id)
		}
		return Document{ID: id, CollectionID: "c1", ContentText: body}, nil
	}
	res, err := u.RebuildCollectionBlockIndex(context.Background(), "c1", nil)
	if err != nil {
		t.Fatalf("RebuildCollectionBlockIndex: %v", err)
	}
	if res.Done != 2 || res.Failed != 0 {
		t.Errorf("res = %+v, want done=2 failed=0", res)
	}
	// 边必须真实产出（空 body 时此处为 0 条——事故特征）。
	if len(idx.refs["d1"]) != 1 || idx.refs["d1"][0].DstDocID != "d2" {
		t.Errorf("refs[d1] = %+v, want 1 edge → d2", idx.refs["d1"])
	}
	if len(idx.refs["d2"]) != 1 || idx.refs["d2"][0].DstDocID != "d1" {
		t.Errorf("refs[d2] = %+v, want 1 edge → d1", idx.refs["d2"])
	}
}

// failOnceBlockIndex 对指定文档的 ReplaceDocBlocks 注入失败。
type failOnceBlockIndex struct {
	*memBlockIndex
	failDoc string
}

func (f *failOnceBlockIndex) ReplaceDocBlocks(ctx context.Context, collectionID, docID string, blocks []KnowledgeBlock, refs []KnowledgeBlockRefInput) ([]KnowledgeBlockRefEdge, error) {
	if docID == f.failDoc {
		return nil, fmt.Errorf("injected failure")
	}
	return f.memBlockIndex.ReplaceDocBlocks(ctx, collectionID, docID, blocks, refs)
}
