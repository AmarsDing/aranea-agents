package twinops

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func newGNS3FaultClearTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in gns3PortInput) (jsonResult, error) {
		port, err := normalizePort(in.Port)
		if err != nil {
			return jsonResult{}, err
		}
		return cfg.gns3Post(ctx, "/fault/sw1-port", map[string]any{"port": port, "state": "up"})
	},
		trpcfunction.WithName("gns3_fault_clear"),
		trpcfunction.WithDescription("【高危·必须审批】恢复 SW1 指定端口（端口 up），清除已注入的故障。"),
	)
}

func normalizePort(p string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(p))
	switch norm {
	case "eth0", "eth1", "eth2", "eth3":
		return norm, nil
	}
	return "", fmt.Errorf("port 仅支持 eth0/eth1/eth2/eth3（SW1 演练端口），收到 %q", p)
}

// configToolTimeout 配置自动化三工具专用超时（备份抓取 + 逐行下发 + 再备份）。
const configToolTimeout = 300 * time.Second

// mcpCall 调 13-aiops MCP 工具执行端点（经 gateway 代签）。
func (c Config) mcpCall(ctx context.Context, toolName string, params map[string]any) (jsonResult, error) {
	return c.gatewayPost(ctx, "/api/v1/monitor/aiops/mcp/call", map[string]any{
		"toolName":   toolName,
		"params":     params,
		"callerType": "agent",
	})
}

// configToolConfig 返回放宽超时的 Config 副本（仅配置自动化三工具使用）。
func configToolConfig(cfg Config) Config {
	cfg.Timeout = configToolTimeout
	return cfg
}

type configDiffInput struct {
	AssetID    int `json:"asset_id" jsonschema:"required,description=目标设备/资产 ID（twin_device_search 返回的 device_id）"`
	BackupID   int `json:"backup_id,omitempty" jsonschema:"description=模式A：基准备份版本 ID（与 against_id 比对；against_id 空=同设备上一版本）"`
	AgainstID  int `json:"against_id,omitempty" jsonschema:"description=模式A：比对目标备份版本 ID（空=同设备上一版本）"`
	TemplateID int `json:"template_id,omitempty" jsonschema:"description=模式B：配置模板 ID（即时抓取设备当前配置与模板渲染结果比对）"`
}

func newConfigDiffTool(cfg Config) trpctool.CallableTool {
	cfg = configToolConfig(cfg)
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in configDiffInput) (jsonResult, error) {
		if in.AssetID <= 0 {
			return jsonResult{}, fmt.Errorf("asset_id 必填")
		}
		if in.BackupID <= 0 && in.TemplateID <= 0 {
			return jsonResult{}, fmt.Errorf("backup_id（备份版本比对）与 template_id（当前配置 vs 模板）至少其一")
		}
		params := map[string]any{"asset_id": in.AssetID}
		if in.BackupID > 0 {
			params["backup_id"] = in.BackupID
			if in.AgainstID > 0 {
				params["against_id"] = in.AgainstID
			}
		} else {
			params["template_id"] = in.TemplateID
		}
		return cfg.mcpCall(ctx, "network.config_diff", params)
	},
		trpcfunction.WithName("twin_config_diff"),
		trpcfunction.WithDescription("配置比对（只读）：模式A 两备份版本比对（传 backup_id）；模式B 设备当前配置与模板渲染结果比对（传 template_id，即时触发一次备份抓取留痕）。返回 unified diff 与增删行数。"),
	)
}

type configPushInput struct {
	AssetID        int      `json:"asset_id" jsonschema:"required,description=目标设备/资产 ID"`
	Commands       []string `json:"commands" jsonschema:"required,description=配置命令列表（逐行下发，含进入/退出配置模式命令，如 conf t ... end）"`
	VerifyCommands []string `json:"verify_commands,omitempty" jsonschema:"description=可选下发后验证命令（show 类只读命令，逐条独立会话执行）"`
	BackupFirst    *bool    `json:"backup_first,omitempty" jsonschema:"description=是否下发前自动备份（默认 true；关闭则设备故障时无回滚基点，需审批人权衡）"`
}

func newConfigPushTool(cfg Config) trpctool.CallableTool {
	cfg = configToolConfig(cfg)
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in configPushInput) (jsonResult, error) {
		if in.AssetID <= 0 {
			return jsonResult{}, fmt.Errorf("asset_id 必填")
		}
		if len(in.Commands) == 0 {
			return jsonResult{}, fmt.Errorf("commands 必填且非空")
		}
		params := map[string]any{"asset_id": in.AssetID, "commands": in.Commands}
		if len(in.VerifyCommands) > 0 {
			params["verify_commands"] = in.VerifyCommands
		}
		if in.BackupFirst != nil {
			params["backup_first"] = *in.BackupFirst
		}
		return cfg.mcpCall(ctx, "network.config_push", params)
	},
		trpcfunction.WithName("twin_config_push"),
		trpcfunction.WithDescription("【高危·必须审批】配置下发：前置备份 → 单次 SSH 会话逐行下发 → 可选验证命令 → 再备份 → 前后版本 diff 取证。任一环失败返回 stage 标识与 rollback_hint。响应 pending=true 表示已转人工审批，批准后异步执行。"),
	)
}

