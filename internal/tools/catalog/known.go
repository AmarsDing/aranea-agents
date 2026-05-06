package catalog

import (
	"aranea-agents/internal/tools/registry"

	"google.golang.org/adk/tool"
)

// Name* mirror [registry] keys for stable call sites in this package.

// 工作区内置文件工具名序列见 [registry.WorkspaceToolNames].
const (
	NameExitLoop      = registry.ExitLoop
	NameGoogleSearch  = registry.GoogleSearch
	NameLoadArtifacts = registry.LoadArtifacts
	NameLoadMemory    = registry.LoadMemory
	NamePreloadMemory = registry.PreloadMemory
	NameExampleTool   = registry.ExampleTool

	NameSkillListSkills        = registry.SkillListSkills
	NameSkillLoadSkill         = registry.SkillLoadSkill
	NameSkillLoadSkillResource = registry.SkillLoadSkillResource

	NameMCPToolset = registry.MCPToolset

	NameFilesystemRead  = registry.ReadFile
	NameFilesystemList  = registry.ListFiles
	NameFilesystemWrite = registry.WriteFile
	NameFilesystemEdit  = registry.EditFile
	NameShellExec       = registry.ShellExec
)

// WithConfirmation wraps a tool.Toolset with HITL confirmation (tool package; experimental upstream API).
func WithConfirmation(ts tool.Toolset, require bool, provider tool.ConfirmationProvider) tool.Toolset {
	return tool.WithConfirmation(ts, require, provider)
}
