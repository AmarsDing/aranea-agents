package biz

import (
	"context"

	"github.com/google/uuid"
)

// ─── StepID constants ────────────────────────────────────────────────
// Each constant identifies a discrete step within the Spirit orchestration
// pipeline.  Values follow the convention "spirit.<phase>.<step>" so that
// loggateway consumers can filter by phase or step.

// Phase 1 — TaskPlanner
const (
	SpiritStepPlannerAssess    = "spirit.planner.assess"
	SpiritStepPlannerRoute     = "spirit.planner.route"
	SpiritStepPlannerMemory    = "spirit.planner.memory"
	SpiritStepPlannerDecompose = "spirit.planner.decompose"
	SpiritStepPlannerPersist   = "spirit.planner.persist"
	SpiritStepPlannerConfirm   = "spirit.planner.confirm"
)

// Phase 2 — AgentAllocator
const (
	SpiritStepAllocatorMatch    = "spirit.allocator.match"
	SpiritStepAllocatorConflict = "spirit.allocator.conflict"
	SpiritStepAllocatorPersist  = "spirit.allocator.persist"
)

// Phase 3 — TaskOrchestrator
const (
	SpiritStepOrchestratorStrategy   = "spirit.orchestrator.strategy"
	SpiritStepOrchestratorGraphBuild = "spirit.orchestrator.graph_build"
	SpiritStepOrchestratorGraphAgent = "spirit.orchestrator.graph_agent"
	SpiritStepOrchestratorExecute    = "spirit.orchestrator.execute"
	SpiritStepOrchestratorCheckpoint = "spirit.orchestrator.checkpoint"
	SpiritStepOrchestratorSynthesize = "spirit.orchestrator.synthesize"
	SpiritStepOrchestratorLearn      = "spirit.orchestrator.learn"
	SpiritStepOrchestratorRecover    = "spirit.orchestrator.recover"
)

// ─── spirit_trace_id ────────────────────────────────────────────────

// NewSpiritTraceID generates a Spirit trace identifier in the form
// "tr_<uuid>" (no braces), used to correlate all steps of a single
// Spirit orchestration run.
func NewSpiritTraceID() string {
	return "tr_" + uuid.NewString()
}

// spiritTraceIDKey is the context key for spirit_trace_id.
type spiritTraceIDKey struct{}

// ContextWithSpiritTraceID returns a copy of ctx carrying the given
// spirit_trace_id.
func ContextWithSpiritTraceID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, spiritTraceIDKey{}, id)
}

// SpiritTraceIDFromContext extracts the spirit_trace_id from ctx.
// Returns ("", false) when absent.
func SpiritTraceIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, ok := ctx.Value(spiritTraceIDKey{}).(string)
	return id, ok
}
