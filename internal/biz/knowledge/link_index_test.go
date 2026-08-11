package knowledge

import (
	"context"
	"fmt"
	"testing"
)

// ── SP1-D D-1：LinkIndex 进程内内存图（正/反邻接 + 版本号 + 可见性过滤） ─────

func refEdge(srcColl, srcDoc, srcBlk, dstColl, dstDoc, dstBlk, raw string) KnowledgeBlockRefEdge {
	return KnowledgeBlockRefEdge{
		CollectionID:    srcColl,
		SrcDocID:        srcDoc,
		SrcBlockID:      srcBlk,
		DstCollectionID: dstColl,
		DstDocID:        dstDoc,
		DstBlockID:      dstBlk,
		RawTarget:       raw,
		EdgeType:        "ref",
		Context:         "ctx " + raw,
	}
}

func edgeSet(edges []KnowledgeBlockRefEdge) map[string]int {
	out := make(map[string]int, len(edges))
	for _, e := range edges {
		out[edgeIdentity(e)]++
	}
	return out
}

func assertEdgesEqual(t *testing.T, got []KnowledgeBlockRefEdge, want ...KnowledgeBlockRefEdge) {
	t.Helper()
	g, w := edgeSet(got), edgeSet(want)
	if len(g) != len(w) {
		t.Fatalf("edges = %v, want %v", got, want)
	}
	for k, n := range w {
		if g[k] != n {
			t.Fatalf("edges = %v, want %v", got, want)
		}
	}
}

func allVisible(edges ...KnowledgeBlockRefEdge) map[string]bool {
	out := map[string]bool{}
	for _, e := range edges {
		out[e.CollectionID] = true
		if e.DstCollectionID != "" {
			out[e.DstCollectionID] = true
		}
	}
	return out
}

// TestLinkIndex_ApplyDocDelta_AddRemove 首次 apply 加边；二次 apply 同文档旧边全摘、
// 新边入，version 单调递增；delta 携带 removed/added。
func TestLinkIndex_ApplyDocDelta_AddRemove(t *testing.T) {
	x := NewLinkIndex()
	e1 := refEdge("c1", "d1", "b1", "c1", "d2", "b9", "Note#^a")
	e2 := refEdge("c1", "d1", "b2", "c2", "d3", "", "Other")

	d := x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{e1, e2})
	if len(d.Removed) != 0 || len(d.Added) != 2 {
		t.Fatalf("首次 apply delta = +%d/-%d, want +2/-0", len(d.Added), len(d.Removed))
	}
	if d.Version != 1 || x.Version() != 1 {
		t.Fatalf("version = %d/%d, want 1", d.Version, x.Version())
	}

	// 重建：e1 内容变更（改锚），e2 保留原文 → delta 只含变化量。
	e1b := refEdge("c1", "d1", "b1", "c1", "d2", "b10", "Note#^b")
	d = x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{e1b, e2})
	assertEdgesEqual(t, d.Removed, e1)
	assertEdgesEqual(t, d.Added, e1b)
	if x.Version() != 2 {
		t.Fatalf("version = %d, want 2", x.Version())
	}
	assertEdgesEqual(t, x.OutEdges("b1", nil), e1b)
}

// TestLinkIndex_ApplyDocDelta_NoChangeEmptyDelta 内容不变的重建产空 delta，
// version 不动（图谱状态未变，不制造 WS 噪声）。
func TestLinkIndex_ApplyDocDelta_NoChangeEmptyDelta(t *testing.T) {
	x := NewLinkIndex()
	e1 := refEdge("c1", "d1", "b1", "c1", "d2", "", "Note")
	x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{e1})
	d := x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{e1})
	if len(d.Added) != 0 || len(d.Removed) != 0 {
		t.Fatalf("无变化重建 delta = +%d/-%d, want 空", len(d.Added), len(d.Removed))
	}
	if x.Version() != 1 {
		t.Fatalf("version = %d, want 1（无变化不递增）", x.Version())
	}
}

