package biz

// TeamCompleteEvent is emitted when a team_run finishes.
// Used by PlanExecutor to correlate team completion with PlanStep status.
// This is NOT an ActivityEvent — it is a lightweight signal carried over a
// channel from TeamOrchestrator to PlanExecutor.
type TeamCompleteEvent struct {
	StepID    string // the PlanStep that triggered this team
	TeamRunID string
	Success   bool
	ErrorMsg  string
}
