package graphrag

import (
	"context"
	"fmt"
	"sort"
)

const (
	// maxAnchorEntities 低层实体锚点上限。
	maxAnchorEntities = 8
	// expansionHops 扩展跳数（低层命中文档的 1 跳邻域）。
	expansionHops = 1
	// expansionDecay 扩展命中的 RRF 贡献权重（1 跳邻域弱于直接命中）。
	expansionDecay = 0.7
	// rrfK 倒数排名融合常数。
	rrfK = 60.0
	// communityIterations 高层社区检测迭代轮数。
	communityIterations = 2
)

// ThemeTrace 高层主题：候选图中融合命中数最高的社区。
type ThemeTrace struct {
	CommunityID int    // 社区号（DetectCommunities 标签）
	DocCount    int    // 社区内文档数（候选图节点）
	Summary     string // 社区命中 chunk 的摘要（Summarizer 非 nil 且成功；否则空）
}

// DualTrace 双层检索各阶段计数 + 高层主题。
type DualTrace struct {
	Entities      int // 实体锚点数
	LowDocs       int // 低层候选文档数（去重）
	LowHits       int // 低层 chunk 命中数
	ExpansionDocs int // 扩展候选文档数（去种子、去重）
	ExpansionHits int // 扩展 chunk 命中数
	FusedHits     int // 融合输出条数（截断后）
	Theme         *ThemeTrace
}

// DualLevelSearch 双层图谱检索：
//   - 低层：query → 实体锚点（≤8）→ 去重候选文档 → ChunkRanker 排序；
//   - 扩展：低层命中文档 1 跳邻域扩展候选文档（去种子、字典序）再排序，
//     RRF 贡献 × expansionDecay(0.7)；
//   - 高层：扩展边集（候选图）跑 DetectCommunities（2 轮迭代），融合命中数
//     最高的社区产出 ThemeTrace（平票取最小社区号）；Summarizer 非 nil 时对
//     该社区命中 chunk（按融合序的 ChunkID 列表）调摘要入 trace——摘要不进
//     排序，失败降级为空摘要而不使整体失败；
//   - 融合：RRF(k=60) 合并低层与扩展列表，截断 topK；Score 为融合分，
//     同分按 ChunkID 字典序，保证确定性。
//
// 错误处理：low/expander 为 nil、topK<=0 及低层/扩展端口错误包装上抛；
// 无实体锚点或无候选文档时直接返回空结果与（部分）trace，不再调用下游端口。
func DualLevelSearch(ctx context.Context, query string, low LowLevel, high Summarizer, expander LinkExpander, topK int) ([]ChunkHit, DualTrace, error) {
	var tr DualTrace
	if low == nil {
		return nil, tr, fmt.Errorf("graphrag: low port is nil")
	}
	if expander == nil {
		return nil, tr, fmt.Errorf("graphrag: expander port is nil")
	}
	if topK <= 0 {
		return nil, tr, fmt.Errorf("graphrag: topK must be positive (%d)", topK)
	}

	// 低层：query → 实体锚点。
	entities, err := low.FindEntities(ctx, query, maxAnchorEntities)
	if err != nil {
		return nil, tr, fmt.Errorf("graphrag: find entities: %w", err)
	}
	tr.Entities = len(entities)
	if len(entities) == 0 {
		return nil, tr, nil
	}
	entityIDs := make([]string, 0, len(entities))
	for _, e := range entities {
		entityIDs = append(entityIDs, e.ID)
	}

	// 实体 → 候选文档（按实体锚定序去重，保持确定性）。
	docsOf, err := low.DocsOfEntities(ctx, entityIDs)
	if err != nil {
		return nil, tr, fmt.Errorf("graphrag: docs of entities: %w", err)
	}
	seen := map[string]bool{}
	lowDocs := make([]string, 0, len(docsOf))
	for _, id := range entityIDs {
		for _, d := range docsOf[id] {
			if !seen[d] {
				seen[d] = true
				lowDocs = append(lowDocs, d)
			}
		}
	}
	tr.LowDocs = len(lowDocs)
	if len(lowDocs) == 0 {
		return nil, tr, nil
	}
	lowHits, err := low.Rank(ctx, query, lowDocs, topK)
	if err != nil {
		return nil, tr, fmt.Errorf("graphrag: rank low: %w", err)
	}
	tr.LowHits = len(lowHits)

	// 扩展：命中文档（去重）1 跳邻域 → 候选文档 → 再排序。
	seeds := make([]string, 0, len(lowHits))
	seenSeed := map[string]bool{}
	for _, h := range lowHits {
		if !seenSeed[h.DocID] {
			seenSeed[h.DocID] = true
			seeds = append(seeds, h.DocID)
		}
	}
	var edges []LinkEdge
	var expHits []ChunkHit
	if len(seeds) > 0 {
		edges, err = expander.Neighbors(ctx, seeds, expansionHops)
		if err != nil {
			return nil, tr, fmt.Errorf("graphrag: expand neighbors: %w", err)
		}
		expSet := map[string]bool{}
		for _, e := range edges {
			for _, d := range [2]string{e.SrcDocID, e.DstDocID} {
				if !seenSeed[d] {
					expSet[d] = true
				}
			}
		}
		expDocs := make([]string, 0, len(expSet))
		for d := range expSet {
			expDocs = append(expDocs, d)
		}
		sort.Strings(expDocs)
		tr.ExpansionDocs = len(expDocs)
		if len(expDocs) > 0 {
			expHits, err = low.Rank(ctx, query, expDocs, topK)
			if err != nil {
				return nil, tr, fmt.Errorf("graphrag: rank expansion: %w", err)
			}
			tr.ExpansionHits = len(expHits)
		}
	}

	// 融合：RRF(k=60)，扩展列表贡献加权 expansionDecay。
	fused := rrfFuse(lowHits, expHits, topK)
	tr.FusedHits = len(fused)

	// 高层：候选图社区检测 → 命中数最高社区 → 主题 trace。
	tr.Theme = themeOf(ctx, edges, fused, high)
	return fused, tr, nil
}

