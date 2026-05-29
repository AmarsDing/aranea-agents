package provider

import (
	"net/http"
	"testing"

	"aranea-agents/internal/biz"
)

func TestMapProviderType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"openai", "openai"},
		{"anthropic", "anthropic"},
		{"gemini", "gemini"},
		{"ollama", "ollama"},
		{"hunyuan", "hunyuan"},
		{"huggingface", "huggingface"},
		{"bedrock", "bedrock"},
		{"deepseek", "openai"},
		{"qwen", "openai"},
		{"", "openai"},
		{"unknown", "openai"},
		{"OpenAI", "openai"},
		{"ANTHROPIC", "anthropic"},
		{"Gemini", "gemini"},
		{"Google Gemini", "gemini"},
		{"  ollama  ", "ollama"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := MapProviderType(tt.input)
			if got != tt.want {
				t.Errorf("MapProviderType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildOpenAISpecificOptions(t *testing.T) {
	t.Run("empty_config_returns_nil", func(t *testing.T) {
		opts := buildOpenAISpecificOptions(CatalogConfig{})
		if opts != nil {
			t.Fatalf("expected nil for empty config, got %v", opts)
		}
	})

	t.Run("optimize_for_cache", func(t *testing.T) {
		opts := buildOpenAISpecificOptions(CatalogConfig{OptimizeForCache: true})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with OptimizeForCache")
		}
	})

	t.Run("reasoning_backfill", func(t *testing.T) {
		opts := buildOpenAISpecificOptions(CatalogConfig{ReasoningBackfill: true})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with ReasoningBackfill")
		}
	})

	t.Run("show_tool_call_delta", func(t *testing.T) {
		opts := buildOpenAISpecificOptions(CatalogConfig{ShowToolCallDelta: true})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with ShowToolCallDelta")
		}
	})

	t.Run("context_window", func(t *testing.T) {
		opts := buildOpenAISpecificOptions(CatalogConfig{ContextWindow: 128000})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with ContextWindow")
		}
	})

	t.Run("multiple_flags", func(t *testing.T) {
		opts := buildOpenAISpecificOptions(CatalogConfig{
			OptimizeForCache:  true,
			ReasoningBackfill: true,
		})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with multiple flags")
		}
	})
}

func TestBuildAnthropicSpecificOptions(t *testing.T) {
	t.Run("empty_config_returns_nil", func(t *testing.T) {
		opts := buildAnthropicSpecificOptions(CatalogConfig{})
		if opts != nil {
			t.Fatalf("expected nil for empty config, got %v", opts)
		}
	})

	t.Run("cache_system_prompt", func(t *testing.T) {
		opts := buildAnthropicSpecificOptions(CatalogConfig{CacheSystemPrompt: true})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with CacheSystemPrompt")
		}
	})

	t.Run("cache_tools", func(t *testing.T) {
		opts := buildAnthropicSpecificOptions(CatalogConfig{CacheTools: true})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with CacheTools")
		}
	})

	t.Run("cache_messages", func(t *testing.T) {
		opts := buildAnthropicSpecificOptions(CatalogConfig{CacheMessages: true})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with CacheMessages")
		}
	})

	t.Run("show_tool_call_delta", func(t *testing.T) {
		opts := buildAnthropicSpecificOptions(CatalogConfig{ShowToolCallDelta: true})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with ShowToolCallDelta")
		}
	})

	t.Run("multiple_cache_flags", func(t *testing.T) {
		opts := buildAnthropicSpecificOptions(CatalogConfig{
			CacheSystemPrompt: true,
			CacheTools:        true,
			CacheMessages:     true,
		})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with multiple cache flags")
		}
	})
}

func TestBuildGeminiSpecificOptions(t *testing.T) {
	t.Run("empty_config_returns_nil", func(t *testing.T) {
		opts := buildGeminiSpecificOptions(CatalogConfig{}, nil)
		if opts != nil {
			t.Fatalf("expected nil for empty config, got %v", opts)
		}
	})

	t.Run("with_api_key", func(t *testing.T) {
		opts := buildGeminiSpecificOptions(CatalogConfig{APIKey: "test-key"}, nil)
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with APIKey")
		}
	})

	t.Run("with_context_window", func(t *testing.T) {
		opts := buildGeminiSpecificOptions(CatalogConfig{ContextWindow: 1000000}, nil)
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with ContextWindow")
		}
	})

	t.Run("with_roundtrip_transport", func(t *testing.T) {
		rt := &RoundTrip{
			HTTP: &http.Client{},
		}
		opts := buildGeminiSpecificOptions(CatalogConfig{APIKey: "key"}, rt)
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with roundtrip")
		}
	})
}

func TestBuildOllamaSpecificOptions(t *testing.T) {
	t.Run("empty_config_returns_nil", func(t *testing.T) {
		opts := buildOllamaSpecificOptions(CatalogConfig{})
		if opts != nil {
			t.Fatalf("expected nil for empty config, got %v", opts)
		}
	})

	t.Run("with_keep_alive", func(t *testing.T) {
		opts := buildOllamaSpecificOptions(CatalogConfig{KeepAliveMinutes: 30})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with KeepAliveMinutes")
		}
	})

	t.Run("with_context_window", func(t *testing.T) {
		opts := buildOllamaSpecificOptions(CatalogConfig{ContextWindow: 8192})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with ContextWindow")
		}
	})

	t.Run("multiple_options", func(t *testing.T) {
		opts := buildOllamaSpecificOptions(CatalogConfig{
			KeepAliveMinutes: 30,
			ContextWindow:    8192,
		})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with multiple settings")
		}
	})
}

