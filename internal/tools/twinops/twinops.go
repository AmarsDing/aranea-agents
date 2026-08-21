// Package twinops implements the TwinMonitor x GNS3 custom toolset (方案文档
// competition/10 §三，17 个工具)。业务层实现，不动 vendored trpc 框架。
//
// 数据源：
//   - twin_* 工具 → TwinMonitor Gateway（默认 http://127.0.0.1:8000），
//     认证走 X-API-Key（网关代签内部 JWT 转发上游微服务）。
//   - gns3_* 工具 → gns3_agent（默认 http://127.0.0.1:18081），无认证（仅演练内网）。
//
// 配置经环境变量注入（Docker 容器内指向 host.docker.internal）：
//
//	TWIN_GATEWAY_URL  默认 http://127.0.0.1:8000
//	TWIN_API_KEY      Gateway API Key（必填，未配置时 twin_* 调用返回结构化错误）
//	GNS3_AGENT_URL    默认 http://127.0.0.1:18081
//	TWINOPS_TIMEOUT_SEC 默认 15
package twinops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// apiKeyHeader 与 twinmonitor gateway biz.APIKeyHeader 保持一致。
const apiKeyHeader = "X-API-Key"

// Config is the connection config for TwinMonitor Gateway and gns3_agent.
type Config struct {
	GatewayBaseURL string
	APIKey         string
	GNS3BaseURL    string
	Timeout        time.Duration
}

// ConfigFromEnv loads Config from environment variables with local-dev defaults.
func ConfigFromEnv() Config {
	cfg := Config{
		GatewayBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("TWIN_GATEWAY_URL")), "/"),
		APIKey:         strings.TrimSpace(os.Getenv("TWIN_API_KEY")),
		GNS3BaseURL:    strings.TrimRight(strings.TrimSpace(os.Getenv("GNS3_AGENT_URL")), "/"),
		Timeout:        15 * time.Second,
	}
	if cfg.GatewayBaseURL == "" {
		cfg.GatewayBaseURL = "http://127.0.0.1:8000"
	}
	if cfg.GNS3BaseURL == "" {
		cfg.GNS3BaseURL = "http://127.0.0.1:18081"
	}
	if v := strings.TrimSpace(os.Getenv("TWINOPS_TIMEOUT_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Timeout = time.Duration(n) * time.Second
		}
	}
	return cfg
}

// jsonResult 统一输出容器。框架 NewFunctionTool 会对输出类型反射生成 schema；
// 若直接用 any，reflect.TypeOf(nil) 返回 nil 导致 panic（框架未防护，FW-R1 不
// 改框架，业务侧用具体类型规避）。
type jsonResult struct {
	Result any `json:"result" jsonschema:"description=上游 API 返回的原始 JSON（对象或数组）；ok=false 表示调用失败，error 为原因"`
}

// ---------- HTTP helpers ----------

// doRequest performs one HTTP call and returns a structured result. Upstream
// failures (unreachable / non-2xx) are returned as structured error objects
// (never Go errors) so the LLM records them as diagnostic evidence instead of
// retrying blindly (方案文档「失败处理约定」).
func (c Config) doRequest(ctx context.Context, gatewayAuth bool, method, rawURL string, query url.Values, body any) (jsonResult, error) {
	if len(query) > 0 {
		rawURL += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return jsonResult{}, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return jsonResult{}, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if gatewayAuth {
		if c.APIKey == "" {
			return jsonResult{Result: map[string]any{
				"ok":    false,
				"error": "TWIN_API_KEY 未配置，无法调用 TwinMonitor Gateway（请在 Aranea 运行环境设置 TWIN_API_KEY）",
			}}, nil
		}
		req.Header.Set(apiKeyHeader, c.APIKey)
	}
	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return jsonResult{Result: map[string]any{"ok": false, "error": "目标不可达: " + err.Error(), "url": redactURL(rawURL)}}, nil
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return jsonResult{Result: map[string]any{"ok": false, "error": "读取响应失败: " + err.Error()}}, nil
	}
	if resp.StatusCode >= 400 {
		snippet := string(data)
		if len(snippet) > 2000 {
			snippet = snippet[:2000]
		}
		return jsonResult{Result: map[string]any{
			"ok":         false,
			"httpStatus": resp.StatusCode,
			"error":      snippet,
			"url":        redactURL(rawURL),
		}}, nil
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		text := string(data)
		if len(text) > 8000 {
			text = text[:8000]
		}
		return jsonResult{Result: map[string]any{"ok": true, "raw": text}}, nil
	}
	return jsonResult{Result: parsed}, nil
}

