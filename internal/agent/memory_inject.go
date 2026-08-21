package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// memoryInjectMarker is a hidden prefix injected into MemoryInject system
// messages so they can be reliably identified during context-compaction
// rebuild.  It is an XML comment so LLMs treat it as inert metadata.
const memoryInjectMarker = "<!-- aranea:memory_inject -->\n"

// proactiveHitsCtxKey is the context key for proactive recall hits (P3-11).
// The service layer stores hits from ProactiveRecall at turn start; the
// before-model hook retrieves them to merge with RecallComposite results.
type proactiveHitsCtxKey struct{}

// WithProactiveHits returns a new context carrying proactive recall hits.
func WithProactiveHits(ctx context.Context, hits []biz.CompositeRecallHit) context.Context {
	return context.WithValue(ctx, proactiveHitsCtxKey{}, hits)
}

// ProactiveHitsFromContext returns the proactive recall hits stored in ctx,
// or nil if none are present.
func ProactiveHitsFromContext(ctx context.Context) []biz.CompositeRecallHit {
	if v, ok := ctx.Value(proactiveHitsCtxKey{}).([]biz.CompositeRecallHit); ok {
		return v
	}
	return nil
}

// isMemoryInjectMessage reports whether msg was injected by the
// MemoryInject before-model hook (identified by the hidden marker prefix).
func isMemoryInjectMessage(msg trpcmodel.Message) bool {
	return strings.HasPrefix(msg.Content, memoryInjectMarker)
}

// memoryInjectCueContent wraps a cue string with the identification marker.
func memoryInjectCueContent(cue string) string {
	return memoryInjectMarker + cue
}

// memoryInjectStripMarker removes the identification marker from a cue string.
func memoryInjectStripMarker(content string) string {
	return strings.TrimPrefix(content, memoryInjectMarker)
}

func memoryRuntimeContext(inv *trpcagent.Invocation, ag biz.Agent) biz.MemoryRuntimeContext {
	rt := biz.MemoryRuntimeContext{
		AgentID: strings.TrimSpace(ag.ID),
	}
	if inv != nil && inv.Session != nil {
		rt.UserID = strings.TrimSpace(inv.Session.UserID)
		rt.Workspace = sessionStateString(inv.Session.State, "workspace")
		rt.TeamID = sessionStateString(inv.Session.State, "team_id")
	}
	// C5: team graph runtime injects team_id into the root invocation's
	// RuntimeState; member invocations inherit it via the graph child state.
	// Fall back to it when the session state carries no team_id.
	if rt.TeamID == "" && inv != nil && inv.RunOptions.RuntimeState != nil {
		if teamID, ok := inv.RunOptions.RuntimeState["team_id"].(string); ok {
			rt.TeamID = strings.TrimSpace(teamID)
		}
	}
	if rt.Workspace == "" && ag.Settings != nil {
		rt.Workspace = strings.TrimSpace(ag.Settings.Workspace)
	}
	return rt
}

func sessionStateString(state map[string][]byte, key string) string {
	if state == nil {
		return ""
	}
	if b, ok := state[key]; ok {
		return strings.TrimSpace(string(b))
	}
	return ""
}

// MemoryCueResult holds the structured output of memory cue building.
type MemoryCueResult struct {
	// ProfileCue is the resident profile card block (FR-12.7): distilled by
	// Sleep-time, injected unconditionally at the first memory-block position.
	ProfileCue string
	// L1Cue is the L1 session summary cue (injectable, changes after compression).
	L1Cue string
	// RecallCue is the combined L2/L3/L4 recall cue (keyword-based, changes every turn).
	RecallCue string
	// RecallHits are the merged composite recall hits injected into RecallCue
	// (R4). Only populated on the composite recall path; used to emit the
	// memory_recalled transparency notice. Nil on L2/L3/L4 standalone paths.
	RecallHits []biz.CompositeRecallHit
	// InjectedFactIDs are the L3 fact IDs actually written into the cue this
	// turn across all three injection paths (pinned preferences, composite
	// recall hits, standalone L3 recall). FR-12.6: the before-model hook
	// increments injected_count for exactly this set once per turn.
	InjectedFactIDs []string
	// L4RecalledEntityIDs are the IDs of the L4 entities actually injected
	// into the cue this turn (design §15.7). Used to trigger memory
	// reconsolidation for exactly the recalled set. Nil when L4 injection is
	// disabled or no entity passed the confidence gate.
	L4RecalledEntityIDs []string
}

