package service

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/team"
)

// GraphOrchestrationProjector starts run-scoped status projectors for graph executions.
type GraphOrchestrationProjector struct {
	bus         event.Bus
	activityBus biz.ActivityEventBus
	mu          sync.Mutex
	// execID -> cancel
	stops map[string]context.CancelFunc
}

var _ biz.GraphExecutionObserver = (*GraphOrchestrationProjector)(nil)

func NewGraphOrchestrationProjector(bus event.Bus, activityBus biz.ActivityEventBus) *GraphOrchestrationProjector {
	return &GraphOrchestrationProjector{
		bus:         bus,
		activityBus: activityBus,
		stops:       make(map[string]context.CancelFunc),
	}
}

// Start begins projecting orchestration status for a graph execution.
func (p *GraphOrchestrationProjector) Start(ctx context.Context, sessionID, execID, graphID string, def *biz.GraphDefinition) {
	if p == nil || p.bus == nil || def == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	execID = strings.TrimSpace(execID)
	if sessionID == "" || execID == "" {
		return
	}
	reg := biz.BuildOrchestrationRegistryFromGraph(def)
	stop := team.StartOrchestrationStatusProjector(ctx, p.bus, team.OrchestrationProjectorConfig{
		RunID:     execID,
		TeamID:    strings.TrimSpace(graphID),
		SessionID: sessionID,
		Registry:  reg,
		Channel:   "graph",
	})
	p.mu.Lock()
	if prev, ok := p.stops[execID]; ok {
		prev()
	}
	p.stops[execID] = stop
	p.mu.Unlock()
}

func (p *GraphOrchestrationProjector) OnGraphExecutionComplete(exec *biz.GraphExecution) {
	if p == nil || exec == nil {
		return
	}
	p.mu.Lock()
	stop, ok := p.stops[exec.ID]
	if ok {
		delete(p.stops, exec.ID)
	}
	p.mu.Unlock()
	if stop != nil {
		stop()
	}
}
