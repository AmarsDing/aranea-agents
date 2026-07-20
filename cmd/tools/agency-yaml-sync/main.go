// Package main implements a one-shot sync tool that updates all agent YAML
// files' display_name and description fields to match taxonomy.yaml.
//
// Usage:
//
//	go run ./cmd/tools/agency-yaml-sync \
//	  -pack internal/scenario/packs/agency-pack
//
// The tool reads taxonomy.yaml, iterates every position, and overwrites the
// display_name / description in agents/{key}__general.yaml with the Chinese
// values from taxonomy.yaml. All other YAML fields are preserved.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// --- taxonomy structures ---

type tVariant struct {
	Key  string `yaml:"key"`
	Name string `yaml:"name"`
}

type tPosition struct {
	Key            string     `yaml:"key"`
	Name           string     `yaml:"name"`
	Description    string     `yaml:"description"`
	SortOrder      int        `yaml:"sort_order"`
	SeniorityLevel string     `yaml:"seniority_level"`
	Variants       []tVariant `yaml:"variants"`
}

type tDepartment struct {
	Key         string      `yaml:"key"`
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	SortOrder   int         `yaml:"sort_order"`
	Positions   []tPosition `yaml:"positions"`
}

type tCompany struct {
	Key         string        `yaml:"key"`
	Name        string        `yaml:"name"`
	Icon        string        `yaml:"icon"`
	Description string        `yaml:"description"`
	SortOrder   int           `yaml:"sort_order"`
	Departments []tDepartment `yaml:"departments"`
}

type taxonomy struct {
	Companies []tCompany `yaml:"companies"`
}

// --- agent YAML structures (only what we need to preserve) ---

type agentFile struct {
	Key              string   `yaml:"key"`
	DisplayName      string   `yaml:"display_name"`
	Description      string   `yaml:"description"`
	Icon             string   `yaml:"icon"`
	PositionKey      string   `yaml:"position_key"`
	Variant          string   `yaml:"variant"`
	VariantDesc      string   `yaml:"variant_description"`
	Provider         string   `yaml:"provider"`
	Model            string   `yaml:"model"`
	ModelTier        string   `yaml:"model_tier"`
	SystemPromptMode string   `yaml:"system_prompt_mode"`
	ContextWindow    int      `yaml:"context_window"`
	ToolsDeny        []string `yaml:"tools_deny"`
	OwnershipKind    string   `yaml:"ownership_kind"`
	Source           string   `yaml:"source"`
	Files            []struct {
		Name string `yaml:"name"`
	} `yaml:"files"`
	// capture any extra fields we don't explicitly model
}

func main() {
	packDir := flag.String("pack", "internal/scenario/packs/agency-pack", "path to agency-pack directory")
	dryRun := flag.Bool("dry-run", false, "print changes without writing files")
	flag.Parse()

	// 1. Read taxonomy.yaml
	taxPath := filepath.Join(*packDir, "taxonomy.yaml")
	taxData, err := os.ReadFile(taxPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR reading taxonomy: %v\n", err)
		os.Exit(1)
	}
	var tax taxonomy
	if err := yaml.Unmarshal(taxData, &tax); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR parsing taxonomy: %v\n", err)
		os.Exit(1)
	}

	// 2. Build map: agentKey -> (name, description)
	type meta struct {
		name, desc string
	}
	agentMetas := make(map[string]meta)
	for _, co := range tax.Companies {
		for _, dept := range co.Departments {
			for _, pos := range dept.Positions {
				agentKey := pos.Key + "__general"
				agentMetas[agentKey] = meta{name: pos.Name, desc: pos.Description}
			}
		}
	}
	fmt.Printf("Loaded %d agents from taxonomy.yaml\n", len(agentMetas))

	// 3. For each agent, read YAML, update display_name/description, write back
	agentsDir := filepath.Join(*packDir, "agents")
	updated := 0
	skipped := 0
	missing := 0

	for agentKey, m := range agentMetas {
		yamlPath := filepath.Join(agentsDir, agentKey+".yaml")
		raw, err := os.ReadFile(yamlPath)
		if err != nil {
			fmt.Printf("  MISSING: %s (%v)\n", agentKey, err)
			missing++
			continue
		}

		// Parse into yaml.Node to preserve everything, then update fields
		var node yaml.Node
		if err := yaml.Unmarshal(raw, &node); err != nil {
			fmt.Printf("  PARSE ERROR: %s (%v)\n", agentKey, err)
			missing++
			continue
		}

		// Use the typed struct for comparison + re-serialization
		var af agentFile
		if err := yaml.Unmarshal(raw, &af); err != nil {
			fmt.Printf("  PARSE ERROR: %s (%v)\n", agentKey, err)
			missing++
			continue
		}

		if af.DisplayName == m.name && af.Description == m.desc {
			skipped++
			continue
		}

		af.DisplayName = m.name
		af.Description = m.desc

		out, err := yaml.Marshal(&af)
		if err != nil {
			fmt.Printf("  MARSHAL ERROR: %s (%v)\n", agentKey, err)
			missing++
			continue
		}

		if *dryRun {
			fmt.Printf("  WOULD UPDATE: %s\n    display_name: %q -> %q\n    description: %q -> %q\n",
				agentKey, af.DisplayName, m.name, truncate(af.Description, 60), truncate(m.desc, 60))
		} else {
			if err := os.WriteFile(yamlPath, out, 0644); err != nil {
				fmt.Printf("  WRITE ERROR: %s (%v)\n", agentKey, err)
				missing++
				continue
			}
		}
		updated++
	}

	fmt.Printf("\nSummary: updated=%d skipped=%d missing=%d total=%d\n",
		updated, skipped, missing, len(agentMetas))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
