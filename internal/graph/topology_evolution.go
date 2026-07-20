package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// topologyEvolutionLLMTimeout bounds the LLM call for edge decisions to avoid
// blocking the execution loop on slow models.
const topologyEvolutionLLMTimeout = 30 * time.Second

// ExecutionInsight describes a runtime observation that may warrant adding a
// new transfer edge between two nodes. Produced by the executor when a node's
// output reveals an unexpected dependency on another node's capability.
type ExecutionInsight struct {
	// SourceNode is the node that produced the insight (potential edge source).
	SourceNode string
	// TargetNode is the node whose capability is now needed (potential edge target).
	TargetNode string
	// Reason is a human-readable explanation of why the edge might be needed.
	Reason string
	// Evidence is a short summary of the source node's output that triggered
	// the insight (e.g. "node A output mentioned needing translation capability").
	Evidence string
}

// TopologyEvolver decides whether to dynamically add a transfer edge to a
// running graph execution based on execution insights. Implementations must be
// safe for concurrent use across executions.
type TopologyEvolver interface {
	OnExecutionInsight(ctx context.Context, exec *biz.GraphExecution, insight ExecutionInsight) (*biz.EdgeDef, error)
}

// TopologyEvolverImpl implements TopologyEvolver using LLM-based edge decisions.
// It tracks edges added per execution ID to prevent duplicates.
type TopologyEvolverImpl struct {
	llm      trpcmodel.Model
	eventBus biz.EventBus
	lg       loggateway.Logger

	mu         sync.Mutex
	addedEdges map[string]map[string]bool // execID -> "from->to" -> added
	// TECH-DEBT: addedEdges is never cleaned up when executions complete,
	// which may cause memory growth in long-running processes. Same pattern
	// as RuntimeReplannerImpl.attemptCount. Add TTL cleanup or clear on
	// execution completion in a future iteration.
}

var _ TopologyEvolver = (*TopologyEvolverImpl)(nil)

// NewTopologyEvolver creates a TopologyEvolverImpl with the given LLM,
// v2 event bus, and logger. The bus publishes graph_stage ActivityBridgeEvents
// (Stage="topology_evolved") so topology changes are observable on the
// frontend timeline. nil logger is replaced with a noop (red line #26).
func NewTopologyEvolver(llm trpcmodel.Model, eventBus biz.EventBus, lg loggateway.Logger) *TopologyEvolverImpl {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &TopologyEvolverImpl{
		llm:        llm,
		eventBus:   eventBus,
		lg:         lg.With(loggateway.Domain("topology_evolver")),
		addedEdges: make(map[string]map[string]bool),
	}
}

