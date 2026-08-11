package biz

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// memoryRecallEmbedTimeout bounds query embedding on the LLM critical path
// (P0-C, 2026-08-11). Recall degrades to non-vector search past this budget.
const memoryRecallEmbedTimeout = 3 * time.Second

// CompositeRecallQuery merges L2 episodes and L3 facts by fused score.
type CompositeRecallQuery struct {
	AgentID   string
	SessionID string
	UserID    string
	Query     string
	Limit     int32
}

// CompositeRecallHit is one ranked line for composite prompt injection.
type CompositeRecallHit struct {
	Layer string
	Line  string
	Score float64
	// P2-04: provenance metadata for L3 facts (empty for L2 episodes).
	FactID        string
	SourceSession string
	Confidence    float64
	Version       int
}

// SessionCompositeRecallStore loads fused L2+L3 candidates (implemented by data memory_shim adapters).
type SessionCompositeRecallStore interface {
	CompositeSearchMemories(ctx context.Context, agentID, sessionID, userID, query string, limit int32) ([]CompositeRecallStoreRow, error)
}

// CompositeRecallStoreRow is the store-neutral composite recall row.
type CompositeRecallStoreRow struct {
	Layer     string
	Title     string
	Summary   string
	Statement string
	Score     float64
	// P2-04: provenance metadata for L3 facts (empty for L2 episodes).
	FactID        string
	SourceSession string
	Confidence    float64
	Version       int
}

// MemoryCompositeRecaller performs cross-layer L2+L3 recall for prompt injection.
type MemoryCompositeRecaller interface {
	RecallComposite(ctx context.Context, q CompositeRecallQuery) ([]CompositeRecallHit, error)
}

// ProactiveRecallContext captures the current conversation state for
// proactive recall. This is the biz-level mirror of the framework's
// ConversationContext type, defined here to avoid importing the framework
// package (red line #2).
type ProactiveRecallContext struct {
	// MentionedEntities are people, places, or topics mentioned in the
	// conversation. Each entity is used as a search keyword to retrieve
	// related memories.
	MentionedEntities []string

	// CurrentTopic is the topic of the current conversation turn.
	CurrentTopic string

	// UserStatement is the user's latest statement, used for contradiction
	// detection.
	UserStatement string
}

// ProactiveRecaller is the biz port for proactive memory recall.
// Implementations live in internal/memory/trpc and delegate to the
// framework's memory.Service.ProactiveRecall method.
type ProactiveRecaller interface {
	ProactiveRecall(ctx context.Context, agentID, userID string, convCtx ProactiveRecallContext) ([]CompositeRecallHit, error)
}

// MemoryCompositeRecallUsecase wraps SessionCompositeRecallStore and
// optionally a ProactiveRecaller for conversation-driven memory surfacing.
type MemoryCompositeRecallUsecase struct {
	store             SessionCompositeRecallStore
	proactiveRecaller ProactiveRecaller
	// P2-R1: layered recallers. When l3 is set, RecallComposite composes the
	// fused L2/L3 usecases directly (embedding + pgvector/FTS RRF + calibrated
	// scores + recalled_count bumps) instead of the degraded legacy store
	// path (raw repo, importance-as-score, no counters).
	l2 MemoryL2Recaller
	l3 MemoryL3Recaller
	// P0-C: shared per-turn query embedding. When set, recallCompositeLayered
	// embeds the query once and passes the vector down to both layer
	// recallers (previously each embedded the same query independently).
	embedder EmbeddingService
	lg       loggateway.Logger
}

// NewMemoryCompositeRecallUsecase wires the composite recall store.
// The proactive recaller is optional; use SetProactiveRecaller to inject it
// after construction (avoids breaking existing Wire providers).
func NewMemoryCompositeRecallUsecase(store SessionCompositeRecallStore) *MemoryCompositeRecallUsecase {
	if store == nil {
		return nil
	}
	return &MemoryCompositeRecallUsecase{store: store}
}

// SetProactiveRecaller injects a proactive recaller after construction.
// This avoids breaking the existing NewMemoryCompositeRecallUsecase signature
// and allows Wire to bind the proactive recaller separately.
func (uc *MemoryCompositeRecallUsecase) SetProactiveRecaller(r ProactiveRecaller) {
	if uc == nil {
		return
	}
	uc.proactiveRecaller = r
}

// SetLayerRecallers injects the fused L2/L3 recall usecases (P2-R1). When l3
// is non-nil, RecallComposite takes the layered path; nil l2 skips episodes.
func (uc *MemoryCompositeRecallUsecase) SetLayerRecallers(l2 MemoryL2Recaller, l3 MemoryL3Recaller) {
	if uc == nil {
		return
	}
	uc.l2 = l2
	uc.l3 = l3
}

