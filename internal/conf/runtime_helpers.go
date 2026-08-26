package conf

import (
	"strings"
	"time"
)

// RuntimeWSConfig returns resolved WebSocket config values with zero-value defaults.
// Zero values in config.yaml fall back to the previous hardcoded constants.
//
// Note: chat turns are intentionally unbounded (No-Timeout principle, see
// chat_orchestrator_turn.go) — turns end on completion or explicit user
// cancel. The former TurnTimeout field was removed (2026-08-24): it was
// never consumed anywhere.
type RuntimeWSConfig struct {
	ReadLimit             int64
	PongWait              time.Duration
	PingPeriod            time.Duration
	WriteWait             time.Duration
	MaxSessionConns       int32
	MaxGlobalMonitorConns int32
	HighCap               int32
	NormalCap             int32
	LowCap                int32
	HighBlockTimeout      time.Duration
	BackpressureInterval  time.Duration
	LowDrainPerLoop       int32
}

func (r *Runtime) WSConfig() RuntimeWSConfig {
	if r == nil || r.Ws == nil {
		return RuntimeWSConfig{
			ReadLimit: 1 << 20, PongWait: 60 * time.Second, PingPeriod: 30 * time.Second,
			WriteWait:       10 * time.Second,
			MaxSessionConns: 5, MaxGlobalMonitorConns: 32,
			HighCap: 64, NormalCap: 128, LowCap: 256,
			HighBlockTimeout: 5 * time.Second, BackpressureInterval: 10 * time.Second,
			LowDrainPerLoop: 8,
		}
	}
	w := r.Ws
	readLimit := w.ReadLimit
	if readLimit <= 0 {
		readLimit = 1 << 20
	}
	pongWait := msToDuration(w.PongWaitMs, 60*time.Second)
	pingPeriod := msToDuration(w.PingPeriodMs, 30*time.Second)
	writeWait := msToDuration(w.WriteWaitMs, 10*time.Second)
	maxSession := w.MaxSessionConns
	if maxSession <= 0 {
		maxSession = 5
	}
	maxMonitor := w.MaxGlobalMonitorConns
	if maxMonitor <= 0 {
		// 32: 桌面单用户多标签页场景（每标签页占 1 条全局连接），实测重度使用
		// 可达 10+ 个活跃标签页；32 提供充足余量且单连接成本极低（3 goroutine）。
		maxMonitor = 32
	}
	highCap := w.HighCap
	if highCap <= 0 {
		highCap = 64
	}
	normalCap := w.NormalCap
	if normalCap <= 0 {
		normalCap = 128
	}
	lowCap := w.LowCap
	if lowCap <= 0 {
		lowCap = 256
	}
	highBlock := msToDuration(w.HighBlockTimeoutMs, 5*time.Second)
	bpInterval := msToDuration(w.BackpressureIntervalMs, 10*time.Second)
	lowDrain := w.LowDrainPerLoop
	if lowDrain <= 0 {
		lowDrain = 8
	}
	return RuntimeWSConfig{
		ReadLimit: readLimit, PongWait: pongWait, PingPeriod: pingPeriod,
		WriteWait:       writeWait,
		MaxSessionConns: maxSession, MaxGlobalMonitorConns: maxMonitor,
		HighCap: highCap, NormalCap: normalCap, LowCap: lowCap,
		HighBlockTimeout: highBlock, BackpressureInterval: bpInterval,
		LowDrainPerLoop: lowDrain,
	}
}

// RuntimeHookConfig returns resolved Hook config values with zero-value defaults.
type RuntimeHookConfig struct {
	DefaultMaxAttempts int32
	DefaultTimeoutSec  int32
	RetryBackoffBase   time.Duration
	RetryPollInterval  time.Duration
	RetryStaleAfter    time.Duration
	RetryBatchSize     int32
	RetryQueryTimeout  time.Duration
}

