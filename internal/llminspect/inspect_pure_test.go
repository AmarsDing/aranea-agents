package llminspect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/pkg/loggateway"
)

func TestDeepSeekOpenAICompatBase(t *testing.T) {
	tests := []struct {
		apiBase string
		want    bool
	}{
		{"", false},
		{"https://api.deepseek.com/v1", true},
		{"https://api.deepseek.com/anthropic", false},
		{"https://api.openai.com/v1", false},
		{"https://API.DEEPSEEK.COM/v1", true},
		{"https://api.deepseek.com/v1/", true},
	}
	for _, tc := range tests {
		got := deepSeekOpenAICompatBase(tc.apiBase)
		if got != tc.want {
			t.Errorf("deepSeekOpenAICompatBase(%q) = %v, want %v", tc.apiBase, got, tc.want)
		}
	}
}

func TestOpenRouterModelsURL(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"", "https://openrouter.ai/api/v1/models"},
		{"https://custom.openrouter.ai/api/v1", "https://custom.openrouter.ai/api/v1/models"},
		{"https://custom.openrouter.ai/api/v1/models", "https://custom.openrouter.ai/api/v1/models"},
		{"https://example.com/other", "https://openrouter.ai/api/v1/models"},
	}
	for _, tc := range tests {
		got := openRouterModelsURL(tc.base)
		if got != tc.want {
			t.Errorf("openRouterModelsURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
}

func TestModelsURL(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/models"},
		{"https://api.anthropic.com/v1/messages", "https://api.anthropic.com/v1/models"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/models"},
		{"not-a-url", "not-a-url"},
	}
	for _, tc := range tests {
		got := modelsURL(tc.base)
		if got != tc.want {
			t.Errorf("modelsURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
}

func TestInferModelSizeLabel(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{"gpt-4o-8b", "8B"},
		{"llama-3-70b", "70B"},
		{"no-size-here", ""},
		{"mixtral 8x7B", "8X7B"},
		{"claude-3.5-sonnet", ""},
	}
	for _, tc := range tests {
		got := inferModelSizeLabel(tc.value)
		if got != tc.want {
			t.Errorf("inferModelSizeLabel(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestAnthropicKnownModelFallback(t *testing.T) {
	in := Input{
		ProviderCode: "ant",
		ProviderType: "anthropic",
		ModelAPIID:   "claude-3-opus",
	}
	result := anthropicKnownModelFallback(in, "test message", loggateway.NewNoop())
	if !result.OK {
		t.Error("expected OK=true")
	}
	if result.Source != "anthropic-known-defaults" {
		t.Errorf("Source = %q, want %q", result.Source, "anthropic-known-defaults")
	}
	if result.ModelAPIID != "claude-3-opus" {
		t.Errorf("ModelAPIID = %q, want %q", result.ModelAPIID, "claude-3-opus")
	}
	if result.Message != "test message" {
		t.Errorf("Message = %q, want %q", result.Message, "test message")
	}
}

func TestAnthropicKnownModelFallbackEmptyProviderType(t *testing.T) {
	in := Input{
		ProviderCode: "ant",
		ProviderType: "",
		ModelAPIID:   "claude-3-opus",
	}
	result := anthropicKnownModelFallback(in, "fallback", loggateway.NewNoop())
	if result.ProviderType != "anthropic" {
		t.Errorf("ProviderType = %q, want %q", result.ProviderType, "anthropic")
	}
}

func TestRunValidationEmptyProviderCode(t *testing.T) {
	_, err := Run(Input{ProviderCode: "", ModelAPIID: "gpt-4o"}, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for empty ProviderCode")
	}
	if !kerrors.IsBadRequest(err) {
		t.Errorf("expected BadRequest error, got %v", err)
	}
}

func TestRunValidationEmptyModelAPIID(t *testing.T) {
	_, err := Run(Input{ProviderCode: "openai", ModelAPIID: ""}, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for empty ModelAPIID")
	}
	if !kerrors.IsBadRequest(err) {
		t.Errorf("expected BadRequest error, got %v", err)
	}
}

func TestRunRoutesOpenRouter(t *testing.T) {
	allowLocalProviderInspect(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "meta-llama/llama-3-8b", "name": "Llama 3 8B"},
			},
		})
	}))
	defer srv.Close()

	result, err := Run(Input{
		ProviderCode: "openrouter",
		ProviderType: "openrouter",
		ModelAPIID:   "meta-llama/llama-3-8b",
		APIBaseURL:   srv.URL + "/api/v1",
		APIKey:       "test",
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Source != "openrouter" {
		t.Fatalf("got %+v", result)
	}
}

func TestRunBedrockNoAWSRegion(t *testing.T) {
	result, err := Run(Input{
		ProviderCode: "aws",
		ProviderType: "bedrock",
		ModelAPIID:   "anthropic.claude-3-sonnet",
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Error("expected OK=false for bedrock without AWSRegion")
	}
}

func TestRunHunyuanNoCredentials(t *testing.T) {
	result, err := Run(Input{
		ProviderCode: "hunyuan",
		ProviderType: "hunyuan",
		ModelAPIID:   "hunyuan-lite",
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Error("expected OK=false for hunyuan without credentials")
	}
}

func TestInspectHunyuanModelNoSecretNoAPIKey(t *testing.T) {
	result, err := inspectHunyuanModel(Input{
		ProviderCode: "hunyuan",
		ProviderType: "hunyuan",
		ModelAPIID:   "hunyuan-lite",
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Error("expected OK=false")
	}
	if result.Message == "" {
		t.Error("expected non-empty Message")
	}
}

func TestInspectBedrockModelNoAWSRegion(t *testing.T) {
	result, err := inspectBedrockModel(Input{
		ProviderCode: "aws",
		ProviderType: "bedrock",
		ModelAPIID:   "anthropic.claude-3-sonnet",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Error("expected OK=false for bedrock without AWSRegion")
	}
}

func TestInspectOpenAICompatibleModelEmptyBaseURL(t *testing.T) {
	result, err := inspectOpenAICompatibleModel(Input{
		ProviderCode: "custom",
		ProviderType: "openai",
		ModelAPIID:   "gpt-4o",
		APIBaseURL:   "",
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Error("expected OK=false for empty APIBaseURL")
	}
}

func TestInspectOpenRouterModelFound(t *testing.T) {
	allowLocalProviderInspect(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "meta-llama/llama-3-8b", "name": "Llama 3 8B"},
			},
		})
	}))
	defer srv.Close()

	result, err := inspectOpenRouterModel(Input{
		ProviderCode: "openrouter",
		ProviderType: "openrouter",
		ModelAPIID:   "meta-llama/llama-3-8b",
		APIBaseURL:   srv.URL + "/api/v1",
		APIKey:       "test",
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Error("expected OK=true")
	}
	if result.Source != "openrouter" {
		t.Errorf("Source = %q, want %q", result.Source, "openrouter")
	}
	if result.ModelAPIID != "meta-llama/llama-3-8b" {
		t.Errorf("ModelAPIID = %q, want %q", result.ModelAPIID, "meta-llama/llama-3-8b")
	}
}

func TestInspectOpenRouterModelNotFound(t *testing.T) {
	allowLocalProviderInspect(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "meta-llama/llama-3-8b", "name": "Llama 3 8B"},
			},
		})
	}))
	defer srv.Close()

	result, err := inspectOpenRouterModel(Input{
		ProviderCode: "openrouter",
		ProviderType: "openrouter",
		ModelAPIID:   "not-found",
		APIBaseURL:   srv.URL + "/api/v1",
		APIKey:       "test",
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Error("expected OK=false for not-found model")
	}
}

