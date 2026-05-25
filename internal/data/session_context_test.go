package data

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
)

func TestComputeSessionContextFromUsage(t *testing.T) {
	cur := biz.Session{ContextUsedRatio: 0.2, MaxContextUsedRatio: 0.35, ContextUsedTokens: 10_000}

	ratio := cur.ContextUsedRatio
	promptTokens := 64_000
	contextWindow := 128_000
	if contextWindow > 0 && promptTokens > 0 {
		ratio = llmcontext.ContextRatio(promptTokens, contextWindow)
	}
	maxR := cur.MaxContextUsedRatio
	if ratio > maxR {
		maxR = ratio
	}
	status := llmcontext.ContextStatusForRatio(ratio)

	if ratio != 0.5 {
		t.Fatalf("ratio got %v want 0.5", ratio)
	}
	if maxR != 0.5 {
		t.Fatalf("maxR got %v want 0.5", maxR)
	}
	if status != "normal" {
		t.Fatalf("status got %q want normal", status)
	}
}

func TestComputeSessionContextAfterCompression(t *testing.T) {
	tok := 28_000
	contextWindow := 128_000
	ratio := llmcontext.ContextRatio(tok, contextWindow)
	status := llmcontext.ContextStatusForRatio(ratio)
	if ratio <= 0.218 || ratio >= 0.219 {
		t.Fatalf("ratio got %v want ~0.21875", ratio)
	}
	if status != "normal" {
		t.Fatalf("status got %q want normal", status)
	}
}
