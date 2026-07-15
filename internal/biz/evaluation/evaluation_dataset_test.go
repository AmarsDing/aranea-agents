package evaluation

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type dsMockRepo struct {
	createDatasetFn func(context.Context, Dataset) (Dataset, error)
	getDatasetFn    func(context.Context, string) (Dataset, error)
	updateDatasetFn func(context.Context, string, string, string) (Dataset, error)
	deleteDatasetFn func(context.Context, string) error
	createRunFn     func(context.Context, Run) (Run, error)
}

func (m *dsMockRepo) CreateDataset(ctx context.Context, d Dataset) (Dataset, error) {
	if m.createDatasetFn != nil {
		return m.createDatasetFn(ctx, d)
	}
	return d, nil
}

func (m *dsMockRepo) GetDataset(ctx context.Context, id string) (Dataset, error) {
	if m.getDatasetFn != nil {
		return m.getDatasetFn(ctx, id)
	}
	return Dataset{}, nil
}

func (m *dsMockRepo) ListDatasets(_ context.Context, _ string, _, _ int) ([]Dataset, int, error) {
	return nil, 0, nil
}

func (m *dsMockRepo) DeleteDataset(ctx context.Context, id string) error {
	if m.deleteDatasetFn != nil {
		return m.deleteDatasetFn(ctx, id)
	}
	return nil
}

func (m *dsMockRepo) UpdateDataset(ctx context.Context, id, name, desc string) (Dataset, error) {
	if m.updateDatasetFn != nil {
		return m.updateDatasetFn(ctx, id, name, desc)
	}
	return Dataset{}, nil
}

func (m *dsMockRepo) UpdateDatasetCaseCount(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *dsMockRepo) InsertCases(_ context.Context, _ []Case) error {
	return nil
}

func (m *dsMockRepo) InsertCasesWithCountUpdate(_ context.Context, _ string, _ []Case) error {
	return nil
}

func (m *dsMockRepo) ListCases(_ context.Context, _ string) ([]Case, error) {
	return nil, nil
}

func (m *dsMockRepo) CreateRun(ctx context.Context, r Run) (Run, error) {
	if m.createRunFn != nil {
		return m.createRunFn(ctx, r)
	}
	return r, nil
}

func (m *dsMockRepo) GetRun(_ context.Context, _ string) (Run, error) {
	return Run{}, nil
}

func (m *dsMockRepo) UpdateRun(_ context.Context, _ Run) error {
	return nil
}

func (m *dsMockRepo) DeleteRun(_ context.Context, _ string) error {
	return nil
}

func (m *dsMockRepo) ListRuns(_ context.Context, _, _ string, _, _ int) ([]Run, int, error) {
	return nil, 0, nil
}

func (m *dsMockRepo) InsertCaseResult(_ context.Context, _ CaseResult) error {
	return nil
}

func (m *dsMockRepo) ListCaseResults(_ context.Context, _ string, _, _ int) ([]CaseResult, int, error) {
	return nil, 0, nil
}

func (m *dsMockRepo) GetCaseResult(_ context.Context, _, _ string) (CaseResult, error) {
	return CaseResult{}, nil
}

func (m *dsMockRepo) UpdateCaseResultAnnotation(_ context.Context, _, _ string, _ CaseResultAnnotation) (CaseResult, error) {
	return CaseResult{}, nil
}

func (m *dsMockRepo) ListTrendPoints(_ context.Context, _, _ string, _ int) ([]TrendPoint, error) {
	return nil, nil
}

func (m *dsMockRepo) GetRunsByIDs(_ context.Context, _ []string) ([]Run, error) {
	return nil, nil
}

