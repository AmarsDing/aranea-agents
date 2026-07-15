package service

import (
	"context"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// assertGraphAccess 验证 caller workspace 是否可访问目标 graph（P2-B IDOR 防护）。
// 使用 AssertWorkspaceOrShared：共享 graph（workspace_id=""）对所有租户可见，
// 租户私有 graph（workspace_id="ws-xxx"）仅归属租户可见。
// 失败时返回 NotFound（避免泄露资源存在性），并记录 Warn 日志。
func (s *GraphService) assertGraphAccess(ctx context.Context, graphID string) error {
	if graphID == "" {
		return nil
	}
	def, err := s.uc.GetGraph(ctx, graphID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound("GRAPH", "graph definition not found")
		}
		return err
	}
	if err := workspace.AssertWorkspaceOrShared(workspace.IDFromContext(ctx), def.WorkspaceID); err != nil {
		s.lg.Warn("graph access denied: workspace mismatch",
			loggateway.StepID("graph.idor"),
			loggateway.Str("graph_id", graphID),
			loggateway.Str("caller_ws", workspace.IDFromContext(ctx)))
		return apierror.NotFound("GRAPH", "graph definition not found")
	}
	return nil
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
			def.Edges[i] = fromProtoEdge(e)
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
	def.VerificationGates = req.VerificationGates
	def.TeamID = req.TeamId
	def.IsTemplate = req.IsTemplate
	// P2-B: 租户 caller 创建的 graph 归属其 workspace；系统 caller 留空（共享）。
	if !workspace.IsSystem(ctx) {
		def.WorkspaceID = workspace.IDFromContext(ctx)
	}
	saved, err := s.uc.CreateGraph(ctx, def)
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(saved, s.lg)
	if err != nil {
		return nil, err
	}
	return &graphv1.CreateGraphResponse{Graph: pb}, nil
}

func (s *GraphService) GetGraph(ctx context.Context, req *graphv1.GetGraphRequest) (*graphv1.GetGraphResponse, error) {
	if err := s.assertGraphAccess(ctx, req.Id); err != nil { // P2-B: IDOR
		return nil, err
	}
	def, err := s.uc.GetGraph(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(def, s.lg)
	if err != nil {
		return nil, err
	}
	return &graphv1.GetGraphResponse{Graph: pb}, nil
}

func (s *GraphService) ListGraphs(ctx context.Context, req *graphv1.ListGraphsRequest) (*graphv1.ListGraphsResponse, error) {
	// P2-B: 系统 caller（workspace_id="")看全部；租户 caller 看 shared + own。
	callerWS := workspace.IDFromContext(ctx)
	if workspace.IsSystem(ctx) {
		callerWS = ""
	}
	defs, nextToken, err := s.uc.ListGraphsByWorkspace(ctx, int(req.PageSize), req.PageToken, callerWS)
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.GraphDefinition, len(defs))
	for i, def := range defs {
		pb, err := toProtoGraph(def, s.lg)
		if err != nil {
			return nil, err
		}
		items[i] = pb
	}
	return &graphv1.ListGraphsResponse{Items: items, NextPageToken: nextToken}, nil
}

func (s *GraphService) UpdateGraph(ctx context.Context, req *graphv1.UpdateGraphRequest) (*graphv1.UpdateGraphResponse, error) {
	if err := s.assertGraphAccess(ctx, req.Id); err != nil { // P2-B: IDOR
		return nil, err
	}
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
			def.Edges[i] = fromProtoEdge(e)
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
	def.VerificationGates = req.VerificationGates
	def.TeamID = req.TeamId
	def.IsTemplate = req.IsTemplate
	saved, err := s.uc.UpdateGraph(ctx, def)
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(saved, s.lg)
	if err != nil {
		return nil, err
	}
	return &graphv1.UpdateGraphResponse{Graph: pb}, nil
}

