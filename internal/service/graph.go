package service

import (
	"context"
	"fmt"
	"strings"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/telemetry/turntrace"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type GraphService struct {
	graphv1.UnimplementedGraphServiceServer

	uc            *biz.GraphUsecase
	taskUC        *biz.TaskUsecase
	graphTel      *GraphExecutionTelemetry
	orchProjector *GraphOrchestrationProjector
}

func NewGraphService(uc *biz.GraphUsecase, taskUC *biz.TaskUsecase, graphTel *GraphExecutionTelemetry, orchProjector *GraphOrchestrationProjector) *GraphService {
	return &GraphService{uc: uc, taskUC: taskUC, graphTel: graphTel, orchProjector: orchProjector}
}

func (s *GraphService) CreateGraph(ctx context.Context, req *graphv1.CreateGraphRequest) (*graphv1.CreateGraphResponse, error) {
	def := &biz.GraphDefinition{
		Name:             req.Name,
		Description:      req.Description,
		EntryPoint:       req.EntryPoint,
		FinishPoint:      req.FinishPoint,
		EnableCheckpoint: req.EnableCheckpoint,
		ExecutionEngine:  biz.ExecutionEngineType(req.ExecutionEngine),
		InterruptBefore:  req.InterruptBefore,
		InterruptAfter:   req.InterruptAfter,
	}
	if req.StateFields != nil {
		def.StateFields = make([]biz.StateFieldDef, len(req.StateFields))
		for i, sf := range req.StateFields {
			def.StateFields[i] = fromProtoStateField(sf)
		}
	}
	if req.Nodes != nil {
		def.Nodes = make([]biz.NodeDef, len(req.Nodes))
		for i, n := range req.Nodes {
			def.Nodes[i] = fromProtoNode(n)
		}
	}
	if req.Edges != nil {
		def.Edges = make([]biz.EdgeDef, len(req.Edges))
		for i, e := range req.Edges {
			def.Edges[i] = biz.EdgeDef{From: e.From, To: e.To}
		}
	}
	if req.ConditionalEdges != nil {
		def.ConditionalEdges = make([]biz.ConditionalEdgeDef, len(req.ConditionalEdges))
		for i, ce := range req.ConditionalEdges {
			def.ConditionalEdges[i] = fromProtoCondEdge(ce)
		}
	}
	if req.Subgraphs != nil {
		def.Subgraphs = make([]biz.SubgraphDef, len(req.Subgraphs))
		for i, sub := range req.Subgraphs {
			def.Subgraphs[i] = fromProtoSubgraph(sub)
		}
	}
	if req.Metadata != nil {
		def.Metadata = req.Metadata.AsMap()
	}
	saved, err := s.uc.CreateGraph(ctx, def)
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(saved)
	if err != nil {
		return nil, err
	}
	return &graphv1.CreateGraphResponse{Graph: pb}, nil
}

func (s *GraphService) GetGraph(ctx context.Context, req *graphv1.GetGraphRequest) (*graphv1.GetGraphResponse, error) {
	def, err := s.uc.GetGraph(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(def)
	if err != nil {
		return nil, err
	}
	return &graphv1.GetGraphResponse{Graph: pb}, nil
}

func (s *GraphService) ListGraphs(ctx context.Context, req *graphv1.ListGraphsRequest) (*graphv1.ListGraphsResponse, error) {
	defs, nextToken, err := s.uc.ListGraphs(ctx, int(req.PageSize), req.PageToken)
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.GraphDefinition, len(defs))
	for i, def := range defs {
		pb, err := toProtoGraph(def)
		if err != nil {
			return nil, err
		}
		items[i] = pb
	}
	return &graphv1.ListGraphsResponse{Items: items, NextPageToken: nextToken}, nil
}

