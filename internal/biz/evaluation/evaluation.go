// Package evaluation implements evaluation dataset/run management workflows.
package evaluation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

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
	// DatasetVersionID / DatasetVersion bind the run to an immutable snapshot.
	DatasetVersionID string
	DatasetVersion   int
	// ExperimentID groups runs that belong to one matrix sweep.
	ExperimentID string
	// VariantLabel is the matrix cell label (agent / model / prompt).
	VariantLabel string
	// Model is an optional model override for this experiment cell.
	Model string
	// Prompt is an optional extra instruction overlay for this cell.
	Prompt string
	// Tools is an optional tool allowlist ("none" or comma-separated keys).
	Tools string
	// LeaseUntil is the RFC3339 heartbeat deadline for in-flight runs.
	LeaseUntil string
	// JudgeCalls / JudgeTokens accumulate LLM-judge invocations for this run.
	JudgeCalls  int
	JudgeTokens int
	// WorkspaceID scopes this run to a tenant workspace.
	// empty = legacy (treated as default workspace); non-empty = tenant-private.
	WorkspaceID string
	CreatedAt   string
}

// Run lifecycle statuses. Transitions: pending → running → completed|failed|cancelled;
// pending → cancelled. Terminal states have no outbound transition.
const (
	RunStatusPending   = "pending"
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
	RunStatusCancelled = "cancelled"
)

// CaseResult is the outcome for one case in a run.
type CaseResult struct {
	ID     string
	RunID  string
	CaseID string
	// Input is the case text joined from eval_cases at read time (annotation
	// UX); empty when the case row no longer exists. Not stored on the result.
	Input            string
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
	SessionID        string
	TraceRunID       string
}

// CaseResultAnnotation is a partial update for human review.
type CaseResultAnnotation struct {
	HumanPass    *bool
	HumanScore   *float32
	HumanComment *string
	// ClearHumanPass / ClearHumanScore reset the field to un-annotated (NULL).
	// A clear flag takes precedence over the corresponding value pointer.
	ClearHumanPass  bool
	ClearHumanScore bool
	AnnotatedBy     string
}

// MaxNumRuns caps the per-case repeat count (MultiRun) accepted from API
// requests and agent auto-eval config. Each run costs one full inference +
// judge pass per case, so unbounded values would multiply LLM cost without
// improving pass@k signal beyond ~20 samples.
const MaxNumRuns = 20

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
	DatasetVersionID   string
	DatasetVersion     int
	ExperimentID       string
	VariantLabel       string
	Model              string
	Prompt             string
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

// Usecase implements dataset/run management. Persistence is injected as
// Stores (ISP); production must not depend on the deprecated Repo aggregate.
type Usecase struct {
	datasets   DatasetStore
	cases      CaseStore
	runs       RunStore
	runQueries RunQueryStore
	results    ResultStore
	gov        GovernanceStore
	versions   VersionStore
	lg         loggateway.Logger
}

// NewUsecase constructs an evaluation Usecase from independent store ports.
func NewUsecase(s Stores, lg loggateway.Logger) *Usecase {
	return &Usecase{
		datasets:   s.Datasets,
		cases:      s.Cases,
		runs:       s.Runs,
		runQueries: s.RunQueries,
		results:    s.Results,
		gov:        s.Governance,
		versions:   s.Versions,
		lg:         lg,
	}
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
	return u.datasets.CreateDataset(ctx, in)
}

// GetDataset returns one dataset.
func (u *Usecase) GetDataset(ctx context.Context, id string) (Dataset, error) {
	if strings.TrimSpace(id) == "" {
		return Dataset{}, apierror.BadRequest("EVAL", "id is required")
	}
	return u.datasets.GetDataset(ctx, id)
}

// ListDatasets returns datasets visible in the workspace.
func (u *Usecase) ListDatasets(ctx context.Context, workspace string, limit, offset int) ([]Dataset, int, error) {
	if limit <= 0 {
		limit = 20
	}
	return u.datasets.ListDatasets(ctx, workspace, limit, offset)
}

// DeleteDataset removes a dataset and its cases.
func (u *Usecase) DeleteDataset(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("EVAL", "id is required")
	}
	return u.datasets.DeleteDataset(ctx, id)
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
	return u.datasets.UpdateDataset(ctx, id, name, description)
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
	if err := u.cases.InsertCasesWithCountUpdate(ctx, datasetID, cases); err != nil {
		return 0, err
	}
	_, _ = u.SnapshotDataset(ctx, datasetID)
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
		in.Status = RunStatusPending
	}
	if err := ValidateTransition("", in.Status); err != nil {
		return Run{}, err
	}
	if strings.TrimSpace(in.TriggerSource) == "" {
		in.TriggerSource = "manual"
	}
	if in.NumRuns <= 0 {
		in.NumRuns = 1
	}
	in.Tools = strings.TrimSpace(in.Tools)
	if in.LeaseUntil == "" {
		in.LeaseUntil = NextLeaseUntil()
	}
	if err := u.bindRunDatasetVersion(ctx, &in); err != nil {
		return Run{}, err
	}
	return u.runs.CreateRun(ctx, in)
}

