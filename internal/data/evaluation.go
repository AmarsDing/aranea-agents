package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizevaluation "aranea-agents/internal/biz/evaluation"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type evalRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ bizevaluation.Repo = (*evalRepo)(nil)

func NewEvalRepo(data *Data, lg loggateway.Logger) biz.EvalRepo {
	if data == nil || data.RWDB() == nil {
		return nil
	}
	return &evalRepo{data: data, lg: lg}
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
			pass_at_k            REAL NOT NULL DEFAULT 0,
			pass_hat_k           REAL NOT NULL DEFAULT 0,
			trigger_source       TEXT NOT NULL DEFAULT 'manual',
			num_runs             INTEGER NOT NULL DEFAULT 1,
			scores_json          TEXT NOT NULL DEFAULT '{}',
			error_message        TEXT NOT NULL DEFAULT '',
			started_at           TEXT NOT NULL DEFAULT '',
			finished_at          TEXT NOT NULL DEFAULT '',
			workspace_id         TEXT NOT NULL DEFAULT '',
			dataset_hash         TEXT NOT NULL DEFAULT '',
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
			annotated_by      TEXT NOT NULL DEFAULT '',
			scores_json       TEXT NOT NULL DEFAULT '{}'
		)`,
		// P3-3: pairwise human preference between two runs of one dataset.
		`CREATE TABLE IF NOT EXISTS eval_run_preferences (
			id            TEXT PRIMARY KEY,
			dataset_id    TEXT NOT NULL,
			run_id_a      TEXT NOT NULL,
			run_id_b      TEXT NOT NULL,
			winner_run_id TEXT NOT NULL,
			comment       TEXT NOT NULL DEFAULT '',
			created_by    TEXT NOT NULL DEFAULT '',
			created_at    TEXT NOT NULL
		)`,
		// P2-1: publish regression gate config (singleton row id='singleton').
		`CREATE TABLE IF NOT EXISTS eval_gate_config (
			id         TEXT PRIMARY KEY,
			enabled    INTEGER NOT NULL DEFAULT 0,
			agent_id   TEXT NOT NULL DEFAULT '',
			dataset_id TEXT NOT NULL DEFAULT '',
			metric     TEXT NOT NULL DEFAULT 'exact_match',
			min_score  REAL NOT NULL DEFAULT 0,
			max_drop   REAL NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return entErrToBizErr(err, "EVAL")
		}
	}
	migrations := []string{
		`ALTER TABLE eval_case_results ADD COLUMN human_pass INTEGER`,
		`ALTER TABLE eval_case_results ADD COLUMN human_score REAL`,
		`ALTER TABLE eval_case_results ADD COLUMN human_comment TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE eval_case_results ADD COLUMN annotated_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE eval_case_results ADD COLUMN annotated_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE eval_runs ADD COLUMN trigger_source TEXT NOT NULL DEFAULT 'manual'`,
		`ALTER TABLE eval_runs ADD COLUMN num_runs INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE eval_runs ADD COLUMN pass_at_k REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE eval_runs ADD COLUMN pass_hat_k REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE eval_runs ADD COLUMN scores_json TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE eval_case_results ADD COLUMN scores_json TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE eval_runs ADD COLUMN workspace_id TEXT NOT NULL DEFAULT ''`,
		// P3-5: dataset content hash snapshot.
		`ALTER TABLE eval_runs ADD COLUMN dataset_hash TEXT NOT NULL DEFAULT ''`,
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
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO eval_datasets (id,name,description,case_count,workspace,created_at,updated_at)
		 VALUES (?,?,?,0,?,?,?)`),
		d.ID, d.Name, d.Description, d.Workspace, t, t)
	return d, entErrToBizErr(err, "EVAL")
}

func (r *evalRepo) GetDataset(ctx context.Context, id string) (biz.EvalDataset, error) {
	var d biz.EvalDataset
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT id,name,description,case_count,workspace,created_at,updated_at FROM eval_datasets WHERE id=?`), []any{id},
		&d.ID, &d.Name, &d.Description, &d.CaseCount, &d.Workspace, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return biz.EvalDataset{}, entErrToBizErr(err, "EVAL")
	}
	return d, nil
}

