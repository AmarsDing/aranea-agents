# 日志体系统一迁移方案

> 日期：2026-05-31
> 状态：P0 ✅ | P1 ✅ | P2 ✅ | P3 待实施

---

## 一、背景

项目存在三套独立日志体系，互不关联，排查问题需要同时看多个输出：

| 体系 | 输出 | 文件持久化 | 前端可见 |
|------|------|-----------|---------|
| FlowLog (`internal/event`) | EventBus → WS + FlowFileAppender | ✅ JSONL | ✅ |
| Kratos 框架日志 | `os.Stdout` only | ❌ | ❌ |
| trpc-agent-go Zap 日志 | `os.Stdout` only | ❌ | ❌ |

**核心问题**：
1. Kratos/Zap 日志无文件输出，stdout 滚走即丢
2. 三套日志无法通过 trace_id 关联
3. 日志配置碎片化（6 个配置点、4 种控制方式）
4. EventBus 故障时 FlowLog 丢失（日志不应比业务更不可靠）

---

## 二、目标

1. **Zap 统一底座**：所有日志汇聚到一个 Zap Core，统一输出目标、格式、轮转
2. **统一日志接口**：代码中只有 `loggateway.Logger` 的 `Debug/Info/Warn/Error/With`
3. **前端可见**：开关打开时日志全量同步到前端，前端本地缓存筛选
4. **可靠持久化**：日志先写 Zap 文件，不依赖 EventBus
5. **Usage 追踪独立**：TraceEmitter 的 span 追踪是领域概念，不归 Logger 管

---

## 三、架构

```
代码调用层（统一接口）
═══════════════════════════════════════════════════════════
  lg.Info("msg", StepID("..."), SessionID("..."))
  lg.Warn("msg", Err(err))
  step := lg.BeginStep("chat.llm.invoke", "调用语言模型")
  step.Done("模型调用完成")
═══════════════════════════════════════════════════════════
         │
         ▼
Zap Core（统一底座）
═══════════════════════════════════════════════════════════
  ├── JSON Encoder
  ├── lumberjack.Writer → aranea-*.log  (全量，所有级别，轮转)
  ├── os.Stdout                         (可配开关)
  └── BusHook (级别阈值可配)
       └── ≥ hookLevel → EnvelopeTypeLog → MonitorBus → WS → 前端
═══════════════════════════════════════════════════════════
         │                           │
    文件持久化                    MonitorBus (已有)
    (可靠，始终写)              (实时，开关控制)
         │                           │
         ▼                           ▼
  aranea-*.log              ┌──────────────────────┐
  (lumberjack 轮转)         │  WS (共享连接)        │
                            │  low 队列: flow_log   │
                            │  low 队列: log ← 新增 │
                            │  enable_log 开关 🔘   │
                            └──────────┬───────────┘
                                       │
                            ┌──────────▼───────────┐
                            │  前端 UI              │
                            │  RingBuffer (5000条)  │
                            │  本地筛选/搜索        │
                            └──────────────────────┘

UsageTracker（独立，不归 Logger 管）
═══════════════════════════════════════════════════════════
  tracker.ObserveFrameworkEvent(ev)
  tracker.CompleteToolCall(id, name, dur, status)
  tracker.MetadataJSON() → usage.metadata_json
═══════════════════════════════════════════════════════════
```

---

## 四、统一接口设计

### Logger 接口

```go
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    With(fields ...Field) Logger
}

type Field = zap.Field
```

### 便捷字段构造

```go
func StepID(v string) Field        { return zap.String("step_id", v) }
func SessionID(v string) Field     { return zap.String("session_id", v) }
func TraceID(v string) Field       { return zap.String("trace_id", v) }
func RunID(v string) Field         { return zap.String("run_id", v) }
func Domain(v string) Field        { return zap.String("domain", v) }
func AgentKey(v string) Field      { return zap.String("agent_key", v) }
func Phase(v string) Field         { return zap.String("phase", v) }
func Duration(ms int64) Field      { return zap.Int64("duration_ms", ms) }
func Source(v string) Field        { return zap.String("source", v) }
func Err(v error) Field            { return zap.Error(v) }
func Str(k, v string) Field        { return zap.String(k, v) }
func Int(k string, v int) Field    { return zap.Int(k, v) }
func Any(k string, v interface{}) Field { return zap.Any(k, v) }
```

