package biz

import (
	"context"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestParseCredentialKeyMaterial_InvalidEnv(t *testing.T) {
	_, err := parseCredentialKeyMaterial("not-a-valid-key")
	if err == nil {
		t.Fatal("expected error for invalid key material")
	}
}

func TestResolveCredentialAESKey_InvalidEnv(t *testing.T) {
	_ = os.Setenv(envCredentialKey, "short")
	defer os.Unsetenv(envCredentialKey)

	_, err := ResolveCredentialAESKey(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when env key is invalid")
	}
}

func TestParseProviderConfigJSON_Invalid(t *testing.T) {
	_, err := parseProviderConfigJSON(`{not-json`)
	if err == nil {
		t.Fatal("expected error for invalid config_json")
	}
}

func TestProcessConfigJSONForStorage_InvalidJSON(t *testing.T) {
	c := NewCredentialCrypto(nil, loggateway.NewNoop())
	_, err := c.ProcessConfigJSONForStorage(context.Background(), `{invalid`)
	if err == nil {
		t.Fatal("expected error for invalid config_json")
	}
}

func TestMergeConfigJSONForUpdate_InvalidPatch(t *testing.T) {
	_, err := mergeConfigJSONForUpdate(`{"api_key_enc":"x"}`, `{bad`)
	if err == nil {
		t.Fatal("expected error for invalid patch config_json")
	}
}

func TestProcessConfigJSONForStorage_RejectsPlaintextWithoutKey(t *testing.T) {
	_ = os.Unsetenv(envCredentialKey)
	c := NewCredentialCrypto(nil, loggateway.NewNoop())

	_, err := c.ProcessConfigJSONForStorage(context.Background(), `{"api_key":"sk-test","provider_type":"openai"}`)
	if err == nil {
		t.Fatal("expected error when storing plaintext without encryption key")
	}
}

func TestProcessConfigJSONForStorage_AllowsMetadataWithoutKey(t *testing.T) {
	_ = os.Unsetenv(envCredentialKey)
	c := NewCredentialCrypto(nil, loggateway.NewNoop())

	out, err := c.ProcessConfigJSONForStorage(context.Background(), `{"provider_type":"openai","api_base_url":"https://api.example.com"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("expected metadata-only config to pass through")
	}
}

func TestProcessConfigJSONForStorage_EncryptsWithEnvKey(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	_ = os.Setenv(envCredentialKey, hex.EncodeToString(key))
	defer os.Unsetenv(envCredentialKey)
	c := NewCredentialCrypto(nil, loggateway.NewNoop())

	out, err := c.ProcessConfigJSONForStorage(context.Background(), `{"api_key":"sk-test","provider_type":"openai"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" || strings.Contains(out, `"api_key":"sk-test"`) {
		t.Fatalf("expected encrypted config, got %s", out)
	}
}

func TestCredentialCrypto_IsAvailable(t *testing.T) {
	_ = os.Unsetenv(envCredentialKey)
	c := NewCredentialCrypto(nil, loggateway.NewNoop())
	if c.IsAvailable() {
		t.Fatal("expected IsAvailable=false with no resolver and no env")
	}

	resolver := func(ctx context.Context) ([]byte, error) { return nil, nil }
	c2 := NewCredentialCrypto(resolver, loggateway.NewNoop())
	if !c2.IsAvailable() {
		t.Fatal("expected IsAvailable=true with resolver")
	}
}
