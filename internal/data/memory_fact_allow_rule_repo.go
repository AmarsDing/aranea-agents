package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// memoryFactAllowRuleRepo is the raw-SQL dual-dialect implementation of
// biz.MemoryFactAllowRuleStore (79-runtime-governance R3 Phase 3.4, E4).
// Table is created by DDL migration 20261256; grant is idempotent on
// (agent_id, verdict) via the unique index + insert-or-ignore.
type memoryFactAllowRuleRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.MemoryFactAllowRuleStore = (*memoryFactAllowRuleRepo)(nil)

// NewMemoryFactAllowRuleRepo constructs the allow-rule store; nil when DB absent.
func NewMemoryFactAllowRuleRepo(data *Data, lg loggateway.Logger) biz.MemoryFactAllowRuleStore {
	if data == nil || data.RWDB() == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &memoryFactAllowRuleRepo{data: data, lg: lg.With(loggateway.Domain("memory_fact_allow_rule_repo"))}
}

const memoryFactAllowRuleCols = "id, agent_id, verdict, created_by, created_at"

// GrantAllowRule persists a rule (idempotent on agent_id+verdict).
func (r *memoryFactAllowRuleRepo) GrantAllowRule(ctx context.Context, agentID, verdict, createdBy string) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(agentID) == "" {
		return nil
	}
	d := r.data.Dialect()
	// Idempotent on the (agent_id, verdict) unique index: the conflict-target
	// form keeps a re-grant a no-op without erroring.
	q := d.BuildInsertOrIgnore("memory_fact_allow_rules", memoryFactAllowRuleCols,
		d.Placeholders(5), "agent_id, verdict")
	_, err := r.data.RWDB().WriteHandle().ExecContext(ctx, q,
		newUUIDString(), agentID, verdict, createdBy, time.Now().Unix(),
	)
	if err != nil {
		return entErrToBizErr(err, "MEMORY_FACT_ALLOW_RULE")
	}
	return nil
}

// HasAllowRule reports whether a rule exists for agent_id+verdict.
func (r *memoryFactAllowRuleRepo) HasAllowRule(ctx context.Context, agentID, verdict string) (bool, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(agentID) == "" {
		return false, nil
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"SELECT id FROM memory_fact_allow_rules WHERE agent_id = ? AND verdict = ? LIMIT 1")
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, agentID, verdict)
	if err != nil {
		return false, entErrToBizErr(err, "MEMORY_FACT_ALLOW_RULE")
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

// RevokeAllowRule deletes the rule; applied=false when absent.
func (r *memoryFactAllowRuleRepo) RevokeAllowRule(ctx context.Context, agentID, verdict string) (bool, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil || strings.TrimSpace(agentID) == "" {
		return false, nil
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"DELETE FROM memory_fact_allow_rules WHERE agent_id = ? AND verdict = ?")
	res, err := r.data.RWDB().WriteHandle().ExecContext(ctx, q, agentID, verdict)
	if err != nil {
		return false, entErrToBizErr(err, "MEMORY_FACT_ALLOW_RULE")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, entErrToBizErr(err, "MEMORY_FACT_ALLOW_RULE")
	}
	return n > 0, nil
}

// ListAllowRules lists rules newest first; empty agentID matches all.
func (r *memoryFactAllowRuleRepo) ListAllowRules(ctx context.Context, agentID string, limit int) ([]biz.MemoryFactAllowRule, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	where := ""
	var args []any
	if s := strings.TrimSpace(agentID); s != "" {
		where = " WHERE agent_id = ?"
		args = append(args, s)
	}
	if limit <= 0 {
		limit = 100
	}
	q := r.data.Dialect().RenumberPlaceholders(
		"SELECT " + memoryFactAllowRuleCols + " FROM memory_fact_allow_rules" + where +
			" ORDER BY created_at DESC, id DESC LIMIT ?")
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_FACT_ALLOW_RULE")
	}
	defer rows.Close()
	out := make([]biz.MemoryFactAllowRule, 0, limit)
	for rows.Next() {
		var rec biz.MemoryFactAllowRule
		if err := rows.Scan(&rec.ID, &rec.AgentID, &rec.Verdict, &rec.CreatedBy, &rec.CreatedAt); err != nil {
			return nil, entErrToBizErr(err, "MEMORY_FACT_ALLOW_RULE")
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_FACT_ALLOW_RULE")
	}
	return out, nil
}
