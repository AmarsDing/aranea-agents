package biz

import "testing"

func TestApplyKnowledgeEmbedPatch_keepsAPIKeyWhenNotUpdating(t *testing.T) {
	t.Parallel()
	cur := KnowledgeEmbedSetting{
		Provider:  "gemini",
		BaseURL:   "",
		Model:     "gemini-embedding-001",
		Dim:       1536,
		APIKey:    "secret",
		HasAPIKey: true,
	}
	out := ApplyKnowledgeEmbedPatch(cur, "gemini", "", "", "gemini-embedding-001", 1536, false)
	if out.APIKey != "secret" || !out.HasAPIKey {
		t.Fatalf("expected key preserved, got %q has=%v", out.APIKey, out.HasAPIKey)
	}
}

func TestKnowledgeEmbedConfigured(t *testing.T) {
	t.Parallel()
	if KnowledgeEmbedConfigured(KnowledgeEmbedSetting{Provider: "huggingface", BaseURL: "http://localhost:8080"}) != true {
		t.Fatal("huggingface with base url")
	}
	if KnowledgeEmbedConfigured(KnowledgeEmbedSetting{Provider: "gemini", HasAPIKey: true}) != true {
		t.Fatal("gemini with key")
	}
	if KnowledgeEmbedConfigured(KnowledgeEmbedSetting{Provider: "gemini"}) != false {
		t.Fatal("gemini without key")
	}
}
