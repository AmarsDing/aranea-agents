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
)

// NeighborLinkReader 文档一跳邻域（双向 links）。生产实现：data.knowledgeRepo.ListLinks。
type NeighborLinkReader interface {
	ListLinks(ctx context.Context, collectionID, docID, linkType string) ([]bizknowledge.Link, error)
}

// NeighborChunkLister 按文档取前 N 个 chunk（chunk_index 升序）。生产实现：data.knowledgeRepo.ListChunksByDocuments。
type NeighborChunkLister interface {
	ListChunksByDocuments(ctx context.Context, collectionID string, docIDs []string, limitPerDoc int) ([]biz.KnowledgeChunk, error)
}

// GraphExpander 在向量/混合检索种子上叠加 1-hop 图扩展。
type GraphExpander struct {
	links  NeighborLinkReader
	chunks NeighborChunkLister
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

// linkTypePriority 邻居边类型权重。
// 实体轨冻结（2026-08-15 评审修订 P1）：entity/semantic 边生产无写入路径
// （SetEntityHook 全仓无生产调用者、semantic 无建边管线），knowledge_links 里
// 只有 explicit 一种边——entity×2/semantic×1 分支当前是死代码，保留仅为兼容
// 未来接线；解冻判据：citation 失败分布证明多跳失败够线（≥30%）再议，勿提前养。
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
