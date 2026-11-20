package biz

import "testing"

func TestMergeInspectConfigJSON_KeepsStoredKeyOnSameEndpoint(t *testing.T) {
	cfg := `{"provider_type":"openai","api_base_url":"https://api.example.com/v1/","api_key":"sk-stored"}`
	in := InspectMerge{APIBaseURL: "https://API.example.com/v1"}
	if err := mergeInspectConfigJSON(cfg, &in); err != nil {
		t.Fatal(err)
	}
	if in.APIKey != "sk-stored" {
		t.Fatalf("same endpoint should reuse stored key, got %q", in.APIKey)
	}
}

func TestMergeInspectConfigJSON_DoesNotSendStoredKeyElsewhere(t *testing.T) {
	cfg := `{"provider_type":"openai","api_base_url":"https://api.example.com/v1","api_key":"sk-stored","secret_key":"s"}`
	in := InspectMerge{APIBaseURL: "https://attacker.example.net/v1"}
	if err := mergeInspectConfigJSON(cfg, &in); err != nil {
		t.Fatal(err)
	}
	if in.APIKey != "" || in.SecretKey != "" {
		t.Fatalf("stored secrets leaked to a caller-supplied endpoint: key=%q secret=%q", in.APIKey, in.SecretKey)
	}
	if in.ProviderType != "openai" {
		t.Fatalf("non-secret fields should still merge, got provider_type=%q", in.ProviderType)
	}
}
