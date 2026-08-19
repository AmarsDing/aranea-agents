// Package knowledge — Phase 9 多模态入库：VisionExtractor 将图片经视觉 LLM
// 理解为结构化 Markdown，产出物与文本类同构，下游 Organize/Chunk/Embed 无模态差异。
package knowledge

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// visionExtractTimeout 是单次图片理解调用的总超时。
// 本地 CPU 视觉推理（如 Ollama qwen2.5vl 无 GPU）单张大图可达数分钟，60s 会永久超时循环。
const visionExtractTimeout = 300 * time.Second

// VisionExtractor 使用多模态 LLM 将图片理解结果输出为结构化 Markdown。
type VisionExtractor struct {
	llm     biz.LLMCaller
	sys     RefineLLMSettingsGetter
	catalog LLMCatalogLister
	lg      loggateway.Logger
}

// NewVisionExtractor 构造视觉提取器；sys/catalog 可为 nil（ResolveVisionLLM 降级到另一来源）。
func NewVisionExtractor(llm biz.LLMCaller, sys RefineLLMSettingsGetter, catalog LLMCatalogLister, lg loggateway.Logger) *VisionExtractor {
	return &VisionExtractor{llm: llm, sys: sys, catalog: catalog, lg: lg}
}

// Supports 判定扩展名或 MIME 是否为图片（png/jpg/jpeg/webp）。
func (VisionExtractor) Supports(ext, mimeType string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/")
}

// IsImageSource 报告来源是否为图片模态（VisionExtractor 路由域），
// 供 service 判定「跳过 MarkdownOrganizer / 留存原图 / metadata 标记」。
func IsImageSource(source, mimeType string) bool {
	return VisionExtractor{}.Supports(strings.ToLower(filepath.Ext(source)), mimeType)
}

// Extract 调用视觉 LLM 输出结构化 Markdown。
// 未配置视觉模型或调用失败时返回明确错误（NFR-12），由 service 落 status=error。
func (v *VisionExtractor) Extract(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	if v == nil || v.llm == nil {
		return "", fmt.Errorf("vision extractor unavailable: no LLM caller configured (%q)", source)
	}
	provider, model, err := ResolveVisionLLM(ctx, v.sys, v.catalog, "vision extract", v.lg)
	if err != nil {
		return "", err
	}

	callCtx, cancel := context.WithTimeout(ctx, visionExtractTimeout)
	defer cancel()

	resp, _, err := v.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System: `你是一名图片内容理解助手。用户会给你一张图片，请将其内容整理为结构清晰的 Markdown。
要求：
1. 转录图中全部可见文字（保持原有结构与顺序）；
2. 描述图表、表格、示意图的含义与关键数据；
3. 用合理的标题层级（#/##/###）、列表与表格组织输出；
4. 只输出 Markdown 正文，不要任何解释、前缀或代码块包裹。`,
		User:   "请将这张图片的内容整理为 Markdown。",
		Images: []biz.LLMImage{{Data: raw, Format: imageFormat(source, mimeType)}},
	})
	if err != nil {
		return "", fmt.Errorf("vision extract %q: %w", source, err)
	}
	md := strings.TrimSpace(stripCodeFence(resp))
	if md == "" {
		return "", fmt.Errorf("vision extract %q: empty response", source)
	}
	return md, nil
}

// imageFormat 推导图片格式标记（png/jpeg/webp），供 trpc Image.Format 使用。
func imageFormat(source, mimeType string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(source)), ".")
	if ext == "jpg" {
		return "jpeg"
	}
	if ext != "" {
		return ext
	}
	if mt := strings.ToLower(strings.TrimSpace(mimeType)); strings.HasPrefix(mt, "image/") {
		f := strings.TrimPrefix(mt, "image/")
		if f == "jpg" {
			return "jpeg"
		}
		return f
	}
	return "png"
}
