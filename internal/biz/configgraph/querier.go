package configgraph

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// ErrNotReady marks queries against an unbuilt graph（service 映射为
// CONFIG_GRAPH.NOT_READY / 503，design §6）。
var ErrNotReady = errors.New("configgraph: graph not built yet")

// 查询参数边界（design §5.1 depth 参数 + NFR-81-02 行数保护）。
const (
	defaultQueryDepth = 3
	maxQueryDepth     = 10
)

// Querier 提供 Impact/Dependencies/NodeEdges 只读查询（design §5；Health 在
// health.go）。代际来源由调用方注入（rebuilder.Current）；gen==0 一律
// ErrNotReady——首启无图时前端/CLI 可提示"建图中"。
type Querier struct {
	repo Repo
	gen  func() int64
}

// NewQuerier 构造查询器；repo 或 gen 为空时返回 nil（装配侧判空跳过）。
func NewQuerier(repo Repo, gen func() int64) *Querier {
	if repo == nil || gen == nil {
		return nil
	}
	return &Querier{repo: repo, gen: gen}
}

// ImpactSignals 是影响面聚合信号（design §5.1 signals）。
type ImpactSignals struct {
	ActiveSessions int64 `json:"active_sessions"`
	CronTasks      int   `json:"cron_tasks"`
	DefaultTeam    bool  `json:"default_team"`
}

// ImpactNode 是闭包中一个命中节点：depth 为最短跳数，via 为该最短路径的
// 边类型链（从查询原点指向本节点）。
type ImpactNode struct {
	Node  Node     `json:"node"`
	Depth int      `json:"depth"`
	Via   []string `json:"via"`
}

// ImpactResult 是 Impact 查询响应（design §5.1；broken 段独立返回——想引
// 用目标但解析失败的边，见 ListBrokenEdgesTargeting）。
type ImpactResult struct {
	Generation int64         `json:"generation"`
	Target     Node          `json:"target"`
	Nodes      []ImpactNode  `json:"nodes"`
	Signals    ImpactSignals `json:"signals"`
	Risk       Risk          `json:"risk"`
	Broken     []StoredEdge  `json:"broken"`
}

// DependenciesResult 是正向闭包查询响应（design §5.2）。
type DependenciesResult struct {
	Generation int64        `json:"generation"`
	Target     Node         `json:"target"`
	Nodes      []ImpactNode `json:"nodes"`
}

// NodeEdgesResult 是单节点邻接边响应（出/入/断三段，evidence 全量，用于
// "为什么这条边存在"的可解释性排查，design §5.3）。
type NodeEdgesResult struct {
	Generation int64        `json:"generation"`
	Target     Node         `json:"target"`
	Out        []StoredEdge `json:"out"`
	In         []StoredEdge `json:"in"`
	Broken     []StoredEdge `json:"broken"`
}

func clampDepth(depth int) int {
	if depth <= 0 {
		return defaultQueryDepth
	}
	if depth > maxQueryDepth {
		return maxQueryDepth
	}
	return depth
}

func (q *Querier) currentGen() (int64, error) {
	gen := q.gen()
	if gen <= 0 {
		return 0, ErrNotReady
	}
	return gen, nil
}

// aggregateWalk 把 CTE 行折叠为「每节点最短 depth + 对应 via」的有序列表
// + 按边身份去重的命中边集（risk 加权输入）。排序：depth asc → node_type
// → node_key（响应确定性）。
func aggregateWalk(rows []WalkRow) ([]ImpactNode, []StoredEdge) {
	best := make(map[string]int, len(rows)) // nodeID → nodes 下标
	nodes := make([]ImpactNode, 0, len(rows))
	seenEdge := make(map[string]struct{}, len(rows))
	edges := make([]StoredEdge, 0, len(rows))
	for _, r := range rows {
		id := r.Node.ID
		if i, ok := best[id]; ok {
			if r.Depth < nodes[i].Depth {
				nodes[i] = ImpactNode{Node: r.Node, Depth: r.Depth, Via: r.Via}
			}
		} else {
			best[id] = len(nodes)
			nodes = append(nodes, ImpactNode{Node: r.Node, Depth: r.Depth, Via: r.Via})
		}
		ek := r.Edge.SrcID + indexSep + r.Edge.DstID + indexSep + r.Edge.Type
		if _, dup := seenEdge[ek]; !dup {
			seenEdge[ek] = struct{}{}
			edges = append(edges, r.Edge)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Depth != nodes[j].Depth {
			return nodes[i].Depth < nodes[j].Depth
		}
		if nodes[i].Node.NodeType != nodes[j].Node.NodeType {
			return nodes[i].Node.NodeType < nodes[j].Node.NodeType
		}
		return nodes[i].Node.NodeKey < nodes[j].Node.NodeKey
	})
	return nodes, edges
}

