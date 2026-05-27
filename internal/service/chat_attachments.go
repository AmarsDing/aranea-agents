package service

import (
	"context"
	"fmt"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/provider"

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
	refs, err := o.resolveUserAttachmentRefs(ctx, sessionID, attachmentIDs)
	if err != nil {
		return userOpts, 0, err
	}
	merged, err := mergeUserAttachmentRefs(userOpts, refs)
	return merged, len(refs), err
}

func (o *ChatOrchestrator) resolveUserAttachmentRefs(ctx context.Context, sessionID string, attachmentIDs []string) ([]artifactbiz.Ref, error) {
	if len(artifactbiz.NormalizeAttachmentIDs(attachmentIDs)) == 0 || o == nil || o.artifacts == nil {
		return nil, nil
	}
	return artifactbiz.ResolveAttachmentRefs(ctx, o.artifacts, sessionID, attachmentIDs)
}

func mergeUserAttachmentRefs(userOpts string, refs []artifactbiz.Ref) (string, error) {
	if len(refs) == 0 {
		return userOpts, nil
	}
	merged, err := artifactbiz.MergeRefsIntoOptionsJSON(userOpts, refs)
	return merged, err
}

func (o *ChatOrchestrator) validateTurnAttachmentCapabilities(ctx context.Context, prov, mod string, refs []artifactbiz.Ref) error {
	if len(refs) == 0 || o == nil {
		return nil
	}
	if hasImageAttachment(refs) && !provider.ModelSupportsImageAttachments(ctx, o.td.Catalog.LLM, prov, mod) {
		return TurnError(TurnErrAttachmentUnsupported, fmt.Sprintf("%s/%s does not support image attachments", strings.TrimSpace(prov), strings.TrimSpace(mod)))
	}
	if hasFileAttachment(refs) && !provider.ModelSupportsFileAttachments(ctx, o.td.Catalog.LLM, prov, mod) {
		return TurnError(TurnErrAttachmentUnsupported, fmt.Sprintf("%s/%s does not support file attachments", strings.TrimSpace(prov), strings.TrimSpace(mod)))
	}
	return nil
}

func hasImageAttachment(refs []artifactbiz.Ref) bool {
	for _, ref := range refs {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref.MimeType)), "image/") {
			return true
		}
	}
	return false
}

func hasFileAttachment(refs []artifactbiz.Ref) bool {
	for _, ref := range refs {
		mime := strings.ToLower(strings.TrimSpace(ref.MimeType))
		if mime != "" && !strings.HasPrefix(mime, "image/") {
			return true
		}
	}
	return false
}

func mergeTurnArtifactRefs(optionsJSON string, refs []artifactbiz.Ref) (string, error) {
	return artifactbiz.MergeRefsIntoOptionsJSON(optionsJSON, refs)
}
