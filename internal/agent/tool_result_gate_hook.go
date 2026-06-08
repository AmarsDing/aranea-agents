package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func newToolResultGateBeforeHook(gate *biz.ToolResultGate, ag biz.Agent, lg loggateway.Logger) callbacks.Callback {
	enabled := toolResultGateEnabled(ag)
	return callbacks.NewBeforeModelHook(3, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if !enabled || gate == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}

		sessionID := sessionIDFromInvocationContext(ctx)
		if sessionID == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}

		turnNumber := estimateTurnNumber(args.Request.Messages)

		for i := range args.Request.Messages {
			msg := &args.Request.Messages[i]
			if msg.Role != trpcmodel.RoleTool {
				continue
			}
			content := extractTextContent(msg)
			if len(content) <= biz.ToolResultSizeThreshold {
				continue
			}

			toolID := msg.ToolID
			if toolID == "" {
				toolID = fmt.Sprintf("auto_%d", i)
			}

			result, err := gate.Check(ctx, sessionID, toolID, msg.ToolName, "", content, turnNumber)
			if err != nil {
				lg.Error("L0 ToolResultGate: persist failed, truncating as fallback", loggateway.StepID("agent.tool_result.gate_fail"), loggateway.Str("session_id", sessionID), loggateway.Err(err))
				msg.Content = truncateContent(content)
				msg.ContentParts = nil
				continue
			}
			if result.DidPersist {
				msg.Content = result.PreviewText
				msg.ContentParts = nil
			}
		}

		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func toolResultGateEnabled(ag biz.Agent) bool {
	if ag.Settings == nil {
		return true
	}
	return ag.Settings.ToolResultGateEnabled
}

func sessionIDFromInvocationContext(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return ""
	}
	return inv.Session.ID
}

func extractTextContent(msg *trpcmodel.Message) string {
	if msg.Content != "" {
		return msg.Content
	}
	if len(msg.ContentParts) == 0 {
		return ""
	}
	for _, p := range msg.ContentParts {
		if p.Type == trpcmodel.ContentTypeText && p.Text != nil && *p.Text != "" {
			return *p.Text
		}
	}
	b, _ := json.Marshal(msg.ContentParts)
	return string(b)
}

func estimateTurnNumber(messages []trpcmodel.Message) int {
	n := 0
	for _, m := range messages {
		if m.Role == trpcmodel.RoleUser {
			n++
		}
	}
	return n
}

func truncateContent(content string) string {
	if len(content) <= biz.ToolResultPreviewSize {
		return content
	}
	return content[:biz.ToolResultPreviewSize] + fmt.Sprintf("\n\n... [truncated %d → %d chars, persist failed] ...", len(content), biz.ToolResultPreviewSize)
}