func TestCreateDataset(t *testing.T) {
	tests := []struct {
		name    string
		input   Dataset
		setup   func(*dsMockRepo)
		wantErr bool
		check   func(t *testing.T, got Dataset)
	}{
		{
			name: "valid_with_defaults",
			input: Dataset{
				Name:        "test-ds",
				Description: "desc",
				Workspace:   "ws-1",
			},
			setup: func(r *dsMockRepo) {
				r.createDatasetFn = func(_ context.Context, d Dataset) (Dataset, error) {
					return d, nil
				}
			},
			check: func(t *testing.T, got Dataset) {
				if got.ID == "" {
					t.Fatal("expected auto-generated ID, got empty")
				}
				if got.Name != "test-ds" {
					t.Fatalf("expected name 'test-ds', got %q", got.Name)
				}
				if got.CaseCount != 0 {
					t.Fatalf("expected CaseCount 0, got %d", got.CaseCount)
				}
			},
		},
		{
			name: "empty_name_returns_error",
			input: Dataset{
				Name: "",
			},
			wantErr: true,
			check:   func(t *testing.T, _ Dataset) {},
		},
		{
			name: "whitespace_name_returns_error",
			input: Dataset{
				Name: "   ",
			},
			wantErr: true,
			check:   func(t *testing.T, _ Dataset) {},
		},
		{
			name: "provided_id_preserved",
			input: Dataset{
				ID:   "custom-id",
				Name: "test-ds",
			},
			setup: func(r *dsMockRepo) {
				r.createDatasetFn = func(_ context.Context, d Dataset) (Dataset, error) {
					return d, nil
				}
			},
			check: func(t *testing.T, got Dataset) {
				if got.ID != "custom-id" {
					t.Fatalf("expected ID 'custom-id', got %q", got.ID)
				}
			},
		},
		{
			name: "repo_error_propagation",
			input: Dataset{
				Name: "test-ds",
			},
			setup: func(r *dsMockRepo) {
				r.createDatasetFn = func(_ context.Context, _ Dataset) (Dataset, error) {
					return Dataset{}, errors.New("db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ Dataset) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &dsMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			got, err := uc.CreateDataset(context.Background(), tt.input)
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
				tt.check(t, got)
			}
		})
	}
}

func TestGetDataset(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		setup   func(*dsMockRepo)
		wantErr bool
		check   func(t *testing.T, got Dataset)
	}{
		{
			name: "returns_dataset",
			id:   "ds-1",
			setup: func(r *dsMockRepo) {
				r.getDatasetFn = func(_ context.Context, id string) (Dataset, error) {
					return Dataset{ID: id, Name: "found"}, nil
				}
			},
			check: func(t *testing.T, got Dataset) {
				if got.ID != "ds-1" {
					t.Fatalf("expected ID 'ds-1', got %q", got.ID)
				}
				if got.Name != "found" {
					t.Fatalf("expected Name 'found', got %q", got.Name)
				}
			},
		},
		{
			name:    "empty_id_returns_error",
			id:      "",
			wantErr: true,
			check:   func(t *testing.T, _ Dataset) {},
		},
		{
			name:    "whitespace_id_returns_error",
			id:      "   ",
			wantErr: true,
			check:   func(t *testing.T, _ Dataset) {},
		},
		{
			name: "not_found",
			id:   "missing",
			setup: func(r *dsMockRepo) {
				r.getDatasetFn = func(_ context.Context, _ string) (Dataset, error) {
					return Dataset{}, errors.New("not found")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ Dataset) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &dsMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			got, err := uc.GetDataset(context.Background(), tt.id)
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
				tt.check(t, got)
			}
		})
	}
}