func (r *evalRepo) ListDatasets(ctx context.Context, workspace string, limit, offset int) ([]biz.EvalDataset, int, error) {
	// Visibility: caller's own workspace plus shared/legacy rows (workspace='').
	// workspace="" (system/internal lookups such as skill replay name
	// convention) keeps the historical "all rows" semantics.
	var total int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(*) FROM eval_datasets WHERE workspace=? OR workspace='' OR ?=''`), []any{workspace, workspace}, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "EVAL")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT id,name,description,case_count,workspace,created_at,updated_at
		 FROM eval_datasets WHERE workspace=? OR workspace='' OR ?='' ORDER BY created_at DESC LIMIT ? OFFSET ?`),
		workspace, workspace, limit, offset)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	var out []biz.EvalDataset
	for rows.Next() {
		var d biz.EvalDataset
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.CaseCount, &d.Workspace, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, 0, entErrToBizErr(err, "EVAL")
		}
		out = append(out, d)
	}
	return out, total, entErrToBizErr(rows.Err(), "EVAL")
}

func (r *evalRepo) DeleteDataset(ctx context.Context, id string) error {
	d := r.data.Dialect()
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, r.data.RWDB().WriteHandle())
		// Y11: pairwise preferences reference both the dataset and its runs —
		// without this cascade they survive as orphan rows pointing at
		// deleted runs.
		if _, err := e.ExecContext(txCtx,
			d.RenumberPlaceholders(`DELETE FROM eval_run_preferences WHERE dataset_id=?`),
			id); err != nil {
			return err
		}
		if _, err := e.ExecContext(txCtx,
			d.RenumberPlaceholders(`DELETE FROM eval_case_results WHERE run_id IN (SELECT id FROM eval_runs WHERE dataset_id=?)`),
			id); err != nil {
			return err
		}
		if _, err := e.ExecContext(txCtx, d.RenumberPlaceholders(`DELETE FROM eval_runs WHERE dataset_id=?`), id); err != nil {
			return err
		}
		if _, err := e.ExecContext(txCtx, d.RenumberPlaceholders(`DELETE FROM eval_cases WHERE dataset_id=?`), id); err != nil {
			return err
		}
		if _, err := e.ExecContext(txCtx, d.RenumberPlaceholders(`DELETE FROM eval_datasets WHERE id=?`), id); err != nil {
			return err
		}
		return nil
	})
	return entErrToBizErr(err, "EVAL")
}

func (r *evalRepo) UpdateDataset(ctx context.Context, id, name, description string) (biz.EvalDataset, error) {
	t := now()
	if _, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE eval_datasets SET name=?, description=?, updated_at=? WHERE id=?`),
		name, description, t, id); err != nil {
		return biz.EvalDataset{}, entErrToBizErr(err, "EVAL")
	}
	return r.GetDataset(ctx, id)
}

func (r *evalRepo) UpdateDatasetCaseCount(ctx context.Context, id string, delta int) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE eval_datasets SET case_count=case_count+?, updated_at=? WHERE id=?`), delta, now(), id)
	return entErrToBizErr(err, "EVAL")
}

// --- Cases ---

func (r *evalRepo) InsertCases(ctx context.Context, cases []biz.EvalCase) error {
	insertSQL := r.data.Dialect().RenumberPlaceholders(`INSERT INTO eval_cases (id,dataset_id,input,expected_output,metadata_json) VALUES (?,?,?,?,?)`)
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, r.data.RWDB().WriteHandle())
		for _, c := range cases {
			if _, err := e.ExecContext(txCtx, insertSQL,
				c.ID, c.DatasetID, c.Input, c.ExpectedOutput, c.MetadataJSON); err != nil {
				return err
			}
		}
		return nil
	})
	return entErrToBizErr(err, "EVAL")
}

// InsertCasesWithCountUpdate inserts cases and bumps dataset.case_count in a
// single transaction so the two writes cannot diverge. The dataset's case_count
// is bumped by len(cases) regardless of the per-case dataset_id values, because
// UploadCases only ever targets one dataset (validated by the usecase).
func (r *evalRepo) InsertCasesWithCountUpdate(ctx context.Context, datasetID string, cases []biz.EvalCase) error {
	insertSQL := r.data.Dialect().RenumberPlaceholders(`INSERT INTO eval_cases (id,dataset_id,input,expected_output,metadata_json) VALUES (?,?,?,?,?)`)
	updateSQL := r.data.Dialect().RenumberPlaceholders(`UPDATE eval_datasets SET case_count=case_count+?, updated_at=? WHERE id=?`)
	t := now()
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, r.data.RWDB().WriteHandle())
		for _, c := range cases {
			if _, err := e.ExecContext(txCtx, insertSQL,
				c.ID, c.DatasetID, c.Input, c.ExpectedOutput, c.MetadataJSON); err != nil {
				return err
			}
		}
		if _, err := e.ExecContext(txCtx, updateSQL, len(cases), t, datasetID); err != nil {
			return err
		}
		return nil
	})
	return entErrToBizErr(err, "EVAL")
}

