package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/migrate"
	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"
)

func openTestDataWithRawDB(t *testing.T) *Data {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("pragma fk: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background(), migrate.WithDropIndex(true)); err != nil {
		t.Fatalf("schema create: %v", err)
	}
	return &Data{entClient: client, readClient: client, rawDB: db, readDB: db, rw: NewReadWriteClient(client, client), rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop()}
}

func seedAgent(t *testing.T, d *Data, id, agentKey, displayName, provider string) {
	t.Helper()
	ctx := context.Background()
	_, err := d.RWDB().WriteHandle().ExecContext(ctx,
		`INSERT INTO agents (id, agent_key, display_name, provider, model, status, kind, position_key, agent_variant, created_at, updated_at)
		 VALUES (?, ?, ?, ?, '', 'active', 'user', ?, 'general', datetime('now'), datetime('now'))`,
		id, agentKey, displayName, provider, agentKey,
	)
	if err != nil {
		t.Fatalf("seed agent %s: %v", id, err)
	}
}

func TestBatchApply_UpdatesRows(t *testing.T) {
	d := openTestDataWithRawDB(t)
	ctx := context.Background()

	_, err := d.RWDB().WriteHandle().ExecContext(ctx,
		`INSERT INTO llm_provider_models (id, provider, model_key, name, model, enabled, config_json, metadata_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"pm-1", "openai", "openai:gpt-4o", "GPT-4o", "gpt-4o", true, `{"catalog_managed":true}`, `{}`,
	)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	backend := &modelRegistryApplyBackend{data: d, llm: nil}
	patches := []modelregistry.ApplyRow{
		{ID: "pm-1", Key: "openai:gpt-4o", Name: "GPT-4o Updated", Provider: "openai", Model: "gpt-4o", Enabled: true, ConfigJSON: `{"catalog_managed":true}`, MetadataJSON: `{"updated":true}`},
	}
	result := backend.BatchApply(ctx, patches, nil)
	if result.RowsUpdated != 1 {
		t.Errorf("expected 1 row updated, got %d", result.RowsUpdated)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	var name string
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx, `SELECT name FROM llm_provider_models WHERE id = ?`, "pm-1").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "GPT-4o Updated" {
		t.Errorf("expected name 'GPT-4o Updated', got %q", name)
	}
}

func TestBatchApply_MixedSuccessAndError(t *testing.T) {
	d := openTestDataWithRawDB(t)
	ctx := context.Background()

	_, err := d.RWDB().WriteHandle().ExecContext(ctx,
		`INSERT INTO llm_provider_models (id, provider, model_key, name, model, enabled, config_json, metadata_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"pm-1", "openai", "openai:gpt-4o", "GPT-4o", "gpt-4o", true, `{}`, `{}`,
	)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	backend := &modelRegistryApplyBackend{data: d, llm: nil}
	patches := []modelregistry.ApplyRow{
		{ID: "pm-1", Key: "openai:gpt-4o", Name: "Updated", Provider: "openai", Model: "gpt-4o", Enabled: true, ConfigJSON: `{}`, MetadataJSON: `{}`},
		{ID: "pm-999", Key: "x:y", Name: "Ghost", Provider: "x", Model: "y", Enabled: true, ConfigJSON: `{}`, MetadataJSON: `{}`},
	}
	result := backend.BatchApply(ctx, patches, nil)
	if result.RowsUpdated != 1 {
		t.Errorf("expected 1 row updated (existing), got %d", result.RowsUpdated)
	}
}

func TestBatchMigrate_SkipsRules(t *testing.T) {
	d := openTestDataWithRawDB(t)
	ctx := context.Background()
	seedAgent(t, d, "ag-1", "test-agent-1", "Test Agent", "legacy_provider")

	backend := &modelRegistryApplyBackend{data: d, llm: nil}
	rules := []modelregistry.ProviderMigrationRule{
		{Legacy: "legacy_provider", Catalog: "new_provider"},
	}
	skipRules := []string{"legacy_provider->new_provider"}
	result := backend.BatchMigrateProviderBindings(ctx, rules, skipRules)

	found := false
	for _, r := range result.CompletedRules {
		if r == "legacy_provider->new_provider" {
			found = true
		}
	}
	if !found {
		t.Error("skipped rule should still appear in CompletedRules")
	}
	if result.Stats.Agents != 0 {
		t.Errorf("skipped rule should have 0 agents migrated, got %d", result.Stats.Agents)
	}

	var provider string
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx, `SELECT provider FROM agents WHERE id = ?`, "ag-1").Scan(&provider); err != nil {
		t.Fatal(err)
	}
	if provider != "legacy_provider" {
		t.Errorf("skipped migration should not change provider, got %q", provider)
	}
}

