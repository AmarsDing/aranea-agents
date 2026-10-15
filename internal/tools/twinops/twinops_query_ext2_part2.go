package twinops

import (
	"context"
	"fmt"
	"net/url"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func newAlarmStatsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in alarmStatsInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "startTime", in.StartTime)
		setQ(q, "endTime", in.EndTime)
		setQI(q, "deviceId", in.DeviceID)
		setQ(q, "alarmLevel", in.AlarmLevel)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/alarm/statistics", q)
	},
		trpcfunction.WithName("twin_alarm_stats"),
		trpcfunction.WithDescription("查询告警统计聚合（总数/活跃/已确认/已恢复/MTTR/MTTA/级别分布/TopN 设备）。回答「今天告警量多少」「哪个设备告警最多」「平均处理时长」类问题。"),
	)
}

func newProblemClustersTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in problemClustersInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "status", in.Status)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/alarm/problem-clusters", q)
	},
		trpcfunction.WithName("twin_problem_clusters"),
		trpcfunction.WithDescription("查询问题簇列表（同源告警自动聚合，一个故事一张卡：根因设备/波及设备/最高级别/持续时长）。回答「现在有哪些故障簇」「这个告警关联哪些设备」类问题。"),
	)
}

func newProblemClusterStatsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in emptyInput) (jsonResult, error) {
		return cfg.gatewayGet(ctx, "/api/v1/monitor/alarm/problem-clusters/stats", nil)
	},
		trpcfunction.WithName("twin_problem_cluster_stats"),
		trpcfunction.WithDescription("查询问题簇统计 KPI（活跃簇数/级联簇数/平均持续时长）。回答「当前有多少活跃故障」「级联故障占比」类问题。"),
	)
}

type assetIDInput struct {
	AssetID int `json:"asset_id" jsonschema:"description=资产 ID（twin_device_search 返回的 asset_id）,required"`
}

func newAssetFullTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in assetIDInput) (jsonResult, error) {
		if in.AssetID <= 0 {
			return jsonResult{}, fmt.Errorf("asset_id 必填且必须为正整数")
		}
		return cfg.gatewayGet(ctx, fmt.Sprintf("/api/v1/monitor/assets/%d/full", in.AssetID), nil)
	},
		trpcfunction.WithName("twin_asset_full"),
		trpcfunction.WithDescription("获取资产完整详情画像（基础信息 + 设备扩展 + 资产扩展 + 3D 模型 + 协议绑定 + 最近履历），一次调用返回全部维度。"),
	)
}

type videoChannelsInput struct {
	Keyword     string `json:"keyword,omitempty" jsonschema:"description=通道名称模糊搜索"`
	GroupID     int    `json:"group_id,omitempty" jsonschema:"description=按分组 ID 筛选"`
	EnabledOnly bool   `json:"enabled_only,omitempty" jsonschema:"description=仅查启用通道"`
	Tree        bool   `json:"tree,omitempty" jsonschema:"description=true 返回分组树结构，false 返回平铺列表"`
	Page        int    `json:"page,omitempty" jsonschema:"description=页码（列表模式）"`
	PageSize    int    `json:"page_size,omitempty" jsonschema:"description=每页条数（列表模式），默认 20"`
}

func newVideoChannelsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in videoChannelsInput) (jsonResult, error) {
		if in.Tree {
			q := url.Values{}
			if in.EnabledOnly {
				q.Set("enabledOnly", "true")
			}
			return cfg.gatewayGet(ctx, "/api/v1/monitor/video/channels/tree", q)
		}
		q := url.Values{}
		setQ(q, "keyword", in.Keyword)
		setQI(q, "groupId", in.GroupID)
		if in.EnabledOnly {
			q.Set("enabledOnly", "true")
		}
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/video/channels", q)
	},
		trpcfunction.WithName("twin_video_channels"),
		trpcfunction.WithDescription("查询视频监控通道列表或分组树（名称/状态/所属分组）。回答「有哪些摄像头」「XX 区域的摄像头状态」类问题。tree=true 返回分组树。"),
	)
}

type virtmanContainersInput struct {
	EndpointID int    `json:"endpoint_id,omitempty" jsonschema:"description=按端点（Docker 主机）ID 筛选"`
	Keyword    string `json:"keyword,omitempty" jsonschema:"description=容器名称模糊搜索"`
	Status     string `json:"status,omitempty" jsonschema:"description=状态过滤：running/exited/paused 等"`
	Page       int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize   int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

func newVirtmanContainersTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in virtmanContainersInput) (jsonResult, error) {
		q := url.Values{}
		setQI(q, "endpointId", in.EndpointID)
		setQ(q, "keyword", in.Keyword)
		setQ(q, "status", in.Status)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/virtman/containers", q)
	},
		trpcfunction.WithName("twin_virtman_containers"),
		trpcfunction.WithDescription("查询容器列表（名称/状态/镜像/所属端点）。回答「哪些容器在运行」「XX 容器状态怎么样」类问题。"),
	)
}

func newVirtmanEndpointsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in deviceSearchInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "keyword", in.Keyword)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/virtman/endpoints", q)
	},
		trpcfunction.WithName("twin_virtman_endpoints"),
		trpcfunction.WithDescription("查询虚拟化端点（Docker 主机）列表（名称/地址/状态/容器数）。回答「有哪些 Docker 主机」「端点连接状态」类问题。"),
	)
}

type aiopsTasksInput struct {
	Status      string `json:"status,omitempty" jsonschema:"description=任务状态：pending/running/waiting_approval/completed/failed/cancelled"`
	TriggerType string `json:"trigger_type,omitempty" jsonschema:"description=触发方式：manual/schedule/alert/remediation"`
	Keyword     string `json:"keyword,omitempty" jsonschema:"description=关键词模糊搜索"`
	Page        int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize    int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

type mcpCallHistoryInput struct {
	ToolName string `json:"tool_name,omitempty" jsonschema:"description=按工具名过滤"`
	ClientID string `json:"client_id,omitempty" jsonschema:"description=按调用方 ID 过滤"`
	Status   string `json:"status,omitempty" jsonschema:"description=状态过滤：success/failed"`
	Page     int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

func newAiopsTasksTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in aiopsTasksInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "status", in.Status)
		setQ(q, "triggerType", in.TriggerType)
		setQ(q, "keyword", in.Keyword)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/aiops/tasks", q)
	},
		trpcfunction.WithName("twin_aiops_tasks"),
		trpcfunction.WithDescription("查询 AI 运维任务列表（状态/触发方式/执行进度）。回答「有没有正在执行的运维任务」「最近 AI 执行了什么操作」类问题。"),
	)
}

func newAiopsMcpOverviewTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in emptyInput) (jsonResult, error) {
		return cfg.gatewayGet(ctx, "/api/v1/monitor/aiops/mcp/overview", nil)
	},
		trpcfunction.WithName("twin_aiops_mcp_overview"),
		trpcfunction.WithDescription("查询 MCP 工具平台概览（注册工具数/调用方数/今日调用量/风险分布）。回答「平台有哪些自动化能力」「MCP 调用情况」类问题。"),
	)
}

func newAiopsMcpCallHistoryTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in mcpCallHistoryInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "toolName", in.ToolName)
		setQ(q, "clientId", in.ClientID)
		setQ(q, "status", in.Status)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/aiops/mcp/call-history", q)
	},
		trpcfunction.WithName("twin_aiops_mcp_call_history"),
		trpcfunction.WithDescription("查询 MCP 工具调用历史（工具名/调用方/状态/耗时/结果摘要）。回答「最近执行了哪些自动化操作」「XX 操作执行结果」类问题。"),
	)
}

type collectorTasksInput struct {
	Keyword  string `json:"keyword,omitempty" jsonschema:"description=任务名称模糊搜索"`
	Page     int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

type collectorQualityInput struct {
	DeviceID     int    `json:"device_id,omitempty" jsonschema:"description=按设备 ID 过滤"`
	ProtocolType string `json:"protocol_type,omitempty" jsonschema:"description=按协议类型过滤"`
	StartTime    int64  `json:"start_time,omitempty" jsonschema:"description=Unix 时间戳（秒），默认 24 小时前"`
	EndTime      int64  `json:"end_time,omitempty" jsonschema:"description=Unix 时间戳（秒），默认当前"`
	Page         int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize     int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

type collectorFailuresInput struct {
	DeviceID  int    `json:"device_id,omitempty" jsonschema:"description=按设备 ID 过滤"`
	TaskID    int    `json:"task_id,omitempty" jsonschema:"description=按采集任务 ID 过滤"`
	ErrorType string `json:"error_type,omitempty" jsonschema:"description=按错误类型过滤"`
	Resolved  *bool  `json:"resolved,omitempty" jsonschema:"description=true=仅已恢复，false=仅未恢复，不传=全部"`
	Page      int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize  int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

func newCollectorTasksTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in collectorTasksInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "keyword", in.Keyword)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/collector/tasks", q)
	},
		trpcfunction.WithName("twin_collector_tasks"),
		trpcfunction.WithDescription("查询采集任务列表（任务名/协议/设备数/调度方式/状态）。回答「有哪些采集任务」「XX 协议采集情况」类问题。"),
	)
}

func newCollectorQualityTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in collectorQualityInput) (jsonResult, error) {
		q := url.Values{}
		setQI(q, "deviceId", in.DeviceID)
		setQ(q, "protocolType", in.ProtocolType)
		if in.StartTime > 0 {
			q.Set("startTime", fmt.Sprintf("%d", in.StartTime))
		}
		if in.EndTime > 0 {
			q.Set("endTime", fmt.Sprintf("%d", in.EndTime))
		}
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/collector/quality", q)
	},
		trpcfunction.WithName("twin_collector_quality"),
		trpcfunction.WithDescription("查询采集质量统计（成功率/失败率/平均耗时，按设备/协议维度）。回答「采集成功率高吗」「哪些设备采集质量差」类问题。"),
	)
}

func newCollectorFailuresTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in collectorFailuresInput) (jsonResult, error) {
		q := url.Values{}
		setQI(q, "deviceId", in.DeviceID)
		setQI(q, "taskId", in.TaskID)
		setQ(q, "errorType", in.ErrorType)
		if in.Resolved != nil {
			if *in.Resolved {
				q.Set("resolved", "true")
			} else {
				q.Set("resolved", "false")
			}
		}
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/collector/failures", q)
	},
		trpcfunction.WithName("twin_collector_failures"),
		trpcfunction.WithDescription("查询采集失败记录（设备/任务/错误类型/恢复状态）。回答「哪些设备采集失败了」「采集失败原因」类问题。"),
	)
}

type ticketsInput struct {
	Keyword  string `json:"keyword,omitempty" jsonschema:"description=按工单编号/标题模糊搜索"`
	Status   string `json:"status,omitempty" jsonschema:"description=状态过滤：open/processing/resolved/closed"`
	Page     int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

func newTicketsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in ticketsInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "keyword", in.Keyword)
		setQ(q, "status", in.Status)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/tickets", q)
	},
		trpcfunction.WithName("twin_tickets"),
		trpcfunction.WithDescription("查询运维工单列表（编号/标题/状态/优先级/处理人）。回答「有没有待处理的工单」「XX 工单进展」类问题。"),
	)
}

func newChangeRecordsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in ticketsInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "keyword", in.Keyword)
		setQ(q, "status", in.Status)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/change-records", q)
	},
		trpcfunction.WithName("twin_change_records"),
		trpcfunction.WithDescription("查询变更记录列表（变更标题/类型/状态/实施人/时间）。回答「最近有什么变更」「XX 变更执行了吗」类问题。"),
	)
}

func newIncidentRecordsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in ticketsInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "keyword", in.Keyword)
		setQ(q, "status", in.Status)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/incident-records", q)
	},
		trpcfunction.WithName("twin_incident_records"),
		trpcfunction.WithDescription("查询事件记录列表（事件标题/级别/状态/影响范围）。回答「最近发生了什么事件」「XX 事件处理进展」类问题。"),
	)
}

type healthScoresInput struct {
	StartDate string  `json:"start_date,omitempty" jsonschema:"description=开始日期 YYYY-MM-DD，默认 7 天前"`
	EndDate   string  `json:"end_date,omitempty" jsonschema:"description=结束日期 YYYY-MM-DD，默认昨天"`
	Limit     int     `json:"limit,omitempty" jsonschema:"description=返回条数，默认 50"`
	MinScore  float64 `json:"min_score,omitempty" jsonschema:"description=最低分过滤（0-100），0=不过滤"`
}

type insightsInput struct {
	Domains   string `json:"domains,omitempty" jsonschema:"description=域过滤，逗号分隔：arrival/device/container/line/alarm/comprehensive，空=全域"`
	StartTime string `json:"start_time,omitempty" jsonschema:"description=窗口起点 RFC3339，默认近 7 天"`
	EndTime   string `json:"end_time,omitempty" jsonschema:"description=窗口终点 RFC3339"`
	Levels    string `json:"levels,omitempty" jsonschema:"description=级别过滤，逗号分隔：info/warning/critical，空=全级别"`
	Refresh   bool   `json:"refresh,omitempty" jsonschema:"description=true 绕过缓存重算"`
}

func newHealthScoresTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in healthScoresInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "startDate", in.StartDate)
		setQ(q, "endDate", in.EndDate)
		setQI(q, "limit", in.Limit)
		if in.MinScore > 0 {
			q.Set("minScore", fmt.Sprintf("%.1f", in.MinScore))
		}
		return cfg.gatewayGet(ctx, "/api/v1/monitor/analysis/statistics/health-scores", q)
	},
		trpcfunction.WithName("twin_analysis_health_scores"),
		trpcfunction.WithDescription("查询设备健康分排行（0-100 分，升序=最不省心在前，含等级 healthy/attention/risk/critical）。回答「哪些设备健康状况差」「设备健康排名」类问题。"),
	)
}

func newInsightsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in insightsInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "domains", in.Domains)
		setQ(q, "start", in.StartTime)
		setQ(q, "end", in.EndTime)
		setQ(q, "levels", in.Levels)
		if in.Refresh {
			q.Set("refresh", "true")
		}
		return cfg.gatewayGet(ctx, "/api/v1/monitor/analysis/insights", q)
	},
		trpcfunction.WithName("twin_analysis_insights"),
		trpcfunction.WithDescription("查询规则洞察（跨域异常检测结论：数据到达/设备/容器/线路/告警等维度的自动分析发现）。回答「平台有什么异常发现」「运维洞察」类问题。"),
	)
}
