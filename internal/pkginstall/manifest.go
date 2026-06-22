// Package pkginstall implements the `aranea pkg install` command.
//
// Architectural contract (redlines):
//   - This package MUST NOT import internal/biz, internal/agent, internal/data,
//     internal/server, or pkg/trpc-agent-go.
//   - All communication with the backend is via HTTP API calls.
//   - Org import reuses the orgimport package.
package pkginstall

import "aranea-agents/internal/orgimport"

// Manifest is the top-level aranea-package.yaml structure.
type Manifest struct {
	Version  int              `yaml:"version"` // 1
	Metadata ManifestMetadata `yaml:"metadata"`
	Spec     ManifestSpec     `yaml:"spec"`
}

// ManifestMetadata holds package identity metadata.
type ManifestMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
	Version     string `yaml:"version"`
}

// ManifestSpec is the installation payload.
type ManifestSpec struct {
	// Org structure (reuse orgimport types).
	Companies []orgimport.OrganizationSpec `yaml:"companies"`
	Agents    []orgimport.AgentSpec        `yaml:"agents"`
	Teams     []orgimport.TeamSpec         `yaml:"teams"`
	// New resource types.
	MCPServers []MCPServerSpec `yaml:"mcp_servers"`
	Skills     []SkillSpec     `yaml:"skills"`
	Graphs     []GraphSpec     `yaml:"graphs"`
}

// MCPServerSpec describes an MCP server to install.
type MCPServerSpec struct {
	Name        string         `yaml:"name"`
	Key         string         `yaml:"key"`
	URL         string         `yaml:"url"`
	Type        string         `yaml:"type"` // sse | stdio
	Description string         `yaml:"description"`
	Config      map[string]any `yaml:"config"`
	Enabled     bool           `yaml:"enabled"`
}

// SkillSpec describes a skill to install.
type SkillSpec struct {
	// Source: from URL (git repo) or local path relative to package root.
	URL      string `yaml:"url"`
	Ref      string `yaml:"ref"`
	Subpath  string `yaml:"subpath"`
	Path     string `yaml:"path"`
	Decision string `yaml:"decision"` // skip|keep|refine; conflict default strategy
}

// GraphSpec describes a graph to import.
type GraphSpec struct {
	// Local JSON file relative to package root directory.
	File string `yaml:"file"`
	Name string `yaml:"name"`
}