func (s *GraphService) UpdateGraph(ctx context.Context, req *graphv1.UpdateGraphRequest) (*graphv1.UpdateGraphResponse, error) {
	def := &biz.GraphDefinition{
		ID:               req.Id,
		Name:             req.Name,
		Description:      req.Description,
		EntryPoint:       req.EntryPoint,
		FinishPoint:      req.FinishPoint,
		EnableCheckpoint: req.EnableCheckpoint,
		ExecutionEngine:  biz.ExecutionEngineType(req.ExecutionEngine),
		InterruptBefore:  req.InterruptBefore,
		InterruptAfter:   req.InterruptAfter,
	}
	if req.StateFields != nil {
		def.StateFields = make([]biz.StateFieldDef, len(req.StateFields))
		for i, sf := range req.StateFields {
			def.StateFields[i] = fromProtoStateField(sf)
		}
	}
	if req.Nodes != nil {
		def.Nodes = make([]biz.NodeDef, len(req.Nodes))
		for i, n := range req.Nodes {
			def.Nodes[i] = fromProtoNode(n)
		}
	}
	if req.Edges != nil {
		def.Edges = make([]biz.EdgeDef, len(req.Edges))
		for i, e := range req.Edges {
			def.Edges[i] = biz.EdgeDef{From: e.From, To: e.To}
		}
	}
	if req.ConditionalEdges != nil {
		def.ConditionalEdges = make([]biz.ConditionalEdgeDef, len(req.ConditionalEdges))
		for i, ce := range req.ConditionalEdges {
			def.ConditionalEdges[i] = fromProtoCondEdge(ce)
		}
	}
	if req.Subgraphs != nil {
		def.Subgraphs = make([]biz.SubgraphDef, len(req.Subgraphs))
		for i, sub := range req.Subgraphs {
			def.Subgraphs[i] = fromProtoSubgraph(sub)
		}
	}
	if req.Metadata != nil {
		def.Metadata = req.Metadata.AsMap()
	}
	saved, err := s.uc.UpdateGraph(ctx, def)
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(saved)
	if err != nil {
		return nil, err
	}
	return &graphv1.UpdateGraphResponse{Graph: pb}, nil
}

