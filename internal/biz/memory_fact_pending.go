package biz

import "context"

// Memory fact write approval (79-runtime-governance R3): high-risk verdicts
// from the automatic fact write pipeline are withheld from storage and land
// in memory_fact_pending for human approval instead. ADD and NOOP verdicts
// write directly (unchanged). Approval executes the original bi-temporal
// write; rejection leaves the target fact untouched. The gate is
// unconditional — 免确认模式不免除记忆审批 (dev plan Phase 3 验证项).

const (
	// MemoryFactPendingVerdictUpdate — adjudicated UPDATE of an existing fact.
	MemoryFactPendingVerdictUpdate = "UPDATE"
	// MemoryFactPendingVerdictDelete — adjudicated DELETE of an existing fact.
	MemoryFactPendingVerdictDelete = "DELETE"
	// MemoryFactPendingVerdictContested — candidate had conflict-band neighbors
	// but no definitive adjudication verdict (adjudicator missing/error, verdict
	// missing, unknown operation, or UPDATE/DELETE target not among neighbors).
	// Writing blindly would duplicate/conflict; a human must arbitrate.
	MemoryFactPendingVerdictContested = "CONTESTED"
)

const (
	MemoryFactPendingStatusPending  = "pending"
	MemoryFactPendingStatusApproved = "approved"
	MemoryFactPendingStatusRejected = "rejected"
)

// MemoryFactPendingRecord is one withheld high-risk write awaiting decision.
type MemoryFactPendingRecord struct {
	ID                string
	AgentID           string
	FactKey           string // target fact id for UPDATE/DELETE; "" for CONTESTED
	Verdict           string // UPDATE | DELETE | CONTESTED
	ProposedBody      string // candidate statement
	PriorBody         string // current statement of the target fact ("" unknown)
	AdjudicatorReason string
	Status            string // pending | approved | rejected
	Approver          string
	CreatedAt         int64 // unix seconds
	DecidedAt         int64 // unix seconds; 0 while pending
}

// MemoryFactPendingStore is the persistence port for withheld writes
// (implemented raw-SQL in internal/data, table from DDL 20261249).
// Stability:evolving
type MemoryFactPendingStore interface {
	// InsertPending persists a withheld write (idempotent on ID).
	InsertPending(ctx context.Context, rec MemoryFactPendingRecord) error
	// GetPending returns one record by id; found=false when absent.
	GetPending(ctx context.Context, id string) (rec MemoryFactPendingRecord, found bool, err error)
	// ListPending lists records, newest first. Empty agentID/status match all.
	ListPending(ctx context.Context, agentID, status string, limit int) ([]MemoryFactPendingRecord, error)
	// MarkDecided transitions a pending row to approved/rejected with the
	// approver identity. Fail-closed: only rows still pending are decidable —
	// returns applied=false when the row is absent or already decided (double
	// decision race).
	MarkDecided(ctx context.Context, id, status, approver string, decidedAt int64) (applied bool, err error)
}

// RouteFactWriteDecision is the R3 verdict gate (pure): ADD and NOOP write
// directly (unchanged behavior); UPDATE / DELETE always pend; a contested
// candidate without a definitive adjudication verdict pends as CONTESTED
// instead of heuristic-ADD writing past a known conflict neighbor.
func RouteFactWriteDecision(d FactWriteDecision) (verdict string, pend bool) {
	switch d.Operation {
	case FactWriteOpUpdate:
		return MemoryFactPendingVerdictUpdate, true
	case FactWriteOpDelete:
		return MemoryFactPendingVerdictDelete, true
	case FactWriteOpAdd:
		if d.Contested && !d.Adjudicated {
			return MemoryFactPendingVerdictContested, true
		}
	}
	return "", false
}
