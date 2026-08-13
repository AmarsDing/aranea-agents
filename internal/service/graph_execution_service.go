package service

import (
	"context"
	"strings"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/telemetry/turntrace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// assertExecutionAccess resolves the execution's graph and enforces the same
// workspace access check as definition-plane RPCs (B3 IDOR fix). It returns
// the execution on success so callers can reuse it without a second lookup.
func (s *GraphService) assertExecutionAccess(ctx context.Context, executionID string) (*biz.GraphExecution, error) {
	exec, err := s.uc.GetExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	if err := s.assertGraphAccess(ctx, exec.GraphID); err != nil {
		return nil, err
	}
	return exec, nil
}

func (s *GraphService) ExecuteGraph(ctx context.Context, req *graphv1.ExecuteGraphRequest) (*graphv1.ExecuteGraphResponse, error) {
	if strings.TrimSpace(req.GetGraphId()) == "" {
		return nil, apierror.BadRequest("GRAPH", "graph_id is required")
	}
	// B3: enforce workspace access before starting the run.
	if err := s.assertGraphAccess(ctx, req.GraphId); err != nil {
		return nil, err
	}
	var initialState map[string]any
	if req.InitialState != nil {
		initialState = req.InitialState.AsMap()
	}
	execID := uuid.NewString()
	s.lg.Info("graph run start requested",
		loggateway.StepID("graph.execute"),
		loggateway.Str("graph_id", req.GetGraphId()),
		loggateway.Str("execution_id", execID),
		loggateway.Str("session_id", req.GetSessionId()),
	)
	ctx, traceBridge, _ := turntrace.Start(ctx, turntrace.Config{
		Domain:    turntrace.DomainGraph,
		SpanName:  "graph.execute",
		SessionID: req.GetSessionId(),
		RunID:     execID,
		AgentKey:  req.GetGraphId(),
	})
	if s.graphTel != nil {
		s.graphTel.Bind(execID, traceBridge)
	}
	// def is used only for the orchestration projector; access was already
	// verified above, so a reload miss here is non-fatal.
	def, defErr := s.uc.GetGraph(ctx, req.GraphId)
	exec, err := s.uc.ExecuteGraph(ctx, req.GraphId, req.SessionId, execID, initialState)
	if err != nil {
		s.lg.Error("graph run start failed",
			loggateway.StepID("graph.execute"),
			loggateway.Str("graph_id", req.GetGraphId()),
			loggateway.Str("execution_id", execID),
			loggateway.Str("session_id", req.GetSessionId()),
			loggateway.Err(err),
		)
		if flow := s.graphFlow(ctx, req.GetSessionId(), execID); flow != nil {
			flow.LogError("system.graph.task_start_fail", "图任务启动失败",
				event.P("graph_id", req.GetGraphId()),
				event.P("execution_id", execID),
				event.P("error", err.Error()),
			)
		}
		if s.graphTel != nil {
			s.graphTel.EnsureFinished(execID, err)
		}
		return nil, err
	}
	s.lg.Info("graph run started",
		loggateway.StepID("graph.execute"),
		loggateway.Str("graph_id", req.GetGraphId()),
		loggateway.Str("execution_id", execID),
		loggateway.Str("session_id", req.GetSessionId()),
		loggateway.Str("status", exec.Status),
	)
	if defErr == nil && def != nil && s.orchProjector != nil {
		s.orchProjector.Start(context.Background(), req.GetSessionId(), execID, req.GetGraphId(), def)
	}
	resp := &graphv1.ExecuteGraphResponse{
		ExecutionId: exec.ID,
		Status:      exec.Status,
	}
	if exec.CurrentState != nil {
		st, err := structpb.NewStruct(exec.CurrentState)
		if err == nil {
			resp.FinalState = st
		}
	}
	return resp, nil
}

func (s *GraphService) GetGraphExecution(ctx context.Context, req *graphv1.GetGraphExecutionRequest) (*graphv1.GetGraphExecutionResponse, error) {
	exec, err := s.assertExecutionAccess(ctx, req.ExecutionId)
	if err != nil {
		return nil, err
	}
	resp := &graphv1.GetGraphExecutionResponse{
		ExecutionId:   exec.ID,
		GraphId:       exec.GraphID,
		SessionId:     exec.SessionID,
		Status:        exec.Status,
		InterruptNode: exec.GetInterruptNode(),
		StartedAt:     timestamppb.New(exec.StartedAt),
	}
	if exec.FinishedAt != nil {
		resp.FinishedAt = timestamppb.New(*exec.FinishedAt)
	}
	if exec.CurrentState != nil {
		st, err := structpb.NewStruct(exec.CurrentState)
		if err == nil {
			resp.CurrentState = st
		}
	}
	if exec.Steps != nil {
		resp.Steps = make([]*graphv1.GraphStepSnapshot, len(exec.Steps))
		for i, step := range exec.Steps {
			resp.Steps[i] = toProtoStep(step)
		}
	}
	return resp, nil
}

func (s *GraphService) ListGraphExecutions(ctx context.Context, req *graphv1.ListGraphExecutionsRequest) (*graphv1.ListGraphExecutionsResponse, error) {
	// B3: enforce workspace access before listing runs.
	if err := s.assertGraphAccess(ctx, req.GraphId); err != nil {
		return nil, err
	}
	var opts []biz.GraphRunListOption
	if req.Status != nil && *req.Status != "" {
		opts = append(opts, biz.GraphRunListOption{Status: *req.Status})
	}
	if req.StartedAfter != nil {
		t := req.StartedAfter.AsTime()
		opts = append(opts, biz.GraphRunListOption{StartedAfter: &t})
	}
	execs, nextToken, err := s.uc.ListExecutions(ctx, req.GraphId, int(req.PageSize), req.PageToken, opts...)
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.GraphExecutionSummary, len(execs))
	for i, exec := range execs {
		summary := &graphv1.GraphExecutionSummary{
			ExecutionId:  exec.ID,
			GraphId:      exec.GraphID,
			SessionId:    exec.SessionID,
			Status:       exec.Status,
			CurrentNode:  exec.CurrentNode,
			LineageId:    exec.LineageID,
			ErrorMessage: exec.ErrorMessage,
			StartedAt:    timestamppb.New(exec.StartedAt),
		}
		if exec.FinishedAt != nil {
			summary.FinishedAt = timestamppb.New(*exec.FinishedAt)
		}
		items[i] = summary
	}
	return &graphv1.ListGraphExecutionsResponse{Items: items, NextPageToken: nextToken}, nil
}

