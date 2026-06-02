package biz

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/pkg/outboundwebhook"
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
	logger SessionLogWriter
}

func NewWebhookDispatcher(repo WebhookRepository, logger SessionLogWriter) *WebhookDispatcher {
	return &WebhookDispatcher{
		repo:   repo,
		client: webhookurl.NewOutboundHTTPClient(webhookHTTPTimeout),
		logger: logger,
	}
}

// Dispatch fans out one event to enabled webhooks that subscribe to eventType.
func (d *WebhookDispatcher) Dispatch(ctx context.Context, eventType, runID, sessionID, status string, data map[string]any) {
	if d == nil || d.repo == nil || strings.TrimSpace(eventType) == "" {
		return
	}
	safego.Go(ctx, "gateway.webhook.dispatch", func() {
		// Budget: 3 attempts × 10 s HTTP timeout + 1.5 s sleep = ~31.5 s worst case.
		// Use 60 s so the third retry is never cancelled by the context itself.
		bg, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		configs, err := d.repo.ListEnabled(bg)
		if err != nil {
			if d.logger != nil {
				d.logger.LogSessionWarn(bg, sessionID, "gateway.webhook.list_fail", "出站 Webhook 配置加载失败",
					LogPair{Key: "event_type", Value: eventType},
					LogPair{Key: "error", Value: err.Error()},
				)
			}
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
			if d.logger != nil {
				d.logger.LogSessionWarn(bg, sessionID, "gateway.webhook.marshal_fail", "出站 Webhook 载荷序列化失败",
					LogPair{Key: "event_type", Value: eventType},
					LogPair{Key: "error", Value: err.Error()},
				)
			}
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
		if d.logger != nil {
			d.logger.LogSessionWarn(ctx, sessionID, "gateway.webhook.url_rejected", "出站 Webhook URL 被拒绝",
				LogPair{Key: "webhook_id", Value: cfg.ID},
				LogPair{Key: "url", Value: target},
				LogPair{Key: "error", Value: err.Error()},
			)
		}
		return
	}

	const maxAttempts = 3
	var lastErr string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
		if err != nil {
			if d.logger != nil {
				d.logger.LogSessionWarn(ctx, sessionID, "gateway.webhook.request_fail", "出站 Webhook 请求构建失败",
					LogPair{Key: "webhook_id", Value: cfg.ID},
					LogPair{Key: "url", Value: cfg.URL},
					LogPair{Key: "error", Value: err.Error()},
				)
			}
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
			outboundwebhook.AddSignatureHeaders(req, secret, body)
		}
		resp, err := d.client.Do(req)
		if err != nil {
			lastErr = err.Error()
			if attempt < maxAttempts {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 300 {
			return
		}
		// Do not retry on 4xx client errors — same result expected on retry.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			if d.logger != nil {
				d.logger.LogSessionWarn(ctx, sessionID, "gateway.webhook.delivery_fail", "出站 Webhook 客户端错误（不重试）",
					LogPair{Key: "webhook_id", Value: cfg.ID},
					LogPair{Key: "url", Value: cfg.URL},
					LogPair{Key: "status_code", Value: resp.StatusCode},
				)
			}
			return
		}
		lastErr = http.StatusText(resp.StatusCode)
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
	}
	if d.logger != nil {
		d.logger.LogSessionWarn(ctx, sessionID, "gateway.webhook.delivery_fail", "出站 Webhook 投递失败（已重试）",
			LogPair{Key: "webhook_id", Value: cfg.ID},
			LogPair{Key: "url", Value: cfg.URL},
			LogPair{Key: "attempts", Value: maxAttempts},
			LogPair{Key: "error", Value: lastErr},
		)
	}
}
