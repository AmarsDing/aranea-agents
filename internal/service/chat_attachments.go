package service

import (
	"context"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func (s *ChatService) buildUserMessage(ctx context.Context, sessionID, content string, refs []*chatv1.AttachmentRef) (trpcmodel.Message, error) {
	return s.orch.buildUserMessage(ctx, sessionID, content, refs)
}
