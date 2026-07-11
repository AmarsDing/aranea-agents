package agent

import (
	"context"
	"math"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// compressionSummaryStateKey is the invocation-state key for the current
// recursive summary. Stored across turns so successive compressions merge
// the existing summary with newly evicted messages (Letta's recursive
// summarisation pattern).
const compressionSummaryStateKey = "aranea:compression_summary"

// compressionMetaStateKey is the invocation-state key for the most recent
// compression metadata. The L0 snapshot hook reads this to populate
// TruncateStrategy / SummaryTokenEstimate / etc. on the snapshot row.
const compressionMetaStateKey = "aranea:compression_meta"

// CompressionMeta records the outcome of the most recent compression pass.
// Stored in invocation state so the L0 snapshot hook (priority 10) can read
// what the compression hook (priority 3) produced.
type CompressionMeta struct {
	// Occurred is true when compression ran successfully this model-call.
	Occurred bool
	// EvictedCount is the number of messages folded into the summary.
	EvictedCount int
	// BeforeChars is the total char count of evicted messages + existing summary.
	BeforeChars int
	// AfterChars is len(summary).
	AfterChars int
	// SummaryText is the generated summary (for token estimation by the snapshot).
	SummaryText string
}

// defaultKeepRatio is the fallback fraction of recent conversation messages
// to retain during compression. Used when the policy value is not set.
// Letta uses ~30%; we follow the same default.
const defaultKeepRatio = 0.30

// newContextCompressionBeforeHook creates a BeforeModel hook that triggers
// recursive context compression when the token usage ratio crosses the
// threshold configured in MemoryRuntimePolicy (default 0.80).
//
// The hook runs at priority 3 (before memory inject at 5 and prompt snapshot
// at 10) so that:
//  1. Old conversation messages are evicted and replaced with a summary.
//  2. Memory inject then adds fresh L1/L2/L3/L4 memory on top of the
//     compressed context.
//  3. Prompt snapshot observes the final (post-compression, post-inject)
//     prompt for L0 Assembly Snapshot persistence.
//
// Returns nil when ContextCompressor is not wired (no-op).
func newContextCompressionBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if deps.ContextCompressor == nil {
		return nil
	}
	// Resolve the memory runtime policy once at hook creation. Compression
	// defaults are always populated (even when memory is disabled) so the
	// hook has valid thresholds.
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	threshold := policy.CompressionThreshold
	if threshold <= 0 {
		threshold = llmcontext.ContextStatusCriticalThreshold
	}
	keepRatio := policy.CompressionKeepRatio
	if keepRatio <= 0 {
		keepRatio = defaultKeepRatio
	}
	return callbacks.NewBeforeModelHook(3, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		lg := deps.Logger()
		// 1. Estimate token usage ratio.
		report := analyzePromptRequest(args.Request.Messages)
		win := resolveCompressionContextWindow(ctx, deps, ag)
		ratio := llmcontext.ContextRatio(report.EstTokens, win)
		// 2. Check if compression is needed (policy-driven threshold).
		if ratio < threshold {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// 3. Partition messages: keep system + last keepRatio of conversation, evict rest.
		keepMsgs, evictedMsgs := partitionMessagesForCompression(args.Request.Messages, keepRatio)
		if len(evictedMsgs) == 0 {
			// Nothing to evict (e.g. very short conversation) — skip compression.
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// 4. Read existing summary from invocation state (recursive summarisation).
		existingSummary := loadCompressionSummary(ctx)
		// 5. Convert evicted messages to ConsolidateMessage for the compressor.
		evictedConsolidate := toConsolidateMessages(evictedMsgs)
		// 6. Call the compressor.
		result, err := deps.ContextCompressor.Compress(ctx, existingSummary, evictedConsolidate)
		if err != nil {
			// Graceful degradation: log warn, leave messages unchanged.
			lg.Warn("context compression failed, skipping (graceful degradation)",
				loggateway.StepID("agent.context.compress"),
				loggateway.Str("phase", "compress"),
				loggateway.Err(err))
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		// 7. Store the new summary for the next compression pass.
		storeCompressionSummary(ctx, result.Summary)
		// 8. Store compression metadata for the L0 snapshot hook.
		storeCompressionMeta(ctx, CompressionMeta{
			Occurred:     true,
			EvictedCount: result.EvictedCount,
			BeforeChars:  result.BeforeChars,
			AfterChars:   result.AfterChars,
			SummaryText:  result.Summary,
		})
		// 9. Rebuild messages: [keep] + [summary system message].
		// The summary is injected as a system message after existing system
		// messages so the LLM treats it as authoritative context.
		summaryMsg := trpcmodel.NewSystemMessage(buildCompressionSummaryBlock(result.Summary))
		args.Request.Messages = append(keepMsgs, summaryMsg)
		lg.Info("context compression completed",
			loggateway.StepID("agent.context.compress"),
			loggateway.Phase("done"),
			loggateway.Int("evicted_count", result.EvictedCount),
			loggateway.Int("before_chars", result.BeforeChars),
			loggateway.Int("after_chars", result.AfterChars),
			loggateway.Any("used_ratio", ratio))
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
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
	evictCount := len(nonSystem) - keepCount
	evictSet := make(map[int]bool, evictCount)
	for i := 0; i < evictCount; i++ {
		evictSet[nonSystem[i].index] = true
	}

	keepMsgs = make([]trpcmodel.Message, 0, len(messages)-evictCount)
	for i, m := range messages {
		if !evictSet[i] {
			keepMsgs = append(keepMsgs, m)
		} else {
			evictedMsgs = append(evictedMsgs, m)
		}
	}
	return keepMsgs, evictedMsgs
}

// toConsolidateMessages converts trpcmodel.Message slice to biz.ConsolidateMessage
// slice for the compressor. Tool messages are mapped to "tool" role.
func toConsolidateMessages(msgs []trpcmodel.Message) []biz.ConsolidateMessage {
	out := make([]biz.ConsolidateMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, biz.ConsolidateMessage{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}
	return out
}

// loadCompressionSummary reads the existing recursive summary from the
// invocation state. Returns empty string if no summary is stored.
func loadCompressionSummary(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	if v, ok := inv.GetState(compressionSummaryStateKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// storeCompressionSummary writes the recursive summary to the invocation
// state for the next compression pass.
func storeCompressionSummary(ctx context.Context, summary string) {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return
	}
	inv.SetState(compressionSummaryStateKey, summary)
}

// resolveCompressionContextWindow resolves the context window size for
// compression threshold computation. Uses the same resolution logic as
// L0 snapshot persistence for consistency.
func resolveCompressionContextWindow(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent) int {
	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	return resolveL0ContextWindow(ctx, deps, ag, prov, mod)
}

// buildCompressionSummaryBlock wraps the summary text in a labelled block
// so the LLM can distinguish it from other system instructions.
func buildCompressionSummaryBlock(summary string) string {
	return "<context_summary>\n" + strings.TrimSpace(summary) + "\n</context_summary>"
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