// TestLinkIndex_BacklinksByBlock 块级反链直读反向邻接；可见性按边源集合过滤。
func TestLinkIndex_BacklinksByBlock(t *testing.T) {
	x := NewLinkIndex()
	pub := refEdge("c1", "d1", "b1", "c2", "d2", "tb", "T#^a")
	priv := refEdge("cPriv", "d9", "b8", "c2", "d2", "tb", "T#^a")
	docLevel := refEdge("c1", "d3", "b3", "c2", "d2", "", "T")
	x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{pub})
	x.ApplyDocDelta("d9", []KnowledgeBlockRefEdge{priv})
	x.ApplyDocDelta("d3", []KnowledgeBlockRefEdge{docLevel})

	got := x.BacklinksByBlock("tb", allVisible(pub, priv))
	assertEdgesEqual(t, got, pub, priv)

	// 源集合不可见 → 该边被过滤（B-1 防泄漏延伸到图谱读侧）。
	got = x.BacklinksByBlock("tb", map[string]bool{"c1": true, "c2": true})
	assertEdgesEqual(t, got, pub)

	// 文档级边不出现在块级反链。
	for _, e := range got {
		if e.SrcBlockID == "b3" {
			t.Fatalf("文档级边不应计入块级反链：%v", e)
		}
	}
}

// TestLinkIndex_BacklinksByDoc 文档反链 = 全部块级 + 文档级入边聚合。
func TestLinkIndex_BacklinksByDoc(t *testing.T) {
	x := NewLinkIndex()
	blk := refEdge("c1", "d1", "b1", "c2", "d2", "tb", "T#^a")
	doc := refEdge("c1", "d3", "b3", "c2", "d2", "", "T")
	other := refEdge("c1", "d4", "b4", "c2", "d5", "", "X")
	x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{blk})
	x.ApplyDocDelta("d3", []KnowledgeBlockRefEdge{doc})
	x.ApplyDocDelta("d4", []KnowledgeBlockRefEdge{other})

	assertEdgesEqual(t, x.BacklinksByDoc("d2", allVisible(blk, doc)), blk, doc)
	assertEdgesEqual(t, x.BacklinksByDoc("d5", allVisible(other)), other)
}

// TestLinkIndex_DanglingByCollection dangling 边（DstDocID 空）按源集合聚合，
// raw_target 保留复活线索。
func TestLinkIndex_DanglingByCollection(t *testing.T) {
	x := NewLinkIndex()
	dangling := refEdge("c1", "d1", "b1", "", "", "", "Ghost")
	resolved := refEdge("c1", "d1", "b2", "c2", "d2", "", "Note")
	x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{dangling, resolved})

	got := x.DanglingByCollection("c1", nil)
	assertEdgesEqual(t, got, dangling)
	if got[0].RawTarget != "Ghost" {
		t.Fatalf("dangling raw_target = %q, want Ghost", got[0].RawTarget)
	}
}

