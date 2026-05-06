package agent

import (
	"context"
	"errors"

	"aranea-agents/internal/tools/stdtools"
)

func nativeToolDefinitionsForKeys(enabled map[string]bool) []map[string]any {
	return stdtools.WorkspaceOpenAISpecs(enabled)
}

func executeNativeFilesystemTool(name string, argsJSON string) (map[string]any, error) {
	resp := stdtools.InvokeJSON(context.Background(), name, argsJSON)
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	if resp.Result == nil {
		resp.Result = map[string]any{}
	}
	return resp.Result, nil
}
