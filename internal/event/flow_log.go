package event

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const FlowLogSchemaVersion = "flow_log/v1"

// FlowPhase marks the phase of a flow step.
type FlowPhase string

const (
	FlowPhaseStart FlowPhase = "start"
	FlowPhaseDone  FlowPhase = "done"
	FlowPhaseError FlowPhase = "error"
	FlowPhaseSkip  FlowPhase = "skip"
)

// Pair is a key-value pair for extra metadata.
type Pair struct {
	Key   string
	Value any
}

// P is a shorthand for creating a Pair.
func P(key string, value any) Pair {
	return Pair{Key: key, Value: value}
}

// FlowSeverity is the user-facing alert level (red/yellow/green).
type FlowSeverity string

const (
	FlowSeverityOK       FlowSeverity = "ok"
	FlowSeverityInfo     FlowSeverity = "info"
	FlowSeverityWarn     FlowSeverity = "warn"
	FlowSeverityError    FlowSeverity = "error"
	FlowSeverityCritical FlowSeverity = "critical"
)

// FlowCorrelation ties a log entry to a trace/run.
type FlowCorrelation struct {
	TraceID   string `json:"trace_id"`
	SessionID string `json:"session_id"`
	RunID     string `json:"run_id"`
	TeamID    string `json:"team_id,omitempty"`
	Domain    string `json:"domain"`
	AgentKey  string `json:"agent_key,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
}

// FlowStep describes the step identity and lifecycle phase.
type FlowStep struct {
	ID        string `json:"id"`
	Phase     string `json:"phase"`
	Subsystem string `json:"subsystem,omitempty"`
}

// FlowTiming holds optional duration data.
type FlowTiming struct {
	DurationMS int64  `json:"duration_ms,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
}

// FlowError is set when severity is error or critical.
type FlowError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// FlowLogEntry is the v2 flow log payload (schema flow_log/v1).
type FlowLogEntry struct {
	SchemaVersion string          `json:"schema_version"`
	ID            string          `json:"id"`
	Timestamp     string          `json:"timestamp"`
	Correlation   FlowCorrelation `json:"correlation"`
	Step          FlowStep        `json:"step"`
	Severity      FlowSeverity    `json:"severity"`
	Title         string          `json:"title"`
	Message       string          `json:"message"`
	Hint          string          `json:"hint,omitempty"`
	Timing        *FlowTiming     `json:"timing,omitempty"`
	Error         *FlowError      `json:"error,omitempty"`
	Extra         map[string]any  `json:"extra,omitempty"`
}

