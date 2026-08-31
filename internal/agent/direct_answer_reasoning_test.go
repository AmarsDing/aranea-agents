package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestDirectAnswerReasoningBudget_CapsSpiritDirect(t *testing.T) {
	hook := newDirectAnswerReasoningBudgetBeforeHook(biz.Agent{AgentKey: biz.SpiritAgentKey})
	if hook == nil {
		t.Fatal("Spirit must install reasoning budget hook")
	}
	fn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{
		Messages: []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "你好"}},
	}}
	if _, err := fn.HandleBeforeModel(context.Background(), args); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if args.Request.ReasoningEffort == nil || *args.Request.ReasoningEffort != directAnswerReasoningEffort {
		t.Fatalf("ReasoningEffort=%v, want %q", args.Request.ReasoningEffort, directAnswerReasoningEffort)
	}
	if args.Request.ThinkingTokens == nil || *args.Request.ThinkingTokens != directAnswerThinkingTokens {
		t.Fatalf("ThinkingTokens=%v, want %d", args.Request.ThinkingTokens, directAnswerThinkingTokens)
	}
}

func TestDirectAnswerReasoningBudget_SkipsToolLoop(t *testing.T) {
	hook := newDirectAnswerReasoningBudgetBeforeHook(biz.Agent{AgentKey: biz.SpiritAgentKey})
	fn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, ToolCalls: []trpcmodel.ToolCall{{ID: "1"}}},
			{Role: trpcmodel.RoleTool, ToolName: "memory_search", Content: "{}"},
		},
	}}
	if _, err := fn.HandleBeforeModel(context.Background(), args); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if args.Request.ReasoningEffort != nil || args.Request.ThinkingTokens != nil {
		t.Fatal("tool-loop turns must keep provider default thinking")
	}
}

func TestDirectAnswerReasoningBudget_SkipsForcePlanning(t *testing.T) {
	hook := newDirectAnswerReasoningBudgetBeforeHook(biz.Agent{AgentKey: biz.SpiritAgentKey})
	fn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	ctx := ContextWithForcePlanningRoute(context.Background(), ForcePlanningRoute{
		TaskPrompt: "排查全网故障",
		Mode:       "dag",
	})
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{
		Messages: []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "排查全网故障"}},
	}}
	if _, err := fn.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if args.Request.ThinkingTokens != nil {
		t.Fatal("planning turns must not cap thinking tokens")
	}
}

func TestDirectAnswerReasoningBudget_NilForSpecialist(t *testing.T) {
	if hook := newDirectAnswerReasoningBudgetBeforeHook(biz.Agent{AgentKey: "ops_fault_diagnosis"}); hook != nil {
		t.Fatal("specialists must not cap direct-answer thinking")
	}
}
