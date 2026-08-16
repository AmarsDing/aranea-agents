package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

const (
	// KnowledgeRelationExtractDefaultInterval 默认扫描周期（30m）。
	// 关系抽取是 LLM 高成本路径：幂等闸门（content_hash）保证稳态零 LLM 调用，
	// 周期只决定「变更高价值词条 → typed edges」的感知延迟。
	KnowledgeRelationExtractDefaultInterval = 30 * time.Minute
	// knowledgeRelationExtractSinceDays 热文档统计窗口（access_log 命中天数）。
	knowledgeRelationExtractSinceDays = 30
	// knowledgeRelationExtractMinHits 热文档命中门槛（窗口内 >= N 次检索命中才抽取）。
	knowledgeRelationExtractMinHits = 3
	// knowledgeRelationExtractPerCollection 单集合单轮热文档上限。
	knowledgeRelationExtractPerCollection = 10
	// knowledgeRelationExtractMaxPerPass 单轮全局 LLM 抽取预算（防成本风暴）。
	knowledgeRelationExtractMaxPerPass = 20
	// knowledgeRelationExtractMaxCollections 单轮集合枚举上限。
	knowledgeRelationExtractMaxCollections = 1000
)

// KnowledgeRelationCollectionLister 集合枚举窄接口（生产实现：bizknowledge.Usecase）。
type KnowledgeRelationCollectionLister interface {
	ListCollections(ctx context.Context, workspace string, limit, offset int) ([]bizknowledge.Collection, int, error)
}

// KnowledgeRelationDocExtractor 单文档关系抽取窄接口
// （生产实现：knowledge.RelationExtractor；幂等由 extractor 内部 state 闸门保证）。
type KnowledgeRelationDocExtractor interface {
	ExtractDoc(ctx context.Context, docID string) (knowledge.RelationExtractStats, error)
}

// KnowledgeRelationExtractWorker 自治理图谱 M2 语义关系层后台工人：
// 周期扫描各集合的高价值词条（knowledge_access_log 命中 >= 阈值），
// 驱动两步 LLM 关系抽取写 semantic typed edges。
//
// 成本控制三道闸：
//  1. 热度闸：只对窗口内高频检索文档抽取（ListHotDocuments）；
//  2. 幂等闸：content_hash 一致即跳过（extractor 内），稳态零 LLM 调用；
//  3. 预算闸：单轮全局上限 maxPerPass，超出顺延下轮。
//
// 所有失败 best-effort：单文档失败记录 Warn 继续，整轮失败下轮重试。
type KnowledgeRelationExtractWorker struct {
	interval    time.Duration
	collections KnowledgeRelationCollectionLister
	hot         bizknowledge.HotDocumentLister
	extractor   KnowledgeRelationDocExtractor
	lg          loggateway.Logger
}

// NewKnowledgeRelationExtractWorker 构造工人；interval <= 0 用默认 30m。
// 任一依赖为 nil 返回 nil（未接线降级，不启动空转工人）。
func NewKnowledgeRelationExtractWorker(
	interval time.Duration,
	collections KnowledgeRelationCollectionLister,
	hot bizknowledge.HotDocumentLister,
	extractor KnowledgeRelationDocExtractor,
	lg loggateway.Logger,
) *KnowledgeRelationExtractWorker {
	if collections == nil || hot == nil || extractor == nil {
		return nil
	}
	if interval <= 0 {
		interval = KnowledgeRelationExtractDefaultInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &KnowledgeRelationExtractWorker{
		interval:    interval,
		collections: collections,
		hot:         hot,
		extractor:   extractor,
		lg:          lg.With(loggateway.Domain("knowledge_relation_extract")),
	}
}

// Start 阻塞至 ctx 取消，按周期触发扫描。
func (w *KnowledgeRelationExtractWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	w.lg.Info("knowledge relation extract worker started",
		loggateway.Str("interval", w.interval.String()))
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce 同步执行一轮扫描。panic 本地recover，不拖垮工人循环。
func (w *KnowledgeRelationExtractWorker) RunOnce(ctx context.Context) {
	if w == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			w.lg.Error("knowledge relation extract panic recovered",
				loggateway.StepID("knowledge.relation_extract"),
				loggateway.Any("panic", r))
		}
	}()
	cols, _, err := w.collections.ListCollections(ctx, "", knowledgeRelationExtractMaxCollections, 0)
	if err != nil {
		w.lg.Warn("relation extract: list collections failed",
			loggateway.StepID("knowledge.relation_extract"),
			loggateway.Err(err))
		return
	}
	budget := knowledgeRelationExtractMaxPerPass
	var extracted, skipped, links, open, candidates, failed int
	for _, col := range cols {
		if budget <= 0 {
			break
		}
		hotIDs, err := w.hot.ListHotDocuments(ctx, col.ID,
			knowledgeRelationExtractSinceDays, knowledgeRelationExtractMinHits, knowledgeRelationExtractPerCollection)
		if err != nil {
			w.lg.Warn("relation extract: list hot documents failed",
				loggateway.StepID("knowledge.relation_extract"),
				loggateway.Str("collection", col.ID),
				loggateway.Err(err))
			continue
		}
		for _, docID := range hotIDs {
			if budget <= 0 {
				break
			}
			stats, err := w.extractor.ExtractDoc(ctx, docID)
			if err != nil {
				failed++
				w.lg.Warn("relation extract: doc failed",
					loggateway.StepID("knowledge.relation_extract"),
					loggateway.Str("collection", col.ID),
					loggateway.Str("doc", docID),
					loggateway.Err(err))
				continue
			}
			if stats.Skipped {
				skipped++ // 幂等/降级跳过零 LLM 成本，不占预算
				continue
			}
			budget--
			extracted++
			links += stats.Links
			open += stats.OpenLinks
			candidates += stats.Candidates
		}
	}
	if extracted > 0 || failed > 0 {
		w.lg.Info("knowledge relation extract pass completed",
			loggateway.StepID("knowledge.relation_extract"),
			loggateway.Int("collections", len(cols)),
			loggateway.Int("extracted", extracted),
			loggateway.Int("skipped", skipped),
			loggateway.Int("failed", failed),
			loggateway.Int("links", links),
			loggateway.Int("open_links", open),
			loggateway.Int("vocab_candidates", candidates))
	}
}

// KnowledgeRelationExtractDisabled 报告工人是否经
// KNOWLEDGE_RELATION_EXTRACT_DISABLED 环境变量禁用。
func KnowledgeRelationExtractDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("KNOWLEDGE_RELATION_EXTRACT_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
