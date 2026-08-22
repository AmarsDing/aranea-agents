package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/deferred"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func TestOrchestrationBriefBeforeHook_InjectsReadyCue(t *testing.T) {
	t.Parallel()
	hook, ok := newOrchestrationBriefBeforeHook().(callbacks.BeforeModelHook)
	if !ok {
		t.Fatal("expected BeforeModelHook")
	}
	brief := biz.FormatOrchestrationBrief(biz.SpiritPhaseReady, []biz.Team{
		{ID: "t1", DisplayName: "核实金鹏科技", Status: biz.TeamStatusCompleted},
	})
	ctx := biz.WithSpiritTurnOrchestration(context.Background(), biz.SpiritTurnOrchestration{
		Phase: biz.SpiritPhaseReady,
		Brief: brief,
	})
	req := &trpcmodel.Request{Messages: []trpcmodel.Message{trpcmodel.NewUserMessage("组建几个团队，分析金鹏科技行情")}}
	if _, err := hook.HandleBeforeModel(ctx, &trpcmodel.BeforeModelArgs{Request: req}); err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(req.Messages))
	}
	if !strings.Contains(req.Messages[1].Content, "phase: ready") || !strings.Contains(req.Messages[1].Content, "get_team_deliverable") {
		t.Fatalf("brief not injected: %s", req.Messages[1].Content)
	}
	if _, err := hook.HandleBeforeModel(ctx, &trpcmodel.BeforeModelArgs{Request: req}); err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 2 {
		t.Fatal("brief must be idempotent")
	}
}

func TestOrchestrationPhasePromoteBeforeHook_ActivatesReadyTools(t *testing.T) {
	t.Parallel()
	dm := deferred.NewDeferredToolManager([]deferred.DeferredToolEntry{
		{Name: "get_team_deliverable", BaseName: "get_team_deliverable", Description: "read deliverable"},
		{Name: "synthesize_results", BaseName: "synthesize_results", Description: "synthesize"},
	})
	noop := func(_ context.Context, _ struct{}) (struct{}, error) { return struct{}{}, nil }
	dm.RegisterTool("get_team_deliverable", trpcfunction.NewFunctionTool(noop, trpcfunction.WithName("get_team_deliverable")))
	dm.RegisterTool("synthesize_results", trpcfunction.NewFunctionTool(noop, trpcfunction.WithName("synthesize_results")))

	hook, ok := newOrchestrationPhasePromoteBeforeHook(dm, nil).(callbacks.BeforeAgentHook)
	if !ok {
		t.Fatal("expected BeforeAgentHook")
	}
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "spirit-s1"}}
	ctx := biz.WithSpiritTurnOrchestration(
		trpcagent.NewInvocationContext(context.Background(), inv),
		biz.SpiritTurnOrchestration{Phase: biz.SpiritPhaseReady},
	)
	if _, err := hook.HandleBeforeAgent(ctx, &trpcagent.BeforeAgentArgs{}); err != nil {
		t.Fatal(err)
	}
	if !dm.IsActivated(ctx, "get_team_deliverable") || !dm.IsActivated(ctx, "synthesize_results") {
		t.Fatal("ready phase must activate closeout tools")
	}
}

func TestOrchestrationPhasePromoteBeforeHook_IdleSkips(t *testing.T) {
	t.Parallel()
	dm := deferred.NewDeferredToolManager([]deferred.DeferredToolEntry{
		{Name: "get_team_deliverable", BaseName: "get_team_deliverable", Description: "read deliverable"},
	})
	hook, ok := newOrchestrationPhasePromoteBeforeHook(dm, nil).(callbacks.BeforeAgentHook)
	if !ok {
		t.Fatal("expected BeforeAgentHook")
	}
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: "spirit-s1"}}
	ctx := biz.WithSpiritTurnOrchestration(
		trpcagent.NewInvocationContext(context.Background(), inv),
		biz.SpiritTurnOrchestration{Phase: biz.SpiritPhaseIdle},
	)
	if _, err := hook.HandleBeforeAgent(ctx, &trpcagent.BeforeAgentArgs{}); err != nil {
		t.Fatal(err)
	}
	if dm.IsActivated(ctx, "get_team_deliverable") {
		t.Fatal("idle must not activate closeout tools")
	}
}
