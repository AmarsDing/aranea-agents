package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/deferred"
	"aranea-agents/internal/tools/officecli"
	"aranea-agents/pkg/loggateway"
	"encoding/json"
	"strings"
)

// officecliEnabledKeys 返回 eff 中启用的 officecli 工具键（固定顺序，确定性）。
func officecliEnabledKeys(eff map[string]bool) []string {
	var out []string
	for _, k := range []string{officecli.ToolRead, officecli.ToolWrite, officecli.ToolRender} {
		if eff[k] {
			out = append(out, k)
		}
	}
	return out
}

// resolveDeferredToolNames 计算延迟工具清单（registry 名），与原
// buildToolsetsForAgent 的两段式分离逻辑一致：手动 ToolsDeferredJSON 优先，
// 否则按 profile 自动分离核心常驻/延迟加载集。结果为合并期
// FinalizeDeferredTools 的输入（分片构建自身不携带延迟处理）。
func resolveDeferredToolNames(ag biz.Agent, eff map[string]bool, lg loggateway.Logger) []string {
	if ag.Settings == nil {
		return nil
	}
	if ag.Settings.ToolsDeferredJSON != "" {
		// 手动配置优先：ToolsDeferredJSON 显式指定延迟工具列表（biz key 粒度），
		// 需转换为 registry 名称供装配层匹配 ToolSet/工具。
		var deferredKeys []string
		if err := json.Unmarshal([]byte(ag.Settings.ToolsDeferredJSON), &deferredKeys); err == nil {
			return deferred.RegistryNamesForBizKeys(deferredKeys)
		}
		return nil
	}

	profile := biz.CanonicalToolProfile(ag.Settings.ToolsProfile)
	effKeys := effKeysList(eff)
	_, deferredKeys := deferred.SplitCoreResidentTools(effKeys, profile)
	deferredKeys = deferred.MergeNonCoreMappedDeferred(deferredKeys, profile)
	out := deferred.RegistryNamesForBizKeys(deferredKeys)
	if len(out) > 0 {
		lg.Info("两段式工具加载：自动分离核心/延迟",
			loggateway.StepID("agent.tool_build"),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Str("profile", profile),
			loggateway.Int("total_tools", len(effKeys)),
			loggateway.Int("deferred_tools", len(out)))
	}
	return out
}

func toolsAllowHasMCPServer(settings *biz.AgentRuntimeSettings) bool {
	if settings == nil || strings.TrimSpace(settings.ToolsAllowJSON) == "" {
		return false
	}
	var keys []string
	if json.Unmarshal([]byte(settings.ToolsAllowJSON), &keys) != nil {
		return false
	}
	for _, k := range keys {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(k)), "mcp:") {
			return true
		}
	}
	return false
}