// TestLinkIndex_TargetRebuildTurnsIncomingDocLevel 目标文档重建时，其旧块被删除，
// DB 侧入向块边经 FK SET NULL 转为文档级（dst_block NULL、dst_doc 保留）；
// 内存图必须镜像该语义：入向块边转文档级，delta 携带转换对。
// 时序注意：ApplyDocDelta 无法区分「初次入库」与「重建」——目标文档的每次
// apply 都会转换入向块边（与 ReplaceDocBlocks 先删后插的 FK 语义一致），
// 因此入向边必须在目标文档初次 apply 之后再进入图。
func TestLinkIndex_TargetRebuildTurnsIncomingDocLevel(t *testing.T) {
	x := NewLinkIndex()
	// d2 初次入库：自引用块边（与后续转换无关，不应受影响）。
	self := refEdge("c2", "d2", "tb", "c2", "d2", "tb", "#^self")
	x.ApplyDocDelta("d2", []KnowledgeBlockRefEdge{self})
	// d1 后索引：解析到已存在的 d2 块 tb（块级入边进入图）。
	incoming := refEdge("c1", "d1", "b1", "c2", "d2", "tb", "T#^a")
	x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{incoming})

	// d2 重建（无论锚块 ID 是否延续，DB 语义均为入向块边 SET NULL）。
	d := x.ApplyDocDelta("d2", []KnowledgeBlockRefEdge{self})

	wantTurned := incoming
	wantTurned.DstBlockID = ""
	// delta：入向边旧形态摘除 + 新形态加入；d2 自身出边无变化不进 delta。
	assertEdgesEqual(t, d.Removed, incoming)
	assertEdgesEqual(t, d.Added, wantTurned)
	// 块级反链不再命中旧块；文档级反链仍命中。
	assertEdgesEqual(t, x.BacklinksByBlock("tb", nil), self)
	assertEdgesEqual(t, x.BacklinksByDoc("d2", nil), wantTurned, self)
}

// TestLinkIndex_RemoveDoc 文档删除：出向边随块级联清除；入向边转 dangling
// （保 raw_target），两类变更都进 delta（G-3 删除同步的内存侧语义）。
func TestLinkIndex_RemoveDoc(t *testing.T) {
	x := NewLinkIndex()
	out := refEdge("c2", "d2", "bx", "c1", "d1", "b1", "D1")
	in := refEdge("c1", "d3", "b3", "c2", "d2", "bx", "D2#^x")
	x.ApplyDocDelta("d2", []KnowledgeBlockRefEdge{out})
	x.ApplyDocDelta("d3", []KnowledgeBlockRefEdge{in})

	d := x.RemoveDoc("d2")
	wantDangling := in
	wantDangling.DstCollectionID, wantDangling.DstDocID, wantDangling.DstBlockID = "", "", ""
	assertEdgesEqual(t, d.Removed, out, in)
	assertEdgesEqual(t, d.Added, wantDangling)

	if got := x.OutEdges("bx", nil); len(got) != 0 {
		t.Fatalf("删除文档出边应清除，got %v", got)
	}
	assertEdgesEqual(t, x.DanglingByCollection("c1", nil), wantDangling)
	assertEdgesEqual(t, x.BacklinksByDoc("d1", nil)) // d2 已删，出边不再存在
}

// TestLinkIndex_RemoveCollection 集合删除（G-3）：源在被删集合的全部边消失
// （库内边 + 跨库出边）；外部集合指向被删集合的入边转 dangling 保 raw_target；
// 两类变更都进 delta；不相关边不受影响。
func TestLinkIndex_RemoveCollection(t *testing.T) {
	x := NewLinkIndex()
	intra := refEdge("c2", "d2", "bx", "c2", "d4", "by", "D4#^y") // 库内边 → 消失
	crossOut := refEdge("c2", "d2", "bz", "c1", "d1", "b1", "D1") // 跨库出边 → 消失
	extIn := refEdge("c1", "d3", "b3", "c2", "d2", "bx", "D2#^x") // 外部入边 → 转 dangling
	unrelated := refEdge("c1", "d3", "b4", "c3", "d5", "", "D5")  // 不相关 → 不动
	x.ApplyDocDelta("d2", []KnowledgeBlockRefEdge{intra, crossOut})
	x.ApplyDocDelta("d3", []KnowledgeBlockRefEdge{extIn, unrelated})

	d := x.RemoveCollection("c2")
	wantDangling := extIn
	wantDangling.DstCollectionID, wantDangling.DstDocID, wantDangling.DstBlockID = "", "", ""
	assertEdgesEqual(t, d.Removed, intra, crossOut, extIn)
	assertEdgesEqual(t, d.Added, wantDangling)

	if got := x.OutEdges("bx", nil); len(got) != 0 {
		t.Fatalf("被删集合出边应清除, got %v", got)
	}
	assertEdgesEqual(t, x.DanglingByCollection("c1", nil), wantDangling)
	assertEdgesEqual(t, x.OutEdges("b4", nil), unrelated)
	if got := x.DanglingByCollection("c2", nil); len(got) != 0 {
		t.Fatalf("被删集合不应残留 dangling 源边, got %v", got)
	}
}