func (s *GraphService) DeleteGraph(ctx context.Context, req *graphv1.DeleteGraphRequest) (*graphv1.DeleteGraphResponse, error) {
	if err := s.assertGraphAccess(ctx, req.Id); err != nil { // P2-B: IDOR
		return nil, err
	}
	err := s.uc.DeleteGraph(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &graphv1.DeleteGraphResponse{Deleted: true}, nil
}

func (s *GraphService) ReorderGraphs(ctx context.Context, req *graphv1.ReorderGraphsRequest) (*graphv1.ReorderGraphsResponse, error) {
	if len(req.Ids) == 0 {
		return &graphv1.ReorderGraphsResponse{}, nil
	}
	// P2-B: IDOR — 校验 caller 对所有待排序 graph 的访问权限。
	for _, id := range req.Ids {
		if err := s.assertGraphAccess(ctx, id); err != nil {
			return nil, err
		}
	}
	err := s.uc.ReorderGraphs(ctx, req.Ids)
	if err != nil {
		return nil, err
	}
	return &graphv1.ReorderGraphsResponse{}, nil
}

func (s *GraphService) ValidateGraph(ctx context.Context, req *graphv1.ValidateGraphRequest) (*graphv1.ValidateGraphResponse, error) {
	if err := s.assertGraphAccess(ctx, req.GraphId); err != nil { // P2-B: IDOR
		return nil, err
	}
	result, err := s.uc.ValidateGraph(ctx, req.GraphId)
	if err != nil {
		return nil, err
	}
	resp := &graphv1.ValidateGraphResponse{
		Valid: !result.HasErrors(),
	}
	for _, e := range result.Errors {
		resp.Errors = append(resp.Errors, &graphv1.ValidationError{
			Code:    e.Code,
			NodeId:  e.NodeID,
			Field:   e.Field,
			Message: e.Message,
		})
	}
	for _, w := range result.Warnings {
		resp.Warnings = append(resp.Warnings, &graphv1.ValidationWarning{
			Code:    w.Code,
			NodeId:  w.NodeID,
			Field:   w.Field,
			Message: w.Message,
		})
	}
	return resp, nil
}

func (s *GraphService) ListGraphTemplates(ctx context.Context, req *graphv1.ListGraphTemplatesRequest) (*graphv1.ListGraphTemplatesResponse, error) {
	tmplList := s.uc.ListGraphTemplates(ctx)
	resp := &graphv1.ListGraphTemplatesResponse{
		Templates: make([]*graphv1.GraphTemplateInfo, 0, len(tmplList)),
	}
	for _, t := range tmplList {
		resp.Templates = append(resp.Templates, bizTemplateToProto(t, s.lg))
	}
	// P2-B: 系统 caller（workspace_id="")看全部用户模板；租户 caller 看 shared + own。
	callerWS := workspace.IDFromContext(ctx)
	if workspace.IsSystem(ctx) {
		callerWS = ""
	}
	userDefs, err := s.uc.ListUserTemplateGraphsByWorkspace(ctx, callerWS)
	if err != nil {
		return nil, err
	}
	for _, def := range userDefs {
		meta := biz.ReadUserTemplateMeta(def)
		if meta == nil {
			continue
		}
		resp.Templates = append(resp.Templates, userTemplateToProto(def, meta, s.lg))
	}
	return resp, nil
}

func (s *GraphService) CreateGraphFromTemplate(ctx context.Context, req *graphv1.CreateGraphFromTemplateRequest) (*graphv1.CreateGraphResponse, error) {
	// P2-B: 从模板创建的 graph 归属 caller workspace（系统 caller 留空=共享）。
	wsID := ""
	if !workspace.IsSystem(ctx) {
		wsID = workspace.IDFromContext(ctx)
	}
	// 若从用户模板创建，需校验对源 graph 的访问权限。
	if id, ok := biz.ParseUserTemplateID(req.TemplateId); ok {
		if err := s.assertGraphAccess(ctx, id); err != nil { // P2-B: IDOR
			return nil, err
		}
	}
	saved, err := s.uc.CreateGraphFromTemplate(ctx, req.TemplateId, req.Name, req.Description, wsID)
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(saved, s.lg)
	if err != nil {
		return nil, err
	}
	return &graphv1.CreateGraphResponse{Graph: pb}, nil
}

func (s *GraphService) ExportGraph(ctx context.Context, req *graphv1.ExportGraphRequest) (*graphv1.ExportGraphResponse, error) {
	if err := s.assertGraphAccess(ctx, req.GraphId); err != nil { // P2-B: IDOR
		return nil, err
	}
	raw, def, err := s.uc.ExportGraph(ctx, req.GraphId)
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(def, s.lg)
	if err != nil {
		return nil, err
	}
	return &graphv1.ExportGraphResponse{Json: string(raw), Graph: pb}, nil
}

func (s *GraphService) ImportGraph(ctx context.Context, req *graphv1.ImportGraphRequest) (*graphv1.ImportGraphResponse, error) {
	// P2-B: 导入的 graph 归属 caller workspace（系统 caller 留空=共享）。
	wsID := ""
	if !workspace.IsSystem(ctx) {
		wsID = workspace.IDFromContext(ctx)
	}
	saved, err := s.uc.ImportGraph(ctx, []byte(req.Json), req.Name, req.Description, wsID)
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(saved, s.lg)
	if err != nil {
		return nil, err
	}
	return &graphv1.ImportGraphResponse{Graph: pb}, nil
}

func (s *GraphService) ListGraphVersions(ctx context.Context, req *graphv1.ListGraphVersionsRequest) (*graphv1.ListGraphVersionsResponse, error) {
	if err := s.assertGraphAccess(ctx, req.GraphId); err != nil { // P2-B: IDOR
		return nil, err
	}
	items, err := s.uc.ListGraphVersions(ctx, req.GraphId)
	if err != nil {
		return nil, err
	}
	resp := &graphv1.ListGraphVersionsResponse{
		Items: make([]*graphv1.GraphVersionInfo, len(items)),
	}
	for i, item := range items {
		resp.Items[i] = &graphv1.GraphVersionInfo{
			Version: int32(item.Version),
			Name:    item.Name,
			SavedAt: timestamppb.New(item.SavedAt),
		}
	}
	return resp, nil
}

func (s *GraphService) RollbackGraphVersion(ctx context.Context, req *graphv1.RollbackGraphVersionRequest) (*graphv1.RollbackGraphVersionResponse, error) {
	if err := s.assertGraphAccess(ctx, req.GraphId); err != nil { // P2-B: IDOR
		return nil, err
	}
	saved, err := s.uc.RollbackGraphVersion(ctx, req.GraphId, int(req.Version))
	if err != nil {
		return nil, err
	}
	pb, err := toProtoGraph(saved, s.lg)
	if err != nil {
		return nil, err
	}
	return &graphv1.RollbackGraphVersionResponse{Graph: pb}, nil
}

func (s *GraphService) SaveGraphAsTemplate(ctx context.Context, req *graphv1.SaveGraphAsTemplateRequest) (*graphv1.SaveGraphAsTemplateResponse, error) {
	if err := s.assertGraphAccess(ctx, req.GraphId); err != nil { // P2-B: IDOR
		return nil, err
	}
	meta, err := s.uc.SaveGraphAsTemplate(ctx, req.GraphId, req.TemplateName, req.Category, req.Description)
	if err != nil {
		return nil, err
	}
	def, err := s.uc.GetGraph(ctx, req.GraphId)
	if err != nil {
		return nil, err
	}
	return &graphv1.SaveGraphAsTemplateResponse{
		TemplateId: meta.TemplateID,
		Template:   userTemplateToProto(def, meta, s.lg),
	}, nil
}

func (s *GraphService) VisualizeGraph(ctx context.Context, req *graphv1.VisualizeGraphRequest) (*graphv1.VisualizeGraphResponse, error) {
	if err := s.assertGraphAccess(ctx, req.GraphId); err != nil { // P2-B: IDOR
		return nil, err
	}
	vg, err := s.uc.VisualizeGraph(ctx, req.GraphId, req.Format)
	if err != nil {
		return nil, err
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
