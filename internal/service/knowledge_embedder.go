package service

import (
	"context"
	"os"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

// NewKnowledgeEmbedder builds the knowledge embedder from env, then system_settings (EP-KN-01).
//
// Priority: KRATOS_KNOWLEDGE_EMBED_* env > system_settings.knowledge_embed_* > provider key env fallbacks.
func NewKnowledgeEmbedder(c *conf.Data, sys biz.SystemSettingRepo, lg loggateway.Logger) *knowledge.Embedder {
	cfg := loadKnowledgeEmbedFromEnv(c)
	if sys != nil {
		if stored, err := sys.GetKnowledgeEmbed(context.Background()); err == nil {
			cfg = mergeKnowledgeEmbedConfig(cfg, stored)
		}
	}
	return knowledge.NewEmbedder(cfg.provider, cfg.baseURL, cfg.apiKey, cfg.model, cfg.dim, lg)
}

type knowledgeEmbedConfig struct {
	provider string
	baseURL  string
	apiKey   string
	model    string
	dim      int
}

func loadKnowledgeEmbedFromEnv(c *conf.Data) knowledgeEmbedConfig {
	provider := strings.TrimSpace(os.Getenv("KRATOS_KNOWLEDGE_EMBED_PROVIDER"))
	baseURL := strings.TrimSpace(os.Getenv("KRATOS_KNOWLEDGE_EMBED_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("KRATOS_KNOWLEDGE_EMBED_API_KEY"))
	model := strings.TrimSpace(os.Getenv("KRATOS_KNOWLEDGE_EMBED_MODEL"))
	dim := envInt("KRATOS_KNOWLEDGE_EMBED_DIM", 0)

	if c != nil && c.GetPostgres() != nil && dim <= 0 {
		dim = int(c.GetPostgres().GetVectorDim())
	}
	if dim <= 0 {
		dim = 1536
	}
	return knowledgeEmbedConfig{provider: provider, baseURL: baseURL, apiKey: apiKey, model: model, dim: dim}
}

func mergeKnowledgeEmbedConfig(env knowledgeEmbedConfig, db biz.KnowledgeEmbedSetting) knowledgeEmbedConfig {
	out := env
	if out.provider == "" {
		out.provider = strings.TrimSpace(db.Provider)
	}
	if out.baseURL == "" {
		out.baseURL = strings.TrimSpace(db.BaseURL)
	}
	if out.model == "" {
		out.model = strings.TrimSpace(db.Model)
	}
	if out.dim <= 0 && db.Dim > 0 {
		out.dim = db.Dim
	}
	if out.apiKey == "" && db.HasAPIKey {
		out.apiKey = strings.TrimSpace(db.APIKey)
	}
	if out.provider == "" {
		out.provider = knowledge.ProviderOpenAI
	}
	if out.model == "" {
		out.model = defaultKnowledgeEmbedModel(out.provider)
	}
	if out.apiKey == "" {
		out.apiKey = defaultKnowledgeEmbedAPIKey(out.provider)
	}
	if out.provider == knowledge.ProviderHuggingFace && out.baseURL == "" {
		out.baseURL = "http://localhost:8080"
	}
	return out
}

func defaultKnowledgeEmbedModel(provider string) string {
	switch provider {
	case knowledge.ProviderGemini:
		return "gemini-embedding-001"
	case knowledge.ProviderOllama:
		return "nomic-embed-text"
	default:
		return "text-embedding-3-small"
	}
}

func defaultKnowledgeEmbedAPIKey(provider string) string {
	switch provider {
	case knowledge.ProviderGemini:
		return strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	default:
		return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
}

// PersistKnowledgeEmbed merges and saves runtime embedder settings to system_settings.
func PersistKnowledgeEmbed(ctx context.Context, sys biz.SystemSettingRepo, provider, baseURL, apiKey, model string, dim int) error {
	if sys == nil {
		return nil
	}
	cur, err := sys.GetKnowledgeEmbed(ctx)
	if err != nil {
		return err
	}
	updateKey := strings.TrimSpace(apiKey) != ""
	merged := biz.ApplyKnowledgeEmbedPatch(cur, provider, baseURL, apiKey, model, dim, updateKey)
	_, err = sys.UpdateKnowledgeEmbed(ctx, merged, updateKey)
	return err
}

func envInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
