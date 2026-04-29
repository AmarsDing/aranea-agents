// Package sqlite：L3 语义记忆的 SQLite 实现，见 `aranea/docs/15 memory-L3-semantic.md`。
// 约定与 sqlite_memory_l2.go 一致：写路径填默认值、JSON 经辅助函数规范化、
// FTS 通过先删后插保持一致。
package sqlite

import (
	mem "arenea/backend/internal/memory/domain"

	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"arenea/backend/internal/kernel/contracts"
)

// L3Repository 为 L3 语义记忆的 SQLite 实现。
type L3Repository struct {
	db *sql.DB
}

// NewL3Repository 使用与 monolith 相同的 *sql.DB。
func NewL3Repository(db *sql.DB) *L3Repository {
	return &L3Repository{db: db}
}

// CreateFact 插入新的 memory_facts 行。调用方负责指纹与嵌入列；仓库补时间戳与 JSON 默认值。
func (r *L3Repository) CreateFact(f mem.MemoryFact) (mem.MemoryFact, error) {
	if f.ID == "" {
		return mem.MemoryFact{}, errors.New("fact id is required")
	}
	if f.ScopeType == "" {
		return mem.MemoryFact{}, errors.New("fact scope_type is required")
	}
	if strings.TrimSpace(f.Statement) == "" {
		return mem.MemoryFact{}, errors.New("fact statement is required")
	}
	now := nowISO()
	if f.CreatedAt == "" {
		f.CreatedAt = now
	}
	f.UpdatedAt = now
	if f.Kind == "" {
		f.Kind = mem.FactGeneric
	}
	if f.Status == "" {
		f.Status = mem.FactStatusActive
	}
	if f.SourceKind == "" {
		f.SourceKind = "user"
	}
	if f.EmbeddingStatus == "" {
		f.EmbeddingStatus = "pending"
	}
	if f.DecayFactor == 0 {
		f.DecayFactor = 0.98
	}
	if f.Version == 0 {
		f.Version = 1
	}
	if f.Confidence == 0 {
		f.Confidence = 0.7
	}
	if f.Importance == 0 {
		f.Importance = 0.5
	}
	tagsJSON := encodeStringList(f.Tags)
	metaJSON := f.MetadataJSON
	if metaJSON == "" {
		metaJSON = "{}"
	}
	pii := 0
	if f.PIIFlag {
		pii = 1
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_facts(
			id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
			statement, statement_normalized, fingerprint, details_markdown,
			fact_kind, tags_json,
			confidence, importance, use_count, hit_count,
			positive_feedback_count, negative_feedback_count, conflict_count,
			source_kind, source_episode_id, source_session_id, source_message_id, source_external,
			version, status, superseded_by,
			embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
			pii_flag, redacted_statement,
			ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
			metadata_json, created_at, updated_at, archived_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, string(f.ScopeType), f.ScopeID, f.WorkspaceID, f.UserID, f.TeamID, f.AgentID,
		f.Statement, f.StatementNormalized, f.Fingerprint, f.DetailsMarkdown,
		string(f.Kind), tagsJSON,
		f.Confidence, f.Importance, f.UseCount, f.HitCount,
		f.PositiveFeedbackCount, f.NegativeFeedbackCount, f.ConflictCount,
		f.SourceKind, f.SourceEpisodeID, f.SourceSessionID, f.SourceMessageID, f.SourceExternal,
		f.Version, f.Status, f.SupersededBy,
		f.EmbeddingStatus, f.EmbeddingModel, f.EmbeddingDim, f.EmbeddingBlob, f.EmbeddingNorm,
		pii, f.RedactedStatement,
		f.TTLDays, f.DecayFactor, f.NextDecayAt, f.LastUsedAt, f.ExpiresAt,
		metaJSON, f.CreatedAt, f.UpdatedAt, f.ArchivedAt, f.DeletedAt,
	)
	if err != nil {
		return mem.MemoryFact{}, err
	}
	return r.GetFact(f.ID)
}

// UpdateFact 更新可变列。嵌入列另走 UpsertFactEmbedding。
func (r *L3Repository) UpdateFact(f mem.MemoryFact) error {
	if f.ID == "" {
		return errors.New("fact id is required")
	}
	now := nowISO()
	f.UpdatedAt = now
	tagsJSON := encodeStringList(f.Tags)
	metaJSON := f.MetadataJSON
	if metaJSON == "" {
		metaJSON = "{}"
	}
	pii := 0
	if f.PIIFlag {
		pii = 1
	}
	_, err := r.db.Exec(
		`UPDATE memory_facts SET
			statement = ?, statement_normalized = ?, fingerprint = ?, details_markdown = ?,
			fact_kind = ?, tags_json = ?,
			confidence = ?, importance = ?,
			source_kind = ?, source_episode_id = ?, source_session_id = ?, source_message_id = ?, source_external = ?,
			version = ?, status = ?, superseded_by = ?,
			pii_flag = ?, redacted_statement = ?,
			ttl_days = ?, decay_factor = ?, next_decay_at = ?, last_used_at = ?, expires_at = ?,
			metadata_json = ?, updated_at = ?, archived_at = ?, deleted_at = ?
		 WHERE id = ?`,
		f.Statement, f.StatementNormalized, f.Fingerprint, f.DetailsMarkdown,
		string(f.Kind), tagsJSON,
		f.Confidence, f.Importance,
		f.SourceKind, f.SourceEpisodeID, f.SourceSessionID, f.SourceMessageID, f.SourceExternal,
		f.Version, f.Status, f.SupersededBy,
		pii, f.RedactedStatement,
		f.TTLDays, f.DecayFactor, f.NextDecayAt, f.LastUsedAt, f.ExpiresAt,
		metaJSON, now, f.ArchivedAt, f.DeletedAt,
		f.ID,
	)
	return err
}

// GetFact 按 ID 返回单条 fact。含软删行供审计/版本路径；Recall/List 由服务层过滤。
func (r *L3Repository) GetFact(id string) (mem.MemoryFact, error) {
	row := r.db.QueryRow(memoryFactSelectSQL()+` WHERE id = ?`, id)
	return scanMemoryFact(row)
}

// GetFactByFingerprint 为 UpsertFact 的去重探针。无匹配时返回 sql.ErrNoRows，调用方可用 errors.Is 分支。
func (r *L3Repository) GetFactByFingerprint(scopeType mem.ScopeType, scopeID, fp string) (mem.MemoryFact, error) {
	row := r.db.QueryRow(
		memoryFactSelectSQL()+` WHERE scope_type = ? AND scope_id = ? AND fingerprint = ?`,
		string(scopeType), scopeID, fp,
	)
	return scanMemoryFact(row)
}

// ListFacts 为 §6.2 GET 端点返回分页、过滤后的 fact。排除软删。
func (r *L3Repository) ListFacts(q contracts.FactListQuery) ([]mem.MemoryFact, int, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 200 {
		q.Limit = 200
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	where := []string{"deleted_at = ''"}
	args := []any{}
	if q.ScopeType != "" {
		where = append(where, "scope_type = ?")
		args = append(args, string(q.ScopeType))
	}
	if q.ScopeID != "" {
		where = append(where, "scope_id = ?")
		args = append(args, q.ScopeID)
	}
	if q.WorkspaceID != "" {
		where = append(where, "workspace_id = ?")
		args = append(args, q.WorkspaceID)
	}
	if q.UserID != "" {
		where = append(where, "user_id = ?")
		args = append(args, q.UserID)
	}
	if q.TeamID != "" {
		where = append(where, "team_id = ?")
		args = append(args, q.TeamID)
	}
	if q.AgentID != "" {
		where = append(where, "agent_id = ?")
		args = append(args, q.AgentID)
	}
	if q.Status != "" {
		where = append(where, "status = ?")
		args = append(args, q.Status)
	}
	if q.Kind != "" {
		where = append(where, "fact_kind = ?")
		args = append(args, string(q.Kind))
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		where = append(where, "(LOWER(statement) LIKE ? OR LOWER(details_markdown) LIKE ?)")
		like := "%" + strings.ToLower(kw) + "%"
		args = append(args, like, like)
	}
	for _, tag := range q.Tags {
		t := strings.TrimSpace(tag)
		if t == "" {
			continue
		}
		where = append(where, "LOWER(tags_json) LIKE ?")
		args = append(args, "%\""+strings.ToLower(t)+"\"%")
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM memory_facts WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, q.Limit, q.Offset)
	rows, err := r.db.Query(
		memoryFactSelectSQL()+` WHERE `+whereSQL+` ORDER BY importance DESC, updated_at DESC LIMIT ? OFFSET ?`,
		listArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []mem.MemoryFact{}
	for rows.Next() {
		v, scanErr := scanMemoryFact(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// UpdateFactConfidence 单条语句完成基于反馈的更新，避免并发写竞态。调用方传目标置信度（限幅 0..1）与计数增量，仓库只持久化。
func (r *L3Repository) UpdateFactConfidence(id string, newConfidence float64, hitInc, posInc, negInc int) error {
	if id == "" {
		return errors.New("fact id is required")
	}
	if newConfidence < 0 {
		newConfidence = 0
	}
	if newConfidence > 1 {
		newConfidence = 1
	}
	_, err := r.db.Exec(
		`UPDATE memory_facts SET
			confidence = ?,
			hit_count = hit_count + ?,
			positive_feedback_count = positive_feedback_count + ?,
			negative_feedback_count = negative_feedback_count + ?,
			updated_at = ?
		 WHERE id = ?`,
		newConfidence, hitInc, posInc, negInc, nowISO(), id,
	)
	return err
}

// UpdateFactStatus 修改生命周期列。传 archivedAt 以记录归档时间；空则保留原值。
func (r *L3Repository) UpdateFactStatus(id, status, supersededBy, archivedAt string) error {
	if id == "" {
		return errors.New("fact id is required")
	}
	if status == "" {
		return errors.New("status is required")
	}
	if archivedAt == "" && status == mem.FactStatusArchived {
		archivedAt = nowISO()
	}
	_, err := r.db.Exec(
		`UPDATE memory_facts SET status = ?, superseded_by = ?, archived_at = COALESCE(NULLIF(?, ''), archived_at), updated_at = ? WHERE id = ?`,
		status, supersededBy, archivedAt, nowISO(), id,
	)
	return err
}

// BumpFactUseStat 在事实已拼入提示后从召回路径调用。实际注入为 hit=true；仅检索未展示为 hit=false。
func (r *L3Repository) BumpFactUseStat(id string, hit bool, atISO string) error {
	if id == "" {
		return errors.New("fact id is required")
	}
	if atISO == "" {
		atISO = nowISO()
	}
	hitInc := 0
	if hit {
		hitInc = 1
	}
	_, err := r.db.Exec(
		`UPDATE memory_facts SET use_count = use_count + 1, hit_count = hit_count + ?, last_used_at = ?, updated_at = ? WHERE id = ?`,
		hitInc, atISO, nowISO(), id,
	)
	return err
}

// InsertFactVersion 在 `memory_fact_versions` 中记录快照供回滚/审计。(fact_id, version) 唯一约束防重复写入。
func (r *L3Repository) InsertFactVersion(fv mem.FactVersion) error {
	if fv.ID == "" {
		return errors.New("version id is required")
	}
	if fv.FactID == "" {
		return errors.New("version fact_id is required")
	}
	if fv.Version <= 0 {
		return errors.New("version number must be positive")
	}
	if fv.CreatedAt == "" {
		fv.CreatedAt = nowISO()
	}
	tagsJSON := encodeStringList(fv.Tags)
	if fv.DiffJSON == "" {
		fv.DiffJSON = "{}"
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_fact_versions(
			id, fact_id, version, statement, details_markdown, tags_json,
			confidence, status, changed_by, change_reason, diff_json, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fv.ID, fv.FactID, fv.Version, fv.Statement, fv.Details, tagsJSON,
		fv.Confidence, fv.Status, fv.ChangedBy, fv.ChangeReason, fv.DiffJSON, "{}", fv.CreatedAt,
	)
	return err
}

// ListFactVersions 返回降序版本历史，最新在前。Limit 控制体积。
func (r *L3Repository) ListFactVersions(factID string, limit int) ([]mem.FactVersion, error) {
	if factID == "" {
		return nil, errors.New("fact id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.db.Query(
		`SELECT id, fact_id, version, statement, details_markdown, tags_json, confidence, status, changed_by, change_reason, diff_json, created_at
		 FROM memory_fact_versions WHERE fact_id = ? ORDER BY version DESC LIMIT ?`,
		factID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mem.FactVersion{}
	for rows.Next() {
		v, scanErr := scanFactVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetFactVersion 查询指定 (fact_id, version)，供回滚路径重放快照。
func (r *L3Repository) GetFactVersion(factID string, version int) (mem.FactVersion, error) {
	if factID == "" {
		return mem.FactVersion{}, errors.New("fact id is required")
	}
	row := r.db.QueryRow(
		`SELECT id, fact_id, version, statement, details_markdown, tags_json, confidence, status, changed_by, change_reason, diff_json, created_at
		 FROM memory_fact_versions WHERE fact_id = ? AND version = ?`,
		factID, version,
	)
	return scanFactVersion(row)
}

// InsertFactFeedback 向 `memory_fact_feedback` 追加一行。服务层再经 UpdateFactConfidence 转为置信度/重要性更新。
func (r *L3Repository) InsertFactFeedback(fb mem.FactFeedback) (mem.FactFeedback, error) {
	if fb.ID == "" {
		return mem.FactFeedback{}, errors.New("feedback id is required")
	}
	if fb.FactID == "" {
		return mem.FactFeedback{}, errors.New("feedback fact_id is required")
	}
	if fb.Type == "" {
		return mem.FactFeedback{}, errors.New("feedback type is required")
	}
	if fb.Source == "" {
		return mem.FactFeedback{}, errors.New("feedback source is required")
	}
	if fb.CreatedAt == "" {
		fb.CreatedAt = nowISO()
	}
	if fb.Weight == 0 {
		fb.Weight = 1.0
	}
	metaJSON := encodeMetaJSONMap(fb.Metadata)
	_, err := r.db.Exec(
		`INSERT INTO memory_fact_feedback(
			id, fact_id, session_id, agent_id, feedback_type, source, weight, comment, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fb.ID, fb.FactID, fb.SessionID, fb.AgentID, fb.Type, fb.Source, fb.Weight, fb.Comment, metaJSON, fb.CreatedAt,
	)
	if err != nil {
		return mem.FactFeedback{}, err
	}
	return fb, nil
}

// ListFactFeedback 返回某 fact 最近的反馈条目。
func (r *L3Repository) ListFactFeedback(factID string, limit int) ([]mem.FactFeedback, error) {
	if factID == "" {
		return nil, errors.New("fact id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.db.Query(
		`SELECT id, fact_id, session_id, agent_id, feedback_type, source, weight, comment, metadata_json, created_at
		 FROM memory_fact_feedback WHERE fact_id = ? ORDER BY created_at DESC LIMIT ?`,
		factID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mem.FactFeedback{}
	for rows.Next() {
		var v mem.FactFeedback
		var meta string
		if err = rows.Scan(&v.ID, &v.FactID, &v.SessionID, &v.AgentID, &v.Type, &v.Source, &v.Weight, &v.Comment, &meta, &v.CreatedAt); err != nil {
			return nil, err
		}
		if meta != "" && meta != "{}" {
			_ = json.Unmarshal([]byte(meta), &v.Metadata)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CountRecentFactFeedback 统计某类型下最近 N 条反馈（§5.4 第 5 步「连续三次拒绝自动生成冲突」）。limit 限制回溯窗口。
func (r *L3Repository) CountRecentFactFeedback(factID, feedbackType string, limit int) (int, error) {
	if factID == "" {
		return 0, errors.New("fact id is required")
	}
	if limit <= 0 {
		limit = 3
	}
	rows, err := r.db.Query(
		`SELECT feedback_type FROM memory_fact_feedback WHERE fact_id = ? ORDER BY created_at DESC LIMIT ?`,
		factID, limit,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var t string
		if err = rows.Scan(&t); err != nil {
			return 0, err
		}
		if t == feedbackType {
			count++
		} else {
			break
		}
	}
	return count, rows.Err()
}

// CountAgentFactFeedbackSince 统计归属于 `agentID`、创建时间不早于 `since`、且
// `feedback_type` 属于 `feedbackTypes` 的 `memory_fact_feedback` 行数。供 EvolutionScanner
// 评估 §5.5 负反馈触发，而无需加载全量历史。
func (r *L3Repository) CountAgentFactFeedbackSince(agentID string, feedbackTypes []string, since string) (int, error) {
	if agentID == "" {
		return 0, errors.New("agent id is required")
	}
	if since == "" {
		return 0, errors.New("since is required")
	}
	args := []any{agentID, since}
	q := `SELECT COUNT(1) FROM memory_fact_feedback
	      WHERE agent_id = ? AND created_at >= ?`
	if len(feedbackTypes) > 0 {
		placeholders := strings.Repeat("?,", len(feedbackTypes))
		placeholders = placeholders[:len(placeholders)-1]
		q += " AND feedback_type IN (" + placeholders + ")"
		for _, t := range feedbackTypes {
			args = append(args, t)
		}
	}
	var n int
	if err := r.db.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// UpsertFactConflict 按 (fact_a_id, fact_b_id) 插入或更新冲突行。服务层会规范化 ID 避免同对重复。
func (r *L3Repository) UpsertFactConflict(c mem.FactConflict) (mem.FactConflict, error) {
	if c.FactAID == "" || c.FactBID == "" {
		return mem.FactConflict{}, errors.New("conflict fact ids are required")
	}
	if c.ID == "" {
		c.ID = c.FactAID + ":" + c.FactBID
	}
	if c.Kind == "" {
		c.Kind = mem.FactConflictContradiction
	}
	if c.Status == "" {
		c.Status = mem.FactConflictStatusOpen
	}
	now := nowISO()
	if c.CreatedAt == "" {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	_, err := r.db.Exec(
		`INSERT INTO memory_fact_conflicts(
			id, fact_a_id, fact_b_id, scope_type, scope_id, conflict_kind, similarity, status,
			detected_by, resolution, resolved_by, resolved_at, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(fact_a_id, fact_b_id) DO UPDATE SET
			conflict_kind = excluded.conflict_kind,
			similarity = excluded.similarity,
			status = excluded.status,
			detected_by = excluded.detected_by,
			updated_at = excluded.updated_at`,
		c.ID, c.FactAID, c.FactBID, string(c.ScopeType), c.ScopeID, c.Kind, c.Similarity, c.Status,
		c.DetectedBy, c.Resolution, c.ResolvedBy, c.ResolvedAt, "{}", c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return mem.FactConflict{}, err
	}
	return r.GetFactConflict(c.ID)
}

// GetFactConflict 按 id 返回冲突记录。
func (r *L3Repository) GetFactConflict(id string) (mem.FactConflict, error) {
	row := r.db.QueryRow(
		`SELECT id, fact_a_id, fact_b_id, scope_type, scope_id, conflict_kind, similarity, status,
			detected_by, resolution, resolved_by, resolved_at, created_at, updated_at
		 FROM memory_fact_conflicts WHERE id = ?`,
		id,
	)
	return scanFactConflict(row)
}

// ListOpenFactConflicts 返回给定范围内 status 为 "open" 的冲突。scope 参数为空则关闭对应过滤。
func (r *L3Repository) ListOpenFactConflicts(scope mem.ScopeType, scopeID string, limit int) ([]mem.FactConflict, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	where := []string{"status = ?"}
	args := []any{mem.FactConflictStatusOpen}
	if scope != "" {
		where = append(where, "scope_type = ?")
		args = append(args, string(scope))
	}
	if scopeID != "" {
		where = append(where, "scope_id = ?")
		args = append(args, scopeID)
	}
	args = append(args, limit)
	rows, err := r.db.Query(
		`SELECT id, fact_a_id, fact_b_id, scope_type, scope_id, conflict_kind, similarity, status,
			detected_by, resolution, resolved_by, resolved_at, created_at, updated_at
		 FROM memory_fact_conflicts WHERE `+strings.Join(where, " AND ")+` ORDER BY created_at DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mem.FactConflict{}
	for rows.Next() {
		v, scanErr := scanFactConflict(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateFactConflictResolution 将冲突标为已解决（或已忽略等）。
func (r *L3Repository) UpdateFactConflictResolution(id, status, resolution, by, resolvedAt string) error {
	if id == "" {
		return errors.New("conflict id is required")
	}
	if status == "" {
		status = mem.FactConflictStatusResolved
	}
	if resolvedAt == "" {
		resolvedAt = nowISO()
	}
	_, err := r.db.Exec(
		`UPDATE memory_fact_conflicts SET status = ?, resolution = ?, resolved_by = ?, resolved_at = ?, updated_at = ? WHERE id = ?`,
		status, resolution, by, resolvedAt, nowISO(), id,
	)
	return err
}

// UpsertFactEmbedding 同时写入 `memory_facts` 的嵌入列与 `memory_fact_index` 镜像，便于向量检索单表取全。
func (r *L3Repository) UpsertFactEmbedding(id, model string, dim int, blob []byte, norm float64) error {
	if id == "" {
		return errors.New("fact id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(
		`UPDATE memory_facts SET embedding_status = 'ready', embedding_model = ?, embedding_dim = ?, embedding_blob = ?, embedding_norm = ?, updated_at = ? WHERE id = ?`,
		model, dim, blob, norm, nowISO(), id,
	); err != nil {
		return err
	}
	row := tx.QueryRow(`SELECT scope_type, scope_id, importance, confidence FROM memory_facts WHERE id = ?`, id)
	var scopeType, scopeID string
	var importance, confidence float64
	if err = row.Scan(&scopeType, &scopeID, &importance, &confidence); err != nil {
		return err
	}
	if _, err = tx.Exec(
		`INSERT INTO memory_fact_index(fact_id, scope_type, scope_id, embedding_model, embedding_dim, embedding_blob, embedding_norm, importance, confidence, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(fact_id) DO UPDATE SET
			embedding_model = excluded.embedding_model,
			embedding_dim = excluded.embedding_dim,
			embedding_blob = excluded.embedding_blob,
			embedding_norm = excluded.embedding_norm,
			importance = excluded.importance,
			confidence = excluded.confidence,
			updated_at = excluded.updated_at`,
		id, scopeType, scopeID, model, dim, blob, norm, importance, confidence, nowISO(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

// UpsertFactsFTS 保持 BM25 索引同步。FTS5 无 upsert，故先删后插；空文本则整行移除。
func (r *L3Repository) UpsertFactsFTS(factID string, scopeType mem.ScopeType, scopeID, kind, text string) error {
	if factID == "" {
		return errors.New("fact id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM memory_facts_fts WHERE fact_id = ?`, factID); err != nil {
		return err
	}
	if strings.TrimSpace(text) != "" {
		if _, err = tx.Exec(
			`INSERT INTO memory_facts_fts(fact_id, scope_type, scope_id, fact_kind, text) VALUES (?, ?, ?, ?, ?)`,
			factID, string(scopeType), scopeID, kind, text,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteFactIndex 删除某 fact 的 BM25 与向量索引项。事实行保留（软删由 UpdateFactStatus 处理）。
func (r *L3Repository) DeleteFactIndex(factID string) error {
	if factID == "" {
		return errors.New("fact id is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM memory_facts_fts WHERE fact_id = ?`, factID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM memory_fact_index WHERE fact_id = ?`, factID); err != nil {
		return err
	}
	return tx.Commit()
}

// SearchFactsBM25 对事实索引做 FTS5 MATCH 并连接元数据。FTS5 的 bm25 为负（越小越好），取反后交给服务层。
func (r *L3Repository) SearchFactsBM25(scopes []mem.ScopeType, scopeIDs []string, query string, limit int) ([]mem.FactRecallHit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	where := []string{"memory_facts_fts MATCH ?", "f.deleted_at = ''", "f.status = ?"}
	args := []any{q, mem.FactStatusActive}
	if scopeFilter, scopeArgs := buildScopeFilter("memory_facts_fts.scope_type", "memory_facts_fts.scope_id", scopes, scopeIDs); scopeFilter != "" {
		where = append(where, scopeFilter)
		args = append(args, scopeArgs...)
	}
	args = append(args, limit)
	rows, err := r.db.Query(
		`SELECT f.id, -bm25(memory_facts_fts) AS score
		 FROM memory_facts_fts
		 JOIN memory_facts f ON f.id = memory_facts_fts.fact_id
		 WHERE `+strings.Join(where, " AND ")+`
		 ORDER BY score DESC
		 LIMIT ?`,
		args...,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "syntax error") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	type row struct {
		id    string
		score float64
	}
	var hits []row
	for rows.Next() {
		var h row
		if err = rows.Scan(&h.id, &h.score); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]mem.FactRecallHit, 0, len(hits))
	for _, h := range hits {
		f, gErr := r.GetFact(h.id)
		if gErr != nil {
			continue
		}
		out = append(out, mem.FactRecallHit{
			Fact:      f,
			BM25Score: h.score,
			Reason:    "bm25",
		})
	}
	return out, nil
}

// SearchFactsVector 在 Go 中计算余弦相似度（SQLite 无原生向量类型）。下推作用域过滤，不加载越权嵌入。
// 阶段一存根：尚无嵌入时（常见首跑）返回空切片且不报错。
func (r *L3Repository) SearchFactsVector(scopes []mem.ScopeType, scopeIDs []string, q []float32, limit int) ([]mem.FactRecallHit, error) {
	if len(q) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	where := []string{"f.deleted_at = ''", "f.status = ?", "f.embedding_status = 'ready'", "i.embedding_dim = ?"}
	args := []any{mem.FactStatusActive, len(q)}
	if scopeFilter, scopeArgs := buildScopeFilter("i.scope_type", "i.scope_id", scopes, scopeIDs); scopeFilter != "" {
		where = append(where, scopeFilter)
		args = append(args, scopeArgs...)
	}
	rows, err := r.db.Query(
		`SELECT f.id, i.embedding_blob, i.embedding_norm
		 FROM memory_fact_index i
		 JOIN memory_facts f ON f.id = i.fact_id
		 WHERE `+strings.Join(where, " AND "),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type cand struct {
		id    string
		score float64
	}
	qNorm := vectorNorm(q)
	if qNorm == 0 {
		return nil, nil
	}
	var cands []cand
	for rows.Next() {
		var id string
		var blob []byte
		var norm float64
		if err = rows.Scan(&id, &blob, &norm); err != nil {
			return nil, err
		}
		vec, decErr := decodeFloat32Blob(blob)
		if decErr != nil || len(vec) != len(q) {
			continue
		}
		denom := norm * qNorm
		if denom == 0 {
			continue
		}
		cands = append(cands, cand{id: id, score: dotProduct(vec, q) / denom})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) > limit {
		cands = cands[:limit]
	}
	out := make([]mem.FactRecallHit, 0, len(cands))
	for _, c := range cands {
		f, gErr := r.GetFact(c.id)
		if gErr != nil {
			continue
		}
		out = append(out, mem.FactRecallHit{
			Fact:        f,
			VectorScore: c.score,
			Reason:      "vector",
		})
	}
	return out, nil
}

// ListFactsDueForDecay 返回已到期应衰减的活跃 fact，供衰减工作进程批量处理。
func (r *L3Repository) ListFactsDueForDecay(before string, limit int) ([]mem.MemoryFact, error) {
	if before == "" {
		before = nowISO()
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := r.db.Query(
		memoryFactSelectSQL()+` WHERE deleted_at = '' AND status = ? AND (next_decay_at = '' OR next_decay_at <= ?) ORDER BY confidence ASC LIMIT ?`,
		mem.FactStatusActive, before, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mem.MemoryFact{}
	for rows.Next() {
		v, scanErr := scanMemoryFact(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ApplyFactDecay 将 confidence 乘以 `factor` 并推进 next_decay_at。下限钳位为防御性；服务层不应传负 factor。
func (r *L3Repository) ApplyFactDecay(factID string, factor float64, nextAt string) error {
	if factID == "" {
		return errors.New("fact id is required")
	}
	if factor <= 0 {
		factor = 0.98
	}
	if nextAt == "" {
		nextAt = nowISO()
	}
	_, err := r.db.Exec(
		`UPDATE memory_facts SET confidence = MAX(0, MIN(1, confidence * ?)), next_decay_at = ?, updated_at = ? WHERE id = ?`,
		factor, nextAt, nowISO(), factID,
	)
	return err
}

// ArchiveFactsBelowConfidence 将置信度低于阈值的活跃 fact 一批标为「已归档」。返回影响行数。
func (r *L3Repository) ArchiveFactsBelowConfidence(threshold float64, limit int) (int, error) {
	if threshold <= 0 {
		threshold = 0.2
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}
	now := nowISO()
	res, err := r.db.Exec(
		`UPDATE memory_facts SET status = ?, archived_at = ?, updated_at = ?
		 WHERE id IN (
		   SELECT id FROM memory_facts WHERE deleted_at = '' AND status = ? AND confidence < ? ORDER BY confidence ASC LIMIT ?
		 )`,
		mem.FactStatusArchived, now, now, mem.FactStatusActive, threshold, limit,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// CountFactsByStatus 按 status 聚合计数，供管理统计面板。scope 为空则不过滤该维度。
func (r *L3Repository) CountFactsByStatus(scope mem.ScopeType, scopeID string) (map[string]int, error) {
	where := []string{"deleted_at = ''"}
	args := []any{}
	if scope != "" {
		where = append(where, "scope_type = ?")
		args = append(args, string(scope))
	}
	if scopeID != "" {
		where = append(where, "scope_id = ?")
		args = append(args, scopeID)
	}
	rows, err := r.db.Query(
		`SELECT status, COUNT(1) FROM memory_facts WHERE `+strings.Join(where, " AND ")+` GROUP BY status`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var c int
		if err = rows.Scan(&s, &c); err != nil {
			return nil, err
		}
		out[s] = c
	}
	return out, rows.Err()
}

// --- 辅助 ---------------------------------------------------------------------

func memoryFactSelectSQL() string {
	return `SELECT id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
		statement, statement_normalized, fingerprint, details_markdown,
		fact_kind, tags_json,
		confidence, importance, use_count, hit_count,
		positive_feedback_count, negative_feedback_count, conflict_count,
		source_kind, source_episode_id, source_session_id, source_message_id, source_external,
		version, status, superseded_by,
		embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
		pii_flag, redacted_statement,
		ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
		metadata_json, created_at, updated_at, archived_at, deleted_at
	 FROM memory_facts`
}

func scanMemoryFact(row scanner) (mem.MemoryFact, error) {
	var v mem.MemoryFact
	var scope, kind, tagsJSON string
	var pii int
	var blob []byte
	if err := row.Scan(
		&v.ID, &scope, &v.ScopeID, &v.WorkspaceID, &v.UserID, &v.TeamID, &v.AgentID,
		&v.Statement, &v.StatementNormalized, &v.Fingerprint, &v.DetailsMarkdown,
		&kind, &tagsJSON,
		&v.Confidence, &v.Importance, &v.UseCount, &v.HitCount,
		&v.PositiveFeedbackCount, &v.NegativeFeedbackCount, &v.ConflictCount,
		&v.SourceKind, &v.SourceEpisodeID, &v.SourceSessionID, &v.SourceMessageID, &v.SourceExternal,
		&v.Version, &v.Status, &v.SupersededBy,
		&v.EmbeddingStatus, &v.EmbeddingModel, &v.EmbeddingDim, &blob, &v.EmbeddingNorm,
		&pii, &v.RedactedStatement,
		&v.TTLDays, &v.DecayFactor, &v.NextDecayAt, &v.LastUsedAt, &v.ExpiresAt,
		&v.MetadataJSON, &v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.DeletedAt,
	); err != nil {
		return mem.MemoryFact{}, err
	}
	v.ScopeType = mem.ScopeType(scope)
	v.Kind = mem.FactKind(kind)
	v.Tags = decodeStringList(tagsJSON)
	v.PIIFlag = pii != 0
	v.EmbeddingBlob = blob
	return v, nil
}

func scanFactVersion(row scanner) (mem.FactVersion, error) {
	var v mem.FactVersion
	var tagsJSON string
	if err := row.Scan(&v.ID, &v.FactID, &v.Version, &v.Statement, &v.Details, &tagsJSON, &v.Confidence, &v.Status, &v.ChangedBy, &v.ChangeReason, &v.DiffJSON, &v.CreatedAt); err != nil {
		return mem.FactVersion{}, err
	}
	v.Tags = decodeStringList(tagsJSON)
	return v, nil
}

func scanFactConflict(row scanner) (mem.FactConflict, error) {
	var v mem.FactConflict
	var scope string
	if err := row.Scan(&v.ID, &v.FactAID, &v.FactBID, &scope, &v.ScopeID, &v.Kind, &v.Similarity, &v.Status, &v.DetectedBy, &v.Resolution, &v.ResolvedBy, &v.ResolvedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return mem.FactConflict{}, err
	}
	v.ScopeType = mem.ScopeType(scope)
	return v, nil
}

// buildScopeFilter 为 FTS 视图或元索引上的作用域查询生成 IN/配对条件。调用方可传与 scopes 等长的
// scopeIDs 一一对应，或仅传 scope 类型列表作宽泛过滤。
func buildScopeFilter(scopeCol, scopeIDCol string, scopes []mem.ScopeType, scopeIDs []string) (string, []any) {
	if len(scopes) == 0 {
		return "", nil
	}
	// 当 scopeIDs 与 scopes 等长时，每项视为 (scope, scope_id) 对，避免跨用户/团队泄漏。
	if len(scopeIDs) == len(scopes) {
		var clauses []string
		var args []any
		for i, sc := range scopes {
			clauses = append(clauses, fmt.Sprintf("(%s = ? AND %s = ?)", scopeCol, scopeIDCol))
			args = append(args, string(sc), scopeIDs[i])
		}
		return "(" + strings.Join(clauses, " OR ") + ")", args
	}
	// 否则仅按 scope 类型过滤。用于 global 或调用方不知道 scope_id 时。
	placeholders := make([]string, 0, len(scopes))
	args := make([]any, 0, len(scopes))
	for _, sc := range scopes {
		placeholders = append(placeholders, "?")
		args = append(args, string(sc))
	}
	return fmt.Sprintf("%s IN (%s)", scopeCol, strings.Join(placeholders, ",")), args
}

func encodeStringList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func decodeStringList(raw string) []string {
	if strings.TrimSpace(raw) == "" || raw == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func encodeMetaJSONMap(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
