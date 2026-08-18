package team

import "sync"

// OrchestrationTemplate defines the contract for team orchestration patterns.
// Stability:stable
type OrchestrationTemplate interface {
	ID() string
	BuildEdges(def Definition, agentIDs []string) []embeddedGraphEdge
}

var (
	templateRegistry   = map[string]OrchestrationTemplate{}
	templateRegistryMu sync.RWMutex
)

func RegisterTemplate(t OrchestrationTemplate) {
	templateRegistryMu.Lock()
	defer templateRegistryMu.Unlock()
	templateRegistry[t.ID()] = t
}

func LookupTemplate(mode string) OrchestrationTemplate {
	templateRegistryMu.RLock()
	defer templateRegistryMu.RUnlock()
	return templateRegistry[mode]
}

type pipelineTemplate struct{}

func (pipelineTemplate) ID() string { return "sequential" }

func (pipelineTemplate) BuildEdges(_ Definition, ids []string) []embeddedGraphEdge {
	out := make([]embeddedGraphEdge, 0, len(ids)-1)
	for i := 0; i < len(ids)-1; i++ {
		out = append(out, embeddedGraphEdge{Source: ids[i], Target: ids[i+1], Label: "flow"})
	}
	return out
}

type parallelReviewTemplate struct{}

func (parallelReviewTemplate) ID() string { return "parallel" }

func (parallelReviewTemplate) BuildEdges(_ Definition, ids []string) []embeddedGraphEdge {
	if len(ids) <= 1 {
		return nil
	}
	finish := ids[len(ids)-1]
	out := make([]embeddedGraphEdge, 0, len(ids)-1)
	for _, id := range ids[:len(ids)-1] {
		out = append(out, embeddedGraphEdge{Source: id, Target: finish, Label: "flow"})
	}
	return out
}

type dispatchTemplate struct{}

func (dispatchTemplate) ID() string { return "coordinator" }

func (dispatchTemplate) BuildEdges(_ Definition, ids []string) []embeddedGraphEdge {
	if len(ids) < 2 {
		return pipelineTemplate{}.BuildEdges(Definition{}, ids)
	}
	hub := ids[0]
	finish := ids[len(ids)-1]
	out := make([]embeddedGraphEdge, 0, len(ids)*2)
	for _, id := range ids[1:] {
		out = append(out, embeddedGraphEdge{Source: hub, Target: id, Label: "dispatch"})
		if id != finish {
			out = append(out, embeddedGraphEdge{Source: id, Target: finish, Label: "flow"})
		}
	}
	if hub != finish {
		out = append(out, embeddedGraphEdge{Source: hub, Target: finish, Label: "flow"})
	}
	return out
}

type reviewLoopTemplate struct{}

func (reviewLoopTemplate) ID() string { return "critic_loop" }

func (reviewLoopTemplate) BuildEdges(_ Definition, ids []string) []embeddedGraphEdge {
	return pipelineTemplate{}.BuildEdges(Definition{}, ids)
}

type adaptiveTemplate struct{}

func (adaptiveTemplate) ID() string { return "adaptive" }

func (adaptiveTemplate) BuildEdges(def Definition, ids []string) []embeddedGraphEdge {
	out := pipelineTemplate{}.BuildEdges(Definition{}, ids)
	maxEdges := maxAdaptiveTransferEdges
	if def.Swarm != nil && def.Swarm.MaxHandoffs > 0 {
		maxEdges = def.Swarm.MaxHandoffs
	}
	transferCount := 0
	for i := 0; i < len(ids) && transferCount < maxEdges; i++ {
		for j := 0; j < len(ids) && transferCount < maxEdges; j++ {
			if i == j || j == i+1 {
				continue
			}
			out = append(out, embeddedGraphEdge{Source: ids[i], Target: ids[j], Label: "transfer"})
			transferCount++
		}
	}
	return out
}

const maxAdaptiveTransferEdges = 30

func init() {
	RegisterTemplate(pipelineTemplate{})
	RegisterTemplate(parallelReviewTemplate{})
	RegisterTemplate(dispatchTemplate{})
	RegisterTemplate(reviewLoopTemplate{})
	RegisterTemplate(adaptiveTemplate{})
}
