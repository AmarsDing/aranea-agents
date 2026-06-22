package evaluation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type fnMockRepo struct {
	createDatasetFn              func(context.Context, Dataset) (Dataset, error)
	getDatasetFn                 func(context.Context, string) (Dataset, error)
	listDatasetsFn               func(context.Context, string, int, int) ([]Dataset, int, error)
	deleteDatasetFn              func(context.Context, string) error
	updateDatasetFn              func(context.Context, string, string, string) (Dataset, error)
	updateDatasetCaseCountFn     func(context.Context, string, int) error
	insertCasesFn                func(context.Context, []Case) error
	listCasesFn                  func(context.Context, string) ([]Case, error)
	createRunFn                  func(context.Context, Run) (Run, error)
	getRunFn                     func(context.Context, string) (Run, error)
	updateRunFn                  func(context.Context, Run) error
	deleteRunFn                  func(context.Context, string) error
	listRunsFn                   func(context.Context, string, string, int, int) ([]Run, int, error)
	insertCaseResultFn           func(context.Context, CaseResult) error
	listCaseResultsFn            func(context.Context, string, int, int) ([]CaseResult, int, error)
	getCaseResultFn              func(context.Context, string, string) (CaseResult, error)
	updateCaseResultAnnotationFn func(context.Context, string, string, CaseResultAnnotation) (CaseResult, error)
	listTrendPointsFn            func(context.Context, string, string, int) ([]TrendPoint, error)
	getRunsByIDsFn               func(context.Context, []string) ([]Run, error)
}

func (m *fnMockRepo) CreateDataset(ctx context.Context, d Dataset) (Dataset, error) {
	if m.createDatasetFn != nil {
		return m.createDatasetFn(ctx, d)
	}
	return d, nil
}

func (m *fnMockRepo) GetDataset(ctx context.Context, id string) (Dataset, error) {
	if m.getDatasetFn != nil {
		return m.getDatasetFn(ctx, id)
	}
	return Dataset{}, nil
}

func (m *fnMockRepo) ListDatasets(ctx context.Context, workspace string, limit, offset int) ([]Dataset, int, error) {
	if m.listDatasetsFn != nil {
		return m.listDatasetsFn(ctx, workspace, limit, offset)
	}
	return nil, 0, nil
}

func (m *fnMockRepo) DeleteDataset(ctx context.Context, id string) error {
	if m.deleteDatasetFn != nil {
		return m.deleteDatasetFn(ctx, id)
	}
	return nil
}

func (m *fnMockRepo) UpdateDataset(ctx context.Context, id, name, desc string) (Dataset, error) {
	if m.updateDatasetFn != nil {
		return m.updateDatasetFn(ctx, id, name, desc)
	}
	return Dataset{}, nil
}

func (m *fnMockRepo) UpdateDatasetCaseCount(ctx context.Context, id string, delta int) error {
	if m.updateDatasetCaseCountFn != nil {
		return m.updateDatasetCaseCountFn(ctx, id, delta)
	}
	return nil
}

func (m *fnMockRepo) InsertCases(ctx context.Context, cases []Case) error {
	if m.insertCasesFn != nil {
		return m.insertCasesFn(ctx, cases)
	}
	return nil
}

func (m *fnMockRepo) ListCases(ctx context.Context, datasetID string) ([]Case, error) {
	if m.listCasesFn != nil {
		return m.listCasesFn(ctx, datasetID)
	}
	return nil, nil
}

func (m *fnMockRepo) CreateRun(ctx context.Context, r Run) (Run, error) {
	if m.createRunFn != nil {
		return m.createRunFn(ctx, r)
	}
	return r, nil
}

func (m *fnMockRepo) GetRun(ctx context.Context, id string) (Run, error) {
	if m.getRunFn != nil {
		return m.getRunFn(ctx, id)
	}
	return Run{}, nil
}

func (m *fnMockRepo) UpdateRun(ctx context.Context, r Run) error {
	if m.updateRunFn != nil {
		return m.updateRunFn(ctx, r)
	}
	return nil
}

func (m *fnMockRepo) DeleteRun(ctx context.Context, id string) error {
	if m.deleteRunFn != nil {
		return m.deleteRunFn(ctx, id)
	}
	return nil
}

