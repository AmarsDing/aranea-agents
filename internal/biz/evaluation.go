package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// EvalDataset holds a set of evaluation test cases.
type EvalDataset struct {
	ID          string
	Name        string
	Description string
	CaseCount   int
	Workspace   string
	CreatedAt   string
	UpdatedAt   string
}

// EvalCase is one input/expected-output pair.
type EvalCase struct {
	ID             string
	DatasetID      string
	Input          string
	ExpectedOutput string
	MetadataJSON   string
}

// EvalCaseUpload is one row in a bulk upload payload.
type EvalCaseUpload struct {
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	MetadataJSON   string `json:"metadata_json,omitempty"`
}

// EvalRun represents one evaluation execution.
type EvalRun struct {
	ID                  string
	DatasetID           string
	AgentID             string
	Status              string
	TotalCases          int
	CompletedCases      int
	ExactMatchScore     float32
	ContainsMatchScore  float32
	LLMJudgeScore       float32
	ToolCallAccuracy    float32
	ErrorMessage        string
	StartedAt           string
	FinishedAt          string
	CreatedAt           string
}

// EvalCaseResult is the outcome for one case in a run.
type EvalCaseResult struct {
	ID               string
	RunID            string
	CaseID           string
	ActualOutput     string
	ExactMatch       bool
	ContainsMatch    bool
	LLMJudgeScore    float32
	ToolCallAccuracy float32
	ErrorMessage     string
	CreatedAt        string
}

// EvalRepo is the persistence interface for evaluation operations.
type EvalRepo interface {
	CreateDataset(ctx context.Context, d EvalDataset) (EvalDataset, error)
	GetDataset(ctx context.Context, id string) (EvalDataset, error)
	ListDatasets(ctx context.Context, workspace string, limit, offset int) ([]EvalDataset, int, error)
	DeleteDataset(ctx context.Context, id string) error
	UpdateDatasetCaseCount(ctx context.Context, id string, delta int) error

	InsertCases(ctx context.Context, cases []EvalCase) error
	ListCases(ctx context.Context, datasetID string) ([]EvalCase, error)

	CreateRun(ctx context.Context, r EvalRun) (EvalRun, error)
	GetRun(ctx context.Context, id string) (EvalRun, error)
	UpdateRun(ctx context.Context, r EvalRun) error
	ListRuns(ctx context.Context, datasetID, agentID string, limit, offset int) ([]EvalRun, int, error)

	InsertCaseResult(ctx context.Context, r EvalCaseResult) error
	ListCaseResults(ctx context.Context, runID string, limit, offset int) ([]EvalCaseResult, int, error)
}

// EvalUsecase implements dataset/run management.
type EvalUsecase struct {
	repo EvalRepo
}

// NewEvalUsecase constructs an EvalUsecase.
func NewEvalUsecase(repo EvalRepo) *EvalUsecase {
	return &EvalUsecase{repo: repo}
}

func newEvalID() string {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "ev-fallback"
	}
	return hex.EncodeToString(buf)
}

// CreateDataset validates and stores a new dataset.
func (u *EvalUsecase) CreateDataset(ctx context.Context, in EvalDataset) (EvalDataset, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return EvalDataset{}, errors.BadRequest("EVAL", "name is required")
	}
	if in.ID == "" {
		in.ID = newEvalID()
	}
	return u.repo.CreateDataset(ctx, in)
}

// GetDataset returns one dataset.
func (u *EvalUsecase) GetDataset(ctx context.Context, id string) (EvalDataset, error) {
	if strings.TrimSpace(id) == "" {
		return EvalDataset{}, errors.BadRequest("EVAL", "id is required")
	}
	return u.repo.GetDataset(ctx, id)
}

// ListDatasets returns datasets visible in the workspace.
func (u *EvalUsecase) ListDatasets(ctx context.Context, workspace string, limit, offset int) ([]EvalDataset, int, error) {
	if limit <= 0 {
		limit = 20
	}
	return u.repo.ListDatasets(ctx, workspace, limit, offset)
}

