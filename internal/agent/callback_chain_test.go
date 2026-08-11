package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestBuildCallbackChainOptions_HasAgentAndModelHooks(t *testing.T) {
	ag := biz.Agent{
		AgentKey: "test",
		Settings: &biz.AgentRuntimeSettings{ToolsEnabled: false},
	}
	opts, _ := buildCallbackChainOptions(context.Background(), ag, TRPCBuilderDeps{}, nil)
	if len(opts) == 0 {
		t.Fatal("expected callback options")
	}
	chain := productCallbackChain(context.Background(), ag, TRPCBuilderDeps{}, nil)
	if chain == nil {
		t.Fatal("expected chain")
	}
	if !chain.HasAgentHooks() || !chain.HasModelHooks() {
		t.Fatalf("agent=%v model=%v", chain.HasAgentHooks(), chain.HasModelHooks())
	}
	ac := chain.AdaptAgentCallbacks()
	if _, err := ac.RunBeforeAgent(context.Background(), &trpcagent.BeforeAgentArgs{}); err != nil {
		t.Fatalf("before agent: %v", err)
	}
	mc := chain.AdaptModelCallbacks()
	if _, err := mc.RunBeforeModel(context.Background(), &trpcmodel.BeforeModelArgs{}); err != nil {
		t.Fatalf("before model: %v", err)
	}
}

func TestBuildCallbackChainOptions_ToolHooksWhenEnabled(t *testing.T) {
	ag := biz.Agent{
		AgentKey: "test",
		Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true},
	}
	chain := productCallbackChain(context.Background(), ag, TRPCBuilderDeps{}, nil)
	if chain == nil || !chain.HasToolHooks() {
		t.Fatal("expected tool hooks when tools enabled")
	}
}
