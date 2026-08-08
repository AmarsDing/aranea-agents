// Package evaluation implements evaluation dataset/run management workflows.
package evaluation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync/atomic"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Dataset holds a set of evaluation test cases.
type Dataset struct {
	ID          string
	Name        string
	Description string
	CaseCount   int
	Workspace   string
	CreatedAt   string
	UpdatedAt   string
}

// Case is one input/expected-output pair.
type Case struct {
	ID             string
	DatasetID      string
	Input          string
	ExpectedOutput string
	MetadataJSON   string
}

// CaseUpload is one row in a bulk upload payload.
type CaseUpload struct {
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	MetadataJSON   string `json:"metadata_json,omitempty"`
}

// Run represents one evaluation execution.
type Run struct {
	ID                 string
	DatasetID          string
	AgentID            string
	Status             string
	TotalCases         int
	CompletedCases     int
	ExactMatchScore    float32
	ContainsMatchScore float32
	LLMJudgeScore      float32
	ToolCallAccuracy   float32
	PassAtK            float32
	PassHatK           float32
	TriggerSource      string
	NumRuns            int
	ScoresJSON         string
	ErrorMessage       string
	StartedAt          string
	FinishedAt         string
	// DatasetHash is a content hash snapshot of the dataset cases at run
	// start (P3-5). Trend/compare surfaces a warning when two runs of the
	// same dataset carry different hashes (scores not directly comparable).
	DatasetHash string
	// WorkspaceID scopes this run to a tenant workspace.
	// empty = legacy (treated as default workspace); non-empty = tenant-private.
	WorkspaceID string
	CreatedAt   string
}

// CaseResult is the outcome for one case in a run.
type CaseResult struct {
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
	HumanPass        *bool
	HumanScore       *float32
	HumanComment     string
	AnnotatedAt      string
	AnnotatedBy      string
	ScoresJSON       string
}

// CaseResultAnnotation is a partial update for human review.
type CaseResultAnnotation struct {
	HumanPass    *bool
	HumanScore   *float32
	HumanComment *string
	AnnotatedBy  string
}

// TrendPoint is one completed run on the agent quality timeline.
type TrendPoint struct {
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

// RunComparison compares metric scores across runs.
type RunComparison struct {
	RunID              string
	AgentID            string
	DatasetID          string
	CreatedAt          string
	DatasetHash        string // P3-5: snapshot for dataset-changed warning
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

// Repo is the persistence interface for evaluation operations.
type Repo interface {
	CreateDataset(ctx context.Context, d Dataset) (Dataset, error)
	GetDataset(ctx context.Context, id string) (Dataset, error)
	ListDatasets(ctx context.Context, workspace string, limit, offset int) ([]Dataset, int, error)
	DeleteDataset(ctx context.Context, id string) error
	UpdateDataset(ctx context.Context, id, name, description string) (Dataset, error)
	UpdateDatasetCaseCount(ctx context.Context, id string, delta int) error

	InsertCases(ctx context.Context, cases []Case) error
	// InsertCasesWithCountUpdate inserts cases and bumps dataset.case_count in a
	// single transaction. Use this instead of separate InsertCases +
	// UpdateDatasetCaseCount calls when the two writes must be atomic
	// (e.g. UploadCases). Without atomicity, a failure between the two writes
	// would leave case_count diverged from the actual row count in eval_cases.
	InsertCasesWithCountUpdate(ctx context.Context, datasetID string, cases []Case) error
	ListCases(ctx context.Context, datasetID string) ([]Case, error)

	CreateRun(ctx context.Context, r Run) (Run, error)
	GetRun(ctx context.Context, id string) (Run, error)
	UpdateRun(ctx context.Context, r Run) error
	DeleteRun(ctx context.Context, id string) error
	ListRuns(ctx context.Context, datasetID, agentID string, limit, offset int) ([]Run, int, error)

	InsertCaseResult(ctx context.Context, r CaseResult) error
	ListCaseResults(ctx context.Context, runID string, limit, offset int) ([]CaseResult, int, error)
	GetCaseResult(ctx context.Context, runID, resultID string) (CaseResult, error)
	UpdateCaseResultAnnotation(ctx context.Context, runID, resultID string, patch CaseResultAnnotation) (CaseResult, error)

	ListTrendPoints(ctx context.Context, agentID, datasetID string, limit int) ([]TrendPoint, error)
	GetRunsByIDs(ctx context.Context, ids []string) ([]Run, error)

	// ListJudgeAnnotatedResults returns results with human_pass set (joined
	// with run dataset/agent scope and case text) for judge calibration (P1-3).
	ListJudgeAnnotatedResults(ctx context.Context, datasetID, agentID string) ([]JudgeAnnotatedResult, error)

	// ListFailureGroups aggregates case-result failures by error_message for
	// one dataset (P2-3, SQL-version failure clustering).
	ListFailureGroups(ctx context.Context, datasetID, agentID string, limit int) ([]FailureGroup, int, error)

	// InsertRunPreference / ListRunPreferences persist pairwise human
	// preferences between two runs of one dataset (P3-3).
	InsertRunPreference(ctx context.Context, p RunPreference) error
	ListRunPreferences(ctx context.Context, datasetID string, limit int) ([]RunPreference, error)

	// GetGateConfig / UpsertGateConfig read/write the publish-gate singleton
	// (P2-1). GetGateConfig returns a disabled zero config when no row exists.
	GetGateConfig(ctx context.Context) (GateConfig, error)
	UpsertGateConfig(ctx context.Context, cfg GateConfig) error
}

// Usecase implements dataset/run management.
type Usecase struct {
	repo Repo
	lg   loggateway.Logger
}

// NewUsecase constructs an evaluation Usecase.
func NewUsecase(repo Repo, lg loggateway.Logger) *Usecase {
	return &Usecase{repo: repo, lg: lg}
}

var fallbackEvalID atomic.Uint64

func newEvalID() string {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		n := fallbackEvalID.Add(1)
		return hex.EncodeToString([]byte{
			byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32),
			byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n),
		})
	}
	return hex.EncodeToString(buf)
}

