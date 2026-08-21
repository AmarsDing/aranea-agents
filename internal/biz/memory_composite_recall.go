package biz

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

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
	// P1-1（2026-08-16）：layered 路径此前未透传 L3 作用域与质量门，
	// 导致 team/user scope 事实与 L3RecallMinScore 在默认主路径上静默失效。
	// 以下字段仅 layered 路径消费；legacy store 路径保持原行为。
	TeamID         string
	Workspace      string
	Scopes         []string
	MinScoreQuery  float64
	MinScorePassive float64
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
	// ValidFrom（2026-08-21 P0-1）：L3 事实的 valid_from（RFC3339，可空）。
	// 同实体变体 tiebreak 的「最新优先」信号——活跃事实 valid_to 恒 NULL，
	// 变体间的新旧只能由 valid_from/version 区分。L2 episode 为空。
	ValidFrom string
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

// compositeVariantTiebreakEpsilon（2026-08-21 P0-1，up-03 根修）：同实体变体的
// 「分数接近」窗口。校准分差 ≤ ε 视为同档进入数值/最新 tiebreak；超过 ε 严格
// 按分排序。注意 ε 窗口比较器不具备传递性（a≈b、b≈c 但 a≉c），对 ≤20 条的
// 小候选集是可接受的启发式，不要把它当作全序。
const compositeVariantTiebreakEpsilon = 0.05

// numericIntentRe 命中「问数值/问时间」意图。update 类问题（"空调现在设多少度"）
// 的正确答案几乎总含数值或时间，而告警/规则类同实体变体常不含——同档候选里
// 含数字的陈述优先。
var numericIntentRe = regexp.MustCompile(`多少|几(个|位|岁|点|号|月|年)|多大|多久|多长|温度|湿度|价格|价钱|多少钱|单价|报价|成本|费用|数量|次数|频率|年龄|身高|体重|尺寸|浓度|电压|电流|功率|转速|时速|日期|时间|(?i)how\s+(much|many|old)|price|temperature|humidity|size|age|cost|voltage|current|power|speed|when`)

// QueryHasNumericIntent reports whether the query asks for a numeric/time value.
func QueryHasNumericIntent(query string) bool {
	return numericIntentRe.MatchString(query)
}

func lineHasDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// compareValidFrom 比较两条事实的 valid_from：都能解析按时间比；都不能解析
// 退字符串序（RFC3339 定宽时字典序即时间序）；一空一非空时非空者优先（有
// 时态信息的事实比缺失的更可信为「新」）。返回 +1/0/-1。
func compareValidFrom(a, b string) int {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		switch {
		case a == b:
			return 0
		case a == "":
			return -1
		default:
			return 1
		}
	}
	ta, ea := time.Parse(time.RFC3339, a)
	tb, eb := time.Parse(time.RFC3339, b)
	if ea == nil && eb == nil {
		switch {
		case ta.After(tb):
			return 1
		case ta.Before(tb):
			return -1
		default:
			return 0
		}
	}
	return strings.Compare(a, b)
}

// CompositeHitTiebreakLess 是融合召回的统一排序器（P0-1）：校准分降序为主；
// 同档（Δ≤ε）时 ① 数值意图下含数字行优先 ② 双 L3 时 valid_from 新者优先
// （再退 version 高者优先）。biz 融合排序与 agent 打包前 mergeCompositeHits
// 共用，保证「候选 rank-1 即注入 rank-1」不被二次重排破坏。
func CompositeHitTiebreakLess(numericIntent bool) func(a, b CompositeRecallHit) bool {
	return func(a, b CompositeRecallHit) bool {
		if d := a.Score - b.Score; math.Abs(d) > compositeVariantTiebreakEpsilon {
			return d > 0
		}
		if numericIntent {
			ad, bd := lineHasDigit(a.Line), lineHasDigit(b.Line)
			if ad != bd {
				return ad
			}
		}
		if a.Layer == "L3" && b.Layer == "L3" {
			if c := compareValidFrom(a.ValidFrom, b.ValidFrom); c != 0 {
				return c > 0
			}
			if a.Version != b.Version {
				return a.Version > b.Version
			}
		}
		return false
	}
}

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
		Runtime: MemoryRuntimeContext{
			AgentID:   agentID,
			UserID:    strings.TrimSpace(q.UserID),
			TeamID:    strings.TrimSpace(q.TeamID),
			Workspace: strings.TrimSpace(q.Workspace),
		},
		Scopes:         q.Scopes,
		Query:          query,
		Limit:          limit,
		MinScoreQuery:  q.MinScoreQuery,
		MinScorePassive: q.MinScorePassive,
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

	// P0-1（2026-08-21）：同档变体按数值意图/最新优先 tiebreak，不再纯按分排。
	less := CompositeHitTiebreakLess(QueryHasNumericIntent(query))
	sort.Slice(all, func(i, j int) bool { return less(all[i], all[j]) })

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
	validFrom, _ := row["valid_from"].(string)
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
		ValidFrom:     validFrom,
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
