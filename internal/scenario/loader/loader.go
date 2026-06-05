package loader

import (
	"fmt"
	"os"
	"path/filepath"
)

// Deps holds dependencies for seed operations (kept for compatibility; SeedFromYAML is removed).
type Deps struct {
	AgentUC     interface{ GetByAgentKey(ctx interface{}, key string) (interface{}, error) }
	TeamUC      interface{}
	Taxonomy    interface{}
	ScenarioDir string
}

func LoadIndustrySpec(scenarioDir, industryKey string) (*IndustrySpec, error) {
	path := filepath.Join(scenarioDir, industryKey, "agents.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var spec IndustrySpec
	if err := yamlUnmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	spec.IndustryKey = industryKey
	fillDefaults(&spec)
	return &spec, nil
}
