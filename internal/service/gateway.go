package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	v1 "aranea-agents/api/kratos/gateway/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// GatewayService implements kratos gateway.v1 webhook CRUD.
type GatewayService struct {
	v1.UnimplementedGatewayServiceServer
	wh *biz.WebhookUsecase
}

func NewGatewayService(wh *biz.WebhookUsecase) *GatewayService {
	return &GatewayService{wh: wh}
}

// maskSecret returns a redacted placeholder when a secret is set so that
// the actual value is never returned in list/get API responses (HK-08).
func maskSecret(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "••••••••"
}

// webhookToProto converts a webhook to its proto form with the secret masked.
// Use for list/get operations where the secret must not be exposed.
func webhookToProto(w biz.WebhookConfig) *v1.Webhook {
	return &v1.Webhook{
		Id:             w.ID,
		Name:           w.Name,
		Url:            w.URL,
		EventTypesJson: w.EventTypesJSON,
		Secret:         maskSecret(w.Secret),
		Headers:        w.Headers,
		Enabled:        w.Enabled,
		CreatedAt:      w.CreatedAt,
		UpdatedAt:      w.UpdatedAt,
	}
}

// webhookToProtoWithSecret converts a webhook to its proto form with the
// plaintext secret included. Use only for create/update responses where the
// caller must be able to record the value they just set (HK-08 / P1-5).
func webhookToProtoWithSecret(w biz.WebhookConfig) *v1.Webhook {
	return &v1.Webhook{
		Id:             w.ID,
		Name:           w.Name,
		Url:            w.URL,
		EventTypesJson: w.EventTypesJSON,
		Secret:         w.Secret,
		Headers:        w.Headers,
		Enabled:        w.Enabled,
		CreatedAt:      w.CreatedAt,
		UpdatedAt:      w.UpdatedAt,
	}
}

func webhookFromCreate(req *v1.CreateWebhookRequest) biz.WebhookConfig {
	if req == nil {
		return biz.WebhookConfig{Enabled: true}
	}
	w := biz.WebhookConfig{
		Name:           req.GetName(),
		URL:            req.GetUrl(),
		EventTypesJSON: req.GetEventTypesJson(),
		Secret:         req.GetSecret(),
		Headers:        req.GetHeaders(),
		Enabled:        true,
	}
	if req.Enabled != nil {
		w.Enabled = req.GetEnabled()
	}
	return w
}

func (s *GatewayService) CreateWebhook(ctx context.Context, req *v1.CreateWebhookRequest) (*v1.Webhook, error) {
	w, err := s.wh.Create(ctx, webhookFromCreate(req))
	if err != nil {
		return nil, err
	}
	// Return the plaintext secret on create so the caller can record it.
	// Subsequent list/get responses will mask it (HK-08 / P1-5).
	return webhookToProtoWithSecret(w), nil
}

func (s *GatewayService) ListWebhooks(ctx context.Context, _ *emptypb.Empty) (*v1.ListWebhooksResponse, error) {
	items, err := s.wh.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.Webhook, 0, len(items))
	for i := range items {
		out = append(out, webhookToProto(items[i]))
	}
	return &v1.ListWebhooksResponse{Items: out}, nil
}

func (s *GatewayService) UpdateWebhook(ctx context.Context, req *v1.UpdateWebhookRequest) (*v1.Webhook, error) {
	if req == nil {
		return nil, kerrors.BadRequest("GATEWAY", "request is required")
	}
	patch := biz.WebhookUpdatePatch{
		ID:             req.GetId(),
		Name:           req.GetName(),
		URL:            req.GetUrl(),
		EventTypesJSON: req.GetEventTypesJson(),
		Secret:         req.GetSecret(),
		Headers:        req.GetHeaders(),
	}
	if req.Enabled != nil {
		v := req.GetEnabled()
		patch.Enabled = &v
	}
	w, err := s.wh.Update(ctx, patch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("GATEWAY", "webhook not found")
		}
		return nil, err
	}
	// Return the plaintext secret on update so the caller can confirm the new value.
	// Subsequent list/get responses will mask it (HK-08 / P1-5).
	return webhookToProtoWithSecret(w), nil
}

func (s *GatewayService) DeleteWebhook(ctx context.Context, req *v1.DeleteWebhookRequest) (*emptypb.Empty, error) {
	if err := s.wh.Delete(ctx, req.GetId()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("GATEWAY", "webhook not found")
		}
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