func redactURL(u string) string {
	if len(u) > 300 {
		return u[:300]
	}
	return u
}

func (c Config) gatewayGet(ctx context.Context, path string, query url.Values) (jsonResult, error) {
	return c.doRequest(ctx, true, http.MethodGet, c.GatewayBaseURL+path, query, nil)
}

func (c Config) gatewayPost(ctx context.Context, path string, body any) (jsonResult, error) {
	return c.doRequest(ctx, true, http.MethodPost, c.GatewayBaseURL+path, nil, body)
}

func (c Config) gns3Get(ctx context.Context, path string) (jsonResult, error) {
	return c.doRequest(ctx, false, http.MethodGet, c.GNS3BaseURL+path, nil, nil)
}

func (c Config) gns3Post(ctx context.Context, path string, body any) (jsonResult, error) {
	return c.doRequest(ctx, false, http.MethodPost, c.GNS3BaseURL+path, nil, body)
}

// setQ puts a query param when the value is non-zero.
func setQ(q url.Values, key, val string) {
	if strings.TrimSpace(val) != "" {
		q.Set(key, val)
	}
}

func setQI(q url.Values, key string, val int) {
	if val > 0 {
		q.Set(key, strconv.Itoa(val))
	}
}

// ---------- 输入结构 ----------

type alarmQueryInput struct {
	AlarmLevel string `json:"alarm_level,omitempty" jsonschema:"description=告警级别过滤，如 critical/major/minor/warning"`
	Status     string `json:"status,omitempty" jsonschema:"description=告警状态过滤，如 active/confirmed/recovered"`
	Keyword    string `json:"keyword,omitempty" jsonschema:"description=模糊搜索 alarm_id/device_name/message"`
	DeviceID   int    `json:"device_id,omitempty" jsonschema:"description=按设备 ID 过滤"`
	RuleID     int    `json:"rule_id,omitempty" jsonschema:"description=按告警规则 ID 过滤"`
	MetricKey  string `json:"metric_key,omitempty" jsonschema:"description=按指标 key 过滤，如 line_outage"`
	StartTime  string `json:"start_time,omitempty" jsonschema:"description=时间窗起点 RFC3339，如 2026-08-14T00:00:00Z"`
	EndTime    string `json:"end_time,omitempty" jsonschema:"description=时间窗终点 RFC3339"`
	Page       int    `json:"page,omitempty" jsonschema:"description=页码，默认 1"`
	PageSize   int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20，最大 100"`
}

type alarmIDInput struct {
	AlarmID string `json:"alarm_id" jsonschema:"description=告警 ID，如 ALM-20260814-xxxxxx,required"`
}

type alarmAckInput struct {
	AlarmID string `json:"alarm_id" jsonschema:"description=告警 ID,required"`
	Comment string `json:"comment,omitempty" jsonschema:"description=确认备注（处理人/处理意见）"`
}

type lineStatusInput struct {
	LineID   int `json:"line_id,omitempty" jsonschema:"description=线路 ID；不传则返回全部线路实时状态列表"`
	Page     int `json:"page,omitempty" jsonschema:"description=页码（列表模式）"`
	PageSize int `json:"page_size,omitempty" jsonschema:"description=每页条数（列表模式），默认 50"`
}

