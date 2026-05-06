package toolapi

// BuildOpenAISpec 构造 OpenAI-compat /chat/completions 中单条函数工具 definition 片段。
func BuildOpenAISpec(name, description string, parameters map[string]any) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": description,
			"parameters":  parameters,
		},
	}
}
