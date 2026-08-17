package modelregistry

import (
	"os"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestSafeProviderLogoID(t *testing.T) {
	if safeProviderLogoID("openai") != "openai" {
		t.Fatal("expected openai")
	}
	if safeProviderLogoID("../x") != "" {
		t.Fatal("path traversal rejected")
	}
}

func TestStoreProviderLogoRoundTrip(t *testing.T) {
	st := NewStore(t.TempDir(), loggateway.NewNoop())
	if err := st.ensureLogosDir(); err != nil {
		t.Fatal(err)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	path := st.ProviderLogoPath("demo")
	if err := os.WriteFile(path, svg, 0o644); err != nil {
		t.Fatal(err)
	}
	if !st.HasProviderLogo("demo") {
		t.Fatal("expected cached logo")
	}
	body, err := st.ReadProviderLogo("demo")
	if err != nil || string(body) != string(svg) {
		t.Fatalf("read logo: err=%v body=%q", err, body)
	}
}

func TestHasProviderLogo_MemorySet(t *testing.T) {
	st := NewStore(t.TempDir(), loggateway.NewNoop())
	if st.HasProviderLogo("demo") {
		t.Fatal("missing logos dir must be false")
	}
	if err := st.ensureLogosDir(); err != nil {
		t.Fatal(err)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	if err := os.WriteFile(st.ProviderLogoPath("demo"), svg, 0o644); err != nil {
		t.Fatal(err)
	}
	// 先前空扫描已入缓存；写入后必须主动失效，列表才能看见新文件。
	st.invalidateLogoCache()
	if !st.HasProviderLogo("demo") {
		t.Fatal("expected demo after invalidate")
	}
	if st.HasProviderLogo("missing") {
		t.Fatal("missing id must stay false")
	}
	for i := 0; i < 64; i++ {
		if !st.HasProviderLogo("demo") || st.HasProviderLogo("missing") {
			t.Fatal("repeated lookups must hit memory set")
		}
	}
	if err := os.WriteFile(st.ProviderLogoPath("other"), svg, 0o644); err != nil {
		t.Fatal(err)
	}
	if st.HasProviderLogo("other") {
		t.Fatal("new file must not appear until cache invalidation")
	}
	st.invalidateLogoCache()
	if !st.HasProviderLogo("other") {
		t.Fatal("expected other after invalidate")
	}
}

func TestProviderLogoURL(t *testing.T) {
	if ProviderLogoURL("openai") != "/v1/model-catalog/logos/openai" {
		t.Fatal(ProviderLogoURL("openai"))
	}
}

func TestValidateLogoSourceURL(t *testing.T) {
	if err := ValidateLogoSourceURL("https://models.dev/logos/openai.svg"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLogoSourceURL("https://evil.test/logos/x.svg"); err == nil {
		t.Fatal("expected reject")
	}
}
