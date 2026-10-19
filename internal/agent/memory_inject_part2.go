package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"context"
	"encoding/json"
	"strings"
)

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

	if err := emitter.EmitNotice(ctx, string(content), memoryRecalledNoticeType); err != nil {
		return ctx
	}
	return context.WithValue(ctx, memoryRecalledEmittedKey{}, true)
}

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

// l4ReconsolidatedKey marks a ctx in which L4 reconsolidation was already
// triggered, so tool-loop re-entries of the before-model hook do not
// re-boost the same entities within one turn.
type l4ReconsolidatedKey struct{}

// triggerL4Reconsolidation asynchronously fires OnRecall for each recalled L4
// entity, boosting its activation and reinforcing connections to co-recalled
// entities via the Hebbian rule. use_count is not incremented on recall
// (C2: count only when the fact is injected/cited). Per design §15.7
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
