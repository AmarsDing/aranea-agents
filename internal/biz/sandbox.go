package biz

import "context"

// SandboxView is the admin-facing snapshot of one live sandbox (M82).
// Timestamps are RFC3339 strings (empty when unset) for wire-readiness.
type SandboxView struct {
	ID         string `json:"id"`
	Profile    string `json:"profile"`
	AgentKey   string `json:"agent_key,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	State      string `json:"state"`
	CreatedAt  string `json:"created_at"`
	Deadline   string `json:"deadline"`
	LastExecAt string `json:"last_exec_at,omitempty"`
	ExecCount  int64  `json:"exec_count"`
}

// SandboxProfileMetrics is the per-profile water level.
type SandboxProfileMetrics struct {
	Profile string `json:"profile"`
	Ready   int    `json:"ready"`
	Active  int    `json:"active"`
}

// SandboxMetrics is the admin-facing metrics snapshot (counters since boot).
type SandboxMetrics struct {
	Profiles    []SandboxProfileMetrics `json:"profiles"`
	GlobalActive int                     `json:"global_active"`
	AcquireWarm int64                    `json:"acquire_warm"`
	AcquireCold int64                    `json:"acquire_cold"`
	AcquireFail int64                    `json:"acquire_fail"`
	ExecOK      int64                    `json:"exec_ok"`
	ExecError   int64                    `json:"exec_error"`
	ExecTimeout int64                    `json:"exec_timeout"`
	Destroy     map[string]int64         `json:"destroy"`
	QuotaReject map[string]int64         `json:"quota_reject"`
}

// SandboxAdminPort is the narrow port between the transport service and
// internal/sandbox.Manager (design §5.3). Methods are Admin-prefixed so the
// Manager can also expose consumer-surface names without collisions.
//
// Wired via wire.Bind onto *sandbox.Manager.
type SandboxAdminPort interface {
	AdminSandboxList(ctx context.Context) ([]SandboxView, error)
	AdminSandboxForceKill(ctx context.Context, id, reason, operator string) error
	AdminSandboxMetrics(ctx context.Context) (*SandboxMetrics, error)
}
