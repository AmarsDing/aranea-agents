package biz

import "strings"

// KnowledgeEmbedSetting is persisted platform default for the knowledge embedder.
type KnowledgeEmbedSetting struct {
	Provider  string
	BaseURL   string
	Model     string
	Dim       int
	APIKey    string // write-only; empty on read from repo
	HasAPIKey bool
}

// KnowledgeEmbedConfigured reports whether stored settings are sufficient for the provider.
func KnowledgeEmbedConfigured(s KnowledgeEmbedSetting) bool {
	p := strings.TrimSpace(s.Provider)
	if p == "" {
		return false
	}
	switch p {
	case "ollama":
		return true
	case "huggingface":
		return strings.TrimSpace(s.BaseURL) != ""
	default:
		return s.HasAPIKey
	}
}

// ApplyKnowledgeEmbedPatch merges an update onto current settings.
// apiKey empty with updateAPIKey false keeps the existing key.
func ApplyKnowledgeEmbedPatch(cur KnowledgeEmbedSetting, provider, baseURL, apiKey, model string, dim int, updateAPIKey bool) KnowledgeEmbedSetting {
	out := cur
	if p := strings.TrimSpace(provider); p != "" {
		out.Provider = p
	}
	if b := strings.TrimRight(strings.TrimSpace(baseURL), "/"); b != "" {
		out.BaseURL = b
	}
	if m := strings.TrimSpace(model); m != "" {
		out.Model = m
	}
	if dim > 0 {
		out.Dim = dim
	}
	if updateAPIKey && strings.TrimSpace(apiKey) != "" {
		out.APIKey = strings.TrimSpace(apiKey)
		out.HasAPIKey = true
	}
	return out
}
