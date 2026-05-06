package specs

// OpenAI builds one OpenAI-compat chat completions tools[] function definition.
func OpenAI(name, description string, parameters map[string]any) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": description,
			"parameters":  parameters,
		},
	}
}
