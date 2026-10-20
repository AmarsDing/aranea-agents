package graphrag

import "context"

// EntitySearcher 查询→实体锚点（低层入口；Stability:evolving）。
// maxN 为返回实体数上限；实现侧按相关度降序。
type EntitySearcher interface {
	FindEntities(ctx context.Context, query string, maxN int) ([]EntityRef, error)
}

// EntityDocLinker 实体→归属文档（knowledge_doc_entities 提及关系）。
// 返回 entityID → docIDs；未命中的实体不出现在结果 key 中。
type EntityDocLinker interface {
	DocsOfEntities(ctx context.Context, entityIDs []string) (map[string][]string, error)
}

// LinkExpander 文档图邻域读取：返回 docIDs hops 跳内触及的全部边。
// 生产实现：bizknowledge.ActiveLinkReader / knowledge_links BFS
// （explicit + entity + semantic 轨，双向读取、active 过滤）。
type LinkExpander interface {
	Neighbors(ctx context.Context, docIDs []string, hops int) ([]LinkEdge, error)
}

// ChunkRanker 候选文档内的 chunk 排序（低层输出）。
// 返回按相关度降序、至多 topK 条；docIDs 为空应返回空结果而不是报错。
type ChunkRanker interface {
	Rank(ctx context.Context, query string, docIDs []string, topK int) ([]ChunkHit, error)
}

// Summarizer 文本集合摘要（高层主题产出；可为 nil = 跳过摘要）。
// 本库传入主题社区命中 chunk 的 ChunkID 列表（按融合降序），
// 正文解析与摘要生成由适配层负责。
type Summarizer interface {
	Summarize(ctx context.Context, texts []string) (string, error)
}

// LowLevel 低层检索组合端口：实体锚定 + 实体→文档 + chunk 排序。
// 生产由 internal/knowledge 检索栈装配（实体检索 + 提及表 + 混合检索）。
type LowLevel interface {
	EntitySearcher
	EntityDocLinker
	ChunkRanker
}
