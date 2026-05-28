package render

import (
	"strings"

	"aranea-agents/internal/skill/manifest"
)

type RenderOptions struct {
	Variables map[string]string
}

func SkillGuidance(m manifest.Manifest, opts RenderOptions) string {
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
