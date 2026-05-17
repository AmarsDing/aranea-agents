package service

import (
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"
	"aranea-agents/internal/knowledge"
	skilltrpc "aranea-agents/internal/skill/trpc"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func NewKnowledgeChunker() *knowledge.Chunker {
	return knowledge.NewChunker(512, 64, knowledge.ChunkByChar)
}

func NewKnowledgeEmbedder() *knowledge.Embedder {
	return knowledge.NewEmbedder("", "", "", "", 1536)
}

func NewEvaluationRunner(uc *biz.EvalUsecase) *evaluation.Runner {
	return evaluation.NewRunner(uc, nil, nil)
}

func NewSkillDBRepository(uc *biz.SkillUsecase) trpcskill.Repository {
	return skilltrpc.NewDBRepositoryAdapter(uc, 2*time.Minute)
}