// OnExecutionInsight evaluates an execution insight and, if the LLM agrees,
// returns a new transfer edge to add to the graph. Returns nil edge when:
//   - LLM decides not to add the edge
//   - LLM call fails (degrade gracefully, no error)
//   - LLM is nil (degrade gracefully, no error)
//   - the edge was already added by this evolver for this execution
//
// Returns an error when:
//   - exec is nil (caller bug, red line #26)
//   - SourceNode or TargetNode is empty
//   - SourceNode equals TargetNode (self-loop)
func (e *TopologyEvolverImpl) OnExecutionInsight(
	ctx context.Context,
	exec *biz.GraphExecution,
	insight ExecutionInsight,
) (*biz.EdgeDef, error) {
	if exec == nil {
		return nil, apierror.BadRequest(apierror.DomainGraph, "topology evolver: execution is nil")
	}
	if strings.TrimSpace(insight.SourceNode) == "" || strings.TrimSpace(insight.TargetNode) == "" {
		return nil, apierror.BadRequest(apierror.DomainGraph,
			"topology evolver: source and target nodes are required")
	}
	if insight.SourceNode == insight.TargetNode {
		return nil, apierror.BadRequest(apierror.DomainGraph,
			"topology evolver: self-loop edge is not allowed (source=%s)", insight.SourceNode)
	}

	edgeKey := insight.SourceNode + "->" + insight.TargetNode
	if e.edgeAlreadyAdded(exec.ID, edgeKey) {
		e.lg.Warn("topology evolver: edge already added, skipping",
			loggateway.StepID("topology.duplicate"),
			loggateway.Str("execution_id", exec.ID),
			loggateway.Str("from", insight.SourceNode),
			loggateway.Str("to", insight.TargetNode),
		)
		return nil, nil
	}

	shouldAdd, err := e.llmDecideEdge(ctx, exec, insight)
	if err != nil {
		// Degrade gracefully: LLM failure should not break execution.
		e.lg.Warn("topology evolver: LLM decision failed, skipping edge addition",
			loggateway.StepID("topology.llm_failed"),
			loggateway.Str("execution_id", exec.ID),
			loggateway.Err(err),
		)
		return nil, nil
	}
	if !shouldAdd {
		e.lg.Info("topology evolver: LLM decided not to add edge",
			loggateway.StepID("topology.skipped"),
			loggateway.Str("execution_id", exec.ID),
			loggateway.Str("from", insight.SourceNode),
			loggateway.Str("to", insight.TargetNode),
		)
		return nil, nil
	}

	newEdge := biz.EdgeDef{
		From: insight.SourceNode,
		To:   insight.TargetNode,
		Kind: biz.EdgeKindTransfer,
	}

	e.markEdgeAdded(exec.ID, edgeKey)
	e.publishTopologyEvolvedEvent(ctx, exec, newEdge, insight)

	e.lg.Info("topology evolver: edge added",
		loggateway.StepID("topology.added"),
		loggateway.Str("execution_id", exec.ID),
		loggateway.Str("from", insight.SourceNode),
		loggateway.Str("to", insight.TargetNode),
		loggateway.Str("reason", insight.Reason),
	)

	return &newEdge, nil
}

// llmDecideEdge calls the LLM to decide whether to add a transfer edge based
// on the execution insight. Returns true when the LLM agrees the edge should
// be added. Returns an error when the LLM call or response parsing fails so
// the caller can log it; the caller is expected to degrade gracefully (no edge)
// on error rather than aborting execution.
func (e *TopologyEvolverImpl) llmDecideEdge(ctx context.Context, exec *biz.GraphExecution, insight ExecutionInsight) (bool, error) {
	if e.llm == nil {
		return false, apierror.Internal(apierror.DomainGraph, "topology evolver: LLM is nil")
	}

	prompt := buildTopologyEvolutionPrompt(exec, insight)
	callCtx, cancel := context.WithTimeout(ctx, topologyEvolutionLLMTimeout)
	defer cancel()

	req := trpcmodel.NewRequest([]trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: topologyEvolutionSystemPrompt()},
		{Role: trpcmodel.RoleUser, Content: prompt},
	})

	respChan, err := e.llm.GenerateContent(callCtx, req)
	if err != nil {
		return false, fmt.Errorf("topology evolver: LLM generate content: %w", err)
	}

	text, err := consumeModelResponse(respChan)
	if err != nil {
		return false, fmt.Errorf("topology evolver: consume LLM response: %w", err)
	}

	return e.parseEdgeDecision(text), nil
}

// edgeDecision is the LLM-parsed edge decision result.
type edgeDecision struct {
	ShouldAddEdge bool   `json:"should_add_edge"`
	Reason        string `json:"reason"`
}

// parseEdgeDecision parses the LLM response as JSON. On failure, logs a
// warning and returns false (degrade gracefully).
func (e *TopologyEvolverImpl) parseEdgeDecision(text string) bool {
	text = stripFences(text)
	var decision edgeDecision
	if err := json.Unmarshal([]byte(text), &decision); err != nil {
		e.lg.Warn("topology evolver: LLM returned malformed JSON, skipping edge addition",
			loggateway.StepID("topology.parse_failed"),
			loggateway.Err(err),
		)
		return false
	}
	return decision.ShouldAddEdge
}

