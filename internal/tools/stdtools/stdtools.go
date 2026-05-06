// Package stdtools wires built-in workspace file tools and ADK-shipped tools for catalog assembly.
package stdtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/tools/edit_file"
	"aranea-agents/internal/tools/list_files"
	"aranea-agents/internal/tools/read_file"
	"aranea-agents/internal/tools/write_file"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/exitlooptool"
	"google.golang.org/adk/tool/geminitool"
	"google.golang.org/adk/tool/loadartifactstool"
	"google.golang.org/adk/tool/loadmemorytool"
	"google.golang.org/adk/tool/preloadmemorytool"
)

// WorkspaceToolNames is the stable order for filesystem tools (matches agent effective-tools policy).
var WorkspaceToolNames = []string{"read_file", "list_files", "write_file", "edit_file"}

// WorkspaceOpenAISpecs returns OpenAI-compat tool definitions for the native /chat/completions tool loop.
func WorkspaceOpenAISpecs(enabled map[string]bool) []map[string]any {
	var out []map[string]any
	for _, key := range WorkspaceToolNames {
		if enabled != nil && !enabled[key] {
			continue
		}
		switch key {
		case "read_file":
			out = append(out, read_file.OpenAIFunctionSpec())
		case "list_files":
			out = append(out, list_files.OpenAIFunctionSpec())
		case "write_file":
			out = append(out, write_file.OpenAIFunctionSpec())
		case "edit_file":
			out = append(out, edit_file.OpenAIFunctionSpec())
		}
	}
	return out
}

// WorkspaceADKTools returns ADK tools for the four filesystem builtins.
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
	case "read_file":
		return read_file.New()
	case "list_files":
		return list_files.New()
	case "write_file":
		return write_file.New()
	case "edit_file":
		return edit_file.New()
	default:
		return nil, fmt.Errorf("stdtools: unknown workspace tool %q", name)
	}
}

// AppendCatalogTool appends one built-in tool by catalog name ([catalog.known] constants).
func AppendCatalogTool(name string, out *[]tool.Tool) error {
	name = strings.TrimSpace(name)
	switch name {
	case "exit_loop":
		t, err := exitlooptool.New()
		if err != nil {
			return err
		}
		*out = append(*out, t)
		return nil
	case "load_memory":
		*out = append(*out, loadmemorytool.New())
		return nil
	case "preload_memory":
		*out = append(*out, preloadmemorytool.New())
		return nil
	case "load_artifacts":
		*out = append(*out, loadartifactstool.New())
		return nil
	case "google_search":
		*out = append(*out, geminitool.GoogleSearch{})
		return nil
	default:
		return fmt.Errorf("stdtools: catalog tool %q is not a built-in ADK export", name)
	}
}

// InvokeResponse is a small envelope for in-process filesystem tool invocation (legacy OpenAI loop).
type InvokeResponse struct {
	OK     bool           `json:"ok"`
	Error  string         `json:"error,omitempty"`
	Result map[string]any `json:"result,omitempty"`
}

// InvokeJSON parses args JSON and dispatches workspace tools only (fast local path).
func InvokeJSON(ctx context.Context, name, argsJSON string) InvokeResponse {
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
	case "read_file":
		res, err = read_file.Run(ctx, args)
	case "list_files":
		res, err = list_files.Run(ctx, args)
	case "write_file":
		res, err = write_file.Run(ctx, args)
	case "edit_file":
		res, err = edit_file.Run(ctx, args)
	default:
		return InvokeResponse{OK: false, Error: fmt.Sprintf("tool %q has no local invoke path", name)}
	}
	if err != nil {
		return InvokeResponse{OK: false, Error: err.Error()}
	}
	if res == nil {
		res = map[string]any{}
	}
	return InvokeResponse{OK: true, Result: res}
}
