package service

import (
	"context"
	"fmt"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"
	"aranea-agents/internal/knowledge"
	skilltrpc "aranea-agents/internal/skill/trpc"

	"github.com/google/uuid"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func NewKnowledgeChunker() *knowledge.Chunker {
	return knowledge.NewChunker(512, 64, knowledge.ChunkByChar)
}

func NewKnowledgeEmbedder() *knowledge.Embedder {
	return knowledge.NewEmbedder("", "", "", "", 1536)
}

// NewEvaluationRunner wires a real AgentRunner backed by ChatService so evaluation
// cases are executed through the same TRPC agent pipeline as normal chat turns.
// EP-BIZ-04: previously passed nil, nil; now injects ChatService as the runner.
func NewEvaluationRunner(uc *biz.EvalUsecase, chat *ChatService) *evaluation.Runner {
	agentRunner := func(ctx context.Context, agentID, input string) (string, error) {
		// Create an ephemeral session for this evaluation case.
		sess, err := chat.td.Sessions.Create(ctx, biz.Session{
			ID:        uuid.NewString(),
			AgentID:   agentID,
			OwnerType: "agent",
			Title:     fmt.Sprintf("eval-%s", agentID),
			UserID:    "1", // evaluation runs as the system user
		})
		if err != nil {
			return "", fmt.Errorf("eval: create session: %w", err)
		}
		_, asst, err := chat.RunNativeTurnUnary(ctx, &chatv1.SendChatMessageRequest{
			SessionId: sess.ID,
			Content:   input,
		})
		if err != nil {
			return "", err
		}
		return asst.ContentMarkdown, nil
	}
	return evaluation.NewRunner(uc, agentRunner, nil)
}

func NewSkillDBRepository(uc *biz.SkillUsecase) trpcskill.Repository {
	return skilltrpc.NewDBRepositoryAdapter(uc, 2*time.Minute)
}
