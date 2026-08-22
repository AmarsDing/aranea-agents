package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const editDisciplineHookPriority = 9 // after workspace sandbox (8), before confirm (10)

// newEditDisciplineBeforeHook rejects save_file overwrites of existing files
// on coding-facing agents (E5). New files still go through save_file; existing
// files must use diff_edit / patch_file / replace_content.
func newEditDisciplineBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.BeforeToolHook {
	lg := deps.Logger()
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return callbacks.NewBeforeToolHook(editDisciplineHookPriority, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil || !ShouldAttachWorkingContract(ag) {
			return callbacks.Pass().BeforeToolResult(ctx), nil
		}
		if canonicalSaveFileName(args.ToolName) != "save_file" {
			return callbacks.Pass().BeforeToolResult(ctx), nil
		}
		rel := extractEditPath(args.Arguments)
		if rel == "" {
			return callbacks.Pass().BeforeToolResult(ctx), nil
		}
		root := sandboxWorkspaceRoot(ctx, ag, deps)
		resolved := rel
		if !filepath.IsAbs(rel) {
			resolved = filepath.Join(root, rel)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return callbacks.Pass().BeforeToolResult(ctx), nil
		}
		if info.IsDir() {
			return callbacks.Pass().BeforeToolResult(ctx), nil
		}
		reason := fmt.Sprintf("edit discipline: %q already exists. Use diff_edit, patch_file, or replace_content for existing files; save_file is only for new files.", rel)
		lg.Info("edit discipline blocked save_file overwrite",
			loggateway.StepID("agent.edit_discipline"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Str("path", rel))
		return callbacks.Reject(reason).BeforeToolResult(ctx), nil
	})
}

func canonicalSaveFileName(toolName string) string {
	n := strings.TrimSpace(toolName)
	if n == "save_file" || n == "write_file" || strings.HasSuffix(n, "_save_file") {
		return "save_file"
	}
	return n
}

func extractEditPath(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, key := range []string{"file_name", "path", "file"} {
		if s, ok := m[key].(string); ok {
			if v := strings.TrimSpace(s); v != "" {
				return v
			}
		}
	}
	return ""
}
