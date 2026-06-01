package knowledge

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type ChunkSearcher interface {
	Search(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error)
}

type ChunkAssessor interface {
	Evaluate(ctx context.Context, query string, chunks []biz.KnowledgeChunk) (*RetrievalAssessment, error)
}

func SearchWithEvaluation(ctx context.Context, searcher ChunkSearcher, assessor ChunkAssessor, query string, q biz.KnowledgeSearchQuery, chunks []biz.KnowledgeChunk, lg loggateway.Logger) ([]biz.KnowledgeChunk, error) {
	if assessor == nil || len(chunks) == 0 {
		return chunks, nil
	}
	assessment, evalErr := assessor.Evaluate(ctx, query, chunks)
	if evalErr != nil {
		lg.Warn("检索评估失败，使用原始结果",
			loggateway.StepID("knowledge.eval.assessment_fail"),
			loggateway.Err(evalErr),
			loggateway.Str("collection_id", q.CollectionID))
		return chunks, nil
	}
	if assessment.Sufficient || assessment.SupplementQuery == "" {
		return chunks, nil
	}
	supQ := q
	supQ.Query = assessment.SupplementQuery
	supQ.TopK = q.TopK
	supChunks, supErr := searcher.Search(ctx, supQ)
	if supErr != nil {
		lg.Warn("补充检索失败，使用原始结果",
			loggateway.StepID("knowledge.eval.supplement_fail"),
			loggateway.Err(supErr),
			loggateway.Str("supplement_query", assessment.SupplementQuery),
			loggateway.Str("collection_id", q.CollectionID))
		return chunks, nil
	}
	if len(supChunks) == 0 {
		return chunks, nil
	}
	return MergeSearchResults(chunks, supChunks, q.TopK), nil
}
