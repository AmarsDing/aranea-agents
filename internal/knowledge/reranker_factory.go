package knowledge

import (
	"fmt"
	"os"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker/cohere"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker/infinity"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker/topk"
)

// NewRerankerFromEnv builds a framework reranker from environment (KN-01).
//
// KRATOS_KNOWLEDGE_RERANKER: off|topk|cohere|infinity (empty = off)
// KRATOS_KNOWLEDGE_RERANK_TOP_K: top-k after rerank (topk mode; 0 = all)
// COHERE_API_KEY, COHERE_RERANK_MODEL, COHERE_RERANK_ENDPOINT
// INFINITY_RERANK_ENDPOINT, INFINITY_RERANK_MODEL, INFINITY_RERANK_API_KEY
func NewRerankerFromEnv() (reranker.Reranker, error) {
	kind := strings.ToLower(strings.TrimSpace(os.Getenv("KRATOS_KNOWLEDGE_RERANKER")))
	if kind == "" || kind == "off" || kind == "none" || kind == "false" {
		return nil, nil
	}
	topK := rerankEnvInt("KRATOS_KNOWLEDGE_RERANK_TOP_K", 0)

	switch kind {
	case "topk":
		return topk.New(topk.WithK(topK)), nil
	case "cohere":
		opts := []cohere.Option{
			cohere.WithAPIKey(strings.TrimSpace(os.Getenv("COHERE_API_KEY"))),
		}
		if m := strings.TrimSpace(os.Getenv("COHERE_RERANK_MODEL")); m != "" {
			opts = append(opts, cohere.WithModel(m))
		}
		if ep := strings.TrimSpace(os.Getenv("COHERE_RERANK_ENDPOINT")); ep != "" {
			opts = append(opts, cohere.WithEndpoint(ep))
		}
		if topK > 0 {
			opts = append(opts, cohere.WithTopN(topK))
		}
		return cohere.New(opts...)
	case "infinity":
		opts := []infinity.Option{}
		if ep := strings.TrimSpace(os.Getenv("INFINITY_RERANK_ENDPOINT")); ep != "" {
			opts = append(opts, infinity.WithEndpoint(ep))
		}
		if key := strings.TrimSpace(os.Getenv("INFINITY_RERANK_API_KEY")); key != "" {
			opts = append(opts, infinity.WithAPIKey(key))
		}
		if m := strings.TrimSpace(os.Getenv("INFINITY_RERANK_MODEL")); m != "" {
			opts = append(opts, infinity.WithModel(m))
		}
		if topK > 0 {
			opts = append(opts, infinity.WithTopN(topK))
		}
		return infinity.New(opts...)
	default:
		return nil, fmt.Errorf("knowledge reranker: unknown KRATOS_KNOWLEDGE_RERANKER %q", kind)
	}
}

func rerankEnvInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil || n < 0 {
		return def
	}
	return n
}
