package knowledge

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// G-33 写侧——对账投影工人（ADR-KN-20261020-H 写侧决策）：
// cron 周期驱动 RunOnce，把「实体/active semantic 边」投影为卡片向量档。
// 三类对账（缺卡/漂移重嵌、孤儿与闭合边墓碑）全部按 (kind,ref_id) 幂等；
// 单集合/单批失败 warn 跳过下轮自愈，不阻断整轮。即时钩子刻意不做
// （EntityPipeline/ReplaceSemanticLinks 后即时投影属优化，v1 避免双写点纪律分裂）。

const (
	// GraphProjectionRoundBudget 全局每轮处理行数上限（缺卡+漂移+墓碑合计，
	// 跨集合累计；余量下轮承接）。
	GraphProjectionRoundBudget = 256
	// GraphProjectionEmbedBatch 嵌入批大小（沿用 defaultEmbedBatchSize 纪律）。
	GraphProjectionEmbedBatch = 32
)

// GraphProjectionCollection 待投影集合（data 侧已过滤 embedding_model<>”——
// 词法库无卡可嵌，工人空转）。
type GraphProjectionCollection struct {
	ID string
}

// GraphCardEntitySource 实体卡原料：实体行 + 归一化别名表。
type GraphCardEntitySource struct {
	EntityID   int64
	Name       string
	EntityType string
	Aliases    []string
}

// GraphCardEdgeSource 边卡原料：active semantic 边行 + 目标文档题面。
type GraphCardEdgeSource struct {
	LinkID        int64
	Relation      string
	Context       string // relationEdgeContext 原文：<subject> — <evidence>
	TargetTitle   string
	TargetRelPath string
}

// GraphCardSource 构造完毕的目标卡片（含漂移对账键）。
type GraphCardSource struct {
	Kind         string
	RefID        int64
	CollectionID string
	Text         string
	SourceHash   string
}

// GraphCardRow 卡片表现存行。
type GraphCardRow struct {
	Kind         string
	RefID        int64
	CollectionID string
	Text         string
	SourceHash   string
}

// GraphCardKey 卡片主键（kind 复用两宿主表，ref_id 无 FK——孤儿由本工人回收）。
type GraphCardKey struct {
	Kind  string
	RefID int64
}

// GraphCardUpsert 重嵌/新卡落库载荷（Embedding 来自 GraphCardEmbedder 批）。
type GraphCardUpsert struct {
	Kind         string
	RefID        int64
	CollectionID string
	Text         string
	SourceHash   string
	Embedding    []float32
}

// GraphCardStore 卡片表窄端口（ADR-H 双轨 DDL 的 data 实现挂载点）。
type GraphCardStore interface {
	// ListGraphCardCollections 返回 embedding_model<>'' 的集合（语义层闸）。
	ListGraphCardCollections(ctx context.Context) ([]GraphProjectionCollection, error)
	// ListGraphCardEntitySources 全部实体行（归一化别名随附）；实体本身由
	// EntityPipeline 热度闸产生，工人只投影不扩产（零新增 LLM 成本面）。
	ListGraphCardEntitySources(ctx context.Context, collectionID string) ([]GraphCardEntitySource, error)
	// ListGraphCardEdgeSources 仅 link_type='semantic' AND valid_to IS NULL（ADR-G 门槛）。
	ListGraphCardEdgeSources(ctx context.Context, collectionID string) ([]GraphCardEdgeSource, error)
	// ListGraphCards 该集合全部现存卡片行。
	ListGraphCards(ctx context.Context, collectionID string) ([]GraphCardRow, error)
	// UpsertGraphCards 按 (kind,ref_id) 幂等重嵌落库。
	UpsertGraphCards(ctx context.Context, cards []GraphCardUpsert) error
	// DeleteGraphCards 墓碑删除（实体归并硬删/边闭合的孤儿行）。
	DeleteGraphCards(ctx context.Context, keys []GraphCardKey) error
}

// GraphCardEmbedder 卡片嵌入窄端口（data 侧实现须走 recordUsage 且
// metadata.task_type='graph_card'——ADR-I 记账口径第三值）。
type GraphCardEmbedder interface {
	EmbedGraphCardTexts(ctx context.Context, texts []string) ([][]float32, error)
}

