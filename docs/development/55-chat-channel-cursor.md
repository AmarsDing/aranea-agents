# M55 — Chat × Channel × Cursor 对标 — 需求文档

> **版本**：2026-06-17
> **读者**：产品、架构、全栈
> **关联**：[55-chat-channel-cursor.design.md](./55-chat-channel-cursor.design.md)（设计） · [55-chat-channel-cursor.development.md](./55-chat-channel-cursor.development.md)（开发计划） · [1-chat.md](./1-chat.md) · [17-channel.md](./17-channel.md) · [51-message-mechanism.md](./51-message-mechanism.md)
> **背景**：飞书长任务 5 分钟超时失败；Channel 会话消息在 Web Chat 不可见或体验异常；需以 Cursor 为参照统一 Chat 产品形态。

---

## 1. 问题陈述（用户可感知现象）

### 1.1 现象 A — 长任务 5 分钟失败

飞书用户下发「需 24 小时」的分析类指令后：

1. 收到 ACK：「收到，正在处理…」
2. 约 5 分钟后失败：`CHAT_AGENT` / `响应超时，请稍后重试 (5m0s)`
3. 出站：「任务执行失败，请稍后重试。」

**用户期望**：小时级批处理 Job。
**系统现状**：分钟级同步 Turn（默认 `DefaultTurnTimeout = 10m`，但飞书历史配置仍可能 5m）。

### 1.2 现象 B — Web 看不到飞书消息

飞书侧 Agent 正常回复；Web Chat 打开同一 Agent 的 `feishu:` Session 时：

- 工具卡片可见，但用户指令 / 助手正文「像不存在」
- 长会话（100+ 条）页面卡顿、滚动异常

**用户期望**：飞书与 Web 双向可见、实时同步。
**系统现状**：数据层多数已落库（Channel 与 Web 共用 Session）；失败主要在 Web 观测平面（Session 亲和、同步协议、Turn 呈现、性能）。

### 1.3 现象 C — Web 时间线"工具淹没正文"

一轮对话里 10+ 个工具卡平铺，正文滚到屏幕外。用户难以聚焦到 Assistant 的自然语言回复。

### 1.4 现象 D — 后台任务无观测壳

async Graph 仅 IM 出站 ACK；Web 看不到 Job 状态 / 进度 / 历史。用户在 Web 端无法判断后台任务是否在跑、何时完成。

### 1.5 现象 E — 多入口路径分裂

HTTP/WS/Channel/Cron 行为接近但仍有细枝末节差异，导致维护成本高、回归风险大。

> P-1（长任务超时）/ P-2（双向不可见）是结构性缺口；P-3（工具淹没）是体验崩坏；P-4（路径分裂）是可维护性债；P-5（无观测壳）是产品完整性缺口。M55 必须把它们一并解决，单点修补会反复回归。

---

## 2. Cursor 对话界面 — 产品参照

Cursor 不是「聊天 App」，而是 **IDE 内嵌 Agent 工作区**。以下维度是 Aranea 对标的权威参照。

### 2.1 双模式：Chat vs Composer / Agent

| 模式 | 用户目标 | UX 特征 |
|------|----------|---------|
| **Chat** | 解释、问答、小改动 | 侧边栏、短回复、低侵入 |
| **Composer / Agent** | 多步任务、改多文件 | 全宽、步骤清单、可后台 |

**产品原则**：**对话 Turn** 与 **后台 Job** 是两种产品壳，不应混在同一超时模型里。

### 2.2 上下文模型

- `@file` / `@folder` / `@codebase` / `@docs` 显式注入
- **Context 用量条**（token %）始终可见
- 用户清楚「本轮 Agent 能看见什么」

### 2.3 工具调用呈现

- 工具 = **可折叠块**，默认收起或单行摘要
- 主视觉 = **Assistant 自然语言 + diff / 产物**
- 终端 / log 在块内滚动，**不撑满时间线**

### 2.4 流式与响应性

- Token 增量渲染，禁止全量 replace
- 生成中 **Stop**；Stop 后保留已生成部分
- **Follow-up Queue**：运行中可继续输入，排队而非阻塞

### 2.5 长任务 / Background Agent

- 长任务 **脱离当前 Turn 超时**
- 独立 **Background 面板**：状态、日志、完成通知
- 用户可继续编辑，Agent 在后台跑

### 2.6 产物导向

- 代码变更 = **inline diff** + **Apply / Reject**
- Checkpoint 可回滚

### 2.7 布局信息层次（示意）

```
┌─────────────────────────────────────────┐
│ Context: @files  68% ctx    [Stop]      │  ← 状态栏
├─────────────────────────────────────────┤
│ [User] 请重构 auth 模块                  │
│ [Assistant] 好的，我将…                  │  ← 主对话
│   ▶ Ran terminal (3)  ▶ Read file (2)    │  ← 工具折叠条
│   ```diff … ```  [Apply]                │  ← 产物
├─────────────────────────────────────────┤
│ [输入框]  @  附件  模型  Enter发送        │
└─────────────────────────────────────────┘
```

