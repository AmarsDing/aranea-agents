package sessionmemory

import (
	"context"
	"errors"
	"fmt"
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

// DeleteFactRowsByIDs soft-deletes multiple memory facts by their IDs and returns the number deleted.
func (st *Store) DeleteFactRowsByIDs(ctx context.Context, ids []string) (int, error) {
	if st == nil || st.client == nil {
		return 0, errors.New("session memory store not wired")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, now, now) // deleted_at, updated_at
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, strings.TrimSpace(id))
	}
	q := fmt.Sprintf(
		`UPDATE memory_facts SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id IN (%s) AND deleted_at = ''`,
		strings.Join(placeholders, ","),
	)
	res, err := st.client.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// IncrementConflictCount increments the conflict_count for a fact and returns the new count.
func (st *Store) IncrementConflictCount(ctx context.Context, factID string) (int32, error) {
	if st == nil || st.client == nil {
		return 0, fmt.Errorf("session memory store not wired")
	}
	var newCount int32
	err := queryOne(ctx, st.client,
		`UPDATE memory_facts SET conflict_count = conflict_count + 1, updated_at = ? WHERE id = ? RETURNING conflict_count`,
		[]any{time.Now().UTC().Format(time.RFC3339Nano), factID},
		&newCount,
	)
	return newCount, err
}

// ListConflictingFacts returns facts with conflict_count > 0 for a given scope.
func (st *Store) ListConflictingFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	if st == nil || st.client == nil {
		return nil, 0, fmt.Errorf("session memory store not wired")
	}
	lim := int(limit)
	if lim <= 0 || lim > 100 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	var total int32
	countQ := `SELECT COUNT(*) FROM memory_facts WHERE conflict_count > 0 AND status != 'deleted'`
	listQ := sqlFactSelect + ` WHERE conflict_count > 0 AND status != 'deleted'`
	args := []any{}
	if scopeType != "" {
		countQ += ` AND scope_type = ?`
		listQ += ` AND scope_type = ?`
		args = append(args, scopeType)
	}
	if scopeID != "" {
		countQ += ` AND scope_id = ?`
		listQ += ` AND scope_id = ?`
		args = append(args, scopeID)
	}
	if err := queryOne(ctx, st.client, countQ, args, &total); err != nil {
		return nil, 0, err
	}
	listQ += ` ORDER BY conflict_count DESC, updated_at DESC LIMIT ? OFFSET ?`
	listArgs := append(args, lim, off)
	rows, err := st.client.QueryContext(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
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

// ListPIIFlaggedFacts returns facts with pii_flag=true.
func (st *Store) ListPIIFlaggedFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	if st == nil || st.client == nil {
		return nil, 0, errors.New("session memory store not wired")
	}
	lim := int(limit)
	if lim <= 0 || lim > 100 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	var total int32
	countQ := `SELECT COUNT(*) FROM memory_facts WHERE pii_flag = 1 AND status != 'deleted'`
	listQ := sqlFactSelect + ` WHERE pii_flag = 1 AND status != 'deleted'`
	args := []any{}
	if scopeType != "" {
		countQ += ` AND scope_type = ?`
		listQ += ` AND scope_type = ?`
		args = append(args, scopeType)
	}
	if scopeID != "" {
		countQ += ` AND scope_id = ?`
		listQ += ` AND scope_id = ?`
		args = append(args, scopeID)
	}
	if err := queryOne(ctx, st.client, countQ, args, &total); err != nil {
		return nil, 0, err
	}
	listQ += ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	listArgs := append(args, lim, off)
	rows, err := st.client.QueryContext(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

// ApprovePIIFact clears the pii_flag on a fact.
func (st *Store) ApprovePIIFact(ctx context.Context, factID string) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	_, err := st.client.ExecContext(ctx,
		`UPDATE memory_facts SET pii_flag = 0, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), factID)
	return err
}

// RejectPIIFact deletes a PII-flagged fact (soft delete by setting status='deleted').
func (st *Store) RejectPIIFact(ctx context.Context, factID string) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	_, err := st.client.ExecContext(ctx,
		`UPDATE memory_facts SET status = 'deleted', updated_at = ? WHERE id = ? AND pii_flag = 1`,
		time.Now().UTC().Format(time.RFC3339Nano), factID)
	return err
}
