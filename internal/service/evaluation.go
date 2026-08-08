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

// --- Datasets ---

func (s *EvaluationService) CreateDataset(ctx context.Context, req *v1.CreateDatasetRequest) (*v1.EvalDataset, error) {
	if strings.TrimSpace(req.GetName()) == "" {
		return nil, apierror.BadRequest("EVAL", "name is required")
	}
	d, err := s.uc.CreateDataset(ctx, biz.EvalDataset{
		Name:        req.GetName(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, err
	}
	return toProtoDataset(d), nil
}

func (s *EvaluationService) GetDataset(ctx context.Context, req *v1.GetDatasetRequest) (*v1.EvalDataset, error) {
	d, err := s.uc.GetDataset(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoDataset(d), nil
}

func (s *EvaluationService) ListDatasets(ctx context.Context, req *v1.ListDatasetsRequest) (*v1.ListDatasetsResponse, error) {
	items, total, err := s.uc.ListDatasets(ctx, "", int(req.GetLimit()), int(req.GetOffset()))
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
	return &emptypb.Empty{}, s.uc.DeleteDataset(ctx, req.GetId())
}

func (s *EvaluationService) UpdateDataset(ctx context.Context, req *v1.UpdateDatasetRequest) (*v1.EvalDataset, error) {
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
	in := biz.EvalRun{
		DatasetID: req.GetDatasetId(),
		AgentID:   req.GetAgentId(),
		NumRuns:   int(req.GetNumRuns()),
	}
	if !workspace.IsSystem(ctx) {
		in.WorkspaceID = workspace.IDFromContext(ctx)
	}
	run, err := s.uc.CreateRun(ctx, in)
	if err != nil {
		return nil, err
	}
	if s.runner != nil {
		numRuns := int(req.GetNumRuns())
		if numRuns <= 0 {
			numRuns = 1
		}
		s.runner.Start(ctx, run, req.GetMetrics(), numRuns, req.GetUseUserSimulation())
	}
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
	by := "system"
	if a, ok := auth.FromContext(ctx); ok && a.UserID > 0 {
		by = fmt.Sprintf("user:%d", a.UserID)
	}
	patch := biz.EvalCaseResultAnnotation{AnnotatedBy: by}
	if req.HumanPass != nil {
		v := req.GetHumanPass()
		patch.HumanPass = &v
	}
	if req.HumanScore != nil {
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
		})
	}
	return &v1.CompareEvalRunsResponse{Items: out}, nil
}

// GetJudgeDivergence reports judge-vs-human agreement for a dataset (P1-3).
func (s *EvaluationService) GetJudgeDivergence(ctx context.Context, req *v1.GetJudgeDivergenceRequest) (*v1.GetJudgeDivergenceResponse, error) {
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
