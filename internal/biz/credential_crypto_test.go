package biz

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestMergeConfigJSONForUpdate_PreservesEncryptedAPIKey(t *testing.T) {
	cur := `{"api_key_enc":"enc-value","api_key_set":true,"provider_type":"openai","api_base_url":"https://api.example.com"}`
	patch := `{"provider_type":"openai","api_base_url":"https://api.example.com/v2"}`
	out, err := mergeConfigJSONForUpdate(cur, patch)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	if m["api_key_enc"] != "enc-value" {
		t.Fatalf("expected preserved api_key_enc, got %#v", m["api_key_enc"])
	}
	if m["api_key_set"] != true {
		t.Fatal("expected api_key_set true")
	}
	if _, ok := m["api_key"]; ok {
		t.Fatal("patch should not introduce plaintext api_key")
	}
}

func TestProcessConfigJSONForStorage_EncryptsAPIKey(t *testing.T) {
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
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["api_key"]; ok {
		t.Fatal("plaintext api_key should be removed")
	}
	if m["api_key_enc"] == nil {
		t.Fatal("expected api_key_enc")
	}
	dec, err := c.DecryptConfigJSONForRuntime(context.Background(), out)
	if err != nil {
		t.Fatalf("decrypt error: %v", err)
	}
	if !strings.Contains(dec, "sk-test") {
		t.Fatalf("decrypt failed: %s", dec)
	}
}
