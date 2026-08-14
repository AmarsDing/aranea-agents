package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func setupSITriggerCooldownStore(t *testing.T) (biz.SITriggerCooldownStore, context.Context) {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`ALTER TABLE system_settings ADD COLUMN si_trigger_cooldown_multipliers TEXT NOT NULL DEFAULT '{}'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("apply test DDL %q: %v", stmt, err)
		}
	}
	if err := ensureDefaultSystemSetting(ctx, client); err != nil {
		t.Fatalf("seed system_setting: %v", err)
	}
	d := newDataFromClient(client, loggateway.NewNoop())
	return NewSITriggerCooldownStore(d), ctx
}

func TestSITriggerCooldownStore_EmptyThenRoundTrip(t *testing.T) {
	store, ctx := setupSITriggerCooldownStore(t)

	got, err := store.LoadTriggerCooldownMultipliers(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("fresh row should be empty, got %v", got)
	}

	want := map[string]float64{biz.TriggerSourceErrorCluster: 4, biz.TriggerSourcePerfBottleneck: 2}
	if err := store.SaveTriggerCooldownMultipliers(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err = store.LoadTriggerCooldownMultipliers(ctx)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if got[biz.TriggerSourceErrorCluster] != 4 || got[biz.TriggerSourcePerfBottleneck] != 2 || len(got) != 2 {
		t.Fatalf("round-trip mismatch: got %v want %v", got, want)
	}
}

func TestParseTriggerCooldownJSON_EmptyAndNull(t *testing.T) {
	for _, raw := range []string{"", "   ", "{}", "null"} {
		got, err := parseTriggerCooldownJSON(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if len(got) != 0 {
			t.Fatalf("parse %q = %v, want empty", raw, got)
		}
	}
}
