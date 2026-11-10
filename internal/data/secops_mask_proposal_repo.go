package data

// 脱敏提案台账 + 闸门事务（E 阶段 W2b D6 V4，DDL 迁移 20261327
// mask_proposals/mask_samples）。查询走 RWDB().ReadDB/WriteDB（事务感知）；
// 审批/激活为 ExecInTx 单事务：状态 CAS + 影子规则写 + 审计闭包一起提交
// 或一起回滚（wb approve 同事务语义，aranea 把 activate 也纳入）。

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

type secopsMaskProposalRepo struct {
	data *Data
}

var (
	_ biz.SecopsMaskProposalRepo  = (*secopsMaskProposalRepo)(nil)
	_ biz.SecopsMaskRuleAdminRepo = (*secopsMaskRepo)(nil)
)

// NewSecopsMaskProposalRepo 装配脱敏提案台账。
func NewSecopsMaskProposalRepo(d *Data) biz.SecopsMaskProposalRepo {
	if d == nil {
		return nil
	}
	return &secopsMaskProposalRepo{data: d}
}

// NewSecopsMaskRuleAdminRepo 装配规则管理读口（提案面 ListMaskRules 用，
// 全量含 disabled/shadow；与运行面只读窄口共用同一实现）。
func NewSecopsMaskRuleAdminRepo(d *Data) biz.SecopsMaskRuleAdminRepo {
	if d == nil {
		return nil
	}
	return &secopsMaskRepo{data: d}
}

// ListMaskRules 全量规则（id ASC；含 disabled/shadow/灰度）。
func (r *secopsMaskRepo) ListMaskRules(ctx context.Context) ([]biz.MaskRule, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, name, entity, pattern, wordlist_json, operator, enabled, shadow, rollout_pct, version FROM mask_rules ORDER BY id ASC`)
	if err != nil {
		return nil, entErrToBizErr(err, "SECOPS")
	}
	defer rows.Close()
	out := make([]biz.MaskRule, 0)
	for rows.Next() {
		var rule biz.MaskRule
		var wordlistJSON string
		var enabled, shadow int
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.Entity, &rule.Pattern, &wordlistJSON,
			&rule.Operator, &enabled, &shadow, &rule.RolloutPct, &rule.Version); err != nil {
			return nil, entErrToBizErr(err, "SECOPS")
		}
		rule.Enabled = enabled != 0
		rule.Shadow = shadow != 0
		if wordlistJSON != "" {
			// 脏 JSON：词表作废但整行不丢（与 ListEnabledMaskRules 同口径）。
			var words []string
			if err := json.Unmarshal([]byte(wordlistJSON), &words); err == nil {
				rule.Wordlist = words
			}
		}
		out = append(out, rule)
	}
	return out, entErrToBizErr(rows.Err(), "SECOPS")
}

// maskProposalColumns 提案行全列（读/扫列序唯一来源）。
const maskProposalColumns = `id, name, entity, pattern, wordlist_json, operator, source, status,
	sample_recall, sample_detail_json, static_findings_json, created_by, reviewed_by,
	reviewed_at, created_at, updated_at`

// ListProposals 分页列提案（status 空 = 全部；created_at DESC, id DESC 稳定序）。
func (r *secopsMaskProposalRepo) ListProposals(ctx context.Context, status string, limit, offset int) ([]biz.MaskProposal, int64, error) {
	where := ""
	args := make([]any, 0, 2)
	if strings.TrimSpace(status) != "" {
		where = " WHERE status = ?"
		args = append(args, status)
	}
	var total int64
	countRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(*) FROM mask_proposals`+where), args...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "SECOPS")
	}
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			countRows.Close()
			return nil, 0, entErrToBizErr(err, "SECOPS")
		}
	}
	if err := countRows.Err(); err != nil {
		countRows.Close()
		return nil, 0, entErrToBizErr(err, "SECOPS")
	}
	countRows.Close()
	query := r.data.Dialect().RenumberPlaceholders(
		`SELECT ` + maskProposalColumns + ` FROM mask_proposals` + where +
			` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`)
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "SECOPS")
	}
	defer rows.Close()
	out := make([]biz.MaskProposal, 0, limit)
	for rows.Next() {
		p, err := scanMaskProposal(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, entErrToBizErr(err, "SECOPS")
	}
	return out, total, nil
}

// GetProposal 读单条；不存在 → NotFound。
func (r *secopsMaskProposalRepo) GetProposal(ctx context.Context, id string) (*biz.MaskProposal, error) {
	return r.queryProposal(ctx, id)
}