// CreateDataset validates and stores a new dataset.
func (u *Usecase) CreateDataset(ctx context.Context, in Dataset) (Dataset, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return Dataset{}, apierror.BadRequest("EVAL", "name is required")
	}
	if in.ID == "" {
		in.ID = newEvalID()
	}
	return u.repo.CreateDataset(ctx, in)
}

// GetDataset returns one dataset.
func (u *Usecase) GetDataset(ctx context.Context, id string) (Dataset, error) {
	if strings.TrimSpace(id) == "" {
		return Dataset{}, apierror.BadRequest("EVAL", "id is required")
	}
	return u.repo.GetDataset(ctx, id)
}

// ListDatasets returns datasets visible in the workspace.
func (u *Usecase) ListDatasets(ctx context.Context, workspace string, limit, offset int) ([]Dataset, int, error) {
	if limit <= 0 {
		limit = 20
	}
	return u.repo.ListDatasets(ctx, workspace, limit, offset)
}

// DeleteDataset removes a dataset and its cases.
func (u *Usecase) DeleteDataset(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("EVAL", "id is required")
	}
	return u.repo.DeleteDataset(ctx, id)
}

// UpdateDataset updates dataset name and/or description.
func (u *Usecase) UpdateDataset(ctx context.Context, id, name, description string) (Dataset, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Dataset{}, apierror.BadRequest("EVAL", "id is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Dataset{}, apierror.BadRequest("EVAL", "name is required")
	}
	return u.repo.UpdateDataset(ctx, id, name, description)
}

// UploadCases parses casesJSON (array) and bulk-inserts into the dataset.
func (u *Usecase) UploadCases(ctx context.Context, datasetID, casesJSON string) (int, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return 0, apierror.BadRequest("EVAL", "dataset_id is required")
	}
	var uploads []CaseUpload
	if err := json.Unmarshal([]byte(casesJSON), &uploads); err != nil {
		return 0, apierror.BadRequest("EVAL", "cases_json must be a valid JSON array: "+err.Error())
	}
	if len(uploads) == 0 {
		return 0, apierror.BadRequest("EVAL", "cases_json array is empty")
	}
	cases := make([]Case, 0, len(uploads))
	for _, up := range uploads {
		if strings.TrimSpace(up.Input) == "" {
			continue
		}
		cases = append(cases, Case{
			ID:             newEvalID(),
			DatasetID:      datasetID,
			Input:          up.Input,
			ExpectedOutput: up.ExpectedOutput,
			MetadataJSON:   up.MetadataJSON,
		})
	}
	if err := u.repo.InsertCasesWithCountUpdate(ctx, datasetID, cases); err != nil {
		return 0, err
	}
	return len(cases), nil
}

