package biz

// Session state keys mirrored from internal/service/run_status_store (list projection only).
const (
	SessionStateRunStatus = "runtime.status"
)

// AgentListExtras is list-row enrichment (not persisted on agents table).
type AgentListExtras struct {
	LastRunStatus         string
	LastRunAt             string
	PendingEvolutionCount int
}
