package team

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 本文件承载 TeamGraphRunCoordinator 的 graph watch 支撑件：执行后端端口、
// steps_json 增量落库与 graph_executions 终态收敛（F-B）、watch notice meta
// 解析辅助。从 team_graph_run_coordinator.go 拆出以控制单文件行数（AS-COG-01）。

// TeamGraphExecutionBackend indexes and resumes team-linked graph executions.
type TeamGraphExecutionBackend interface {
	RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID string, ct *biz.CompiledTeam) error
	MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error
	RecordTeamGraphNodeEnd(ctx context.Context, execID, nodeID string, stepIndex int, status, errMsg string) error
	FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error
	ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*biz.GraphExecution, error)
	GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error)
}

// recordGraphNodeEnd mirrors the standalone graph path's node_end step upsert
// for team executions (F-B): the coordinator's watch is the only consumer of
// node lifecycle on this path, so it must persist steps_json itself.
func (c *TeamGraphRunCoordinator) recordGraphNodeEnd(ctx context.Context, execID string, st *biz.AgentNodeState, meta map[string]any) {
	if c == nil || c.graphs == nil || st == nil {
		return
	}
	status := "completed"
	errText := ""
	switch st.Status {
	case biz.AgentNodeStatusSkipped:
		status = "skipped"
	case biz.AgentNodeStatusFailed:
		status = "failed"
		errText = st.ErrorMessage
	}
	if err := c.graphs.RecordTeamGraphNodeEnd(ctx, strings.TrimSpace(execID), st.NodeID, metaInt(meta, "step_number"), status, errText); err != nil {
		c.lg.Warn("RecordTeamGraphNodeEnd failed",
			loggateway.StepID("team.graph.step_record_fail"),
			loggateway.Str("exec_id", strings.TrimSpace(execID)),
			loggateway.Str("node_id", st.NodeID),
			loggateway.Err(err))
	}
}

// FinalizeTeamGraphExecution converges the graph_executions row when the owning
// team run reaches a terminal state (F-B). Called on runner-driven paths
// (initial run success/failure) where no graph watch observes a terminal
// notice, and from finalizeTeamRun on watch-driven paths. Idempotent on the
// biz side, so duplicate calls (e.g. watch + runner) are safe.
func (c *TeamGraphRunCoordinator) FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error {
	if c == nil || c.graphs == nil {
		return nil
	}
	execID = strings.TrimSpace(execID)
	if execID == "" {
		return nil
	}
	if err := c.graphs.FinalizeTeamGraphExecution(ctx, execID, failed, errMsg); err != nil {
		c.lg.Warn("FinalizeTeamGraphExecution failed",
			loggateway.StepID("team.graph.exec_finalize_fail"),
			loggateway.Str("exec_id", execID),
			loggateway.Bool("failed", failed),
			loggateway.Err(err))
		return err
	}
	return nil
}

func resumeStepNodeID(meta map[string]any, reg biz.OrchestrationRegistry) string {
	if meta == nil {
		return ""
	}
	step, ok := meta["step"].(map[string]any)
	if !ok {
		return ""
	}
	if agentID, ok := step["agent_id"].(string); ok && strings.TrimSpace(agentID) != "" {
		for nodeID, entry := range reg.ByNodeID {
			if strings.EqualFold(entry.AgentID, strings.TrimSpace(agentID)) {
				return nodeID
			}
		}
	}
	return ""
}

func resumeMetaBool(meta map[string]any, key string) bool {
	if meta == nil {
		return false
	}
	v, ok := meta[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func metaInt(meta map[string]any, key string) int {
	if meta == nil {
		return 0
	}
	switch v := meta[key].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	}
	return 0
}

func metaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}
