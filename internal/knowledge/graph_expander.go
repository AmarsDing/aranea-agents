package knowledge

import (
	"context"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// Lazy GraphRAG：查询时用已有 knowledge_links（explicit/entity/semantic）做一跳扩展。
// 不新建实体表、不嵌入摄取主链路——索引期零额外成本，检索期把「给人看的图谱」接到召回。

const (
	graphExpandSeedDocs      = 5
	graphExpandMaxNeighbors  = 8
	graphExpandChunksPerDoc  = 2
	graphExpandTimeout       = 800 * time.Millisecond
	graphExpandNeighborDecay = float32(0.72)
	// 自治理图谱 M1-4 扩散激活：2 跳、每跳能量 ×0.5、侧抑制 top-N。
	graphSpreadHops          = 2
	graphSpreadHopDecay      = 0.5
	graphExpandActivationCap = 10
)

// NeighborLinkReader 文档一跳邻域（双向 links）。生产实现：data.knowledgeRepo.ListLinks。
type NeighborLinkReader interface {
	ListLinks(ctx context.Context, collectionID, docID, linkType string) ([]bizknowledge.Link, error)
}

// NeighborChunkLister 按文档取前 N 个 chunk（chunk_index 升序）。生产实现：data.knowledgeRepo.ListChunksByDocuments。
type NeighborChunkLister interface {
	ListChunksByDocuments(ctx context.Context, collectionID string, docIDs []string, limitPerDoc int) ([]biz.KnowledgeChunk, error)
}

// GraphExpander 在向量/混合检索种子上叠加图扩展。
// 接线 ActiveLinkReader 后走 M1-4 受限扩散激活（2 跳 BFS）；否则保持 1 跳旧行为。
type GraphExpander struct {
	links  NeighborLinkReader
	chunks NeighborChunkLister
	active bizknowledge.ActiveLinkReader
	lg     loggateway.Logger
}

func NewGraphExpander(links NeighborLinkReader, chunks NeighborChunkLister, lg loggateway.Logger) *GraphExpander {
	if links == nil || chunks == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &GraphExpander{links: links, chunks: chunks, lg: lg}
}

// SetActiveLinks 接线 active 边读取（M1-4 扩散激活；nil = 维持 1 跳旧行为）。
func (e *GraphExpander) SetActiveLinks(r bizknowledge.ActiveLinkReader) {
	if e == nil {
		return
	}
	e.active = r
}

// Expand 返回种子 ∪ 邻居文档 chunks，按分数截断到 q.TopK（<=0 时默认 5）。
// 即时定位查询（路径/扩展名/引号短语）跳过扩展，避免污染精确命中。
// 任一依赖失败时返回原始种子（降级，不报错）。
func (e *GraphExpander) Expand(ctx context.Context, q biz.KnowledgeSearchQuery, seeds []biz.KnowledgeChunk) []biz.KnowledgeChunk {
	if e == nil || len(seeds) == 0 {
		return seeds
	}
	if strings.TrimSpace(q.CollectionID) == "" {
		return seeds
	}
	if ClassifySearchIntent(q.Query) == IntentInstant {
		return seeds
	}

	topK := q.TopK
	if topK <= 0 {
		topK = 5
	}

	expandCtx, cancel := context.WithTimeout(ctx, graphExpandTimeout)
	defer cancel()

	seedDocs := uniqueSeedDocs(seeds, graphExpandSeedDocs)
	if e.active != nil {
		return e.spreadExpand(expandCtx, q, seeds, seedDocs, topK)
	}
	neighbors := e.collectNeighbors(expandCtx, q.CollectionID, seedDocs)
	if len(neighbors) == 0 {
		return trimChunks(seeds, topK)
	}

	neighborChunks, err := e.chunks.ListChunksByDocuments(expandCtx, q.CollectionID, neighbors, graphExpandChunksPerDoc)
	if err != nil {
		e.lg.Warn("图扩展取邻居 chunks 失败，使用种子结果",
			loggateway.StepID("knowledge.graph_expand.chunks_fail"),
			loggateway.Err(err),
			loggateway.Str("collection_id", q.CollectionID))
		return trimChunks(seeds, topK)
	}
	if len(neighborChunks) == 0 {
		return trimChunks(seeds, topK)
	}

	seedIDs := make(map[string]struct{}, len(seeds))
	maxSeed := float32(0)
	for _, ch := range seeds {
		seedIDs[ch.ID] = struct{}{}
		if ch.Score > maxSeed {
			maxSeed = ch.Score
		}
	}
	if maxSeed <= 0 {
		maxSeed = 1
	}

	scored := make([]biz.KnowledgeChunk, 0, len(neighborChunks))
	for _, ch := range neighborChunks {
		if _, dup := seedIDs[ch.ID]; dup {
			continue
		}
		ch.Score = maxSeed * graphExpandNeighborDecay
		scored = append(scored, ch)
	}
	if len(scored) == 0 {
		return trimChunks(seeds, topK)
	}
	return MergeSearchResults(seeds, scored, topK)
}

type neighborRank struct {
	docID string
	score int
}

func (e *GraphExpander) collectNeighbors(ctx context.Context, collectionID string, seedDocs []string) []string {
	seedSet := make(map[string]struct{}, len(seedDocs))
	for _, id := range seedDocs {
		seedSet[id] = struct{}{}
	}
	ranks := make(map[string]int)
	for _, docID := range seedDocs {
		if ctx.Err() != nil {
			break
		}
		links, err := e.links.ListLinks(ctx, collectionID, docID, "")
		if err != nil {
			e.lg.Warn("图扩展读关联失败",
				loggateway.StepID("knowledge.graph_expand.links_fail"),
				loggateway.Err(err),
				loggateway.Str("doc_id", docID))
			continue
		}
		for _, l := range links {
			other := l.TargetDocID
			if other == docID {
				other = l.DocID
			}
			if other == "" || other == docID {
				continue
			}
			if _, isSeed := seedSet[other]; isSeed {
				continue
			}
			w := l.Weight
			if w < 1 {
				w = 1
			}
			ranks[other] += linkTypePriority(l.LinkType) * w
		}
	}
	if len(ranks) == 0 {
		return nil
	}
	ordered := make([]neighborRank, 0, len(ranks))
	for id, score := range ranks {
		ordered = append(ordered, neighborRank{docID: id, score: score})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].score != ordered[j].score {
			return ordered[i].score > ordered[j].score
		}
		return ordered[i].docID < ordered[j].docID
	})
	limit := graphExpandMaxNeighbors
	if len(ordered) < limit {
		limit = len(ordered)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, ordered[i].docID)
	}
	return out
}

