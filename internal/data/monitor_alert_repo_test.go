package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// setupMonitorAlertRepo builds a Data with the production monitor_alert_rules DDL
// on a real Postgres schema (regression: ReplaceAlertRules used SQLite-style ?
// placeholders which fail with pq syntax error 42601 on Postgres).
func setupMonitorAlertRepo(t *testing.T) *monitorRepo {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	if err := EnsureMonitorAlertSchema(ctx, client, DialectPostgres); err != nil {
		t.Fatalf("ensure monitor alert schema: %v", err)
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
	return &monitorRepo{data: d}
}

func TestReplaceAlertRulesInsertUpdateDeleteOnPostgres(t *testing.T) {
	ctx := context.Background()
	repo := setupMonitorAlertRepo(t)

	// Insert two rules.
	err := repo.ReplaceAlertRules(ctx, []biz.MonitorAlertRule{
		{ID: "r1", Name: "Runner error rate", MetricKey: "runner.error_rate", Threshold: 0.25, WindowMinutes: 60, Enabled: true, Severity: "warning", CooldownMinutes: 60},
		{ID: "r2", Name: "Dead letters", MetricKey: "sequencer.dead_letter_count", Threshold: 1, WindowMinutes: 5, Enabled: true, Severity: "critical", CooldownMinutes: 30},
	})
	if err != nil {
		t.Fatalf("insert rules: %v", err)
	}
	rules, err := repo.ListAlertRules(ctx)
	if err != nil {
		t.Fatalf("list after insert: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}

	// Update r1, delete r2 (stale id not in new set).
	err = repo.ReplaceAlertRules(ctx, []biz.MonitorAlertRule{
		{ID: "r1", Name: "Runner error rate v2", MetricKey: "runner.error_rate", Threshold: 0.5, WindowMinutes: 30, Enabled: false, Severity: "critical", CooldownMinutes: 15},
	})
	if err != nil {
		t.Fatalf("update/delete rules: %v", err)
	}
	rules, err = repo.ListAlertRules(ctx)
	if err != nil {
		t.Fatalf("list after update: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after delete, got %d", len(rules))
	}
	got := rules[0]
	if got.ID != "r1" || got.Name != "Runner error rate v2" || got.Threshold != 0.5 || got.WindowMinutes != 30 {
		t.Fatalf("unexpected updated rule: %+v", got)
	}
	if got.Enabled {
		t.Fatal("expected rule disabled after update")
	}
	if got.Severity != "critical" || got.CooldownMinutes != 15 {
		t.Fatalf("unexpected severity/cooldown: %+v", got)
	}
}
