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
