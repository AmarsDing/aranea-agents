//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package main

import (
	"context"
	"math"
	"testing"
)

func TestEvaluateQMSumMetricsExactMatch(t *testing.T) {
	t.Parallel()

	got := evaluateQMSumMetrics(
		context.Background(),
		nil,
		"What was decided?",
		"They approved the budget.",
		"They approved the budget.",
	)

	if !almostEqual(got.F1, 1) {
		t.Fatalf("F1 = %v, want 1", got.F1)
	}
	if !almostEqual(got.BLEU, 1) {
		t.Fatalf("BLEU = %v, want 1", got.BLEU)
	}
	if !almostEqual(got.ROUGE1, 1) || !almostEqual(got.ROUGE2, 1) || !almostEqual(got.ROUGEL, 1) {
		t.Fatalf("unexpected ROUGE values: %+v", got)
	}
}

func TestEvaluateQMSumMetricsPartialMatch(t *testing.T) {
	t.Parallel()

	got := evaluateQMSumMetrics(
		context.Background(),
		nil,
		"What was decided?",
		"They approved the annual budget and delayed hiring.",
		"They approved the budget.",
	)

	if got.F1 <= 0 || got.F1 >= 1 {
		t.Fatalf("expected partial F1 in (0,1), got %v", got.F1)
	}
	if got.ROUGEL <= 0 || got.ROUGEL >= 1 {
		t.Fatalf("expected partial ROUGE-L in (0,1), got %v", got.ROUGEL)
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
