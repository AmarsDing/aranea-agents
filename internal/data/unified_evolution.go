package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type UnifiedEvolutionRepo struct {
	data *Data
	lg   loggateway.Logger
}

// NOTE: 本 repo 使用裸 SQL 而非 Ent ORM。
// 原因：unified_evolution_suggestions 表无 Ent schema（raw SQL DDL 管理，见迁移
// 20260706/20261111），且部分查询（如动态 WHERE 拼接、方言感知 JSON 路径表达式、
// RowsAffected 批量更新）用 Ent 表达不便。

var (
	_ biz.UnifiedEvolutionCheckReader      = (*UnifiedEvolutionRepo)(nil)
	_ biz.UnifiedEvolutionQueryReader      = (*UnifiedEvolutionRepo)(nil)
	_ biz.UnifiedEvolutionPatternReader    = (*UnifiedEvolutionRepo)(nil)
	_ biz.UnifiedEvolutionMutationWriter   = (*UnifiedEvolutionRepo)(nil)
	_ biz.UnifiedEvolutionExpirationWriter = (*UnifiedEvolutionRepo)(nil)
	_ biz.UnifiedEvolutionReader           = (*UnifiedEvolutionRepo)(nil)
	_ biz.UnifiedEvolutionWriter           = (*UnifiedEvolutionRepo)(nil)
	_ biz.UnifiedEvolutionDiversityReader  = (*UnifiedEvolutionRepo)(nil)
)

func NewUnifiedEvolutionRepo(data *Data, lg loggateway.Logger) *UnifiedEvolutionRepo {
	return &UnifiedEvolutionRepo{data: data, lg: lg}
}

func (r *UnifiedEvolutionRepo) Create(ctx context.Context, suggestion biz.UnifiedEvolutionSuggestion) error {
	q := r.data.Dialect().RenumberPlaceholders(`INSERT INTO unified_evolution_suggestions
		(id, target_type, target_id, action_type, trigger_source, trigger_reason,
		 status, priority, draft_body, draft_name, merge_target_id,
		 lifecycle_status, sandbox_passed, sandbox_result, metadata,
		 created_at, approved_by, applied_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	var sandboxResultStr *string
	if suggestion.SandboxResult != nil {
		s := string(suggestion.SandboxResult)
		sandboxResultStr = &s
	}
	var metadataStr *string
	if suggestion.Metadata != nil {
		s := string(suggestion.Metadata)
		metadataStr = &s
	}
	var appliedAt *string
	if suggestion.AppliedAt != nil {
		s := suggestion.AppliedAt.UTC().Format(time.RFC3339)
		appliedAt = &s
	}
	// bool→int：列类型为 INTEGER（与 UpdateSandboxResult/scan 一致），
	// 直接传 bool 在 Postgres 下报 22P02。
	var sandboxPassed int
	if suggestion.SandboxPassed {
		sandboxPassed = 1
	}

	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q,
		suggestion.ID,
		string(suggestion.TargetType),
		suggestion.TargetID,
		string(suggestion.ActionType),
		suggestion.TriggerSource,
		suggestion.TriggerReason,
		suggestion.Status,
		suggestion.Priority,
		suggestion.DraftBody,
		suggestion.DraftName,
		suggestion.MergeTargetID,
		suggestion.LifecycleStatus,
		sandboxPassed,
		sandboxResultStr,
		metadataStr,
		suggestion.CreatedAt.UTC().Format(time.RFC3339),
		suggestion.ApprovedBy,
		appliedAt,
	)
	if err != nil {
		return entErrToBizErr(err, "UNIFIED_EVO")
	}
	return nil
}

func (r *UnifiedEvolutionRepo) HasPendingForTarget(ctx context.Context, targetType string, targetID string) (bool, error) {
	q := r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(*) FROM unified_evolution_suggestions
	      WHERE target_type = ? AND target_id = ? AND status = 'pending'`)
	var count int
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), q, []any{targetType, targetID}, &count)
	if err != nil {
		return false, entErrToBizErr(err, "UNIFIED_EVO")
	}
	return count > 0, nil
}

