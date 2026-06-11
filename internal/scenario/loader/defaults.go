package loader

import "gopkg.in/yaml.v3"

// 默认值常量集中在此处，为 loader 包和 data/pack_convert.go 的唯一真相源。
// data/pack_convert.go 直接引用这些常量，避免在 biz/pack 层重复定义。
const (
	DefaultProvider         = "openrouter"
	DefaultFastModel        = "gpt-4.1-mini"
	DefaultStrongModel      = "gpt-4.1"
	DefaultSystemPromptMode = "file"
	DefaultContextWindow    = 64000
	DefaultCodeExecutor     = "local"
	DefaultVariant          = "general"
	DefaultModelTier        = "fast"
	DefaultToolsProfile     = "general"
)

// DefaultToolsDeny 是 AgentDefaults 缺省拒绝的工具集。
// 出于"安全默认"考虑，文件 / shell / bash 类工具在没显式 allow 时一律 deny。
var DefaultToolsDeny = []string{"workspace_exec", "filesystem", "shell", "bash"}

func yamlUnmarshal(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

func fillDefaults(spec *CompanySpec) {
	d := &spec.Defaults
	if d.Provider == "" {
		d.Provider = DefaultProvider
	}
	if d.FastModel == "" {
		d.FastModel = DefaultFastModel
	}
	if d.StrongModel == "" {
		d.StrongModel = DefaultStrongModel
	}
	if d.SystemPromptMode == "" {
		d.SystemPromptMode = DefaultSystemPromptMode
	}
	if d.ContextWindow == 0 {
		d.ContextWindow = DefaultContextWindow
	}
	if d.CodeExecutor == "" {
		d.CodeExecutor = DefaultCodeExecutor
	}
	if len(d.ToolsDeny) == 0 {
		d.ToolsDeny = append([]string(nil), DefaultToolsDeny...)
	}
	for i := range spec.Agents {
		a := &spec.Agents[i]
		if a.Variant == "" {
			a.Variant = DefaultVariant
		}
		if a.ModelTier == "" {
			a.ModelTier = DefaultModelTier
		}
		if a.ToolsProfile == "" {
			a.ToolsProfile = DefaultToolsProfile
		}
	}
}
