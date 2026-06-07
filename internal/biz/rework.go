package biz

// ReworkStrategy defines how a team should be re-executed after rejection.
type ReworkStrategy string

const (
	ReworkStrategyFullTeam ReworkStrategy = "full_team" // Re-execute entire team (initial implementation)
)

// ReworkConfig defines the rework configuration.
type ReworkConfig struct {
	Strategy   ReworkStrategy `json:"strategy"`
	MaxRetries int            `json:"max_retries"` // default 3
}

// DefaultReworkConfig returns the default rework configuration.
func DefaultReworkConfig() ReworkConfig {
	return ReworkConfig{
		Strategy:   ReworkStrategyFullTeam,
		MaxRetries: 3,
	}
}

// ReworkTracker tracks rework attempts for a team.
type ReworkTracker struct {
	TeamID     string
	Attempt    int
	MaxRetries int
	LastReason string
}

// CanRetry returns true if the team can be reworked again.
func (r *ReworkTracker) CanRetry() bool {
	return r.Attempt < r.MaxRetries
}

// IncrementAttempt increments the rework attempt counter and returns the new count.
func (r *ReworkTracker) IncrementAttempt() int {
	r.Attempt++
	return r.Attempt
}
