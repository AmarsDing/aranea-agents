package service

import (
	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
		RetryMaxAttempts:         int(n.RetryMaxAttempts),
		FailureAction:            n.FailureAction,
		FallbackAgent:            n.FallbackAgent,
		InputMapperJSON:          n.InputMapperJson,
		OutputMapperJSON:         n.OutputMapperJson,
		IsolatedMessages:         n.IsolatedMessages,
		InputFromLastResponse:    n.InputFromLastResponse,
		CacheEnabled:             n.CacheEnabled,
		CacheTTLSeconds:          int(n.CacheTtlSeconds),
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

func toProtoGraph(def *biz.GraphDefinition, lg loggateway.Logger) (*graphv1.GraphDefinition, error) {
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
		Version:          int32(biz.GraphVersion(def)),
		SortOrder:        int32(def.SortOrder),
		CreatedAt:        timestamppb.New(def.CreatedAt),
		UpdatedAt:        timestamppb.New(def.UpdatedAt),
	}
	if def.StateFields != nil {
		pb.StateFields = make([]*graphv1.StateFieldDef, len(def.StateFields))
		for i, sf := range def.StateFields {
			pb.StateFields[i] = toProtoStateField(sf, lg)
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
				RetryMaxAttempts:         int32(n.RetryMaxAttempts),
				FailureAction:            n.FailureAction,
				FallbackAgent:            n.FallbackAgent,
				InputMapperJson:          n.InputMapperJSON,
				OutputMapperJson:         n.OutputMapperJSON,
				IsolatedMessages:         n.IsolatedMessages,
				InputFromLastResponse:    n.InputFromLastResponse,
				CacheEnabled:             n.CacheEnabled,
				CacheTtlSeconds:          int32(n.CacheTTLSeconds),
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

func toProtoStateField(sf biz.StateFieldDef, lg loggateway.Logger) *graphv1.StateFieldDef {
	pb := &graphv1.StateFieldDef{
		Name:            sf.Name,
		Type:            sf.Type,
		Reducer:         string(sf.Reducer),
		Required:        sf.Required,
		DisableDeepCopy: sf.DisableDeepCopy,
	}
	if sf.DefaultValue != nil {
		v, err := structpb.NewValue(sf.DefaultValue)
		if err != nil {
			lg.Warn("structpb.NewValue failed for state field default", loggateway.StepID("graph.mapping"), loggateway.Str("field", sf.Name), loggateway.Err(err))
		} else {
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

func userTemplateToProto(def *biz.GraphDefinition, meta *biz.UserTemplateMeta, lg loggateway.Logger) *graphv1.GraphTemplateInfo {
	info := &graphv1.GraphTemplateInfo{
		Id:          meta.TemplateID,
		Name:        meta.Name,
		Description: meta.Description,
		Category:    meta.Category,
		EntryPoint:  def.EntryPoint,
		FinishPoint: def.FinishPoint,
	}
	for _, n := range def.Nodes {
		info.Nodes = append(info.Nodes, &graphv1.TemplateNodeInfo{
			NodeId:      n.ID,
			Type:        n.Type,
			Label:       n.ID,
			Description: n.Description,
		})
	}
	for _, e := range def.Edges {
		info.Edges = append(info.Edges, &graphv1.TemplateEdgeInfo{
			FromNode: e.From,
			ToNode:   e.To,
			Type:     "edge",
		})
	}
	for _, sf := range def.StateFields {
		info.StateFields = append(info.StateFields, toProtoStateField(sf, lg))
	}
	return info
}

func templateToProto(t graphtrpc.GraphTemplate, lg loggateway.Logger) *graphv1.GraphTemplateInfo {
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
		}, lg)
	}
	return info
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