// rrfFuse RRF(k=60) 融合：低层列表贡献 1/(k+rank)，扩展列表贡献
// decay×1/(k+rank)（rank 从 1 计）；同一 chunk 跨列表累加。
// 输出按融合分降序、同分按 ChunkID 字典序（确定性），截断 topK。
func rrfFuse(low, expansion []ChunkHit, topK int) []ChunkHit {
	type agg struct {
		docID string
		score float64
	}
	m := map[string]*agg{}
	add := func(h ChunkHit, rank int, w float64) {
		a := m[h.ChunkID]
		if a == nil {
			a = &agg{docID: h.DocID}
			m[h.ChunkID] = a
		}
		a.score += w / (rrfK + float64(rank))
	}
	for i, h := range low {
		add(h, i+1, 1)
	}
	for i, h := range expansion {
		add(h, i+1, expansionDecay)
	}
	out := make([]ChunkHit, 0, len(m))
	for id, a := range m {
		out = append(out, ChunkHit{ChunkID: id, DocID: a.docID, Score: a.score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ChunkID < out[j].ChunkID
	})
	if len(out) > topK {
		out = out[:topK]
	}
	return out
}

// themeOf 高层主题：在扩展边集（候选图）上做社区检测，取融合命中数最高的社区
// （平票取最小社区号）；Summarizer 非 nil 时对该社区命中 chunk 的 ChunkID 列表
// （按融合降序）调摘要，失败降级为空摘要。无边或无社区命中返回 nil。
func themeOf(ctx context.Context, edges []LinkEdge, fused []ChunkHit, high Summarizer) *ThemeTrace {
	if len(edges) == 0 || len(fused) == 0 {
		return nil
	}
	comm := DetectCommunities(edges, communityIterations)
	if len(comm) == 0 {
		return nil
	}
	hits := map[int]int{}
	for _, h := range fused {
		if c, ok := comm[h.DocID]; ok {
			hits[c]++
		}
	}
	if len(hits) == 0 {
		return nil
	}
	best, bestN := -1, -1
	for c, n := range hits {
		if n > bestN || (n == bestN && (best < 0 || c < best)) {
			best, bestN = c, n
		}
	}
	docs := 0
	for _, c := range comm {
		if c == best {
			docs++
		}
	}
	th := &ThemeTrace{CommunityID: best, DocCount: docs}
	if high != nil {
		ids := make([]string, 0, bestN)
		for _, h := range fused {
			if c, ok := comm[h.DocID]; ok && c == best {
				ids = append(ids, h.ChunkID)
			}
		}
		summary, err := high.Summarize(ctx, ids)
		if err == nil {
			th.Summary = summary
		}
	}
	return th
}
