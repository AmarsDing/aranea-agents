package service

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	defaultWebhookRateLimitPerMin   = 120
	flowStepChannelWebhookRateLimit = "channel.webhook.rate_limit"
)

type webhookRateLimiter struct {
	mu     sync.Mutex
	window time.Time
	count  int
	limit  int
}

var webhookRateLimits sync.Map // channel_key -> *webhookRateLimiter

var webhookRateLimitsLastCleaned atomic.Int64

const webhookRateLimitsCleanupInterval = 5 * time.Minute

func allowWebhookRequest(channelKey string) bool {
	channelKey = trimKey(channelKey)
	if channelKey == "" {
		return true
	}
	now := time.Now()
	if now.Sub(time.Unix(webhookRateLimitsLastCleaned.Load(), 0)) >= webhookRateLimitsCleanupInterval {
		webhookRateLimitsLastCleaned.Store(now.Unix())
		safego.Go(context.Background(), "channel.webhook.rate_limit_cleanup", func() {
			cleanupStaleWebhookRateLimits()
		})
	}
	v, _ := webhookRateLimits.LoadOrStore(channelKey, &webhookRateLimiter{limit: defaultWebhookRateLimitPerMin})
	rl := v.(*webhookRateLimiter)
	now = time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.window.IsZero() || now.Sub(rl.window) >= time.Minute {
		rl.window = now
		rl.count = 0
	}
	if rl.count >= rl.limit {
		loggateway.Global().Warn("Channel Webhook 入站限流",
			loggateway.StepID(flowStepChannelWebhookRateLimit),
			loggateway.Str("channel_key", channelKey),
			loggateway.Int("limit_per_min", rl.limit),
		)
		return false
	}
	rl.count++
	return true
}

// cleanupStaleWebhookRateLimits removes rate limiter entries whose window has expired.
func cleanupStaleWebhookRateLimits() {
	now := time.Now()
	webhookRateLimits.Range(func(key, value any) bool {
		rl, ok := value.(*webhookRateLimiter)
		if !ok {
			webhookRateLimits.Delete(key)
			return true
		}
		rl.mu.Lock()
		stale := !rl.window.IsZero() && now.Sub(rl.window) >= 5*time.Minute
		rl.mu.Unlock()
		if stale {
			webhookRateLimits.Delete(key)
		}
		return true
	})
}

func trimKey(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c != ' ' && c != '\t' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

func webhookRateLimitResponse(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
}