func (r *UnifiedEvolutionRepo) GetLatestByTarget(ctx context.Context, targetType string, targetID string) (*biz.UnifiedEvolutionSuggestion, error) {
	q := r.data.Dialect().RenumberPlaceholders(`SELECT id, target_type, target_id, action_type, trigger_source, trigger_reason,
	             status, priority, draft_body, draft_name, merge_target_id,
	             lifecycle_status, sandbox_passed, sandbox_result, metadata,
	             created_at, approved_by, applied_at
	      FROM unified_evolution_suggestions
	      WHERE target_type = ? AND target_id = ?
	      ORDER BY created_at DESC LIMIT 1`)
	s, err := r.scanOne(ctx, q, targetType, targetID)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *UnifiedEvolutionRepo) GetLatestByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string) (*biz.UnifiedEvolutionSuggestion, error) {
	q := r.data.Dialect().RenumberPlaceholders(`SELECT id, target_type, target_id, action_type, trigger_source, trigger_reason,
	             status, priority, draft_body, draft_name, merge_target_id,
	             lifecycle_status, sandbox_passed, sandbox_result, metadata,
	             created_at, approved_by, applied_at
	      FROM unified_evolution_suggestions
	      WHERE target_type = ? AND target_id = ? AND action_type = ?
	      ORDER BY created_at DESC LIMIT 1`)
	s, err := r.scanOne(ctx, q, targetType, targetID, actionType)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *UnifiedEvolutionRepo) ListByTarget(ctx context.Context, targetType string, targetID string, status string, limit, offset int) ([]biz.UnifiedEvolutionSuggestion, error) {
	return r.listByTarget(ctx, targetType, targetID, "", status, limit, offset)
}

func (r *UnifiedEvolutionRepo) ListByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string, status string, limit, offset int) ([]biz.UnifiedEvolutionSuggestion, error) {
	return r.listByTarget(ctx, targetType, targetID, actionType, status, limit, offset)
}