// TestLinkIndex_RemoveCollection_EmptyNoVersion 删除无图集合产空 delta，
// version 不动（与 ApplyDocDelta 无变化语义一致）。
func TestLinkIndex_RemoveCollection_EmptyNoVersion(t *testing.T) {
	x := NewLinkIndex()
	x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{refEdge("c1", "d1", "b1", "c1", "d2", "", "A")})
	v := x.Version()
	d := x.RemoveCollection("cGhost")
	if !d.Empty() {
		t.Fatalf("无图集合 delta 应为空, got +%d/-%d", len(d.Added), len(d.Removed))
	}
	if x.Version() != v {
		t.Fatalf("version = %d, want %d（空 delta 不递增）", x.Version(), v)
	}
}

// TestLinkIndex_ConsistencyWithReplay 验收：增量 apply 序列后的内存图与
// 从最终边集全量重放（LoadAll）结果一致。
func TestLinkIndex_ConsistencyWithReplay(t *testing.T) {
	x := NewLinkIndex()
	v1a := refEdge("c1", "d1", "b1", "c2", "d2", "tb", "A#^x")
	v1b := refEdge("c1", "d1", "b2", "", "", "", "Ghost")
	v2a := refEdge("c1", "d1", "b1", "c2", "d2", "", "A") // 重建后改文档级
	d2e := refEdge("c2", "d2", "tb", "c1", "d1", "b1", "D1")
	d3e := refEdge("c3", "d3", "k1", "c1", "d1", "", "D1")

	x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{v1a, v1b})
	x.ApplyDocDelta("d2", []KnowledgeBlockRefEdge{d2e})
	x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{v2a, v1b}) // d1 重建
	x.ApplyDocDelta("d3", []KnowledgeBlockRefEdge{d3e})
	x.ApplyDocDelta("d2", nil) // d2 重建后无出边（d2e 随 d2 块删除级联清除）
	x.RemoveDoc("d3")

	// 全量重放：按 DB 终态推演——
	// d1 出边：v2a + v1b（dangling）
	// d2e：d1 重建时 dst_block SET NULL 转文档级；但 d2 随后重建为 nil 出边，
	//     src 块 tb 被删 → d2e 整体级联删除（出边随块消失，不留 dangling）
	// d3e：d3 删除 → 出边清除
	replayed := NewLinkIndex()
	replayed.LoadAll([]KnowledgeBlockRefEdge{v2a, v1b})

	for _, doc := range []string{"d1", "d2", "d3"} {
		if got, want := x.BacklinksByDoc(doc, nil), replayed.BacklinksByDoc(doc, nil); len(got) != len(want) {
			t.Fatalf("BacklinksByDoc(%s) 增量 %v vs 重放 %v", doc, got, want)
		}
	}
	for _, blk := range []string{"b1", "b2", "tb", "k1"} {
		if got, want := x.OutEdges(blk, nil), replayed.OutEdges(blk, nil); len(got) != len(want) {
			t.Fatalf("OutEdges(%s) 增量 %v vs 重放 %v", blk, got, want)
		}
		if got, want := x.BacklinksByBlock(blk, nil), replayed.BacklinksByBlock(blk, nil); len(got) != len(want) {
			t.Fatalf("BacklinksByBlock(%s) 增量 %v vs 重放 %v", blk, got, want)
		}
	}
	for _, coll := range []string{"c1", "c2", "c3"} {
		if got, want := x.DanglingByCollection(coll, nil), replayed.DanglingByCollection(coll, nil); len(got) != len(want) {
			t.Fatalf("DanglingByCollection(%s) 增量 %v vs 重放 %v", coll, got, want)
		}
	}
}

