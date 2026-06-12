package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
)

type skillInvocationStatsRepo struct {
	data *Data
}

var _ biz.SkillInvocationStatsReader = (*skillInvocationStatsRepo)(nil)

func NewSkillInvocationStatsRepo(data *Data) biz.SkillInvocationStatsReader {
	return &skillInvocationStatsRepo{data: data}
}

func (r *skillInvocationStatsRepo) GetSkillInvocationStats(ctx context.Context, agentID string, since time.Time) ([]biz.SkillInvocationStat, error) {
	// Query skill_invocation (not tool_invocations) and join platform_skill
	// to resolve skill_key as the human-readable identifier.
	const q = `SELECT ps.skill_key, COUNT(*) as cnt,
       SUM(CASE WHEN si.status = 'success' THEN 1 ELSE 0 END) as success_cnt,
       COALESCE(SUM(si.duration_ms), 0) as total_dur
FROM skill_invocation si
JOIN platform_skill ps ON ps.id = si.skill_id AND ps.deleted_at = ''
WHERE si.agent_id = ? AND COALESCE(NULLIF(si.started_at, ''), si.created_at) >= ?
GROUP BY ps.skill_key`
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx, q, agentID, since.Format(time.RFC3339))
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_STATS")
	}
	defer rows.Close()

	var result []biz.SkillInvocationStat
	for rows.Next() {
		var name string
		var count int
		var successCount int
		var totalDurationMs int64
		if err := rows.Scan(&name, &count, &successCount, &totalDurationMs); err != nil {
			return nil, entErrToBizErr(err, "SKILL_STATS")
		}
		rate := 0.0
		if count > 0 {
			rate = float64(successCount) / float64(count)
		}
		avgMs := int64(0)
		if count > 0 {
			avgMs = totalDurationMs / int64(count)
		}
		result = append(result, biz.SkillInvocationStat{
			SkillName:     name,
			Count:         count,
			SuccessRate:   rate,
			AvgDurationMs: avgMs,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "SKILL_STATS")
	}
	return result, nil
}
