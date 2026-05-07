package agent

import (
	"context"
	"errors"

	"aranea-agents/internal/tools/registry"
)

func nativeToolDefinitionsForKeys(enabled map[string]bool) []map[string]any {
	return registry.WorkspaceOpenAISpecs(enabled)
}

func executeNativeFilesystemTool(name string, argsJSON string) (map[string]any, error) {
	resp := registry.InvokeWorkspaceJSON(context.Background(), name, argsJSON)
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	if resp.Result == nil {
		resp.Result = map[string]any{}
	}
	return resp.Result, nil
}
