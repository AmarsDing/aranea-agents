package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	knowledgeCueMaxCollections = 10
	knowledgeCueMaxChars       = 1800
	knowledgeCueChunkChars     = 280
	knowledgeCueTopK           = 4
	knowledgeCueSearchTimeout  = 2 * time.Second
)

func newKnowledgeCueBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if deps.KnowledgeUsecase == nil {
		return nil
	}
	// P2-1（2026-08-16）：知识预检索不再被 ToolsEnabled 门控——关工具的
	// agent 仍可获得预检索命中的 chunks；仅"调用 knowledge_search 继续检索"
	// 的引导文案依赖工具开关（见 buildKnowledgeCue/formatKnowledgeCue）。
	toolsEnabled := ag.Settings != nil && ag.Settings.ToolsEnabled
	groundedOnly := biz.ParseAgentKnowledgeConfig(ag.ConfigJSON).GroundedOnly
	return callbacks.NewBeforeModelHook(6, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		cue := buildKnowledgeCue(ctx, deps.KnowledgeUsecase, deps.Logger(), args.Request.Messages, toolsEnabled, groundedOnly)
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		recordContextBudgetOnce(ctx, ContextBudgetCategoryKnowledgeCue, utf8.RuneCountInString(cue))
		sys := trpcmodel.NewSystemMessage(cue)
		args.Request.Messages = append(args.Request.Messages, sys)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func buildKnowledgeCue(ctx context.Context, uc *biz.KnowledgeUsecase, lg loggateway.Logger, msgs []trpcmodel.Message, toolsEnabled, groundedOnly bool) string {
	scopedIDs := knowledgetool.KnowledgeCollectionsFromContext(ctx)

	// C-01 回填：目录枚举按调用方 workspace 过滤（system 见全部）——
	// 否则 "Available Knowledge Bases" 列表泄露其他租户的库名/ID。
	collections, _, err := uc.ListCollections(ctx, workspace.ReadableFilterID(ctx), knowledgeCueMaxCollections, 0)
	if err != nil {
		lg.Warn("知识库摘要注入失败", loggateway.StepID("agent.knowledge.cue_fail"), loggateway.Err(err))
		return ""
	}

	filtered := filterCueCollections(collections, scopedIDs)
	// P1-2（2026-08-16）：检索查询与记忆召回同口径清洗（去客套前缀/多句
	// 切分/120 字预算尾部优先），team 纯词法库（tsvector/trigram）下未清洗
	// 的整句查询会显著拖低 FTS 命中率。
	query := lastUserQuery(msgs)
	if query != "" {
		query = cleanRecallQuery(query)
	}
	var chunks []biz.KnowledgeChunk
	if query != "" {
		chunks = retrieveCueChunks(ctx, query, scopedIDs, filtered, lg)
		// 引用闭环补全（P1）：首轮预检索注入也发 knowledge_recalled——日常供粮
		// 主路径由此进入 cited 回采。lastUserQuery 只在每轮用户消息首次模型调用
		// 取查询，工具循环续跑天然不重复发；同 chunk 同 turn 由 citations 账本幂等。
		knowledgetool.EmitKnowledgeRecalledNotice(ctx, cueRenderedChunks(chunks))
	}
	return formatKnowledgeCue(filtered, chunks, toolsEnabled, groundedOnly)
}

// cueRenderedChunks 过滤出 formatKnowledgeCue 实际渲染的 chunks（非空正文、
// knowledgeCueTopK 截断），保证 notice 载荷与注入内容一致。
func cueRenderedChunks(chunks []biz.KnowledgeChunk) []biz.KnowledgeChunk {
	out := make([]biz.KnowledgeChunk, 0, len(chunks))
	for _, ch := range chunks {
		if len(out) >= knowledgeCueTopK {
			break
		}
		if strings.TrimSpace(ch.Content) == "" {
			continue
		}
		out = append(out, ch)
	}
	return out
}

func filterCueCollections(collections []biz.KnowledgeCollection, scopedIDs []string) []biz.KnowledgeCollection {
	if len(scopedIDs) == 0 {
		return collections
	}
	idSet := make(map[string]struct{}, len(scopedIDs))
	for _, id := range scopedIDs {
		idSet[id] = struct{}{}
	}
	var filtered []biz.KnowledgeCollection
	for _, col := range collections {
		if _, ok := idSet[col.ID]; ok {
			filtered = append(filtered, col)
		}
	}
	return filtered
}

// lastUserQuery 只在本轮第一次模型调用（最新非 system 消息是 user）时取查询。
// 工具循环续跑时跳过，避免每轮重复检索拖慢 TTFT。
func lastUserQuery(msgs []trpcmodel.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		role := msgs[i].Role
		if role == trpcmodel.RoleSystem {
			continue
		}
		if role == trpcmodel.RoleUser {
			return strings.TrimSpace(msgs[i].Content)
		}
		return ""
	}
	return ""
}

