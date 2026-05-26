package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/event"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/safego"
)

// MonitorAlertNotifier sends monitor alerts via webhook and optional platform channel webhook URL.
type MonitorAlertNotifier struct {
	channels *biz.ChannelUsecase
	bus      event.Bus
}

func NewMonitorAlertNotifier(channels *biz.ChannelUsecase, bus event.Bus) *MonitorAlertNotifier {
	return &MonitorAlertNotifier{channels: channels, bus: bus}
}

func (n *MonitorAlertNotifier) Notify(ctx context.Context, rule biz.MonitorAlertRule, payload map[string]any) {
	if n == nil {
		return
	}
	safego.Go(ctx, "monitor.alert.notify", func() {
		bg := context.Background()
		webhookStatus := ""
		channelStatus := ""
		if url := strings.TrimSpace(rule.NotifyWebhookURL); url != "" {
			webhookStatus = "ok"
			if err := postAlertWebhook(url, payload); err != nil {
				webhookStatus = "error"
				event.SysLogWarn("system.monitor.alert_webhook_fail", "告警 Webhook 发送失败", event.P("rule_id", rule.ID), event.P("error", err))
			}
			metrics.AlertNotifyTotal.WithLabelValues("webhook", webhookStatus).Inc()
		}
		if chID := strings.TrimSpace(rule.NotifyChannelID); chID != "" && n.channels != nil {
			channelStatus = "ok"
			if err := n.notifyViaChannel(bg, chID, rule, payload); err != nil {
				channelStatus = "error"
				event.SysLogWarn("system.monitor.alert_channel_fail", "告警通道发送失败", event.P("rule_id", rule.ID), event.P("channel_id", chID), event.P("error", err))
			}
			metrics.AlertNotifyTotal.WithLabelValues("channel", channelStatus).Inc()
		}
		n.publishNotifyEvent(bg, rule, payload, webhookStatus, channelStatus)
	})
}

func (n *MonitorAlertNotifier) notifyViaChannel(ctx context.Context, channelID string, rule biz.MonitorAlertRule, payload map[string]any) error {
	ch, err := n.channels.Get(ctx, channelID)
	if err != nil {
		return err
	}
	if !ch.Enabled {
		return fmt.Errorf("channel disabled")
	}
	creds, err := n.channels.ListCredentialsRaw(ctx, channelID)
	if err != nil {
		return err
	}
	webhookURL, err := resolveCredentialPlain(ctx, creds, "webhook_url")
	if err != nil || webhookURL == "" {
		return fmt.Errorf("channel has no webhook_url credential")
	}
	text := fmt.Sprintf("[Monitor Alert] %s — error_rate=%v (rule %s)", rule.Name, payload["error_rate"], rule.ID)
	body := map[string]any{"text": text, "alert": payload}
	return postAlertWebhook(webhookURL, body)
}

func (n *MonitorAlertNotifier) publishNotifyEvent(ctx context.Context, rule biz.MonitorAlertRule, payload map[string]any, webhookStatus, channelStatus string) {
	if n.bus == nil {
		return
	}
	meta, _ := json.Marshal(payload)
	env := event.NewEnvelope(event.EnvelopeTypeAlertNotify, "monitor", "")
	env.Channel = "monitor"
	overall := "ok"
	if webhookStatus == "error" || channelStatus == "error" {
		overall = "error"
	}
	env.Metadata = map[string]any{
		"rule_id":         rule.ID,
		"name":            rule.Name,
		"status":          overall,
		"webhook_status":  webhookStatus,
		"channel_status":  channelStatus,
	}
	env.Content = &event.EnvelopeContent{Text: string(meta)}
	n.bus.Publish(ctx, env)
}

var alertWebhookClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
	},
}

func postAlertWebhook(url string, payload map[string]any) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("postAlertWebhook: invalid URL scheme, must be http:// or https://: %q", url)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := alertWebhookClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}
