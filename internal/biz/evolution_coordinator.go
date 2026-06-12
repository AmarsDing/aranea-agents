package biz

import (
	"context"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// EvolutionCoordinator is deprecated. Use SkillEvolutionOrchestrator instead.
// All methods delegate to SkillEvolutionOrchestrator; the legacy fallback
// code paths have been removed as part of DEV-04 unification.
//
// Deprecated: Use SkillEvolutionOrchestrator directly. This type will be
// removed in a future release once all callers are migrated.
type EvolutionCoordinator struct {
	orchestrator *SkillEvolutionOrchestrator
	lg           loggateway.Logger
}

// NewEvolutionCoordinator creates a new coordinator.
//
// Deprecated: Use NewSkillEvolutionOrchestrator instead.
func NewEvolutionCoordinator(
	lg loggateway.Logger,
) *EvolutionCoordinator {
	return &EvolutionCoordinator{
		lg: lg,
	}
}

// EvolutionTarget identifies the entity an evolution suggestion targets.
type EvolutionTarget struct {
	Type string // "agent" or "skill"
	ID   string // agentID or skillID
}

// SetOrchestrator sets the unified evolution orchestrator.
// When set, HasPendingEvolution delegates to the orchestrator first.
// NOTE: Must only be called during initialization, before any concurrent access.
func (c *EvolutionCoordinator) SetOrchestrator(o *SkillEvolutionOrchestrator) {
	c.orchestrator = o
}

// HasPendingEvolution checks whether the unified orchestrator
// has already created a pending suggestion for the given target.
// Returns true if a pending suggestion exists, false otherwise.
//
// Deprecated: Use SkillEvolutionOrchestrator.HasPendingForTarget instead.
func (c *EvolutionCoordinator) HasPendingEvolution(ctx context.Context, target EvolutionTarget) bool {
	if c.orchestrator == nil {
		c.lg.Warn("coordinator: no orchestrator configured, cannot check pending evolution",
			loggateway.StepID("evo_coordinator.has_pending"),
			loggateway.Str("target_type", target.Type),
			loggateway.Str("target_id", target.ID))
		return false
	}
	hasPending, err := c.orchestrator.HasPendingForTarget(ctx, target.Type, target.ID)
	if err != nil {
		c.lg.Warn("coordinator: orchestrator check failed",
			loggateway.StepID("evo_coordinator.has_pending"),
			loggateway.Err(err))
		return false
	}
	return hasPending
}

// RequireNoPendingEvolution is a guard that returns an error if a pending
// evolution already exists for the target. Use this when you want to
// hard-block duplicate creation rather than silently skip.
//
// Deprecated: Use SkillEvolutionOrchestrator.HasPendingForTarget directly.
func (c *EvolutionCoordinator) RequireNoPendingEvolution(ctx context.Context, target EvolutionTarget) error {
	if c.HasPendingEvolution(ctx, target) {
		return apierror.BadRequest("EVO_COORDINATOR", "pending evolution already exists for %s %s", target.Type, target.ID)
	}
	return nil
}
