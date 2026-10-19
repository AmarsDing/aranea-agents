package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"context"
	"database/sql"
)

func scanEvalCaseResult(row interface {
	Scan(dest ...any) error
}) (biz.EvalCaseResult, error) {
	var res biz.EvalCaseResult
	var em, cm int
	var humanPass sql.NullInt64
	var humanScore sql.NullFloat64
	if err := row.Scan(&res.ID, &res.RunID, &res.CaseID, &res.ActualOutput, &em, &cm,
		&res.LLMJudgeScore, &res.ToolCallAccuracy, &res.ErrorMessage, &res.CreatedAt,
		&humanPass, &humanScore, &res.HumanComment, &res.AnnotatedAt, &res.AnnotatedBy, &res.ScoresJSON,
		&res.Input, &res.SessionID, &res.TraceRunID); err != nil {
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
		 (id,run_id,case_id,actual_output,exact_match,contains_match,llm_judge_score,tool_call_accuracy,error_message,created_at,scores_json,session_id,trace_run_id)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		res.ID, res.RunID, res.CaseID, res.ActualOutput, em, cm,
		res.LLMJudgeScore, res.ToolCallAccuracy, res.ErrorMessage, res.CreatedAt,
		normalizeEvalScoresJSON(res.ScoresJSON), res.SessionID, res.TraceRunID)
	return entErrToBizErr(err, "EVAL")
}

func (r *evalRepo) ListCaseResults(ctx context.Context, runID string, limit, offset int) ([]biz.EvalCaseResult, int, error) {
	var total int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(*) FROM eval_case_results WHERE run_id=?`), []any{runID}, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "EVAL")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(evalCaseResultSelect+` WHERE r.run_id=? ORDER BY r.created_at LIMIT ? OFFSET ?`),
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(evalCaseResultSelect+` WHERE r.run_id=? AND r.id=?`), runID, resultID)
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
