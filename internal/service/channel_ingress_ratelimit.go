package service

import (
	"net/http"
	"sync"
	"time"

	"aranea-agents/internal/event"
)

const (
	defaultWebhookRateLimitPerMin = 120
	flowStepChannelWebhookRateLimit = "channel.webhook.rate_limit"
)

type webhookRateLimiter struct {
	mu      sync.Mutex
	window  time.Time
	count   int
	limit   int
}

var webhookRateLimits sync.Map // channel_key -> *webhookRateLimiter

func allowWebhookRequest(channelKey string) bool {
	channelKey = trimKey(channelKey)
	if channelKey == "" {
		return true
	}
	v, _ := webhookRateLimits.LoadOrStore(channelKey, &webhookRateLimiter{limit: defaultWebhookRateLimitPerMin})
	rl := v.(*webhookRateLimiter)
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.window.IsZero() || now.Sub(rl.window) >= time.Minute {
		rl.window = now
		rl.count = 0
	}
	if rl.count >= rl.limit {
		event.SysLogWarn(flowStepChannelWebhookRateLimit, "Channel Webhook 入站限流",
			event.P("channel_key", channelKey),
			event.P("limit_per_min", rl.limit),
		)
		return false
	}
	rl.count++
	return true
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
