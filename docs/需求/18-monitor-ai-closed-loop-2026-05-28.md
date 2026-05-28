# Monitor AI 闭环追踪方案

> **关联**：[`18 monitor.md`](./18%20monitor.md) · [`18-monitor-optimization-2026-05-26.md`](./18-monitor-optimization-2026-05-26.md) · [`52-flow-logger.design.md`](./52-flow-logger.design.md) · 代码 Review [`2026-05-26-Monitor-Code-Review.md`](../review/2026-05-26-Monitor-Code-Review.md)
> **状态**：📐 设计草案，待评审
> **创建**：2026-05-28

---

## 0. 需求原文与问题定义

### 0.1 原始需求

> 通过后台的 logs 日志，记录服务的所有运行状态，AI 可以根据日志运行的记录文件追踪到问题，定位问题，形成闭环。

### 0.2 需求拆解

| 子需求 | 含义 | 当前状态 | 差距 |
|--------|------|----------|------|
| **记录所有运行状态** | 每个关键业务动作都有结构化日志 | 🟡 FlowLog 覆盖 ~60 个 step_id，但部分关键路径仍用 slog/zap | 需补全遗漏路径 + 统一日志出口 |
| **日志持久化到文件** | 日志写入磁盘文件，进程重启后可回溯 | ❌ FlowLog 仅走 EventBus → WS/DB，无文件落盘 | 需新增文件 Appender |
| **AI 可读取日志** | 日志格式对 AI 友好（结构化、可检索、带关联 ID） | 🟡 FlowLogEntry 已结构化，但文件输出为 ConsoleEncoder | 需 JSON Lines 文件输出 |
| **AI 追踪到问题** | 从一条错误日志出发，沿 trace_id / session_id 回溯完整链路 | 🟡 FlowLog 有 trace_id 关联，但跨表/跨文件追踪需手动拼接 | 需诊断包自动聚合 |
| **定位问题** | AI 能给出根因分析 + 修复建议 | ❌ 无自动化根因分析能力 | 需诊断规则引擎 + AI Prompt 模板 |
| **形成闭环** | 问题从发现 → 追踪 → 定位 → 修复 → 验证 全链路可追溯 | ❌ 发现靠人工看 Monitor 页面，修复后无自动验证 | 需闭环工作流 |

### 0.3 闭环定义

