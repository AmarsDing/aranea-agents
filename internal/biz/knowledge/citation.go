package knowledge

import (
	"context"
	"time"
)

// ── 29-token P2-2: knowledge chunk citation tracking（cited 回采） ──────────
//
// 记忆侧三段计数（FR-12.6 recalled/injected/cited）已闭环；知识侧缺最后一
// 公里——knowledge_search / knowledge_reflect 返回的 chunk 是否被助手回复
// 真正引用，此前无回采追踪，命中率无法度量。本组端口支撑知识侧 cited 段：
//
//	trace reader — 扫描 knowledge_recalled notice（每次检索调用注入的
//	               chunk 集合），join 该 turn 终态回复，批量解析 chunk 全文
//	backfill worker — 启发式检测回复是否引用了 chunk（cronrunner/jobs）
//	recorder — (chunk, turn) 去重账本 + cited_count 首次命中 +1

// ChunkCitation is one (chunk, turn) citation pair detected by the citation
// backfill worker.
type ChunkCitation struct {
	ChunkID string
	TurnID  string
}

// KnowledgeChunkCitationRecorder records citations into the dedup ledger and
// increments cited_count for first-seen (chunk, turn) pairs only.
// Stability:evolving
type KnowledgeChunkCitationRecorder interface {
	RecordChunkCitations(ctx context.Context, citations []ChunkCitation) error
}

// CitationChunkRef is a candidate chunk from one knowledge_recalled notice,
// carrying the full content for heuristic citation matching.
type CitationChunkRef struct {
	ChunkID string
	Content string
}

// KnowledgeCitationCandidate bundles one recall notice's chunks and the
// assistant reply text for citation detection. One turn may carry multiple
// candidates (multiple knowledge_search/knowledge_reflect calls per turn).
type KnowledgeCitationCandidate struct {
	TurnID    string
	ReplyText string
	Chunks    []CitationChunkRef
}

// KnowledgeCitationTraceReader loads recent knowledge_recalled notices joined
// with their turn's assistant reply and full chunk contents. Implemented by
// the data layer over steps_v2 + knowledge_chunks.
// Stability:evolving
type KnowledgeCitationTraceReader interface {
	// ListKnowledgeCitationCandidates returns candidates from knowledge_recalled
	// notices created at or after since, newest first, capped at limit notices.
	// Candidates with no parseable chunks or no reply text are skipped.
	ListKnowledgeCitationCandidates(ctx context.Context, since time.Time, limit int) ([]KnowledgeCitationCandidate, error)
}
