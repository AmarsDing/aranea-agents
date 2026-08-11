package data

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/graphdefinition"
	"aranea-agents/internal/data/ent/graphexecution"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// keysetCursor 是 keyset（seek）分页游标：graph 域 ID 为随机 UUID，与排序
// 字段无序关系，游标必须由排序键组成才能保证翻页连续、不重不漏。
// Ts 用 Unix 微秒保留 timestamptz 精度。
type keysetCursor struct {
	SortOrder int    `json:"s,omitempty"`
	Ts        int64  `json:"c,omitempty"`
	ID        string `json:"i,omitempty"`
}

func encodeKeysetCursor(c keysetCursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeKeysetCursor(token string, domain string) (keysetCursor, error) {
	var c keysetCursor
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return c, apierror.BadRequest(domain, "invalid page token")
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, apierror.BadRequest(domain, "invalid page token")
	}
	return c, nil
}

type graphRepo struct {
	data *Data
}

var (
	_ biz.GraphRepo    = (*graphRepo)(nil)
	_ biz.GraphReader  = (*graphRepo)(nil)
	_ biz.GraphWriter  = (*graphRepo)(nil)
	_ biz.GraphRunRepo = (*graphRunRepo)(nil)
)

func NewGraphRepo(data *Data) biz.GraphRepo {
	return &graphRepo{data: data}
}

func (r *graphRepo) SaveDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	client := r.data.RW().Write(ctx)
	stateFieldsJSON, _ := json.Marshal(def.StateFields)
	nodesJSON, _ := json.Marshal(def.Nodes)
	edgesJSON, _ := json.Marshal(def.Edges)
	condEdgesJSON, _ := json.Marshal(def.ConditionalEdges)
	subgraphsJSON, _ := json.Marshal(def.Subgraphs)
	interruptBeforeJSON, _ := json.Marshal(def.InterruptBefore)
	interruptAfterJSON, _ := json.Marshal(def.InterruptAfter)
	metadataJSON, _ := json.Marshal(def.Metadata)

	builder := client.GraphDefinition.Create().
		SetName(def.Name).
		SetDescription(def.Description).
		SetStateFields(string(stateFieldsJSON)).
		SetNodes(string(nodesJSON)).
		SetEdges(string(edgesJSON)).
		SetConditionalEdges(string(condEdgesJSON)).
		SetSubgraphs(string(subgraphsJSON)).
		SetEntryPoint(def.EntryPoint).
		SetFinishPoint(def.FinishPoint).
		SetEnableCheckpoint(def.EnableCheckpoint).
		SetExecutionEngine(string(def.ExecutionEngine)).
		SetInterruptBefore(string(interruptBeforeJSON)).
		SetInterruptAfter(string(interruptAfterJSON)).
		SetMetadata(string(metadataJSON)).
		SetVerificationGates(def.VerificationGates).
		SetSortOrder(def.SortOrder).
		SetTeamID(def.TeamID).
		SetIsTemplate(def.IsTemplate).
		SetWorkspaceID(def.WorkspaceID) // P2-B: tenant isolation

	if def.ID != "" {
		builder.SetID(def.ID)
	}
	createdAt := def.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := def.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	saved, err := builder.
		SetCreatedAt(createdAt).
		SetUpdatedAt(updatedAt).
		Save(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "GRAPH")
	}
	return entGraphToBiz(saved, r.data.lg), nil
}

func (r *graphRepo) GetDefinition(ctx context.Context, id string) (*biz.GraphDefinition, error) {
	client := r.data.RW().Read(ctx)
	row, err := client.GraphDefinition.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, apierror.NotFound("GRAPH", "graph definition not found")
		}
		return nil, entErrToBizErr(err, "GRAPH")
	}
	return entGraphToBiz(row, r.data.lg), nil
}

func (r *graphRepo) GetDefinitionByName(ctx context.Context, name string) (*biz.GraphDefinition, error) {
	client := r.data.RW().Read(ctx)
	row, err := client.GraphDefinition.Query().
		Where(graphdefinition.NameEQ(name)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, apierror.NotFound("GRAPH", "graph definition not found")
		}
		return nil, entErrToBizErr(err, "GRAPH")
	}
	return entGraphToBiz(row, r.data.lg), nil
}

