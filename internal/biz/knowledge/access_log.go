package knowledge

import "context"

// AccessLogEntry 一次检索命中（M1 统计联想层数据源）。
// 每条命中 chunk 的文档记一行；QueryHash 标识同批召回（Hebbian 共激活分组键）。
type AccessLogEntry struct {
	CollectionID string
	DocID        string
	QueryHash    string
}

// AccessLogRepo 检索命中日志端口（自治理知识图谱 M1）：
//   - LogAccess：检索返回后记录命中（base-level 激活分与 Hebbian 共激活的原始数据）。
//   - BaseLevelScores：ACT-R base-level 激活分 ln(Σ age_days^-0.5)，按文档聚合。
//
// 实现：data 层 knowledgeRepo（knowledge_access_log 表）。
type AccessLogRepo interface {
	LogAccess(ctx context.Context, entries []AccessLogEntry) error
	BaseLevelScores(ctx context.Context, collectionID string, docIDs []string) (map[string]float64, error)
}

// CoActivationRepo Hebbian 共激活边端口（自治理知识图谱 M1-3）：
// 同批召回的文档两两写 co_activated 边（无向，规范化方向 doc_id<target_doc_id），
// weight_f 累加 eta；已衰减（valid_to 置位）的边被复活。端点文档不存在时跳过该对。
// 周期衰减由 dream_cycle 负责（M4），本端口只管强化。
type CoActivationRepo interface {
	StrengthenCoActivations(ctx context.Context, collectionID string, docIDs []string, eta float64) error
}
