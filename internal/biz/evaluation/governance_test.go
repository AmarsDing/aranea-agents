package evaluation

import (
	"context"
	"testing"
)

// P2-1/P2-3/P3-3 governance usecase tests (review BP14: error-path coverage).

func TestSubmitRunPreferenceValidation(t *testing.T) {
	twoRuns := []Run{
		{ID: "run-a", DatasetID: "ds-1"},
		{ID: "run-b", DatasetID: "ds-1"},
	}
	tests := []struct {
		name    string
		in      RunPreference
		repo    *mockRepo
		wantErr bool
	}{
		{
			name: "happy path persists with generated id and default creator",
			in:   RunPreference{DatasetID: "ds-1", RunIDA: "run-a", RunIDB: "run-b", WinnerRunID: "run-a"},
			repo: &mockRepo{getRunsByIDsRuns: twoRuns},
		},
		{
			name:    "missing fields rejected",
			in:      RunPreference{DatasetID: "ds-1", RunIDA: "run-a"},
			repo:    &mockRepo{getRunsByIDsRuns: twoRuns},
			wantErr: true,
		},
		{
			name:    "same run on both sides rejected",
			in:      RunPreference{DatasetID: "ds-1", RunIDA: "run-a", RunIDB: "run-a", WinnerRunID: "run-a"},
			repo:    &mockRepo{getRunsByIDsRuns: twoRuns},
			wantErr: true,
		},
		{
			name:    "winner outside the pair rejected",
			in:      RunPreference{DatasetID: "ds-1", RunIDA: "run-a", RunIDB: "run-b", WinnerRunID: "run-c"},
			repo:    &mockRepo{getRunsByIDsRuns: twoRuns},
			wantErr: true,
		},
		{
			name:    "missing run rejected",
			in:      RunPreference{DatasetID: "ds-1", RunIDA: "run-a", RunIDB: "run-b", WinnerRunID: "run-a"},
			repo:    &mockRepo{getRunsByIDsRuns: twoRuns[:1]},
			wantErr: true,
		},
		{
			name: "run from another dataset rejected",
			in:   RunPreference{DatasetID: "ds-1", RunIDA: "run-a", RunIDB: "run-b", WinnerRunID: "run-a"},
			repo: &mockRepo{getRunsByIDsRuns: []Run{
				{ID: "run-a", DatasetID: "ds-1"},
				{ID: "run-b", DatasetID: "ds-2"},
			}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewUsecase(StoresFrom(tt.repo), nil)
			p, err := uc.SubmitRunPreference(context.Background(), tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.ID == "" {
				t.Error("expected generated ID")
			}
			if p.CreatedBy != "system" {
				t.Errorf("expected default creator system, got %q", p.CreatedBy)
			}
			if len(tt.repo.preferences) != 1 {
				t.Fatalf("expected 1 persisted preference, got %d", len(tt.repo.preferences))
			}
		})
	}
}

func TestUpdateGateConfigValidation(t *testing.T) {
	t.Run("enabled without agent or dataset rejected", func(t *testing.T) {
		uc := NewUsecase(StoresFrom(&mockRepo{}), nil)
		if _, err := uc.UpdateGateConfig(context.Background(), GateConfig{Enabled: true}); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
	t.Run("scores clamped to [0,1] and metric defaulted", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(StoresFrom(repo), nil)
		cfg, err := uc.UpdateGateConfig(context.Background(), GateConfig{
			Enabled:   true,
			AgentID:   "agent-1",
			DatasetID: "ds-1",
			MinScore:  1.7,
			MaxDrop:   -0.2,
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Metric != "exact_match" {
			t.Errorf("expected default metric exact_match, got %q", cfg.Metric)
		}
		if cfg.MinScore != 1 || cfg.MaxDrop != 0 {
			t.Errorf("expected clamped 1/0, got %v/%v", cfg.MinScore, cfg.MaxDrop)
		}
	})
	t.Run("disabled config persists without targets", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(StoresFrom(repo), nil)
		if _, err := uc.UpdateGateConfig(context.Background(), GateConfig{Enabled: false}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestGetFailureGroupsValidation(t *testing.T) {
	uc := NewUsecase(StoresFrom(&mockRepo{}), nil)
	if _, err := uc.GetFailureGroups(context.Background(), "  ", "", 0); err == nil {
		t.Fatal("expected error for empty dataset_id, got nil")
	}
	report, err := uc.GetFailureGroups(context.Background(), "ds-1", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalFailed != 0 || len(report.Groups) != 0 {
		t.Errorf("expected empty report, got %+v", report)
	}
}
