package evaluation

import (
	"aranea-agents/pkg/apierror"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// TouchRunLease refreshes the in-flight heartbeat so a live executor is
// not swept by another instance's startup cleaner.
func (u *Usecase) TouchRunLease(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" || u == nil || u.runs == nil {
		return nil
	}
	run, err := u.runs.GetRun(ctx, id)
	if err != nil {
		return err
	}
	if run.Status != RunStatusPending && run.Status != RunStatusRunning {
		return nil
	}
	run.LeaseUntil = NextLeaseUntil()
	return u.runs.UpdateRun(ctx, run)
}

// DeleteRun removes a run and its case results.
func (u *Usecase) DeleteRun(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("EVAL", "id is required")
	}
	return u.runs.DeleteRun(ctx, id)
}

// ListCaseResults returns per-case results for a run.
func (u *Usecase) ListCaseResults(ctx context.Context, runID string, limit, offset int) ([]CaseResult, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return u.results.ListCaseResults(ctx, runID, limit, offset)
}

// InsertCaseResult persists one case result.
func (u *Usecase) InsertCaseResult(ctx context.Context, r CaseResult) error {
	return u.results.InsertCaseResult(ctx, r)
}

// AnnotateCaseResult updates human review fields for one case result.
func (u *Usecase) AnnotateCaseResult(ctx context.Context, runID, resultID string, patch CaseResultAnnotation) (CaseResult, error) {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(resultID) == "" {
		return CaseResult{}, apierror.BadRequest("EVAL", "run_id and result_id are required")
	}
	if strings.TrimSpace(patch.AnnotatedBy) == "" {
		patch.AnnotatedBy = "system"
	}
	return u.results.UpdateCaseResultAnnotation(ctx, runID, resultID, patch)
}

// ListCases returns all cases for a dataset.
func (u *Usecase) ListCases(ctx context.Context, datasetID string) ([]Case, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, apierror.BadRequest("EVAL", "dataset_id is required")
	}
	return u.cases.ListCases(ctx, datasetID)
}

// UpdateCase updates one case's input / expected output / metadata.
func (u *Usecase) UpdateCase(ctx context.Context, c Case) (Case, error) {
	c.DatasetID = strings.TrimSpace(c.DatasetID)
	c.ID = strings.TrimSpace(c.ID)
	c.Input = strings.TrimSpace(c.Input)
	if c.DatasetID == "" || c.ID == "" {
		return Case{}, apierror.BadRequest("EVAL", "dataset_id and id are required")
	}
	if c.Input == "" {
		return Case{}, apierror.BadRequest("EVAL", "input is required")
	}
	out, err := u.cases.UpdateCase(ctx, c)
	if err != nil {
		return Case{}, err
	}
	_, _ = u.SnapshotDataset(ctx, c.DatasetID)
	return out, nil
}

// DeleteCase removes one case and decrements dataset.case_count.
func (u *Usecase) DeleteCase(ctx context.Context, datasetID, caseID string) error {
	datasetID = strings.TrimSpace(datasetID)
	caseID = strings.TrimSpace(caseID)
	if datasetID == "" || caseID == "" {
		return apierror.BadRequest("EVAL", "dataset_id and id are required")
	}
	if err := u.cases.DeleteCase(ctx, datasetID, caseID); err != nil {
		return err
	}
	_, _ = u.SnapshotDataset(ctx, datasetID)
	return nil
}

// inFlightScanLimit bounds the recent-run scan for FindInFlightRun.
const inFlightScanLimit = 20

// FindInFlightRun reports the newest pending/running run for dataset+agent.
// Used by manual RunEvaluation and AfterTurn to share one in-flight lock.
func (u *Usecase) FindInFlightRun(ctx context.Context, datasetID, agentID string) (Run, bool, error) {
	datasetID = strings.TrimSpace(datasetID)
	agentID = strings.TrimSpace(agentID)
	if datasetID == "" || agentID == "" {
		return Run{}, false, nil
	}
	runs, _, err := u.runs.ListRuns(ctx, datasetID, agentID, inFlightScanLimit, 0)
	if err != nil {
		return Run{}, false, err
	}
	for _, r := range runs {
		if r.Status == RunStatusPending || r.Status == RunStatusRunning {
			return r, true, nil
		}
	}
	return Run{}, false, nil
}

