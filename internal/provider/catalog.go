package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// CatalogConfig is the runtime connection + routing key built from a biz catalog row.
type CatalogConfig struct {
	ProviderType string // from ConfigJSON provider_type
	BaseURL      string // from ConfigJSON api_base_url
	APIKey       string // from ConfigJSON api_key
	ModelAPI     string // row.Model (API id)
}

type catalogConfigJSON struct {
	ProviderType string `json:"provider_type"`
	APIBaseURL   string `json:"api_base_url"`
	APIKey       string `json:"api_key"`
}

// CatalogFromProviderModel parses biz.ProviderModel into a CatalogConfig.
func CatalogFromProviderModel(pm biz.ProviderModel) (CatalogConfig, error) {
	base := strings.TrimSpace(pm.Model)
	if base == "" {
		return CatalogConfig{}, fmt.Errorf("provider model: empty model id")
	}
	var c catalogConfigJSON
	_ = json.Unmarshal([]byte(strings.TrimSpace(pm.ConfigJSON)), &c)
	return CatalogConfig{
		ProviderType: strings.TrimSpace(c.ProviderType),
		BaseURL:      strings.TrimRight(strings.TrimSpace(c.APIBaseURL), "/"),
		APIKey:       strings.TrimSpace(c.APIKey),
		ModelAPI:     base,
	}, nil
}

// CatalogFromEndpoints builds routing configuration from connection fields (e.g. merged from biz + session).
func CatalogFromEndpoints(providerType, baseURL, apiKey string) CatalogConfig {
	return CatalogConfig{
		ProviderType: strings.TrimSpace(providerType),
		BaseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:       strings.TrimSpace(apiKey),
	}
}

// MergeCatalogConfig overlays connection fields from JSON when the struct is partial (e.g. tests).
func MergeCatalogConfig(cfg CatalogConfig, configJSON string) CatalogConfig {
	raw := strings.TrimSpace(configJSON)
	if raw == "" {
		return cfg
	}
	var c catalogConfigJSON
	if json.Unmarshal([]byte(strings.TrimSpace(configJSON)), &c) != nil {
		return cfg
	}
	if cfg.ProviderType == "" {
		cfg.ProviderType = strings.TrimSpace(c.ProviderType)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = strings.TrimRight(strings.TrimSpace(c.APIBaseURL), "/")
	}
	if cfg.APIKey == "" {
		cfg.APIKey = strings.TrimSpace(c.APIKey)
	}
	return cfg
}