---

## 3. 用户故事

### US-1：飞书用户下发长任务不超时

**作为** 飞书用户，
**我希望** 下发「全量分析」「研报」等小时级指令后，系统能识别为后台任务并 ACK 一个 Job ID，
**以便** 不在 5 分钟后收到失败通知，且能在 24 小时内拿到结果。

**验收**：
- 配置 `turn_timeout_sec=900` 后 10min 内可完成的多工具任务成功（M55-LT-01）
- 「全量分析」类指令自动或配置走 `async`；ACK + Job ID；不触发 5m 超时（M55-LT-02）

### US-2：Web 用户实时看到飞书会话

**作为** Web 用户，
**我希望** 飞书 Turn 进行中或完成后，Web 打开同一 Session 能在 5 秒内看到 user 消息 + running 状态 + assistant 正文，
**以便** 不必手动刷新，且能在 Web 端继续操作。

**验收**：
- 飞书 Turn 进行中，Web 打开同 Session → 5s 内看到 user 消息 + running 状态（M55-SYNC-01）
- 飞书 Turn 完成，Web 已打开 → 增量出现 assistant 正文，无需手动刷新（M55-SYNC-02）

### US-3：长会话滚动流畅、工具默认折叠

**作为** Web 用户，
**我希望** 100+ 消息的 Session 滚动流畅，工具调用默认折叠不淹没正文，
**以便** 快速定位到 Assistant 的关键回复。

**验收**：
- 100+ 消息 Session → 滚动流畅；最后一轮 user/assistant 可见；工具默认折叠（M55-UI-01）
- Turn 内 20+ 工具调用 → ToolStrip 折叠；展开后卡片正常（M55-UI-02）

### US-4：后台任务在 Web 可观测

**作为** Web 用户，
**我希望** 飞书 `/async` 触发的后台任务在 Web 面板 3s 内显示，并能看到状态、进度、Graph 深链，
**以便** 不必切换到飞书也能跟踪任务。

**验收**：
- 飞书下发 `/async` → Web 面板 3s 内显示新 Job；点击跳转 Graph Run 页（M55-JOB-01）

### US-5：飞书与 Web 双向不回声

**作为** 用户，
**我希望** 从 Web 触发的 Turn 不会在飞书侧重复出站，
**以便** 避免双向回声干扰。

### US-6：Cursor 式 @ 引用与 diff Apply

**作为** Web 用户，
**我希望** 在 Composer 输入框用 `@` 引用 agent files / knowledge / session 历史，且工具产出的 diff 有 Apply / Reject 按钮，
**以便** 像 Cursor 一样精确控制上下文与产物落地。

### US-7：思考过程可折叠/侧栏化

**作为** Web 用户，
**我希望** Assistant 的 reasoning 默认折叠或可切到侧栏，正文与思考视觉分离，
**以便** 聚焦正文，需要时再展开思考链。

### US-8：上下文压力可视化

**作为** Web 用户，
**我希望** 看到当前 Context 占比（圆环 + 点击展开 Prompt 占比分解），且超过阈值时有 warning/critical 提示，
**以便** 决定是否压缩或开新会话。

---

## 4. 功能需求清单

### FR-1：长任务路由与配置

| ID | 需求 |
|----|------|
| FR-1.1 | 系统支持 Channel 长任务 preset（如 `feishu_long_analysis`），前端编辑器一键应用 |
| FR-1.2 | `execution_mode=auto` + `/async` 前缀路由到 Job 平面 |
| FR-1.3 | `execution_mode=async` 强制路由到 Job 平面 |
| FR-1.4 | sync Turn 硬上限 15 分钟；超过必须自动改判走 Job，不允许 silent 超时 |
| FR-1.5 | 超时错误文案区分 sync vs async，引导用户使用 `/async` |
| FR-1.6 | Run 两阶段升格：interactive（分钟级）→ durable（小时级），用户可在软预算到时确认升级 |

### FR-2：Channel ↔ Web 同步

| ID | 需求 |
|----|------|
| FR-2.1 | 每次 Turn 完成（user 入库 + assistant 入库 + activity 收口）`session_revision` +1 |
| FR-2.2 | Envelope 在 terminal / sync 态必带 `session_revision` |
| FR-2.3 | API 支持增量拉取：`GET /v1/sessions/{id}/messages?after_revision=N` |
| FR-2.4 | Web 选中 Session 时强制建立 Session WS 订阅 |
| FR-2.5 | Web 收到 `session_revision=R > local` 时 debounced 200ms hydrate `after_revision=local` |
| FR-2.6 | 回放窗口期间累积 envelope，回放结束后再统一 merge |
| FR-2.7 | Channel 入站注入 `source=channel`，Web UserBubble 显示来源徽标 |
| FR-2.8 | Channel 入站触发 Web 端 Session 列表刷新 + 全局铃铛通知 |

