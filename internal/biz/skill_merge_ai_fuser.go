package biz

import (
	"context"
	"fmt"
	"sort"
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
	// 规则化融合策略（与 append 的区别）：
	// 1. 以 target body 为主体
	// 2. 对于 source 和 target 共有的 ## 段落：合并内容（target 在前，source 补充在后）
	// 3. 对于 source 独有的 ## 段落：追加到 target body 末尾
	// 4. 合并标签集

	targetSections := extractSections(target.Body)
	sourceSections := extractSections(source.Body)

	// Collect source sections that need to be merged or appended.
	var mergedSections []string   // sections merged into existing target headings
	var appendedSections []string // sections unique to source

	// Sort headings for deterministic output order.
	sortedHeadings := make([]string, 0, len(sourceSections))
	for heading := range sourceSections {
		sortedHeadings = append(sortedHeadings, heading)
	}
	sort.Strings(sortedHeadings)

	for _, heading := range sortedHeadings {
		sourceContent := sourceSections[heading]
		if _, exists := targetSections[heading]; exists {
			// Same heading exists in both: merge source content after target content.
			// Strip the heading line from sourceContent since the heading already exists in target.
			sourceBody := stripHeading(sourceContent)
			if strings.TrimSpace(sourceBody) != "" {
				merged := "\n\n---\n> Merged from: " + source.Name + "\n\n" + sourceBody
				mergedSections = append(mergedSections, merged)
			}
		} else {
			appendedSections = append(appendedSections, sourceContent)
		}
	}

	// Build fused body: start with target, then append merged additions and new sections.
	fusedBody := target.Body
	if len(mergedSections) > 0 || len(appendedSections) > 0 {
		fusedBody += fmt.Sprintf("\n\n---\n\n# Merged from: %s\n", source.Name)
	}

	for _, merged := range mergedSections {
		fusedBody += merged
	}
	for _, appended := range appendedSections {
		fusedBody += "\n" + appended
	}

	return &FusedContent{
		Body: fusedBody,
		Tags: mergeStringSets(target.Tags, source.Tags),
	}, nil
}

// stripHeading removes the leading ## heading line from a section content string.
func stripHeading(section string) string {
	lines := strings.SplitN(section, "\n", 2)
	if len(lines) > 1 && strings.HasPrefix(strings.TrimSpace(lines[0]), "## ") {
		return lines[1]
	}
	return section
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