func (u *Usecase) bindRunDatasetVersion(ctx context.Context, in *Run) error {
	if in == nil {
		return nil
	}
	if vid := strings.TrimSpace(in.DatasetVersionID); vid != "" {
		v, err := u.GetDatasetVersion(ctx, vid)
		if err != nil {
			return err
		}
		if v.ID == "" {
			return apierror.NotFound("EVAL", "dataset version not found")
		}
		if v.DatasetID != in.DatasetID {
			return apierror.BadRequest("EVAL", "dataset_version_id does not belong to this dataset")
		}
		in.DatasetVersionID = v.ID
		in.DatasetVersion = v.Version
		in.DatasetHash = v.Hash
		return nil
	}
	if snap, err := u.SnapshotDataset(ctx, in.DatasetID); err == nil && snap.ID != "" {
		in.DatasetVersionID = snap.ID
		in.DatasetVersion = snap.Version
		in.DatasetHash = snap.Hash
	}
	return nil
}

// GetRun returns one run.
func (u *Usecase) GetRun(ctx context.Context, id string) (Run, error) {
	if strings.TrimSpace(id) == "" {
		return Run{}, apierror.BadRequest("EVAL", "id is required")
	}
	return u.runs.GetRun(ctx, id)
}

// ListRuns returns runs optionally filtered by dataset/agent.
func (u *Usecase) ListRuns(ctx context.Context, datasetID, agentID string, limit, offset int) ([]Run, int, error) {
	if limit <= 0 {
		limit = 20
	}
	return u.runs.ListRuns(ctx, datasetID, agentID, limit, offset)
}

// UpdateRun persists run progress/result changes after validating the
// status transition (AS-FSM-01). Same-status writes are allowed.
func (u *Usecase) UpdateRun(ctx context.Context, r Run) error {
	if r.ID != "" {
		if cur, err := u.runs.GetRun(ctx, r.ID); err == nil {
			if err := ValidateTransition(cur.Status, r.Status); err != nil {
				return err
			}
		}
	}
	// Progress writes carry the in-memory snapshot; refresh the lease
	// so a 30s-old LeaseUntil cannot overwrite a live heartbeat.
	if r.Status == RunStatusPending || r.Status == RunStatusRunning {
		r.LeaseUntil = NextLeaseUntil()
	}
	return u.runs.UpdateRun(ctx, r)
}

// StaleRunGrace is the fallback age for rows that never wrote a lease
// (pre-lease schema). Live runs refresh LeaseUntil; expired leases are
// swept regardless of created_at. olderThan<=0 still uses this grace
// for lease-less rows.
const StaleRunGrace = 15 * time.Minute

// LeaseTTL is how long an in-flight run remains immune to stale sweep.
const LeaseTTL = 2 * time.Minute

// NextLeaseUntil returns now+LeaseTTL in RFC3339 UTC.
func NextLeaseUntil() string {
	return time.Now().UTC().Add(LeaseTTL).Format(time.RFC3339)
}

// FailStaleRuns sweeps non-terminal runs whose lease expired (or, for
// lease-less rows, created_at older than olderThan). olderThan<=0 means
// StaleRunGrace for the lease-less fallback.
func (u *Usecase) FailStaleRuns(ctx context.Context, olderThan time.Duration) (int, error) {
	if olderThan <= 0 {
		olderThan = StaleRunGrace
	}
	return u.runs.FailStaleRuns(ctx, time.Now().Add(-olderThan))
}

// TouchRunLease refreshes the in-flight heartbeat so a live executor is
// not swept by another instance's startup cleaner.

// DeleteRun removes a run and its case results.

// ListCaseResults returns per-case results for a run.

// InsertCaseResult persists one case result.

// AnnotateCaseResult updates human review fields for one case result.

// ListCases returns all cases for a dataset.

// UpdateCase updates one case's input / expected output / metadata.

// DeleteCase removes one case and decrements dataset.case_count.

// inFlightScanLimit bounds the recent-run scan for FindInFlightRun.

// FindInFlightRun reports the newest pending/running run for dataset+agent.
// Used by manual RunEvaluation and AfterTurn to share one in-flight lock.

// FindInFlightExperimentCell reports an in-flight run for one matrix cell.
// Same agent may have several cells (model/prompt); the lock key is variant_label.

// CancelRun marks a pending/running run cancelled. Callers should also
// cancel the Runner context so the executor stops promptly.

// GetAgentEvalTrend returns recent completed runs for trend charts.

// CompareEvalRuns loads runs by ID and computes metric deltas vs the first run (baseline).

// ISSUE-004: baseline must be the earliest run by creation time, not
// whatever order the caller/store happened to return. CreatedAt is
// RFC3339, so lexicographic order matches chronological order.

// ── Scores helpers ────────────────────────────────────────────────────────────

// Scores holds arbitrary metric key → score mappings.

// ParseScores unmarshals scores_json; invalid input yields empty map.

// MarshalScores serializes scores to JSON object string.

// ── LLM Setting ───────────────────────────────────────────────────────────────

// LLMSetting holds platform defaults for evaluation UserSim and LLM-as-Judge.

// SimConfigured returns true if sim provider and model are set.

// JudgeConfigured returns true if judge provider and model are set.

// ApplyLLMPatch merges an update onto current eval LLM settings.
