// Package memory provides the Hebbian weight update engine for the L4 entity
// graph. It implements the "fire together, wire together" rule (Δw = η * pre *
// post) with weight saturation and long-term decay for unused connections.
package memory

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// Hebbian learning constants (per design §15.6).
const (
	// hebbianLearningRate (η) controls how much each co-activation event
	// strengthens a connection. Default 0.1 = 10% of pre*post product.
	hebbianLearningRate = 0.1

	// hebbianDecayFactor is the per-cycle weight multiplier for unused
	// connections. 0.95 means a connection loses 5% of its weight per decay
	// cycle (aligned with the Ebbinghaus forgetting curve).
	hebbianDecayFactor = 0.95

	// hebbianArchiveThreshold is the weight below which a relation is marked
	// status='archived' during decay. Weak connections are preserved in
	// history but excluded from active traversal.
	hebbianArchiveThreshold = 0.1

	// hebbianWeightMax is the upper saturation bound for relation weights.
	hebbianWeightMax = 1.0
)

// HebbianUpdater implements the Hebbian reinforcement rule and long-term decay
// for L4 memory relations. It depends on the narrow biz.L4HebbianStore
// interface (3 methods) rather than the full L4GraphRepo, so mock
// implementations only need to stub FindRelation / UpdateRelationWeight /
// DecayUnusedRelations.
type HebbianUpdater struct {
	store biz.L4HebbianStore
	lg    loggateway.Logger
}

// NewHebbianUpdater constructs a HebbianUpdater. The store may be nil (methods
// become no-ops); lg falls back to a noop logger if nil.
func NewHebbianUpdater(store biz.L4HebbianStore, lg loggateway.Logger) *HebbianUpdater {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &HebbianUpdater{store: store, lg: lg.With(loggateway.Domain("hebbian"))}
}

// ReinforceConnection applies the Hebbian rule to the relation between nodeA
// and nodeB:
//
//	Δw = η * pre_activation * post_activation
//	newWeight = min(weight + Δw, 1.0)
//
// Co-activation count is incremented and last_reinforced_at is set to now.
// If no relation exists, this is a graceful no-op (returns nil).
func (u *HebbianUpdater) ReinforceConnection(
	ctx context.Context, nodeA, nodeB, relationType string,
) error {
	if u == nil || u.store == nil {
		return nil
	}

	rel, found, err := u.store.FindRelation(ctx, nodeA, nodeB, relationType)
	if err != nil {
		u.lg.Warn("hebbian: FindRelation failed",
			loggateway.Str("nodeA", nodeA),
			loggateway.Str("nodeB", nodeB),
			loggateway.Str("relationType", relationType),
			loggateway.Err(err))
		return err
	}
	if !found {
		// No existing relation — graceful no-op. Relations are created by
		// LinkEvolutionService; HebbianUpdater only reinforces existing ones.
		return nil
	}

	// Hebbian rule: Δw = η * pre_activation * post_activation
	delta := hebbianLearningRate * rel.SourceActivation * rel.TargetActivation
	newWeight := rel.Weight + delta
	if newWeight > hebbianWeightMax {
		newWeight = hebbianWeightMax
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := u.store.UpdateRelationWeight(ctx, rel.ID, newWeight, rel.CoActivationCount+1, now); err != nil {
		u.lg.Warn("hebbian: UpdateRelationWeight failed",
			loggateway.Str("relationID", rel.ID),
			loggateway.Err(err))
		return err
	}
	return nil
}

// DecayUnused decays weights of relations whose last_reinforced_at (or
// created_at fallback) is older than the given threshold duration. Weight is
// multiplied by hebbianDecayFactor (0.95); relations whose weight drops below
// hebbianArchiveThreshold (0.1) are marked status='archived'.
//
// This is typically called by a periodic background job (e.g. daily) to
// implement Ebbinghaus-style forgetting for unused connections.
func (u *HebbianUpdater) DecayUnused(ctx context.Context, threshold time.Duration) (biz.L4DecayResult, error) {
	if u == nil || u.store == nil {
		return biz.L4DecayResult{}, nil
	}
	if threshold <= 0 {
		return biz.L4DecayResult{}, nil
	}
	cutoff := time.Now().UTC().Add(-threshold).Format(time.RFC3339)
	result, err := u.store.DecayUnusedRelations(ctx, cutoff)
	if err != nil {
		u.lg.Warn("hebbian: DecayUnusedRelations failed",
			loggateway.Str("cutoff", cutoff),
			loggateway.Err(err))
		return biz.L4DecayResult{}, err
	}
	return result, nil
}
