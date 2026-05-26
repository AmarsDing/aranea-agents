package sessionmemory

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

// PurgeEpisodesOlderThan soft-deletes consolidated episodes ended before cutoffRFC3339 for one agent.
func (st *Store) PurgeEpisodesOlderThan(ctx context.Context, agentID, cutoffRFC3339 string) (int, error) {
	if st == nil || st.client == nil {
		return 0, nil
	}
	agentID = strings.TrimSpace(agentID)
	cutoffRFC3339 = strings.TrimSpace(cutoffRFC3339)
	if agentID == "" || cutoffRFC3339 == "" {
		return 0, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := st.client.ExecContext(ctx, `
UPDATE memory_episodes SET
 deleted_at = ?,
 updated_at = ?
WHERE agent_id = ? AND deleted_at = '' AND ended_at != '' AND ended_at < ?`,
		now, now, agentID, cutoffRFC3339)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		st.recordPolicyBestEffort(ctx, MemoryActionLogInsert{
			Action:        "L2_RETENTION_PURGE",
			TargetKind:    "episode_scope",
			TargetID:      agentID,
			Reason:        cutoffRFC3339,
			PolicyVersion: biz.PolicyVersionL2DecayV1,
		})
	}
	return int(n), nil
}
