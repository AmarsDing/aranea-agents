package agent

import (
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
)

func TestBizAgentRegistryOptions(t *testing.T) {
	a := trpcllmagent.New("agent-a")
	opts := BizAgentRegistryOptions(map[string]trpcagent.Agent{"agent-a": a})
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
	opts = BizAgentRegistryOptions(nil)
	if len(opts) != 0 {
		t.Fatalf("expected no options")
	}
}
