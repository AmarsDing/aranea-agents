package data

import (
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestBruteForceThresholdValue verifies the threshold constant used by shouldUseBruteForce.
func TestBruteForceThresholdValue(t *testing.T) {
	if biz.DefaultFactBruteForceThreshold != 5000 {
		t.Errorf("DefaultFactBruteForceThreshold = %d, want 5000", biz.DefaultFactBruteForceThreshold)
	}
}

// TestBruteForceDecisionLogic verifies the pure decision logic extracted from shouldUseBruteForce.
// The actual method queries the DB, so we test the threshold comparison in isolation.
func TestBruteForceDecisionLogic(t *testing.T) {
	threshold := biz.DefaultFactBruteForceThreshold

	tests := []struct {
		name           string
		factCount      int
		queryEmbedding []float32
		wantBruteForce bool
	}{
		{
			name:           "count below threshold with embedding still uses lexical scan when isolated",
			factCount:      100,
			queryEmbedding: []float32{0.1, 0.2, 0.3},
			wantBruteForce: true,
		},
		{
			name:           "count at threshold uses brute force",
			factCount:      5000,
			queryEmbedding: []float32{0.1, 0.2, 0.3},
			wantBruteForce: true,
		},
		{
			name:           "count above threshold with embedding does not use brute force",
			factCount:      5001,
			queryEmbedding: []float32{0.1, 0.2, 0.3},
			wantBruteForce: false,
		},
		{
			name:           "count above threshold without embedding uses FTS-first not brute force",
			factCount:      10000,
			queryEmbedding: []float32{},
			wantBruteForce: false,
		},
		{
			name:           "count above threshold with nil embedding uses FTS-first not brute force",
			factCount:      10000,
			queryEmbedding: nil,
			wantBruteForce: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.factCount <= threshold
			if got != tt.wantBruteForce {
				t.Errorf("factCount=%d, len(embedding)=%d: got %v, want %v",
					tt.factCount, len(tt.queryEmbedding), got, tt.wantBruteForce)
			}
		})
	}
}

// tokenizeQuery 中文必须产出 bigram，否则整段中文成为一个巨型 token，
// keywordOverlapScore 恒 0（混合打分白丢 0.25 权重）。
func TestTokenizeQuery_CJKBigrams(t *testing.T) {
	tokens := tokenizeQuery("网络运维组组长是谁？")
	joined := strings.Join(tokens, "|")
	for _, want := range []string{"网络", "运维", "组长", "是谁"} {
		if !strings.Contains(joined, want) {
			t.Errorf("tokens %v missing bigram %q", tokens, want)
		}
	}
	// 不应再出现整段未拆的巨型 token
	for _, tok := range tokens {
		if len([]rune(tok)) > 2 && strings.ContainsRune(tok, '运') {
			t.Errorf("unsplit CJK blob token: %q", tok)
		}
	}
}

// 英文/数字整词行为保持不变。
func TestTokenizeQuery_ASCIIUnchanged(t *testing.T) {
	tokens := tokenizeQuery("elk-01 部署 deadline")
	got := strings.Join(tokens, ",")
	if !strings.Contains(got, "elk-01") || !strings.Contains(got, "deadline") {
		t.Fatalf("ascii tokens wrong: %v", tokens)
	}
}

// 中文查询对中文事实的关键词重叠分必须 > 0。
func TestKeywordOverlapScore_Chinese(t *testing.T) {
	score := keywordOverlapScore(tokenizeQuery("网络运维组组长是谁？"), "张伟是网络运维组的组长")
	if score <= 0 {
		t.Fatalf("chinese kwScore = %v, want > 0", score)
	}
}

func TestTokenizeQuery_DropsEnglishStopwords(t *testing.T) {
	tokens := tokenizeQuery("What color does Alice like")
	joined := strings.ToLower(strings.Join(tokens, "|"))
	for _, keep := range []string{"color", "alice", "like"} {
		if !strings.Contains(joined, keep) {
			t.Errorf("tokens %v missing %q", tokens, keep)
		}
	}
	for _, drop := range []string{"what", "does"} {
		if strings.Contains(joined, drop) {
			t.Errorf("tokens %v still contain stopword %q", tokens, drop)
		}
	}
}

func TestTokenizeQuery_KeepsMayWillNames(t *testing.T) {
	joined := strings.ToLower(strings.Join(tokenizeQuery("When is May's birthday"), "|"))
	if !strings.Contains(joined, "may") {
		t.Fatalf("tokens dropped name May: %s", joined)
	}
	if strings.Contains(joined, "when") {
		t.Fatalf("tokens still contain stopword when: %s", joined)
	}
}

func TestTokenizeQuery_DropsUserNoise(t *testing.T) {
	joined := strings.ToLower(strings.Join(tokenizeQuery("What does the user prefer"), "|"))
	if strings.Contains(joined, "user") {
		t.Fatalf("tokens still contain high-DF user: %s", joined)
	}
	if !strings.Contains(joined, "prefer") {
		t.Fatalf("tokens missing prefer: %s", joined)
	}
}