type lineEventsInput struct {
	LineID    int    `json:"line_id,omitempty" jsonschema:"description=线路 ID 过滤"`
	EventType string `json:"event_type,omitempty" jsonschema:"description=事件类型过滤，如 outage/recovered"`
	Status    string `json:"status,omitempty" jsonschema:"description=事件状态过滤，如 active/recovered"`
	Keyword   string `json:"keyword,omitempty" jsonschema:"description=关键字过滤"`
	Page      int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize  int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

type deviceIDInput struct {
	DeviceID int `json:"device_id" jsonschema:"description=TwinMonitor 设备 ID,required"`
}

type deviceSearchInput struct {
	Keyword  string `json:"keyword,omitempty" jsonschema:"description=按名称/IP/资产编号模糊搜索"`
	Page     int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

type deviceMetricsInput struct {
	DeviceID   int    `json:"device_id" jsonschema:"description=设备 ID,required"`
	MetricKeys string `json:"metric_keys,omitempty" jsonschema:"description=指标 key 列表，逗号分隔，如 alive,latency,loss"`
	StartTime  string `json:"start_time,omitempty" jsonschema:"description=时间窗起点 RFC3339"`
	EndTime    string `json:"end_time,omitempty" jsonschema:"description=时间窗终点 RFC3339"`
	Page       int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize   int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 50"`
}

type remediationStatusInput struct {
	ExecutionID int    `json:"execution_id,omitempty" jsonschema:"description=执行单 ID；不传则按条件列出执行单"`
	Status      string `json:"status,omitempty" jsonschema:"description=执行单状态过滤（列表模式）"`
	Page        int    `json:"page,omitempty" jsonschema:"description=页码（列表模式）"`
	PageSize    int    `json:"page_size,omitempty" jsonschema:"description=每页条数（列表模式），默认 20"`
}

type alarmRuleInput struct {
	RuleID   int `json:"rule_id,omitempty" jsonschema:"description=告警规则 ID；不传则列出全部规则"`
	Page     int `json:"page,omitempty" jsonschema:"description=页码（列表模式）"`
	PageSize int `json:"page_size,omitempty" jsonschema:"description=每页条数（列表模式），默认 20"`
}

type collectorStatusInput struct {
	DeviceID int `json:"device_id" jsonschema:"description=设备 ID,required"`
}

type lineProbeInput struct {
	LineID int `json:"line_id" jsonschema:"description=线路 ID,required"`
}

type inspectionQueryInput struct {
	Keyword  string `json:"keyword,omitempty" jsonschema:"description=关键词（匹配资产名/IP/摘要）"`
	Status   string `json:"status,omitempty" jsonschema:"description=按结果过滤：success/failed/partial"`
	TaskID   int    `json:"task_id,omitempty" jsonschema:"description=按巡检任务 ID 过滤"`
	Page     int    `json:"page,omitempty" jsonschema:"description=页码"`
	PageSize int    `json:"page_size,omitempty" jsonschema:"description=每页条数，默认 20"`
}

type gns3HealthInput struct {
	Device string `json:"device,omitempty" jsonschema:"description=设备名（如 sw1/pc1）；不传返回全部设备健康"`
}

type gns3ExecInput struct {
	Device string `json:"device" jsonschema:"description=目标设备名（gns3_agent 已纳管的设备）,required"`
	Cmd    string `json:"cmd" jsonschema:"description=控制台命令。只读白名单：ping/show/ip 查询类/traceroute/arp/cat/echo/curl/hostname/uptime/vtysh -c show；写操作一律拒绝,required"`
}

type gns3PortInput struct {
	Port string `json:"port" jsonschema:"description=SW1 端口，enum=eth0,enum=eth1,enum=eth2,enum=eth3,required"`
}

// ---------- gns3_exec 命令白名单 ----------

var execAllowPrefixes = []string{
	"ping ", "ping\t", "show", "ip addr", "ip route", "ip link show", "ip neigh",
	"ip -", "traceroute ", "tracepath ", "arp", "cat ", "echo ", "hostname",
	"uptime", "curl ", "vtysh -c",
}

var execDenySubstrings = []string{
	"link set", "addr add", "addr del", "route add", "route del", "route flush",
	"write", "reload", "shutdown", "reboot", "conf t", "delete", "kill",
	"iptables", " no ", ">", "|", ";", "&", "`", "$(",
}

func checkExecWhitelist(cmd string) error {
	norm := strings.ToLower(strings.TrimSpace(cmd))
	if norm == "" {
		return fmt.Errorf("cmd 不能为空")
	}
	for _, deny := range execDenySubstrings {
		if strings.Contains(norm, deny) {
			return fmt.Errorf("命令被白名单拒绝（含禁止片段 %q）。本工具仅允许只读探测命令", deny)
		}
	}
	for _, allow := range execAllowPrefixes {
		if strings.HasPrefix(norm, allow) {
			return nil
		}
	}
	return fmt.Errorf("命令被白名单拒绝。允许前缀: %s", strings.Join(execAllowPrefixes, ", "))
}

// ---------- 工具构造函数 ----------

func newAlarmQueryTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in alarmQueryInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "alarmLevel", in.AlarmLevel)
		setQ(q, "status", in.Status)
		setQ(q, "keyword", in.Keyword)
		setQI(q, "deviceId", in.DeviceID)
		setQI(q, "ruleId", in.RuleID)
		setQ(q, "metricKey", in.MetricKey)
		setQ(q, "startTime", in.StartTime)
		setQ(q, "endTime", in.EndTime)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/alarm/events", q)
	},
		trpcfunction.WithName("twin_alarm_query"),
		trpcfunction.WithDescription("查询 TwinMonitor 告警事件列表（按级别/状态/关键字/设备/规则/时间窗过滤）。返回告警摘要列表与分页信息。"),
	)
}

func newAlarmGetTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in alarmIDInput) (jsonResult, error) {
		if strings.TrimSpace(in.AlarmID) == "" {
			return jsonResult{}, fmt.Errorf("alarm_id 必填")
		}
		return cfg.gatewayGet(ctx, "/api/v1/monitor/alarm/events/"+url.PathEscape(in.AlarmID), nil)
	},
		trpcfunction.WithName("twin_alarm_get"),
		trpcfunction.WithDescription("获取单条告警事件详情（关联设备/线路、指标、消息、状态流转时间等全量字段）。"),
	)
}

func newAlarmAckTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in alarmAckInput) (jsonResult, error) {
		if strings.TrimSpace(in.AlarmID) == "" {
			return jsonResult{}, fmt.Errorf("alarm_id 必填")
		}
		return cfg.gatewayPost(ctx, "/api/v1/monitor/alarm/events/"+url.PathEscape(in.AlarmID)+"/confirm",
			map[string]any{"comment": in.Comment})
	},
		trpcfunction.WithName("twin_alarm_ack"),
		trpcfunction.WithDescription("确认告警（标记处理中）。写操作，需先获得值班长/人工授权。"),
	)
}

func newLineStatusTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in lineStatusInput) (jsonResult, error) {
		if in.LineID > 0 {
			return cfg.gatewayGet(ctx, fmt.Sprintf("/api/v1/monitor/linemonitor/lines/%d/realtime", in.LineID), nil)
		}
		q := url.Values{}
		q.Set("status", "-1") // linemonitor ListLines 默认只查禁用，必须显式 -1 查全部
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/linemonitor/lines", q)
	},
		trpcfunction.WithName("twin_line_status"),
		trpcfunction.WithDescription("查询线路实时探测状态。传 line_id 返回该线路最新探测结果；不传返回全部线路状态列表。"),
	)
}

func newLineEventsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in lineEventsInput) (jsonResult, error) {
		q := url.Values{}
		setQI(q, "lineId", in.LineID)
		setQ(q, "eventType", in.EventType)
		setQ(q, "status", in.Status)
		setQ(q, "keyword", in.Keyword)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/linemonitor/events", q)
	},
		trpcfunction.WithName("twin_line_events"),
		trpcfunction.WithDescription("查询线路中断/恢复事件历史（outage/recovered），用于故障时间线取证。"),
	)
}

// extractListItems 从网关分页响应中取 items；上游 ok=false 或结构不符时返回 false。
func extractListItems(res jsonResult) ([]any, bool) {
	m, ok := res.Result.(map[string]any)
	if !ok {
		return nil, false
	}
	if okFlag, exists := m["ok"].(bool); exists && !okFlag {
		return nil, false
	}
	items, _ := m["items"].([]any)
	return items, true
}

