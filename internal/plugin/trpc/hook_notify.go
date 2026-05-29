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
	"aranea-agents/internal/biz/hook"
	"aranea-agents/internal/event"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/outboundwebhook"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/webhookurl"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// HookNotifier queues and delivers Hook notify webhooks.
const (
	hookDefaultMaxAttempts = 3
	hookDefaultTimeoutSec  = 8
	hookRetryBackoffBase   = 500 * time.Millisecond
)

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
			if err := deliverHookWebhook(url, body, opts.WebhookSecret, opts.TimeoutSec); err != nil {
				getHookLogger().Warn("hook.notify: fire-and-forget delivery failed", "hook", rh.Hook.Key, "error", err)
			}
		})
		return nil
	}

	d := biz.HookDelivery{
		ID:             uuid.NewString(),
		HookKey:        rh.Hook.Key,
		HookID:         rh.Hook.ID,
		WebhookURL:     url,
		WebhookSecret:  opts.WebhookSecret,
		PayloadJSON:    string(body),
		Status:         biz.HookDeliveryPending,
		MaxAttempts:    opts.MaxAttempts,
		IdempotencyKey: hook.DeliveryIdempotencyKey(rh.Hook.ID, payload),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	safego.Go(ctx, "hook.notify.enqueue."+rh.Hook.Key, func() {
		maxAttempts := opts.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = hookDefaultMaxAttempts
		}
		maxRetryDuration := time.Duration(maxAttempts)*hookRetryBackoffBase + time.Duration(opts.TimeoutSec)*time.Second + 5*time.Second
		bg, cancel := context.WithTimeout(context.Background(), maxRetryDuration)
		defer cancel()
		if err := n.repo.Insert(bg, d); err != nil {
			getHookLogger().Warn("hook.notify: enqueue failed", "hook", rh.Hook.Key, "error", err)
			if ferr := deliverHookWebhook(url, body, opts.WebhookSecret, opts.TimeoutSec); ferr != nil {
				getHookLogger().Warn("hook.notify: fallback delivery failed", "hook", rh.Hook.Key, "error", ferr)
			}
			return
		}
		n.processDelivery(bg, d, opts.TimeoutSec)
	})
	return nil
}

// processDelivery attempts delivery starting from attempt number 1.
// For retries after crash recovery use processDeliveryFrom with startAttempt=d.AttemptCount+1.
func (n *HookNotifier) processDelivery(ctx context.Context, d biz.HookDelivery, timeoutSec int) {
	n.processDeliveryFrom(ctx, d, timeoutSec, 1)
}

// processDeliveryFrom runs delivery attempts from startAttempt up to d.MaxAttempts.
// Callers must ensure startAttempt > 0 and that the DB attempt_count has already
// been advanced to startAttempt-1 (e.g. via TryClaimForRetry) before calling.
func (n *HookNotifier) processDeliveryFrom(ctx context.Context, d biz.HookDelivery, timeoutSec, startAttempt int) {
	if n == nil || n.repo == nil {
		return
	}
	max := d.MaxAttempts
	if max <= 0 {
		max = hookDefaultMaxAttempts
	}
	if startAttempt <= 0 {
		startAttempt = 1
	}
	if startAttempt > max {
		// All attempts already consumed — mark failed and return.
		if err := n.repo.UpdateResult(ctx, d.ID, biz.HookDeliveryFailed, max, "max attempts reached"); err != nil {
			event.SysLogWarn("system.hook.delivery_fail", "hook.notify: UpdateResult failed", event.P("id", d.ID), event.P("error", err.Error()))
		}
		return
	}

	var lastErr string
	for attempt := startAttempt; attempt <= max; attempt++ {
		err := deliverHookWebhook(d.WebhookURL, []byte(d.PayloadJSON), d.WebhookSecret, timeoutSec)
		if err == nil {
			if uerr := n.repo.UpdateResult(ctx, d.ID, biz.HookDeliverySuccess, attempt, ""); uerr != nil {
				event.SysLogWarn("system.hook.delivery_fail", "hook.notify: UpdateResult(success) failed", event.P("id", d.ID), event.P("error", uerr.Error()))
			}
			return
		}
		lastErr = err.Error()
		if uerr := n.repo.UpdateResult(ctx, d.ID, biz.HookDeliveryPending, attempt, lastErr); uerr != nil {
			event.SysLogWarn("system.hook.delivery_fail", "hook.notify: UpdateResult(pending) failed", event.P("id", d.ID), event.P("attempt", attempt), event.P("error", uerr.Error()))
		}
		if attempt < max {
			select {
			case <-time.After(time.Duration(attempt) * hookRetryBackoffBase):
			case <-ctx.Done():
				return
			}
		}
	}
	if uerr := n.repo.UpdateResult(ctx, d.ID, biz.HookDeliveryFailed, max, lastErr); uerr != nil {
		event.SysLogWarn("system.hook.delivery_fail", "hook.notify: UpdateResult(failed) failed", event.P("id", d.ID), event.P("error", uerr.Error()))
	}
	arametrics.PluginInvokeTotal.WithLabelValues("hook:"+d.HookKey, "notify", "delivery_failed").Inc()
}

func deliverHookWebhook(url string, body []byte, secret string, timeoutSec int) error {
	if err := webhookurl.ValidateNotifyURL(url); err != nil {
		return err
	}
	if timeoutSec <= 0 {
		timeoutSec = hookDefaultTimeoutSec
	}
	client := webhookurl.NewOutboundHTTPClient(time.Duration(timeoutSec) * time.Second)
	reqCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	outboundwebhook.AddSignatureHeaders(req, secret, body)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return kerrors.InternalServer("HOOK", fmt.Sprintf("webhook HTTP %d", resp.StatusCode))
	}
	return nil
}
