package plugintrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/webhookurl"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// HookNotifier queues and delivers Hook notify webhooks.
type HookNotifier struct {
	repo biz.HookDeliveryRepo
}

// NewHookNotifier creates a notifier; repo may be nil (fire-and-forget only).
func NewHookNotifier(repo biz.HookDeliveryRepo) *HookNotifier {
	return &HookNotifier{repo: repo}
}

// EnqueueNotify schedules a webhook delivery. Synchronous validation/marshal errors are returned;
// durable enqueue and HTTP delivery run asynchronously via safego.
func (n *HookNotifier) EnqueueNotify(ctx context.Context, rh biz.ResolvedHook, payload map[string]any) error {
	url := strings.TrimSpace(rh.Rule.Action.WebhookURL)
	if url == "" {
		return kerrors.BadRequest("HOOK", "webhook_url required for notify action")
	}
	if err := webhookurl.ValidateNotifyURL(url); err != nil {
		return kerrors.BadRequest("HOOK", err.Error())
	}
	opts := biz.ParseHookNotifyOptions(rh.Rule.Action)
	body, err := json.Marshal(payload)
	if err != nil {
		return kerrors.InternalServer("HOOK", "notify payload marshal failed")
	}

	if n == nil || n.repo == nil {
		safego.Go(ctx, "hook.notify."+rh.Hook.Key, func() {
			_ = deliverHookWebhook(url, body, opts.TimeoutSec)
		})
		return nil
	}

	d := biz.HookDelivery{
		ID:           uuid.NewString(),
		HookKey:      rh.Hook.Key,
		HookID:       rh.Hook.ID,
		WebhookURL:   url,
		PayloadJSON:  string(body),
		Status:       biz.HookDeliveryPending,
		MaxAttempts:  opts.MaxAttempts,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	safego.Go(ctx, "hook.notify.enqueue."+rh.Hook.Key, func() {
		bg := context.Background()
		if err := n.repo.Insert(bg, d); err != nil {
			hookLogger.Warn("hook.notify: enqueue failed", "hook", rh.Hook.Key, "error", err)
			_ = deliverHookWebhook(url, body, opts.TimeoutSec)
			return
		}
		n.processDelivery(bg, d, opts.TimeoutSec)
	})
	return nil
}

func (n *HookNotifier) processDelivery(ctx context.Context, d biz.HookDelivery, timeoutSec int) {
	if n == nil || n.repo == nil {
		return
	}
	max := d.MaxAttempts
	if max <= 0 {
		max = 3
	}
	var lastErr string
	for attempt := 1; attempt <= max; attempt++ {
		err := deliverHookWebhook(d.WebhookURL, []byte(d.PayloadJSON), timeoutSec)
		if err == nil {
			_ = n.repo.UpdateResult(ctx, d.ID, biz.HookDeliverySuccess, attempt, "")
			return
		}
		lastErr = err.Error()
		_ = n.repo.UpdateResult(ctx, d.ID, biz.HookDeliveryPending, attempt, lastErr)
		if attempt < max {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
	}
	_ = n.repo.UpdateResult(ctx, d.ID, biz.HookDeliveryFailed, max, lastErr)
	arametrics.PluginInvokeTotal.WithLabelValues("hook:"+d.HookKey, "notify", "delivery_failed").Inc()
}

func deliverHookWebhook(url string, body []byte, timeoutSec int) error {
	if err := webhookurl.ValidateNotifyURL(url); err != nil {
		return err
	}
	if timeoutSec <= 0 {
		timeoutSec = 8
	}
	client := webhookurl.NewOutboundHTTPClient(time.Duration(timeoutSec) * time.Second)
	reqCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook HTTP %d", resp.StatusCode)
	}
	return nil
}
