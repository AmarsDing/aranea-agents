package configgraph

import (
	"context"
	"math"
	"sort"
	"strings"
)

// Health 四类分析（design §5.4）：
//
//	god node   fan-in/fan-out 超 P95（非零值最近秩）或绝对阈值（≥20/≥30）的并集，
//	           附主导方向 top 边明细
//	引用环     内存三色 DFS（skill_parent/org_parent/linked_graph↔graph_owned_by
//	           混合环），规范化旋转去重
//	断边       按 edge_type 分组计数
//	重复 prompt prompt_file 节点按 attrs.body_hash 分组取 COUNT>1
//
// profile 噪音裁定（验收 1.4「eval_memory_probe 在 god node 前列且不被
// profile 噪音淹没」）：grant_origin=profile 的 granted_tool 边在 fan-in 与
// fan-out 两个方向都不计入——design §5.4 字面只在 fan-out 上标注排除，但
// 其理由（防基础工具告警疲劳）对入向对称成立：profile 内置工具若计入
// fan-in，每个 agent 都贡献一条入边，真正被显式授予的 god node（如
// eval_memory_probe）会被淹没。故此处按验收语义双向排除。

const (
	godFanInAbsThreshold  = 20
	godFanOutAbsThreshold = 30
	godTopEdgesCap        = 10
	godPercentile         = 0.95
)

// cycleEdgeTypes 是引用环检测的边类型子图（design §5.4：skill_parent、
// org_parent、linked_graph↔graph_owned_by 混合环）。
var cycleEdgeTypes = map[string]bool{
	EdgeTypeSkillParent:   true,
	EdgeTypeOrgParent:     true,
	EdgeTypeLinkedGraph:   true,
	EdgeTypeGraphOwnedBy:  true,
}

// NodeRef 是健康报告中节点的轻量引用（不带 attrs，控制响应体积）。
type NodeRef struct {
	ID          string `json:"id"`
	NodeType    string `json:"node_type"`
	NodeKey     string `json:"node_key"`
	DisplayName string `json:"display_name"`
}

func nodeRefOf(n Node) NodeRef {
	return NodeRef{ID: n.ID, NodeType: n.NodeType, NodeKey: n.NodeKey, DisplayName: n.DisplayName}
}

// GodNode 是 fan 统计超阈值的节点；TopEdges 为主导方向（fan 较大侧）的
// 明细边（≤10 条，edge_type→对端 id 排序，确定性输出）。
type GodNode struct {
	Node     NodeRef      `json:"node"`
	FanIn    int          `json:"fan_in"`
	FanOut   int          `json:"fan_out"`
	TopEdges []StoredEdge `json:"top_edges"`
}

// Cycle 是一条引用环：Nodes 按环序排列（最小节点 ID 开头，规范化），
// Edges[i] 是 Nodes[i] → Nodes[i+1] 的边类型（末元素指回 Nodes[0]），
// len(Edges) == len(Nodes)。
type Cycle struct {
	Nodes []NodeRef `json:"nodes"`
	Edges []string  `json:"edges"`
}

// BrokenGroup 按边类型聚合断边。
type BrokenGroup struct {
	EdgeType string `json:"edge_type"`
	Count    int    `json:"count"`
}

// PromptDupGroup 是一组 body_hash 相同的活跃 prompt_file 节点。
type PromptDupGroup struct {
	BodyHash string    `json:"body_hash"`
	Count    int       `json:"count"`
	Nodes    []NodeRef `json:"nodes"`
}

// HealthReport 是四类健康度分析的完整响应（design §6 GET /config-graph/health）。
type HealthReport struct {
	Generation       int64            `json:"generation"`
	GodNodes         []GodNode        `json:"god_nodes"`
	Cycles           []Cycle          `json:"cycles"`
	BrokenByType     []BrokenGroup    `json:"broken_by_type"`
	DuplicatePrompts []PromptDupGroup `json:"duplicate_prompts"`
}

// Health 计算当前代的四类健康度分析（design §5.4）。gen==0 → ErrNotReady。
func (q *Querier) Health(ctx context.Context) (*HealthReport, error) {
	gen, err := q.currentGen()
	if err != nil {
		return nil, err
	}
	nodes, err := q.repo.ListAllNodes(ctx, gen)
	if err != nil {
		return nil, err
	}
	edges, err := q.repo.ListAllEdges(ctx, gen)
	if err != nil {
		return nil, err
	}
	return &HealthReport{
		Generation:       gen,
		GodNodes:         detectGodNodes(nodes, edges),
		Cycles:           detectCycles(nodes, edges),
		BrokenByType:     groupBrokenEdges(edges),
		DuplicatePrompts: groupDuplicatePrompts(nodes),
	}, nil
}

// ── god node ─────────────────────────────────────────────────────────────

