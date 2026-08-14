package heal

import (
	"context"
	"time"
)

// FailurePatternSource classifies where a failure pattern originates.
type FailurePatternSource string

const (
	FailurePatternSourceRuntime FailurePatternSource = "runtime"
	FailurePatternSourceCI      FailurePatternSource = "ci"
	FailurePatternSourceMined   FailurePatternSource = "mined"
)

// FailurePattern is a domain model representing a unified failure pattern in the knowledge base.
type FailurePattern struct {
	ID           string               `json:"id"`
	Source       FailurePatternSource `json:"source"`
	Type         string               `json:"type"`
	PatternHash  string               `json:"pattern_hash"`
	PatternRegex string               `json:"pattern_regex"`
	FixAction    FixAction            `json:"fix_action"`
	Confidence   float64              `json:"confidence"`
	SuccessCount int                  `json:"success_count"`
	FailCount    int                  `json:"fail_count"`
	Version      int                  `json:"version"`
	IsActive     bool                 `json:"is_active"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

// FailurePatternReader provides read access to the failure pattern knowledge base.
type FailurePatternReader interface {
	// ListBySource returns all patterns matching the given source.
	ListBySource(ctx context.Context, source FailurePatternSource) ([]FailurePattern, error)
	// GetByPatternHash returns the pattern with the given hash, or nil if not found.
	GetByPatternHash(ctx context.Context, hash string) (*FailurePattern, error)
	// ListActive returns all active patterns, ordered by confidence descending.
	ListActive(ctx context.Context) ([]FailurePattern, error)
}

// FailurePatternWriter provides write access to the failure pattern knowledge base.
type FailurePatternWriter interface {
	// Create inserts a new failure pattern.
	Create(ctx context.Context, pattern FailurePattern) error
	// Update updates an existing failure pattern by ID.
	Update(ctx context.Context, pattern FailurePattern) error
	// IncrementSuccess atomically increments the success_count for the pattern with the given ID.
	IncrementSuccess(ctx context.Context, id string) error
	// IncrementFail atomically increments the fail_count for the pattern with the given ID.
	IncrementFail(ctx context.Context, id string) error
	// Deactivate sets is_active = false for the pattern with the given ID.
	Deactivate(ctx context.Context, id string) error
}
