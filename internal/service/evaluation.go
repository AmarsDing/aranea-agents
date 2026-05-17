package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/evaluation/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"

	kerrors "github.com/go-kratos/kratos/v2/errors"
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
		return nil, kerrors.BadRequest("EVAL", "name is required")
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

func (s *EvaluationService) UploadCases(ctx context.Context, req *v1.UploadCasesRequest) (*v1.UploadCasesResponse, error) {
	if strings.TrimSpace(req.GetDatasetId()) == "" {
		return nil, kerrors.BadRequest("EVAL", "dataset_id is required")
	}
	if strings.TrimSpace(req.GetCasesJson()) == "" {
		return nil, kerrors.BadRequest("EVAL", "cases_json is required")
	}
	n, err := s.uc.UploadCases(ctx, req.GetDatasetId(), req.GetCasesJson())
	if err != nil {
		return nil, err
	}
	return &v1.UploadCasesResponse{Imported: int32(n)}, nil
}

// --- Runs ---

func (s *EvaluationService) RunEvaluation(ctx context.Context, req *v1.RunEvaluationRequest) (*v1.EvalRun, error) {
	if strings.TrimSpace(req.GetDatasetId()) == "" {
		return nil, kerrors.BadRequest("EVAL", "dataset_id is required")
	}
	if strings.TrimSpace(req.GetAgentId()) == "" {
		return nil, kerrors.BadRequest("EVAL", "agent_id is required")
	}
	run, err := s.uc.CreateRun(ctx, biz.EvalRun{
		DatasetID: req.GetDatasetId(),
		AgentID:   req.GetAgentId(),
	})
	if err != nil {
		return nil, err
	}
	// Fire async runner.
	if s.runner != nil {
		s.runner.Start(ctx, run, req.GetMetrics())
	}
	return toProtoRun(run), nil
}

func (s *EvaluationService) GetRun(ctx context.Context, req *v1.GetRunRequest) (*v1.EvalRun, error) {
	run, err := s.uc.GetRun(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoRun(run), nil
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
		Id:                  r.ID,
		DatasetId:           r.DatasetID,
		AgentId:             r.AgentID,
		Status:              r.Status,
		TotalCases:          int32(r.TotalCases),
		CompletedCases:      int32(r.CompletedCases),
		ExactMatchScore:     r.ExactMatchScore,
		ContainsMatchScore:  r.ContainsMatchScore,
		LlmJudgeScore:       r.LLMJudgeScore,
		ToolCallAccuracy:    r.ToolCallAccuracy,
		ErrorMessage:        r.ErrorMessage,
		StartedAt:           r.StartedAt,
		FinishedAt:          r.FinishedAt,
		CreatedAt:           r.CreatedAt,
	}
}

func toProtoCaseResult(r biz.EvalCaseResult) *v1.EvalCaseResult {
	return &v1.EvalCaseResult{
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
	}
}
