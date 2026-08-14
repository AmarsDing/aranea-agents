package evaluation

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// P1-3: judge-vs-human divergence statistics — the entry point of judge
// calibration (industry direction T6). Human annotations and llm_as_judge
// scores already coexist in eval_case_results; this aggregates their
// agreement so judge failure modes (too lenient / too strict) become visible.

const (
	// DefaultJudgePassThreshold mirrors the framework's llm_as_judge pass
	// cutoff (framework_metrics.go registers the judge with threshold 0.5).
	DefaultJudgePassThreshold = 0.5
	// LLMAsJudgeScoresKey is the scores_json key written by the runner's
	// applyMetricResult when the judge actually scored a case. Its presence
	// distinguishes a real judge score of 0 from the column default 0
	// (legacy path never computes llm_as_judge).
	LLMAsJudgeScoresKey = "llm_as_judge"

	// DivergenceFalsePass marks judge-pass/human-fail rows (judge too lenient).
	DivergenceFalsePass = "false_pass"
	// DivergenceFalseFail marks judge-fail/human-pass rows (judge too strict).
	DivergenceFalseFail = "false_fail"

	judgeDivergenceDefaultLimit = 50
)

// JudgeAnnotatedResult extends a CaseResult with the case text, so divergence
// listings can show what was asked without a second query.
type JudgeAnnotatedResult struct {
	CaseResult
	Input          string
	ExpectedOutput string
}

// JudgeDivergenceCase is one annotated result where judge and human disagree.
type JudgeDivergenceCase struct {
	ResultID       string
	RunID          string
	CaseID         string
	Input          string
	ExpectedOutput string
	ActualOutput   string
	LLMJudgeScore  float32
	HumanPass      bool
	HumanComment   string
	Kind           string // DivergenceFalsePass | DivergenceFalseFail
	CreatedAt      string
}

// JudgeDivergence aggregates judge/human agreement over a dataset's runs.
type JudgeDivergence struct {
	Threshold      float32
	AnnotatedTotal int // annotated results where the judge actually ran
	AgreeCount     int
	DivergeCount   int
	AgreementRate  float32
	FalsePassCount int
	FalseFailCount int
	Cases          []JudgeDivergenceCase
}

// GetJudgeDivergence computes judge/human agreement for a dataset, optionally
// restricted to one agent's runs. Only results whose scores_json carries the
// llm_as_judge key participate — that is the sole reliable signal that the
// judge ran (the llm_judge_score column defaults to 0 otherwise).
func (u *Usecase) GetJudgeDivergence(ctx context.Context, datasetID, agentID string, threshold float32, limit int) (JudgeDivergence, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return JudgeDivergence{}, apierror.BadRequest("EVAL", "dataset_id is required")
	}
	if threshold <= 0 {
		threshold = DefaultJudgePassThreshold
	}
	if limit <= 0 {
		limit = judgeDivergenceDefaultLimit
	}
	rows, err := u.results.ListJudgeAnnotatedResults(ctx, datasetID, agentID)
	if err != nil {
		return JudgeDivergence{}, err
	}

	out := JudgeDivergence{Threshold: threshold, Cases: []JudgeDivergenceCase{}}
	for _, row := range rows {
		if row.HumanPass == nil {
			continue // repo filters human_pass IS NOT NULL; defensive
		}
		score, judged := ParseScores(row.ScoresJSON)[LLMAsJudgeScoresKey]
		if !judged {
			continue
		}
		out.AnnotatedTotal++
		judgePass := score >= threshold
		humanPass := *row.HumanPass
		if judgePass == humanPass {
			out.AgreeCount++
			continue
		}
		out.DivergeCount++
		kind := DivergenceFalseFail
		if judgePass {
			out.FalsePassCount++
			kind = DivergenceFalsePass
		} else {
			out.FalseFailCount++
		}
		// Counts cover the full divergent set; the case list is capped.
		if len(out.Cases) < limit {
			out.Cases = append(out.Cases, JudgeDivergenceCase{
				ResultID:       row.ID,
				RunID:          row.RunID,
				CaseID:         row.CaseID,
				Input:          row.Input,
				ExpectedOutput: row.ExpectedOutput,
				ActualOutput:   row.ActualOutput,
				LLMJudgeScore:  score,
				HumanPass:      humanPass,
				HumanComment:   row.HumanComment,
				Kind:           kind,
				CreatedAt:      row.CreatedAt,
			})
		}
	}
	if out.AnnotatedTotal > 0 {
		out.AgreementRate = float32(out.AgreeCount) / float32(out.AnnotatedTotal)
	}
	return out, nil
}