// stepTitleRegistry maps step_id → default Chinese title for humans.
var stepTitleRegistry = map[string]string{
	"chat.turn.enter":                      "开始处理对话",
	"chat.agent.build":                     "构建 Agent",
	"chat.plugins_load":                    "加载插件",
	"chat.runner.create":                   "创建 Runner",
	"chat.user_msg_persist":                "保存用户消息",
	"chat.intent.pass":                     "意图识别",
	"chat.llm.invoke":                      "调用语言模型",
	"chat.stream.consume":                  "处理模型输出",
	"chat.assistant_msg_persist":           "保存助手回复",
	"chat.turn.execute":                    "对话轮次",
	"chat.turn.timeout":                    "对话超时",
	"chat.turn.empty_reply":                "未收到模型回复",
	"chat.first_byte_timeout":              "模型响应过慢",
	"chat.usage_record":                    "用量记录",
	"chat.pending_dequeue":                 "处理排队消息",
	"knowledge.rerank.fallback":            "重排降级为向量排序",
	"event_bus.usage.record":               "用量事件写入失败",
	"event_bus.monitor.persist":            "监控事件持久化失败",
	"event_bus.state.persist":              "会话状态保存失败",
	"event_bus.state.apply":                "会话状态应用失败",
	"system.bus.drop":                      "事件总线丢弃消息",
	"system.ws.upgrade_failed":             "WebSocket 升级失败",
	"system.ws.read_error":                 "WebSocket 读错误",
	"system.ws.send_drop":                  "WebSocket 发送缓冲区满",
	"system.ws.parse_error":                "WebSocket 消息解析失败",
	"system.ws.send_failed":                "WebSocket 发送失败",
	"system.cron.job_dead":                 "定时任务进入死信",
	"system.cron.retry":                    "定时任务重试",
	"system.cron.panic":                    "定时任务 panic",
	"system.cron.dispatch_skipped":         "定时任务跳过（会话忙）",
	"system.telemetry.init":                "遥测初始化",
	"system.telemetry.noop":                "遥测未配置",
	"system.telemetry.error":               "遥测初始化失败",
	"system.auth.bypass_warn":              "认证绕过警告",
	"system.auth.bypass_refused":           "认证绕过被拒绝",
	"system.auth.bypass_active":            "认证绕过已启用",
	"system.plugin.seed_fail":              "插件种子同步失败",
	"system.plugin.reload_fail":            "插件运行时重载失败",
	"system.hook.reload_fail":              "Hook 重载失败",
	"system.mcp.health_list_fail":          "MCP 健康检查列表失败",
	"system.mcp.health_persist_fail":       "MCP 健康状态保存失败",
	"system.agent.cache_hit":               "Agent 缓存命中",
	"system.agent.cache_miss":              "Agent 缓存未命中",
	"system.agent.db_resolve":              "Agent 数据库解析",
	"system.agent.skill_build":             "Agent 技能构建",
	"system.agent.tool_build":              "Agent 工具构建",
	"system.agent.memory_disabled":         "Agent 记忆未配置",
	"system.provider.catalog_fail":         "模型目录查询失败",
	"system.provider.preflight_fail":       "模型预检失败",
	"system.provider.config_resolved":      "模型配置已解析",
	"system.provider.preflight_ok":         "模型预检通过",
	"system.provider.ha_failover":          "HA 故障切换",
	"system.provider.ha_hedge":             "HA 对冲切换",
	"system.tool.record_fail":              "工具调用记录失败",
	"system.intent.pass_done":              "意图识别完成",
	"system.memory_worker.enqueue":         "自动记忆任务入队",
	"system.auto_memory.extract_fail":      "自动记忆提取失败",
	"system.auto_memory.extract_max_retry": "自动记忆提取重试耗尽",
	"system.auto_memory.l4_fail":           "L4 图谱写入失败",
	"system.auto_memory.done":              "自动记忆提取完成",
	"system.monitor.alert_webhook_fail":    "告警 Webhook 失败",
	"system.monitor.alert_channel_fail":    "告警通道发送失败",
	"system.builtin_tools_sync_fail":       "内置工具同步失败",
	"system.session.compress":              "会话上下文压缩",
	"system.session.title_fail":            "会话标题生成失败",
	"system.session.rollback_fail":         "会话回滚失败",
	"system.graph.task_start_fail":         "图任务启动失败",
	"system.graph.task_status_fail":        "图任务状态发布失败",
	"system.graph.task_resume_fail":        "图任务恢复失败",
	"system.graph.runtime_run_fail":        "图运行时启动失败",
	"system.graph.runtime_resume_fail":     "图运行时恢复失败",
	"system.task.timeout_update_fail":      "任务超时更新失败",
	"system.task.release_claim_fail":       "任务释放声明失败",
	"system.task.dispatcher_tick_fail":     "任务调度器 tick 失败",
	"system.task.check_timeout_fail":       "任务超时检查失败",
	"system.task.claim_fail":               "任务声明失败",
	"system.task.dispatch_run_fail":        "任务分发运行失败",
	"system.channel.dead_letter":           "渠道投递死信",
	"system.knowledge.embed_fail":          "知识嵌入失败",
	"system.safego.panic":                  "协程 panic 已恢复",
	"system.grpc.unauthenticated":          "gRPC 未认证请求",
	"system.data.builtin_tool_sync":        "内置工具同步",
	"chat.intent.merge_fail":               "意图结果合并失败",
	"chat.usage_record_fail":               "会话用量记录失败",
	"team.intent.merge_fail":               "团队意图合并失败",
	"team.intent_anchor_fallback":          "团队意图锚点回退",
	"team.usage_record_fail":               "团队成员用量记录失败",
	"team.turn.usage":                      "团队轮次用量",
}

