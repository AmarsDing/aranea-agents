# FlowLogger 流程日志 — 产品需求

> **版本**：2026-05-21 | **状态**：✅ v2 Phase 1a/1b/1c/3 已落地；Phase 2 落库可选  
> **设计**：[52-flow-logger.design.md](./52-flow-logger.design.md)  
> **开发计划**：[52-flow-logger-development.md](./52-flow-logger-development.md)（**实现差距以开发计划为准**）  
> **关联**：[51 消息机制](./51%20消息机制.md) · [51a 后端](./51a%20后端消息机制.md) · [changelog DocSync](../changelog/2026-05-21-Message-FlowLogger-DocSync.md)  
> **背景**：[changelog FlowLogger 初版](../changelog/2026-05-20-Agent-No-Response-Debug-And-FlowLogger.md) · [Slog 移除](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md)

---

## 1. 背景与问题

v1 `FlowLogger`（`internal/event/flow_logger.go`，已删除）曾用于 Chat Turn 排障，存在局限；**v2 已由 `TraceEmitter` + `EnvelopeTypeFlowLog` 替代**（见开发计划）。历史局限如下：


| 问题                               | 影响                                   |
| -------------------------------- | ------------------------------------ |
| 仅按 `session_id` + `agent_key` 关联 | 无法按 **一次用户请求 / run / team_run** 聚合查看 |
| `flow_step` 扁平、无域分层              | 用户与 AI 难以快速理解「卡在哪一层」                 |
| 严重级别与 `phase` 混用                 | 前端无法稳定映射 **红 / 黄 / 绿**               |
| 文案偏开发向（`chat.llm_call.start`）    | 业务用户看不懂                              |
| 无持久化查询 API                       | 刷新后丢失，无法按链路回放                        |
| 与 OTel Trace / Usage Span 割裂     | Monitor 三处数据对不齐                      |


**目标**：建设 **框架友好、人类可读、AI 可解析** 的流程日志体系，支撑「按链路查日志」与「按严重级别一眼识别问题」。

---

## 2. 与 Tracing 的关系（能否用 Tracing 作为 Monitor 日志输出？）

项目中目前存在 **三条「跟踪」相关能力**，容易混淆：


| 能力                     | 代码锚点                                                            | 主要输出                                           | 面向谁        | 典型 UI                  |
| ---------------------- | --------------------------------------------------------------- | ---------------------------------------------- | ---------- | ---------------------- |
| **OTel Tracing**       | `internal/service/turn_trace.go`、`internal/server/telemetry.go` | OTLP → Jaeger/Tempo                            | SRE、跨服务排障  | Jaeger UI              |
| **Turn Span（业务 Span）** | `TraceEmitter.MetadataJSON()` → `recordTurnUsage`（`turn_spans.go` 已删） | `model_token_usage_events.metadata_json.spans` | 产品运维       | Monitor **Traces** 瀑布图 |
| **FlowLogger v2（流程日志）** | `internal/event/trace_emitter.go` → WS `monitor` `flow_log`     | `EnvelopeTypeFlowLog`（与进程 `log` 分流）       | 业务用户、AI 排障 | Monitor Logs「流程」+ Traces「流程」Tab |


### 2.1 本质区别


| 维度        | Tracing（OTel + Span 树）                  | FlowLogger（流程日志）                                         |
| --------- | --------------------------------------- | -------------------------------------------------------- |
| **核心问题**  | 「哪段代码花了多久、调用关系如何？」                      | 「业务上发生了什么、是否出错、用户该怎么办？」                                  |
| **数据结构**  | Span：name、parent_id、duration、attributes | FlowLog：step、**severity**、**title**、**message**、**hint** |
| **严重级别**  | 通常仅 OK/ERROR（status）                    | **ok / info / warn / error / critical**（红黄绿）             |
| **文案**    | 偏技术（`chat.turn`）                        | 偏业务中文（「正在调用语言模型」）                                        |
| **实时推送**  | OTel 默认不走 Monitor WS                    | 设计为 Monitor WS + 可落库查询                                   |
| **AI 排障** | 需理解 Span 语义                             | 结构化 JSONL + hint，开箱即用                                    |


结论：**Tracing 不能单独替代 FlowLogger**——若 Monitor「日志」只展示 OTel Span，业务用户和 AI 会缺少 severity、中文说明与排障建议。

### 2.2 推荐：Tracing 作骨干，Flow 作 Monitor 统一输出的「语义层」

**可以且应当统一**，但不是「只保留 Jaeger」，而是：

```text
                    ┌─────────────────────────────────────┐
  业务代码一次打点   │  TraceEmitter（统一写入 API，v2）      │
                    └──────────────┬──────────────────────┘
                                   │ 同一 trace_id / run_id
           ┌───────────────────────┼───────────────────────┐
           ▼                       ▼                       ▼
   FlowLog 投影器          Span 投影器              OTel 投影器
   (Monitor 主输出)    (Usage 瀑布图)          (Jaeger，可选)
   WS flow_log + DB      metadata.spans         OTLP export
```