func jsonNumber(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// newDeviceSearchTool 搜索设备/资产。monitor-devices 从表无名称字段且网关
// List 不支持 keyword，故拉取资产主表 + 设备从表本地拼接过滤（2026-08-15
// P3 复盘：诊断岗 8 次搜索零贡献的根修）。
func newDeviceSearchTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in deviceSearchInput) (jsonResult, error) {
		big := url.Values{}
		big.Set("page", "1")
		big.Set("pageSize", "500")
		assetsRes, err := cfg.gatewayGet(ctx, "/api/v1/monitor/monitor-assets", big)
		if err != nil {
			return assetsRes, err
		}
		assets, ok := extractListItems(assetsRes)
		if !ok {
			return assetsRes, nil // 上游结构化错误原样透传
		}
		devsRes, err := cfg.gatewayGet(ctx, "/api/v1/monitor/monitor-devices", big)
		if err != nil {
			return devsRes, err
		}
		devs, ok := extractListItems(devsRes)
		if !ok {
			return devsRes, nil
		}
		// assetId → 设备监控配置
		type devInfo struct {
			deviceID      float64
			monitorStatus float64
			internal      float64
		}
		devByAsset := make(map[float64]devInfo, len(devs))
		for _, d := range devs {
			dm, ok := d.(map[string]any)
			if !ok {
				continue
			}
			devByAsset[jsonNumber(dm["assetId"])] = devInfo{
				deviceID:      jsonNumber(dm["id"]),
				monitorStatus: jsonNumber(dm["monitorStatus"]),
				internal:      jsonNumber(dm["internal"]),
			}
		}
		kw := strings.ToLower(strings.TrimSpace(in.Keyword))
		matched := make([]map[string]any, 0, len(assets))
		for _, a := range assets {
			am, ok := a.(map[string]any)
			if !ok {
				continue
			}
			haystack := strings.ToLower(fmt.Sprintf("%v %v %v %v",
				am["assetCode"], am["name"], am["alias"], am["location"]))
			if kw != "" && !strings.Contains(haystack, kw) {
				continue
			}
			item := map[string]any{
				"asset_id":   am["id"],
				"asset_code": am["assetCode"],
				"name":       am["name"],
				"location":   am["location"],
			}
			if di, exists := devByAsset[jsonNumber(am["id"])]; exists {
				item["device_id"] = di.deviceID
				item["monitor_status"] = di.monitorStatus
				item["collect_interval_sec"] = di.internal
			}
			matched = append(matched, item)
		}
		// 本地分页
		page, pageSize := in.Page, in.PageSize
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 20
		}
		total := len(matched)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		return jsonResult{Result: map[string]any{
			"ok": true, "total": total, "page": page, "page_size": pageSize,
			"items": matched[start:end],
		}}, nil
	},
		trpcfunction.WithName("twin_device_search"),
		trpcfunction.WithDescription("按关键字搜索 TwinMonitor 监控设备/资产列表（名称/IP/资产编号），返回设备摘要（含名称、资产编号、device_id），是诊断入手第一步。"),
	)
}

func newDeviceGetTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in deviceIDInput) (jsonResult, error) {
		if in.DeviceID <= 0 {
			return jsonResult{}, fmt.Errorf("device_id 必填且必须为正整数")
		}
		devRes, err := cfg.gatewayGet(ctx, fmt.Sprintf("/api/v1/monitor/monitor-devices/%d", in.DeviceID), nil)
		if err != nil {
			return devRes, err
		}
		dm, ok := devRes.Result.(map[string]any)
		if !ok {
			return devRes, nil
		}
		if okFlag, exists := dm["ok"].(bool); exists && !okFlag {
			return devRes, nil
		}
		// 拼接资产主表名称/编号，避免匿名配置（与 search 同源根修）
		assetID := int64(jsonNumber(dm["assetId"]))
		if assetID <= 0 {
			return devRes, nil
		}
		assetRes, err := cfg.gatewayGet(ctx, fmt.Sprintf("/api/v1/monitor/monitor-assets/%d", assetID), nil)
		if err != nil {
			return devRes, nil // 设备数据已可用，资产详情失败不阻塞
		}
		am, ok := assetRes.Result.(map[string]any)
		if !ok {
			return devRes, nil
		}
		if okFlag, exists := am["ok"].(bool); exists && !okFlag {
			return devRes, nil
		}
		dm["asset_code"] = am["assetCode"]
		dm["asset_name"] = am["name"]
		dm["asset_location"] = am["location"]
		return jsonResult{Result: dm}, nil
	},
		trpcfunction.WithName("twin_device_get"),
		trpcfunction.WithDescription("获取设备/资产详情画像（名称、资产编号、监控状态、采集间隔、系统信息等）。"),
	)
}

func newDeviceMetricsTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in deviceMetricsInput) (jsonResult, error) {
		if in.DeviceID <= 0 {
			return jsonResult{}, fmt.Errorf("device_id 必填且必须为正整数")
		}
		q := url.Values{}
		setQ(q, "metricKeys", in.MetricKeys)
		setQ(q, "startTime", in.StartTime)
		setQ(q, "endTime", in.EndTime)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, fmt.Sprintf("/api/v1/monitor/query/devices/%d/history", in.DeviceID), q)
	},
		trpcfunction.WithName("twin_device_metrics"),
		trpcfunction.WithDescription("查询设备指标历史序列（在线状态/时延/丢包等），用于趋势判断与基线对比。"),
	)
}

func newRemediationStatusTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in remediationStatusInput) (jsonResult, error) {
		if in.ExecutionID > 0 {
			return cfg.gatewayGet(ctx, fmt.Sprintf("/api/v1/monitor/remediation/executions/%d", in.ExecutionID), nil)
		}
		q := url.Values{}
		setQ(q, "status", in.Status)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/remediation/executions", q)
	},
		trpcfunction.WithName("twin_remediation_status"),
		trpcfunction.WithDescription("查询故障处置执行单状态与日志摘要。传 execution_id 查单条详情，不传按状态列出。"),
	)
}

func newAlarmRuleTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in alarmRuleInput) (jsonResult, error) {
		if in.RuleID > 0 {
			return cfg.gatewayGet(ctx, fmt.Sprintf("/api/v1/monitor/alarm/ap-alarm-rules/%d", in.RuleID), nil)
		}
		q := url.Values{}
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/alarm/ap-alarm-rules", q)
	},
		trpcfunction.WithName("twin_alarm_rule_get"),
		trpcfunction.WithDescription("查询告警规则详情（触发条件/阈值/级别/通知策略），用于解释告警为何触发。只读。"),
	)
}

func newCollectorStatusTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in collectorStatusInput) (jsonResult, error) {
		if in.DeviceID <= 0 {
			return jsonResult{}, fmt.Errorf("device_id 必填且必须为正整数")
		}
		status, err := cfg.gatewayGet(ctx, fmt.Sprintf("/api/v1/monitor/collector/devices/%d/status", in.DeviceID), nil)
		if err != nil {
			return jsonResult{}, err
		}
		out := map[string]any{"deviceId": in.DeviceID, "collectorStatus": status.Result}
		q := url.Values{}
		q.Set("unresolvedOnly", "true")
		failures, ferr := cfg.gatewayGet(ctx, fmt.Sprintf("/api/v1/monitor/collector/devices/%d/failures", in.DeviceID), q)
		if ferr != nil {
			return jsonResult{}, ferr
		}
		out["unresolvedFailures"] = failures.Result
		return jsonResult{Result: out}, nil
	},
		trpcfunction.WithName("twin_collector_status"),
		trpcfunction.WithDescription("查询设备采集层状态（在线/连续失败次数/最近变更原因）与未恢复采集失败记录，用于区分「设备故障」与「采集故障」。"),
	)
}

func newLineProbeTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in lineProbeInput) (jsonResult, error) {
		if in.LineID <= 0 {
			return jsonResult{}, fmt.Errorf("line_id 必填且必须为正整数")
		}
		return cfg.gatewayPost(ctx, fmt.Sprintf("/api/v1/monitor/linemonitor/lines/%d/probe-test", in.LineID), map[string]any{})
	},
		trpcfunction.WithName("twin_line_probe"),
		trpcfunction.WithDescription("主动触发一次线路探测（不等 30s 探测周期），返回本次探测结果。用于处置后快速验证。"),
	)
}

func newInspectionQueryTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in inspectionQueryInput) (jsonResult, error) {
		q := url.Values{}
		setQ(q, "keyword", in.Keyword)
		setQ(q, "status", in.Status)
		setQI(q, "taskId", in.TaskID)
		setQI(q, "page", in.Page)
		setQI(q, "pageSize", in.PageSize)
		return cfg.gatewayGet(ctx, "/api/v1/monitor/opstools/inspection/records", q)
	},
		trpcfunction.WithName("twin_inspection_query"),
		trpcfunction.WithDescription("查询 TwinMonitor 巡检记录（按关键词/结果/任务过滤），用于验证环节核对与复盘取证。只读。"),
	)
}

func newGNS3HealthTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in gns3HealthInput) (jsonResult, error) {
		if strings.TrimSpace(in.Device) != "" {
			return cfg.gns3Get(ctx, "/health/"+url.PathEscape(strings.TrimSpace(in.Device)))
		}
		return cfg.gns3Get(ctx, "/health")
	},
		trpcfunction.WithName("gns3_health_check"),
		trpcfunction.WithDescription("探测 GNS3 仿真设备健康状态（HTTP 业务级健康检查）。传 device 查单台，不传返回全部设备。"),
	)
}

func newGNS3ExecTool(cfg Config) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in gns3ExecInput) (jsonResult, error) {
		if strings.TrimSpace(in.Device) == "" {
			return jsonResult{}, fmt.Errorf("device 必填")
		}
		if err := checkExecWhitelist(in.Cmd); err != nil {
			return jsonResult{}, err
		}
		res, err := cfg.gns3Post(ctx, "/exec", map[string]any{"device": strings.TrimSpace(in.Device), "cmd": in.Cmd})
		if err == nil {
			enrichExecResult(in.Cmd, res.Result)
		}
		return res, err
	},
		trpcfunction.WithName("gns3_exec"),
		trpcfunction.WithDescription("在 GNS3 仿真设备控制台执行只读命令（白名单：ping/show/ip 查询/traceroute/arp/cat/echo/curl 等；写操作一律拒绝）。"),
	)
}

// embedPortDownHint 在 gns3_exec 成功结果含端口 DOWN 证据时内嵌行动指引
// （2026-08-16 B2 根修：模型拿到「eth1 state DOWN」证据后仍顽固重发同参取证
// 直至烧光预算；在证据现场直接写明「取证已完成、禁止重发、推进下一步」，
// 把纠偏时机从「第 3 次被拦截」提前到「第 1 次拿到证据」）。
// 文案不写死具体下一跳工具：diagnose（只读）与 remediate（变更）都会跑
// ip link show，下一动作由各节点任务指令决定，防诱导只读节点越权调 fault_clear。
func embedPortDownHint(result any) {
	m, ok := result.(map[string]any)
	if !ok {
		return
	}
	// gns3_agent 成功响应无 ok 键（{"device","cmd","output"}）；仅当 ok 键
	// 存在且为 false（doRequest 的错误包装）时跳过。
	if okFlag, exists := m["ok"].(bool); exists && !okFlag {
		return
	}
	b, err := json.Marshal(m)
	if err != nil || !strings.Contains(strings.ToLower(string(b)), "state down") {
		return
	}
	if _, exists := m["next_action_hint"]; !exists {
		m["next_action_hint"] = "检测到端口 state DOWN——取证已完成，禁止为「再确认」重发本命令" +
			"（重复调用将被系统拦截并白白消耗调用预算）；立即按本节点任务指令推进到下一步动作。"
	}
}

