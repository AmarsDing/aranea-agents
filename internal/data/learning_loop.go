package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type obsRepo struct{ data *Data }
type patternRepo struct{ data *Data }
type proposalRepo struct{ data *Data }

type ObservationReadWriter = biz.ObservationReadWriter
type PatternReadWriter = biz.PatternReadWriter
type ProposalReadWriter = biz.ProposalReadWriter

var (
	_ biz.ObservationReader = (*obsRepo)(nil)
	_ biz.ObservationWriter = (*obsRepo)(nil)
	_ biz.PatternReader     = (*patternRepo)(nil)
	_ biz.PatternWriter     = (*patternRepo)(nil)
	_ biz.ProposalReader    = (*proposalRepo)(nil)
	_ biz.ProposalWriter    = (*proposalRepo)(nil)
)

func NewObservationRepo(data *Data) biz.ObservationReadWriter { return &obsRepo{data: data} }
func NewPatternRepo(data *Data) biz.PatternReadWriter         { return &patternRepo{data: data} }
func NewProposalRepo(data *Data) biz.ProposalReadWriter       { return &proposalRepo{data: data} }

func (r *obsRepo) ListByAgent(ctx context.Context, agentID string, since time.Time) ([]biz.Observation, error) {
	q := `SELECT id, agent_id, session_id, kind, content, metadata, observed_at
	       FROM learning_observations WHERE agent_id = ? AND observed_at >= ? ORDER BY observed_at ASC`
	rows, err := r.data.RawDB().QueryContext(ctx, q, agentID, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, kerrors.InternalServer("LEARNING", "query observations: "+err.Error())
	}
	defer rows.Close()
	var result []biz.Observation
	for rows.Next() {
		var o biz.Observation
		var observedAt string
		if err := rows.Scan(&o.ID, &o.AgentID, &o.SessionID, &o.Kind, &o.Content, &o.Metadata, &observedAt); err != nil {
			return nil, kerrors.InternalServer("LEARNING", "scan observation: "+err.Error())
		}
		t, err := time.Parse(time.RFC3339, observedAt)
		if err != nil {
			return nil, kerrors.InternalServer("LEARNING", "parse observed_at: "+err.Error())
		}
		o.ObservedAt = t
		result = append(result, o)
	}
	return result, nil
}

func (r *obsRepo) CountByAgent(ctx context.Context, agentID string, since time.Time) (int64, error) {
	q := `SELECT COUNT(*) FROM learning_observations WHERE agent_id = ? AND observed_at >= ?`
	var count int64
	err := r.data.RawDB().QueryRowContext(ctx, q, agentID, since.UTC().Format(time.RFC3339)).Scan(&count)
	if err != nil {
		return 0, kerrors.InternalServer("LEARNING", "count observations: "+err.Error())
	}
	return count, nil
}

func (r *obsRepo) Create(ctx context.Context, obs biz.Observation) (biz.Observation, error) {
	q := `INSERT INTO learning_observations (id, agent_id, session_id, kind, content, metadata, observed_at)
	      VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := r.data.RawDB().ExecContext(ctx, q,
		obs.ID, obs.AgentID, obs.SessionID, string(obs.Kind), obs.Content, obs.Metadata,
		obs.ObservedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return biz.Observation{}, kerrors.InternalServer("LEARNING", "insert observation: "+err.Error())
	}
	return obs, nil
}

func (r *obsRepo) BatchCreate(ctx context.Context, obs []biz.Observation) error {
	if len(obs) == 0 {
		return nil
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, r.data.RawDB())
		q := `INSERT INTO learning_observations (id, agent_id, session_id, kind, content, metadata, observed_at)
		      VALUES (?, ?, ?, ?, ?, ?, ?)`
		for _, o := range obs {
			_, err := e.ExecContext(txCtx, q,
				o.ID, o.AgentID, o.SessionID, string(o.Kind), o.Content, o.Metadata,
				o.ObservedAt.UTC().Format(time.RFC3339),
			)
			if err != nil {
				return kerrors.InternalServer("LEARNING", "batch insert observation: "+err.Error())
			}
		}
		return nil
	})
}

func (r *patternRepo) ListByAgent(ctx context.Context, agentID string, status string) ([]biz.Pattern, error) {
	q := `SELECT id, agent_id, kind, description, frequency, confidence, evidence, status, detected_at
	       FROM learning_patterns WHERE agent_id = ?`
	args := []any{agentID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY detected_at DESC`
	rows, err := r.data.RawDB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, kerrors.InternalServer("LEARNING", "query patterns: "+err.Error())
	}
	defer rows.Close()
	var result []biz.Pattern
	for rows.Next() {
		var p biz.Pattern
		var detectedAt string
		if err := rows.Scan(&p.ID, &p.AgentID, &p.Kind, &p.Description, &p.Frequency, &p.Confidence, &p.Evidence, &p.Status, &detectedAt); err != nil {
			return nil, kerrors.InternalServer("LEARNING", "scan pattern: "+err.Error())
		}
		t, err := time.Parse(time.RFC3339, detectedAt)
		if err != nil {
			return nil, kerrors.InternalServer("LEARNING", "parse detected_at: "+err.Error())
		}
		p.DetectedAt = t
		result = append(result, p)
	}
	return result, nil
}

