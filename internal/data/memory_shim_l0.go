package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

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

func (r *l0SnapshotRepo) ListL0SnapshotRows(ctx context.Context, sessionID, agentID string, limit int32) ([][]byte, error) {
	if sessionID == "" {
		return nil, apierror.BadRequest("MEMORY", "session id is required")
	}
	lim := int(limit)
	if lim <= 0 || lim > 100 {
		lim = 20
	}
	q := sqlL0Select + ` WHERE session_id = ?`
	args := []any{sessionID}
	if agentID != "" {
		q += ` AND agent_id = ?`
		args = append(args, agentID)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, lim)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L0")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanL0SnapshotRow(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L0")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L0")
}

func (r *l0SnapshotRepo) GetL0SnapshotRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, apierror.BadRequest("MEMORY", "session id is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apierror.BadRequest("MEMORY", "snapshot id is required")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(sqlL0Select+` WHERE id = ? AND session_id = ?`), id, sessionID)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L0")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, apierror.NotFound("MEMORY", "l0 snapshot not found")
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
		return apierror.BadRequest("MEMORY", "session id is required")
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
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(sqlL0Insert),
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
	return entErrToBizErr(err, "MEMORY_L0")
}

func (r *l0SnapshotRepo) UpdateL0SnapshotActual(ctx context.Context, id, sessionID string, actualPromptTokens, contextWindowTokens int) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return apierror.BadRequest("MEMORY", "snapshot id is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return apierror.BadRequest("MEMORY", "session id is required")
	}
	if actualPromptTokens < 0 {
		actualPromptTokens = 0
	}
	var usedRatio float64
	if contextWindowTokens > 0 {
		usedRatio = float64(actualPromptTokens) / float64(contextWindowTokens)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_l0_assembly_snapshots
		 SET prompt_token_actual = ?, used_ratio = ?, warning_codes_json = ?
		 WHERE id = ? AND session_id = ?`),
		actualPromptTokens, usedRatio, biz.L0WarningCodesJSON(biz.L0WarningCodesFromRatio(usedRatio)), id, sessionID,
	)
	return entErrToBizErr(err, "MEMORY_L0")
}

// newUUIDString generates a new UUID string.
func newUUIDString() string {
	return uuid.NewString()
}
