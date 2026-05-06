package agent

import (
	"context"
	"errors"

	"aranea-agents/internal/tools/toolapi"
)

func nativeToolDefinitionsForKeys(enabled map[string]bool) []map[string]any {
	return toolapi.Default().WorkspaceOpenAISpecs(enabled)
}

func executeNativeFilesystemTool(name string, argsJSON string) (map[string]any, error) {
	resp := toolapi.Default().InvokeJSON(context.Background(), name, argsJSON)
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	if resp.Result == nil {
		resp.Result = map[string]any{}
	}
	return resp.Result, nil
}
