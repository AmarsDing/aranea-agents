package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ── SP1-C C-1：LinkResolver 跨库双链解析（B-1 确定性规则） ──────────────────

type stubResolveIndex struct {
	candidates []ResolveDocCandidate
	gotCollIDs []string
	byAnchor   map[string]string // docID|anchor → blockID
	byHeading  map[string]string // docID|H1/H2 → blockID
	err        error
}

func (s *stubResolveIndex) ListResolveCandidates(_ context.Context, collectionIDs []string) ([]ResolveDocCandidate, error) {
	s.gotCollIDs = collectionIDs
	if s.err != nil {
		return nil, s.err
	}
	return s.candidates, nil
}

func (s *stubResolveIndex) FindBlockByAnchor(_ context.Context, docID, anchor string) (string, bool, error) {
	id, ok := s.byAnchor[docID+"|"+anchor]
	return id, ok, nil
}

func (s *stubResolveIndex) FindBlockByHeadingPath(_ context.Context, docID string, path []string) (string, bool, error) {
	key := docID + "|"
	for i, p := range path {
		if i > 0 {
			key += "/"
		}
		key += p
	}
	id, ok := s.byHeading[key]
	return id, ok, nil
}

func resolverOf(idx *stubResolveIndex) *LinkResolver { return NewLinkResolver(idx) }