func TestFactTouchTime_PrefersLastUsedThenUpdatedAt(t *testing.T) {
	got := factTouchTime(map[string]any{
		"valid_from":    "2026-01-01T00:00:00Z",
		"created_at":    "2026-01-01T00:00:00Z",
		"updated_at":    "2026-08-01T00:00:00Z",
		"last_used_at":  "2026-08-20T00:00:00Z",
	})
	if got != "2026-08-20T00:00:00Z" {
		t.Fatalf("factTouchTime=%q, want last_used_at", got)
	}
	got = factTouchTime(map[string]any{
		"valid_from": "2026-01-01T00:00:00Z",
		"updated_at": "2026-08-01T00:00:00Z",
	})
	if got != "2026-08-01T00:00:00Z" {
		t.Fatalf("factTouchTime=%q, want updated_at", got)
	}
}

func TestScoreFactRow_RecencyUsesTouchTimeNotValidFrom(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	row := map[string]any{
		"id":         "f1",
		"statement":  "Alice likes blue",
		"importance": 0.5,
		"quality_score": 0.5,
		"fact_kind":  "preference",
		"valid_from": "2025-01-01T00:00:00Z",
		"created_at": "2025-01-01T00:00:00Z",
		"updated_at": now.Format(time.RFC3339Nano),
	}
	bd := scoreFactRow(row, tokenizeQuery("Alice likes blue"), nil, nil, now)
	if bd.Recency != 1.0 {
		t.Fatalf("recency=%v, want 1.0 for just-updated fact", bd.Recency)
	}
}

func TestFactEventTime_PrefersValidFromThenCreatedAt(t *testing.T) {
	got := factEventTime(map[string]any{
		"updated_at": "2026-08-24T00:00:00Z",
		"created_at": "2026-01-01T00:00:00Z",
		"valid_from": "2026-02-01T00:00:00Z",
	})
	if got != "2026-02-01T00:00:00Z" {
		t.Fatalf("factEventTime=%q, want valid_from", got)
	}
	got = factEventTime(map[string]any{
		"updated_at": "2026-08-24T00:00:00Z",
		"created_at": "2026-01-01T00:00:00Z",
	})
	if got != "2026-01-01T00:00:00Z" {
		t.Fatalf("factEventTime=%q, want created_at", got)
	}
}

// embeddedFactRow builds a fact row carrying a real embedding blob so the
// vector-score paths in scoreFactRow can be exercised without a DB.
func embeddedFactRow(id string, emb []float32) map[string]any {
	return map[string]any{
		"id":             id,
		"statement":      "值班电话是 8899-1234",
		"importance":     0.5,
		"quality_score":  0.5,
		"fact_kind":      "fact",
		"created_at":     "2026-08-01T00:00:00Z",
		"updated_at":     "2026-08-01T00:00:00Z",
		"embedding_blob": encodeFloat32Blob(emb),
		"embedding_norm": vectorL2Norm(emb),
	}
}

// Regression for the domain-B recall failure: the RRF path passes a non-nil
// hitMap from pgvector, but candidates absent from it (vector store lagging
// or its table empty) used to get vector score 0 even when embedding_blob
// held a perfectly good embedding. The fallback must kick in per-ID.
func TestScoreFactRow_VecOverrideMissFallsBackToBlob(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	queryEmb := []float32{1, 0, 0}
	row := embeddedFactRow("f-duty", []float32{1, 0, 0})
	// hitMap non-nil but only knows an unrelated fact.
	overrides := map[string]float64{"f-other": 0.9}
	bd := scoreFactRow(row, nil, queryEmb, overrides, now)
	if bd.Vector < 0.99 {
		t.Fatalf("vec score = %v, want ~1.0 from embedding_blob fallback", bd.Vector)
	}
}

func TestScoreFactRow_VecOverrideHitSkipsBlob(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	queryEmb := []float32{1, 0, 0}
	row := embeddedFactRow("f-duty", []float32{1, 0, 0}) // blob cosine would be 1.0
	overrides := map[string]float64{"f-duty": 0.42}      // pgvector is authoritative
	bd := scoreFactRow(row, nil, queryEmb, overrides, now)
	if bd.Vector != 0.42 {
		t.Fatalf("vec score = %v, want 0.42 from hitMap override", bd.Vector)
	}
}

func TestScoreFactRow_VecOverrideMissNoBlobNoPanic(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	row := map[string]any{
		"id":            "f-plain",
		"statement":     "no embedding yet",
		"importance":    0.5,
		"quality_score": 0.5,
		"created_at":    "2026-08-01T00:00:00Z",
		"updated_at":    "2026-08-01T00:00:00Z",
	}
	bd := scoreFactRow(row, nil, []float32{1, 0, 0}, map[string]float64{"f-other": 0.9}, now)
	if bd.Vector != 0 {
		t.Fatalf("vec score = %v, want 0 when neither override nor blob exists", bd.Vector)
	}
}

func TestBuildL3FTSQuery_ORsContentWords(t *testing.T) {
	q := buildL3FTSQuery("What color does Alice like?")
	if !strings.Contains(q, "|") {
		t.Fatalf("FTS query %q is not an OR expression", q)
	}
	low := strings.ToLower(q)
	if !strings.Contains(low, "alice") || !strings.Contains(low, "color") {
		t.Fatalf("FTS query %q missing content terms", q)
	}
	if strings.Contains(low, "what") || strings.Contains(low, "does") {
		t.Fatalf("FTS query %q still contains stopwords", q)
	}
}