func TestUpdateDataset(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		updateName string
		updateDesc string
		setup      func(*dsMockRepo)
		wantErr    bool
		wantReason string
		wantCode   apierror.Code
		check      func(t *testing.T, got Dataset)
	}{
		{
			name:       "valid_update",
			id:         "ds-1",
			updateName: "new-name",
			updateDesc: "new-desc",
			setup: func(r *dsMockRepo) {
				r.updateDatasetFn = func(_ context.Context, id, name, desc string) (Dataset, error) {
					return Dataset{ID: id, Name: name, Description: desc}, nil
				}
			},
			check: func(t *testing.T, got Dataset) {
				if got.ID != "ds-1" {
					t.Fatalf("expected ID 'ds-1', got %q", got.ID)
				}
				if got.Name != "new-name" {
					t.Fatalf("expected Name 'new-name', got %q", got.Name)
				}
				if got.Description != "new-desc" {
					t.Fatalf("expected Description 'new-desc', got %q", got.Description)
				}
			},
		},
		{
			name:       "empty_id_returns_error",
			id:         "",
			updateName: "name",
			wantErr:    true,
			wantReason: "EVAL",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ Dataset) {},
		},
		{
			name:       "empty_name_returns_error",
			id:         "ds-1",
			updateName: "",
			wantErr:    true,
			wantReason: "EVAL",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ Dataset) {},
		},
		{
			name:       "repo_not_found",
			id:         "ds-missing",
			updateName: "name",
			setup: func(r *dsMockRepo) {
				r.updateDatasetFn = func(_ context.Context, _, _, _ string) (Dataset, error) {
					return Dataset{}, errors.New("not found")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ Dataset) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &dsMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			got, err := uc.UpdateDataset(context.Background(), tt.id, tt.updateName, tt.updateDesc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantReason != "" {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror, got %T", err)
					}
					if se.Domain != tt.wantReason {
						t.Fatalf("expected domain %q, got %q", tt.wantReason, se.Domain)
					}
					if se.Code != tt.wantCode {
						t.Fatalf("expected code %s, got %s", tt.wantCode, se.Code)
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

func TestDeleteDataset(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		setup   func(*dsMockRepo)
		wantErr bool
		check   func(t *testing.T)
	}{
		{
			name: "valid_delete",
			id:   "ds-1",
			setup: func(r *dsMockRepo) {
				r.deleteDatasetFn = func(_ context.Context, _ string) error {
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
			name: "not_found",
			id:   "ds-missing",
			setup: func(r *dsMockRepo) {
				r.deleteDatasetFn = func(_ context.Context, _ string) error {
					return errors.New("not found")
				}
			},
			wantErr: true,
			check:   func(t *testing.T) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &dsMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			err := uc.DeleteDataset(context.Background(), tt.id)
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
				tt.check(t)
			}
		})
	}
}

func TestCreateRun(t *testing.T) {
	tests := []struct {
		name    string
		input   Run
		setup   func(*dsMockRepo)
		wantErr bool
		check   func(t *testing.T, got Run)
	}{
		{
			name: "valid_with_defaults",
			input: Run{
				DatasetID: "ds-1",
				AgentID:   "agent-1",
			},
			setup: func(r *dsMockRepo) {
				r.createRunFn = func(_ context.Context, run Run) (Run, error) {
					return run, nil
				}
			},
			check: func(t *testing.T, got Run) {
				if got.ID == "" {
					t.Fatal("expected auto-generated ID, got empty")
				}
				if got.Status != "pending" {
					t.Fatalf("expected Status 'pending', got %q", got.Status)
				}
				if got.DatasetID != "ds-1" {
					t.Fatalf("expected DatasetID 'ds-1', got %q", got.DatasetID)
				}
				if got.AgentID != "agent-1" {
					t.Fatalf("expected AgentID 'agent-1', got %q", got.AgentID)
				}
			},
		},
		{
			name: "empty_dataset_id_returns_error",
			input: Run{
				DatasetID: "",
				AgentID:   "agent-1",
			},
			wantErr: true,
			check:   func(t *testing.T, _ Run) {},
		},
		{
			name: "empty_agent_id_returns_error",
			input: Run{
				DatasetID: "ds-1",
				AgentID:   "",
			},
			wantErr: true,
			check:   func(t *testing.T, _ Run) {},
		},
		{
			name: "default_trigger_source",
			input: Run{
				DatasetID: "ds-1",
				AgentID:   "agent-1",
			},
			setup: func(r *dsMockRepo) {
				r.createRunFn = func(_ context.Context, run Run) (Run, error) {
					return run, nil
				}
			},
			check: func(t *testing.T, got Run) {
				if got.TriggerSource != "manual" {
					t.Fatalf("expected TriggerSource 'manual', got %q", got.TriggerSource)
				}
			},
		},
		{
			name: "default_num_runs",
			input: Run{
				DatasetID: "ds-1",
				AgentID:   "agent-1",
				NumRuns:   0,
			},
			setup: func(r *dsMockRepo) {
				r.createRunFn = func(_ context.Context, run Run) (Run, error) {
					return run, nil
				}
			},
			check: func(t *testing.T, got Run) {
				if got.NumRuns != 1 {
					t.Fatalf("expected NumRuns 1, got %d", got.NumRuns)
				}
			},
		},
		{
			name: "provided_id_and_status_preserved",
			input: Run{
				ID:        "custom-run",
				DatasetID: "ds-1",
				AgentID:   "agent-1",
				Status:    "running",
			},
			setup: func(r *dsMockRepo) {
				r.createRunFn = func(_ context.Context, run Run) (Run, error) {
					return run, nil
				}
			},
			check: func(t *testing.T, got Run) {
				if got.ID != "custom-run" {
					t.Fatalf("expected ID 'custom-run', got %q", got.ID)
				}
				if got.Status != "running" {
					t.Fatalf("expected Status 'running', got %q", got.Status)
				}
			},
		},
		{
			name: "repo_error_propagation",
			input: Run{
				DatasetID: "ds-1",
				AgentID:   "agent-1",
			},
			setup: func(r *dsMockRepo) {
				r.createRunFn = func(_ context.Context, _ Run) (Run, error) {
					return Run{}, errors.New("db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ Run) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &dsMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, loggateway.NewNoop())
			got, err := uc.CreateRun(context.Background(), tt.input)
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
				tt.check(t, got)
			}
		})
	}
}