func TestInspectAnthropicModelFallbackOnHTTPError(t *testing.T) {
	allowLocalProviderInspect(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	result, err := inspectAnthropicModel(Input{
		ProviderCode: "ant",
		ProviderType: "anthropic",
		ModelAPIID:   "claude-3-opus",
		APIBaseURL:   srv.URL + "/v1",
		APIKey:       "test",
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Error("expected OK=true from fallback")
	}
	if result.Source != "anthropic-known-defaults" {
		t.Errorf("Source = %q, want %q", result.Source, "anthropic-known-defaults")
	}
}

func TestInspectOpenAICompatibleModelFound(t *testing.T) {
	allowLocalProviderInspect(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o", "object": "model", "owned_by": "openai"},
			},
		})
	}))
	defer srv.Close()

	result, err := inspectOpenAICompatibleModel(Input{
		ProviderCode: "custom",
		ProviderType: "openai",
		ModelAPIID:   "gpt-4o",
		APIBaseURL:   srv.URL + "/v1",
		APIKey:       "test",
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Error("expected OK=true")
	}
	if result.Source != "openai-compatible" {
		t.Errorf("Source = %q, want %q", result.Source, "openai-compatible")
	}
	if result.ModelAPIID != "gpt-4o" {
		t.Errorf("ModelAPIID = %q, want %q", result.ModelAPIID, "gpt-4o")
	}
}

func TestInspectOpenAICompatibleModelNotFound(t *testing.T) {
	allowLocalProviderInspect(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o", "object": "model", "owned_by": "openai"},
			},
		})
	}))
	defer srv.Close()

	result, err := inspectOpenAICompatibleModel(Input{
		ProviderCode: "custom",
		ProviderType: "openai",
		ModelAPIID:   "not-found",
		APIBaseURL:   srv.URL + "/v1",
		APIKey:       "test",
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Error("expected OK=false for not-found model")
	}
}
