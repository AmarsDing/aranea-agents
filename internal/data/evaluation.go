package data

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
)

// evalRepo implements biz.EvalRepo using raw SQL against SQLite or PostgreSQL.
type evalRepo struct {
	db *sql.DB
}

// NewEvalRepo returns a biz.EvalRepo backed by the provided *sql.DB.
func NewEvalRepo(db *sql.DB) biz.EvalRepo {
	return &evalRepo{db: db}
}

// EnsureEvalSchema creates the evaluation tables when they do not exist.
func EnsureEvalSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS eval_datasets (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			case_count  INTEGER NOT NULL DEFAULT 0,
			workspace   TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS eval_cases (
			id              TEXT PRIMARY KEY,
			dataset_id      TEXT NOT NULL,
			input           TEXT NOT NULL,
			expected_output TEXT NOT NULL DEFAULT '',
			metadata_json   TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE TABLE IF NOT EXISTS eval_runs (
			id                   TEXT PRIMARY KEY,
			dataset_id           TEXT NOT NULL,
			agent_id             TEXT NOT NULL,
			status               TEXT NOT NULL DEFAULT 'pending',
			total_cases          INTEGER NOT NULL DEFAULT 0,
			completed_cases      INTEGER NOT NULL DEFAULT 0,
			exact_match_score    REAL NOT NULL DEFAULT 0,
			contains_match_score REAL NOT NULL DEFAULT 0,
			llm_judge_score      REAL NOT NULL DEFAULT 0,
			tool_call_accuracy   REAL NOT NULL DEFAULT 0,
			error_message        TEXT NOT NULL DEFAULT '',
			started_at           TEXT NOT NULL DEFAULT '',
			finished_at          TEXT NOT NULL DEFAULT '',
			created_at           TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS eval_case_results (
			id                TEXT PRIMARY KEY,
			run_id            TEXT NOT NULL,
			case_id           TEXT NOT NULL,
			actual_output     TEXT NOT NULL DEFAULT '',
			exact_match       INTEGER NOT NULL DEFAULT 0,
			contains_match    INTEGER NOT NULL DEFAULT 0,
			llm_judge_score   REAL NOT NULL DEFAULT 0,
			tool_call_accuracy REAL NOT NULL DEFAULT 0,
			error_message     TEXT NOT NULL DEFAULT '',
			created_at        TEXT NOT NULL,
			human_pass        INTEGER,
			human_score       REAL,
			human_comment     TEXT NOT NULL DEFAULT '',
			annotated_at      TEXT NOT NULL DEFAULT '',
			annotated_by      TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("eval schema: %w", err)
		}
	}
	migrations := []string{
		`ALTER TABLE eval_case_results ADD COLUMN human_pass INTEGER`,
		`ALTER TABLE eval_case_results ADD COLUMN human_score REAL`,
		`ALTER TABLE eval_case_results ADD COLUMN human_comment TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE eval_case_results ADD COLUMN annotated_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE eval_case_results ADD COLUMN annotated_by TEXT NOT NULL DEFAULT ''`,
	}
	for _, s := range migrations {
		_, _ = db.ExecContext(ctx, s) // best-effort for existing DBs
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// --- Dataset ---

func (r *evalRepo) CreateDataset(ctx context.Context, d biz.EvalDataset) (biz.EvalDataset, error) {
	t := now()
	d.CreatedAt = t
	d.UpdatedAt = t
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO eval_datasets (id,name,description,case_count,workspace,created_at,updated_at)
		 VALUES (?,?,?,0,?,?,?)`,
		d.ID, d.Name, d.Description, d.Workspace, t, t)
	return d, err
}

func (r *evalRepo) GetDataset(ctx context.Context, id string) (biz.EvalDataset, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id,name,description,case_count,workspace,created_at,updated_at FROM eval_datasets WHERE id=?`, id)
	var d biz.EvalDataset
	if err := row.Scan(&d.ID, &d.Name, &d.Description, &d.CaseCount, &d.Workspace, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return biz.EvalDataset{}, err
	}
	return d, nil
}

func (r *evalRepo) ListDatasets(ctx context.Context, workspace string, limit, offset int) ([]biz.EvalDataset, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM eval_datasets WHERE workspace=? OR ?=''`, workspace, workspace).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,name,description,case_count,workspace,created_at,updated_at
		 FROM eval_datasets WHERE workspace=? OR ?='' ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		workspace, workspace, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []biz.EvalDataset
	for rows.Next() {
		var d biz.EvalDataset
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.CaseCount, &d.Workspace, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

func (r *evalRepo) DeleteDataset(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM eval_datasets WHERE id=?`, id)
	return err
}

func (r *evalRepo) UpdateDatasetCaseCount(ctx context.Context, id string, delta int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE eval_datasets SET case_count=case_count+?, updated_at=? WHERE id=?`, delta, now(), id)
	return err
}

// --- Cases ---

func (r *evalRepo) InsertCases(ctx context.Context, cases []biz.EvalCase) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, c := range cases {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO eval_cases (id,dataset_id,input,expected_output,metadata_json) VALUES (?,?,?,?,?)`,
			c.ID, c.DatasetID, c.Input, c.ExpectedOutput, c.MetadataJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *evalRepo) ListCases(ctx context.Context, datasetID string) ([]biz.EvalCase, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,dataset_id,input,expected_output,metadata_json FROM eval_cases WHERE dataset_id=?`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.EvalCase
	for rows.Next() {
		var c biz.EvalCase
		if err := rows.Scan(&c.ID, &c.DatasetID, &c.Input, &c.ExpectedOutput, &c.MetadataJSON); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- Runs ---

func (r *evalRepo) CreateRun(ctx context.Context, rn biz.EvalRun) (biz.EvalRun, error) {
	rn.CreatedAt = now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO eval_runs
		 (id,dataset_id,agent_id,status,total_cases,completed_cases,
		  exact_match_score,contains_match_score,llm_judge_score,tool_call_accuracy,
		  error_message,started_at,finished_at,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		rn.ID, rn.DatasetID, rn.AgentID, rn.Status, rn.TotalCases, rn.CompletedCases,
		rn.ExactMatchScore, rn.ContainsMatchScore, rn.LLMJudgeScore, rn.ToolCallAccuracy,
		rn.ErrorMessage, rn.StartedAt, rn.FinishedAt, rn.CreatedAt)
	return rn, err
}

func (r *evalRepo) GetRun(ctx context.Context, id string) (biz.EvalRun, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id,dataset_id,agent_id,status,total_cases,completed_cases,
		        exact_match_score,contains_match_score,llm_judge_score,tool_call_accuracy,
		        error_message,started_at,finished_at,created_at
		 FROM eval_runs WHERE id=?`, id)
	var rn biz.EvalRun
	err := row.Scan(&rn.ID, &rn.DatasetID, &rn.AgentID, &rn.Status, &rn.TotalCases, &rn.CompletedCases,
		&rn.ExactMatchScore, &rn.ContainsMatchScore, &rn.LLMJudgeScore, &rn.ToolCallAccuracy,
		&rn.ErrorMessage, &rn.StartedAt, &rn.FinishedAt, &rn.CreatedAt)
	return rn, err
}

