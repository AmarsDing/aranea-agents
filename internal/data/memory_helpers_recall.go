package data

import (
	"aranea-agents/internal/biz"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode"
)

const (
	l2RecallCandidatePool = 40
	l2DecayHalfLifeDays   = 14.0
	l2ScoreWeightKeyword  = 0.35
	l2ScoreWeightVector   = 0.45
	l2ScoreWeightImport   = 0.15
	l2ScoreWeightSession  = 0.05

	l3RecallCandidatePool = 60
	l3DecayHalfLifeDays   = 30.0
	l3ScoreWeightKeyword  = 0.25
	l3ScoreWeightVector   = 0.30
	l3ScoreWeightImport   = 0.20
	l3ScoreWeightRecency  = 0.15
	l3ScoreWeightQuality  = 0.10

	ceRerankWeight = 0.15

	defaultEpisodeConfidence   = 0.75
	defaultL1ArchiveImportance = 0.6
	defaultFactQualityScore    = 0.5
)

type recallScoreBreakdown struct {
	Keyword      float64 `json:"keyword"`
	Vector       float64 `json:"vector"`
	Importance   float64 `json:"importance"`
	Recency      float64 `json:"recency"`
	QualityScore float64 `json:"quality_score"`
	CrossEncoder float64 `json:"cross_encoder"`
	RRF          float64 `json:"rrf,omitempty"`
	Total        float64 `json:"total"`
}

// recallDebugRow is one scored recall candidate for admin debug RPC.
type recallDebugRow struct {
	Layer     string               `json:"layer"`
	ID        string               `json:"id"`
	Title     string               `json:"title,omitempty"`
	Summary   string               `json:"summary,omitempty"`
	Statement string               `json:"statement,omitempty"`
	Scores    recallScoreBreakdown `json:"scores"`
	Raw       json.RawMessage      `json:"raw,omitempty"`
}

type scoredEpisode struct {
	raw       []byte
	id        string
	title     string
	summary   string
	score     float64
	breakdown recallScoreBreakdown
}

type scoredFact struct {
	raw       []byte
	id        string
	stmt      string
	details   string
	score     float64
	breakdown recallScoreBreakdown
}

func tokenizeQuery(q string) []string {
	if q == "" {
		return nil
	}
	parts := strings.FieldsFunc(q, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '!' || r == '?' ||
			r == '\t' || r == '\n' || r == '\r' ||
			r == '，' || r == '。' || r == '！' || r == '？' || r == '；' ||
			r == '：' || r == '、' || r == '（' || r == '）' || r == '“' || r == '”'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = appendKeywordTokens(out, p)
	}
	return filterRecallStopwords(out)
}

// recallStopwords are function words that dominate question-form queries
// (What/does/the) and flatten keywordOverlapScore. Content tokens (names,
// nouns, codes) are kept. CJK bigrams are never dropped here.
var recallStopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "is": {}, "are": {}, "was": {}, "were": {},
	"be": {}, "been": {}, "being": {}, "am": {}, "do": {}, "does": {}, "did": {},
	"what": {}, "which": {}, "who": {}, "whom": {}, "when": {}, "where": {},
	"why": {}, "how": {}, "can": {}, "could": {}, "would": {}, "should": {},
	"might": {}, "shall": {}, "i": {}, "me": {}, "my": {},
	"we": {}, "our": {}, "you": {}, "your": {}, "he": {}, "she": {}, "it": {},
	"they": {}, "them": {}, "their": {}, "this": {}, "that": {}, "these": {},
	"those": {}, "or": {}, "and": {}, "but": {}, "if": {}, "of": {}, "to": {},
	"for": {}, "in": {}, "on": {}, "at": {}, "by": {}, "with": {}, "from": {},
	"as": {}, "into": {}, "about": {}, "please": {}, "tell": {},

	"user": {},
	"的":    {}, "了": {}, "是": {}, "在": {}, "有": {}, "和": {}, "就": {},
	"不": {}, "都": {}, "也": {}, "很": {}, "到": {}, "要": {}, "去": {},
	"会": {}, "着": {}, "这": {}, "那": {}, "吗": {}, "呢": {}, "吧": {},
	"用户": {},
}

func filterRecallStopwords(tokens []string) []string {
	if len(tokens) == 0 {
		return tokens
	}
	out := tokens[:0]
	for _, tok := range tokens {
		key := strings.ToLower(tok)
		if _, skip := recallStopwords[key]; skip {
			continue
		}
		out = append(out, tok)
	}
	if len(out) == 0 {
		return tokens
	}
	return out
}

