package agent

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/officecli"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// resolveOfficeCLITools 装配 OfficeCLI 办公文档工具集（officecli_read/write/render）：
// 按 effective keys 白名单逐个挂载；文件参数强制围栏到 Agent 工作区根目录
// （与 filesystem/shell_exec 共用 {base}/workspace/{wsID}/{agentKey} 布局）。
// 工作区解析失败时 fail-closed 跳过挂载（无围栏根目录不放行文件操作）。
func resolveOfficeCLITools(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, eff map[string]bool) []trpctool.Tool {
	if !officecli.AnyEnabled(eff) {
		return nil
	}
	root, err := resolveAgentFilesystemDir(ctx, ag, deps, "")
	if err != nil {
		deps.Logger().Warn("officecli 工作区解析失败，跳过 Office 工具挂载",
			loggateway.StepID("agent.tool_build"),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Err(err))
		return nil
	}
	return officecli.EnabledTools(eff, officecli.ConfigFromEnv(), root, deps.ArtifactWriter)
}