// DeleteDataset removes a dataset and its cases.
func (u *EvalUsecase) DeleteDataset(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("EVAL", "id is required")
	}
	return u.repo.DeleteDataset(ctx, id)
}

// UploadCases parses casesJSON (array) and bulk-inserts into the dataset.
func (u *EvalUsecase) UploadCases(ctx context.Context, datasetID, casesJSON string) (int, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return 0, errors.BadRequest("EVAL", "dataset_id is required")
	}
	var uploads []EvalCaseUpload
	if err := json.Unmarshal([]byte(casesJSON), &uploads); err != nil {
		return 0, errors.BadRequest("EVAL", "cases_json must be a valid JSON array: "+err.Error())
	}
	if len(uploads) == 0 {
		return 0, errors.BadRequest("EVAL", "cases_json array is empty")
	}
	cases := make([]EvalCase, 0, len(uploads))
	for _, up := range uploads {
		if strings.TrimSpace(up.Input) == "" {
			continue
		}
		cases = append(cases, EvalCase{
			ID:             newEvalID(),
			DatasetID:      datasetID,
			Input:          up.Input,
			ExpectedOutput: up.ExpectedOutput,
			MetadataJSON:   up.MetadataJSON,
		})
	}
	if err := u.repo.InsertCases(ctx, cases); err != nil {
		return 0, err
	}
	_ = u.repo.UpdateDatasetCaseCount(ctx, datasetID, len(cases))
	return len(cases), nil
}

// CreateRun starts a new async evaluation run.
func (u *EvalUsecase) CreateRun(ctx context.Context, in EvalRun) (EvalRun, error) {
	in.DatasetID = strings.TrimSpace(in.DatasetID)
	in.AgentID = strings.TrimSpace(in.AgentID)
	if in.DatasetID == "" {
		return EvalRun{}, errors.BadRequest("EVAL", "dataset_id is required")
	}
	if in.AgentID == "" {
		return EvalRun{}, errors.BadRequest("EVAL", "agent_id is required")
	}
	if in.ID == "" {
		in.ID = newEvalID()
	}
	if in.Status == "" {
		in.Status = "pending"
	}
	return u.repo.CreateRun(ctx, in)
}

// GetRun returns one run.
func (u *EvalUsecase) GetRun(ctx context.Context, id string) (EvalRun, error) {
	if strings.TrimSpace(id) == "" {
		return EvalRun{}, errors.BadRequest("EVAL", "id is required")
	}
	return u.repo.GetRun(ctx, id)
}

// ListRuns returns runs optionally filtered by dataset/agent.
func (u *EvalUsecase) ListRuns(ctx context.Context, datasetID, agentID string, limit, offset int) ([]EvalRun, int, error) {
	if limit <= 0 {
		limit = 20
	}
	return u.repo.ListRuns(ctx, datasetID, agentID, limit, offset)
}

// UpdateRun persists run progress/result changes.
func (u *EvalUsecase) UpdateRun(ctx context.Context, r EvalRun) error {
	return u.repo.UpdateRun(ctx, r)
}

// ListCaseResults returns per-case results for a run.
func (u *EvalUsecase) ListCaseResults(ctx context.Context, runID string, limit, offset int) ([]EvalCaseResult, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return u.repo.ListCaseResults(ctx, runID, limit, offset)
}

// InsertCaseResult persists one case result.
func (u *EvalUsecase) InsertCaseResult(ctx context.Context, r EvalCaseResult) error {
	return u.repo.InsertCaseResult(ctx, r)
}

// ListCases returns all cases for a dataset.
func (u *EvalUsecase) ListCases(ctx context.Context, datasetID string) ([]EvalCase, error) {
	return u.repo.ListCases(ctx, datasetID)
}
