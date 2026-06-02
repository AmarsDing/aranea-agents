package data

import (
	"context"
	"time"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type skillProposalRepo struct{ data *Data }

var _ biz.SkillProposalReadWriter = (*skillProposalRepo)(nil)

func NewSkillProposalRepo(data *Data) biz.SkillProposalReadWriter {
	return &skillProposalRepo{data: data}
}

func (r *skillProposalRepo) ListByAgent(ctx context.Context, agentID string, status string) ([]biz.SkillProposal, error) {
	q := `SELECT id, agent_id, pattern_hash, pattern_desc, skill_name, skill_md, status, approved_by, rejected_by, created_at, approved_at
	       FROM skill_proposals WHERE agent_id = ?`
	args := []any{agentID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := r.data.RawDB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, kerrors.InternalServer("SKILL_EVO", "query skill proposals: "+err.Error())
	}
	defer rows.Close()
	var result []biz.SkillProposal
	for rows.Next() {
		var p biz.SkillProposal
		var createdAt string
		var approvedAt *string
		if err := rows.Scan(&p.ID, &p.AgentID, &p.PatternHash, &p.PatternDesc, &p.SkillName, &p.SkillMD, &p.Status, &p.ApprovedBy, &p.RejectedBy, &createdAt, &approvedAt); err != nil {
			return nil, kerrors.InternalServer("SKILL_EVO", "scan skill proposal: "+err.Error())
		}
		t, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, kerrors.InternalServer("SKILL_EVO", "parse created_at: "+err.Error())
		}
		p.CreatedAt = t
		if approvedAt != nil && *approvedAt != "" {
			at, err := time.Parse(time.RFC3339, *approvedAt)
			if err != nil {
				return nil, kerrors.InternalServer("SKILL_EVO", "parse approved_at: "+err.Error())
			}
			p.ApprovedAt = &at
		}
		result = append(result, p)
	}
	return result, nil
}

func (r *skillProposalRepo) GetByID(ctx context.Context, id string) (biz.SkillProposal, error) {
	q := `SELECT id, agent_id, pattern_hash, pattern_desc, skill_name, skill_md, status, approved_by, rejected_by, created_at, approved_at
	       FROM skill_proposals WHERE id = ?`
	var p biz.SkillProposal
	var createdAt string
	var approvedAt *string
	err := r.data.RawDB().QueryRowContext(ctx, q, id).Scan(
		&p.ID, &p.AgentID, &p.PatternHash, &p.PatternDesc, &p.SkillName, &p.SkillMD, &p.Status, &p.ApprovedBy, &p.RejectedBy, &createdAt, &approvedAt,
	)
	if err != nil {
		return biz.SkillProposal{}, kerrors.NotFound("SKILL_EVO", "skill proposal not found")
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return biz.SkillProposal{}, kerrors.InternalServer("SKILL_EVO", "parse created_at: "+err.Error())
	}
	p.CreatedAt = t
	if approvedAt != nil && *approvedAt != "" {
		at, err := time.Parse(time.RFC3339, *approvedAt)
		if err != nil {
			return biz.SkillProposal{}, kerrors.InternalServer("SKILL_EVO", "parse approved_at: "+err.Error())
		}
		p.ApprovedAt = &at
	}
	return p, nil
}

func (r *skillProposalRepo) GetByPatternHash(ctx context.Context, agentID string, hash string) (*biz.SkillProposal, error) {
	q := `SELECT id, agent_id, pattern_hash, pattern_desc, skill_name, skill_md, status, approved_by, rejected_by, created_at, approved_at
	       FROM skill_proposals WHERE agent_id = ? AND pattern_hash = ? ORDER BY created_at DESC LIMIT 1`
	var p biz.SkillProposal
	var createdAt string
	var approvedAt *string
	err := r.data.RawDB().QueryRowContext(ctx, q, agentID, hash).Scan(
		&p.ID, &p.AgentID, &p.PatternHash, &p.PatternDesc, &p.SkillName, &p.SkillMD, &p.Status, &p.ApprovedBy, &p.RejectedBy, &createdAt, &approvedAt,
	)
	if err != nil {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, kerrors.InternalServer("SKILL_EVO", "parse created_at: "+err.Error())
	}
	p.CreatedAt = t
	if approvedAt != nil && *approvedAt != "" {
		at, err := time.Parse(time.RFC3339, *approvedAt)
		if err != nil {
			return nil, kerrors.InternalServer("SKILL_EVO", "parse approved_at: "+err.Error())
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
	q := `INSERT INTO skill_proposals (id, agent_id, pattern_hash, pattern_desc, skill_name, skill_md, status, approved_by, rejected_by, created_at, approved_at)
	      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.data.RawDB().ExecContext(ctx, q,
		p.ID, p.AgentID, p.PatternHash, p.PatternDesc, p.SkillName, p.SkillMD, string(p.Status),
		p.ApprovedBy, p.RejectedBy,
		p.CreatedAt.UTC().Format(time.RFC3339), approvedAt,
	)
	if err != nil {
		return biz.SkillProposal{}, kerrors.InternalServer("SKILL_EVO", "insert skill proposal: "+err.Error())
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
	_, err := r.data.RawDB().ExecContext(ctx, q, args...)
	if err != nil {
		return biz.SkillProposal{}, kerrors.InternalServer("SKILL_EVO", "update skill proposal status: "+err.Error())
	}
	return r.GetByID(ctx, id)
}