// fanCountable 判定边是否计入 fan 统计：断边与 profile 来源的 granted_tool
// 边不计（双向排除，见文件头裁定）。
func fanCountable(e StoredEdge) bool {
	if e.Broken() || e.DstID == "" {
		return false
	}
	if e.Type == EdgeTypeGrantedTool && grantOriginOf(e.Evidence) == GrantOriginProfile {
		return false
	}
	return true
}

// percentile95 最近秩法 P95（输入仅非零值；空输入返回 0）。
func percentile95(vals []int) int {
	if len(vals) == 0 {
		return 0
	}
	sorted := append([]int(nil), vals...)
	sort.Ints(sorted)
	rank := int(math.Ceil(godPercentile * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

func nonZeroValues(m map[string]int) []int {
	vals := make([]int, 0, len(m))
	for _, v := range m {
		if v > 0 {
			vals = append(vals, v)
		}
	}
	return vals
}

// detectGodNodes 计算 god node 列表：P95（严格超过）或绝对阈值（≥）的并集，
// 按 max(fanIn,fanOut) 降序 → node_type → node_key 排序。
func detectGodNodes(nodes []Node, edges []StoredEdge) []GodNode {
	fanIn := make(map[string]int, len(nodes))
	fanOut := make(map[string]int, len(nodes))
	inEdges := make(map[string][]StoredEdge)
	outEdges := make(map[string][]StoredEdge)
	for _, e := range edges {
		if !fanCountable(e) {
			continue
		}
		fanOut[e.SrcID]++
		fanIn[e.DstID]++
		outEdges[e.SrcID] = append(outEdges[e.SrcID], e)
		inEdges[e.DstID] = append(inEdges[e.DstID], e)
	}
	p95In := percentile95(nonZeroValues(fanIn))
	p95Out := percentile95(nonZeroValues(fanOut))
	nodeByID := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		nodeByID[n.ID] = n
	}
	out := make([]GodNode, 0, 8)
	for id, n := range nodeByID {
		fi, fo := fanIn[id], fanOut[id]
		if fi == 0 && fo == 0 {
			continue
		}
		god := fi >= godFanInAbsThreshold || fo >= godFanOutAbsThreshold ||
			(fi > p95In && p95In > 0) || (fo > p95Out && p95Out > 0)
		if !god {
			continue
		}
		out = append(out, GodNode{
			Node:     nodeRefOf(n),
			FanIn:    fi,
			FanOut:   fo,
			TopEdges: godTopEdges(fi, fo, inEdges[id], outEdges[id]),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		mi, mj := max(out[i].FanIn, out[i].FanOut), max(out[j].FanIn, out[j].FanOut)
		if mi != mj {
			return mi > mj
		}
		if out[i].Node.NodeType != out[j].Node.NodeType {
			return out[i].Node.NodeType < out[j].Node.NodeType
		}
		return out[i].Node.NodeKey < out[j].Node.NodeKey
	})
	return out
}

// godTopEdges 取主导方向（fan 较大侧，平局取入向）的明细边，排序截断。
func godTopEdges(fanIn, fanOut int, inEdges, outEdges []StoredEdge) []StoredEdge {
	dominant := inEdges
	if fanOut > fanIn {
		dominant = outEdges
	}
	sorted := append([]StoredEdge(nil), dominant...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type < sorted[j].Type
		}
		if sorted[i].SrcID != sorted[j].SrcID {
			return sorted[i].SrcID < sorted[j].SrcID
		}
		return sorted[i].DstID < sorted[j].DstID
	})
	if len(sorted) > godTopEdgesCap {
		sorted = sorted[:godTopEdgesCap]
	}
	return sorted
}

// ── 引用环 ────────────────────────────────────────────────────────────────

// detectCycles 三色迭代 DFS 找环（每个回边报告一条环，规范化旋转去重；
// 非穷举所有基本环——v1 健康报告只需检出存在性 + 可读链）。
func detectCycles(nodes []Node, edges []StoredEdge) []Cycle {
	type arc struct {
		to  string
		typ string
	}
	adj := make(map[string][]arc)
	for _, e := range edges {
		if e.Broken() || e.DstID == "" || !cycleEdgeTypes[e.Type] {
			continue
		}
		adj[e.SrcID] = append(adj[e.SrcID], arc{to: e.DstID, typ: e.Type})
	}
	if len(adj) == 0 {
		return []Cycle{}
	}
	for id := range adj { // 邻接排序保证遍历确定
		sort.Slice(adj[id], func(i, j int) bool {
			if adj[id][i].to != adj[id][j].to {
				return adj[id][i].to < adj[id][j].to
			}
			return adj[id][i].typ < adj[id][j].typ
		})
	}
	starts := make([]string, 0, len(adj))
	for id := range adj {
		starts = append(starts, id)
	}
	sort.Strings(starts)

	nodeByID := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		nodeByID[n.ID] = n
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(adj))
	seen := make(map[string]bool)
	cycles := make([]Cycle, 0, 2)

	type frame struct {
		id       string
		childIdx int
	}
	for _, start := range starts {
		if color[start] != white {
			continue
		}
		color[start] = gray
		stack := []frame{{id: start}}
		pathTypes := make([]string, 0, 8) // pathTypes[i-1] = stack[i-1]→stack[i] 边类型
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.childIdx >= len(adj[top.id]) {
				color[top.id] = black
				stack = stack[:len(stack)-1]
				if len(pathTypes) > 0 {
					pathTypes = pathTypes[:len(pathTypes)-1]
				}
				continue
			}
			a := adj[top.id][top.childIdx]
			top.childIdx++
			switch color[a.to] {
			case white:
				color[a.to] = gray
				stack = append(stack, frame{id: a.to})
				pathTypes = append(pathTypes, a.typ)
			case gray:
				// 回边 → 环：stack 中 a.to 位置到栈顶 + 回边闭合。
				pos := 0
				for i := range stack {
					if stack[i].id == a.to {
						pos = i
						break
					}
				}
				ids := make([]string, 0, len(stack)-pos)
				for _, f := range stack[pos:] {
					ids = append(ids, f.id)
				}
				types := append(append([]string(nil), pathTypes[pos:]...), a.typ)
				key := canonicalCycleKey(ids)
				if !seen[key] {
					seen[key] = true
					cycles = append(cycles, buildCycle(ids, types, nodeByID))
				}
			}
		}
	}
	return cycles
}

