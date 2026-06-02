package biz

import (
	"context"
	"time"
)

type SkillInvocationStat struct {
	SkillName     string
	Count         int
	SuccessRate   float64
	AvgDurationMs int64
}

type SkillInvocationStatsReader interface {
	GetSkillInvocationStats(ctx context.Context, agentID string, since time.Time) ([]SkillInvocationStat, error)
}
