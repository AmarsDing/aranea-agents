package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// setupSkillIntelligenceRepo builds a SkillIntelligenceRepo on a real Postgres
// schema (experience_reports table comes from Ent Schema.Create in SetupTestPG).
func setupSkillIntelligenceRepo(t *testing.T) *SkillIntelligenceRepo {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
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
	return NewSkillIntelligenceRepo(d, loggateway.NewNoop())
}

func seedExperienceReport(t *testing.T, repo *SkillIntelligenceRepo, id, skillID string, isSuccess bool, score int) {
	t.Helper()
	err := repo.Create(context.Background(), biz.ExperienceReport{
		ID:        id,
		SkillID:   skillID,
		IsSuccess: isSuccess,
		Score:     score,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed report %s: %v", id, err)
	}
}

// 锁定 GROUP BY is_success + AVG(score) 在 Postgres 上的真实行为与加权平均组装逻辑。
func TestGetExperienceReportStatsFilteredOnPostgres(t *testing.T) {
	ctx := context.Background()
	repo := setupSkillIntelligenceRepo(t)

	seedExperienceReport(t, repo, "r1", "skill-a", true, 90)
	seedExperienceReport(t, repo, "r2", "skill-a", true, 80)
	seedExperienceReport(t, repo, "r3", "skill-a", false, 40)
	// 另一个 skill 的记录不应计入 skill-a 的筛选结果。
	seedExperienceReport(t, repo, "r4", "skill-b", false, 10)

	t.Run("no filter aggregates all reports", func(t *testing.T) {
		stats, err := repo.GetExperienceReportStatsFiltered(ctx, "", nil, nil)
		if err != nil {
			t.Fatalf("GetExperienceReportStatsFiltered: %v", err)
		}
		if stats.SuccessCount != 2 || stats.FailureCount != 2 {
			t.Errorf("counts = %d/%d, want 2/2", stats.SuccessCount, stats.FailureCount)
		}
		// (90+80+40+10)/4 = 55
		if stats.AvgScore != 55 {
			t.Errorf("AvgScore = %v, want 55", stats.AvgScore)
		}
	})

	t.Run("skill filter scopes the aggregation", func(t *testing.T) {
		stats, err := repo.GetExperienceReportStatsFiltered(ctx, "skill-a", nil, nil)
		if err != nil {
			t.Fatalf("GetExperienceReportStatsFiltered: %v", err)
		}
		if stats.SuccessCount != 2 || stats.FailureCount != 1 {
			t.Errorf("counts = %d/%d, want 2/1", stats.SuccessCount, stats.FailureCount)
		}
		// (90+80+40)/3 = 70
		if stats.AvgScore != 70 {
			t.Errorf("AvgScore = %v, want 70", stats.AvgScore)
		}
	})

	t.Run("empty scope returns zero stats", func(t *testing.T) {
		stats, err := repo.GetExperienceReportStatsFiltered(ctx, "skill-nonexistent", nil, nil)
		if err != nil {
			t.Fatalf("GetExperienceReportStatsFiltered: %v", err)
		}
		if stats.SuccessCount != 0 || stats.FailureCount != 0 || stats.AvgScore != 0 {
			t.Errorf("expected zero stats, got %+v", stats)
		}
	})
}
