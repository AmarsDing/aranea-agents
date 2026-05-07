// Package edit_file 在已有文件中做「单次、唯一匹配」的小块文本替换。
package edit_file

import (
	"context"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/tools/argmap"
	"aranea-agents/internal/tools/specs"
	"aranea-agents/internal/tools/workspace"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type efArgs struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

const desc = `Replace exactly one occurrence of old_string with new_string in an existing file. Prefer read_file first.`

func Run(ctx context.Context, argsMap map[string]any) (map[string]any, error) {
	_ = ctx
	oldStr := ""
	if v, ok := argsMap["old_string"]; ok && v != nil {
		oldStr = fmt.Sprint(v)
	}
	if strings.TrimSpace(oldStr) == "" {
		return nil, fmt.Errorf("old_string is required")
	}
	newStr := ""
	if v, ok := argsMap["new_string"]; ok && v != nil {
		newStr = fmt.Sprint(v)
	}
	path := argmap.String(argsMap, "path")
	abs, rel, err := workspace.ResolvePath(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return nil, fmt.Errorf("old_string was not found in %q", rel)
	}
	if count > 1 {
		return nil, fmt.Errorf("old_string matched %d times in %q; provide more context", count, rel)
	}
	next := strings.Replace(content, oldStr, newStr, 1)
	if err := os.WriteFile(abs, []byte(next), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "replacements": 1}, nil
}

func New() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "edit_file",
		Description: desc,
	}, func(_ tool.Context, in efArgs) (map[string]any, error) {
		return Run(context.Background(),
			map[string]any{"path": in.Path, "old_string": in.OldString, "new_string": in.NewString})
	})
}

func OpenAIFunctionSpec() map[string]any {
	return specs.OpenAI("edit_file", desc, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":       map[string]any{"type": "string"},
			"old_string": map[string]any{"type": "string"},
			"new_string": map[string]any{"type": "string"},
		},
		"required": []string{"path", "old_string", "new_string"},
	})
}
