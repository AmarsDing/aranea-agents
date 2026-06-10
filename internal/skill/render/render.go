package render

import (
	"strings"

	"aranea-agents/internal/skill/manifest"
)

const (
	// ModeFull renders the complete skill guidance (default).
	ModeFull = "full"
	// ModeAIOptimized renders a compact version optimized for LLM context windows.
	ModeAIOptimized = "ai_optimized"
)

// Headings to exclude in AI-optimized mode (background/intro/example sections).
var aiOptimizedExcludeHeadings = []string{
	"介绍", "背景", "概述", "简介", "overview", "introduction", "background",
	"完整步骤", "详细步骤", "示例", "example", "full steps",
	"changelog", "变更日志", "历史",
}

type RenderOptions struct {
	Variables map[string]string
	Mode      string // "full" (default) or "ai_optimized"
}

func SkillGuidance(m manifest.Manifest, opts RenderOptions) string {
	if opts.Mode == ModeAIOptimized {
		return skillGuidanceAIOptimized(m, opts)
	}
	return skillGuidanceFull(m, opts)
}

func skillGuidanceFull(m manifest.Manifest, opts RenderOptions) string {
	var b strings.Builder
	if m.Name != "" {
		b.WriteString("## ")
		b.WriteString(m.Name)
		b.WriteString("\n")
	}
	if m.Description != "" {
		b.WriteString(m.Description)
		b.WriteString("\n")
	}
	body := m.Body
	if len(opts.Variables) > 0 {
		for k, v := range opts.Variables {
			body = strings.ReplaceAll(body, "{{"+k+"}}", v)
		}
	}
	b.WriteString(body)
	return b.String()
}

func skillGuidanceAIOptimized(m manifest.Manifest, opts RenderOptions) string {
	var b strings.Builder
	if m.Name != "" {
		b.WriteString("## ")
		b.WriteString(m.Name)
		b.WriteString("\n")
	}
	if m.Description != "" {
		desc := m.Description
		runes := []rune(desc)
		if len(runes) > 120 {
			desc = string(runes[:117]) + "..."
		}
		b.WriteString(desc)
		b.WriteString("\n")
	}
	// Triggers
	if len(m.Triggers) > 0 {
		b.WriteString("Triggers: ")
		b.WriteString(strings.Join(m.Triggers, ", "))
		b.WriteString("\n")
	}
	// Tools
	if len(m.Tools) > 0 {
		b.WriteString("Tools: ")
		b.WriteString(strings.Join(m.Tools, ", "))
		b.WriteString("\n")
	}
	// Body: only include decision-tree sections (## headings that are NOT in exclude list)
	body := m.Body
	if len(opts.Variables) > 0 {
		for k, v := range opts.Variables {
			body = strings.ReplaceAll(body, "{{"+k+"}}", v)
		}
	}
	b.WriteString(filterDecisionSections(body))
	return b.String()
}

// filterDecisionSections keeps only ## sections that are NOT in the exclude list.
// Lines before the first ## heading are dropped (typically intro text).
func filterDecisionSections(body string) string {
	var b strings.Builder
	lines := strings.Split(body, "\n")
	included := false
	skipSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			skipSection = isExcludedHeading(heading)
			if !skipSection {
				included = true
				b.WriteString(line)
				b.WriteString("\n")
			}
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			// Top-level heading, skip in AI mode (Name already rendered)
			continue
		}
		if skipSection {
			continue
		}
		if included {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func isExcludedHeading(heading string) bool {
	lower := strings.ToLower(strings.TrimSpace(heading))
	for _, excl := range aiOptimizedExcludeHeadings {
		if lower == strings.ToLower(excl) {
			return true
		}
	}
	// Also match if the heading starts with an excluded keyword followed by
	// a separator (e.g., "Overview:" or "Introduction to...").
	for _, excl := range aiOptimizedExcludeHeadings {
		prefix := strings.ToLower(excl)
		if strings.HasPrefix(lower, prefix) && len(lower) > len(prefix) {
			ch := rune(lower[len(prefix)])
			if ch == ' ' || ch == ':' || ch == '—' || ch == '-' || ch == '：' {
				return true
			}
		}
	}
	return false
}
