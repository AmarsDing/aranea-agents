package tool

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

// CircuitBreakerStateEntry represents the persistent state of a circuit breaker.
type CircuitBreakerStateEntry struct {
	State           string    // closed | open | half_open
	FailureCount    int
	SuccessCount    int
	HalfOpenProbes  int
	LastFailureTime time.Time
	LastStateChange time.Time
	UpdatedAt       time.Time
}

// CircuitBreakerStateRepo persists circuit breaker runtime state for crash recovery.
// Stability:evolving
type CircuitBreakerStateRepo interface {
	SaveState(ctx context.Context, key string, state CircuitBreakerStateEntry) error
	LoadState(ctx context.Context, key string) (CircuitBreakerStateEntry, error)
	LoadAllStates(ctx context.Context) (map[string]CircuitBreakerStateEntry, error)
}

var DefaultCircuitBreakerPresets = map[string]CircuitBreakerConfig{
	"fast_api":   {FailureThreshold: 3, RecoveryTimeoutSec: 30, HalfOpenMaxProbe: 1},
	"slow_tools": {FailureThreshold: 2, RecoveryTimeoutSec: 120, HalfOpenMaxProbe: 1},
	"external":   {FailureThreshold: 2, RecoveryTimeoutSec: 60, HalfOpenMaxProbe: 1},
}

var CategoryPresetMapping = map[string]string{
	"web":           "fast_api",
	"search":        "fast_api",
	"execution":     "slow_tools",
	"integration":   "external",
	"communication": "external",
	"filesystem":    "slow_tools",
	"memory":        "fast_api",
	"system":        "fast_api",
	"coding":        "slow_tools",
	"browser":       "slow_tools",
	"skill":         "fast_api",
	"media":         "slow_tools",
	"runtime":       "slow_tools",
	"messaging":     "external",
	"composition":   "fast_api",
	"knowledge":     "fast_api",
	"session":       "fast_api",
}

type CircuitBreakerRegistryOption func(*CircuitBreakerRegistry)

func WithRegistryOnStateChange(fn func(name string, from, to CircuitState)) CircuitBreakerRegistryOption {
	return func(r *CircuitBreakerRegistry) {
		r.onStateChange = fn
	}
}

func WithStateRepo(repo CircuitBreakerStateRepo) CircuitBreakerRegistryOption {
	return func(r *CircuitBreakerRegistry) {
		r.stateRepo = repo
	}
}

func WithLogger(lg loggateway.Logger) CircuitBreakerRegistryOption {
	return func(r *CircuitBreakerRegistry) {
		r.lg = lg
	}
}

type CircuitBreakerRegistry struct {
	mu            sync.RWMutex
	breakers      map[string]*CircuitBreaker
	defaults      map[string]CircuitBreakerConfig
	overrides     map[string]CircuitBreakerConfig
	onStateChange func(name string, from, to CircuitState)
	stateRepo     CircuitBreakerStateRepo
	lg            loggateway.Logger
}

func NewCircuitBreakerRegistry(opts ...CircuitBreakerRegistryOption) *CircuitBreakerRegistry {
	r := &CircuitBreakerRegistry{
		breakers:  make(map[string]*CircuitBreaker),
		defaults:  DefaultCircuitBreakerPresets,
		overrides: make(map[string]CircuitBreakerConfig),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *CircuitBreakerRegistry) Get(toolName, category string) *CircuitBreaker {
	r.mu.RLock()
	if cb, ok := r.breakers[toolName]; ok {
		r.mu.RUnlock()
		return cb
	}
	r.mu.RUnlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if cb, ok := r.breakers[toolName]; ok {
		return cb
	}
	cfg := r.resolveConfig(toolName, category)
	var opts []CircuitBreakerOption
	// Build combined onStateChange callback: user callback + persistence.
	if r.onStateChange != nil || r.stateRepo != nil {
		opts = append(opts, WithOnStateChange(func(name string, from, to CircuitState) {
			if r.onStateChange != nil {
				r.onStateChange(name, from, to)
			}
			if r.stateRepo != nil {
				r.persistState(name)
			}
		}))
	}
	cb := NewCircuitBreaker(toolName, cfg, opts...)
	// Restore persisted state if available.
	if r.stateRepo != nil {
		if entry, err := r.stateRepo.LoadState(context.Background(), toolName); err == nil && entry.State != "" {
			cb.restoreFromEntry(entry)
		}
	}
	r.breakers[toolName] = cb
	return cb
}

func (r *CircuitBreakerRegistry) SetOverride(toolName string, cfg CircuitBreakerConfig) {
	r.mu.Lock()
	r.overrides[toolName] = cfg
	cb, exists := r.breakers[toolName]
	r.mu.Unlock()
	if exists {
		cb.UpdateConfig(cfg)
	}
}

func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cb := range r.breakers {
		cb.Reset()
	}
}

func (r *CircuitBreakerRegistry) ResetTool(toolName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cb, ok := r.breakers[toolName]; ok {
		cb.Reset()
	}
}

func (r *CircuitBreakerRegistry) OpenBreakers() []string {
	r.mu.RLock()
	breakers := make([]*CircuitBreaker, 0, len(r.breakers))
	for _, cb := range r.breakers {
		breakers = append(breakers, cb)
	}
	r.mu.RUnlock()
	var names []string
	for _, cb := range breakers {
		if cb.State() == CircuitOpen {
			names = append(names, cb.Name())
		}
	}
	return names
}

func (r *CircuitBreakerRegistry) resolveConfig(toolName, category string) CircuitBreakerConfig {
	if cfg, ok := r.overrides[toolName]; ok {
		return cfg
	}
	presetKey, ok := CategoryPresetMapping[category]
	if !ok {
		presetKey = "fast_api"
	}
	cfg, ok := r.defaults[presetKey]
	if !ok {
		cfg = CircuitBreakerConfig{FailureThreshold: 3, RecoveryTimeoutSec: 30, HalfOpenMaxProbe: 1}
	}
	return cfg
}

// persistState saves the current state of a circuit breaker to the stateRepo.
// Called from the onStateChange callback; must be nil-safe.
func (r *CircuitBreakerRegistry) persistState(name string) {
	if r.stateRepo == nil {
		return
	}
	r.mu.RLock()
	cb, ok := r.breakers[name]
	r.mu.RUnlock()
	if !ok {
		return
	}
	entry := cb.snapshotEntry()
	if err := r.stateRepo.SaveState(context.Background(), name, entry); err != nil {
		if r.lg != nil {
			r.lg.Warn("circuit_breaker: failed to persist state",
				loggateway.Str("name", name),
				loggateway.Err(err),
			)
		}
	}
}

// RestoreStates loads all persisted circuit breaker states and applies them
// to existing breakers. Call this once after registry construction (e.g. at
// startup) to recover from a process restart.
func (r *CircuitBreakerRegistry) RestoreStates(ctx context.Context) {
	if r.stateRepo == nil {
		return
	}
	states, err := r.stateRepo.LoadAllStates(ctx)
	if err != nil {
		if r.lg != nil {
			r.lg.Warn("circuit_breaker: failed to load persisted states",
				loggateway.Err(err),
			)
		}
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, entry := range states {
		if cb, ok := r.breakers[name]; ok {
			cb.restoreFromEntry(entry)
		}
	}
}
