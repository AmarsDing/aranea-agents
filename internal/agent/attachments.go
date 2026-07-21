package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// UserInputGate 对超限文本落地 blob 并返回 preview（窄接口，消费侧定义）。
// *biz.ToolResultGate 天然满足此接口。
type UserInputGate interface {
	CheckUserInput(ctx context.Context, sessionID, messageID, source, fullContent string) (biz.ToolResultGateResult, error)
}

// BuildUserMessageFromArtifacts assembles a multimodal user message from artifact attachment IDs.
// gate 非 nil 时，超阈值（biz.ToolResultSizeThreshold）的非 image 附件全文落地 blob，
// prompt 中以 preview 文本 part 替换 File part；nil 时保持原行为。
func BuildUserMessageFromArtifacts(
	ctx context.Context,
	artifacts *artifactbiz.Usecase,
	gate UserInputGate,
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
		// 单轮超限治理：超阈值文本附件落地 blob，以 preview 文本 part 替换 File part。
		if gate != nil && utf8.RuneCount(data) > biz.ToolResultSizeThreshold {
			res, gerr := gate.CheckUserInput(ctx, sessionID, ref.ID, biz.ToolResultSourceAttachment, string(data))
			if gerr != nil {
				return trpcmodel.Message{}, fmt.Errorf("gate attachment %s: %w", ref.ID, gerr)
			}
			if res.DidPersist {
				text := fmt.Sprintf("[附件 %s 内容过大已转存]\n%s", meta.Name, res.PreviewText)
				parts = append(parts, trpcmodel.ContentPart{Type: trpcmodel.ContentTypeText, Text: &text})
				continue
			}
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
