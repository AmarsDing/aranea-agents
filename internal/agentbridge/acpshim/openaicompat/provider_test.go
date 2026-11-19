package openaicompat

import "testing"

// TestParseProvider 覆盖通用 shim 子命令 --provider 参数的解析：
// 已知 vendor 精确命中，未知/空回退 generic（无语义差异，仅日志标识）。
func TestParseProvider(t *testing.T) {
	cases := []struct {
		in   string
		want Provider
	}{
		{"deepseek", ProviderDeepseek},
		{"DeepSeek", ProviderDeepseek},
		{"grok", ProviderGrok},
		{" GROK ", ProviderGrok},
		{"generic", ProviderGeneric},
		{"", ProviderGeneric},
		{"moonshot", ProviderGeneric},
	}
	for _, c := range cases {
		if got := ParseProvider(c.in); got != c.want {
			t.Errorf("ParseProvider(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestProviderString_Generic 保证 generic 分支日志标识稳定（log 检索锚点）。
func TestProviderString_Generic(t *testing.T) {
	if got := ProviderGeneric.String(); got != "generic" {
		t.Fatalf("ProviderGeneric.String() = %q, want generic", got)
	}
}

// TestFallbackNonStream_Defaults 回退档默认：deepseek 开、grok/generic 关；
// WithNonStreamFallback 可显式覆盖（通用 shim --fallback-non-stream）。
func TestFallbackNonStream_Defaults(t *testing.T) {
	if !New("https://x", "k", ProviderDeepseek, nil, nil).fallbackNonStream {
		t.Fatal("deepseek default fallback should be on")
	}
	if New("https://x", "k", ProviderGrok, nil, nil).fallbackNonStream {
		t.Fatal("grok default fallback should be off")
	}
	c := New("https://x", "k", ProviderGeneric, nil, nil)
	if c.fallbackNonStream {
		t.Fatal("generic default fallback should be off")
	}
	if !c.WithNonStreamFallback(true).fallbackNonStream {
		t.Fatal("WithNonStreamFallback(true) should enable")
	}
	if New("https://x", "k", ProviderDeepseek, nil, nil).WithNonStreamFallback(false).fallbackNonStream {
		t.Fatal("WithNonStreamFallback(false) should disable deepseek default")
	}
}
