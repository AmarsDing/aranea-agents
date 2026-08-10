package knowledge

import (
	"context"
	"strings"

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

// collectionLacksSemanticLayer 判定目标集合是否为「无语义层词法库」
// （embedding_model 留空，R-4 契约：空 = 无语义层）。§V5 降级矩阵 #3：
// 无语义层 Vault 的检索必须自动降级 L0+L1 词法检索——chunks 无向量，
// dense 路径对 NULL embedding 排序恒空，静默返回空结果且用户无感知
// （2026-08-10 运行时事故）。
//
// GetCollection 失败（如集合不存在）时返回 false：保持原路径，
// 由 SearchChunks 产生原有错误语义。collectionID 为空（federated 逐集合
// 分发前）同样返回 false——调用方保证逐集合设置后再检索。
func collectionLacksSemanticLayer(ctx context.Context, repo biz.KnowledgeRepo, collectionID string) bool {
	if repo == nil || collectionID == "" {
		return false
	}
	col, err := repo.GetCollection(ctx, collectionID)
	if err != nil {
		return false
	}
	return strings.TrimSpace(col.EmbeddingModel) == ""
}