// SetEmbedder injects the shared query embedder (P0-C). When set, the
// layered path embeds the query once per turn instead of letting L2 and L3
// each embed independently.
func (uc *MemoryCompositeRecallUsecase) SetEmbedder(e EmbeddingService, lg loggateway.Logger) {
	if uc == nil {
		return
	}
	uc.embedder = e
	uc.lg = lg
}

func (uc *MemoryCompositeRecallUsecase) RecallComposite(ctx context.Context, q CompositeRecallQuery) ([]CompositeRecallHit, error) {
	if uc == nil {
		return nil, nil
	}
	if uc.l3 != nil {
		return uc.recallCompositeLayered(ctx, q)
	}
	if uc.store == nil {
		return nil, nil
	}
	agentID := strings.TrimSpace(q.AgentID)
	if agentID == "" {
		return nil, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	rows, err := uc.store.CompositeSearchMemories(ctx, agentID, strings.TrimSpace(q.SessionID), strings.TrimSpace(q.UserID), strings.TrimSpace(q.Query), limit)
	if err != nil {
		return nil, err
	}
	out := make([]CompositeRecallHit, 0, len(rows))
	for _, row := range rows {
		line := formatCompositeRecallLine(row)
		if line == "" {
			continue
		}
		out = append(out, CompositeRecallHit{
			Layer:         row.Layer,
			Line:          line,
			Score:         row.Score,
			FactID:        row.FactID,
			SourceSession: row.SourceSession,
			Confidence:    row.Confidence,
			Version:       row.Version,
		})
	}
	return out, nil
}

// compositeMMRLambda controls the relevance-vs-diversity trade-off in the
// merged L2+L3 rerank. 0.7 = 70% relevance weight + 30% diversity penalty.
const compositeMMRLambda = 0.7

// recallCompositeLayered composes the fused L2/L3 recall usecases (P2-R1).
// Both layers arrive with calibrated score breakdowns annotated into the raw
// JSON ("scores" key); the merged set is ranked by scores.total, MMR-reranked
// for diversity, and capped at limit.
func (uc *MemoryCompositeRecallUsecase) recallCompositeLayered(ctx context.Context, q CompositeRecallQuery) ([]CompositeRecallHit, error) {
	agentID := strings.TrimSpace(q.AgentID)
	if agentID == "" {
		return nil, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	query := strings.TrimSpace(q.Query)

	// P0-C: embed the query once per turn and share the vector with both
	// layer recallers. EmbedAttempted is set even on failure so layers
	// degrade to non-vector search instead of re-embedding independently.
	// E 预算表分解（2026-08-11）：语音真机实测 memory cue build 2579ms 阻塞
	// LLM 关键路径，拆出 embed/L2/L3 子阶段耗时定位根因。
	stageStart := time.Now()
	embedMs := int64(-1)
	var qvec []float32
	embedAttempted := false
	if query != "" && uc.embedder != nil {
		embedAttempted = true
		embedCtx, cancel := context.WithTimeout(ctx, memoryRecallEmbedTimeout)
		vec, err := uc.embedder.Embed(embedCtx, query)
		cancel()
		embedMs = time.Since(stageStart).Milliseconds()
		if err == nil {
			qvec = vec
		} else if uc.lg != nil {
			uc.lg.Warn("composite recall embed failed, layers degrade to non-vector search",
				loggateway.StepID("memory.composite_embed_fail"),
				loggateway.Err(err))
		}
	}

	var all []CompositeRecallHit
	l2Ms := int64(-1)
	l3Ms := int64(-1)

	// L2 episodes (fused usecase: embedding + vector/cross-encoder rerank).
	if uc.l2 != nil {
		l2Start := time.Now()
		epRows, err := uc.l2.RecallEpisodes(ctx, L2RecallQuery{
			AgentID:        agentID,
			SessionID:      strings.TrimSpace(q.SessionID),
			Query:          query,
			Limit:          limit,
			QueryEmbedding: qvec,
			EmbedAttempted: embedAttempted,
		})
		l2Ms = time.Since(l2Start).Milliseconds()
		if err == nil {
			for _, raw := range epRows {
				hit := compositeHitFromEpisodeJSON(raw)
				if hit.Line != "" {
					all = append(all, hit)
				}
			}
		}
	}

	// L3 facts (fused usecase: embedding + pgvector/FTS RRF + decay fusion +
	// recalled_count bumps inside the scored store adapter).
	l3Start := time.Now()
	factRows, err := uc.l3.RecallFactsFused(ctx, L3FusedRecallQuery{
		Runtime:        MemoryRuntimeContext{AgentID: agentID, UserID: strings.TrimSpace(q.UserID)},
		Query:          query,
		Limit:          limit,
		QueryEmbedding: qvec,
		EmbedAttempted: embedAttempted,
	})
	l3Ms = time.Since(l3Start).Milliseconds()
	if err == nil {
		for _, raw := range factRows {
			hit := compositeHitFromFactJSON(raw)
			if hit.Line != "" {
				all = append(all, hit)
			}
		}
	}

	if uc.lg != nil {
		uc.lg.Info("composite recall stage timing",
			loggateway.StepID("memory.composite_recall.stages"),
			loggateway.Any("embed_ms", embedMs),
			loggateway.Any("l2_ms", l2Ms),
			loggateway.Any("l3_ms", l3Ms),
			loggateway.Any("total_ms", time.Since(stageStart).Milliseconds()),
			loggateway.Any("hits", len(all)))
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })

	// MMR diversity rerank (same policy as the legacy data-adapter path).
	if len(all) > 1 {
		texts := make([]string, len(all))
		scores := make([]float64, len(all))
		for i, hit := range all {
			texts[i] = hit.Line
			scores[i] = hit.Score
		}
		order := MMRRerankTexts(texts, scores, int(limit), compositeMMRLambda)
		reranked := make([]CompositeRecallHit, 0, len(order))
		for _, idx := range order {
			reranked = append(reranked, all[idx])
		}
		all = reranked
	}

	if len(all) > int(limit) {
		all = all[:limit]
	}
	return all, nil
}

// compositeHitFromEpisodeJSON maps a raw L2 episode row to a composite hit.
// Ranking uses the annotated scores.total (P2-R1), falling back to raw
// importance for rows produced before score annotation existed.
func compositeHitFromEpisodeJSON(raw []byte) CompositeRecallHit {
	var row map[string]any
	if json.Unmarshal(raw, &row) != nil {
		return CompositeRecallHit{}
	}
	title, _ := row["title"].(string)
	summary, _ := row["outcome_summary"].(string)
	line := formatCompositeRecallLine(CompositeRecallStoreRow{Layer: "L2", Title: title, Summary: summary})
	return CompositeRecallHit{
		Layer: "L2",
		Line:  line,
		Score: compositeRowScore(row),
	}
}

// compositeHitFromFactJSON maps a raw L3 fact row to a composite hit,
// propagating provenance metadata (P2-04) for the transparency notice.
func compositeHitFromFactJSON(raw []byte) CompositeRecallHit {
	var row map[string]any
	if json.Unmarshal(raw, &row) != nil {
		return CompositeRecallHit{}
	}
	stmt, _ := row["statement"].(string)
	factID, _ := row["id"].(string)
	srcSess, _ := row["source_session_id"].(string)
	var confidence float64
	if v, ok := row["confidence"].(float64); ok {
		confidence = v
	}
	var version int
	if v, ok := row["version"].(float64); ok {
		version = int(v)
	}
	return CompositeRecallHit{
		Layer:         "L3",
		Line:          formatCompositeRecallLine(CompositeRecallStoreRow{Layer: "L3", Statement: stmt}),
		Score:         compositeRowScore(row),
		FactID:        factID,
		SourceSession: srcSess,
		Confidence:    confidence,
		Version:       version,
	}
}

// compositeRowScore extracts the calibrated scores.total annotated by the
// recall pipeline; falls back to the raw importance field when absent.
func compositeRowScore(row map[string]any) float64 {
	if sc, ok := row["scores"].(map[string]any); ok {
		if total, ok := sc["total"].(float64); ok && total > 0 {
			return total
		}
	}
	if v, ok := row["importance"].(float64); ok {
		return v
	}
	return 0
}

// ProactiveRecall retrieves memories based on the conversation context
// (mentioned entities, current topic, user statement) without requiring an
// explicit query. It is intended to be called before each conversation turn
// to surface relevant memories that the agent should consider.
//
// Returns empty list (not error) when no proactive recaller is wired or
// when the conversation context carries no usable signal.
func (uc *MemoryCompositeRecallUsecase) ProactiveRecall(ctx context.Context, agentID, userID string, convCtx ProactiveRecallContext) ([]CompositeRecallHit, error) {
	if uc == nil || uc.proactiveRecaller == nil {
		return nil, nil
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, nil
	}
	return uc.proactiveRecaller.ProactiveRecall(ctx, agentID, strings.TrimSpace(userID), convCtx)
}

func formatCompositeRecallLine(row CompositeRecallStoreRow) string {
	layer := strings.ToUpper(strings.TrimSpace(row.Layer))
	switch layer {
	case "L2", "L2_EPISODE", "EPISODE":
		title := strings.TrimSpace(row.Title)
		summary := strings.TrimSpace(row.Summary)
		if title == "" {
			title = summary
		}
		if title == "" {
			return ""
		}
		if summary != "" && summary != title {
			return title + ": " + summary
		}
		return title
	default:
		stmt := strings.TrimSpace(row.Statement)
		if stmt == "" {
			stmt = strings.TrimSpace(row.Summary)
		}
		return stmt
	}
}
