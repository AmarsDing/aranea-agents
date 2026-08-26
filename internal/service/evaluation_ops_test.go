package service

import (
	"context"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/evaluation/v1"
	beval "aranea-agents/internal/biz/evaluation"
	"aranea-agents/internal/evaluation"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type evalOpsRepo struct {
	datasets map[string]beval.Dataset
	cases    map[string][]beval.Case
	runs     map[string]beval.Run
}

func newEvalOpsRepo() *evalOpsRepo {
	return &evalOpsRepo{
		datasets: map[string]beval.Dataset{
			"ds-own": {ID: "ds-own", Name: "own", Workspace: "ws-a", CaseCount: 1},
			"ds-oth": {ID: "ds-oth", Name: "oth", Workspace: "ws-b", CaseCount: 1},
		},
		cases: map[string][]beval.Case{
			"ds-own": {{ID: "c1", DatasetID: "ds-own", Input: "q"}},
		},
		runs: map[string]beval.Run{
			"r-live": {ID: "r-live", DatasetID: "ds-own", AgentID: "a1", Status: beval.RunStatusRunning, WorkspaceID: "ws-a"},
		},
	}
}

func (m *evalOpsRepo) CreateDataset(_ context.Context, d beval.Dataset) (beval.Dataset, error) {
	return d, nil
}
func (m *evalOpsRepo) GetDataset(_ context.Context, id string) (beval.Dataset, error) {
	d, ok := m.datasets[id]
	if !ok {
		return beval.Dataset{}, apierror.NotFound("EVAL", "dataset not found")
	}
	return d, nil
}
func (m *evalOpsRepo) ListDatasets(_ context.Context, _ string, _, _ int) ([]beval.Dataset, int, error) {
	return nil, 0, nil
}
func (m *evalOpsRepo) DeleteDataset(_ context.Context, _ string) error { return nil }
func (m *evalOpsRepo) UpdateDataset(_ context.Context, id, name, desc string) (beval.Dataset, error) {
	d := m.datasets[id]
	d.Name, d.Description = name, desc
	m.datasets[id] = d
	return d, nil
}
func (m *evalOpsRepo) InsertCasesWithCountUpdate(_ context.Context, _ string, _ []beval.Case) error {
	return nil
}
func (m *evalOpsRepo) ListCases(_ context.Context, datasetID string) ([]beval.Case, error) {
	return m.cases[datasetID], nil
}
func (m *evalOpsRepo) UpdateCase(_ context.Context, c beval.Case) (beval.Case, error) { return c, nil }
func (m *evalOpsRepo) DeleteCase(_ context.Context, _, _ string) error                { return nil }
func (m *evalOpsRepo) CreateRun(_ context.Context, r beval.Run) (beval.Run, error)    { return r, nil }
func (m *evalOpsRepo) GetRun(_ context.Context, id string) (beval.Run, error) {
	r, ok := m.runs[id]
	if !ok {
		return beval.Run{}, apierror.NotFound("EVAL", "run not found")
	}
	return r, nil
}
func (m *evalOpsRepo) UpdateRun(_ context.Context, r beval.Run) error {
	m.runs[r.ID] = r
	return nil
}
func (m *evalOpsRepo) DeleteRun(_ context.Context, _ string) error { return nil }
func (m *evalOpsRepo) ListRuns(_ context.Context, datasetID, agentID string, _, _ int) ([]beval.Run, int, error) {
	var out []beval.Run
	for _, r := range m.runs {
		if (datasetID == "" || r.DatasetID == datasetID) && (agentID == "" || r.AgentID == agentID) {
			out = append(out, r)
		}
	}
	return out, len(out), nil
}
func (m *evalOpsRepo) FailStaleRuns(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}
func (m *evalOpsRepo) InsertCaseResult(_ context.Context, _ beval.CaseResult) error { return nil }
func (m *evalOpsRepo) ListCaseResults(_ context.Context, _ string, _, _ int) ([]beval.CaseResult, int, error) {
	return nil, 0, nil
}
func (m *evalOpsRepo) GetCaseResult(_ context.Context, _, _ string) (beval.CaseResult, error) {
	return beval.CaseResult{}, nil
}
func (m *evalOpsRepo) UpdateCaseResultAnnotation(_ context.Context, _, _ string, _ beval.CaseResultAnnotation) (beval.CaseResult, error) {
	return beval.CaseResult{}, nil
}
func (m *evalOpsRepo) ListJudgeAnnotatedResults(_ context.Context, _, _ string) ([]beval.JudgeAnnotatedResult, error) {
	return nil, nil
}
func (m *evalOpsRepo) ListFailureGroups(_ context.Context, _, _ string, _ int) ([]beval.FailureGroup, int, error) {
	return nil, 0, nil
}
func (m *evalOpsRepo) ListTrendPoints(_ context.Context, _, _ string, _ int) ([]beval.TrendPoint, error) {
	return nil, nil
}
func (m *evalOpsRepo) GetRunsByIDs(_ context.Context, _ []string) ([]beval.Run, error) {
	return nil, nil
}
func (m *evalOpsRepo) InsertRunPreference(_ context.Context, _ beval.RunPreference) error { return nil }
func (m *evalOpsRepo) ListRunPreferences(_ context.Context, _ string, _ int) ([]beval.RunPreference, error) {
	return nil, nil
}
func (m *evalOpsRepo) GetGateConfig(_ context.Context, _ string) (beval.GateConfig, error) {
	return beval.GateConfig{}, nil
}
func (m *evalOpsRepo) UpsertGateConfig(_ context.Context, _ beval.GateConfig) error { return nil }

func newEvalOpsService(t *testing.T) (*EvaluationService, *evalOpsRepo) {
	t.Helper()
	repo := newEvalOpsRepo()
	uc := beval.NewUsecase(beval.StoresFrom(repo), loggateway.NewNoop())
	return NewEvaluationService(uc, evaluation.NewRunner(uc, nil, loggateway.NewNoop())), repo
}

func TestEvaluationService_ListCases_IDOR(t *testing.T) {
	svc, _ := newEvalOpsService(t)
	ctx := workspace.WithContext(context.Background(), "ws-a")
	if _, err := svc.ListCases(ctx, &v1.ListCasesRequest{DatasetId: "ds-oth"}); err == nil {
		t.Fatal("cross-workspace list must fail")
	}
	resp, err := svc.ListCases(ctx, &v1.ListCasesRequest{DatasetId: "ds-own"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetItems()) != 1 || resp.GetItems()[0].GetId() != "c1" {
		t.Fatalf("items = %+v", resp.GetItems())
	}
}

func TestEvaluationService_RunEvaluation_InFlight(t *testing.T) {
	svc, _ := newEvalOpsService(t)
	ctx := workspace.WithContext(context.Background(), "ws-a")
	_, err := svc.RunEvaluation(ctx, &v1.RunEvaluationRequest{DatasetId: "ds-own", AgentId: "a1"})
	if err == nil {
		t.Fatal("in-flight run must conflict")
	}
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("err = %v, want conflict", err)
	}
}

func TestEvaluationService_CancelRun(t *testing.T) {
	svc, repo := newEvalOpsService(t)
	ctx := workspace.WithContext(context.Background(), "ws-a")
	got, err := svc.CancelRun(ctx, &v1.CancelRunRequest{Id: "r-live"})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetStatus() != beval.RunStatusCancelled {
		t.Fatalf("status = %q", got.GetStatus())
	}
	if repo.runs["r-live"].Status != beval.RunStatusCancelled {
		t.Fatal("repo status not persisted")
	}
}