func (r *graphRepo) ListDefinitions(ctx context.Context, pageSize int, pageToken string) ([]*biz.GraphDefinition, string, error) {
	client := r.data.RW().Read(ctx)
	query := client.GraphDefinition.Query().Order(
		ent.Asc(graphdefinition.FieldSortOrder),
		ent.Asc(graphdefinition.FieldCreatedAt),
		ent.Asc(graphdefinition.FieldID),
	)
	if pageSize <= 0 {
		pageSize = 20
	}
	query = query.Limit(pageSize + 1)
	if pageToken != "" {
		cur, err := decodeKeysetCursor(pageToken, "GRAPH")
		if err != nil {
			return nil, "", err
		}
		query = query.Where(graphDefinitionKeyset(cur))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, "", entErrToBizErr(err, "GRAPH")
	}
	var nextToken string
	if len(rows) > pageSize {
		last := rows[pageSize-1]
		nextToken = encodeKeysetCursor(keysetCursor{SortOrder: last.SortOrder, Ts: last.CreatedAt.UnixMicro(), ID: last.ID})
		rows = rows[:pageSize]
	}
	result := make([]*biz.GraphDefinition, len(rows))
	for i, row := range rows {
		result[i] = entGraphToBiz(row, r.data.lg)
	}
	return result, nextToken, nil
}

// graphDefinitionKeyset 生成 (sort_order, created_at, id) 的 keyset 续页谓词，
// 与 ListDefinitions* 的 ORDER BY 严格一致（ASC）。
func graphDefinitionKeyset(cur keysetCursor) predicate.GraphDefinition {
	ts := time.UnixMicro(cur.Ts).UTC()
	return graphdefinition.Or(
		graphdefinition.SortOrderGT(cur.SortOrder),
		graphdefinition.And(
			graphdefinition.SortOrderEQ(cur.SortOrder),
			graphdefinition.CreatedAtGT(ts),
		),
		graphdefinition.And(
			graphdefinition.SortOrderEQ(cur.SortOrder),
			graphdefinition.CreatedAtEQ(ts),
			graphdefinition.IDGT(cur.ID),
		),
	)
}

func (r *graphRepo) ListUserTemplateDefinitions(ctx context.Context, pageSize int) ([]*biz.GraphDefinition, error) {
	defs, _, err := r.ListDefinitions(ctx, pageSize, "")
	if err != nil {
		return nil, err
	}
	out := make([]*biz.GraphDefinition, 0)
	for _, def := range defs {
		if biz.ReadUserTemplateMeta(def) != nil {
			out = append(out, def)
		}
	}
	return out, nil
}

// ListDefinitionsByWorkspace returns graph definitions visible to the given workspace (P2-B).
// empty workspaceID = system caller (see all); non-empty = tenant caller (see shared + own).
func (r *graphRepo) ListDefinitionsByWorkspace(ctx context.Context, pageSize int, pageToken string, workspaceID string) ([]*biz.GraphDefinition, string, error) {
	client := r.data.RW().Read(ctx)
	query := client.GraphDefinition.Query().Order(
		ent.Asc(graphdefinition.FieldSortOrder),
		ent.Asc(graphdefinition.FieldCreatedAt),
		ent.Asc(graphdefinition.FieldID),
	)
	// P2-B: workspace 过滤。空 WorkspaceID = system caller（看全部）。
	// 租户 caller 只看：自己私有的（workspace_id == caller）+ 全局共享的（workspace_id == ""）。
	if workspaceID != "" {
		query = query.Where(graphdefinition.Or(
			graphdefinition.WorkspaceIDEQ(""),
			graphdefinition.WorkspaceIDEQ(workspaceID),
		))
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query = query.Limit(pageSize + 1)
	if pageToken != "" {
		cur, err := decodeKeysetCursor(pageToken, "GRAPH")
		if err != nil {
			return nil, "", err
		}
		query = query.Where(graphDefinitionKeyset(cur))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, "", entErrToBizErr(err, "GRAPH")
	}
	var nextToken string
	if len(rows) > pageSize {
		last := rows[pageSize-1]
		nextToken = encodeKeysetCursor(keysetCursor{SortOrder: last.SortOrder, Ts: last.CreatedAt.UnixMicro(), ID: last.ID})
		rows = rows[:pageSize]
	}
	result := make([]*biz.GraphDefinition, len(rows))
	for i, row := range rows {
		result[i] = entGraphToBiz(row, r.data.lg)
	}
	return result, nextToken, nil
}

// ListUserTemplateDefinitionsByWorkspace returns user template graph definitions
// visible to the given workspace (P2-B).
func (r *graphRepo) ListUserTemplateDefinitionsByWorkspace(ctx context.Context, pageSize int, workspaceID string) ([]*biz.GraphDefinition, error) {
	defs, _, err := r.ListDefinitionsByWorkspace(ctx, pageSize, "", workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]*biz.GraphDefinition, 0)
	for _, def := range defs {
		if biz.ReadUserTemplateMeta(def) != nil {
			out = append(out, def)
		}
	}
	return out, nil
}