func factEventTime(row map[string]any) string {
	for _, key := range []string{"valid_from", "created_at", "updated_at"} {
		if v, _ := row[key].(string); strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// factTouchTime is when the row was last confirmed or used. Recency boost
// uses this so an in-place upsert (same fingerprint, preserved valid_from)
// still ranks as recently touched. Decay keeps factEventTime.
func factTouchTime(row map[string]any) string {
	for _, key := range []string{"last_used_at", "updated_at"} {
		if v, _ := row[key].(string); strings.TrimSpace(v) != "" {
			return v
		}
	}
	return factEventTime(row)
}

// appendKeywordTokens emits keyword tokens for one punctuation-free part.
// 连续 CJK 片段发重叠 bigram（单字发 unigram），避免整段中文成为一个不可拆的
// 巨型 token 导致 keywordOverlapScore 恒 0；非 CJK 片段保持整词（len>=2 字节）。
func appendKeywordTokens(out []string, part string) []string {
	runes := []rune(part)
	asciiStart := 0
	flushASCII := func(end int) {
		if end > asciiStart {
			if w := string(runes[asciiStart:end]); len(w) >= 2 {
				out = append(out, w)
			}
		}
	}
	i := 0
	for i < len(runes) {
		if !unicode.Is(unicode.Han, runes[i]) {
			i++
			continue
		}
		flushASCII(i)
		j := i
		for j < len(runes) && unicode.Is(unicode.Han, runes[j]) {
			j++
		}
		if j-i == 1 {
			out = append(out, string(runes[i:j]))
		} else {
			for k := i; k+1 < j; k++ {
				out = append(out, string(runes[k:k+2]))
			}
		}
		i = j
		asciiStart = j
	}
	flushASCII(len(runes))
	return out
}

func keywordOverlapScore(tokens []string, text string) float64 {
	if len(tokens) == 0 || text == "" {
		return 0
	}
	lowerText := strings.ToLower(text)
	hits := 0
	for _, tok := range tokens {
		if strings.Contains(lowerText, strings.ToLower(tok)) {
			hits++
		}
	}
	return float64(hits) / float64(len(tokens))
}

func decayFactor(endedAt string, now time.Time) float64 {
	endedAt = strings.TrimSpace(endedAt)
	if endedAt == "" {
		return 1
	}
	t, err := time.Parse(time.RFC3339Nano, endedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, endedAt)
	}
	if err != nil {
		return 1
	}
	days := now.Sub(t).Hours() / 24
	if days <= 0 {
		return 1
	}
	return math.Pow(0.5, days/l2DecayHalfLifeDays)
}

func factRecencyDecay(updatedAt string, now time.Time) float64 {
	updatedAt = strings.TrimSpace(updatedAt)
	if updatedAt == "" {
		return 1
	}
	t, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, updatedAt)
	}
	if err != nil {
		return 1
	}
	days := now.Sub(t).Hours() / 24
	if days <= 0 {
		return 1
	}
	return math.Pow(0.5, days/l3DecayHalfLifeDays)
}

// isEvergreenFactKind reports whether the fact kind represents stable,
// long-lived knowledge that should not decay over time.
// Evergreen kinds: user identity, user preference, agent instruction,
// domain knowledge — these are set once and remain valid.
func isEvergreenFactKind(kind string) bool {
	switch biz.CanonicalizeFactKind(kind, "") {
	case "user_identity", "preference", "agent_instruction", "domain_knowledge":
		return true
	default:
		return false
	}
}

// factDecayWithKind returns the recency decay factor for a fact, applying
// the evergreen exemption: evergreen fact kinds always return 1.0 (no decay),
// while non-evergreen kinds use the standard half-life decay.
func factDecayWithKind(factKind, updatedAt string, now time.Time) float64 {
	if isEvergreenFactKind(factKind) {
		return 1.0
	}
	return factRecencyDecay(updatedAt, now)
}

func recencyBoost(updatedAt string, now time.Time) float64 {
	updatedAt = strings.TrimSpace(updatedAt)
	if updatedAt == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, updatedAt)
	}
	if err != nil {
		return 0
	}
	days := now.Sub(t).Hours() / 24
	if days <= 7 {
		return 1.0
	}
	if days <= 30 {
		return 0.5
	}
	return 0.1
}

func episodePassage(raw []byte) string {
	var row map[string]any
	if json.Unmarshal(raw, &row) != nil {
		return ""
	}
	title, _ := row["title"].(string)
	summary, _ := row["outcome_summary"].(string)
	return strings.TrimSpace(title + " " + summary)
}

func factPassage(stmt, details string) string {
	return strings.TrimSpace(stmt + " " + details)
}

func applyCrossEncoderRerankToScored(reranker biz.Reranker, query string, scored []scoredEpisode, passages []string, apply func(i int, ceScore, total float64)) {
	if reranker == nil || strings.TrimSpace(query) == "" || len(scored) == 0 || len(scored) != len(passages) {
		return
	}
	for i := range scored {
		ceScore := reranker.Score(query, passages[i])
		total := (1-ceRerankWeight)*scored[i].score + ceRerankWeight*ceScore
		apply(i, ceScore, total)
	}
}

