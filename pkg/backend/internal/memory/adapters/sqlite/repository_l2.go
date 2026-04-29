package sqlite

import (
	mem "arenea/backend/internal/memory/domain"

	"arenea/backend/internal/kernel/errs"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// L2Repository 为 L2 情节记忆的 SQLite 实现。
type L2Repository struct {
	db *sql.DB
}

// NewL2Repository 使用与 monolith 相同的 *sql.DB。
func NewL2Repository(db *sql.DB) *L2Repository {
	return &L2Repository{db: db}
}

// CreateEpisode 插入新的 memory_episodes 行。调用方应已填好标识、计数、类型、重要性等；
// 仓库补充时间戳与 JSON 默认值，使不同服务路径产生的行一致。
func (r *L2Repository) CreateEpisode(e mem.MemoryEpisode) (mem.MemoryEpisode, error) {
	if e.ID == "" {
		return mem.MemoryEpisode{}, errors.New("episode id is required")
	}
	if e.SessionID == "" {
		return mem.MemoryEpisode{}, errors.New("episode session_id is required")
	}
	if e.Kind == "" {
		e.Kind = mem.EpisodeKindTask
	}
	now := nowISO()
	if e.CreatedAt == "" {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
	if e.EmbeddingStatus == "" {
		e.EmbeddingStatus = "pending"
	}
	if e.ConsolidationStatus == "" {
		e.ConsolidationStatus = "pending"
	}
	if e.L1SnapshotJSON == "" {
		e.L1SnapshotJSON = "{}"
	}
	e.KeyDecisionsJSON = normalizeJSONList(e.KeyDecisionsJSON)
	e.KeyArtifactsJSON = normalizeJSONList(e.KeyArtifactsJSON)
	if e.MetadataJSON == "" {
		e.MetadataJSON = "{}"
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_episodes(
			id, session_id, run_id, team_id, agent_id, l1_task_id,
			episode_kind, title, goal, outcome, outcome_summary,
			result_preview, failure_reason,
			importance, confidence, user_feedback, critic_score,
			span_count, message_count, tool_call_count, skill_call_count, mcp_call_count,
			total_tokens, total_cost_micro_usd, duration_ms,
			l1_snapshot_json, key_decisions_json, key_artifacts_json,
			embedding_status, embedding_model, embedding_dim, embedding_norm,
			consolidation_status, consolidated_at, consolidated_l3_count, consolidated_l4_count,
			metadata_json, started_at, ended_at,
			created_at, updated_at, archived_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.SessionID, e.RunID, e.TeamID, e.AgentID, e.L1TaskID,
		string(e.Kind), e.Title, e.Goal, e.Outcome, e.OutcomeSummary,
		e.ResultPreview, e.FailureReason,
		e.Importance, e.Confidence, e.UserFeedback, e.CriticScore,
		e.SpanCount, e.MessageCount, e.ToolCallCount, e.SkillCallCount, e.MCPCallCount,
		e.TotalTokens, e.TotalCostMicroUSD, e.DurationMS,
		e.L1SnapshotJSON, e.KeyDecisionsJSON, e.KeyArtifactsJSON,
		e.EmbeddingStatus, e.EmbeddingModel, e.EmbeddingDim, e.EmbeddingNorm,
		e.ConsolidationStatus, e.ConsolidatedAt, e.ConsolidatedL3Count, e.ConsolidatedL4Count,
		e.MetadataJSON, e.StartedAt, e.EndedAt,
		e.CreatedAt, e.UpdatedAt, e.ArchivedAt, e.DeletedAt,
	)
	if err != nil {
		return mem.MemoryEpisode{}, err
	}
	return r.GetEpisode(e.ID)
}

// UpdateEpisode 更新可变列。嵌入与巩固相关列不在此更新——L3 管道/索引工作协程有独立写入方法。
func (r *L2Repository) UpdateEpisode(e mem.MemoryEpisode) error {
	if e.ID == "" {
		return errors.New("episode id is required")
	}
	if e.MetadataJSON == "" {
		e.MetadataJSON = "{}"
	}
	e.KeyDecisionsJSON = normalizeJSONList(e.KeyDecisionsJSON)
	e.KeyArtifactsJSON = normalizeJSONList(e.KeyArtifactsJSON)
	_, err := r.db.Exec(
		`UPDATE memory_episodes SET
			title = ?, goal = ?, outcome = ?, outcome_summary = ?,
			result_preview = ?, failure_reason = ?,
			importance = ?, confidence = ?, user_feedback = ?, critic_score = ?,
			span_count = ?, message_count = ?, tool_call_count = ?, skill_call_count = ?, mcp_call_count = ?,
			total_tokens = ?, total_cost_micro_usd = ?, duration_ms = ?,
			l1_snapshot_json = ?, key_decisions_json = ?, key_artifacts_json = ?,
			metadata_json = ?, started_at = ?, ended_at = ?,
			updated_at = ?, archived_at = ?, deleted_at = ?
		 WHERE id = ?`,
		e.Title, e.Goal, e.Outcome, e.OutcomeSummary,
		e.ResultPreview, e.FailureReason,
		e.Importance, e.Confidence, e.UserFeedback, e.CriticScore,
		e.SpanCount, e.MessageCount, e.ToolCallCount, e.SkillCallCount, e.MCPCallCount,
		e.TotalTokens, e.TotalCostMicroUSD, e.DurationMS,
		e.L1SnapshotJSON, e.KeyDecisionsJSON, e.KeyArtifactsJSON,
		e.MetadataJSON, e.StartedAt, e.EndedAt,
		nowISO(), e.ArchivedAt, e.DeletedAt,
		e.ID,
	)
	return err
}

// GetEpisode 按 ID 返回单条 episode。软删除行仍会返回，便于 HTTP 层展示审计；Recall/List 由服务层过滤。
func (r *L2Repository) GetEpisode(id string) (mem.MemoryEpisode, error) {
	row := r.db.QueryRow(memoryEpisodeSelectSQL()+` WHERE id = ?`, id)
	return scanMemoryEpisode(row)
}

// ListEpisodes 返回某会话下最近的 episode，按 ended_at 降序（再按 created_at 降序作稳定排序）。
// 排除软删除；包含已归档以便界面展示历史。
func (r *L2Repository) ListEpisodes(sessionID, kind string, limit, offset int) ([]mem.MemoryEpisode, int, error) {
	if sessionID == "" {
		return nil, 0, errors.New("session id is required")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	clauses := []string{"session_id = ?", "deleted_at = ''"}
	args := []any{sessionID}
	if kind != "" {
		clauses = append(clauses, "episode_kind = ?")
		args = append(args, kind)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")

	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM memory_episodes`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(
		memoryEpisodeSelectSQL()+where+` ORDER BY ended_at DESC, created_at DESC LIMIT ? OFFSET ?`,
		append(append([]any{}, args...), limit, offset)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []mem.MemoryEpisode
	for rows.Next() {
		v, scanErr := scanMemoryEpisode(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// ListPendingConsolidation 为 L3/L4 巩固工作进程供数。重要性低于智能体阈值的 episode 会跳过，避免低信号浪费预算。
func (r *L2Repository) ListPendingConsolidation(minImportance float64, limit int) ([]mem.MemoryEpisode, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.db.Query(
		memoryEpisodeSelectSQL()+` WHERE consolidation_status = 'pending' AND deleted_at = '' AND importance >= ?
		 ORDER BY importance DESC, ended_at ASC LIMIT ?`,
		minImportance, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []mem.MemoryEpisode
	for rows.Next() {
		v, scanErr := scanMemoryEpisode(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateEpisodeConsolidationStatus 由巩固工作进程在 L3/L4 吸收 episode 后调用（或判定不可巩固时）。
func (r *L2Repository) UpdateEpisodeConsolidationStatus(id, status string, l3Count, l4Count int) error {
	if id == "" {
		return errors.New("episode id is required")
	}
	now := nowISO()
	_, err := r.db.Exec(
		`UPDATE memory_episodes SET
			consolidation_status = ?,
			consolidated_at = ?,
			consolidated_l3_count = consolidated_l3_count + ?,
			consolidated_l4_count = consolidated_l4_count + ?,
			updated_at = ?
		 WHERE id = ?`,
		status, now, l3Count, l4Count, now, id,
	)
	return err
}

// UpdateEpisodeEmbedding 在索引工作进程完成 BM25 行并算好嵌入后调用。两步均可幂等，重试安全。
func (r *L2Repository) UpdateEpisodeEmbedding(id, status, model string, dim int, norm float64) error {
	if id == "" {
		return errors.New("episode id is required")
	}
	if status == "" {
		status = "ready"
	}
	_, err := r.db.Exec(
		`UPDATE memory_episodes SET embedding_status = ?, embedding_model = ?, embedding_dim = ?, embedding_norm = ?, updated_at = ? WHERE id = ?`,
		status, model, dim, norm, nowISO(), id,
	)
	return err
}

// SoftDeleteEpisode 设置 deleted_at，使行从 List/Recall 中隐藏。同步清除 FTS，避免已删行再入搜索结果。
func (r *L2Repository) SoftDeleteEpisode(id string) error {
	if id == "" {
		return errors.New("episode id is required")
	}
	now := nowISO()
	if _, err := r.db.Exec(
		`UPDATE memory_episodes SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id,
	); err != nil {
		return err
	}
	return r.DeleteL2Index(id)
}

// UpsertL2Index 在单事务中同时写入元数据行与 FTS 行，避免只成功一半。
func (r *L2Repository) UpsertL2Index(entry mem.MemoryL2IndexEntry, text string) error {
	if entry.EpisodeID == "" {
		return errors.New("index episode_id is required")
	}
	if entry.SessionID == "" {
		return errors.New("index session_id is required")
	}
	if entry.TextKind == "" {
		entry.TextKind = "episode"
	}
	if entry.ID == "" {
		entry.ID = entry.EpisodeID + ":" + entry.TextKind
	}
	now := nowISO()
	if entry.CreatedAt == "" {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(
		`INSERT INTO memory_l2_index_meta(
			id, episode_id, session_id, agent_id, text_kind, text_preview,
			token_estimate, embedding_model, embedding_dim, embedding_norm,
			importance, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(episode_id, text_kind) DO UPDATE SET
			text_preview = excluded.text_preview,
			token_estimate = excluded.token_estimate,
			embedding_model = excluded.embedding_model,
			embedding_dim = excluded.embedding_dim,
			embedding_norm = excluded.embedding_norm,
			importance = excluded.importance,
			updated_at = excluded.updated_at`,
		entry.ID, entry.EpisodeID, entry.SessionID, entry.AgentID, entry.TextKind, entry.TextPreview,
		entry.TokenEstimate, entry.EmbeddingModel, entry.EmbeddingDim, entry.EmbeddingNorm,
		entry.Importance, entry.CreatedAt, entry.UpdatedAt,
	); err != nil {
		return err
	}
	// FTS5 无 upsert：先删后插以保持索引一致。
	if _, err = tx.Exec(
		`DELETE FROM memory_l2_index_fts WHERE episode_id = ? AND text_kind = ?`,
		entry.EpisodeID, entry.TextKind,
	); err != nil {
		return err
	}
	if strings.TrimSpace(text) != "" {
		if _, err = tx.Exec(
			`INSERT INTO memory_l2_index_fts(episode_id, session_id, agent_id, text_kind, text)
			 VALUES (?, ?, ?, ?, ?)`,
			entry.EpisodeID, entry.SessionID, entry.AgentID, entry.TextKind, text,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteL2Index 删除某 episode 的元数据与 FTS 行。无行时也可安全调用（DELETE 为 no-op）。
func (r *L2Repository) DeleteL2Index(episodeID string) error {
	if episodeID == "" {
		return errors.New("episode id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM memory_l2_index_meta WHERE episode_id = ?`, episodeID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM memory_l2_index_fts WHERE episode_id = ?`, episodeID); err != nil {
		return err
	}
	return tx.Commit()
}

// SearchL2BM25 对索引执行 FTS5 MATCH，返回匹配的 episode 及其元数据。FTS5 的 bm25 为负（越小越好），
// 这里取反使更大数值表示“更相关”，服务层会再归一化。
func (r *L2Repository) SearchL2BM25(sessionID, query string, minImportance float64, limit int) ([]mem.MemoryL2RecallResult, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := r.db.Query(
		`SELECT e.id, -bm25(memory_l2_index_fts) AS score, m.importance
		 FROM memory_l2_index_fts
		 JOIN memory_l2_index_meta m
		   ON m.episode_id = memory_l2_index_fts.episode_id
		  AND m.text_kind = memory_l2_index_fts.text_kind
		 JOIN memory_episodes e
		   ON e.id = memory_l2_index_fts.episode_id
		 WHERE memory_l2_index_fts MATCH ?
		   AND memory_l2_index_fts.session_id = ?
		   AND e.deleted_at = ''
		   AND e.archived_at = ''
		   AND m.importance >= ?
		 ORDER BY score DESC
		 LIMIT ?`,
		q, sessionID, minImportance, limit,
	)
	if err != nil {
		// FTS5 查询语法错误时视为无命中，便于调用方回退到无 BM25 的召回，而非向上冒泡错误。
		if strings.Contains(strings.ToLower(err.Error()), "syntax error") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	type hit struct {
		id    string
		score float64
		imp   float64
	}
	var hits []hit
	for rows.Next() {
		var h hit
		if err = rows.Scan(&h.id, &h.score, &h.imp); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]mem.MemoryL2RecallResult, 0, len(hits))
	for _, h := range hits {
		ep, gErr := r.GetEpisode(h.id)
		if gErr != nil {
			continue
		}
		out = append(out, mem.MemoryL2RecallResult{
			Episode:   ep,
			BM25Score: h.score,
			FinalRank: h.score,
		})
	}
	return out, nil
}

// UpsertEventMark 利用 (ref_kind, ref_id, mark_type, marked_by) 唯一约束，
// 同一行为者对同一事件重复打标为幂等——仅刷新 `reason`、`weight` 与元数据。
func (r *L2Repository) UpsertEventMark(m mem.MemoryEventMark) (mem.MemoryEventMark, error) {
	if m.SessionID == "" {
		return mem.MemoryEventMark{}, errors.New("mark session_id is required")
	}
	if m.RefKind == "" || m.RefID == "" || m.MarkType == "" {
		return mem.MemoryEventMark{}, errors.New("mark ref_kind, ref_id and mark_type are required")
	}
	if m.ID == "" {
		m.ID = m.RefKind + ":" + m.RefID + ":" + m.MarkType + ":" + m.MarkedBy
	}
	if m.Weight == 0 {
		m.Weight = 1.0
	}
	if m.MetadataJSON == "" {
		if len(m.Metadata) == 0 {
			m.MetadataJSON = "{}"
		} else {
			raw, err := json.Marshal(m.Metadata)
			if err != nil {
				return mem.MemoryEventMark{}, err
			}
			m.MetadataJSON = string(raw)
		}
	}
	now := nowISO()
	if m.CreatedAt == "" {
		m.CreatedAt = now
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_event_marks(
			id, session_id, episode_id, ref_kind, ref_id, mark_type, marked_by,
			reason, weight, metadata_json, created_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(ref_kind, ref_id, mark_type, marked_by) DO UPDATE SET
			session_id = excluded.session_id,
			episode_id = excluded.episode_id,
			reason = excluded.reason,
			weight = excluded.weight,
			metadata_json = excluded.metadata_json,
			deleted_at = ''`,
		m.ID, m.SessionID, m.EpisodeID, m.RefKind, m.RefID, m.MarkType, m.MarkedBy,
		m.Reason, m.Weight, m.MetadataJSON, m.CreatedAt, m.DeletedAt,
	)
	if err != nil {
		return mem.MemoryEventMark{}, err
	}
	return r.getMark(m.RefKind, m.RefID, m.MarkType, m.MarkedBy)
}

// SoftDeleteEventMark 设置 deleted_at，历史标记仍可查审计，但不参与重要性重算。
func (r *L2Repository) SoftDeleteEventMark(id string) error {
	if id == "" {
		return errors.New("mark id is required")
	}
	_, err := r.db.Exec(
		`UPDATE memory_event_marks SET deleted_at = ? WHERE id = ?`,
		nowISO(), id,
	)
	return err
}

// ListEventMarks 列出某会话下未删除的标记，可选按 mark_type 过滤。新到旧，便于界面展示最近活动。
func (r *L2Repository) ListEventMarks(sessionID, markType string, limit int) ([]mem.MemoryEventMark, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	clauses := []string{"session_id = ?", "deleted_at = ''"}
	args := []any{sessionID}
	if markType != "" {
		clauses = append(clauses, "mark_type = ?")
		args = append(args, markType)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	rows, err := r.db.Query(
		memoryEventMarkSelectSQL()+where+` ORDER BY created_at DESC LIMIT ?`,
		append(append([]any{}, args...), limit)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []mem.MemoryEventMark
	for rows.Next() {
		v, scanErr := scanMemoryEventMark(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *L2Repository) getMark(refKind, refID, markType, markedBy string) (mem.MemoryEventMark, error) {
	row := r.db.QueryRow(
		memoryEventMarkSelectSQL()+` WHERE ref_kind = ? AND ref_id = ? AND mark_type = ? AND marked_by = ?`,
		refKind, refID, markType, markedBy,
	)
	return scanMemoryEventMark(row)
}

// ListMarksForEpisode 返回指向某 episode 或该 episode 内产生的事件的全部标记。供 episode 详情（§8.2）使用。
func (r *L2Repository) ListMarksForEpisode(episodeID string) ([]mem.MemoryEventMark, error) {
	if episodeID == "" {
		return nil, errors.New("episode id is required")
	}
	rows, err := r.db.Query(
		memoryEventMarkSelectSQL()+` WHERE episode_id = ? AND deleted_at = '' ORDER BY created_at DESC`,
		episodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []mem.MemoryEventMark
	for rows.Next() {
		v, scanErr := scanMemoryEventMark(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListL2Events 实现规范 §16 中的跨表合并思路。阶段一从 messages / tool_invocations /
// skill_invocation / team_run_steps / monitor_events 取数；session_trace_spans 接入后阶段二再纳入 trace。
// 结果在内存中按 occurred_at 降序合并，再按 (offset, limit) 分页。
func (r *L2Repository) ListL2Events(q mem.MemoryL2EventQuery) ([]mem.MemoryL2Event, int, error) {
	if q.SessionID == "" {
		return nil, 0, errors.New("session id is required")
	}
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	allowed := map[string]bool{}
	for _, k := range q.Kinds {
		allowed[strings.TrimSpace(strings.ToLower(k))] = true
	}
	keep := func(kind string) bool {
		if len(allowed) == 0 {
			return true
		}
		return allowed[kind]
	}

	keyword := strings.TrimSpace(strings.ToLower(q.Keyword))
	matchKeyword := func(values ...string) bool {
		if keyword == "" {
			return true
		}
		for _, v := range values {
			if strings.Contains(strings.ToLower(v), keyword) {
				return true
			}
		}
		return false
	}

	statusAllowed := map[string]bool{}
	for _, s := range q.StatusIn {
		statusAllowed[strings.TrimSpace(strings.ToLower(s))] = true
	}
	matchStatus := func(status string) bool {
		if len(statusAllowed) == 0 {
			return true
		}
		return statusAllowed[strings.ToLower(status)]
	}

	actorAllowed := map[string]bool{}
	for _, a := range q.ActorIDs {
		actorAllowed[strings.TrimSpace(a)] = true
	}
	matchActor := func(id string) bool {
		if len(actorAllowed) == 0 {
			return true
		}
		return actorAllowed[id]
	}

	matchTimeWindow := func(at string) bool {
		if q.StartTimeUTC != "" && at < q.StartTimeUTC {
			return false
		}
		if q.EndTimeUTC != "" && at > q.EndTimeUTC {
			return false
		}
		return true
	}

	events := make([]mem.MemoryL2Event, 0, 64)

	if keep("message") {
		mRows, err := r.db.Query(
			`SELECT id, role, status, content_markdown, token_in, token_out, latency_ms, created_at
			 FROM messages WHERE session_id = ? ORDER BY created_at DESC LIMIT 500`,
			q.SessionID,
		)
		if err != nil {
			return nil, 0, err
		}
		for mRows.Next() {
			var id, role, status, content, createdAt string
			var tokenIn, tokenOut, latency int
			if err = mRows.Scan(&id, &role, &status, &content, &tokenIn, &tokenOut, &latency, &createdAt); err != nil {
				mRows.Close()
				return nil, 0, err
			}
			if !matchStatus(status) || !matchActor(role) || !matchTimeWindow(createdAt) || !matchKeyword(content, role) {
				continue
			}
			events = append(events, mem.MemoryL2Event{
				ID:         id,
				Kind:       "message",
				SessionID:  q.SessionID,
				ActorType:  role,
				ActorName:  role,
				Status:     status,
				Title:      role,
				Preview:    previewText(content, 200),
				OccurredAt: createdAt,
				DurationMS: latency,
				TokensIn:   tokenIn,
				TokensOut:  tokenOut,
				RefTable:   "messages",
				RefID:      id,
			})
		}
		mRows.Close()
	}

	if keep("tool_call") {
		tRows, err := r.db.Query(
			`SELECT id, tool_key, agent_id, status, input_preview, output_preview, started_at, duration_ms
			 FROM tool_invocations WHERE session_id = ? ORDER BY started_at DESC LIMIT 500`,
			q.SessionID,
		)
		if err != nil {
			return nil, 0, err
		}
		for tRows.Next() {
			var id, toolKey, agentID, status, inputPrev, outputPrev, startedAt string
			var duration int
			if err = tRows.Scan(&id, &toolKey, &agentID, &status, &inputPrev, &outputPrev, &startedAt, &duration); err != nil {
				tRows.Close()
				return nil, 0, err
			}
			if !matchStatus(status) || !matchActor(agentID) || !matchTimeWindow(startedAt) || !matchKeyword(toolKey, inputPrev, outputPrev) {
				continue
			}
			events = append(events, mem.MemoryL2Event{
				ID:         id,
				Kind:       "tool_call",
				SessionID:  q.SessionID,
				ActorType:  "agent",
				ActorID:    agentID,
				ActorName:  toolKey,
				Status:     status,
				Title:      toolKey,
				Preview:    previewText(firstNonEmpty(outputPrev, inputPrev), 200),
				OccurredAt: startedAt,
				DurationMS: duration,
				RefTable:   "tool_invocations",
				RefID:      id,
			})
		}
		tRows.Close()
	}

	if keep("skill_call") {
		// 当前 `skill_invocation` 无 session_id（见 migrations/0001_init.sql），
		// 故通过曾出现在本会话 tool_invocations 中的 agent 过滤（现唯一能关联 agent_id 与 session_id 的旁路表）。
		sRows, err := r.db.Query(
			`SELECT si.id, si.skill_id, si.agent_id, si.status, si.input_json, si.output_json, si.created_at, si.updated_at
			 FROM skill_invocation si
			 WHERE si.agent_id IN (SELECT DISTINCT agent_id FROM tool_invocations WHERE session_id = ? AND agent_id != '')
			 ORDER BY si.created_at DESC LIMIT 500`,
			q.SessionID,
		)
		if err != nil {
			return nil, 0, err
		}
		for sRows.Next() {
			var id, skillID, agentID, status, inputJSON, outputJSON, createdAt, updatedAt string
			if err = sRows.Scan(&id, &skillID, &agentID, &status, &inputJSON, &outputJSON, &createdAt, &updatedAt); err != nil {
				sRows.Close()
				return nil, 0, err
			}
			occurred := createdAt
			if updatedAt != "" {
				occurred = updatedAt
			}
			if !matchStatus(status) || !matchActor(agentID) || !matchTimeWindow(occurred) || !matchKeyword(skillID, inputJSON, outputJSON) {
				continue
			}
			events = append(events, mem.MemoryL2Event{
				ID:         id,
				Kind:       "skill_call",
				SessionID:  q.SessionID,
				ActorType:  "agent",
				ActorID:    agentID,
				ActorName:  skillID,
				Status:     status,
				Title:      skillID,
				Preview:    previewText(firstNonEmpty(outputJSON, inputJSON), 200),
				OccurredAt: occurred,
				RefTable:   "skill_invocation",
				RefID:      id,
			})
		}
		sRows.Close()
	}

	if keep("model_call") {
		uRows, err := r.db.Query(
			`SELECT id, agent_id, agent_key, model_api_id, status, input_tokens, output_tokens, total_cost_micro_usd, latency_ms, occurred_at
			 FROM model_token_usage_events WHERE session_id = ? ORDER BY occurred_at DESC LIMIT 500`,
			q.SessionID,
		)
		if err != nil {
			return nil, 0, err
		}
		for uRows.Next() {
			var id, agentID, agentKey, model, status, occurredAt string
			var inTokens, outTokens, latency int
			var cost int64
			if err = uRows.Scan(&id, &agentID, &agentKey, &model, &status, &inTokens, &outTokens, &cost, &latency, &occurredAt); err != nil {
				uRows.Close()
				return nil, 0, err
			}
			if !matchStatus(status) || !matchActor(agentID) || !matchTimeWindow(occurredAt) || !matchKeyword(model, agentKey) {
				continue
			}
			events = append(events, mem.MemoryL2Event{
				ID:         id,
				Kind:       "model_call",
				SessionID:  q.SessionID,
				ActorType:  "agent",
				ActorID:    agentID,
				ActorName:  firstNonEmpty(agentKey, agentID),
				Status:     status,
				Title:      model,
				Preview:    fmt.Sprintf("model=%s in=%d out=%d", model, inTokens, outTokens),
				OccurredAt: occurredAt,
				DurationMS: latency,
				TokensIn:   inTokens,
				TokensOut:  outTokens,
				CostMicro:  cost,
				RefTable:   "model_token_usage_events",
				RefID:      id,
			})
		}
		uRows.Close()
	}

	if keep("agent_handoff") {
		stRows, err := r.db.Query(
			`SELECT id, run_id, team_id, agent_id, agent_name, role, status, output_preview, started_at, duration_ms
			 FROM team_run_steps WHERE run_id IN (SELECT id FROM team_runs WHERE session_id = ?) ORDER BY started_at DESC LIMIT 500`,
			q.SessionID,
		)
		if err != nil {
			return nil, 0, err
		}
		for stRows.Next() {
			var id, runID, teamID, agentID, agentName, role, status, outputPrev, startedAt string
			var duration int
			if err = stRows.Scan(&id, &runID, &teamID, &agentID, &agentName, &role, &status, &outputPrev, &startedAt, &duration); err != nil {
				stRows.Close()
				return nil, 0, err
			}
			if !matchStatus(status) || !matchActor(agentID) || !matchTimeWindow(startedAt) || !matchKeyword(agentName, role, outputPrev) {
				continue
			}
			events = append(events, mem.MemoryL2Event{
				ID:         id,
				Kind:       "agent_handoff",
				SessionID:  q.SessionID,
				RunID:      runID,
				ActorType:  "agent",
				ActorID:    agentID,
				ActorName:  firstNonEmpty(agentName, role),
				Status:     status,
				Title:      role,
				Preview:    previewText(outputPrev, 200),
				OccurredAt: startedAt,
				DurationMS: duration,
				RefTable:   "team_run_steps",
				RefID:      id,
				Metadata:   map[string]any{"team_id": teamID},
			})
		}
		stRows.Close()
	}

	// 在所有来源上按 OccurredAt 降序插入排序，再分页。
	for i := 1; i < len(events); i++ {
		j := i
		for j > 0 && events[j].OccurredAt > events[j-1].OccurredAt {
			events[j], events[j-1] = events[j-1], events[j]
			j--
		}
	}
	total := len(events)
	if offset >= total {
		return []mem.MemoryL2Event{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return events[offset:end], total, nil
}

// ArchiveEpisodesBeforeDate 为早于截止时间的未归档行设置 archived_at。返回影响行数供指标/审计。
func (r *L2Repository) ArchiveEpisodesBeforeDate(sessionID, before string) (int, error) {
	if before == "" {
		return 0, errors.New("before is required")
	}
	now := nowISO()
	args := []any{now, now, before}
	stmt := `UPDATE memory_episodes SET archived_at = ?, updated_at = ?
	         WHERE archived_at = '' AND deleted_at = '' AND ended_at != '' AND ended_at < ?`
	if sessionID != "" {
		stmt += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	res, err := r.db.Exec(stmt, args...)
	if err != nil {
		return 0, err
	}
	rows, _ := res.RowsAffected()
	return int(rows), nil
}

// CountAgentEpisodesSince 统计属于 `agentID` 且未删除的 episode 数量，
// 其 ended_at（进行中则回退为 created_at）不早于 `since`。供 EvolutionScanner
// 按活动量门控 `RunEvolutionScan`（§5.5 第 3 步）。
func (r *L2Repository) CountAgentEpisodesSince(agentID, since string) (int, error) {
	if agentID == "" {
		return 0, errors.New("agent_id is required")
	}
	q := `SELECT COUNT(1) FROM memory_episodes
	      WHERE agent_id = ? AND deleted_at = ''
	        AND COALESCE(NULLIF(ended_at, ''), created_at) >= ?`
	var n int
	if err := r.db.QueryRow(q, agentID, since).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// DeleteArchivedEpisodesBefore 物理删除 archived_at 或 deleted_at 早于截止时间的行。供保留策略定时任务使用。
func (r *L2Repository) DeleteArchivedEpisodesBefore(before string) (int, error) {
	if before == "" {
		return 0, errors.New("before is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(
		`DELETE FROM memory_episodes
		 WHERE (archived_at != '' AND archived_at < ?)
		    OR (deleted_at != '' AND deleted_at < ?)`,
		before, before,
	)
	if err != nil {
		return 0, err
	}
	rows, _ := res.RowsAffected()
	if _, err = tx.Exec(
		`DELETE FROM memory_l2_index_meta WHERE episode_id NOT IN (SELECT id FROM memory_episodes)`,
	); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(
		`DELETE FROM memory_l2_index_fts WHERE episode_id NOT IN (SELECT id FROM memory_episodes)`,
	); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return int(rows), nil
}

// --- 辅助 ---------------------------------------------------------------------

func memoryEpisodeSelectSQL() string {
	return `SELECT id, session_id, run_id, team_id, agent_id, l1_task_id,
		episode_kind, title, goal, outcome, outcome_summary,
		result_preview, failure_reason,
		importance, confidence, user_feedback, critic_score,
		span_count, message_count, tool_call_count, skill_call_count, mcp_call_count,
		total_tokens, total_cost_micro_usd, duration_ms,
		l1_snapshot_json, key_decisions_json, key_artifacts_json,
		embedding_status, embedding_model, embedding_dim, embedding_norm,
		consolidation_status, consolidated_at, consolidated_l3_count, consolidated_l4_count,
		metadata_json, started_at, ended_at,
		created_at, updated_at, archived_at, deleted_at
	 FROM memory_episodes`
}

func scanMemoryEpisode(row scanner) (mem.MemoryEpisode, error) {
	var v mem.MemoryEpisode
	var kind string
	if err := row.Scan(
		&v.ID, &v.SessionID, &v.RunID, &v.TeamID, &v.AgentID, &v.L1TaskID,
		&kind, &v.Title, &v.Goal, &v.Outcome, &v.OutcomeSummary,
		&v.ResultPreview, &v.FailureReason,
		&v.Importance, &v.Confidence, &v.UserFeedback, &v.CriticScore,
		&v.SpanCount, &v.MessageCount, &v.ToolCallCount, &v.SkillCallCount, &v.MCPCallCount,
		&v.TotalTokens, &v.TotalCostMicroUSD, &v.DurationMS,
		&v.L1SnapshotJSON, &v.KeyDecisionsJSON, &v.KeyArtifactsJSON,
		&v.EmbeddingStatus, &v.EmbeddingModel, &v.EmbeddingDim, &v.EmbeddingNorm,
		&v.ConsolidationStatus, &v.ConsolidatedAt, &v.ConsolidatedL3Count, &v.ConsolidatedL4Count,
		&v.MetadataJSON, &v.StartedAt, &v.EndedAt,
		&v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return v, errs.ErrNotFound
		}
		return v, err
	}
	v.Kind = mem.EpisodeKind(kind)
	return v, nil
}

func memoryEventMarkSelectSQL() string {
	return `SELECT id, session_id, episode_id, ref_kind, ref_id, mark_type, marked_by,
		reason, weight, metadata_json, created_at, deleted_at
	 FROM memory_event_marks`
}

func scanMemoryEventMark(row scanner) (mem.MemoryEventMark, error) {
	var v mem.MemoryEventMark
	if err := row.Scan(
		&v.ID, &v.SessionID, &v.EpisodeID, &v.RefKind, &v.RefID, &v.MarkType, &v.MarkedBy,
		&v.Reason, &v.Weight, &v.MetadataJSON, &v.CreatedAt, &v.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return v, errs.ErrNotFound
		}
		return v, err
	}
	if v.MetadataJSON != "" && v.MetadataJSON != "{}" {
		_ = json.Unmarshal([]byte(v.MetadataJSON), &v.Metadata)
	}
	return v, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
