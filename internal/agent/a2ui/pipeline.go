package a2ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

const (
	planGCInterval    = 30 * time.Minute
	planMaxAge        = 2 * time.Hour
)

type Pipeline struct {
	encoder  *Encoder
	decoder  *Decoder
	surfaces *SurfaceManager
	lg       loggateway.Logger

	mu        sync.RWMutex
	plans     map[string]*Plan
	surfaceID atomic.Uint64

	stopCh chan struct{}
}

func NewPipeline(lg loggateway.Logger) *Pipeline {
	p := &Pipeline{
		encoder:  NewEncoder(),
		decoder:  NewDecoder(),
		surfaces: NewSurfaceManager(),
		lg:       lg,
		plans:    make(map[string]*Plan),
		stopCh:   make(chan struct{}),
	}
	safego.Go(context.Background(), "a2ui.plan_gc", p.planGC)
	return p
}

func (p *Pipeline) NextSurfaceID() string {
	return fmt.Sprintf("plan_surface_%d", p.surfaceID.Add(1))
}

func (p *Pipeline) StorePlan(plan *Plan) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now()
	}
	p.plans[plan.ID] = plan
}

func (p *Pipeline) GetPlan(planID string) (*Plan, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	plan, ok := p.plans[planID]
	if !ok {
		return nil, false
	}
	cp := *plan
	if plan.Steps != nil {
		cp.Steps = make([]PlanStep, len(plan.Steps))
		copy(cp.Steps, plan.Steps)
	}
	if plan.Dependencies != nil {
		cp.Dependencies = make(map[string][]string, len(plan.Dependencies))
		for k, v := range plan.Dependencies {
			dv := make([]string, len(v))
			copy(dv, v)
			cp.Dependencies[k] = dv
		}
	}
	return &cp, true
}

// DeletePlan removes a plan from the plans map.
func (p *Pipeline) DeletePlan(planID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.plans, planID)
}

// PlanCount returns the number of plans currently stored, for monitoring.
func (p *Pipeline) PlanCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.plans)
}

// Close stops the background GC goroutine.
func (p *Pipeline) Close() {
	select {
	case p.stopCh <- struct{}{}:
	default:
	}
}

// planGC runs a background goroutine that periodically removes plans older than planMaxAge.
func (p *Pipeline) planGC() {
	ticker := time.NewTicker(planGCInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.evictOldPlans()
		}
	}
}

// evictOldPlans removes plans whose CreatedAt is older than planMaxAge.
func (p *Pipeline) evictOldPlans() {
	now := time.Now()
	p.mu.Lock()
	var evicted int
	for id, plan := range p.plans {
		if !plan.CreatedAt.IsZero() && now.Sub(plan.CreatedAt) > planMaxAge {
			delete(p.plans, id)
			evicted++
		}
	}
	p.mu.Unlock()
	if evicted > 0 {
		p.lg.Debug("a2ui pipeline GC evicted stale plans",
			loggateway.StepID("a2ui.plan_gc"),
			loggateway.Int("evicted", evicted))
	}
}

func (p *Pipeline) EncodePlan(ctx context.Context, plan *Plan) ([]json.RawMessage, error) {
	surfaceID := p.NextSurfaceID()
	plan.ID = generatePlanID(surfaceID)
	p.StorePlan(plan)
	p.surfaces.BeginSurface(surfaceID, "plan_root", &SurfaceStyles{PrimaryColor: "#00BFFF"})
	return p.encoder.EncodePlanAsSurface(ctx, plan, surfaceID)
}

func (p *Pipeline) DecodeUserAction(ctx context.Context, payload []byte) (*UserAction, error) {
	return p.decoder.DecodeUserAction(ctx, payload)
}

func (p *Pipeline) UpdateSurface(ctx context.Context, surfaceID string, update SurfaceUpdate) (json.RawMessage, error) {
	p.surfaces.ApplySurfaceUpdate(update)
	return p.encoder.EncodeSurfaceUpdate(ctx, update)
}

func (p *Pipeline) UpdateDataModel(ctx context.Context, surfaceID string, contents []DataEntry) (json.RawMessage, error) {
	update := DataModelUpdate{SurfaceID: surfaceID, Contents: contents}
	p.surfaces.ApplyDataModelUpdate(update)
	return p.encoder.EncodeDataModelUpdate(ctx, update)
}

func (p *Pipeline) DeleteSurface(ctx context.Context, surfaceID string) (json.RawMessage, error) {
	p.surfaces.DeleteSurface(surfaceID)
	return p.encoder.EncodeDeleteSurface(ctx, DeleteSurface{SurfaceID: surfaceID})
}

func (p *Pipeline) UpdateStepProgress(ctx context.Context, surfaceID, stepID, status string) (json.RawMessage, error) {
	return p.encoder.EncodeStepProgressUpdate(ctx, surfaceID, stepID, status)
}

func (p *Pipeline) EmitPlanEvents(ctx context.Context, inv *trpcagent.Invocation, ch chan<- *trpcevent.Event, plan *Plan) error {
	messages, err := p.EncodePlan(ctx, plan)
	if err != nil {
		return fmt.Errorf("a2ui pipeline: encode plan: %w", err)
	}
	for _, msg := range messages {
		evt := trpcevent.New(inv.InvocationID, "a2ui",
			trpcevent.WithTag("a2ui_message"),
			trpcevent.WithExtension("a2ui_payload", msg),
		)
		if err := trpcagent.EmitEvent(ctx, inv, ch, evt); err != nil {
			p.lg.Warn("a2ui pipeline emit",
				loggateway.StepID("a2ui.emit_failed"),
				loggateway.Err(err))
		}
	}
	return nil
}

func (p *Pipeline) IsApproval(action *UserAction) bool {
	return p.decoder.IsApproval(action)
}

func (p *Pipeline) IsRejection(action *UserAction) bool {
	return p.decoder.IsRejection(action)
}

func (p *Pipeline) ActionPlanID(action *UserAction) string {
	return p.decoder.ActionPlanID(action)
}

func generatePlanID(surfaceID string) string {
	return "plan_" + surfaceID
}
