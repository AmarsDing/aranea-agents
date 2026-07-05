package service

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/runtime"
	"aranea-agents/internal/team"
)

// GraphOrchestrationProjector starts run-scoped status projectors for graph executions.
type GraphOrchestrationProjector struct {
	eventBus biz.EventBus // Phase 3b-D: was biz.ActivityEventBus, migrated to v2 EventBus
	seq      runtime.EventPublisher
	mu       sync.Mutex
	// execID -> cancel
	stops map[string]context.CancelFunc
}

var _ biz.GraphExecutionObserver = (*GraphOrchestrationProjector)(nil)

func NewGraphOrchestrationProjector(eventBus biz.EventBus) *GraphOrchestrationProjector {
	return &GraphOrchestrationProjector{
		eventBus: eventBus,
		stops:    make(map[string]context.CancelFunc),
	}
}

// SetSeq injects the v2 Sequencer post-construction.
// Called from ProvideChatService after V2ProjectorFactory is available.
func (p *GraphOrchestrationProjector) SetSeq(seq runtime.EventPublisher) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.seq = seq
	p.mu.Unlock()
}

// Start begins projecting orchestration status for a graph execution.
func (p *GraphOrchestrationProjector) Start(ctx context.Context, sessionID, execID, graphID string, def *biz.GraphDefinition) {
	if p == nil || p.eventBus == nil || def == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	execID = strings.TrimSpace(execID)
	if sessionID == "" || execID == "" {
		return
	}
	reg := biz.BuildOrchestrationRegistryFromGraph(def)
	stop := team.StartOrchestrationStatusProjector(ctx, team.OrchestrationProjectorConfig{
		RunID:     execID,
		TeamID:    strings.TrimSpace(graphID),
		SessionID: sessionID,
		Registry:  reg,
		Channel:   "graph",
		EventBus:  p.eventBus,
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
