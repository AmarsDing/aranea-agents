package registry

import (
	"fmt"

	"aranea-agents/internal/tools/edit_file"
	"aranea-agents/internal/tools/list_files"
	"aranea-agents/internal/tools/read_file"
	"aranea-agents/internal/tools/write_file"

	"google.golang.org/adk/tool"
)

// WorkspaceOpenAISpecs returns OpenAI-compat tool definitions for the legacy /chat/completions loop.
func WorkspaceOpenAISpecs(enabled map[string]bool) []map[string]any {
	var out []map[string]any
	for _, key := range WorkspaceToolNames {
		if enabled != nil && !enabled[key] {
			continue
		}
		switch key {
		case ReadFile:
			out = append(out, read_file.OpenAIFunctionSpec())
		case ListFiles:
			out = append(out, list_files.OpenAIFunctionSpec())
		case WriteFile:
			out = append(out, write_file.OpenAIFunctionSpec())
		case EditFile:
			out = append(out, edit_file.OpenAIFunctionSpec())
		}
	}
	return out
}

// WorkspaceADKTools returns ADK tools for enabled workspace filesystem builtins.
func WorkspaceADKTools(enabled map[string]bool) ([]tool.Tool, error) {
	var out []tool.Tool
	for _, key := range WorkspaceToolNames {
		if enabled != nil && !enabled[key] {
			continue
		}
		t, err := workspaceADK(key)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func workspaceADK(name string) (tool.Tool, error) {
	switch name {
	case ReadFile:
		return read_file.New()
	case ListFiles:
		return list_files.New()
	case WriteFile:
		return write_file.New()
	case EditFile:
		return edit_file.New()
	default:
		return nil, fmt.Errorf("registry: unknown workspace tool %q", name)
	}
}
