package loader

import (
	"fmt"
	"os"
	"path/filepath"
)

type TaxonomyIndustrySpec struct {
	Key         string                 `yaml:"key"`
	Name        string                 `yaml:"name"`
	Icon        string                 `yaml:"icon"`
	Description string                 `yaml:"description"`
	SortOrder   int                    `yaml:"sort_order"`
	Departments []TaxonomyDeptSpec     `yaml:"departments"`
}

type TaxonomyDeptSpec struct {
	Key         string                  `yaml:"key"`
	Name        string                  `yaml:"name"`
	Description string                  `yaml:"description"`
	SortOrder   int                     `yaml:"sort_order"`
	Positions   []TaxonomyPositionSpec  `yaml:"positions"`
}

type TaxonomyPositionSpec struct {
	Key         string `yaml:"key"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	SortOrder   int    `yaml:"sort_order"`
}

type TaxonomySpec struct {
	Industries []TaxonomyIndustrySpec `yaml:"industries"`
}

func LoadTaxonomySpec(scenarioDir string) (*TaxonomySpec, error) {
	path := filepath.Join(scenarioDir, "taxonomy.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var spec TaxonomySpec
	if err := yamlUnmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &spec, nil
}
