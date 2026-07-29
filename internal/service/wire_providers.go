package service

import (
	"time"

	"aranea-agents/internal/biz"
	skilltrpc "aranea-agents/internal/skill/trpc"
	"aranea-agents/pkg/loggateway"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// NewKnowledgeEmbedder is defined in knowledge_embedder.go (EP-KN-01).

// NewEvaluationRunner is defined in evaluation_runner.go (EP-RT-08).

func NewSkillDBRepository(uc *biz.SkillUsecase, lg loggateway.Logger) trpcskill.Repository {
	adapter := skilltrpc.NewDBRepositoryAdapter(uc, 2*time.Minute, lg)
	// P0：Skill 变更（启用/删除/回滚/正文）主动失效运行时快照，替代纯 TTL 兜底。
	uc.SetRuntimeCacheInvalidator(adapter)
	return adapter
}