// TestLinkIndex_LoadAllResets LoadAll 为重置语义（启动/重建全量构建），version 归零。
func TestLinkIndex_LoadAllResets(t *testing.T) {
	x := NewLinkIndex()
	x.ApplyDocDelta("d1", []KnowledgeBlockRefEdge{refEdge("c1", "d1", "b1", "c1", "d2", "", "A")})
	x.LoadAll([]KnowledgeBlockRefEdge{refEdge("c9", "d9", "k1", "c9", "d8", "", "B")})
	if x.Version() != 0 {
		t.Fatalf("LoadAll 后 version = %d, want 0", x.Version())
	}
	assertEdgesEqual(t, x.OutEdges("b1", nil))
	assertEdgesEqual(t, x.BacklinksByDoc("d8", nil), refEdge("c9", "d9", "k1", "c9", "d8", "", "B"))
}

// TestLinkIndex_ConcurrentReadApply 并发读写在 -race 下安全（构造性冒烟）。
func TestLinkIndex_ConcurrentReadApply(t *testing.T) {
	x := NewLinkIndex()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			x.ApplyDocDelta(fmt.Sprintf("d%d", i%5), []KnowledgeBlockRefEdge{
				refEdge("c1", fmt.Sprintf("d%d", i%5), fmt.Sprintf("b%d", i), "c1", "dx", "", "T"),
			})
		}
		close(done)
	}()
	for i := 0; i < 50; i++ {
		_ = x.BacklinksByDoc("dx", nil)
		_ = x.DanglingByCollection("c1", nil)
		_ = x.Version()
	}
	<-done
}

// ── SP1-D D-3：管线钩子（Usecase 装配/启动全量加载/删除同步） ────────────────

// stubBlockIndexLoader BlockIndexRepo + LinkEdgeLoader 双端口桩（启动加载用）。
type stubBlockIndexLoader struct {
	edges []KnowledgeBlockRefEdge
	err   error
}

func (s stubBlockIndexLoader) ReplaceDocBlocks(context.Context, string, string, []KnowledgeBlock, []KnowledgeBlockRefInput) ([]KnowledgeBlockRefEdge, error) {
	return nil, nil
}
func (s stubBlockIndexLoader) ListDocBlocks(context.Context, string) ([]KnowledgeBlock, error) {
	return nil, nil
}
func (s stubBlockIndexLoader) UpdateDocLinkKeys(context.Context, string, string, []string) error {
	return nil
}
func (s stubBlockIndexLoader) ListDocsMissingBlockIndex(context.Context, string, int) ([]string, error) {
	return nil, nil
}
func (s stubBlockIndexLoader) ListAllRefEdges(context.Context) ([]KnowledgeBlockRefEdge, error) {
	return s.edges, s.err
}

// captureGraphPub 捕获发布的图谱增量。
type captureGraphPub struct{ deltas []GraphDelta }

func (c *captureGraphPub) PublishGraphDelta(_ context.Context, d GraphDelta) {
	c.deltas = append(c.deltas, d)
}

