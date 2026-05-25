package service

import (
	"context"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	artifactbiz "aranea-agents/internal/biz/artifact"

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

func (o *ChatOrchestrator) mergeUserAttachmentRefs(ctx context.Context, sessionID, userOpts string, attachmentIDs []string) (string, int, error) {
	if len(artifactbiz.NormalizeAttachmentIDs(attachmentIDs)) == 0 || o == nil || o.artifacts == nil {
		return userOpts, 0, nil
	}
	refs, err := artifactbiz.ResolveAttachmentRefs(ctx, o.artifacts, sessionID, attachmentIDs)
	if err != nil {
		return userOpts, 0, err
	}
	merged, err := artifactbiz.MergeRefsIntoOptionsJSON(userOpts, refs)
	return merged, len(refs), err
}

func mergeTurnArtifactRefs(optionsJSON string, refs []artifactbiz.Ref) (string, error) {
	return artifactbiz.MergeRefsIntoOptionsJSON(optionsJSON, refs)
}
