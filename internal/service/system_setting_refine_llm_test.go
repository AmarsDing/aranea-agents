package service

import (
	"testing"

	v1 "aranea-agents/api/kratos/system_setting/v1"
	"aranea-agents/internal/biz"
)

// PGO-3-PROTO-02: default_refine 经 proto 暴露后，API 边界绝不回传 api_key，
// 仅回传 has_api_key 标记（与 knowledge_embed/web_research/speech 同惯例）。
func TestToProtoRefineLLM_NeverExposesAPIKey(t *testing.T) {
	row := biz.RefineLLMSetting{
		Provider:  "deepseek",
		Model:     "deepseek-v4-flash",
		BaseURL:   "https://api.deepseek.com",
		APIKey:    "sk-secret",
		HasAPIKey: true,
	}
	out := toProtoRefineLLM(row)
	if out.GetProvider() != "deepseek" || out.GetModel() != "deepseek-v4-flash" {
		t.Fatalf("provider/model must pass through: %#v", out)
	}
	if out.GetBaseUrl() != "https://api.deepseek.com" {
		t.Fatalf("base_url must pass through: %#v", out)
	}
	if !out.GetConfigured() {
		t.Fatal("provider+model non-empty must map configured=true")
	}
	if !out.GetHasApiKey() {
		t.Fatal("stored key must map has_api_key=true")
	}
}

func TestToProtoRefineLLM_NotConfigured(t *testing.T) {
	out := toProtoRefineLLM(biz.RefineLLMSetting{Provider: "deepseek"})
	if out.GetConfigured() {
		t.Fatal("model empty must map configured=false")
	}
	if out.GetHasApiKey() {
		t.Fatal("no stored key must map has_api_key=false")
	}
}

func TestHasRefineLLMUpdate(t *testing.T) {
	if hasRefineLLMUpdate(nil) {
		t.Fatal("nil req must be false")
	}
	if hasRefineLLMUpdate(&v1.UpdateSystemSettingsRequest{}) {
		t.Fatal("empty req must be false")
	}
	if !hasRefineLLMUpdate(&v1.UpdateSystemSettingsRequest{RefineLlmProvider: "deepseek"}) {
		t.Fatal("provider must trigger")
	}
	if !hasRefineLLMUpdate(&v1.UpdateSystemSettingsRequest{RefineLlmModel: "m"}) {
		t.Fatal("model must trigger")
	}
	if !hasRefineLLMUpdate(&v1.UpdateSystemSettingsRequest{RefineLlmBaseUrl: "https://x"}) {
		t.Fatal("base_url must trigger")
	}
	if !hasRefineLLMUpdate(&v1.UpdateSystemSettingsRequest{RefineLlmApiKey: "sk-1"}) {
		t.Fatal("non-empty api_key must trigger")
	}
	// 空白 api_key 不触发（避免把存值轮换为空串）。
	if hasRefineLLMUpdate(&v1.UpdateSystemSettingsRequest{RefineLlmApiKey: "   "}) {
		t.Fatal("blank api_key must not trigger")
	}
}
