// Package registry maps platform tool_key strings to built-in implementations (workspace, ADK-shipped, host).
package registry

// Tool keys match internal/biz effective tools and internal/data builtin tool seeds.
const (
	ExitLoop      = "exit_loop"
	GoogleSearch  = "google_search" // legacy catalog alias; effective tools use WebSearch
	WebSearch     = "web_search"
	WebFetch      = "web_fetch"
	LoadArtifacts = "load_artifacts"
	LoadMemory    = "load_memory"
	PreloadMemory = "preload_memory"

	ShellExec = "shell_exec"
	// ShellAlias is accepted in effective-tools UIs that label the tool "shell" only.
	ShellAlias = "shell"

	ReadFile    = "read_file"
	ListFiles  = "list_files"
	WriteFile  = "write_file"
	EditFile   = "edit_file"
	ExampleTool = "example_tool"

	SkillListSkills        = "list_skills"
	SkillLoadSkill         = "load_skill"
	SkillLoadSkillResource = "load_skill_resource"

	MCPToolset = "mcp_tool_set"
)

// WorkspaceToolNames is the stable order for filesystem builtins.
var WorkspaceToolNames = []string{ReadFile, ListFiles, WriteFile, EditFile}

// ADKBuiltinOrder is the order ADK catalog tools are appended after workspace tools.
var ADKBuiltinOrder = []string{
	ExitLoop,
	WebSearch,
	WebFetch,
	LoadArtifacts,
	LoadMemory,
	PreloadMemory,
}
