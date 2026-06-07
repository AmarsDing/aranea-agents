package loader

import (
	"fmt"
	"os"
	"path/filepath"
)

type OrgCompanySpec struct {
	Key         string                 `yaml:"key"`
	Name        string                 `yaml:"name"`
	Icon        string                 `yaml:"icon"`
	Description string                 `yaml:"description"`
	SortOrder   int                    `yaml:"sort_order"`
	Departments []OrgDepartmentSpec    `yaml:"departments"`
}

type OrgDepartmentSpec struct {
	Key         string                  `yaml:"key"`
	Name        string                  `yaml:"name"`
	Description string                  `yaml:"description"`
	SortOrder   int                     `yaml:"sort_order"`
	Positions   []OrgPositionSpec       `yaml:"positions"`
}

type OrgPositionSpec struct {
	Key             string                `yaml:"key"`
	Name            string                `yaml:"name"`
	Description     string                `yaml:"description"`
	SortOrder       int                   `yaml:"sort_order"`
	SeniorityLevel  string                `yaml:"seniority_level"`
	SkillsRequired  []string              `yaml:"skills_required"`
	Responsibilities []string             `yaml:"responsibilities"`
	Variants        []OrgVariantSpec      `yaml:"variants"`
}

type OrgVariantSpec struct {
	Key  string `yaml:"key"`
	Name string `yaml:"name"`
}

type OrganizationSpec struct {
	Companies []OrgCompanySpec `yaml:"companies"`
	// LegacyIndustries supports reading the old "industries" key for backward compatibility.
	// If Companies is empty but Industries is populated, the loader migrates automatically.
	LegacyIndustries []OrgCompanySpec `yaml:"industries"`
}

// ResolvedCompanies returns the companies list, falling back to the legacy
// "industries" key when the new "companies" key is absent or empty.
func (s *OrganizationSpec) ResolvedCompanies() []OrgCompanySpec {
	if len(s.Companies) > 0 {
		return s.Companies
	}
	return s.LegacyIndustries
}

func LoadOrganizationSpec(scenarioDir string) (*OrganizationSpec, error) {
	// Prefer organization.yaml; fall back to taxonomy.yaml for backward compatibility.
	path := filepath.Join(scenarioDir, "organization.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		legacyPath := filepath.Join(scenarioDir, "taxonomy.yaml")
		data, err = os.ReadFile(legacyPath)
		if err != nil {
			return nil, fmt.Errorf("read organization.yaml (and fallback taxonomy.yaml): %w", err)
		}
		path = legacyPath
	}
	var spec OrganizationSpec
	if err := yamlUnmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &spec, nil
}