func (r *graphRepo) DeleteDefinition(ctx context.Context, id string) error {
	client := r.data.RW().Write(ctx)
	err := client.GraphDefinition.DeleteOneID(id).Exec(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return entErrToBizErr(err, "GRAPH")
	}
	return nil
}

func (r *graphRepo) UpdateDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	client := r.data.RW().Write(ctx)
	stateFieldsJSON, _ := json.Marshal(def.StateFields)
	nodesJSON, _ := json.Marshal(def.Nodes)
	edgesJSON, _ := json.Marshal(def.Edges)
	condEdgesJSON, _ := json.Marshal(def.ConditionalEdges)
	subgraphsJSON, _ := json.Marshal(def.Subgraphs)
	interruptBeforeJSON, _ := json.Marshal(def.InterruptBefore)
	interruptAfterJSON, _ := json.Marshal(def.InterruptAfter)
	metadataJSON, _ := json.Marshal(def.Metadata)

	saved, err := client.GraphDefinition.UpdateOneID(def.ID).
		SetName(def.Name).
		SetDescription(def.Description).
		SetStateFields(string(stateFieldsJSON)).
		SetNodes(string(nodesJSON)).
		SetEdges(string(edgesJSON)).
		SetConditionalEdges(string(condEdgesJSON)).
		SetSubgraphs(string(subgraphsJSON)).
		SetEntryPoint(def.EntryPoint).
		SetFinishPoint(def.FinishPoint).
		SetEnableCheckpoint(def.EnableCheckpoint).
		SetExecutionEngine(string(def.ExecutionEngine)).
		SetInterruptBefore(string(interruptBeforeJSON)).
		SetInterruptAfter(string(interruptAfterJSON)).
		SetMetadata(string(metadataJSON)).
		SetVerificationGates(def.VerificationGates).
		SetSortOrder(def.SortOrder).
		SetTeamID(def.TeamID).
		SetIsTemplate(def.IsTemplate).
		SetUpdatedAt(def.UpdatedAt).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, apierror.NotFound("GRAPH", "graph definition not found")
		}
		return nil, entErrToBizErr(err, "GRAPH")
	}
	return entGraphToBiz(saved, r.data.lg), nil
}

