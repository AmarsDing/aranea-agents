package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/mcp/classify"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/telemetry/turntrace"
	"aranea-agents/internal/tools/preview"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

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

func recordToolInvocationAfter(ctx context.Context, args *trpctool.AfterToolArgs, ag biz.Agent, deps TRPCBuilderDeps) {
	toolKey := strings.TrimSpace(args.ToolName)
	if toolKey == "" {
		return
	}
	ended := time.Now().UTC()
	started := ended
	var durationMS int
	if t, ok := ctx.Value(toolCallStartKey{}).(time.Time); ok {
		started = t
		durationMS = int(ended.Sub(t).Milliseconds())
	}
	status, errCode, errMsg := invocationStatusFromAfter(args)
	if status == "blocked" && errCode == "confirmation_required" {
		return
	}
	var streaming bool
	var chunkCount int
	if deps.ToolUC != nil {
		if tool, err := deps.ToolUC.GetTool(ctx, toolKey); err == nil {
			streaming, chunkCount = classifyToolInvocationStreaming(tool, args.Meta)
		}
	}
	write := biz.ToolInvocationWrite{
		ToolKey:       toolKey,
		Status:        status,
		DurationMS:    durationMS,
		StartedAt:     started.Format(time.RFC3339),
		EndedAt:       ended.Format(time.RFC3339),
		InputPreview:  previewFromToolArgs(args.Arguments),
		OutputPreview: previewFromToolResult(args.Result),
		ErrorCode:     errCode,
		ErrorMessage:  errMsg,
		Source:        biz.ToolInvocationSourceRuntime,
		ToolCallID:    args.ToolCallID,
		Streaming:     streaming,
		ChunkCount:    chunkCount,
	}
	recordToolInvocationWrite(ctx, write, args.Result, ag, deps)
}

func previewFromToolArgs(args []byte) string {
	return preview.RedactAndTruncate(string(args), 2000)
}

func previewFromToolResult(result any) string {
	if result == nil {
		return ""
	}
	b, err := json.Marshal(result)
	if err != nil {
		return preview.RedactAndTruncate(fmt.Sprint(result), 2000)
	}
	return preview.RedactAndTruncate(string(b), 2000)
}

func recordToolInvocationWrite(ctx context.Context, write biz.ToolInvocationWrite, toolResult any, ag biz.Agent, deps TRPCBuilderDeps) {
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
		write.Source = biz.ToolInvocationSourceRuntime
	}
	status := strings.TrimSpace(write.Status)
	if status == "" {
		status = "success"
	}
	metrics.ToolInvocationTotal.WithLabelValues(write.ToolKey, status).Inc()
	if classify.IsMCPToolInvocation(write.ToolKey, toolResult) {
		metrics.MCPInvocationTotal.WithLabelValues(write.ToolKey, status).Inc()
	}

	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.CompleteToolCall(write.ToolCallID, write.ToolKey, write.DurationMS, status)
	}
	if bridge := turntrace.FromContext(ctx); bridge != nil {
		var hookErr error
		if status != "success" && write.ErrorMessage != "" {
			hookErr = fmt.Errorf("%s", write.ErrorMessage)
		}
		bridge.RecordToolCallEnd(write.ToolCallID, write.ToolKey, hookErr)
	}

	var toolDelta, mcpDelta, skillDelta int
	countSession := strings.TrimSpace(sessionID) != "" && deps.Sessions != nil && status != "blocked"
	if countSession {
		toolDelta = 1
		mcp, skill := classifyToolInvocation(ctx, write.ToolKey, toolResult, deps)
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
			auditWrite := biz.ToolInvocationAuditWrite{
				InvocationID:  write.ToolCallID,
				ToolKey:       write.ToolKey,
				AgentID:       write.AgentID,
				UserID:        write.UserID,
				SessionID:     write.SessionID,
				Action:        "tool.call",
				ResultSummary: write.OutputPreview,
				Status:        write.Status,
				Source:        write.Source,
			}
			if auditWrite.ResultSummary == "" && write.ErrorMessage != "" {
				auditWrite.ResultSummary = write.ErrorMessage
			}
			if err := deps.ToolUC.RecordToolInvocationAudit(bg, auditWrite); err != nil {
				event.CtxFlowLogWarn(ctx, "system.tool.audit_fail", "工具调用审计写入失败", event.P("tool", write.ToolKey), event.P("error", err))
			}
		}
		if countSession {
			if err := deps.Sessions.IncrementInvocationCounts(bg, sessionID, toolDelta, mcpDelta, skillDelta); err != nil {
				event.SessionSysLogWarn(ctx, sessionID, "system.tool.record_fail", "会话工具调用计数更新失败", event.P("tool", write.ToolKey), event.P("error", err))
			}
		}
	})
}
