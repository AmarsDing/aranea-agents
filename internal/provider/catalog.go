package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// ModelCatalogInput is the provider-model slice needed to build a runtime model config (no biz import).
type ModelCatalogInput struct {
	Model      string
	ConfigJSON string
}

// RetryConfig holds retry configuration for provider HTTP calls.
type RetryConfig struct {
	MaxAttempts int // default 0 (disabled)
	BaseDelayMs int // default 1000
	MaxDelayMs  int // default 30000
}

// CBConfig holds circuit-breaker configuration for provider HTTP calls.
type CBConfig struct {
	Enabled          bool // default false
	FailureThreshold int  // default 3
	RecoverySec      int  // default 30
}

// HAConfig holds high-availability configuration for provider models.
type HAConfig struct {
	Mode         string
	Candidates   []HACandidateConfig
	HedgeDelayMs int
}

// CacheConfig holds prompt-caching configuration for provider models.
type CacheConfig struct {
	SystemPrompt bool
	Tools        bool
	Messages     bool
}

// ProviderModelConfig holds the resolved connection parameters parsed from a
// provider-model catalog row. It is NOT "the config of the catalog" — it is
// "the model config derived from a catalog entry".
type ProviderModelConfig struct {
	ProviderType         string
	Variant              string
	BaseURL              string
	APIKey               string
	ModelAPI             string
	SecretID             string
	SecretKey            string
	AWSRegion            string
	EnableTokenTailoring bool
	ContextWindow        int
	MaxInputTokens       int
	OptimizeForCache     bool
	ReasoningBackfill    bool
	ShowToolCallDelta    bool
	Cache                CacheConfig
	KeepAliveMinutes     int
	ChannelBufferSize    int
	HA                   HAConfig
	RateLimitRPM         int
	Capabilities         biz.ModelCapabilities
	Retry                RetryConfig
	CB                   CBConfig
}

type HACandidateConfig struct {
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
}

type catalogConfigJSON struct {
	ProviderType         string              `json:"provider_type"`
	Variant              string              `json:"variant"`
	APIBaseURL           string              `json:"api_base_url"`
	APIKey               string              `json:"api_key"`
	SecretID             string              `json:"secret_id"`
	SecretKey            string              `json:"secret_key"`
	AWSRegion            string              `json:"aws_region"`
	EnableTokenTailoring *bool               `json:"enable_token_tailoring"`
	ContextWindowK       int                 `json:"context_window_k"`
	MaxInputTokens       int                 `json:"max_input_tokens"`
	OptimizeForCache     *bool               `json:"optimize_for_cache"`
	ReasoningBackfill    *bool               `json:"reasoning_content_backfill"`
	ShowToolCallDelta    *bool               `json:"show_tool_call_delta"`
	CacheSystemPrompt    *bool               `json:"cache_system_prompt"`
	CacheTools           *bool               `json:"cache_tools"`
	CacheMessages        *bool               `json:"cache_messages"`
	KeepAliveMinutes     int                 `json:"keep_alive_minutes"`
	ChannelBufferSize    int                 `json:"channel_buffer_size"`
	HAMode               string              `json:"ha_mode"`
	HACandidates         []HACandidateConfig `json:"ha_candidates"`
	HAHedgeDelayMs       int                 `json:"ha_hedge_delay_ms"`
	RateLimitRPM         int                 `json:"rate_limit_rpm"`
	Capabilities         biz.ModelCapabilities `json:"capabilities"`
	RetryMaxAttempts     int                 `json:"retry_max_attempts"`
	RetryBaseDelayMs     int                 `json:"retry_base_delay_ms"`
	RetryMaxDelayMs      int                 `json:"retry_max_delay_ms"`
	CircuitBreakerEnabled          bool `json:"circuit_breaker_enabled"`
	CircuitBreakerFailureThreshold int  `json:"circuit_breaker_failure_threshold"`
	CircuitBreakerRecoverySec      int  `json:"circuit_breaker_recovery_sec"`
}

func ResolveModelConfig(in ModelCatalogInput) (ProviderModelConfig, error) {
	base := strings.TrimSpace(in.Model)
	if base == "" {
		return ProviderModelConfig{}, fmt.Errorf("provider model: empty model id")
	}
	raw := strings.TrimSpace(in.ConfigJSON)
	if raw == "" {
		return ProviderModelConfig{ModelAPI: base}, nil
	}
	var c catalogConfigJSON
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return ProviderModelConfig{}, fmt.Errorf("%w: %w", ErrInvalidConfigJSON, err)
	}
	return catalogConfigToConfig(c, base), nil
}

func catalogConfigToConfig(c catalogConfigJSON, modelAPI string) ProviderModelConfig {
	cfg := ProviderModelConfig{
		ProviderType:      strings.TrimSpace(c.ProviderType),
		Variant:           strings.TrimSpace(c.Variant),
		BaseURL:           strings.TrimRight(strings.TrimSpace(c.APIBaseURL), "/"),
		APIKey:            strings.TrimSpace(c.APIKey),
		ModelAPI:          modelAPI,
		SecretID:          strings.TrimSpace(c.SecretID),
		SecretKey:         strings.TrimSpace(c.SecretKey),
		AWSRegion:         strings.TrimSpace(c.AWSRegion),
		MaxInputTokens:    c.MaxInputTokens,
		KeepAliveMinutes:  c.KeepAliveMinutes,
		ChannelBufferSize: c.ChannelBufferSize,
		RateLimitRPM:      c.RateLimitRPM,
		Capabilities:      c.Capabilities,
		// HA
		HA: HAConfig{
			Mode:         strings.TrimSpace(c.HAMode),
			Candidates:   c.HACandidates,
			HedgeDelayMs: c.HAHedgeDelayMs,
		},
		// Retry
		Retry: RetryConfig{
			MaxAttempts: c.RetryMaxAttempts,
			BaseDelayMs: c.RetryBaseDelayMs,
			MaxDelayMs:  c.RetryMaxDelayMs,
		},
		// Circuit breaker
		CB: CBConfig{
			Enabled:          c.CircuitBreakerEnabled,
			FailureThreshold: c.CircuitBreakerFailureThreshold,
			RecoverySec:      c.CircuitBreakerRecoverySec,
		},
	}
	if c.EnableTokenTailoring != nil {
		cfg.EnableTokenTailoring = *c.EnableTokenTailoring
	}
	if c.ContextWindowK > 0 {
		cfg.ContextWindow = c.ContextWindowK * 1000
	}
	if c.OptimizeForCache != nil {
		cfg.OptimizeForCache = *c.OptimizeForCache
	}
	if c.ReasoningBackfill != nil {
		cfg.ReasoningBackfill = *c.ReasoningBackfill
	}
	if c.ShowToolCallDelta != nil {
		cfg.ShowToolCallDelta = *c.ShowToolCallDelta
	}
	if c.CacheSystemPrompt != nil {
		cfg.Cache.SystemPrompt = *c.CacheSystemPrompt
	}
	if c.CacheTools != nil {
		cfg.Cache.Tools = *c.CacheTools
	}
	if c.CacheMessages != nil {
		cfg.Cache.Messages = *c.CacheMessages
	}
	return cfg
}

func hasExplicitCapabilities(c biz.ModelCapabilities) bool {
	return c.Text || c.Vision || c.Audio || c.File || c.ToolCall || c.Cache || c.Thinking || c.TextOnly
}