// FindInFlightExperimentCell reports an in-flight run for one matrix cell.
// Same agent may have several cells (model/prompt); the lock key is variant_label.
func (u *Usecase) FindInFlightExperimentCell(ctx context.Context, datasetID, agentID, variantLabel string) (Run, bool, error) {
	variantLabel = strings.TrimSpace(variantLabel)
	if variantLabel == "" {
		return u.FindInFlightRun(ctx, datasetID, agentID)
	}
	datasetID = strings.TrimSpace(datasetID)
	agentID = strings.TrimSpace(agentID)
	if datasetID == "" || agentID == "" {
		return Run{}, false, nil
	}
	runs, _, err := u.runs.ListRuns(ctx, datasetID, agentID, inFlightScanLimit, 0)
	if err != nil {
		return Run{}, false, err
	}
	for _, r := range runs {
		if (r.Status == RunStatusPending || r.Status == RunStatusRunning) && strings.TrimSpace(r.VariantLabel) == variantLabel {
			return r, true, nil
		}
	}
	return Run{}, false, nil
}

// CancelRun marks a pending/running run cancelled. Callers should also
// cancel the Runner context so the executor stops promptly.
func (u *Usecase) CancelRun(ctx context.Context, id string) (Run, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Run{}, apierror.BadRequest("EVAL", "id is required")
	}
	run, err := u.runs.GetRun(ctx, id)
	if err != nil {
		return Run{}, err
	}
	if run.Status != RunStatusPending && run.Status != RunStatusRunning {
		return Run{}, apierror.Conflict("EVAL", "run is not cancellable")
	}
	run.Status = RunStatusCancelled
	run.ErrorMessage = "cancelled"
	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if err := u.runs.UpdateRun(ctx, run); err != nil {
		return Run{}, err
	}
	return run, nil
}

// GetAgentEvalTrend returns recent completed runs for trend charts.
func (u *Usecase) GetAgentEvalTrend(ctx context.Context, agentID, datasetID string, limit int) ([]TrendPoint, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, apierror.BadRequest("EVAL", "agent_id is required")
	}
	if limit <= 0 {
		limit = 30
	}
	return u.runQueries.ListTrendPoints(ctx, agentID, datasetID, limit)
}

// CompareEvalRuns loads runs by ID and computes metric deltas vs the first run (baseline).
func (u *Usecase) CompareEvalRuns(ctx context.Context, runIDs []string) ([]RunComparison, error) {
	if len(runIDs) < 2 {
		return nil, apierror.BadRequest("EVAL", "at least two run_ids are required")
	}
	runs, err := u.runQueries.GetRunsByIDs(ctx, runIDs)
	if err != nil {
		return nil, err
	}
	if len(runs) < 2 {
		return nil, apierror.BadRequest("EVAL", "could not load at least two runs")
	}

	sort.SliceStable(runs, func(i, j int) bool { return runs[i].CreatedAt < runs[j].CreatedAt })
	base := runs[0]
	out := make([]RunComparison, 0, len(runs))
	for _, r := range runs {
		out = append(out, RunComparison{
			RunID:              r.ID,
			AgentID:            r.AgentID,
			DatasetID:          r.DatasetID,
			CreatedAt:          r.CreatedAt,
			DatasetHash:        r.DatasetHash,
			DatasetVersionID:   r.DatasetVersionID,
			DatasetVersion:     r.DatasetVersion,
			ExperimentID:       r.ExperimentID,
			VariantLabel:       r.VariantLabel,
			Model:              r.Model,
			Prompt:             r.Prompt,
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

// Scores holds arbitrary metric key → score mappings.
type Scores map[string]float32

// ParseScores unmarshals scores_json; invalid input yields empty map.
func ParseScores(raw string) Scores {
	if raw == "" || raw == "{}" {
		return Scores{}
	}
	var m Scores
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return Scores{}
	}
	return m
}

// MarshalScores serializes scores to JSON object string.
func MarshalScores(m Scores) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// LLMSetting holds platform defaults for evaluation UserSim and LLM-as-Judge.
type LLMSetting struct {
	SimProvider   string
	SimModel      string
	JudgeProvider string
	JudgeModel    string
}

// SimConfigured returns true if sim provider and model are set.
func (s LLMSetting) SimConfigured() bool {
	return strings.TrimSpace(s.SimProvider) != "" && strings.TrimSpace(s.SimModel) != ""
}

// JudgeConfigured returns true if judge provider and model are set.
func (s LLMSetting) JudgeConfigured() bool {
	return strings.TrimSpace(s.JudgeProvider) != "" && strings.TrimSpace(s.JudgeModel) != ""
}

// ApplyLLMPatch merges an update onto current eval LLM settings.
func ApplyLLMPatch(cur LLMSetting, simProvider, simModel, judgeProvider, judgeModel string) LLMSetting {
	out := cur
	if simProvider != "" || simModel != "" {
		out.SimProvider = strings.TrimSpace(simProvider)
		out.SimModel = strings.TrimSpace(simModel)
	}
	if judgeProvider != "" || judgeModel != "" {
		out.JudgeProvider = strings.TrimSpace(judgeProvider)
		out.JudgeModel = strings.TrimSpace(judgeModel)
	}
	return out
}
