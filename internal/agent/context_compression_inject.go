package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// compressionMetaStateKey is the invocation-state key for the most recent
// compression metadata. The L0 snapshot hook reads this to populate
// TruncateStrategy / TruncatedMessageCount on the snapshot row.
const compressionMetaStateKey = "aranea:compression_meta"

// CompressionMeta records the outcome of the most recent compression pass.
// Stored in invocation state so the L0 snapshot hook (priority 10) can read
// what the compression hook (priority 9) produced.
type CompressionMeta struct {
	// Occurred is true when compression ran successfully this model-call.
	Occurred bool
	// EvictedCount is the number of messages dropped by emergency truncation.
	EvictedCount int
	// DroppedCueCount is the number of tail dynamic cues dropped by the
	// degradation chain when history eviction alone could not reach the
	// truncation target (P0-C).
	DroppedCueCount int
	// BeforeChars is the total char count of evicted messages.
	BeforeChars int
	// AfterChars is 0 for deterministic truncation (no summary is produced).
	AfterChars int
	// SummaryText is always empty for deterministic truncation (kept for the
	// L0 snapshot overlay contract).
	SummaryText string
}

// truncationTargetFactor is the hysteresis applied under the hard trigger
// ratio: when the gate fires it truncates down to threshold×factor of the
// context window, so the next model call does not immediately re-trigger.
const truncationTargetFactor = 0.9

