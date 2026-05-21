package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// EvalTrendPoint is one completed run on the agent quality timeline (US-7).
type EvalTrendPoint struct {
	RunID              string
	CreatedAt          string
	TriggerSource      string
	ExactMatchScore    float32
	ContainsMatchScore float32
	LLMJudgeScore      float32
	ToolCallAccuracy   float32
	PassAtK            float32
	PassHatK           float32
}

// EvalRunComparison compares metric scores across runs (A/B or multi-run delta).
type EvalRunComparison struct {
	RunID              string
	AgentID            string
	DatasetID          string
	CreatedAt          string
	ExactMatchScore    float32
	ContainsMatchScore float32
	LLMJudgeScore      float32
	ToolCallAccuracy   float32
	PassAtK            float32
	PassHatK           float32
	DeltaExactMatch    float32
	DeltaContainsMatch float32
	DeltaLLMJudge      float32
	DeltaToolAccuracy  float32
}

// GetAgentEvalTrend returns recent completed runs for trend charts.
func (u *EvalUsecase) GetAgentEvalTrend(ctx context.Context, agentID, datasetID string, limit int) ([]EvalTrendPoint, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.BadRequest("EVAL", "agent_id is required")
	}
	if limit <= 0 {
		limit = 30
	}
	return u.repo.ListTrendPoints(ctx, agentID, datasetID, limit)
}

// CompareEvalRuns loads runs by ID and computes metric deltas vs the first run (baseline).
func (u *EvalUsecase) CompareEvalRuns(ctx context.Context, runIDs []string) ([]EvalRunComparison, error) {
	if len(runIDs) < 2 {
		return nil, errors.BadRequest("EVAL", "at least two run_ids are required")
	}
	runs, err := u.repo.GetRunsByIDs(ctx, runIDs)
	if err != nil {
		return nil, err
	}
	if len(runs) < 2 {
		return nil, errors.BadRequest("EVAL", "could not load at least two runs")
	}
	base := runs[0]
	out := make([]EvalRunComparison, 0, len(runs))
	for _, r := range runs {
		out = append(out, EvalRunComparison{
			RunID:              r.ID,
			AgentID:            r.AgentID,
			DatasetID:          r.DatasetID,
			CreatedAt:          r.CreatedAt,
			ExactMatchScore:    r.ExactMatchScore,
			ContainsMatchScore: r.ContainsMatchScore,
			LLMJudgeScore:      r.LLMJudgeScore,
			ToolCallAccuracy:   r.ToolCallAccuracy,
			PassAtK:            r.PassAtK,
			PassHatK:           r.PassHatK,
			DeltaExactMatch:    r.ExactMatchScore - base.ExactMatchScore,
			DeltaContainsMatch: r.ContainsMatchScore - base.ContainsMatchScore,
			DeltaLLMJudge:      r.LLMJudgeScore - base.LLMJudgeScore,
			DeltaToolAccuracy:  r.ToolCallAccuracy - base.ToolCallAccuracy,
		})
	}
	return out, nil
}