func TestBuildHunyuanSpecificOptions(t *testing.T) {
	t.Run("empty_config_returns_nil", func(t *testing.T) {
		opts := buildHunyuanSpecificOptions(CatalogConfig{})
		if opts != nil {
			t.Fatalf("expected nil for empty config, got %v", opts)
		}
	})

	t.Run("with_secret_id", func(t *testing.T) {
		opts := buildHunyuanSpecificOptions(CatalogConfig{SecretID: "sid-123"})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with SecretID")
		}
	})

	t.Run("with_secret_key", func(t *testing.T) {
		opts := buildHunyuanSpecificOptions(CatalogConfig{SecretKey: "skey-456"})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with SecretKey")
		}
	})

	t.Run("with_context_window", func(t *testing.T) {
		opts := buildHunyuanSpecificOptions(CatalogConfig{ContextWindow: 32768})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with ContextWindow")
		}
	})

	t.Run("with_secret_id_and_key", func(t *testing.T) {
		opts := buildHunyuanSpecificOptions(CatalogConfig{
			SecretID:      "sid-123",
			SecretKey:     "skey-456",
			ContextWindow: 32768,
		})
		if len(opts) == 0 {
			t.Fatal("expected non-nil options with SecretID, SecretKey and ContextWindow")
		}
	})

	t.Run("whitespace_only_secret_id_returns_nil", func(t *testing.T) {
		opts := buildHunyuanSpecificOptions(CatalogConfig{SecretID: "   "})
		if opts != nil {
			t.Fatalf("expected nil for whitespace-only SecretID, got %v", opts)
		}
	})
}

func TestCapabilitiesForProviderModel(t *testing.T) {
	t.Run("explicit_capabilities_returned_directly", func(t *testing.T) {
		pm := biz.ProviderModel{
			CapabilitiesExplicit: true,
			Capabilities: biz.ModelCapabilities{
				Text:     true,
				Vision:   true,
				ToolCall: true,
			},
		}
		caps := CapabilitiesForProviderModel(pm)
		if !caps.Text || !caps.Vision || !caps.ToolCall {
			t.Fatal("expected explicit capabilities to be returned directly")
		}
		if caps.Audio || caps.File || caps.Cache || caps.Thinking || caps.TextOnly {
			t.Fatal("expected unset explicit capabilities to remain false")
		}
	})

	t.Run("default_capabilities_when_not_explicit_and_no_config", func(t *testing.T) {
		pm := biz.ProviderModel{
			CapabilitiesExplicit: false,
		}
		caps := CapabilitiesForProviderModel(pm)
		if !caps.Text || !caps.File || !caps.ToolCall {
			t.Fatal("expected default capabilities (Text, File, ToolCall) when not explicit")
		}
	})

	t.Run("deepseek_provider_gets_text_only", func(t *testing.T) {
		pm := biz.ProviderModel{
			Provider:             "deepseek",
			Model:                "deepseek-chat",
			CapabilitiesExplicit: false,
		}
		caps := CapabilitiesForProviderModel(pm)
		if !caps.Text || !caps.TextOnly {
			t.Fatal("expected DeepSeek to have Text and TextOnly capabilities")
		}
		if caps.Vision || caps.Audio {
			t.Fatal("expected DeepSeek to not have Vision or Audio capabilities")
		}
	})

	t.Run("deepseek_in_model_name_gets_text_only", func(t *testing.T) {
		pm := biz.ProviderModel{
			Provider:             "openai",
			Model:                "deepseek-v3",
			CapabilitiesExplicit: false,
		}
		caps := CapabilitiesForProviderModel(pm)
		if !caps.TextOnly {
			t.Fatal("expected model name containing 'deepseek' to have TextOnly")
		}
	})

	t.Run("cache_inferred_from_config_flags", func(t *testing.T) {
		pm := biz.ProviderModel{
			Provider:             "anthropic",
			Model:                "claude-3-5-sonnet",
			ConfigJSON:           `{"cache_system_prompt": true}`,
			CapabilitiesExplicit: false,
		}
		caps := CapabilitiesForProviderModel(pm)
		if !caps.Cache {
			t.Fatal("expected Cache to be inferred from CacheSystemPrompt config flag")
		}
	})

	t.Run("thinking_inferred_from_reasoning_backfill", func(t *testing.T) {
		pm := biz.ProviderModel{
			Provider:             "openai",
			Model:                "o1",
			ConfigJSON:           `{"reasoning_content_backfill": true}`,
			CapabilitiesExplicit: false,
		}
		caps := CapabilitiesForProviderModel(pm)
		if !caps.Thinking {
			t.Fatal("expected Thinking to be inferred from ReasoningBackfill config flag")
		}
	})

	t.Run("explicit_capabilities_preserved_when_set", func(t *testing.T) {
		pm := biz.ProviderModel{
			Provider:             "openai",
			Model:                "gpt-4o",
			ConfigJSON:           `{"capabilities": {"vision": true, "cache": true}, "optimize_for_cache": true}`,
			CapabilitiesExplicit: false,
		}
		caps := CapabilitiesForProviderModel(pm)
		if !caps.Vision {
			t.Fatal("expected Vision from config capabilities")
		}
		if !caps.Cache {
			t.Fatal("expected Cache inferred from OptimizeForCache + capabilities cache")
		}
	})
}
