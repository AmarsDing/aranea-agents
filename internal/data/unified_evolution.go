package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type UnifiedEvolutionRepo struct {
	data *Data
	lg   loggateway.Logger
}

// NOTE: 本 repo 使用裸 SQL 而非 Ent ORM，与 skill_evolution.go (skillProposalRepo) 保持一致。
// 原因：unified_evolution_suggestions 表尚无 Ent schema，且部分查询（如动态 WHERE 拼接、
// RowsAffected 批量更新）用 Ent 表达不便。待 Ent schema 补齐后可迁移。

var (
	_ biz.UnifiedEvolutionReader = (*UnifiedEvolutionRepo)(nil)
	_ biz.UnifiedEvolutionWriter = (*UnifiedEvolutionRepo)(nil)
)

func NewUnifiedEvolutionRepo(data *Data, lg loggateway.Logger) *UnifiedEvolutionRepo {
	return &UnifiedEvolutionRepo{data: data, lg: lg}
}

func (r *UnifiedEvolutionRepo) Create(ctx context.Context, suggestion biz.UnifiedEvolutionSuggestion) error {
	q := `INSERT INTO unified_evolution_suggestions
		(id, target_type, target_id, action_type, trigger_source, trigger_reason,
		 status, priority, draft_body, draft_name, merge_target_id,
		 lifecycle_status, sandbox_passed, sandbox_result, metadata,
		 created_at, approved_by, applied_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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
		suggestion.SandboxPassed,
		sandboxResultStr,
		metadataStr,
		suggestion.CreatedAt.UTC().Format(time.RFC3339),
		suggestion.ApprovedBy,
		appliedAt,
	)
	if err != nil {
		return kerrors.InternalServer("UNIFIED_EVO", "insert unified evolution suggestion: "+err.Error())
	}
	return nil
}

func (r *UnifiedEvolutionRepo) HasPendingForTarget(ctx context.Context, targetType string, targetID string) (bool, error) {
	q := `SELECT COUNT(*) FROM unified_evolution_suggestions
	      WHERE target_type = ? AND target_id = ? AND status = 'pending'`
	var count int
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), q, []any{targetType, targetID}, &count)
	if err != nil {
		return false, kerrors.InternalServer("UNIFIED_EVO", "check pending: "+err.Error())
	}
	return count > 0, nil
}

func (r *UnifiedEvolutionRepo) GetLatestByTarget(ctx context.Context, targetType string, targetID string) (*biz.UnifiedEvolutionSuggestion, error) {
	q := `SELECT id, target_type, target_id, action_type, trigger_source, trigger_reason,
	             status, priority, draft_body, draft_name, merge_target_id,
	             lifecycle_status, sandbox_passed, sandbox_result, metadata,
	             created_at, approved_by, applied_at
	      FROM unified_evolution_suggestions
	      WHERE target_type = ? AND target_id = ?
	      ORDER BY created_at DESC LIMIT 1`
	s, err := r.scanOne(ctx, q, targetType, targetID)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *UnifiedEvolutionRepo) ListByTarget(ctx context.Context, targetType string, targetID string, status string, limit, offset int) ([]biz.UnifiedEvolutionSuggestion, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id, target_type, target_id, action_type, trigger_source, trigger_reason,
	             status, priority, draft_body, draft_name, merge_target_id,
	             lifecycle_status, sandbox_passed, sandbox_result, metadata,
	             created_at, approved_by, applied_at
	      FROM unified_evolution_suggestions
	      WHERE target_type = ? AND target_id = ?`
	args := []any{targetType, targetID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, kerrors.InternalServer("UNIFIED_EVO", "list by target: "+err.Error())
	}
	defer rows.Close()

	var result []biz.UnifiedEvolutionSuggestion
	for rows.Next() {
		s, err := scanUnifiedEvolutionRow(rows)
		if err != nil {
			return nil, kerrors.InternalServer("UNIFIED_EVO", "scan row: "+err.Error())
		}
		result = append(result, *s)
	}
	return result, nil
}

