package agent

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/agent/callbacks"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// Context budget ledger (设计文档 29-token.design.md §9.6, 任务 0.1):
// a per-request collector mounted on the turn context. BeforeModel injection
// hooks record the char size of what they inject; at turn end the service
// layer reads a Snapshot and emits the chat.context_budget process log.
// Pure observability — no injection logic is changed.
//
// Lifecycle: single goroutine writes during the turn, read once at turn end.
// No locks.

// Budget categories.
const (
	ContextBudgetCategoryStaticPrefix    = "static_prefix"
	ContextBudgetCategoryToolsSchema     = "tools_schema"
	ContextBudgetCategoryMemoryL1        = "memory_l1"
	ContextBudgetCategoryMemoryL4        = "memory_l4"
	ContextBudgetCategoryMemoryComposite = "memory_composite"
	ContextBudgetCategoryKnowledgeCue    = "knowledge_cue"
	ContextBudgetCategorySkillGuidance   = "skill_guidance"
	ContextBudgetCategoryOtherDynamic    = "other_dynamic"
)

// contextBudgetCtxKey carries the per-request ContextBudget.
type contextBudgetCtxKey struct{}

// ContextBudget accumulates per-category injected chars for one request.
type ContextBudget struct {
	chars      map[string]int
	recorded   map[string]bool
	toolsCount int
}

// ContextBudgetSnapshot is the turn-end readout: per-category chars and
// estimated tokens, plus the tools count and the estimated total input.
type ContextBudgetSnapshot struct {
	Chars         map[string]int
	EstTokens     map[string]int
	ToolsCount    int
	EstTotalInput int
}

// WithContextBudget mounts a fresh ContextBudget on ctx and returns both.
func WithContextBudget(ctx context.Context) (context.Context, *ContextBudget) {
	b := &ContextBudget{chars: map[string]int{}, recorded: map[string]bool{}}
	return context.WithValue(ctx, contextBudgetCtxKey{}, b), b
}

// ContextBudgetFromContext returns the mounted ContextBudget, or nil.
func ContextBudgetFromContext(ctx context.Context) *ContextBudget {
	v, _ := ctx.Value(contextBudgetCtxKey{}).(*ContextBudget)
	return v
}

// RecordContextBudget accumulates chars into the category. Silent no-op when
// no collector is mounted — callers never check for presence.
func RecordContextBudget(ctx context.Context, category string, chars int) {
	if b := ContextBudgetFromContext(ctx); b != nil {
		b.chars[category] += chars
	}
}

// recordContextBudgetOnce records chars only on the first call per category
// per request. Used by BeforeModel hooks that re-fire on every tool-loop
// model call: the first model call's composition is the representative
// per-request baseline, later re-injections of the same cue must not
// multiply the ledger.
func recordContextBudgetOnce(ctx context.Context, category string, chars int) {
	if b := ContextBudgetFromContext(ctx); b != nil && !b.recorded[category] {
		b.recorded[category] = true
		b.chars[category] += chars
	}
}

// has reports whether the category was already recorded this request.
func (b *ContextBudget) has(category string) bool {
	return b.recorded[category] || b.chars[category] > 0
}

// SetToolsCount stores the number of tool schemas sent with the request.
func (b *ContextBudget) SetToolsCount(n int) {
	b.toolsCount = n
}

// Snapshot returns the per-category chars and estimated tokens.
// est_tokens 口径：chars/3.5 向上取整。
func (b *ContextBudget) Snapshot() ContextBudgetSnapshot {
	snap := ContextBudgetSnapshot{
		Chars:      make(map[string]int, len(b.chars)),
		EstTokens:  make(map[string]int, len(b.chars)),
		ToolsCount: b.toolsCount,
	}
	for cat, chars := range b.chars {
		snap.Chars[cat] = chars
		est := estimateTokens(chars)
		snap.EstTokens[cat] = est
		snap.EstTotalInput += est
	}
	return snap
}

// estimateTokens converts chars to a rough token count (chars/3.5, ceil).
// 0 chars → 0 tokens.
func estimateTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	// ceil(chars / 3.5) == ceil(2*chars / 7) in integer arithmetic.
	return (2*chars + 6) / 7
}

// newContextBudgetToolsBeforeHook meters the tools_schema component. The
// framework preprocess populates args.Request.Tools (post tool-filter, the
// set actually sent to the model) before BeforeModel callbacks run, so
// serializing declarations here measures the real per-turn schema payload
// without touching build-time tool assembly. Per-request dedupe via
// recordContextBudgetOnce keeps tool-loop re-entries from re-marshaling.
func newContextBudgetToolsBeforeHook() callbacks.Callback {
	return callbacks.NewBeforeModelHook(0, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		b := ContextBudgetFromContext(ctx)
		if b == nil || args == nil || args.Request == nil || b.has(ContextBudgetCategoryToolsSchema) {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		totalChars := 0
		for _, t := range args.Request.Tools {
			d := t.Declaration()
			if d == nil {
				continue
			}
			if raw, err := json.Marshal(d); err == nil {
				totalChars += len(raw)
			}
		}
		recordContextBudgetOnce(ctx, ContextBudgetCategoryToolsSchema, totalChars)
		b.SetToolsCount(len(args.Request.Tools))
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
