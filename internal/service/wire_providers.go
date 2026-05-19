package service

import (
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	skilltrpc "aranea-agents/internal/skill/trpc"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func NewKnowledgeChunker() *knowledge.Chunker {
	return knowledge.NewChunker(512, 64, knowledge.ChunkByChar)
}

// NewKnowledgeEmbedder is defined in knowledge_embedder.go (EP-KN-01).

// NewEvaluationRunner is defined in evaluation_runner.go (EP-RT-08).

func NewSkillDBRepository(uc *biz.SkillUsecase) trpcskill.Repository {
	return skilltrpc.NewDBRepositoryAdapter(uc, 2*time.Minute)
}