func (r *evalRepo) UpdateRun(ctx context.Context, rn biz.EvalRun) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE eval_runs SET status=?,total_cases=?,completed_cases=?,
		        exact_match_score=?,contains_match_score=?,llm_judge_score=?,tool_call_accuracy=?,
		        error_message=?,started_at=?,finished_at=?
		 WHERE id=?`,
		rn.Status, rn.TotalCases, rn.CompletedCases,
		rn.ExactMatchScore, rn.ContainsMatchScore, rn.LLMJudgeScore, rn.ToolCallAccuracy,
		rn.ErrorMessage, rn.StartedAt, rn.FinishedAt, rn.ID)
	return err
}

func (r *evalRepo) ListRuns(ctx context.Context, datasetID, agentID string, limit, offset int) ([]biz.EvalRun, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM eval_runs WHERE (dataset_id=? OR ?='') AND (agent_id=? OR ?='')`,
		datasetID, datasetID, agentID, agentID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,dataset_id,agent_id,status,total_cases,completed_cases,
		        exact_match_score,contains_match_score,llm_judge_score,tool_call_accuracy,
		        error_message,started_at,finished_at,created_at
		 FROM eval_runs WHERE (dataset_id=? OR ?='') AND (agent_id=? OR ?='')
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		datasetID, datasetID, agentID, agentID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []biz.EvalRun
	for rows.Next() {
		var rn biz.EvalRun
		if err := rows.Scan(&rn.ID, &rn.DatasetID, &rn.AgentID, &rn.Status, &rn.TotalCases, &rn.CompletedCases,
			&rn.ExactMatchScore, &rn.ContainsMatchScore, &rn.LLMJudgeScore, &rn.ToolCallAccuracy,
			&rn.ErrorMessage, &rn.StartedAt, &rn.FinishedAt, &rn.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, rn)
	}
	return out, total, rows.Err()
}