// findTarget 双解定位目标节点（ref_id 优先，node_key 兜底）。
func (q *Querier) findTarget(ctx context.Context, gen int64, nodeType, ref string) (Node, error) {
	return q.repo.FindNode(ctx, gen, strings.TrimSpace(nodeType), strings.TrimSpace(ref))
}

// Impact 反向闭包（blast radius）+ signals 聚合 + risk 加权（design §5.1）。
func (q *Querier) Impact(ctx context.Context, nodeType, ref string, depth int) (*ImpactResult, error) {
	gen, err := q.currentGen()
	if err != nil {
		return nil, err
	}
	target, err := q.findTarget(ctx, gen, nodeType, ref)
	if err != nil {
		return nil, err
	}
	rows, err := q.repo.WalkGraph(ctx, gen, target.ID, true, clampDepth(depth))
	if err != nil {
		return nil, err
	}
	nodes, edges := aggregateWalk(rows)

	// signals 聚合（design §5.1；v1 running_cron 简化为命中 cron_task 节点数）。
	var sig ImpactSignals
	agentIDs := make([]string, 0, 8)
	teamIDs := make([]string, 0, 4)
	for _, n := range nodes {
		switch n.Node.NodeType {
		case NodeTypeAgent:
			agentIDs = append(agentIDs, n.Node.RefID)
		case NodeTypeTeam:
			teamIDs = append(teamIDs, n.Node.RefID)
			if !sig.DefaultTeam {
				if b, _ := n.Node.Attrs["is_default"].(bool); b {
					sig.DefaultTeam = true
				}
			}
		case NodeTypeCronTask:
			sig.CronTasks++
		}
	}
	if len(agentIDs) > 0 || len(teamIDs) > 0 {
		sig.ActiveSessions, err = q.repo.CountActiveSessions(ctx, agentIDs, teamIDs)
		if err != nil {
			return nil, err
		}
	}

	// broken 段：dst_key 指向目标（ref_id 或 node_key）的断边。
	keys := make([]string, 0, 2)
	if target.RefID != "" {
		keys = append(keys, target.RefID)
	}
	if target.NodeKey != "" && target.NodeKey != target.RefID {
		keys = append(keys, target.NodeKey)
	}
	broken, err := q.repo.ListBrokenEdgesTargeting(ctx, gen, keys)
	if err != nil {
		return nil, err
	}

	return &ImpactResult{
		Generation: gen,
		Target:     target,
		Nodes:      nodes,
		Signals:    sig,
		Risk:       RiskScore(target, edges, sig),
		Broken:     broken,
	}, nil
}

// Dependencies 正向闭包（design §5.2；agent 的 granted_tool 段与
// GetEffectiveTools 同来源，天然一致）。
func (q *Querier) Dependencies(ctx context.Context, nodeType, ref string, depth int) (*DependenciesResult, error) {
	gen, err := q.currentGen()
	if err != nil {
		return nil, err
	}
	target, err := q.findTarget(ctx, gen, nodeType, ref)
	if err != nil {
		return nil, err
	}
	rows, err := q.repo.WalkGraph(ctx, gen, target.ID, false, clampDepth(depth))
	if err != nil {
		return nil, err
	}
	nodes, _ := aggregateWalk(rows)
	return &DependenciesResult{Generation: gen, Target: target, Nodes: nodes}, nil
}

// NodeEdges 单节点邻接边（出边+入边+broken，design §5.3）。
func (q *Querier) NodeEdges(ctx context.Context, nodeType, ref string) (*NodeEdgesResult, error) {
	gen, err := q.currentGen()
	if err != nil {
		return nil, err
	}
	target, err := q.findTarget(ctx, gen, nodeType, ref)
	if err != nil {
		return nil, err
	}
	out, in, broken, err := q.repo.ListNodeEdges(ctx, gen, target.ID)
	if err != nil {
		return nil, err
	}
	return &NodeEdgesResult{Generation: gen, Target: target, Out: out, In: in, Broken: broken}, nil
}