func (r *patternRepo) GetByID(ctx context.Context, id string) (biz.Pattern, error) {
	q := `SELECT id, agent_id, kind, description, frequency, confidence, evidence, status, detected_at
	       FROM learning_patterns WHERE id = ?`
	var p biz.Pattern
	var detectedAt string
	err := r.data.RawDB().QueryRowContext(ctx, q, id).Scan(
		&p.ID, &p.AgentID, &p.Kind, &p.Description, &p.Frequency, &p.Confidence, &p.Evidence, &p.Status, &detectedAt,
	)
	if err != nil {
		return biz.Pattern{}, kerrors.NotFound("LEARNING", "pattern not found")
	}
	t, err := time.Parse(time.RFC3339, detectedAt)
	if err != nil {
		return biz.Pattern{}, kerrors.InternalServer("LEARNING", "parse detected_at: "+err.Error())
	}
	p.DetectedAt = t
	return p, nil
}

func (r *patternRepo) Create(ctx context.Context, p biz.Pattern) (biz.Pattern, error) {
	evidenceJSON, err := json.Marshal(p.Evidence)
	if err != nil {
		return biz.Pattern{}, kerrors.InternalServer("LEARNING", "marshal evidence: "+err.Error())
	}
	q := `INSERT INTO learning_patterns (id, agent_id, kind, description, frequency, confidence, evidence, status, detected_at)
	      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = r.data.RawDB().ExecContext(ctx, q,
		p.ID, p.AgentID, p.Kind, p.Description, p.Frequency, p.Confidence, string(evidenceJSON),
		string(p.Status), p.DetectedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return biz.Pattern{}, kerrors.InternalServer("LEARNING", "insert pattern: "+err.Error())
	}
	return p, nil
}

func (r *patternRepo) UpdateStatus(ctx context.Context, id string, status biz.PatternStatus) (biz.Pattern, error) {
	q := `UPDATE learning_patterns SET status = ? WHERE id = ?`
	_, err := r.data.RawDB().ExecContext(ctx, q, string(status), id)
	if err != nil {
		return biz.Pattern{}, kerrors.InternalServer("LEARNING", "update pattern status: "+err.Error())
	}
	return r.GetByID(ctx, id)
}

func (r *proposalRepo) ListByAgent(ctx context.Context, agentID string, status string) ([]biz.KnowledgeProposal, error) {
	q := `SELECT id, agent_id, pattern_id, title, content, kind, status, validated_at, approved_by, created_at, updated_at
	       FROM learning_proposals WHERE agent_id = ?`
	args := []any{agentID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := r.data.RawDB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, kerrors.InternalServer("LEARNING", "query proposals: "+err.Error())
	}
	defer rows.Close()
	var result []biz.KnowledgeProposal
	for rows.Next() {
		var p biz.KnowledgeProposal
		var validatedAt, createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.AgentID, &p.PatternID, &p.Title, &p.Content, &p.Kind, &p.Status, &validatedAt, &p.ApprovedBy, &createdAt, &updatedAt); err != nil {
			return nil, kerrors.InternalServer("LEARNING", "scan proposal: "+err.Error())
		}
		if validatedAt != "" {
			t, err := time.Parse(time.RFC3339, validatedAt)
			if err != nil {
				return nil, kerrors.InternalServer("LEARNING", "parse validated_at: "+err.Error())
			}
			p.ValidatedAt = &t
		}
		cat, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, kerrors.InternalServer("LEARNING", "parse created_at: "+err.Error())
		}
		p.CreatedAt = cat
		uat, err := time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, kerrors.InternalServer("LEARNING", "parse updated_at: "+err.Error())
		}
		p.UpdatedAt = uat
		result = append(result, p)
	}
	return result, nil
}

func (r *proposalRepo) GetByID(ctx context.Context, id string) (biz.KnowledgeProposal, error) {
	q := `SELECT id, agent_id, pattern_id, title, content, kind, status, validated_at, approved_by, created_at, updated_at
	       FROM learning_proposals WHERE id = ?`
	var p biz.KnowledgeProposal
	var validatedAt, createdAt, updatedAt string
	err := r.data.RawDB().QueryRowContext(ctx, q, id).Scan(
		&p.ID, &p.AgentID, &p.PatternID, &p.Title, &p.Content, &p.Kind, &p.Status,
		&validatedAt, &p.ApprovedBy, &createdAt, &updatedAt,
	)
	if err != nil {
		return biz.KnowledgeProposal{}, kerrors.NotFound("LEARNING", "proposal not found")
	}
	if validatedAt != "" {
		t, err := time.Parse(time.RFC3339, validatedAt)
		if err != nil {
			return biz.KnowledgeProposal{}, kerrors.InternalServer("LEARNING", "parse validated_at: "+err.Error())
		}
		p.ValidatedAt = &t
	}
	cat, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return biz.KnowledgeProposal{}, kerrors.InternalServer("LEARNING", "parse created_at: "+err.Error())
	}
	p.CreatedAt = cat
	uat, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return biz.KnowledgeProposal{}, kerrors.InternalServer("LEARNING", "parse updated_at: "+err.Error())
	}
	p.UpdatedAt = uat
	return p, nil
}

func (r *proposalRepo) Create(ctx context.Context, p biz.KnowledgeProposal) (biz.KnowledgeProposal, error) {
	var validatedAt *string
	if p.ValidatedAt != nil {
		s := p.ValidatedAt.UTC().Format(time.RFC3339)
		validatedAt = &s
	}
	q := `INSERT INTO learning_proposals (id, agent_id, pattern_id, title, content, kind, status, validated_at, approved_by, created_at, updated_at)
	      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.data.RawDB().ExecContext(ctx, q,
		p.ID, p.AgentID, p.PatternID, p.Title, p.Content, p.Kind, string(p.Status),
		validatedAt, p.ApprovedBy,
		p.CreatedAt.UTC().Format(time.RFC3339), p.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return biz.KnowledgeProposal{}, kerrors.InternalServer("LEARNING", "insert proposal: "+err.Error())
	}
	return p, nil
}

func (r *proposalRepo) UpdateStatus(ctx context.Context, id string, status biz.ProposalStatus, approvedBy string) (biz.KnowledgeProposal, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	q := `UPDATE learning_proposals SET status = ?, approved_by = ?, updated_at = ?`
	args := []any{string(status), approvedBy, now}
	if status == biz.ProposalStatusValidated {
		q += `, validated_at = ?`
		args = append(args, now)
	}
	q += ` WHERE id = ?`
	args = append(args, id)
	_, err := r.data.RawDB().ExecContext(ctx, q, args...)
	if err != nil {
		return biz.KnowledgeProposal{}, kerrors.InternalServer("LEARNING", "update proposal status: "+err.Error())
	}
	return r.GetByID(ctx, id)
}
