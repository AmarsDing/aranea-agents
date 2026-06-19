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
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const errToolConfirmationRequired = "TOOL_CONFIRMATION_REQUIRED"

// defaultToolConfirmationTimeout is the maximum duration to wait for a user
// to approve or deny a tool confirmation request. After this deadline the
// tool invocation is rejected with ErrorCodeConfirmationTimeout.
const defaultToolConfirmationTimeout = 5 * time.Minute

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
			if markErr := trpcagent.MarkAwaitingUserReply(inv); markErr != nil {
				// Non-fatal: the confirmation flow can still proceed, but
				// the UI may not show the "awaiting reply" indicator.
				h.deps.Logger().Warn("MarkAwaitingUserReply failed",
					loggateway.StepID("agent.tool_confirm"),
					loggateway.Err(markErr))
			}
		}
		// N-21: Emit a confirm Activity so the frontend can render a
		// confirmation card in the activity timeline.
		emitter := biz.ActivityEmitterFromContext(ctx)
		var confirmActivityID string
		if emitter != nil {
			confirmContent := fmt.Sprintf("工具 %s 需要确认后执行", toolKey)
			id, emitErr := emitter.EmitConfirmRequest(ctx, biz.ActivityConfirmParams{
				ToolName:      toolKey,
				ToolArguments: string(args.Arguments),
				Content:       confirmContent,
			})
			if emitErr != nil {
				h.deps.Logger().Warn("EmitConfirmRequest failed",
					loggateway.StepID("agent.tool_confirm"),
					loggateway.Err(emitErr))
			} else {
				confirmActivityID = id
			}
		}
		confirmCtx := serviceawaitreply.WithToolConfirmRequest(ctx, serviceawaitreply.ToolConfirmRequest{
			ToolKey:    toolKey,
			ToolCallID: args.ToolCallID,
		})
		// Apply confirmation timeout to prevent indefinite blocking.
		confirmCtx, confirmCancel := context.WithTimeout(confirmCtx, defaultToolConfirmationTimeout)
		defer confirmCancel()
		reply, err := fn(confirmCtx)
		// N-21: Update the confirm Activity with the user's response.
		if emitter != nil && confirmActivityID != "" {
			approved := err == nil && toolConfirmApproved(reply)
			if emitErr := emitter.EmitConfirmResult(ctx, confirmActivityID, approved); emitErr != nil {
				h.deps.Logger().Warn("EmitConfirmResult failed",
					loggateway.StepID("agent.tool_confirm"),
					loggateway.Err(emitErr))
			}
		}
		if err != nil {
			// Distinguish between confirmation timeout and other errors.
			// Only report ErrorCodeConfirmationTimeout when the confirmation
			// deadline itself expired but the parent context is still alive.
			// If the parent context (e.g., tool execution timeout) also expired,
			// the error is not a confirmation timeout — it will be handled by
			// the execution timeout logic upstream.
			if confirmCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
				recordToolInvocationWrite(ctx, biz.ToolInvocationWrite{
					ToolKey:      toolKey,
					AgentID:      h.ag.ID,
					Status:       "blocked",
					ErrorCode:    event.ErrorCodeConfirmationTimeout,
					ErrorMessage: fmt.Sprintf("tool confirmation timed out after %s", defaultToolConfirmationTimeout),
					InputPreview: previewFromToolArgs(args.Arguments),
					StartedAt:    time.Now().UTC().Format(time.RFC3339),
					EndedAt:      time.Now().UTC().Format(time.RFC3339),
					Source:       biz.ToolInvocationSourceRuntime,
					ToolCallID:   args.ToolCallID,
				}, nil, h.ag, h.deps)
				return nil, fmt.Errorf("%s: tool %s confirmation timed out after %s", errToolConfirmationRequired, toolKey, defaultToolConfirmationTimeout)
			}
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

// toolConfirmationBypass reports whether tool confirmation should be
// skipped entirely. This is a development-only escape hatch.
//
// Security model: the bypass requires ARANEA_DEV_MODE to be set (only
// in local/docker-compose dev environments). In production, where
// ARANEA_DEV_MODE is never set, the bypass is impossible regardless
// of any other environment variable.
//
// The legacy KRATOS_TOOL_AUTO_APPROVE env var is also gated by
// ARANEA_DEV_MODE for backward compatibility.
func toolConfirmationBypass() bool {
	if strings.TrimSpace(os.Getenv("ARANEA_DEV_MODE")) == "" {
		return false
	}
	// Both the legacy and new env vars are accepted when in dev mode.
	if strings.TrimSpace(os.Getenv("KRATOS_TOOL_AUTO_APPROVE")) == "1" {
		return true
	}
	return strings.TrimSpace(os.Getenv("ARANEA_TOOL_AUTO_APPROVE")) == "1"
}