func retrieveCueChunks(ctx context.Context, query string, scoped []string, catalog []biz.KnowledgeCollection, lg loggateway.Logger) []biz.KnowledgeChunk {
	// 词条优先写回：日记流水（inbox/writeback-*）仅 provenance，不进默认预检索。
	q := biz.KnowledgeSearchQuery{Query: query, TopK: knowledgeCueTopK, ExcludePathPrefixes: []string{biz.KnowledgeWriteBackInboxPrefix}}
	searchCtx, cancel := context.WithTimeout(ctx, knowledgeCueSearchTimeout)
	defer cancel()

	fr := knowledgetool.FederatedRetrieverFromContext(ctx)
	if fr != nil && len(scoped) != 1 {
		var chunks []biz.KnowledgeChunk
		var err error
		if len(scoped) == 0 {
			chunks, err = fr.SearchAll(searchCtx, q, nil, "", workspace.ReadableFilterID(ctx))
		} else {
			chunks, err = fr.Search(searchCtx, scoped, q, nil, "")
		}
		if err != nil {
			lg.Warn("知识库预检索失败", loggateway.StepID("agent.knowledge.cue_retrieve_fail"), loggateway.Err(err))
			return nil
		}
		return chunks
	}

	if len(scoped) == 1 {
		q.CollectionID = scoped[0]
	} else if len(catalog) == 1 {
		q.CollectionID = catalog[0].ID
	}

	if router := knowledgetool.AdaptiveRouterFromContext(ctx); router != nil && q.CollectionID != "" {
		chunks, err := router.Search(searchCtx, q, nil, "")
		if err != nil {
			lg.Warn("知识库预检索失败", loggateway.StepID("agent.knowledge.cue_retrieve_fail"), loggateway.Err(err))
			return nil
		}
		return chunks
	}
	if r := knowledgetool.RetrieverFromContext(ctx); r != nil && q.CollectionID != "" {
		chunks, err := r.Search(searchCtx, q)
		if err != nil {
			lg.Warn("知识库预检索失败", loggateway.StepID("agent.knowledge.cue_retrieve_fail"), loggateway.Err(err))
			return nil
		}
		return chunks
	}
	return nil
}