func (r *evalRepo) ListCases(ctx context.Context, datasetID string) ([]biz.EvalCase, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT id,dataset_id,input,expected_output,metadata_json FROM eval_cases WHERE dataset_id=?`), datasetID)
	if err != nil {
		return nil, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	var out []biz.EvalCase
	for rows.Next() {
		var c biz.EvalCase
		if err := rows.Scan(&c.ID, &c.DatasetID, &c.Input, &c.ExpectedOutput, &c.MetadataJSON); err != nil {
			return nil, entErrToBizErr(err, "EVAL")
		}
		out = append(out, c)
	}
	return out, entErrToBizErr(rows.Err(), "EVAL")
}

// --- Runs ---

func (r *evalRepo) CreateRun(ctx context.Context, rn biz.EvalRun) (biz.EvalRun, error) {
	rn.CreatedAt = now()
	if strings.TrimSpace(rn.TriggerSource) == "" {
		rn.TriggerSource = "manual"
	}
	if rn.NumRuns <= 0 {
		rn.NumRuns = 1
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO eval_runs
		 (id,dataset_id,agent_id,status,total_cases,completed_cases,
		  exact_match_score,contains_match_score,llm_judge_score,tool_call_accuracy,
		  pass_at_k,pass_hat_k,trigger_source,num_runs,scores_json,
		  error_message,started_at,finished_at,workspace_id,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		rn.ID, rn.DatasetID, rn.AgentID, rn.Status, rn.TotalCases, rn.CompletedCases,
		rn.ExactMatchScore, rn.ContainsMatchScore, rn.LLMJudgeScore, rn.ToolCallAccuracy,
		rn.PassAtK, rn.PassHatK, rn.TriggerSource, rn.NumRuns, normalizeEvalScoresJSON(rn.ScoresJSON),
		rn.ErrorMessage, rn.StartedAt, rn.FinishedAt, rn.WorkspaceID, rn.CreatedAt)
	return rn, entErrToBizErr(err, "EVAL")
}

const evalRunSelect = `SELECT id,dataset_id,agent_id,status,total_cases,completed_cases,
	exact_match_score,contains_match_score,llm_judge_score,tool_call_accuracy,
	pass_at_k,pass_hat_k,trigger_source,num_runs,scores_json,
	error_message,started_at,finished_at,workspace_id,dataset_hash,created_at FROM eval_runs`

func normalizeEvalScoresJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

func scanEvalRun(row interface{ Scan(dest ...any) error }) (biz.EvalRun, error) {
	var rn biz.EvalRun
	err := row.Scan(&rn.ID, &rn.DatasetID, &rn.AgentID, &rn.Status, &rn.TotalCases, &rn.CompletedCases,
		&rn.ExactMatchScore, &rn.ContainsMatchScore, &rn.LLMJudgeScore, &rn.ToolCallAccuracy,
		&rn.PassAtK, &rn.PassHatK, &rn.TriggerSource, &rn.NumRuns, &rn.ScoresJSON,
		&rn.ErrorMessage, &rn.StartedAt, &rn.FinishedAt, &rn.WorkspaceID, &rn.DatasetHash, &rn.CreatedAt)
	return rn, entErrToBizErr(err, "EVAL")
}

func (r *evalRepo) GetRun(ctx context.Context, id string) (biz.EvalRun, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(evalRunSelect+` WHERE id=?`), id)
	if err != nil {
		return biz.EvalRun{}, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.EvalRun{}, apierror.NotFound(apierror.DomainData, "not found")
	}
	return scanEvalRun(rows)
}

func (r *evalRepo) UpdateRun(ctx context.Context, rn biz.EvalRun) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE eval_runs SET status=?,total_cases=?,completed_cases=?,
	        exact_match_score=?,contains_match_score=?,llm_judge_score=?,tool_call_accuracy=?,
	        pass_at_k=?,pass_hat_k=?,scores_json=?,
	        error_message=?,started_at=?,finished_at=?,dataset_hash=?
		 WHERE id=?`),
		rn.Status, rn.TotalCases, rn.CompletedCases,
		rn.ExactMatchScore, rn.ContainsMatchScore, rn.LLMJudgeScore, rn.ToolCallAccuracy,
		rn.PassAtK, rn.PassHatK, normalizeEvalScoresJSON(rn.ScoresJSON),
		rn.ErrorMessage, rn.StartedAt, rn.FinishedAt, rn.DatasetHash, rn.ID)
	return entErrToBizErr(err, "EVAL")
}

