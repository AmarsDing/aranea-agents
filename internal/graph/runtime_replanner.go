// Package graph implements the runtime graph orchestration components,
// including NL2Graph conversion and runtime replanning on node failures.
package graph

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// ReplanType identifies the kind of replan action to take after a node failure.
type ReplanType string

const (
	// ReplanRetry re-executes the failed node (transient failures).
	ReplanRetry ReplanType = "retry"
	// ReplanReroute routes execution to an alternative path (blocked/unreachable downstream).
	ReplanReroute ReplanType = "reroute"
	// ReplanInsertFallback inserts a fallback node to handle the failed node's task.
	ReplanInsertFallback ReplanType = "insert_fallback"
	// ReplanRebuildSubgraph replaces the failed node with a new subgraph.
	ReplanRebuildSubgraph ReplanType = "rebuild_subgraph"
)

// maxReplanAttempts bounds the number of replans per execution to prevent
// infinite replan loops (see design §十 risk #3).
const maxReplanAttempts = 3

// Failure severity levels identified by the rule-based analyzer.
const (
	failureSeverityTransient      = "transient"
	failureSeverityAgentIncapable = "agent_incapable"
	failureSeveritySubtaskInvalid = "subtask_invalid"
	failureSeverityRouteBlocked   = "route_blocked"
	failureSeverityUnknown        = "unknown"
)

// ReplanAction describes the action to take after a node failure.
// The caller (executor) is responsible for applying the action to the graph.
type ReplanAction struct {
	Type      ReplanType
	NewNodes  []biz.NodeDef
	NewEdges  []biz.EdgeDef
	SkipNodes []string
}

// FailureAnalysis describes the analyzed cause of a node failure.
type FailureAnalysis struct {
	Severity        string
	Reason          string
	SuggestedAction ReplanType
}

// RuntimeReplanner analyzes node failures and decides replan actions.
// Implementations must be safe for concurrent use across executions.
type RuntimeReplanner interface {
	OnNodeFailure(ctx context.Context, exec *biz.GraphExecution, failedNode string, err error) (*ReplanAction, error)
}

// RuntimeReplannerImpl implements RuntimeReplanner using rule-based failure
// analysis. It tracks replan attempts per execution ID to enforce the
// maxReplanAttempts limit.
type RuntimeReplannerImpl struct {
	eventBus biz.EventBus
	lg       loggateway.Logger

	// attemptCount tracks per-execution replan attempts. ttl=0 means no
	// automatic TTL cleanup; callers must invoke ReleaseExecution(execID)
	// when an execution completes to prevent unbounded growth (A5).
	attemptCount *lifecycle.ManagedMap[string, int]
}

var _ RuntimeReplanner = (*RuntimeReplannerImpl)(nil)

// NewRuntimeReplanner creates a RuntimeReplannerImpl with the given v2
// event bus and logger. The bus is used to publish graph_stage
// ActivityBridgeEvents (Stage="replanned") so the replan is observable on
// the frontend timeline.
func NewRuntimeReplanner(eventBus biz.EventBus, lg loggateway.Logger) *RuntimeReplannerImpl {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &RuntimeReplannerImpl{
		eventBus:     eventBus,
		lg:           lg.With(loggateway.Domain("runtime_replanner")),
		attemptCount: lifecycle.NewManagedMap[string, int](0),
	}
}

// OnNodeFailure analyzes a node failure and returns the replan action to take.
// Returns an error when:
//   - exec is nil (caller bug, red line #26)
//   - err is nil (no failure to analyze)
//   - failure severity is unknown (cannot decide a replan)
//   - max replan attempts exceeded for this execution
func (r *RuntimeReplannerImpl) OnNodeFailure(
	ctx context.Context,
	exec *biz.GraphExecution,
	failedNode string,
	err error,
) (*ReplanAction, error) {
	if exec == nil {
		return nil, apierror.BadRequest(apierror.DomainGraph, "runtime replanner: execution is nil")
	}
	if err == nil {
		return nil, apierror.BadRequest(apierror.DomainGraph, "runtime replanner: failure error is nil")
	}

	if !r.tryAcquireAttempt(exec.ID) {
		r.lg.Warn("runtime replanner: max attempts exceeded, failing execution",
			loggateway.StepID("replan.max_exceeded"),
			loggateway.Str("execution_id", exec.ID),
			loggateway.Str("failed_node", failedNode),
			loggateway.Int("max_attempts", maxReplanAttempts),
		)
		return nil, apierror.Internal(apierror.DomainGraph,
			"runtime replanner: max replan attempts exceeded for execution %s", exec.ID)
	}

	analysis := r.analyzeFailure(err)
	action, err := r.buildAction(ctx, exec, failedNode, analysis)
	if err != nil {
		// Release the attempt on error so the caller can retry with a different strategy.
		r.releaseAttempt(exec.ID)
		return nil, err
	}

	r.publishReplanEvent(ctx, exec, failedNode, action, analysis)
	metrics.GraphReplanTotal.WithLabelValues(string(action.Type)).Inc()

	r.lg.Info("runtime replanner: replan action decided",
		loggateway.StepID("replan.decided"),
		loggateway.Str("execution_id", exec.ID),
		loggateway.Str("failed_node", failedNode),
		loggateway.Str("replan_type", string(action.Type)),
		loggateway.Str("severity", analysis.Severity),
	)

	return action, nil
}

