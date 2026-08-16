package knowledge

import (
	"context"
	"time"
)

// 自治理知识图谱 M2 语义关系层端口。
// 谓词归一化走核心闭集（CoreRelations）；词表外谓词经 RelationVocabRepo 落
// candidate 层，由治理周期（M4）归并提升——受控涌现，不退化为 folksonomy。

// 核心谓词闭集（设计 §3.4 硬编码闭环；迁移 20261221 同步播种 tier=core）。
// co_activated 是统计边 link_type（无谓词语义），不在此集合。
var CoreRelations = []string{
	"is-a", "part-of", "depends-on", "causes",
	"applies-to", "contradicts", "supersedes", "evolves-from",
}

// SemanticLink 一条语义类型边（link_type=semantic + relation 谓词 + confidence）。
type SemanticLink struct {
	TargetDocID string
	Relation    string  // 归一化后的谓词（core 或 candidate）
	Confidence  float64 // LLM 置信度；Closed 边仅留痕不进主图谱
	Context     string  // 证据（主语实体 / 来源句片段）
	Closed      bool    // true = 写入即关闭（valid_to=now，低置信留痕）
}

// SemanticLinkRepo 语义边持久化窄接口（M2）。
// 派生索引纪律：某文档的 semantic 出链整体重建（删旧插新），可全量重放。
type SemanticLinkRepo interface {
	ReplaceSemanticLinks(ctx context.Context, collectionID, docID string, links []SemanticLink) error
}

// RelationVocabRepo 谓词词表窄接口（M2 受控涌现）。
type RelationVocabRepo interface {
	// UpsertCandidate 词表外谓词落 candidate 层（已存在 candidate 时 use_count+1；
	// core/promoted 不降级不计数——core 谓词由 CoreRelations 短路，本方法只为词表外服务）。
	UpsertCandidate(ctx context.Context, relation, proposedBy string) error
}

// HotDocumentLister 高频检索文档窄接口（M2 成本闸门：只对高价值词条做关系抽取）。
type HotDocumentLister interface {
	// ListHotDocuments 返回 sinceDays 窗口内命中次数 >= minHits 的文档 ID
	// （按命中数降序，limit 截断）。数据源 knowledge_access_log。
	ListHotDocuments(ctx context.Context, collectionID string, sinceDays, minHits, limit int) ([]string, error)
}

// RelationState 关系/实体抽取幂等状态（按 content_hash 判重）。
type RelationState struct {
	DocID                string
	CollectionID         string
	ContentHash          string
	EntitiesExtractedAt  time.Time // 零值 = 从未抽实体
	RelationsExtractedAt time.Time // 零值 = 从未抽关系
}

// RelationStateRepo 抽取幂等状态窄接口（M2）。派生状态，可清空重建（代价=重抽）。
type RelationStateRepo interface {
	// GetRelationState 取文档抽取状态；未抽取过返回零值（found=false），不报错。
	GetRelationState(ctx context.Context, docID string) (st RelationState, found bool, err error)
	// UpsertRelationState 登记/刷新抽取状态（按 doc_id 冲突更新）。
	UpsertRelationState(ctx context.Context, st RelationState) error
}

// EntityIDResolver 只读实体解析（M2：三元组宾语 → 既有实体 ID）。
// 与 ReplaceDocEntities 的写式解析不同：不新建字典条目，未知名返回缺席。
type EntityIDResolver interface {
	// ResolveEntityIDs 批量解析实体名 → 实体 ID（归一化 name_norm → 别名命中 keeper）。
	// 返回 map 键为入参原名（保留首见写法）；未知名不出现。
	ResolveEntityIDs(ctx context.Context, collectionID string, names []string) (map[string]int64, error)
}