// newContextCompressionBeforeHook creates the FINAL-REVIEW compression gate:
// a BeforeModel hook at priority 9 (LayerDynamic) that runs AFTER every
// injection hook (memory 5 / knowledge 6 / cues) and BEFORE the prompt
// snapshot (10), so its token accounting covers the fully-injected request
// (P0-A: 注入后计数).
//
// Architecture (ADR: dual-compression unification, 2026-07-20; final-review
// gate 2026-08-14):
//   - This hook is the intra-turn last-ditch guard: it fires before every
//     model call (including mid-turn tool loops that the Session Compressor
//     cannot intercept) and mechanically drops the oldest messages with
//     tool-pair-safe boundaries. NO LLM call, NO summary — deterministic,
//     no failure modes.
//   - The Session Compressor (internal/session) owns inter-turn durable
//     compression with LLM rolling summaries, quality gates, suppression
//     and caching.
//
// Gate pipeline when the post-injection ratio crosses the per-agent hard
// threshold (HardTriggerRatio, default 0.90):
//  1. History truncation (token-budget口径, P0-D): evict the oldest
//     conversation messages until the estimate fits threshold×targetFactor
//     of the window (reserving the marker's own tokens up front). System
//     messages and trailing dynamic cues are never evicted by this stage.
//  2. Degradation chain (P0-C): if the estimate is still over target
//     (oversized dynamic tail cues), drop trailing cue messages
//     largest-first until the estimate fits or no droppable cue remains.
//  3. Re-verify: the post-gate estimate is recomputed; if it still exceeds
//     the target with nothing left to evict/drop (oversized static head),
//     a Warn is emitted (K3) and the request proceeds unmodified-beyond-
//     what-was-done — the provider call is never blocked by this hook.
//
// The truncation marker is placed at the true eviction boundary — after the
// head (static) system run and BEFORE the first surviving conversation
// message — never after the dynamic tail cues (P0-D marker 落点修正).
//
// Returns nil when compression is disabled for the agent
// (L0SnapshotMode=off without ContextCompactionEnabled).
func newContextCompressionBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if !contextCompressionEnabledForAgent(ag) {
		return nil
	}
	threshold := hardTriggerRatioForAgent(ag)
	return callbacks.NewBeforeModelHook(9, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		lg := deps.Logger()
		// 1. Estimate token usage ratio over the FULLY-INJECTED request.
		report := analyzePromptRequest(args.Request.Messages)
		win := resolveCompressionContextWindow(ctx, deps, ag)
		ratio := llmcontext.ContextRatio(report.EstTokens, win)
		// 2. Hard threshold gate.
		if ratio < threshold {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		target := int(float64(win) * threshold * truncationTargetFactor)
		// Capture the zones before mutation: the marker sits at headLen (the
		// true eviction boundary); the degradation chain may only drop from
		// the tail zone (dynamic trailing system cues), never the static
		// head, never conversation messages. Positions must come from this
		// split — when the entire conversation is evicted, head+tail merge
		// into one system run and a "leading system run" scan would
		// misplace the marker after the tail cues.
		head, conv, tail := splitPromptZones(args.Request.Messages)
		// 3. History truncation: reserve the marker's tokens up front so the
		// post-insertion estimate stays under target in a single pass.
		markerReserve := estTokensFromChars(utf8.RuneCountInString(buildTruncationMarker(len(conv))))
		keepMsgs, evictedMsgs := partitionMessagesByTokenBudget(args.Request.Messages, target-markerReserve)
		beforeChars := 0
		for _, m := range evictedMsgs {
			beforeChars += len(m.Content)
		}
		out := keepMsgs
		if len(evictedMsgs) > 0 {
			marker := trpcmodel.NewSystemMessage(buildTruncationMarker(len(evictedMsgs)))
			out = insertMarkerAfterHead(keepMsgs, len(head), marker)
			// 上下文预算台账（29-token §9.6）：仅计量，不改注入逻辑。
			recordContextBudgetOnce(ctx, ContextBudgetCategoryOtherDynamic, utf8.RuneCountInString(marker.Content))
		}
		// 4. Degradation chain: drop tail dynamic cues (largest first) while
		// the estimate still exceeds target. Tail cues occupy the final
		// len(tail) positions of `out` (head + marker? + keptConv + tail).
		droppedCues := 0
		for remaining := len(tail); remaining > 0 && analyzePromptRequest(out).EstTokens > target; remaining-- {
			tailStart := len(out) - remaining
			largest := tailStart
			largestTok := -1
			for i := tailStart; i < len(out); i++ {
				if tok := estTokensFromChars(messageCharLen(out[i])); tok > largestTok {
					largestTok = tok
					largest = i
				}
			}
			out = append(out[:largest], out[largest+1:]...)
			droppedCues++
		}
		if len(evictedMsgs) == 0 && droppedCues == 0 {
			// Nothing evictable/droppable (oversized static head) — never
			// block the call; surface for ops instead (K3).
			lg.Warn("context over hard threshold but nothing evictable",
				loggateway.StepID("agent.context.compress"),
				loggateway.Phase("degraded"),
				loggateway.Any("used_ratio", ratio),
				loggateway.Int("est_tokens", report.EstTokens),
				loggateway.Int("context_window", win))
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		args.Request.Messages = out
		// 5. Re-verify (post-gate accounting for the log + snapshot overlay).
		finalEst := analyzePromptRequest(out).EstTokens
		if finalEst > target {
			lg.Warn("context still over target after truncation+degradation",
				loggateway.StepID("agent.context.compress"),
				loggateway.Phase("degraded"),
				loggateway.Int("est_tokens", finalEst),
				loggateway.Int("target_tokens", target))
		}
		// 6. Store compression metadata for the L0 snapshot hook.
		storeCompressionMeta(ctx, CompressionMeta{
			Occurred:        true,
			EvictedCount:    len(evictedMsgs),
			DroppedCueCount: droppedCues,
			BeforeChars:     beforeChars,
			AfterChars:      0,
			SummaryText:     "",
		})
		// R7（G-1）：压缩事件结构化持久化——此前仅运行时日志，run 结束后无法
		// 回溯「第几轮触发了什么」。双写 system_guard 决策记录（outcome=
		// truncated），run 统计经 decision_records 聚合回放。
		emitCompactGateDecision(ctx, deps.DecisionCollector, len(evictedMsgs), droppedCues, beforeChars, report.EstTokens, win)
		lg.Info("context emergency truncation completed",
			loggateway.StepID("agent.context.compress"),
			loggateway.Phase("done"),
			loggateway.Int("evicted_count", len(evictedMsgs)),
			loggateway.Int("dropped_cue_count", droppedCues),
			loggateway.Int("before_chars", beforeChars),
			loggateway.Int("est_tokens", finalEst),
			loggateway.Int("target_tokens", target),
			loggateway.Any("used_ratio", ratio))
		// E7b: Codex mid-turn compact re-injects world state
		// BeforeLastUserMessage after eviction. Restore the last snapshot
		// when history or tail cues were dropped.
		if len(evictedMsgs) > 0 || droppedCues > 0 {
			args.Request.Messages = reinjectWorldStateAfterCompact(ctx, args.Request.Messages)
		}
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// emitCompactGateDecision 把一次实际终审压缩双写为 system_guard 决策记录
// （R7 G-1）。run 归属经 gateRunID（2026-08-27 H5 根修）：team 图谱成员节点
// 取 ctx 注入的 team run id（框架 Clone 补丁后 InvocationID 已非 run.ID）。
// observed_value 记驱逐消息数；before_chars/dropped_cues/est_tokens/window
// 落 metadata 供取证回放（不进 stats 聚合字段）。
func emitCompactGateDecision(ctx context.Context, c decision.Collector, evicted, droppedCues, beforeChars, estTokens, window int) {
	runID := gateRunID(ctx)
	event.EmitGate(ctx, c, decision.GateDecision{
		TriggerRule:   decision.TriggerContextCompacted,
		Outcome:       "truncated",
		Scenario:      "上下文终审压缩",
		Reasoning:     fmt.Sprintf("注入后估算 %d tokens 超硬阈值（窗口 %d），驱逐历史 %d 条 / 丢弃尾部 cue %d 条", estTokens, window, evicted, droppedCues),
		GuardName:     "context_compression",
		RunID:         runID,
		SessionID:     gateSessionID(ctx),
		ObservedValue: evicted,
		Threshold:     window,
		Action:        "truncate",
		Extra:         map[string]any{"before_chars": beforeChars, "dropped_cues": droppedCues, "est_tokens": estTokens},
	})
}

// contextCompressionEnabledForAgent mirrors sessionCompressEnabled semantics
// (internal/session/compress_policy.go): compression is on by default;
// L0SnapshotMode=off disables it unless ContextCompactionEnabled explicitly
// opts in. Duplicated locally to avoid an agent → session package dependency.
func contextCompressionEnabledForAgent(ag biz.Agent) bool {
	if ag.Settings == nil {
		return true
	}
	if strings.ToLower(strings.TrimSpace(ag.Settings.L0SnapshotMode)) == "off" && !ag.Settings.ContextCompactionEnabled {
		return false
	}
	return true
}

// hardTriggerRatioForAgent resolves the emergency-truncation threshold from
// per-agent settings, mirroring CompressPolicy.Threshold.HardTriggerRatio.
func hardTriggerRatioForAgent(ag biz.Agent) float64 {
	ratio := biz.DefaultHardTriggerRatio
	if ag.Settings != nil && ag.Settings.HardTriggerRatio > 0 {
		ratio = ag.Settings.HardTriggerRatio
	}
	if usesConversationalContextBudget(ag) {
		// Absolute soft top ~32K on a 64K conversational window, not 90%×256K.
		chat := float64(conversationalCompressSoftTopTokens) / float64(conversationalCompressWindowTokens)
		if chat < ratio {
			ratio = chat
		}
	}
	return ratio
}

// splitPromptZones splits the message list into three zones:
//   - head: the leading run of system messages (static prefix — identity,
//     instructions, session-stable cues)
//   - conv: everything between head and tail (user/assistant/tool history,
//     plus any system message sandwiched mid-conversation)
//   - tail: the trailing run of dynamic cues (user-role sentinels and any
//     leftover system messages appended after the last user message)
//
// An all-system list is treated as entirely head (nothing evictable).
func splitPromptZones(messages []trpcmodel.Message) (head, conv, tail []trpcmodel.Message) {
	firstConv, lastConv := -1, -1
	for i, m := range messages {
		if !isPromptFixedMessage(m) {
			if firstConv < 0 {
				firstConv = i
			}
			lastConv = i
		}
	}
	if firstConv < 0 {
		return messages, nil, nil
	}
	return messages[:firstConv], messages[firstConv : lastConv+1], messages[lastConv+1:]
}

// partitionMessagesByTokenBudget splits the message list by a TOKEN budget
// (P0-D: token 口径截断, replacing the old message-count keepRatio):
//   - keepMsgs: all system messages + the newest conversation messages whose
//     estimated tokens fit the budget after the fixed system cost
//   - evictedMsgs: the oldest conversation messages that did not fit
//
// System messages and trailing dynamic cues (head static block AND
// dynamic tail cues) are never evicted by this function — their cost is
// subtracted from the budget up front; runaway tail cues are the
// degradation chain's job. Eviction walks
// conversation messages newest→oldest and stops at the first message that
// would exceed the remaining budget, then snaps the boundary so no
// tool-call / tool-result pair is split (pairs move to the keep side).
func partitionMessagesByTokenBudget(messages []trpcmodel.Message, budgetTokens int) (keepMsgs, evictedMsgs []trpcmodel.Message) {
	if len(messages) == 0 {
		return nil, nil
	}
	fixed := 0
	var convIdx []int
	for i, m := range messages {
		if isPromptFixedMessage(m) {
			fixed += estTokensFromChars(messageCharLen(m))
		} else {
			convIdx = append(convIdx, i)
		}
	}
	convBudget := budgetTokens - fixed
	if convBudget < 0 {
		convBudget = 0
	}
	// Walk conversation newest→oldest, accumulating until the budget breaks.
	keepFrom := len(convIdx)
	acc := 0
	for k := len(convIdx) - 1; k >= 0; k-- {
		tok := estTokensFromChars(messageCharLen(messages[convIdx[k]]))
		if acc+tok > convBudget {
			break
		}
		acc += tok
		keepFrom = k
	}
	if keepFrom == 0 {
		return messages, nil
	}
	evictSet := make(map[int]bool, keepFrom)
	for k := 0; k < keepFrom; k++ {
		evictSet[convIdx[k]] = true
	}
	// Tool-pair safety: snap the evict boundary so that no tool_call /
	// tool_result pair is split across the keep/evicted boundary.
	evictSet = snapToSafeBoundary(messages, evictSet)

	keepMsgs = make([]trpcmodel.Message, 0, len(messages)-len(evictSet))
	for i, m := range messages {
		if !evictSet[i] {
			keepMsgs = append(keepMsgs, m)
		} else {
			evictedMsgs = append(evictedMsgs, m)
		}
	}
	return keepMsgs, evictedMsgs
}

// snapToSafeBoundary adjusts the evict set so that no tool-call / tool-result
// pair is split across the keep/evicted boundary. The strategy is: if an
// assistant message with ToolCalls is in the evict set but its corresponding
// tool-result is not (or vice versa), remove the entire pair from the evict
// set (moving them to the keep side). This is the safer choice because the
// keep side contains the most recent context that the LLM sees.
func snapToSafeBoundary(messages []trpcmodel.Message, evictSet map[int]bool) map[int]bool {
	// Build index maps for tool calls and tool results.
	// toolCallIdx[toolCallID] = index of the assistant message containing this tool call.
	toolCallIdx := make(map[string]int)
	// toolResultIdx[toolCallID] = index of the tool result message.
	toolResultIdx := make(map[string]int)
	for i, m := range messages {
		for _, tc := range m.ToolCalls {
			toolCallIdx[tc.ID] = i
		}
		if m.Role == trpcmodel.RoleTool && m.ToolID != "" {
			toolResultIdx[m.ToolID] = i
		}
	}

	// Find pairs that are split by the evict boundary.
	// Iterate until no more changes (a single assistant message may have
	// multiple tool calls whose results are scattered across the boundary).
	for changed := true; changed; {
		changed = false
		for id, callIdx := range toolCallIdx {
			resultIdx, hasResult := toolResultIdx[id]
			if !hasResult {
				continue
			}
			callEvicted := evictSet[callIdx]
			resultEvicted := evictSet[resultIdx]
			if callEvicted != resultEvicted {
				// Pair is split — keep both in the keep side.
				delete(evictSet, callIdx)
				delete(evictSet, resultIdx)
				changed = true
			}
		}
	}
	return evictSet
}

// buildTruncationMarker wraps the truncation notice in a labelled block so
// the LLM can distinguish it from other system instructions.
func buildTruncationMarker(evictedCount int) string {
	return fmt.Sprintf("<context_truncated>\n%d earlier messages were removed to fit the context window.\n</context_truncated>", evictedCount)
}

// insertMarkerAfterHead inserts the truncation marker at position headLen —
// right after the head (static) system zone captured by splitPromptZones,
// i.e. BEFORE the first surviving conversation message and before any
// dynamic tail cue. The position must be passed explicitly: when the whole
// conversation was evicted, head and tail merge into a single system run
// and scanning for the "leading system run" would skip past the tail cues.
func insertMarkerAfterHead(msgs []trpcmodel.Message, headLen int, marker trpcmodel.Message) []trpcmodel.Message {
	if headLen < 0 {
		headLen = 0
	}
	if headLen > len(msgs) {
		headLen = len(msgs)
	}
	out := make([]trpcmodel.Message, 0, len(msgs)+1)
	out = append(out, msgs[:headLen]...)
	out = append(out, marker)
	out = append(out, msgs[headLen:]...)
	return out
}

// insertAfterLastSystem inserts the marker message after the last system
// message in msgs (position where the evicted middle used to be). Falls back
// to the front when no system message exists.
func insertAfterLastSystem(msgs []trpcmodel.Message, marker trpcmodel.Message) []trpcmodel.Message {
	pos := 0
	for i, m := range msgs {
		if m.Role == trpcmodel.RoleSystem {
			pos = i + 1
		}
	}
	out := make([]trpcmodel.Message, 0, len(msgs)+1)
	out = append(out, msgs[:pos]...)
	out = append(out, marker)
	out = append(out, msgs[pos:]...)
	return out
}

// resolveCompressionContextWindow returns the product 256K chat-context
// budget (or a test override on ctx). Provider catalog windows are ignored.
func resolveCompressionContextWindow(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent) int {
	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	win := resolveL0ContextWindow(ctx, deps, ag, prov, mod)
	if usesConversationalContextBudget(ag) && win > conversationalCompressWindowTokens {
		return conversationalCompressWindowTokens
	}
	return win
}

// storeCompressionMeta writes compression metadata to the invocation state
// for the L0 snapshot hook to read.
func storeCompressionMeta(ctx context.Context, meta CompressionMeta) {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return
	}
	inv.SetState(compressionMetaStateKey, meta)
}

// LoadCompressionMeta reads the most recent compression metadata from the
// invocation state. Returns a zero-value CompressionMeta (Occurred=false)
// when no compression occurred this model-call.
//
// Exported so that l0_snapshot_persist.go can populate the snapshot row.
func LoadCompressionMeta(ctx context.Context) CompressionMeta {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return CompressionMeta{}
	}
	if v, ok := inv.GetState(compressionMetaStateKey); ok {
		if m, ok := v.(CompressionMeta); ok {
			return m
		}
	}
	return CompressionMeta{}
}