```
┌──────────────────────────────────────────────────────────────────────┐
│                                                                      │
│  ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌────┐ │
│  │ 1.发现   │───▶│ 2.追踪   │───▶│ 3.定位   │───▶│ 4.修复   │───▶│5.验 │ │
│  │ Detect  │    │ Trace   │    │ Root    │    │ Fix     │    │证   │ │
│  │         │    │         │    │ Cause   │    │         │    │Verify│ │
│  └─────────┘    └─────────┘    └─────────┘    └─────────┘    └────┘ │
│       ▲                                                    │       │
│       └────────────────────────────────────────────────────┘       │
│                                                                      │
│  数据源：结构化日志文件（JSON Lines）                                   │
│  关联键：trace_id + session_id + run_id                              │
│  AI 角色：自动执行 1→2→3，辅助 4，自动执行 5                           │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 1. 现状分析

### 1.1 日志体系现状

项目存在 **三套并行的日志体系**，尚未统一：

| 体系 | 实现 | 输出目标 | 结构化 | 关联 ID | AI 可读 |
|------|------|----------|--------|---------|---------|
| **框架层 zap** | `pkg/trpc-agent-go/log` | stdout（ConsoleEncoder） | ❌ 彩色控制台 | ❌ 无 trace_id | ❌ |
| **应用层 FlowLog** | `internal/event/trace_emitter.go` | EventBus → WS + DB | ✅ `flow_log/v1` | ✅ trace_id/session_id/run_id | ✅ |
| **系统域 SysLog** | `internal/event/system_flow.go` | EventBus → MonitorBus | ✅ `flow_log/v1` | 🟡 部分（system 域无 session） | ✅ |

### 1.2 关键差距

| 差距编号 | 描述 | 影响 | 关联 |
|----------|------|------|------|
| **GAP-01** | FlowLog 无文件落盘 | 进程重启/DB 损坏后无法回溯历史 | 新增 |
| **GAP-02** | 框架层 zap 日志无结构化输出 | AI 无法解析 stdout 彩色文本 | 新增 |
| **GAP-03** | 部分关键路径仍用 slog 而非 FlowLog | 关键错误无 trace_id 关联，AI 无法追踪 | MON-Q-07 相关 |
| **GAP-04** | `monitor_traces` 表无写入路径 | Traces Tab 空白，AI 无法获取 span 树 | MON-Q-05 / MON-OPT-05 |
| **GAP-05** | 无诊断包自动聚合 | AI 需手动跨表/跨文件拼接信息 | 52-flow-logger §7 基础版 |
| **GAP-06** | 无根因分析规则引擎 | AI 只能展示日志，无法自动推导因果链 | 新增 |
| **GAP-07** | 告警触发后无自动追踪动作 | 告警 → 人工看日志 → 手动排查，未闭环 | MON-OPT-02 相关 |
| **GAP-08** | Chat FlowLog 仍走 SessionBus | 全局 Monitor 连接需双 pump，可能丢失 | MON-Q-01 / MON-OPT-01 |

### 1.3 已有优化方案覆盖情况

| 本方案编号 | 已有方案覆盖 | 说明 |
|------------|-------------|------|
| LOG-01（文件落盘） | ❌ 无 | 全新能力 |
| LOG-02（zap 结构化） | ❌ 无 | 全新能力 |
| LOG-03（路径补全） | 🟡 MON-OPT-01 间接改善 | Bus 分离后 FlowLog 可靠性提升，但不补路径 |
| TRACE-01（Trace 写入） | ✅ MON-OPT-05 | 本方案引用，不重复设计 |
| DIAG-01（诊断包） | 🟡 52-flow-logger §7 | 基础 JSONL 导出已有，需增强 |
| DIAG-02（根因引擎） | ❌ 无 | 全新能力 |
| LOOP-01（闭环工作流） | ❌ 无 | 全新能力 |

---

## 2. 方案设计

### 2.1 总体架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        服务运行时                                        │
│                                                                         │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐            │
│  │  业务代码     │     │  框架层       │     │  系统域       │            │
│  │  TraceEmitter │     │  zap Logger  │     │  SysLog*     │            │
│  └──────┬───────┘     └──────┬───────┘     └──────┬───────┘            │
│         │                    │                    │                     │
│         ▼                    ▼                    ▼                     │
│  ┌──────────────────────────────────────────────────────┐              │
│  │              EventBus（MonitorBus + SessionBus）       │              │
│  └──────────────────────┬───────────────────────────────┘              │
│                         │                                               │
│         ┌───────────────┼───────────────┐                              │
│         ▼               ▼               ▼                              │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐                      │
│  │ WS 推送      │ │ DB 持久化    │ │ 文件 Appender│  ← LOG-01 新增     │
│  │ (现有)       │ │ (现有+增强)  │ │ (新增)       │                      │
│  └─────────────┘ └─────────────┘ └──────┬──────┘                      │
│                                          │                              │
│                                          ▼                              │
│                                 ┌─────────────────┐                    │
│                                 │ JSON Lines 文件  │                    │
│                                 │ /var/log/aranea/ │                    │
│                                 │   flow-*.jsonl   │                    │
│                                 │   system-*.jsonl │                    │
│                                 │   trace-*.jsonl  │                    │
│                                 └────────┬────────┘                    │
│                                          │                              │
└──────────────────────────────────────────┼──────────────────────────────┘
                                           │
                                           ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                        AI 闭环追踪层                                      │
│                                                                          │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐             │
│  │ 1.日志扫描    │────▶│ 2.链路追踪    │────▶│ 3.根因分析    │             │
│  │ LogScanner   │     │ TraceWalker  │     │ RootCause    │             │
│  │              │     │              │     │ Engine       │             │
│  └──────────────┘     └──────────────┘     └──────┬───────┘             │
│                                                    │                    │
│                              ┌─────────────────────┼──────────────┐     │
│                              ▼                     ▼              ▼     │
│                       ┌──────────┐          ┌──────────┐   ┌─────────┐ │
│                       │ 4.诊断包  │          │ 5.修复建议 │   │ 6.验证   │ │
│                       │ DiagPack │          │ FixSuggest│   │ Verify  │ │
│                       └──────────┘          └──────────┘   └─────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
```

### 2.2 设计原则

