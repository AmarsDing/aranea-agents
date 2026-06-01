package modelregistry

import "testing"

func TestValidateDirectorySourceURL(t *testing.T) {
	if err := ValidateDirectorySourceURL("https://models.dev/api.json"); err != nil {
		t.Fatalf("models.dev should be allowed: %v", err)
	}
	if err := ValidateDirectorySourceURL("https://models.dev/models.json"); err == nil {
		t.Fatal("models.json should be rejected")
	}
	if err := ValidateDirectorySourceURL("http://models.dev/api.json"); err == nil {
		t.Fatal("http should be rejected")
	}
	if err := ValidateDirectorySourceURL("https://127.0.0.1/x"); err == nil {
		t.Fatal("loopback should be rejected")
	}
}

func TestNormalizePolicy(t *testing.T) {
	p, err := NormalizePolicy(Policy{SyncPolicy: "scheduled", AutoApply: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if p.SourceURL != DefaultPolicy().SourceURL {
		t.Fatalf("default url: %q", p.SourceURL)
	}
	if _, err := NormalizePolicy(Policy{SourceURL: "http://evil.test/x", SyncPolicy: "scheduled", AutoApply: "none"}); err == nil {
		t.Fatal("expected invalid url error")
	}
	if _, err := NormalizePolicy(Policy{SourceURL: "https://models.dev/api.json", SyncPolicy: "bogus", AutoApply: "none"}); err == nil {
		t.Fatal("expected invalid sync_policy")
	}
}

func TestShouldSkipDirectoryApply(t *testing.T) {
	if !shouldSkipDirectoryApply(map[string]any{"catalog_source": "custom"}, "") {
		t.Fatal("custom source should skip")
	}
	if !shouldSkipDirectoryApply(map[string]any{"catalog_managed": false}, "") {
		t.Fatal("catalog_managed=false should skip")
	}
	if !shouldSkipDirectoryApply(map[string]any{}, `{"catalog_source":"custom"}`) {
		t.Fatal("metadata custom should skip")
	}
	if shouldSkipDirectoryApply(map[string]any{"catalog_managed": true, "catalog_source": "models.dev"}, "") {
		t.Fatal("catalog managed should not skip")
	}
}
