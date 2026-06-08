package data

import (
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
