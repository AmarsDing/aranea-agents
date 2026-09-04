package biz

import (
	"context"
	"encoding/base64"
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

// decryptCredential must fail closed when no AES key is configured: returning
// the ciphertext as "plaintext" lets secrets leak into platform auth flows and
// produces confusing upstream errors (CH-R3).
func TestDecryptCredential_FailsClosedWhenKeyMissing(t *testing.T) {
	_ = os.Unsetenv(envCredentialKey)
	c := NewCredentialCrypto(nil, loggateway.NewNoop())
	enc := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdefPADDING"))

	plain, err := c.decryptCredential(context.Background(), enc)
	if err == nil {
		t.Fatalf("expected error when key missing, got ciphertext passthrough %q", plain)
	}

	ref := channelSecretRefPrefix + enc
	if plain, err := c.DecryptChannelSecretRef(context.Background(), ref); err == nil {
		t.Fatalf("DecryptChannelSecretRef: expected error when key missing, got %q", plain)
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

// 回归（2026-09-03 108 实证）：List 响应注入的统计键随前端整包 PATCH 回写
// 被固化进 glm-4.7 存库 config_json。写入边界必须剥离全部响应装饰键。
func TestProcessConfigJSONForStorage_StripsInjectedStatsKeys(t *testing.T) {
	_ = os.Unsetenv(envCredentialKey) // 无密钥环境同样必须剥离
	c := NewCredentialCrypto(nil, loggateway.NewNoop())

	in := `{"provider_type":"openai","model_hotness_score":0.9,"usage_call_count_30d":12,` +
		`"usage_total_tokens_30d":3456,"usage_cost_micro_usd_30d":78,"success_rate_30d":0.5,` +
		`"avg_latency_ms_30d":123,"p50_latency_ms_30d":100,"p95_latency_ms_30d":200}`
	out, err := c.ProcessConfigJSONForStorage(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range injectedStatsConfigKeys {
		if _, ok := m[k]; ok {
			t.Fatalf("injected stats key %q must be stripped before storage", k)
		}
	}
	if m["provider_type"] != "openai" {
		t.Fatalf("real config key must survive, got %#v", m["provider_type"])
	}
	if len(m) != 1 {
		t.Fatalf("expected only provider_type left, got %d keys: %#v", len(m), m)
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
