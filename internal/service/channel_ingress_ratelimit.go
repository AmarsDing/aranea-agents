package service

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/conf"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	flowStepChannelWebhookRateLimit = "channel.webhook.rate_limit"
)

// webhookConf holds the resolved webhook config, initialized with defaults.
// Call InitWebhookRateLimitConfig to override from *conf.Runtime.
var (
	webhookConf   = conf.RuntimeWebhookConfig{RateLimitPerMin: 120, StaleThreshold: 5 * time.Minute}
	webhookConfMu sync.RWMutex
)

// InitWebhookRateLimitConfig sets the webhook config from *conf.Runtime.
func InitWebhookRateLimitConfig(r *conf.Runtime) {
	webhookConfMu.Lock()
	defer webhookConfMu.Unlock()
	webhookConf = r.WebhookConfig()
}

// readWebhookConf returns a snapshot of the current webhook config.
func readWebhookConf() conf.RuntimeWebhookConfig {
	webhookConfMu.RLock()
	defer webhookConfMu.RUnlock()
	return webhookConf
}

// webhookLimiterSnapshot is an immutable snapshot of rate-limit state.
// Replaced atomically via sync.Map CompareAndSwap — no mutex needed.
type webhookLimiterSnapshot struct {
	windowStart time.Time
	count       int
	limit       int
}

var webhookRateLimits sync.Map // channel_key -> *webhookLimiterSnapshot

var webhookRateLimitsLastCleaned atomic.Int64

const webhookRateLimitsCleanupInterval = 5 * time.Minute

func allowWebhookRequest(channelKey string, lg loggateway.Logger) bool {
	channelKey = trimKey(channelKey)
	if channelKey == "" {
		return true
	}
	now := time.Now()
	if now.Sub(time.Unix(webhookRateLimitsLastCleaned.Load(), 0)) >= webhookRateLimitsCleanupInterval {
		webhookRateLimitsLastCleaned.Store(now.Unix())
		safego.Go(appctx.Ctx(), "channel.webhook.rate_limit_cleanup", func() {
			cleanupStaleWebhookRateLimits()
		})
	}
	limit := int(readWebhookConf().RateLimitPerMin)
	// Load or create initial snapshot.
	newSnap := &webhookLimiterSnapshot{windowStart: now, count: 1, limit: limit}
	actual, loaded := webhookRateLimits.LoadOrStore(channelKey, newSnap)
	if !loaded {
		// First request in a new window.
		return true
	}
	snap := actual.(*webhookLimiterSnapshot)
	for {
		// If window expired, reset.
		if now.Sub(snap.windowStart) >= time.Minute {
			replacement := &webhookLimiterSnapshot{windowStart: now, count: 1, limit: limit}
			if webhookRateLimits.CompareAndSwap(channelKey, snap, replacement) {
				return true
			}
			// CAS failed; reload and retry.
			v, ok := webhookRateLimits.Load(channelKey)
			if !ok {
				// Entry was deleted; re-insert.
				webhookRateLimits.Store(channelKey, &webhookLimiterSnapshot{windowStart: now, count: 1, limit: limit})
				return true
			}
			snap = v.(*webhookLimiterSnapshot)
			continue
		}
		if snap.count >= snap.limit {
			lg.Warn("Channel Webhook 入站限流",
				loggateway.StepID(flowStepChannelWebhookRateLimit),
				loggateway.Str("channel_key", channelKey),
				loggateway.Int("limit_per_min", snap.limit),
			)
			return false
		}
		replacement := &webhookLimiterSnapshot{windowStart: snap.windowStart, count: snap.count + 1, limit: snap.limit}
		if webhookRateLimits.CompareAndSwap(channelKey, snap, replacement) {
			return true
		}
		// CAS failed; reload and retry.
		v, ok := webhookRateLimits.Load(channelKey)
		if !ok {
			webhookRateLimits.Store(channelKey, &webhookLimiterSnapshot{windowStart: now, count: 1, limit: limit})
			return true
		}
		snap = v.(*webhookLimiterSnapshot)
	}
}

// cleanupStaleWebhookRateLimits removes rate limiter entries whose window has expired.
func cleanupStaleWebhookRateLimits() {
	now := time.Now()
	staleThreshold := readWebhookConf().StaleThreshold
	webhookRateLimits.Range(func(key, value any) bool {
		snap, ok := value.(*webhookLimiterSnapshot)
		if !ok {
			webhookRateLimits.Delete(key)
			return true
		}
		if !snap.windowStart.IsZero() && now.Sub(snap.windowStart) >= staleThreshold {
			// Only delete if CAS succeeds — if it failed, another goroutine
			// updated the entry (it is still live) and we must not remove it.
			if webhookRateLimits.CompareAndSwap(key, snap, nil) {
				webhookRateLimits.Delete(key)
			}
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
