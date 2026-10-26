package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/apierror"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// T-1a（2026-10-26 评审二轮）多模态受理门：此前图片附件无尺寸/数量上限，
// 字节随 trpc 会话历史逐轮全量内联（10MB 图 ≈ 13.3MB base64/轮），压缩/
// est/admission 全失明。超门 fail-fast（与 UserInputHardLimitChars 同款
// 闸前拒绝），不放行失明流量。
const (
	// maxImageAttachmentsPerMessage 单条消息图片附件数上限。
	maxImageAttachmentsPerMessage = 4
	// maxImageInlineBytes 单图内联字节上限（4MiB 原始 ≈ 5.5MB base64）。
	maxImageInlineBytes = 4 << 20
)

// maxAttachmentNameRunes 是消毒后文件名的 rune 上限（T-4）。
const maxAttachmentNameRunes = 100

// UserInputGate 对超限文本落地 blob 并返回 preview（窄接口，消费侧定义）。
// *biz.ToolResultGate 天然满足此接口。
type UserInputGate interface {
	CheckUserInput(ctx context.Context, sessionID, messageID, source, fullContent string) (biz.ToolResultGateResult, error)
}

// userInputMessageID 从内容派生确定性 messageID，供闸幂等（同内容重发复用同一 blob）。
func userInputMessageID(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "user_input_" + hex.EncodeToString(sum[:8])
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
	// 硬上限：单条纯文本 > 20 万字符直接拒绝（闸前拦截，不产生 blob）。
	if n := utf8.RuneCountInString(content); n > biz.UserInputHardLimitChars {
		return trpcmodel.Message{}, apierror.BadRequest("CHAT_INPUT",
			"user input %d chars exceeds hard limit %d", n, biz.UserInputHardLimitChars)
	}
	// 超阈值纯文本落地 blob，prompt 中注入头部预览（LLM 可用 read_tool_result 分段读全文）。
	if gate != nil && utf8.RuneCountInString(content) > biz.ToolResultSizeThreshold {
		res, gerr := gate.CheckUserInput(ctx, sessionID, userInputMessageID(content), biz.ToolResultSourceUserInput, content)
		if gerr != nil {
			return trpcmodel.Message{}, fmt.Errorf("gate user input: %w", gerr)
		}
		if res.DidPersist {
			content = "[输入内容过大已转存]\n" + res.PreviewText
		}
	}
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
	imageCount := 0
	for _, ref := range refs {
		meta, data, err := artifacts.Load(ctx, ref.ID, 0)
		if err != nil {
			return trpcmodel.Message{}, fmt.Errorf("load attachment %s: %w", ref.ID, err)
		}
		mime := strings.ToLower(strings.TrimSpace(meta.MimeType))
		// T-4：进入 prompt 文本 / File.Name 的文件名先消毒——剥离控制字符
		// （含 \r\n，防文件名注入伪造指令行），截断防爆行。
		safeName := sanitizeAttachmentName(meta.Name)
		if strings.HasPrefix(mime, "image/") {
			// T-1a 受理门：数量/单图字节超限即拒（fail-fast，闸前拦截）。
			imageCount++
			if imageCount > maxImageAttachmentsPerMessage {
				return trpcmodel.Message{}, apierror.BadRequest("CHAT_INPUT",
					"too many image attachments: %d exceeds limit %d", imageCount, maxImageAttachmentsPerMessage)
			}
			if len(data) > maxImageInlineBytes {
				return trpcmodel.Message{}, apierror.BadRequest("CHAT_INPUT",
					"image attachment %s is %d bytes, exceeds inline limit %d", safeName, len(data), maxImageInlineBytes)
			}
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
				text := fmt.Sprintf("[附件 %s 内容过大已转存]\n%s", safeName, res.PreviewText)
				parts = append(parts, trpcmodel.ContentPart{Type: trpcmodel.ContentTypeText, Text: &text})
				continue
			}
		}
		// T-2 小文本抽取短路：text/* 或扩展名白名单、合法 UTF-8、且未超
		// blob 阈值的小文件以文本 part 内联——此前原始字节按自报 MIME 直进
		// input_file，DeepSeek 静默丢弃（模型编造"已读文件"）、OpenAI
		// 严格校验整轮 400。超阈值文本走上方 blob 闸；nil gate 时回退
		// File part（保持既有行为，不全量裸内联）。
		if isTextLikeAttachment(meta.Name, mime) && utf8.Valid(data) && utf8.RuneCount(data) <= biz.ToolResultSizeThreshold {
			text := fmt.Sprintf("[附件 %s]\n%s", safeName, string(data))
			parts = append(parts, trpcmodel.ContentPart{Type: trpcmodel.ContentTypeText, Text: &text})
			continue
		}
		parts = append(parts, trpcmodel.ContentPart{
			Type: trpcmodel.ContentTypeFile,
			File: &trpcmodel.File{
				Name:     safeName,
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

// sanitizeAttachmentName 消毒进入 prompt 文本与 File.Name 的附件文件名
// （T-4）：剥离全部控制字符（含 \r\n——文件名注入可在 prompt 内伪造指令
// 行/对话分隔），修剪空白，截断 maxAttachmentNameRunes，空名回退
// "attachment"。纯函数，不改变存储侧原始文件名。
func sanitizeAttachmentName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > maxAttachmentNameRunes {
		name = string([]rune(name)[:maxAttachmentNameRunes])
	}
	if name == "" {
		return "attachment"
	}
	return name
}

// textLikeExts 是 T-2 文本抽取短路的扩展名白名单（MIME 自报不可信时的第
// 二判据）。只收录明确纯文本形态；办公复合格式（docx/xlsx 等 zip 容器）
// 不在列——utf8.Valid 也会拦，但白名单先行更省一次全量扫描。
var textLikeExts = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".log": true,
	".json": true, ".jsonl": true, ".csv": true, ".tsv": true,
	".yaml": true, ".yml": true, ".xml": true, ".toml": true,
	".ini": true, ".cfg": true, ".conf": true, ".env": true,
	".go": true, ".py": true, ".js": true, ".jsx": true,
	".ts": true, ".tsx": true, ".java": true, ".c": true,
	".h": true, ".cpp": true, ".hpp": true, ".cc": true,
	".rs": true, ".rb": true, ".php": true, ".sh": true,
	".bat": true, ".ps1": true, ".sql": true, ".html": true,
	".htm": true, ".css": true, ".scss": true, ".less": true,
	".vue": true, ".svg": true,
}

// isTextLikeAttachment 判定附件是否走文本抽取短路（T-2）：text/* MIME、
// 已知纯文本 application/* MIME，或扩展名白名单。调用方仍需 utf8.Valid
// 复核字节（自报 MIME 与扩展名均可伪造）。
func isTextLikeAttachment(name, mime string) bool {
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	switch mime {
	case "application/json", "application/xml", "application/javascript",
		"application/x-yaml", "application/yaml", "application/toml",
		"application/x-sh", "application/sql", "image/svg+xml":
		return true
	}
	ext := strings.ToLower(path.Ext(strings.TrimSpace(name)))
	return textLikeExts[ext]
}
