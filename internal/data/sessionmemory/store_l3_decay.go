package sessionmemory

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

// ApplyAllFactImportanceDecay batch-decays fact importance for stale L3 rows.
func (st *Store) ApplyAllFactImportanceDecay(ctx context.Context, cutoffRFC3339 string, factor float64) (int, error) {
	if st == nil || st.client == nil {
		return 0, nil
	}
	cutoffRFC3339 = strings.TrimSpace(cutoffRFC3339)
	if cutoffRFC3339 == "" || factor <= 0 || factor >= 1 {
		return 0, nil
	}
	res, err := st.client.ExecContext(ctx, `
UPDATE memory_facts SET
 importance = MAX(0.05, importance * ?),
 decay_factor = decay_factor * ?,
 updated_at = ?
WHERE deleted_at = '' AND status = 'active' AND updated_at != '' AND updated_at < ?`,
		factor, factor, time.Now().UTC().Format(time.RFC3339Nano), cutoffRFC3339)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		st.recordPolicyBestEffort(ctx, MemoryActionLogInsert{
			Action:        "L3_DECAY",
			TargetKind:    "fact_scope",
			TargetID:      "global",
			Reason:        cutoffRFC3339,
			PolicyVersion: biz.PolicyVersionL3DecayV1,
		})
	}
	return int(n), nil
}
