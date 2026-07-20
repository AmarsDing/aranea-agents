// Package knowledge — 统一摄取管线：MarkdownOrganizer 将提取文本经 LLM 整理为结构化 Markdown。
// 降级原则：LLM 不可用/超时/失败 → 透传原文本 organized=false，不阻塞入库（NFR-11）。
package knowledge

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

const (
	// markdownOrganizeTimeout 是单次整理调用的总超时（含全部窗口）。
	markdownOrganizeTimeout = 60 * time.Second
	// organizeWindowChars 是单次 LLM 整理的输入窗口（字节数）；
	// 超长文档按行边界切窗逐段整理后拼接，保证标题层级连续。
	organizeWindowChars = 6000
)

// MarkdownOrganizer 将提取出的原始文本整理为结构化 Markdown。
type MarkdownOrganizer struct {
	llm     biz.LLMCaller
	sys     RefineLLMSettingsGetter
	catalog LLMCatalogLister
	lg      loggateway.Logger
}

// NewMarkdownOrganizer 构造整理器；sys/catalog 可为 nil（ResolveLLM 降级到另一来源）。
func NewMarkdownOrganizer(llm biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) *MarkdownOrganizer {
	o := &MarkdownOrganizer{llm: llm, lg: lg}
	if sys != nil {
		o.sys = sys
	}
	if catalog != nil {
		o.catalog = catalog
	}
	return o
}

// Organize 输入提取文本与来源信息，输出结构化 Markdown。
// 任何失败路径均降级为原文本 + organized=false + nil error（不阻塞摄取）。
func (o *MarkdownOrganizer) Organize(ctx context.Context, text, source, mimeType string) (string, bool, error) {
	if o == nil || o.llm == nil || strings.TrimSpace(text) == "" {
		return text, false, nil
	}
	provider, model, err := ResolveLLM(ctx, o.sys, o.catalog, "markdown organize", o.lg)
	if err != nil {
		o.lg.Warn("Markdown 整理跳过：无可用 LLM",
			loggateway.StepID("knowledge.markdown_organize.skip"),
			loggateway.Str("source", source),
			loggateway.Err(err))
		return text, false, nil
	}

	orgCtx, cancel := context.WithTimeout(ctx, markdownOrganizeTimeout)
	defer cancel()

	windows := splitOrganizeWindows(text, organizeWindowChars)
	out := make([]string, 0, len(windows))
	for i, w := range windows {
		md, callErr := o.organizeWindow(orgCtx, w, source, mimeType, provider, model)
		if callErr != nil {
			o.lg.Warn("Markdown 整理失败，降级原文本",
				loggateway.StepID("knowledge.markdown_organize.fail"),
				loggateway.Str("source", source),
				loggateway.Int("window", i),
				loggateway.Err(callErr))
			return text, false, nil
		}
		out = append(out, md)
	}
	md := strings.TrimSpace(strings.Join(out, "\n\n"))
	if md == "" {
		return text, false, nil
	}
	return md, true, nil
}

// organizeWindow 调用 LLM 整理单个窗口文本，剥离可能的代码块包裹。
func (o *MarkdownOrganizer) organizeWindow(ctx context.Context, window, source, mimeType, provider, model string) (string, error) {
	sys := `你是一名技术文档整理助手。用户会给你一段从文档（` + mimeType + `）中提取的原始文本，请将其整理为结构清晰的 Markdown。
要求：
1. 完整保留原文的全部事实与信息，不新增、不删减、不改写观点；
2. 重建合理的标题层级（#/##/###）、段落、列表与表格；
3. 修复提取造成的断行与错乱空白；
4. 只输出 Markdown 正文，不要任何解释、前缀或代码块包裹。`

	resp, _, err := o.llm.Call(ctx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   sys,
		User:     window,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripCodeFence(resp)), nil
}

// splitOrganizeWindows 按行边界将文本切分为不超过 window 字节的窗口。
// 单行超窗时单独成窗（不截断不丢弃）。
func splitOrganizeWindows(text string, window int) []string {
	if len(text) <= window {
		return []string{text}
	}
	var out []string
	var b strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		if b.Len()+len(line) > window && b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
		b.WriteString(line)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
