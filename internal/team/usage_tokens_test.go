package team

import (
	"testing"

	"aranea-agents/internal/agent"
)

func TestStepTokensForMember_prefersStreamMap(t *testing.T) {
	result := agent.EventStreamResult{
		MemberUsage: map[string]agent.MemberTokenUsage{
			"worker-b": {PromptTokens: 40, CompletionTokens: 20},
		},
	}
	in, out := stepTokensForMember("worker-b", 1, result, 100, 50)
	if in != 40 || out != 20 {
		t.Fatalf("got in=%d out=%d", in, out)
	}
}

func TestStepTokensForMember_anchorFallback(t *testing.T) {
	in, out := stepTokensForMember("anchor", 0, agent.EventStreamResult{}, 100, 50)
	if in != 100 || out != 50 {
		t.Fatalf("got in=%d out=%d", in, out)
	}
}
