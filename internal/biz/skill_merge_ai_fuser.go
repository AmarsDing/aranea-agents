package biz

import (
	"context"
	"fmt"
	"strings"
)

// RuleBasedContentFuser 基于规则的融合器（v1，无需 LLM）
// 后续可替换为 LLM 驱动的融合器。
// Used by both "ai_fuse" (deprecated) and "rule_fuse" merge strategies.
type RuleBasedContentFuser struct{}

func NewRuleBasedContentFuser() *RuleBasedContentFuser {
	return &RuleBasedContentFuser{}
}

func (f *RuleBasedContentFuser) Fuse(ctx context.Context, target SkillMergeSource, source SkillMergeSource) (*FusedContent, error) {
	// 规则化融合策略：
	// 1. 以 target body 为主体
	// 2. 从 source body 中提取 target 没有的 ## 段落
	// 3. 追加到 target body 末尾
	// 4. 合并标签集

	sourceSections := extractSections(source.Body)
	targetSections := extractSectionHeadings(target.Body)

	var newSections []string
	for heading, content := range sourceSections {
		if !targetSections[heading] {
			newSections = append(newSections, content)
		}
	}

	fusedBody := target.Body
	if len(newSections) > 0 {
		fusedBody += fmt.Sprintf("\n\n---\n\n# Merged from: %s\n", source.Name)
		for _, s := range newSections {
			fusedBody += "\n" + s
		}
	}

	return &FusedContent{
		Body: fusedBody,
		Tags: mergeStringSets(target.Tags, source.Tags),
	}, nil
}

// extractSections 从 markdown 中提取 ## 级别的段落
func extractSections(body string) map[string]string {
	sections := make(map[string]string)
	var currentHeading string
	var currentContent strings.Builder

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if currentHeading != "" {
				sections[currentHeading] = currentContent.String()
			}
			currentHeading = trimmed
			currentContent.Reset()
			currentContent.WriteString(line + "\n")
		} else if currentHeading != "" {
			currentContent.WriteString(line + "\n")
		}
	}
	if currentHeading != "" {
		sections[currentHeading] = currentContent.String()
	}
	return sections
}

// extractSectionHeadings 提取所有 ## 级别的 heading
func extractSectionHeadings(body string) map[string]bool {
	headings := make(map[string]bool)
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			headings[trimmed] = true
		}
	}
	return headings
}
