package knowledge

import (
	"testing"
)

func TestEmbedConfigured(t *testing.T) {
	tests := []struct {
		name string
		s    EmbedSetting
		want bool
	}{
		{"empty provider", EmbedSetting{}, false},
		{"whitespace provider", EmbedSetting{Provider: "  "}, false},
		{"ollama always configured", EmbedSetting{Provider: "ollama"}, true},
		{"ollama with spaces trimmed", EmbedSetting{Provider: " ollama "}, true},
		{"huggingface with base url", EmbedSetting{Provider: "huggingface", BaseURL: "http://hf.io"}, true},
		{"huggingface without base url", EmbedSetting{Provider: "huggingface"}, false},
		{"huggingface with whitespace base url", EmbedSetting{Provider: "huggingface", BaseURL: "  "}, false},
		{"openai with api key", EmbedSetting{Provider: "openai", HasAPIKey: true}, true},
		{"openai without api key", EmbedSetting{Provider: "openai", HasAPIKey: false}, false},
		{"azure with api key", EmbedSetting{Provider: "azure", HasAPIKey: true}, true},
		{"azure without api key", EmbedSetting{Provider: "azure"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EmbedConfigured(tt.s); got != tt.want {
				t.Errorf("EmbedConfigured(%+v) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestApplyEmbedPatch(t *testing.T) {
	tests := []struct {
		name        string
		cur         EmbedSetting
		provider    string
		baseURL     string
		apiKey      string
		model       string
		dim         int
		updateAPIKey bool
		want        EmbedSetting
	}{
		{
			"patch provider only",
			EmbedSetting{Provider: "ollama", Model: "old", Dim: 512},
			"openai", "", "", "", 0, false,
			EmbedSetting{Provider: "openai", BaseURL: "", Model: "", Dim: 512, HasAPIKey: false},
		},
		{
			"patch all fields",
			EmbedSetting{},
			"ollama", "http://localhost:11434/", "sk-123", "nomic", 768, true,
			EmbedSetting{Provider: "ollama", BaseURL: "http://localhost:11434", Model: "nomic", Dim: 768, APIKey: "sk-123", HasAPIKey: true},
		},
		{
			"empty provider does not override",
			EmbedSetting{Provider: "openai"},
			"  ", "", "", "", 0, false,
			EmbedSetting{Provider: "openai", BaseURL: "", Model: "", Dim: 0, HasAPIKey: false},
		},
		{
			"base url trailing slash stripped",
			EmbedSetting{},
			"", "http://api.example.com/v1/", "", "", 0, false,
			EmbedSetting{BaseURL: "http://api.example.com/v1"},
		},
		{
			"base url whitespace trimmed",
			EmbedSetting{},
			"", "  http://api.example.com  ", "", "", 0, false,
			EmbedSetting{BaseURL: "http://api.example.com"},
		},
		{
			"dim zero does not override",
			EmbedSetting{Dim: 1536},
			"", "", "", "", 0, false,
			EmbedSetting{Dim: 1536},
		},
		{
			"dim positive overrides",
			EmbedSetting{Dim: 1536},
			"", "", "", "", 768, false,
			EmbedSetting{Dim: 768},
		},
		{
			"api key update with non empty key",
			EmbedSetting{APIKey: "old", HasAPIKey: true},
			"", "", "new-key", "", 0, true,
			EmbedSetting{APIKey: "new-key", HasAPIKey: true},
		},
		{
			"api key update false does not set",
			EmbedSetting{APIKey: "old", HasAPIKey: true},
			"", "", "new-key", "", 0, false,
			EmbedSetting{APIKey: "old", HasAPIKey: true},
		},
		{
			"api key update with empty key does not set",
			EmbedSetting{APIKey: "old", HasAPIKey: true},
			"", "", "  ", "", 0, true,
			EmbedSetting{APIKey: "old", HasAPIKey: true},
		},
		{
			"model trimmed",
			EmbedSetting{Model: "old"},
			"", "", "", "  new-model  ", 0, false,
			EmbedSetting{Model: "new-model"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyEmbedPatch(tt.cur, tt.provider, tt.baseURL, tt.apiKey, tt.model, tt.dim, tt.updateAPIKey)
			if got.Provider != tt.want.Provider {
				t.Errorf("Provider = %q, want %q", got.Provider, tt.want.Provider)
			}
			if got.BaseURL != tt.want.BaseURL {
				t.Errorf("BaseURL = %q, want %q", got.BaseURL, tt.want.BaseURL)
			}
			if got.Model != tt.want.Model {
				t.Errorf("Model = %q, want %q", got.Model, tt.want.Model)
			}
			if got.Dim != tt.want.Dim {
				t.Errorf("Dim = %d, want %d", got.Dim, tt.want.Dim)
			}
			if got.APIKey != tt.want.APIKey {
				t.Errorf("APIKey = %q, want %q", got.APIKey, tt.want.APIKey)
			}
			if got.HasAPIKey != tt.want.HasAPIKey {
				t.Errorf("HasAPIKey = %v, want %v", got.HasAPIKey, tt.want.HasAPIKey)
			}
		})
	}
}
