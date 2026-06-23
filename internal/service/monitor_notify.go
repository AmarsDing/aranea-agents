package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// MonitorAlertNotifier sends monitor alerts via webhook and optional platform channel webhook URL.
type MonitorAlertNotifier struct {
	channels *biz.ChannelUsecase
	bus      event.Bus
	lg       loggateway.Logger
}

func NewMonitorAlertNotifier(channels *biz.ChannelUsecase, bus event.Bus, lg loggateway.Logger) *MonitorAlertNotifier {
	return &MonitorAlertNotifier{channels: channels, bus: bus, lg: lg}
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
				n.lg.Warn("告警 Webhook 发送失败",
					loggateway.StepID("monitor.alert_webhook_fail"),
					loggateway.Str("rule_id", rule.ID),
					loggateway.Err(err),
				)
			}
			metrics.AlertNotifyTotal.WithLabelValues("webhook", webhookStatus).Inc()
		}
		if chID := strings.TrimSpace(rule.NotifyChannelID); chID != "" && n.channels != nil {
			channelStatus = "ok"
			if err := n.notifyViaChannel(bg, chID, rule, payload); err != nil {
				channelStatus = "error"
				n.lg.Warn("告警通道发送失败",
					loggateway.StepID("monitor.alert_channel_fail"),
					loggateway.Str("rule_id", rule.ID),
					loggateway.Str("channel_id", chID),
					loggateway.Err(err),
				)
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
		return apierror.BadRequest("MONITOR", "channel disabled")
	}
	creds, err := n.channels.ListCredentialsRaw(ctx, channelID)
	if err != nil {
		return err
	}
	webhookURL, err := resolveCredentialPlain(ctx, n.channels, creds, "webhook_url", n.lg)
	if err != nil || webhookURL == "" {
		return apierror.BadRequest("MONITOR", "channel has no webhook_url credential")
	}
	metricKey := rule.MetricKey
	if metricKey == "" {
		metricKey = "unknown"
	}
	metricVal := payload["error_rate"]
	if metricVal == nil {
		metricVal = payload["missing_count"]
	}
	if metricVal == nil {
		metricVal = payload[metricKey]
	}
	text := fmt.Sprintf("[Monitor Alert] %s — %s=%v (rule %s)", rule.Name, metricKey, metricVal, rule.ID)
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
		"rule_id":        rule.ID,
		"name":           rule.Name,
		"status":         overall,
		"webhook_status": webhookStatus,
		"channel_status": channelStatus,
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

func postAlertWebhook(rawURL string, payload map[string]any) error {
	if !strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "http://") {
		return apierror.BadRequest("MONITOR", "invalid URL scheme, must be http:// or https://: %s", rawURL)
	}
	// SSRF protection: reject URLs pointing to private/internal networks.
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return apierror.BadRequest("MONITOR", "invalid URL")
	}
	host := parsed.Hostname()
	if host == "" {
		return apierror.BadRequest("MONITOR", "URL has no host")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return apierror.Internal("MONITOR", "DNS lookup failed for %s", host)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return apierror.BadRequest("MONITOR", "host %s resolves to internal/reserved IP %s — SSRF blocked", host, ip.String())
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
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
		return apierror.Internal("MONITOR", "webhook status %d", resp.StatusCode)
	}
	return nil
}
