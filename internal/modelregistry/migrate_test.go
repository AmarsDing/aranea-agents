package modelregistry

import (
	"context"
	"errors"
	"testing"
)

type stubPreviewBackend struct {
	rows    []ApplyRow
	rowErr  error
	stats   ApplyMigrationStats
	statErr error
}

func (s *stubPreviewBackend) ListProviderModels(_ context.Context) ([]ApplyRow, error) {
	return s.rows, s.rowErr
}

func (s *stubPreviewBackend) CountProviderBindings(_ context.Context, _ string) (ApplyMigrationStats, error) {
	return s.stats, s.statErr
}

func (s *stubPreviewBackend) SaveProviderModel(_ context.Context, _ ApplyRow) error {
	return nil
}

func (s *stubPreviewBackend) UpsertModelPricing(_ context.Context, _, _ string, _ MicroPricing, _ string) error {
	return nil
}

func (s *stubPreviewBackend) BatchApply(_ context.Context, _ []ApplyRow, _ []PricingUpsert) BatchApplyResult {
	return BatchApplyResult{}
}

func (s *stubPreviewBackend) MigrateProviderBindings(_ context.Context, _, _ string) (ApplyMigrationStats, error) {
	return ApplyMigrationStats{}, nil
}

func (s *stubPreviewBackend) BatchMigrateProviderBindings(_ context.Context, _ []ProviderMigrationRule, _ []string) BatchMigrationResult {
	return BatchMigrationResult{}
}

func TestPreviewMigration_NilBackend(t *testing.T) {
	preview, err := PreviewMigration(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(preview.Items) != 0 {
		t.Fatalf("expected empty items, got %d", len(preview.Items))
	}
}

func TestPreviewMigration_ListProviderModelsError(t *testing.T) {
	backend := &stubPreviewBackend{rowErr: errors.New("db error")}
	_, err := PreviewMigration(context.Background(), backend)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPreviewMigration_CountProviderBindingsError(t *testing.T) {
	backend := &stubPreviewBackend{
		rows:    []ApplyRow{{Provider: "gemini"}},
		statErr: errors.New("count error"),
	}
	_, err := PreviewMigration(context.Background(), backend)
	if err == nil {
		t.Fatal("expected error from CountProviderBindings")
	}
}

func TestPreviewMigration_FiltersZeroItems(t *testing.T) {
	backend := &stubPreviewBackend{
		rows:  []ApplyRow{},
		stats: ApplyMigrationStats{},
	}
	preview, err := PreviewMigration(context.Background(), backend)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 0 {
		t.Fatalf("expected zero items when all counts are zero, got %d", len(preview.Items))
	}
}

func TestPreviewMigration_WithBindings(t *testing.T) {
	backend := &stubPreviewBackend{
		rows:  []ApplyRow{{Provider: "gemini"}, {Provider: "gemini"}},
		stats: ApplyMigrationStats{Agents: 3, Sessions: 1},
	}
	preview, err := PreviewMigration(context.Background(), backend)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) == 0 {
		t.Fatal("expected at least one item with non-zero bindings")
	}
	found := false
	for _, item := range preview.Items {
		if item.LegacyProvider == "gemini" {
			found = true
			if item.LLMRows != 2 {
				t.Fatalf("expected 2 LLM rows for gemini, got %d", item.LLMRows)
			}
			if item.Agents != 3 {
				t.Fatalf("expected 3 agents, got %d", item.Agents)
			}
		}
	}
	if !found {
		t.Fatal("expected gemini migration item")
	}
}