func TestBatchMigrate_AppliesRules(t *testing.T) {
	d := openTestDataWithRawDB(t)
	ctx := context.Background()
	seedAgent(t, d, "ag-1", "test-agent-1", "Agent 1", "legacy_provider")
	seedAgent(t, d, "ag-2", "test-agent-2", "Agent 2", "legacy_provider")

	backend := &modelRegistryApplyBackend{data: d, llm: nil}
	rules := []modelregistry.ProviderMigrationRule{
		{Legacy: "legacy_provider", Catalog: "new_provider"},
	}
	result := backend.BatchMigrateProviderBindings(ctx, rules, nil)

	if result.Stats.Agents != 2 {
		t.Errorf("expected 2 agents migrated, got %d", result.Stats.Agents)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
	found := false
	for _, r := range result.CompletedRules {
		if r == "legacy_provider->new_provider" {
			found = true
		}
	}
	if !found {
		t.Error("expected completed rule in result")
	}

	var count int
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agents WHERE provider = ?`, "new_provider",
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 agents with new_provider, got %d", count)
	}
}

func TestBatchMigrate_MultipleRules(t *testing.T) {
	d := openTestDataWithRawDB(t)
	ctx := context.Background()
	seedAgent(t, d, "ag-1", "agent-a", "Agent A", "old_a")
	seedAgent(t, d, "ag-2", "agent-b", "Agent B", "old_b")

	backend := &modelRegistryApplyBackend{data: d, llm: nil}
	rules := []modelregistry.ProviderMigrationRule{
		{Legacy: "old_a", Catalog: "new_a"},
		{Legacy: "old_b", Catalog: "new_b"},
	}
	result := backend.BatchMigrateProviderBindings(ctx, rules, nil)

	if len(result.CompletedRules) != 2 {
		t.Errorf("expected 2 completed rules, got %d", len(result.CompletedRules))
	}
	if result.Stats.Agents != 2 {
		t.Errorf("expected 2 total agents migrated, got %d", result.Stats.Agents)
	}

	var countA, countB int
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx, `SELECT COUNT(*) FROM agents WHERE provider = ?`, "new_a").Scan(&countA); err != nil {
		t.Fatal(err)
	}
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx, `SELECT COUNT(*) FROM agents WHERE provider = ?`, "new_b").Scan(&countB); err != nil {
		t.Fatal(err)
	}
	if countA != 1 || countB != 1 {
		t.Errorf("expected 1 agent each for new_a/new_b, got %d/%d", countA, countB)
	}
}

func TestBatchMigrate_PartialSkip(t *testing.T) {
	d := openTestDataWithRawDB(t)
	ctx := context.Background()
	seedAgent(t, d, "ag-1", "agent-a", "Agent A", "old_a")
	seedAgent(t, d, "ag-2", "agent-b", "Agent B", "old_b")

	backend := &modelRegistryApplyBackend{data: d, llm: nil}
	rules := []modelregistry.ProviderMigrationRule{
		{Legacy: "old_a", Catalog: "new_a"},
		{Legacy: "old_b", Catalog: "new_b"},
	}
	skipRules := []string{"old_a->new_a"}
	result := backend.BatchMigrateProviderBindings(ctx, rules, skipRules)

	if len(result.CompletedRules) != 2 {
		t.Errorf("expected 2 completed rules (1 skipped + 1 applied), got %d", len(result.CompletedRules))
	}
	if result.Stats.Agents != 1 {
		t.Errorf("expected 1 agent migrated (old_b only), got %d", result.Stats.Agents)
	}

	var providerA string
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx, `SELECT provider FROM agents WHERE id = ?`, "ag-1").Scan(&providerA); err != nil {
		t.Fatal(err)
	}
	if providerA != "old_a" {
		t.Errorf("old_a should not be migrated, got %q", providerA)
	}

	var providerB string
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx, `SELECT provider FROM agents WHERE id = ?`, "ag-2").Scan(&providerB); err != nil {
		t.Fatal(err)
	}
	if providerB != "new_b" {
		t.Errorf("old_b should be migrated to new_b, got %q", providerB)
	}
}

func TestBatchApply_PricingInsert(t *testing.T) {
	d := openTestDataWithRawDB(t)
	ctx := context.Background()

	backend := &modelRegistryApplyBackend{data: d, llm: nil}
	pricing := []modelregistry.PricingUpsert{
		{
			Provider: "openai",
			Model:    "gpt-4o",
			Micro:    modelregistry.MicroPricing{Input: 2500, Output: 10000},
			Source:   "models.dev-sync",
		},
	}
	result := backend.BatchApply(ctx, nil, pricing)
	if result.PricingUpdated != 1 {
		t.Errorf("expected 1 pricing updated, got %d", result.PricingUpdated)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	var count int
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM model_pricing_rules WHERE provider_code = ? AND model_api_id = ?`,
		"openai", "gpt-4o",
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 pricing rule, got %d", count)
	}

	var inputMicro int64
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx,
		`SELECT input_price_micro_usd_per_1k FROM model_pricing_rules WHERE provider_code = ? AND model_api_id = ?`,
		"openai", "gpt-4o",
	).Scan(&inputMicro); err != nil {
		t.Fatal(err)
	}
	if inputMicro != 2500 {
		t.Errorf("expected input_price_micro_usd_per_1k=2500, got %d", inputMicro)
	}
}