// edgeAlreadyAdded checks whether an edge (by "from->to" key) has already been
// added by this evolver for the given execution. Safe for concurrent use.
func (e *TopologyEvolverImpl) edgeAlreadyAdded(execID, edgeKey string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if edges, ok := e.addedEdges[execID]; ok {
		return edges[edgeKey]
	}
	return false
}

// markEdgeAdded records that an edge has been added for the given execution.
// Safe for concurrent use.
func (e *TopologyEvolverImpl) markEdgeAdded(execID, edgeKey string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.addedEdges[execID] == nil {
		e.addedEdges[execID] = make(map[string]bool)
	}
	e.addedEdges[execID][edgeKey] = true
}

// publishTopologyEvolvedEvent publishes a graph_stage ActivityBridgeEvent
// (Stage="topology_evolved") so the frontend timeline can render the topology
// change. Classified as Important (AS-EVT-01): loss causes topology drift but
// execution continues.
func (e *TopologyEvolverImpl) publishTopologyEvolvedEvent(
	ctx context.Context,
	exec *biz.GraphExecution,
	edge biz.EdgeDef,
	insight ExecutionInsight,
) {
	if e.eventBus == nil {
		return
	}
	// Phase 3b-D: bridge to v2 EventBus. graph_stage has no typed v2 EventKind;
	// ActivityBridgeEvent preserves the v1 payload for the frontend timeline.
	ev := biz.ActivityEvent{
		Event: biz.ActivityEventUpdated,
		Activity: biz.Activity{
			ID:              uuid.NewString(),
			Kind:            biz.ActivityKindGraphStage,
			Status:          biz.ActivityStatusRunning,
			SessionID:       exec.SessionID,
			SpiritSessionID: exec.SpiritSessionID,
			Timestamp:       time.Now().UTC(),
			Stage:           "topology_evolved",
			Meta: map[string]any{
				"execution_id": exec.ID,
				"graph_id":     exec.GraphID,
				"from_node":    edge.From,
				"to_node":      edge.To,
				"edge_kind":    edge.Kind,
				"reason":       insight.Reason,
				"evidence":     insight.Evidence,
				"author":       "topology-evolver",
			},
		},
		Domain: biz.ActivityDomainChat,
	}
	e.eventBus.Publish(ctx, biz.NewActivityBridgeEvent(ev))
}

// buildTopologyEvolutionPrompt builds the user prompt for the LLM edge decision.
func buildTopologyEvolutionPrompt(exec *biz.GraphExecution, insight ExecutionInsight) string {
	var sb strings.Builder
	sb.WriteString("A graph execution has produced an insight suggesting a new transfer edge.\n\n")
	sb.WriteString("Execution ID: " + exec.ID + "\n")
	sb.WriteString("Graph ID: " + exec.GraphID + "\n\n")
	sb.WriteString("Insight details:\n")
	sb.WriteString("  Source node: " + insight.SourceNode + "\n")
	sb.WriteString("  Target node: " + insight.TargetNode + "\n")
	sb.WriteString("  Reason: " + insight.Reason + "\n")
	sb.WriteString("  Evidence: " + insight.Evidence + "\n\n")
	sb.WriteString("Decide whether adding a transfer edge from source to target would improve the execution.\n")
	sb.WriteString("Output ONLY a JSON object with the schema specified in the system prompt.\n")
	return sb.String()
}

// topologyEvolutionSystemPrompt returns the system prompt for the LLM edge decision.
func topologyEvolutionSystemPrompt() string {
	return `You are a graph orchestration topology advisor. Decide whether a new transfer edge should be added to a running graph execution based on an execution insight.

Return JSON with this schema:
{
  "should_add_edge": true,
  "reason": "explanation for the decision"
}

Rules:
- Output ONLY a JSON object, no markdown fences, no explanation outside JSON
- should_add_edge: true if the edge represents a genuine capability gap that the target node can fill
- should_add_edge: false if the edge is redundant, would create a cycle, or the insight is speculative
- reason: brief justification (1-2 sentences)
- Be conservative: prefer false when uncertain to avoid topology drift`
}