func (r *UnifiedEvolutionRepo) listByTarget(ctx context.Context, targetType string, targetID string, actionType string, status string, limit, offset int) ([]biz.UnifiedEvolutionSuggestion, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id, target_type, target_id, action_type, trigger_source, trigger_reason,
	             status, priority, draft_body, draft_name, merge_target_id,
	             lifecycle_status, sandbox_passed, sandbox_result, metadata,
	             created_at, approved_by, applied_at
	      FROM unified_evolution_suggestions
	      WHERE target_type = ?`
	args := []any{targetType}
	// Empty targetID is a wildcard (mirrors the legacy L1 ListByAgent semantics:
	// the service layer passes req.GetAgentId() straight through).
	if targetID != "" {
		q += ` AND target_id = ?`
		args = append(args, targetID)
	}
	if actionType != "" {
		q += ` AND action_type = ?`
		args = append(args, actionType)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "UNIFIED_EVO")
	}
	defer rows.Close()

	var result []biz.UnifiedEvolutionSuggestion
	for rows.Next() {
		s, err := scanUnifiedEvolutionRow(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "UNIFIED_EVO")
		}
		result = append(result, *s)
	}
	return result, nil
}

func (r *UnifiedEvolutionRepo) CountByTarget(ctx context.Context, targetType string, targetID string, status string) (int, error) {
	return r.countByTarget(ctx, targetType, targetID, "", status)
}

func (r *UnifiedEvolutionRepo) CountByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string, status string) (int, error) {
	return r.countByTarget(ctx, targetType, targetID, actionType, status)
}

func (r *UnifiedEvolutionRepo) countByTarget(ctx context.Context, targetType string, targetID string, actionType string, status string) (int, error) {
	q := `SELECT COUNT(*) FROM unified_evolution_suggestions
	      WHERE target_type = ?`
	args := []any{targetType}
	// Empty targetID is a wildcard (mirrors the legacy L1 CountByAgent semantics).
	if targetID != "" {
		q += ` AND target_id = ?`
		args = append(args, targetID)
	}
	if actionType != "" {
		q += ` AND action_type = ?`
		args = append(args, actionType)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	var count int
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders(q), args, &count)
	if err != nil {
		return 0, entErrToBizErr(err, "UNIFIED_EVO")
	}
	return count, nil
}

// GetLatestByPatternHash returns the newest L1 skill-creation proposal for the
// given agent whose metadata.pattern_hash matches. Mirrors the legacy
// skill_proposals dedup lookup (skillProposalRepo.GetByPatternHash): no status
// filter — the caller decides how to interpret the latest row's status.
// Returns (nil, nil) when no row matches.
func (r *UnifiedEvolutionRepo) GetLatestByPatternHash(ctx context.Context, agentID string, patternHash string) (*biz.UnifiedEvolutionSuggestion, error) {
	hashExpr := r.data.Dialect().JSONExtractPath("metadata", biz.EvoMetaPatternHash)
	q := `SELECT id, target_type, target_id, action_type, trigger_source, trigger_reason,
	             status, priority, draft_body, draft_name, merge_target_id,
	             lifecycle_status, sandbox_passed, sandbox_result, metadata,
	             created_at, approved_by, applied_at
	      FROM unified_evolution_suggestions
	      WHERE target_type = ? AND target_id = ? AND action_type = ?
	        AND ` + hashExpr + ` = ?
	      ORDER BY created_at DESC LIMIT 1`
	s, err := r.scanOne(ctx, r.data.Dialect().RenumberPlaceholders(q),
		string(biz.EvolutionTargetAgent), agentID, string(biz.EvolutionActionCreate), patternHash)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *UnifiedEvolutionRepo) GetByID(ctx context.Context, id string) (*biz.UnifiedEvolutionSuggestion, error) {
	q := r.data.Dialect().RenumberPlaceholders(`SELECT id, target_type, target_id, action_type, trigger_source, trigger_reason,
	             status, priority, draft_body, draft_name, merge_target_id,
	             lifecycle_status, sandbox_passed, sandbox_result, metadata,
	             created_at, approved_by, applied_at
	      FROM unified_evolution_suggestions WHERE id = ?`)
	s, err := r.scanOne(ctx, q, id)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateStatus transitions the suggestion status and merges the legacy
// status-related fields into metadata so the service-layer views can
// reconstruct legacy proto messages (A6):
//
//	approved:  approved_by column + metadata.{approved_at, resolved_at}     (L1/L2)
//	rejected:  approved_by column + metadata.{rejected_by, rejection_reason, resolved_at} (L1/L2)
//	applied:   applied_at column                                            (L2/L3)
//
// Metadata merge uses the dialect-aware JSON helpers (JSONBBase + JSONSetMulti)
// so it works on both SQLite TEXT and Postgres jsonb columns.
func (r *UnifiedEvolutionRepo) UpdateStatus(ctx context.Context, id string, status string, actor string, reason string) error {
	d := r.data.Dialect()
	now := time.Now().UTC().Format(time.RFC3339)
	setClauses := []string{"status = ?"}
	args := []any{status}
	switch status {
	case string(biz.UnifiedEvolutionStateApproved):
		setClauses = append(setClauses, "approved_by = ?")
		args = append(args, actor)
		setClauses = append(setClauses, "metadata = "+d.JSONSetMulti(d.JSONBBase("metadata"),
			[2]string{biz.EvoMetaApprovedAt, "?"},
			[2]string{biz.EvoMetaResolvedAt, "?"}))
		args = append(args, now, now)
	case string(biz.UnifiedEvolutionStateRejected):
		setClauses = append(setClauses, "approved_by = ?")
		args = append(args, actor)
		setClauses = append(setClauses, "metadata = "+d.JSONSetMulti(d.JSONBBase("metadata"),
			[2]string{biz.EvoMetaRejectedBy, "?"},
			[2]string{biz.EvoMetaRejectionReason, "?"},
			[2]string{biz.EvoMetaResolvedAt, "?"}))
		args = append(args, actor, reason, now)
	case string(biz.UnifiedEvolutionStateApplied):
		setClauses = append(setClauses, "applied_at = ?")
		args = append(args, now)
	}
	q := `UPDATE unified_evolution_suggestions SET ` + strings.Join(setClauses, ", ") + ` WHERE id = ?`
	args = append(args, id)

	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, d.RenumberPlaceholders(q), args...)
	if err != nil {
		return entErrToBizErr(err, "UNIFIED_EVO")
	}
	return nil
}

// UpdateMetadataKey sets a single JSON-string key in the metadata column,
// preserving all other keys (A6: e.g. EvoMetaPreApplySnapshot).
func (r *UnifiedEvolutionRepo) UpdateMetadataKey(ctx context.Context, id string, key string, value string) error {
	d := r.data.Dialect()
	q := `UPDATE unified_evolution_suggestions SET metadata = ` +
		d.JSONSetMulti(d.JSONBBase("metadata"), [2]string{key, "?"}) + ` WHERE id = ?`
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, d.RenumberPlaceholders(q), value, id)
	if err != nil {
		return entErrToBizErr(err, "UNIFIED_EVO")
	}
	return nil
}

func (r *UnifiedEvolutionRepo) UpdateDraftBody(ctx context.Context, id string, draftBody string) error {
	q := r.data.Dialect().RenumberPlaceholders(`UPDATE unified_evolution_suggestions SET draft_body = ? WHERE id = ?`)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, draftBody, id)
	if err != nil {
		return entErrToBizErr(err, "UNIFIED_EVO")
	}
	return nil
}

func (r *UnifiedEvolutionRepo) UpdateLifecycleStatus(ctx context.Context, id string, lifecycleStatus string) error {
	q := r.data.Dialect().RenumberPlaceholders(`UPDATE unified_evolution_suggestions SET lifecycle_status = ? WHERE id = ?`)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, lifecycleStatus, id)
	if err != nil {
		return entErrToBizErr(err, "UNIFIED_EVO")
	}
	return nil
}

func (r *UnifiedEvolutionRepo) UpdateSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error {
	q := r.data.Dialect().RenumberPlaceholders(`UPDATE unified_evolution_suggestions SET sandbox_passed = ?, sandbox_result = ? WHERE id = ?`)
	var sandboxPassed int
	if passed {
		sandboxPassed = 1
	}
	var resultStr *string
	if result != nil {
		s := string(result)
		resultStr = &s
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, sandboxPassed, resultStr, id)
	if err != nil {
		return entErrToBizErr(err, "UNIFIED_EVO")
	}
	return nil
}

func (r *UnifiedEvolutionRepo) ExpireOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	q := r.data.Dialect().RenumberPlaceholders(`UPDATE unified_evolution_suggestions SET status = 'expired'
	      WHERE status = 'pending' AND created_at < ?`)
	result, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, entErrToBizErr(err, "UNIFIED_EVO")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		r.lg.Warn("ExpireOlderThan: RowsAffected failed",
			loggateway.StepID("unified_evolution.expire"),
			loggateway.Err(err))
		return 0, nil
	}
	return int(affected), nil
}

// defaultDiversityTopTools 是 GetDiversityOverview 在 topTools <= 0 时的默认截断。
const defaultDiversityTopTools = 5

// GetDiversityOverview 聚合 since 以来的建议：按 trigger_source 分桶统计
// count + MAX(created_at)，并统计每桶 dims.tools 频次 TopN。
//
// 分桶用纯 SQL GROUP BY；工具频次在 Go 侧解析 metadata.dims.tools 统计——
// 建议表是人工审阅量级，且单一代码路径避免 jsonb_array_elements/json_each
// 双方言分叉。metadata 缺失/无 dims/解析失败的行被容忍（best-effort 观测）。
func (r *UnifiedEvolutionRepo) GetDiversityOverview(ctx context.Context, since time.Time, topTools int) ([]biz.EvolutionDiversitySourceStat, error) {
	if topTools <= 0 {
		topTools = defaultDiversityTopTools
	}
	sinceStr := since.UTC().Format(time.RFC3339)

	bucketQ := r.data.Dialect().RenumberPlaceholders(`SELECT trigger_source, COUNT(*), MAX(created_at)
		FROM unified_evolution_suggestions
		WHERE created_at >= ?
		GROUP BY trigger_source
		ORDER BY COUNT(*) DESC, trigger_source ASC`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, bucketQ, sinceStr)
	if err != nil {
		return nil, entErrToBizErr(err, "UNIFIED_EVO")
	}
	var stats []biz.EvolutionDiversitySourceStat
	for rows.Next() {
		var st biz.EvolutionDiversitySourceStat
		var latest string
		if err := rows.Scan(&st.TriggerSource, &st.Count, &latest); err != nil {
			rows.Close()
			return nil, entErrToBizErr(err, "UNIFIED_EVO")
		}
		if t, perr := time.Parse(time.RFC3339, latest); perr == nil {
			st.LatestAt = t
		}
		stats = append(stats, st)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "UNIFIED_EVO")
	}
	if len(stats) == 0 {
		return stats, nil
	}

	toolQ := r.data.Dialect().RenumberPlaceholders(`SELECT trigger_source, metadata
		FROM unified_evolution_suggestions
		WHERE created_at >= ? AND metadata IS NOT NULL AND metadata <> ''`)
	mrows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, toolQ, sinceStr)
	if err != nil {
		return nil, entErrToBizErr(err, "UNIFIED_EVO")
	}
	freq := map[string]map[string]int{}
	for mrows.Next() {
		var source, metadataStr string
		if err := mrows.Scan(&source, &metadataStr); err != nil {
			mrows.Close()
			return nil, entErrToBizErr(err, "UNIFIED_EVO")
		}
		var meta struct {
			Dims *biz.EvolutionDims `json:"dims"`
		}
		if json.Unmarshal([]byte(metadataStr), &meta) != nil || meta.Dims == nil {
			continue
		}
		for _, tool := range meta.Dims.Tools {
			tool = strings.TrimSpace(tool)
			if tool == "" {
				continue
			}
			fm := freq[source]
			if fm == nil {
				fm = map[string]int{}
				freq[source] = fm
			}
			fm[tool]++
		}
	}
	mrows.Close()
	if err := mrows.Err(); err != nil {
		return nil, entErrToBizErr(err, "UNIFIED_EVO")
	}

	for i := range stats {
		fm := freq[stats[i].TriggerSource]
		if len(fm) == 0 {
			continue
		}
		tools := make([]string, 0, len(fm))
		for tool := range fm {
			tools = append(tools, tool)
		}
		// 频次降序；同频次按工具名字典序，保证聚合结果稳定可断言。
		sort.Slice(tools, func(a, b int) bool {
			if fm[tools[a]] != fm[tools[b]] {
				return fm[tools[a]] > fm[tools[b]]
			}
			return tools[a] < tools[b]
		})
		if len(tools) > topTools {
			tools = tools[:topTools]
		}
		stats[i].TopTools = tools
	}
	return stats, nil
}

// ── Scan helpers ─────────────────────────────────────────────────────────────

func (r *UnifiedEvolutionRepo) scanOne(ctx context.Context, q string, args ...any) (*biz.UnifiedEvolutionSuggestion, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "UNIFIED_EVO")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	s, scanErr := scanUnifiedEvolutionRow(rows)
	if scanErr != nil {
		return nil, entErrToBizErr(scanErr, "UNIFIED_EVO")
	}
	return s, nil
}

func scanUnifiedEvolutionRow(rows *sql.Rows) (*biz.UnifiedEvolutionSuggestion, error) {
	var s biz.UnifiedEvolutionSuggestion
	var createdAt string
	var appliedAt *string
	var sandboxResultStr *string
	var metadataStr *string
	var sandboxPassed int

	err := rows.Scan(
		&s.ID,
		&s.TargetType,
		&s.TargetID,
		&s.ActionType,
		&s.TriggerSource,
		&s.TriggerReason,
		&s.Status,
		&s.Priority,
		&s.DraftBody,
		&s.DraftName,
		&s.MergeTargetID,
		&s.LifecycleStatus,
		&sandboxPassed,
		&sandboxResultStr,
		&metadataStr,
		&createdAt,
		&s.ApprovedBy,
		&appliedAt,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "UNIFIED_EVO")
	}

	s.SandboxPassed = sandboxPassed != 0
	if parsedAt, parseErr := time.Parse(time.RFC3339, createdAt); parseErr != nil {
		return nil, entErrToBizErr(parseErr, "UNIFIED_EVO")
	} else {
		s.CreatedAt = parsedAt
	}
	if appliedAt != nil && *appliedAt != "" {
		t, parseErr := time.Parse(time.RFC3339, *appliedAt)
		if parseErr == nil {
			s.AppliedAt = &t
		}
	}
	if sandboxResultStr != nil && *sandboxResultStr != "" {
		s.SandboxResult = json.RawMessage(*sandboxResultStr)
	}
	if metadataStr != nil && *metadataStr != "" {
		s.Metadata = json.RawMessage(*metadataStr)
	}
	return &s, nil
}
