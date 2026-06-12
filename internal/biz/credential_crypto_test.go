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

func TestMergeConfigJSONForUpdate_NewAPIKeyReplacesEncrypted(t *testing.T) {
	cur := `{"api_key_enc":"enc-old","api_key_set":true,"provider_type":"openai"}`
	patch := `{"api_key":"sk-new","provider_type":"openai"}`
	out, err := mergeConfigJSONForUpdate(cur, patch)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	if m["api_key"] != "sk-new" {
		t.Fatalf("expected new api_key, got %#v", m["api_key"])
	}
	if _, ok := m["api_key_enc"]; ok {
		t.Fatal("old api_key_enc should not be present when new api_key provided")
	}
}

func TestMergeConfigJSONForUpdate_EmptyPatchReturnsCurrent(t *testing.T) {
	cur := `{"api_key_enc":"enc-value","provider_type":"openai"}`
	out, err := mergeConfigJSONForUpdate(cur, "")
	if err != nil {
		t.Fatal(err)
	}
	if out != cur {
		t.Fatalf("expected current unchanged, got %s", out)
	}
}

func TestMergeConfigJSONForUpdate_NilCurrentWithPatch(t *testing.T) {
	patch := `{"api_key":"sk-test","provider_type":"anthropic"}`
	out, err := mergeConfigJSONForUpdate("", patch)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	if m["api_key"] != "sk-test" {
		t.Fatalf("expected api_key from patch, got %#v", m["api_key"])
	}
}

func TestMergeConfigJSONForUpdate_PreservesHACandidateSecrets(t *testing.T) {
	cur := `{"api_key_enc":"enc-main","ha_candidates":[{"name":"fallback","api_key_enc":"enc-ha"}]}`
	patch := `{"ha_candidates":[{"name":"fallback","base_url":"https://fallback.example.com"}]}`
	out, err := mergeConfigJSONForUpdate(cur, patch)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	cands, ok := m["ha_candidates"].([]any)
	if !ok || len(cands) == 0 {
		t.Fatal("expected ha_candidates")
	}
	cm, ok := cands[0].(map[string]any)
	if !ok {
		t.Fatal("expected candidate to be a map")
	}
	if cm["api_key_enc"] != "enc-ha" {
		t.Fatalf("expected preserved ha api_key_enc, got %#v", cm["api_key_enc"])
	}
	if _, ok := cm["api_key"]; ok {
		t.Fatal("patch should not introduce plaintext api_key in ha_candidates")
	}
}

func TestMergeConfigJSONForUpdate_HACandidateNewAPIKey(t *testing.T) {
	cur := `{"ha_candidates":[{"name":"fallback","api_key_enc":"enc-old"}]}`
	patch := `{"ha_candidates":[{"name":"fallback","api_key":"sk-ha-new"}]}`
	out, err := mergeConfigJSONForUpdate(cur, patch)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	cands := m["ha_candidates"].([]any)
	cm := cands[0].(map[string]any)
	if cm["api_key"] != "sk-ha-new" {
		t.Fatalf("expected new ha api_key, got %#v", cm["api_key"])
	}
}

func TestMergeConfigJSONForUpdate_PreservesSecretKey(t *testing.T) {
	cur := `{"secret_key_enc":"enc-secret","secret_id":"sid-123","provider_type":"hunyuan"}`
	patch := `{"provider_type":"hunyuan","api_base_url":"https://hunyuan.example.com"}`
	out, err := mergeConfigJSONForUpdate(cur, patch)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	if m["secret_key_enc"] != "enc-secret" {
		t.Fatalf("expected preserved secret_key_enc, got %#v", m["secret_key_enc"])
	}
	if m["secret_id"] != "sid-123" {
		t.Fatalf("expected preserved secret_id, got %#v", m["secret_id"])
	}
}

func TestMergeConfigJSONForUpdate_InvalidPatchJSON(t *testing.T) {
	_, err := mergeConfigJSONForUpdate("", `{invalid`)
	if err == nil {
		t.Fatal("expected error for invalid patch JSON")
	}
}