### BeginStep/Done（替代 TraceEmitter 的 LogStart/LogDone）

```go
step := lg.BeginStep("chat.llm.invoke", "调用语言模型")
// ... LLM 调用 ...
step.Done("模型调用完成")
// 或
step.Error("模型调用超时", loggateway.Err(err))
```

---

## 五、迁移计划

### P0：基础设施（不改任何调用点）

| 步骤 | 内容 | 影响文件 | 状态 |
|------|------|---------|------|
| P0-1 | `conf.proto` 增加 `Logging` message + `make api` | 1 文件 + 生成物 | ✅ |
| P0-2 | 新建 `pkg/loggateway` — Logger 接口 + Zap Core + lumberjack + BusHook + KratosAdapter + BeginStep | 5 新文件 | ✅ |
| P0-3 | 修改 `cmd/admin/main.go` — loggateway 初始化替换 `log.NewStdLogger(os.Stdout)` | 1 文件 | ✅ |
| P0-4 | 修改 `pkg/trpc-agent-go/log/log.go` — SetOutput 支持 | main.go 中替换全局 logger | ✅ |
| P0-5 | 修改 `ws.go` enable_log — 增加 level 参数 + SetHookLevel | 1 文件 | ✅ |
| P0-6 | 修改 FlowFileAppender — EnvelopeTypeLog 订阅 + log-*.jsonl | 1 文件 | ✅ |
| P0-7 | 修改 `configs/config.yaml` — 增加 logging 段 | 1 文件 | ✅ |
| P0-8 | 修复 `graph_task_runtime.go` log.DefaultLogger 泄漏 | 1 文件 | ✅ |

**P0 完成后效果**：Kratos 框架日志 + trpc-agent-go Zap 日志 → Zap 文件 + BusHook → 前端可见。旧接口全部保留，零迁移。

### P1：迁移 Kratos log.NewHelper（78 处）

- 范围：`internal/cronrunner/jobs/` + `internal/service/` 中的 `log.NewHelper`
- 方式：替换为 `loggateway.Logger`，旧接口标记 `// Deprecated`

### P2：迁移 FlowLog SysLog*（262 处） ✅

- 范围：`event.SysLog*` / `event.SessionSysLog*` 全部调用点
- 方式：替换为 `loggateway.Logger`，保留 step_id 语义
- 结果：220+ 处调用已迁移，`event.SysLog*` / `event.SessionSysLog*` 调用归零
- 涉及层级：biz / service / agent / session / channel / graph / plugin / cronrunner / knowledge / skill / data / tools / memory / team / mcp / artifact / runtime
- Wire 重新生成：`make wire` 已执行，`wire_gen.go` 已更新
- 结构体新增 `lg loggateway.Logger` 字段：20+ 个核心结构体（Usecase / SessionUsecase / AgentUsecase / ChannelIngress / ChatOrchestrator / WSServer / Manager / TraceProjector / Embedder / Runner 等）
- 独立函数使用 `loggateway.Global()` 或 `loggateway.NewNoop()`
- 注意：SysLog* 内部的节流逻辑迁移到 BusHook

### P3：迁移 CtxFlowLog* + TraceEmitter（154 处）

- 范围：`event.CtxFlowLog*` + `TraceEmitter.LogStart/LogDone/LogError` 等
- 方式：`With()` 替代 TraceContext 绑定，`BeginStep/Done` 替代 LogStart/LogDone
- TraceEmitter 的 Usage 追踪功能保留为 `UsageTracker`

### 前端：日志 Monitor UI

- 新建 `logMonitor` Store — RingBuffer 5000 条 + 筛选
- 扩展 `enable_log` 命令 — 增加 level 参数
- 日志面板 UI — 级别/来源/搜索筛选

---

## 六、配置

### conf.proto

