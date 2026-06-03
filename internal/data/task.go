package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/graphtask"
	"aranea-agents/internal/data/ent/graphtaskcomment"
	"aranea-agents/internal/data/ent/graphtaskevent"
	"aranea-agents/internal/data/ent/graphtasklog"
	"aranea-agents/internal/data/ent/graphtaskrun"

	"github.com/go-kratos/kratos/v2/errors"
)

type taskRepo struct {
	data *Data
}

var _ biz.TaskRepo = (*taskRepo)(nil)

func NewTaskRepo(data *Data) biz.TaskRepo {
	return &taskRepo{data: data}
}

func (r *taskRepo) SaveTask(ctx context.Context, task *biz.GraphTask) error {
	client := r.data.RW().Write(ctx)
	builder := client.GraphTask.Create().
		SetID(task.TaskID).
		SetNodeID(task.NodeID).
		SetExecutionID(task.ExecutionID).
		SetAssignee(task.Assignee).
		SetStatus(string(task.Status)).
		SetContext(task.Context).
		SetInput(task.Input).
		SetOutput(task.Output).
		SetSummary(task.Summary).
		SetMetadata(task.Metadata).
		SetRequiredRole(task.RequiredRole).
		SetAssignmentMode(task.AssignmentMode).
		SetAssignmentStrategy(task.AssignmentStrategy).
		SetCreatedAt(task.CreatedAt)
	if task.ClaimedAt != nil {
		builder.SetClaimedAt(*task.ClaimedAt)
	}
	if task.CompletedAt != nil {
		builder.SetCompletedAt(*task.CompletedAt)
	}
	_, err := builder.Save(ctx)
	if err != nil {
		return fmt.Errorf("task repo save: %w", err)
	}
	return nil
}

func (r *taskRepo) GetTask(ctx context.Context, taskID string) (*biz.GraphTask, error) {
	client := r.data.RW().Read(ctx)
	row, err := client.GraphTask.Get(ctx, taskID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.NotFound("TASK", "task not found")
		}
		return nil, fmt.Errorf("task repo get: %w", err)
	}
	return entTaskToBiz(row), nil
}

func (r *taskRepo) GetTasksByIDs(ctx context.Context, taskIDs []string) ([]*biz.GraphTask, error) {
	if len(taskIDs) == 0 {
		return nil, nil
	}
	client := r.data.RW().Read(ctx)
	rows, err := client.GraphTask.Query().
		Where(graphtask.IDIn(taskIDs...)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("task repo get by ids: %w", err)
	}
	result := make([]*biz.GraphTask, len(rows))
	for i, row := range rows {
		result[i] = entTaskToBiz(row)
	}
	return result, nil
}

func (r *taskRepo) GetActiveTaskByExecutionNode(ctx context.Context, executionID, nodeID string) (*biz.GraphTask, error) {
	client := r.data.RW().Read(ctx)
	row, err := client.GraphTask.Query().
		Where(
			graphtask.ExecutionIDEQ(executionID),
			graphtask.NodeIDEQ(nodeID),
			graphtask.StatusNotIn(
				string(biz.TaskStatusComplete),
				string(biz.TaskStatusCancelled),
			),
		).
		Order(ent.Desc(graphtask.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.NotFound("TASK", "task not found")
		}
		return nil, fmt.Errorf("task repo get active: %w", err)
	}
	return entTaskToBiz(row), nil
}

func (r *taskRepo) ListTasksByStatuses(ctx context.Context, statuses []biz.TaskStatus, limit int) ([]*biz.GraphTask, error) {
	if limit <= 0 {
		limit = 50
	}
	client := r.data.RW().Read(ctx)
	strs := make([]string, 0, len(statuses))
	for _, s := range statuses {
		if s != "" {
			strs = append(strs, string(s))
		}
	}
	query := client.GraphTask.Query().Order(ent.Asc(graphtask.FieldCreatedAt)).Limit(limit)
	if len(strs) > 0 {
		query = query.Where(graphtask.StatusIn(strs...))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("task repo list statuses: %w", err)
	}
	result := make([]*biz.GraphTask, len(rows))
	for i, row := range rows {
		result[i] = entTaskToBiz(row)
	}
	return result, nil
}

