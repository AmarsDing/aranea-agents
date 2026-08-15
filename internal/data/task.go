package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/graphtask"
	"aranea-agents/internal/data/ent/graphtaskcomment"
	"aranea-agents/internal/data/ent/graphtaskevent"
	"aranea-agents/internal/data/ent/graphtasklog"
	"aranea-agents/internal/data/ent/graphtaskrun"
	"aranea-agents/pkg/apierror"
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
		return entErrToBizErr(err, "TASK")
	}
	return nil
}

func (r *taskRepo) GetTask(ctx context.Context, taskID string) (*biz.GraphTask, error) {
	client := r.data.RW().Read(ctx)
	row, err := client.GraphTask.Get(ctx, taskID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, apierror.NotFound("TASK", "task not found")
		}
		return nil, entErrToBizErr(err, "TASK")
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
		return nil, entErrToBizErr(err, "TASK")
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
				string(biz.GraphTaskStatusComplete),
				string(biz.GraphTaskStatusCancelled),
			),
		).
		Order(ent.Desc(graphtask.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, apierror.NotFound("TASK", "task not found")
		}
		return nil, entErrToBizErr(err, "TASK")
	}
	return entTaskToBiz(row), nil
}

func (r *taskRepo) ListTasksByStatuses(ctx context.Context, statuses []biz.GraphTaskStatus, limit int) ([]*biz.GraphTask, error) {
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
		return nil, entErrToBizErr(err, "TASK")
	}
	result := make([]*biz.GraphTask, len(rows))
	for i, row := range rows {
		result[i] = entTaskToBiz(row)
	}
	return result, nil
}

func (r *taskRepo) ListTasksByExecution(ctx context.Context, executionID string, status biz.GraphTaskStatus, pageSize int, pageToken string) ([]*biz.GraphTask, string, error) {
	client := r.data.RW().Read(ctx)
	query := client.GraphTask.Query().
		Where(graphtask.ExecutionIDEQ(executionID)).
		Order(ent.Asc(graphtask.FieldCreatedAt), ent.Asc(graphtask.FieldID))
	if status != "" {
		query = query.Where(graphtask.StatusEQ(string(status)))
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query = query.Limit(pageSize + 1)
	if pageToken != "" {
		cur, err := decodeKeysetCursor(pageToken, "TASK")
		if err != nil {
			return nil, "", err
		}
		// 与 ORDER BY created_at ASC, id ASC 严格一致的 keyset 续页谓词。
		ts := time.UnixMicro(cur.Ts).UTC()
		query = query.Where(graphtask.Or(
			graphtask.CreatedAtGT(ts),
			graphtask.And(
				graphtask.CreatedAtEQ(ts),
				graphtask.IDGT(cur.ID),
			),
		))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, "", entErrToBizErr(err, "TASK")
	}
	var nextToken string
	if len(rows) > pageSize {
		last := rows[pageSize-1]
		nextToken = encodeKeysetCursor(keysetCursor{Ts: last.CreatedAt.UnixMicro(), ID: last.ID})
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
			return apierror.NotFound("TASK", "task not found")
		}
		return entErrToBizErr(err, "TASK")
	}
	return nil
}

func (r *taskRepo) BatchUpdateGraphTaskStatus(ctx context.Context, taskIDs []string, status biz.GraphTaskStatus) error {
	if len(taskIDs) == 0 {
		return nil
	}
	client := r.data.RW().Write(ctx)
	_, err := client.GraphTask.Update().
		Where(graphtask.IDIn(taskIDs...)).
		SetStatus(string(status)).
		Save(ctx)
	if err != nil {
		return entErrToBizErr(err, "TASK")
	}
	return nil
}

// taskStatusStrings converts biz statuses to the string slice ent predicates need.
func taskStatusStrings(statuses []biz.GraphTaskStatus) []string {
	strs := make([]string, 0, len(statuses))
	for _, s := range statuses {
		if s != "" {
			strs = append(strs, string(s))
		}
	}
	return strs
}

