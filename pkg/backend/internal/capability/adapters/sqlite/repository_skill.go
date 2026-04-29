package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"arenea/backend/internal/domain"
)

// SkillRepository 承载技能相关的 SQLite 实现（hexagonal 适配器）。
type SkillRepository struct {
	db *sql.DB
}

// NewSkillRepository 从 *sql.DB 构建技能仓库。
func NewSkillRepository(db *sql.DB) *SkillRepository {
	return &SkillRepository{db: db}
}

// SearchSkills 分页检索技能。
func (r *SkillRepository) SearchSkills(query domain.SkillListQuery) (domain.SkillListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	where, args := skillWhereClause(query)
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM skill s WHERE `+where, args...).Scan(&total); err != nil {
		return domain.SkillListResult{}, err
	}

	listArgs := append([]any{time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339)}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := r.db.Query(skillSelectSQL()+` WHERE `+where+` ORDER BY s.updated_at DESC, s.created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.SkillListResult{}, err
	}
	defer rows.Close()

	items, err := scanSkills(rows)
	if err != nil {
		return domain.SkillListResult{}, err
	}
	return domain.SkillListResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

// GetSkillByID 按主键取技能（未删除行）。
func (r *SkillRepository) GetSkillByID(id string) (domain.Skill, error) {
	rows, err := r.db.Query(skillSelectSQL()+` WHERE s.id = ? AND s.deleted_at = '' LIMIT 1`, time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339), id)
	if err != nil {
		return domain.Skill{}, err
	}
	defer rows.Close()
	items, err := scanSkills(rows)
	if err != nil {
		return domain.Skill{}, err
	}
	if len(items) == 0 {
		return domain.Skill{}, sql.ErrNoRows
	}
	return items[0], nil
}