func (r *taskRepo) ListTasksByExecution(ctx context.Context, executionID string, status biz.TaskStatus, pageSize int, pageToken string) ([]*biz.GraphTask, string, error) {
	client := r.data.RW().Read(ctx)
	query := client.GraphTask.Query().
		Where(graphtask.ExecutionIDEQ(executionID)).
		Order(ent.Asc(graphtask.FieldCreatedAt))
	if status != "" {
		query = query.Where(graphtask.StatusEQ(string(status)))
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query = query.Limit(pageSize + 1)
	if pageToken != "" {
		query = query.Where(graphtask.IDGT(pageToken))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("task repo list: %w", err)
	}
	var nextToken string
	if len(rows) > pageSize {
		nextToken = rows[pageSize-1].ID
		rows = rows[:pageSize]
	}
	result := make([]*biz.GraphTask, len(rows))
	for i, row := range rows {
		result[i] = entTaskToBiz(row)
	}
	return result, nextToken, nil
}

func (r *taskRepo) UpdateTask(ctx context.Context, task *biz.GraphTask) error {
	client := r.data.RW().Write(ctx)
	builder := client.GraphTask.UpdateOneID(task.TaskID).
		SetAssignee(task.Assignee).
		SetStatus(string(task.Status)).
		SetOutput(task.Output).
		SetSummary(task.Summary).
		SetMetadata(task.Metadata)
	if task.ClaimedAt != nil {
		builder.SetClaimedAt(*task.ClaimedAt)
	}
	if task.CompletedAt != nil {
		builder.SetCompletedAt(*task.CompletedAt)
	}
	_, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errors.NotFound("TASK", "task not found")
		}
		return fmt.Errorf("task repo update: %w", err)
	}
	return nil
}

func (r *taskRepo) BatchUpdateTaskStatus(ctx context.Context, taskIDs []string, status biz.TaskStatus) error {
	if len(taskIDs) == 0 {
		return nil
	}
	client := r.data.RW().Write(ctx)
	_, err := client.GraphTask.Update().
		Where(graphtask.IDIn(taskIDs...)).
		SetStatus(string(status)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("task repo batch update status: %w", err)
	}
	return nil
}

func (r *taskRepo) SaveTaskComment(ctx context.Context, comment *biz.TaskComment) error {
	client := r.data.RW().Write(ctx)
	_, err := client.GraphTaskComment.Create().
		SetID(comment.CommentID).
		SetTaskID(comment.TaskID).
		SetAuthor(comment.Author).
		SetContent(comment.Content).
		SetType(comment.Type).
		SetCreatedAt(comment.CreatedAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("task comment repo save: %w", err)
	}
	return nil
}

func (r *taskRepo) ListTaskComments(ctx context.Context, taskID string) ([]*biz.TaskComment, error) {
	client := r.data.RW().Read(ctx)
	rows, err := client.GraphTaskComment.Query().
		Where(graphtaskcomment.TaskIDEQ(taskID)).
		Order(ent.Asc(graphtaskcomment.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("task comment repo list: %w", err)
	}
	result := make([]*biz.TaskComment, len(rows))
	for i, row := range rows {
		result[i] = &biz.TaskComment{
			CommentID: row.ID,
			TaskID:    row.TaskID,
			Author:    row.Author,
			Content:   row.Content,
			Type:      row.Type,
			CreatedAt: row.CreatedAt,
		}
	}
	return result, nil
}

func (r *taskRepo) SaveTaskLog(ctx context.Context, log *biz.TaskLog) error {
	client := r.data.RW().Write(ctx)
	_, err := client.GraphTaskLog.Create().
		SetID(log.LogID).
		SetTaskID(log.TaskID).
		SetStream(log.Stream).
		SetContent(log.Content).
		SetLevel(log.Level).
		SetTimestamp(log.Timestamp).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("task log repo save: %w", err)
	}
	return nil
}

func (r *taskRepo) ListTaskLogs(ctx context.Context, taskID string, stream string, level string, pageSize int) ([]*biz.TaskLog, error) {
	client := r.data.RW().Read(ctx)
	query := client.GraphTaskLog.Query().
		Where(graphtasklog.TaskIDEQ(taskID)).
		Order(ent.Asc(graphtasklog.FieldTimestamp))
	if stream != "" {
		query = query.Where(graphtasklog.StreamEQ(stream))
	}
	if level != "" {
		query = query.Where(graphtasklog.LevelEQ(level))
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	query = query.Limit(pageSize)
	rows, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("task log repo list: %w", err)
	}
	result := make([]*biz.TaskLog, len(rows))
	for i, row := range rows {
		result[i] = &biz.TaskLog{
			LogID:     row.ID,
			TaskID:    row.TaskID,
			Stream:    row.Stream,
			Content:   row.Content,
			Level:     row.Level,
			Timestamp: row.Timestamp,
		}
	}
	return result, nil
}

