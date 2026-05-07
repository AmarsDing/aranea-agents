package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/tools/edit_file"
	"aranea-agents/internal/tools/list_files"
	"aranea-agents/internal/tools/read_file"
	"aranea-agents/internal/tools/write_file"
)

// InvokeResponse is the legacy OpenAI tool-loop envelope for in-process workspace tools.
type InvokeResponse struct {
	OK     bool           `json:"ok"`
	Error  string         `json:"error,omitempty"`
	Result map[string]any `json:"result,omitempty"`
}

// InvokeWorkspaceJSON dispatches workspace filesystem tools only.
func InvokeWorkspaceJSON(ctx context.Context, name, argsJSON string) InvokeResponse {
	name = strings.TrimSpace(name)
	if name == "" {
		return InvokeResponse{OK: false, Error: "empty tool name"}
	}
	var args map[string]any
	if strings.TrimSpace(argsJSON) != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return InvokeResponse{OK: false, Error: fmt.Sprintf("invalid JSON args: %v", err)}
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	var res map[string]any
	var err error
	switch name {
	case ReadFile:
		res, err = read_file.Run(ctx, args)
	case ListFiles:
		res, err = list_files.Run(ctx, args)
	case WriteFile:
		res, err = write_file.Run(ctx, args)
	case EditFile:
		res, err = edit_file.Run(ctx, args)
	default:
		return InvokeResponse{OK: false, Error: fmt.Sprintf("tool %q has no workspace invoke path", name)}
	}
	if err != nil {
		return InvokeResponse{OK: false, Error: err.Error()}
	}
	if res == nil {
		res = map[string]any{}
	}
	return InvokeResponse{OK: true, Result: res}
}
