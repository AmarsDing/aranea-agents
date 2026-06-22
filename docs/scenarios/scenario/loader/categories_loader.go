package loader

import (
	"fmt"
	"os"
	"path/filepath"
)

type CategoryIndustrySpec struct {
	Key         string             `yaml:"key"`
	Name        string             `yaml:"name"`
	Icon        string             `yaml:"icon"`
	Description string             `yaml:"description"`
	SortOrder   int                `yaml:"sort_order"`
	Departments []CategoryDeptSpec `yaml:"departments"`
}

type CategoryDeptSpec struct {
	Key         string                 `yaml:"key"`
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	SortOrder   int                    `yaml:"sort_order"`
	Positions   []CategoryPositionSpec `yaml:"positions"`
}

type CategoryPositionSpec struct {
	Key         string `yaml:"key"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	SortOrder   int    `yaml:"sort_order"`
}

type CategoriesSpec struct {
	Industries []CategoryIndustrySpec `yaml:"industries"`
}

func LoadCategoriesSpec(scenarioDir string) (*CategoriesSpec, error) {
	path := filepath.Join(scenarioDir, "categories.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var spec CategoriesSpec
	if err := yamlUnmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &spec, nil
}
