package loader

import (
	"strings"

	"gopkg.in/yaml.v3"
)

func yamlUnmarshal(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

func fillDefaults(spec *IndustrySpec) {
	d := &spec.Defaults
	if d.Provider == "" {
		d.Provider = "openrouter"
	}
	if d.FastModel == "" {
		d.FastModel = "gpt-4.1-mini"
	}
	if d.StrongModel == "" {
		d.StrongModel = "gpt-4.1"
	}
	if d.SystemPromptMode == "" {
		d.SystemPromptMode = "file"
	}
	if d.ContextWindow == 0 {
		d.ContextWindow = 64000
	}
	if d.CodeExecutor == "" {
		d.CodeExecutor = "local"
	}
	if len(d.ToolsDeny) == 0 {
		d.ToolsDeny = []string{"workspace_exec", "filesystem", "shell", "bash"}
	}
	for i := range spec.Agents {
		a := &spec.Agents[i]
		if a.Variant == "" {
			a.Variant = "general"
		}
		if a.ModelTier == "" {
			a.ModelTier = "fast"
		}
		if a.ToolsProfile == "" {
			if len(a.ToolsAllow) > 0 {
				a.ToolsProfile = deriveToolsProfile(a.PositionKey)
			} else {
				a.ToolsProfile = "general"
			}
		}
	}
}

func deriveToolsProfile(positionKey string) string {
	pk := strings.ToLower(positionKey)
	switch {
	case strings.Contains(pk, "analyst") || strings.Contains(pk, "research"):
		return "analyst"
	case strings.Contains(pk, "engineer") || strings.Contains(pk, "developer") || strings.Contains(pk, "programmer"):
		return "general"
	case strings.Contains(pk, "coordinator") || strings.Contains(pk, "manager") || strings.Contains(pk, "director"):
		return "coordinator"
	case strings.Contains(pk, "writer") || strings.Contains(pk, "editor"):
		return "writer"
	case strings.Contains(pk, "designer") || strings.Contains(pk, "artist"):
		return "creative"
	default:
		return "general"
	}
}
