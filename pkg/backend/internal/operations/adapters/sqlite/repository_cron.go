package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"arenea/backend/internal/domain"
)

// ListCronTaskRuns 返回定时任务最近执行历史。LEFT JOIN 补全任务名，已删任务仍显示最后已知名称而非空串。
func (s *Store) ListCronTaskRuns(query domain.CronTaskRunQuery) ([]domain.CronTaskRun, error) {
	conditions := []string{"1=1"}
	args := []any{}
	if id := strings.TrimSpace(query.TaskID); id != "" {
		conditions = append(conditions, "ctr.task_id = ?")
		args = append(args, id)
	}
	if status := strings.TrimSpace(query.Status); status != "" {
		conditions = append(conditions, "ctr.status = ?")
		args = append(args, status)
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	sqlText := `SELECT ctr.id, ctr.task_id, COALESCE(ct.name, ''), ctr.status,
		   ctr.started_at, ctr.finished_at, ctr.output_json, ctr.error_message, ctr.created_at
		FROM cron_task_run ctr
		LEFT JOIN cron_task ct ON ct.id = ctr.task_id
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY ctr.created_at DESC
		LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []domain.CronTaskRun{}
	for rows.Next() {
		run, scanErr := scanCronTaskRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, run)
	}
	return items, rows.Err()
}

// AddCronTaskRun 插入一条 cron 执行记录。
func (s *Store) AddCronTaskRun(run domain.CronTaskRun) (domain.CronTaskRun, error) {
	if run.ID == "" || run.TaskID == "" {
		return domain.CronTaskRun{}, errors.New("missing required fields")
	}
	if run.Status == "" {
		run.Status = "pending"
	}
	now := nowISO()
	if run.CreatedAt == "" {
		run.CreatedAt = now
	}
	if run.StartedAt == "" {
		run.StartedAt = now
	}
	_, err := s.db.Exec(
		`INSERT INTO cron_task_run(id, task_id, status, started_at, finished_at, output_json, error_message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.TaskID, run.Status, run.StartedAt, run.FinishedAt, run.OutputJSON, run.ErrorMessage, run.CreatedAt,
	)
	return run, err
}

// UpdateCronTaskRun 更新执行记录行。
func (s *Store) UpdateCronTaskRun(run domain.CronTaskRun) (domain.CronTaskRun, error) {
	if run.ID == "" {
		return domain.CronTaskRun{}, errors.New("missing required fields")
	}
	_, err := s.db.Exec(
		`UPDATE cron_task_run SET status = ?, started_at = ?, finished_at = ?, output_json = ?, error_message = ? WHERE id = ?`,
		run.Status, run.StartedAt, run.FinishedAt, run.OutputJSON, run.ErrorMessage, run.ID,
	)
	return run, err
}

func scanCronTaskRun(sq scanner) (domain.CronTaskRun, error) {
	var run domain.CronTaskRun
	var started, finished sql.NullString
	if err := sq.Scan(
		&run.ID, &run.TaskID, &run.TaskName, &run.Status,
		&started, &finished, &run.OutputJSON, &run.ErrorMessage, &run.CreatedAt,
	); err != nil {
		return domain.CronTaskRun{}, err
	}
	run.StartedAt = started.String
	run.FinishedAt = finished.String
	var output struct {
		Trigger string `json:"trigger"`
		RunID   string `json:"run_id"`
	}
	outputJSON := run.OutputJSON
	if strings.TrimSpace(outputJSON) == "" {
		outputJSON = "{}"
	}
	if json.Unmarshal([]byte(outputJSON), &output) == nil {
		run.Trigger = output.Trigger
		run.RunID = output.RunID
	}
	if run.Trigger == "" {
		run.Trigger = "schedule"
	}
	return run, nil
}
