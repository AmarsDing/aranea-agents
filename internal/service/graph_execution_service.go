package service

import (
	"context"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/telemetry/turntrace"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *GraphService) ExecuteGraph(ctx context.Context, req *graphv1.ExecuteGraphRequest) (*graphv1.ExecuteGraphResponse, error) {
	var initialState map[string]any
	if req.InitialState != nil {
		initialState = req.InitialState.AsMap()
	}
	execID := uuid.NewString()
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
	def, defErr := s.uc.GetGraph(ctx, req.GraphId)
	exec, err := s.uc.ExecuteGraph(ctx, req.GraphId, req.SessionId, execID, initialState)
	if err != nil {
		if s.graphTel != nil {
			s.graphTel.EnsureFinished(execID, err)
		}
		return nil, err
	}
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
	exec, err := s.uc.GetExecution(ctx, req.ExecutionId)
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
	err := s.uc.CancelExecution(ctx, req.ExecutionId)
	if err != nil {
		return nil, err
	}
	exec, _ := s.uc.GetExecution(ctx, req.ExecutionId)
	status := "cancelled"
	if exec != nil {
		status = exec.Status
	}
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
	exec, err := s.uc.ResumeExecution(ctx, req.ExecutionId, resumeValue)
	if err != nil {
		return nil, err
	}
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
	result, err := s.uc.TimeTravelGetState(ctx, req.ExecutionId, "", "")
	if err != nil {
		exec, execErr := s.uc.GetExecution(ctx, req.ExecutionId)
		if execErr != nil {
			return nil, execErr
		}
		idx := int(req.StepIndex)
		if idx < 0 || idx >= len(exec.Steps) {
			return nil, biz.ErrNotFound
		}
		step := exec.Steps[idx]
		resp := &graphv1.TimeTravelGraphResponse{
			ExecutionId: exec.ID,
			StepIndex:   int32(idx),
			NodeId:      step.NodeID,
		}
		if step.OutputState != nil {
			st, err := structpb.NewStruct(step.OutputState)
			if err == nil {
				resp.StateSnapshot = st
			}
		}
		return resp, nil
	}
	snapshot, _ := result.(*trpcgraph.StateSnapshot)
	resp := &graphv1.TimeTravelGraphResponse{
		ExecutionId: req.ExecutionId,
		StepIndex:   req.StepIndex,
	}
	if snapshot != nil && snapshot.State != nil {
		st, err := structpb.NewStruct(snapshot.State)
		if err == nil {
			resp.StateSnapshot = st
		}
	}
	return resp, nil
}

func (s *GraphService) ListCheckpoints(ctx context.Context, req *graphv1.ListCheckpointsRequest) (*graphv1.ListCheckpointsResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}
	checkpoints, err := s.uc.ListCheckpoints(ctx, req.ExecutionId, "", limit)
	if err != nil {
		return nil, err
	}
	cpList, ok := checkpoints.([]trpcgraph.CheckpointInfo)
	if !ok {
		return nil, kerrors.InternalServer("GRAPH", "list checkpoints: unexpected result type")
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
	result, err := s.uc.GetStateSnapshot(ctx, req.ExecutionId, req.CheckpointId, req.Namespace)
	if err != nil {
		return nil, err
	}
	snapshot, _ := result.(*trpcgraph.StateSnapshot)
	resp := &graphv1.GetStateSnapshotResponse{}
	if snapshot != nil {
		info := &graphv1.CheckpointInfo{
			LineageId:          snapshot.Ref.LineageID,
			Namespace:          snapshot.Ref.Namespace,
			CheckpointId:       snapshot.Ref.CheckpointID,
			ParentCheckpointId: snapshot.ParentCheckpoint,
			Source:             snapshot.Source,
			Step:               int32(snapshot.Step),
		}
		if !snapshot.Timestamp.IsZero() {
			info.Timestamp = timestamppb.New(snapshot.Timestamp)
		}
		resp.Snapshot = &graphv1.StateSnapshot{
			CheckpointInfo: info,
		}
		if snapshot.State != nil {
			st, err := structpb.NewStruct(snapshot.State)
			if err == nil {
				resp.Snapshot.State = st
			}
		}
		if snapshot.NextNodes != nil {
			resp.Snapshot.NextNodes = snapshot.NextNodes
		}
	}
	return resp, nil
}

func (s *GraphService) EditState(ctx context.Context, req *graphv1.EditStateRequest) (*graphv1.EditStateResponse, error) {
	var patch map[string]any
	if req.Patch != nil {
		patch = req.Patch.AsMap()
	}
	result, err := s.uc.EditState(ctx, req.ExecutionId, req.CheckpointId, req.Namespace, patch)
	if err != nil {
		return nil, err
	}
	ref, ok := result.(trpcgraph.CheckpointRef)
	if !ok {
		return nil, kerrors.InternalServer("GRAPH", "edit state: unexpected result type")
	}
	return &graphv1.EditStateResponse{
		NewCheckpointId: ref.CheckpointID,
		LineageId:       ref.LineageID,
		Namespace:       ref.Namespace,
	}, nil
}
