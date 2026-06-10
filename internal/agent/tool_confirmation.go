package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/metrics"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const errToolConfirmationRequired = "TOOL_CONFIRMATION_REQUIRED"

type toolCallStartKey struct{}

type toolConfirmationBeforeHook struct {
	gate *toolConfirmGate
	ag   biz.Agent
	deps TRPCBuilderDeps
}

var _ callbacks.BeforeToolHook = (*toolConfirmationBeforeHook)(nil)

func newToolConfirmationBeforeHook(gate *toolConfirmGate, ag biz.Agent, deps TRPCBuilderDeps) *toolConfirmationBeforeHook {
	return &toolConfirmationBeforeHook{gate: gate, ag: ag, deps: deps}
}

func (h *toolConfirmationBeforeHook) Point() callbacks.CallbackPoint { return callbacks.PointBeforeTool }
func (h *toolConfirmationBeforeHook) Priority() int                    { return 10 }

func (h *toolConfirmationBeforeHook) HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil || h.gate == nil {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	toolKey := strings.TrimSpace(args.ToolName)
	if toolKey == "" || toolConfirmationBypass() {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	if h.gate.pluginAllowWithoutChannel(toolKey, args.Arguments) {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	if !h.gate.needsConfirm(toolKey, args.Arguments) {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}

	if fn := serviceawaitreply.ReplyFuncFromContext(ctx); fn != nil {
		if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
			_ = trpcagent.MarkAwaitingUserReply(inv)
		}
		confirmCtx := serviceawaitreply.WithToolConfirmRequest(ctx, serviceawaitreply.ToolConfirmRequest{
			ToolKey:    toolKey,
			ToolCallID: args.ToolCallID,
		})
		reply, err := fn(confirmCtx)
		if err != nil {
			return nil, fmt.Errorf("%s: awaiting user confirmation failed: %w", errToolConfirmationRequired, err)
		}
		if toolConfirmApproved(reply) {
			metrics.PluginInvokeTotal.WithLabelValues("confirm_gate", "before_tool", "success").Inc()
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		recordToolInvocationWrite(ctx, biz.ToolInvocationWrite{
			ToolKey:      toolKey,
			AgentID:      h.ag.ID,
			Status:       "blocked",
			ErrorCode:    event.ErrorCodeConfirmationDenied,
			ErrorMessage: "user denied tool confirmation",
			InputPreview: previewFromToolArgs(args.Arguments),
			StartedAt:    time.Now().UTC().Format(time.RFC3339),
			EndedAt:      time.Now().UTC().Format(time.RFC3339),
			Source:       biz.ToolInvocationSourceRuntime,
			ToolCallID:   args.ToolCallID,
		}, nil, h.ag, h.deps)
		return nil, fmt.Errorf("%s: tool %s confirmation denied", errToolConfirmationRequired, toolKey)
	}

	recordToolInvocationWrite(ctx, biz.ToolInvocationWrite{
		ToolKey:      toolKey,
		AgentID:      h.ag.ID,
		Status:       "blocked",
		ErrorCode:    event.ErrorCodeConfirmationRequired,
		ErrorMessage: "tool requires user confirmation before execution",
		InputPreview: previewFromToolArgs(args.Arguments),
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		EndedAt:      time.Now().UTC().Format(time.RFC3339),
		Source:       biz.ToolInvocationSourceRuntime,
		ToolCallID:   args.ToolCallID,
	}, nil, h.ag, h.deps)
	return nil, fmt.Errorf("%s: tool %s requires user confirmation", errToolConfirmationRequired, toolKey)
}

func toolConfirmationBypass() bool {
	return strings.TrimSpace(os.Getenv("KRATOS_TOOL_AUTO_APPROVE")) == "1"
}