// CreateRun starts a new async evaluation run.
func (u *Usecase) CreateRun(ctx context.Context, in Run) (Run, error) {
	in.DatasetID = strings.TrimSpace(in.DatasetID)
	in.AgentID = strings.TrimSpace(in.AgentID)
	if in.DatasetID == "" {
		return Run{}, apierror.BadRequest("EVAL", "dataset_id is required")
	}
	if in.AgentID == "" {
		return Run{}, apierror.BadRequest("EVAL", "agent_id is required")
	}
	if in.ID == "" {
		in.ID = newEvalID()
	}
	if in.Status == "" {
		in.Status = "pending"
	}
	if strings.TrimSpace(in.TriggerSource) == "" {
		in.TriggerSource = "manual"
	}
	if in.NumRuns <= 0 {
		in.NumRuns = 1
	}
	return u.repo.CreateRun(ctx, in)
}

// GetRun returns one run.
func (u *Usecase) GetRun(ctx context.Context, id string) (Run, error) {
	if strings.TrimSpace(id) == "" {
		return Run{}, apierror.BadRequest("EVAL", "id is required")
	}
	return u.repo.GetRun(ctx, id)
}

// ListRuns returns runs optionally filtered by dataset/agent.
func (u *Usecase) ListRuns(ctx context.Context, datasetID, agentID string, limit, offset int) ([]Run, int, error) {
	if limit <= 0 {
		limit = 20
	}
	return u.repo.ListRuns(ctx, datasetID, agentID, limit, offset)
}

// UpdateRun persists run progress/result changes.
func (u *Usecase) UpdateRun(ctx context.Context, r Run) error {
	return u.repo.UpdateRun(ctx, r)
}

// DeleteRun removes a run and its case results.
func (u *Usecase) DeleteRun(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("EVAL", "id is required")
	}
	return u.repo.DeleteRun(ctx, id)
}

// ListCaseResults returns per-case results for a run.
func (u *Usecase) ListCaseResults(ctx context.Context, runID string, limit, offset int) ([]CaseResult, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return u.repo.ListCaseResults(ctx, runID, limit, offset)
}

// InsertCaseResult persists one case result.
func (u *Usecase) InsertCaseResult(ctx context.Context, r CaseResult) error {
	return u.repo.InsertCaseResult(ctx, r)
}

// AnnotateCaseResult updates human review fields for one case result.
func (u *Usecase) AnnotateCaseResult(ctx context.Context, runID, resultID string, patch CaseResultAnnotation) (CaseResult, error) {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(resultID) == "" {
		return CaseResult{}, apierror.BadRequest("EVAL", "run_id and result_id are required")
	}
	if strings.TrimSpace(patch.AnnotatedBy) == "" {
		patch.AnnotatedBy = "system"
	}
	return u.repo.UpdateCaseResultAnnotation(ctx, runID, resultID, patch)
}

// ListCases returns all cases for a dataset.
func (u *Usecase) ListCases(ctx context.Context, datasetID string) ([]Case, error) {
	return u.repo.ListCases(ctx, datasetID)
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
	return u.repo.ListTrendPoints(ctx, agentID, datasetID, limit)
}

// CompareEvalRuns loads runs by ID and computes metric deltas vs the first run (baseline).
func (u *Usecase) CompareEvalRuns(ctx context.Context, runIDs []string) ([]RunComparison, error) {
	if len(runIDs) < 2 {
		return nil, apierror.BadRequest("EVAL", "at least two run_ids are required")
	}
	runs, err := u.repo.GetRunsByIDs(ctx, runIDs)
	if err != nil {
		return nil, err
	}
	if len(runs) < 2 {
		return nil, apierror.BadRequest("EVAL", "could not load at least two runs")
	}
	// ISSUE-004: baseline must be the earliest run by creation time, not
	// whatever order the caller/store happened to return. CreatedAt is
	// RFC3339, so lexicographic order matches chronological order.
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

// ── Scores helpers ────────────────────────────────────────────────────────────

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

// ── LLM Setting ───────────────────────────────────────────────────────────────

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
