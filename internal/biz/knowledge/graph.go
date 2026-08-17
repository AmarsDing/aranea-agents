package knowledge

import (
	"context"
	"strings"
)

// ── G4-B8：单库全量图谱（3D 知识图谱数据源，设计 §V12.3 B8 / §V12.7） ────────

// CollectionGraphNode 图谱节点：文档 + 连接度（大小映射；孤立节点 degree=0 保留，
// 前端「显示孤立节点」开关控制显隐）。
type CollectionGraphNode struct {
	DocID   string
	Name    string
	RelPath string
	DocType string
	Degree  int
}

// CollectionGraphEdge 图谱边：doc_id 间有向关联（explicit/entity/semantic）。
type CollectionGraphEdge struct {
	Source string
	Target string
	Type   string
}

// CollectionGraph 单库全量图谱（<2k 节点一次性返回，不做分页）。
type CollectionGraph struct {
	Nodes []CollectionGraphNode
	Edges []CollectionGraphEdge
}

// CollectionLinkReader 库级关联读取窄接口（G4-B8；与文档级 LinkRepo 分离保持窄接口）。
// Stability:evolving
type CollectionLinkReader interface {
	// ListCollectionLinks 列出库内全部关联；linkTypes 空 = 全部类型。
	ListCollectionLinks(ctx context.Context, collectionID string, linkTypes []string) ([]Link, error)
}

// SetGraphRepo 接线库级关联读取（可选；未接线时图谱降级为仅节点无边）。
func (u *Usecase) SetGraphRepo(links CollectionLinkReader) {
	u.graphLinks = links
}

// maxGraphDocs 单库图谱节点上限保护（与 maxLinkCandidates 同级）。
const maxGraphDocs = 10000

// normalizeGraphPrefix 目录前缀归一：去空白 + 去首尾斜杠 + 补尾斜杠（"" = 全库）。
// 与 B7 pathPrefixClause 语义一致：目录边界（note 不命中 notes/）。
func normalizeGraphPrefix(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	return p + "/"
}

// ListCollectionGraph 返回单库全量图谱：节点 = 库内文档（pathPrefix 过滤后），
// 边 = 关联（linkTypes 过滤；端点须在节点集内——出范围/悬空剔除），degree 按入边集统计。
// 未接线 CollectionLinkReader 时降级为仅节点（与 ListDocumentLinks 未接线降级一致）。
func (u *Usecase) ListCollectionGraph(ctx context.Context, collectionID string, linkTypes []string, pathPrefix string) (*CollectionGraph, error) {
	if err := u.requireRepo(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(collectionID) == "" {
		return nil, ErrCollectionIDRequired
	}
	docs, _, err := u.ListDocuments(ctx, collectionID, maxGraphDocs, 0)
	if err != nil {
		return nil, err
	}
	prefix := normalizeGraphPrefix(pathPrefix)
	g := &CollectionGraph{Nodes: make([]CollectionGraphNode, 0, len(docs))}
	inScope := make(map[string]int, len(docs)) // docID → node index
	for _, d := range docs {
		if prefix != "" && !strings.HasPrefix(d.RelPath, prefix) {
			continue
		}
		inScope[d.ID] = len(g.Nodes)
		g.Nodes = append(g.Nodes, CollectionGraphNode{
			DocID:   d.ID,
			Name:    d.Source,
			RelPath: d.RelPath,
			DocType: d.DocType,
		})
	}
	if u.graphLinks == nil {
		return g, nil
	}
	// 空串类型过滤（前端 chips 全不选语义 = 全部，但防御空串混入微废 ANY 匹配）。
	types := make([]string, 0, len(linkTypes))
	for _, lt := range linkTypes {
		if lt = strings.TrimSpace(lt); lt != "" {
			types = append(types, lt)
		}
	}
	links, err := u.graphLinks.ListCollectionLinks(ctx, collectionID, types)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		si, okS := inScope[l.DocID]
		ti, okT := inScope[l.TargetDocID]
		if !okS || !okT {
			continue // 端点出范围或悬空（文档已删）→ 剔除
		}
		g.Edges = append(g.Edges, CollectionGraphEdge{Source: l.DocID, Target: l.TargetDocID, Type: l.LinkType})
		g.Nodes[si].Degree++
		g.Nodes[ti].Degree++
	}
	return g, nil
}

// ── SP2-8：文档邻域子图（右栏局部图数据源） ─────────────────────────────

// 邻域 BFS 跳数：默认 2、上限 5（与右栏局部图 slider 范围一致）。
const (
	defaultNeighborhoodHops = 2
	maxNeighborhoodHops     = 5
)

// ListDocumentNeighborhood 返回以 docID 为根的 N 跳无向邻域子图（SP2-8 右栏局部图）：
// 复用全量图谱组装后服务端 BFS 裁剪，仅向客户端传输小邻域（大库免全图传输）；
// 节点 Degree 保留全图口径（与此前前端客户端 BFS 的展示语义一致）。
func (u *Usecase) ListDocumentNeighborhood(ctx context.Context, docID string, hops int) (*CollectionGraph, error) {
	if err := u.requireRepo(); err != nil {
		return nil, err
	}
	doc, err := u.GetDocument(ctx, docID)
	if err != nil {
		return nil, err
	}
	if hops <= 0 {
		hops = defaultNeighborhoodHops
	}
	if hops > maxNeighborhoodHops {
		hops = maxNeighborhoodHops
	}
	full, err := u.ListCollectionGraph(ctx, doc.CollectionID, nil, "")
	if err != nil {
		return nil, err
	}
	return bfsNeighborhood(full, doc.ID, hops), nil
}

// bfsNeighborhood 以 rootID 为根无向 BFS 裁剪 N 跳邻域（反链/出链均可达）；
// root 不在节点集时返回仅含可达边的空图（实际 root 恒在——由文档列表组装）。
func bfsNeighborhood(g *CollectionGraph, rootID string, hops int) *CollectionGraph {
	adj := make(map[string][]string, len(g.Edges)*2)
	for _, e := range g.Edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		adj[e.Target] = append(adj[e.Target], e.Source)
	}
	seen := map[string]bool{rootID: true}
	frontier := []string{rootID}
	for h := 0; h < hops && len(frontier) > 0; h++ {
		var next []string
		for _, id := range frontier {
			for _, nb := range adj[id] {
				if !seen[nb] {
					seen[nb] = true
					next = append(next, nb)
				}
			}
		}
		frontier = next
	}
	out := &CollectionGraph{}
	for _, n := range g.Nodes {
		if seen[n.DocID] {
			out.Nodes = append(out.Nodes, n)
		}
	}
	for _, e := range g.Edges {
		if seen[e.Source] && seen[e.Target] {
			out.Edges = append(out.Edges, e)
		}
	}
	return out
}