// tryAcquireAttempt increments the per-execution replan counter and returns
// false if the max has already been reached. Safe for concurrent use.
//
// 使用 ManagedMap.UpdateOrStoreWithResult 原子地 check-and-increment，
// 根治 TOCTOU 窗口（A5）。
func (r *RuntimeReplannerImpl) tryAcquireAttempt(execID string) bool {
	_, allowed := r.attemptCount.UpdateOrStoreWithResult(execID, func(existing int, ok bool) (int, bool) {
		if ok && existing >= maxReplanAttempts {
			// 已达上限，不递增，返回 false
			return existing, false
		}
		// 无 entry（ok=false, existing=0）或未达上限，递增并返回 true
		return existing + 1, true
	})
	return allowed
}

// releaseAttempt decrements the per-execution replan counter. Used when a
// replan attempt errors out before producing an action, so the caller can
// retry with a different strategy without being blocked by the limit.
func (r *RuntimeReplannerImpl) releaseAttempt(execID string) {
	r.attemptCount.UpdateOrStore(execID, func(existing int, ok bool) int {
		if !ok || existing <= 0 {
			return 0
		}
		return existing - 1
	})
}

// ReleaseExecution 清理指定 execution 的 replan 计数 entry。
// 必须在 execution 终态（完成/失败/取消）时由调用方调用，防止 map 无限增长（A5）。
func (r *RuntimeReplannerImpl) ReleaseExecution(execID string) {
	r.attemptCount.Delete(execID)
}

// buildAction maps a FailureAnalysis to a concrete ReplanAction.
func (r *RuntimeReplannerImpl) buildAction(
	_ context.Context,
	exec *biz.GraphExecution,
	failedNode string,
	analysis FailureAnalysis,
) (*ReplanAction, error) {
	switch analysis.Severity {
	case failureSeverityTransient:
		return &ReplanAction{Type: ReplanRetry}, nil
	case failureSeverityAgentIncapable:
		return r.buildInsertFallbackAction(exec, failedNode, analysis), nil
	case failureSeveritySubtaskInvalid:
		return r.buildRebuildSubgraphAction(exec, failedNode, analysis), nil
	case failureSeverityRouteBlocked:
		return &ReplanAction{Type: ReplanReroute}, nil
	default:
		return nil, apierror.Internal(apierror.DomainGraph,
			"runtime replanner: unknown failure severity %q for node %s", analysis.Severity, failedNode)
	}
}

// buildInsertFallbackAction creates a fallback node that takes over the
// failed node's task. The fallback node is wired in via two edges:
// prev→fallback and fallback→next. For simplicity the "prev"/"next" IDs
// are derived from the failed node ID; the executor is responsible for
// resolving them against the actual graph topology.
func (r *RuntimeReplannerImpl) buildInsertFallbackAction(
	exec *biz.GraphExecution,
	failedNode string,
	analysis FailureAnalysis,
) *ReplanAction {
	fallbackID := failedNode + "_fallback"
	fallback := biz.NodeDef{
		ID:          fallbackID,
		Type:        biz.NodeTypeAgent,
		Description: "Fallback for " + failedNode + ": " + analysis.Reason,
		FuncRef:     agentInvokeFuncRef,
	}
	return &ReplanAction{
		Type:     ReplanInsertFallback,
		NewNodes: []biz.NodeDef{fallback},
		NewEdges: []biz.EdgeDef{
			{From: failedNode + "_prev", To: fallbackID, Kind: biz.EdgeKindFallback},
			{From: fallbackID, To: failedNode + "_next", Kind: biz.EdgeKindFlow},
		},
	}
}

