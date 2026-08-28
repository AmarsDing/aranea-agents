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

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
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
		cue, cited, fresh := resolveKnowledgeCue(ctx, deps.KnowledgeUsecase, deps.Logger(), args.Request.Messages, toolsEnabled, groundedOnly)
		if cue == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		recordContextBudgetOnce(ctx, ContextBudgetCategoryKnowledgeCue, utf8.RuneCountInString(cue))
		// 引用闭环 notice 仅 fresh 轮发送（与 MemoryInject P2-3 同构）：缓存轮
		// 复用同一 cue，重复发 notice 会让前端脚注/cited 回采重复计数。
		if fresh && len(cited) > 0 {
			// 引用闭环：notice.n 与 cue [n] 同一顺序，前端脚注与 cited 回采共用。
			knowledgetool.EmitNumberedKnowledgeRecalledNotice(ctx, cited)
		}
		args.Request.Messages = replaceDynamicCue(args.Request.Messages, knowledgeCueMarker, knowledgeCueMarker+cue)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// knowledgeCueTurnCacheStateKey 是 per-turn 知识 cue 缓存在 invocation state
// 中的键（与 memoryCueTurnCacheStateKey 同构，2026-08-21 全链路审查 B2）。
// 框架工具循环（llmflow.runOneStep）每轮重进 BeforeModel hook；缓存前每轮
// 都 ListCollections（DB）并把目录+策略文案重复 append 进请求，既费 DB 又
// 费 token。首轮构建后缓存，续轮复用重注（cue 保持在上下文内）。
const knowledgeCueTurnCacheStateKey = "aranea.knowledge_cue.turn_cache"

type knowledgeCueTurnCache struct {
	query string
	cue   string
	cited []biz.KnowledgeChunk
}