### FR-3：TurnBlock UI

| ID | 需求 |
|----|------|
| FR-3.1 | 一轮对话一个 TurnBlock 容器：UserBubble + ToolStrip（折叠）+ AssistantBubble + ArtifactRow |
| FR-3.2 | 工具默认折叠为单行摘要（`▶ 3 tools · 12.4s · 1 failure`），展开后为 ChatExecutionCard |
| FR-3.3 | Team 子成员消息保留独立色条 lane，不并入主 TurnBlock 的 ToolStrip |
| FR-3.4 | 滚动锚定到最后 Assistant 气泡顶部，避免被工具长结果挤到屏幕外 |
| FR-3.5 | 虚拟列表阈值 `CHAT_VIRTUAL_SCROLL_THRESHOLD=40`，启用 `q-virtual-scroll` |
| FR-3.6 | WS patch rAF 批处理，禁止 `runner_completion` 全量 replace messages |
| FR-3.7 | 思考/ReAct 互斥；空 reasoning 不展示「思考过程」壳 |
| FR-3.8 | 流式「正在思考…」单行态 + spinner；首字节前 UX |
| FR-3.9 | 双 ToolStrip 去重/合并：单轮一条摘要 |
| FR-3.10 | TurnBlock 思考/正文视觉分离（`turn-block__response` 分区） |
| FR-3.11 | Reasoning 侧栏模式：内联替换为可点击提示 |

### FR-4：Background Job 面板

| ID | 需求 |
|----|------|
| FR-4.1 | `GET /v1/chat/jobs` 按 session_id / agent_id / status 过滤，JOIN 无 N+1 |
| FR-4.2 | `POST /v1/chat/jobs/{id}/cancel` 取消运行中 Job |
| FR-4.3 | Web Jobs 侧栏：列表 + 详情 + Graph execution 深链 |
| FR-4.4 | Job 状态变更走 `run_status` Envelope 实时刷新 |
| FR-4.5 | 飞书完成通知携带 Session 深链 |
| FR-4.6 | Job 面板显示运行耗时、阶段（interactive/durable）、来源 |

### FR-5：上下文与 Apply

| ID | 需求 |
|----|------|
| FR-5.1 | Composer `@` 触发候选列表（agent files / knowledge / session 历史） |
| FR-5.2 | `SendMessageOptions.context_refs` 字段透传到后端 Turn 链路 |
| FR-5.3 | 上下文清单抽屉：圆环 + 点击展开 Prompt 占比分解（精确数据优先） |
| FR-5.4 | `context_usage` envelope 推送 `prompt_breakdown` 字段 |
| FR-5.5 | diff Apply 卡片：inline diff + Apply / Reject 按钮 |
| FR-5.6 | `EnvelopeToolCall` 结构化 diff 字段透传 |

### FR-6：24h Durable Job

| ID | 需求 |
|----|------|
| FR-6.1 | Worker deadline 24h 取代内存 goroutine watch |
| FR-6.2 | Job 入队时持久化 `deadline_at = now + 24h` |
| FR-6.3 | 独立 Worker 周期扫描 → 续跑 Graph checkpoint / 超时标记 |
| FR-6.4 | 进程重启后 Job 不丢；24h 超时正确标记 timeout |
| FR-6.5 | Graph / trpc checkpoint resume：会话快照 + 合成 prompt |
| FR-6.6 | IM 进度百分比、取消/重试 Job API |

---

## 5. 非功能需求

| ID | 指标 | 目标 |
|----|------|------|
| NFR-1 | Web 选中 Session 到首屏 | ≤500ms（cache hit）/ ≤1500ms（cold） |
| NFR-2 | Channel 入站 ACK | ≤2s |
| NFR-3 | Turn 首字节（sync） | ≤30s（FirstByteTimeout） |
| NFR-4 | WS envelope → 渲染 | ≤50ms |
| NFR-5 | 500 条消息列表滚动 | ≥55fps |
| NFR-6 | `session_revision` 并发 bump | 100 次并发无丢失，最终值 == 100 |
| NFR-7 | 架构红线 | `make runtime-boundary` 通过；biz 不 import trpc-agent-go |
| NFR-8 | 兼容性 | 旧 client 无 `after_revision` 参数时退化为全量返回 |
| NFR-9 | A11y | 所有 chip / badge / button 有 `aria-label`；折叠/展开带 `aria-expanded` |
| NFR-10 | i18n | 新增 `chat.turn.block.*` / `chat.job.*` 命名空间，中英双语 |

