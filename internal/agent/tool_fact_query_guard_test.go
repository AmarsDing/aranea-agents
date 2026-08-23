package agent

import (
	"context"
	"strings"
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func factQueryInvocationCtx(userText string) context.Context {
	inv := &trpcagent.Invocation{
		Message: trpcmodel.Message{Role: trpcmodel.RoleUser, Content: userText},
	}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

func TestFactQueryWebGuard_NilWhenWebResearchUnavailable(t *testing.T) {
	if hook := newFactQueryWebGuardBeforeHook(false); hook != nil {
		t.Fatal("guard must not register when web_research was pruned")
	}
}

func TestFactQueryWebGuard_BlocksWebFetchOnWeather(t *testing.T) {
	hook := newFactQueryWebGuardBeforeHook(true)
	res, err := hook.HandleBeforeTool(factQueryInvocationCtx("北京今天天气怎么样"), &trpctool.BeforeToolArgs{ToolName: "web_fetch"})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res == nil || res.CustomResult == nil {
		t.Fatal("weather fact query must block web_fetch")
	}
	guidance, _ := res.CustomResult.(string)
	if !strings.Contains(guidance, "web_research") {
		t.Fatalf("guidance=%q", guidance)
	}
}

func TestFactQueryWebGuard_AllowsWebResearch(t *testing.T) {
	hook := newFactQueryWebGuardBeforeHook(true)
	res, err := hook.HandleBeforeTool(factQueryInvocationCtx("北京今天天气怎么样"), &trpctool.BeforeToolArgs{ToolName: "web_research"})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res != nil && res.CustomResult != nil {
		t.Fatalf("web_research must stay allowed, got %+v", res.CustomResult)
	}
}

func TestFactQueryWebGuard_AllowsWebFetchOnNonFactTask(t *testing.T) {
	hook := newFactQueryWebGuardBeforeHook(true)
	res, err := hook.HandleBeforeTool(factQueryInvocationCtx("把这篇文档整理成机房巡检说明"), &trpctool.BeforeToolArgs{ToolName: "web_fetch"})
	if err != nil {
		t.Fatalf("HandleBeforeTool: %v", err)
	}
	if res != nil && res.CustomResult != nil {
		t.Fatalf("non-fact task must not block web_fetch, got %+v", res.CustomResult)
	}
}
