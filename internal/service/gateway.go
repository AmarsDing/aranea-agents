package service

import (
	"context"
	"database/sql"
	"errors"

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

func webhookToProto(w biz.WebhookConfig) *v1.Webhook {
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
	return webhookToProto(w), nil
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
	return webhookToProto(w), nil
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
