package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"
	"aranea-agents/internal/provider"

	"github.com/google/uuid"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

// NewEvaluationRunner wires AgentRunner + LLMJudge for evaluation runs (EP-RT-08).
func NewEvaluationRunner(
	uc *biz.EvalUsecase,
	chat *ChatService,
	catalog *biz.LlmProviderModelUsecase,
) *evaluation.Runner {
	rt := &provider.RoundTrip{HTTP: &http.Client{Timeout: 120 * time.Second}}
	agentRunner := func(ctx context.Context, agentID, input string) (string, error) {
		sess, err := chat.td.Sessions.Create(ctx, biz.Session{
			ID:        uuid.NewString(),
			AgentID:   agentID,
			OwnerType: "agent",
			Title:     fmt.Sprintf("eval-%s", agentID),
			UserID:    "1",
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
	judge := evaluation.NewLLMJudge(catalog, rt)
	runFactory := func(agentID string) (runner.Runner, error) {
		return evaluation.NewChatRunnerAdapter(agentID, agentRunner), nil
	}
	framework := evaluation.NewFrameworkBridge(runFactory, judge)
	return evaluation.NewRunner(uc, agentRunner, judge, framework)
}