| 原则 | 说明 |
|------|------|
| **日志即真相源** | 所有运行状态以 JSON Lines 文件为最终持久化形态，DB/WS 为投影 |
| **一条链路一个 trace_id** | 沿用 FlowLog 现有设计，trace_id 贯穿日志、Trace、Usage |
| **AI First 格式** | 日志输出 JSON Lines，每行自描述（含 schema_version），无需额外 schema 文件 |
| **零侵入追加** | 文件 Appender 作为 EventBus 消费者接入，不修改现有 FlowLog/TraceEmitter 代码 |
| **闭环可验证** | 每个闭环步骤有明确的输入/输出契约，可自动化测试 |

---

## 3. LOG-01：FlowLog 文件落盘

### 3.1 目标

将所有 FlowLog + 系统日志持久化到磁盘 JSON Lines 文件，确保进程重启、DB 异常后仍可回溯。

### 3.2 文件布局

```
/var/log/aranea/
├── flow-2026-05-28.jsonl          # 当日 FlowLog（业务域 + 系统域）
├── flow-2026-05-27.jsonl          # 昨日（轮转后）
├── system-2026-05-28.jsonl        # 当日系统域日志（独立文件，高频）
├── trace-2026-05-28.jsonl         # 当日 Trace 完成事件（span 聚合后）
└── alert-2026-05-28.jsonl         # 当日告警事件
```

### 3.3 FlowFileAppender

```go
type FlowFileAppender struct {
    dir        string
    flowFile   *rotatingFile
    systemFile *rotatingFile
    traceFile  *rotatingFile
    alertFile  *rotatingFile
}

type rotatingFile struct {
    mu       sync.Mutex
    path     string
    file     *os.File
    encoder  *json.Encoder
    date     string
    maxSize  int64
}
```

**路由规则**：

| Envelope Type | Channel | 目标文件 |
|---------------|---------|----------|
| `flow_log` | `monitor` | `system-YYYY-MM-DD.jsonl` |
| `flow_log` | 其他（chat/team/...） | `flow-YYYY-MM-DD.jsonl` |
| `alert.fired` / `alert.recovered` / `alert.notify` | 任意 | `alert-YYYY-MM-DD.jsonl` |
| `runner.completion` | 任意 | `trace-YYYY-MM-DD.jsonl` |

### 3.4 文件轮转

| 参数 | 默认值 | 配置项 |
|------|--------|--------|
| 轮转周期 | 每日 | `server.monitor.log_rotation` |
| 单文件最大 | 500 MB | `server.monitor.log_max_size_mb` |
| 保留天数 | 30 天 | `server.monitor.log_retention_days` |
| 压缩 | gzip（>1 天的文件） | `server.monitor.log_compress` |

### 3.5 接入方式

作为 EventBus 消费者，与现有 `flowLogPersistConsumer` 并行：

```go
func newFlowFileAppender(infra *event.Infra, cfg *conf.Monitor) *FlowFileAppender {
    a := &FlowFileAppender{dir: cfg.LogDir}
    infra.MonitorBus().Subscribe(event.SubscribeOptions{
        Channel:   "monitor",
        BufferSize: 4096,
        DropPolicy: event.DropOldest,
        Handler:   a.onEnvelope,
    })
    return a
}
```

### 3.6 验收标准

| 指标 | 目标 |
|------|------|
| FlowLog 写入文件延迟 | < 10 ms（异步） |
| 文件轮转无数据丢失 | ✅ |
| 30 天内任意历史日志可查 | ✅ |
| 磁盘异常时服务不受影响 | ✅ 降级为 SysLogWarn |

---

## 4. LOG-02：框架层 zap 日志结构化

### 4.1 目标

将 `pkg/trpc-agent-go/log` 的 ConsoleEncoder 替换为 JSON Encoder，使框架层日志也可被 AI 解析。

### 4.2 方案

```go
var Default Logger = zap.New(
    zapcore.NewCore(
        zapcore.NewJSONEncoder(jsonEncoderConfig),  // Console → JSON
        zapcore.NewMultiWriteSyncer(
            zapcore.AddSync(os.Stdout),
            zapcore.AddSync(fileSync),              // 同时写文件
        ),
        zapLevel,
    ),
    zap.AddCaller(),
    zap.AddCallerSkip(1),
).Sugar()
```

**JSON 编码器配置**：

```go
jsonEncoderConfig := zap.NewProductionEncoderConfig()
jsonEncoderConfig.TimeKey = "ts"
jsonEncoderConfig.LevelKey = "level"
jsonEncoderConfig.MessageKey = "msg"
jsonEncoderConfig.CallerKey = "caller"
jsonEncoderConfig.StacktraceKey = "stack"
jsonEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
jsonEncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
```