---

## 6. 交互规格（用户视角）

### 6.1 飞书长任务交互流

1. 用户在飞书下发「请做全量分析」
2. 系统 ACK：「收到，正在处理…」（≤2s）
3. 路由判定为 Job → 创建 ChannelTurnJob + SessionRun(escalating/durable)
4. 飞书出站：「后台任务已创建（Job: X）」+ 升级确认卡片（若软预算到）
5. 用户点击「升级为后台任务」→ durable phase + Checkpoint
6. 24h 内完成 → 飞书出站完成卡片（含 Session 深链）+ Web 面板绿点

### 6.2 Web 端 Channel 入站通知流

1. 飞书用户下发消息
2. Web 端 Header 铃铛红点 + toast「飞书入站：[Session 标题]」+「查看」按钮
3. 用户点击 → 选中该 Session + 强制 WS 订阅
4. 5s 内 TurnBlock 出现 user 消息 + running 状态 + 「来自飞书 · 进行中」横条
5. Turn 完成 → assistant 正文增量出现，横条 5s 后消失

### 6.3 TurnBlock 交互流

1. 用户发送消息 → TurnBlock #N 出现 UserBubble
2. 工具调用 → ToolStrip 折叠条出现「▶ N tools · Xs」
3. Assistant 流式 → AssistantBubble 增量渲染 + 「正在思考…」spinner
4. 完成 → ToolStrip 显示总耗时；Assistant 正文完全展开
5. 用户点击 ToolStrip → 展开工具卡片详情
6. 用户点击 reasoning 折叠条 → 展开思考链（或切到侧栏模式）

### 6.4 Background Job 面板交互流

1. 用户点击 Chat 右侧抽屉「后台任务」Tab
2. 列表显示运行中 Job（红点）+ 最近完成 Job（绿点）
3. 用户点击 Job → 详情面板：状态 / 阶段 / 运行耗时 / Graph 深链
4. 用户点击「取消」→ `POST /v1/chat/jobs/{id}/cancel` → 状态变 cancelled
5. 用户点击 Graph 深链 → 跳转 Graph Run 页

### 6.5 上下文压力警告交互流

1. Composer 输入框上方显示 Context 圆环（% + 颜色）
2. 圆环点击 → Popover 展开 Prompt 占比分解（system / agent / history / tools / user）
3. 占比超过 warning 阈值 → 黄色 banner「上下文压力较高，建议压缩或开新会话」
4. 占比超过 critical 阈值 → 红色 banner + 「开新会话」按钮
5. 收到压缩通知 → toast「上下文已压缩：80% → 45%」

---

## 7. 验收标准（系统级）

| ID | 场景 | 验收 |
|----|------|------|
| M55-LT-01 | 飞书下发 10min 内可完成的多工具任务 | 配置 `turn_timeout_sec=900` 后成功；IM transcript 可见进度 |
| M55-LT-02 | 飞书下发「全量分析」类指令 | 自动或配置走 `async`；ACK + Job ID；不触发 5m 超时 |
| M55-SYNC-01 | 飞书 Turn 进行中，Web 打开同 Session | 5s 内看到 user 消息 + running 状态 |
| M55-SYNC-02 | 飞书 Turn 完成，Web 已打开 | 增量出现 assistant 正文，无需手动刷新 |
| M55-UI-01 | 100+ 消息 Session | 滚动流畅；最后一轮 user/assistant 可见；工具默认折叠 |
| M55-UI-02 | Turn 内 20+ 工具调用 | ToolStrip 折叠；展开后卡片正常 |
| M55-JOB-01 | 飞书 `/async` 触发 | Web Background Jobs 面板 3s 内显示 |
| M55-RUN-01 | Agent 单轮正常完成 | 发送消息 → 流式输出 → runner_completion → 增量 hydrate |
| M55-RUN-02 | Agent 工具调用 | diff_edit/patch_file → ChatDiffViewer 渲染 → Apply/Reject 按钮 |
| M55-RUN-03 | Team 多成员协作 | 并行执行 → 成员 lane → completion 合并 |
| M55-RUN-04 | Durable Job 生命周期 | interactive → escalating → durable → resume → completed |

---

## 8. 文档索引

| 文档 | 用途 |
|------|------|
| [55-chat-channel-cursor.design.md](./55-chat-channel-cursor.design.md) | 架构设计、数据模型、Envelope/API 契约、UX 规范 |
| [55-chat-channel-cursor.development.md](./55-chat-channel-cursor.development.md) | 分阶段任务与排期、代码锚点、现状评估 |
| [17-channel.md](./17-channel.md) §8 | Channel 长任务配置规格 |
| [1-chat.md](./1-chat.md) | Chat 模块需求 |
| [51-message-mechanism.md](./51-message-mechanism.md) | WS / session_revision 协议 |