// FailStaleRuns sweeps orphan runs (Y10): any row still pending/running with
// created_at before cutoff belonged to a dead process — the async executor
// goroutine died with it. Mark failed so the UI stops showing phantom
// "running" rows and trend queries exclude them.
func (r *evalRepo) FailStaleRuns(ctx context.Context, cutoff time.Time) (int, error) {
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE eval_runs SET status='failed', error_message=?, finished_at=?
		 WHERE status IN ('pending','running') AND created_at < ?`),
		"interrupted: process restarted before run completion", now(), cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, entErrToBizErr(err, "EVAL")
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *evalRepo) DeleteRun(ctx context.Context, id string) error {
	d := r.data.Dialect()
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, r.data.RWDB().WriteHandle())
		// Y11: preferences naming this run as either side must not outlive it.
		if _, err := e.ExecContext(txCtx,
			d.RenumberPlaceholders(`DELETE FROM eval_run_preferences WHERE run_id_a=? OR run_id_b=?`),
			id, id); err != nil {
			return err
		}
		if _, err := e.ExecContext(txCtx, d.RenumberPlaceholders(`DELETE FROM eval_case_results WHERE run_id=?`), id); err != nil {
			return err
		}
		if _, err := e.ExecContext(txCtx, d.RenumberPlaceholders(`DELETE FROM eval_runs WHERE id=?`), id); err != nil {
			return err
		}
		return nil
	})
	return entErrToBizErr(err, "EVAL")
}

// evalRunsWorkspaceFilter returns a SQL fragment (prefixed with " AND") and
// matching args for workspace visibility filtering of eval_runs.
//   - system caller (workspace.IsSystem): no filtering (sees all rows)
//   - default workspace caller: sees own rows plus legacy rows (workspace_id=”)
//   - other tenant callers: strict equality (own rows only)
func evalRunsWorkspaceFilter(ctx context.Context) (string, []any) {
	if workspace.IsSystem(ctx) {
		return "", nil
	}
	callerWS := workspace.IDFromContext(ctx)
	if callerWS == workspace.DefaultWorkspaceID {
		return ` AND workspace_id IN (?, ?)`, []any{callerWS, ""}
	}
	return ` AND workspace_id=?`, []any{callerWS}
}

func (r *evalRepo) ListRuns(ctx context.Context, datasetID, agentID string, limit, offset int) ([]biz.EvalRun, int, error) {
	wsClause, wsArgs := evalRunsWorkspaceFilter(ctx)
	baseWhere := `(dataset_id=? OR ?='') AND (agent_id=? OR ?='')`
	baseArgs := []any{datasetID, datasetID, agentID, agentID}

	var total int
	countArgs := append(baseArgs, wsArgs...)
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(*) FROM eval_runs WHERE `+baseWhere+wsClause),
		countArgs, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "EVAL")
	}
	selectArgs := append(append(baseArgs, wsArgs...), limit, offset)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(evalRunSelect+` WHERE `+baseWhere+wsClause+
			` ORDER BY created_at DESC LIMIT ? OFFSET ?`),
		selectArgs...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	var out []biz.EvalRun
	for rows.Next() {
		rn, err := scanEvalRun(rows)
		if err != nil {
			return nil, 0, entErrToBizErr(err, "EVAL")
		}
		out = append(out, rn)
	}
	return out, total, entErrToBizErr(rows.Err(), "EVAL")
}

func (r *evalRepo) ListTrendPoints(ctx context.Context, agentID, datasetID string, limit int) ([]biz.EvalTrendPoint, error) {
	wsClause, wsArgs := evalRunsWorkspaceFilter(ctx)
	q := `SELECT id,created_at,trigger_source,exact_match_score,contains_match_score,llm_judge_score,tool_call_accuracy,pass_at_k,pass_hat_k
		FROM eval_runs WHERE agent_id=? AND status='completed'`
	args := []any{agentID}
	if strings.TrimSpace(datasetID) != "" {
		q += ` AND dataset_id=?`
		args = append(args, datasetID)
	}
	q += wsClause + ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, wsArgs...)
	args = append(args, limit)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	var out []biz.EvalTrendPoint
	for rows.Next() {
		var p biz.EvalTrendPoint
		if err := rows.Scan(&p.RunID, &p.CreatedAt, &p.TriggerSource,
			&p.ExactMatchScore, &p.ContainsMatchScore, &p.LLMJudgeScore, &p.ToolCallAccuracy,
			&p.PassAtK, &p.PassHatK); err != nil {
			return nil, entErrToBizErr(err, "EVAL")
		}
		out = append(out, p)
	}
	return out, entErrToBizErr(rows.Err(), "EVAL")
}

