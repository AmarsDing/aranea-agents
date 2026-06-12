package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/sessionruncheckpoint"
	"aranea-agents/pkg/apierror"
)

type sessionRunCheckpointRepo struct {
	data *Data
}

var _ biz.SessionRunCheckpointRepo = (*sessionRunCheckpointRepo)(nil)

// NewSessionRunCheckpointRepo implements biz.SessionRunCheckpointRepo.
func NewSessionRunCheckpointRepo(d *Data) biz.SessionRunCheckpointRepo {
	return &sessionRunCheckpointRepo{data: d}
}

func (r *sessionRunCheckpointRepo) readClient(ctx context.Context) *ent.Client {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RW().Read(ctx)
}

func (r *sessionRunCheckpointRepo) writeClient(ctx context.Context) *ent.Client {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RW().Write(ctx)
}

// entSessionRunCheckpointToBiz converts an Ent SessionRunCheckpoint entity to a biz SessionRunCheckpoint.
func entSessionRunCheckpointToBiz(e *ent.SessionRunCheckpoint) biz.SessionRunCheckpoint {
	if e == nil {
		return biz.SessionRunCheckpoint{}
	}
	return biz.SessionRunCheckpoint{
		ID:           e.ID,
		SessionRunID: e.SessionRunID,
		SessionID:    e.SessionID,
		TurnID:       e.TurnID,
		AgentID:      e.AgentID,
		PayloadJSON:  e.PayloadJSON,
		CreatedAt:    e.CreatedAt,
	}
}

func (r *sessionRunCheckpointRepo) Create(ctx context.Context, cp biz.SessionRunCheckpoint) (string, error) {
	client := r.writeClient(ctx)
	if client == nil {
		return cp.ID, nil
	}
	id := strings.TrimSpace(cp.ID)
	_, err := client.SessionRunCheckpoint.Create().
		SetID(id).
		SetSessionRunID(cp.SessionRunID).
		SetSessionID(cp.SessionID).
		SetTurnID(cp.TurnID).
		SetAgentID(cp.AgentID).
		SetPayloadJSON(cp.PayloadJSON).
		SetCreatedAt(cp.CreatedAt).
		Save(ctx)
	return id, err
}

func (r *sessionRunCheckpointRepo) Get(ctx context.Context, id string) (biz.SessionRunCheckpoint, error) {
	client := r.readClient(ctx)
	if client == nil {
		return biz.SessionRunCheckpoint{}, apierror.NotFound(apierror.DomainSession, "not found")
	}
	item, err := client.SessionRunCheckpoint.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.SessionRunCheckpoint{}, apierror.NotFound(apierror.DomainSession, "not found")
		}
		return biz.SessionRunCheckpoint{}, err
	}
	return entSessionRunCheckpointToBiz(item), nil
}

func (r *sessionRunCheckpointRepo) GetBySessionRunID(ctx context.Context, sessionRunID string) (biz.SessionRunCheckpoint, error) {
	client := r.readClient(ctx)
	if client == nil {
		return biz.SessionRunCheckpoint{}, apierror.NotFound(apierror.DomainSession, "not found")
	}
	item, err := client.SessionRunCheckpoint.Query().
		Where(sessionruncheckpoint.SessionRunIDEQ(strings.TrimSpace(sessionRunID))).
		Order(ent.Desc(sessionruncheckpoint.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.SessionRunCheckpoint{}, apierror.NotFound(apierror.DomainSession, "not found")
		}
		return biz.SessionRunCheckpoint{}, err
	}
	return entSessionRunCheckpointToBiz(item), nil
}
