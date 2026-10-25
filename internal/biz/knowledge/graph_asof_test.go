package knowledge

// ── G-05-asof（ADR-KN-20261020-J/K）：ListCollectionGraphAsOf 时态图腿 ──────
//
// 契约：span 零值 → 字节级委托 ListCollectionGraph（current 快报，旧端口服务）；
// span 非零且看板 → 节点集/裁剪/degree 组装照旧（当前节点集 × 边时态，ADR-J），
// 边源走 TemporalCollectionLinkReader；时态端口未接线 → 回退当前快照（任一依赖
// 降级不阻断的既有纪律）。邻域经全量图内部复用自动覆盖。

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTemporalCollectionLinkReader struct {
	links    []Link
	err      error
	gotSpan  TemporalSpan
	gotTypes []string
	calls    int
}

func (s *stubTemporalCollectionLinkReader) ListCollectionLinksAsOf(_ context.Context, _ string, linkTypes []string, span TemporalSpan) ([]Link, error) {
	s.calls++
	s.gotSpan = span
	s.gotTypes = linkTypes
	return s.links, s.err
}

func TestUsecase_ListCollectionGraphAsOf_ZeroSpanDelegates(t *testing.T) {
	// 零 span = 字节级等价旧路径：时态端口硬错，旧端口照常服务。
	u := graphUsecase(graphDocs())
	u.SetGraphRepo(&stubCollectionLinkReader{links: []Link{
		{DocID: "d1", TargetDocID: "d2", LinkType: LinkTypeExplicit},
	}})
	u.SetTemporalGraphRepo(&stubTemporalCollectionLinkReader{err: errors.New("temporal port must not be used without span")})

	g, err := u.ListCollectionGraphAsOf(context.Background(), "col-1", nil, "", TemporalSpan{})
	require.NoError(t, err)
	require.Len(t, g.Edges, 1)
	assert.Equal(t, CollectionGraphEdge{Source: "d1", Target: "d2", Type: LinkTypeExplicit}, g.Edges[0])
}

func TestUsecase_ListCollectionGraphAsOf_UsesTemporalPort(t *testing.T) {
	u := graphUsecase(graphDocs())
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	tr := &stubTemporalCollectionLinkReader{links: []Link{
		{DocID: "d1", TargetDocID: "d3", LinkType: LinkTypeExplicit},   // 彼时有效（双端在库）
		{DocID: "d1", TargetDocID: "ghost", LinkType: LinkTypeExplicit}, // 悬空（目标已删）剔除
	}}
	u.SetGraphRepo(&stubCollectionLinkReader{err: errors.New("current port must not be used under as-of")})
	u.SetTemporalGraphRepo(tr)

	span := TemporalSpan{ValidAt: t0}
	g, err := u.ListCollectionGraphAsOf(context.Background(), "col-1", []string{LinkTypeExplicit, ""}, "", span)
	require.NoError(t, err)
	require.Equal(t, 1, tr.calls)
	assert.Equal(t, span, tr.gotSpan, "查询 span 原样下发（值拷贝携带）")
	assert.Equal(t, []string{LinkTypeExplicit}, tr.gotTypes, "空串类型过滤与 current 同规")
	require.Len(t, g.Edges, 1, "悬空边剔除规则与 current 同规")
	assert.Equal(t, CollectionGraphEdge{Source: "d1", Target: "d3", Type: LinkTypeExplicit}, g.Edges[0])
	// degree 按 as-of 边集统计；节点保留当前文档集（孤立节点照常在场）。
	for _, n := range g.Nodes {
		switch n.DocID {
		case "d1", "d3":
			assert.Equal(t, 1, n.Degree)
		default:
			assert.Equal(t, 0, n.Degree)
		}
	}
}

func TestUsecase_ListCollectionGraphAsOf_FallsBackWhenPortMissing(t *testing.T) {
	// span 非零但时态端口未接线 → 回退当前快照（不报错、不空图）。
	u := graphUsecase(graphDocs())
	u.SetGraphRepo(&stubCollectionLinkReader{links: []Link{
		{DocID: "d1", TargetDocID: "d2", LinkType: LinkTypeExplicit},
	}})

	g, err := u.ListCollectionGraphAsOf(context.Background(), "col-1", nil, "",
		TemporalSpan{ValidAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	require.Len(t, g.Edges, 1)
	assert.Equal(t, "d2", g.Edges[0].Target)
}

func TestUsecase_ListDocumentNeighborhoodAsOf_TemporalBFS(t *testing.T) {
	// 彼时边集只有 root→h1a（h1a→h2 等尚不存在）：邻域裁剪饿死在后。
	docs := neighborhoodDocs()
	mr := noOpMockRepo()
	mr.docListFn = func(_ context.Context, _ string, _, _ int) ([]Document, int, error) {
		return docs, len(docs), nil
	}
	byID := make(map[string]Document, len(docs))
	for _, d := range docs {
		byID[d.ID] = d
	}
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return byID[id], nil
	}
	u := NewUsecaseFromRepo(mr)
	span := TemporalSpan{ValidAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	u.SetGraphRepo(&stubCollectionLinkReader{err: errors.New("current port must not be used under as-of")})
	u.SetTemporalGraphRepo(&stubTemporalCollectionLinkReader{links: []Link{
		{DocID: "root", TargetDocID: "h1a", LinkType: LinkTypeExplicit},
	}})

	g, err := u.ListDocumentNeighborhoodAsOf(context.Background(), "root", 2, span)
	require.NoError(t, err)
	ids := nodeIDs(g)
	assert.Len(t, g.Nodes, 2, "彼时邻域=root+h1a（BFS 沿时态边集传播）")
	assert.True(t, ids["root"] && ids["h1a"])
	assert.Len(t, g.Edges, 1)

	// 零 span → 字节级委托旧邻域（旧端口在时会硬错，故换正常旧端口再验路径选择）。
	u.SetGraphRepo(&stubCollectionLinkReader{links: []Link{
		{DocID: "root", TargetDocID: "h1b", LinkType: LinkTypeExplicit},
	}})
	g, err = u.ListDocumentNeighborhoodAsOf(context.Background(), "root", 1, TemporalSpan{})
	require.NoError(t, err)
	ids = nodeIDs(g)
	assert.Len(t, g.Nodes, 2)
	assert.True(t, ids["h1b"])
}
