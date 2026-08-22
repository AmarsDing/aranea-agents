package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/tools/deferred"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func TestParseIntentArtifactFromUserText(t *testing.T) {
	t.Parallel()
	text := "Original user message:\nedit the file\n\n---\nDerived intent (align your plan and tools to this JSON):\n" +
		`{"refined_goal":"edit file","intent_kind":"code_change","tool_hints":["diff_edit","search_content"]}`
	art := parseIntentArtifactFromUserText(text)
	if art == nil || art.RefinedGoal != "edit file" {
		t.Fatalf("artifact = %+v", art)
	}
	if len(art.ToolHints) != 2 || art.ToolHints[0] != "diff_edit" {
		t.Fatalf("tool_hints = %v", art.ToolHints)
	}
}

func TestIntentToolHintPromoteActivatesCatalogTool(t *testing.T) {
	t.Parallel()
	inner := deferred.NewDeferredToolManager([]deferred.DeferredToolEntry{
		{Name: "diff_edit", BaseName: "diff_edit", Description: "Apply a patch", Category: "file"},
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch URL", Category: "web"},
	})
	noop := func(_ context.Context, _ struct{}) (struct{}, error) { return struct{}{}, nil }
	inner.RegisterTool("diff_edit", trpcfunction.NewFunctionTool(noop, trpcfunction.WithName("diff_edit")))
	hook, ok := newIntentToolHintPromoteBeforeHook(inner, nil).(callbacks.BeforeAgentHook)
	if !ok {
		t.Fatal("expected BeforeAgentHook")
	}
	sess := &trpcsession.Session{ID: "s1"}
	inv := &trpcagent.Invocation{
		Session: sess,
		Message: trpcmodel.NewUserMessage(`Original user message:
patch it

---
Derived intent (align your plan and tools to this JSON):
{"refined_goal":"patch the module","tool_hints":["diff_edit"]}`),
	}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	if _, err := hook.HandleBeforeAgent(ctx, &trpcagent.BeforeAgentArgs{Invocation: inv}); err != nil {
		t.Fatal(err)
	}
	if !inner.IsActivated(ctx, "diff_edit") {
		t.Fatal("diff_edit must be pre-activated from tool_hints")
	}
}
