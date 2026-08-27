package biz

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

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
	// PayloadJSON is the withheld FactWriteDecision snapshot (candidate full
	// fields + target id), serialized at pend time; approval replays the
	// original bi-temporal write from it (Phase 3.3, DDL 20261255).
	PayloadJSON string
	Status      string // pending | approved | rejected
	Approver    string
	CreatedAt   int64 // unix seconds
	DecidedAt   int64 // unix seconds; 0 while pending
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

// MemoryFactPendingCounter 是 pending 审批的 COUNT 窄能力（79-runtime-governance
// R8 P5.2）：ListPending 是 newest-first + limit 截断——diagnostics 的积压总数
// 与 stale 分档计数若取自截断列表，pending 超 limit 时总数显示失真，且最老的
// stale 行被截漏（>72h 积压恒 0 的假阴性）。diagnostics 经 type-assertion
// 解析本接口；无该能力的实现回落列表口径（主契约 MemoryFactPendingStore 不动、
// 测试替身零波及，同 R7 窄口模式）。
// Stability:evolving
type MemoryFactPendingCounter interface {
	// CountPendingByAge 对 status='pending' 行按 age 互斥分档 COUNT：
	// staleWarn = warnAgeSec < age <= failAgeSec；staleFail = age > failAgeSec
	// （与 audit.py §六分档语义一致）；total = 全部 pending 数。
	CountPendingByAge(ctx context.Context, warnAgeSec, failAgeSec, nowUnix int64) (total, staleWarn, staleFail int64, err error)
}

// ---------------------------------------------------------------------------
// Phase 3.4 — E4 四档决议（与工具 HITL 对齐）
// ---------------------------------------------------------------------------

// Four-tier decision for a withheld memory write, aligned with the tool HITL
// gate-card semantics (feishu_gate_card.go): approve (once) / deny /
// approve_session (always within the source session) / approve_always
// (persisted rule keyed by agent_id+verdict).
const (
	MemoryFactDecisionApprove        = "approve"
	MemoryFactDecisionDeny           = "deny"
	MemoryFactDecisionApproveSession = "approve_session"
	MemoryFactDecisionApproveAlways  = "approve_always"
)

// NormalizeMemoryFactDecision maps decision aliases onto the four canonical
// tiers (same alias set as the tool HITL bridge normalizer). ok=false for
// empty/unknown values — callers then fall back to the legacy approved bool.
func NormalizeMemoryFactDecision(raw string) (decision string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "approve", "approved", "__aranea:tool_confirm:approve":
		return MemoryFactDecisionApprove, true
	case "deny", "rejected", "reject", "__aranea:tool_confirm:deny":
		return MemoryFactDecisionDeny, true
	case "approve_session", "__aranea:tool_confirm:approve_session":
		return MemoryFactDecisionApproveSession, true
	case "approve_always", "always", "__aranea:tool_confirm:approve_always":
		return MemoryFactDecisionApproveAlways, true
	}
	return "", false
}

// MemoryFactAllowRule is one persisted approve_always grant: while the row
// exists, same (agent_id, verdict) writes bypass the pending gate.
type MemoryFactAllowRule struct {
	ID        string
	AgentID   string
	Verdict   string // UPDATE | DELETE | CONTESTED
	CreatedBy string // approver identity that granted the rule
	CreatedAt int64  // unix seconds
}

// MemoryFactAllowRuleStore is the persistence port for approve_always rules
// (implemented raw-SQL in internal/data, table from DDL 20261256).
// Stability:evolving
type MemoryFactAllowRuleStore interface {
	// GrantAllowRule persists a rule (idempotent on agent_id+verdict).
	GrantAllowRule(ctx context.Context, agentID, verdict, createdBy string) error
	// HasAllowRule reports whether a rule exists for agent_id+verdict.
	HasAllowRule(ctx context.Context, agentID, verdict string) (bool, error)
	// RevokeAllowRule deletes the rule; applied=false when absent.
	RevokeAllowRule(ctx context.Context, agentID, verdict string) (applied bool, err error)
	// ListAllowRules lists rules, newest first. Empty agentID matches all.
	ListAllowRules(ctx context.Context, agentID string, limit int) ([]MemoryFactAllowRule, error)
}

// MemoryFactSessionGrants holds process-local approve_session grants keyed by
// (agent_id, verdict, session_id) — same lifetime semantics as the tool HITL
// session grant store (restart clears; acceptable for session scope).
type MemoryFactSessionGrants struct {
	mu sync.Mutex
	ks map[string]struct{}
}

