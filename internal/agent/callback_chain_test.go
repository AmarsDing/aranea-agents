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
	opts, _ := buildCallbackChainOptions(context.Background(), ag, TRPCBuilderDeps{}, nil, nil, nil, nil)
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

// S2 (2026-08-18): ReplyReminderEnabled=false must exclude both reminder hooks.
func TestCallbackChain_ReplyReminderDisabled(t *testing.T) {
	ag := biz.Agent{
		AgentKey: "test",
		Settings: &biz.AgentRuntimeSettings{
			ToolsEnabled:          true,
			ReplyReminderEnabled:  false,
			IntentPassEnabled:     false,
			ClarificationEnabled:  false,
			MemoryEnabled:         false,
			ContextCompactionEnabled: false,
		},
	}
	chain := productCallbackChain(context.Background(), ag, TRPCBuilderDeps{}, nil)
	if chain == nil {
		t.Fatal("expected chain")
	}
	// BeforeModel hook should not inject reminder cue.
	mc := chain.AdaptModelCallbacks()
	inv := &trpcagent.Invocation{}
	inv.SetState(replyReminderStateKey, true)
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	msgs := []trpcmodel.Message{
		trpcmodel.NewSystemMessage("S"),
		trpcmodel.NewUserMessage("U"),
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	_, err := mc.RunBeforeModel(ctx, args)
	if err != nil {
		t.Fatalf("before model: %v", err)
	}
	for _, m := range args.Request.Messages {
		if m.Content == replyReminderCue {
			t.Fatal("reply reminder cue must not be injected when disabled")
		}
	}
}

// S2: default (enabled) must still inject the reminder.
func TestCallbackChain_ReplyReminderEnabledByDefault(t *testing.T) {
	ag := biz.Agent{
		AgentKey: "test",
		Settings: &biz.AgentRuntimeSettings{
			ToolsEnabled:  true,
			ReplyReminderEnabled: true,
			IntentPassEnabled:    false,
			ClarificationEnabled: false,
			MemoryEnabled:        false,
			ContextCompactionEnabled: false,
		},
	}
	chain := productCallbackChain(context.Background(), ag, TRPCBuilderDeps{}, nil)
	if chain == nil {
		t.Fatal("expected chain")
	}
	mc := chain.AdaptModelCallbacks()
	inv := &trpcagent.Invocation{}
	inv.SetState(replyReminderStateKey, true)
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	msgs := []trpcmodel.Message{
		trpcmodel.NewSystemMessage("S"),
		trpcmodel.NewUserMessage("U"),
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	_, err := mc.RunBeforeModel(ctx, args)
	if err != nil {
		t.Fatalf("before model: %v", err)
	}
	found := false
	for _, m := range args.Request.Messages {
		if m.Content == replyReminderCueMarker+replyReminderCue {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("reply reminder cue expected when enabled")
	}
}
