package service

import (
	"context"
	"fmt"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func (s *ChatService) buildUserMessage(ctx context.Context, sessionID, content string, refs []*chatv1.AttachmentRef) (trpcmodel.Message, error) {
	content = strings.TrimSpace(content)
	if len(refs) == 0 {
		return trpcmodel.NewUserMessage(content), nil
	}
	if s == nil || s.artifacts == nil {
		if content == "" {
			return trpcmodel.Message{}, fmt.Errorf("attachments require artifact service")
		}
		return trpcmodel.NewUserMessage(content), nil
	}
	parts := make([]trpcmodel.ContentPart, 0, len(refs)+1)
	if content != "" {
		text := content
		parts = append(parts, trpcmodel.ContentPart{Type: trpcmodel.ContentTypeText, Text: &text})
	}
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		id := strings.TrimSpace(ref.GetId())
		if id == "" {
			continue
		}
		meta, data, err := s.artifacts.Load(ctx, id, 0)
		if err != nil {
			return trpcmodel.Message{}, fmt.Errorf("load attachment %s: %w", id, err)
		}
		if strings.TrimSpace(meta.SessionID) != "" && strings.TrimSpace(sessionID) != "" && meta.SessionID != sessionID {
			return trpcmodel.Message{}, fmt.Errorf("attachment %s belongs to another session", id)
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
		return trpcmodel.NewUserMessage(content), nil
	}
	return trpcmodel.Message{Role: trpcmodel.RoleUser, ContentParts: parts}, nil
}