func (r *Runtime) HookConfig() RuntimeHookConfig {
	if r == nil || r.Hook == nil {
		return RuntimeHookConfig{
			DefaultMaxAttempts: 3, DefaultTimeoutSec: 8,
			RetryBackoffBase: 500 * time.Millisecond, RetryPollInterval: 60 * time.Second,
			RetryStaleAfter: 5 * time.Minute, RetryBatchSize: 20, RetryQueryTimeout: 30 * time.Second,
		}
	}
	h := r.Hook
	maxAttempts := h.DefaultMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	timeoutSec := h.DefaultTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 8
	}
	backoffBase := msToDuration(h.RetryBackoffBaseMs, 500*time.Millisecond)
	pollInterval := msToDuration(h.RetryPollIntervalMs, 60*time.Second)
	staleAfter := msToDuration(h.RetryStaleAfterMs, 5*time.Minute)
	batchSize := h.RetryBatchSize
	if batchSize <= 0 {
		batchSize = 20
	}
	queryTimeout := msToDuration(h.RetryQueryTimeoutMs, 30*time.Second)
	return RuntimeHookConfig{
		DefaultMaxAttempts: maxAttempts, DefaultTimeoutSec: timeoutSec,
		RetryBackoffBase: backoffBase, RetryPollInterval: pollInterval,
		RetryStaleAfter: staleAfter, RetryBatchSize: batchSize,
		RetryQueryTimeout: queryTimeout,
	}
}

// RuntimeSelfHealConfig returns resolved SelfHeal config values with zero-value defaults.
type RuntimeSelfHealConfig struct {
	MinConfidence            float64
	MaxHistory               int32
	CircuitBreakerWindow     time.Duration
	CircuitBreakerThreshold  int32
	CircuitBreakerResetAfter time.Duration
	SeverityCooldownCritical time.Duration
	SeverityCooldownHigh     time.Duration
	SeverityCooldownMedium   time.Duration
	SeverityCooldownLow      time.Duration
}

func (r *Runtime) SelfHealConfig() RuntimeSelfHealConfig {
	if r == nil || r.SelfHeal == nil {
		return RuntimeSelfHealConfig{
			MinConfidence: 0.7, MaxHistory: 1000,
			CircuitBreakerWindow: 10 * time.Minute, CircuitBreakerThreshold: 5,
			CircuitBreakerResetAfter: 30 * time.Minute,
			SeverityCooldownCritical: 30 * time.Minute, SeverityCooldownHigh: 10 * time.Minute,
			SeverityCooldownMedium: 5 * time.Minute, SeverityCooldownLow: 2 * time.Minute,
		}
	}
	s := r.SelfHeal
	minConf := s.MinConfidence
	if minConf <= 0 {
		minConf = 0.7
	}
	maxHist := s.MaxHistory
	if maxHist <= 0 {
		maxHist = 1000
	}
	cbWindow := msToDuration(s.CircuitBreakerWindowMs, 10*time.Minute)
	cbThreshold := s.CircuitBreakerThreshold
	if cbThreshold <= 0 {
		cbThreshold = 5
	}
	cbReset := msToDuration(s.CircuitBreakerResetAfterMs, 30*time.Minute)
	cdCritical := msToDuration(s.SeverityCooldownCriticalMs, 30*time.Minute)
	cdHigh := msToDuration(s.SeverityCooldownHighMs, 10*time.Minute)
	cdMedium := msToDuration(s.SeverityCooldownMediumMs, 5*time.Minute)
	cdLow := msToDuration(s.SeverityCooldownLowMs, 2*time.Minute)
	return RuntimeSelfHealConfig{
		MinConfidence: minConf, MaxHistory: maxHist,
		CircuitBreakerWindow: cbWindow, CircuitBreakerThreshold: cbThreshold,
		CircuitBreakerResetAfter: cbReset,
		SeverityCooldownCritical: cdCritical, SeverityCooldownHigh: cdHigh,
		SeverityCooldownMedium: cdMedium, SeverityCooldownLow: cdLow,
	}
}

// RuntimeMemoryQueueConfig returns resolved MemoryQueue config values with zero-value defaults.
type RuntimeMemoryQueueConfig struct {
	HighCap              int32
	NormalCap            int32
	LowCap               int32
	MaxTenantNormalSlots int32
	Debounce             time.Duration
}

