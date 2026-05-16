package data

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/graphdefinition"
	"aranea-agents/internal/data/ent/graphexecution"

	"github.com/go-kratos/kratos/v2/errors"
)

type graphRepo struct {
	data *Data
}

func NewGraphRepo(data *Data) biz.GraphRepo {
	return &graphRepo{data: data}
}

func (r *graphRepo) SaveDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	client := r.data.Ent()
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
		SetMetadata(string(metadataJSON))

	if def.ID != "" {
		builder.SetID(def.ID)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("graph repo save: %w", err)
	}
	return entGraphToBiz(saved), nil
}

func (r *graphRepo) GetDefinition(ctx context.Context, id string) (*biz.GraphDefinition, error) {
	client := r.data.Ent()
	row, err := client.GraphDefinition.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.NotFound("GRAPH", "graph definition not found")
		}
		return nil, fmt.Errorf("graph repo get: %w", err)
	}
	return entGraphToBiz(row), nil
}

func (r *graphRepo) ListDefinitions(ctx context.Context, pageSize int, pageToken string) ([]*biz.GraphDefinition, string, error) {
	client := r.data.Ent()
	query := client.GraphDefinition.Query().Order(ent.Asc(graphdefinition.FieldCreatedAt))
	if pageSize <= 0 {
		pageSize = 20
	}
	query = query.Limit(pageSize + 1)
	if pageToken != "" {
		query = query.Where(graphdefinition.IDGT(pageToken))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("graph repo list: %w", err)
	}
	var nextToken string
	if len(rows) > pageSize {
		nextToken = rows[pageSize-1].ID
		rows = rows[:pageSize]
	}
	result := make([]*biz.GraphDefinition, len(rows))
	for i, row := range rows {
		result[i] = entGraphToBiz(row)
	}
	return result, nextToken, nil
}

func (r *graphRepo) DeleteDefinition(ctx context.Context, id string) error {
	client := r.data.Ent()
	err := client.GraphDefinition.DeleteOneID(id).Exec(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("graph repo delete: %w", err)
	}
	return nil
}

func (r *graphRepo) UpdateDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error) {
	client := r.data.Ent()
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
		SetUpdatedAt(def.UpdatedAt).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.NotFound("GRAPH", "graph definition not found")
		}
		return nil, fmt.Errorf("graph repo update: %w", err)
	}
	return entGraphToBiz(saved), nil
}

func entGraphToBiz(row *ent.GraphDefinition) *biz.GraphDefinition {
	def := &biz.GraphDefinition{
		ID:               row.ID,
		Name:             row.Name,
		Description:      row.Description,
		EntryPoint:       row.EntryPoint,
		FinishPoint:      row.FinishPoint,
		EnableCheckpoint: row.EnableCheckpoint,
		ExecutionEngine:  graphtrpc.ExecutionEngineType(row.ExecutionEngine),
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
	_ = json.Unmarshal([]byte(row.StateFields), &def.StateFields)
	_ = json.Unmarshal([]byte(row.Nodes), &def.Nodes)
	_ = json.Unmarshal([]byte(row.Edges), &def.Edges)
	_ = json.Unmarshal([]byte(row.ConditionalEdges), &def.ConditionalEdges)
	_ = json.Unmarshal([]byte(row.Subgraphs), &def.Subgraphs)
	_ = json.Unmarshal([]byte(row.InterruptBefore), &def.InterruptBefore)
	_ = json.Unmarshal([]byte(row.InterruptAfter), &def.InterruptAfter)
	_ = json.Unmarshal([]byte(row.Metadata), &def.Metadata)
	return def
}

type graphRunRepo struct {
	data *Data
}

func NewGraphRunRepo(data *Data) biz.GraphRunRepo {
	return &graphRunRepo{data: data}
}

func (r *graphRunRepo) SaveRun(ctx context.Context, exec *biz.GraphExecution) error {
	client := r.data.Ent()
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
		return fmt.Errorf("graph run repo save: %w", err)
	}
	return nil
}

func (r *graphRunRepo) GetRun(ctx context.Context, id string) (*biz.GraphExecution, error) {
	client := r.data.Ent()
	row, err := client.GraphExecution.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.NotFound("GRAPH_RUN", "graph execution not found")
		}
		return nil, fmt.Errorf("graph run repo get: %w", err)
	}
	return entGraphRunToBiz(row), nil
}

func (r *graphRunRepo) ListRunsByGraph(ctx context.Context, graphID string, pageSize int, pageToken string) ([]*biz.GraphExecution, string, error) {
	client := r.data.Ent()
	query := client.GraphExecution.Query().
		Where(graphexecution.GraphIDEQ(graphID)).
		Order(ent.Desc(graphexecution.FieldStartedAt))

	if pageSize <= 0 {
		pageSize = 20
	}
	query = query.Limit(pageSize + 1)
	if pageToken != "" {
		query = query.Where(graphexecution.IDLT(pageToken))
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("graph run repo list: %w", err)
	}
	var nextToken string
	if len(rows) > pageSize {
		nextToken = rows[pageSize-1].ID
		rows = rows[:pageSize]
	}
	result := make([]*biz.GraphExecution, len(rows))
	for i, row := range rows {
		result[i] = entGraphRunToBiz(row)
	}
	return result, nextToken, nil
}

func (r *graphRunRepo) UpdateRun(ctx context.Context, exec *biz.GraphExecution) error {
	client := r.data.Ent()
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
			return errors.NotFound("GRAPH_RUN", "graph execution not found")
		}
		return fmt.Errorf("graph run repo update: %w", err)
	}
	return nil
}

func entGraphRunToBiz(row *ent.GraphExecution) *biz.GraphExecution {
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
	_ = json.Unmarshal([]byte(row.CurrentStateJSON), &exec.CurrentState)
	_ = json.Unmarshal([]byte(row.StepsJSON), &exec.Steps)
	return exec
}

func NewGraphCheckpointSaver(data *Data) (*graphtrpc.SQLiteCheckpointSaver, error) {
	if data == nil {
		return nil, fmt.Errorf("data is nil")
	}
	db := data.RawDB()
	if db == nil {
		return nil, fmt.Errorf("sqlite raw db is nil")
	}
	return graphtrpc.NewSQLiteCheckpointSaver(db)
}
