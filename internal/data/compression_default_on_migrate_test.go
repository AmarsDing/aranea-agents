package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TestRunCompressionDefaultOnMigration covers N2 (2026-08-13 链路审查):
// rows carrying the old false defaults must be flipped to true so the
// compression cascade (framework summary consumption + request-level
// compaction) actually engages; already-true rows stay untouched and the
// version gate makes re-runs no-ops.
func TestRunCompressionDefaultOnMigration(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()

	client, _ := testhelper.SetupTestPG(t)
	d := newDataFromClient(client, lg)

	// Row A: legacy all-false row → all three must flip to true.
	if _, err := client.AgentRuntimeSetting.Create().
		SetID("ag-legacy").
		SetContextCompactionEnabled(false).
		SetMemoryCompactEnabled(false).
		SetSessionSummaryEnabled(false).
		Save(ctx); err != nil {
		t.Fatalf("create ag-legacy: %v", err)
	}
	// Row B: already all-true → untouched.
	if _, err := client.AgentRuntimeSetting.Create().
		SetID("ag-current").
		SetContextCompactionEnabled(true).
		SetMemoryCompactEnabled(true).
		SetSessionSummaryEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("create ag-current: %v", err)
	}
	// Row C: mixed state (compaction on, summary off) → summary flips.
	if _, err := client.AgentRuntimeSetting.Create().
		SetID("ag-mixed").
		SetContextCompactionEnabled(true).
		SetMemoryCompactEnabled(true).
		SetSessionSummaryEnabled(false).
		Save(ctx); err != nil {
		t.Fatalf("create ag-mixed: %v", err)
	}

	if err := RunCompressionDefaultOnMigration(ctx, client, d.Dialect(), lg); err != nil {
		t.Fatalf("migration: %v", err)
	}
	// Idempotent re-run (version gate).
	if err := RunCompressionDefaultOnMigration(ctx, client, d.Dialect(), lg); err != nil {
		t.Fatalf("migration second run: %v", err)
	}

	flagsOf := func(id string) (bool, bool, bool) {
		t.Helper()
		row, err := client.AgentRuntimeSetting.Get(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		return row.ContextCompactionEnabled, row.MemoryCompactEnabled, row.SessionSummaryEnabled
	}

	if c, m, s := flagsOf("ag-legacy"); !c || !m || !s {
		t.Errorf("ag-legacy = (%v,%v,%v), want all true", c, m, s)
	}
	if c, m, s := flagsOf("ag-current"); !c || !m || !s {
		t.Errorf("ag-current = (%v,%v,%v), want all true preserved", c, m, s)
	}
	if c, m, s := flagsOf("ag-mixed"); !c || !m || !s {
		t.Errorf("ag-mixed = (%v,%v,%v), want all true", c, m, s)
	}
}
