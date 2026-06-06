package biz

import (
	"context"
	"encoding/json"
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
	// GetFailureTagCounts returns failure tag counts for a skill over the given time window.
	// Used by Curator Agent to detect repeated failure patterns (e.g., same tag >= 5 times).
	GetFailureTagCounts(ctx context.Context, skillID string, since time.Time) ([]FailureTagCount, error)
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

// SkillEvolutionSuggestionReader reads skill evolution suggestions.
type SkillEvolutionSuggestionReader interface {
	// ListBySkill returns evolution suggestions for a given skill, optionally filtered by status.
	ListBySkill(ctx context.Context, skillID string, status EvolutionSuggestionStatus, limit, offset int) ([]SkillEvolutionSuggestion, error)
	// GetByID returns a single evolution suggestion by ID.
	GetByID(ctx context.Context, id string) (*SkillEvolutionSuggestion, error)
	// ListPending returns pending evolution suggestions, ordered by created_at desc.
	ListPending(ctx context.Context, limit, offset int) ([]SkillEvolutionSuggestion, error)
	// GetLatestBySkill returns the most recent evolution suggestion for a skill.
	GetLatestBySkill(ctx context.Context, skillID string) (*SkillEvolutionSuggestion, error)
}

// SkillEvolutionSuggestionWriter writes skill evolution suggestions.
type SkillEvolutionSuggestionWriter interface {
	// Create persists a new evolution suggestion.
	Create(ctx context.Context, suggestion SkillEvolutionSuggestion) error
	// UpdateStatus updates the status and resolution info of an evolution suggestion.
	UpdateStatus(ctx context.Context, id string, status EvolutionSuggestionStatus, resolvedBy string, reason string) error
	// UpdateDraftBody updates the draft skill body of an evolution suggestion.
	UpdateDraftBody(ctx context.Context, id string, draftBody string) error
	// UpdateSandboxResult updates the sandbox validation result of an evolution suggestion.
	UpdateSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error
	// UpdateLifecycleStatus updates the lifecycle status of an evolution suggestion.
	UpdateLifecycleStatus(ctx context.Context, id string, lifecycleStatus string) error
}

// SkillInvocationUnanalyzedReader reads unanalyzed skill invocations for batch processing.
type SkillInvocationUnanalyzedReader interface {
	// ListUnanalyzed returns skill invocations that have not been analyzed yet
	// (analyzed_at is empty), ordered by created_at asc, limited to batchSize.
	ListUnanalyzed(ctx context.Context, batchSize int) ([]SkillInvocationWrite, error)
	// MarkAnalyzed sets the analyzed_at timestamp for a skill invocation.
	MarkAnalyzed(ctx context.Context, id string) error
}
