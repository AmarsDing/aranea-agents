package knowledge

// ── G4-B8：ListCollectionGraph 单库全量图谱（3D 图谱数据源） ────────────────

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubCollectionLinkReader struct {
	links     []Link
	err       error
	gotTypes  []string
	gotCollID string
}

func (s *stubCollectionLinkReader) ListCollectionLinks(_ context.Context, collectionID string, linkTypes []string) ([]Link, error) {
	s.gotCollID = collectionID
	s.gotTypes = linkTypes
	return s.links, s.err
}

func graphUsecase(docs []Document) *Usecase {
	mr := noOpMockRepo()
	mr.docListFn = func(_ context.Context, _ string, _, _ int) ([]Document, int, error) {
		return docs, len(docs), nil
	}
	return NewUsecaseFromRepo(mr)
}

func graphDocs() []Document {
	return []Document{
		{ID: "d1", Source: "a.md", RelPath: "notes/a.md", DocType: "note"},
		{ID: "d2", Source: "b.md", RelPath: "notes/b.md", DocType: "note"},
		{ID: "d3", Source: "q1.md", RelPath: "reports/q1.md", DocType: "report"},
		{ID: "d4", Source: "readme.md", RelPath: "readme.md"}, // 孤立节点
	}
}

func TestUsecase_ListCollectionGraph_Basic(t *testing.T) {
	u := graphUsecase(graphDocs())
	lr := &stubCollectionLinkReader{links: []Link{
		{DocID: "d1", TargetDocID: "d2", LinkType: LinkTypeExplicit},
		{DocID: "d2", TargetDocID: "d1", LinkType: LinkTypeExplicit}, // 反向独立边（显式双链双向）
		{DocID: "d1", TargetDocID: "d3", LinkType: LinkTypeEntity},
	}}
	u.SetGraphRepo(lr)

	g, err := u.ListCollectionGraph(context.Background(), "col-1", nil, "")
	require.NoError(t, err)
	assert.Equal(t, "col-1", lr.gotCollID)

	require.Len(t, g.Nodes, 4)
	byID := map[string]CollectionGraphNode{}
	for _, n := range g.Nodes {
		byID[n.DocID] = n
	}
	assert.Equal(t, 3, byID["d1"].Degree, "d1: 出 d2/d3 + 入 d2")
	assert.Equal(t, 2, byID["d2"].Degree)
	assert.Equal(t, 1, byID["d3"].Degree)
	assert.Equal(t, 0, byID["d4"].Degree, "孤立节点 degree=0 且保留（前端开关控制显隐）")
	assert.Equal(t, "a.md", byID["d1"].Name)
	assert.Equal(t, "notes/a.md", byID["d1"].RelPath)
	assert.Equal(t, "note", byID["d1"].DocType)

	require.Len(t, g.Edges, 3)
	assert.Equal(t, CollectionGraphEdge{Source: "d1", Target: "d2", Type: LinkTypeExplicit}, g.Edges[0])
	assert.Equal(t, CollectionGraphEdge{Source: "d1", Target: "d3", Type: LinkTypeEntity}, g.Edges[2])
}

func TestUsecase_ListCollectionGraph_LinkTypeFilter(t *testing.T) {
	u := graphUsecase(graphDocs())
	lr := &stubCollectionLinkReader{links: []Link{
		{DocID: "d1", TargetDocID: "d2", LinkType: LinkTypeExplicit},
	}}
	u.SetGraphRepo(lr)

	types := []string{LinkTypeExplicit, ""} // 空串由 biz 过滤，不下发 SQL
	g, err := u.ListCollectionGraph(context.Background(), "col-1", types, "")
	require.NoError(t, err)
	assert.Equal(t, []string{LinkTypeExplicit}, lr.gotTypes, "空串类型必须过滤")
	require.Len(t, g.Edges, 1)
	assert.Equal(t, LinkTypeExplicit, g.Edges[0].Type)
	// degree 只按过滤后的边计。
	for _, n := range g.Nodes {
		switch n.DocID {
		case "d1", "d2":
			assert.Equal(t, 1, n.Degree)
		default:
			assert.Equal(t, 0, n.Degree)
		}
	}
}

func TestUsecase_ListCollectionGraph_PathPrefix(t *testing.T) {
	u := graphUsecase(graphDocs())
	lr := &stubCollectionLinkReader{links: []Link{
		{DocID: "d1", TargetDocID: "d2", LinkType: LinkTypeExplicit}, // 两端均在 notes/ → 保留
		{DocID: "d1", TargetDocID: "d3", LinkType: LinkTypeExplicit}, // d3 出范围 → 剔除
	}}
	u.SetGraphRepo(lr)

	// 首尾斜杠容忍（"/notes/" ≡ "notes/"）。
	for _, prefix := range []string{"notes/", "/notes/", "notes"} {
		g, err := u.ListCollectionGraph(context.Background(), "col-1", nil, prefix)
		require.NoError(t, err, "prefix=%q", prefix)
		require.Len(t, g.Nodes, 2, "prefix=%q：仅 notes/ 下文档", prefix)
		require.Len(t, g.Edges, 1, "prefix=%q：跨界边剔除", prefix)
		assert.Equal(t, 1, g.Nodes[0].Degree)
		assert.Equal(t, 1, g.Nodes[1].Degree)
	}

	// 目录边界：note 不得误中 notes/。
	g, err := u.ListCollectionGraph(context.Background(), "col-1", nil, "note")
	require.NoError(t, err)
	assert.Empty(t, g.Nodes)
}

func TestUsecase_ListCollectionGraph_DanglingEdgeDropped(t *testing.T) {
	u := graphUsecase(graphDocs())
	lr := &stubCollectionLinkReader{links: []Link{
		{DocID: "d1", TargetDocID: "ghost", LinkType: LinkTypeExplicit}, // 目标文档已删
		{DocID: "ghost", TargetDocID: "d1", LinkType: LinkTypeExplicit}, // 源文档已删
	}}
	u.SetGraphRepo(lr)

	g, err := u.ListCollectionGraph(context.Background(), "col-1", nil, "")
	require.NoError(t, err)
	assert.Empty(t, g.Edges, "悬空边（端点不在文档集）必须剔除")
	for _, n := range g.Nodes {
		assert.Equal(t, 0, n.Degree)
	}
}

func TestUsecase_ListCollectionGraph_NotWired(t *testing.T) {
	u := graphUsecase(graphDocs())
	g, err := u.ListCollectionGraph(context.Background(), "col-1", nil, "")
	require.NoError(t, err, "未接线 CollectionLinkReader 降级为仅节点（与 ListDocumentLinks 一致）")
	require.Len(t, g.Nodes, 4)
	assert.Empty(t, g.Edges)
}

func TestUsecase_ListCollectionGraph_Errors(t *testing.T) {
	u := graphUsecase(nil)
	_, err := u.ListCollectionGraph(context.Background(), "", nil, "")
	require.ErrorIs(t, err, ErrCollectionIDRequired)

	u.SetGraphRepo(&stubCollectionLinkReader{err: fmt.Errorf("db down")})
	_, err = u.ListCollectionGraph(context.Background(), "col-1", nil, "")
	require.Error(t, err, "repo 错误透传")
}
