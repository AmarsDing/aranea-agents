package biz

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	mcpconfig "aranea-agents/internal/mcp/config"
	"aranea-agents/pkg/loggateway"
)

func TestProcessMCPConfigJSONForStorage_EncryptsSecrets(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	_ = os.Setenv(envCredentialKey, hex.EncodeToString(key))
	defer os.Unsetenv(envCredentialKey)
	c := NewCredentialCrypto(nil, loggateway.NewNoop())

	raw := `{"transport":"stdio","command":"npx","auth":{"api_key":"sk-live","client_secret":"cs-secret"},"headers":{"Authorization":"Bearer tok","X-Custom":"ok"}}`
	stored, err := c.ProcessMCPConfigJSONForStorage(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"sk-live", "cs-secret", "Bearer tok"} {
		if strings.Contains(stored, secret) {
			t.Fatalf("plaintext %q leaked into storage: %s", secret, stored)
		}
	}
	if !strings.Contains(stored, channelSecretRefPrefix) {
		t.Fatalf("expected enc: refs in stored config: %s", stored)
	}
	if !strings.Contains(stored, `"X-Custom":"ok"`) {
		t.Fatalf("non-secret header should remain: %s", stored)
	}

	redacted := mcpconfig.RedactConfigJSON(stored)
	if strings.Contains(redacted, channelSecretRefPrefix) {
		t.Fatalf("API redact should hide enc: ciphertext: %s", redacted)
	}
	for _, secret := range []string{"sk-live", "cs-secret", "Bearer tok"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("secret %q leaked in redacted API payload: %s", secret, redacted)
		}
	}

	runtime, err := c.DecryptMCPConfigJSONForRuntime(context.Background(), stored)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := mcpconfig.ParseServerConfigJSON(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Auth.APIKey != "sk-live" || sc.Auth.ClientSecret != "cs-secret" {
		t.Fatalf("runtime decrypt mismatch: api_key=%q client_secret=%q", sc.Auth.APIKey, sc.Auth.ClientSecret)
	}
	if sc.Headers["Authorization"] != "Bearer tok" {
		t.Fatalf("runtime Authorization=%q", sc.Headers["Authorization"])
	}
}

func TestMCPServerUsecase_CreateEncryptsAndGetPathRedacts(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 3)
	}
	_ = os.Setenv(envCredentialKey, hex.EncodeToString(key))
	defer os.Unsetenv(envCredentialKey)
	crypto := NewCredentialCrypto(nil, loggateway.NewNoop())
	repo := &stubMCPRepo{}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, crypto)

	created, err := uc.Create(context.Background(), MCPServer{
		Key:        "srv1",
		Name:       "Server 1",
		ConfigJSON: `{"transport":"stdio","command":"npx","auth":{"api_key":"sk-store-me"},"headers":{"Authorization":"Bearer tok"}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(created.ConfigJSON, "sk-store-me") {
		t.Fatalf("Create should encrypt before store: %s", created.ConfigJSON)
	}
	if !strings.Contains(created.ConfigJSON, channelSecretRefPrefix) {
		t.Fatalf("expected enc: in stored config: %s", created.ConfigJSON)
	}

	// Simulate API layer redaction.
	apiJSON := mcpconfig.RedactConfigJSON(created.ConfigJSON)
	if strings.Contains(apiJSON, "sk-store-me") || strings.Contains(apiJSON, channelSecretRefPrefix) {
		t.Fatalf("redacted API payload leaked secret: %s", apiJSON)
	}

	runtime, err := uc.PrepareConfigJSONForRuntime(context.Background(), created.ConfigJSON)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(runtime), &m); err != nil {
		t.Fatal(err)
	}
	auth, _ := m["auth"].(map[string]any)
	if auth["api_key"] != "sk-store-me" {
		t.Fatalf("runtime decrypt api_key=%v", auth["api_key"])
	}
}

func TestMCPServerUsecase_Create_NilCryptoStoresPlaintext(t *testing.T) {
	repo := &stubMCPRepo{}
	uc := NewMCPServerUsecase(repo, stubMCPCredRepo{}, nil, mcpMetadataAdapter{}, nil)
	created, err := uc.Create(context.Background(), MCPServer{
		Key:        "srv-nil-crypto",
		Name:       "No Crypto",
		ConfigJSON: `{"transport":"stdio","command":"npx","auth":{"api_key":"plain-key"}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(created.ConfigJSON, "plain-key") {
		t.Fatalf("nil crypto should leave plaintext: %s", created.ConfigJSON)
	}
}

func TestMergeMCPConfigJSONForUpdate_PreservesEncryptedOnRedactedPatch(t *testing.T) {
	cur := `{"transport":"sse","auth":{"api_key":"enc:preserved"}}`
	patch := `{"transport":"sse","auth":{"api_key":"******","client_id":"cid"}}`
	out, err := MergeMCPConfigJSONForUpdate(cur, patch)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	auth, _ := m["auth"].(map[string]any)
	if auth["api_key"] != "enc:preserved" {
		t.Fatalf("expected preserved enc ref, got %#v", auth["api_key"])
	}
	if auth["client_id"] != "cid" {
		t.Fatalf("client_id=%v", auth["client_id"])
	}
}
