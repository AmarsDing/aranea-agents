package data

import (
	"context"
	"sort"
	"strings"
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
			// 只统计真实运行时调用；filesystem_* 同步记录不参与健康指标。
			skillinvocation.SourceEQ(biz.SkillInvocationSourceRuntime),
		).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_HEALTH")
	}

	dailyBuckets := make(map[string]*dailyBucket)
	var inv7d, succ7d int
	var routed7d, loaded7d int
	var durations7d []int
	var inv30d, succ30d int
	var routed30d, loaded30d int
	var durations30d []int

	for _, row := range rows {
		day := types.DayFromCreatedAt(row.CreatedAt)
		b, ok := dailyBuckets[day]
		if !ok {
			b = &dailyBucket{}
			dailyBuckets[day] = b
		}
		b.count++
		b.totalDurationMs += row.DurationMs
		if types.IsSuccess(row.Outcome, row.Status) {
			b.successes++
		}

		// Track route hit rate.
		hasRouted := len(row.RoutedSlugs) > 0
		hasLoaded := strings.TrimSpace(row.LoadedSlug) != ""
		if hasRouted {
			b.routedCount++
		}
		if hasLoaded {
			b.loadedCount++
		}

		// 30d window
		inv30d++
		if types.IsSuccess(row.Outcome, row.Status) {
			succ30d++
		}
		if hasRouted {
			routed30d++
		}
		if hasLoaded {
			loaded30d++
		}
		durations30d = append(durations30d, row.DurationMs)

		// 7d window
		if row.CreatedAt >= since7d.Format(time.RFC3339) {
			inv7d++
			if types.IsSuccess(row.Outcome, row.Status) {
				succ7d++
			}
			if hasRouted {
				routed7d++
			}
			if hasLoaded {
				loaded7d++
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
			RoutedCount:   b.routedCount,
			LoadedCount:   b.loadedCount,
		})
	}
	sort.Slice(dailyMetrics, func(i, j int) bool { return dailyMetrics[i].Date < dailyMetrics[j].Date })

	result := &types.SkillHealthDetail{
		SkillID:             skillID,
		TotalInvocations7d:  inv7d,
		SuccessCount7d:      succ7d,
		SuccessRate7d:       types.SafeRate(succ7d, inv7d),
		P95DurationMs7d:     types.P95(durations7d),
		RouteHitRate7d:      types.SafeRate(loaded7d, routed7d),
		TotalInvocations30d: inv30d,
		SuccessCount30d:     succ30d,
		SuccessRate30d:      types.SafeRate(succ30d, inv30d),
		P95DurationMs30d:    types.P95(durations30d),
		RouteHitRate30d:     types.SafeRate(loaded30d, routed30d),
		DailyMetrics:        dailyMetrics,
	}
	return result, nil
}

type dailyBucket struct {
	count           int
	successes       int
	totalDurationMs int
	routedCount     int
	loadedCount     int
}
