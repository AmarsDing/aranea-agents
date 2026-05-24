package service

import (
	"context"

	chatv1 "aranea-agents/api/kratos/chat/v1"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func (s *ChatService) buildUserMessage(ctx context.Context, sessionID, content string, attachmentIDs []string) (trpcmodel.Message, error) {
	return s.orch.buildUserMessage(ctx, sessionID, content, attachmentIDs)
}

// buildUserMessageFromProto converts proto attachment refs to IDs and delegates to the biz-level buildUserMessage.
func (s *ChatService) buildUserMessageFromProto(ctx context.Context, sessionID, content string, refs []*chatv1.AttachmentRef) (trpcmodel.Message, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		if r != nil {
			ids = append(ids, r.GetId())
		}
	}
	return s.orch.buildUserMessage(ctx, sessionID, content, ids)
}
