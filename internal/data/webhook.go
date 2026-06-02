package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/gatewaywebhook"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

type webhookRepo struct {
	data *Data
}

var _ biz.WebhookRepository = (*webhookRepo)(nil)

func NewWebhookRepo(d *Data) biz.WebhookRepository {
	return &webhookRepo{data: d}
}

func entToWebhook(lg loggateway.Logger, e *ent.GatewayWebhook) biz.WebhookConfig {
	if e == nil {
		return biz.WebhookConfig{}
	}
	headers := map[string]string{}
	if raw := strings.TrimSpace(e.HeadersJSON); raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &headers); err != nil {
			lg.Warn("unmarshal webhook headers failed", loggateway.StepID("data.webhook"), loggateway.Err(err))
		}
	}
	return biz.WebhookConfig{
		ID:             e.ID,
		Name:           e.Name,
		URL:            e.URL,
		EventTypesJSON: e.EventTypesJSON,
		Secret:         e.Secret,
		Headers:        headers,
		Enabled:        e.Enabled,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

func headersToJSON(headers map[string]string) string {
	if len(headers) == 0 {
		return "{}"
	}
	b, err := json.Marshal(headers)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (r *webhookRepo) Create(ctx context.Context, w biz.WebhookConfig) (biz.WebhookConfig, error) {
	if strings.TrimSpace(w.ID) == "" {
		w.ID = uuid.NewString()
	}
	now := nowRFC3339()
	if w.CreatedAt == "" {
		w.CreatedAt = now
	}
	w.UpdatedAt = now
	row, err := r.data.entClient.GatewayWebhook.Create().
		SetID(w.ID).
		SetName(w.Name).
		SetURL(w.URL).
		SetEventTypesJSON(w.EventTypesJSON).
		SetSecret(w.Secret).
		SetHeadersJSON(headersToJSON(w.Headers)).
		SetEnabled(w.Enabled).
		SetCreatedAt(w.CreatedAt).
		SetUpdatedAt(w.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.WebhookConfig{}, err
	}
	return entToWebhook(r.data.lg, row), nil
}

func (r *webhookRepo) Get(ctx context.Context, id string) (biz.WebhookConfig, error) {
	row, err := r.data.ReadClient(ctx).GatewayWebhook.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.WebhookConfig{}, sql.ErrNoRows
		}
		return biz.WebhookConfig{}, err
	}
	return entToWebhook(r.data.lg, row), nil
}

func (r *webhookRepo) List(ctx context.Context) ([]biz.WebhookConfig, error) {
	rows, err := r.data.ReadClient(ctx).GatewayWebhook.Query().
		Order(gatewaywebhook.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return entRowsToWebhooks(r.data.lg, rows), nil
}

func (r *webhookRepo) ListEnabled(ctx context.Context) ([]biz.WebhookConfig, error) {
	rows, err := r.data.ReadClient(ctx).GatewayWebhook.Query().
		Where(gatewaywebhook.EnabledEQ(true)).
		Order(gatewaywebhook.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return entRowsToWebhooks(r.data.lg, rows), nil
}

func entRowsToWebhooks(lg loggateway.Logger, rows []*ent.GatewayWebhook) []biz.WebhookConfig {
	out := make([]biz.WebhookConfig, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToWebhook(lg, row))
	}
	return out
}

func (r *webhookRepo) Update(ctx context.Context, w biz.WebhookConfig) (biz.WebhookConfig, error) {
	row, err := r.data.entClient.GatewayWebhook.UpdateOneID(w.ID).
		SetName(w.Name).
		SetURL(w.URL).
		SetEventTypesJSON(w.EventTypesJSON).
		SetSecret(w.Secret).
		SetHeadersJSON(headersToJSON(w.Headers)).
		SetEnabled(w.Enabled).
		SetUpdatedAt(w.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.WebhookConfig{}, err
	}
	return entToWebhook(r.data.lg, row), nil
}

func (r *webhookRepo) Delete(ctx context.Context, id string) error {
	return r.data.entClient.GatewayWebhook.DeleteOneID(strings.TrimSpace(id)).Exec(ctx)
}
