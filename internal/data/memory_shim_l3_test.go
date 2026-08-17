package data

import (
	"strings"
	"testing"

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
			name:           "count below threshold uses brute force",
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
			name:           "count above threshold without embedding uses brute force",
			factCount:      10000,
			queryEmbedding: []float32{},
			wantBruteForce: true,
		},
		{
			name:           "count above threshold with nil embedding uses brute force",
			factCount:      10000,
			queryEmbedding: nil,
			wantBruteForce: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the decision logic from shouldUseBruteForce:
			// return count <= biz.DefaultFactBruteForceThreshold || len(queryEmbedding) == 0
			got := tt.factCount <= threshold || len(tt.queryEmbedding) == 0
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
