package data

import (
	"context"
	"database/sql"

	"aranea-agents/internal/biz"
)

// P1-3: judge calibration query — annotated case results joined with run
// scope (dataset/agent) and case text. The judge-ran signal (scores_json
// llm_as_judge key) is evaluated in the biz layer to keep this query
// dialect-agnostic (no JSON functions).

func (r *evalRepo) ListJudgeAnnotatedResults(ctx context.Context, datasetID, agentID string) ([]biz.EvalJudgeAnnotatedResult, error) {
	wsClause, wsArgs := evalRunsWorkspaceFilter(ctx)
	q := `SELECT r.id,r.run_id,r.case_id,r.actual_output,r.exact_match,r.contains_match,
		r.llm_judge_score,r.tool_call_accuracy,r.error_message,r.created_at,
		r.human_pass,r.human_score,r.human_comment,r.annotated_at,r.annotated_by,r.scores_json,
		c.input,c.expected_output
		FROM eval_case_results r
		JOIN eval_runs runs ON runs.id = r.run_id
		JOIN eval_cases c ON c.id = r.case_id
		WHERE runs.dataset_id = ? AND r.human_pass IS NOT NULL AND (? = '' OR runs.agent_id = ?)` +
		wsClause + ` ORDER BY r.created_at DESC`
	args := append([]any{datasetID, agentID, agentID}, wsArgs...)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "EVAL")
	}
	defer rows.Close()
	var out []biz.EvalJudgeAnnotatedResult
	for rows.Next() {
		var res biz.EvalJudgeAnnotatedResult
		var em, cm int
		var humanPass sql.NullInt64
		var humanScore sql.NullFloat64
		if err := rows.Scan(&res.ID, &res.RunID, &res.CaseID, &res.ActualOutput, &em, &cm,
			&res.LLMJudgeScore, &res.ToolCallAccuracy, &res.ErrorMessage, &res.CreatedAt,
			&humanPass, &humanScore, &res.HumanComment, &res.AnnotatedAt, &res.AnnotatedBy, &res.ScoresJSON,
			&res.Input, &res.ExpectedOutput); err != nil {
			return nil, entErrToBizErr(err, "EVAL")
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
		out = append(out, res)
	}
	return out, entErrToBizErr(rows.Err(), "EVAL")
}
