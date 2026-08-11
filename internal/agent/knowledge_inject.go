package agent

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	knowledgeCueMaxCollections = 10
	knowledgeCueMaxChars       = 1500
)

func newKnowledgeCueBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if deps.KnowledgeUsecase == nil {
		return nil
	}
	if ag.Settings == nil || !ag.Settings.ToolsEnabled {
		return nil
	}
	return callbacks.NewBeforeModelHook(6, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		cue := buildKnowledgeCue(ctx, deps.KnowledgeUsecase, deps.Logger())
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// Prefix stabilization: append after the existing system block.
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = insertAfterLastSystem(args.Request.Messages, sys)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func buildKnowledgeCue(ctx context.Context, uc *biz.KnowledgeUsecase, lg loggateway.Logger) string {
	scopedIDs := knowledgetool.KnowledgeCollectionsFromContext(ctx)

	collections, _, err := uc.ListCollections(ctx, "", knowledgeCueMaxCollections, 0)
	if err != nil {
		lg.Warn("知识库摘要注入失败", loggateway.StepID("agent.knowledge.cue_fail"), loggateway.Err(err))
		return ""
	}

	var filtered []biz.KnowledgeCollection
	if len(scopedIDs) > 0 {
		idSet := make(map[string]struct{}, len(scopedIDs))
		for _, id := range scopedIDs {
			idSet[id] = struct{}{}
		}
		for _, col := range collections {
			if _, ok := idSet[col.ID]; ok {
				filtered = append(filtered, col)
			}
		}
	} else {
		filtered = collections
	}

	if len(filtered) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Available Knowledge Bases\n")
	b.WriteString("The following knowledge bases are available for search. Use `knowledge_search` to search a specific collection, or `knowledge_reflect` to search across multiple collections and evaluate result quality.\n\n")

	for i, col := range filtered {
		if i >= knowledgeCueMaxCollections {
			break
		}
		fmt.Fprintf(&b, "- **%s** (ID: `%s`)", col.Name, col.ID)
		if col.Description != "" {
			desc := col.Description
			if len([]rune(desc)) > 120 {
				desc = string([]rune(desc)[:120]) + "..."
			}
			fmt.Fprintf(&b, ": %s", desc)
		}
		fmt.Fprintf(&b, " [%d docs, %d chunks]", col.DocumentCount, col.ChunkCount)
		b.WriteByte('\n')
	}

	b.WriteString("\n**Search strategy tips:**\n")
	b.WriteString("- For specific factual questions → `knowledge_search` (omit collection_id to auto-route across all bases)\n")
	b.WriteString("- For broad or multi-topic questions → `knowledge_reflect` (omit collection_ids to search all bases and evaluate quality)\n")
	b.WriteString("- If initial results are insufficient → `knowledge_reflect` will suggest supplementary queries\n")

	result := b.String()
	if len([]rune(result)) > knowledgeCueMaxChars {
		result = string([]rune(result)[:knowledgeCueMaxChars]) + "..."
	}
	return result
}