### 4.3 Context 注入关联 ID

扩展 zap Logger，从 context 中提取 `trace_id` / `session_id` / `run_id` 并注入日志字段：

```go
func WithTraceFields(ctx context.Context, logger *zap.SugaredLogger) *zap.SugaredLogger {
    if tc, ok := event.TraceContextFromContext(ctx); ok {
        return logger.With(
            "trace_id", tc.TraceID,
            "session_id", tc.SessionID,
            "run_id", tc.RunID,
        )
    }
    return logger
}
```

### 4.4 输出示例

```json
{"ts":"2026-05-28T10:00:01.123+0800","level":"warn","caller":"trpc-agent-go/agent.go:42","msg":"tool execution timeout","trace_id":"tr_abc123","session_id":"sess_def456","tool":"search","duration_ms":30000}
```

### 4.5 验收标准

| 指标 | 目标 |
|------|------|
| 框架层日志 100% JSON 输出 | ✅ |
| 含 trace_id 的日志占比（Turn 内路径） | ≥ 90% |
| JSON 输出与现有 Console 输出性能差异 | < 5% |

---

## 5. LOG-03：关键路径 FlowLog 补全

### 5.1 目标

将仍使用 slog/zap 的关键业务路径迁移到 FlowLog，确保 AI 可通过 trace_id 追踪完整链路。

### 5.2 需补全的路径

| 路径 | 当前方式 | 迁移目标 | 优先级 |
|------|----------|----------|--------|
| Provider 调用失败/重试 | slog | `provider.call.error` / `provider.call.retry` | P1 |
| Memory 读写（L0-L4） | 部分 FlowLog | 补全 `memory.read.miss` / `memory.write.error` | P1 |
| MCP 连接/调用 | slog | `mcp.session.connect` / `mcp.tool.invoke` | P1 |
| Knowledge 检索失败 | 部分 FlowLog | `knowledge.search.error` / `knowledge.chunk.empty` | P2 |
| Plugin 沙箱执行 | slog | `plugin.sandbox.execute` / `plugin.sandbox.timeout` | P2 |
| Graph 节点执行 | 部分 FlowLog | `graph.node.enter` / `graph.node.error` | P2 |
| Session 状态持久化 | slog | `session.state.persist` / `session.state.restore` | P2 |
| Token 配额检查 | slog | `token.quota.check` / `token.quota.exceeded` | P3 |

### 5.3 补全原则

1. **只补关键路径**：start/done/error 三阶段，skip 可选
2. **复用 TraceEmitter**：从 context 获取，不新建
3. **slog 保留**：非业务路径（如 Kratos 框架内部）保留 slog，通过 LOG-02 结构化
4. **step_id 注册表同步**：每新增 step_id 必须更新 `flow_log.go` 的 `stepTitleRegistry`

### 5.4 验收标准

| 指标 | 目标 |
|------|------|
| P1 路径 100% 覆盖 FlowLog | ✅ |
| 关键错误（provider/memory/mcp）均有 trace_id | ≥ 95% |
| 步骤注册表与实际调用点对齐 | ✅ |

---

## 6. TRACE-01：Trace 写入回路