// buildRebuildSubgraphAction replaces the failed node with a small sequential
// subgraph (split→process→merge). The failed node is added to SkipNodes so
// the executor does not re-execute it.
func (r *RuntimeReplannerImpl) buildRebuildSubgraphAction(
	_ *biz.GraphExecution,
	failedNode string,
	analysis FailureAnalysis,
) *ReplanAction {
	splitID := failedNode + "_split"
	processID := failedNode + "_process"
	mergeID := failedNode + "_merge"
	nodes := []biz.NodeDef{
		{ID: splitID, Type: biz.NodeTypeFunction, Description: "Split input for " + failedNode, FuncRef: "function.split"},
		{ID: processID, Type: biz.NodeTypeAgent, Description: "Rebuilt processing for " + failedNode + ": " + analysis.Reason, FuncRef: agentInvokeFuncRef},
		{ID: mergeID, Type: biz.NodeTypeFunction, Description: "Merge output for " + failedNode, FuncRef: "function.merge"},
	}
	edges := []biz.EdgeDef{
		{From: splitID, To: processID, Kind: biz.EdgeKindFlow},
		{From: processID, To: mergeID, Kind: biz.EdgeKindFlow},
	}
	return &ReplanAction{
		Type:      ReplanRebuildSubgraph,
		NewNodes:  nodes,
		NewEdges:  edges,
		SkipNodes: []string{failedNode},
	}
}

// publishReplanEvent publishes a graph_stage ActivityBridgeEvent (Stage="replanned")
// so the frontend timeline can render the replan decision. Classified as
// Important (AS-EVT-01): loss causes topology drift but execution continues.
func (r *RuntimeReplannerImpl) publishReplanEvent(
	ctx context.Context,
	exec *biz.GraphExecution,
	failedNode string,
	action *ReplanAction,
	analysis FailureAnalysis,
) {
	if r.eventBus == nil {
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
			Stage:           "replanned",
			Meta: map[string]any{
				"execution_id":   exec.ID,
				"graph_id":       exec.GraphID,
				"failed_node":    failedNode,
				"replan_type":    string(action.Type),
				"severity":       analysis.Severity,
				"reason":         analysis.Reason,
				"new_node_count": len(action.NewNodes),
				"skip_nodes":     action.SkipNodes,
				"author":         "runtime-replanner",
			},
		},
		Domain: biz.ActivityDomainChat,
	}
	r.eventBus.Publish(ctx, biz.NewActivityBridgeEvent(ev))
}

// analyzeFailure inspects the error message using keyword rules and returns
// a FailureAnalysis with the severity and suggested replan action. The
// analysis is deterministic and side-effect free, making it easy to test.
//
// Rule precedence (first match wins):
//  1. transient      → retry          (timeout/connection/temporary/deadline)
//  2. agent_incapable → insert_fallback (incapable/unable/not supported/unsupported)
//  3. subtask_invalid → rebuild_subgraph (invalid/malformed/incorrect/bad request)
//  4. route_blocked   → reroute        (blocked/unreachable/denied/forbidden)
//  5. unknown         → error          (no keyword matched)
func (r *RuntimeReplannerImpl) analyzeFailure(err error) FailureAnalysis {
	if err == nil {
		return FailureAnalysis{Severity: failureSeverityUnknown}
	}
	msg := strings.ToLower(err.Error())

	for _, kw := range transientKeywords {
		if strings.Contains(msg, kw) {
			return FailureAnalysis{
				Severity:        failureSeverityTransient,
				Reason:          "transient failure: " + err.Error(),
				SuggestedAction: ReplanRetry,
			}
		}
	}
	for _, kw := range incapableKeywords {
		if strings.Contains(msg, kw) {
			return FailureAnalysis{
				Severity:        failureSeverityAgentIncapable,
				Reason:          "agent incapable: " + err.Error(),
				SuggestedAction: ReplanInsertFallback,
			}
		}
	}
	for _, kw := range subtaskInvalidKeywords {
		if strings.Contains(msg, kw) {
			return FailureAnalysis{
				Severity:        failureSeveritySubtaskInvalid,
				Reason:          "subtask invalid: " + err.Error(),
				SuggestedAction: ReplanRebuildSubgraph,
			}
		}
	}
	for _, kw := range routeBlockedKeywords {
		if strings.Contains(msg, kw) {
			return FailureAnalysis{
				Severity:        failureSeverityRouteBlocked,
				Reason:          "route blocked: " + err.Error(),
				SuggestedAction: ReplanReroute,
			}
		}
	}
	return FailureAnalysis{Severity: failureSeverityUnknown, Reason: err.Error()}
}

// Keyword tables for rule-based failure analysis. Kept as package-level vars
// so they can be extended without modifying the analyzer function.
var (
	transientKeywords = []string{
		"timeout", "timed out", "connection", "temporary", "transient",
		"deadline", "unavailable", "retryable",
	}
	incapableKeywords = []string{
		"incapable", "unable to handle", "not supported", "unsupported",
		"cannot handle", "no capability",
	}
	subtaskInvalidKeywords = []string{
		"invalid", "malformed", "incorrect", "bad request",
		"schema error", "validation failed",
	}
	routeBlockedKeywords = []string{
		"blocked", "unreachable", "denied", "forbidden",
		"not allowed", "access refused",
	}
)
