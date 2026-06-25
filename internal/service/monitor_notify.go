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
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// MonitorAlertNotifier sends monitor alerts via webhook and optional platform channel webhook URL.
type MonitorAlertNotifier struct {
	channels   *biz.ChannelUsecase
	monitorBus contract.MonitorBus
	lg         loggateway.Logger
}

func NewMonitorAlertNotifier(channels *biz.ChannelUsecase, monitorBus contract.MonitorBus, lg loggateway.Logger) *MonitorAlertNotifier {
	return &MonitorAlertNotifier{channels: channels, monitorBus: monitorBus, lg: lg}
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
		return apierror.BadRequest(apierror.DomainMonitor, "channel disabled")
	}
	creds, err := n.channels.ListCredentialsRaw(ctx, channelID)
	if err != nil {
		return err
	}
	webhookURL, err := resolveCredentialPlain(ctx, n.channels, creds, "webhook_url", n.lg)
	if err != nil || webhookURL == "" {
		return apierror.BadRequest(apierror.DomainMonitor, "channel has no webhook_url credential")
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
	if n.monitorBus == nil {
		return
	}
	meta, _ := json.Marshal(payload)
	overall := "ok"
	if webhookStatus == "error" || channelStatus == "error" {
		overall = "error"
	}
	ev := contract.NewMonitorEvent(contract.MonitorEventTypeAlertNotify, "monitor")
	ev.Metadata = map[string]any{
		"rule_id":        rule.ID,
		"name":           rule.Name,
		"status":         overall,
		"webhook_status": webhookStatus,
		"channel_status": channelStatus,
		"payload":        string(meta),
	}
	n.monitorBus.Publish(ctx, ev)
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
		return apierror.BadRequest(apierror.DomainMonitor, "invalid URL scheme, must be http:// or https://: %s", rawURL)
	}
	// SSRF protection: reject URLs pointing to private/internal networks.
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return apierror.BadRequest(apierror.DomainMonitor, "invalid URL")
	}
	host := parsed.Hostname()
	if host == "" {
		return apierror.BadRequest(apierror.DomainMonitor, "URL has no host")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return apierror.Internal(apierror.DomainMonitor, "DNS lookup failed for %s", host)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return apierror.BadRequest(apierror.DomainMonitor, "host %s resolves to internal/reserved IP — SSRF blocked", host)
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
		return apierror.Internal(apierror.DomainMonitor, "webhook status %d", resp.StatusCode)
	}
	return nil
}
