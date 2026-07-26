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
	// ListFiltered returns experience reports with optional skillID and time range filters.
	// skillID empty = no skill filter; startTime/endTime nil = no time boundary.
	// Returns the matched reports and total count for pagination.
	ListFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time, limit, offset int) ([]ExperienceReport, int, error)
}

// ExperienceReportStatsReader reads aggregate statistics for experience reports.
type ExperienceReportStatsReader interface {
	// GetFailureTagCountsFiltered returns failure tag counts with optional skillID and time range filters.
	// skillID empty = no skill filter; startTime/endTime nil = no time boundary.
	GetFailureTagCountsFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time) ([]FailureTagCount, error)
	// GetRootCauseReportsFiltered returns experience reports that have root cause analysis,
	// with optional skillID and time range filters, ordered by created_at desc.
	GetRootCauseReportsFiltered(ctx context.Context, skillID string, startTime, endTime *time.Time, limit int) ([]ExperienceReport, error)
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
	// GetFailureTagCounts returns failure tag counts for a skill over the given time window.
	// Used by Curator Agent to detect repeated failure patterns (e.g., same tag >= 5 times).
	GetFailureTagCounts(ctx context.Context, skillID string, since time.Time) ([]FailureTagCount, error)
}

// SkillHealthMetrics holds aggregated health metrics for a single skill.
type SkillHealthMetrics struct {
	SkillID         string
	InvocationCount int
	SuccessCount    int
	SuccessRate     float64
	AvgDurationMS   float64
	P95DurationMS   int
	AvgTokenUsage   int     // average token_usage.total across invocations; 0 = unavailable
	FeedbackScore   float64 // heuristic feedback score 0-1; 0 = unavailable
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

// FailureTagCount is a count of failures by failure tag.
type FailureTagCount struct {
	Tag   string
	Count int
}

// SkillInvocationUnanalyzedReader reads unanalyzed skill invocations for batch processing.
type SkillInvocationUnanalyzedReader interface {
	// ListUnanalyzed returns skill invocations that have not been analyzed yet
	// (analyzed_at is empty), ordered by created_at asc, limited to batchSize.
	ListUnanalyzed(ctx context.Context, batchSize int) ([]SkillInvocationWrite, error)
	// MarkAnalyzed sets the analyzed_at timestamp for a skill invocation.
	MarkAnalyzed(ctx context.Context, id string) error
}
