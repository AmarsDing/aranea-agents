package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

// l0SnapshotRepo implements biz.L0AdminStore using direct Raw SQL.
type l0SnapshotRepo struct {
	data *Data
}

func newL0SnapshotRepo(data *Data) *l0SnapshotRepo {
	if data == nil {
		return nil
	}
	return &l0SnapshotRepo{data: data}
}

// Compile-time interface check.
var _ biz.L0AdminStore = (*l0SnapshotRepo)(nil)

func (r *l0SnapshotRepo) ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	lim := int(limit)
	if lim <= 0 || lim > 100 {
		lim = 20
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, sqlL0Select+` WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`, sessionID, lim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanL0SnapshotRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *l0SnapshotRepo) GetL0SnapshotRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("snapshot id is required")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, sqlL0Select+` WHERE id = ? AND session_id = ?`, id, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, errors.New("l0 snapshot not found")
	}
	return scanL0SnapshotRow(rows)
}

func (r *l0SnapshotRepo) InsertL0AssemblySnapshot(ctx context.Context, in biz.L0AssemblySnapshotInsert) error {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = newUUIDString()
	}
	sessID := strings.TrimSpace(in.SessionID)
	if sessID == "" {
		return errors.New("session id is required")
	}
	segs := strings.TrimSpace(in.SegmentsJSON)
	if segs == "" {
		segs = "[]"
	}
	warns := strings.TrimSpace(in.WarningCodesJSON)
	if warns == "" {
		warns = "[]"
	}
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	created := strings.TrimSpace(in.CreatedAt)
	if created == "" {
		created = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, sqlL0Insert,
		id, sessID,
		strings.TrimSpace(in.RunID),
		strings.TrimSpace(in.TurnID),
		strings.TrimSpace(in.SpanID),
		strings.TrimSpace(in.AgentID),
		strings.TrimSpace(in.TeamID),
		strings.TrimSpace(in.Provider),
		strings.TrimSpace(in.Model),
		in.ContextWindowTokens,
		in.BudgetTokens,
		in.RecentWindowTurns,
		in.RecentWindowTokens,
		in.SummaryTokenEstimate,
		in.L1FieldCount,
		in.L1TokenEstimate,
		in.L3ChunkCount,
		in.L3TokenEstimate,
		in.L4PathCount,
		in.L4TokenEstimate,
		in.PromptTokenEstimate,
		in.PromptTokenActual,
		in.UsedRatio,
		strings.TrimSpace(in.TruncateStrategy),
		in.TruncatedMessageCount,
		in.SummarizedTurnFrom,
		in.SummarizedTurnTo,
		segs, warns, meta, created,
	)
	return err
}

func (r *l0SnapshotRepo) UpdateL0SnapshotActual(ctx context.Context, id string, actualPromptTokens, contextWindowTokens int) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("snapshot id is required")
	}
	if actualPromptTokens < 0 {
		actualPromptTokens = 0
	}
	var usedRatio float64
	if contextWindowTokens > 0 {
		usedRatio = float64(actualPromptTokens) / float64(contextWindowTokens)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_l0_assembly_snapshots
		 SET prompt_token_actual = ?, used_ratio = ?, warning_codes_json = ?
		 WHERE id = ?`,
		actualPromptTokens, usedRatio, biz.L0WarningCodesJSON(biz.L0WarningCodesFromRatio(usedRatio)), id,
	)
	return err
}

// newUUIDString generates a new UUID string.
func newUUIDString() string {
	return uuid.NewString()
}