func uniqueSeedDocs(seeds []biz.KnowledgeChunk, max int) []string {
	seen := make(map[string]struct{}, max)
	out := make([]string, 0, max)
	for _, ch := range seeds {
		id := strings.TrimSpace(ch.DocID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if len(out) >= max {
			break
		}
	}
	return out
}

// linkTypePriority 邻居边类型权重（旧 1 跳路径排序分）。
// entity/semantic 边由 M2.1 SetEntityHook 生产装配触发（vault_sync 索引成功后
// 异步执行），当前已接线；若未来语义边管线补齐，semantic 分支自然生效。
func linkTypePriority(linkType string) int {
	switch linkType {
	case bizknowledge.LinkTypeExplicit:
		return 3
	case bizknowledge.LinkTypeEntity:
		return 2
	case bizknowledge.LinkTypeSemantic:
		return 1
	default:
		return 1
	}
}

// ── M1-4 受限扩散激活（2 跳 BFS）───────────────────────────────────────────

// spreadEdgeTypeWeight 扩散激活边类型权重：co_activated 为 Hebbian 弱信号，刻意压低。
func spreadEdgeTypeWeight(linkType string) float64 {
	switch linkType {
	case bizknowledge.LinkTypeExplicit:
		return 1.0
	case bizknowledge.LinkTypeSemantic:
		return 0.9
	case bizknowledge.LinkTypeEntity:
		return 0.7
	case bizknowledge.LinkTypeCoActivated:
		return 0.4
	default:
		return 0.5
	}
}

// spreadExpand 沿 active 边做 2 跳扩散激活：种子能量 1.0，每跳 ×0.5；
// 节点激活值 = Σ 入边 (跳能量 × 类型权重 × weight_f)；侧抑制只保留 top-N；
// 激活值映射为扩展 chunk 分数并入最终排序。零 LLM；失败降级为种子。
func (e *GraphExpander) spreadExpand(ctx context.Context, q biz.KnowledgeSearchQuery, seeds []biz.KnowledgeChunk, seedDocs []string, topK int) []biz.KnowledgeChunk {
	visited := make(map[string]struct{}, len(seedDocs))
	for _, id := range seedDocs {
		visited[id] = struct{}{}
	}
	activation := map[string]float64{}
	frontier := seedDocs
	energy := 1.0
	for hop := 0; hop < graphSpreadHops && len(frontier) > 0; hop++ {
		energy *= graphSpreadHopDecay
		links, err := e.active.ListActiveLinks(ctx, q.CollectionID, frontier)
		if err != nil {
			e.lg.Warn("扩散激活读 active 边失败，截断扩散",
				loggateway.StepID("knowledge.graph_spread.links_fail"),
				loggateway.Err(err),
				loggateway.Str("collection_id", q.CollectionID),
				loggateway.Int("hop", hop))
			break
		}
		inFrontier := make(map[string]struct{}, len(frontier))
		for _, id := range frontier {
			inFrontier[id] = struct{}{}
		}
		var next []string
		for _, l := range links {
			// 边的两个端点：frontier 侧为源，对侧获激活；两侧都在 frontier = 内部边跳过。
			_, srcIn := inFrontier[l.DocID]
			_, tgtIn := inFrontier[l.TargetDocID]
			var other string
			switch {
			case srcIn && !tgtIn:
				other = l.TargetDocID
			case tgtIn && !srcIn:
				other = l.DocID
			default:
				continue
			}
			if _, seen := visited[other]; seen {
				continue
			}
			w := l.WeightF
			if w <= 0 {
				w = 1.0
			}
			gain := energy * spreadEdgeTypeWeight(l.LinkType) * w
			if _, ok := activation[other]; !ok {
				next = append(next, other)
			}
			activation[other] += gain
		}
		for _, id := range next {
			visited[id] = struct{}{}
		}
		frontier = next
	}
	if len(activation) == 0 {
		return trimChunks(seeds, topK)
	}

	// 侧抑制：激活值 top-N（并列按 docID 定序）。
	ordered := make([]string, 0, len(activation))
	for id := range activation {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if activation[ordered[i]] != activation[ordered[j]] {
			return activation[ordered[i]] > activation[ordered[j]]
		}
		return ordered[i] < ordered[j]
	})
	if len(ordered) > graphExpandActivationCap {
		ordered = ordered[:graphExpandActivationCap]
	}

	neighborChunks, err := e.chunks.ListChunksByDocuments(ctx, q.CollectionID, ordered, graphExpandChunksPerDoc)
	if err != nil {
		e.lg.Warn("扩散激活取邻居 chunks 失败，使用种子结果",
			loggateway.StepID("knowledge.graph_spread.chunks_fail"),
			loggateway.Err(err),
			loggateway.Str("collection_id", q.CollectionID))
		return trimChunks(seeds, topK)
	}
	if len(neighborChunks) == 0 {
		return trimChunks(seeds, topK)
	}

	seedIDs := make(map[string]struct{}, len(seeds))
	maxSeed := float32(0)
	for _, ch := range seeds {
		seedIDs[ch.ID] = struct{}{}
		if ch.Score > maxSeed {
			maxSeed = ch.Score
		}
	}
	if maxSeed <= 0 {
		maxSeed = 1
	}
	scored := make([]biz.KnowledgeChunk, 0, len(neighborChunks))
	for _, ch := range neighborChunks {
		if _, dup := seedIDs[ch.ID]; dup {
			continue
		}
		ch.Score = maxSeed * graphExpandNeighborDecay * float32(activation[ch.DocID])
		scored = append(scored, ch)
	}
	if len(scored) == 0 {
		return trimChunks(seeds, topK)
	}
	return MergeSearchResults(seeds, scored, topK)
}
