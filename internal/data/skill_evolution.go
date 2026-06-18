package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

type skillProposalRepo struct{ data *Data }

var _ biz.SkillProposalReadWriter = (*skillProposalRepo)(nil)

func NewSkillProposalRepo(data *Data) biz.SkillProposalReadWriter {
	return &skillProposalRepo{data: data}
}

func (r *skillProposalRepo) ListByAgent(ctx context.Context, agentID string, status string, limit int, offset int) ([]biz.SkillProposal, error) {
	q := `SELECT id, agent_id, pattern_hash, pattern_desc, skill_name, skill_md, status, approved_by, rejected_by, created_at, approved_at
	       FROM skill_proposals WHERE 1=1`
	args := []any{}
	if agentID != "" {
		q += ` AND agent_id = ?`
		args = append(args, agentID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC`
	if limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_EVO")
	}
	defer rows.Close()
	var result []biz.SkillProposal
	for rows.Next() {
		var p biz.SkillProposal
		var createdAt string
		var approvedAt *string
		if err := rows.Scan(&p.ID, &p.AgentID, &p.PatternHash, &p.PatternDesc, &p.SkillName, &p.SkillMD, &p.Status, &p.ApprovedBy, &p.RejectedBy, &createdAt, &approvedAt); err != nil {
			return nil, entErrToBizErr(err, "SKILL_EVO")
		}
		t, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, entErrToBizErr(err, "SKILL_EVO")
		}
		p.CreatedAt = t
		if approvedAt != nil && *approvedAt != "" {
			at, err := time.Parse(time.RFC3339, *approvedAt)
			if err != nil {
				return nil, entErrToBizErr(err, "SKILL_EVO")
			}
			p.ApprovedAt = &at
		}
		result = append(result, p)
	}
	return result, nil
}

func (r *skillProposalRepo) GetByID(ctx context.Context, id string) (biz.SkillProposal, error) {
	q := r.data.Dialect().RenumberPlaceholders(`SELECT id, agent_id, pattern_hash, pattern_desc, skill_name, skill_md, status, approved_by, rejected_by, created_at, approved_at
	       FROM skill_proposals WHERE id = ?`)
	var p biz.SkillProposal
	var createdAt string
	var approvedAt *string
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), q, []any{id},
		&p.ID, &p.AgentID, &p.PatternHash, &p.PatternDesc, &p.SkillName, &p.SkillMD, &p.Status, &p.ApprovedBy, &p.RejectedBy, &createdAt, &approvedAt)
	if err != nil {
		return biz.SkillProposal{}, apierror.NotFound("SKILL_EVO", "skill proposal not found")
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return biz.SkillProposal{}, entErrToBizErr(err, "SKILL_EVO")
	}
	p.CreatedAt = t
	if approvedAt != nil && *approvedAt != "" {
		at, err := time.Parse(time.RFC3339, *approvedAt)
		if err != nil {
			return biz.SkillProposal{}, entErrToBizErr(err, "SKILL_EVO")
		}
		p.ApprovedAt = &at
	}
	return p, nil
}

func (r *skillProposalRepo) GetByPatternHash(ctx context.Context, agentID string, hash string) (*biz.SkillProposal, error) {
	q := r.data.Dialect().RenumberPlaceholders(`SELECT id, agent_id, pattern_hash, pattern_desc, skill_name, skill_md, status, approved_by, rejected_by, created_at, approved_at
	       FROM skill_proposals WHERE agent_id = ? AND pattern_hash = ? ORDER BY created_at DESC LIMIT 1`)
	var p biz.SkillProposal
	var createdAt string
	var approvedAt *string
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), q, []any{agentID, hash},
		&p.ID, &p.AgentID, &p.PatternHash, &p.PatternDesc, &p.SkillName, &p.SkillMD, &p.Status, &p.ApprovedBy, &p.RejectedBy, &createdAt, &approvedAt)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, nil
		}
		return nil, entErrToBizErr(err, "SKILL_EVO")
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_EVO")
	}
	p.CreatedAt = t
	if approvedAt != nil && *approvedAt != "" {
		at, err := time.Parse(time.RFC3339, *approvedAt)
		if err != nil {
			return nil, entErrToBizErr(err, "SKILL_EVO")
		}
		p.ApprovedAt = &at
	}
	return &p, nil
}

func (r *skillProposalRepo) Create(ctx context.Context, p biz.SkillProposal) (biz.SkillProposal, error) {
	var approvedAt *string
	if p.ApprovedAt != nil {
		s := p.ApprovedAt.UTC().Format(time.RFC3339)
		approvedAt = &s
	}
	q := r.data.Dialect().RenumberPlaceholders(`INSERT INTO skill_proposals (id, agent_id, pattern_hash, pattern_desc, skill_name, skill_md, status, approved_by, rejected_by, created_at, approved_at)
	      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q,
		p.ID, p.AgentID, p.PatternHash, p.PatternDesc, p.SkillName, p.SkillMD, string(p.Status),
		p.ApprovedBy, p.RejectedBy,
		p.CreatedAt.UTC().Format(time.RFC3339), approvedAt,
	)
	if err != nil {
		return biz.SkillProposal{}, entErrToBizErr(err, "SKILL_EVO")
	}
	return p, nil
}

func (r *skillProposalRepo) UpdateStatus(ctx context.Context, id string, status biz.SkillProposalStatus, operator string) (biz.SkillProposal, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	q := `UPDATE skill_proposals SET status = ?`
	args := []any{string(status)}
	switch status {
	case biz.SkillProposalStatusApproved:
		q += `, approved_by = ?, approved_at = ?`
		args = append(args, operator, now)
	case biz.SkillProposalStatusRejected:
		q += `, rejected_by = ?`
		args = append(args, operator)
	}
	q += ` WHERE id = ?`
	args = append(args, id)

	d := r.data.Dialect()
	writeDB := r.data.RWDB().WriteHandle()
	tx, txErr := writeDB.BeginTx(ctx, nil)
	if txErr != nil {
		return biz.SkillProposal{}, entErrToBizErr(txErr, "SKILL_EVO")
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, d.RenumberPlaceholders(q), args...); err != nil {
		return biz.SkillProposal{}, entErrToBizErr(err, "SKILL_EVO")
	}

	var p biz.SkillProposal
	var createdAt string
	var approvedAt *string
	selectQ := d.RenumberPlaceholders(`SELECT id, agent_id, pattern_hash, pattern_desc, skill_name, skill_md, status, approved_by, rejected_by, created_at, approved_at FROM skill_proposals WHERE id = ?`)
	if err := tx.QueryRowContext(ctx, selectQ, id).Scan(&p.ID, &p.AgentID, &p.PatternHash, &p.PatternDesc, &p.SkillName, &p.SkillMD, &p.Status, &p.ApprovedBy, &p.RejectedBy, &createdAt, &approvedAt); err != nil {
		return biz.SkillProposal{}, entErrToBizErr(err, "SKILL_EVO")
	}
	if err := tx.Commit(); err != nil {
		return biz.SkillProposal{}, entErrToBizErr(err, "SKILL_EVO")
	}

	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return biz.SkillProposal{}, entErrToBizErr(err, "SKILL_EVO")
	}
	p.CreatedAt = t
	if approvedAt != nil && *approvedAt != "" {
		at, err := time.Parse(time.RFC3339, *approvedAt)
		if err != nil {
			return biz.SkillProposal{}, entErrToBizErr(err, "SKILL_EVO")
		}
		p.ApprovedAt = &at
	}
	return p, nil
}
