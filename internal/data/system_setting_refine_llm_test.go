package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// setupRefineLLMSettingRepo builds a Data with the Ent-managed system_settings
// table plus the refine_llm_* DDL columns (mirroring the refine-LLM DDL
// migration, which Ent auto-migration does not cover). Same harness as
// setupSpeechSettingRepo.
func setupRefineLLMSettingRepo(t *testing.T) *systemSettingRepo {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS refine_llm_provider TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS refine_llm_model TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS refine_llm_base_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS refine_llm_api_key TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("add refine_llm column: %v", err)
		}
	}
	d := &Data{
		entClient:  client,
		readClient: client,
		rw:         NewReadWriteClient(client, client),
		rawDB:      db,
		readDB:     db,
		rwDB:       NewReadWriteDB(db, db),
		lg:         loggateway.NewNoop(),
		dialect:    DialectPostgres,
	}
	if err := ensureDefaultSystemSetting(ctx, client); err != nil {
		t.Fatalf("seed singleton: %v", err)
	}
	return &systemSettingRepo{data: d}
}

// TestSystemSettingRepo_RefineLLMUpdateReturnsHasAPIKey pins the contract that
// UpdateRefineLLM returns the effective stored state — including the HasAPIKey
// marker — so the PUT /v1/system-settings response reflects the just-saved key
// without a follow-up GET (frontend syncs the form from the PUT response).
func TestSystemSettingRepo_RefineLLMUpdateReturnsHasAPIKey(t *testing.T) {
	r := setupRefineLLMSettingRepo(t)
	ctx := context.Background()

	// Fresh row: redacted read shows nothing stored.
	fresh, err := r.getRefineLLMRedacted(ctx)
	if err != nil {
		t.Fatalf("getRefineLLMRedacted fresh: %v", err)
	}
	if fresh.Provider != "" || fresh.Model != "" || fresh.HasAPIKey {
		t.Fatalf("fresh row must be empty with HasAPIKey=false, got %#v", fresh)
	}

	// Rotate with a new key: the returned state must mark HasAPIKey=true.
	got, err := r.UpdateRefineLLM(ctx, biz.RefineLLMSetting{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		APIKey:   "sk-test-1",
	}, true)
	if err != nil {
		t.Fatalf("UpdateRefineLLM with key: %v", err)
	}
	if got.Provider != "deepseek" || got.Model != "deepseek-v4-flash" {
		t.Fatalf("provider/model not reflected, got %#v", got)
	}
	if !got.HasAPIKey {
		t.Fatalf("HasAPIKey must be true right after key rotation, got %#v", got)
	}

	// Update provider/model without touching the key: HasAPIKey must still
	// report the retained stored key.
	got, err = r.UpdateRefineLLM(ctx, biz.RefineLLMSetting{
		Provider: "openai",
		Model:    "gpt-5-mini",
	}, false)
	if err != nil {
		t.Fatalf("UpdateRefineLLM keep key: %v", err)
	}
	if got.Provider != "openai" || got.Model != "gpt-5-mini" {
		t.Fatalf("provider/model not updated, got %#v", got)
	}
	if !got.HasAPIKey {
		t.Fatalf("HasAPIKey must stay true when key is retained, got %#v", got)
	}

	// Redacted read agrees with the update return value.
	redacted, err := r.getRefineLLMRedacted(ctx)
	if err != nil {
		t.Fatalf("getRefineLLMRedacted: %v", err)
	}
	if redacted.Provider != "openai" || redacted.Model != "gpt-5-mini" || !redacted.HasAPIKey {
		t.Fatalf("redacted read mismatch, got %#v", redacted)
	}
}