// canonicalCycleKey 规范化环（最小 ID 旋转开头）用于去重——同一有向环从
// 任一成员出发得到相同 key。
func canonicalCycleKey(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	minIdx := 0
	for i := 1; i < len(ids); i++ {
		if ids[i] < ids[minIdx] {
			minIdx = i
		}
	}
	var sb strings.Builder
	for i := range ids {
		if i > 0 {
			sb.WriteString("→")
		}
		sb.WriteString(ids[(minIdx+i)%len(ids)])
	}
	return sb.String()
}

// buildCycle 构造规范化输出的环：节点从最小 ID 开始按环序排列，边类型链
// 同步旋转（Edges[i] = Nodes[i]→Nodes[i+1]，末位指回 Nodes[0]）。
func buildCycle(ids, types []string, nodeByID map[string]Node) Cycle {
	minIdx := 0
	for i := 1; i < len(ids); i++ {
		if ids[i] < ids[minIdx] {
			minIdx = i
		}
	}
	n := len(ids)
	cyc := Cycle{Nodes: make([]NodeRef, 0, n), Edges: make([]string, 0, n)}
	for i := 0; i < n; i++ {
		id := ids[(minIdx+i)%n]
		cyc.Nodes = append(cyc.Nodes, nodeRefOf(nodeByID[id]))
		// types[k] 是 ids[k]→ids[k+1] 的边（末位指回 ids[0]），随节点同步旋转。
		cyc.Edges = append(cyc.Edges, types[(minIdx+i)%n])
	}
	return cyc
}

// ── 断边分组 ──────────────────────────────────────────────────────────────

func groupBrokenEdges(edges []StoredEdge) []BrokenGroup {
	counts := make(map[string]int)
	for _, e := range edges {
		if e.Broken() {
			counts[e.Type]++
		}
	}
	out := make([]BrokenGroup, 0, len(counts))
	for typ, n := range counts {
		out = append(out, BrokenGroup{EdgeType: typ, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].EdgeType < out[j].EdgeType
	})
	return out
}

// ── 重复 prompt ───────────────────────────────────────────────────────────

func groupDuplicatePrompts(nodes []Node) []PromptDupGroup {
	groups := make(map[string][]Node)
	for _, n := range nodes {
		if n.NodeType != NodeTypePromptFile || n.Status == NodeStatusDeleted {
			continue
		}
		h, _ := n.Attrs["body_hash"].(string)
		if h == "" {
			continue
		}
		groups[h] = append(groups[h], n)
	}
	out := make([]PromptDupGroup, 0, len(groups))
	for h, ns := range groups {
		if len(ns) < 2 {
			continue
		}
		sort.Slice(ns, func(i, j int) bool { return ns[i].NodeKey < ns[j].NodeKey })
		refs := make([]NodeRef, 0, len(ns))
		for _, n := range ns {
			refs = append(refs, nodeRefOf(n))
		}
		out = append(out, PromptDupGroup{BodyHash: h, Count: len(ns), Nodes: refs})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].BodyHash < out[j].BodyHash
	})
	return out
}
