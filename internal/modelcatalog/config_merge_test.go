package modelcatalog

import (
	"encoding/json"
	"testing"
)

func TestMergeCatalogMetadata_envAndDates(t *testing.T) {
	prov := Provider{
		ID:  "openai",
		Doc: "https://docs.example",
		Npm: "@ai-sdk/openai",
		Env: []string{"OPENAI_API_KEY"},
	}
	model := Model{
		ID:          "gpt-4o",
		Family:      "gpt",
		Status:      "active",
		ReleaseDate: "2024-05-13",
		LastUpdated: "2025-01-01",
	}
	out, changed := mergeCatalogMetadata("", prov, model)
	if !changed {
		t.Fatal("expected metadata change")
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(out), &meta); err != nil {
		t.Fatal(err)
	}
	env, ok := meta["catalog_env"].([]any)
	if !ok || len(env) != 1 || env[0] != "OPENAI_API_KEY" {
		t.Fatalf("catalog_env: %#v", meta["catalog_env"])
	}
	if meta["release_date"] != "2024-05-13" || meta["last_updated"] != "2025-01-01" {
		t.Fatalf("dates: %#v", meta)
	}
}

func TestMergeCatalogMetadata_skipsCustom(t *testing.T) {
	_, changed := mergeCatalogMetadata(`{"catalog_source":"custom"}`, Provider{ID: "x"}, Model{ID: "m"})
	if changed {
		t.Fatal("custom metadata should skip")
	}
}

func TestRuntimeOverlayEmbed(t *testing.T) {
	if _, ok := RuntimeProfileFor("huggingface"); !ok {
		t.Fatal("expected huggingface in embedded overlay")
	}
	if _, ok := RuntimeProfileFor("openai"); !ok {
		t.Fatal("expected openai in embedded overlay")
	}
}

func TestProviderUsageQueryCodes(t *testing.T) {
	codes := ProviderUsageQueryCodes("alibaba-cn")
	found := map[string]bool{}
	for _, c := range codes {
		found[c] = true
	}
	if !found["alibaba-cn"] || !found["aliyun-qwen"] {
		t.Fatalf("expected canonical + legacy, got %v", codes)
	}
}

func TestMigrateProviderCode_builtin(t *testing.T) {
	if got := MigrateProviderCode("gemini"); got != "google" {
		t.Fatalf("expected google, got %q", got)
	}
	if got := MigrateProviderCode("openai"); got != "openai" {
		t.Fatalf("unchanged id: %q", got)
	}
}