func (m *fnMockRepo) ListRuns(ctx context.Context, datasetID, agentID string, limit, offset int) ([]Run, int, error) {
	if m.listRunsFn != nil {
		return m.listRunsFn(ctx, datasetID, agentID, limit, offset)
	}
	return nil, 0, nil
}

func (m *fnMockRepo) InsertCaseResult(ctx context.Context, r CaseResult) error {
	if m.insertCaseResultFn != nil {
		return m.insertCaseResultFn(ctx, r)
	}
	return nil
}

func (m *fnMockRepo) ListCaseResults(ctx context.Context, runID string, limit, offset int) ([]CaseResult, int, error) {
	if m.listCaseResultsFn != nil {
		return m.listCaseResultsFn(ctx, runID, limit, offset)
	}
	return nil, 0, nil
}

func (m *fnMockRepo) GetCaseResult(ctx context.Context, runID, resultID string) (CaseResult, error) {
	if m.getCaseResultFn != nil {
		return m.getCaseResultFn(ctx, runID, resultID)
	}
	return CaseResult{}, nil
}

func (m *fnMockRepo) UpdateCaseResultAnnotation(ctx context.Context, runID, resultID string, patch CaseResultAnnotation) (CaseResult, error) {
	if m.updateCaseResultAnnotationFn != nil {
		return m.updateCaseResultAnnotationFn(ctx, runID, resultID, patch)
	}
	return CaseResult{}, nil
}

func (m *fnMockRepo) ListTrendPoints(ctx context.Context, agentID, datasetID string, limit int) ([]TrendPoint, error) {
	if m.listTrendPointsFn != nil {
		return m.listTrendPointsFn(ctx, agentID, datasetID, limit)
	}
	return nil, nil
}

func (m *fnMockRepo) GetRunsByIDs(ctx context.Context, ids []string) ([]Run, error) {
	if m.getRunsByIDsFn != nil {
		return m.getRunsByIDsFn(ctx, ids)
	}
	return nil, nil
}

func TestListDatasets(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		limit     int
		offset    int
		setup     func(*fnMockRepo)
		wantErr   bool
		check     func(t *testing.T, datasets []Dataset, total int)
	}{
		{
			name:      "default_limit_when_zero",
			workspace: "ws-1",
			limit:     0,
			setup: func(r *fnMockRepo) {
				r.listDatasetsFn = func(_ context.Context, _ string, limit, _ int) ([]Dataset, int, error) {
					if limit != 20 {
						return nil, 0, errors.New("limit not defaulted to 20")
					}
					return []Dataset{{ID: "ds-1"}}, 1, nil
				}
			},
			check: func(t *testing.T, datasets []Dataset, total int) {
				if len(datasets) != 1 {
					t.Fatalf("expected 1 dataset, got %d", len(datasets))
				}
				if total != 1 {
					t.Fatalf("expected total 1, got %d", total)
				}
			},
		},
		{
			name:      "default_limit_when_negative",
			workspace: "ws-1",
			limit:     -5,
			setup: func(r *fnMockRepo) {
				r.listDatasetsFn = func(_ context.Context, _ string, limit, _ int) ([]Dataset, int, error) {
					if limit != 20 {
						return nil, 0, errors.New("limit not defaulted to 20")
					}
					return []Dataset{{ID: "ds-1"}}, 1, nil
				}
			},
			check: func(t *testing.T, datasets []Dataset, total int) {
				if len(datasets) != 1 {
					t.Fatalf("expected 1 dataset, got %d", len(datasets))
				}
			},
		},
		{
			name:      "positive_limit_passed_through",
			workspace: "ws-1",
			limit:     50,
			offset:    10,
			setup: func(r *fnMockRepo) {
				r.listDatasetsFn = func(_ context.Context, _ string, limit, offset int) ([]Dataset, int, error) {
					if limit != 50 {
						return nil, 0, errors.New("limit not passed through")
					}
					if offset != 10 {
						return nil, 0, errors.New("offset not passed through")
					}
					return []Dataset{{ID: "ds-1"}, {ID: "ds-2"}}, 5, nil
				}
			},
			check: func(t *testing.T, datasets []Dataset, total int) {
				if len(datasets) != 2 {
					t.Fatalf("expected 2 datasets, got %d", len(datasets))
				}
				if total != 5 {
					t.Fatalf("expected total 5, got %d", total)
				}
			},
		},
		{
			name:      "repo_error",
			workspace: "ws-1",
			limit:     10,
			setup: func(r *fnMockRepo) {
				r.listDatasetsFn = func(_ context.Context, _ string, _, _ int) ([]Dataset, int, error) {
					return nil, 0, errors.New("db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ []Dataset, _ int) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			datasets, total, err := uc.ListDatasets(context.Background(), tt.workspace, tt.limit, tt.offset)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, datasets, total)
			}
		})
	}
}

