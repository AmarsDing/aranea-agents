package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	chatagent "aranea-agents/internal/agent"

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

func (o *ChatOrchestrator) mergeUserAttachmentRefs(ctx context.Context, userOpts string, attachmentIDs []string) (string, error) {
	if len(attachmentIDs) == 0 || o == nil || o.artifacts == nil {
		return userOpts, nil
	}
	refs := make([]chatagent.MessageAttachmentRef, 0, len(attachmentIDs))
	for _, id := range attachmentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		meta, _, err := o.artifacts.Load(ctx, id, 0)
		if err != nil {
			continue
		}
		refs = append(refs, chatagent.MessageAttachmentRef{
			ID:       meta.ID,
			Name:     meta.Name,
			MimeType: meta.MimeType,
			Size:     meta.Size,
		})
	}
	return chatagent.MergeAttachmentsIntoUserOptionsJSON(userOpts, refs)
}
