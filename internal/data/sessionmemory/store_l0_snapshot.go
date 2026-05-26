package sessionmemory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/internal/biz"
)

const sqlL0Insert = `INSERT INTO memory_l0_assembly_snapshots (
 id, session_id, run_id, turn_id, span_id, agent_id, team_id, provider, model,
 context_window_tokens, budget_tokens, recent_window_turns, recent_window_tokens, summary_token_estimate,
 l1_field_count, l1_token_estimate, l3_chunk_count, l3_token_estimate, l4_path_count, l4_token_estimate,
 prompt_token_estimate, prompt_token_actual, used_ratio, truncate_strategy, truncated_message_count,
 summarized_turn_from, summarized_turn_to, segments_json, warning_codes_json, metadata_json, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

// InsertL0AssemblySnapshot persists one L0 assembly observation row.
func (st *Store) InsertL0AssemblySnapshot(ctx context.Context, in biz.L0AssemblySnapshotInsert) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = uuid.NewString()
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
	_, err := st.client.ExecContext(ctx, sqlL0Insert,
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

// UpdateL0SnapshotActual fills prompt_token_actual after the model responds.
func (st *Store) UpdateL0SnapshotActual(ctx context.Context, id string, actualPromptTokens, contextWindowTokens int) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
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
	_, err := st.client.ExecContext(ctx,
		`UPDATE memory_l0_assembly_snapshots
		 SET prompt_token_actual = ?, used_ratio = ?, warning_codes_json = ?
		 WHERE id = ?`,
		actualPromptTokens, usedRatio, biz.L0WarningCodesJSON(biz.L0WarningCodesFromRatio(usedRatio)), id,
	)
	return err
}
