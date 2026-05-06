package catalog

// ADKStandardTools returns Options with every stateless ADK tool from pkg/adk-go/tool enabled
// except exampletool, MCP, skills, agenttool children, and workspace filesystem.
// Turn on Filesystem in Options for project file tools; add Examples / SkillsFS / MCP / SubAgents as needed.
func ADKStandardTools() Options {
	return Options{
		ExitLoop:      true,
		GoogleSearch:  true,
		LoadArtifacts: true,
		LoadMemory:    true,
		PreloadMemory: true,
	}
}