func (s *GraphService) DeleteGraph(ctx context.Context, req *graphv1.DeleteGraphRequest) (*graphv1.DeleteGraphResponse, error) {
	err := s.uc.DeleteGraph(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &graphv1.DeleteGraphResponse{Deleted: true}, nil
}

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
		Status:        exec.Status,
		InterruptNode: exec.InterruptNode,
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
	execs, nextToken, err := s.uc.ListExecutions(ctx, req.GraphId, int(req.PageSize), req.PageToken)
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

func (s *GraphService) VisualizeGraph(ctx context.Context, req *graphv1.VisualizeGraphRequest) (*graphv1.VisualizeGraphResponse, error) {
	result, err := s.uc.VisualizeGraph(ctx, req.GraphId, req.Format)
	if err != nil {
		return nil, err
	}
	vg, ok := result.(*graphtrpc.VisualGraph)
	if !ok {
		return nil, fmt.Errorf("visualize: unexpected result type")
	}
	format := req.Format
	if format == "" {
		format = "dot"
	}
	resp := &graphv1.VisualizeGraphResponse{
		Content: vg.DOT,
		Format:  format,
	}
	if vg.Nodes != nil {
		resp.Nodes = make([]*graphv1.VisualGraphNode, len(vg.Nodes))
		for i, n := range vg.Nodes {
			resp.Nodes[i] = &graphv1.VisualGraphNode{
				Id:          n.ID,
				Label:       n.Label,
				Type:        n.Type,
				Shape:       n.Shape,
				FillColor:   n.FillColor,
				BorderColor: n.BorderColor,
			}
		}
	}
	if vg.Edges != nil {
		resp.Edges = make([]*graphv1.VisualGraphEdge, len(vg.Edges))
		for i, e := range vg.Edges {
			resp.Edges[i] = &graphv1.VisualGraphEdge{
				From:  e.From,
				To:    e.To,
				Type:  e.Type,
				Label: e.Label,
			}
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
		return nil, fmt.Errorf("list checkpoints: unexpected result type")
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
		return nil, fmt.Errorf("edit state: unexpected result type")
	}
	return &graphv1.EditStateResponse{
		NewCheckpointId: ref.CheckpointID,
		LineageId:       ref.LineageID,
		Namespace:       ref.Namespace,
	}, nil
}

func fromProtoStateField(sf *graphv1.StateFieldDef) biz.StateFieldDef {
	def := biz.StateFieldDef{
		Name:            sf.Name,
		Type:            sf.Type,
		Reducer:         biz.ReducerType(sf.Reducer),
		Required:        sf.Required,
		DisableDeepCopy: sf.DisableDeepCopy,
	}
	if sf.DefaultValue != nil {
		def.DefaultValue = sf.DefaultValue.AsInterface()
	}
	return def
}

func fromProtoNode(n *graphv1.NodeDef) biz.NodeDef {
	return biz.NodeDef{
		ID:                       n.Id,
		FuncRef:                  n.FuncRef,
		Type:                     n.Type,
		Description:              n.Description,
		Instruction:              n.Instruction,
		ModelName:                n.ModelName,
		ToolNames:                n.ToolNames,
		AgentName:                n.AgentName,
		InterruptBefore:          n.InterruptBefore,
		InterruptAfter:           n.InterruptAfter,
		Destinations:             n.Destinations,
		RequiredRole:             n.RequiredRole,
		AssignmentMode:           n.AssignmentMode,
		AssignmentStrategy:       n.AssignmentStrategy,
		ReviewerAgent:            n.ReviewerAgent,
		ReviewRules:              n.ReviewRules,
		TimeoutSeconds:           int(n.TimeoutSeconds),
		HeartbeatIntervalSeconds: int(n.HeartbeatIntervalSeconds),
		EnableLeaseExtension:     n.EnableLeaseExtension,
	}
}

func fromProtoCondEdge(ce *graphv1.ConditionalEdgeDef) biz.ConditionalEdgeDef {
	return biz.ConditionalEdgeDef{
		From:        ce.From,
		CondFuncRef: ce.CondFuncRef,
		PathMap:     ce.PathMap,
	}
}

func fromProtoSubgraph(sub *graphv1.SubgraphDef) biz.SubgraphDef {
	return biz.SubgraphDef{
		ID:              sub.Id,
		InterruptBefore: sub.InterruptBefore,
		InterruptAfter:  sub.InterruptAfter,
	}
}

func toProtoGraph(def *biz.GraphDefinition) (*graphv1.GraphDefinition, error) {
	pb := &graphv1.GraphDefinition{
		Id:               def.ID,
		Name:             def.Name,
		Description:      def.Description,
		EntryPoint:       def.EntryPoint,
		FinishPoint:      def.FinishPoint,
		EnableCheckpoint: def.EnableCheckpoint,
		ExecutionEngine:  string(def.ExecutionEngine),
		InterruptBefore:  def.InterruptBefore,
		InterruptAfter:   def.InterruptAfter,
		CreatedAt:        timestamppb.New(def.CreatedAt),
		UpdatedAt:        timestamppb.New(def.UpdatedAt),
	}
	if def.StateFields != nil {
		pb.StateFields = make([]*graphv1.StateFieldDef, len(def.StateFields))
		for i, sf := range def.StateFields {
			pb.StateFields[i] = toProtoStateField(sf)
		}
	}
	if def.Nodes != nil {
		pb.Nodes = make([]*graphv1.NodeDef, len(def.Nodes))
		for i, n := range def.Nodes {
			pb.Nodes[i] = &graphv1.NodeDef{
				Id:                       n.ID,
				FuncRef:                  n.FuncRef,
				Type:                     n.Type,
				Description:              n.Description,
				Instruction:              n.Instruction,
				ModelName:                n.ModelName,
				ToolNames:                n.ToolNames,
				AgentName:                n.AgentName,
				InterruptBefore:          n.InterruptBefore,
				InterruptAfter:           n.InterruptAfter,
				Destinations:             n.Destinations,
				RequiredRole:             n.RequiredRole,
				AssignmentMode:           n.AssignmentMode,
				AssignmentStrategy:       n.AssignmentStrategy,
				ReviewerAgent:            n.ReviewerAgent,
				ReviewRules:              n.ReviewRules,
				TimeoutSeconds:           int32(n.TimeoutSeconds),
				HeartbeatIntervalSeconds: int32(n.HeartbeatIntervalSeconds),
				EnableLeaseExtension:     n.EnableLeaseExtension,
			}
		}
	}
	if def.Edges != nil {
		pb.Edges = make([]*graphv1.EdgeDef, len(def.Edges))
		for i, e := range def.Edges {
			pb.Edges[i] = &graphv1.EdgeDef{From: e.From, To: e.To}
		}
	}
	if def.ConditionalEdges != nil {
		pb.ConditionalEdges = make([]*graphv1.ConditionalEdgeDef, len(def.ConditionalEdges))
		for i, ce := range def.ConditionalEdges {
			pb.ConditionalEdges[i] = &graphv1.ConditionalEdgeDef{
				From:        ce.From,
				CondFuncRef: ce.CondFuncRef,
				PathMap:     ce.PathMap,
			}
		}
	}
	if def.Subgraphs != nil {
		pb.Subgraphs = make([]*graphv1.SubgraphDef, len(def.Subgraphs))
		for i, sub := range def.Subgraphs {
			pb.Subgraphs[i] = &graphv1.SubgraphDef{
				Id:              sub.ID,
				InterruptBefore: sub.InterruptBefore,
				InterruptAfter:  sub.InterruptAfter,
			}
		}
	}
	if def.Metadata != nil {
		st, err := structpb.NewStruct(def.Metadata)
		if err != nil {
			return nil, err
		}
		pb.Metadata = st
	}
	return pb, nil
}

func toProtoStateField(sf biz.StateFieldDef) *graphv1.StateFieldDef {
	pb := &graphv1.StateFieldDef{
		Name:            sf.Name,
		Type:            sf.Type,
		Reducer:         string(sf.Reducer),
		Required:        sf.Required,
		DisableDeepCopy: sf.DisableDeepCopy,
	}
	if sf.DefaultValue != nil {
		v, err := structpb.NewValue(sf.DefaultValue)
		if err == nil {
			pb.DefaultValue = v
		}
	}
	return pb
}

func toProtoStep(step biz.GraphStepSnapshot) *graphv1.GraphStepSnapshot {
	pb := &graphv1.GraphStepSnapshot{
		NodeId:    step.NodeID,
		StepIndex: int32(step.StepIndex),
	}
	if step.InputState != nil {
		st, err := structpb.NewStruct(step.InputState)
		if err == nil {
			pb.InputState = st
		}
	}
	if step.OutputState != nil {
		st, err := structpb.NewStruct(step.OutputState)
		if err == nil {
			pb.OutputState = st
		}
	}
	return pb
}

func (s *GraphService) ValidateGraph(ctx context.Context, req *graphv1.ValidateGraphRequest) (*graphv1.ValidateGraphResponse, error) {
	result, err := s.uc.ValidateGraph(ctx, req.GraphId)
	if err != nil {
		return nil, err
	}
	vr, ok := result.(*graphtrpc.ValidationResult)
	if !ok {
		return nil, fmt.Errorf("validate: unexpected result type")
	}
	resp := &graphv1.ValidateGraphResponse{
		Valid: !vr.HasErrors(),
	}
	for _, e := range vr.Errors {
		resp.Errors = append(resp.Errors, &graphv1.ValidationError{
			Code:    string(e.Code),
			NodeId:  e.NodeID,
			Field:   e.Field,
			Message: e.Message,
		})
	}
	for _, w := range vr.Warnings {
		resp.Warnings = append(resp.Warnings, &graphv1.ValidationWarning{
			Code:    string(w.Code),
			NodeId:  w.NodeID,
			Field:   w.Field,
			Message: w.Message,
		})
	}
	return resp, nil
}

func (s *GraphService) ListGraphTemplates(ctx context.Context, req *graphv1.ListGraphTemplatesRequest) (*graphv1.ListGraphTemplatesResponse, error) {
	templates := s.uc.ListGraphTemplates(ctx)
	tmplList, ok := templates.([]graphtrpc.GraphTemplate)
	if !ok {
		return nil, fmt.Errorf("list templates: unexpected result type")
	}
	resp := &graphv1.ListGraphTemplatesResponse{
		Templates: make([]*graphv1.GraphTemplateInfo, len(tmplList)),
	}
	for i, t := range tmplList {
		resp.Templates[i] = templateToProto(t)
	}
	return resp, nil
}

func (s *GraphService) CreateGraphFromTemplate(ctx context.Context, req *graphv1.CreateGraphFromTemplateRequest) (*graphv1.CreateGraphResponse, error) {
	saved, err := s.uc.CreateGraphFromTemplate(ctx, req.TemplateId, req.Name, req.Description)
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(saved)
	if err != nil {
		return nil, err
	}
	return &graphv1.CreateGraphResponse{Graph: pb}, nil
}

func templateToProto(t graphtrpc.GraphTemplate) *graphv1.GraphTemplateInfo {
	info := &graphv1.GraphTemplateInfo{
		Id:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Category:    t.Category,
		EntryPoint:  t.EntryPoint,
		FinishPoint: t.FinishPoint,
	}
	info.Nodes = make([]*graphv1.TemplateNodeInfo, len(t.Nodes))
	for i, n := range t.Nodes {
		info.Nodes[i] = &graphv1.TemplateNodeInfo{
			NodeId:      n.NodeID,
			Type:        n.Type,
			Label:       n.Label,
			Description: n.Description,
		}
	}
	info.Edges = make([]*graphv1.TemplateEdgeInfo, len(t.Edges))
	for i, e := range t.Edges {
		info.Edges[i] = &graphv1.TemplateEdgeInfo{
			FromNode: e.FromNode,
			ToNode:   e.ToNode,
			Type:     e.Type,
			Label:    e.Label,
		}
	}
	info.StateFields = make([]*graphv1.StateFieldDef, len(t.StateFields))
	for i, sf := range t.StateFields {
		info.StateFields[i] = toProtoStateField(biz.StateFieldDef{
			Name:            sf.Name,
			Type:            sf.Type,
			Reducer:         biz.ReducerType(sf.Reducer),
			DefaultValue:    sf.DefaultValue,
			Required:        sf.Required,
			DisableDeepCopy: sf.DisableDeepCopy,
		})
	}
	return info
}

func (s *GraphService) ListTasks(ctx context.Context, req *graphv1.ListTasksRequest) (*graphv1.ListTasksResponse, error) {
	tasks, _, err := s.taskUC.ListTasks(ctx, req.ExecutionId, biz.TaskStatus(req.StatusFilter), int(req.PageSize), req.PageToken)
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.Task, len(tasks))
	for i, t := range tasks {
		items[i] = toProtoTask(t)
	}
	return &graphv1.ListTasksResponse{Items: items}, nil
}

func (s *GraphService) GetTask(ctx context.Context, req *graphv1.GetTaskRequest) (*graphv1.GetTaskResponse, error) {
	task, err := s.taskUC.GetTask(ctx, req.TaskId)
	if err != nil {
		return nil, err
	}
	return &graphv1.GetTaskResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) ClaimTask(ctx context.Context, req *graphv1.ClaimTaskRequest) (*graphv1.ClaimTaskResponse, error) {
	task, err := s.taskUC.ClaimTask(ctx, req.TaskId, req.AgentKey)
	if err != nil {
		return nil, err
	}
	s.publishTaskOrchestrationStatus(ctx, task, nil)
	return &graphv1.ClaimTaskResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) SubmitTaskResult(ctx context.Context, req *graphv1.SubmitTaskResultRequest) (*graphv1.SubmitTaskResultResponse, error) {
	task, err := s.taskUC.SubmitTaskResult(ctx, req.TaskId, req.Output, req.Summary, req.Metadata)
	if err != nil {
		return nil, err
	}
	s.publishTaskOrchestrationStatus(ctx, task, nil)
	return &graphv1.SubmitTaskResultResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) Heartbeat(ctx context.Context, req *graphv1.HeartbeatRequest) (*graphv1.HeartbeatResponse, error) {
	ack, ext, err := s.taskUC.Heartbeat(ctx, req.TaskId, req.AgentKey, req.Metadata)
	if err != nil {
		return nil, err
	}
	return &graphv1.HeartbeatResponse{Acknowledged: ack, LeaseExtensionSeconds: ext}, nil
}

func (s *GraphService) ReportBlocked(ctx context.Context, req *graphv1.ReportBlockedRequest) (*graphv1.ReportBlockedResponse, error) {
	task, err := s.taskUC.ReportBlocked(ctx, req.TaskId, req.Reason, req.Metadata)
	if err != nil {
		return nil, err
	}
	s.publishTaskOrchestrationStatus(ctx, task, nil)
	return &graphv1.ReportBlockedResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) ReviewTask(ctx context.Context, req *graphv1.ReviewTaskRequest) (*graphv1.ReviewTaskResponse, error) {
	task, err := s.taskUC.ReviewTask(ctx, req.TaskId, req.ReviewerAgent, req.Approved, req.Comment)
	if err != nil {
		return nil, err
	}
	extra := map[string]any{}
	if !req.Approved {
		extra["review_rejected"] = true
		if c := strings.TrimSpace(req.Comment); c != "" {
			extra["review_comment"] = c
		}
	}
	s.publishTaskOrchestrationStatus(ctx, task, extra)
	return &graphv1.ReviewTaskResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) ListTaskComments(ctx context.Context, req *graphv1.ListTaskCommentsRequest) (*graphv1.ListTaskCommentsResponse, error) {
	comments, err := s.taskUC.ListTaskComments(ctx, req.TaskId)
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.TaskComment, len(comments))
	for i, c := range comments {
		items[i] = &graphv1.TaskComment{
			CommentId: c.CommentID,
			TaskId:    c.TaskID,
			Author:    c.Author,
			Content:   c.Content,
			CreatedAt: timestamppb.New(c.CreatedAt),
		}
	}
	return &graphv1.ListTaskCommentsResponse{Comments: items}, nil
}

