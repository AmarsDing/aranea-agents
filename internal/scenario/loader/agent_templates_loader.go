package loader

import (
	"fmt"
	"os"
	"path/filepath"
)

type AgentTemplateSpec struct {
	Key         string `yaml:"key"`
	Label       string `yaml:"label"`
	Icon        string `yaml:"icon"`
	DisplayName string `yaml:"display_name"`
	Provider    string `yaml:"provider"`
	Model       string `yaml:"model"`
	Description string `yaml:"description"`
	SortOrder   int    `yaml:"sort_order"`
}

type AgentTemplatesSpec struct {
	Templates []AgentTemplateSpec `yaml:"templates"`
}

func LoadAgentTemplatesSpec(scenarioDir string) (*AgentTemplatesSpec, error) {
	path := filepath.Join(scenarioDir, "agent_templates.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var spec AgentTemplatesSpec
	if err := yamlUnmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &spec, nil
}
