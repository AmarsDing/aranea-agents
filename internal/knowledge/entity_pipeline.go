// Package knowledge — 自治理知识图谱 M2.1：实体共现轨（entity track）管线。
//
// 管线（每文档，幂等）：
//
//	词条 body
//	 → [LLM 实体抽取]（与 M2.2 Step1 同源 llmExtractEntities）
//	 → ReplaceDocEntities（name_norm 归一化字典 + 别名路由）
//	 → FindEntityCooccurrences（R-3 频次过滤：超 maxDocFreq 的实体视为噪声）
//	 → ReplaceEntityLinks（link_type=entity，context=共享实体名，weight=共享数）
//
// 成本闸门：content_hash 幂等（正文未变不重抽），状态落
// knowledge_relation_state.entities_extracted_at（与关系轨 relations_extracted_at
// 同表分列，upsert 零值时间不动既有列）。LLM 缺失/调用失败上抛 error，state 不推进。
package knowledge

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// entityMaxDocFreq R-3 频次过滤：出现在超过该数文档的实体视为噪声（不参与共现建链）。
const entityMaxDocFreq = 50

// EntityTrackWriter entity 轨持久化窄接口（生产实现：*bizknowledge.Usecase，
// 未接线 EntityRepo/LinkRepo 时对应方法 no-op 降级）。
type EntityTrackWriter interface {
	ReplaceDocEntities(ctx context.Context, collectionID, docID string, entities []bizknowledge.DocEntity) ([]int64, error)
	FindEntityCooccurrences(ctx context.Context, collectionID string, entityIDs []int64, excludeDocID string, maxDocFreq int) ([]bizknowledge.EntityCooccurrence, error)
	ReplaceEntityLinks(ctx context.Context, collectionID, docID string, links []bizknowledge.Link) error
}

// EntityPipelineStats 单文档实体轨统计（日志/对账用）。
type EntityPipelineStats struct {
	Entities      int    // 抽取实体数（截断后）
	Cooccurrences int    // 共现文档对数
	Links         int    // 落库 entity 出链数
	Skipped       bool   // true = 未执行抽取（原因见 SkipReason）
	SkipReason    string // unchanged / empty_body
}

// EntityPipeline 实体共现轨抽取器。全部外部依赖经窄接口注入。
type EntityPipeline struct {
	llm     biz.LLMCaller
	sys     RefineLLMSettingsGetter
	catalog LLMCatalogLister
	docs    RelationDocReader
	track   EntityTrackWriter
	state   bizknowledge.RelationStateRepo
	lg      loggateway.Logger
}

// NewEntityPipeline 构造实体轨管线；lg 为 nil 时降级 Noop。
func NewEntityPipeline(
	llm biz.LLMCaller,
	sys RefineLLMSettingsGetter,
	catalog LLMCatalogLister,
	docs RelationDocReader,
	track EntityTrackWriter,
	state bizknowledge.RelationStateRepo,
	lg loggateway.Logger,
) *EntityPipeline {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &EntityPipeline{
		llm: llm, sys: sys, catalog: catalog,
		docs: docs, track: track, state: state, lg: lg,
	}
}

// EntityPipelineDisabled 环境开关（与其他 worker 同纪律）：置 1 时生产装配跳过接线。
func EntityPipelineDisabled() bool {
	return strings.TrimSpace(os.Getenv("KNOWLEDGE_ENTITY_PIPELINE_DISABLED")) == "1"
}