func (s *GraphService) AddTaskComment(ctx context.Context, req *graphv1.AddTaskCommentRequest) (*graphv1.AddTaskCommentResponse, error) {
	comment, err := s.taskUC.AddTaskComment(ctx, req.TaskId, req.Author, req.Content, req.Type)
	if err != nil {
		return nil, err
	}
	return &graphv1.AddTaskCommentResponse{Comment: &graphv1.TaskComment{
		CommentId: comment.CommentID,
		TaskId:    comment.TaskID,
		Author:    comment.Author,
		Content:   comment.Content,
		CreatedAt: timestamppb.New(comment.CreatedAt),
	}}, nil
}

func (s *GraphService) ListTaskLogs(ctx context.Context, req *graphv1.ListTaskLogsRequest) (*graphv1.ListTaskLogsResponse, error) {
	logs, err := s.taskUC.ListTaskLogs(ctx, req.TaskId, req.Stream, req.Level, int(req.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.TaskLog, len(logs))
	for i, l := range logs {
		items[i] = &graphv1.TaskLog{
			LogId:     l.LogID,
			TaskId:    l.TaskID,
			Stream:    l.Stream,
			Content:   l.Content,
			Level:     l.Level,
			Timestamp: timestamppb.New(l.Timestamp),
		}
	}
	return &graphv1.ListTaskLogsResponse{Logs: items}, nil
}

func (s *GraphService) ListTaskRuns(ctx context.Context, req *graphv1.ListTaskRunsRequest) (*graphv1.ListTaskRunsResponse, error) {
	runs, err := s.taskUC.ListTaskRuns(ctx, req.TaskId)
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.TaskRun, len(runs))
	for i, r := range runs {
		items[i] = &graphv1.TaskRun{
			RunId:     r.RunID,
			TaskId:    r.TaskID,
			StartedAt: timestamppb.New(r.StartedAt),
			ExitCode:  int32(r.ExitCode),
			LogRef:    r.LogRef,
		}
		if r.FinishedAt != nil {
			items[i].FinishedAt = timestamppb.New(*r.FinishedAt)
		}
	}
	return &graphv1.ListTaskRunsResponse{Runs: items}, nil
}

