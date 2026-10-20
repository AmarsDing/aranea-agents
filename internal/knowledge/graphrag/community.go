package graphrag

import "sort"

// DetectCommunities 确定性标签传播社区检测（LPA 变体）。
//
// 输入边无向化：同一无序文档对的多条边权重累加（edge.Weight<=0 按 1 计），
// 自环忽略。初始化每点一社区：社区号 = 节点文档 ID 的字典序位次。
// 每轮为同步更新（全部节点基于上一轮快照同时重算）：按文档字典序扫描节点，
// 取邻居社区票数（权重累加）最高者，平票取最小社区号；无邻居节点保持原社区。
// iterations<=0 时直接返回初始分配。
//
// 邻居按字典序累加票数，避免浮点累加顺序影响结果：同一输入必得同一输出，
// 满足主题 trace 的可复现要求。
func DetectCommunities(edges []LinkEdge, iterations int) map[string]int {
	adj := map[string]map[string]float64{}
	add := func(a, b string, w float64) {
		if a == b {
			return
		}
		m := adj[a]
		if m == nil {
			m = map[string]float64{}
			adj[a] = m
		}
		m[b] += w
	}
	for _, e := range edges {
		w := e.Weight
		if !(w > 0) { // 非正/NaN 均按 1 计，保证票数可比较、argmax 确定
			w = 1
		}
		add(e.SrcDocID, e.DstDocID, w)
		add(e.DstDocID, e.SrcDocID, w)
	}
	nodes := make([]string, 0, len(adj))
	for id := range adj {
		nodes = append(nodes, id)
	}
	sort.Strings(nodes)
	label := make(map[string]int, len(nodes))
	for i, id := range nodes {
		label[id] = i
	}
	for it := 0; it < iterations; it++ {
		next := make(map[string]int, len(nodes))
		for _, id := range nodes {
			nbrs := make([]string, 0, len(adj[id]))
			for nb := range adj[id] {
				nbrs = append(nbrs, nb)
			}
			sort.Strings(nbrs)
			votes := map[int]float64{}
			for _, nb := range nbrs {
				votes[label[nb]] += adj[id][nb]
			}
			best, bestV, ok := label[id], 0.0, false
			for l, v := range votes {
				if !ok || v > bestV || (v == bestV && l < best) {
					best, bestV, ok = l, v, true
				}
			}
			next[id] = best
		}
		label = next
	}
	return label
}
