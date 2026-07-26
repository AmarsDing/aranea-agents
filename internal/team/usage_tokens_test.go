package team

import (
	"testing"

	"aranea-agents/internal/agent"
)

func TestStepTokensForMember_prefersStreamMap(t *testing.T) {
	result := agent.EventStreamResult{
		MemberUsage: map[string]agent.MemberTokenUsage{
			"worker-b": {PromptTokens: 40, CompletionTokens: 20, CachedTokens: 15},
		},
	}
	in, out, cached := stepTokensForMember("worker-b", 1, result, 100, 50, 30)
	if in != 40 || out != 20 {
		t.Fatalf("got in=%d out=%d", in, out)
	}
	if cached != 15 {
		t.Fatalf("got cached=%d, want 15", cached)
	}
}

func TestStepTokensForMember_anchorFallback(t *testing.T) {
	in, out, cached := stepTokensForMember("anchor", 0, agent.EventStreamResult{}, 100, 50, 30)
	if in != 100 || out != 50 {
		t.Fatalf("got in=%d out=%d", in, out)
	}
	if cached != 30 {
		t.Fatalf("got cached=%d, want 30 (anchor fallback)", cached)
	}
}

func TestStepTokensForMember_nonAnchorMiss(t *testing.T) {
	in, out, cached := stepTokensForMember("worker-c", 2, agent.EventStreamResult{}, 100, 50, 30)
	if in != 0 || out != 0 || cached != 0 {
		t.Fatalf("got in=%d out=%d cached=%d, want all zero", in, out, cached)
	}
}