func applyCrossEncoderRerankToFactScored(reranker biz.Reranker, query string, scored []scoredFact, passages []string) {
	if reranker == nil || strings.TrimSpace(query) == "" || len(scored) == 0 || len(scored) != len(passages) {
		return
	}
	for i := range scored {
		ceScore := reranker.Score(query, passages[i])
		total := (1-ceRerankWeight)*scored[i].score + ceRerankWeight*ceScore
		scored[i].breakdown.CrossEncoder = ceScore
		scored[i].breakdown.Total = total
		scored[i].score = total
	}
}

// scoreFactRow computes the L3 hybrid recall score for one fact row JSON.
// vecOverrides, when non-nil, supplies the vector score per fact ID (pgvector
// path). An ID missing from vecOverrides (e.g. the vector store lags behind
// memory_facts, or its table is empty) falls back to the embedding_blob
// cosine when a query embedding is available, so the vector term never
// silently zeroes out for otherwise strong candidates.
func scoreFactRow(row map[string]any, tokens []string, queryEmbedding []float32, vecOverrides map[string]float64, now time.Time) recallScoreBreakdown {
	id, _ := row["id"].(string)
	stmt, _ := row["statement"].(string)
	details, _ := row["details_markdown"].(string)
	imp := anyFloat(row, "importance")
	qScore := anyFloat(row, "quality_score")
	eventAt := factEventTime(row)
	touchAt := factTouchTime(row)
	factKind, _ := row["fact_kind"].(string)

	kwScore := keywordOverlapScore(tokens, stmt+" "+details)
	var vecScore float64
	vecOK := false
	if vecOverrides != nil {
		vecScore, vecOK = vecOverrides[id]
	}
	if !vecOK && len(queryEmbedding) > 0 {
		// JSON marshal renders []byte as a base64 string (encoding/json
		// contract), so the string branch must base64-decode first; falling
		// back to raw bytes keeps compatibility with non-JSON producers.
		var embBlob []byte
		switch v := row["embedding_blob"].(type) {
		case []byte:
			embBlob = v
		case string:
			if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
				embBlob = decoded
			} else {
				embBlob = []byte(v)
			}
		}
		if embNorm, ok := row["embedding_norm"].(float64); ok && embNorm > 0 && len(embBlob) > 0 {
			emb := decodeFloat32Blob(embBlob)
			if len(emb) == len(queryEmbedding) {
				vecScore = cosineSimilarity(queryEmbedding, emb)
			}
		}
	}
	recency := recencyBoost(touchAt, now)
	decay := factDecayWithKind(factKind, eventAt, now)
	total := l3ScoreWeightKeyword*kwScore +
		l3ScoreWeightVector*vecScore +
		l3ScoreWeightImport*imp*decay +
		l3ScoreWeightRecency*recency +
		l3ScoreWeightQuality*qScore
	return recallScoreBreakdown{
		Keyword:      kwScore,
		Vector:       vecScore,
		Importance:   imp * decay,
		Recency:      recency,
		QualityScore: qScore,
		Total:        total,
	}
}

// scoreFactRows scans fact row JSON from rows and computes the hybrid recall
// score for each, keeping only rows at or above minScore. Callers that apply
// AdaptiveRecallMinScore after ranking pass minScore=0 here so the candidate
// distribution is visible to finalizeScoredFacts.
func scoreFactRows(rows *sql.Rows, tokens []string, queryEmbedding []float32, vecOverrides map[string]float64, minScore float64, now time.Time) []scoredFact {
	var scored []scoredFact
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			continue
		}
		var row map[string]any
		if json.Unmarshal(b, &row) != nil {
			continue
		}
		bd := scoreFactRow(row, tokens, queryEmbedding, vecOverrides, now)
		if bd.Total < minScore {
			continue
		}
		stmt, _ := row["statement"].(string)
		details, _ := row["details_markdown"].(string)
		id, _ := row["id"].(string)
		scored = append(scored, scoredFact{
			raw:       b,
			id:        id,
			stmt:      stmt,
			details:   details,
			score:     bd.Total,
			breakdown: bd,
		})
	}
	return scored
}

// annotateFactScores injects the computed score breakdown into the fact row
// JSON under the "scores" key so downstream consumers (scored recall adapter,
// debug tools) can read the ranking without re-computing it. On any error the
// original row is returned unchanged.
func annotateFactScores(raw []byte, bd recallScoreBreakdown) []byte {
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err != nil {
		return raw
	}
	row["scores"] = bd
	out, err := json.Marshal(row)
	if err != nil {
		return raw
	}
	return out
}

// annotateEpisodeScores is the L2 twin of annotateFactScores (P2-R1): the
// composite layered path ranks episodes and facts by the calibrated
// scores.total, so episode rows must expose the same "scores" contract.
func annotateEpisodeScores(raw []byte, bd recallScoreBreakdown) []byte {
	return annotateFactScores(raw, bd)
}

// VectorSearcher abstracts vector similarity search for recall operations.
type VectorSearcher interface {
	Search(ctx context.Context, embedding []float64, topK int, minScore float64) ([]VectorSearchHit, error)
}

// VectorSearchHit represents a single search result from a VectorSearcher.
type VectorSearchHit struct {
	ID    string
	Score float64
}
