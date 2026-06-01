package modelregistry

type CapabilityChip struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Source string `json:"source"`
}

func BuildCapabilityChips(m Model) []CapabilityChip {
	out := make([]CapabilityChip, 0, 6)
	add := func(key, label string) {
		out = append(out, CapabilityChip{Key: key, Label: label, Source: "catalog"})
	}
	if m.ToolCall {
		add("tool_call", "工具调用")
	}
	if m.Reasoning {
		add("reasoning", "推理")
	}
	if m.Attachment {
		add("attachment", "附件")
	}
	if m.StructuredOutput != nil && *m.StructuredOutput {
		add("structured_output", "结构化输出")
	}
	if m.Temperature != nil && *m.Temperature {
		add("temperature", "Temperature")
	}
	if m.OpenWeights {
		add("open_weights", "开放权重")
	}
	switch m.Status {
	case "deprecated":
		add("deprecated", "已废弃")
	case "beta":
		add("beta", "Beta")
	case "alpha":
		add("alpha", "Alpha")
	}
	for _, mod := range m.Modalities.Input {
		if mod == "image" || mod == "video" {
			add("vision", "视觉")
			break
		}
	}
	return out
}