func (r *Runtime) MemoryQueueConfig() RuntimeMemoryQueueConfig {
	if r == nil || r.MemoryQueue == nil {
		return RuntimeMemoryQueueConfig{
			HighCap: 64, NormalCap: 256, LowCap: 128,
			MaxTenantNormalSlots: 128, Debounce: 30 * time.Second,
		}
	}
	m := r.MemoryQueue
	highCap := m.HighCap
	if highCap <= 0 {
		highCap = 64
	}
	normalCap := m.NormalCap
	if normalCap <= 0 {
		normalCap = 256
	}
	lowCap := m.LowCap
	if lowCap <= 0 {
		lowCap = 128
	}
	maxTenant := m.MaxTenantNormalSlots
	if maxTenant <= 0 {
		maxTenant = 128
	}
	debounce := msToDuration(m.DebounceMs, 30*time.Second)
	return RuntimeMemoryQueueConfig{
		HighCap: highCap, NormalCap: normalCap, LowCap: lowCap,
		MaxTenantNormalSlots: maxTenant, Debounce: debounce,
	}
}

// RuntimeWebhookConfig returns resolved Webhook rate-limit config values with zero-value defaults.
type RuntimeWebhookConfig struct {
	RateLimitPerMin int32
	StaleThreshold  time.Duration
}

func (r *Runtime) WebhookConfig() RuntimeWebhookConfig {
	if r == nil || r.Webhook == nil {
		return RuntimeWebhookConfig{
			RateLimitPerMin: 120, StaleThreshold: 5 * time.Minute,
		}
	}
	w := r.Webhook
	rateLimit := w.RateLimitPerMin
	if rateLimit <= 0 {
		rateLimit = 120
	}
	staleThreshold := msToDuration(w.StaleThresholdMs, 5*time.Minute)
	return RuntimeWebhookConfig{RateLimitPerMin: rateLimit, StaleThreshold: staleThreshold}
}

// RuntimeAutoMemoryConfig returns resolved AutoMemory worker config values with zero-value defaults.
type RuntimeAutoMemoryConfig struct {
	MaxRetries     int32
	MaxMessages    int32
	DrainBatchSize int32
}

func (r *Runtime) AutoMemoryConfig() RuntimeAutoMemoryConfig {
	if r == nil || r.AutoMemory == nil {
		return RuntimeAutoMemoryConfig{MaxRetries: 3, MaxMessages: 40, DrainBatchSize: 50}
	}
	a := r.AutoMemory
	maxRetries := a.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	maxMsgs := a.MaxMessages
	if maxMsgs <= 0 {
		maxMsgs = 40
	}
	drainBatch := a.DrainBatchSize
	if drainBatch <= 0 {
		drainBatch = 50
	}
	return RuntimeAutoMemoryConfig{MaxRetries: maxRetries, MaxMessages: maxMsgs, DrainBatchSize: drainBatch}
}

// RuntimeActivityFlusherConfig returns resolved ActivityFlusher config values with zero-value defaults.
type RuntimeActivityFlusherConfig struct {
	BatchSize     int32
	FlushInterval time.Duration
	ChannelBuffer int32
	DBTimeout     time.Duration
}

func (r *Runtime) ActivityFlusherConfig() RuntimeActivityFlusherConfig {
	if r == nil || r.ActivityFlusher == nil {
		return RuntimeActivityFlusherConfig{
			BatchSize: 10, FlushInterval: 500 * time.Millisecond,
			ChannelBuffer: 64, DBTimeout: 5 * time.Second,
		}
	}
	a := r.ActivityFlusher
	batchSize := a.BatchSize
	if batchSize <= 0 {
		batchSize = 10
	}
	flushInterval := msToDuration(a.FlushIntervalMs, 500*time.Millisecond)
	channelBuffer := a.ChannelBuffer
	if channelBuffer <= 0 {
		channelBuffer = 64
	}
	dbTimeout := msToDuration(a.DbTimeoutMs, 5*time.Second)
	return RuntimeActivityFlusherConfig{
		BatchSize: batchSize, FlushInterval: flushInterval,
		ChannelBuffer: channelBuffer, DBTimeout: dbTimeout,
	}
}