// --- Case Results ---

const evalCaseResultSelect = `SELECT id,run_id,case_id,actual_output,exact_match,contains_match,llm_judge_score,tool_call_accuracy,error_message,created_at,
	human_pass,human_score,human_comment,annotated_at,annotated_by
	FROM eval_case_results`

func scanEvalCaseResult(row interface {
	Scan(dest ...any) error
}) (biz.EvalCaseResult, error) {
	var res biz.EvalCaseResult
	var em, cm int
	var humanPass sql.NullInt64
	var humanScore sql.NullFloat64
	if err := row.Scan(&res.ID, &res.RunID, &res.CaseID, &res.ActualOutput, &em, &cm,
		&res.LLMJudgeScore, &res.ToolCallAccuracy, &res.ErrorMessage, &res.CreatedAt,
		&humanPass, &humanScore, &res.HumanComment, &res.AnnotatedAt, &res.AnnotatedBy); err != nil {
		return biz.EvalCaseResult{}, err
	}
	res.ExactMatch = em == 1
	res.ContainsMatch = cm == 1
	if humanPass.Valid {
		v := humanPass.Int64 == 1
		res.HumanPass = &v
	}
	if humanScore.Valid {
		v := float32(humanScore.Float64)
		res.HumanScore = &v
	}
	return res, nil
}

func (r *evalRepo) InsertCaseResult(ctx context.Context, res biz.EvalCaseResult) error {
	res.CreatedAt = now()
	em := 0
	if res.ExactMatch {
		em = 1
	}
	cm := 0
	if res.ContainsMatch {
		cm = 1
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO eval_case_results
		 (id,run_id,case_id,actual_output,exact_match,contains_match,llm_judge_score,tool_call_accuracy,error_message,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		res.ID, res.RunID, res.CaseID, res.ActualOutput, em, cm,
		res.LLMJudgeScore, res.ToolCallAccuracy, res.ErrorMessage, res.CreatedAt)
	return err
}

func (r *evalRepo) ListCaseResults(ctx context.Context, runID string, limit, offset int) ([]biz.EvalCaseResult, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM eval_case_results WHERE run_id=?`, runID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx,
		evalCaseResultSelect+` WHERE run_id=? ORDER BY created_at LIMIT ? OFFSET ?`,
		runID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []biz.EvalCaseResult
	for rows.Next() {
		res, err := scanEvalCaseResult(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, res)
	}
	return out, total, rows.Err()
}

func (r *evalRepo) GetCaseResult(ctx context.Context, runID, resultID string) (biz.EvalCaseResult, error) {
	row := r.db.QueryRowContext(ctx, evalCaseResultSelect+` WHERE run_id=? AND id=?`, runID, resultID)
	res, err := scanEvalCaseResult(row)
	if err == sql.ErrNoRows {
		return biz.EvalCaseResult{}, sql.ErrNoRows
	}
	return res, err
}

func (r *evalRepo) UpdateCaseResultAnnotation(ctx context.Context, runID, resultID string, patch biz.EvalCaseResultAnnotation) (biz.EvalCaseResult, error) {
	cur, err := r.GetCaseResult(ctx, runID, resultID)
	if err != nil {
		return biz.EvalCaseResult{}, err
	}
	if patch.HumanPass != nil {
		cur.HumanPass = patch.HumanPass
	}
	if patch.HumanScore != nil {
		cur.HumanScore = patch.HumanScore
	}
	if patch.HumanComment != nil {
		cur.HumanComment = *patch.HumanComment
	}
	cur.AnnotatedAt = now()
	cur.AnnotatedBy = patch.AnnotatedBy

	var humanPass any
	if cur.HumanPass != nil {
		if *cur.HumanPass {
			humanPass = 1
		} else {
			humanPass = 0
		}
	} else {
		humanPass = nil
	}
	var humanScore any
	if cur.HumanScore != nil {
		humanScore = *cur.HumanScore
	} else {
		humanScore = nil
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE eval_case_results SET human_pass=?, human_score=?, human_comment=?, annotated_at=?, annotated_by=? WHERE run_id=? AND id=?`,
		humanPass, humanScore, cur.HumanComment, cur.AnnotatedAt, cur.AnnotatedBy, runID, resultID)
	if err != nil {
		return biz.EvalCaseResult{}, err
	}
	return cur, nil
}
