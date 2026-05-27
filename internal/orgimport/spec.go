// Package orgimport implements PGO-4: CLI-driven automated import of
// organization (industry → department → position → agent + team) structure.
//
// Architectural contract (redlines):
//   - This package MUST NOT import internal/biz, internal/agent, internal/data,
//     internal/server, or pkg/trpc-agent-go.
//   - All communication with the backend is via HTTP API calls (see applier.go).
//   - The CLI binary (cmd/aranea) imports this package and nothing else from internal/.
//
// Package name is orgimport rather than "import" to avoid collision with the Go keyword.
package orgimport

// ─── YAML spec types ─────────────────────────────────────────────────────────

// Spec is the top-level import specification document.
// Loaded from YAML (direct user authoring) or produced by the LLM extractor
// (markdown → YAML → Spec).
type Spec struct {
	Version  int          `yaml:"version"`
	Metadata SpecMetadata `yaml:"metadata"`
	Spec     SpecBody     `yaml:"spec"`
}

// SpecMetadata holds audit and idempotency fields.
type SpecMetadata struct {
	CorrelationID string `yaml:"correlation_id"` // uuid-like; empty → CLI generates one
	SourceFile    string `yaml:"source_file"`
	GeneratedBy   string `yaml:"generated_by"`
}

// SpecBody is the payload of the spec document.
type SpecBody struct {
	Industries []IndustrySpec `yaml:"industries"`
	Agents     []AgentSpec    `yaml:"agents"`
	Teams      []TeamSpec     `yaml:"teams"`
}

// IndustrySpec describes a top-level industry node.
type IndustrySpec struct {
	Key         string           `yaml:"key"`
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Departments []DepartmentSpec `yaml:"departments"`
}

// DepartmentSpec describes a mid-level department node.
type DepartmentSpec struct {
	Key         string         `yaml:"key"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Positions   []PositionSpec `yaml:"positions"`
}

// PositionSpec describes a leaf-level position node (岗位).
type PositionSpec struct {
	Key         string `yaml:"key"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"` // 岗位职责
}

// AgentSpec describes one agent to create / update.
type AgentSpec struct {
	Key              string            `yaml:"key"`
	DisplayName      string            `yaml:"display_name"`
	CategoryPosition string            `yaml:"category_position"` // path: "ind/dept/pos"
	Provider         string            `yaml:"provider"`
	Model            string            `yaml:"model"`
	SystemPromptMode string            `yaml:"system_prompt_mode"` // complete|task|minimized|none
	AgentDescription string            `yaml:"agent_description"`
	Files            map[string]string `yaml:"files"` // filename → body; empty → use V2 defaults
	Refine           bool              `yaml:"refine"` // trigger AI refinement for this agent
}

// TeamSpec describes one team to create.
type TeamSpec struct {
	Key        string       `yaml:"key"`
	Name       string       `yaml:"name"`
	Members    []MemberSpec `yaml:"members"`
}

// MemberSpec is one agent within a team.
type MemberSpec struct {
	AgentKey string `yaml:"agent_key"` // references AgentSpec.Key
	Role     string `yaml:"role"`      // orchestrator | member
}