| Monitor 子模块           | v2 定位            | 数据来源                          |
| --------------------- | ---------------- | ----------------------------- |
| **流程日志**（新 Tab / 主视图） | 时间线 + 红/黄/绿 + 中文 | **FlowLog**（主）                |
| **Traces / 瀑布图**      | 同一条链路的耗时结构       | **Span 投影**（与 Flow 同源，非第二套打点） |
| **Logs（LogStream）**   | 进程/Gateway 文本日志  | 不变，非业务 Trace                  |


**对用户**：在 Monitor 选一个 `trace_id`，同时看到：

1. **流程时间线**（FlowLog）：「意图识别完成 → 调用模型 → 流式消费完成」+ severity 配色
2. **耗时瀑布**（Span）：同 trace 下的 `chat.turn` / `chat.llm.invoke` 时长条（可与 Flow 步骤对齐）

**对 AI**：导出一份 **Trace Diagnostic Bundle** = 该 `trace_id` 下全部 FlowLog（JSONL）+ 可选 Span 摘要；不以原始 Jaeger 导出为主。

### 2.3 与「用 Tracing 作为 Monitor Logs 输出」的直接回答


| 方案                                                                   | 是否采纳 | 说明                                                          |
| -------------------------------------------------------------------- | ---- | ----------------------------------------------------------- |
| Monitor Logs **仅**展示 OTel/Jaeger 数据                                  | ❌    | 无 severity/中文/hint；依赖外部 Jaeger；无 WS 实时业务语义                  |
| Monitor Logs **仅**展示 Usage 里 `metadata.spans`                        | ❌    | 仅有耗时条，无步骤说明与告警分级；与 Flow 步骤重复维护                              |
| **TraceEmitter 一次写、多投影**；Monitor 主 UI 读 **FlowLog**，瀑布图读 **Span 投影** | ✅    | 见 [52-flow-logger.design.md §3](./52-flow-logger.design.md) |
| FlowLog 条目内嵌 `span_id` / `parent_span_id`，与 OTel TraceID 对齐          | ✅    | 同一链路可跳转 Jaeger（运维高级能力）                                      |


### 2.4 实施对需求的影响

- §3.1 的 `trace_id` **必须与 OTel TraceID（W3C）一致**（有 OTel 上下文时复用，无则生成并在整个 Turn 内固定）。  
- §3.6 前端「流程日志」与现有 **TraceList/TraceWaterfall** 合并为 **同一 Trace 详情页** 的两个视图（时间线 / 瀑布），而非三个互不关联的入口。  
- §6「不在范围」补充：FlowLogger v2 **不替代** OTel/Jaeger；而是 **收敛 Monitor 内多套打点** 为一次 `TraceEmitter` 写入。

---

## 3. 用户与场景


| 角色           | 场景                                                                |
| ------------ | ----------------------------------------------------------------- |
| 业务用户         | 聊天无响应时，在会话侧栏查看「本次请求走到哪一步、哪步报错」                                    |
| 运维 / 管理员     | Monitor 按 `trace_id` / `run_id` 过滤，查看完整链路时间线                      |
| AI 助手（内置或外部） | 导出结构化 JSON，根据 `severity` + `step` + `hint` 快速定位根因                 |
| 开发者          | 在 `internal/service` 打点，不触发 SlogBridge 死锁，与 trpc-agent-go 运行时边界一致 |


---

## 4. 功能需求

### 4.1 链路（跟踪对象）分类

日志必须挂载在明确的 **TraceContext（跟踪上下文）** 上，至少包含：


| 字段                        | 说明                           | 示例                                                                        |
| ------------------------- | ---------------------------- | ------------------------------------------------------------------------- |
| `trace_id`                | 全链路唯一 ID（可与 OTel TraceID 对齐） | `tr_01J...`                                                               |
| `session_id`              | 会话                           | `sess_...`                                                                |
| `run_id`                  | Chat / Agent 单次 Turn         | `run_...`                                                                 |
| `team_id` / `team_run_id` | Team 编排                      |                                                                           |
| `graph_run_id`            | Graph 执行                     |                                                                           |
| `channel_id`              | 渠道入站                         |                                                                           |
| `domain`                  | 业务域                          | `chat` / `team` / `graph` / `channel` / `knowledge` / `plugin` / `system` |
| `agent_key` / `agent_id`  | 主体 Agent                     |                                                                           |


**验收**：前端可选择「按会话」「按 run」「按 trace_id」查看，且同一 `trace_id` 下日志顺序与时间一致。

### 4.2 步骤（Step）细分类

步骤 ID 采用稳定点分命名：`{domain}.{subsystem}.{action}`。


| 域           | 子系统示例                                          | 步骤示例                        |
| ----------- | ---------------------------------------------- | --------------------------- |
| `chat`      | `turn` / `intent` / `llm` / `stream` / `usage` | `chat.llm.invoke`           |
| `team`      | `run` / `member` / `summary`                   | `team.run.start`            |
| `graph`     | `node` / `checkpoint`                          | `graph.node.llm`            |
| `knowledge` | `search` / `rerank` / `ingest`                 | `knowledge.rerank.fallback` |
| `plugin`    | `hook` / `guard`                               | `plugin.cost_guard.block`   |
| `event_bus` | `runner` / `state`                             | `event_bus.usage.record`    |
| `mcp`       | `session`                                      | `mcp.session.reconnect`     |


