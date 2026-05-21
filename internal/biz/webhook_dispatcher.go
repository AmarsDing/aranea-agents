package biz

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/webhookurl"
)

const webhookHTTPTimeout = 10 * time.Second

// WebhookPayload is the JSON body sent to outbound webhook targets.
type WebhookPayload struct {
	EventType string         `json:"event_type"`
	RunID     string         `json:"run_id"`
	SessionID string         `json:"session_id"`
	Status    string         `json:"status"`
	Timestamp string         `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

// WebhookDispatcher delivers run lifecycle callbacks asynchronously.
type WebhookDispatcher struct {
	repo   WebhookRepository
	client *http.Client
}

func NewWebhookDispatcher(repo WebhookRepository) *WebhookDispatcher {
	return &WebhookDispatcher{
		repo:   repo,
		client: webhookurl.NewOutboundHTTPClient(webhookHTTPTimeout),
	}
}

// Dispatch fans out one event to enabled webhooks that subscribe to eventType.
func (d *WebhookDispatcher) Dispatch(ctx context.Context, eventType, runID, sessionID, status string, data map[string]any) {
	if d == nil || d.repo == nil || strings.TrimSpace(eventType) == "" {
		return
	}
	safego.Go(ctx, "gateway.webhook.dispatch", func() {
		bg, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		configs, err := d.repo.ListEnabled(bg)
		if err != nil {
			event.SessionSysLogWarn(bg, sessionID, "gateway.webhook.list_fail", "出站 Webhook 配置加载失败",
				event.P("event_type", eventType),
				event.P("error", err.Error()),
			)
			return
		}
		payload := WebhookPayload{
			EventType: strings.TrimSpace(eventType),
			RunID:     strings.TrimSpace(runID),
			SessionID: strings.TrimSpace(sessionID),
			Status:    strings.TrimSpace(status),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Data:      data,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			event.SessionSysLogWarn(bg, sessionID, "gateway.webhook.marshal_fail", "出站 Webhook 载荷序列化失败",
				event.P("event_type", eventType),
				event.P("error", err.Error()),
			)
			return
		}
		for _, cfg := range configs {
			if !WebhookSubscribes(cfg.EventTypesJSON, eventType) {
				continue
			}
			d.postOne(bg, sessionID, cfg, body)
		}
	})
}

func (d *WebhookDispatcher) postOne(ctx context.Context, sessionID string, cfg WebhookConfig, body []byte) {
	target := strings.TrimSpace(cfg.URL)
	if err := webhookurl.ValidateNotifyURL(target); err != nil {
		event.SessionSysLogWarn(ctx, sessionID, "gateway.webhook.url_rejected", "出站 Webhook URL 被拒绝",
			event.P("webhook_id", cfg.ID),
			event.P("url", target),
			event.P("error", err.Error()),
		)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		event.SessionSysLogWarn(ctx, sessionID, "gateway.webhook.request_fail", "出站 Webhook 请求构建失败",
			event.P("webhook_id", cfg.ID),
			event.P("url", cfg.URL),
			event.P("error", err.Error()),
		)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Aranea-Gateway-Webhook/1.0")
	for k, v := range cfg.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}
	if secret := strings.TrimSpace(cfg.Secret); secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		req.Header.Set("X-Webhook-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		event.SessionSysLogWarn(ctx, sessionID, "gateway.webhook.delivery_fail", "出站 Webhook 投递失败",
			event.P("webhook_id", cfg.ID),
			event.P("url", cfg.URL),
			event.P("error", err.Error()),
		)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		event.SessionSysLogWarn(ctx, sessionID, "gateway.webhook.delivery_fail", "出站 Webhook 非 2xx 响应",
			event.P("webhook_id", cfg.ID),
			event.P("url", cfg.URL),
			event.P("status_code", resp.StatusCode),
		)
	}
}
