// Package memory provides memory reconsolidation for the L4 entity graph.
//
// Reconsolidation is the process by which recalling a memory updates the
// memory itself — boosting the recalled neuron's activation, incrementing its
// use_count, and reinforcing its connections to co-recalled neurons via the
// Hebbian rule ("fire together, wire together"). This mirrors the
// reconsolidation theory from neuroscience and is invoked asynchronously
// after a successful recall so that it never blocks model calls.
package memory

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// reconsolidationBoostDelta is the activation boost applied when an entity is
// recalled. Per design §15.7, activation is boosted by 0.2 and saturated to
// 1.0 inside the store (BoostActivation uses min(activation + delta, 1.0)).
const reconsolidationBoostDelta = 0.2

// ReconsolidationService implements memory reconsolidation: when an entity is
// recalled, its activation is boosted, its use_count is incremented, and the
// Hebbian rule is applied to all co-recalled entities to reinforce their
// connections.
//
// It depends on the narrow biz.L4ReconsolidationStore (2 methods) plus a
// *HebbianUpdater so that mock implementations only need to stub the relevant
// surface (per DB-N3 / AS-FIT-01).
type ReconsolidationService struct {
	store   biz.L4ReconsolidationStore
	hebbian *HebbianUpdater
	lg      loggateway.Logger
}

// NewReconsolidationService constructs a ReconsolidationService. store may be
// nil (OnRecall becomes a no-op); hebbian may be nil (Step 3 is skipped); lg
// falls back to a noop logger if nil.
func NewReconsolidationService(store biz.L4ReconsolidationStore, hebbian *HebbianUpdater, lg loggateway.Logger) *ReconsolidationService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ReconsolidationService{
		store:   store,
		hebbian: hebbian,
		lg:      lg.With(loggateway.Domain("reconsolidation")),
	}
}

// OnRecall triggers memory reconsolidation for a recalled entity:
//  1. Boost activation by reconsolidationBoostDelta (0.2), saturated to 1.0
//     inside the store. Hard-failing step — errors propagate.
//  2. Increment use_count. Best-effort: errors are logged but do not abort.
//  3. For each co-recalled entity, apply the Hebbian rule to reinforce the
//     connection. Best-effort: errors are logged but do not abort.
//
// If the entity is not found (BoostActivation returns ok=false), the call is a
// graceful no-op (returns nil).
//
// The service is safe for nil receivers and nil stores (returns nil).
func (s *ReconsolidationService) OnRecall(
	ctx context.Context, nodeID string, recalledWith []string,
) error {
	if s == nil || s.store == nil {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Step 1: boost activation (hard-failing).
	ok, err := s.store.BoostActivation(ctx, nodeID, reconsolidationBoostDelta, now)
	if err != nil {
		s.lg.Warn("reconsolidation: BoostActivation failed",
			loggateway.Str("nodeID", nodeID),
			loggateway.Err(err))
		return err
	}
	if !ok {
		// Entity not found (or soft-deleted) — graceful no-op.
		return nil
	}

	// Step 2: increment use_count (best-effort).
	if _, err := s.store.IncrementUseCount(ctx, nodeID); err != nil {
		s.lg.Warn("reconsolidation: IncrementUseCount failed (best-effort)",
			loggateway.Str("nodeID", nodeID),
			loggateway.Err(err))
	}

	// Step 3: Hebbian reinforcement for co-recalled entities (best-effort).
	if s.hebbian != nil {
		for _, otherID := range recalledWith {
			if err := s.hebbian.ReinforceConnection(ctx, nodeID, otherID, biz.RelationRelatedTo); err != nil {
				s.lg.Warn("reconsolidation: Hebbian reinforcement failed (best-effort)",
					loggateway.Str("nodeA", nodeID),
					loggateway.Str("nodeB", otherID),
					loggateway.Err(err))
			}
		}
	}

	return nil
}
