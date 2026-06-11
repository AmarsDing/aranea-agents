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
	"aranea-agents/pkg/loggateway"
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
			return "blocked", event.ErrorCodeConfirmationRequired, truncateErr(msg)
		}
		// Use "failed" (not "error") for the runtime status. The wire contract uses
		// {running, success, failed, blocked, cancelled} as the canonical set, and
		// the frontend normalizeToolStatus collapses both "failed" and legacy
		// "error" to a single canonical status. Emitting "error" here made the
		// frontend's `error_code` fallback unreachable and erased error info.
		return "failed", event.ErrorCodeToolError, truncateErr(msg)
	}
	return "success", "", ""
}

func truncateErr(msg string) string {
	return safeTruncate(msg, 500)
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
	if status == "blocked" && errCode == event.ErrorCodeConfirmationRequired {
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
	var isSkillCall bool
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
		isSkillCall = skill
	} else {
		// Classify skill even when not counting session, for skill_invocation recording.
		_, isSkillCall = classifyToolInvocation(ctx, write.ToolKey, toolResult, deps)
	}

	safego.Go(ctx, "recordToolInvocation", func() {
		bg := context.Background()
		lg := deps.Logger()
		if deps.ToolUC != nil {
			if err := deps.ToolUC.RecordToolInvocation(bg, write); err != nil {
				lg.Warn("工具调用记录失败", loggateway.StepID("agent.tool.record_fail"), loggateway.Str("tool", write.ToolKey), loggateway.Err(err))
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
				lg.Warn("工具调用审计写入失败", loggateway.StepID("agent.tool.audit_fail"), loggateway.Str("tool", write.ToolKey), loggateway.Err(err))
			}
		}
		if countSession {
			if err := deps.Sessions.IncrementInvocationCounts(bg, sessionID, toolDelta, mcpDelta, skillDelta); err != nil {
				deps.Logger().With(loggateway.SessionID(sessionID)).Warn("会话工具调用计数更新失败", loggateway.StepID("agent.tool.record_fail"), loggateway.Str("tool", write.ToolKey), loggateway.Err(err))
			}
		}
		// Record skill_invocation for skill-type tool calls.
		if isSkillCall && deps.SkillUC != nil {
			recordSkillInvocation(bg, ctx, write, ag, deps)
		}
	})
}

// recordSkillInvocation creates a skill_invocation row when a skill-type tool call completes.
// It reads selection_reason from invocation state (set by skill_guidance_inject) and derives
// outcome from the tool invocation status.
func recordSkillInvocation(bg context.Context, origCtx context.Context, write biz.ToolInvocationWrite, ag biz.Agent, deps TRPCBuilderDeps) {
	lg := deps.Logger()

	// Derive outcome from tool invocation status.
	outcome := ""
	switch write.Status {
	case "success":
		outcome = "success"
	case "failed", "error":
		// "failed" is the canonical runtime status (tool_invocation_recorder.go).
		// "error" is accepted here for backward compatibility with rows written
		// before the status was normalized.
		outcome = "failure"
	default:
		outcome = "partial"
	}

	// Read selection_reason from invocation state.
	var selectionReason map[string]interface{}
	if inv, ok := trpcagent.InvocationFromContext(origCtx); ok {
		if raw, ok2 := inv.GetState(skillSelectionReasonStateKey); ok2 {
			if m, ok3 := raw.(map[string]string); ok3 {
				selectionReason = make(map[string]interface{}, len(m))
				for k, v := range m {
					selectionReason[k] = v
				}
			}
		}
	}

	// Read token_usage from invocation state.
	var tokenUsage map[string]interface{}
	if inv, ok := trpcagent.InvocationFromContext(origCtx); ok {
		if raw, ok2 := inv.GetState(skillTokenUsageStateKey); ok2 {
			if snap, ok3 := raw.(tokenUsageSnapshot); ok3 {
				tokenUsage = map[string]interface{}{
					"prompt":     snap.PromptTokens,
					"completion": snap.CompletionTokens,
					"total":      snap.TotalTokens,
				}
			}
		}
	}

	// Read routed_slugs from invocation state.
	var routedSlugs []string
	if inv, ok := trpcagent.InvocationFromContext(origCtx); ok {
		if raw, ok2 := inv.GetState(skillRoutedSlugsStateKey); ok2 {
			if slugs, ok3 := raw.([]string); ok3 {
				routedSlugs = slugs
			}
		}
	}

	// Read loaded_slug from invocation state (set by newSkillLoadCaptureAfterHook).
	loadedSlug := ""
	if inv, ok := trpcagent.InvocationFromContext(origCtx); ok {
		if raw, ok2 := inv.GetState(skillLoadedSlugStateKey); ok2 {
			if s, ok3 := raw.(string); ok3 {
				loadedSlug = s
			}
		}
	}

	// Resolve skill_id from tool_key (slug).
	skillID := ""
	slug := strings.TrimPrefix(write.ToolKey, "use_skill_")
	if sk, err := deps.SkillUC.GetBySlug(bg, slug); err == nil {
		skillID = sk.ID
	} else {
		// Fallback: try the raw tool key as slug.
		if sk2, err2 := deps.SkillUC.GetBySlug(bg, write.ToolKey); err2 == nil {
			skillID = sk2.ID
		}
	}

	skillWrite := biz.SkillInvocationWrite{
		SkillID:         skillID,
		AgentID:         ag.ID,
		UserID:          write.UserID,
		SessionID:       write.SessionID,
		Status:          write.Status,
		DurationMS:      write.DurationMS,
		StartedAt:       write.StartedAt,
		EndedAt:         write.EndedAt,
		InputPreview:    write.InputPreview,
		InputHash:       write.InputHash,
		OutputPreview:   write.OutputPreview,
		ErrorCode:       write.ErrorCode,
		ErrorMessage:    write.ErrorMessage,
		Source:          "runtime",
		ActivationID:    write.ToolCallID,
		SelectionReason: selectionReason,
		Outcome:         outcome,
		TokenUsage:      tokenUsage,
		RoutedSlugs:     routedSlugs,
		LoadedSlug:      loadedSlug,
	}
	if err := deps.SkillUC.RecordInvocation(bg, skillWrite); err != nil {
		lg.Warn("skill invocation 记录失败", loggateway.StepID("agent.skill.record_fail"), loggateway.Str("tool", write.ToolKey), loggateway.Err(err))
	}
}