// TestUsecase_LoadLinkIndex 启动全量加载：经 LinkEdgeLoader 重放进内存图并返回
// 边数；未接线/未实现 loader 时 no-op；loader 错误上抛（调用方降级记日志）。
func TestUsecase_LoadLinkIndex(t *testing.T) {
	edges := []KnowledgeBlockRefEdge{
		refEdge("c1", "d1", "b1", "c2", "d2", "tb", "A#^x"),
		refEdge("c1", "d1", "b2", "", "", "", "Ghost"),
	}
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetBlockIndexRepos(stubBlockIndexLoader{edges: edges}, nil)
	idx := NewLinkIndex()
	u.SetLinkIndex(idx, nil)

	n, err := u.LoadLinkIndex(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("LoadLinkIndex = %d/%v, want 2/nil", n, err)
	}
	assertEdgesEqual(t, idx.BacklinksByBlock("tb", nil), edges[0])
	assertEdgesEqual(t, idx.DanglingByCollection("c1", nil), edges[1])
	if idx.Version() != 0 {
		t.Fatalf("全量加载 version = %d, want 0（LoadAll 重置语义）", idx.Version())
	}

	// 未接线 linkIndex → no-op。
	u2 := NewUsecaseFromRepo(noOpMockRepo())
	u2.SetBlockIndexRepos(stubBlockIndexLoader{edges: edges}, nil)
	if n, err := u2.LoadLinkIndex(context.Background()); n != 0 || err != nil {
		t.Fatalf("未接线 LoadLinkIndex = %d/%v, want 0/nil", n, err)
	}
	// blockIndex 未实现 loader → no-op。
	u3 := NewUsecaseFromRepo(noOpMockRepo())
	u3.SetBlockIndexRepos(stubBlockIndexRepo{}, nil)
	u3.SetLinkIndex(NewLinkIndex(), nil)
	if n, err := u3.LoadLinkIndex(context.Background()); n != 0 || err != nil {
		t.Fatalf("无 loader LoadLinkIndex = %d/%v, want 0/nil", n, err)
	}
	// loader 错误上抛。
	u4 := NewUsecaseFromRepo(noOpMockRepo())
	u4.SetBlockIndexRepos(stubBlockIndexLoader{err: fmt.Errorf("db down")}, nil)
	u4.SetLinkIndex(NewLinkIndex(), nil)
	if _, err := u4.LoadLinkIndex(context.Background()); err == nil {
		t.Fatal("loader 错误应上抛")
	}
}

// stubBlockIndexRepo 仅 BlockIndexRepo（无 LinkEdgeLoader）桩。
type stubBlockIndexRepo struct{}

func (stubBlockIndexRepo) ReplaceDocBlocks(context.Context, string, string, []KnowledgeBlock, []KnowledgeBlockRefInput) ([]KnowledgeBlockRefEdge, error) {
	return nil, nil
}
func (stubBlockIndexRepo) ListDocBlocks(context.Context, string) ([]KnowledgeBlock, error) {
	return nil, nil
}
func (stubBlockIndexRepo) UpdateDocLinkKeys(context.Context, string, string, []string) error {
	return nil
}
func (stubBlockIndexRepo) ListDocsMissingBlockIndex(context.Context, string, int) ([]string, error) {
	return nil, nil
}

