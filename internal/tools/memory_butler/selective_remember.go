package memory_butler

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type selectiveRememberInput struct {
	Content string `json:"content" jsonschema:"description=要记忆的内容,required"`
	Context string `json:"context" jsonschema:"description=记忆的上下文信息"`
	AgentID string `json:"agent_id" jsonschema:"description=Agent ID,required"`
}

type selectiveRememberOutput struct {
	Remembered bool   `json:"remembered"`
	Reason     string `json:"reason"`
}

func newSelectiveRememberTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input selectiveRememberInput) (selectiveRememberOutput, error) {
		if input.AgentID == "" {
			return selectiveRememberOutput{}, ErrAgentIDRequired
		}
		if input.Content == "" {
			return selectiveRememberOutput{}, ErrContentRequired
		}

		// Check for redundancy by listing existing facts and comparing.
		rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, "agent", input.AgentID, "", "", "", 100, 0)
		if err != nil {
			return selectiveRememberOutput{}, err
		}

		contentLower := strings.ToLower(strings.TrimSpace(input.Content))
		for _, raw := range rows {
			m, _ := jsonutil.ParseMap(raw)
			if m == nil {
				continue
			}
			existing := strings.ToLower(strings.TrimSpace(jsonutil.IfaceStr(m, "statement")))
			if existing == "" {
				continue
			}
			// Simplified redundancy check: if the new content is a substring of an existing
			// fact or vice versa with high overlap, consider it redundant.
			// TODO(P1): Replace with embedding-based cosine similarity (threshold 0.85).
			if contentLower == existing {
				return selectiveRememberOutput{Remembered: false, Reason: "redundant with existing memory"}, nil
			}
			if len(contentLower) > 20 && len(existing) > 20 {
				if strings.Contains(contentLower, existing) || strings.Contains(existing, contentLower) {
					return selectiveRememberOutput{Remembered: false, Reason: "redundant with existing memory"}, nil
				}
			}
		}

		// Novel content — write as a new fact.
		_, err = deps.MemoryAdmin.UpsertFactRow(ctx, biz.FactUpsert{
			Statement:  input.Content,
			ScopeType:  "agent",
			ScopeID:    input.AgentID,
			AgentID:    input.AgentID,
			FactKind:   "semantic",
			SourceKind: "selective_remember",
			Status:     "active",
		})
		if err != nil {
			return selectiveRememberOutput{}, err
		}
		return selectiveRememberOutput{Remembered: true, Reason: "novel content worth remembering"}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_selective_remember"),
		function.WithDescription("选择性记忆：判断内容是否值得记忆，避免冗余存储。若内容与已有记忆重复则跳过，否则写入新记忆。"),
	)
}
