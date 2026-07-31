package biz

import "testing"

func TestCrossEncoderReranker_Score(t *testing.T) {
	ce := NewCrossEncoderReranker()
	high := ce.Score("deploy kubernetes cluster", "how to deploy kubernetes cluster safely")
	low := ce.Score("deploy kubernetes cluster", "favorite pizza toppings")
	if high <= low {
		t.Fatalf("high=%v low=%v", high, low)
	}
}

// TestCrossEncoderReranker_SingleTokenQuery guards the single-token blindspot:
// a one-word query can never intersect the passage's word-pair bigrams, so the
// scorer must fall back to unigram Jaccard. A systematic false 0 here scales
// every recall Total by 0.85 and drops borderline hits below the minScore
// threshold (Bug A chain).
func TestCrossEncoderReranker_SingleTokenQuery(t *testing.T) {
	ce := NewCrossEncoderReranker()
	hit := ce.Score("coffee", "user likes coffee a lot")
	if hit <= 0 {
		t.Fatalf("single-token containment score=%v, want > 0", hit)
	}
	miss := ce.Score("coffee", "user enjoys tea")
	if miss != 0 {
		t.Fatalf("single-token miss score=%v, want 0", miss)
	}
}
