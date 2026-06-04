package data

import (
	"context"
	"sort"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/types"
	"aranea-agents/internal/data/ent/skillinvocation"
)

type skillHealthRepo struct {
	data *Data
}

var _ biz.SkillHealthReader = (*skillHealthRepo)(nil)

func NewSkillHealthRepo(data *Data) biz.SkillHealthReader {
	return &skillHealthRepo{data: data}
}

func (r *skillHealthRepo) GetSkillHealth(ctx context.Context, skillID string, since7d, since30d time.Time) (*types.SkillHealthDetail, error) {
	since30dStr := since30d.Format(time.RFC3339)
	rows, err := r.data.RW().Read(ctx).SkillInvocation.Query().
		Where(
			skillinvocation.SkillIDEQ(skillID),
			skillinvocation.CreatedAtGTE(since30dStr),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	dailyBuckets := make(map[string]*dailyBucket)
	var inv7d, succ7d int
	var durations7d []int
	var inv30d, succ30d int
	var durations30d []int

	for _, row := range rows {
		day := dayFromCreatedAt(row.CreatedAt)
		b, ok := dailyBuckets[day]
		if !ok {
			b = &dailyBucket{}
			dailyBuckets[day] = b
		}
		b.count++
		b.totalDurationMs += row.DurationMs
		if isSuccess(row.Outcome, row.Status) {
			b.successes++
		}

		// 30d window
		inv30d++
		if isSuccess(row.Outcome, row.Status) {
			succ30d++
		}
		durations30d = append(durations30d, row.DurationMs)

		// 7d window
		if row.CreatedAt >= since7d.Format(time.RFC3339) {
			inv7d++
			if isSuccess(row.Outcome, row.Status) {
				succ7d++
			}
			durations7d = append(durations7d, row.DurationMs)
		}
	}

	dailyMetrics := make([]types.DailyMetric, 0, len(dailyBuckets))
	for day, b := range dailyBuckets {
		avgDur := 0.0
		if b.count > 0 {
			avgDur = float64(b.totalDurationMs) / float64(b.count)
		}
		dailyMetrics = append(dailyMetrics, types.DailyMetric{
			Date:          day,
			Invocations:   b.count,
			Successes:     b.successes,
			AvgDurationMs: avgDur,
		})
	}
	sort.Slice(dailyMetrics, func(i, j int) bool { return dailyMetrics[i].Date < dailyMetrics[j].Date })

	result := &types.SkillHealthDetail{
		SkillID:             skillID,
		TotalInvocations7d:  inv7d,
		SuccessCount7d:      succ7d,
		SuccessRate7d:       safeRate(succ7d, inv7d),
		P95DurationMs7d:     p95(durations7d),
		TotalInvocations30d: inv30d,
		SuccessCount30d:     succ30d,
		SuccessRate30d:      safeRate(succ30d, inv30d),
		P95DurationMs30d:    p95(durations30d),
		DailyMetrics:        dailyMetrics,
	}
	return result, nil
}

type dailyBucket struct {
	count           int
	successes       int
	totalDurationMs int
}

func isSuccess(outcome, status string) bool {
	if outcome == "success" {
		return true
	}
	if outcome == "" && (status == "completed" || status == "success") {
		return true
	}
	return false
}

func safeRate(success, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(success) / float64(total)
}

func p95(durations []int) int {
	if len(durations) == 0 {
		return 0
	}
	sort.Ints(durations)
	idx := int(float64(len(durations)) * 0.95)
	if idx >= len(durations) {
		idx = len(durations) - 1
	}
	return durations[idx]
}

func dayFromCreatedAt(createdAt string) string {
	if len(createdAt) >= 10 {
		return createdAt[:10]
	}
	return createdAt
}
