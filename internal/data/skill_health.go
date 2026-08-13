package data

import (
	"context"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/types"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/pkg/apierror"
)

type skillHealthRepo struct {
	data *Data
}

var _ biz.SkillHealthReader = (*skillHealthRepo)(nil)

func NewSkillHealthRepo(data *Data) biz.SkillHealthReader {
	return &skillHealthRepo{data: data}
}

// GetSkillHealth 聚合单个 Skill 的健康指标。
//
// 调用量/成功率/延迟口径：skill_id = 本 Skill 的运行时调用记录。
//
// 路由命中率口径（A1 精确匹配修复）：RouteHitRate = 本 Skill 被加载的去重轮次 /
// 本 Skill 被路由的去重轮次。轮次按 activation_id 去重（为空时按行 id），避免同轮
// 多次工具调用重复计数。分母来自所有 Skill 的调用记录中 routed_slugs 含本 Skill
// slug 的行（jsonb 包含查询），分子来自 loaded_slug = 本 Skill slug 的行。
//
// 已知残留：一轮路由后未调用任何 skill 工具时不产生调用记录，该轮不计入分母
// （彻底补齐需 turn 级路由落库，见 20-skill.design.md §7.10）。
func (r *skillHealthRepo) GetSkillHealth(ctx context.Context, skillID string, since7d, since30d time.Time) (*types.SkillHealthDetail, error) {
	since30dStr := since30d.Format(time.RFC3339)
	since7dStr := since7d.Format(time.RFC3339)

	slug, err := r.skillSlug(ctx, skillID)
	if err != nil {
		return nil, err
	}

	rows, err := r.data.RW().Read(ctx).SkillInvocation.Query().
		Where(
			skillinvocation.SkillIDEQ(skillID),
			skillinvocation.CreatedAtGTE(since30dStr),
			// 只统计真实运行时调用；filesystem_* 同步记录不参与健康指标。
			skillinvocation.SourceEQ(biz.SkillInvocationSourceRuntime),
		).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}

	dailyBuckets := make(map[string]*dailyBucket)
	var inv7d, succ7d int
	var durations7d []int
	var inv30d, succ30d int
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

		// 30d window
		inv30d++
		if types.IsSuccess(row.Outcome, row.Status) {
			succ30d++
		}
		durations30d = append(durations30d, row.DurationMs)

		// 7d window
		if row.CreatedAt >= since7dStr {
			inv7d++
			if types.IsSuccess(row.Outcome, row.Status) {
				succ7d++
			}
			durations7d = append(durations7d, row.DurationMs)
		}
	}

	// 路由/加载信号：跨 Skill 聚合本 slug 被路由/被加载的去重轮次。
	routed30d, loaded30d, routed7d, loaded7d, err := r.routeLoadCounts(ctx, slug, since30dStr, since7dStr, dailyBuckets)
	if err != nil {
		return nil, err
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
			RoutedCount:   len(b.routedActs),
			LoadedCount:   len(b.loadedActs),
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
		RoutedCount7d:       routed7d,
		LoadedCount7d:       loaded7d,
		RoutedCount30d:      routed30d,
		LoadedCount30d:      loaded30d,
		DailyMetrics:        dailyMetrics,
	}
	return result, nil
}

// skillSlug 解析 Skill 的规范 slug（routed_slugs/loaded_slug 均按 slug 记录）。
func (r *skillHealthRepo) skillSlug(ctx context.Context, skillID string) (string, error) {
	sk, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDEQ(skillID)).
		Select(platformskill.FieldSkillKey).
		Only(ctx)
	if err != nil {
		return "", entErrToBizErr(err, apierror.DomainSkill)
	}
	return strings.TrimSpace(sk.SkillKey), nil
}

// routeLoadCounts 统计 slug 被路由/被加载的去重轮次（30d/7d 窗口 + 按日分桶）。
// 信号行 = 任意 Skill 的运行时调用记录中 routed_slugs 含 slug 或 loaded_slug = slug。
func (r *skillHealthRepo) routeLoadCounts(ctx context.Context, slug, since30dStr, since7dStr string, dailyBuckets map[string]*dailyBucket) (routed30d, loaded30d, routed7d, loaded7d int, err error) {
	if slug == "" {
		return 0, 0, 0, 0, nil
	}
	rows, err := r.data.RW().Read(ctx).SkillInvocation.Query().
		Where(
			skillinvocation.CreatedAtGTE(since30dStr),
			skillinvocation.SourceEQ(biz.SkillInvocationSourceRuntime),
			skillinvocation.Or(
				func(s *sql.Selector) {
					s.Where(sqljson.ValueContains(skillinvocation.FieldRoutedSlugs, slug))
				},
				skillinvocation.LoadedSlugEQ(slug),
			),
		).
		Select(
			skillinvocation.FieldID,
			skillinvocation.FieldActivationID,
			skillinvocation.FieldCreatedAt,
			skillinvocation.FieldRoutedSlugs,
			skillinvocation.FieldLoadedSlug,
		).
		All(ctx)
	if err != nil {
		return 0, 0, 0, 0, entErrToBizErr(err, apierror.DomainSkill)
	}

	routedActs30d := make(map[string]bool)
	loadedActs30d := make(map[string]bool)
	routedActs7d := make(map[string]bool)
	loadedActs7d := make(map[string]bool)

	for _, row := range rows {
		turnKey := strings.TrimSpace(row.ActivationID)
		if turnKey == "" {
			turnKey = "row:" + row.ID
		}
		routed := containsString(row.RoutedSlugs, slug)
		loaded := strings.TrimSpace(row.LoadedSlug) == slug

		if routed {
			routedActs30d[turnKey] = true
		}
		if loaded {
			loadedActs30d[turnKey] = true
		}
		if row.CreatedAt >= since7dStr {
			if routed {
				routedActs7d[turnKey] = true
			}
			if loaded {
				loadedActs7d[turnKey] = true
			}
		}

		day := types.DayFromCreatedAt(row.CreatedAt)
		b, ok := dailyBuckets[day]
		if !ok {
			b = &dailyBucket{}
			dailyBuckets[day] = b
		}
		if routed {
			if b.routedActs == nil {
				b.routedActs = make(map[string]bool)
			}
			b.routedActs[turnKey] = true
		}
		if loaded {
			if b.loadedActs == nil {
				b.loadedActs = make(map[string]bool)
			}
			b.loadedActs[turnKey] = true
		}
	}

	return len(routedActs30d), len(loadedActs30d), len(routedActs7d), len(loadedActs7d), nil
}

func containsString(items []string, want string) bool {
	for _, it := range items {
		if it == want {
			return true
		}
	}
	return false
}

type dailyBucket struct {
	count           int
	successes       int
	totalDurationMs int
	routedActs      map[string]bool
	loadedActs      map[string]bool
}