// ClaimTaskWhereStatus atomically claims a task: only rows whose status is in
// fromStatuses transition to claimed with assignee/claimed_at set. Zero rows
// matched → (nil, false, nil) so the usecase can surface a Conflict instead of
// silently overwriting a concurrent winner (DB-R5: errors via entErrToBizErr).
func (r *taskRepo) ClaimTaskWhereStatus(ctx context.Context, taskID string, agentKey string, fromStatuses []biz.GraphTaskStatus) (*biz.GraphTask, bool, error) {
	from := taskStatusStrings(fromStatuses)
	if len(from) == 0 {
		return nil, false, nil
	}
	client := r.data.RW().Write(ctx)
	n, err := client.GraphTask.Update().
		Where(graphtask.IDIn(taskID), graphtask.StatusIn(from...)).
		SetStatus(string(biz.GraphTaskStatusClaimed)).
		SetAssignee(agentKey).
		SetClaimedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return nil, false, entErrToBizErr(err, "TASK")
	}
	if n == 0 {
		return nil, false, nil
	}
	t, err := r.GetTask(ctx, taskID)
	if err != nil {
		return nil, false, err
	}
	return t, true, nil
}

// CompleteTaskWhereStatus atomically submits a result: only rows in
// claimed/review_required (and, when submitter is non-empty, assigned to that
// submitter) are updated. Zero rows matched → (nil, false, nil).
func (r *taskRepo) CompleteTaskWhereStatus(ctx context.Context, taskID string, submitter string, output string, summary string, metadata string, toStatus biz.GraphTaskStatus) (*biz.GraphTask, bool, error) {
	client := r.data.RW().Write(ctx)
	builder := client.GraphTask.Update().
		Where(
			graphtask.IDIn(taskID),
			graphtask.StatusIn(string(biz.GraphTaskStatusClaimed), string(biz.GraphTaskStatusReviewRequired)),
		).
		SetStatus(string(toStatus)).
		SetOutput(output).
		SetSummary(summary).
		SetMetadata(metadata).
		SetCompletedAt(time.Now())
	if submitter != "" {
		builder = builder.Where(graphtask.AssigneeEQ(submitter))
	}
	n, err := builder.Save(ctx)
	if err != nil {
		return nil, false, entErrToBizErr(err, "TASK")
	}
	if n == 0 {
		return nil, false, nil
	}
	t, err := r.GetTask(ctx, taskID)
	if err != nil {
		return nil, false, err
	}
	return t, true, nil
}

// TransitionTaskStatusWhere atomically transitions status only (no field
// writes), guarding on fromStatuses. Zero rows matched → (nil, false, nil).
func (r *taskRepo) TransitionTaskStatusWhere(ctx context.Context, taskID string, fromStatuses []biz.GraphTaskStatus, toStatus biz.GraphTaskStatus) (*biz.GraphTask, bool, error) {
	from := taskStatusStrings(fromStatuses)
	if len(from) == 0 {
		return nil, false, nil
	}
	client := r.data.RW().Write(ctx)
	n, err := client.GraphTask.Update().
		Where(graphtask.IDIn(taskID), graphtask.StatusIn(from...)).
		SetStatus(string(toStatus)).
		Save(ctx)
	if err != nil {
		return nil, false, entErrToBizErr(err, "TASK")
	}
	if n == 0 {
		return nil, false, nil
	}
	t, err := r.GetTask(ctx, taskID)
	if err != nil {
		return nil, false, err
	}
	return t, true, nil
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
		return entErrToBizErr(err, "TASK")
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
		return nil, entErrToBizErr(err, "TASK")
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
		return entErrToBizErr(err, "TASK")
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
		return nil, entErrToBizErr(err, "TASK")
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
		return entErrToBizErr(err, "TASK")
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
		return nil, entErrToBizErr(err, "TASK")
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
		return entErrToBizErr(err, "TASK")
	}
	return nil
}

func (r *taskRepo) ListTaskEvents(ctx context.Context, executionID string, taskID string, eventType string, pageSize int) ([]*biz.TaskEvent, error) {
	client := r.data.RW().Read(ctx)
	query := client.GraphTaskEvent.Query().
		Order(ent.Desc(graphtaskevent.FieldTimestamp))
	// NOTE: executionID is unused because graph_task_events has no execution_id column.
	// Filtering by execution_id would require a JOIN on graph_tasks or a schema migration.
	_ = executionID // lint:ignore
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
		return nil, entErrToBizErr(err, "TASK")
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
		Status:             biz.GraphTaskStatus(row.Status),
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
