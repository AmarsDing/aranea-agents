package knowledge

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

type ChunkSearcher interface {
	Search(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error)
}

type ChunkAssessor interface {
	Evaluate(ctx context.Context, query string, chunks []biz.KnowledgeChunk) (*RetrievalAssessment, error)
}

func SearchWithEvaluation(ctx context.Context, searcher ChunkSearcher, assessor ChunkAssessor, query string, q biz.KnowledgeSearchQuery, chunks []biz.KnowledgeChunk) ([]biz.KnowledgeChunk, error) {
	if assessor == nil || len(chunks) == 0 {
		return chunks, nil
	}
	assessment, evalErr := assessor.Evaluate(ctx, query, chunks)
	if evalErr != nil {
		event.SysLogWarn("knowledge.eval.assessment_fail", "检索评估失败，使用原始结果",
			event.P("error", evalErr.Error()), event.P("collection_id", q.CollectionID))
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
		event.SysLogWarn("knowledge.eval.supplement_fail", "补充检索失败，使用原始结果",
			event.P("error", supErr.Error()), event.P("supplement_query", assessment.SupplementQuery), event.P("collection_id", q.CollectionID))
		return chunks, nil
	}
	if len(supChunks) == 0 {
		return chunks, nil
	}
	return MergeSearchResults(chunks, supChunks, q.TopK), nil
}
