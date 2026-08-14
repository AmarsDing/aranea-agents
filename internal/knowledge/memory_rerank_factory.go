package knowledge

import (
	"os"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// NewMemoryReranker creates the biz.Reranker used by memory L2/L3 recall.
//
// If KRATOS_MEMORY_RERANKER is set to cohere/infinity, it wraps the Knowledge
// module's reranker factory (KRATOS_KNOWLEDGE_RERANKER / Cohere / Infinity env)
// via KnowledgeRerankerAdapter. Otherwise it falls back to CrossEncoderReranker
// (bigram Jaccard). Wire injects the result into data.NewData; data does not
// construct or import the framework reranker.
func NewMemoryReranker(lg loggateway.Logger) biz.Reranker {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("KRATOS_MEMORY_RERANKER")))
	switch backend {
	case "cohere", "infinity":
		r, err := NewRerankerFromEnv()
		if err != nil {
			lg.Warn("memory reranker backend init failed, falling back to jaccard",
				loggateway.StepID("knowledge.memory_reranker"), loggateway.Str("backend", backend), loggateway.Err(err))
			return biz.NewCrossEncoderReranker()
		}
		if r == nil {
			lg.Warn("memory reranker backend returned nil, falling back to jaccard",
				loggateway.StepID("knowledge.memory_reranker"), loggateway.Str("backend", backend))
			return biz.NewCrossEncoderReranker()
		}
		lg.Info("memory reranker using knowledge backend",
			loggateway.StepID("knowledge.memory_reranker"), loggateway.Str("backend", backend))
		return NewKnowledgeRerankerAdapter(r, lg)
	default:
		return biz.NewCrossEncoderReranker()
	}
}