// CreateProposal 插入提案（status=proposed；AI/手工同等）。
func (r *secopsMaskProposalRepo) CreateProposal(ctx context.Context, p biz.MaskProposal) error {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return apierror.Internal(apierror.DomainAdmin, "repository unavailable")
	}
	created := p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	if p.CreatedAt.IsZero() {
		created = nowRFC3339()
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO mask_proposals
			(id, name, entity, pattern, wordlist_json, operator, source, status, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		p.ID, p.Name, p.Entity, p.Pattern, p.WordlistJSON, p.Operator, p.Source, p.Status, p.CreatedBy, created, created)
	return entErrToBizErr(err, "SECOPS")
}

// ListSamplesByEntity 列同 entity 样本集（id ASC 稳定序）。
func (r *secopsMaskProposalRepo) ListSamplesByEntity(ctx context.Context, entity string) ([]biz.MaskSample, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT id, entity, text, positive FROM mask_samples WHERE entity = ? ORDER BY id ASC`), entity)
	if err != nil {
		return nil, entErrToBizErr(err, "SECOPS")
	}
	defer rows.Close()
	out := make([]biz.MaskSample, 0)
	for rows.Next() {
		var s biz.MaskSample
		var positive int
		if err := rows.Scan(&s.ID, &s.Entity, &s.Text, &positive); err != nil {
			return nil, entErrToBizErr(err, "SECOPS")
		}
		s.Positive = positive != 0
		out = append(out, s)
	}
	return out, entErrToBizErr(rows.Err(), "SECOPS")
}

// UpdateGates 闸门2/3 CAS 迁移（WHERE id AND status=fromStatus）。
func (r *secopsMaskProposalRepo) UpdateGates(ctx context.Context, id, fromStatus, toStatus string, recall float64, sampleDetailJSON, findingsJSON string) (bool, error) {
	if r.data.RWDB().WriteDB(ctx) == nil {
		return false, apierror.Internal(apierror.DomainAdmin, "repository unavailable")
	}
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE mask_proposals
			SET status = ?, sample_recall = ?, sample_detail_json = ?, static_findings_json = ?, updated_at = ?
			WHERE id = ? AND status = ?`),
		toStatus, recall, sampleDetailJSON, findingsJSON, nowRFC3339(), id, fromStatus)
	if err != nil {
		return false, entErrToBizErr(err, "SECOPS")
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, entErrToBizErr(err, "SECOPS")
	}
	return affected == 1, nil
}