func (r *UnifiedEvolutionRepo) CountByTarget(ctx context.Context, targetType string, targetID string, status string) (int, error) {
	q := `SELECT COUNT(*) FROM unified_evolution_suggestions
	      WHERE target_type = ? AND target_id = ?`
	args := []any{targetType, targetID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	var count int
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), q, args, &count)
	if err != nil {
		return 0, kerrors.InternalServer("UNIFIED_EVO", "count by target: "+err.Error())
	}
	return count, nil
}

func (r *UnifiedEvolutionRepo) GetByID(ctx context.Context, id string) (*biz.UnifiedEvolutionSuggestion, error) {
	q := `SELECT id, target_type, target_id, action_type, trigger_source, trigger_reason,
	             status, priority, draft_body, draft_name, merge_target_id,
	             lifecycle_status, sandbox_passed, sandbox_result, metadata,
	             created_at, approved_by, applied_at
	      FROM unified_evolution_suggestions WHERE id = ?`
	s, err := r.scanOne(ctx, q, id)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *UnifiedEvolutionRepo) UpdateStatus(ctx context.Context, id string, status string, actor string, reason string) error {
	q := `UPDATE unified_evolution_suggestions SET status = ?`
	args := []any{status}
	switch status {
	case "approved":
		q += `, approved_by = ?`
		args = append(args, actor)
	case "rejected":
		// reason stored in metadata if needed; actor recorded
		q += `, approved_by = ?`
		args = append(args, actor)
	case "applied":
		q += `, applied_at = ?`
		args = append(args, time.Now().UTC().Format(time.RFC3339))
	}
	q += ` WHERE id = ?`
	args = append(args, id)

	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, args...)
	if err != nil {
		return kerrors.InternalServer("UNIFIED_EVO", "update status: "+err.Error())
	}
	return nil
}

func (r *UnifiedEvolutionRepo) UpdateDraftBody(ctx context.Context, id string, draftBody string) error {
	q := `UPDATE unified_evolution_suggestions SET draft_body = ? WHERE id = ?`
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, draftBody, id)
	if err != nil {
		return kerrors.InternalServer("UNIFIED_EVO", "update draft body: "+err.Error())
	}
	return nil
}

func (r *UnifiedEvolutionRepo) UpdateLifecycleStatus(ctx context.Context, id string, lifecycleStatus string) error {
	q := `UPDATE unified_evolution_suggestions SET lifecycle_status = ? WHERE id = ?`
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, lifecycleStatus, id)
	if err != nil {
		return kerrors.InternalServer("UNIFIED_EVO", "update lifecycle status: "+err.Error())
	}
	return nil
}

func (r *UnifiedEvolutionRepo) UpdateSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error {
	q := `UPDATE unified_evolution_suggestions SET sandbox_passed = ?, sandbox_result = ? WHERE id = ?`
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
		return kerrors.InternalServer("UNIFIED_EVO", "update sandbox result: "+err.Error())
	}
	return nil
}

func (r *UnifiedEvolutionRepo) ExpireOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	q := `UPDATE unified_evolution_suggestions SET status = 'expired'
	      WHERE status = 'pending' AND created_at < ?`
	result, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, kerrors.InternalServer("UNIFIED_EVO", "expire older than: "+err.Error())
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

// ── Scan helpers ─────────────────────────────────────────────────────────────

func (r *UnifiedEvolutionRepo) scanOne(ctx context.Context, q string, args ...any) (*biz.UnifiedEvolutionSuggestion, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, kerrors.InternalServer("UNIFIED_EVO", "query: "+err.Error())
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	s, scanErr := scanUnifiedEvolutionRow(rows)
	if scanErr != nil {
		return nil, kerrors.InternalServer("UNIFIED_EVO", "scan: "+scanErr.Error())
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
		return nil, fmt.Errorf("scan unified evolution suggestion: %w", err)
	}

	s.SandboxPassed = sandboxPassed != 0
	if parsedAt, parseErr := time.Parse(time.RFC3339, createdAt); parseErr != nil {
		return nil, fmt.Errorf("parse created_at %q: %w", createdAt, parseErr)
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
