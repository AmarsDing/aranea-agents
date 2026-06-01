package tool

import (
	"sync"
)

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

type CircuitBreakerRegistry struct {
	mu            sync.RWMutex
	breakers      map[string]*CircuitBreaker
	defaults      map[string]CircuitBreakerConfig
	overrides     map[string]CircuitBreakerConfig
	onStateChange func(name string, from, to CircuitState)
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
	if r.onStateChange != nil {
		opts = append(opts, WithOnStateChange(r.onStateChange))
	}
	cb := NewCircuitBreaker(toolName, cfg, opts...)
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