// ReviewProposal 审批事务：锁谓（CAS）状态迁移 + 影子规则写 + 审计闭包。
//
//	approve=true → 插 mask_rules（id=ruleID, shadow=1, rollout_pct=100），
//	提案 → shadow；approve=false → 提案 → rejected。
//
// 行内再校验 pending_approval 与不可自批（biz 已前置校验，这里防并发窗口）。
func (r *secopsMaskProposalRepo) ReviewProposal(ctx context.Context, id, reviewerID string, approve bool, ruleID string, audit func(context.Context) error) (*biz.MaskProposal, error) {
	var out *biz.MaskProposal
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		p, err := r.getProposalTx(txCtx, id)
		if err != nil {
			return err
		}
		if p.Status != biz.MaskProposalStatusPendingApproval {
			return apierror.FailedPrecondition(apierror.DomainAdmin,
				"proposal is %s, only pending_approval can be reviewed", p.Status)
		}
		if reviewerID != "" && p.CreatedBy == reviewerID {
			return apierror.Forbidden(apierror.DomainAdmin, "cannot review own mask proposal")
		}
		now := nowRFC3339()
		status := biz.MaskProposalStatusRejected
		if approve {
			status = biz.MaskProposalStatusShadow
			wordlistJSON := strings.TrimSpace(p.WordlistJSON)
			if wordlistJSON == "" {
				wordlistJSON = "[]"
			}
			name := p.Name
			if name == "" {
				name = p.Entity
			}
			if _, err := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(`INSERT INTO mask_rules
					(id, name, entity, pattern, wordlist_json, operator, enabled, shadow, rollout_pct, version, created_at)
					VALUES (?, ?, ?, ?, ?, ?, 1, 1, 100, 1, ?)`),
				ruleID, name, p.Entity, p.Pattern, wordlistJSON, p.Operator, now); err != nil {
				return entErrToBizErr(err, "SECOPS")
			}
		}
		res, err := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE mask_proposals
				SET status = ?, reviewed_by = ?, reviewed_at = ?, updated_at = ?
				WHERE id = ? AND status = ?`),
			status, reviewerID, now, now, id, biz.MaskProposalStatusPendingApproval)
		if err != nil {
			return entErrToBizErr(err, "SECOPS")
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return entErrToBizErr(err, "SECOPS")
		}
		if affected != 1 {
			return apierror.FailedPrecondition(apierror.DomainAdmin, "proposal already reviewed by another approver")
		}
		if err := audit(txCtx); err != nil {
			return fmt.Errorf("audit persist failed: %w", err)
		}
		p.Status = status
		p.ReviewedBy = reviewerID
		p.ReviewedAt = parseRFC3339Time(now)
		p.UpdatedAt = p.ReviewedAt
		out = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ActivateProposal 激活事务：影子规则转正（shadow=0 + 灰度 + version+1）→
// 提案 shadow→activated → 审计闭包。影子规则缺失 → Internal（wb 500 语义）。
func (r *secopsMaskProposalRepo) ActivateProposal(ctx context.Context, id string, rolloutPct int, ruleID string, audit func(context.Context) error) (*biz.MaskProposal, error) {
	var out *biz.MaskProposal
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		p, err := r.getProposalTx(txCtx, id)
		if err != nil {
			return err
		}
		if p.Status != biz.MaskProposalStatusShadow {
			return apierror.FailedPrecondition(apierror.DomainAdmin,
				"proposal is %s, only shadow can be activated", p.Status)
		}
		now := nowRFC3339()
		res, err := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE mask_rules
				SET shadow = 0, rollout_pct = ?, version = version + 1
				WHERE id = ? AND shadow = 1`),
			rolloutPct, ruleID)
		if err != nil {
			return entErrToBizErr(err, "SECOPS")
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return entErrToBizErr(err, "SECOPS")
		}
		if affected == 0 {
			return apierror.Internal(apierror.DomainAdmin, "shadow rule %q not found or already activated", ruleID)
		}
		res, err = r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE mask_proposals
				SET status = ?, updated_at = ? WHERE id = ? AND status = ?`),
			biz.MaskProposalStatusActivated, now, id, biz.MaskProposalStatusShadow)
		if err != nil {
			return entErrToBizErr(err, "SECOPS")
		}
		affected, err = res.RowsAffected()
		if err != nil {
			return entErrToBizErr(err, "SECOPS")
		}
		if affected != 1 {
			return apierror.FailedPrecondition(apierror.DomainAdmin, "proposal status changed concurrently, refresh and retry")
		}
		if err := audit(txCtx); err != nil {
			return fmt.Errorf("audit persist failed: %w", err)
		}
		p.Status = biz.MaskProposalStatusActivated
		p.UpdatedAt = parseRFC3339Time(now)
		out = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *secopsMaskProposalRepo) getProposalTx(ctx context.Context, id string) (*biz.MaskProposal, error) {
	return r.queryProposal(ctx, id)
}

// queryProposal 单行读（execer 无 QueryRowContext：走 QueryContext + 首行，
// 与 secops_classification_repo 同例）。
func (r *secopsMaskProposalRepo) queryProposal(ctx context.Context, id string) (*biz.MaskProposal, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT `+maskProposalColumns+` FROM mask_proposals WHERE id = ?`), id)
	if err != nil {
		return nil, entErrToBizErr(err, "SECOPS")
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, entErrToBizErr(err, "SECOPS")
		}
		return nil, apierror.NotFound(apierror.DomainAdmin, "mask proposal not found")
	}
	return scanMaskProposal(rows)
}

// maskProposalScanner 让 *sql.Row / *sql.Rows 共用同一扫码列序。
type maskProposalScanner interface {
	Scan(dest ...any) error
}

func scanMaskProposal(s maskProposalScanner) (*biz.MaskProposal, error) {
	var p biz.MaskProposal
	var recall sql.NullFloat64
	var reviewedAt, createdAt, updatedAt string
	if err := s.Scan(&p.ID, &p.Name, &p.Entity, &p.Pattern, &p.WordlistJSON, &p.Operator,
		&p.Source, &p.Status, &recall, &p.SampleDetailJSON, &p.StaticFindingsJSON,
		&p.CreatedBy, &p.ReviewedBy, &reviewedAt, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, apierror.NotFound(apierror.DomainAdmin, "mask proposal not found")
		}
		return nil, entErrToBizErr(err, "SECOPS")
	}
	if recall.Valid {
		v := recall.Float64
		p.SampleRecall = &v
	}
	p.ReviewedAt = parseRFC3339Time(reviewedAt)
	p.CreatedAt = parseRFC3339Time(createdAt)
	p.UpdatedAt = parseRFC3339Time(updatedAt)
	return &p, nil
}
