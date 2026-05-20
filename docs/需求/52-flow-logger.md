# FlowLogger 流程日志 — 产品需求

> **版本**：2026-05-20 | **状态**：v2 实施中（Phase 1a 已合入）  
> **设计**：[52-flow-logger.design.md](./52-flow-logger.design.md)  
> **开发计划**：[52-flow-logger-development.md](./52-flow-logger-development.md)  
> **步骤注册表**：[flow-log-step-registry.md](../guides/flow-log-step-registry.md)  
> **背景**：[changelog FlowLogger 初版](../changelog/2026-05-20-Agent-No-Response-Debug-And-FlowLogger.md)

---

## 1. 背景与问题

当前流程日志已用于 Chat Turn 排障，但 v1 存在局限：

| 问题 | 影响 |
|------|------|
| 仅按 `session_id` + `agent_key` 关联 | 无法按 **一次用户请求 / run** 聚合 |
| `flow_step` 扁平、无域分层 | 难以快速理解「卡在哪一层」 |
| 严重级别与 `phase` 混用 | 前端无法稳定映射 **红 / 黄 / 绿** |
| 文案偏开发向（`chat.llm_call.done`） | 业务用户看不懂 |
| 与 OTel Trace / Usage Span 割裂 | Monitor 多处数据对不齐 |
| v1 用 `EnvelopeTypeLog` 与进程日志混流 | Logs Tab 无法区分业务 Flow |

**目标**：建设 **框架友好、人类可读、AI 可解析** 的流程日志体系，支撑「按链路查日志」与「按严重级别一眼识别问题」。

---

## 2. 与 Tracing 的关系

项目中存在三条易混淆的「跟踪」能力：

| 能力 | 代码锚点 | 主要输出 | 典型 UI |
|------|----------|----------|---------|
| **OTel Tracing** | `internal/service/turn_trace.go` | OTLP → Jaeger | Jaeger UI |
| **Span 投影** | `TraceEmitter` → `metadata_json.spans` | 耗时结构 | Monitor **Traces 瀑布图** |
| **FlowLog** | `TraceEmitter` → WS `flow_log` | 业务语义 + severity | Monitor **Logs**（流程）/ Traces 详情「流程」 |

### 2.1 本质区别

| 维度 | Tracing（OTel + Span） | FlowLogger（流程日志） |
|------|------------------------|------------------------|
| 核心问题 | 哪段花了多久、调用关系 | 业务发生了什么、是否出错、怎么办 |
| 数据结构 | Span：name、parent、duration | FlowLog：step、**severity**、**title**、**message**、**hint** |
| 文案 | 偏技术 | 偏业务中文 |
| 实时推送 | OTel 默认不走 Monitor WS | Monitor WS `flow_log` |
| AI 排障 | 需理解 Span | JSONL + hint，开箱即用 |

**结论**：Tracing **不能单独替代** FlowLogger；Monitor 业务日志以 **FlowLog 为主**，瀑布图读 **同源 Span**。

### 2.2 推荐架构（v2）

```text
业务代码一次打点 → TraceEmitter
  ├─► FlowLog   → WS flow_log + Monitor Logs（主输出）
  ├─► Span      → usage.metadata_json.spans（瀑布图）
  └─► OTel      → Jaeger（可选，运维）
```

| Monitor 子模块 | 定位 | 数据来源 |
|----------------|------|----------|
| **Logs Tab** | 实时流程日志（中文）+ 可选进程日志 | `flow_log`（默认）/ `log`（需开启） |
| **Traces 详情** | 同链路耗时 + 流程时间线 | Span + FlowLog（同源 `trace_id`） |
| **Logs（进程）** | Gateway 文本 | `enable_log`，非业务 Trace |

**对用户**：选一个 `trace_id`，可同时看流程时间线（红/黄/绿）与瀑布图耗时。

**对 AI**：导出 **Flow Diagnostic Bundle**（JSONL），不以 Jaeger 导出为主。

---

## 3. 用户与场景

| 角色 | 场景 |
|------|------|
| 业务用户 | 聊天无响应时，在 Monitor Logs 或会话侧栏查看「走到哪一步、哪步报错」 |
| 运维 / 管理员 | 按 `trace_id` / `run_id` 过滤完整链路 |
| AI 助手 | 根据 `severity` + `step` + `hint` 定位根因 |
| 开发者 | Turn 用 `TraceEmitter`；基础设施用 `SysLog*` / `CtxFlowLog*`；**禁止 slog**（已移除 SlogBridge） |

