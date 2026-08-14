package service

import (
	"context"
	"fmt"
	"strings"

	v1 "aranea-agents/api/kratos/evaluation/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// EvaluationService implements kratos evaluation.v1.
type EvaluationService struct {
	v1.UnimplementedEvaluationServiceServer
	uc     *biz.EvalUsecase
	runner *evaluation.Runner
}

// NewEvaluationService constructs an EvaluationService.
func NewEvaluationService(uc *biz.EvalUsecase, runner *evaluation.Runner) *EvaluationService {
	return &EvaluationService{uc: uc, runner: runner}
}

// inFlightScanLimit caps the recent-run scan behind the EVAL-08 in-flight
// dedup check (same order of magnitude as the gate's baseline scan).
const inFlightScanLimit = 20

// assertSystemCaller returns Forbidden unless the caller is a system
// principal or an admin. The publish gate is a platform-global singleton —
// letting any tenant reconfigure it would change publish behavior for every
// workspace (EVAL-04).
func (s *EvaluationService) assertSystemCaller(ctx context.Context) error {
	if workspace.IsSystem(ctx) {
		return nil
	}
	if a, ok := auth.FromContext(ctx); ok && a.HasAdminAccess() {
		return nil
	}
	return apierror.Forbidden("EVAL", "system or admin privileges required")
}

// --- Datasets ---

// assertEvalDatasetAccess loads a dataset and verifies the caller may read it.
// Shared/legacy datasets (workspace="") are readable by every workspace.
// Returns NotFound on access denial to avoid leaking resource existence (IDOR).
func (s *EvaluationService) assertEvalDatasetAccess(ctx context.Context, datasetID string) (biz.EvalDataset, error) {
	d, err := s.uc.GetDataset(ctx, datasetID)
	if err != nil {
		return biz.EvalDataset{}, err
	}
	if err := workspace.AssertWorkspaceOrShared(workspace.IDFromContext(ctx), d.Workspace); err != nil {
		return biz.EvalDataset{}, apierror.NotFound("EVAL", "dataset not found")
	}
	return d, nil
}

// assertEvalDatasetMutate verifies the caller may modify the dataset.
// Legacy datasets (workspace="") belong to the default workspace — same
// convention as eval_runs legacy rows (evalRunsWorkspaceFilter).
func (s *EvaluationService) assertEvalDatasetMutate(ctx context.Context, datasetID string) (biz.EvalDataset, error) {
	d, err := s.uc.GetDataset(ctx, datasetID)
	if err != nil {
		return biz.EvalDataset{}, err
	}
	if err := workspace.AssertWorkspace(workspace.IDFromContext(ctx), d.Workspace); err != nil {
		return biz.EvalDataset{}, apierror.NotFound("EVAL", "dataset not found")
	}
	return d, nil
}

func (s *EvaluationService) CreateDataset(ctx context.Context, req *v1.CreateDatasetRequest) (*v1.EvalDataset, error) {
	if strings.TrimSpace(req.GetName()) == "" {
		return nil, apierror.BadRequest("EVAL", "name is required")
	}
	d := biz.EvalDataset{
		Name:        req.GetName(),
		Description: req.GetDescription(),
	}
	if !workspace.IsSystem(ctx) {
		d.Workspace = workspace.IDFromContext(ctx)
	}
	created, err := s.uc.CreateDataset(ctx, d)
	if err != nil {
		return nil, err
	}
	return toProtoDataset(created), nil
}

func (s *EvaluationService) GetDataset(ctx context.Context, req *v1.GetDatasetRequest) (*v1.EvalDataset, error) {
	d, err := s.assertEvalDatasetAccess(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoDataset(d), nil
}

func (s *EvaluationService) ListDatasets(ctx context.Context, req *v1.ListDatasetsRequest) (*v1.ListDatasetsResponse, error) {
	// System callers (cron/admin) see all rows; tenants see their own
	// workspace plus shared/legacy datasets.
	ws := ""
	if !workspace.IsSystem(ctx) {
		ws = workspace.IDFromContext(ctx)
	}
	items, total, err := s.uc.ListDatasets(ctx, ws, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.EvalDataset, 0, len(items))
	for _, d := range items {
		out = append(out, toProtoDataset(d))
	}
	return &v1.ListDatasetsResponse{Items: out, Total: int32(total)}, nil
}

func (s *EvaluationService) DeleteDataset(ctx context.Context, req *v1.DeleteDatasetRequest) (*emptypb.Empty, error) {
	if _, err := s.assertEvalDatasetMutate(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.uc.DeleteDataset(ctx, req.GetId())
}

func (s *EvaluationService) UpdateDataset(ctx context.Context, req *v1.UpdateDatasetRequest) (*v1.EvalDataset, error) {
	if _, err := s.assertEvalDatasetMutate(ctx, req.GetId()); err != nil {
		return nil, err
	}
	d, err := s.uc.UpdateDataset(ctx, req.GetId(), req.GetName(), req.GetDescription())
	if err != nil {
		return nil, err
	}
	return toProtoDataset(d), nil
}

func (s *EvaluationService) UploadCases(ctx context.Context, req *v1.UploadCasesRequest) (*v1.UploadCasesResponse, error) {
	if strings.TrimSpace(req.GetDatasetId()) == "" {
		return nil, apierror.BadRequest("EVAL", "dataset_id is required")
	}
	if strings.TrimSpace(req.GetCasesJson()) == "" {
		return nil, apierror.BadRequest("EVAL", "cases_json is required")
	}
	if _, err := s.assertEvalDatasetMutate(ctx, req.GetDatasetId()); err != nil {
		return nil, err
	}
	n, err := s.uc.UploadCases(ctx, req.GetDatasetId(), req.GetCasesJson())
	if err != nil {
		return nil, err
	}
	return &v1.UploadCasesResponse{Imported: int32(n)}, nil
}

// --- Runs ---

// assertEvalRunAccess loads a run and verifies the caller's workspace owns it.
// Returns NotFound on access denial to avoid leaking resource existence (IDOR).
func (s *EvaluationService) assertEvalRunAccess(ctx context.Context, runID string) (biz.EvalRun, error) {
	run, err := s.uc.GetRun(ctx, runID)
	if err != nil {
		return biz.EvalRun{}, err
	}
	if err := workspace.AssertWorkspace(workspace.IDFromContext(ctx), run.WorkspaceID); err != nil {
		// TECH-DEBT(P2-B): add Warn log once EvaluationService gets lg field injected via wire.
		return biz.EvalRun{}, apierror.NotFound("EVAL", "run not found")
	}
	return run, nil
}

func (s *EvaluationService) RunEvaluation(ctx context.Context, req *v1.RunEvaluationRequest) (*v1.EvalRun, error) {
	if strings.TrimSpace(req.GetDatasetId()) == "" {
		return nil, apierror.BadRequest("EVAL", "dataset_id is required")
	}
	if strings.TrimSpace(req.GetAgentId()) == "" {
		return nil, apierror.BadRequest("EVAL", "agent_id is required")
	}
	numRuns := int(req.GetNumRuns())
	if numRuns < 0 || numRuns > biz.EvalMaxNumRuns {
		return nil, apierror.BadRequest("EVAL", fmt.Sprintf("num_runs must be between 0 (default) and %d", biz.EvalMaxNumRuns))
	}
	// Y6: reject unknown metric keys — a typo used to fall back to the full
	// default set deep inside the framework, reporting unrequested scores.
	if err := evaluation.ValidateMetricNames(req.GetMetrics()); err != nil {
		return nil, apierror.BadRequest("EVAL", err.Error())
	}
	// The dataset must be readable by the caller — otherwise a tenant could
	// run evaluations against another workspace's private dataset (IDOR).
	if _, err := s.assertEvalDatasetAccess(ctx, req.GetDatasetId()); err != nil {
		return nil, err
	}
	// EVAL-13: without a runner the run would sit in "pending" until the next
	// process restart sweep (Y10) — fail fast instead of creating a zombie.
	if s.runner == nil {
		return nil, apierror.Unavailable("EVAL", "evaluation runner is not available")
	}
	// EVAL-08: one in-flight run per (dataset, agent). Each run fans out one
	// inference (+ judge call) per case, so an unguarded double-click or a
	// retry loop multiplies LLM cost for an identical result.
	runs, _, err := s.uc.ListRuns(ctx, req.GetDatasetId(), req.GetAgentId(), inFlightScanLimit, 0)
	if err != nil {
		return nil, err
	}
	for _, r := range runs {
		if r.Status == "pending" || r.Status == "running" {
			return nil, apierror.Conflict("EVAL",
				fmt.Sprintf("an evaluation run is already in flight for this dataset+agent (run_id=%s)", r.ID))
		}
	}
	in := biz.EvalRun{
		DatasetID: req.GetDatasetId(),
		AgentID:   req.GetAgentId(),
		NumRuns:   numRuns,
	}
	if !workspace.IsSystem(ctx) {
		in.WorkspaceID = workspace.IDFromContext(ctx)
	}
	run, err := s.uc.CreateRun(ctx, in)
	if err != nil {
		return nil, err
	}
	if numRuns <= 0 {
		numRuns = 1
	}
	s.runner.Start(ctx, run, req.GetMetrics(), numRuns, req.GetUseUserSimulation())
	return toProtoRun(run), nil
}

func (s *EvaluationService) GetRun(ctx context.Context, req *v1.GetRunRequest) (*v1.EvalRun, error) {
	run, err := s.assertEvalRunAccess(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoRun(run), nil
}

func (s *EvaluationService) DeleteRun(ctx context.Context, req *v1.DeleteRunRequest) (*emptypb.Empty, error) {
	if _, err := s.assertEvalRunAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.uc.DeleteRun(ctx, req.GetId())
}

func (s *EvaluationService) ListRuns(ctx context.Context, req *v1.ListRunsRequest) (*v1.ListRunsResponse, error) {
	runs, total, err := s.uc.ListRuns(ctx, req.GetDatasetId(), req.GetAgentId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.EvalRun, 0, len(runs))
	for _, r := range runs {
		out = append(out, toProtoRun(r))
	}
	return &v1.ListRunsResponse{Items: out, Total: int32(total)}, nil
}

func (s *EvaluationService) GetRunResults(ctx context.Context, req *v1.GetRunResultsRequest) (*v1.GetRunResultsResponse, error) {
	if _, err := s.assertEvalRunAccess(ctx, req.GetRunId()); err != nil {
		return nil, err
	}
	results, total, err := s.uc.ListCaseResults(ctx, req.GetRunId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.EvalCaseResult, 0, len(results))
	for _, r := range results {
		out = append(out, toProtoCaseResult(r))
	}
	return &v1.GetRunResultsResponse{Items: out, Total: int32(total)}, nil
}

func (s *EvaluationService) AnnotateCaseResult(ctx context.Context, req *v1.AnnotateCaseResultRequest) (*v1.EvalCaseResult, error) {
	runID := strings.TrimSpace(req.GetRunId())
	resultID := strings.TrimSpace(req.GetResultId())
	if runID == "" || resultID == "" {
		return nil, apierror.BadRequest("EVAL", "run_id and result_id are required")
	}
	if _, err := s.assertEvalRunAccess(ctx, runID); err != nil {
		return nil, err
	}
	by := "system"
	if a, ok := auth.FromContext(ctx); ok && a.UserID > 0 {
		by = fmt.Sprintf("user:%d", a.UserID)
	}
	patch := biz.EvalCaseResultAnnotation{AnnotatedBy: by}
	if req.GetClearHumanPass() {
		patch.ClearHumanPass = true
	} else if req.HumanPass != nil {
		v := req.GetHumanPass()
		patch.HumanPass = &v
	}
	if req.GetClearHumanScore() {
		patch.ClearHumanScore = true
	} else if req.HumanScore != nil {
		v := req.GetHumanScore()
		patch.HumanScore = &v
	}
	if req.HumanComment != nil {
		c := strings.TrimSpace(req.GetHumanComment())
		patch.HumanComment = &c
	}
	res, err := s.uc.AnnotateCaseResult(ctx, runID, resultID, patch)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("EVAL_NOT_FOUND", "case result not found")
		}
		return nil, err
	}
	return toProtoCaseResult(res), nil
}

func (s *EvaluationService) GetAgentEvalTrend(ctx context.Context, req *v1.GetAgentEvalTrendRequest) (*v1.GetAgentEvalTrendResponse, error) {
	points, err := s.uc.GetAgentEvalTrend(ctx, req.GetAgentId(), req.GetDatasetId(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.EvalTrendPoint, 0, len(points))
	for _, p := range points {
		out = append(out, &v1.EvalTrendPoint{
			RunId:              p.RunID,
			CreatedAt:          p.CreatedAt,
			TriggerSource:      p.TriggerSource,
			ExactMatchScore:    p.ExactMatchScore,
			ContainsMatchScore: p.ContainsMatchScore,
			LlmJudgeScore:      p.LLMJudgeScore,
			ToolCallAccuracy:   p.ToolCallAccuracy,
			PassAtK:            p.PassAtK,
			PassHatK:           p.PassHatK,
		})
	}
	return &v1.GetAgentEvalTrendResponse{Points: out}, nil
}

func (s *EvaluationService) CompareEvalRuns(ctx context.Context, req *v1.CompareEvalRunsRequest) (*v1.CompareEvalRunsResponse, error) {
	items, err := s.uc.CompareEvalRuns(ctx, req.GetRunIds())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.EvalRunComparison, 0, len(items))
	for _, c := range items {
		out = append(out, &v1.EvalRunComparison{
			RunId:                 c.RunID,
			AgentId:               c.AgentID,
			DatasetId:             c.DatasetID,
			CreatedAt:             c.CreatedAt,
			ExactMatchScore:       c.ExactMatchScore,
			ContainsMatchScore:    c.ContainsMatchScore,
			LlmJudgeScore:         c.LLMJudgeScore,
			ToolCallAccuracy:      c.ToolCallAccuracy,
			PassAtK:               c.PassAtK,
			PassHatK:              c.PassHatK,
			DeltaExactMatch:       c.DeltaExactMatch,
			DeltaContainsMatch:    c.DeltaContainsMatch,
			DeltaLlmJudge:         c.DeltaLLMJudge,
			DeltaToolCallAccuracy: c.DeltaToolAccuracy,
			DatasetHash:           c.DatasetHash,
		})
	}
	return &v1.CompareEvalRunsResponse{Items: out}, nil
}

// GetJudgeDivergence reports judge-vs-human agreement for a dataset (P1-3).
func (s *EvaluationService) GetJudgeDivergence(ctx context.Context, req *v1.GetJudgeDivergenceRequest) (*v1.GetJudgeDivergenceResponse, error) {
	if _, err := s.assertEvalDatasetAccess(ctx, req.GetDatasetId()); err != nil {
		return nil, err
	}
	d, err := s.uc.GetJudgeDivergence(ctx, req.GetDatasetId(), req.GetAgentId(), req.GetThreshold(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	cases := make([]*v1.JudgeDivergenceCase, 0, len(d.Cases))
	for _, c := range d.Cases {
		cases = append(cases, &v1.JudgeDivergenceCase{
			ResultId:       c.ResultID,
			RunId:          c.RunID,
			CaseId:         c.CaseID,
			Input:          c.Input,
			ExpectedOutput: c.ExpectedOutput,
			ActualOutput:   c.ActualOutput,
			LlmJudgeScore:  c.LLMJudgeScore,
			HumanPass:      c.HumanPass,
			HumanComment:   c.HumanComment,
			DivergenceKind: c.Kind,
			CreatedAt:      c.CreatedAt,
		})
	}
	return &v1.GetJudgeDivergenceResponse{
		Threshold:      d.Threshold,
		AnnotatedTotal: int32(d.AnnotatedTotal),
		AgreeCount:     int32(d.AgreeCount),
		DivergeCount:   int32(d.DivergeCount),
		AgreementRate:  d.AgreementRate,
		FalsePassCount: int32(d.FalsePassCount),
		FalseFailCount: int32(d.FalseFailCount),
		DivergentCases: cases,
	}, nil
}

// --- Governance (P2-1 publish gate / P2-3 failure grouping / P3-3 pairwise) ---

// GetFailureGroups groups failed case results of a dataset by error_message (P2-3).
func (s *EvaluationService) GetFailureGroups(ctx context.Context, req *v1.GetFailureGroupsRequest) (*v1.GetFailureGroupsResponse, error) {
	if _, err := s.assertEvalDatasetAccess(ctx, req.GetDatasetId()); err != nil {
		return nil, err
	}
	report, err := s.uc.GetFailureGroups(ctx, req.GetDatasetId(), req.GetAgentId(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	groups := make([]*v1.EvalFailureGroup, 0, len(report.Groups))
	for _, g := range report.Groups {
		groups = append(groups, &v1.EvalFailureGroup{
			ErrorMessage: g.ErrorMessage,
			Count:        int32(g.Count),
			RunCount:     int32(g.RunCount),
			LatestAt:     g.LatestAt,
		})
	}
	return &v1.GetFailureGroupsResponse{TotalFailed: int32(report.TotalFailed), Groups: groups}, nil
}

// SubmitRunPreference records one pairwise human judgment (P3-3).
func (s *EvaluationService) SubmitRunPreference(ctx context.Context, req *v1.SubmitRunPreferenceRequest) (*v1.EvalRunPreference, error) {
	if _, err := s.assertEvalDatasetAccess(ctx, req.GetDatasetId()); err != nil {
		return nil, err
	}
	createdBy := "system"
	if a, ok := auth.FromContext(ctx); ok && a.UserID > 0 {
		createdBy = fmt.Sprintf("user:%d", a.UserID)
	}
	p, err := s.uc.SubmitRunPreference(ctx, biz.EvalRunPreference{
		DatasetID:   req.GetDatasetId(),
		RunIDA:      req.GetRunIdA(),
		RunIDB:      req.GetRunIdB(),
		WinnerRunID: req.GetWinnerRunId(),
		Comment:     req.GetComment(),
		CreatedBy:   createdBy,
	})
	if err != nil {
		return nil, err
	}
	return toProtoRunPreference(p), nil
}

// ListRunPreferences lists recorded pairwise judgments for a dataset (P3-3).
func (s *EvaluationService) ListRunPreferences(ctx context.Context, req *v1.ListRunPreferencesRequest) (*v1.ListRunPreferencesResponse, error) {
	if _, err := s.assertEvalDatasetAccess(ctx, req.GetDatasetId()); err != nil {
		return nil, err
	}
	items, err := s.uc.ListRunPreferences(ctx, req.GetDatasetId(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.EvalRunPreference, 0, len(items))
	for _, p := range items {
		out = append(out, toProtoRunPreference(p))
	}
	return &v1.ListRunPreferencesResponse{Items: out}, nil
}

// GetEvalGate returns the singleton publish-gate config (P2-1).
func (s *EvaluationService) GetEvalGate(ctx context.Context, _ *v1.GetEvalGateRequest) (*v1.EvalGateConfig, error) {
	cfg, err := s.uc.GetGateConfig(ctx)
	if err != nil {
		return nil, err
	}
	return toProtoGateConfig(cfg), nil
}

// UpdateEvalGate validates and persists the publish-gate config (P2-1).
// The config is a platform-global singleton → system/admin only (EVAL-04).
func (s *EvaluationService) UpdateEvalGate(ctx context.Context, req *v1.UpdateEvalGateRequest) (*v1.EvalGateConfig, error) {
	if err := s.assertSystemCaller(ctx); err != nil {
		return nil, err
	}
	// Y12: reject unknown gate metric keys at config time. A typo used to
	// surface only at publish time — the gate then blocked every publish
	// with "缺少指标 X 得分", and the cause was hard to trace.
	if err := evaluation.ValidateMetricNames(req.GetMetric()); err != nil {
		return nil, apierror.BadRequest("EVAL", err.Error())
	}
	cfg, err := s.uc.UpdateGateConfig(ctx, biz.EvalGateConfig{
		Enabled:   req.GetEnabled(),
		AgentID:   req.GetAgentId(),
		DatasetID: req.GetDatasetId(),
		Metric:    req.GetMetric(),
		MinScore:  req.GetMinScore(),
		MaxDrop:   req.GetMaxDrop(),
	})
	if err != nil {
		return nil, err
	}
	return toProtoGateConfig(cfg), nil
}

func toProtoRunPreference(p biz.EvalRunPreference) *v1.EvalRunPreference {
	return &v1.EvalRunPreference{
		Id:          p.ID,
		DatasetId:   p.DatasetID,
		RunIdA:      p.RunIDA,
		RunIdB:      p.RunIDB,
		WinnerRunId: p.WinnerRunID,
		Comment:     p.Comment,
		CreatedBy:   p.CreatedBy,
		CreatedAt:   p.CreatedAt,
	}
}

func toProtoGateConfig(c biz.EvalGateConfig) *v1.EvalGateConfig {
	return &v1.EvalGateConfig{
		Enabled:   c.Enabled,
		AgentId:   c.AgentID,
		DatasetId: c.DatasetID,
		Metric:    c.Metric,
		MinScore:  c.MinScore,
		MaxDrop:   c.MaxDrop,
		UpdatedAt: c.UpdatedAt,
	}
}

// --- proto conversion helpers ---

func toProtoDataset(d biz.EvalDataset) *v1.EvalDataset {
	return &v1.EvalDataset{
		Id:          d.ID,
		Name:        d.Name,
		Description: d.Description,
		CaseCount:   int32(d.CaseCount),
		Workspace:   d.Workspace,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

func toProtoRun(r biz.EvalRun) *v1.EvalRun {
	return &v1.EvalRun{
		Id:                 r.ID,
		DatasetId:          r.DatasetID,
		AgentId:            r.AgentID,
		Status:             r.Status,
		TotalCases:         int32(r.TotalCases),
		CompletedCases:     int32(r.CompletedCases),
		ExactMatchScore:    r.ExactMatchScore,
		ContainsMatchScore: r.ContainsMatchScore,
		LlmJudgeScore:      r.LLMJudgeScore,
		ToolCallAccuracy:   r.ToolCallAccuracy,
		PassAtK:            r.PassAtK,
		PassHatK:           r.PassHatK,
		TriggerSource:      r.TriggerSource,
		NumRuns:            int32(r.NumRuns),
		ScoresJson:         r.ScoresJSON,
		ErrorMessage:       r.ErrorMessage,
		StartedAt:          r.StartedAt,
		FinishedAt:         r.FinishedAt,
		DatasetHash:        r.DatasetHash,
		CreatedAt:          r.CreatedAt,
	}
}

func toProtoCaseResult(r biz.EvalCaseResult) *v1.EvalCaseResult {
	out := &v1.EvalCaseResult{
		Id:               r.ID,
		RunId:            r.RunID,
		CaseId:           r.CaseID,
		ActualOutput:     r.ActualOutput,
		ExactMatch:       r.ExactMatch,
		ContainsMatch:    r.ContainsMatch,
		LlmJudgeScore:    r.LLMJudgeScore,
		ToolCallAccuracy: r.ToolCallAccuracy,
		ErrorMessage:     r.ErrorMessage,
		CreatedAt:        r.CreatedAt,
		HumanComment:     r.HumanComment,
		AnnotatedAt:      r.AnnotatedAt,
		AnnotatedBy:      r.AnnotatedBy,
		ScoresJson:       r.ScoresJSON,
	}
	if r.HumanPass != nil {
		v := *r.HumanPass
		out.HumanPass = &v
	}
	if r.HumanScore != nil {
		v := *r.HumanScore
		out.HumanScore = &v
	}
	return out
}