func (s *GraphService) CancelGraphExecution(ctx context.Context, req *graphv1.CancelGraphExecutionRequest) (*graphv1.CancelGraphExecutionResponse, error) {
	s.lg.Info("graph run cancel requested",
		loggateway.StepID("graph.cancel"),
		loggateway.Str("execution_id", req.GetExecutionId()),
	)
	if _, err := s.assertExecutionAccess(ctx, req.ExecutionId); err != nil {
		return nil, err
	}
	err := s.uc.CancelExecution(ctx, req.ExecutionId)
	if err != nil {
		s.lg.Error("graph run cancel failed",
			loggateway.StepID("graph.cancel"),
			loggateway.Str("execution_id", req.GetExecutionId()),
			loggateway.Err(err),
		)
		return nil, err
	}
	exec, execErr := s.uc.GetExecution(ctx, req.ExecutionId)
	if execErr != nil {
		s.lg.Warn("get execution after cancel failed", loggateway.Err(execErr))
	}
	status := string(biz.GraphExecCancelled)
	if exec != nil {
		status = exec.Status
	}
	s.lg.Info("graph run cancelled",
		loggateway.StepID("graph.cancel"),
		loggateway.Str("execution_id", req.GetExecutionId()),
		loggateway.Str("status", status),
	)
	return &graphv1.CancelGraphExecutionResponse{
		ExecutionId: req.ExecutionId,
		Status:      status,
	}, nil
}

func (s *GraphService) ResumeGraph(ctx context.Context, req *graphv1.ResumeGraphRequest) (*graphv1.ResumeGraphResponse, error) {
	var resumeValue map[string]any
	if req.ResumeValue != nil {
		resumeValue = req.ResumeValue.AsMap()
	}
	s.lg.Info("graph run resume requested",
		loggateway.StepID("graph.resume"),
		loggateway.Str("execution_id", req.GetExecutionId()),
	)
	if _, err := s.assertExecutionAccess(ctx, req.ExecutionId); err != nil {
		return nil, err
	}
	exec, err := s.uc.ResumeExecution(ctx, req.ExecutionId, resumeValue)
	if err != nil {
		s.lg.Error("graph run resume failed",
			loggateway.StepID("graph.resume"),
			loggateway.Str("execution_id", req.GetExecutionId()),
			loggateway.Err(err),
		)
		// Best-effort session correlation for the flow log; the execution is
		// usually still cached in memory so this is cheap.
		var sessionID, graphID string
		if prev, prevErr := s.uc.GetExecution(ctx, req.ExecutionId); prevErr == nil && prev != nil {
			sessionID = prev.SessionID
			graphID = prev.GraphID
		}
		if flow := s.graphFlow(ctx, sessionID, req.GetExecutionId()); flow != nil {
			flow.LogError("system.graph.task_resume_fail", "图任务恢复失败",
				event.P("graph_id", graphID),
				event.P("execution_id", req.GetExecutionId()),
				event.P("error", err.Error()),
			)
		}
		return nil, err
	}
	s.lg.Info("graph run resumed",
		loggateway.StepID("graph.resume"),
		loggateway.Str("execution_id", req.GetExecutionId()),
		loggateway.Str("status", exec.Status),
	)
	resp := &graphv1.ResumeGraphResponse{
		ExecutionId: exec.ID,
		Status:      exec.Status,
	}
	if exec.CurrentState != nil {
		st, err := structpb.NewStruct(exec.CurrentState)
		if err == nil {
			resp.FinalState = st
		}
	}
	return resp, nil
}

