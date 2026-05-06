// Package write_file 在工作区内创建或覆盖 UTF-8 文本文件并按需创建父目录。
package write_file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/tools/argmap"
	"aranea-agents/internal/tools/specs"
	"aranea-agents/internal/tools/workspace"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type wfArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

const desc = `Create or overwrite a UTF-8 file under the workspace. Parent directories are created as needed.`

func Run(ctx context.Context, argsMap map[string]any) (map[string]any, error) {
	_ = ctx
	path := argmap.String(argsMap, "path")
	var content string
	if v, ok := argsMap["content"]; ok && v != nil {
		content = fmt.Sprint(v)
	}
	abs, rel, err := workspace.ResolvePath(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "written": len(content)}, nil
}

func New() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "write_file",
		Description: desc,
	}, func(_ tool.Context, in wfArgs) (map[string]any, error) {
		return Run(context.Background(), map[string]any{"path": in.Path, "content": in.Content})
	})
}

func OpenAIFunctionSpec() map[string]any {
	return specs.OpenAI("write_file", desc, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Relative path under workspace root"},
			"content": map[string]any{"type": "string", "description": "Full file contents"},
		},
		"required": []string{"path", "content"},
	})
}
