package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/toolspill"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// newToolResultGateBeforeHook registers the L0 ToolResultGate BeforeModel
// hook (priority 3). The optional spillOpt carries the P0-2 spill
// dependencies (batch-A gray switch, design §3.4): when the PG blob persist
// inside gate.Check fails, the content is spilled to the workspace store and
// replaced by a locator pointer instead of the legacy hard truncation; if the
// spill itself fails, the content is preserved whole with an explicit
// enforcement=partial note (never silently truncated) and the Important-level
// tool_result.spill_failed signal is reported. With no deps the legacy
// truncateContent fallback runs byte-identically.
//
// spillOpt is variadic so existing call sites keep compiling and the
// auto-derived callback-chain hook name
// ("agent.newToolResultGateBeforeHook.func1", pinned by the C3 execution-order
// golden) is stable whether or not deps are passed.
func newToolResultGateBeforeHook(gate *biz.ToolResultGate, ag biz.Agent, lg loggateway.Logger, spillOpt ...*toolspill.Deps) callbacks.Callback {
	enabled := toolResultGateEnabled(ag)
	var spill *toolspill.Deps
	if len(spillOpt) > 0 {
		spill = spillOpt[0]
	}
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
		exemptLast := lastNToolResultIndexes(args.Request.Messages, toolResultGateExemptLastN)

		for i := range args.Request.Messages {
			msg := &args.Request.Messages[i]
			if msg.Role != trpcmodel.RoleTool {
				continue
			}
			if exemptLast[i] {
				continue
			}
			content := extractTextContent(msg)
			if utf8.RuneCountInString(content) <= biz.ToolResultSizeThreshold {
				continue
			}

			toolID := msg.ToolID
			if toolID == "" {
				toolID = fmt.Sprintf("auto_%d", i)
			}

			result, err := gate.Check(ctx, sessionID, toolID, msg.ToolName, "", content, turnNumber)
			if err != nil {
				// P0-2 spill branch (design §3.4): the truncation point is a
				// preservation point. Persist the full content to the spill
				// store and hand the model a resolvable locator; fail-soft
				// keeps the content whole with an explicit enforcement=partial
				// note when even the spill store is unavailable.
				if spill.Enabled() {
					loc, serr := toolspill.PutWithRetry(ctx, spill.Store, toolspill.Record{
						SessionID: sessionID,
						Tool:      msg.ToolName,
						Content:   []byte(content),
					})
					if serr == nil {
						lg.Info("L0 ToolResultGate: persist failed, spilled to workspace store",
							loggateway.StepID("agent.tool_result.gate_spill"),
							loggateway.Str("session_id", sessionID),
							loggateway.Str("tool", msg.ToolName),
							loggateway.Str("locator", loc.String()),
							loggateway.Err(err),
						)
						msg.Content = toolspill.PointerText(msg.ToolName, len(content), loc)
						msg.ContentParts = nil
						continue
					}
					lg.Warn("L0 ToolResultGate: persist and spill both failed, preserving full content (enforcement=partial)",
						loggateway.StepID("agent.tool_result.gate_spill"),
						loggateway.Str("session_id", sessionID),
						loggateway.Str("tool", msg.ToolName),
						loggateway.Err(serr),
					)
					spill.ReportSpillFailed(ctx, lg, "agent.tool_result.gate_spill", sessionID, serr)
					msg.Content = content + toolspill.PartialPreservedNote
					msg.ContentParts = nil
					continue
				}
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
	b, err := json.Marshal(msg.ContentParts)
	if err != nil {
		return fmt.Sprintf("[unserializable content parts: %v]", msg.ContentParts)
	}
	return string(b)
}

// toolResultGateExemptLastN keeps the most recent RoleTool results in this
// request (typically this turn's last N tool payloads) so the model can
// still read what it just got. Older oversized results still persist+pointer.
const toolResultGateExemptLastN = 3

func lastNToolResultIndexes(messages []trpcmodel.Message, n int) map[int]bool {
	exempt := make(map[int]bool)
	if n <= 0 {
		return exempt
	}
	idxs := make([]int, 0, 8)
	for i, m := range messages {
		if m.Role == trpcmodel.RoleTool {
			idxs = append(idxs, i)
		}
	}
	start := len(idxs) - n
	if start < 0 {
		start = 0
	}
	for _, i := range idxs[start:] {
		exempt[i] = true
	}
	return exempt
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
	runeCount := utf8.RuneCountInString(content)
	if runeCount <= biz.ToolResultPreviewSize {
		return content
	}
	return safeTruncate(content, biz.ToolResultPreviewSize) + fmt.Sprintf("\n\n... [truncated %d → %d chars, persist failed] ...", runeCount, biz.ToolResultPreviewSize)
}
