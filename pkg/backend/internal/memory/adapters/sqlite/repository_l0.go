package sqlite

import (
	mem "arenea/backend/internal/memory/domain"

	"database/sql"
	"errors"
)

// L0Repository 承载 L0 快照与 session_summaries 的 SQLite 实现。
type L0Repository struct {
	db *sql.DB
}

// NewL0Repository 在共享 DB 句柄上构建（与 repository.SQLiteRepository 同库）。
func NewL0Repository(db *sql.DB) *L0Repository {
	return &L0Repository{db: db}
}

// InsertL0AssemblySnapshot 持久化一次组装运行。调用方（MemoryL0Service）自行生成 ID，
// 以便在行写入 SQLite 之前即可从模型调用 span 引用该快照。
func (r *L0Repository) InsertL0AssemblySnapshot(snap mem.L0AssemblySnapshot) error {
	if snap.ID == "" || snap.SessionID == "" {
		return errors.New("snapshot id and session_id are required")
	}
	if snap.CreatedAt == "" {
		snap.CreatedAt = nowISO()
	}
	if snap.SegmentsJSON == "" {
		snap.SegmentsJSON = "[]"
	}
	if snap.WarningCodesJSON == "" {
		snap.WarningCodesJSON = "[]"
	}
	if snap.MetadataJSON == "" {
		snap.MetadataJSON = "{}"
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_l0_assembly_snapshots(
		   id, session_id, run_id, turn_id, span_id, agent_id, team_id,
		   provider, model, context_window_tokens, budget_tokens,
		   recent_window_turns, recent_window_tokens, summary_token_estimate,
		   l1_field_count, l1_token_estimate,
		   l3_chunk_count, l3_token_estimate,
		   l4_path_count, l4_token_estimate,
		   prompt_token_estimate, prompt_token_actual, used_ratio,
		   truncate_strategy, truncated_message_count,
		   summarized_turn_from, summarized_turn_to,
		   segments_json, warning_codes_json, metadata_json, created_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.SessionID, snap.RunID, snap.TurnID, snap.SpanID, snap.AgentID, snap.TeamID,
		snap.Provider, snap.Model, snap.ContextWindowTokens, snap.BudgetTokens,
		snap.RecentWindowTurns, snap.RecentWindowTokens, snap.SummaryTokenEstimate,
		snap.L1FieldCount, snap.L1TokenEstimate,
		snap.L3ChunkCount, snap.L3TokenEstimate,
		snap.L4PathCount, snap.L4TokenEstimate,
		snap.PromptTokenEstimate, snap.PromptTokenActual, snap.UsedRatio,
		snap.TruncateStrategy, snap.TruncatedMessageCount,
		snap.SummarizedTurnFrom, snap.SummarizedTurnTo,
		snap.SegmentsJSON, snap.WarningCodesJSON, snap.MetadataJSON, snap.CreatedAt,
	)
	return err
}

// UpdateL0AssemblySnapshotActualTokens 在模型调用返回用量后调用。
// 保存真实 prompt token 数便于分析管道衡量估计器漂移并驱动智能体演进闭环。
func (r *L0Repository) UpdateL0AssemblySnapshotActualTokens(snapshotID string, actualPromptTokens int, usedRatio float64) error {
	if snapshotID == "" {
		return errors.New("snapshot id is required")
	}
	if usedRatio < 0 {
		usedRatio = 0
	}
	if usedRatio > 1 {
		usedRatio = 1
	}
	_, err := r.db.Exec(
		`UPDATE memory_l0_assembly_snapshots SET prompt_token_actual = ?, used_ratio = ? WHERE id = ?`,
		actualPromptTokens, usedRatio, snapshotID,
	)
	return err
}

func (r *L0Repository) GetL0AssemblySnapshotByID(id string) (mem.L0AssemblySnapshot, error) {
	row := r.db.QueryRow(memoryL0SelectSQL()+` WHERE id = ?`, id)
	return scanL0Snapshot(row)
}

func (r *L0Repository) ListL0AssemblySnapshotsBySession(sessionID string, limit int) ([]mem.L0AssemblySnapshot, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.Query(memoryL0SelectSQL()+` WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanL0Snapshots(rows)
}

func (r *L0Repository) ListL0AssemblySnapshotsBySpan(spanID string) ([]mem.L0AssemblySnapshot, error) {
	if spanID == "" {
		return nil, errors.New("span id is required")
	}
	rows, err := r.db.Query(memoryL0SelectSQL()+` WHERE span_id = ? ORDER BY created_at DESC`, spanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanL0Snapshots(rows)
}

// ListSessionSummaries 返回 `session_summaries` 中已存的浓缩片段，按新到旧排序。
// L0 服务通常只需最近一小段窗口（常见为最后 8 条）用于填充提示头。
func (r *L0Repository) ListSessionSummaries(sessionID string, limit int) ([]mem.SessionSummary, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 8
	}
	rows, err := r.db.Query(
		`SELECT id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at
		 FROM session_summaries WHERE session_id = ? ORDER BY to_turn DESC, created_at DESC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]mem.SessionSummary, 0, limit)
	for rows.Next() {
		var v mem.SessionSummary
		if err = rows.Scan(&v.ID, &v.SessionID, &v.SummaryMarkdown, &v.FromTurn, &v.ToTurn, &v.TokenEstimate, &v.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// AddSessionSummary 插入由 SummaryService 生成的摘要行。
// 调用方填写 `ID` / `CreatedAt`，以免本文件引入额外依赖。
func (r *L0Repository) AddSessionSummary(summary mem.SessionSummary) (mem.SessionSummary, error) {
	if summary.ID == "" || summary.SessionID == "" {
		return mem.SessionSummary{}, errors.New("summary id and session_id are required")
	}
	if summary.CreatedAt == "" {
		summary.CreatedAt = nowISO()
	}
	_, err := r.db.Exec(
		`INSERT INTO session_summaries(id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		summary.ID, summary.SessionID, summary.SummaryMarkdown, summary.FromTurn, summary.ToTurn, summary.TokenEstimate, summary.CreatedAt,
	)
	if err != nil {
		return mem.SessionSummary{}, err
	}
	return summary, nil
}

func memoryL0SelectSQL() string {
	return `SELECT id, session_id, run_id, turn_id, span_id, agent_id, team_id, provider, model,
	 context_window_tokens, budget_tokens, recent_window_turns, recent_window_tokens, summary_token_estimate,
	 l1_field_count, l1_token_estimate, l3_chunk_count, l3_token_estimate, l4_path_count, l4_token_estimate,
	 prompt_token_estimate, prompt_token_actual, used_ratio, truncate_strategy, truncated_message_count,
	 summarized_turn_from, summarized_turn_to, segments_json, warning_codes_json, metadata_json, created_at
	 FROM memory_l0_assembly_snapshots`
}

func scanL0Snapshot(row scanner) (mem.L0AssemblySnapshot, error) {
	var v mem.L0AssemblySnapshot
	err := row.Scan(
		&v.ID, &v.SessionID, &v.RunID, &v.TurnID, &v.SpanID, &v.AgentID, &v.TeamID, &v.Provider, &v.Model,
		&v.ContextWindowTokens, &v.BudgetTokens, &v.RecentWindowTurns, &v.RecentWindowTokens, &v.SummaryTokenEstimate,
		&v.L1FieldCount, &v.L1TokenEstimate, &v.L3ChunkCount, &v.L3TokenEstimate, &v.L4PathCount, &v.L4TokenEstimate,
		&v.PromptTokenEstimate, &v.PromptTokenActual, &v.UsedRatio, &v.TruncateStrategy, &v.TruncatedMessageCount,
		&v.SummarizedTurnFrom, &v.SummarizedTurnTo, &v.SegmentsJSON, &v.WarningCodesJSON, &v.MetadataJSON, &v.CreatedAt,
	)
	return v, err
}

func scanL0Snapshots(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]mem.L0AssemblySnapshot, error) {
	result := make([]mem.L0AssemblySnapshot, 0, 16)
	for rows.Next() {
		v, err := scanL0Snapshot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