type configRollbackInput struct {
	AssetID  int `json:"asset_id" jsonschema:"required,description=目标设备/资产 ID"`
	BackupID int `json:"backup_id" jsonschema:"required,description=回滚目标备份版本 ID（须属于本设备；twin_config_diff 模式A 可发现历史版本）"`
}

func newConfigRollbackTool(cfg Config) trpctool.CallableTool {
	cfg = configToolConfig(cfg)
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in configRollbackInput) (jsonResult, error) {
		if in.AssetID <= 0 {
			return jsonResult{}, fmt.Errorf("asset_id 必填")
		}
		if in.BackupID <= 0 {
			return jsonResult{}, fmt.Errorf("backup_id 必填")
		}
		return cfg.mcpCall(ctx, "network.config_rollback", map[string]any{
			"asset_id":  in.AssetID,
			"backup_id": in.BackupID,
		})
	},
		trpcfunction.WithName("twin_config_rollback"),
		trpcfunction.WithDescription("【高危·必须审批】配置回滚：回滚前即时备份留痕 → 目标版本原文逐行下发 → 再备份 → sha256 校验回滚到位。一期面向仿真/测试设备；生产整份回滚仍应走人工变更窗口。响应 pending=true 表示已转人工审批。"),
	)
}

// NewToolset returns all 59 twinops tools.
func NewToolset(cfg Config) []trpctool.Tool {
	registerCompensationPairs()
	return []trpctool.Tool{
		newAlarmQueryTool(cfg),
		newAlarmGetTool(cfg),
		newAlarmAckTool(cfg),
		newLineStatusTool(cfg),
		newLineEventsTool(cfg),
		newDeviceGetTool(cfg),
		newDeviceMetricsTool(cfg),
		newRemediationStatusTool(cfg),
		newGNS3HealthTool(cfg),
		newGNS3ExecTool(cfg),
		newGNS3FaultInjectTool(cfg),
		newGNS3FaultClearTool(cfg),
		newDeviceSearchTool(cfg),
		newAlarmRuleTool(cfg),
		newCollectorStatusTool(cfg),
		newLineProbeTool(cfg),
		newInspectionQueryTool(cfg),
		newConfigDiffTool(cfg),
		newConfigPushTool(cfg),
		newConfigRollbackTool(cfg),

		newArrivalOverviewTool(cfg),
		newArrivalStatusTool(cfg),
		newNoticeRecordsTool(cfg),
		newReportTasksTool(cfg),
		newKBSearchTool(cfg),

		newFlowSummaryTool(cfg),
		newFlowTopTool(cfg),
		newFlowTrendTool(cfg),
		newFlowAnomaliesTool(cfg),
		newFlowRecordsTool(cfg),

		newMetarReportsTool(cfg),
		newTafReportsTool(cfg),
		newMetarStationsTool(cfg),

		newRadarSitesTool(cfg),
		newRadarScansTool(cfg),

		newSatelliteFilesTool(cfg),
		newSatelliteStatsTool(cfg),

		newWindfieldFilesTool(cfg),

		newFlightPositionsTool(cfg),
		newFlightTrackTool(cfg),
		newFlightStatsTool(cfg),

		newAlarmStatsTool(cfg),
		newProblemClustersTool(cfg),
		newProblemClusterStatsTool(cfg),

		newAssetFullTool(cfg),

		newVideoChannelsTool(cfg),

		newVirtmanContainersTool(cfg),
		newVirtmanEndpointsTool(cfg),

		newAiopsTasksTool(cfg),
		newAiopsMcpOverviewTool(cfg),
		newAiopsMcpCallHistoryTool(cfg),

		newCollectorTasksTool(cfg),
		newCollectorQualityTool(cfg),
		newCollectorFailuresTool(cfg),

		newTicketsTool(cfg),
		newChangeRecordsTool(cfg),
		newIncidentRecordsTool(cfg),

		newHealthScoresTool(cfg),
		newInsightsTool(cfg),
	}
}

// EnabledTools returns only the tools whose name (= platform tool key) is
// enabled in the agent's effective key set — 白名单最小授权。
func EnabledTools(eff map[string]bool, cfg Config) []trpctool.Tool {
	if len(eff) == 0 {
		return nil
	}
	var out []trpctool.Tool
	for _, t := range NewToolset(cfg) {
		if d := t.Declaration(); d != nil && eff[d.Name] {
			out = append(out, t)
		}
	}
	return out
}

// EnabledKeys 返回 effective key 集合中启用的 twinops 工具名（排序，确定性）。
// 供 P0-2 阶段A 分片指纹使用：无需为计算指纹构造带真实配置的工具实例。
func EnabledKeys(eff map[string]bool) []string {
	if len(eff) == 0 {
		return nil
	}
	var out []string
	for _, t := range NewToolset(Config{}) {
		if d := t.Declaration(); d != nil && eff[d.Name] {
			out = append(out, d.Name)
		}
	}
	sort.Strings(out)
	return out
}
