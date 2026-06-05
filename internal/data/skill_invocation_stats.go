package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const toolInvocationStatusSuccess = "success"

type skillInvocationStatsRepo struct {
	data *Data
}

var _ biz.SkillInvocationStatsReader = (*skillInvocationStatsRepo)(nil)

func NewSkillInvocationStatsRepo(data *Data) biz.SkillInvocationStatsReader {
	return &skillInvocationStatsRepo{data: data}
}

func (r *skillInvocationStatsRepo) GetSkillInvocationStats(ctx context.Context, agentID string, since time.Time) ([]biz.SkillInvocationStat, error) {
	q := `SELECT tool_key, COUNT(*) as cnt, SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as success_cnt, COALESCE(SUM(duration_ms), 0) as total_dur FROM tool_invocations WHERE agent_id = ? AND created_at >= ? GROUP BY tool_key`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, toolInvocationStatusSuccess, agentID, since.Format(time.RFC3339))
	if err != nil {
		return nil, kerrors.InternalServer("SKILL_STATS", "query skill invocation stats: "+err.Error())
	}
	defer rows.Close()

	var result []biz.SkillInvocationStat
	for rows.Next() {
		var name string
		var count int
		var successCount int
		var totalDurationMs int64
		if err := rows.Scan(&name, &count, &successCount, &totalDurationMs); err != nil {
			return nil, kerrors.InternalServer("SKILL_STATS", "scan skill invocation stat: "+err.Error())
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
	return result, nil
}
