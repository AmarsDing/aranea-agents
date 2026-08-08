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
	// SpanID is the OTel span ID of the turn root, enabling cross-reference
	// between FlowLog and OTel trace (Jaeger). Populated via SetOtelRefs.
	// Empty when OTel tracing is not configured. Phase 1 of Problem 4.
	SpanID string `json:"span_id,omitempty"`
	// ParentSpanID is the OTel parent span ID of the turn root. Empty for
	// turn-root spans (no upstream OTel parent). Reserved for future phases
	// that may populate per-step parent linkage.
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
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
	"chat.turn.usage_source":               "用量来源追踪",
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
	"system.agent.build":                   "Agent 构建",
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
	"system.memory_canary.failed":          "记忆闭环金丝雀告警",
	"system.memory_l1_archive.failed":      "L1 归档连续失败告警",
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
	"team.run.start":                       "开始团队协作",
	"team.run.execute":                     "执行团队任务",
	"team.run.finish":                      "团队任务结束",
	"team.run.graph":                       "构建团队 GraphAgent",
	"team.member":                          "团队成员执行",
	"chat.team.invoke":                     "委派团队会话",
	// 2026-07-29 日志补齐批次 0：已发射但缺标题的 stepID
	"chat.receive":                  "收到消息",
	"chat.active_check":             "检查活跃运行",
	"chat.session_fetch":            "加载会话",
	"chat.session_ownership":        "会话归属校验",
	"chat.agent_hydrate":            "加载 Agent 配置",
	"chat.provider_resolve":         "解析模型配置",
	"chat.attachment.preflight":     "附件预检",
	"chat.pre_planning_gate":        "规划门决策",
	"chat.clarification_gate":       "澄清门",
	"chat.proactive_recall":         "主动召回",
	"chat.runner.ralph_loop":        "Ralph Loop 配置",
	"chat.runner.rollback":          "Runner 会话回滚",
	"chat.runner.rollback_boundary": "Runner 回滚边界",
	"chat.turn.timeout_with_reply":  "超时但已保存回复",
	"run.start":                     "创建会话运行",
	"channel.turn.background":       "渠道后台继续执行",
	// 2026-07-29 日志补齐：P0 核心链路
	"cron.job.trigger":         "定时任务触发",
	"cron.job.dispatch":        "定时任务分发",
	"cron.job.execute":         "定时任务执行",
	"graph.run.start":          "图运行开始",
	"graph.run.finish":         "图运行结束",
	"graph.run.resume":         "图运行恢复",
	"graph.node.execute":       "图节点执行",
	"graph.checkpoint.save":    "检查点保存",
	"graph.hitl.wait":          "等待人工确认",
	"skill.import.start":       "Skill 包导入开始",
	"skill.import.validate":    "Skill 包校验",
	"skill.import.conflict":    "Skill 冲突决策",
	"skill.import.done":        "Skill 导入完成",
	"skill.watch.reload":       "Skill 热重载",
	"skill.execute":            "Skill 运行时执行",
	"knowledge.ingest.start":   "知识文档摄取开始",
	"knowledge.ingest.parse":   "文档解析分块",
	"knowledge.ingest.embed":   "文档向量嵌入",
	"knowledge.ingest.done":    "知识摄取完成",
	"knowledge.vault.sync":     "Vault 同步",
	"knowledge.search":         "知识库检索",
	"knowledge.entity.merge":   "知识实体合并",
	"a2a.invoke.start":         "A2A 联邦调用开始",
	"a2a.invoke.governance":    "A2A 治理链检查",
	"a2a.invoke.remote":        "A2A 远端调用",
	"a2a.invoke.done":          "A2A 调用完成",
	"system.startup.migration": "数据库迁移",
	"system.startup.seed":      "基础数据种子",
	"system.startup.ready":     "服务就绪",
	"system.startup.shutdown":  "服务关闭",
	// 2026-07-29 日志补齐：P1 重要辅助
	"session.create":             "会话创建",
	"session.delete":             "会话删除",
	"session.rename":             "会话重命名",
	"agent.crud.create":          "Agent 创建",
	"agent.crud.update":          "Agent 更新",
	"agent.crud.delete":          "Agent 删除",
	"provider.catalog.sync":      "模型目录同步",
	"mcp.server.add":             "MCP 服务器添加",
	"mcp.server.remove":          "MCP 服务器移除",
	"memory.auto.extract":        "自动记忆提取",
	"media.generate":             "媒体生成",
	"evaluation.run":             "评测集运行",
	"channel.connect.open":       "渠道连接建立",
	"channel.connect.close":      "渠道连接断开",
	"channel.connect.error":      "渠道连接异常",
	"gateway.webhook.delivery":   "出站 Webhook 投递",
	"monitor.alert.evaluate":     "告警评估",
	"monitor.selfcheck.run":      "系统自检",
	"event_bus.flow_log.persist": "流程日志落库失败",
	// 2026-07-29 日志补齐：P2 低频管理
	"settings.update":        "系统设置更新",
	"settings.hot_reload":    "配置热更新",
	"ecosystem.pack.install": "生态包安装",
	// voice companion（M74）：与 internal/voice 实际发射对齐
	"voice.session.start":     "语音会话开始",
	"voice.session.done":      "语音会话结束",
	"voice.asr.final":         "语音识别终稿",
	"voice.asr.idle_reclaim":  "语音 ASR 空闲回收",
	"voice.tts.start":         "语音播报开始",
	"voice.tts.end":           "语音播报结束",
	"voice.tts.enqueue_fail":  "语音播报入队失败",
	"voice.barge_in":          "语音打断",
	"voice.provider.fallback": "语音服务降级",
	"voice.error":             "语音链路错误",
	"voice.confirm.resolved":  "语音确认决议",
	// client tool bridge（M74 V2-T3）：与 internal/tools/clientbridge 实际发射对齐
	"client_tool.invoke":  "调用客户端工具",
	"client_tool.result":  "客户端工具执行完成",
	"client_tool.timeout": "客户端工具执行超时",
}

func stepTitle(stepID string) string {
	if t, ok := stepTitleRegistry[stepID]; ok {
		return t
	}
	// Dynamic-suffix step IDs (e.g. "team.member.member-1") fall back to
	// their static prefix ("team.member") for title resolution.
	if i := strings.LastIndex(stepID, "."); i > 0 {
		if t, ok := stepTitleRegistry[stepID[:i]]; ok {
			return t
		}
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
	if e.SpanID != "" {
		m["span_id"] = e.SpanID
	}
	if e.ParentSpanID != "" {
		m["parent_span_id"] = e.ParentSpanID
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

func newFlowLogEntry(tc TraceContext, rootSpanID, stepID string, phase FlowPhase, sev FlowSeverity, title, message, hint string, timing *FlowTiming, flowErr *FlowError, extra map[string]any) FlowLogEntry {
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
		SpanID:   rootSpanID,
		Extra:    extra,
	}
}