// BuildGraphCardSource 把原料行构造为目标卡片（ADR-G 卡片文 + sha1 对账键）；
// 弱化类型（entity/edge 二选）由调用侧按原料类型保证——一个超载入口换调用点
// 两处分支的清晰。
func BuildGraphCardSource(collectionID string, raw interface{}) GraphCardSource {
	switch src := raw.(type) {
	case GraphCardEntitySource:
		text := EntityCardText(src.Name, src.EntityType, src.Aliases)
		return GraphCardSource{
			Kind: GraphCardKindEntity, RefID: src.EntityID, CollectionID: collectionID,
			Text: text, SourceHash: GraphCardSourceHash(text),
		}
	case GraphCardEdgeSource:
		subject, evidence := ParseRelationEdgeContext(src.Context)
		object := strings.TrimSpace(src.TargetTitle)
		if object == "" {
			object = docBaseNameNoExt(src.TargetRelPath)
		}
		text := EdgeCardText(subject, src.Relation, object, evidence)
		return GraphCardSource{
			Kind: GraphCardKindEdge, RefID: src.LinkID, CollectionID: collectionID,
			Text: text, SourceHash: GraphCardSourceHash(text),
		}
	default:
		return GraphCardSource{}
	}
}

// ParseRelationEdgeContext 解析 relationEdgeContext 落库格式 `<subject> — <evidence>`
// 的首个分隔符：主语为抽取期快照名（刻意不随实体归并漂白——重写纪律只服 entity
// 轨），evidence 内 em-dash 原样保留；无分隔符时整段视为主语（与空证据锐写对称）。
func ParseRelationEdgeContext(context string) (subject, evidence string) {
	s := strings.TrimSpace(context)
	if i := strings.Index(s, " — "); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+len(" — "):])
	}
	return s, ""
}

// docBaseNameNoExt rel_path 取 basename 去扩展名（与 internal/knowledge
// docDisplayName 同名规则的本层自留副本——biz 不反向依赖检索层）。
func docBaseNameNoExt(relPath string) string {
	name := strings.TrimSpace(relPath)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[:i]
	}
	return name
}

// PlanGraphCardReconcile 纯函数对账：目标源 vs 现存卡 → 待重嵌 upsert 与墓碑
// delete。预算语义：墓碑优先（召回卫生——闭合边/归并实体先退出召回），余量
// 按目标源顺序承接 upsert；同名同 hash 的行不动。
func PlanGraphCardReconcile(sources []GraphCardSource, existing []GraphCardRow, budget int) (upserts []GraphCardSource, deletes []GraphCardKey) {
	existingByKey := make(map[GraphCardKey]GraphCardRow, len(existing))
	for _, row := range existing {
		existingByKey[GraphCardKey{Kind: row.Kind, RefID: row.RefID}] = row
	}
	sourceKeys := make(map[GraphCardKey]struct{}, len(sources))

	pending := make([]GraphCardSource, 0, len(sources))
	for _, src := range sources {
		key := GraphCardKey{Kind: src.Kind, RefID: src.RefID}
		sourceKeys[key] = struct{}{}
		row, ok := existingByKey[key]
		if ok && row.SourceHash == src.SourceHash {
			continue
		}
		pending = append(pending, src)
	}
	for _, row := range existing {
		key := GraphCardKey{Kind: row.Kind, RefID: row.RefID}
		if _, alive := sourceKeys[key]; !alive {
			deletes = append(deletes, key)
		}
	}

	if budget < 0 {
		budget = 0
	}
	if len(deletes) > budget {
		return nil, deletes[:budget]
	}
	rest := budget - len(deletes)
	if len(pending) > rest {
		upserts = pending[:rest]
	} else {
		upserts = pending
	}
	return upserts, deletes
}

// GraphProjectionStats 单轮对账统计（日志与观测口径）。
type GraphProjectionStats struct {
	Collections int // 成功承接的集合数
	Upserted    int // 重嵌落库行数
	Deleted     int // 墓碑行数
	Failed      int // 单条/单集合跳过合计（嵌入批失败、store 错误）
}

// GraphProjectionWorker 卡片投影工人（cron 5m 驱动）。
type GraphProjectionWorker struct {
	store    GraphCardStore
	embedder GraphCardEmbedder
	lg       loggateway.Logger
}

// NewGraphProjectionWorker 装配点；embedder 为 nil 属接线缺陷（RunOnce 直接报错——
// 与检索腿「nil=关闭」语义不同：工人没有关闭语义，只有装配正确性）。
func NewGraphProjectionWorker(store GraphCardStore, embedder GraphCardEmbedder, lg loggateway.Logger) *GraphProjectionWorker {
	return &GraphProjectionWorker{store: store, embedder: embedder, lg: lg}
}