func TestBatchApply_PricingUpsert(t *testing.T) {
	d := openTestDataWithRawDB(t)
	ctx := context.Background()

	backend := &modelRegistryApplyBackend{data: d, llm: nil}
	pricing1 := []modelregistry.PricingUpsert{
		{
			Provider: "openai",
			Model:    "gpt-4o",
			Micro:    modelregistry.MicroPricing{Input: 2500, Output: 10000},
			Source:   "models.dev-sync",
		},
	}
	result1 := backend.BatchApply(ctx, nil, pricing1)
	if result1.PricingUpdated != 1 {
		t.Fatalf("first insert: expected 1 pricing updated, got %d", result1.PricingUpdated)
	}

	pricing2 := []modelregistry.PricingUpsert{
		{
			Provider: "openai",
			Model:    "gpt-4o",
			Micro:    modelregistry.MicroPricing{Input: 3000, Output: 12000},
			Source:   "models.dev-sync",
		},
	}
	result2 := backend.BatchApply(ctx, nil, pricing2)
	if result2.PricingUpdated != 1 {
		t.Fatalf("upsert: expected 1 pricing updated, got %d", result2.PricingUpdated)
	}

	var count int
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM model_pricing_rules WHERE provider_code = ? AND model_api_id = ?`,
		"openai", "gpt-4o",
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("upsert should keep 1 row, got %d", count)
	}

	var inputMicro int64
	if err := d.RWDB().ReadHandle().QueryRowContext(ctx,
		`SELECT input_price_micro_usd_per_1k FROM model_pricing_rules WHERE provider_code = ? AND model_api_id = ?`,
		"openai", "gpt-4o",
	).Scan(&inputMicro); err != nil {
		t.Fatal(err)
	}
	if inputMicro != 3000 {
		t.Errorf("upsert should update input_price to 3000, got %d", inputMicro)
	}
}

func TestBatchApply_EmptyInputs(t *testing.T) {
	d := openTestDataWithRawDB(t)
	ctx := context.Background()

	backend := &modelRegistryApplyBackend{data: d, llm: nil}
	result := backend.BatchApply(ctx, nil, nil)
	if result.RowsUpdated != 0 {
		t.Errorf("expected 0 rows updated for empty input, got %d", result.RowsUpdated)
	}
	if result.PricingUpdated != 0 {
		t.Errorf("expected 0 pricing updated for empty input, got %d", result.PricingUpdated)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestBatchApply_PatchesAndPricingTogether(t *testing.T) {
	d := openTestDataWithRawDB(t)
	ctx := context.Background()

	_, err := d.RWDB().WriteHandle().ExecContext(ctx,
		`INSERT INTO llm_provider_models (id, provider, model_key, name, model, enabled, config_json, metadata_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"pm-1", "openai", "openai:gpt-4o", "GPT-4o", "gpt-4o", true, `{"catalog_managed":true}`, `{}`,
	)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	backend := &modelRegistryApplyBackend{data: d, llm: nil}
	patches := []modelregistry.ApplyRow{
		{ID: "pm-1", Key: "openai:gpt-4o", Name: "GPT-4o Updated", Provider: "openai", Model: "gpt-4o", Enabled: true, ConfigJSON: `{"catalog_managed":true}`, MetadataJSON: `{}`},
	}
	pricing := []modelregistry.PricingUpsert{
		{
			Provider: "openai",
			Model:    "gpt-4o",
			Micro:    modelregistry.MicroPricing{Input: 2500, Output: 10000},
			Source:   "models.dev-sync",
		},
	}
	result := backend.BatchApply(ctx, patches, pricing)
	if result.RowsUpdated != 1 {
		t.Errorf("expected 1 row updated, got %d", result.RowsUpdated)
	}
	if result.PricingUpdated != 1 {
		t.Errorf("expected 1 pricing updated, got %d", result.PricingUpdated)
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}
