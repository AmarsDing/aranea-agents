package biz

import (
	"context"
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/pkg/loggateway"
)

// EvolutionCoordinator is deprecated. Use SkillEvolutionOrchestrator instead.
// Kept for backward compatibility during migration.
//
// Before creating a new suggestion, each pipeline calls
// HasPendingEvolution to check whether another pipeline has already
// produced a pending suggestion for the same target.
type EvolutionCoordinator struct {
	orchestrator *SkillEvolutionOrchestrator
	// legacy fields kept for backward compat
	agentSuggRepo  EvolutionSuggestionRepo
	skillSuggRepo  SkillEvolutionSuggestionReader
	skillPropRepo  SkillProposalReadWriter
	lg             loggateway.Logger
}

// NewEvolutionCoordinator creates a new coordinator.
func NewEvolutionCoordinator(
	agentSuggRepo EvolutionSuggestionRepo,
	skillSuggRepo SkillEvolutionSuggestionReader,
	skillPropRepo SkillProposalReadWriter,
	lg loggateway.Logger,
) *EvolutionCoordinator {
	return &EvolutionCoordinator{
		agentSuggRepo:  agentSuggRepo,
		skillSuggRepo:  skillSuggRepo,
		skillPropRepo:  skillPropRepo,
		lg:             lg,
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

// HasPendingEvolution checks whether any of the three evolution pipelines
// has already created a pending suggestion for the given target.
// Returns true if a pending suggestion exists, false otherwise.
func (c *EvolutionCoordinator) HasPendingEvolution(ctx context.Context, target EvolutionTarget) bool {
	if c.orchestrator != nil {
		hasPending, err := c.orchestrator.HasPendingForTarget(ctx, target.Type, target.ID)
		if err != nil {
			c.lg.Warn("coordinator: orchestrator check failed, falling back to legacy", loggateway.Err(err))
		} else {
			return hasPending
		}
	}
	// Legacy fallback
	switch target.Type {
	case "agent":
		return c.hasPendingForAgent(ctx, target.ID)
	case "skill":
		return c.hasPendingForSkill(ctx, target.ID)
	default:
		return false
	}
}

func (c *EvolutionCoordinator) hasPendingForAgent(ctx context.Context, agentID string) bool {
	// Check EvolutionUsecase suggestions (agent-level).
	if agentSuggs, err := c.agentSuggRepo.ListByAgent(ctx, agentID, "pending"); err == nil && len(agentSuggs) > 0 {
		c.lg.Debug("EvolutionCoordinator: agent already has pending EvolutionSuggestion",
			loggateway.Str("agent_id", agentID),
			loggateway.Int("count", len(agentSuggs)))
		return true
	}
	// Check SkillEvolutionUsecase proposals (agent-level pattern proposals).
	if proposals, err := c.skillPropRepo.ListByAgent(ctx, agentID, string(SkillProposalStatusPending), 1, 0); err == nil && len(proposals) > 0 {
		c.lg.Debug("EvolutionCoordinator: agent already has pending SkillProposal",
			loggateway.Str("agent_id", agentID),
			loggateway.Int("count", len(proposals)))
		return true
	}
	return false
}

func (c *EvolutionCoordinator) hasPendingForSkill(ctx context.Context, skillID string) bool {
	// Check SkillIntelligenceUsecase suggestions (skill-level).
	if skillSuggs, err := c.skillSuggRepo.ListBySkill(ctx, skillID, EvoSuggestionPending, 1, 0); err == nil && len(skillSuggs) > 0 {
		c.lg.Debug("EvolutionCoordinator: skill already has pending SkillEvolutionSuggestion",
			loggateway.Str("skill_id", skillID),
			loggateway.Int("count", len(skillSuggs)))
		return true
	}
	return false
}

// RequireNoPendingEvolution is a guard that returns an error if a pending
// evolution already exists for the target. Use this when you want to
// hard-block duplicate creation rather than silently skip.
func (c *EvolutionCoordinator) RequireNoPendingEvolution(ctx context.Context, target EvolutionTarget) error {
	if c.HasPendingEvolution(ctx, target) {
		return kerrors.BadRequest("EVO_COORDINATOR", fmt.Sprintf("pending evolution already exists for %s %s", target.Type, target.ID))
	}
	return nil
}
