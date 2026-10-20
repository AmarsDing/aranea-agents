package twinops

import (
	"context"
	"strings"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// enrichExecResult 对 gns3_exec 成功结果做证据增强：ip link show 类命令附加
// ports 结构化数组并把故障端口点名进 hint；其余命令回退通用 state DOWN 指引。
func enrichExecResult(cmd string, result any) {
	m, ok := result.(map[string]any)
	if !ok {
		return
	}

	if okFlag, exists := m["ok"].(bool); exists && !okFlag {
		return
	}
	output, _ := m["output"].(string)
	if !isLinkShowCmd(cmd) || strings.TrimSpace(output) == "" {
		embedPortDownHint(result)
		return
	}
	ports := parseLinkShowPorts(output)
	if len(ports) == 0 {
		embedPortDownHint(result)
		return
	}
	m["ports"] = ports
	var adminDown, carrierDown []string
	for _, p := range ports {
		if p["state"] != "DOWN" {
			continue
		}
		name, _ := p["name"].(string)
		if nc, _ := p["no_carrier"].(bool); nc {
			carrierDown = append(carrierDown, name)
		} else {
			adminDown = append(adminDown, name)
		}
	}
	switch {
	case len(adminDown) > 0:
		hint := "实测管理性 DOWN 端口（故障端口，即修复目标）：" + strings.Join(adminDown, "、") + "。"
		if len(carrierDown) > 0 {
			hint += "端口 " + strings.Join(carrierDown, "、") + " 带 NO-CARRIER 属链路未接常态，禁止碰。"
		}
		hint += "端口判别已完成（见 ports 结构化数组），取证结束——禁止重发本命令" +
			"（重复调用将被系统拦截并白白消耗调用预算），立即按本节点任务指令推进到下一步动作。"
		m["next_action_hint"] = hint
	case len(carrierDown) > 0:
		m["next_action_hint"] = "DOWN 端口 " + strings.Join(carrierDown, "、") + " 均带 NO-CARRIER（链路未接常态），" +
			"未发现管理性 DOWN 端口；若任务预期存在故障端口，按本节点任务指令处理，禁止为「再确认」重发本命令。"
	}
}

func newGNS3FaultInjectTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in gns3PortInput) (jsonResult, error) {
		port, err := normalizePort(in.Port)
		if err != nil {
			return jsonResult{}, err
		}
		return cfg.gns3Post(ctx, "/fault/sw1-port", map[string]any{"port": port, "state": "down"})
	},
		trpcfunction.WithName("gns3_fault_inject"),
		trpcfunction.WithDescription("【高危·必须审批】向 SW1 指定端口注入故障（端口 down），用于演练故障场景。生产环境严禁调用。"),
	)
}
