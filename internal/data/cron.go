package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/crontask"
	"aranea-agents/internal/data/ent/crontaskrun"

	entsql "entgo.io/ent/dialect/sql"
)

type cronRepo struct {
	data *Data
}

// NewCronRepo implements biz.CronRepo for cron_task / cron_task_run.
func NewCronRepo(d *Data) biz.CronRepo {
	return &cronRepo{data: d}
}

func entToBizCronTask(e *ent.CronTask) biz.CronTask {
	if e == nil {
		return biz.CronTask{}
	}
	return biz.CronTask{
		ID:           e.ID,
		TaskKey:      e.TaskKey,
		Name:         e.Name,
		Description:  e.Description,
		Status:       e.Status,
		Enabled:      e.Enabled,
		SortOrder:    e.SortOrder,
		AgentID:      e.AgentID,
		ConfigJSON:   e.ConfigJSON,
		MetadataJSON: e.MetadataJSON,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
		DeletedAt:    e.DeletedAt,
	}
}

func outputJSONExtras(outputJSON string) (trigger, runID string) {
	var output struct {
		Trigger string `json:"trigger"`
		RunID   string `json:"run_id"`
	}
	j := strings.TrimSpace(outputJSON)
	if j == "" {
		j = "{}"
	}
	if json.Unmarshal([]byte(j), &output) == nil {
		trigger = output.Trigger
		runID = output.RunID
	}
	if trigger == "" {
		trigger = "schedule"
	}
	return trigger, runID
}

func entToBizCronTaskRun(e *ent.CronTaskRun, taskName string) biz.CronTaskRun {
	if e == nil {
		return biz.CronTaskRun{}
	}
	trigger, runID := outputJSONExtras(e.OutputJSON)
	return biz.CronTaskRun{
		ID:           e.ID,
		TaskID:       e.TaskID,
		TaskName:     taskName,
		Status:       e.Status,
		StartedAt:    e.StartedAt,
		FinishedAt:   e.FinishedAt,
		OutputJSON:   e.OutputJSON,
		ErrorMessage: e.ErrorMessage,
		CreatedAt:    e.CreatedAt,
		Trigger:      trigger,
		RunID:        runID,
	}
}

func (r *cronRepo) ListCronTasks(ctx context.Context) ([]biz.CronTask, error) {
	rows, err := r.data.entClient.CronTask.Query().
		Where(crontask.DeletedAtEQ("")).
		Order(
			crontask.BySortOrder(),
			crontask.ByCreatedAt(entsql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.CronTask, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizCronTask(e))
	}
	return out, nil
}

func (r *cronRepo) GetCronTask(ctx context.Context, id string) (biz.CronTask, error) {
	row, err := r.data.entClient.CronTask.Query().
		Where(crontask.IDEQ(id), crontask.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.CronTask{}, sql.ErrNoRows
		}
		return biz.CronTask{}, err
	}
	return entToBizCronTask(row), nil
}

func (r *cronRepo) CreateCronTask(ctx context.Context, t biz.CronTask) (biz.CronTask, error) {
	now := nowRFC3339()
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	saved, err := r.data.entClient.CronTask.Create().
		SetID(t.ID).
		SetTaskKey(t.TaskKey).
		SetName(t.Name).
		SetDescription(t.Description).
		SetStatus(t.Status).
		SetEnabled(t.Enabled).
		SetSortOrder(t.SortOrder).
		SetAgentID(t.AgentID).
		SetConfigJSON(t.ConfigJSON).
		SetMetadataJSON(t.MetadataJSON).
		SetCreatedAt(t.CreatedAt).
		SetUpdatedAt(t.UpdatedAt).
		SetDeletedAt("").
		Save(ctx)
	if err != nil {
		return biz.CronTask{}, err
	}
	return entToBizCronTask(saved), nil
}

func (r *cronRepo) UpdateCronTask(ctx context.Context, t biz.CronTask) (biz.CronTask, error) {
	t.UpdatedAt = nowRFC3339()
	err := r.data.entClient.CronTask.UpdateOneID(t.ID).
		SetTaskKey(t.TaskKey).
		SetName(t.Name).
		SetDescription(t.Description).
		SetStatus(t.Status).
		SetEnabled(t.Enabled).
		SetSortOrder(t.SortOrder).
		SetAgentID(t.AgentID).
		SetConfigJSON(t.ConfigJSON).
		SetMetadataJSON(t.MetadataJSON).
		SetUpdatedAt(t.UpdatedAt).
		Exec(ctx)
	if err != nil {
		return biz.CronTask{}, err
	}
	return r.GetCronTask(ctx, t.ID)
}

func (r *cronRepo) DeleteCronTask(ctx context.Context, id string) error {
	now := nowRFC3339()
	return r.data.entClient.CronTask.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Exec(ctx)
}

func (r *cronRepo) ListCronTaskRuns(ctx context.Context, q biz.CronTaskRunQuery) ([]biz.CronTaskRun, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	query := r.data.entClient.CronTaskRun.Query().
		Order(crontaskrun.ByCreatedAt(entsql.OrderDesc())).
		Limit(limit)
	if tid := strings.TrimSpace(q.TaskID); tid != "" {
		query = query.Where(crontaskrun.TaskIDEQ(tid))
	}
	if st := strings.TrimSpace(q.Status); st != "" {
		query = query.Where(crontaskrun.StatusEQ(st))
	}
	runs, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(runs))
	seen := map[string]struct{}{}
	for _, run := range runs {
		if _, ok := seen[run.TaskID]; ok {
			continue
		}
		seen[run.TaskID] = struct{}{}
		ids = append(ids, run.TaskID)
	}
	tasks, err := r.data.entClient.CronTask.Query().
		Where(crontask.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	nameByID := make(map[string]string, len(tasks))
	for _, t := range tasks {
		nameByID[t.ID] = t.Name
	}
	out := make([]biz.CronTaskRun, 0, len(runs))
	for _, run := range runs {
		name := nameByID[run.TaskID]
		out = append(out, entToBizCronTaskRun(run, name))
	}
	return out, nil
}

func (r *cronRepo) InsertCronTaskRun(ctx context.Context, in biz.CronTaskRunInput) error {
	_, err := r.data.entClient.CronTaskRun.Create().
		SetID(in.ID).
		SetTaskID(in.TaskID).
		SetStatus(in.Status).
		SetStartedAt(in.StartedAt).
		SetFinishedAt("").
		SetOutputJSON(in.OutputJSON).
		SetErrorMessage("").
		SetCreatedAt(in.CreatedAt).
		Save(ctx)
	return err
}

func (r *cronRepo) UpdateCronTaskRun(ctx context.Context, id, status, finishedAt, outputJSON, errorMessage string) error {
	return r.data.entClient.CronTaskRun.UpdateOneID(id).
		SetStatus(status).
		SetFinishedAt(finishedAt).
		SetOutputJSON(outputJSON).
		SetErrorMessage(errorMessage).
		Exec(ctx)
}
