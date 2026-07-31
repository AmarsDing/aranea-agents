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
