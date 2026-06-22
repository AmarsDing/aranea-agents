package memory_butler

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type consolidateEpisodesInput struct {
	AgentID string `json:"agent_id" jsonschema:"description=Agent ID,required"`
}

type consolidateEpisodesOutput struct {
	DistilledCount int `json:"distilled_count"`
}

func newConsolidateEpisodesTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input consolidateEpisodesInput) (consolidateEpisodesOutput, error) {
		if input.AgentID == "" {
			return consolidateEpisodesOutput{}, ErrAgentIDRequired
		}

		// List episodic facts (kind="episode") for the agent.
		rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, biz.ListFactRowsParams{
			ScopeType: "agent",
			ScopeID:   input.AgentID,
			Kind:      "episode",
			Limit:     defaultFactListLimit,
			Offset:    0,
		})
		if err != nil {
			return consolidateEpisodesOutput{}, err
		}
		if len(rows) == 0 {
			return consolidateEpisodesOutput{DistilledCount: 0}, nil
		}

		// P0: Simple concatenation + dedup of episode statements.
		// TODO(P1): Use LLM-based distillation for higher quality summaries.
		seen := make(map[string]bool)
		var unique []string
		for _, raw := range rows {
			m, _ := jsonutil.ParseMap(raw)
			if m == nil {
				continue
			}
			stmt := strings.TrimSpace(jsonutil.IfaceStr(m, "statement"))
			if stmt == "" {
				continue
			}
			lower := strings.ToLower(stmt)
			if !seen[lower] {
				seen[lower] = true
				unique = append(unique, stmt)
			}
		}

		if len(unique) == 0 {
			return consolidateEpisodesOutput{DistilledCount: 0}, nil
		}

		// Build a distilled summary statement.
		var sb strings.Builder
		sb.WriteString("[Distilled from episodes] ")
		for i, s := range unique {
			if i > 0 {
				sb.WriteString("; ")
			}
			sb.WriteString(s)
		}

		_, err = deps.MemoryAdmin.UpsertFactRow(ctx, biz.FactUpsert{
			Statement:  sb.String(),
			ScopeType:  "agent",
			ScopeID:    input.AgentID,
			AgentID:    input.AgentID,
			FactKind:   "semantic",
			SourceKind: "consolidate_episodes",
			Status:     "active",
		})
		if err != nil {
			return consolidateEpisodesOutput{}, err
		}

		// Delete the original episodic facts now that they have been distilled.
		var episodeIDs []string
		for _, raw := range rows {
			m, parseErr := jsonutil.ParseMap(raw)
			if parseErr != nil || m == nil {
				continue
			}
			id := jsonutil.IfaceStr(m, "id")
			if id != "" {
				episodeIDs = append(episodeIDs, id)
			}
		}
		if len(episodeIDs) > 0 {
			if _, delErr := deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, episodeIDs); delErr != nil {
				// Log but don't fail — the distillation succeeded, deletion is best-effort.
				if deps.LG != nil {
					deps.LG.Warn("consolidate_episodes: failed to delete original episodic facts",
						loggateway.StepID("memory_butler.consolidate.delete_episodes"),
						loggateway.Str("agent_id", input.AgentID),
						loggateway.Err(delErr))
				}
			}
		}

		return consolidateEpisodesOutput{DistilledCount: len(unique)}, nil
	}

	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_consolidate_episodes"),
		function.WithDescription("整合情景记忆：将零散的情景记忆（episode）提炼为语义记忆，减少冗余并提取规律性知识。"),
	)
}
