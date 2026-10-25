package knowledge

import (
	"context"
	"time"

	"aranea-agents/pkg/apierror"
)

// ErrUnsupportedOperation 契约拒绝哨兵（ChunkTemporalReader 章）：chunks 无
// recorded 轴台账，span.RecordedAt 非零的结构化拒绝统一走本值——不误报为
// BadRequest 语义以外的可变错误，调用方可用 apierror.Is 精确匹配。
var ErrUnsupportedOperation = apierror.BadRequest(apierror.DomainKnowledge, "recorded-axis as-of reads are not supported for knowledge chunks")

// chunk_temporal_port.go G-32 T3b 全量时态窄端口声明（ADR-KN-20261022-I 决策 12）。
// 仅声明（Stability: evolving，AS-STA-01）；实现/接线 T4 末段吸收一次到位：
//   - data 层内核已就位于 internal/data/knowledge_chunk_archive.go——稠密：
//     listChunksAsOfCore / queryChunksAsOfANN（per-leg KNN，决策12③字面形态）；
//     sparse：queryChunksAsOfTsvector / queryChunksAsOfTrigram /
//     queryChunksAsOfSubstring / queryChunksAsOfBigramOverlap（CON-2 补齐——
//     与 SearchChunksBM25 同词素口径、去 recency 化）；
//   - 接线点：knowledgeRepo 实现（data 新文件内）+ Usecase 方法
//     （biz/knowledge 新文件方法，免触 knowledge.go 外源脏区）+ 服务/RPC 装配；
//     sparse 的 LexicalSearchQueries fan-out 与 RRF 融合在服务层复用
//     mergeBM25Rankings 同轨（rrfK=60）。

// KnowledgeChunkIncarnation 化身投影（视图 knowledge_chunk_incarnations 行的
// biz 侧形状）。Archived=false 为当前腿（valid_to 恒 infinity——as-of 查询
// 非 now() 时由存活谓词裁剪），true 为归档腿（chunk_archive 物化档）。
// Score 仅召回路径有意义——RecallChunksAsOf=纯余弦相似度（1-余弦距离），
// SparseRecallChunksAsOf=RRF 融合分；list 路径恒 0。
type KnowledgeChunkIncarnation struct {
	ChunkID       string
	DocID         string
	CollectionID  string
	Content       string
	ContentKey    string
	ChunkRole     string
	ParentChunkID string
	ChunkIndex    int
	CitedCount    int
	Metadata      string
	Archived      bool
	Score         float64
}

// ChunkArchiveReader 引用链内容回落端口（决策 7）。
// Stability: evolving——随 T4 引用链接线冻结为 Stable。
//
// 契约：按 chunk_id 精确回落 MAX(revision_no) 最新死亡化身的 content（历史
// 内容）；无档 id 静默缺席（「有档回落、无档仍弃」，DropsMissingChunks 钉的
// 演化形态）。授权不归此口——调用方维持 collection 级当下口径（回落授权=
// collection 级当下、内容用历史）。
type ChunkArchiveReader interface {
	LatestArchivedChunkContents(ctx context.Context, chunkIDs []string) (map[string]string, error)
}

// ChunkTemporalReader 全量时态读端口（决策 12）。
// Stability: evolving——as-of 谓词族随 T5 e2e 钉⑨收紧。
//
// 契约（与 temporal.go G-05 四象限不同——此处仅 T 轴）：
//   - span 仅消费 ValidAt；RecordedAt 非零即 ErrUnsupportedOperation
//     （chunks 无 recorded 轴台账，谎报二次迹不如结构性拒绝）；
//   - ValidAt 零值 = current 快报等价（调用方应走原路径，本口不承诺
//     recency/stale 快手路径的位序一致）；
//   - 存活谓词左闭右开 [valid_from, valid_to)：边界等值归新区间；
//   - 热度副作用对时态查询结构性关闭（与 G-05 同原则——历史读不拨动当下权重）。
type ChunkTemporalReader interface {
	// ListChunksAsOf 集合（docID 非空收窄单文档）在 span.ValidAt 存活的
	// child 化身全量投影，排序 (doc_id, chunk_index, chunk_id)。
	ListChunksAsOf(ctx context.Context, collectionID string, span TemporalSpan, docID string) ([]KnowledgeChunkIncarnation, error)
	// RecallChunksAsOf 双腿 top-k 稠密召回：current 腿（knowledge_chunks ×
	// chunk_embeddings）与归档腿（chunk_archive × chunk_embeddings_archive）
	// 经化身视图统一存活谓词后【各自 per-leg KNN】再合并去重（chunk_id 级
	// 取距离最小者；决策12③字面形态，CON-8 重构后 HNSW 可达），得分=纯
	// 余弦相似度（历史透镜不做现时新鲜度衰减，决策 12 登记的对称计分）。
	RecallChunksAsOf(ctx context.Context, collectionID string, queryEmbedding []float32, span TemporalSpan, topK int, docID string) ([]KnowledgeChunkIncarnation, error)
	// SparseRecallChunksAsOf 决策12③ sparse 对型召回（CON-2 补齐声明）：
	// 词法四分支（tsvector/trigram/substring/CJK bigram）在化身视图存活谓词
	// 下各跑 top-k 后 RRF 融合（与 SearchChunksBM25 同 fan-out/融合轨），
	// 分支原生得分直出、不套 recency 衰减。短针门槛与 bigram 抽取复用
	// LexicalSearchQueries/CJKBigrams 服务层语义。
	SparseRecallChunksAsOf(ctx context.Context, collectionID string, query string, span TemporalSpan, topK int, docID string) ([]KnowledgeChunkIncarnation, error)
}

// ChunkAsOfClock 注入时针（ListChunksAsOf/RecallChunksAsOf 的 span.ValidAt 为空
// 时的等价时点求值器）。生产实现 = time.Now；测试实现可冻结。T4 接线时由
// Usecase 装配持有，本声明先行冻结形状。
type ChunkAsOfClock func() time.Time
