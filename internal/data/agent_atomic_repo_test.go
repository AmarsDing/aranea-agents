package data_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TS9-BUG-3 (data-layer blocker): the pack importer builds runtime settings with
// an empty AgentID and relies on the atomic writer to backfill it — exactly as
// CreateAgentAtomic does. UpdateAgentAtomic previously skipped the backfill, so
// every ConflictOverwrite re-import carrying settings failed with
// "agent id is required" and the stale (zero-value) row survived.
func TestUpdateAgentAtomic_BackfillsSettingsAgentID(t *testing.T) {
	client, db := testhelper.SetupTestPG(t)
	d := &data.Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	repo := data.NewAgentRepo(d)
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339)
	created, err := repo.CreateAgent(ctx, biz.Agent{
		ID:          "agent_atomic_1",
		AgentKey:    "atomic_agent__general",
		DisplayName: "Atomic",
		Provider:    "deepseek",
		Model:       "deepseek-chat",
		Status:      "active",
		Kind:        "ecosystem_preset",
		AgentKind:   biz.AgentKindLLM,
		Source:      "imported",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Importer semantics: settings built with empty AgentID, backfilled by atomic.
	settings := biz.DefaultAgentRuntimeSettings()
	settings.ToolsAllowJSON = `["shell_exec"]`
	settings.ToolsDenyJSON = `["workspace_exec"]`

	updated, err := repo.UpdateAgentAtomic(ctx, biz.Agent{
		ID:          created.ID,
		AgentKey:    created.AgentKey,
		DisplayName: "Atomic v2",
		Provider:    "deepseek",
		Model:       "deepseek-chat",
		Status:      "active",
		Kind:        "ecosystem_preset",
		AgentKind:   biz.AgentKindLLM,
		Source:      "imported",
	}, nil, &settings)
	if err != nil {
		t.Fatalf("UpdateAgentAtomic must backfill settings.AgentID: %v", err)
	}

	got, err := repo.GetAgentRuntimeSettings(ctx, updated.ID)
	if err != nil {
		t.Fatalf("GetAgentRuntimeSettings: %v", err)
	}
	if got.ToolsAllowJSON != `["shell_exec"]` {
		t.Errorf("ToolsAllowJSON = %q, want %q", got.ToolsAllowJSON, `["shell_exec"]`)
	}
	if got.ToolsDenyJSON != `["workspace_exec"]` {
		t.Errorf("ToolsDenyJSON = %q, want %q", got.ToolsDenyJSON, `["workspace_exec"]`)
	}
	if !got.ToolsEnabled {
		t.Error("ToolsEnabled must persist from platform defaults")
	}
}
