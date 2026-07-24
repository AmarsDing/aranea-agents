package data

import (
	"context"
	"testing"
	"time"

	bizmonitor "aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/types"
)

// openTestDataWithRWDB opens a Postgres backed Data instance (isolated test schema).
func openTestDataWithRWDB(t *testing.T) *Data {
	t.Helper()
	return newTestDataPG(t)
}

func TestSelfCheckReportRepo_InsertAndList(t *testing.T) {
	d := openTestDataWithRWDB(t)
	// Create the self_check_reports table
	if _, err := d.RWDB().WriteDB(context.Background()).ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS self_check_reports (
			id TEXT PRIMARY KEY,
			check_results_json TEXT NOT NULL DEFAULT '[]',
			overall_status TEXT NOT NULL,
			repair_actions_json TEXT NOT NULL DEFAULT '[]',
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	repo := NewSelfCheckReportRepo(d)
	ctx := context.Background()

	now := time.Now().UTC()
	report := bizmonitor.SelfCheckReport{
		ID: "test-report-1",
		CheckResults: []types.SelfCheckResult{
			{CheckID: "c1", Checker: "db_health", Status: types.SelfCheckStatusPassed, Message: "ok", CheckedAt: now},
		},
		OverallStatus: types.SelfCheckStatusPassed,
		RepairActions: nil,
		StartedAt:     now,
		FinishedAt:    now.Add(100 * time.Millisecond),
		DurationMs:    100,
	}

	if err := repo.InsertSelfCheckReport(ctx, report); err != nil {
		t.Fatalf("insert: %v", err)
	}

	reports, total, err := repo.ListSelfCheckReports(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].ID != "test-report-1" {
		t.Errorf("expected id test-report-1, got %s", reports[0].ID)
	}
	if reports[0].OverallStatus != types.SelfCheckStatusPassed {
		t.Errorf("expected passed, got %s", reports[0].OverallStatus)
	}
}

func TestSelfCheckReportRepo_DeleteOlderThan(t *testing.T) {
	d := openTestDataWithRWDB(t)
	if _, err := d.RWDB().WriteDB(context.Background()).ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS self_check_reports (
			id TEXT PRIMARY KEY,
			check_results_json TEXT NOT NULL DEFAULT '[]',
			overall_status TEXT NOT NULL,
			repair_actions_json TEXT NOT NULL DEFAULT '[]',
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	repo := NewSelfCheckReportRepo(d)
	ctx := context.Background()

	now := time.Now().UTC()
	oldReport := bizmonitor.SelfCheckReport{
		ID:            "old-report",
		CheckResults:  []types.SelfCheckResult{},
		OverallStatus: types.SelfCheckStatusPassed,
		StartedAt:     now.Add(-60 * 24 * time.Hour),
		FinishedAt:    now.Add(-60*24*time.Hour + time.Second),
		DurationMs:    1000,
	}
	newReport := bizmonitor.SelfCheckReport{
		ID:            "new-report",
		CheckResults:  []types.SelfCheckResult{},
		OverallStatus: types.SelfCheckStatusPassed,
		StartedAt:     now,
		FinishedAt:    now.Add(time.Second),
		DurationMs:    1000,
	}

	if err := repo.InsertSelfCheckReport(ctx, oldReport); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if err := repo.InsertSelfCheckReport(ctx, newReport); err != nil {
		t.Fatalf("insert new: %v", err)
	}

	cutoff := now.Add(-30 * 24 * time.Hour)
	deleted, err := repo.DeleteSelfCheckReportsOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	_, total, _ := repo.ListSelfCheckReports(ctx, 10, 0)
	if total != 1 {
		t.Errorf("expected 1 remaining, got %d", total)
	}
}

func TestSelfCheckReportRepo_ListPagination(t *testing.T) {
	d := openTestDataWithRWDB(t)
	if _, err := d.RWDB().WriteDB(context.Background()).ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS self_check_reports (
			id TEXT PRIMARY KEY,
			check_results_json TEXT NOT NULL DEFAULT '[]',
			overall_status TEXT NOT NULL,
			repair_actions_json TEXT NOT NULL DEFAULT '[]',
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	repo := NewSelfCheckReportRepo(d)
	ctx := context.Background()

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		report := bizmonitor.SelfCheckReport{
			ID:            "report-" + string(rune('a'+i)),
			CheckResults:  []types.SelfCheckResult{},
			OverallStatus: types.SelfCheckStatusPassed,
			StartedAt:     now,
			FinishedAt:    now.Add(time.Second),
			DurationMs:    1000,
		}
		if err := repo.InsertSelfCheckReport(ctx, report); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	reports, total, err := repo.ListSelfCheckReports(ctx, 2, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(reports) != 2 {
		t.Errorf("expected 2 items, got %d", len(reports))
	}
}
