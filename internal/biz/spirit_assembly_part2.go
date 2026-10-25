package biz

import (
	"aranea-agents/pkg/loggateway"
	"encoding/json"
	"strings"
)

func buildSpiritTeamDefinitionJSON(mode string, agentKeys []string, lg loggateway.Logger, requireDeliverable bool, deliverables []DeliverableContract, gates []VerificationGate, parallelCfgJSON ...string) string {
	type member struct {
		AgentKey   string `json:"agent_id"`
		Role       string `json:"role"`
		Enabled    *bool  `json:"enabled"`
		TaskPrompt string `json:"task_prompt,omitempty"`
	}
	members := make([]member, 0, len(agentKeys))

	deliverableChannel := len(agentKeys) > 1 || requireDeliverable
	roles := make([]string, len(agentKeys))
	for i := range agentKeys {
		role := RoleWorker
		// D8-6 防御纵深：合成者前提=存在其他成员产出——coordinator 分支与
		// parallel 末成员规则同源，任何不经 AssembleTeam 归一化（或归一化
		// 被未来调用方绕过）的调用方在此也不会给单成员授 synthesizer。
		if mode == TeamModeCoordinator && i == 0 && len(agentKeys) > 1 {
			role = RoleSynthesizer
		}
		if mode == TeamModeParallel && i == len(agentKeys)-1 && len(agentKeys) > 1 {
			role = RoleSynthesizer
		}
		roles[i] = role
	}

	roster := buildMemberRosterPrompt(agentKeys, roles)
	for i, key := range agentKeys {
		enabled := true
		members = append(members, member{
			AgentKey:   strings.TrimSpace(key),
			Role:       roles[i],
			Enabled:    &enabled,
			TaskPrompt: memberRoleTaskPrompt(roles[i], deliverableChannel, roster),
		})
	}
	maxConcurrency := SpiritTeamDefaultMaxConc
	if len(parallelCfgJSON) > 0 && parallelCfgJSON[0] != "" {
		cfg := ParseParallelConfig(parallelCfgJSON[0], lg)
		if cfg.MaxTeamConcurrency > 0 {
			maxConcurrency = cfg.MaxTeamConcurrency
		}
	}
	def := map[string]any{
		"version":            SpiritTeamDefVersion,
		"mode":               mode,
		"runtime_engine":     RuntimeEngineGraph,
		"team_graph_runtime": true,
		"members":            members,
		"max_concurrency":    maxConcurrency,

		"timeout_seconds": 0,
	}

	def["failure_policy"] = map[string]any{
		"default":       FailurePolicyRetryThenBlock,
		"retry":         map[string]any{"max_attempts": 2},
		"parallel_fail": FailurePolicyContinue,
	}
	if mode == TeamModeParallel && len(agentKeys) > 1 {
		def["synthesizer_agent_id"] = strings.TrimSpace(agentKeys[len(agentKeys)-1])
	}

	if len(agentKeys) > 1 || requireDeliverable {
		def["enable_state_deliverable"] = true
	}

	if entries := MemberEntriesFromDeliverableContracts(deliverables); len(entries) > 0 {
		def["deliverable_contract"] = MemberDeliverableContract{Entries: entries}
	}

	if len(gates) > 0 {
		def["verification_gates"] = gates
	}
	out, err := json.Marshal(def)
	if err != nil {
		return "{}"
	}
	return string(out)
}

// memberRoleTaskPrompt 按团队角色生成差异化任务书（2026-09-01 修复：此前
// 成员定义不写 task_prompt，图编译节点 instruction 为空，synthesizer 与
// worker 收到完全相同的输入，分工仅靠各自 system prompt，存在产出重叠与
// 越界风险——如调研角色产出远超设计角色）。任务书经 definition JSON →
// 图编译节点 instruction（embedded_graph.go）→ 注入成员用户消息头部
// （graph/trpc/agent_instruction.go），不触碰 system prompt（保前缀缓存）。
func memberRoleTaskPrompt(role string, deliverableChannel bool, roster string) string {
	deliver := "产出通过 set_deliverable 提交。"
	if !deliverableChannel {
		deliver = "产出直接作为你的最终回复。"
	}
	switch role {
	case RoleSynthesizer:
		p := "你是本团队的合成者：基于其他成员的产出做整合、去重与冲突裁决，形成团队最终交付物；" + deliver +
			"不要重复执行成员已完成的调研/执行工作，输入中已有成员产出时在其基础上综合而非重做。"
		if roster != "" {
			p += roster +
				"整合前先对照名册核查：名册成员的产出未出现在输入中（或收到其失败通告）即该成员执行失败已被跳过——" +
				"在交付物中如实标注缺失部分及其影响，严禁编造其内容。"
		}
		return p
	case RoleWorker:
		return "你是本团队的执行者：只完成与你专业职责直接相关的子任务，聚焦你的专业领域产出具体成果；" + deliver +
			"团队最终汇总由合成者负责，不要代劳；不要越界承担其他成员的职责。"
	default:
		return ""
	}
}

// buildMemberRosterPrompt 生成团队名册文本（仅合成者使用）。名册身份用
// agent key——与图节点 ID / 消息作者同口径，合成者可直接对照输入消息的作者。
func buildMemberRosterPrompt(agentKeys []string, roles []string) string {
	if len(agentKeys) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("团队名册：")
	for i, key := range agentKeys {
		label := "执行者"
		if i < len(roles) && roles[i] == RoleSynthesizer {
			label = "合成者"
		}
		if i > 0 {
			b.WriteString("、")
		}
		b.WriteString(strings.TrimSpace(key))
		b.WriteString("（")
		b.WriteString(label)
		b.WriteString("）")
	}
	b.WriteString("。")
	return b.String()
}