// TestUsecase_DeleteDocument_LinkIndex 删除文档后内存图同步（出边清除、入边
// 转 dangling 保 raw_target）且发布非空 delta；repo 删除失败时不动内存图。
func TestUsecase_DeleteDocument_LinkIndex(t *testing.T) {
	idx := NewLinkIndex()
	pub := &captureGraphPub{}
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkIndex(idx, pub)

	out := refEdge("c2", "d2", "bx", "c1", "d1", "b1", "D1")
	in := refEdge("c1", "d3", "b3", "c2", "d2", "bx", "D2#^x")
	idx.LoadAll([]KnowledgeBlockRefEdge{out, in})

	if err := u.DeleteDocument(context.Background(), "d2"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	wantDangling := in
	wantDangling.DstCollectionID, wantDangling.DstDocID, wantDangling.DstBlockID = "", "", ""
	assertEdgesEqual(t, idx.DanglingByCollection("c1", nil), wantDangling)
	if got := idx.OutEdges("bx", nil); len(got) != 0 {
		t.Fatalf("删除后出边应清除, got %v", got)
	}
	if len(pub.deltas) != 1 {
		t.Fatalf("发布 delta 数 = %d, want 1", len(pub.deltas))
	}
	d := pub.deltas[0]
	assertEdgesEqual(t, d.Removed, out, in)
	assertEdgesEqual(t, d.Added, wantDangling)
	if d.Version != 1 {
		t.Fatalf("delta version = %d, want 1（LoadAll 归零后首次变更）", d.Version)
	}

	// 删除不存在的文档：空 delta 不发布。
	pub.deltas = nil
	if err := u.DeleteDocument(context.Background(), "ghost"); err != nil {
		t.Fatalf("DeleteDocument ghost: %v", err)
	}
	if len(pub.deltas) != 0 {
		t.Fatalf("空 delta 不应发布, got %d", len(pub.deltas))
	}

	// repo 删除失败：不动内存图、不发布。
	mr := noOpMockRepo()
	mr.docDeleteFn = func(context.Context, string) error { return fmt.Errorf("db down") }
	u2 := NewUsecaseFromRepo(mr)
	idx2 := NewLinkIndex()
	pub2 := &captureGraphPub{}
	u2.SetLinkIndex(idx2, pub2)
	idx2.LoadAll([]KnowledgeBlockRefEdge{out})
	if err := u2.DeleteDocument(context.Background(), "d2"); err == nil {
		t.Fatal("repo 失败应上抛")
	}
	assertEdgesEqual(t, idx2.OutEdges("bx", nil), out)
	if len(pub2.deltas) != 0 {
		t.Fatal("失败路径不应发布 delta")
	}
}

// TestUsecase_DeleteCollection_LinkIndex 集合删除后内存图同步（G-3：源边消失、
// 外部入边转 dangling）且发布非空 delta；repo 删除失败时不动内存图。
func TestUsecase_DeleteCollection_LinkIndex(t *testing.T) {
	idx := NewLinkIndex()
	pub := &captureGraphPub{}
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkIndex(idx, pub)

	out := refEdge("c2", "d2", "bx", "c1", "d1", "b1", "D1")   // 被删集合出边
	in := refEdge("c1", "d3", "b3", "c2", "d2", "bx", "D2#^x") // 外部入边
	keep := refEdge("c1", "d3", "b4", "c3", "d5", "", "D5")    // 不相关
	idx.LoadAll([]KnowledgeBlockRefEdge{out, in, keep})

	if err := u.DeleteCollection(context.Background(), "c2"); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}
	wantDangling := in
	wantDangling.DstCollectionID, wantDangling.DstDocID, wantDangling.DstBlockID = "", "", ""
	if got := idx.OutEdges("bx", nil); len(got) != 0 {
		t.Fatalf("删除后源边应清除, got %v", got)
	}
	assertEdgesEqual(t, idx.DanglingByCollection("c1", nil), wantDangling)
	assertEdgesEqual(t, idx.OutEdges("b4", nil), keep)
	if len(pub.deltas) != 1 {
		t.Fatalf("发布 delta 数 = %d, want 1", len(pub.deltas))
	}
	d := pub.deltas[0]
	assertEdgesEqual(t, d.Removed, out, in)
	assertEdgesEqual(t, d.Added, wantDangling)

	// repo 删除失败：不动内存图、不发布。
	mr := noOpMockRepo()
	mr.collDeleteFn = func(context.Context, string) error { return fmt.Errorf("db down") }
	u2 := NewUsecaseFromRepo(mr)
	idx2 := NewLinkIndex()
	pub2 := &captureGraphPub{}
	u2.SetLinkIndex(idx2, pub2)
	idx2.LoadAll([]KnowledgeBlockRefEdge{out})
	if err := u2.DeleteCollection(context.Background(), "c2"); err == nil {
		t.Fatal("repo 失败应上抛")
	}
	assertEdgesEqual(t, idx2.OutEdges("bx", nil), out)
	if len(pub2.deltas) != 0 {
		t.Fatal("失败路径不应发布 delta")
	}
}
