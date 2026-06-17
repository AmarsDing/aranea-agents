# M56 — 业务逻辑优化（Business Logic Optimization, BLO）

> **版本**：2026-06-17 · **状态**：📋 需求草案 · **优先级**：P0–P2
> **依赖**：M53 Team/Graph · M55 Chat × Channel Cursor 对标
> **影响范围**：`internal/biz/{chat,session,channel,turn,backgroundjob}` · `internal/service` · `internal/runtime` · `internal/team` · `web/src/{components/chat,features/chat}`

> **配套文档**：
> - 设计文档：[56-business-logic-optimization.design.md](./56-business-logic-optimization.design.md)
> - 开发计划：[56-business-logic-optimization.development.md](./56-business-logic-optimization.development.md)

---

## 1. 模块定位

M56 不是一次代码重构，而是对 **Channel → Chat → Agent/Team** 主链路的 **5 个业务模型缺陷** 的纠偏：

| 主题 | 业务问题 | 期望业务价值 |
|------|----------|--------------|
| **BLO-1 Intent-Aware Admission** | 用户连发消息时，Web 排队 / Channel 中断，**跨端体验不一致** | 同一 Session 在不同入口下表现一致；为「打断 / 补充 / 新话题 / 澄清」四种语义提供差异化响应 |
| **BLO-2 Multi-Signal Escalation** | SessionRun 升级到 Durable **只看用户主动声明**，忽略 token、tool、graph 等真实复杂度信号 | 长任务用户主动声明 + 系统按复杂度自动升级；不再"短任务被冤枉" |
| **BLO-3 Channel Trigger Rules** | 群机器人 **只能"被 @"才工作**，无法实现日报 / 关键词 / Reaction / 静默观察 | 把 Channel 入口从"消息→Turn"升级为"事件流→触发器"，覆盖企业群机器人主流场景 |
| **BLO-4 Non-Blocking HITL (PendingTask)** | `await_user_reply` 状态下整个 Session 被锁，**用户不能在等待期间做别的事** | HITL 异步化为 PendingTask，Session 期间可继续处理无关 Turn |
| **BLO-5 Unified BackgroundJob** | 两套异步系统：Channel `/async` Graph + Chat SessionRun durable，**Jobs 表 / Worker / 面板三处分裂** | 统一 BackgroundJob 抽象，支持任务依赖、优先级调度、跨入口可见性 |

**核心定位**：M56 是 **Multi-Agent 平台从"会话工具"走向"任务平台"的转折点**。BLO-5 是基础设施，BLO-1/2/4 在其上做语义升级，BLO-3 在其上扩展产品形态。

---

## 2. 用户故事

### 2.1 用户故事级痛点

| 角色 | 故事 | 痛点 |
|------|------|------|
| 飞书群用户 | "刚才那条算了，我想问 X" | 第二条直接打断 LLM，丢失推理 |
| Web 用户 | "我要 Agent 跑半小时深度研究" | 必须用户主动声明才升 durable，期间网页关掉就丢 |
| 产品经理 | "群里每天 18 点出日报" | 现在只能挂 Cron，无法跨群灵活配置 |
| 运营 | "bot 问'要 A 还是 B'，我下班了想想看" | 期间用户在群里问别的，Session 全部被拒 |
| 运维 | "现在系统里跑了多少后台任务" | 要查 Chat Jobs + Channel Jobs 两处 |

### 2.2 功能需求清单

#### BLO-1 Intent-Aware Admission

- **FR-BLO-1-01**：用户连发消息时，系统识别第二条消息的意图（interrupt / append / new_topic / clarify / unknown）
- **FR-BLO-1-02**：同一 Session 在 Web / Channel / API 入口下，相同输入得到一致的 admission 决策
- **FR-BLO-1-03**：intent 置信度低于阈值时回退到原 ingress 策略，不引入新风险
- **FR-BLO-1-04**：classifier 可通过 feature flag 一键关闭

#### BLO-2 Multi-Signal Escalation

- **FR-BLO-2-01**：系统根据多维度信号（tool 调用次数、token 数、是否进入 graph、嵌套 agent 数、流逝时间）自动决定 Run 升级到 escalating / durable
- **FR-BLO-2-02**：用户主动 `/background` 立即升 durable，不可被任何规则绕过
- **FR-BLO-2-03**：升级通知（IM 卡片 / WS）携带 `Reason` 字段，前端展示"为什么转后台"
- **FR-BLO-2-04**：升级规则阈值可通过配置热调

#### BLO-3 Channel Trigger Rules

