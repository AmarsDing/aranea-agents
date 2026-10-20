package provider

import "testing"

// 治理B：token tailoring 默认启用语义。
// 显式配置优先；未配置但有 context_window_k 时默认启用（截断优于 API 硬错误）。

func TestCatalogConfigToConfig_TailoringDefaultOnWithWindow(t *testing.T) {
	cfg := catalogConfigToConfig(catalogConfigJSON{ContextWindowK: 128}, "deepseek-v4-flash")
	if !cfg.EnableTokenTailoring {
		t.Fatal("expected tailoring enabled by default when context_window_k > 0 and not explicitly configured")
	}
	if cfg.ContextWindow != 128000 {
		t.Fatalf("ContextWindow = %d, want 128000", cfg.ContextWindow)
	}
}

func TestCatalogConfigToConfig_TailoringDefaultOffWithoutWindow(t *testing.T) {
	cfg := catalogConfigToConfig(catalogConfigJSON{}, "m")
	if cfg.EnableTokenTailoring {
		t.Fatal("expected tailoring disabled when no explicit flag and no context_window_k")
	}
}

func TestCatalogConfigToConfig_TailoringExplicitFalseWins(t *testing.T) {
	f := false
	cfg := catalogConfigToConfig(catalogConfigJSON{ContextWindowK: 128, EnableTokenTailoring: &f}, "m")
	if cfg.EnableTokenTailoring {
		t.Fatal("explicit enable_token_tailoring=false must win over default-on")
	}
}

func TestCatalogConfigToConfig_TailoringExplicitTrueWithoutWindow(t *testing.T) {
	tr := true
	cfg := catalogConfigToConfig(catalogConfigJSON{EnableTokenTailoring: &tr}, "m")
	if !cfg.EnableTokenTailoring {
		t.Fatal("explicit enable_token_tailoring=true must win even without context_window_k")
	}
}

// P1 cache-enable（ADR cache-anthropic-default-on，2026-10-20）：
// Anthropic 行未显式配置时三项缓存断点默认开启；显式 false 可逐键关闭；
// 非 anthropic 行不得被默认置位（避免向 OpenAI 兼容网关泄漏 anthropic 语义）。

func TestCatalogConfigToConfig_AnthropicCacheDefaultsOn(t *testing.T) {
	cfg := catalogConfigToConfig(catalogConfigJSON{ProviderType: "anthropic"}, "claude-sonnet-4-5")
	if !cfg.Cache.SystemPrompt || !cfg.Cache.Tools || !cfg.Cache.Messages {
		t.Fatalf("anthropic unset flags must default on, got %+v", cfg.Cache)
	}
}

func TestCatalogConfigToConfig_AnthropicCacheExplicitFalseWins(t *testing.T) {
	f := false
	cfg := catalogConfigToConfig(catalogConfigJSON{
		ProviderType:      "anthropic",
		CacheSystemPrompt: &f,
	}, "claude-2.1")
	if cfg.Cache.SystemPrompt {
		t.Fatal("explicit cache_system_prompt=false must win over anthropic default-on")
	}
	if !cfg.Cache.Tools || !cfg.Cache.Messages {
		t.Fatalf("unset tools/messages still default on for anthropic, got %+v", cfg.Cache)
	}
}

func TestCatalogConfigToConfig_NonAnthropicCacheDefaultsOff(t *testing.T) {
	cfg := catalogConfigToConfig(catalogConfigJSON{ProviderType: "openai"}, "gpt-5")
	if cfg.Cache.SystemPrompt || cfg.Cache.Tools || cfg.Cache.Messages {
		t.Fatalf("non-anthropic unset flags must stay off, got %+v", cfg.Cache)
	}
	// Empty provider_type (legacy rows) must not receive anthropic defaults.
	cfg = catalogConfigToConfig(catalogConfigJSON{}, "m")
	if cfg.Cache.SystemPrompt || cfg.Cache.Tools || cfg.Cache.Messages {
		t.Fatalf("unset provider_type must stay off, got %+v", cfg.Cache)
	}
}

func TestCatalogConfigToConfig_NonAnthropicCacheExplicitTrueWins(t *testing.T) {
	tr := true
	cfg := catalogConfigToConfig(catalogConfigJSON{ProviderType: "openai", CacheTools: &tr}, "m")
	if !cfg.Cache.Tools {
		t.Fatal("explicit cache_tools=true must be honoured for non-anthropic providers")
	}
	if cfg.Cache.SystemPrompt || cfg.Cache.Messages {
		t.Fatalf("other unset flags must stay off for non-anthropic, got %+v", cfg.Cache)
	}
}