func (r *taskRepo) SaveTaskRun(ctx context.Context, run *biz.TaskRun) error {
	client := r.data.RW().Write(ctx)
	builder := client.GraphTaskRun.Create().
		SetID(run.RunID).
		SetTaskID(run.TaskID).
		SetStartedAt(run.StartedAt).
		SetExitCode(run.ExitCode).
		SetLogRef(run.LogRef)
	if run.FinishedAt != nil {
		builder.SetFinishedAt(*run.FinishedAt)
	}
	_, err := builder.Save(ctx)
	if err != nil {
		return fmt.Errorf("task run repo save: %w", err)
	}
	return nil
}

func (r *taskRepo) ListTaskRuns(ctx context.Context, taskID string) ([]*biz.TaskRun, error) {
	client := r.data.RW().Read(ctx)
	rows, err := client.GraphTaskRun.Query().
		Where(graphtaskrun.TaskIDEQ(taskID)).
		Order(ent.Desc(graphtaskrun.FieldStartedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("task run repo list: %w", err)
	}
	result := make([]*biz.TaskRun, len(rows))
	for i, row := range rows {
		r := &biz.TaskRun{
			RunID:     row.ID,
			TaskID:    row.TaskID,
			StartedAt: row.StartedAt,
			ExitCode:  row.ExitCode,
			LogRef:    row.LogRef,
		}
		if row.FinishedAt != nil {
			fa := *row.FinishedAt
			r.FinishedAt = &fa
		}
		result[i] = r
	}
	return result, nil
}

func (r *taskRepo) SaveTaskEvent(ctx context.Context, event *biz.TaskEvent) error {
	client := r.data.RW().Write(ctx)
	_, err := client.GraphTaskEvent.Create().
		SetID(event.EventID).
		SetTaskID(event.TaskID).
		SetEventType(event.EventType).
		SetSourceNode(event.SourceNode).
		SetDescription(event.Description).
		SetTimestamp(event.Timestamp).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("task event repo save: %w", err)
	}
	return nil
}

func (r *taskRepo) ListTaskEvents(ctx context.Context, executionID string, taskID string, eventType string, pageSize int) ([]*biz.TaskEvent, error) {
	client := r.data.RW().Read(ctx)
	query := client.GraphTaskEvent.Query().
		Order(ent.Desc(graphtaskevent.FieldTimestamp))
	// NOTE: executionID is unused because graph_task_events has no execution_id column.
	// Filtering by execution_id would require a JOIN on graph_tasks or a schema migration.
	_ = executionID
	if taskID != "" {
		query = query.Where(graphtaskevent.TaskIDEQ(taskID))
	}
	if eventType != "" {
		query = query.Where(graphtaskevent.EventTypeEQ(eventType))
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	query = query.Limit(pageSize)
	rows, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("task event repo list: %w", err)
	}
	result := make([]*biz.TaskEvent, len(rows))
	for i, row := range rows {
		result[i] = &biz.TaskEvent{
			EventID:     row.ID,
			TaskID:      row.TaskID,
			EventType:   row.EventType,
			SourceNode:  row.SourceNode,
			Description: row.Description,
			Timestamp:   row.Timestamp,
		}
	}
	return result, nil
}

func entTaskToBiz(row *ent.GraphTask) *biz.GraphTask {
	task := &biz.GraphTask{
		TaskID:             row.ID,
		NodeID:             row.NodeID,
		ExecutionID:        row.ExecutionID,
		Assignee:           row.Assignee,
		Status:             biz.TaskStatus(row.Status),
		Context:            row.Context,
		Input:              row.Input,
		Output:             row.Output,
		Summary:            row.Summary,
		Metadata:           row.Metadata,
		RequiredRole:       row.RequiredRole,
		AssignmentMode:     row.AssignmentMode,
		AssignmentStrategy: row.AssignmentStrategy,
		CreatedAt:          row.CreatedAt,
	}
	if row.ClaimedAt != nil {
		ca := *row.ClaimedAt
		task.ClaimedAt = &ca
	}
	if row.CompletedAt != nil {
		ca := *row.CompletedAt
		task.CompletedAt = &ca
	}
	return task
}
