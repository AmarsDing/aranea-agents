package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// setupSIRiskRuleRepo builds the repo on an isolated PG schema and applies
// the DDL-migration columns (testhelper only runs Ent auto-migration).
func setupSIRiskRuleRepo(t *testing.T) (biz.SIRiskRuleRepo, context.Context) {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`ALTER TABLE system_settings ADD COLUMN si_risk_low_max_lines INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE system_settings ADD COLUMN si_risk_medium_max_lines INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE system_settings ADD COLUMN si_risk_core_path_globs TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN si_risk_daily_auto_quota INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("apply test DDL %q: %v", stmt, err)
		}
	}
	if err := ensureDefaultSystemSetting(ctx, client); err != nil {
		t.Fatalf("seed system_setting: %v", err)
	}
	d := newDataFromClient(client, loggateway.NewNoop())
	return NewSIRiskRuleRepo(d), ctx
}

func TestSIRiskRuleRepo_DefaultsRoundTrip(t *testing.T) {
	repo, ctx := setupSIRiskRuleRepo(t)

	got, err := repo.GetSIRiskRules(ctx)
	if err != nil {
		t.Fatalf("GetSIRiskRules: %v", err)
	}
	if got.LowMaxLines != 0 || got.MediumMaxLines != 0 || got.DailyAutoQuota != 0 || len(got.CorePathGlobs) != 0 {
		t.Errorf("fresh row should be all-zero (inherit defaults), got %+v", got)
	}

	want := biz.SIRiskRules{
		LowMaxLines:    50,
		MediumMaxLines: 200,
		CorePathGlobs:  []string{"internal/service/**", "**/*.proto"},
		DailyAutoQuota: 2,
	}
	if _, err := repo.UpdateSIRiskRules(ctx, want); err != nil {
		t.Fatalf("UpdateSIRiskRules: %v", err)
	}
	got, err = repo.GetSIRiskRules(ctx)
	if err != nil {
		t.Fatalf("GetSIRiskRules after update: %v", err)
	}
	if got.LowMaxLines != want.LowMaxLines || got.MediumMaxLines != want.MediumMaxLines || got.DailyAutoQuota != want.DailyAutoQuota {
		t.Errorf("thresholds mismatch: got %+v want %+v", got, want)
	}
	if len(got.CorePathGlobs) != 2 || got.CorePathGlobs[0] != "internal/service/**" || got.CorePathGlobs[1] != "**/*.proto" {
		t.Errorf("globs mismatch: got %v", got.CorePathGlobs)
	}
}

func TestSIRiskRuleRepo_GlobsWhitespaceTrimmed(t *testing.T) {
	repo, ctx := setupSIRiskRuleRepo(t)

	if _, err := repo.UpdateSIRiskRules(ctx, biz.SIRiskRules{
		CorePathGlobs: []string{"  internal/a/**  ", "", "internal/b/**"},
	}); err != nil {
		t.Fatalf("UpdateSIRiskRules: %v", err)
	}
	got, err := repo.GetSIRiskRules(ctx)
	if err != nil {
		t.Fatalf("GetSIRiskRules: %v", err)
	}
	if len(got.CorePathGlobs) != 2 || got.CorePathGlobs[0] != "internal/a/**" || got.CorePathGlobs[1] != "internal/b/**" {
		t.Errorf("globs should be trimmed and blanks dropped, got %q", got.CorePathGlobs)
	}
}