// RuntimePluginConfig returns resolved Plugin stats recorder config values with zero-value defaults.
type RuntimePluginConfig struct {
	PersistSuccessRuns bool
}

func (r *Runtime) PluginConfig() RuntimePluginConfig {
	if r == nil || r.Plugin == nil {
		return RuntimePluginConfig{PersistSuccessRuns: false}
	}
	return RuntimePluginConfig{
		PersistSuccessRuns: r.Plugin.PersistSuccessRuns,
	}
}

func msToDuration(ms int64, fallback time.Duration) time.Duration {
	if ms <= 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

// RuntimeToolResultPruneConfig returns resolved ToolResultPrune config values
// (79-runtime-governance R2) with zero-value defaults: pruning ON, K=8 turns,
// S=4096 bytes, no exemptions. `enabled: false` is the documented kill switch
// (dev plan Phase 1 回退项).
type RuntimeToolResultPruneConfig struct {
	Enabled     bool
	AfterTurns  int
	SizeBytes   int64
	ExemptTools map[string]bool
}

func (r *Runtime) ToolResultPruneConfig() RuntimeToolResultPruneConfig {
	cfg := RuntimeToolResultPruneConfig{Enabled: true, AfterTurns: 8, SizeBytes: 4096}
	if r == nil || r.ToolResultPrune == nil {
		return cfg
	}
	p := r.ToolResultPrune
	// proto3 optional: nil = unset → default ON; explicit false = kill switch.
	if p.Enabled != nil {
		cfg.Enabled = *p.Enabled
	}
	if p.AfterTurns > 0 {
		cfg.AfterTurns = int(p.AfterTurns)
	}
	if p.SizeBytes > 0 {
		cfg.SizeBytes = p.SizeBytes
	}
	if len(p.ExemptTools) > 0 {
		cfg.ExemptTools = make(map[string]bool, len(p.ExemptTools))
		for _, name := range p.ExemptTools {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				cfg.ExemptTools[trimmed] = true
			}
		}
	}
	return cfg
}

// RuntimeNoProgressAuditorConfig returns resolved NoProgressAuditor config
// values (79-runtime-governance R5) with zero-value defaults: auditor ON,
// correct after 3 consecutive same-fingerprint statuses, cancel after 2 more.
// `enabled: false` is the documented kill switch (dev plan Phase 2 回退项).
type RuntimeNoProgressAuditorConfig struct {
	Enabled      bool
	CorrectAfter int
	CancelAfter  int
}

func (r *Runtime) NoProgressAuditorConfig() RuntimeNoProgressAuditorConfig {
	cfg := RuntimeNoProgressAuditorConfig{Enabled: true, CorrectAfter: 3, CancelAfter: 2}
	if r == nil || r.NoProgressAuditor == nil {
		return cfg
	}
	a := r.NoProgressAuditor
	// proto3 optional: nil = unset → default ON; explicit false = kill switch.
	if a.Enabled != nil {
		cfg.Enabled = *a.Enabled
	}
	if a.CorrectAfter > 0 {
		cfg.CorrectAfter = int(a.CorrectAfter)
	}
	if a.CancelAfter > 0 {
		cfg.CancelAfter = int(a.CancelAfter)
	}
	return cfg
}

// DefaultCacheHitAlertDrift is the default cache-hit-ratio drift alert
// threshold (79-runtime-governance 1.5): ±10% around the recorded baseline.
const DefaultCacheHitAlertDrift = 0.10

// CacheHitAlertDriftThreshold returns the drift alert threshold for eval baseline
// scripts: a run whose weighted cache-hit ratio deviates from the recorded
// baseline by more than this fraction is flagged. Zero/unset = 0.10;
// negative values are clamped to the default (alerting cannot be disabled
// by config — silence by fixing the regression or re-baselining).
// (Named …Threshold to avoid colliding with the generated struct field.)
func (r *Runtime) CacheHitAlertDriftThreshold() float64 {
	if r == nil || r.CacheHitAlertDrift <= 0 {
		return DefaultCacheHitAlertDrift
	}
	return r.CacheHitAlertDrift
}