func (r *evalRepo) GetRunsByIDs(ctx context.Context, ids []string) ([]biz.EvalRun, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := r.data.Dialect().Placeholders(len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	// Cross-workspace IDs are filtered out (IDOR): compare/preference callers
	// only ever see runs visible to their workspace.
	wsClause, wsArgs := evalRunsWorkspaceFilter(ctx)
	args = append(args, wsArgs...)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(evalRunSelect+` WHERE id IN (`+placeholders+`)`+wsClause), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	byID := make(map[string]biz.EvalRun, len(ids))
	for rows.Next() {
		rn, err := scanEvalRun(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "EVAL")
		}
		byID[rn.ID] = rn
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "EVAL")
	}
	out := make([]biz.EvalRun, 0, len(ids))
	for _, id := range ids {
		if rn, ok := byID[id]; ok {
			out = append(out, rn)
		}
	}
	return out, nil
}

// --- Case Results ---

const evalCaseResultSelect = `SELECT id,run_id,case_id,actual_output,exact_match,contains_match,llm_judge_score,tool_call_accuracy,error_message,created_at,
	human_pass,human_score,human_comment,annotated_at,annotated_by,scores_json
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
		&humanPass, &humanScore, &res.HumanComment, &res.AnnotatedAt, &res.AnnotatedBy, &res.ScoresJSON); err != nil {
		return biz.EvalCaseResult{}, entErrToBizErr(err, "EVAL")
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
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO eval_case_results
		 (id,run_id,case_id,actual_output,exact_match,contains_match,llm_judge_score,tool_call_accuracy,error_message,created_at,scores_json)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`),
		res.ID, res.RunID, res.CaseID, res.ActualOutput, em, cm,
		res.LLMJudgeScore, res.ToolCallAccuracy, res.ErrorMessage, res.CreatedAt,
		normalizeEvalScoresJSON(res.ScoresJSON))
	return entErrToBizErr(err, "EVAL")
}

func (r *evalRepo) ListCaseResults(ctx context.Context, runID string, limit, offset int) ([]biz.EvalCaseResult, int, error) {
	var total int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(*) FROM eval_case_results WHERE run_id=?`), []any{runID}, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "EVAL")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(evalCaseResultSelect+` WHERE run_id=? ORDER BY created_at LIMIT ? OFFSET ?`),
		runID, limit, offset)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	var out []biz.EvalCaseResult
	for rows.Next() {
		res, err := scanEvalCaseResult(rows)
		if err != nil {
			return nil, 0, entErrToBizErr(err, "EVAL")
		}
		out = append(out, res)
	}
	return out, total, entErrToBizErr(rows.Err(), "EVAL")
}

func (r *evalRepo) GetCaseResult(ctx context.Context, runID, resultID string) (biz.EvalCaseResult, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(evalCaseResultSelect+` WHERE run_id=? AND id=?`), runID, resultID)
	if err != nil {
		return biz.EvalCaseResult{}, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.EvalCaseResult{}, apierror.NotFound(apierror.DomainData, "not found")
	}
	res, err := scanEvalCaseResult(rows)
	if err != nil {
		return biz.EvalCaseResult{}, err
	}
	return res, nil
}

func (r *evalRepo) UpdateCaseResultAnnotation(ctx context.Context, runID, resultID string, patch biz.EvalCaseResultAnnotation) (biz.EvalCaseResult, error) {
	cur, err := r.GetCaseResult(ctx, runID, resultID)
	if err != nil {
		return biz.EvalCaseResult{}, err
	}
	if patch.ClearHumanPass {
		cur.HumanPass = nil
	} else if patch.HumanPass != nil {
		cur.HumanPass = patch.HumanPass
	}
	if patch.ClearHumanScore {
		cur.HumanScore = nil
	} else if patch.HumanScore != nil {
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

	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE eval_case_results SET human_pass=?, human_score=?, human_comment=?, annotated_at=?, annotated_by=? WHERE run_id=? AND id=?`),
		humanPass, humanScore, cur.HumanComment, cur.AnnotatedAt, cur.AnnotatedBy, runID, resultID)
	if err != nil {
		return biz.EvalCaseResult{}, entErrToBizErr(err, "EVAL")
	}
	return cur, nil
}