func entGraphToBiz(row *ent.GraphDefinition, lg loggateway.Logger) *biz.GraphDefinition {
	def := &biz.GraphDefinition{
		ID:                row.ID,
		Name:              row.Name,
		Description:       row.Description,
		EntryPoint:        row.EntryPoint,
		FinishPoint:       row.FinishPoint,
		EnableCheckpoint:  row.EnableCheckpoint,
		ExecutionEngine:   biz.ExecutionEngineType(row.ExecutionEngine),
		VerificationGates: row.VerificationGates,
		SortOrder:         row.SortOrder,
		TeamID:            row.TeamID,
		IsTemplate:        row.IsTemplate,
		WorkspaceID:       row.WorkspaceID, // P2-B: tenant isolation
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
	if err := json.Unmarshal([]byte(row.StateFields), &def.StateFields); err != nil {
		lg.Warn("graph json unmarshal failed", loggateway.StepID("data.graph"), loggateway.Err(err))
	}
	if err := json.Unmarshal([]byte(row.Nodes), &def.Nodes); err != nil {
		lg.Warn("graph json unmarshal failed", loggateway.StepID("data.graph"), loggateway.Err(err))
	}
	if err := json.Unmarshal([]byte(row.Edges), &def.Edges); err != nil {
		lg.Warn("graph json unmarshal failed", loggateway.StepID("data.graph"), loggateway.Err(err))
	}
	if err := json.Unmarshal([]byte(row.ConditionalEdges), &def.ConditionalEdges); err != nil {
		lg.Warn("graph json unmarshal failed", loggateway.StepID("data.graph"), loggateway.Err(err))
	}
	if err := json.Unmarshal([]byte(row.Subgraphs), &def.Subgraphs); err != nil {
		lg.Warn("graph json unmarshal failed", loggateway.StepID("data.graph"), loggateway.Err(err))
	}
	if err := json.Unmarshal([]byte(row.InterruptBefore), &def.InterruptBefore); err != nil {
		lg.Warn("graph json unmarshal failed", loggateway.StepID("data.graph"), loggateway.Err(err))
	}
	if err := json.Unmarshal([]byte(row.InterruptAfter), &def.InterruptAfter); err != nil {
		lg.Warn("graph json unmarshal failed", loggateway.StepID("data.graph"), loggateway.Err(err))
	}
	if err := json.Unmarshal([]byte(row.Metadata), &def.Metadata); err != nil {
		lg.Warn("graph json unmarshal failed", loggateway.StepID("data.graph"), loggateway.Err(err))
	}
	def.Version = biz.GraphVersion(def)
	return def
}

func (r *graphRepo) ReorderGraphs(ctx context.Context, ids []string) error {
	return r.data.ExecInTx(ctx, func(ctx context.Context) error {
		client := EntClientFromCtx(ctx, r.data.entClient)
		for i, id := range ids {
			if err := client.GraphDefinition.UpdateOneID(id).
				SetSortOrder(i + 1).
				Exec(ctx); err != nil {
				return entErrToBizErr(err, "GRAPH")
			}
		}
		return nil
	})
}

type graphRunRepo struct {
	data *Data
}

func NewGraphRunRepo(data *Data) biz.GraphRunRepo {
	return &graphRunRepo{data: data}
}

func (r *graphRunRepo) SaveRun(ctx context.Context, exec *biz.GraphExecution) error {
	client := r.data.RW().Write(ctx)
	currentStateJSON, _ := json.Marshal(exec.CurrentState)
	stepsJSON, _ := json.Marshal(exec.Steps)

	builder := client.GraphExecution.Create().
		SetID(exec.ID).
		SetGraphID(exec.GraphID).
		SetSessionID(exec.SessionID).
		SetStatus(exec.Status).
		SetCurrentNode(exec.CurrentNode).
		SetLineageID(exec.LineageID).
		SetErrorMessage(exec.ErrorMessage).
		SetCurrentStateJSON(string(currentStateJSON)).
		SetStepsJSON(string(stepsJSON)).
		SetStartedAt(exec.StartedAt)

	if exec.FinishedAt != nil {
		builder.SetFinishedAt(*exec.FinishedAt)
	}
	_, err := builder.Save(ctx)
	if err != nil {
		return entErrToBizErr(err, "GRAPH_RUN")
	}
	return nil
}

func (r *graphRunRepo) GetRun(ctx context.Context, id string) (*biz.GraphExecution, error) {
	client := r.data.RW().Read(ctx)
	row, err := client.GraphExecution.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, apierror.NotFound("GRAPH_RUN", "graph execution not found")
		}
		return nil, entErrToBizErr(err, "GRAPH_RUN")
	}
	return entGraphRunToBiz(row, r.data.lg), nil
}