```protobuf
message Logging {
  string level = 1;           // debug/info/warn/error. Default: info
  string output_dir = 2;      // 日志文件目录. Default: /var/log/aranea
  int32 max_size_mb = 3;      // 单文件最大 MB. Default: 100
  int32 max_backups = 4;      // 保留轮转文件数. Default: 10
  int32 max_age_days = 5;     // 保留天数. Default: 30
  bool compress = 6;          // 压缩旧文件. Default: true
  bool stdout_enabled = 7;    // 同时输出 stdout. Default: true
  string hook_level = 8;      // BusHook 转发级别. Default: info
}

message Bootstrap {
  Server server = 1;
  Data data = 2;
  Logging logging = 3;
}
```

### config.yaml

```yaml
logging:
  level: info
  output_dir: ""
  max_size_mb: 100
  max_backups: 10
  max_age_days: 30
  compress: true
  stdout_enabled: true
  hook_level: info
```

---

## 七、文件布局

```
/var/log/aranea/                          ← Logging.output_dir
├── aranea-2026-05-31.log                 ← Zap 统一日志（JSON，所有来源）
├── aranea-2026-05-31.log.1               ← lumberjack 轮转
├── aranea-2026-05-31.log.2.gz            ← 压缩备份
├── flow-2026-05-31.jsonl                 ← FlowLog 业务事件归档（已有）
├── system-2026-05-31.jsonl               ← FlowLog 系统域归档（已有）
├── log-2026-05-31.jsonl                  ← 框架日志归档（新增）
├── trace-2026-05-31.jsonl                ← Runner 完成追踪（已有）
└── alert-2026-05-31.jsonl                ← 告警归档（已有）
```

### 统一日志文件格式

```json
{"ts":"2026-05-31T10:15:30.123Z","level":"info","logger":"kratos","caller":"server/http.go:58","msg":"request","method":"/v1/chat/send","took":"1.2s","trace_id":"tr_abc"}
{"ts":"2026-05-31T10:15:30.456Z","level":"warn","logger":"trpc-agent","caller":"runner/runner.go:979","msg":"panic in runner event loop","error":"..."}
{"ts":"2026-05-31T10:15:30.789Z","level":"error","logger":"flow","step_id":"chat.llm.invoke","phase":"error","session_id":"s_123","trace_id":"tr_abc","title":"调用语言模型","message":"模型调用超时","duration_ms":5000}
```

三个来源通过 `logger` 字段区分，通过 `trace_id` 关联。

---

## 八、WS 承载分析

### 当前 WS 保护机制

| 机制 | 说明 |
|------|------|
| 双总线 | SessionBus（业务）+ MonitorBus（监控），物理隔离 |
| 三级队列 | high(64) / normal(128) / low(256)，日志走 low |
| DropNewest | low/normal 队列满时丢弃新消息，不影响业务 |
| wsLowDrainPerLoop=8 | 每次循环最多排 8 条 low，防止饿死 high/normal |
| backpressure 通知 | 每 10s 报告丢弃数量 |
| logEnabled 开关 | 前端控制是否接收 EnvelopeTypeLog |

### 流量估算

| 场景 | 日志条数/秒 | WS 流量 | low 队列消耗 |
|------|-----------|---------|-------------|
| 单用户 info | ~5 条/s | ~1.5KB/s | 2%/s |
| 5 用户 info | ~25 条/s | ~7.5KB/s | 10%/s |
| 10 用户 debug | ~200 条/s | ~100KB/s | 78%/s ⚠️ |

**结论**：info 级别完全没问题；debug 级别有保护机制（DropNewest + backpressure），不影响业务。

---

## 九、设计决策记录

| 决策 | 选择 | 理由 |
|------|------|------|
| 日志底座 | Zap | 已在依赖树，性能最优，原生支持 MultiWriteSyncer |
| 文件轮转 | lumberjack | Go 生态标准，零风险 |
| 日志与事件 | 双写（Zap 文件 + EventBus） | Zap 保证可靠，EventBus 保证实时 |
| WS 独立连接 | 不需要 | 三级队列 + 双总线已提供逻辑隔离 |
| 前端缓存 | RingBuffer 5000 条 | O(1) 写入，~1.5MB 内存 |
| Usage 追踪 | 独立 UsageTracker | 领域概念，不是日志 |
| BusHook 级别 | 可配，默认 info | 防止 debug 洪泛 WS |
| 迁移策略 | 4 期渐进 | P0 不改调用点，P1-P3 逐批替换 |
