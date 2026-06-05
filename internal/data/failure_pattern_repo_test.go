package data

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	bizmonitor "aranea-agents/internal/biz/monitor"
)

func createFailurePatternTable(t *testing.T, d *Data) {
	t.Helper()
	_, err := d.RWDB().WriteDB(context.Background()).ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS failure_pattern (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL,
			type TEXT NOT NULL,
			pattern_hash TEXT NOT NULL,
			pattern_regex TEXT NOT NULL,
			fix_action TEXT NOT NULL,
			confidence REAL NOT NULL DEFAULT 0.5,
			success_count INTEGER NOT NULL DEFAULT 0,
			fail_count INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			is_active INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Create indexes
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_fp_source_type ON failure_pattern (source, type)`,
		`CREATE INDEX IF NOT EXISTS idx_fp_pattern_hash ON failure_pattern (pattern_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_fp_active_confidence ON failure_pattern (is_active, confidence)`,
	}
	for _, idx := range indexes {
		if _, err := d.RWDB().WriteDB(context.Background()).ExecContext(context.Background(), idx); err != nil {
			t.Fatalf("create index: %v", err)
		}
	}
}

func TestFailurePatternRepo_CreateAndGetByHash(t *testing.T) {
	d := openTestDataWithRWDB(t)
	createFailurePatternTable(t, d)
	repo := NewFailurePatternRepo(d)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	fixAction := bizmonitor.FixAction{
		Type:        "retry",
		MaxAttempts: 2,
		Params:      map[string]any{"backoff_ms": 2000},
	}
	fixActionJSON, _ := json.Marshal(fixAction)

	pattern := bizmonitor.FailurePattern{
		ID:           "fp-test-1",
		Source:       bizmonitor.FailurePatternSourceRuntime,
		Type:         "provider_timeout",
		PatternHash:  "sha256:abc123",
		PatternRegex: `(?i)(timeout|timed out)`,
		FixAction:    fixAction,
		Confidence:   0.9,
		SuccessCount: 0,
		FailCount:    0,
		Version:      1,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := repo.Create(ctx, pattern); err != nil {
		t.Fatalf("create: %v", err)
	}

	_ = fixActionJSON // fix_action stored as JSON string

	got, err := repo.GetByPatternHash(ctx, "sha256:abc123")
	if err != nil {
		t.Fatalf("get by hash: %v", err)
	}
	if got == nil {
		t.Fatal("expected pattern, got nil")
	}
	if got.ID != "fp-test-1" {
		t.Errorf("expected id fp-test-1, got %s", got.ID)
	}
	if got.Source != bizmonitor.FailurePatternSourceRuntime {
		t.Errorf("expected source runtime, got %s", got.Source)
	}
	if got.PatternHash != "sha256:abc123" {
		t.Errorf("expected hash sha256:abc123, got %s", got.PatternHash)
	}
	if got.Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", got.Confidence)
	}
	if got.FixAction.Type != "retry" {
		t.Errorf("expected fix_action type retry, got %s", got.FixAction.Type)
	}
	if got.FixAction.MaxAttempts != 2 {
		t.Errorf("expected fix_action max_attempts 2, got %d", got.FixAction.MaxAttempts)
	}
}

func TestFailurePatternRepo_ListBySource(t *testing.T) {
	d := openTestDataWithRWDB(t)
	createFailurePatternTable(t, d)
	repo := NewFailurePatternRepo(d)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	for i, src := range []bizmonitor.FailurePatternSource{
		bizmonitor.FailurePatternSourceRuntime,
		bizmonitor.FailurePatternSourceRuntime,
		bizmonitor.FailurePatternSourceCI,
	} {
		pattern := bizmonitor.FailurePattern{
			ID:           "fp-" + string(src) + "-" + string(rune('a'+i)),
			Source:       src,
			Type:         "test_type",
			PatternHash:  "hash-" + string(rune('a'+i)),
			PatternRegex: `test`,
			FixAction:    bizmonitor.FixAction{Type: "log_only"},
			Confidence:   0.5,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := repo.Create(ctx, pattern); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	patterns, err := repo.ListBySource(ctx, bizmonitor.FailurePatternSourceRuntime)
	if err != nil {
		t.Fatalf("list by source: %v", err)
	}
	if len(patterns) != 2 {
		t.Errorf("expected 2 runtime patterns, got %d", len(patterns))
	}
}

func TestFailurePatternRepo_ListActive(t *testing.T) {
	d := openTestDataWithRWDB(t)
	createFailurePatternTable(t, d)
	repo := NewFailurePatternRepo(d)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	// Create one active, one inactive
	active := bizmonitor.FailurePattern{
		ID: "fp-active", Source: bizmonitor.FailurePatternSourceRuntime, Type: "t1",
		PatternHash: "hash-active", PatternRegex: `test`, FixAction: bizmonitor.FixAction{Type: "retry"},
		Confidence: 0.9, IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
	inactive := bizmonitor.FailurePattern{
		ID: "fp-inactive", Source: bizmonitor.FailurePatternSourceRuntime, Type: "t2",
		PatternHash: "hash-inactive", PatternRegex: `test`, FixAction: bizmonitor.FixAction{Type: "log_only"},
		Confidence: 0.3, IsActive: false, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, active); err != nil {
		t.Fatalf("create active: %v", err)
	}
	if err := repo.Create(ctx, inactive); err != nil {
		t.Fatalf("create inactive: %v", err)
	}

	patterns, err := repo.ListActive(ctx)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(patterns) != 1 {
		t.Errorf("expected 1 active pattern, got %d", len(patterns))
	}
	if patterns[0].ID != "fp-active" {
		t.Errorf("expected fp-active, got %s", patterns[0].ID)
	}
}

func TestFailurePatternRepo_IncrementSuccess(t *testing.T) {
	d := openTestDataWithRWDB(t)
	createFailurePatternTable(t, d)
	repo := NewFailurePatternRepo(d)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	pattern := bizmonitor.FailurePattern{
		ID: "fp-inc", Source: bizmonitor.FailurePatternSourceMined, Type: "test",
		PatternHash: "hash-inc", PatternRegex: `test`, FixAction: bizmonitor.FixAction{Type: "retry"},
		Confidence: 0.5, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, pattern); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.IncrementSuccess(ctx, "fp-inc"); err != nil {
		t.Fatalf("increment success: %v", err)
	}
	if err := repo.IncrementSuccess(ctx, "fp-inc"); err != nil {
		t.Fatalf("increment success 2: %v", err)
	}

	got, err := repo.GetByPatternHash(ctx, "hash-inc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SuccessCount != 2 {
		t.Errorf("expected success_count 2, got %d", got.SuccessCount)
	}
}

func TestFailurePatternRepo_IncrementFail(t *testing.T) {
	d := openTestDataWithRWDB(t)
	createFailurePatternTable(t, d)
	repo := NewFailurePatternRepo(d)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	pattern := bizmonitor.FailurePattern{
		ID: "fp-fail", Source: bizmonitor.FailurePatternSourceMined, Type: "test",
		PatternHash: "hash-fail", PatternRegex: `test`, FixAction: bizmonitor.FixAction{Type: "retry"},
		Confidence: 0.5, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, pattern); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.IncrementFail(ctx, "fp-fail"); err != nil {
		t.Fatalf("increment fail: %v", err)
	}

	got, err := repo.GetByPatternHash(ctx, "hash-fail")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FailCount != 1 {
		t.Errorf("expected fail_count 1, got %d", got.FailCount)
	}
}

func TestFailurePatternRepo_Deactivate(t *testing.T) {
	d := openTestDataWithRWDB(t)
	createFailurePatternTable(t, d)
	repo := NewFailurePatternRepo(d)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	pattern := bizmonitor.FailurePattern{
		ID: "fp-deact", Source: bizmonitor.FailurePatternSourceMined, Type: "test",
		PatternHash: "hash-deact", PatternRegex: `test`, FixAction: bizmonitor.FixAction{Type: "retry"},
		Confidence: 0.5, IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, pattern); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.Deactivate(ctx, "fp-deact"); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	got, err := repo.GetByPatternHash(ctx, "hash-deact")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.IsActive {
		t.Error("expected is_active=false after deactivate")
	}
}

func TestFailurePatternRepo_Update(t *testing.T) {
	d := openTestDataWithRWDB(t)
	createFailurePatternTable(t, d)
	repo := NewFailurePatternRepo(d)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	pattern := bizmonitor.FailurePattern{
		ID: "fp-update", Source: bizmonitor.FailurePatternSourceRuntime, Type: "test",
		PatternHash: "hash-update", PatternRegex: `old_regex`, FixAction: bizmonitor.FixAction{Type: "retry"},
		Confidence: 0.5, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, pattern); err != nil {
		t.Fatalf("create: %v", err)
	}

	pattern.Version = 2
	pattern.PatternRegex = `new_regex`
	pattern.Confidence = 0.8
	pattern.UpdatedAt = time.Now().UTC().Truncate(time.Second)

	if err := repo.Update(ctx, pattern); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := repo.GetByPatternHash(ctx, "hash-update")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("expected version 2, got %d", got.Version)
	}
	if got.PatternRegex != "new_regex" {
		t.Errorf("expected new_regex, got %s", got.PatternRegex)
	}
	if got.Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %f", got.Confidence)
	}
}
