package provider

import "testing"

func TestInferVariantFromProviderType(t *testing.T) {
	got := InferVariant(ProviderModelConfig{
		ProviderType: "deepseek",
		BaseURL:      "https://example.com/v1",
	})
	if got != "deepseek" {
		t.Fatalf("inferVariant() = %q, want deepseek", got)
	}
}

func TestInferVariantFromExplicitVariant(t *testing.T) {
	got := InferVariant(ProviderModelConfig{
		ProviderType: "openai",
		Variant:      "deepseek",
		BaseURL:      "https://example.com/v1",
	})
	if got != "deepseek" {
		t.Fatalf("inferVariant() = %q, want deepseek", got)
	}
}

func TestInferVariantFromDeepSeekBaseURL(t *testing.T) {
	got := InferVariant(ProviderModelConfig{
		ProviderType: "openai",
		BaseURL:      "https://api.deepseek.com/v1",
	})
	if got != "deepseek" {
		t.Fatalf("inferVariant() = %q, want deepseek", got)
	}
}

func TestModelSupportsImageAttachments_blocksLikelyDeepSeekWithoutCatalog(t *testing.T) {
	if ModelSupportsImageAttachments(t.Context(), nil, "deepseek", "deepseek-chat") {
		t.Fatal("expected DeepSeek-like model to be treated as text-only for images")
	}
	if !ModelSupportsImageAttachments(t.Context(), nil, "openai", "gpt-4o") {
		t.Fatal("expected non-DeepSeek model to allow images when catalog is unavailable")
	}
}
