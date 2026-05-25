package modelcatalog

import (
	"os"
	"testing"
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
	st := NewStore(t.TempDir())
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