func stepTitle(stepID string) string {
	if t, ok := stepTitleRegistry[stepID]; ok {
		return t
	}
	return stepID
}

func stepSubsystem(stepID string) string {
	parts := strings.SplitN(stepID, ".", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func severityForPhase(phase FlowPhase, explicit FlowSeverity) FlowSeverity {
	if explicit != "" {
		return explicit
	}
	switch phase {
	case FlowPhaseError:
		return FlowSeverityError
	case FlowPhaseSkip:
		return FlowSeverityWarn
	case FlowPhaseDone:
		return FlowSeverityOK
	case FlowPhaseStart:
		return FlowSeverityInfo
	default:
		return FlowSeverityInfo
	}
}

func (e FlowLogEntry) displayText() string {
	var b strings.Builder
	if e.Title != "" {
		b.WriteString(e.Title)
	}
	if e.Message != "" {
		if b.Len() > 0 {
			b.WriteString(" — ")
		}
		b.WriteString(e.Message)
	}
	if e.Timing != nil && e.Timing.DurationMS > 0 {
		fmt.Fprintf(&b, " (%dms)", e.Timing.DurationMS)
	}
	if b.Len() == 0 {
		b.WriteString(e.Step.ID)
		b.WriteByte('.')
		b.WriteString(e.Step.Phase)
	}
	return b.String()
}

func (e FlowLogEntry) toMetadata() map[string]any {
	m := map[string]any{
		"schema_version": e.SchemaVersion,
		"flow_id":        e.ID,
		"trace_id":       e.Correlation.TraceID,
		"session_id":     e.Correlation.SessionID,
		"run_id":         e.Correlation.RunID,
		"domain":         e.Correlation.Domain,
		"agent_key":      e.Correlation.AgentKey,
		"agent_id":       e.Correlation.AgentID,
		"step_id":        e.Step.ID,
		"flow_phase":     e.Step.Phase,
		"severity":       string(e.Severity),
		"title":          e.Title,
		"message":        e.Message,
	}
	if e.Hint != "" {
		m["hint"] = e.Hint
	}
	if e.Timing != nil {
		if e.Timing.DurationMS > 0 {
			m["duration_ms"] = e.Timing.DurationMS
		}
	}
	if e.Error != nil {
		m["error_code"] = e.Error.Code
		m["error_message"] = e.Error.Message
	}
	for k, v := range e.Extra {
		m[k] = v
	}
	return m
}

func newFlowLogEntry(tc TraceContext, stepID string, phase FlowPhase, sev FlowSeverity, title, message, hint string, timing *FlowTiming, flowErr *FlowError, extra map[string]any) FlowLogEntry {
	if title == "" {
		title = stepTitle(stepID)
	}
	return FlowLogEntry{
		SchemaVersion: FlowLogSchemaVersion,
		ID:            "fl_" + uuid.NewString(),
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		Correlation: FlowCorrelation{
			TraceID:   tc.TraceID,
			SessionID: tc.SessionID,
			RunID:     tc.RunID,
			TeamID:    tc.TeamID,
			Domain:    string(tc.Domain),
			AgentKey:  tc.AgentKey,
			AgentID:   tc.AgentID,
		},
		Step: FlowStep{
			ID:        stepID,
			Phase:     string(phase),
			Subsystem: stepSubsystem(stepID),
		},
		Severity: severityForPhase(phase, sev),
		Title:    title,
		Message:  message,
		Hint:     hint,
		Timing:   timing,
		Error:    flowErr,
		Extra:    extra,
	}
}
