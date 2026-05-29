package service

import (
	"time"

	"aranea-agents/internal/biz"
	skilltrpc "aranea-agents/internal/skill/trpc"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// NewKnowledgeEmbedder is defined in knowledge_embedder.go (EP-KN-01).

// NewEvaluationRunner is defined in evaluation_runner.go (EP-RT-08).

func NewSkillDBRepository(uc *biz.SkillUsecase) trpcskill.Repository {
	return skilltrpc.NewDBRepositoryAdapter(uc, 2*time.Minute)
}