// NewMemoryFactSessionGrants builds an empty grant store.
func NewMemoryFactSessionGrants() *MemoryFactSessionGrants {
	return &MemoryFactSessionGrants{ks: make(map[string]struct{})}
}

func memoryFactSessionGrantKey(agentID, verdict, sessionID string) string {
	return agentID + "\x00" + verdict + "\x00" + sessionID
}

// Grant records a session-scoped exemption. Empty sessionID is a no-op
// (session-less writes cannot hold a session grant).
func (g *MemoryFactSessionGrants) Grant(agentID, verdict, sessionID string) {
	if g == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	g.mu.Lock()
	g.ks[memoryFactSessionGrantKey(agentID, verdict, sessionID)] = struct{}{}
	g.mu.Unlock()
}

// Has reports whether a session-scoped exemption exists.
func (g *MemoryFactSessionGrants) Has(agentID, verdict, sessionID string) bool {
	if g == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	g.mu.Lock()
	_, ok := g.ks[memoryFactSessionGrantKey(agentID, verdict, sessionID)]
	g.mu.Unlock()
	return ok
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

// ---------------------------------------------------------------------------
// Phase 3.3 — 审批中心接入（C7 桥 twinmonitor ai_approvals）
// ---------------------------------------------------------------------------

// memoryFactPendingPayload is the payload_json snapshot schema: the full
// withheld decision (candidate + target) so approval can replay the original
// bi-temporal write without metadata loss.
type memoryFactPendingPayload struct {
	Candidate    FactWriteCandidate `json:"candidate"`
	TargetFactID string             `json:"target_fact_id,omitempty"`
}

// MarshalFactWriteDecisionSnapshot serializes the withheld decision for
// MemoryFactPendingRecord.PayloadJSON. Empty on marshal failure (best-effort —
// the top-level columns still carry statement/reason for display).
func MarshalFactWriteDecisionSnapshot(d FactWriteDecision) string {
	b, err := json.Marshal(memoryFactPendingPayload{Candidate: d.Candidate, TargetFactID: d.TargetFactID})
	if err != nil {
		return ""
	}
	return string(b)
}

// unmarshalFactWriteDecisionSnapshot restores the withheld decision; ok=false
// when the snapshot is absent/corrupt (legacy rows predating DDL 20261255).
func unmarshalFactWriteDecisionSnapshot(payloadJSON string) (FactWriteDecision, bool) {
	var p memoryFactPendingPayload
	if strings.TrimSpace(payloadJSON) == "" || json.Unmarshal([]byte(payloadJSON), &p) != nil {
		return FactWriteDecision{}, false
	}
	if strings.TrimSpace(p.Candidate.Statement) == "" {
		return FactWriteDecision{}, false
	}
	return FactWriteDecision{Candidate: p.Candidate, TargetFactID: p.TargetFactID}, true
}

// MemoryFactPendingNotifier pushes one withheld write to the approval center
// (twinmonitor ai_approvals via the signed webhook bridge). Implementations
// must be non-blocking and best-effort: notification failure never rolls back
// the pending insert (audit.py backlog check covers the gap).
// Stability:evolving
type MemoryFactPendingNotifier interface {
	NotifyFactWritePending(ctx context.Context, rec MemoryFactPendingRecord)
}

// MemoryFactPendingDecider executes approval-center decisions on withheld
// writes. Approve-tier decisions replay the original bi-temporal write from
// the snapshot; deny only marks the row (target fact untouched). E4 four-tier
// semantics: approve_session additionally grants a process-local session
// exemption; approve_always persists an (agent_id, verdict) allow rule.
type MemoryFactPendingDecider struct {
	pending       MemoryFactPendingStore
	writer        L3FactWriter
	actionLog     MemoryActionLogWriter
	allowRules    MemoryFactAllowRuleStore
	sessionGrants *MemoryFactSessionGrants
	lg            loggateway.Logger
}

// NewMemoryFactPendingDecider builds the decider; nil when the pending store
// is absent (writer optional — approve then only marks the row). allowRules /
// sessionGrants are optional; without them approve_always / approve_session
// still replay the current write but cannot exempt future ones (warn-logged).
func NewMemoryFactPendingDecider(pending MemoryFactPendingStore, writer L3FactWriter, actionLog MemoryActionLogWriter, allowRules MemoryFactAllowRuleStore, sessionGrants *MemoryFactSessionGrants, lg loggateway.Logger) *MemoryFactPendingDecider {
	if pending == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MemoryFactPendingDecider{
		pending: pending, writer: writer, actionLog: actionLog,
		allowRules: allowRules, sessionGrants: sessionGrants,
		lg: lg.With(loggateway.Domain("memory_fact_pending_decider")),
	}
}

// Decide applies one approval-center decision (four-tier; see
// NormalizeMemoryFactDecision). Fail-closed: MarkDecided's pending-only guard
// runs first, so double decisions / unknown ids return applied=false and
// never replay twice. When approved (any approve tier), the original write is
// replayed from the snapshot after the state transition; a replay failure is
// reported as replayErr while the decision stands (applied=true) — the row is
// already decided and must not be re-decided by a retry.
func (d *MemoryFactPendingDecider) Decide(ctx context.Context, id, decision, approver, comment string) (applied bool, replayErr error) {
	if d == nil {
		return false, nil
	}
	tier, ok := NormalizeMemoryFactDecision(decision)
	if !ok {
		return false, nil
	}
	approved := tier != MemoryFactDecisionDeny
	status := MemoryFactPendingStatusRejected
	if approved {
		status = MemoryFactPendingStatusApproved
	}
	applied, err := d.pending.MarkDecided(ctx, id, status, approver, time.Now().Unix())
	if err != nil || !applied {
		return false, err
	}
	d.audit(ctx, "fact_write_pending."+status, id, approver, "decision="+tier+" "+comment)
	if !approved {
		return true, nil
	}
	rec, found, err := d.pending.GetPending(ctx, id)
	if err != nil || !found {
		return true, err
	}
	d.grantExemption(ctx, rec, tier, approver)
	replayErr = d.replay(ctx, rec)
	if replayErr == nil {
		d.audit(ctx, "fact_write_pending.replayed", id, approver,
			"verdict="+rec.Verdict+" target="+rec.FactKey)
	}
	return true, replayErr
}

// grantExemption applies the E4 exemption side effects of the two durable
// approve tiers. Runs before replay so a later replay failure never loses the
// operator's explicit grant intent.
func (d *MemoryFactPendingDecider) grantExemption(ctx context.Context, rec MemoryFactPendingRecord, tier, approver string) {
	switch tier {
	case MemoryFactDecisionApproveAlways:
		if d.allowRules == nil {
			d.lg.Warn("memory fact pending: approve_always without rule store, exemption not persisted",
				loggateway.Str("pending_id", rec.ID))
			return
		}
		if err := d.allowRules.GrantAllowRule(ctx, rec.AgentID, rec.Verdict, approver); err != nil {
			d.lg.Warn("memory fact pending: allow rule grant failed",
				loggateway.Str("pending_id", rec.ID), loggateway.Err(err))
			return
		}
		d.audit(ctx, "fact_write_allow_rule.granted", rec.ID, approver,
			"agent="+rec.AgentID+" verdict="+rec.Verdict)
	case MemoryFactDecisionApproveSession:
		sessionID := memoryFactPendingSessionID(rec)
		if sessionID == "" {
			d.lg.Warn("memory fact pending: approve_session without source session, exemption skipped",
				loggateway.Str("pending_id", rec.ID))
			return
		}
		d.sessionGrants.Grant(rec.AgentID, rec.Verdict, sessionID)
		d.audit(ctx, "fact_write_session_grant.granted", rec.ID, approver,
			"agent="+rec.AgentID+" verdict="+rec.Verdict)
	}
}

// memoryFactPendingSessionID extracts the source session from the snapshot
// candidate ("" for legacy rows / session-less writes).
func memoryFactPendingSessionID(rec MemoryFactPendingRecord) string {
	if dec, ok := unmarshalFactWriteDecisionSnapshot(rec.PayloadJSON); ok {
		return strings.TrimSpace(dec.Candidate.SourceSessionID)
	}
	return ""
}

// MemoryFactWriteBypassed reports whether a gated verdict may skip the pending
// gate under E4 exemptions: a persisted approve_always rule (agent_id+verdict)
// or a process-local approve_session grant (agent_id+verdict+session). Store
// errors fail closed (pend as usual — never silently bypass on doubt).
func MemoryFactWriteBypassed(ctx context.Context, rules MemoryFactAllowRuleStore, grants *MemoryFactSessionGrants, d FactWriteDecision, verdict string) bool {
	if grants != nil && grants.Has(d.Candidate.AgentID, verdict, strings.TrimSpace(d.Candidate.SourceSessionID)) {
		return true
	}
	if rules == nil {
		return false
	}
	ok, err := rules.HasAllowRule(ctx, d.Candidate.AgentID, verdict)
	return err == nil && ok
}

// replay re-executes the withheld bi-temporal write from the snapshot:
// UPDATE → invalidate+upsert; DELETE → invalidate; CONTESTED → plain add.
func (d *MemoryFactPendingDecider) replay(ctx context.Context, rec MemoryFactPendingRecord) error {
	if d.writer == nil {
		d.lg.Warn("memory fact pending replay skipped: writer missing",
			loggateway.Str("pending_id", rec.ID), loggateway.Str("verdict", rec.Verdict))
		return nil
	}
	dec, ok := unmarshalFactWriteDecisionSnapshot(rec.PayloadJSON)
	if !ok {
		// Legacy row without snapshot: replay from top-level columns (metadata
		// degraded — statement/scope minimum). Never fail the decision.
		d.lg.Warn("memory fact pending replay: snapshot absent, minimal replay",
			loggateway.Str("pending_id", rec.ID), loggateway.Str("verdict", rec.Verdict))
		dec = FactWriteDecision{
			Candidate:    FactWriteCandidate{Statement: rec.ProposedBody, AgentID: rec.AgentID},
			TargetFactID: rec.FactKey,
		}
	}
	var err error
	switch rec.Verdict {
	case MemoryFactPendingVerdictUpdate:
		_, err = d.writer.InvalidateAndUpsertFactTx(ctx, dec.TargetFactID, buildFactUpsertFromCandidate(dec.Candidate))
	case MemoryFactPendingVerdictDelete:
		_, err = d.writer.InvalidateFact(ctx, dec.TargetFactID)
		if err == nil {
			// R3 3.4：DELETE 执行一律软归档（bi-temporal invalidate，行保留可
			// 恢复），归档动作本身针对目标 fact 留 audit（恢复追溯链）。
			d.auditFactArchived(ctx, dec.TargetFactID, rec.ID)
		}
	case MemoryFactPendingVerdictContested:
		_, err = d.writer.UpsertFactRow(ctx, buildFactUpsertFromCandidate(dec.Candidate))
	}
	if err != nil {
		d.lg.Warn("memory fact pending replay failed",
			loggateway.Str("pending_id", rec.ID), loggateway.Str("verdict", rec.Verdict),
			loggateway.Str("target", dec.TargetFactID), loggateway.Err(err))
		return err
	}
	return nil
}

// audit writes one decision/grant/replay record to the memory action log
// (best-effort).
func (d *MemoryFactPendingDecider) audit(ctx context.Context, action, id, approver, reason string) {
	if d.actionLog == nil {
		return
	}
	rec := MemoryPolicyRecord{
		Action:        action,
		TargetKind:    "memory_fact_pending",
		TargetID:      id,
		Reason:        strings.TrimSpace("approver=" + approver + " " + reason),
		PolicyVersion: "fact_write_pipeline_v1",
	}
	if err := d.actionLog.WriteMemoryActionLog(ctx, rec); err != nil {
		d.lg.Debug("memory fact pending decision audit failed", loggateway.Err(err))
	}
}

// auditFactArchived records the soft-archive of the target fact after an
// approved DELETE replay (R3 3.4 软归档 audit). Targets the fact row itself
// (TargetKind=memory_fact) so restore tooling can trace who archived it via
// which pending id.
func (d *MemoryFactPendingDecider) auditFactArchived(ctx context.Context, factID, pendingID string) {
	if d.actionLog == nil || strings.TrimSpace(factID) == "" {
		return
	}
	rec := MemoryPolicyRecord{
		Action:        "fact_write_pending.delete_archived",
		TargetKind:    "memory_fact",
		TargetID:      factID,
		Reason:        "pending=" + pendingID,
		PolicyVersion: "fact_write_pipeline_v1",
	}
	if err := d.actionLog.WriteMemoryActionLog(ctx, rec); err != nil {
		d.lg.Debug("memory fact pending delete-archive audit failed", loggateway.Err(err))
	}
}