**验收**：步骤清单在 design 文档中维护注册表；新增步骤须登记，禁止随意字符串。

### 4.3 严重级别（告警类别）与展示色

与 `phase`（start/done/skip）**解耦**，单独字段 `severity`：


| severity   | 含义          | 前端色    | 典型场景           |
| ---------- | ----------- | ------ | -------------- |
| `ok`       | 正常完成        | 绿色     | `done` 且无异常    |
| `info`     | 信息          | 绿色（弱化） | 跳过、降级成功        |
| `warn`     | 需关注         | 黄色     | 重试、降级、配置缺失     |
| `error`    | 失败可恢复       | 红色     | 单步失败、落库失败      |
| `critical` | 严重 / 用户可见故障 | 深红     | Turn 失败、超时、空回复 |


**验收**：

- Monitor / 会话 Flow 面板按 `severity` 着色，不依赖解析文案。
- 同一步骤 `start` 可为 `info`，`error` 阶段为 `error` 或 `critical`。

### 4.4 人类可读文案

每条日志包含：


| 字段        | 说明                                  |
| --------- | ----------------------------------- |
| `title`   | 短标题（≤40 字），业务语言，如「正在调用语言模型」         |
| `message` | 一句完整说明，含结果，如「模型返回，开始处理流式输出（3240ms）」 |
| `hint`    | 可选，排障建议，如「检查 Provider API Key 或网络」  |


**验收**：非开发人员阅读 `title` + `message` 即可理解发生了什么；禁止仅展示 `chat.llm_call.done`。

### 4.5 AI 可解析结构

每条日志的 `metadata`（或独立 `flow_log` 载荷）须满足：

- 固定 `schema_version`（如 `flow_log/v1`）
- 机器字段 snake_case、类型稳定（数字不用字符串混用）
- 含 `correlation` 对象（trace_id、session_id、run_id…）
- 含 `timing`（`duration_ms`、`started_at`）
- 含 `error` 对象（`code`、`message`）当 severity ≥ error
- 支持 **一键导出** 为 JSON Lines，供 AI 分析

**验收**：提供示例 Prompt：「根据以下 flow_log 列表判断失败步骤与可能原因」可在 1 轮内定位。

### 4.6 前端查看能力


| 能力   | 说明                                                        |
| ---- | --------------------------------------------------------- |
| 链路视图 | 时间线 + 按 `domain` 分组折叠                                     |
| 过滤   | trace_id / session_id / run_id / severity / domain / step |
| 实时   | WS `monitor` 通道推送（与现网一致）                                  |
| 历史   | HTTP 按 trace_id 查询（持久化后）                                  |
| 关联跳转 | 从 Chat 会话、Trace 详情、Team Run 面板跳入同一 trace                  |


**验收**：Monitor 新增「流程日志」Tab 或增强 Logs/Events；会话页可查看当前 run 的 Flow。

### 4.7 框架与架构约束

- 实现位于 `internal/event`，**禁止** `internal/biz` import `trpc-agent-go`。
- Turn 热路径 **禁止** 使用 `slog`（SlogBridge 已移除）；统一 `TraceEmitter` + `TraceContext`。
- `internal/service` 创建上下文并注入 `context.Context`；`internal/agent` 仅通过 context 取 logger。
- 与 [AGENT_RUNTIME_BOUNDARY](../AGENT_RUNTIME_BOUNDARY.md) / [trpc-agent-framework-first](../../.cursor/rules/trpc-agent-framework-first.mdc) 一致。

---

## 5. 非功能需求


| 项   | 要求                                                  |
| --- | --------------------------------------------------- |
| 性能  | 单 Turn 日志条数上限可配置（默认 500）；异步发布，不阻塞 LLM               |
| 存储  | 热数据内存 Buffer + 可选落库 `flow_log_events`（见 design）     |
| 保留  | 默认 7 天（可配置），与 audit/monitor 策略一致                    |
| 安全  | 不落库完整 Prompt；敏感字段走现有脱敏策略                            |
| 兼容  | v1 `EnvelopeTypeLog` + `channel=monitor` 只读兼容 1 个版本 |


---

## 6. 验收标准（总览）

- 任意一次 Chat Turn 可通过 `trace_id` 拉取完整有序流程日志
- 前端严重级别红/黄/绿展示正确率 100%（对照 severity 字段）
- 业务用户可读懂 `title`/`message`，无需懂 step 编码
- 导出 JSON 可被 AI 用于定位「Agent 无响应」类问题（含超时、空回复、落库失败）
- 文档：需求 / 设计 / 开发计划 / 步骤注册表齐全

---

## 7. 不在范围（v2.0）

- 替代 Prometheus / OTel 指标系统
- 替代 Gateway 文本 Logs 流（`LogStream` 仍服务进程日志）
- 用 Jaeger 单独替代 Monitor 内流程日志 Tab
- 跨集群日志聚合与 ELK 对接（后续 Observability 迭代）

