package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// mockExperienceReportStatsReader implements ExperienceReportStatsReader for testing.
type mockExperienceReportStatsReader struct {
	tagCounts  []FailureTagCount
	rcaReports []ExperienceReport
	stats      *ExperienceReportStats
	statsErr   error
}

func (m *mockExperienceReportStatsReader) GetFailureTagCountsFiltered(_ context.Context, _ string, _, _ *time.Time) ([]FailureTagCount, error) {
	return m.tagCounts, nil
}

func (m *mockExperienceReportStatsReader) GetRootCauseReportsFiltered(_ context.Context, _ string, _, _ *time.Time, _ int) ([]ExperienceReport, error) {
	return m.rcaReports, nil
}

func (m *mockExperienceReportStatsReader) GetExperienceReportStatsFiltered(_ context.Context, _ string, _, _ *time.Time) (*ExperienceReportStats, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	return m.stats, nil
}

func TestSkillReportUsecase_GetExperienceReportsFiltered_PopulatesStats(t *testing.T) {
	reader := &mockExperienceReportReader{reports: []ExperienceReport{{ID: "r1"}}}
	statsReader := &mockExperienceReportStatsReader{
		stats: &ExperienceReportStats{SuccessCount: 8, FailureCount: 2, AvgScore: 87.5},
	}
	uc := NewSkillReportUsecase(reader, nil, statsReader, nil, nil, loggateway.NewNoop())

	result, err := uc.GetExperienceReportsFiltered(context.Background(), "", nil, nil, 20, 0)
	if err != nil {
		t.Fatalf("GetExperienceReportsFiltered: %v", err)
	}
	if result.Stats == nil {
		t.Fatal("expected Stats to be populated, got nil")
	}
	if result.Stats.SuccessCount != 8 || result.Stats.FailureCount != 2 {
		t.Errorf("unexpected stats counts: %+v", result.Stats)
	}
	if result.Stats.AvgScore != 87.5 {
		t.Errorf("AvgScore = %v, want 87.5", result.Stats.AvgScore)
	}
}

// 聚合统计失败时应降级（Stats=nil + Warn 日志），不阻断列表主流程——
// 与 GetFailureTagCountsFiltered 失败时的既有行为保持一致。
func TestSkillReportUsecase_GetExperienceReportsFiltered_StatsErrorDegrades(t *testing.T) {
	reader := &mockExperienceReportReader{reports: []ExperienceReport{{ID: "r1"}}}
	statsReader := &mockExperienceReportStatsReader{statsErr: errors.New("db down")}
	uc := NewSkillReportUsecase(reader, nil, statsReader, nil, nil, loggateway.NewNoop())

	result, err := uc.GetExperienceReportsFiltered(context.Background(), "", nil, nil, 20, 0)
	if err != nil {
		t.Fatalf("stats error should degrade without failing the request, got: %v", err)
	}
	if result.Stats != nil {
		t.Errorf("expected Stats nil on stats error, got %+v", result.Stats)
	}
	if len(result.Reports) != 1 {
		t.Errorf("expected reports unaffected, got %d", len(result.Reports))
	}
}
