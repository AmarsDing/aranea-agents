package agent

import (
	"context"
	"fmt"
	"strings"

	artifactbiz "aranea-agents/internal/biz/artifact"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// BuildUserMessageFromArtifacts assembles a multimodal user message from artifact attachment IDs.
func BuildUserMessageFromArtifacts(
	ctx context.Context,
	artifacts *artifactbiz.Usecase,
	sessionID, content string,
	attachmentIDs []string,
) (trpcmodel.Message, error) {
	content = strings.TrimSpace(content)
	refs, err := artifactbiz.ResolveAttachmentRefs(ctx, artifacts, sessionID, attachmentIDs)
	if err != nil {
		return trpcmodel.Message{}, err
	}
	if len(refs) == 0 {
		return trpcmodel.NewUserMessage(content), nil
	}
	parts := make([]trpcmodel.ContentPart, 0, len(refs)+1)
	if content != "" {
		text := content
		parts = append(parts, trpcmodel.ContentPart{Type: trpcmodel.ContentTypeText, Text: &text})
	}
	for _, ref := range refs {
		meta, data, err := artifacts.Load(ctx, ref.ID, 0)
		if err != nil {
			return trpcmodel.Message{}, fmt.Errorf("load attachment %s: %w", ref.ID, err)
		}
		mime := strings.ToLower(strings.TrimSpace(meta.MimeType))
		if strings.HasPrefix(mime, "image/") {
			format := strings.TrimPrefix(mime, "image/")
			parts = append(parts, trpcmodel.ContentPart{
				Type: trpcmodel.ContentTypeImage,
				Image: &trpcmodel.Image{
					Data:   data,
					Format: format,
					Detail: "auto",
				},
			})
			continue
		}
		parts = append(parts, trpcmodel.ContentPart{
			Type: trpcmodel.ContentTypeFile,
			File: &trpcmodel.File{
				Name:     meta.Name,
				Data:     data,
				MimeType: meta.MimeType,
			},
		})
	}
	if len(parts) == 0 {
		if content == "" {
			return trpcmodel.Message{}, fmt.Errorf("attachments require message content or valid attachment data")
		}
		return trpcmodel.NewUserMessage(content), nil
	}
	return trpcmodel.Message{Role: trpcmodel.RoleUser, ContentParts: parts}, nil
}
