package sessionmemory

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ListFactRowsForUser lists active facts scoped to agent/user (trpc memory adapter read path).
func (st *Store) ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	w := []string{"deleted_at = ''", "status = 'active'"}
	args := []any{}
	if stype := strings.TrimSpace(scopeType); stype != "" {
		w = append(w, "scope_type = ?")
		args = append(args, stype)
	}
	if sid := strings.TrimSpace(scopeID); sid != "" {
		w = append(w, "scope_id = ?")
		args = append(args, sid)
	}
	if uid := strings.TrimSpace(userID); uid != "" {
		w = append(w, "(user_id = ? OR user_id = '')")
		args = append(args, uid)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		w = append(w, "(LOWER(statement) LIKE ? OR LOWER(details_markdown) LIKE ?)")
		like := "%" + strings.ToLower(kw) + "%"
		args = append(args, like, like)
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 50
	}
	if lim > 200 {
		lim = 200
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	whereSQL := strings.Join(w, " AND ")
	q := sqlFactSelect + ` WHERE ` + whereSQL + ` ORDER BY importance DESC, updated_at DESC LIMIT ? OFFSET ?`
	listArgs := append(append([]any{}, args...), lim, off)
	rows, err := st.client.QueryContext(ctx, q, listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteFactByID soft-deletes a memory fact by primary key.
func (st *Store) DeleteFactByID(ctx context.Context, id string) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("fact id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := st.client.ExecContext(ctx,
		`UPDATE memory_facts SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = ''`,
		now, now, id)
	if err != nil {
		return err
	}
	if err := st.recordPolicyBestEffort(ctx, MemoryActionLogInsert{
		Action:        "DELETE",
		TargetKind:    "fact",
		TargetID:      id,
		PolicyVersion: "consolidate_v1",
	}); err != nil {
		return err
	}
	return nil
}

// ClearFacts soft-deletes facts for a scope, optionally filtered by user_id.
func (st *Store) ClearFacts(ctx context.Context, scopeType, scopeID, userID string) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return errors.New("scope_type and scope_id are required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	args := []any{now, now, scopeType, scopeID}
	q := `UPDATE memory_facts SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE scope_type = ? AND scope_id = ? AND deleted_at = ''`
	if uid := strings.TrimSpace(userID); uid != "" {
		q += ` AND user_id = ?`
		args = append(args, uid)
	}
	_, err := st.client.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if err := st.recordPolicyBestEffort(ctx, MemoryActionLogInsert{
		Action:        "CLEAR",
		TargetKind:    "fact_scope",
		TargetID:      scopeType + ":" + scopeID,
		Reason:        strings.TrimSpace(userID),
		PolicyVersion: "consolidate_v1",
	}); err != nil {
		return err
	}
	return nil
}
