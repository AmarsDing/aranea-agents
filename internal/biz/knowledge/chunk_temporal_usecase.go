package knowledge

import (
	"context"
	"time"
)

// chunk_temporal_usecase.go — G-32 T4 item 4 时态读腿 Usecase 接线（
// ADR-KN-20261022-I 决策 12；端口声明在 chunk_temporal_port.go，data 实现在
// internal/data/knowledge_temporal_reader.go）。方法新文件落位，免触
// knowledge.go 外源脏区（temporalFields 载体在 domains.go）。
//
// 责任划分：
//   - 时针注入：span.ValidAt 零值在本层解析（装配时钟，生产=time.Now，测试
//     可冻结）后调用 reader——端口声明「T4 接线时由 Usecase 装配持有」；
//   - 空腿拒绝：未接线 ChunkTemporalReader 时返回 ErrUnavailable，不蜕化为
//     current 快报（谎报时态等价不如结构性拒绝，与 recorded 轴拒绝同原则）；
//   - RecordedAt 拒绝：reader 侧 ErrUnsupportedOperation 守卫，本层透传
//     不吞错。

// SetChunkTemporalReader 接线全量时态读端口（可选；未接线时时态读显式
// ErrUnavailable）。clock=nil → time.Now（生产形态）。
func (u *Usecase) SetChunkTemporalReader(reader ChunkTemporalReader, clock ChunkAsOfClock) {
	u.temporalReader = reader
	if clock == nil {
		clock = time.Now
	}
	u.asOfClock = clock
}

// resolveTemporalSpan 时针解析：ValidAt 零值 → 装配时钟现值；RecordedAt 保持
// 原样移交 reader 守卫（不在本层预拒绝，单一守卫点位口径在 reader 侧）。
func (u *Usecase) resolveTemporalSpan(span TemporalSpan) TemporalSpan {
	if span.ValidAt.IsZero() && u.asOfClock != nil {
		span.ValidAt = u.asOfClock()
	}
	return span
}

func (u *Usecase) requireTemporalReader() (ChunkTemporalReader, error) {
	if u == nil || u.temporalReader == nil {
		return nil, ErrUnavailable
	}
	return u.temporalReader, nil
}

// ListChunksAsOf 集合（docID 非空收窄单文档）在 span.ValidAt 存活的 child 化身
// 全量投影（archived 双标；排序 (doc_id, chunk_index, chunk_id)）。
func (u *Usecase) ListChunksAsOf(ctx context.Context, collectionID string, span TemporalSpan, docID string) ([]KnowledgeChunkIncarnation, error) {
	reader, err := u.requireTemporalReader()
	if err != nil {
		return nil, err
	}
	return reader.ListChunksAsOf(ctx, collectionID, u.resolveTemporalSpan(span), docID)
}

// RecallChunksAsOf 双腿 per-leg KNN 时态稠密召回（纯余弦相似度，无现时新鲜度
// 衰减；topK<=0 退化 data 内核默认 5）。
func (u *Usecase) RecallChunksAsOf(ctx context.Context, collectionID string, queryEmbedding []float32, span TemporalSpan, topK int, docID string) ([]KnowledgeChunkIncarnation, error) {
	reader, err := u.requireTemporalReader()
	if err != nil {
		return nil, err
	}
	return reader.RecallChunksAsOf(ctx, collectionID, queryEmbedding, u.resolveTemporalSpan(span), topK, docID)
}

// SparseRecallChunksAsOf 词法四分支时态召回（决策 12③ sparse 对型；RRF 融合
// rrfK=60 与 SearchChunksBM25 同轨、分支原生直出无 recency 衰减）。
func (u *Usecase) SparseRecallChunksAsOf(ctx context.Context, collectionID, query string, span TemporalSpan, topK int, docID string) ([]KnowledgeChunkIncarnation, error) {
	reader, err := u.requireTemporalReader()
	if err != nil {
		return nil, err
	}
	return reader.SparseRecallChunksAsOf(ctx, collectionID, query, u.resolveTemporalSpan(span), topK, docID)
}
