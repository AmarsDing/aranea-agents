package agent

import (
	"context"
	"fmt"

	"aranea-agents/internal/event"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const errToolConfirmationRequired = "TOOL_CONFIRMATION_REQUIRED"

type toolCallStartKey struct{}

// buildToolConfirmationPolicy resolves per-tool_key confirmation from catalog + agent overrides.
func buildToolConfirmationPolicy(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) map[string]bool {
	if deps.ToolUC == nil || strings.TrimSpace(ag.ID) == "" {
		return nil
	}
	eff := loadEffectiveToolKeys(ctx, deps, ag.ID)
	if len(eff) == 0 {
		return nil
	}
	overrides, err := deps.ToolUC.ListToolAgentOverridesByAgent(ctx, ag.ID)
	if err != nil {
		overrides = nil
	}
	overrideByKey := make(map[string]biz.ToolAgentOverride, len(overrides))
	for _, o := range overrides {
		overrideByKey[strings.TrimSpace(o.ToolKey)] = o
	}
	out := make(map[string]bool)
	for key, enabled := range eff {
		if !enabled {
			continue
		}
		tool, err := deps.ToolUC.GetTool(ctx, key)
		if err != nil {
			continue
		}
		ov, hasOV := overrideByKey[key]
		if biz.ToolRequiresConfirmation(tool, ov, hasOV) {
			out[key] = true
		}
	}
	return out
}

func toolConfirmationBypass() bool {
	return strings.TrimSpace(os.Getenv("KRATOS_TOOL_AUTO_APPROVE")) == "1"
}

type toolConfirmationBeforeHook struct {
	policy map[string]bool
	ag     biz.Agent
	deps   TRPCBuilderDeps
}

var _ callbacks.BeforeToolHook = (*toolConfirmationBeforeHook)(nil)

func newToolConfirmationBeforeHook(policy map[string]bool, ag biz.Agent, deps TRPCBuilderDeps) *toolConfirmationBeforeHook {
	return &toolConfirmationBeforeHook{policy: policy, ag: ag, deps: deps}
}

func (h *toolConfirmationBeforeHook) Point() callbacks.CallbackPoint { return callbacks.PointBeforeTool }
func (h *toolConfirmationBeforeHook) Priority() int                    { return 10 }

func (h *toolConfirmationBeforeHook) HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	toolKey := strings.TrimSpace(args.ToolName)
	if toolKey == "" || len(h.policy) == 0 || !h.policy[toolKey] || toolConfirmationBypass() {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	recordToolInvocationWrite(ctx, biz.ToolInvocationWrite{
		ToolKey:      toolKey,
		AgentID:      h.ag.ID,
		Status:       "blocked",
		ErrorCode:    "confirmation_required",
		ErrorMessage: "tool requires user confirmation before execution",
		InputPreview: previewFromArgs(args.Arguments),
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		EndedAt:      time.Now().UTC().Format(time.RFC3339),
		Source:       "adk",
		ToolCallID:   args.ToolCallID,
	}, h.ag, h.deps)
	return nil, fmt.Errorf("%s: tool %s requires user confirmation", errToolConfirmationRequired, toolKey)
}

func newToolCallTimingBeforeHook() callbacks.BeforeToolHook {
	return callbacks.NewBeforeToolHook(5, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		_ = args
		started := time.Now().UTC()
		ctx = context.WithValue(ctx, toolCallStartKey{}, started)
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	})
}

func invocationStatusFromAfter(args *trpctool.AfterToolArgs) (status, errCode, errMsg string) {
	if args.Error != nil {
		msg := args.Error.Error()
		if strings.Contains(msg, errToolConfirmationRequired) {
			return "blocked", "confirmation_required", truncateErr(msg)
		}
		return "error", "tool_error", truncateErr(msg)
	}
	return "success", "", ""
}

func truncateErr(msg string) string {
	if len(msg) > 500 {
		return msg[:500]
	}
	return msg
}

func recordToolInvocationWrite(ctx context.Context, write biz.ToolInvocationWrite, ag biz.Agent, deps TRPCBuilderDeps) {
	if strings.TrimSpace(write.ToolKey) == "" {
		return
	}
	if deps.ToolUC == nil && deps.Sessions == nil {
		return
	}
	var sessionID, userID, agentKey string
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
		if inv.Session != nil {
			sessionID = inv.Session.ID
			userID = inv.Session.UserID
		}
		agentKey = inv.AgentName
	}
	write.AgentID = ag.ID
	write.AgentKey = strutil.FirstNonEmpty(agentKey, ag.AgentKey)
	write.SessionID = sessionID
	write.UserID = userID
	if strings.TrimSpace(write.Source) == "" {
		write.Source = "adk"
	}
	status := strings.TrimSpace(write.Status)
	if status == "" {
		status = "success"
	}
	metrics.ToolInvocationTotal.WithLabelValues(write.ToolKey, status).Inc()

	var toolDelta, mcpDelta, skillDelta int
	countSession := strings.TrimSpace(sessionID) != "" && deps.Sessions != nil && status != "blocked"
	if countSession {
		toolDelta = 1
		mcp, skill := classifyToolInvocation(ctx, write.ToolKey, deps)
		if mcp {
			mcpDelta = 1
		}
		if skill {
			skillDelta = 1
		}
	}

	safego.Go(ctx, "recordToolInvocation", func() {
		bg := context.Background()
		if deps.ToolUC != nil {
			if err := deps.ToolUC.RecordToolInvocation(bg, write); err != nil {
				event.CtxFlowLogWarn(ctx, "system.tool.record_fail", "工具调用记录失败", event.P("tool", write.ToolKey), event.P("error", err))
			}
		}
		if countSession {
			if err := deps.Sessions.IncrementInvocationCounts(bg, sessionID, toolDelta, mcpDelta, skillDelta); err != nil {
				event.SessionSysLogWarn(ctx, sessionID, "system.tool.record_fail", "会话工具调用计数更新失败", event.P("tool", write.ToolKey), event.P("error", err))
			}
		}
	})
}