// UpdateSkillEnabled 更新技能启用位。
func (r *SkillRepository) UpdateSkillEnabled(id string, enabled bool) (domain.Skill, error) {
	if id == "" {
		return domain.Skill{}, errors.New("skill id is required")
	}
	if _, err := r.db.Exec(`UPDATE skill SET enabled = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, enabled, nowISO(), id); err != nil {
		return domain.Skill{}, err
	}
	return r.GetSkillByID(id)
}

// DuplicateSkill 复制技能为草稿副本。
func (r *SkillRepository) DuplicateSkill(id string) (domain.Skill, error) {
	current, err := r.GetSkillByID(id)
	if err != nil {
		return domain.Skill{}, err
	}
	newID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	newKey := fmt.Sprintf("%s-copy-%d", current.Slug, time.Now().UTC().Unix())
	if strings.TrimSpace(current.Slug) == "" {
		newKey = newID
	}
	now := nowISO()
	var configJSON, metadataJSON string
	if err = r.db.QueryRow(`SELECT config_json, metadata_json FROM skill WHERE id = ? AND deleted_at = ''`, id).Scan(&configJSON, &metadataJSON); err != nil {
		return domain.Skill{}, err
	}
	_, err = r.db.Exec(
		`INSERT INTO skill(id, skill_key, name, description, status, enabled, sort_order, config_json, metadata_json, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, 'draft', 0, 0, ?, ?, ?, ?, '')`,
		newID, newKey, current.Name+" Copy", current.Description, configJSON, metadataJSON, now, now,
	)
	if err != nil {
		return domain.Skill{}, err
	}
	return r.GetSkillByID(newID)
}

// DeleteSkill 软删技能，与平台资源删除语义一致。
func (r *SkillRepository) DeleteSkill(id string) error {
	_, err := r.db.Exec(`UPDATE skill SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = ''`, nowISO(), nowISO(), id)
	return err
}

// SearchSkillInvocations 查询技能执行记录。
func (r *SkillRepository) SearchSkillInvocations(query domain.SkillRunQuery) (domain.SkillRunResult, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	where := []string{"1 = 1"}
	args := []any{}
	if query.SkillID != "" {
		where = append(where, "si.skill_id = ?")
		args = append(args, query.SkillID)
	}
	if query.AgentID != "" {
		where = append(where, "si.agent_id = ?")
		args = append(args, query.AgentID)
	}
	if query.SessionID != "" {
		where = append(where, "si.session_id = ?")
		args = append(args, query.SessionID)
	}
	if query.Status != "" {
		where = append(where, "si.status = ?")
		args = append(args, query.Status)
	}
	if query.From != "" {
		where = append(where, "COALESCE(NULLIF(si.started_at, ''), si.created_at) >= ?")
		args = append(args, query.From)
	}
	if query.To != "" {
		where = append(where, "COALESCE(NULLIF(si.started_at, ''), si.created_at) <= ?")
		args = append(args, query.To)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM skill_invocation si WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return domain.SkillRunResult{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := r.db.Query(`
		SELECT si.id, si.skill_id, COALESCE(s.name, ''), si.skill_version, si.agent_id, COALESCE(a.display_name, ''),
		       si.user_id, si.session_id, si.status, si.duration_ms,
		       COALESCE(NULLIF(si.started_at, ''), si.created_at), si.ended_at,
		       si.input_preview, si.input_hash, si.output_preview, si.error_code, si.error_message
		FROM skill_invocation si
		LEFT JOIN skill s ON s.id = si.skill_id
		LEFT JOIN agents a ON a.id = si.agent_id
		WHERE `+whereSQL+`
		ORDER BY COALESCE(NULLIF(si.started_at, ''), si.created_at) DESC
		LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.SkillRunResult{}, err
	}
	defer rows.Close()
	items := []domain.SkillInvocation{}
	for rows.Next() {
		var item domain.SkillInvocation
		if err = rows.Scan(
			&item.ID, &item.SkillID, &item.SkillName, &item.SkillVersion, &item.AgentID, &item.AgentDisplayName,
			&item.UserID, &item.SessionID, &item.Status, &item.DurationMS, &item.StartedAt, &item.EndedAt,
			&item.InputPreview, &item.InputHash, &item.OutputPreview, &item.ErrorCode, &item.ErrorMessage,
		); err != nil {
			return domain.SkillRunResult{}, err
		}
		item.Permissions = domain.SkillInvocationPermissions{CanViewDetail: true}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return domain.SkillRunResult{}, err
	}
	return domain.SkillRunResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

// ListSkillSimilaritySources 列出可用于相似性索引的技能源。
func (r *SkillRepository) ListSkillSimilaritySources() ([]domain.SkillSimilaritySource, error) {
	rows, err := r.db.Query(`
		SELECT s.id, s.name, s.skill_key, s.description,
		       COALESCE((SELECT sv.version FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1), ''),
		       COALESCE((SELECT sv.content_markdown FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1), '')
		FROM skill s
		WHERE s.deleted_at = ''
		ORDER BY s.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.SkillSimilaritySource{}
	for rows.Next() {
		var item domain.SkillSimilaritySource
		if err = rows.Scan(&item.ID, &item.Name, &item.Slug, &item.Description, &item.Version, &item.Body); err != nil {
			return nil, err
		}
		item.BodyPreview = previewText(item.Body, 240)
		items = append(items, item)
	}
	return items, rows.Err()
}

// CreateSkillWithVersion 创建技能及 1.0.0 版本行。
func (r *SkillRepository) CreateSkillWithVersion(input domain.SkillCreateInput) (domain.Skill, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.TrimSpace(input.Slug)
	input.Description = strings.TrimSpace(input.Description)
	input.Body = strings.TrimSpace(input.Body)
	if input.Name == "" || input.Slug == "" || input.Body == "" {
		return domain.Skill{}, errors.New("skill name, slug and body are required")
	}
	now := nowISO()
	skillID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	versionID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
	metadata := struct {
		Tags       []domain.SkillTag `json:"tags"`
		StorageDir string            `json:"storage_dir"`
	}{Tags: input.Tags, StorageDir: input.StorageDir}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return domain.Skill{}, err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return domain.Skill{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(
		`INSERT INTO skill(id, skill_key, name, description, status, enabled, sort_order, config_json, metadata_json, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, 'draft', 0, 0, '', ?, ?, ?, '')`,
		skillID, input.Slug, input.Name, input.Description, string(metadataJSON), now, now,
	); err != nil {
		return domain.Skill{}, err
	}
	if _, err = tx.Exec(
		`INSERT INTO skill_version(id, skill_id, version, status, content_markdown, metadata_json, created_at, updated_at) VALUES (?, ?, '1.0.0', 'pass', ?, ?, ?, ?)`,
		versionID, skillID, input.Body, string(metadataJSON), now, now,
	); err != nil {
		return domain.Skill{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Skill{}, err
	}
	return r.GetSkillByID(skillID)
}

// GetSkillStorageDir 从 metadata 读取技能工作目录配置。
func (r *SkillRepository) GetSkillStorageDir(id string) (string, error) {
	var raw string
	if err := r.db.QueryRow(`SELECT metadata_json FROM skill WHERE id = ? AND deleted_at = ''`, id).Scan(&raw); err != nil {
		return "", err
	}
	var metadata struct {
		StorageDir string `json:"storage_dir"`
	}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return "", err
	}
	if strings.TrimSpace(metadata.StorageDir) == "" {
		return "", errors.New("skill storage directory is not configured")
	}
	return metadata.StorageDir, nil
}

func skillSelectSQL() string {
	return `
		SELECT s.id, s.skill_key, s.name, s.description, s.status, s.enabled, s.config_json, s.metadata_json, s.created_at, s.updated_at,
		       (SELECT sv.id FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1),
		       (SELECT sv.version FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1),
		       (SELECT sv.status FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1),
		       (SELECT sv.created_at FROM skill_version sv WHERE sv.skill_id = s.id ORDER BY sv.created_at DESC LIMIT 1),
		       (SELECT COUNT(1) FROM skill_invocation si WHERE si.skill_id = s.id),
		       (SELECT COALESCE(SUM(CASE WHEN si.status = 'success' THEN 1 ELSE 0 END), 0) FROM skill_invocation si WHERE si.skill_id = s.id),
		       (SELECT COALESCE(SUM(CASE WHEN si.status = 'failure' THEN 1 ELSE 0 END), 0) FROM skill_invocation si WHERE si.skill_id = s.id),
		       (SELECT COUNT(1) FROM skill_invocation si WHERE si.skill_id = s.id AND COALESCE(NULLIF(si.started_at, ''), si.created_at) >= ?),
		       (SELECT AVG(NULLIF(si.duration_ms, 0)) FROM skill_invocation si WHERE si.skill_id = s.id),
		       (SELECT si.agent_id FROM skill_invocation si WHERE si.skill_id = s.id ORDER BY COALESCE(NULLIF(si.started_at, ''), si.created_at) DESC LIMIT 1),
		       (SELECT COALESCE(a.display_name, '') FROM skill_invocation si LEFT JOIN agents a ON a.id = si.agent_id WHERE si.skill_id = s.id ORDER BY COALESCE(NULLIF(si.started_at, ''), si.created_at) DESC LIMIT 1),
		       (SELECT COALESCE(NULLIF(si.started_at, ''), si.created_at) FROM skill_invocation si WHERE si.skill_id = s.id ORDER BY COALESCE(NULLIF(si.started_at, ''), si.created_at) DESC LIMIT 1),
		       (SELECT si.duration_ms FROM skill_invocation si WHERE si.skill_id = s.id ORDER BY COALESCE(NULLIF(si.started_at, ''), si.created_at) DESC LIMIT 1)
		FROM skill s`
}

func skillWhereClause(query domain.SkillListQuery) (string, []any) {
	where := []string{"s.deleted_at = ''"}
	args := []any{}
	if q := strings.TrimSpace(query.Search); q != "" {
		where = append(where, "(LOWER(s.skill_key) LIKE ? OR LOWER(s.name) LIKE ? OR LOWER(s.description) LIKE ?)")
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like, like)
	}
	if query.Tags != "" {
		for _, tag := range strings.Split(query.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			where = append(where, "LOWER(s.metadata_json) LIKE ?")
			args = append(args, "%"+strings.ToLower(tag)+"%")
		}
	}
	if query.Enabled == "true" {
		where = append(where, "s.enabled = 1")
	}
	if query.Enabled == "false" {
		where = append(where, "s.enabled = 0")
	}
	if query.Status != "" {
		if query.Status == "published" {
			where = append(where, "s.status IN ('published', 'active')")
		} else {
			where = append(where, "s.status = ?")
			args = append(args, query.Status)
		}
	}
	return strings.Join(where, " AND "), args
}

func scanSkills(rows *sql.Rows) ([]domain.Skill, error) {
	items := []domain.Skill{}
	for rows.Next() {
		var item domain.Skill
		var enabled bool
		var configJSON string
		var metadataJSON string
		var versionID, version, validationStatus, publishedAt sql.NullString
		var avgDuration sql.NullFloat64
		var lastAgentID, lastAgentName, lastInvokedAt sql.NullString
		var lastDuration sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.Slug, &item.Name, &item.Description, &item.Status, &enabled, &configJSON, &metadataJSON, &item.CreatedAt, &item.UpdatedAt,
			&versionID, &version, &validationStatus, &publishedAt,
			&item.InvokeCount, &item.SuccessCount, &item.FailureCount, &item.UsageCount7d, &avgDuration,
			&lastAgentID, &lastAgentName, &lastInvokedAt, &lastDuration,
		); err != nil {
			return nil, err
		}
		item.Status = normalizeSkillStatus(item.Status)
		item.Enabled = enabled
		item.Tags = parseSkillTags(metadataJSON)
		if len(item.Tags) == 0 {
			item.Tags = parseSkillTags(configJSON)
		}
		if versionID.Valid {
			status := validationStatus.String
			if status == "" || status == "active" {
				status = "pass"
			}
			item.CurrentVersion = &domain.SkillVersionSummary{
				ID:               versionID.String,
				Version:          version.String,
				ValidationStatus: status,
				PublishedAt:      publishedAt.String,
			}
		}
		if avgDuration.Valid {
			v := avgDuration.Float64
			item.AvgDurationMS = &v
		}
		if lastDuration.Valid {
			v := int(lastDuration.Int64)
			item.LastDurationMS = &v
		}
		item.LastAgentID = lastAgentID.String
		item.LastAgentDisplayName = lastAgentName.String
		item.LastInvokedAt = lastInvokedAt.String
		item.Permissions = domain.SkillPermissions{
			CanEdit:          true,
			CanDelete:        true,
			CanToggleEnabled: true,
			CanDuplicate:     true,
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func normalizeSkillStatus(status string) string {
	switch status {
	case "", "active", "published":
		return "published"
	case "inactive":
		return "archived"
	default:
		return status
	}
}

func parseSkillTags(raw string) []domain.SkillTag {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []domain.SkillTag{}
	}
	var envelope struct {
		Tags []domain.SkillTag `json:"tags"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil && len(envelope.Tags) > 0 {
		return normalizeSkillTags(envelope.Tags)
	}
	var tags []domain.SkillTag
	if err := json.Unmarshal([]byte(raw), &tags); err == nil {
		return normalizeSkillTags(tags)
	}
	return []domain.SkillTag{}
}

func normalizeSkillTags(tags []domain.SkillTag) []domain.SkillTag {
	result := []domain.SkillTag{}
	seen := map[string]bool{}
	for _, tag := range tags {
		tag.Name = strings.TrimSpace(tag.Name)
		if tag.Name == "" || seen[strings.ToLower(tag.Name)] {
			continue
		}
		if tag.Source == "" {
			tag.Source = "user"
		}
		seen[strings.ToLower(tag.Name)] = true
		result = append(result, tag)
	}
	return result
}