// formatKnowledgeCue 渲染知识 cue。toolsEnabled=false 时只渲染预检索命中的
// chunks（不输出工具引导文案与目录——agent 无法调用检索工具，列出可搜库
// 只会误导）；此时若无命中 chunks 则不注入。groundedOnly 时禁止用世界知识，
// 无命中也注入拒答指令。P3-2：整体预算按块/条目边界截断。
func formatKnowledgeCue(filtered []biz.KnowledgeCollection, chunks []biz.KnowledgeChunk, toolsEnabled, groundedOnly bool) string {
	rendered := cueRenderedChunks(chunks)
	if groundedOnly && len(rendered) == 0 && !toolsEnabled {
		return "## Retrieved Knowledge\n" +
			"The knowledge base has no passages for this question. You MUST say you do not have evidence in the knowledge base. Do not use world knowledge.\n"
	}
	if !toolsEnabled {
		// 无工具：仅"有实质命中"才值得注入。
		if len(rendered) == 0 {
			return ""
		}
		filtered = nil
	}
	if groundedOnly {
		// Grounded：目录与「不够就用常识」会诱导世界知识，一律不列。
		filtered = nil
	}
	if len(filtered) == 0 && len(chunks) == 0 && !groundedOnly {
		return ""
	}
	if groundedOnly && len(rendered) == 0 && toolsEnabled {
		return "## Retrieved Knowledge\n" +
			"Pre-retrieval found no passages. You may call `knowledge_search` or `knowledge_reflect`. If those also return nothing, say the knowledge base has no evidence. Do not use world knowledge.\n"
	}

	var b strings.Builder
	used := 0
	// writeBlock 仅在整段放得下时写入，保证截断发生在块/条目边界。
	writeBlock := func(s string) bool {
		n := utf8.RuneCountInString(s)
		if used+n > knowledgeCueMaxChars {
			return false
		}
		b.WriteString(s)
		used += n
		return true
	}

	if len(chunks) > 0 {
		header := "## Retrieved Knowledge\n" +
			"The following passages were retrieved for the current user question. Cite them by [n] when using this knowledge."
		if groundedOnly {
			header += " Use ONLY these passages. If they do not answer the question, say the knowledge base has no evidence. Do not use world knowledge."
			if toolsEnabled {
				header += " You may call `knowledge_search` or `knowledge_reflect` for more passages from the knowledge base, then refuse if still insufficient."
			}
		} else if toolsEnabled {
			header += " If they are insufficient, call `knowledge_search` or `knowledge_reflect`."
		}
		writeBlock(header + "\n\n")
		for i, ch := range chunks {
			if i >= knowledgeCueTopK {
				break
			}
			content := strings.TrimSpace(ch.Content)
			if content == "" {
				continue
			}
			if utf8.RuneCountInString(content) > knowledgeCueChunkChars {
				content = string([]rune(content)[:knowledgeCueChunkChars]) + "..."
			}
			if !writeBlock(fmt.Sprintf("[%d] (doc=%s score=%.2f)\n%s\n\n", i+1, ch.DocID, ch.Score, content)) {
				break
			}
		}
	}

	if len(filtered) > 0 {
		writeBlock("## Available Knowledge Bases\n")
		if len(chunks) == 0 {
			writeBlock("The following knowledge bases are available for search. Use `knowledge_search` to search a specific collection, or `knowledge_reflect` to search across multiple collections and evaluate result quality.\n\n")
		} else {
			writeBlock("Additional collections you can search if the passages above are not enough:\n\n")
		}

		for i, col := range filtered {
			if i >= knowledgeCueMaxCollections {
				break
			}
			var line strings.Builder
			fmt.Fprintf(&line, "- **%s** (ID: `%s`)", col.Name, col.ID)
			if col.Description != "" && len(chunks) == 0 {
				desc := col.Description
				if len([]rune(desc)) > 120 {
					desc = string([]rune(desc)[:120]) + "..."
				}
				fmt.Fprintf(&line, ": %s", desc)
			}
			fmt.Fprintf(&line, " [%d docs, %d chunks]\n", col.DocumentCount, col.ChunkCount)
			if !writeBlock(line.String()) {
				break
			}
		}

		if len(chunks) == 0 {
			writeBlock("\n**Search strategy tips:**\n" +
				"- For specific factual questions → `knowledge_search` (omit collection_id to auto-route across all bases)\n" +
				"- For broad or multi-topic questions → `knowledge_reflect` (omit collection_ids to search all bases and evaluate quality)\n" +
				"- If initial results are insufficient → `knowledge_reflect` will suggest supplementary queries\n")
		}
	}

	return b.String()
}