// RunOnce 单轮：遍历语义层集合，预算内承接对账；集合/批次失败 warn 计数续跑。
func (w *GraphProjectionWorker) RunOnce(ctx context.Context) (GraphProjectionStats, error) {
	var stats GraphProjectionStats
	if w.store == nil || w.embedder == nil {
		return stats, errors.New("knowledge.graph_projection: store/embedder 未装配")
	}
	cols, err := w.store.ListGraphCardCollections(ctx)
	if err != nil {
		return stats, err
	}
	budget := GraphProjectionRoundBudget
	for _, col := range cols {
		if budget <= 0 {
			break
		}
		upserts, deletes, planned := w.plan(ctx, col.ID, budget, &stats)
		if !planned {
			continue // 集合级失败（plan 已计 Failed）
		}
		budget -= len(upserts) + len(deletes)
		// 墓碑批。
		if len(deletes) > 0 {
			if err := w.store.DeleteGraphCards(ctx, deletes); err != nil {
				w.lg.Warn("图谱卡片墓碑删除失败（下轮重试）",
					loggateway.StepID("knowledge.graph_projection.delete"),
					loggateway.Str("collection_id", col.ID),
					loggateway.Err(err))
				stats.Failed += len(deletes)
			} else {
				stats.Deleted += len(deletes)
			}
		}
		// 重嵌批（GraphProjectionEmbedBatch 一批一嵌入；批失败只丢该批）。
		for start := 0; start < len(upserts); start += GraphProjectionEmbedBatch {
			end := start + GraphProjectionEmbedBatch
			if end > len(upserts) {
				end = len(upserts)
			}
			batch := upserts[start:end]
			texts := make([]string, len(batch))
			for i, c := range batch {
				texts[i] = c.Text
			}
			vecs, err := w.embedder.EmbedGraphCardTexts(ctx, texts)
			if err != nil || len(vecs) != len(batch) {
				if err == nil {
					err = errors.New("嵌入返回行数与批次不符")
				}
				w.lg.Warn("图谱卡片嵌入批失败（本批跳过下轮重嵌）",
					loggateway.StepID("knowledge.graph_projection.embed"),
					loggateway.Str("collection_id", col.ID),
					loggateway.Err(err))
				stats.Failed += len(batch)
				continue
			}
			rows := make([]GraphCardUpsert, len(batch))
			for i, c := range batch {
				rows[i] = GraphCardUpsert{
					Kind: c.Kind, RefID: c.RefID, CollectionID: c.CollectionID,
					Text: c.Text, SourceHash: c.SourceHash, Embedding: vecs[i],
				}
			}
			if err := w.store.UpsertGraphCards(ctx, rows); err != nil {
				w.lg.Warn("图谱卡片落库失败（本批跳过下轮重试）",
					loggateway.StepID("knowledge.graph_projection.upsert"),
					loggateway.Str("collection_id", col.ID),
					loggateway.Err(err))
				stats.Failed += len(batch)
				continue
			}
			stats.Upserted += len(rows)
		}
		stats.Collections++
	}
	return stats, nil
}

func (w *GraphProjectionWorker) plan(ctx context.Context, collectionID string, budget int, stats *GraphProjectionStats) ([]GraphCardSource, []GraphCardKey, bool) {
	entities, err := w.store.ListGraphCardEntitySources(ctx, collectionID)
	if err != nil {
		w.lg.Warn("图谱卡片实体源读取失败（本集合跳过）",
			loggateway.StepID("knowledge.graph_projection.list_entities"),
			loggateway.Str("collection_id", collectionID),
			loggateway.Err(err))
		stats.Failed++
		return nil, nil, false
	}
	edges, err := w.store.ListGraphCardEdgeSources(ctx, collectionID)
	if err != nil {
		w.lg.Warn("图谱卡片边源读取失败（本集合跳过）",
			loggateway.StepID("knowledge.graph_projection.list_edges"),
			loggateway.Str("collection_id", collectionID),
			loggateway.Err(err))
		stats.Failed++
		return nil, nil, false
	}
	existing, err := w.store.ListGraphCards(ctx, collectionID)
	if err != nil {
		w.lg.Warn("图谱卡片现存行读取失败（本集合跳过）",
			loggateway.StepID("knowledge.graph_projection.list_cards"),
			loggateway.Str("collection_id", collectionID),
			loggateway.Err(err))
		stats.Failed++
		return nil, nil, false
	}
	sources := make([]GraphCardSource, 0, len(entities)+len(edges))
	for _, e := range entities {
		sources = append(sources, BuildGraphCardSource(collectionID, e))
	}
	for _, e := range edges {
		sources = append(sources, BuildGraphCardSource(collectionID, e))
	}
	upserts, deletes := PlanGraphCardReconcile(sources, existing, budget)
	return upserts, deletes, true
}
