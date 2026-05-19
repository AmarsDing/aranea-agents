package service

import (
	"os"
	"strconv"
	"strings"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/knowledge"
)

// NewKnowledgeEmbedder builds the knowledge embedder from config and environment (EP-KN-01).
//
// Environment (override config):
//   - KRATOS_KNOWLEDGE_EMBED_PROVIDER  (openai|ollama)
//   - KRATOS_KNOWLEDGE_EMBED_BASE_URL
//   - KRATOS_KNOWLEDGE_EMBED_API_KEY    (falls back to OPENAI_API_KEY)
//   - KRATOS_KNOWLEDGE_EMBED_MODEL
//   - KRATOS_KNOWLEDGE_EMBED_DIM
func NewKnowledgeEmbedder(c *conf.Data) *knowledge.Embedder {
	provider := strings.TrimSpace(os.Getenv("KRATOS_KNOWLEDGE_EMBED_PROVIDER"))
	baseURL := strings.TrimSpace(os.Getenv("KRATOS_KNOWLEDGE_EMBED_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("KRATOS_KNOWLEDGE_EMBED_API_KEY"))
	model := strings.TrimSpace(os.Getenv("KRATOS_KNOWLEDGE_EMBED_MODEL"))
	dim := envInt("KRATOS_KNOWLEDGE_EMBED_DIM", 0)

	if c != nil && c.GetPostgres() != nil {
		if dim <= 0 {
			dim = int(c.GetPostgres().GetVectorDim())
		}
	}
	if dim <= 0 {
		dim = 1536
	}
	if provider == "" {
		provider = "openai"
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	return knowledge.NewEmbedder(provider, baseURL, apiKey, model, dim)
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
