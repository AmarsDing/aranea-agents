package agent

import (
	"testing"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestAccumulateStreamUsage_memberByAgentKey(t *testing.T) {
	var result EventStreamResult
	meta := ProjectMeta{
		TeamID:          "team-1",
		MemberAgentKeys: map[string]struct{}{"worker-a": {}, "worker-b": {}},
	}
	ev := &trpcevent.Event{
		Author: "worker-b",
		Response: &trpcmodel.Response{
			Usage: &trpcmodel.Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
			},
		},
	}
	accumulateStreamUsage(&result, ev, meta, 100, 50)
	if result.PromptTok != 100 || result.CompletionTok != 50 {
		t.Fatalf("aggregate tokens: in=%d out=%d", result.PromptTok, result.CompletionTok)
	}
	u, ok := result.MemberUsage["worker-b"]
	if !ok || u.PromptTokens != 100 || u.CompletionTokens != 50 {
		t.Fatalf("member usage: %+v ok=%v", result.MemberUsage, ok)
	}
}

func TestAccumulateStreamUsage_skipsTeamRootAuthor(t *testing.T) {
	var result EventStreamResult
	meta := ProjectMeta{TeamID: "team-1", MemberAgentKeys: map[string]struct{}{"worker-a": {}}}
	ev := &trpcevent.Event{
		Author: "team-parallel",
		Response: &trpcmodel.Response{
			Usage: &trpcmodel.Usage{PromptTokens: 10, CompletionTokens: 5},
		},
	}
	accumulateStreamUsage(&result, ev, meta, 10, 5)
	if len(result.MemberUsage) != 0 {
		t.Fatalf("expected no member usage for team root author, got %+v", result.MemberUsage)
	}
}
