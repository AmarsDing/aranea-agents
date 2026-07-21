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
