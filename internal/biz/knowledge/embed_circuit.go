package knowledge

import (
	"context"
	"time"
)

// SP2 #9 embedding 熔断（SiYuan block_embeddings fail_count 同源）：
// embed 失败不再拖垮词法索引——chunks 写 NULL 向量照常落库，文档记熔断计数，
// 后台按指数退避重试补齐向量。故障期内容变更跳过 embed 尝试（不打 API）。

const (
	// embedCircuitBaseBackoff 退避基数：fc=1 时 1 分钟。
	embedCircuitBaseBackoff = time.Minute
	// embedCircuitMaxShift 指数左移上限：1min << 6 = 64min 封顶。
	embedCircuitMaxShift = 6
)

// EmbedCircuitAllow 判定当前是否允许对文档发起 embed 尝试。
// failCount<=0 或从未失败（lastTried 零值）恒放行；否则须越过退避窗口
// （base << (fc-1)，封顶 64min）。
func EmbedCircuitAllow(failCount int, lastTried, now time.Time) bool {
	if failCount <= 0 || lastTried.IsZero() {
		return true
	}
	shift := failCount - 1
	if shift > embedCircuitMaxShift {
		shift = embedCircuitMaxShift
	}
	return now.Sub(lastTried) >= embedCircuitBaseBackoff<<shift
}

// ChunkEmbedding 是单 chunk 的向量回填单元（按 chunk ID 精确寻址——
// ChunkIndex 源自 splitter 元数据，不保证 0..N-1 连续，禁止按位置映射）。
type ChunkEmbedding struct {
	ChunkID   string
	Embedding []float32
}

// EmbedCircuitRepo 是 embedding 熔断状态与降级恢复的持久化端口（SP2 #9）。
// Stability:evolving
type EmbedCircuitRepo interface {
	// UpdateDocumentEmbedCircuit 回写文档熔断状态（failCount=0 表示复位）。
	UpdateDocumentEmbedCircuit(ctx context.Context, id string, failCount int, lastTried time.Time) error
	// ListEmbedDegradedDocuments 列出熔断中（failCount>0）的文档，供退避重试。
	ListEmbedDegradedDocuments(ctx context.Context, collectionID string, limit int) ([]Document, error)
	// UpdateChunkEmbeddings 恢复成功时按 chunk ID 回填向量。
	UpdateChunkEmbeddings(ctx context.Context, docID string, vecs []ChunkEmbedding) error
}

// SetEmbedCircuitRepo 接线 embedding 熔断端口（可选能力；未接线时熔断读写降级 no-op——
// embed 失败仍降级词法索引，仅失去跨进程熔断记忆与后台重试）。
func (u *Usecase) SetEmbedCircuitRepo(ec EmbedCircuitRepo) {
	u.embedCircuit = ec
}

// UpdateDocumentEmbedCircuit 回写熔断状态；未接线时 no-op。
func (u *Usecase) UpdateDocumentEmbedCircuit(ctx context.Context, id string, failCount int, lastTried time.Time) error {
	if u == nil || u.embedCircuit == nil {
		return nil
	}
	return u.embedCircuit.UpdateDocumentEmbedCircuit(ctx, id, failCount, lastTried)
}

// ListEmbedDegradedDocuments 列出熔断中文档；未接线时返回空。
func (u *Usecase) ListEmbedDegradedDocuments(ctx context.Context, collectionID string, limit int) ([]Document, error) {
	if u == nil || u.embedCircuit == nil {
		return nil, nil
	}
	return u.embedCircuit.ListEmbedDegradedDocuments(ctx, collectionID, limit)
}

// UpdateChunkEmbeddings 回填 chunk 向量；未接线时 no-op。
func (u *Usecase) UpdateChunkEmbeddings(ctx context.Context, docID string, vecs []ChunkEmbedding) error {
	if u == nil || u.embedCircuit == nil {
		return nil
	}
	return u.embedCircuit.UpdateChunkEmbeddings(ctx, docID, vecs)
}