func (r *graphRunRepo) ListRunsByGraph(ctx context.Context, graphID string, pageSize int, pageToken string, opts ...biz.GraphRunListOption) ([]*biz.GraphExecution, string, error) {
	client := r.data.RW().Read(ctx)
	query := client.GraphExecution.Query().
		Where(graphexecution.GraphIDEQ(graphID)).
		Order(ent.Desc(graphexecution.FieldStartedAt), ent.Desc(graphexecution.FieldID))

	// 合并全部过滤条件（此前只取 opts[0]，status+startedAfter 同传时后者被静默丢弃）。
	for _, opt := range opts {
		if opt.Status != "" {
			query = query.Where(graphexecution.StatusEQ(opt.Status))
		}
		if opt.StartedAfter != nil {
			query = query.Where(graphexecution.StartedAtGTE(*opt.StartedAfter))
		}
	}

	if pageSize <= 0 {
		pageSize = 20
	}
	query = query.Limit(pageSize + 1)
	if pageToken != "" {
		cur, err := decodeKeysetCursor(pageToken, "GRAPH_RUN")
		if err != nil {
			return nil, "", err
		}
		// 与 ORDER BY started_at DESC, id DESC 严格一致的 keyset 续页谓词。
		ts := time.UnixMicro(cur.Ts).UTC()
		query = query.Where(graphexecution.Or(
			graphexecution.StartedAtLT(ts),
			graphexecution.And(
				graphexecution.StartedAtEQ(ts),
				graphexecution.IDLT(cur.ID),
			),
		))
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, "", entErrToBizErr(err, "GRAPH_RUN")
	}
	var nextToken string
	if len(rows) > pageSize {
		last := rows[pageSize-1]
		nextToken = encodeKeysetCursor(keysetCursor{Ts: last.StartedAt.UnixMicro(), ID: last.ID})
		rows = rows[:pageSize]
	}
	result := make([]*biz.GraphExecution, len(rows))
	for i, row := range rows {
		result[i] = entGraphRunToBiz(row, r.data.lg)
	}
	return result, nextToken, nil
}

func (r *graphRunRepo) UpdateRun(ctx context.Context, exec *biz.GraphExecution) error {
	client := r.data.RW().Write(ctx)
	currentStateJSON, _ := json.Marshal(exec.CurrentState)
	stepsJSON, _ := json.Marshal(exec.Steps)

	builder := client.GraphExecution.UpdateOneID(exec.ID).
		SetStatus(exec.Status).
		SetCurrentNode(exec.CurrentNode).
		SetLineageID(exec.LineageID).
		SetErrorMessage(exec.ErrorMessage).
		SetCurrentStateJSON(string(currentStateJSON)).
		SetStepsJSON(string(stepsJSON))

	if exec.FinishedAt != nil {
		builder.SetFinishedAt(*exec.FinishedAt)
	}
	_, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return apierror.NotFound("GRAPH_RUN", "graph execution not found")
		}
		return entErrToBizErr(err, "GRAPH_RUN")
	}
	return nil
}

func entGraphRunToBiz(row *ent.GraphExecution, lg loggateway.Logger) *biz.GraphExecution {
	exec := &biz.GraphExecution{
		ID:           row.ID,
		GraphID:      row.GraphID,
		SessionID:    row.SessionID,
		Status:       row.Status,
		CurrentNode:  row.CurrentNode,
		LineageID:    row.LineageID,
		ErrorMessage: row.ErrorMessage,
		StartedAt:    row.StartedAt,
	}
	if row.FinishedAt != nil {
		fa := *row.FinishedAt
		exec.FinishedAt = &fa
	}
	if err := json.Unmarshal([]byte(row.CurrentStateJSON), &exec.CurrentState); err != nil {
		lg.Warn("graph json unmarshal failed", loggateway.StepID("data.graph"), loggateway.Err(err))
	}
	if err := json.Unmarshal([]byte(row.StepsJSON), &exec.Steps); err != nil {
		lg.Warn("graph json unmarshal failed", loggateway.StepID("data.graph"), loggateway.Err(err))
	}
	return exec
}