---

## 4. 功能需求

### 4.1 链路（跟踪上下文）

日志须挂载在 **TraceContext** 上，至少包含：

| 字段 | 说明 |
|------|------|
| `trace_id` | 全链路 ID（对齐 OTel TraceID，无则 `tr_`+uuid） |
| `session_id` | 会话 |
| `run_id` | Chat Turn / team_run 主键 |
| `domain` | `chat` / `team` / `graph` / `knowledge` / `plugin` / `system` |
| `agent_key` / `agent_id` | 主体 Agent |

**验收**：同一 `trace_id` 下日志时间有序；可按 session / run / trace 过滤。

### 4.2 步骤（Step）

- 命名：`{domain}.{subsystem}.{action}`，登记于 [flow-log-step-registry.md](../guides/flow-log-step-registry.md)。
- **禁止**随意字符串。

### 4.3 严重级别（severity）与展示色

与 `phase`（start/done/skip/error）**解耦**：

| severity | 含义 | 前端色 |
|----------|------|--------|
| `ok` | 正常完成 | 绿 |
| `info` | 信息 | 绿（弱化） |
| `warn` | 需关注 | 黄 |
| `error` | 失败可恢复 | 红 |
| `critical` | 严重故障 | 深红 |

**验收**：Monitor 按 `severity` 着色，不依赖解析文案。

### 4.4 人类可读文案

每条日志含 `title`（≤40 字）、`message`、`hint`（可选）。

**验收**：非开发人员读 `title` + `message` 即可理解；禁止仅展示 step 编码。

### 4.5 AI 可解析结构

- `schema_version`: `flow_log/v1`
- `correlation`、`timing`、`error` 字段稳定
- 支持导出 JSON Lines

### 4.6 前端查看能力

| 能力 | 说明 |
|------|------|
| **Logs Tab** | 实时接收 `flow_log`，展示中文流程日志；进程 `log` 单独开关 |
| **Traces 详情「流程」Tab** | 与瀑布图同页，按 `run_id` / `trace_id` 过滤（Phase 1b） |
| 过滤 | trace_id / session / run / severity / domain |
| 关联跳转 | 从 Chat、Trace 详情跳入同一 trace |

**约束**：**不新增** Monitor 第 7 个顶层 Tab。

### 4.7 架构约束

- 实现位于 `internal/event`；**禁止** `internal/biz` import `pkg/trpc-agent-go`。
- Turn 热路径 **禁止 slog**；统一 `TraceEmitter`。
- 与 [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md) 一致。

### 4.8 v2 传输（不保留 v1）

- **仅**发布 `EnvelopeTypeFlowLog`（`channel=monitor`）。
- **不再**用 `EnvelopeTypeLog` + `metadata.flow_step` 表示流程日志。

---

## 5. 非功能需求

| 项 | 要求 |
|----|------|
| 性能 | 单 Turn 默认最多 500 条；异步 `bus.Publish` |
| 存储 | Phase 2 可选 `flow_log_events` 表 |
| 保留 | 默认 7 天（与 monitor 策略一致） |
| 安全 | 不落库完整 Prompt；敏感字段脱敏 |
| WS | `flow_log` 无需 `enable_log`；进程 `log` 仍需开启 |

---

## 6. 验收标准

- [x] Chat Turn 通过 `TraceEmitter` 发布 `flow_log`
- [x] Monitor Logs Tab 实时展示中文流程日志（`flow_log`）
- [x] 与 Usage `metadata.spans` 同源、`trace_id` 一致
- [x] Traces 详情「流程」Tab + JSONL 导出（Phase 1b）
- [ ] 单次 Turn 可按 `trace_id` HTTP 查询历史（Phase 2）
- [ ] 业务用户无需懂 step_id 即可理解全流程

---

## 7. 不在范围（v2.0）

- 替代 Prometheus / OTel 指标
- 替代 Gateway 进程 Logs（`LogStream` 进程视图保留）
- 用 Jaeger 单独替代 Monitor 内流程日志
- 跨集群 ELK 聚合（后续 Observability 迭代）
- 保留 v1 `EnvelopeTypeLog` 流程双写
