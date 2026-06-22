package port

import "testing"

func TestLocalKeyFromMeta_thread(t *testing.T) {
	key := LocalKeyFromMeta("feishu", map[string]string{
		MetaChatID:   "oc_1",
		MetaThreadID: "omt_2",
	})
	if key != "oc_1:omt_2" {
		t.Fatalf("key=%q", key)
	}
}

func TestLocalKeyFromMeta_recipientFallback(t *testing.T) {
	key := LocalKeyFromMeta("telegram", map[string]string{MetaRecipient: "12345"})
	if key != "12345" {
		t.Fatalf("key=%q", key)
	}
}

func TestValidateOutboundMeta_requiredRecipient(t *testing.T) {
	issues := ValidateOutboundMeta("feishu", map[string]string{MetaChatID: "oc_1"})
	if len(issues) != 1 || issues[0] != "missing required meta: recipient" {
		t.Fatalf("issues=%v", issues)
	}
}

func TestValidateOutboundMeta_unknownKey(t *testing.T) {
	issues := ValidateOutboundMeta("feishu", map[string]string{
		MetaRecipient:  "ou_1",
		"custom_field": "x",
	})
	if len(issues) != 1 || issues[0] != "unknown meta key: custom_field" {
		t.Fatalf("issues=%v", issues)
	}
}

func TestValidateOutboundMeta_allowsXPrefix(t *testing.T) {
	issues := ValidateOutboundMeta("feishu", map[string]string{
		MetaRecipient: "ou_1",
		"x_vendor":    "ok",
	})
	if len(issues) != 0 {
		t.Fatalf("issues=%v", issues)
	}
}

func TestNormalizeOutboundMeta_trims(t *testing.T) {
	got := NormalizeOutboundMeta(map[string]string{" recipient ": " ou_1 "})
	if got["recipient"] != "ou_1" {
		t.Fatalf("got=%v", got)
	}
}
