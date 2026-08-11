package knowledge

import "context"

// KnowledgeBlock 块行（biz 模型）。与 blockparse.BlockRow 对齐，叠加存储字段。
// ID 规则（设计 S2）：显式锚定后 = 文本中 ^anchor；未锚块由存储层生成。
type KnowledgeBlock struct {
	ID           string
	CollectionID string
	DocID        string
	Ordinal      int
	Kind         string // heading/paragraph/list_item/code_block/blockquote/table/math
	Anchor       string // 显式 ^anchor（未锚为空）
	HeadingPath  []string
	ContentHash  string
	TextExcerpt  string
	PromotedFrom string // 晋升谱系（US-27），无外键（源可删除）
	PromotedTo   string
}

// KnowledgeBlockRefInput 整文档重建时的引用输入。
// SrcOrdinal 由存储层映射到本次插入的块 ID；Dst* 为 Resolver 产物（空 = dangling）。
type KnowledgeBlockRefInput struct {
	SrcOrdinal int
	RawTarget  string // 原始目标文本（dangling 复活唯一线索）
	Alias      string
	EdgeType   string // ref / embed
	Context    string
	DstDocID   string
	DstBlockID string
	// DstCollectionID 目标文档所属集合（Resolver 产物；dangling 时为空）。
	// 派生冗余：DB 侧可经 dst_doc_id JOIN 推导，随输入传递供内存图（SP1-D LinkIndex）
	// 免回表装配边 scope。
	DstCollectionID string
	// DstSelfOrdinal 自文档引用的目标块 ordinal（Resolver 产物，目标块尚未落库无 ID）；
	// 非 nil 时由存储层按本次插入的 ordinal→ID 映射回填 DstBlockID。
	DstSelfOrdinal *int
	Ambiguous      bool // 跨库多义时 Resolver 记 true（B-1 评审修订）
}

// KnowledgeBlockRefEdge 物化后的引用边（存储层产物，SP1-D LinkIndex apply 单元）。
// 与 KnowledgeBlockRefInput 的区别：Src/Dst 均为落库后的真实块/文档 ID。
// 边身份 = 内容元组（SrcBlockID, DstBlockID, DstDocID, EdgeType, RawTarget），
// 不含 BIGSERIAL 行 id（整文档重建后行 id 变，内容身份不变）。
type KnowledgeBlockRefEdge struct {
	CollectionID    string // 源集合（= refs.collection_id）
	SrcBlockID      string
	SrcDocID        string
	DstCollectionID string // 空 = dangling
	DstDocID        string // 空 = dangling
	DstBlockID      string // 空 = 文档级边或 dangling
	RawTarget       string
	EdgeType        string // ref / embed
	Context         string
	Ambiguous       bool
}

// LinkEdgeLoader 启动全量加载端口（SP1-D）：从 knowledge_block_refs 重放全部边
// 构建内存图（LinkIndex）。DstCollectionID 由实现方 JOIN 推导。
// Stability:evolving
type LinkEdgeLoader interface {
	ListAllRefEdges(ctx context.Context) ([]KnowledgeBlockRefEdge, error)
}

// BlockIndexRepo 块级派生索引物化端口。
// 写模式（SiYuan deleteRefsByPathTx 语义）：整文档删了重插，refs 不做 diff。
// Stability:evolving
type BlockIndexRepo interface {
	// ReplaceDocBlocks 在单事务内：删 doc 全部块（级联清 src 边；指向旧块的入向边
	// 由 FK ON DELETE SET NULL 转 dangling 保 raw_target）→ 插新块 → 插新边。
	// 返回本次物化的引用边（落库后真实块/文档 ID，SP1-D LinkIndex apply 单元）。
	ReplaceDocBlocks(ctx context.Context, collectionID, docID string, blocks []KnowledgeBlock, refs []KnowledgeBlockRefInput) ([]KnowledgeBlockRefEdge, error)
	// ListDocBlocks 按 ordinal 序列出文档全部块（校验/调试/反链组装用）。
	ListDocBlocks(ctx context.Context, docID string) ([]KnowledgeBlock, error)
	// UpdateDocLinkKeys 物化文档解析键（frontmatter title/aliases，Resolver 文档键）。
	// 与 ReplaceDocBlocks 同事务非必需：失败仅解析键滞后，下次重建自愈。
	UpdateDocLinkKeys(ctx context.Context, docID, title string, aliases []string) error
	// ListDocsMissingBlockIndex 列出「已索引但块索引缺失」的文档 ID（SP2 #4 下游
	// 收敛）：rebuildBlockIndex 失败仅 Warn 降级而 content_hash 已落库，下轮不再
	// 重试——靠此低频校验检出漂移并自动重建。空内容文档（合法 0 块）须排除。
	ListDocsMissingBlockIndex(ctx context.Context, collectionID string, limit int) ([]string, error)
}
