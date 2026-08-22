package evaluation

import (
	"context"
	"time"
)

// Stores groups independent evaluation persistence ports (ISP).
// This is a DTO of single-aggregate interfaces, not a god interface: callers
// must use the field they need. Replaces the deprecated Repo aggregate field
// on the production path (Wire / Usecase).
type Stores struct {
	Datasets   DatasetStore
	Cases      CaseStore
	Runs       RunStore
	RunQueries RunQueryStore
	Results    ResultStore
	Governance GovernanceStore
	Versions   VersionStore
}

// StoresFrom maps one Repo implementer onto the ISP DTO (same adapter, many
// mouths). Nil r yields a zero Stores.
func StoresFrom(r Repo) Stores {
	if r == nil {
		return Stores{}
	}
	s := Stores{
		Datasets:   r,
		Cases:      r,
		Runs:       r,
		RunQueries: r,
		Results:    r,
		Governance: r,
	}
	if v, ok := r.(VersionStore); ok {
		s.Versions = v
	}
	return s
}

// DatasetStore persists evaluation datasets.
// Stability:evolving
type DatasetStore interface {
	CreateDataset(ctx context.Context, d Dataset) (Dataset, error)
	GetDataset(ctx context.Context, id string) (Dataset, error)
	ListDatasets(ctx context.Context, workspace string, limit, offset int) ([]Dataset, int, error)
	DeleteDataset(ctx context.Context, id string) error
	UpdateDataset(ctx context.Context, id, name, description string) (Dataset, error)
}

// CaseStore persists dataset cases.
// Stability:evolving
type CaseStore interface {
	// InsertCasesWithCountUpdate inserts cases and bumps dataset.case_count in a
	// single transaction. The two writes must stay atomic — a failure between
	// them would leave case_count diverged from the actual row count in
	// eval_cases (EVAL-06: the former separate InsertCases +
	// UpdateDatasetCaseCount pair was removed; it had no production caller).
	InsertCasesWithCountUpdate(ctx context.Context, datasetID string, cases []Case) error
	ListCases(ctx context.Context, datasetID string) ([]Case, error)
	UpdateCase(ctx context.Context, c Case) (Case, error)
	DeleteCase(ctx context.Context, datasetID, caseID string) error
}

// RunStore persists evaluation run lifecycle (CRUD + crash/orphan recovery).
// Stability:evolving
type RunStore interface {
	CreateRun(ctx context.Context, r Run) (Run, error)
	GetRun(ctx context.Context, id string) (Run, error)
	UpdateRun(ctx context.Context, r Run) error
	DeleteRun(ctx context.Context, id string) error
	ListRuns(ctx context.Context, datasetID, agentID string, limit, offset int) ([]Run, int, error)
	// FailStaleRuns marks every non-terminal run created before cutoff as
	// failed (crash/orphan recovery, Y10). Returns the swept row count.
	FailStaleRuns(ctx context.Context, cutoff time.Time) (int, error)
}

// RunQueryStore reads runs for trend/compare surfaces.
// Stability:evolving
type RunQueryStore interface {
	ListTrendPoints(ctx context.Context, agentID, datasetID string, limit int) ([]TrendPoint, error)
	GetRunsByIDs(ctx context.Context, ids []string) ([]Run, error)
}

// ResultStore persists per-case results, human annotations, and result
// aggregations (judge calibration / failure grouping).
// Stability:evolving
type ResultStore interface {
	InsertCaseResult(ctx context.Context, r CaseResult) error
	ListCaseResults(ctx context.Context, runID string, limit, offset int) ([]CaseResult, int, error)
	GetCaseResult(ctx context.Context, runID, resultID string) (CaseResult, error)
	UpdateCaseResultAnnotation(ctx context.Context, runID, resultID string, patch CaseResultAnnotation) (CaseResult, error)
	// ListJudgeAnnotatedResults returns results with human_pass set (joined
	// with run dataset/agent scope and case text) for judge calibration (P1-3).
	ListJudgeAnnotatedResults(ctx context.Context, datasetID, agentID string) ([]JudgeAnnotatedResult, error)
	// ListFailureGroups aggregates case-result failures by error_message for
	// one dataset (P2-3, SQL-version failure clustering).
	ListFailureGroups(ctx context.Context, datasetID, agentID string, limit int) ([]FailureGroup, int, error)
}

// GovernanceStore persists publish-gate config and pairwise run preferences.
// Stability:evolving
type GovernanceStore interface {
	// InsertRunPreference / ListRunPreferences persist pairwise human
	// preferences between two runs of one dataset (P3-3).
	InsertRunPreference(ctx context.Context, p RunPreference) error
	ListRunPreferences(ctx context.Context, datasetID string, limit int) ([]RunPreference, error)
	// GetGateConfig / UpsertGateConfig read/write publish-gate config.
	// agentID="" is the platform default (legacy singleton). A non-empty
	// agentID prefers that agent's row and falls back to the default.
	GetGateConfig(ctx context.Context, agentID string) (GateConfig, error)
	UpsertGateConfig(ctx context.Context, cfg GateConfig) error
}

// Repo is the composed persistence port for evaluation.
//
// Deprecated: This composed interface has grown too large (dataset/case/run/
// result/governance). Production paths (Wire, Usecase) no longer depend on this
// type. New code must use Stores or the fine-grained sub-interfaces
// (DatasetStore, CaseStore, RunStore, RunQueryStore, ResultStore,
// GovernanceStore). Retained only for tests and the data-layer adapter
// compile-time check.
type Repo interface {
	DatasetStore
	CaseStore
	RunStore
	RunQueryStore
	ResultStore
	GovernanceStore
}