// ProcessDoc 对单文档执行实体共现抽取（幂等：content_hash 一致且已抽过则跳过）。
// 派生索引纪律：实体记录与 entity 出链随文档内容整体重建（Replace* 删旧插新）。
func (p *EntityPipeline) ProcessDoc(ctx context.Context, collectionID, docID string) (EntityPipelineStats, error) {
	var stats EntityPipelineStats
	if p == nil || p.llm == nil || p.docs == nil || p.track == nil || p.state == nil {
		return stats, fmt.Errorf("entity pipeline not fully wired (llm/docs/track/state required)")
	}
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return stats, fmt.Errorf("doc id required")
	}

	doc, err := p.docs.GetDocument(ctx, docID)
	if err != nil {
		return stats, fmt.Errorf("get document %s: %w", docID, err)
	}
	if collectionID == "" {
		collectionID = doc.CollectionID
	}
	body := strings.TrimSpace(doc.ContentText)
	if body == "" {
		stats.Skipped, stats.SkipReason = true, "empty_body"
		return stats, nil
	}
	contentHash := strings.TrimSpace(doc.ContentHash)
	if contentHash == "" {
		sum := sha1.Sum([]byte(body))
		contentHash = hex.EncodeToString(sum[:])
	}

	// 幂等闸门：正文未变且已抽过实体 → 跳过（控 LLM 成本）。
	st, found, err := p.state.GetRelationState(ctx, docID)
	if err != nil {
		return stats, fmt.Errorf("get relation state %s: %w", docID, err)
	}
	if found && st.ContentHash == contentHash && !st.EntitiesExtractedAt.IsZero() {
		stats.Skipped, stats.SkipReason = true, "unchanged"
		return stats, nil
	}

	provider, model, err := ResolveLLM(ctx, p.sys, p.catalog, "entity extract", p.lg)
	if err != nil {
		return stats, err
	}
	body = truncateRunes(body, relationBodyMaxRunes)
	title := docDisplayName(doc.RelPath, doc.Source)

	entities, err := llmExtractEntities(ctx, p.llm, provider, model, title, body)
	if err != nil {
		return stats, fmt.Errorf("extract entities %s: %w", docID, err)
	}
	stats.Entities = len(entities)

	// 写实体字典（归一化/别名路由），拿解析后实体 ID。
	docEntities := make([]bizknowledge.DocEntity, 0, len(entities))
	for _, en := range entities {
		docEntities = append(docEntities, bizknowledge.DocEntity{
			Name:       en.Name,
			EntityType: en.Type,
			Mentions:   countEntityMentions(body, en.Name),
		})
	}
	entityIDs, err := p.track.ReplaceDocEntities(ctx, collectionID, docID, docEntities)
	if err != nil {
		return stats, fmt.Errorf("replace doc entities %s: %w", docID, err)
	}

	// 共现建链：共享实体的其他文档 → entity 出链（context=共享实体名）。
	coocs, err := p.track.FindEntityCooccurrences(ctx, collectionID, entityIDs, docID, entityMaxDocFreq)
	if err != nil {
		return stats, fmt.Errorf("find entity cooccurrences %s: %w", docID, err)
	}
	stats.Cooccurrences = len(coocs)
	links := make([]bizknowledge.Link, 0, len(coocs))
	for _, c := range coocs {
		if c.DocID == "" || c.DocID == docID || len(c.SharedEntities) == 0 {
			continue
		}
		links = append(links, bizknowledge.Link{
			TargetDocID: c.DocID,
			LinkType:    bizknowledge.LinkTypeEntity,
			Context:     truncateRunes(strings.Join(c.SharedEntities, ", "), relationContextMaxRunes),
			Weight:      len(c.SharedEntities),
		})
	}
	stats.Links = len(links)
	if err := p.track.ReplaceEntityLinks(ctx, collectionID, docID, links); err != nil {
		return stats, fmt.Errorf("replace entity links %s: %w", docID, err)
	}

	if err := p.state.UpsertRelationState(ctx, bizknowledge.RelationState{
		DocID:               docID,
		CollectionID:        collectionID,
		ContentHash:         contentHash,
		EntitiesExtractedAt: time.Now(),
	}); err != nil {
		return stats, fmt.Errorf("upsert relation state %s: %w", docID, err)
	}
	return stats, nil
}

// countEntityMentions 实体在正文中的提及次数（大小写不敏感；下限 1——能被抽到即至少一次）。
func countEntityMentions(body, name string) int {
	n := strings.Count(strings.ToLower(body), strings.ToLower(strings.TrimSpace(name)))
	if n < 1 {
		return 1
	}
	return n
}
