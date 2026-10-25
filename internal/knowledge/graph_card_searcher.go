package knowledge

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// G-33（ADR-KN-20261020-I）：图谱卡片第三腿——卡片命中投影为合成 chunk 候选，
// 经 rrfMergeN 与 dense/sparse 同权融合。候选持有真实 DocID/CollectionID
// （父回填/base-boost/access_log/Hebbian/GraphExpander 种子自然继承零回归），
// 卡语义由合成 ID 前缀 + metadata_json 的 g33 标记承载（零 proto side-channel）。

const (
	// graphCardEntityIDPrefix 合成候选 ID 前缀（r=relation edge，避让摄取
	// 命名空间 <doc>-ch-<i>/<doc>-p-<i>）。
	graphCardEntityIDPrefix = "g33e/"
	graphCardEdgeIDPrefix   = "g33r/"
)

// GraphCardSearcher 卡片腿窄端口（照 SparseSearcher 先例：独立接口、装配层
// 注入、nil=关闭）。vec 非空走 ANN（kind 过滤各 ≤GraphCardSearchKindLimit）；
// vec 为空走词法降级（实体卡 name_norm 精确/前缀 ∪ alias_norm 精确，edge 腿
// 静默关闭）——v1 装配只从 searchRRF 喂 vec，词法分支为降级路径备役。
type GraphCardSearcher interface {
	SearchGraphCardChunks(ctx context.Context, q biz.KnowledgeSearchQuery, vec []float32) ([]biz.KnowledgeChunk, error)
}

type knowledgeGraphCardSearcher struct {
	dense   bizknowledge.GraphCardVectorSearcher
	lexical bizknowledge.GraphCardLexicalSearcher
	hosts   bizknowledge.GraphCardHostDocLister
	lg      loggateway.Logger
}

// NewGraphCardSearcher 三窄 repo 接口均由 data 知识仓储实现（可见性谓词同一
// 中央扩点）；任一缺失视为未接线（装配层断言后 nil=关闭本腿）。
func NewGraphCardSearcher(dense bizknowledge.GraphCardVectorSearcher, lexical bizknowledge.GraphCardLexicalSearcher, hosts bizknowledge.GraphCardHostDocLister, lg loggateway.Logger) GraphCardSearcher {
	if dense == nil || lexical == nil || hosts == nil {
		return nil
	}
	return &knowledgeGraphCardSearcher{dense: dense, lexical: lexical, hosts: hosts, lg: lg}
}

func (s *knowledgeGraphCardSearcher) SearchGraphCardChunks(ctx context.Context, q biz.KnowledgeSearchQuery, vec []float32) ([]biz.KnowledgeChunk, error) {
	// metadata 过滤属于真实 chunk，不属于整篇宿主。卡片尚无精确来源块，
	// 不能以「宿主任一 chunk 符合」冒充卡文来源符合；保持其他召回腿可用。
	if strings.TrimSpace(q.FilterJSON) != "" {
		return nil, nil
	}
	var hits []bizknowledge.GraphCardHit
	if len(vec) > 0 {
		entityHits, err := s.dense.SearchGraphCards(ctx, q.CollectionID, bizknowledge.GraphCardKindEntity, vec, bizknowledge.GraphCardSearchKindLimit)
		if err != nil {
			return nil, err
		}
		hits = append(hits, entityHits...)
		// 关系卡只有当前投影；时态查询在卡片具备时态端口前关闭此腿。
		// 实体卡仍遵守「当前节点集」的时态契约。
		if !q.Temporal.Any() {
			edgeHits, err := s.dense.SearchGraphCards(ctx, q.CollectionID, bizknowledge.GraphCardKindEdge, vec, bizknowledge.GraphCardSearchKindLimit)
			if err != nil {
				return nil, err
			}
			hits = append(hits, edgeHits...)
		}
	} else {
		// 词法降级：仅实体卡（edge 陈述无结构词法匹配是噪声源，静默关闭）。
		entityHits, err := s.lexical.SearchEntityCardsLexical(ctx, q.CollectionID, q.Query, bizknowledge.GraphCardSearchKindLimit)
		if err != nil {
			return nil, err
		}
		hits = entityHits
	}

	// 宿主投影：每卡出题 ≤3（entity）/≤2（edge）真实文档，硬上界 40 候选。
	out := make([]biz.KnowledgeChunk, 0, 2*len(hits))
	for _, hit := range hits {
		hostLimit := bizknowledge.GraphCardEntityHostLimit
		if hit.Kind == bizknowledge.GraphCardKindEdge {
			hostLimit = bizknowledge.GraphCardEdgeHostLimit
		}
		docs, err := s.hosts.ListGraphCardHostDocs(ctx, q.CollectionID, hit.Kind, hit.RefID, hostLimit)
		if err != nil {
			// 单卡宿主读取失败只丢该卡（可见性谓词同库查询理论同生死，warn 留痕）。
			s.lg.Warn("图谱卡片宿主投影失败（本卡跳过）",
				loggateway.StepID("knowledge.graph_card.host_docs"),
				loggateway.Str("collection_id", q.CollectionID),
				loggateway.Err(err))
			continue
		}
		if hasSearchScope(q) {
			docs, err = s.scopedHosts(ctx, q, hit.Kind, docs)
			if err != nil {
				return nil, err
			}
		}
		for _, docID := range docs {
			out = append(out, biz.KnowledgeChunk{
				ID:           graphCardChunkID(hit.Kind, hit.RefID, docID),
				DocID:        docID,
				CollectionID: q.CollectionID,
				Content:      hit.Text,
				Score:        hit.Score,
				EntityScore:  hit.Score, // tagEntityScores：卡片相似度
				MetadataJSON: graphCardMarker(hit.Kind, hit.RefID),
			})
		}
	}
	return out, nil
}

func (s *knowledgeGraphCardSearcher) scopedHosts(ctx context.Context, q biz.KnowledgeSearchQuery, kind string, docs []string) ([]string, error) {
	scoped, ok := s.hosts.(ScopedNeighborChunkLister)
	if !ok {
		// 未具备范围过滤能力时关闭卡片补召回，不能放宽用户的筛选范围。
		return nil, nil
	}
	chunks, err := scoped.ListChunksByDocumentsForSearch(ctx, q, docs, 1)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(chunks))
	for _, chunk := range chunks {
		allowed[chunk.DocID] = true
	}
	filtered := make([]string, 0, len(docs))
	for _, doc := range docs {
		if allowed[doc] {
			filtered = append(filtered, doc)
		}
	}
	// 边卡陈述涉及双端，任一端不在范围内即整卡排除。
	if kind == bizknowledge.GraphCardKindEdge && len(filtered) != len(docs) {
		return nil, nil
	}
	return filtered, nil
}

func graphCardChunkID(kind string, refID int64, docID string) string {
	prefix := graphCardEntityIDPrefix
	if kind == bizknowledge.GraphCardKindEdge {
		prefix = graphCardEdgeIDPrefix
	}
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString(strconv.FormatInt(refID, 10))
	b.WriteString("/")
	b.WriteString(docID)
	return b.String()
}

// graphCardMarker side-channel 标记（G-12 metadata_json 先例）：Search/chips/
// hit-test 可读；proto 分解字段留前端协调轮。
func graphCardMarker(kind string, refID int64) string {
	raw, err := json.Marshal(map[string]interface{}{"g33": kind, "g33_ref": refID})
	if err != nil {
		return ""
	}
	return string(raw)
}
