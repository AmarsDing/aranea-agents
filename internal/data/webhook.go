package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/gatewaywebhook"

	"github.com/google/uuid"
)

type webhookRepo struct {
	data *Data
}

func NewWebhookRepo(d *Data) biz.WebhookRepository {
	return &webhookRepo{data: d}
}

func entToWebhook(e *ent.GatewayWebhook) biz.WebhookConfig {
	if e == nil {
		return biz.WebhookConfig{}
	}
	headers := map[string]string{}
	if raw := strings.TrimSpace(e.HeadersJSON); raw != "" && raw != "{}" {
		_ = json.Unmarshal([]byte(raw), &headers)
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
	return entToWebhook(row), nil
}

func (r *webhookRepo) Get(ctx context.Context, id string) (biz.WebhookConfig, error) {
	row, err := r.data.entClient.GatewayWebhook.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.WebhookConfig{}, sql.ErrNoRows
		}
		return biz.WebhookConfig{}, err
	}
	return entToWebhook(row), nil
}

func (r *webhookRepo) List(ctx context.Context) ([]biz.WebhookConfig, error) {
	rows, err := r.data.entClient.GatewayWebhook.Query().
		Order(gatewaywebhook.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return entRowsToWebhooks(rows), nil
}

func (r *webhookRepo) ListEnabled(ctx context.Context) ([]biz.WebhookConfig, error) {
	rows, err := r.data.entClient.GatewayWebhook.Query().
		Where(gatewaywebhook.EnabledEQ(true)).
		Order(gatewaywebhook.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return entRowsToWebhooks(rows), nil
}

func entRowsToWebhooks(rows []*ent.GatewayWebhook) []biz.WebhookConfig {
	out := make([]biz.WebhookConfig, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToWebhook(row))
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
	return entToWebhook(row), nil
}

func (r *webhookRepo) Delete(ctx context.Context, id string) error {
	return r.data.entClient.GatewayWebhook.DeleteOneID(strings.TrimSpace(id)).Exec(ctx)
}
