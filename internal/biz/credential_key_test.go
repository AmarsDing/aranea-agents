package biz

import (
	"context"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
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

	resolver := CredentialKeyResolver(func(ctx context.Context) ([]byte, error) { return nil, nil })
	c2 := NewCredentialCrypto(resolver, loggateway.NewNoop())
	if !c2.IsAvailable() {
		t.Fatal("expected IsAvailable=true with resolver")
	}
}

// TestCredentialCrypto_RequireKeyForSecret G-38 ⑤ 铁律链钉（W5）：无 key
// 拒写硬错（不静默明文降级）、非法长度拒、32B 通过、resolver 故障透传。
func TestCredentialCrypto_RequireKeyForSecret(t *testing.T) {
	_ = os.Unsetenv(envCredentialKey)
	// 无 resolver 无 env → 硬错
	c := NewCredentialCrypto(nil, loggateway.NewNoop())
	if err := c.RequireKeyForSecret(context.Background()); err == nil {
		t.Fatal("no key must hard-fail")
	}
	// env 非法长度 → 硬错
	_ = os.Setenv(envCredentialKey, "deadbeef")
	if err := c.RequireKeyForSecret(context.Background()); err == nil {
		t.Fatal("non-32-byte key must hard-fail")
	}
	// resolver 返回错误 → 透传硬错
	badResolver := CredentialKeyResolver(func(ctx context.Context) ([]byte, error) {
		return nil, apierror.Internal("CREDENTIAL_KEY", "resolver down")
	})
	c2 := NewCredentialCrypto(badResolver, loggateway.NewNoop())
	if err := c2.RequireKeyForSecret(context.Background()); err == nil {
		t.Fatal("resolver error must hard-fail")
	}
	// 32B 合法 key → 通过
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	_ = os.Setenv(envCredentialKey, hex.EncodeToString(key))
	defer os.Unsetenv(envCredentialKey)
	if err := c.RequireKeyForSecret(context.Background()); err != nil {
		t.Fatalf("valid 32-byte key must pass: %v", err)
	}
}