- **FR-BLO-3-01**：群机器人支持 `mention` / `keyword` / `reaction` / `schedule` / `threshold` / `silent` 六种触发规则
- **FR-BLO-3-02**：默认所有 Channel 创建时插入 `kind=mention` 规则，保护现有"被 @"行为
- **FR-BLO-3-03**：`kind=silent` 启用后写入静默观察记录，不触发 Turn，但可被 L2 记忆召回
- **FR-BLO-3-04**：`kind=schedule` 通过 BackgroundJob 调度，到点自动触发对应 Agent
- **FR-BLO-3-05**：`kind=reaction` 收到表情回复后写入 evaluation feedback
- **FR-BLO-3-06**：群内首次启用 `silent` 规则时 IM 通知"本群已开启智能观察"
- **FR-BLO-3-07**：管理员可通过 UI 配置 / 修改 / 删除触发规则（schedule 仅 admin 可改）

#### BLO-4 Non-Blocking HITL (PendingTask)

- **FR-BLO-4-01**：`await_user_reply` 触发后，Session 锁释放，用户可在同 Session 发起新 Turn
- **FR-BLO-4-02**：用户通过 IM 卡片按钮或 Web UI 提交回复，路由到对应 PendingTask
- **FR-BLO-4-03**：PendingTask 超时后 Run 自动标记 failed 并通知用户
- **FR-BLO-4-04**：用户回复时若 PendingTask 已超时，返回 410 Gone + 提示重新发起
- **FR-BLO-4-05**：同一 Run 不允许并存多个相同 kind 的 PendingTask

#### BLO-5 Unified BackgroundJob

- **FR-BLO-5-01**：所有后台任务（Channel /async、Chat SessionRun durable、scheduled）统一通过 BackgroundJob 抽象管理
- **FR-BLO-5-02**：支持任务依赖（ParentJobID DAG），子 Job 在父 Job 完成后执行
- **FR-BLO-5-03**：支持优先级调度（实时池 priority<50，后台池 priority>=50）
- **FR-BLO-5-04**：取消父 Job 级联取消所有未启动的子 Job
- **FR-BLO-5-05**：跨入口可见性——`GET /v1/background-jobs?owner_type=session` 与 `?owner_type=channel` 返回统一 schema
- **FR-BLO-5-06**：失败任务可重试（attempts < max_attempts）

---

## 3. 非功能需求

- **NFR-01**：不重写 trpc-agent-go 框架；所有改动在 `internal/biz` + `internal/service` + `internal/runtime`
- **NFR-02**：不改造已交付的 Memory L0-L4 写入路径（与 M56 正交）
- **NFR-03**：不重做前端 TurnBlock；前端仅适配新事件类型与 Job 视图
- **NFR-04**：不引入新外部依赖（Redis / Kafka 等）；继续基于 SQLite/PG + 进程内 worker
- **NFR-05**：所有 BLO 主题灰度 flag 可单独开关；关闭后行为与现状一致
- **NFR-06**：`make ci` 全绿 · `make runtime-boundary` 红线检查通过
- **NFR-07**：DB schema 用 additive migration：新表/新列，不删旧表
- **NFR-08**：任意 BLO 主题关闭 feature flag 即回到现状路径（可回滚）
- **NFR-09**：`biz` 不 import `pkg/trpc-agent-go`；Runner 装配在 `service` + Wire
- **NFR-10**：不改 OpenAPI 不向后兼容；新增字段全部 optional，旧字段语义不变

---

## 4. 验收标准

### 4.1 业务级

| BLO | 验收 |
|-----|------|
| BLO-1 | 飞书群内连发 3 条 `"等等"/"对了 X"/"我再问 Y"` 分别得到 interrupt/append/new_topic 三种处理；Web 同样输入得到一致体验 |
| BLO-2 | 触发 `tool_calls > 8` 的任务自动升 durable 且 IM 卡片显示理由；用户 `/background` 立即升 durable |
| BLO-3 | 群里配置 cron `0 18 * * *`，到点自动发送日报；某条消息 reaction 后写入 evaluation |
| BLO-4 | `await_user_reply` 等待期间，同 Session 用户问"现在几点"得到正常回复 |
| BLO-5 | `GET /v1/background-jobs?owner_type=session` 与 `?owner_type=channel` 返回统一 schema；某 Job 取消后子 Job 自动取消 |

### 4.2 工程级

- 所有 BLO 主题灰度 flag 可单独开关；关闭后行为与现状一致
- `make ci` 全绿 · `make runtime-boundary` 红线检查通过
- 新增端到端测试：`go test ./internal/service/... -run 'BLO_'` 与 `go test ./internal/biz/backgroundjob/... -run 'Dispatcher|Repo'`
- Datadog 看板：BackgroundJob P95 / PendingTask 超时率 / Intent classifier 召回率 / Escalation 决策分布

> **详细测试用例**：见 [开发计划 §9 关键验收用例](./56-business-logic-optimization.development.md#9-关键验收用例)

---

## 5. 参考

- 设计文档：[56-business-logic-optimization.design.md](./56-business-logic-optimization.design.md)
- 开发计划：[56-business-logic-optimization.development.md](./56-business-logic-optimization.development.md)
- M55 Chat × Channel Cursor 对标：[55-chat-channel-cursor.md](./55-chat-channel-cursor.md)
- Channel 长任务需求：[17-channel.md](./17-channel.md)
