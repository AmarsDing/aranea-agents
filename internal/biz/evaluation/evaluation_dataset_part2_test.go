package evaluation

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"errors"
	"testing"
)

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
				if got.LeaseUntil == "" {
					t.Fatal("expected CreateRun to stamp LeaseUntil")
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
			uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
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
