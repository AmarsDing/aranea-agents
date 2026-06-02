package sessionmemory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// GetL0SnapshotRow returns one L0 assembly snapshot row as JSON.
func (st *Store) GetL0SnapshotRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("snapshot id is required")
	}
	rows, err := st.client.QueryContext(ctx, sqlL0Select+` WHERE id = ? AND session_id = ?`, id, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, fmt.Errorf("l0 snapshot %s not found in session %s", id, sessionID)
	}
	var (
		sid, sessID, runID, turnID, spanID, agentID, teamID, provider, model string
		cwt, bt, rwt, rwtok, ste, l1fc, l1te, l3c, l3te, l4p, l4te          int
		pte, pta                                                            int
		ur                                                                  float64
		ts                                                                  string
		tmc, stf, ste2                                                      int
		segs, warns, meta, cat                                              string
	)
	if err := rows.Scan(
		&sid, &sessID, &runID, &turnID, &spanID, &agentID, &teamID, &provider, &model,
		&cwt, &bt, &rwt, &rwtok, &ste, &l1fc, &l1te, &l3c, &l3te, &l4p, &l4te,
		&pte, &pta, &ur, &ts, &tmc, &stf, &ste2,
		&segs, &warns, &meta, &cat,
	); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": sid, "session_id": sessID, "run_id": runID, "turn_id": turnID, "span_id": spanID,
		"agent_id": agentID, "team_id": teamID, "provider": provider, "model": model,
		"context_window_tokens": cwt, "budget_tokens": bt,
		"recent_window_turns": rwt, "recent_window_tokens": rwtok,
		"summary_token_estimate": ste,
		"l1_field_count":         l1fc, "l1_token_estimate": l1te,
		"l3_chunk_count": l3c, "l3_token_estimate": l3te,
		"l4_path_count": l4p, "l4_token_estimate": l4te,
		"prompt_token_estimate": pte, "prompt_token_actual": pta,
		"used_ratio": ur, "truncate_strategy": ts,
		"truncated_message_count": tmc, "summarized_turn_from": stf, "summarized_turn_to": ste2,
		"segments_json": segs, "warning_codes_json": warns, "metadata_json": meta,
		"created_at": cat,
	}
	return json.Marshal(m)
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