> 本节引用 [MON-OPT-05](./18-monitor-optimization-2026-05-26.md#5-mon-opt-05trace-写入回路--run-全链路视图) 的设计，不重复。仅补充 AI 闭环所需的接口。

### 6.1 AI 闭环依赖的 Trace 能力

| 能力 | 用途 | MON-OPT-05 覆盖 |
|------|------|-----------------|
| `monitor_traces` 写入 | AI 按 trace_id 查询完整运行 | ✅ MonitorTraceProjector |
| `monitor_trace_spans` 写入 | AI 查看每步耗时和状态 | ✅ span 投影 |
| 跨 turn/跨 team 关联 | AI 追踪跨 Agent 调用链 | ✅ parent_trace_id |
| Trace 文件落盘 | AI 直接读文件，不依赖 DB | ❌ 本方案 TRACE-01 补充 |

### 6.2 Trace 文件落盘

在 `FlowFileAppender` 中增加 Trace 完成事件写入：

```jsonl
{"schema_version":"trace_complete/v1","trace_id":"tr_abc","session_id":"sess_def","run_id":"run_ghi","status":"error","duration_ms":5230,"span_count":5,"error_count":1,"total_tokens":1520,"spans":[{"id":"s1","name":"chat.turn","kind":"root","status":"ok","duration_ms":5230},{"id":"s2","name":"llm.call","kind":"llm","status":"ok","duration_ms":3200},{"id":"s3","name":"tool.search","kind":"tool","status":"error","duration_ms":1500,"error":"timeout"}]}
```

### 6.3 验收标准

| 指标 | 目标 |
|------|------|
| 新 Turn 100% 产生 trace 行 + 文件记录 | ✅ |
| trace-*.jsonl 可被 AI 直接解析 | ✅ |

---

## 7. DIAG-01：AI 诊断包

### 7.1 目标

从一条错误日志出发，自动聚合相关联的所有上下文信息，生成 AI 可直接消费的诊断包。

### 7.2 诊断包结构

```
diagnostic_bundle/
├── manifest.json              # 元数据
├── flow.jsonl                 # 按 trace_id 过滤的 FlowLog 条目
├── trace.json                 # Trace + Spans 完整数据
├── usage.json                 # Token/Cost 用量
├── alerts.jsonl               # 相关告警事件
├── system.jsonl               # 相关系统日志（按时间窗口）
├── config_redacted.json       # 脱敏后的 Agent/Provider 配置快照
└── summary.json               # AI 生成的摘要（可选）
```

### 7.3 manifest.json

```json
{
  "schema_version": "diag_bundle/v1",
  "bundle_id": "db_01J...",
  "created_at": "2026-05-28T10:05:00Z",
  "trigger": {
    "type": "error",
    "source": "flow_log",
    "trace_id": "tr_abc123",
    "session_id": "sess_def456",
    "run_id": "run_ghi789",
    "step_id": "chat.llm.invoke",
    "severity": "error",
    "message": "Provider timeout after 30s",
    "timestamp": "2026-05-28T10:00:05Z"
  },
  "scope": {
    "time_range": ["2026-05-28T09:59:00Z", "2026-05-28T10:05:00Z"],
    "trace_ids": ["tr_abc123"],
    "session_ids": ["sess_def456"],
    "run_ids": ["run_ghi789"]
  },
  "files": {
    "flow.jsonl": { "entries": 23, "size_bytes": 4096 },
    "trace.json": { "spans": 5 },
    "usage.json": { "records": 1 },
    "alerts.jsonl": { "entries": 1 },
    "system.jsonl": { "entries": 8 },
    "config_redacted.json": { "agents": 1, "providers": 1 }
  }
}
```

### 7.4 诊断包生成 API

```protobuf
service MonitorService {
  rpc GenerateDiagnosticBundle(GenerateDiagnosticBundleRequest) returns (GenerateDiagnosticBundleResponse);
}

message GenerateDiagnosticBundleRequest {
  string trigger_type = 1;    // error | alert | manual
  string trace_id = 2;        // 入口关联键
  string session_id = 3;
  string run_id = 4;
  string step_id = 5;
  int32  context_minutes = 6; // 前后时间窗口（默认 5 分钟）
}

message GenerateDiagnosticBundleResponse {
  string bundle_id = 1;
  string download_url = 2;    // 临时下载链接
  string manifest_json = 3;   // 内联 manifest
  int32  total_entries = 4;
}
```

### 7.5 自动触发规则

| 触发条件 | 动作 |
|----------|------|
| FlowLog severity=critical | 自动生成诊断包 + 写入 `alert-*.jsonl` |
| 告警规则 firing | 自动生成诊断包 + 附加到告警通知 |
| 用户在 Monitor 页面点击「诊断」 | 手动触发，可指定 trace_id |
| API 调用 `GenerateDiagnosticBundle` | 外部 AI 工具触发 |

### 7.6 验收标准

| 指标 | 目标 |
|------|------|
| 从一条 critical FlowLog 到诊断包生成 | < 5 s |
| 诊断包包含完整 trace 链路 | ≥ 95% |
| 诊断包可被 GPT-4/Claude 直接解析 | ✅ |
| 诊断包大小 | < 1 MB（单次运行） |

---

## 8. DIAG-02：根因分析规则引擎

### 8.1 目标

基于诊断包中的结构化数据，自动推导错误根因，减少 AI 的推理负担。

### 8.2 规则模型

```go
type RootCauseRule struct {
    ID          string
    Name        string
    Description string
    Condition   RootCauseCondition
    RootCause   string
    FixSuggest  string
    Severity    string
}

type RootCauseCondition struct {
    StepID      string            // 匹配的 step_id
    Phase       string            // error / critical
    ErrorCodes  []string          // 匹配的 error.code
    Pattern     string            // 正则匹配 error.message
    Prerequisites []Prerequisite  // 前置条件（增强准确率）
}

type Prerequisite struct {
    StepID   string
    Phase    string
    Severity string
    Pattern  string
}
```

### 8.3 内置规则

| 规则 ID | 匹配条件 | 根因 | 修复建议 |
|---------|----------|------|----------|
| `RC-001` | step=`chat.llm.invoke`, phase=error, pattern=`timeout` | Provider 响应超时 | 1. 检查 Provider 状态 2. 增大超时 3. 切换 Provider |
| `RC-002` | step=`chat.first_byte_timeout`, phase=error | 模型首 Token 延迟过高 | 1. 检查网络 2. 切换更快的模型 3. 减小 max_tokens |
| `RC-003` | step=`chat.turn.empty_reply`, phase=error | 模型返回空响应 | 1. 检查 prompt 长度 2. 检查 content filter 3. 重试 |
| `RC-004` | step=`provider.call.error`, pattern=`429\|rate_limit` | Provider 限流 | 1. 降低并发 2. 启用多 Provider 轮换 3. 检查配额 |
| `RC-005` | step=`provider.call.error`, pattern=`401\|invalid_api_key` | API Key 无效 | 1. 检查 Provider 配置 2. 更新 API Key |
| `RC-006` | step=`knowledge.search.error` | 知识库检索失败 | 1. 检查 Embedding 服务 2. 检查索引状态 3. 重建索引 |
| `RC-007` | step=`mcp.tool.invoke`, phase=error | MCP 工具调用失败 | 1. 检查 MCP 服务状态 2. 检查工具参数 3. 重连 MCP |
| `RC-008` | step=`memory.write.error` | 记忆写入失败 | 1. 检查 DB 连接 2. 检查存储空间 3. 检查 schema |
| `RC-009` | step=`chat.turn.timeout` | Turn 整体超时 | 1. 检查是否有死循环工具调用 2. 增大 turn 超时 3. 检查 Agent 配置 |
| `RC-010` | step=`system.bus.drop`, pattern=`flow_log` | FlowLog 被丢弃 | 1. 检查 Bus buffer 配置 2. 检查消费者处理速度 |
| `RC-011` | step=`plugin.sandbox.timeout` | 插件沙箱超时 | 1. 优化插件逻辑 2. 增大超时 3. 检查资源限制 |
| `RC-012` | step=`token.quota.exceeded` | Token 配额耗尽 | 1. 充值/提升配额 2. 优化 prompt 3. 启用缓存 |

### 8.4 规则评估

```go
func (e *RootCauseEngine) Evaluate(bundle *DiagnosticBundle) []RootCauseResult {
    var results []RootCauseResult
    for _, entry := range bundle.FlowLogEntries {
        if entry.Severity != "error" && entry.Severity != "critical" {
            continue
        }
        for _, rule := range e.rules {
            if matchRule(rule, entry, bundle) {
                results = append(results, RootCauseResult{
                    Rule:      rule,
                    Entry:     entry,
                    Confidence: calcConfidence(rule, bundle),
                })
            }
        }
    }
    sort.Slice(results, func(i, j int) bool {
        return results[i].Confidence > results[j].Confidence
    })
    return results
}
```

### 8.5 置信度计算

| 因素 | 权重 | 说明 |
|------|------|------|
| 规则直接匹配 | 0.4 | step_id + error pattern 完全匹配 |
| 前置条件满足 | 0.3 | Prerequisites 全部满足 |
| 时间关联性 | 0.2 | 错误发生在相关步骤之后 |
| 频率关联性 | 0.1 | 同类错误在近期重复出现 |

### 8.6 验收标准

| 指标 | 目标 |
|------|------|
| 内置规则覆盖 Top 12 常见错误 | ✅ |
| 根因命中率（人工标注） | ≥ 80% |
| 规则评估延迟 | < 100 ms |
| 新增规则无需改代码 | ✅（配置驱动） |

---

## 9. LOOP-01：闭环工作流

### 9.1 目标

将「发现 → 追踪 → 定位 → 修复 → 验证」串联为自动化工作流，AI 可端到端执行。

### 9.2 闭环状态机

```
detected ──[auto/manual]──▶ tracing ──[bundle ready]──▶ analyzing ──[root cause found]──▶ suggested
    │                          │                         │                              │
    │                          │                         │                              ▼
    │                          │                         │                         fixing ──[fix applied]──▶ verifying
    │                          │                         │                                              │
    │                          │                         │                              ┌───────────────┘
    │                          │                         │                              ▼
    │                          │                         │                         verified ──[pass]──▶ closed
    │                          │                         │                              │
    └──────────────────────────┴─────────────────────────┴──────────────────────────────┘
                                                                                          │
                                                                                    [fail] └──▶ reopened
```

### 9.3 闭环事件

| 事件 | 触发 | 数据 |
|------|------|------|
| `loop.detected` | FlowLog critical / Alert firing | trace_id, severity, step_id |
| `loop.tracing` | 自动/手动触发诊断包生成 | bundle_id |
| `loop.analyzed` | 根因引擎完成评估 | root_cause_id, confidence, fix_suggest |
| `loop.fix_suggested` | AI 生成修复建议 | fix_actions[] |
| `loop.fix_applied` | 人工/AI 执行修复 | fix_result |
| `loop.verifying` | 修复后自动验证 | verify_plan |
| `loop.verified` | 验证通过 | verify_result |
| `loop.closed` | 闭环完成 | summary |
| `loop.reopened` | 验证失败 | fail_reason |

### 9.4 验证策略

| 验证类型 | 说明 | 示例 |
|----------|------|------|
| **重放验证** | 用相同输入重试失败步骤 | 重新调用 Provider 检查是否恢复 |
| **指标验证** | 检查相关指标是否恢复正常 | error_rate < threshold |
| **日志验证** | 检查后续日志是否无同类错误 | 5 分钟内无同 step_id error |
| **功能验证** | 执行健康检查端点 | `GET /healthz` 返回 200 |

### 9.5 闭环记录

每次闭环产生一条 `loop_record`，持久化到 `monitor_events`：

```json
{
  "event_key": "loop.closed",
  "status": "ok",
  "metadata_json": {
    "loop_id": "lp_01J...",
    "trigger_trace_id": "tr_abc123",
    "trigger_step_id": "chat.llm.invoke",
    "root_cause_rule": "RC-001",
    "confidence": 0.85,
    "fix_actions": ["增大 Provider 超时至 60s"],
    "verify_result": "pass",
    "duration_ms": 125000,
    "total_entries_analyzed": 23
  }
}
```

### 9.6 AI Prompt 模板

AI 在执行闭环时的系统 Prompt 模板：

```markdown
## 角色
你是 Aranea 平台的运维 AI 助手，负责根据日志诊断和修复服务问题。

## 输入
- 诊断包 manifest.json
- flow.jsonl（按时间排序的 FlowLog 条目）
- trace.json（Span 树）
- 根因分析结果（规则 ID + 置信度）

## 工作流
1. 阅读 manifest.json 了解问题概要
2. 扫描 flow.jsonl 中 severity=error/critical 的条目
3. 根据 trace_id 追踪完整调用链
4. 对照根因分析结果，确认或修正根因
5. 给出具体修复建议（含操作步骤）
6. 建议验证方案

## 输出格式
```json
{
  "root_cause": "...",
  "confidence": 0.0-1.0,
  "evidence": ["step_id:xxx -> ...", "..."],
  "fix_suggestions": [
    {"action": "...", "priority": "high/medium/low", "steps": ["..."]}
  ],
  "verify_plan": {"type": "metric|replay|log|functional", "params": {...}}
}
```

## 注意
- 不要猜测，基于日志证据推导
- 如果证据不足，明确说明需要哪些额外信息
- 修复建议必须是可执行的操作，不要模糊描述
- 敏感信息（API Key、Token）不要出现在输出中
```

### 9.7 验收标准

| 指标 | 目标 |
|------|------|
| 从 critical 错误到诊断包生成 | < 10 s |
| 从诊断包到根因分析完成 | < 5 s |
| 闭环记录 100% 写入 monitor_events | ✅ |
| 闭环记录可通过 Monitor Events Tab 查看 | ✅ |

---

## 10. 配置汇总

### 10.1 新增配置项

```yaml
server:
  monitor:
    log_dir: "/var/log/aranea"           # 日志文件目录
    log_rotation: "daily"                # daily | hourly | size
    log_max_size_mb: 500                 # 单文件最大 MB
    log_retention_days: 30               # 保留天数
    log_compress: true                   # 轮转后 gzip 压缩
    log_file_enabled: true               # 文件落盘开关
    diagnostic_auto_trigger: true        # critical 时自动生成诊断包
    diagnostic_context_minutes: 5        # 诊断包时间窗口
    root_cause_engine_enabled: true      # 根因引擎开关
    loop_workflow_enabled: true          # 闭环工作流开关
```

### 10.2 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `ARANEA_LOG_DIR` | `/var/log/aranea` | 日志目录（覆盖 config.yaml） |
| `ARANEA_LOG_FILE_ENABLED` | `true` | 文件落盘开关 |
| `ARANEA_DIAG_AUTO_TRIGGER` | `true` | 自动诊断触发 |
| `ARANEA_ROOT_CAUSE_ENABLED` | `true` | 根因引擎开关 |

---

## 11. 与已有优化方案的关系

| 已有方案 | 本方案关系 | 冲突 |
|----------|-----------|------|
| MON-OPT-01（Bus 分离） | **前置依赖**：FlowFileAppender 订阅 MonitorBus，需 Bus 分离完成 | 无 |
| MON-OPT-02（告警冷却持久化） | **协作**：闭环工作流的 `loop.detected` 可由 alert.fired 触发 | 无 |
| MON-OPT-03（告警批量化） | **协作**：RingBuffer 数据可被根因引擎复用 | 无 |
| MON-OPT-04（WS 反压） | **独立**：文件 Appender 不走 WS，不受反压影响 | 无 |
| MON-OPT-05（Trace 写入） | **前置依赖**：诊断包依赖 trace 数据 | 无 |
| MON-OPT-06（规则注册表） | **协作**：根因规则可注册到 AlertMetricRegistry | 无 |
| 52-flow-logger Phase 2 | **前置依赖**：FlowLog 落库是文件落盘的基础 | 无 |

### 建议实施顺序

```
MON-OPT-01 (Bus 分离)
    │
    ├──▶ LOG-01 (文件落盘)
    │       │
    │       └──▶ LOG-02 (zap 结构化)
    │
    ├──▶ MON-OPT-05 (Trace 写入)
    │       │
    │       └──▶ TRACE-01 (Trace 文件落盘)
    │
    ├──▶ LOG-03 (路径补全)
    │
    └──▶ DIAG-01 (诊断包)
            │
            ├──▶ DIAG-02 (根因引擎)
            │
            └──▶ LOOP-01 (闭环工作流)
```

---

## 12. 排期建议

| 阶段 | 内容 | 依赖 |
|------|------|------|
| **Phase A** | LOG-01 文件落盘 + LOG-02 zap 结构化 | MON-OPT-01 |
| **Phase B** | LOG-03 路径补全（P1 部分）+ TRACE-01 Trace 文件落盘 | MON-OPT-05 |
| **Phase C** | DIAG-01 诊断包生成 API + 自动触发 | Phase A + B |
| **Phase D** | DIAG-02 根因引擎（12 条内置规则）+ LOG-03 P2 部分 | Phase C |
| **Phase E** | LOOP-01 闭环工作流 + AI Prompt 模板 | Phase D |

---

## 13. 不在本方案范围

| 项 | 理由 |
|----|------|
| 日志搜索服务（ELK/Loki） | 本方案聚焦文件落盘 + AI 直接消费，不引入额外基础设施 |
| 实时流式 AI 分析 | 当前为按需生成诊断包，实时分析需更大架构变更 |
| 自动修复执行 | 闭环到「修复建议」为止，自动执行修复需人工审批 |
| 多语言日志 | 当前日志以中文 title 为主，AI Prompt 为英文，暂不调整 |
| 前端闭环 UI | 本方案聚焦后端能力，前端展示在后续迭代规划 |

---

## 14. 关联文档

- 业务需求：[`18 monitor.md`](./18%20monitor.md)
- 设计文档：[`18 monitor.design.md`](./18%20monitor.design.md)
- 优化方案：[`18-monitor-optimization-2026-05-26.md`](./18-monitor-optimization-2026-05-26.md)
- FlowLogger 设计：[`52-flow-logger.design.md`](./52-flow-logger.design.md)
- 代码 Review：[`2026-05-26-Monitor-Code-Review.md`](../review/2026-05-26-Monitor-Code-Review.md)
- 历史 Review：[`18-monitor-review.md`](../review/18-monitor-review.md)
- Bus 分流规则：[`monitor-streams-wire.mdc`](../../.cursor/rules/monitor-streams-wire.mdc)
