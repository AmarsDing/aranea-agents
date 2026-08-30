package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/deferred"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func knowledgeIntentTestAgent(toolsEnabled bool) biz.Agent {
	return biz.Agent{
		ID:       "agent___spirit__",
		Settings: &biz.AgentRuntimeSettings{ToolsEnabled: toolsEnabled, ToolsProfile: "spirit"},
	}
}

func runKnowledgeIntentHook(t *testing.T, hook callbacks.BeforeAgentHook, inv *trpcagent.Invocation) context.Context {
	t.Helper()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	if _, err := hook.HandleBeforeAgent(ctx, &trpcagent.BeforeAgentArgs{Invocation: inv}); err != nil {
		t.Fatalf("hook error: %v", err)
	}
	return ctx
}

func TestKnowledgeIntentPromote_ActivatesRetrievalTools(t *testing.T) {
	t.Parallel()
	dm := deferred.NewDeferredToolManager([]deferred.DeferredToolEntry{
		{Name: "knowledge_search", BaseName: "knowledge_search", Description: "Search KB", Category: "knowledge"},
		{Name: "knowledge_reflect", BaseName: "knowledge_reflect", Description: "Reflect KB", Category: "knowledge"},
	})
	hook, ok := newKnowledgeIntentPromoteBeforeHook(knowledgeIntentTestAgent(true), dm, nil).(callbacks.BeforeAgentHook)
	if !ok {
		t.Fatal("expected BeforeAgentHook")
	}
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{ID: "s-kb-1"},
		Message: trpcmodel.NewUserMessage("AWOS 的 LT31 无前向散射告警但 MOR 骤降，可能是什么原因？结合知识库回答。"),
	}
	ctx := runKnowledgeIntentHook(t, hook, inv)
	if !dm.IsActivated(ctx, "knowledge_search") {
		t.Fatal("knowledge_search must be activated on explicit KB intent")
	}
	if !dm.IsActivated(ctx, "knowledge_reflect") {
		t.Fatal("knowledge_reflect must be activated on explicit KB intent")
	}
	if !knowledgeSearchOnFace(ctx, knowledgeIntentTestAgent(true), dm) {
		t.Fatal("knowledgeSearchOnFace must report true after dynamic activation")
	}
}

func TestKnowledgeIntentPromote_NonKnowledgeTurnNoop(t *testing.T) {
	t.Parallel()
	dm := deferred.NewDeferredToolManager([]deferred.DeferredToolEntry{
		{Name: "knowledge_search", BaseName: "knowledge_search", Description: "Search KB", Category: "knowledge"},
	})
	hook, ok := newKnowledgeIntentPromoteBeforeHook(knowledgeIntentTestAgent(true), dm, nil).(callbacks.BeforeAgentHook)
	if !ok {
		t.Fatal("expected BeforeAgentHook")
	}
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{ID: "s-kb-2"},
		Message: trpcmodel.NewUserMessage("今天天气怎么样"),
	}
	ctx := runKnowledgeIntentHook(t, hook, inv)
	if dm.IsActivated(ctx, "knowledge_search") {
		t.Fatal("non-knowledge turn must not activate knowledge_search")
	}
	if knowledgeSearchOnFace(ctx, knowledgeIntentTestAgent(true), dm) {
		t.Fatal("knowledgeSearchOnFace must be false for spirit without activation")
	}
}

func TestKnowledgeIntentPromote_NotInCatalogSkips(t *testing.T) {
	t.Parallel()
	// 目录为空：agent 未启用知识工具（eff 门禁装配期已挡），hook 不得报错。
	dm := deferred.NewDeferredToolManager(nil)
	hook, ok := newKnowledgeIntentPromoteBeforeHook(knowledgeIntentTestAgent(true), dm, nil).(callbacks.BeforeAgentHook)
	if !ok {
		t.Fatal("expected BeforeAgentHook")
	}
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{ID: "s-kb-3"},
		Message: trpcmodel.NewUserMessage("知识库里有 SOP 吗"),
	}
	runKnowledgeIntentHook(t, hook, inv) // 不 panic 即通过
}

func TestKnowledgeIntentPromote_ToolsDisabledReturnsNil(t *testing.T) {
	t.Parallel()
	dm := deferred.NewDeferredToolManager([]deferred.DeferredToolEntry{
		{Name: "knowledge_search", BaseName: "knowledge_search"},
	})
	if hook := newKnowledgeIntentPromoteBeforeHook(knowledgeIntentTestAgent(false), dm, nil); hook != nil {
		t.Fatal("tools disabled agent must get nil hook")
	}
	if hook := newKnowledgeIntentPromoteBeforeHook(knowledgeIntentTestAgent(true), nil, nil); hook != nil {
		t.Fatal("nil deferred manager must get nil hook")
	}
}

func TestKnowledgeSearchOnFace_StaticProfile(t *testing.T) {
	t.Parallel()
	coding := biz.Agent{
		ID:       "a1",
		Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "coding"},
	}
	if !knowledgeSearchOnFace(context.Background(), coding, nil) {
		t.Fatal("coding profile statically has knowledge_search on face")
	}
	spirit := knowledgeIntentTestAgent(true)
	if knowledgeSearchOnFace(context.Background(), spirit, nil) {
		t.Fatal("spirit profile statically excluded without dynamic activation")
	}
}
