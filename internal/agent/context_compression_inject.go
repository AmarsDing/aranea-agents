package agent

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
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
// what the compression hook (priority 3) produced.
type CompressionMeta struct {
	// Occurred is true when compression ran successfully this model-call.
	Occurred bool
	// EvictedCount is the number of messages dropped by emergency truncation.
	EvictedCount int
	// BeforeChars is the total char count of evicted messages.
	BeforeChars int
	// AfterChars is 0 for deterministic truncation (no summary is produced).
	AfterChars int
	// SummaryText is always empty for deterministic truncation (kept for the
	// L0 snapshot overlay contract).
	SummaryText string
}

// defaultKeepRatio is the fallback fraction of recent conversation messages
// to retain during compression. Used when the policy value is not set.
// Letta uses ~30%; we follow the same default.
const defaultKeepRatio = 0.30

// newContextCompressionBeforeHook creates a BeforeModel hook that performs
// deterministic emergency truncation when the token usage ratio crosses the
// per-agent hard threshold (HardTriggerRatio, default 0.90 — mirroring
// CompressPolicy in internal/session; duplicated here to avoid a package
// dependency from agent → session).
//
// Architecture (ADR: dual-compression unification, 2026-07-20):
//   - This hook is the intra-turn last-ditch guard: it fires before every
//     model call (including mid-turn tool loops that the Session Compressor
//     cannot intercept) and mechanically drops the oldest messages with
//     tool-pair-safe boundaries. NO LLM call, NO summary — deterministic,
//     no failure modes.
//   - The Session Compressor (internal/session) owns inter-turn durable
//     compression with LLM rolling summaries, quality gates, suppression
//     and caching.
//
// The hook runs at priority 3 (before memory inject at 5 and prompt snapshot
// at 10) so that:
//  1. Old conversation messages are dropped and a truncation marker inserted.
//  2. Memory inject then adds fresh L1/L2/L3/L4 memory on top.
//  3. Prompt snapshot observes the final (post-truncation, post-inject)
//     prompt for L0 Assembly Snapshot persistence.
//
// Returns nil when compression is disabled for the agent
// (L0SnapshotMode=off without ContextCompactionEnabled).
func newContextCompressionBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if !contextCompressionEnabledForAgent(ag) {
		return nil
	}
	threshold := hardTriggerRatioForAgent(ag)
	keepRatio := defaultKeepRatio
	return callbacks.NewBeforeModelHook(3, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		lg := deps.Logger()
		// 1. Estimate token usage ratio.
		report := analyzePromptRequest(args.Request.Messages)
		win := resolveCompressionContextWindow(ctx, deps, ag)
		ratio := llmcontext.ContextRatio(report.EstTokens, win)
		// 2. Check if emergency truncation is needed (hard threshold).
		if ratio < threshold {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// 3. Partition messages: keep system + last keepRatio of conversation, drop rest.
		keepMsgs, evictedMsgs := partitionMessagesForCompression(args.Request.Messages, keepRatio)
		if len(evictedMsgs) == 0 {
			// Nothing to drop (e.g. very short conversation) — skip.
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// 4. Rebuild messages with a deterministic truncation marker placed
		// where the messages were removed (after the last system message).
		beforeChars := 0
		for _, m := range evictedMsgs {
			beforeChars += len(m.Content)
		}
		marker := trpcmodel.NewSystemMessage(buildTruncationMarker(len(evictedMsgs)))
		args.Request.Messages = insertAfterLastSystem(keepMsgs, marker)
		// 上下文预算台账（29-token §9.6）：仅计量，不改注入逻辑。
		recordContextBudgetOnce(ctx, ContextBudgetCategoryOtherDynamic, utf8.RuneCountInString(marker.Content))
		// 5. Store compression metadata for the L0 snapshot hook.
		storeCompressionMeta(ctx, CompressionMeta{
			Occurred:     true,
			EvictedCount: len(evictedMsgs),
			BeforeChars:  beforeChars,
			AfterChars:   0,
			SummaryText:  "",
		})
		lg.Info("context emergency truncation completed",
			loggateway.StepID("agent.context.compress"),
			loggateway.Phase("done"),
			loggateway.Int("evicted_count", len(evictedMsgs)),
			loggateway.Int("before_chars", beforeChars),
			loggateway.Any("used_ratio", ratio))
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
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
	if ag.Settings != nil && ag.Settings.HardTriggerRatio > 0 {
		return ag.Settings.HardTriggerRatio
	}
	return biz.DefaultHardTriggerRatio
}

// partitionMessagesForCompression splits the message list into:
//   - keepMsgs: all system messages + the last keepRatio of non-system messages
//   - evictedMsgs: the first (1 - keepRatio) of non-system messages
//
// System messages are always kept (they contain identity, instructions, and
// injected memory). Only user/assistant/tool messages are eligible for
// eviction.
//
// When there are too few non-system messages (<= 1), nothing is evicted.
func partitionMessagesForCompression(messages []trpcmodel.Message, keepRatio float64) (keepMsgs, evictedMsgs []trpcmodel.Message) {
	if len(messages) == 0 {
		return nil, nil
	}
	// Clamp keepRatio to [0, 1].
	if keepRatio < 0 {
		keepRatio = 0
	}
	if keepRatio > 1 {
		keepRatio = 1
	}

	// Walk through messages, collecting indices of non-system messages.
	type msgWithIndex struct {
		index int
		msg   trpcmodel.Message
	}
	var nonSystem []msgWithIndex
	for i, m := range messages {
		if m.Role != trpcmodel.RoleSystem {
			nonSystem = append(nonSystem, msgWithIndex{index: i, msg: m})
		}
	}

	// If there's only 0 or 1 non-system message, keep everything.
	if len(nonSystem) <= 1 {
		return messages, nil
	}

	// Calculate how many recent messages to keep.
	keepCount := int(math.Ceil(float64(len(nonSystem)) * keepRatio))
	if keepCount < 1 {
		keepCount = 1
	}
	if keepCount >= len(nonSystem) {
		return messages, nil
	}

	// Split: first (len - keepCount) are evicted, last keepCount are kept.
	// Then snap the boundary to avoid splitting tool-call / tool-result pairs.
	evictCount := len(nonSystem) - keepCount
	evictSet := make(map[int]bool, evictCount)
	for i := 0; i < evictCount; i++ {
		evictSet[nonSystem[i].index] = true
	}

	// Tool-pair safety: snap the evict boundary so that no tool_call/tool_result
	// pair is split across the keep/evicted boundary. When a boundary would
	// split a pair, move the boundary so the pair stays together in the keep
	// side (safer — keeps the most recent context intact).
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

// resolveCompressionContextWindow resolves the context window size for
// compression threshold computation. Uses the same resolution logic as
// L0 snapshot persistence for consistency.
func resolveCompressionContextWindow(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent) int {
	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	return resolveL0ContextWindow(ctx, deps, ag, prov, mod)
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
