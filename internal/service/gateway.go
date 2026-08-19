package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/gateway/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// GatewayService implements kratos gateway.v1 webhook CRUD.
type GatewayService struct {
	v1.UnimplementedGatewayServiceServer
	wh  *biz.WebhookUsecase
	whd *biz.WebhookDispatcher
}

func NewGatewayService(wh *biz.WebhookUsecase, whd *biz.WebhookDispatcher) *GatewayService {
	return &GatewayService{wh: wh, whd: whd}
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

func (s *GatewayService) ListWebhooks(ctx context.Context, req *v1.ListWebhooksRequest) (*v1.ListWebhooksResponse, error) {
	search := strings.TrimSpace(req.GetSearch())
	if search == "" {
		// Fallback for direct HTTP callers hitting the endpoint with legacy
		// query keys (q/keyword) outside the proto-bound fields.
		search = searchQueryFromContext(ctx)
	}
	page, pageSize := req.GetPage(), req.GetPageSize()
	if page > 0 || pageSize > 0 || search != "" {
		limit, offset, page, pageSize := biz.PageToLimitOffset(page, pageSize)
		result, err := s.wh.ListPaged(ctx, biz.WebhookListQuery{
			Search: search,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		out := make([]*v1.Webhook, 0, len(result.Items))
		for i := range result.Items {
			out = append(out, webhookToProto(result.Items[i]))
		}
		return &v1.ListWebhooksResponse{
			Items:    out,
			Total:    int32(result.Total),
			Page:     page,
			PageSize: pageSize,
		}, nil
	}
	items, err := s.wh.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.Webhook, 0, len(items))
	for i := range items {
		out = append(out, webhookToProto(items[i]))
	}
	return &v1.ListWebhooksResponse{
		Items:    out,
		Total:    int32(len(out)),
		Page:     1,
		PageSize: int32(len(out)),
	}, nil
}

func (s *GatewayService) UpdateWebhook(ctx context.Context, req *v1.UpdateWebhookRequest) (*v1.Webhook, error) {
	if req == nil {
		return nil, apierror.BadRequest("GATEWAY", "request is required")
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
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("GATEWAY", "webhook not found")
		}
		return nil, err
	}
	// Return the plaintext secret on update so the caller can confirm the new value.
	// Subsequent list/get responses will mask it (HK-08 / P1-5).
	return webhookToProtoWithSecret(w), nil
}

func (s *GatewayService) DeleteWebhook(ctx context.Context, req *v1.DeleteWebhookRequest) (*emptypb.Empty, error) {
	if err := s.wh.Delete(ctx, req.GetId()); err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("GATEWAY", "webhook not found")
		}
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// TestWebhook sends one synthetic webhook.test event to the stored config so
// operators can verify URL reachability, headers, and signature end-to-end.
// The delivery outcome is reported in the response, not as a RPC error.
func (s *GatewayService) TestWebhook(ctx context.Context, req *v1.TestWebhookRequest) (*v1.TestWebhookResponse, error) {
	if s.whd == nil {
		return nil, apierror.Internal("GATEWAY", "webhook dispatcher not configured")
	}
	w, err := s.wh.Get(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("GATEWAY", "webhook not found")
		}
		return nil, err
	}
	res := s.whd.TestDeliver(ctx, w)
	return &v1.TestWebhookResponse{
		Success:    res.Success,
		StatusCode: int32(res.StatusCode),
		Error:      res.Error,
		DurationMs: res.DurationMs,
	}, nil
}
