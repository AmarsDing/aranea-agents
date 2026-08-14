package data

import (
	"context"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/types"
	"aranea-agents/internal/data/ent/platformskill"
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

	// 调用量/成功率/延迟聚合下推 SQL：不再把 30 天全量行（含大 JSON 列）拉进
	// 内存。日桶按 created_at 前 10 字符（与 types.DayFromCreatedAt 同口径）
	// GROUP BY；窗口计数/时长用条件聚合一次扫出。
	// 成功判定与 biz/types.IsSuccess 语义一致（S4 同口径）。
	d := r.data.Dialect()
	aggQuery := d.RenumberPlaceholders(`SELECT
  substr(si.created_at, 1, 10) AS day,
  COUNT(*) AS cnt,
  SUM(CASE WHEN si.outcome = 'success' OR (si.outcome = '' AND si.status IN ('completed', 'success')) THEN 1 ELSE 0 END) AS succ,
  SUM(si.duration_ms) AS total_dur,
  SUM(CASE WHEN si.created_at >= ? THEN 1 ELSE 0 END) AS cnt7d,
  SUM(CASE WHEN si.created_at >= ? AND (si.outcome = 'success' OR (si.outcome = '' AND si.status IN ('completed', 'success'))) THEN 1 ELSE 0 END) AS succ7d
FROM skill_invocation si
WHERE si.skill_id = ? AND si.created_at >= ? AND si.source = 'runtime'
GROUP BY substr(si.created_at, 1, 10)`)
	aggRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, aggQuery, since7dStr, since7dStr, skillID, since30dStr)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer aggRows.Close()

	dailyBuckets := make(map[string]*dailyBucket)
	var inv7d, succ7d, inv30d, succ30d int
	for aggRows.Next() {
		var day string
		var cnt, succ, totalDur, cnt7, succ7 int
		if err := aggRows.Scan(&day, &cnt, &succ, &totalDur, &cnt7, &succ7); err != nil {
			return nil, entErrToBizErr(err, apierror.DomainSkill)
		}
		dailyBuckets[day] = &dailyBucket{count: cnt, successes: succ, totalDurationMs: totalDur}
		inv30d += cnt
		succ30d += succ
		inv7d += cnt7
		succ7d += succ7
	}
	if err := aggRows.Err(); err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}

	// P95 需要原始时长分布，无法下推；仅取 duration_ms 单列（裁剪大 JSON 列）。
	durations30d, durations7d, err := r.windowDurations(ctx, skillID, since30dStr, since7dStr)
	if err != nil {
		return nil, err
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

// windowDurations 拉取 30d/7d 窗口的调用时长（duration_ms 单列），供 P95 计算。
// 只裁剪到所需列，避免把 selection_reason/token_usage 等大 JSON 列读进内存。
func (r *skillHealthRepo) windowDurations(ctx context.Context, skillID, since30dStr, since7dStr string) (durations30d, durations7d []int, err error) {
	d := r.data.Dialect()
	q := d.RenumberPlaceholders(`SELECT si.duration_ms, si.created_at
FROM skill_invocation si
WHERE si.skill_id = ? AND si.created_at >= ? AND si.source = 'runtime'`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, skillID, since30dStr)
	if err != nil {
		return nil, nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer rows.Close()

	for rows.Next() {
		var dur int
		var createdAt string
		if err := rows.Scan(&dur, &createdAt); err != nil {
			return nil, nil, entErrToBizErr(err, apierror.DomainSkill)
		}
		durations30d = append(durations30d, dur)
		if createdAt >= since7dStr {
			durations7d = append(durations7d, dur)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	return durations30d, durations7d, nil
}

// routeLoadCounts 统计 slug 被路由/被加载的去重轮次（30d/7d 窗口 + 按日分桶）。
// 信号行 = 任意 Skill 的运行时调用记录中 routed_slugs 含 slug 或 loaded_slug = slug。
// 去重按 activation_id（为空时回退行 id，与原内存口径一致）下推 SQL COUNT DISTINCT，
// 不再把 jsonb 全量行拉进内存去重。
func (r *skillHealthRepo) routeLoadCounts(ctx context.Context, slug, since30dStr, since7dStr string, dailyBuckets map[string]*dailyBucket) (routed30d, loaded30d, routed7d, loaded7d int, err error) {
	if slug == "" {
		return 0, 0, 0, 0, nil
	}

	d := r.data.Dialect()
	signalFilter := `(si.routed_slugs @> to_jsonb(ARRAY[?]::text[]) OR si.loaded_slug = ?)`
	// 窗口聚合一枪算出 30d/7d 去重轮次。
	windowQuery := d.RenumberPlaceholders(`SELECT
  COUNT(DISTINCT CASE WHEN si.routed_slugs @> to_jsonb(ARRAY[?]::text[]) THEN COALESCE(NULLIF(si.activation_id, ''), 'row:' || si.id) END) AS routed30d,
  COUNT(DISTINCT CASE WHEN si.loaded_slug = ? THEN COALESCE(NULLIF(si.activation_id, ''), 'row:' || si.id) END) AS loaded30d,
  COUNT(DISTINCT CASE WHEN si.routed_slugs @> to_jsonb(ARRAY[?]::text[]) AND si.created_at >= ? THEN COALESCE(NULLIF(si.activation_id, ''), 'row:' || si.id) END) AS routed7d,
  COUNT(DISTINCT CASE WHEN si.loaded_slug = ? AND si.created_at >= ? THEN COALESCE(NULLIF(si.activation_id, ''), 'row:' || si.id) END) AS loaded7d
FROM skill_invocation si
WHERE si.created_at >= ? AND si.source = 'runtime' AND ` + signalFilter)
	wRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, windowQuery,
		slug, slug, slug, since7dStr, slug, since7dStr, since30dStr, slug, slug)
	if err != nil {
		return 0, 0, 0, 0, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer wRows.Close()
	if wRows.Next() {
		if err := wRows.Scan(&routed30d, &loaded30d, &routed7d, &loaded7d); err != nil {
			return 0, 0, 0, 0, entErrToBizErr(err, apierror.DomainSkill)
		}
	}
	if err := wRows.Err(); err != nil {
		return 0, 0, 0, 0, entErrToBizErr(err, apierror.DomainSkill)
	}

	// 日桶去重轮次：按日（created_at 前 10 字符）GROUP BY，SQL 侧 COUNT DISTINCT。
	dailyQuery := d.RenumberPlaceholders(`SELECT
  substr(si.created_at, 1, 10) AS day,
  COUNT(DISTINCT CASE WHEN si.routed_slugs @> to_jsonb(ARRAY[?]::text[]) THEN COALESCE(NULLIF(si.activation_id, ''), 'row:' || si.id) END) AS routed,
  COUNT(DISTINCT CASE WHEN si.loaded_slug = ? THEN COALESCE(NULLIF(si.activation_id, ''), 'row:' || si.id) END) AS loaded
FROM skill_invocation si
WHERE si.created_at >= ? AND si.source = 'runtime' AND ` + signalFilter + `
GROUP BY substr(si.created_at, 1, 10)`)
	dRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, dailyQuery, slug, slug, since30dStr, slug, slug)
	if err != nil {
		return 0, 0, 0, 0, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer dRows.Close()
	for dRows.Next() {
		var day string
		var routed, loaded int
		if err := dRows.Scan(&day, &routed, &loaded); err != nil {
			return 0, 0, 0, 0, entErrToBizErr(err, apierror.DomainSkill)
		}
		b, ok := dailyBuckets[day]
		if !ok {
			b = &dailyBucket{}
			dailyBuckets[day] = b
		}
		b.routedCount = routed
		b.loadedCount = loaded
	}
	if err := dRows.Err(); err != nil {
		return 0, 0, 0, 0, entErrToBizErr(err, apierror.DomainSkill)
	}

	return routed30d, loaded30d, routed7d, loaded7d, nil
}

type dailyBucket struct {
	count           int
	successes       int
	totalDurationMs int
	routedCount     int
	loadedCount     int
}