func TestGetRun(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		setup   func(*fnMockRepo)
		wantErr bool
		check   func(t *testing.T, got Run)
	}{
		{
			name:    "empty_id_returns_error",
			id:      "",
			wantErr: true,
			check:   func(t *testing.T, _ Run) {},
		},
		{
			name:    "whitespace_id_returns_error",
			id:      "   ",
			wantErr: true,
			check:   func(t *testing.T, _ Run) {},
		},
		{
			name: "returns_run_from_repo",
			id:   "run-1",
			setup: func(r *fnMockRepo) {
				r.getRunFn = func(_ context.Context, id string) (Run, error) {
					return Run{ID: id, Status: "completed", DatasetID: "ds-1"}, nil
				}
			},
			check: func(t *testing.T, got Run) {
				if got.ID != "run-1" {
					t.Fatalf("expected ID 'run-1', got %q", got.ID)
				}
				if got.Status != "completed" {
					t.Fatalf("expected Status 'completed', got %q", got.Status)
				}
				if got.DatasetID != "ds-1" {
					t.Fatalf("expected DatasetID 'ds-1', got %q", got.DatasetID)
				}
			},
		},
		{
			name: "repo_error",
			id:   "run-1",
			setup: func(r *fnMockRepo) {
				r.getRunFn = func(_ context.Context, _ string) (Run, error) {
					return Run{}, errors.New("db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ Run) {},
		},
		{
			name:    "empty_id_bad_request_reason",
			id:      "",
			wantErr: true,
			check:   func(t *testing.T, _ Run) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			got, err := uc.GetRun(context.Background(), tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.name == "empty_id_bad_request_reason" || tt.name == "empty_id_returns_error" || tt.name == "whitespace_id_returns_error" {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror, got %T", err)
					}
					if se.Domain != "EVAL" {
						t.Fatalf("expected domain 'EVAL', got %q", se.Domain)
					}
					if se.Code != apierror.CodeBadRequest {
						t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, se.Code)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestListRuns(t *testing.T) {
	tests := []struct {
		name      string
		datasetID string
		agentID   string
		limit     int
		offset    int
		setup     func(*fnMockRepo)
		wantErr   bool
		check     func(t *testing.T, runs []Run, total int)
	}{
		{
			name:      "default_limit_when_zero",
			datasetID: "ds-1",
			agentID:   "agent-1",
			limit:     0,
			setup: func(r *fnMockRepo) {
				r.listRunsFn = func(_ context.Context, _, _ string, limit, _ int) ([]Run, int, error) {
					if limit != 20 {
						return nil, 0, errors.New("limit not defaulted to 20")
					}
					return []Run{{ID: "r1"}}, 1, nil
				}
			},
			check: func(t *testing.T, runs []Run, total int) {
				if len(runs) != 1 {
					t.Fatalf("expected 1 run, got %d", len(runs))
				}
			},
		},
		{
			name:      "default_limit_when_negative",
			datasetID: "ds-1",
			agentID:   "",
			limit:     -1,
			setup: func(r *fnMockRepo) {
				r.listRunsFn = func(_ context.Context, _, _ string, limit, _ int) ([]Run, int, error) {
					if limit != 20 {
						return nil, 0, errors.New("limit not defaulted to 20")
					}
					return []Run{{ID: "r1"}}, 1, nil
				}
			},
			check: func(t *testing.T, runs []Run, total int) {
				if len(runs) != 1 {
					t.Fatalf("expected 1 run, got %d", len(runs))
				}
			},
		},
		{
			name:      "positive_limit_passed_through",
			datasetID: "ds-1",
			agentID:   "agent-1",
			limit:     100,
			offset:    20,
			setup: func(r *fnMockRepo) {
				r.listRunsFn = func(_ context.Context, datasetID, agentID string, limit, offset int) ([]Run, int, error) {
					if datasetID != "ds-1" {
						return nil, 0, errors.New("datasetID not passed")
					}
					if agentID != "agent-1" {
						return nil, 0, errors.New("agentID not passed")
					}
					if limit != 100 {
						return nil, 0, errors.New("limit not passed")
					}
					if offset != 20 {
						return nil, 0, errors.New("offset not passed")
					}
					return []Run{{ID: "r1"}, {ID: "r2"}}, 10, nil
				}
			},
			check: func(t *testing.T, runs []Run, total int) {
				if len(runs) != 2 {
					t.Fatalf("expected 2 runs, got %d", len(runs))
				}
				if total != 10 {
					t.Fatalf("expected total 10, got %d", total)
				}
			},
		},
		{
			name:      "repo_error",
			datasetID: "ds-1",
			limit:     10,
			setup: func(r *fnMockRepo) {
				r.listRunsFn = func(_ context.Context, _, _ string, _, _ int) ([]Run, int, error) {
					return nil, 0, errors.New("db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ []Run, _ int) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			runs, total, err := uc.ListRuns(context.Background(), tt.datasetID, tt.agentID, tt.limit, tt.offset)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, runs, total)
			}
		})
	}
}

func TestUpdateRun(t *testing.T) {
	tests := []struct {
		name    string
		input   Run
		setup   func(*fnMockRepo)
		wantErr bool
	}{
		{
			name:  "successful_update",
			input: Run{ID: "run-1", Status: "completed"},
			setup: func(r *fnMockRepo) {
				r.updateRunFn = func(_ context.Context, run Run) error {
					if run.ID != "run-1" {
						return errors.New("wrong run ID")
					}
					if run.Status != "completed" {
						return errors.New("wrong status")
					}
					return nil
				}
			},
		},
		{
			name:  "repo_error",
			input: Run{ID: "run-1"},
			setup: func(r *fnMockRepo) {
				r.updateRunFn = func(_ context.Context, _ Run) error {
					return errors.New("db error")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			err := uc.UpdateRun(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDeleteRun(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		setup   func(*fnMockRepo)
		wantErr bool
		check   func(t *testing.T)
	}{
		{
			name: "valid_delete",
			id:   "run-1",
			setup: func(r *fnMockRepo) {
				r.deleteRunFn = func(_ context.Context, id string) error {
					if id != "run-1" {
						return errors.New("wrong ID")
					}
					return nil
				}
			},
			check: func(t *testing.T) {},
		},
		{
			name:    "empty_id_returns_error",
			id:      "",
			wantErr: true,
			check:   func(t *testing.T) {},
		},
		{
			name:    "whitespace_id_returns_error",
			id:      "   ",
			wantErr: true,
			check:   func(t *testing.T) {},
		},
		{
			name: "repo_error",
			id:   "run-1",
			setup: func(r *fnMockRepo) {
				r.deleteRunFn = func(_ context.Context, _ string) error {
					return apierror.Internal("EVAL", "db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			err := uc.DeleteRun(context.Background(), tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				se, ok := apierror.From(err)
				if !ok {
					t.Fatalf("expected apierror, got %T", err)
				}
				if tt.id == "" || tt.id == "   " {
					if se.Domain != "EVAL" {
						t.Fatalf("expected domain 'EVAL', got %q", se.Domain)
					}
					if se.Code != apierror.CodeBadRequest {
						t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, se.Code)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t)
			}
		})
	}
}

func TestListCaseResults(t *testing.T) {
	tests := []struct {
		name    string
		runID   string
		limit   int
		offset  int
		setup   func(*fnMockRepo)
		wantErr bool
		check   func(t *testing.T, results []CaseResult, total int)
	}{
		{
			name:  "default_limit_when_zero",
			runID: "run-1",
			limit: 0,
			setup: func(r *fnMockRepo) {
				r.listCaseResultsFn = func(_ context.Context, _ string, limit, _ int) ([]CaseResult, int, error) {
					if limit != 50 {
						return nil, 0, errors.New("limit not defaulted to 50")
					}
					return []CaseResult{{ID: "cr-1"}}, 1, nil
				}
			},
			check: func(t *testing.T, results []CaseResult, total int) {
				if len(results) != 1 {
					t.Fatalf("expected 1 result, got %d", len(results))
				}
			},
		},
		{
			name:  "default_limit_when_negative",
			runID: "run-1",
			limit: -1,
			setup: func(r *fnMockRepo) {
				r.listCaseResultsFn = func(_ context.Context, _ string, limit, _ int) ([]CaseResult, int, error) {
					if limit != 50 {
						return nil, 0, errors.New("limit not defaulted to 50")
					}
					return []CaseResult{{ID: "cr-1"}}, 1, nil
				}
			},
			check: func(t *testing.T, results []CaseResult, total int) {
				if len(results) != 1 {
					t.Fatalf("expected 1 result, got %d", len(results))
				}
			},
		},
		{
			name:   "positive_limit_passed_through",
			runID:  "run-1",
			limit:  100,
			offset: 25,
			setup: func(r *fnMockRepo) {
				r.listCaseResultsFn = func(_ context.Context, runID string, limit, offset int) ([]CaseResult, int, error) {
					if runID != "run-1" {
						return nil, 0, errors.New("runID not passed")
					}
					if limit != 100 {
						return nil, 0, errors.New("limit not passed")
					}
					if offset != 25 {
						return nil, 0, errors.New("offset not passed")
					}
					return []CaseResult{{ID: "cr-1"}, {ID: "cr-2"}}, 5, nil
				}
			},
			check: func(t *testing.T, results []CaseResult, total int) {
				if len(results) != 2 {
					t.Fatalf("expected 2 results, got %d", len(results))
				}
				if total != 5 {
					t.Fatalf("expected total 5, got %d", total)
				}
			},
		},
		{
			name:  "repo_error",
			runID: "run-1",
			limit: 10,
			setup: func(r *fnMockRepo) {
				r.listCaseResultsFn = func(_ context.Context, _ string, _, _ int) ([]CaseResult, int, error) {
					return nil, 0, errors.New("db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ []CaseResult, _ int) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			results, total, err := uc.ListCaseResults(context.Background(), tt.runID, tt.limit, tt.offset)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, results, total)
			}
		})
	}
}

func TestInsertCaseResult(t *testing.T) {
	tests := []struct {
		name    string
		input   CaseResult
		setup   func(*fnMockRepo)
		wantErr bool
	}{
		{
			name:  "successful_insert",
			input: CaseResult{ID: "cr-1", RunID: "run-1", CaseID: "case-1", ActualOutput: "result"},
			setup: func(r *fnMockRepo) {
				r.insertCaseResultFn = func(_ context.Context, cr CaseResult) error {
					if cr.ID != "cr-1" {
						return errors.New("wrong ID")
					}
					if cr.RunID != "run-1" {
						return errors.New("wrong RunID")
					}
					return nil
				}
			},
		},
		{
			name:  "repo_error",
			input: CaseResult{ID: "cr-1", RunID: "run-1"},
			setup: func(r *fnMockRepo) {
				r.insertCaseResultFn = func(_ context.Context, _ CaseResult) error {
					return errors.New("db error")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			err := uc.InsertCaseResult(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAnnotateCaseResult(t *testing.T) {
	tests := []struct {
		name     string
		runID    string
		resultID string
		patch    CaseResultAnnotation
		setup    func(*fnMockRepo)
		wantErr  bool
		check    func(t *testing.T, got CaseResult)
	}{
		{
			name:     "empty_runID_returns_error",
			runID:    "",
			resultID: "cr-1",
			patch:    CaseResultAnnotation{},
			wantErr:  true,
			check:    func(t *testing.T, _ CaseResult) {},
		},
		{
			name:     "empty_resultID_returns_error",
			runID:    "run-1",
			resultID: "",
			patch:    CaseResultAnnotation{},
			wantErr:  true,
			check:    func(t *testing.T, _ CaseResult) {},
		},
		{
			name:     "whitespace_runID_returns_error",
			runID:    "   ",
			resultID: "cr-1",
			patch:    CaseResultAnnotation{},
			wantErr:  true,
			check:    func(t *testing.T, _ CaseResult) {},
		},
		{
			name:     "whitespace_resultID_returns_error",
			runID:    "run-1",
			resultID: "   ",
			patch:    CaseResultAnnotation{},
			wantErr:  true,
			check:    func(t *testing.T, _ CaseResult) {},
		},
		{
			name:     "default_annotated_by",
			runID:    "run-1",
			resultID: "cr-1",
			patch:    CaseResultAnnotation{HumanPass: boolPtr(true)},
			setup: func(r *fnMockRepo) {
				r.updateCaseResultAnnotationFn = func(_ context.Context, _, _ string, patch CaseResultAnnotation) (CaseResult, error) {
					if patch.AnnotatedBy != "system" {
						return CaseResult{}, errors.New("expected annotated_by 'system'")
					}
					return CaseResult{ID: "cr-1", AnnotatedBy: patch.AnnotatedBy}, nil
				}
			},
			check: func(t *testing.T, got CaseResult) {
				if got.AnnotatedBy != "system" {
					t.Fatalf("expected AnnotatedBy 'system', got %q", got.AnnotatedBy)
				}
			},
		},
		{
			name:     "annotated_by_preserved",
			runID:    "run-1",
			resultID: "cr-1",
			patch:    CaseResultAnnotation{AnnotatedBy: "alice", HumanPass: boolPtr(false)},
			setup: func(r *fnMockRepo) {
				r.updateCaseResultAnnotationFn = func(_ context.Context, _, _ string, patch CaseResultAnnotation) (CaseResult, error) {
					if patch.AnnotatedBy != "alice" {
						return CaseResult{}, errors.New("expected annotated_by 'alice'")
					}
					return CaseResult{ID: "cr-1", AnnotatedBy: patch.AnnotatedBy}, nil
				}
			},
			check: func(t *testing.T, got CaseResult) {
				if got.AnnotatedBy != "alice" {
					t.Fatalf("expected AnnotatedBy 'alice', got %q", got.AnnotatedBy)
				}
			},
		},
		{
			name:     "repo_error",
			runID:    "run-1",
			resultID: "cr-1",
			patch:    CaseResultAnnotation{},
			setup: func(r *fnMockRepo) {
				r.updateCaseResultAnnotationFn = func(_ context.Context, _, _ string, _ CaseResultAnnotation) (CaseResult, error) {
					return CaseResult{}, errors.New("db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ CaseResult) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			got, err := uc.AnnotateCaseResult(context.Background(), tt.runID, tt.resultID, tt.patch)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if strings.TrimSpace(tt.runID) == "" || strings.TrimSpace(tt.resultID) == "" {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror, got %T", err)
					}
					if se.Domain != "EVAL" {
						t.Fatalf("expected domain 'EVAL', got %q", se.Domain)
					}
					if se.Code != apierror.CodeBadRequest {
						t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, se.Code)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestListCases(t *testing.T) {
	tests := []struct {
		name      string
		datasetID string
		setup     func(*fnMockRepo)
		wantErr   bool
		check     func(t *testing.T, cases []Case)
	}{
		{
			name:      "returns_cases_from_repo",
			datasetID: "ds-1",
			setup: func(r *fnMockRepo) {
				r.listCasesFn = func(_ context.Context, datasetID string) ([]Case, error) {
					if datasetID != "ds-1" {
						return nil, errors.New("wrong datasetID")
					}
					return []Case{
						{ID: "case-1", DatasetID: "ds-1", Input: "q1"},
						{ID: "case-2", DatasetID: "ds-1", Input: "q2"},
					}, nil
				}
			},
			check: func(t *testing.T, cases []Case) {
				if len(cases) != 2 {
					t.Fatalf("expected 2 cases, got %d", len(cases))
				}
				if cases[0].Input != "q1" {
					t.Fatalf("expected first case input 'q1', got %q", cases[0].Input)
				}
			},
		},
		{
			name:      "repo_error",
			datasetID: "ds-1",
			setup: func(r *fnMockRepo) {
				r.listCasesFn = func(_ context.Context, _ string) ([]Case, error) {
					return nil, errors.New("db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ []Case) {},
		},
		{
			name:      "empty_result",
			datasetID: "ds-empty",
			setup: func(r *fnMockRepo) {
				r.listCasesFn = func(_ context.Context, _ string) ([]Case, error) {
					return nil, nil
				}
			},
			check: func(t *testing.T, cases []Case) {
				if cases != nil {
					t.Fatalf("expected nil, got %v", cases)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			cases, err := uc.ListCases(context.Background(), tt.datasetID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, cases)
			}
		})
	}
}

func TestGetAgentEvalTrend(t *testing.T) {
	tests := []struct {
		name      string
		agentID   string
		datasetID string
		limit     int
		setup     func(*fnMockRepo)
		wantErr   bool
		check     func(t *testing.T, points []TrendPoint)
	}{
		{
			name:    "empty_agentID_returns_error",
			agentID: "",
			wantErr: true,
			check:   func(t *testing.T, _ []TrendPoint) {},
		},
		{
			name:    "whitespace_agentID_returns_error",
			agentID: "   ",
			wantErr: true,
			check:   func(t *testing.T, _ []TrendPoint) {},
		},
		{
			name:      "default_limit_when_zero",
			agentID:   "agent-1",
			datasetID: "ds-1",
			limit:     0,
			setup: func(r *fnMockRepo) {
				r.listTrendPointsFn = func(_ context.Context, _, _ string, limit int) ([]TrendPoint, error) {
					if limit != 30 {
						return nil, errors.New("limit not defaulted to 30")
					}
					return []TrendPoint{{RunID: "r1"}}, nil
				}
			},
			check: func(t *testing.T, points []TrendPoint) {
				if len(points) != 1 {
					t.Fatalf("expected 1 point, got %d", len(points))
				}
			},
		},
		{
			name:      "default_limit_when_negative",
			agentID:   "agent-1",
			datasetID: "",
			limit:     -5,
			setup: func(r *fnMockRepo) {
				r.listTrendPointsFn = func(_ context.Context, _, _ string, limit int) ([]TrendPoint, error) {
					if limit != 30 {
						return nil, errors.New("limit not defaulted to 30")
					}
					return []TrendPoint{{RunID: "r1"}}, nil
				}
			},
			check: func(t *testing.T, points []TrendPoint) {
				if len(points) != 1 {
					t.Fatalf("expected 1 point, got %d", len(points))
				}
			},
		},
		{
			name:      "positive_limit_passed_through",
			agentID:   "agent-1",
			datasetID: "ds-1",
			limit:     10,
			setup: func(r *fnMockRepo) {
				r.listTrendPointsFn = func(_ context.Context, agentID, datasetID string, limit int) ([]TrendPoint, error) {
					if agentID != "agent-1" {
						return nil, errors.New("agentID not passed")
					}
					if datasetID != "ds-1" {
						return nil, errors.New("datasetID not passed")
					}
					if limit != 10 {
						return nil, errors.New("limit not passed")
					}
					return []TrendPoint{
						{RunID: "r1", ExactMatchScore: 0.8},
						{RunID: "r2", ExactMatchScore: 0.9},
					}, nil
				}
			},
			check: func(t *testing.T, points []TrendPoint) {
				if len(points) != 2 {
					t.Fatalf("expected 2 points, got %d", len(points))
				}
			},
		},
		{
			name:    "repo_error",
			agentID: "agent-1",
			limit:   10,
			setup: func(r *fnMockRepo) {
				r.listTrendPointsFn = func(_ context.Context, _, _ string, _ int) ([]TrendPoint, error) {
					return nil, apierror.Internal("EVAL", "db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ []TrendPoint) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			points, err := uc.GetAgentEvalTrend(context.Background(), tt.agentID, tt.datasetID, tt.limit)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				se, ok := apierror.From(err)
				if !ok {
					t.Fatalf("expected apierror, got %T", err)
				}
				if tt.agentID == "" || tt.agentID == "   " {
					if se.Domain != "EVAL" {
						t.Fatalf("expected domain 'EVAL', got %q", se.Domain)
					}
					if se.Code != apierror.CodeBadRequest {
						t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, se.Code)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, points)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
