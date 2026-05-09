package service

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestMergeSessionSummariesMarkdown_joinsChunks(t *testing.T) {
	s := mergeSessionSummariesMarkdown([]biz.SessionSummary{
		{SummaryMarkdown: "A"},
		{SummaryMarkdown: "B"},
	})
	if s != "A\n\n---\n\nB" {
		t.Fatalf("unexpected merged output %q", s)
	}
}

func TestTimelineUserAssistant_filtersRoles(t *testing.T) {
	out := timelineUserAssistant([]biz.ChatMessage{
		{Role: "system", ContentMarkdown: "x"},
		{Role: "user", ContentMarkdown: "hi"},
		{Role: "assistant", ContentMarkdown: ""},
		{Role: "assistant", ContentMarkdown: "there"},
	})
	if len(out) != 2 || out[0].Role != "user" || out[1].Role != "assistant" {
		t.Fatalf("got %+v", out)
	}
}

func TestAtFullContextUsage(t *testing.T) {
	if !atFullContextUsage(biz.Session{ContextUsedRatio: 1.0}) {
		t.Fatal("ratio 1 should be full")
	}
	if !atFullContextUsage(biz.Session{ContextUsedTokens: 8000, LastContextWindowTokens: 8000}) {
		t.Fatal("tokens at window should be full")
	}
	if atFullContextUsage(biz.Session{ContextUsedRatio: 0.99, ContextUsedTokens: 100, LastContextWindowTokens: 8000}) {
		t.Fatal("below window should not be full")
	}
}

func TestEstimateCompactedPromptTokens_positive(t *testing.T) {
	n := estimateCompactedPromptTokens("summary", []biz.ChatMessage{{ContentMarkdown: strings.Repeat("a", 40)}})
	if n < 1 {
		t.Fatalf("want positive estimate got %d", n)
	}
}
