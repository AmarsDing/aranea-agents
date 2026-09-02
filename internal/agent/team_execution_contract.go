package agent

import (
	_ "embed"
	"strings"

	"aranea-agents/internal/biz"
)

//go:embed prompts/team_execution_contract.md
var teamExecutionContractBody string

// ShouldAttachTeamExecutionContract reports whether the team/spirit
// orchestration execution contract belongs on this agent's system prompt
// (LBG-1).
//
// 挂载点精确到 spirit 编排面：team 成员与单聊共用同一构建缓存
// （internal/team/trpc_build.go 复用 BuildTRPCAgent/BuildSystemPrompt），
// 凭 biz.Agent 属性无法区分"本次构建用于 team run 还是单聊"；spirit 是
// team run 的无人值守编排面，且被 ShouldAttachWorkingContract 显式排除，
// 是契约的精确落点。
//
// 与 ShouldAttachWorkingContract 互斥：spirit 显式挂 coding/computer-use
// allow 扩展时归 working_contract 覆盖，避免双契约重复（六类冗余之
// "重复指令"）。
func ShouldAttachTeamExecutionContract(ag biz.Agent) bool {
	if strings.TrimSpace(ag.AgentKey) != biz.SpiritAgentKey {
		return false
	}
	return !ShouldAttachWorkingContract(ag)
}

// TeamExecutionContractBlock returns the tagged team execution contract
// prompt, or empty when the agent should not receive it.
func TeamExecutionContractBlock(ag biz.Agent) string {
	if !ShouldAttachTeamExecutionContract(ag) {
		return ""
	}
	body := strings.TrimSpace(teamExecutionContractBody)
	if body == "" {
		return ""
	}
	return "<team_execution_contract>\n" + body + "\n</team_execution_contract>"
}