// IsEmpty reports whether the result contains any cue content.
func (r *MemoryCueResult) IsEmpty() bool {
	return r.ProfileCue == "" && r.L1Cue == "" && r.RecallCue == ""
}

// JoinCues returns the combined cue text for injection. Block order:
// resident profile card (always-on) → L1 scratchpad summary → recall cues.
func (r *MemoryCueResult) JoinCues() string {
	var parts []string
	if r.ProfileCue != "" {
		parts = append(parts, r.ProfileCue)
	}
	if r.L1Cue != "" {
		parts = append(parts, r.L1Cue)
	}
	if r.RecallCue != "" {
		parts = append(parts, r.RecallCue)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

// memoryCueTruncMarker 是统一预算截断的可见标记（P3-2：块级截断后追加）。
const memoryCueTruncMarker = "…\n[memory cue truncated by prompt budget]"

// JoinCuesWithTokenBudget returns the combined cue text, truncated to the
// given token budget (P2-04). budgetTokens <= 0 means unlimited.
//
// 2026-08-20 token 成本审查（方案D）：统一预算从字符口径改为 token 口径，
// 与 L2/L3 召回块预算同一计量单位（此前字符口径与模型真实计费单位偏差大，
// CJK 文本 4000 chars ≈ 1600+ tokens，预算形同虚设）。估算走共享校准估算器
// llmcontext.EstimateTokensFromChars。
//
// P3-2（2026-08-16）块级截断：超预算时优先整块丢弃尾部块（recall → L1），
// 避免半条事实/半段图谱进 prompt 产生误导；仅当首个块（profile 卡）自身
// 就超预算时才退化为按 rune 硬切。
func (r *MemoryCueResult) JoinCuesWithTokenBudget(budgetTokens int) string {
	combined := r.JoinCues()
	if budgetTokens <= 0 {
		return combined
	}
	combinedRunes := len([]rune(combined))
	combinedEst := llmcontext.EstimateTokensFromChars(combinedRunes)
	if combinedEst <= budgetTokens {
		return combined
	}
	reserve := llmcontext.EstimateTokensFromChars(len([]rune(memoryCueTruncMarker)))
	blocks := make([]string, 0, 3)
	for _, blk := range []string{r.ProfileCue, r.L1Cue, r.RecallCue} {
		if blk != "" {
			blocks = append(blocks, blk)
		}
	}
	kept := make([]string, 0, len(blocks))
	used := 0
	for _, blk := range blocks {
		n := llmcontext.EstimateTokensFromChars(len([]rune(blk)))
		if len(kept) > 0 {
			n += 1 // "\n\n" 分隔符 ≈ 1 token
		}
		// +1：输出形态是 join(kept)+"\n"+marker，marker 前的换行也占预算。
		if used+n+reserve+1 > budgetTokens {
			break
		}
		kept = append(kept, blk)
		used += n
	}
	if len(kept) > 0 {
		return strings.Join(kept, "\n\n") + "\n" + memoryCueTruncMarker
	}
	// 首个块就超预算：硬切。切点按本段文本的真实 token/rune 比率换算，
	// 避免固定比率对 CJK/英文混排的系统性误判；按 rune 切，不劈开多字节字符。
	runes := []rune(combined)
	cut := 0
	if combinedEst > 0 {
		cut = (budgetTokens - reserve) * combinedRunes / combinedEst
	}
	if cut < 0 {
		cut = 0
	}
	if cut > len(runes) {
		cut = len(runes)
	}
	return string(runes[:cut]) + memoryCueTruncMarker
}

func newMemoryInjectBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled || !policy.AnyInject() {
		return nil
	}
	hasDep := (policy.InjectL1 && deps.L1Reader != nil) || (policy.InjectL4 && deps.L4Entities != nil)
	hasDep = hasDep || (policy.RecallL2 && deps.MemoryL2Recall != nil)
	hasDep = hasDep || (policy.InjectL3 && deps.MemoryL3Recall != nil)
	hasDep = hasDep || (policy.RecallL2 && policy.InjectL3 && deps.MemoryCompositeRecall != nil)
	hasDep = hasDep || (policy.InjectL3 && deps.MemoryPreferenceLister != nil)
	hasDep = hasDep || (policy.InjectL3 && deps.MemoryProfileCardReader != nil)
	hasDep = hasDep || deps.AgentCaseRecaller != nil
	if !hasDep {
		return nil
	}
	return callbacks.NewBeforeModelHook(5, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// E 预算表分解（P0-A，2026-08-11）：记忆 cue 构建在 LLM 关键路径上，
		// 含多次 DB 读 + embedding，计时以便从 TTFT 黑盒中拆出本段耗时。
		cueStart := time.Now()
		result, fresh := buildRuntimeMemoryCue(ctx, deps, ag, args.Request.Messages)
		cueElapsed := time.Since(cueStart)
		sessionID := ""
		if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.Session != nil {
			sessionID = strings.TrimSpace(inv.Session.ID)
		}
		deps.Logger().Info("memory cue build timing",
			loggateway.StepID("agent.memory_cue.build"),
			loggateway.SessionID(sessionID),
			loggateway.Duration(cueElapsed.Milliseconds()),
			loggateway.Int("cue_chars", len([]rune(result.JoinCues()))),
			loggateway.Int("recall_hits", len(result.RecallHits)),
			loggateway.Int("injected_facts", len(result.InjectedFactIDs)),
			loggateway.Bool("cache_hit", !fresh),
			loggateway.Bool("empty", result.IsEmpty()))
		if result.IsEmpty() {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// P2-3（2026-08-16）：召回透明 notice / injected_count / L4 重巩固
		// 只在 fresh 轮（本 turn 首次真实召回）触发。此前用 ctx.Value 标记
		// 防重，但框架工具循环每轮以外层 ctx 重进 hook（llmflow.runOneStep），
		// ctx 标记跨轮即失效——副作用每轮重复触发。缓存轮直接复用结果，
		// 天然 once-per-turn。
		if fresh {
			// R4: surface the injected recall hits to the chat UI before mutating
			// the request — the notice must reflect what this turn actually used.
			ctx = emitMemoryRecalledNotice(ctx, result.RecallHits)
			// FR-12.6: bump injected_count for the facts actually written into the
			// prompt (async, once per turn — must never block the model call).
			ctx = bumpFactInjectedCounts(ctx, deps, result.InjectedFactIDs)
			// FR-10.5: trigger memory reconsolidation for the recalled L4 entities
			// (async, once per turn — must never block the model call).
			ctx = triggerL4Reconsolidation(ctx, deps, result.L4RecalledEntityIDs)
		}
		// P2-04: apply unified prompt budget across L1+L2+L3+L4 cues.
		// 2026-08-20（方案D）：token 口径，与 L2/L3 召回块预算同一单位。
		cue := result.JoinCuesWithTokenBudget(policy.MemoryPromptTotalBudgetTokens)
		// P2 TTFT + DeepSeek system-prefix: append as a user-role cue at the
		// END so the [system block + history + user] prefix stays cacheable
		// and the cue does not enter the provider's system prefix.
		args.Request.Messages = appendDynamicCue(args.Request.Messages, memoryInjectCueContent(cue))
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// memoryCueTurnCacheStateKey 是 P2-3 per-turn 记忆 cue 缓存在 invocation
// state 中的键。框架工具循环（llmflow.runOneStep）每轮以同一外层 ctx 重进
// BeforeModel hook，ctx.Value 无法跨轮携带状态；invocation state 随整个
// run 存活，是唯一不改框架的跨轮载体。
const memoryCueTurnCacheStateKey = "aranea.memory_cue.turn_cache"

// memoryCueTurnCache 缓存一个 turn 内昂贵且基本不变的召回结果（profile 卡、
// project-state/pinned/composite/case/L4 融合块）。L1 工作记忆不缓存——
// working_memory 工具可能在工具循环中修改它，每轮重建保持新鲜（2 次轻量
// DB 读，远小于 embed+多路检索的开销）。
type memoryCueTurnCache struct {
	keyword             string
	profileCue          string
	recallCue           string
	recallHits          []biz.CompositeRecallHit
	injectedFactIDs     []string
	l4RecalledEntityIDs []string
}

// buildRuntimeMemoryCue 返回 cue 结果与 fresh 标记：fresh=true 表示本轮做了
// 真实召回（结果已写入 invocation 缓存），false 表示复用缓存。调用方据此
// 把召回副作用（notice/计数/重巩固）限制为 once-per-turn。
func buildRuntimeMemoryCue(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent, messages []trpcmodel.Message) (*MemoryCueResult, bool) {
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled {
		return &MemoryCueResult{}, false
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return &MemoryCueResult{}, false
	}
	rt := memoryRuntimeContext(inv, ag)
	sessionID := strings.TrimSpace(inv.Session.ID)
	keyword := RecallKeywordFromMessages(messages)

	// P2-3 缓存命中：复用召回块，仅重建 L1。
	if v, ok := inv.GetState(memoryCueTurnCacheStateKey); ok {
		if c, ok := v.(*memoryCueTurnCache); ok && c != nil && c.keyword == keyword {
			result := &MemoryCueResult{
				ProfileCue:          c.profileCue,
				RecallCue:           c.recallCue,
				RecallHits:          c.recallHits,
				InjectedFactIDs:     c.injectedFactIDs,
				L4RecalledEntityIDs: c.l4RecalledEntityIDs,
			}
			if policy.InjectL1 {
				if l1 := L1MemoryCue(ctx, deps.L1Reader, ag, policy, sessionID, deps.LG); l1 != nil {
					result.L1Cue = l1.Cue
					recordContextBudgetOnce(ctx, ContextBudgetCategoryMemoryL1, utf8.RuneCountInString(l1.Cue))
				}
			}
			knowledgetool.MarkMemoryL3Grounded(ctx, memoryCueHasL3(result))
			return result, false
		}
	}

	result := &MemoryCueResult{}

	// FR-12.7: resident profile card — first memory-block position, no recall
	// scoring, always injected when L3 injection is enabled.
	if policy.InjectL3 {
		if card := ProfileCardCue(ctx, deps.MemoryProfileCardReader, rt.AgentID, rt.UserID); card != "" {
			result.ProfileCue = card
		}
	}

	// L1: session summary (changes after compression rebuild)
	var l1FieldValues []string
	if policy.InjectL1 {
		if l1 := L1MemoryCue(ctx, deps.L1Reader, ag, policy, sessionID, deps.LG); l1 != nil {
			result.L1Cue = l1.Cue
			l1FieldValues = l1.FieldValues
			// 上下文预算台账（29-token §9.6）：仅计量，不改注入逻辑。
			recordContextBudgetOnce(ctx, ContextBudgetCategoryMemoryL1, utf8.RuneCountInString(l1.Cue))
		}
	}

	// L2/L3/L4: recall-based cues (keyword-driven, changes every turn)
	var recallParts []string
	// P2-4 中期 project-state：team graph runtime 把结构化项目状态带进成员
	// invocation（node-start RuntimeState 快照 / session 回退），按切片预算
	// 注入，替代长任务对话历史全量拼接。块序：L1 之后、recall 之前——它是
	// "当前正在发生什么"，优先于按关键词召回的历史记忆。未携带时返回 ""。
	if slice := ProjectStateCueFromInvocation(inv, projectStateCueBudgetRunes); slice != "" {
		recallParts = append(recallParts, slice)
	}
	// FR-M3: pinned preference/constraint block precedes recall blocks — no
	// vector scoring, always injected when L3 injection is enabled.
	if policy.InjectL3 && deps.MemoryPreferenceLister != nil {
		if pinned, pinnedIDs := PinnedPreferenceCueWithIDs(ctx, deps.MemoryPreferenceLister, rt.AgentID, rt.UserID); pinned != "" {
			recallParts = append(recallParts, pinned)
			result.InjectedFactIDs = append(result.InjectedFactIDs, pinnedIDs...)
		}
	}
	if policy.RecallL2 && policy.InjectL3 && deps.MemoryCompositeRecall != nil {
		proactiveHits := ProactiveHitsFromContext(ctx)
		if composite, hits := CompositeMemoryCueWithHits(ctx, deps.MemoryCompositeRecall, ag, policy, rt, sessionID, keyword, 0, proactiveHits); composite != "" {
			recallParts = append(recallParts, composite)
			result.RecallHits = hits
			// 上下文预算台账（29-token §9.6）：仅计量，不改注入逻辑。
			recordContextBudgetOnce(ctx, ContextBudgetCategoryMemoryComposite, utf8.RuneCountInString(composite))
			for _, h := range hits {
				if h.Layer == "L3" && strings.TrimSpace(h.FactID) != "" {
					result.InjectedFactIDs = append(result.InjectedFactIDs, strings.TrimSpace(h.FactID))
				}
			}
		}
	} else {
		if policy.RecallL2 {
			if l2 := L2MemoryCue(ctx, deps.MemoryL2Recall, ag, policy, sessionID, keyword, 0, deps.LG); l2 != "" {
				recallParts = append(recallParts, l2)
			}
		}
		if policy.InjectL3 {
			if l3, l3IDs := L3MemoryCueWithIDs(ctx, deps.MemoryL3Recall, ag, policy, rt, keyword, 0, l1FieldValues, deps.LG); l3 != "" {
				recallParts = append(recallParts, l3)
				result.InjectedFactIDs = append(result.InjectedFactIDs, l3IDs...)
			}
		}
	}
	// P3 M3: Agent Case 召回（任务经验），与 L2/L3 并列、位于 L4 之前。
	if caseCue := CaseMemoryCue(ctx, deps.AgentCaseRecaller, rt.AgentID, keyword); caseCue != "" {
		recallParts = append(recallParts, caseCue)
	}
	if policy.InjectL4 {
		if l4, entityIDs := L4MemoryCue(ctx, deps.L4Entities, ag, policy, keyword, deps.LG); l4 != "" {
			recallParts = append(recallParts, l4)
			result.L4RecalledEntityIDs = entityIDs
			// 上下文预算台账（29-token §9.6）：仅计量，不改注入逻辑。
			recordContextBudgetOnce(ctx, ContextBudgetCategoryMemoryL4, utf8.RuneCountInString(l4))
		}
	}
	if len(recallParts) > 0 {
		result.RecallCue = strings.TrimSpace(strings.Join(recallParts, "\n\n"))
	}

	// P2-3：写入 per-turn 缓存。工具循环后续轮次（同一 invocation、keyword
	// 不变）命中缓存直接复用召回块，embed+多路检索每 turn 只做一次。
	inv.SetState(memoryCueTurnCacheStateKey, &memoryCueTurnCache{
		keyword:             keyword,
		profileCue:          result.ProfileCue,
		recallCue:           result.RecallCue,
		recallHits:          result.RecallHits,
		injectedFactIDs:     result.InjectedFactIDs,
		l4RecalledEntityIDs: result.L4RecalledEntityIDs,
	})
	knowledgetool.MarkMemoryL3Grounded(ctx, memoryCueHasL3(result))
	return result, true
}

func memoryCueHasL3(r *MemoryCueResult) bool {
	if r == nil {
		return false
	}
	if len(r.InjectedFactIDs) > 0 {
		return true
	}
	for _, h := range r.RecallHits {
		if strings.EqualFold(strings.TrimSpace(h.Layer), "L3") {
			return true
		}
	}
	return false
}

func lastUserMessageText(messages []trpcmodel.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != trpcmodel.RoleUser || isDynamicCueMessage(messages[i]) {
			continue
		}
		if t := strings.TrimSpace(messages[i].Content); t != "" {
			// P1-1: no truncation here — cleanRecallQuery owns the 120-rune
			// budget (truncating here would cut off the tail question before
			// segmentation, defeating question-last packing).
			return t
		}
	}
	return ""
}

// NOTE (N8, 2026-08-13 链路审查；2026-08-14 P0-A 更新): 压缩后无需单独的
// memory 重建入口——MemoryInject 是 BeforeModel hook（priority 5），在框架压缩
// （llmflow.maybeCompactContextBeforeLLM）之后执行；Aranea 终审压缩闸门
// （priority 9，注入后计数）会在 memory/knowledge cue 注入后对整条请求做
// token 口径截断，极端溢出时按降级链丢弃尾部 cue。压缩轮次会用最新
// L1/L2/L3/L4 数据重建完整 cue 并追加到消息尾部。memory-inject 消息是
// 请求级装饰，从不持久化为 session event，框架重建的 request 不会残留旧
// cue。原 RebuildMemoryInjectForCompaction 的"原地打补丁"场景不成立，且若
// 接入框架 tail-processor 槽位会与本 hook 双重注入，已删除。

// ── memory_recalled transparency notice (R4) ─────────────────────────────

// memoryRecalledNoticeType is the NoticeType for recall-transparency notices.
// The chat UI renders the injected memory hits below the assistant turn.
const memoryRecalledNoticeType = "memory_recalled"

const (
	// memoryRecalledMaxHits caps the hits carried by one notice payload.
	memoryRecalledMaxHits = 10
	// memoryRecalledMaxLineRunes caps one hit line inside the payload.
	memoryRecalledMaxLineRunes = 120
)

// memoryRecalledHit is one recall hit inside the notice payload.
type memoryRecalledHit struct {
	Layer      string  `json:"layer"`
	Line       string  `json:"line"`
	Score      float64 `json:"score"`
	FactID     string  `json:"fact_id,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Version    int     `json:"version,omitempty"`
}

// memoryRecalledNoticePayload is the JSON content of a memory_recalled notice.
type memoryRecalledNoticePayload struct {
	Hits []memoryRecalledHit `json:"hits"`
}

// memoryRecalledEmittedKey marks a ctx in which the notice was already
// emitted, so tool-loop re-entries of the before-model hook do not re-emit.
type memoryRecalledEmittedKey struct{}

// emitMemoryRecalledNotice emits one memory_recalled notice carrying the
// injected recall hits (best-effort, Informational per AS-EVT-01). It is a
// no-op when there are no hits, no ActivityEmitter is present in ctx, or the
// ctx was already marked by an earlier hook invocation in the same turn.
// Returns the (possibly marked) ctx.
func emitMemoryRecalledNotice(ctx context.Context, hits []biz.CompositeRecallHit) context.Context {
	if len(hits) == 0 {
		return ctx
	}
	if ctx.Value(memoryRecalledEmittedKey{}) != nil {
		return ctx
	}
	emitter := biz.ActivityEmitterFromContext(ctx)
	if emitter == nil {
		return ctx
	}
	payload := memoryRecalledNoticePayload{Hits: make([]memoryRecalledHit, 0, len(hits))}
	for i, h := range hits {
		if i >= memoryRecalledMaxHits {
			break
		}
		line := strings.TrimSpace(h.Line)
		if line == "" {
			continue
		}
		if runes := []rune(line); len(runes) > memoryRecalledMaxLineRunes {
			line = string(runes[:memoryRecalledMaxLineRunes]) + "…"
		}
		payload.Hits = append(payload.Hits, memoryRecalledHit{
			Layer:      strings.TrimSpace(h.Layer),
			Line:       line,
			Score:      h.Score,
			FactID:     strings.TrimSpace(h.FactID),
			Confidence: h.Confidence,
			Version:    h.Version,
		})
	}
	if len(payload.Hits) == 0 {
		return ctx
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return ctx
	}
	// Emit failures are non-fatal: the notice is transparency-only and must
	// never break a turn (same contract as PinnedPreferenceCue degradation).
	if err := emitter.EmitNotice(ctx, string(content), memoryRecalledNoticeType); err != nil {
		return ctx
	}
	return context.WithValue(ctx, memoryRecalledEmittedKey{}, true)
}

// ── FR-12.6: injected_count bump (three-stage counters, report §6.5) ─────

// factInjectCountedKey marks a ctx in which injected_count was already
// bumped, so tool-loop re-entries of the before-model hook do not re-count
// the same facts within one turn.
type factInjectCountedKey struct{}

// bumpFactInjectedCounts asynchronously increments injected_count for the
// facts actually written into the prompt this turn. Per FR-12.6 this is the
// only "usage" count shown to users: recalled ≠ injected ≠ cited. The bump
// must never block the model call (safego.Go + background context). At most
// one bump per turn: the returned ctx carries a marker so subsequent hook
// invocations in the same turn (tool loop, multi-round model calls) skip it.
// No-op when no facts were injected or the counter is not wired.
func bumpFactInjectedCounts(ctx context.Context, deps TRPCBuilderDeps, factIDs []string) context.Context {
	if len(factIDs) == 0 || deps.MemoryFactInjectCounter == nil {
		return ctx
	}
	if ctx.Value(factInjectCountedKey{}) != nil {
		return ctx
	}
	counter := deps.MemoryFactInjectCounter
	ids := append([]string(nil), factIDs...)
	lg := deps.Logger()
	safego.Go(ctx, "memory.fact_inject_count", func() {
		if err := counter.IncrementFactInjectedCount(context.Background(), ids); err != nil {
			lg.Warn("L3 事实注入计数失败", loggateway.StepID("agent.memory_inject_count_fail"), loggateway.Err(err))
		}
	})
	return context.WithValue(ctx, factInjectCountedKey{}, true)
}

// ── L4 memory reconsolidation trigger (design §15.7, FR-10.5) ──────────

// l4ReconsolidatedKey marks a ctx in which L4 reconsolidation was already
// triggered, so tool-loop re-entries of the before-model hook do not
// re-boost the same entities within one turn.
type l4ReconsolidatedKey struct{}

// triggerL4Reconsolidation asynchronously fires OnRecall for each recalled L4
// entity, boosting its activation, incrementing use_count, and reinforcing
// connections to co-recalled entities via the Hebbian rule. Per design §15.7
// the trigger must never block the model call, hence safego.Go + background
// context. At most one trigger per turn: the returned ctx carries a marker so
// subsequent hook invocations in the same turn (tool loop, multi-round model
// calls) skip it. No-op when there are no recalled entities or the
// reconsolidator is not wired.
func triggerL4Reconsolidation(ctx context.Context, deps TRPCBuilderDeps, entityIDs []string) context.Context {
	if len(entityIDs) == 0 || deps.MemoryReconsolidator == nil {
		return ctx
	}
	if ctx.Value(l4ReconsolidatedKey{}) != nil {
		return ctx
	}
	reconsolidator := deps.MemoryReconsolidator
	ids := append([]string(nil), entityIDs...)
	lg := deps.Logger()
	safego.Go(ctx, "memory.reconsolidate", func() {
		bg := context.Background()
		for _, id := range ids {
			others := make([]string, 0, len(ids)-1)
			for _, o := range ids {
				if o != id {
					others = append(others, o)
				}
			}
			if err := reconsolidator.OnRecall(bg, id, others); err != nil {
				lg.Warn("L4 记忆重巩固失败", loggateway.StepID("agent.memory_reconsolidate_fail"), loggateway.Err(err))
			}
		}
	})
	return context.WithValue(ctx, l4ReconsolidatedKey{}, true)
}
