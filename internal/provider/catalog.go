package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// ModelCatalogInput is the provider-model slice needed to build a runtime catalog (no biz import).
type ModelCatalogInput struct {
	Model      string
	ConfigJSON string
}

type CatalogConfig struct {
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
	CacheSystemPrompt    bool
	CacheTools           bool
	CacheMessages        bool
	KeepAliveMinutes     int
	ChannelBufferSize    int
	HAMode               string
	HACandidates         []HACandidateConfig
	HAHedgeDelayMs       int
	RateLimitRPM         int
	Capabilities         biz.ModelCapabilities
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
	Capabilities         biz.ModelCapabilities   `json:"capabilities"`
}

func CatalogFromModel(in ModelCatalogInput) (CatalogConfig, error) {
	base := strings.TrimSpace(in.Model)
	if base == "" {
		return CatalogConfig{}, fmt.Errorf("provider model: empty model id")
	}
	var c catalogConfigJSON
	_ = json.Unmarshal([]byte(strings.TrimSpace(in.ConfigJSON)), &c)
	return catalogConfigToConfig(c, base), nil
}

func CatalogFromEndpoints(providerType, baseURL, apiKey string) CatalogConfig {
	return CatalogConfig{
		ProviderType: strings.TrimSpace(providerType),
		BaseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:       strings.TrimSpace(apiKey),
	}
}

func MergeCatalogConfig(cfg CatalogConfig, configJSON string) CatalogConfig {
	raw := strings.TrimSpace(configJSON)
	if raw == "" {
		return cfg
	}
	var c catalogConfigJSON
	if json.Unmarshal([]byte(strings.TrimSpace(configJSON)), &c) != nil {
		return cfg
	}
	merged := cfg
	if merged.ProviderType == "" {
		merged.ProviderType = strings.TrimSpace(c.ProviderType)
	}
	if merged.Variant == "" {
		merged.Variant = strings.TrimSpace(c.Variant)
	}
	if merged.BaseURL == "" {
		merged.BaseURL = strings.TrimRight(strings.TrimSpace(c.APIBaseURL), "/")
	}
	if merged.APIKey == "" {
		merged.APIKey = strings.TrimSpace(c.APIKey)
	}
	if merged.SecretID == "" {
		merged.SecretID = strings.TrimSpace(c.SecretID)
	}
	if merged.SecretKey == "" {
		merged.SecretKey = strings.TrimSpace(c.SecretKey)
	}
	if merged.AWSRegion == "" {
		merged.AWSRegion = strings.TrimSpace(c.AWSRegion)
	}
	if !merged.EnableTokenTailoring && c.EnableTokenTailoring != nil {
		merged.EnableTokenTailoring = *c.EnableTokenTailoring
	}
	if merged.ContextWindow == 0 && c.ContextWindowK > 0 {
		merged.ContextWindow = c.ContextWindowK * 1000
	}
	if merged.MaxInputTokens == 0 && c.MaxInputTokens > 0 {
		merged.MaxInputTokens = c.MaxInputTokens
	}
	if !merged.OptimizeForCache && c.OptimizeForCache != nil {
		merged.OptimizeForCache = *c.OptimizeForCache
	}
	if !merged.ReasoningBackfill && c.ReasoningBackfill != nil {
		merged.ReasoningBackfill = *c.ReasoningBackfill
	}
	if !merged.ShowToolCallDelta && c.ShowToolCallDelta != nil {
		merged.ShowToolCallDelta = *c.ShowToolCallDelta
	}
	if !merged.CacheSystemPrompt && c.CacheSystemPrompt != nil {
		merged.CacheSystemPrompt = *c.CacheSystemPrompt
	}
	if !merged.CacheTools && c.CacheTools != nil {
		merged.CacheTools = *c.CacheTools
	}
	if !merged.CacheMessages && c.CacheMessages != nil {
		merged.CacheMessages = *c.CacheMessages
	}
	if merged.KeepAliveMinutes == 0 && c.KeepAliveMinutes > 0 {
		merged.KeepAliveMinutes = c.KeepAliveMinutes
	}
	if merged.ChannelBufferSize == 0 && c.ChannelBufferSize > 0 {
		merged.ChannelBufferSize = c.ChannelBufferSize
	}
	if merged.HAMode == "" {
		merged.HAMode = strings.TrimSpace(c.HAMode)
	}
	if len(merged.HACandidates) == 0 && len(c.HACandidates) > 0 {
		merged.HACandidates = c.HACandidates
	}
	if merged.HAHedgeDelayMs == 0 && c.HAHedgeDelayMs > 0 {
		merged.HAHedgeDelayMs = c.HAHedgeDelayMs
	}
	if merged.RateLimitRPM == 0 && c.RateLimitRPM > 0 {
		merged.RateLimitRPM = c.RateLimitRPM
	}
	if hasExplicitCapabilities(c.Capabilities) {
		merged.Capabilities = mergeCapabilities(merged.Capabilities, c.Capabilities)
	}
	return merged
}

func catalogConfigToConfig(c catalogConfigJSON, modelAPI string) CatalogConfig {
	cfg := CatalogConfig{
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
		HAMode:            strings.TrimSpace(c.HAMode),
		HACandidates:      c.HACandidates,
		HAHedgeDelayMs:    c.HAHedgeDelayMs,
		RateLimitRPM:      c.RateLimitRPM,
		Capabilities:      c.Capabilities,
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
		cfg.CacheSystemPrompt = *c.CacheSystemPrompt
	}
	if c.CacheTools != nil {
		cfg.CacheTools = *c.CacheTools
	}
	if c.CacheMessages != nil {
		cfg.CacheMessages = *c.CacheMessages
	}
	return cfg
}

func hasExplicitCapabilities(c biz.ModelCapabilities) bool {
	return c.Text || c.Vision || c.Audio || c.File || c.ToolCall || c.Cache || c.Thinking || c.TextOnly
}

func mergeCapabilities(base, override biz.ModelCapabilities) biz.ModelCapabilities {
	if override.Text {
		base.Text = true
	}
	if override.Vision {
		base.Vision = true
	}
	if override.Audio {
		base.Audio = true
	}
	if override.File {
		base.File = true
	}
	if override.ToolCall {
		base.ToolCall = true
	}
	if override.Cache {
		base.Cache = true
	}
	if override.Thinking {
		base.Thinking = true
	}
	if override.TextOnly {
		base.TextOnly = true
	}
	return base
}
