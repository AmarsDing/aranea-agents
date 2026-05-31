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
	return callbacks.NewBeforeModelHook(6, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		cue := buildKnowledgeCue(ctx, deps.KnowledgeUsecase)
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func buildKnowledgeCue(ctx context.Context, uc *biz.KnowledgeUsecase) string {
	scopedIDs := knowledgetool.KnowledgeCollectionsFromContext(ctx)

	collections, _, err := uc.ListCollections(ctx, "", knowledgeCueMaxCollections, 0)
	if err != nil {
		loggateway.Global().Warn("知识库摘要注入失败", loggateway.StepID("knowledge.cue.list_fail"), loggateway.Str("error", err.Error()))
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
			if len(desc) > 120 {
				desc = desc[:120] + "..."
			}
			fmt.Fprintf(&b, ": %s", desc)
		}
		fmt.Fprintf(&b, " [%d docs, %d chunks]", col.DocumentCount, col.ChunkCount)
		b.WriteByte('\n')
	}

	b.WriteString("\n**Search strategy tips:**\n")
	b.WriteString("- For specific factual questions → `knowledge_search` with the most relevant collection_id\n")
	b.WriteString("- For broad or multi-topic questions → `knowledge_reflect` with multiple collection_ids\n")
	b.WriteString("- If initial results are insufficient → `knowledge_reflect` will suggest supplementary queries\n")

	result := b.String()
	if len(result) > knowledgeCueMaxChars {
		result = result[:knowledgeCueMaxChars] + "..."
	}
	return result
}