func resolveOne(t *testing.T, r *LinkResolver, srcColl, srcDoc string, visible []string, rawTarget string, self []KnowledgeBlock) KnowledgeBlockRefInput {
	t.Helper()
	refs := []KnowledgeBlockRefInput{{SrcOrdinal: 0, RawTarget: rawTarget, EdgeType: "ref"}}
	out, err := r.ResolveRefs(context.Background(), srcColl, srcDoc, visible, refs, self)
	if err != nil {
		t.Fatalf("ResolveRefs: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("refs = %d, want 1", len(out))
	}
	return out[0]
}

var (
	t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
)

// TestResolve_SameCollectionFirst ①同库命中优先（即使他库路径更短）。
func TestResolve_SameCollectionFirst(t *testing.T) {
	idx := &stubResolveIndex{candidates: []ResolveDocCandidate{
		{DocID: "p1", CollectionID: "personal", RelPath: "note.md", CollectionCreatedAt: t0},
		{DocID: "t1", CollectionID: "team", RelPath: "projects/note.md", CollectionCreatedAt: t0},
	}}
	got := resolveOne(t, resolverOf(idx), "team", "doc-src", []string{"team", "personal"}, "Note", nil)
	if got.DstDocID != "t1" {
		t.Errorf("DstDocID = %q, want t1（同库优先）", got.DstDocID)
	}
	if got.Ambiguous {
		t.Error("同库唯一命中不应 ambiguous")
	}
}

// TestResolve_ShortestPath ②同层候选取最短路径。
func TestResolve_ShortestPath(t *testing.T) {
	idx := &stubResolveIndex{candidates: []ResolveDocCandidate{
		{DocID: "deep", CollectionID: "c1", RelPath: "a/b/note.md", CollectionCreatedAt: t0},
		{DocID: "shallow", CollectionID: "c1", RelPath: "note.md", CollectionCreatedAt: t0},
	}}
	got := resolveOne(t, resolverOf(idx), "c1", "d0", []string{"c1"}, "note", nil)
	if got.DstDocID != "shallow" {
		t.Errorf("DstDocID = %q, want shallow（最短路径）", got.DstDocID)
	}
	if !got.Ambiguous {
		t.Error("多个 basename 同级候选应 ambiguous=true")
	}
}

// TestResolve_AmbiguousDeterministic ③多义时按（collection 创建序、路径字典序）确定性取首。
func TestResolve_AmbiguousDeterministic(t *testing.T) {
	mk := func() *stubResolveIndex {
		return &stubResolveIndex{candidates: []ResolveDocCandidate{
			{DocID: "b-note", CollectionID: "cB", RelPath: "y/note.md", CollectionCreatedAt: t1},
			{DocID: "a-note", CollectionID: "cA", RelPath: "x/note.md", CollectionCreatedAt: t0},
		}}
	}
	first := resolveOne(t, resolverOf(mk()), "cSrc", "d0", []string{"cA", "cB"}, "note", nil)
	if first.DstDocID != "a-note" {
		t.Errorf("DstDocID = %q, want a-note（创建序优先）", first.DstDocID)
	}
	if !first.Ambiguous {
		t.Error("多义应 ambiguous=true")
	}
	// 输入顺序无关（确定性）。
	idx2 := &stubResolveIndex{candidates: []ResolveDocCandidate{
		{DocID: "a-note", CollectionID: "cA", RelPath: "x/note.md", CollectionCreatedAt: t0},
		{DocID: "b-note", CollectionID: "cB", RelPath: "y/note.md", CollectionCreatedAt: t1},
	}}
	second := resolveOne(t, resolverOf(idx2), "cSrc", "d0", []string{"cA", "cB"}, "note", nil)
	if second.DstDocID != first.DstDocID {
		t.Errorf("输入顺序影响结果: %q vs %q", second.DstDocID, first.DstDocID)
	}
}

// TestResolve_VisibilityPassthrough 可见集合原样传给数据端口（不可见库不参与由 SQL 保证）。
func TestResolve_VisibilityPassthrough(t *testing.T) {
	idx := &stubResolveIndex{}
	resolveOne(t, resolverOf(idx), "c1", "d0", []string{"c1", "c2"}, "anything", nil)
	if len(idx.gotCollIDs) != 2 || idx.gotCollIDs[0] != "c1" || idx.gotCollIDs[1] != "c2" {
		t.Errorf("visible ids 未透传: %v", idx.gotCollIDs)
	}
}

// TestResolve_TitleAndAlias 标题/别名作为文档键。
func TestResolve_TitleAndAlias(t *testing.T) {
	idx := &stubResolveIndex{candidates: []ResolveDocCandidate{
		{DocID: "d1", CollectionID: "c1", RelPath: "x/a1.md", Title: "设计稿", CollectionCreatedAt: t0},
		{DocID: "d2", CollectionID: "c1", RelPath: "y/b2.md", Aliases: []string{"别名B", "b2"}, CollectionCreatedAt: t0},
	}}
	if got := resolveOne(t, resolverOf(idx), "c1", "s", []string{"c1"}, "设计稿", nil); got.DstDocID != "d1" {
		t.Errorf("标题匹配 DstDocID = %q, want d1", got.DstDocID)
	}
	if got := resolveOne(t, resolverOf(idx), "c1", "s", []string{"c1"}, "别名B", nil); got.DstDocID != "d2" {
		t.Errorf("别名匹配 DstDocID = %q, want d2", got.DstDocID)
	}
}

// TestResolve_CaseInsensitive 大小写不敏感。
func TestResolve_CaseInsensitive(t *testing.T) {
	idx := &stubResolveIndex{candidates: []ResolveDocCandidate{
		{DocID: "d1", CollectionID: "c1", RelPath: "Notes/Idea.md", CollectionCreatedAt: t0},
	}}
	if got := resolveOne(t, resolverOf(idx), "c1", "s", []string{"c1"}, "notes/idea", nil); got.DstDocID != "d1" {
		t.Errorf("DstDocID = %q, want d1", got.DstDocID)
	}
}

// TestResolve_EmbedAsset 嵌入资源按 basename 命中（带扩展名）。
func TestResolve_EmbedAsset(t *testing.T) {
	idx := &stubResolveIndex{candidates: []ResolveDocCandidate{
		{DocID: "asset1", CollectionID: "c1", RelPath: "media/img.png", CollectionCreatedAt: t0},
	}}
	got := resolveOne(t, resolverOf(idx), "c1", "s", []string{"c1"}, "img.png", nil)
	if got.DstDocID != "asset1" {
		t.Errorf("DstDocID = %q, want asset1", got.DstDocID)
	}
}

// TestResolve_HeadingBlock 标题块定位（FindBlockByHeadingPath）。
func TestResolve_HeadingBlock(t *testing.T) {
	idx := &stubResolveIndex{
		candidates: []ResolveDocCandidate{{DocID: "d1", CollectionID: "c1", RelPath: "note.md", CollectionCreatedAt: t0}},
		byHeading:  map[string]string{"d1|H1/H2": "blk-h2"},
	}
	got := resolveOne(t, resolverOf(idx), "c1", "s", []string{"c1"}, "Note#H1#H2", nil)
	if got.DstDocID != "d1" || got.DstBlockID != "blk-h2" {
		t.Errorf("dst = %q/%q, want d1/blk-h2", got.DstDocID, got.DstBlockID)
	}
}

// TestResolve_AnchorBlock 锚块定位（FindBlockByAnchor）。
func TestResolve_AnchorBlock(t *testing.T) {
	idx := &stubResolveIndex{
		candidates: []ResolveDocCandidate{{DocID: "d1", CollectionID: "c1", RelPath: "note.md", CollectionCreatedAt: t0}},
		byAnchor:   map[string]string{"d1|b1": "blk-b1"},
	}
	got := resolveOne(t, resolverOf(idx), "c1", "s", []string{"c1"}, "Note#^b1", nil)
	if got.DstDocID != "d1" || got.DstBlockID != "blk-b1" {
		t.Errorf("dst = %q/%q, want d1/blk-b1", got.DstDocID, got.DstBlockID)
	}
}

// TestResolve_BlockMissKeepsDoc 块未命中：doc 级已解析，块级悬空（dst_block 空）。
func TestResolve_BlockMissKeepsDoc(t *testing.T) {
	idx := &stubResolveIndex{
		candidates: []ResolveDocCandidate{{DocID: "d1", CollectionID: "c1", RelPath: "note.md", CollectionCreatedAt: t0}},
	}
	got := resolveOne(t, resolverOf(idx), "c1", "s", []string{"c1"}, "Note#不存在", nil)
	if got.DstDocID != "d1" {
		t.Errorf("DstDocID = %q, want d1", got.DstDocID)
	}
	if got.DstBlockID != "" {
		t.Errorf("DstBlockID = %q, want 空（块级悬空）", got.DstBlockID)
	}
}

// TestResolve_DocMissDangling 文档未命中：全悬空（dst 全空，raw_target 保留复活线索）。
func TestResolve_DocMissDangling(t *testing.T) {
	idx := &stubResolveIndex{}
	got := resolveOne(t, resolverOf(idx), "c1", "s", []string{"c1"}, "Ghost", nil)
	if got.DstDocID != "" || got.DstBlockID != "" {
		t.Errorf("dangling 应 dst 全空: %q/%q", got.DstDocID, got.DstBlockID)
	}
	if got.RawTarget != "Ghost" {
		t.Errorf("RawTarget = %q", got.RawTarget)
	}
}

// TestResolve_SelfAnchor 自文档锚引用：按内存块解析，记 DstSelfOrdinal 供存储层映射。
func TestResolve_SelfAnchor(t *testing.T) {
	idx := &stubResolveIndex{}
	self := []KnowledgeBlock{
		{Ordinal: 0, Kind: "paragraph"},
		{Ordinal: 1, Kind: "paragraph", Anchor: "a1"},
	}
	got := resolveOne(t, resolverOf(idx), "c1", "d-self", []string{"c1"}, "#^a1", self)
	if got.DstDocID != "d-self" {
		t.Errorf("DstDocID = %q, want d-self", got.DstDocID)
	}
	if got.DstSelfOrdinal == nil || *got.DstSelfOrdinal != 1 {
		t.Errorf("DstSelfOrdinal = %v, want 1", got.DstSelfOrdinal)
	}
	if got.DstBlockID != "" {
		t.Errorf("自引用块 ID 由存储层映射，解析期应为空, got %q", got.DstBlockID)
	}
}

// TestResolve_SelfHeading 自文档标题引用：仅匹配 heading 块的标题路径。
func TestResolve_SelfHeading(t *testing.T) {
	idx := &stubResolveIndex{}
	self := []KnowledgeBlock{
		{Ordinal: 0, Kind: "heading", HeadingPath: []string{"Alpha"}},
		{Ordinal: 1, Kind: "heading", HeadingPath: []string{"Alpha", "Beta"}},
		{Ordinal: 2, Kind: "paragraph", HeadingPath: []string{"Alpha", "Beta"}}, // 段落同路径不命中
	}
	got := resolveOne(t, resolverOf(idx), "c1", "d-self", []string{"c1"}, "#Alpha#Beta", self)
	if got.DstSelfOrdinal == nil || *got.DstSelfOrdinal != 1 {
		t.Errorf("DstSelfOrdinal = %v, want 1（heading 块）", got.DstSelfOrdinal)
	}
}

// TestResolve_IndexError 数据端口故障原样上抛（调用方降级）。
func TestResolve_IndexError(t *testing.T) {
	idx := &stubResolveIndex{err: errors.New("db down")}
	_, err := NewLinkResolver(idx).ResolveRefs(context.Background(), "c1", "d0", []string{"c1"},
		[]KnowledgeBlockRefInput{{SrcOrdinal: 0, RawTarget: "X", EdgeType: "ref"}}, nil)
	if err == nil {
		t.Fatal("端口错误应上抛")
	}
}

// TestResolve_NoDocPart 空目标段已在解析层过滤，Resolver 不接收；纯块引用不查候选端口。
func TestResolve_NoCandidateQueryForSelfRefs(t *testing.T) {
	idx := &stubResolveIndex{}
	self := []KnowledgeBlock{{Ordinal: 0, Kind: "heading", HeadingPath: []string{"H"}}}
	got := resolveOne(t, resolverOf(idx), "c1", "d-self", nil, "#H", self)
	if got.DstSelfOrdinal == nil || *got.DstSelfOrdinal != 0 {
		t.Errorf("DstSelfOrdinal = %v, want 0", got.DstSelfOrdinal)
	}
	if idx.gotCollIDs != nil {
		t.Errorf("纯自文档引用不应查询候选端口, got %v", idx.gotCollIDs)
	}
}
