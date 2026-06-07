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
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundwebhook"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/webhookurl"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// HookNotifier queues and delivers Hook notify webhooks.
// TECH-DEBT(BR1): HookNotifier 在 safego.Go 回调中直接调用 repo 写库，
// 未经过 EventBus 统一管道。当前已通过 safego.Go 异步化不阻塞回调热路径，
// 但应迁移到 EventBus + consumer 模式以保持架构一致性。
// 迁移时需确保 hook delivery 的重试语义和错误处理不变。
const (
	hookDefaultMaxAttempts = 3
	hookDefaultTimeoutSec  = 8
	hookRetryBackoffBase   = 500 * time.Millisecond
)

type HookNotifier struct {
	repo biz.HookDeliveryRepo
	lg   loggateway.Logger
}

func NewHookNotifier(repo biz.HookDeliveryRepo, lg loggateway.Logger) *HookNotifier {
	return &HookNotifier{repo: repo, lg: lg}
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
			// B-06 fix: do NOT fallback to synchronous HTTP delivery in the
			// callback hot path. Log the enqueue failure and let the
			// HookDeliveryRetryWorker pick it up on the next cycle.
			getHookLogger().Warn("hook.notify: enqueue failed; will be retried by delivery worker",
				"hook", rh.Hook.Key, "error", err)
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
			n.lg.Warn("hook.notify: UpdateResult failed",
				loggateway.StepID("plugin.hook.update_result_fail"),
				loggateway.Str("id", d.ID),
				loggateway.Err(err))
		}
		return
	}

	var lastErr string
	for attempt := startAttempt; attempt <= max; attempt++ {
		err := deliverHookWebhook(d.WebhookURL, []byte(d.PayloadJSON), d.WebhookSecret, timeoutSec)
		if err == nil {
			if uerr := n.repo.UpdateResult(ctx, d.ID, biz.HookDeliverySuccess, attempt, ""); uerr != nil {
				n.lg.Warn("hook.notify: UpdateResult(success) failed",
					loggateway.StepID("plugin.hook.update_result_fail"),
					loggateway.Str("id", d.ID),
					loggateway.Err(uerr))
			}
			return
		}
		lastErr = err.Error()
		if uerr := n.repo.UpdateResult(ctx, d.ID, biz.HookDeliveryPending, attempt, lastErr); uerr != nil {
			n.lg.Warn("hook.notify: UpdateResult(pending) failed",
				loggateway.StepID("plugin.hook.update_result_fail"),
				loggateway.Str("id", d.ID),
				loggateway.Int("attempt", attempt),
				loggateway.Err(uerr))
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
		n.lg.Warn("hook.notify: UpdateResult(failed) failed",
			loggateway.StepID("plugin.hook.update_result_fail"),
			loggateway.Str("id", d.ID),
			loggateway.Err(uerr))
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
