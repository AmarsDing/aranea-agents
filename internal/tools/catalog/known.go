package catalog

import "google.golang.org/adk/tool"

// 工作区内置文件工具名序列见 [stdtools.WorkspaceToolNames].
const (
	NameExitLoop      = "exit_loop"
	NameGoogleSearch  = "google_search"
	NameLoadArtifacts = "load_artifacts"
	NameLoadMemory    = "load_memory"
	NamePreloadMemory = "preload_memory"
	NameExampleTool   = "example_tool"

	NameSkillListSkills        = "list_skills"
	NameSkillLoadSkill         = "load_skill"
	NameSkillLoadSkillResource = "load_skill_resource"

	NameMCPToolset = "mcp_tool_set"

	NameFilesystemRead  = "read_file"
	NameFilesystemList  = "list_files"
	NameFilesystemWrite = "write_file"
	NameFilesystemEdit  = "edit_file"
)

// WithConfirmation wraps a tool.Toolset with HITL confirmation (tool package; experimental upstream API).
func WithConfirmation(ts tool.Toolset, require bool, provider tool.ConfirmationProvider) tool.Toolset {
	return tool.WithConfirmation(ts, require, provider)
}