func (s *GraphService) TimeTravelGraph(ctx context.Context, req *graphv1.TimeTravelGraphRequest) (*graphv1.TimeTravelGraphResponse, error) {
	// Use step_index to locate the execution step directly. The biz-layer
	// TimeTravelGetState signature accepts checkpointID/namespace (not stepIndex),
	// so we resolve the step from the execution and use its OutputState.
	exec, err := s.assertExecutionAccess(ctx, req.ExecutionId)
	if err != nil {
		return nil, err
	}
	idx := int(req.StepIndex)
	if idx < 0 || idx >= len(exec.Steps) {
		return nil, apierror.NotFound("GRAPH", "step index out of range")
	}
	step := exec.Steps[idx]
	resp := &graphv1.TimeTravelGraphResponse{
		ExecutionId: exec.ID,
		StepIndex:   int32(idx),
		NodeId:      step.NodeID,
	}
	if step.OutputState != nil {
		st, stErr := structpb.NewStruct(step.OutputState)
		if stErr == nil {
			resp.StateSnapshot = st
		}
	}
	return resp, nil
}

func (s *GraphService) ListCheckpoints(ctx context.Context, req *graphv1.ListCheckpointsRequest) (*graphv1.ListCheckpointsResponse, error) {
	if _, err := s.assertExecutionAccess(ctx, req.ExecutionId); err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}
	cpList, err := s.uc.ListCheckpoints(ctx, req.ExecutionId, "", limit)
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.CheckpointInfo, len(cpList))
	for i, cp := range cpList {
		items[i] = &graphv1.CheckpointInfo{
			LineageId:          cp.Ref.LineageID,
			Namespace:          cp.Ref.Namespace,
			CheckpointId:       cp.Ref.CheckpointID,
			ParentCheckpointId: cp.ParentCheckpoint,
			Source:             cp.Source,
			Step:               int32(cp.Step),
		}
		if !cp.Timestamp.IsZero() {
			items[i].Timestamp = timestamppb.New(cp.Timestamp)
		}
	}
	return &graphv1.ListCheckpointsResponse{Items: items}, nil
}

func (s *GraphService) GetStateSnapshot(ctx context.Context, req *graphv1.GetStateSnapshotRequest) (*graphv1.GetStateSnapshotResponse, error) {
	if _, err := s.assertExecutionAccess(ctx, req.ExecutionId); err != nil {
		return nil, err
	}
	result, err := s.uc.GetStateSnapshot(ctx, req.ExecutionId, req.CheckpointId, req.Namespace)
	if err != nil {
		return nil, err
	}
	resp := &graphv1.GetStateSnapshotResponse{}
	if result != nil {
		info := &graphv1.CheckpointInfo{
			LineageId:          result.Ref.LineageID,
			Namespace:          result.Ref.Namespace,
			CheckpointId:       result.Ref.CheckpointID,
			ParentCheckpointId: result.ParentCheckpoint,
			Source:             result.Source,
			Step:               int32(result.Step),
		}
		if !result.Timestamp.IsZero() {
			info.Timestamp = timestamppb.New(result.Timestamp)
		}
		resp.Snapshot = &graphv1.StateSnapshot{
			CheckpointInfo: info,
		}
		if result.State != nil {
			st, err := structpb.NewStruct(result.State)
			if err == nil {
				resp.Snapshot.State = st
			}
		}
		if result.NextNodes != nil {
			resp.Snapshot.NextNodes = result.NextNodes
		}
	}
	return resp, nil
}

func (s *GraphService) EditState(ctx context.Context, req *graphv1.EditStateRequest) (*graphv1.EditStateResponse, error) {
	if _, err := s.assertExecutionAccess(ctx, req.ExecutionId); err != nil {
		return nil, err
	}
	var patch map[string]any
	if req.Patch != nil {
		patch = req.Patch.AsMap()
	}
	result, err := s.uc.EditState(ctx, req.ExecutionId, req.CheckpointId, req.Namespace, patch)
	if err != nil {
		return nil, err
	}
	return &graphv1.EditStateResponse{
		NewCheckpointId: result.Ref.CheckpointID,
		LineageId:       result.Ref.LineageID,
		Namespace:       result.Ref.Namespace,
	}, nil
}