func (s *GraphService) ListTaskEvents(ctx context.Context, req *graphv1.ListTaskEventsRequest) (*graphv1.ListTaskEventsResponse, error) {
	events, err := s.taskUC.ListTaskEvents(ctx, req.ExecutionId, req.TaskId, req.EventType, int(req.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.TaskEvent, len(events))
	for i, e := range events {
		items[i] = &graphv1.TaskEvent{
			EventId:     e.EventID,
			TaskId:      e.TaskID,
			EventType:   e.EventType,
			SourceNode:  e.SourceNode,
			Description: e.Description,
			Timestamp:   timestamppb.New(e.Timestamp),
		}
	}
	return &graphv1.ListTaskEventsResponse{Events: items}, nil
}

func toProtoTask(task *biz.GraphTask) *graphv1.Task {
	pb := &graphv1.Task{
		TaskId:         task.TaskID,
		NodeId:         task.NodeID,
		ExecutionId:    task.ExecutionID,
		Assignee:       task.Assignee,
		Status:         bizTaskStatusToProto(task.Status),
		Context:        task.Context,
		Input:          task.Input,
		Output:         task.Output,
		Summary:        task.Summary,
		Metadata:       task.Metadata,
		RequiredRole:   task.RequiredRole,
		AssignmentMode: task.AssignmentMode,
		CreatedAt:      timestamppb.New(task.CreatedAt),
	}
	if task.ClaimedAt != nil {
		pb.ClaimedAt = timestamppb.New(*task.ClaimedAt)
	}
	if task.CompletedAt != nil {
		pb.CompletedAt = timestamppb.New(*task.CompletedAt)
	}
	return pb
}

func bizTaskStatusToProto(status biz.TaskStatus) graphv1.TaskStatus {
	switch status {
	case biz.TaskStatusPending:
		return graphv1.TaskStatus_TASK_PENDING
	case biz.TaskStatusClaimed:
		return graphv1.TaskStatus_TASK_CLAIMED
	case biz.TaskStatusComplete:
		return graphv1.TaskStatus_TASK_COMPLETE
	case biz.TaskStatusBlocked:
		return graphv1.TaskStatus_TASK_BLOCKED
	case biz.TaskStatusReviewRequired:
		return graphv1.TaskStatus_TASK_REVIEW_REQUIRED
	case biz.TaskStatusFailed:
		return graphv1.TaskStatus_TASK_FAILED
	case biz.TaskStatusTimedOut:
		return graphv1.TaskStatus_TASK_TIMED_OUT
	case biz.TaskStatusCancelled:
		return graphv1.TaskStatus_TASK_CANCELLED
	case biz.TaskStatusCrashed:
		return graphv1.TaskStatus_TASK_CRASHED
	case biz.TaskStatusPendingAssignment:
		return graphv1.TaskStatus_TASK_PENDING_ASSIGNMENT
	default:
		return graphv1.TaskStatus_TASK_PENDING
	}
}
