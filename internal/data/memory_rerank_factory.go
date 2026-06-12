package data

import (
	"os"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

// newMemoryReranker creates the appropriate biz.Reranker based on environment configuration.
//
// If KRATOS_MEMORY_RERANKER is set to cohere/infinity, it creates a KnowledgeRerankerAdapter
// that delegates to the Knowledge module's reranker factory.
// Otherwise it falls back to CrossEncoderReranker (bigram Jaccard).
func newMemoryReranker(lg loggateway.Logger) biz.Reranker {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("KRATOS_MEMORY_RERANKER")))
	switch backend {
	case "cohere", "infinity":
		r, err := knowledge.NewRerankerFromEnv()
		if err != nil {
			lg.Warn("memory reranker backend init failed, falling back to jaccard",
				loggateway.StepID("data.reranker"), loggateway.Str("backend", backend), loggateway.Err(err))
			return biz.NewCrossEncoderReranker()
		}
		if r == nil {
			lg.Warn("memory reranker backend returned nil, falling back to jaccard",
				loggateway.StepID("data.reranker"), loggateway.Str("backend", backend))
			return biz.NewCrossEncoderReranker()
		}
		lg.Info("memory reranker using knowledge backend",
			loggateway.StepID("data.reranker"), loggateway.Str("backend", backend))
		return NewKnowledgeRerankerAdapter(r, lg)
	default:
		return biz.NewCrossEncoderReranker()
	}
}
