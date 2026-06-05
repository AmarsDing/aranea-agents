package biz

import (
	"context"
	"time"
)

// ExperienceReportReader reads experience reports.
type ExperienceReportReader interface {
	// ListBySkill returns experience reports for a given skill, ordered by created_at desc.
	ListBySkill(ctx context.Context, skillID string, limit, offset int) ([]ExperienceReport, error)
	// GetByID returns a single experience report by ID.
	GetByID(ctx context.Context, id string) (*ExperienceReport, error)
	// ListByTimeRange returns experience reports within a time range, ordered by created_at desc.
	ListByTimeRange(ctx context.Context, from, to time.Time, limit, offset int) ([]ExperienceReport, error)
}

// ExperienceReportWriter writes experience reports.
type ExperienceReportWriter interface {
	// Create persists a new experience report.
	Create(ctx context.Context, report ExperienceReport) error
	// BatchCreate persists multiple experience reports in a single transaction.
	BatchCreate(ctx context.Context, reports []ExperienceReport) error
}

// SkillHealthAggregator computes aggregate health metrics from skill_invocation data.
// It reuses the same data source as SkillHealthReader but provides additional
// aggregation methods needed by SkillIntelligenceUsecase.
type SkillHealthAggregator interface {
	// GetHealthMetrics returns health metrics (success rate, avg latency) for a skill
	// over the given time window.
	GetHealthMetrics(ctx context.Context, skillID string, since time.Time) (*SkillHealthMetrics, error)
	// GetFailureStats returns failure statistics for a skill over the given time window.
	GetFailureStats(ctx context.Context, skillID string, since time.Time) (*SkillFailureStats, error)
}

// SkillHealthMetrics holds aggregated health metrics for a single skill.
type SkillHealthMetrics struct {
	SkillID       string
	InvocationCount int
	SuccessCount    int
	SuccessRate     float64
	AvgDurationMS   float64
	P95DurationMS   int
}

// SkillFailureStats holds failure statistics for a single skill.
type SkillFailureStats struct {
	SkillID       string
	FailureCount  int
	TopErrorCodes []ErrorCodeCount
}

// ErrorCodeCount is a count of failures by error code.
type ErrorCodeCount struct {
	ErrorCode string
	Count     int
}
