package modelcatalog

import (
	"encoding/json"
	"strings"
)

func mergeCatalogIntoConfig(existingJSON string, prov Provider, model Model, autoApply string, preserveBaseURL string) (string, bool) {
	cfg := map[string]any{}
	if strings.TrimSpace(existingJSON) != "" {
		_ = json.Unmarshal([]byte(existingJSON), &cfg)
	}
	if shouldSkipCatalogApply(cfg, "") {
		return existingJSON, false
	}

	changed := false
	set := func(key string, val any) {
		cfg[key] = val
		changed = true
	}

	mode := strings.ToLower(strings.TrimSpace(autoApply))
	if mode == "" || mode == "none" {
		return existingJSON, false
	}

	if dn := strings.TrimSpace(prov.Name); dn != "" {
		if cur, _ := cfg["provider_display_name"].(string); cur != dn {
			set("provider_display_name", dn)
		}
	}
	if dn := strings.TrimSpace(model.Name); dn != "" {
		set("model_display_name", dn)
	}

	costUSD, _ := MicroPricingFromModelCost(model.Cost)
	if costUSD.Input > 0 || costUSD.Output > 0 || costUSD.CacheRead > 0 || costUSD.CacheWrite > 0 || costUSD.Reasoning > 0 {
		set("cost", costUSD)
	}

	if mode == "full_spec" || mode == "full_spec_and_runtime_overlay" {
		if model.Limit.Context > 0 {
			ctxK := int(model.Limit.Context / 1000)
			set("context_window_k", ctxK)
		}
		if model.Limit.Output > 0 {
			set("max_output_tokens", model.Limit.Output)
		}
		set("limit", map[string]any{
			"context_tokens": model.Limit.Context,
			"input_tokens":   model.Limit.Input,
			"output_tokens":  model.Limit.Output,
		})
		set("capability_chips", BuildCapabilityChips(model))
		applyInterleavedHints(cfg, model, set)
	} else if mode == "metadata_and_pricing" {
		chips := BuildCapabilityChips(model)
		if len(chips) > 0 {
			set("capability_chips", chips)
		}
	}

	if mode == "full_spec_and_runtime_overlay" {
		if rt, ok := RuntimeProfileFor(prov.ID); ok {
			if rt.ProviderType != "" {
				set("provider_type", rt.ProviderType)
			}
			if rt.Variant != "" {
				set("variant", rt.Variant)
			}
		}
		base := strings.TrimSpace(prov.API)
		if base == "" {
			if rt, ok := RuntimeProfileFor(prov.ID); ok {
				base = strings.TrimSpace(rt.APIBaseURL)
			}
		}
		curBase := strings.TrimSpace(preserveBaseURL)
		if base != "" && (curBase == "" || curBase == base) {
			set("api_base_url", base)
		}
	}

	set("catalog_managed", true)
	set("catalog_source", "models.dev")
	set("metadata_source", "models.dev")

	if !changed {
		return existingJSON, false
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return existingJSON, false
	}
	return string(b), true
}

func shouldSkipCatalogApply(cfg map[string]any, metadataJSON string) bool {
	if src, _ := cfg["catalog_source"].(string); strings.EqualFold(strings.TrimSpace(src), "custom") {
		return true
	}
	if managed, ok := cfg["catalog_managed"].(bool); ok && !managed {
		return true
	}
	if strings.TrimSpace(metadataJSON) != "" {
		var meta map[string]any
		if json.Unmarshal([]byte(metadataJSON), &meta) == nil {
			if src, _ := meta["catalog_source"].(string); strings.EqualFold(strings.TrimSpace(src), "custom") {
				return true
			}
		}
	}
	return false
}

func mergeCatalogMetadata(existingJSON string, prov Provider, model Model) (string, bool) {
	meta := map[string]any{}
	if strings.TrimSpace(existingJSON) != "" {
		_ = json.Unmarshal([]byte(existingJSON), &meta)
	}
	if src, _ := meta["catalog_source"].(string); strings.EqualFold(strings.TrimSpace(src), "custom") {
		return existingJSON, false
	}
	changed := false
	set := func(k string, v any) {
		meta[k] = v
		changed = true
	}
	set("catalog_source", "models.dev")
	if prov.Doc != "" {
		set("catalog_doc", prov.Doc)
	}
	if prov.Npm != "" {
		set("catalog_npm", prov.Npm)
	}
	if model.Family != "" {
		set("catalog_family", model.Family)
	}
	if model.Status != "" {
		set("catalog_status", model.Status)
	}
	if model.Knowledge != "" {
		set("catalog_knowledge", model.Knowledge)
	}
	if len(model.Modalities.Input) > 0 || len(model.Modalities.Output) > 0 {
		set("catalog_modalities", model.Modalities)
	}
	if len(prov.Env) > 0 {
		set("catalog_env", prov.Env)
	}
	if rd := strings.TrimSpace(model.ReleaseDate); rd != "" {
		set("release_date", rd)
	}
	if lu := strings.TrimSpace(model.LastUpdated); lu != "" {
		set("last_updated", lu)
	}
	if !changed {
		return existingJSON, false
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return existingJSON, false
	}
	return string(b), true
}

func extractAPIBaseURL(configJSON string) string {
	var cfg struct {
		APIBaseURL string `json:"api_base_url"`
	}
	_ = json.Unmarshal([]byte(configJSON), &cfg)
	return cfg.APIBaseURL
}

func applyInterleavedHints(cfg map[string]any, model Model, set func(string, any)) {
	if len(model.Interleaved) == 0 || string(model.Interleaved) == "null" {
		return
	}
	var v any
	if json.Unmarshal(model.Interleaved, &v) != nil {
		return
	}
	switch t := v.(type) {
	case bool:
		if !t {
			return
		}
		set("interleaved", true)
		set("reasoning_content_backfill", true)
	case map[string]any:
		set("interleaved", t)
		if field, _ := t["field"].(string); strings.TrimSpace(field) != "" {
			set("interleaved_field", strings.TrimSpace(field))
			set("reasoning_content_backfill", true)
		}
	default:
		set("interleaved", v)
	}
}