// resolveKnowledgeCue 返回 (cue, cited, fresh)。fresh=true 表示本轮真实构建
// （结果已写入 per-turn 缓存）；false 表示复用缓存。工具循环续轮
// （lastUserQuery==""，最新非固定消息是 assistant/tool）直接复用首轮缓存；
// 无 invocation 上下文时（单测/异常路径）退化为每轮 fresh 构建，保持旧行为。
func resolveKnowledgeCue(ctx context.Context, uc *biz.KnowledgeUsecase, lg loggateway.Logger, msgs []trpcmodel.Message, toolsEnabled, groundedOnly bool) (string, []biz.KnowledgeChunk, bool) {
	query := lastUserQuery(msgs)
	if query != "" {
		// P1-2（2026-08-16）：检索查询与记忆召回同口径清洗（去客套前缀/多句
		// 切分/120 字预算尾部优先），team 纯词法库（tsvector/trigram）下未清洗
		// 的整句查询会显著拖低 FTS 命中率。
		query = cleanRecallQuery(query)
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if ok && inv != nil {
		if v, found := inv.GetState(knowledgeCueTurnCacheStateKey); found {
			if c, cached := v.(*knowledgeCueTurnCache); cached && c != nil && (query == "" || query == c.query) {
				return c.cue, c.cited, false
			}
		}
	}
	if cue, cited, hit := knowledgeCueFromPrefetch(ctx, query); hit {
		if ok && inv != nil {
			inv.SetState(knowledgeCueTurnCacheStateKey, &knowledgeCueTurnCache{query: query, cue: cue, cited: cited})
		}
		return cue, cited, true
	}
	if !ok || inv == nil {
		cue, cited := buildKnowledgeCue(ctx, uc, lg, query, toolsEnabled, groundedOnly, knowledgetool.MemoryL3GroundedFromContext(ctx))
		return cue, cited, true
	}
	cue, cited := buildKnowledgeCue(ctx, uc, lg, query, toolsEnabled, groundedOnly, knowledgetool.MemoryL3GroundedFromContext(ctx))
	inv.SetState(knowledgeCueTurnCacheStateKey, &knowledgeCueTurnCache{query: query, cue: cue, cited: cited})
	return cue, cited, true
}

func knowledgeCueFromPrefetch(ctx context.Context, query string) (string, []biz.KnowledgeChunk, bool) {
	p := turnCuePrefetchFromContext(ctx)
	if p == nil || p.knowledge == nil {
		return "", nil, false
	}
	if query != "" && p.knowledge.query != "" && query != p.knowledge.query {
		return "", nil, false
	}
	return p.knowledge.cue, p.knowledge.cited, true
}

func buildKnowledgeCue(ctx context.Context, uc *biz.KnowledgeUsecase, lg loggateway.Logger, query string, toolsEnabled, groundedOnly, memoryGrounded bool) (string, []biz.KnowledgeChunk) {
	scopedIDs := knowledgetool.KnowledgeCollectionsFromContext(ctx)

	// C-01 回填：目录枚举按调用方 workspace 过滤（system 见全部）——
	// 否则 "Available Knowledge Bases" 列表泄露其他租户的库名/ID。
	collections, _, err := uc.ListCollections(ctx, workspace.ReadableFilterID(ctx), knowledgeCueMaxCollections, 0)
	if err != nil {
		lg.Warn("知识库摘要注入失败", loggateway.StepID("agent.knowledge.cue_fail"), loggateway.Err(err))
		return "", nil
	}

	filtered := filterCueCollections(collections, scopedIDs)
	var chunks []biz.KnowledgeChunk
	if query != "" {
		chunks = retrieveCueChunks(ctx, query, scopedIDs, filtered, lg)
	}
	return formatKnowledgeCue(filtered, chunks, toolsEnabled, groundedOnly, memoryGrounded)
}

// cueRenderedChunks 过滤出 formatKnowledgeCue 实际渲染的 chunks（非空正文、
// 非空 ID、knowledgeCueTopK 截断）。预算截断在 formatKnowledgeCue 内再裁一次。
func cueRenderedChunks(chunks []biz.KnowledgeChunk) []biz.KnowledgeChunk {
	out := make([]biz.KnowledgeChunk, 0, len(chunks))
	for _, ch := range chunks {
		if len(out) >= knowledgeCueTopK {
			break
		}
		if strings.TrimSpace(ch.Content) == "" || strings.TrimSpace(ch.ID) == "" {
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

// lastUserQuery 只在本轮第一次模型调用（最新非 system / 非动态 cue 消息是
// user）时取查询。工具循环续跑时跳过，避免每轮重复检索拖慢 TTFT。尾部
// user-role 动态 cue 必须跳过，否则会把 cue 正文当成检索词。
func lastUserQuery(msgs []trpcmodel.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if isPromptFixedMessage(msgs[i]) {
			continue
		}
		if msgs[i].Role == trpcmodel.RoleUser {
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
func formatKnowledgeCue(filtered []biz.KnowledgeCollection, chunks []biz.KnowledgeChunk, toolsEnabled, groundedOnly, memoryGrounded bool) (string, []biz.KnowledgeChunk) {
	rendered := cueRenderedChunks(chunks)
	if memoryGrounded {
		// P0（2026-08-21）：本轮已注入 L3 时不再列出知识库目录——目录+「去搜」
		// 会把记忆题拐去 knowledge_search（域 B sh-04 / up-03）。
		filtered = nil
	}
	if groundedOnly && len(rendered) == 0 && !toolsEnabled {
		return "## Retrieved Knowledge\n" +
			"The knowledge base has no passages for this question. You MUST say you do not have evidence in the knowledge base. Do not use world knowledge.\n", nil
	}
	if !toolsEnabled {
		// 无工具：仅"有实质命中"才值得注入。
		if len(rendered) == 0 {
			return "", nil
		}
		filtered = nil
	}
	if groundedOnly {
		// Grounded：目录与「不够就用常识」会诱导世界知识，一律不列。
		filtered = nil
	}
	if memoryGrounded && len(rendered) == 0 {
		return "## Retrieved Knowledge\n" +
			"Matching memory facts were already injected this turn (see ## L2+L3 memory). Answer from those facts. Do not call `knowledge_search` unless they cannot answer the question. If search also returns nothing, say you do not have that record. Do not invent names, report IDs, brands, phone numbers, or personal preferences.\n", nil
	}
	if len(filtered) == 0 && len(chunks) == 0 && !groundedOnly {
		return "", nil
	}
	if groundedOnly && len(rendered) == 0 && toolsEnabled {
		return "## Retrieved Knowledge\n" +
			"Pre-retrieval found no passages. You may call `knowledge_search` or `knowledge_reflect`. If those also return nothing, say the knowledge base has no evidence. Do not use world knowledge.\n", nil
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

	var cited []biz.KnowledgeChunk
	if len(rendered) > 0 {
		header := "## Retrieved Knowledge\n" +
			"The following passages were retrieved for the current user question. Cite them by [n] when using this knowledge."
		if groundedOnly {
			header += " Use ONLY these passages. If they do not answer the question, say the knowledge base has no evidence. Do not use world knowledge."
			if toolsEnabled {
				header += " You may call `knowledge_search` or `knowledge_reflect` for more passages from the knowledge base, then refuse if still insufficient."
			}
		} else if toolsEnabled && !memoryGrounded {
			header += " If they are insufficient, call `knowledge_search` or `knowledge_reflect`."
		} else if toolsEnabled && memoryGrounded {
			header += " Prefer injected L2+L3 memory over these passages when they conflict. Call `knowledge_search` only if neither answers."
		}
		writeBlock(header + "\n\n")
		for i, ch := range rendered {
			content := strings.TrimSpace(ch.Content)
			if utf8.RuneCountInString(content) > knowledgeCueChunkChars {
				content = string([]rune(content)[:knowledgeCueChunkChars]) + "..."
			}
			if !writeBlock(fmt.Sprintf("[%d] (doc=%s score=%.2f)\n%s\n\n", i+1, ch.DocID, ch.Score, content)) {
				break
			}
			cited = append(cited, ch)
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
				"- If initial results are insufficient → `knowledge_reflect` will suggest supplementary queries\n" +
				"- If search returns no chunks, say you do not have that record. Do not invent names, report IDs, brands, phone numbers, or personal preferences.\n")
		}
	}

	return b.String(), cited
}
