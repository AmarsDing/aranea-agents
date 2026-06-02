package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz"

	toolinvocationpkg "aranea-agents/internal/data/ent/toolinvocation"
)

type skillInvocationStatsRepo struct {
	data *Data
}

var _ biz.SkillInvocationStatsReader = (*skillInvocationStatsRepo)(nil)

func NewSkillInvocationStatsRepo(data *Data) biz.SkillInvocationStatsReader {
	return &skillInvocationStatsRepo{data: data}
}

func (r *skillInvocationStatsRepo) GetSkillInvocationStats(ctx context.Context, agentID string, since time.Time) ([]biz.SkillInvocationStat, error) {
	rows, err := r.data.Ent().ToolInvocation.Query().
		Where(
			toolinvocationpkg.AgentIDEQ(agentID),
			toolinvocationpkg.CreatedAtGTE(since.Format(time.RFC3339)),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	buckets := make(map[string]*skillStatBucket)
	for _, row := range rows {
		b, ok := buckets[row.ToolKey]
		if !ok {
			b = &skillStatBucket{}
			buckets[row.ToolKey] = b
		}
		b.count++
		b.totalDurationMs += int64(row.DurationMs)
		if row.Status == "success" {
			b.successCount++
		}
	}
	result := make([]biz.SkillInvocationStat, 0, len(buckets))
	for name, b := range buckets {
		rate := 0.0
		if b.count > 0 {
			rate = float64(b.successCount) / float64(b.count)
		}
		avgMs := int64(0)
		if b.count > 0 {
			avgMs = b.totalDurationMs / int64(b.count)
		}
		result = append(result, biz.SkillInvocationStat{
			SkillName:     name,
			Count:         b.count,
			SuccessRate:   rate,
			AvgDurationMs: avgMs,
		})
	}
	return result, nil
}

type skillStatBucket struct {
	count            int
	successCount     int
	totalDurationMs  int64
}