// 方案 A（2026-08-16 终验根修）：ip link show 结果结构化消歧。
//
// 实证根因：B3+B4 终验轮 remediate 两次拿到含「eth1 state DOWN」的结果仍顽固
// 重发取证直至被 B4 终止——输出中 eth1/eth3 双 DOWN 并存，需靠 NO-CARRIER 标志
// 在控制台噪声文本（提示符回显、单行压平）中判别故障口，flash 级模型判别失败
// 触发「再确认」反射，全部文字纠偏（指令/hint/拦截回放）失效。
// 根治：工具侧把 ip link show 输出解析为 ports 结构化数组（state/NO-CARRIER
// 已判别好），hint 直接点名管理性 DOWN 端口——模型零解析负担，歧义在源头消除。
// hint 仍不写死下一跳工具名（只读/变更节点共用本工具，下一动作归节点指令）。

var (
	// 端口段头：`3: eth1: <BROADCAST,MULTICAST>`。MAC（0c:37:...）与提示符
	// （root@OpenWrt:~#）均不满足「数字: 字母开头名: <标志>」形态，不会误配。
	linkShowHeaderRe = regexp.MustCompile(`(?:^|\s)(\d+):\s+([A-Za-z][\w@.\-]*):\s+<([^>]*)>`)
	linkShowStateRe  = regexp.MustCompile(`\bstate\s+([A-Za-z]+)`)
)

func isLinkShowCmd(cmd string) bool {
	return strings.HasPrefix(strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(cmd))), " "), "ip link")
}

// parseLinkShowPorts 把（可能被压平成单行的）ip link show 输出解析为端口数组。
// 滚动回显中同一端口出现多次时以最后一次为准（最新状态），顺序按最后出现排。
func parseLinkShowPorts(output string) []map[string]any {
	idx := linkShowHeaderRe.FindAllStringSubmatchIndex(output, -1)
	if len(idx) == 0 {
		return nil
	}
	order := []string{}
	byName := map[string]map[string]any{}
	for i, loc := range idx {
		name := output[loc[4]:loc[5]]
		flags := output[loc[6]:loc[7]]
		segEnd := len(output)
		if i+1 < len(idx) {
			segEnd = idx[i+1][0]
		}
		seg := output[loc[1]:segEnd]
		state := ""
		if sm := linkShowStateRe.FindStringSubmatch(seg); sm != nil {
			state = strings.ToUpper(sm[1])
		}
		noCarrier := false
		for _, f := range strings.Split(flags, ",") {
			if strings.TrimSpace(f) == "NO-CARRIER" {
				noCarrier = true
				break
			}
		}
		if _, seen := byName[name]; !seen {
			order = append(order, name)
		}
		byName[name] = map[string]any{"name": name, "state": state, "no_carrier": noCarrier}
	}
	ports := make([]map[string]any, 0, len(order))
	for _, name := range order {
		ports = append(ports, byName[name])
	}
	return ports
}

// enrichExecResult 对 gns3_exec 成功结果做证据增强：ip link show 类命令附加
// ports 结构化数组并把故障端口点名进 hint；其余命令回退通用 state DOWN 指引。
func enrichExecResult(cmd string, result any) {
	m, ok := result.(map[string]any)
	if !ok {
		return
	}
	// gns3_agent 成功响应无 ok 键；仅当 ok 键存在且为 false 时跳过。
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

// ---------- 配置自动化工具（Phase B，3 个） ----------
//
// 统一经 TwinMonitor 13-aiops MCP 安全层执行（POST /api/v1/monitor/aiops/mcp/call）：
// 风险分级/审批门禁/调用审计由 MCP 侧兜底。push/rollback 为 destructive，
// MCP grant_policy=approval 时响应 pending=true + approvalId（审批批准后异步执行，
// 结果查 call-history trace_id=approval-{id}），本层原样透传给 LLM。
//
// 编排耗时较长（前置备份 SSH 抓取 → 会话式下发 → 再备份 → diff），
// 单工具超时放宽至 300s（与 aiops 侧 opstool client 超时对齐）。

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

// NewToolset returns all 20 twinops tools.
func NewToolset(cfg Config) []trpctool.Tool {
	registerCompensationPairs() // P0-1：补偿对声明（见 compensation.go）
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
