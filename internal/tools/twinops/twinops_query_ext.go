// twinops_query_ext.go — twin_butler 只读问答工具面扩展（2026-09-02）。
//
// 覆盖 twinops.go 既有诊断工具之外的「平台态势问答」高频入口：
//   - 数据到达监控（monitoralarm DataArrivalService，五源统一口径）
//   - 通知发送记录（monitornotice NoticeRecordService）
//   - 运维报表任务（report MonitorReportService）
//   - 业务知识库检索（admin KnowledgeBaseService，RCA/经验沉淀）
//
// 全部为只读 GET/POST 查询，不改变 TwinMonitor 任何状态；
// 失败处理与 twinops.go 一致：上游错误一律返回结构化 ok=false，不抛 Go error。

package twinops

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// ---------- 输入结构 ----------

// emptyInput 无参数工具输入占位（GenerateJSONSchema 需要具体类型，禁用 any）。
type emptyInput struct{}

type arrivalStatusInput struct {
	SourceType string `json:"source_type,omitempty" jsonschema:"description=源类型过滤：metar/radar/satellite/windfield/flight，空=全部"`
}

type noticeRecordsInput struct {
	AlarmID    string `json:"alarm_id,omitempty" jsonschema:"description=按关联告警 ID 过滤，如 ALM-20260814-xxxxxx"`
	Status     string `json:"status,omitempty" jsonschema:"description=发送状态过滤：pending/sent/failed"`
	Channel    string `json:"channel,omitempty" jsonschema:"description=通知渠道过滤，如 sms/email/webhook"`
	NoticeType string `json:"notice_type,omitempty" jsonschema:"description=通知类型过滤：alarm/task/approval/repair/test"`
	StartTime  string `json:"start_time,omitempty" jsonschema:"description=时间窗起点 RFC3339，如 2026-09-02T00:00:00+08:00"`
	EndTime    string `json:"end_time,omitempty" jsonschema:"description=时间窗终点 RFC3339"`
	Page       int    `json:"page,omitempty" jsonschema:"description=页码，默认 1"`
	PageSize   int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20，最大 100"`
}

type reportTasksInput struct {
	Status   string `json:"status,omitempty" jsonschema:"description=任务状态过滤，如 pending/running/success/failed"`
	Keyword  string `json:"keyword,omitempty" jsonschema:"description=按任务编号/报表名称模糊搜索"`
	Page     int    `json:"page,omitempty" jsonschema:"description=页码，默认 1"`
	PageSize int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

type kbSearchInput struct {
	Query    string `json:"query" jsonschema:"description=检索关键词/短语（故障现象/设备类型/处置方法）,required"`
	TopK     int    `json:"top_k,omitempty" jsonschema:"description=返回条数，默认 5，最大 20"`
	Category string `json:"category,omitempty" jsonschema:"description=限定知识分类"`
}

// ---------- 工具构造函数 ----------

// newArrivalOverviewTool 数据到达监控汇总卡：五源（metar/radar/satellite/windfield/flight）
// 流总数/正常/迟到/缺失/任务异常/活跃告警/今日准时率，一屏总览。
func newArrivalOverviewTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in emptyInput) (jsonResult, error) {
		return cfg.gatewayGet(ctx, "/api/v1/monitor/alarm/data-arrival/overview", nil)
	},
		trpcfunction.WithName("twin_arrival_overview"),
		trpcfunction.WithDescription("数据到达监控汇总卡：五源（metar/radar/satellite/windfield/flight）监控流总数、正常/迟到/缺失数、任务异常数、活跃告警数、今日准时率。回答「数据到齐了吗」「今天数据正常吗」类问题的首选入口。"),
	)
}

// newArrivalStatusTool 流实时状态表：每条监控流的最近数据时刻/下次期望/延迟秒数/
// 连续缺失数/当前状态（ON_TIME/LATE/MISSING/PENDING）/是否有活跃告警。
func newArrivalStatusTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in arrivalStatusInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "sourceType", in.SourceType)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/alarm/data-arrival/status", q)
	},
		trpcfunction.WithName("twin_arrival_status"),
		trpcfunction.WithDescription("查询数据到达监控流实时状态列表（最近数据时刻/延迟/连缺/当前状态 ON_TIME/LATE/MISSING）。可按源类型（metar/radar/satellite/windfield/flight）过滤，用于定位具体哪条数据流迟到或缺失。"),
	)
}

// newNoticeRecordsTool 通知发送记录：回答「告警有没有通知到人」「发给谁了」
// 「为什么没收到通知」（status=failed 看 error 字段）类问题。
func newNoticeRecordsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in noticeRecordsInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "alarmId", in.AlarmID)
		setQ(q, "status", in.Status)
		setQ(q, "channel", in.Channel)
		setQ(q, "noticeType", in.NoticeType)
		setQ(q, "startTime", in.StartTime)
		setQ(q, "endTime", in.EndTime)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/notice/records", q)
	},
		trpcfunction.WithName("twin_notice_records"),
		trpcfunction.WithDescription("查询告警通知发送记录（渠道/接收人/状态 pending/sent/failed/错误信息）。用于核实「告警是否已通知到人」「通知为何发送失败」。"),
	)
}

// newReportTasksTool 运维报表任务查询：生成状态/产物格式/下载信息。
func newReportTasksTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in reportTasksInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "status", in.Status)
		setQ(q, "keyword", in.Keyword)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/report/tasks", q)
	},
		trpcfunction.WithName("twin_report_tasks"),
		trpcfunction.WithDescription("查询综合监控报表任务列表（生成状态/输出格式/创建时间）。用于回答「XX 报表生成好了吗」「最近有哪些报表」。"),
	)
}

// newKBSearchTool TwinMonitor 业务知识库检索（RCA 结论沉淀/运维经验/处置手册）。
// 与 aranea 本地 knowledge_search 互补：本工具查的是 TwinMonitor 侧知识库。
func newKBSearchTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in kbSearchInput) (jsonResult, error) {
		if strings.TrimSpace(in.Query) == "" {
			return jsonResult{}, fmt.Errorf("query 必填")
		}
		topK := in.TopK
		if topK <= 0 {
			topK = 5
		}
		if topK > 20 {
			topK = 20
		}
		body := map[string]any{"query": in.Query, "topK": topK}
		if strings.TrimSpace(in.Category) != "" {
			body["category"] = strings.TrimSpace(in.Category)
		}
		return cfg.gatewayPost(ctx, "/api/v1/monitor/knowledge-base/search", body)
	},
		trpcfunction.WithName("twin_kb_search"),
		trpcfunction.WithDescription("检索 TwinMonitor 业务知识库（RCA 根因沉淀/运维经验/处置手册）。遇到故障类问题先查知识库看是否有历史处置经验，再结合实时数据回答。"),
	)
}
