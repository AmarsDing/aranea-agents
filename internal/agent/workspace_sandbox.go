package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const workspaceSandboxHookPriority = 8 // before tool confirmation (10)

// newWorkspaceSandboxBeforeHook enforces the session permission posture and
// workspace path policy before a tool runs. Read-only agents cannot write;
// any path/file/cwd argument must stay under the agent workspace root.
// OS-level sandbox (Windows restricted token / bwrap) is out of scope.
func newWorkspaceSandboxBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.BeforeToolHook {
	lg := deps.Logger()
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return callbacks.NewBeforeToolHook(workspaceSandboxHookPriority, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil || strings.TrimSpace(args.ToolName) == "" {
			return callbacks.Pass().BeforeToolResult(ctx), nil
		}
		name := strings.TrimSpace(args.ToolName)
		mode := InferPermissionMode(ag)
		if mode == PermissionReadOnly && writeToolSignals[name] {
			reason := fmt.Sprintf("read-only mode: tool %q cannot create, edit, or delete files, run shell commands, or operate the desktop. Switch this agent's tool profile if the user needs a write.", name)
			lg.Info("workspace sandbox blocked write in read-only mode",
				loggateway.StepID("agent.workspace_sandbox"),
				loggateway.Str("tool", name),
				loggateway.Str("mode", string(mode)))
			return callbacks.Reject(reason).BeforeToolResult(ctx), nil
		}

		candidate := extractSandboxPath(args.Arguments)
		if candidate == "" {
			return callbacks.Pass().BeforeToolResult(ctx), nil
		}
		root := sandboxWorkspaceRoot(ctx, ag, deps)
		resolved := candidate
		if !filepath.IsAbs(candidate) {
			resolved = filepath.Join(root, candidate)
		}
		if _, err := containPathUnderRoot(resolved, root); err != nil {
			reason := fmt.Sprintf("workspace sandbox: path %q is outside workspace root %q. You may only read or edit files under the workspace.", candidate, root)
			lg.Info("workspace sandbox blocked path escape",
				loggateway.StepID("agent.workspace_sandbox"),
				loggateway.Str("tool", name),
				loggateway.Str("path", candidate),
				loggateway.Err(err))
			return callbacks.Reject(reason).BeforeToolResult(ctx), nil
		}
		return callbacks.Pass().BeforeToolResult(ctx), nil
	})
}

func sandboxWorkspaceRoot(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) string {
	base := toolWorkspaceBase(ctx, deps)
	wsID := workspace.IDFromContext(ctx)
	if strings.TrimSpace(wsID) == "" {
		wsID = workspace.DefaultWorkspaceID
	}
	root := filepath.Join(base, "workspace", wsID)
	if k := strings.TrimSpace(ag.AgentKey); k != "" {
		root = filepath.Join(root, k)
	}
	return root
}

func extractSandboxPath(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, key := range []string{"path", "file", "cwd"} {
		if s, ok := m[key].(string); ok {
			if v := strings.TrimSpace(s); v != "" {
				return v
			}
		}
	}
	return ""
}
