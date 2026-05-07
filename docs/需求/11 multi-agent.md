# Multi-Agent / Team 编排需求优化与实施规划

## 1. 背景与目标

当前系统已经具备 Agent 管理、Agent 分类、Team 基础数据、Session、Tool、Skill、Plugin、模型管理和运行审计等模块。Multi-Agent 不应作为一个完全独立的新系统，而应基于现有 `Team` 概念扩展为“多智能体编排单元”，让用户可以把多个专业 Agent 组织成一个可运行、可观测、可配置的协作团队。

核心目标：

- 让用户在 UI 中创建 Team，并为 Team 绑定多个 Agent。
- 支持常见编排模式：顺序、并行、主控分派、生成-评审闭环。
- Team 可以像单个 Agent 一样发起会话，并在 Chat 页面中运行。
- 编排过程可追踪：展示每个子 Agent 的输入、输出、状态、耗时、错误和成本。
- 初期以确定性配置为主，不做复杂自适应拓扑、强化学习调度和跨框架 A2A。

## 2. 范围定义

### 本期 MVP 做

- Team 管理页从占位页升级为真实管理页。
- Team 表单支持配置成员 Agent、角色、顺序、启用状态和编排模式。
- Chat 页支持选择 Team 发起会话，后端按 Team 定义执行多 Agent 编排。
- 支持 4 种 MVP 编排模式：
  - `sequential`：按顺序执行多个 Agent，上一个结果进入下一个 Agent。
  - `parallel`：多个 Agent 并行处理同一任务，最后由汇总 Agent 合成结果。
  - `coordinator`：主控 Agent 先制定子任务，再分派给成员 Agent。
  - `critic_loop`：生成 Agent 与评审 Agent 多轮迭代，达到条件或最大轮数后结束。
- 运行记录落库，支持在监控页或 Team 详情页查看编排轨迹。

### 本期不做

- 不做真正的 A2A 跨框架协议发现。
- 不做强化学习调度、PPO 或自动拓扑训练。
- 不做图工作流编辑器。
- 不做自动生成新 Agent。
- 不做外部 Team 市场或模板商店。
- 不做复杂人工审批流，仅预留高风险工具拦截接口。

## 3. 用户场景

### 场景 1：顺序流水线

用户创建“研究报告 Team”，包含：

- 资料收集 Agent
- 分析 Agent
- 写作 Agent
- 校对 Agent

用户在 Chat 中选择该 Team 输入主题，系统按顺序调用成员，最终返回校对后的报告。

### 场景 2：并行评审

用户创建“代码审查 Team”，包含：

- 安全审计 Agent
- 性能分析 Agent
- 可维护性审查 Agent
- 汇总 Agent

系统并行调用前三个 Agent，再由汇总 Agent 输出统一审查报告。

### 场景 3：生成-评审闭环

用户创建“文案优化 Team”，包含：

- 生成 Agent
- 评审 Agent

生成 Agent 产出初稿，评审 Agent 给出修改意见；系统最多迭代 N 轮，直到评分达标或轮数耗尽。

## 4. 产品需求

### 4.1 Team 管理页

入口：侧栏 `Team 管理`。

页面能力：

- Team 列表：展示名称、Key、编排模式、成员数、启用状态、最近运行状态、创建时间。
- 搜索与筛选：按名称、Key、模式、启用状态筛选。
- 新增 Team：创建基础信息和编排配置。
- 编辑 Team：修改成员、模式、配置 JSON。
- 删除 Team：软删除，默认 Team 不允许删除。
- 复制 Team：复制定义并生成新 Key。

列表字段建议：

| 字段 | 说明 |
| --- | --- |
| Team | 显示名称 + team_key |
| 模式 | sequential / parallel / coordinator / critic_loop |
| 成员 | 成员数量和主控/汇总 Agent |
| 状态 | enabled / disabled / draft |
| 最近运行 | 成功 / 失败 / 运行中 / 未运行 |
| 操作 | 编辑、复制、删除、运行测试 |

### 4.2 Team 编辑器

基础字段：

- `display_name`：Team 名称。
- `team_key`：唯一标识，支持小写字母、数字、连字符。
- `status`：draft / active / archived。
- `adk_app_name`：运行时应用名，可自动生成。
- `description`：Team 说明，建议放入 `definition_json`。

成员配置：

- 选择已有 Agent。
- 设置成员角色：coordinator / worker / synthesizer / critic / generator。
- 设置执行顺序。
- 设置是否启用。
- 设置成员级别模型覆盖，可选。

编排配置：

- `mode`：编排模式。
- `max_concurrency`：并行上限。
- `max_iterations`：循环上限。
- `timeout_seconds`：单次运行超时。
- `synthesizer_agent_id`：汇总 Agent。
- `critic_agent_id`：评审 Agent。
- `stop_condition`：停止条件，MVP 可先用 `max_iterations` 和 `critic_score >= threshold`。

### 4.3 Chat 页面 Team 运行

现有 Chat 页已经有 Team 概念和 Team Session 的 UI 雏形。本期需要把它接入真实后端：

- Team 列表从 `/api/v1/teams` 获取。
- 选择 Team 后，创建 `owner_type=team` 的 session。
- 发送消息时携带 `team_id`。
- 后端识别 Team 会话并调用 Team 编排运行时。
- 前端消息区展示最终答案；详情面板可查看子 Agent 轨迹。

### 4.4 编排运行轨迹

每次 Team 运行应产生一条 orchestration run：

- run id
- team id
- session id
- user message id
- mode
- status
- started_at / finished_at
- duration_ms
- token_in / token_out
- cost
- error_message

每个子 Agent 调用应产生 step：

- step id
- run id
- agent id
- role
- input_summary
- output_summary
- status
- started_at / finished_at
- duration_ms
- error_message

MVP 可以先把 run / step 存在 `monitor-events` 或新增表；长期建议新增专表，便于查询和可视化。

## 5. 技术设计

### 5.1 Team 定义 JSON

当前后端 `Team` 已有 `definition_json` 字段，建议先以结构化 JSON 承载编排定义，避免初期数据库表过度拆分。

示例：

```json
{
  "version": 1,
  "description": "代码审查 Team",
  "mode": "parallel",
  "max_concurrency": 3,
  "timeout_seconds": 180,
  "members": [
    {
      "agent_id": "agent_security",
      "role": "worker",
      "name": "安全审计",
      "enabled": true,
      "sort_order": 10
    },
    {
      "agent_id": "agent_perf",
      "role": "worker",
      "name": "性能分析",
      "enabled": true,
      "sort_order": 20
    },
    {
      "agent_id": "agent_summary",
      "role": "synthesizer",
      "name": "汇总报告",
      "enabled": true,
      "sort_order": 90
    }
  ],
  "critic_loop": {
    "max_iterations": 3,
    "score_threshold": 0.8
  }
}
```

### 5.2 后端接口

现有：

- `GET /api/v1/teams`

建议补齐：

- `POST /api/v1/teams`
- `PATCH /api/v1/teams/{id}`
- `DELETE /api/v1/teams/{id}`
- `POST /api/v1/teams/{id}/duplicate`
- `POST /api/v1/teams/{id}/run-test`
- `GET /api/v1/team-runs?team_id=&session_id=&status=`
- `GET /api/v1/team-runs/{id}`

Chat 接口需要支持：

- `POST /api/v1/chat/messages`
- `POST /api/v1/chat/messages/stream`

当 `session.owner_type = team` 或请求体携带 `team_id` 时，进入 Team Runtime。

### 5.3 后端服务模块

建议新增或扩展：

- `TeamService`：Team CRUD、定义校验、成员 Agent 校验。
- `TeamRuntime`：根据 `definition_json` 执行 sequential / parallel / coordinator / critic_loop。
- `TeamRunRepository`：保存 run / step。
- `ChatService`：根据 session owner_type 路由到 AgentRuntime 或 TeamRuntime。

### 5.4 运行时策略

`sequential`：

1. 用用户输入作为第一个 Agent 输入。
2. 每个 Agent 输出作为下一个 Agent 的上下文。
3. 最后一个 Agent 输出作为最终答案。

`parallel`：

1. 将同一用户输入发送给所有 worker。
2. 等待全部完成或超时。
3. 将 worker 输出交给 synthesizer Agent。
4. synthesizer 输出最终答案。

`coordinator`：

1. coordinator Agent 根据用户输入产出任务拆分 JSON。
2. 后端按 JSON 匹配成员 Agent。
3. 执行子任务。
4. synthesizer 或 coordinator 生成最终答案。

`critic_loop`：

1. generator Agent 生成初稿。
2. critic Agent 输出评分和修改意见。
3. 若评分达标或达到最大轮数，结束。
4. 否则把意见回传 generator 继续生成。

## 6. 前端实施规划

### 阶段 1：Team 管理页真实化

- 新增 `TeamsPage.vue`，替换当前 GenericPage 占位。
- 新增 `features/teams/api.ts` 和 `features/teams/types.ts`。
- 实现 Team 列表、搜索、新增、编辑、删除。
- 编辑器先使用表单 + 成员列表，不做复杂图编辑器。

验收：

- 可以创建 Team。
- 可以选择已有 Agent 作为成员。
- 可以保存 `definition_json`。
- 刷新页面后配置仍存在。

### 阶段 2：Chat 接入 Team

- Chat Team 列表改为后端真实数据。
- 创建 Team session。
- 发送消息时后端进入 TeamRuntime。
- 前端展示最终回答。

验收：

- 选择 Team 后可以正常发起对话。
- sequential Team 能按成员顺序产生最终结果。
- parallel Team 能并行执行并汇总。

### 阶段 3：运行轨迹与可观测

- 新增 Team Runs 页面或在 Team 详情中展示运行记录。
- 展示 run 状态、耗时、成本、错误。
- 展示 step 列表和每个子 Agent 的摘要。

验收：

- 每次 Team 对话都能看到运行轨迹。
- 子 Agent 失败时能定位失败步骤。
- 超时、取消、失败状态清晰。

### 阶段 4：高级编排能力

- coordinator 模式支持结构化任务拆分。
- critic_loop 支持评分阈值。
- 支持 Team 模板。
- 支持 Plugin 风控：高风险工具调用前阻断或确认。

## 7. 后端实施规划

### 阶段 1：Team CRUD

- 扩展 `TeamService`。
- 扩展 repository：create/update/delete/duplicate team。
- 校验 `definition_json`：
  - mode 合法。
  - members 非空。
  - agent_id 存在。
  - sort_order 不重复。
  - parallel 必须有 synthesizer。
  - critic_loop 必须有 generator 和 critic。

### 阶段 2：Team Runtime

- 新增 `internal/runtime/team_runtime.go`。
- 复用现有 Agent Runtime 调用能力。
- 支持 sequential 和 parallel。
- 统一返回最终 Message。

### 阶段 3：Run / Step 存储

- 新增 `team_runs` 表。
- 新增 `team_run_steps` 表。
- 每个 step 记录输入输出摘要、状态、耗时、错误。

### 阶段 4：流式输出

- sequential：可先只流最终 Agent 输出。
- parallel：worker 阶段显示“处理中”，synthesizer 阶段流式输出。
- 长期：支持 step event SSE，让前端实时展示每个子 Agent 状态。

## 8. 数据表建议

### `teams`

当前已有字段可继续使用：

- `id`
- `team_key`
- `display_name`
- `status`
- `is_default`
- `definition_json`
- `adk_app_name`
- `created_at`
- `updated_at`
- `deleted_at`

### `team_runs`

建议新增：

- `id`
- `team_id`
- `session_id`
- `message_id`
- `mode`
- `status`
- `input_preview`
- `output_preview`
- `token_in`
- `token_out`
- `cost_micro_usd`
- `duration_ms`
- `error_message`
- `created_at`
- `updated_at`

### `team_run_steps`

建议新增：

- `id`
- `run_id`
- `agent_id`
- `agent_key`
- `role`
- `sort_order`
- `status`
- `input_preview`
- `output_preview`
- `token_in`
- `token_out`
- `duration_ms`
- `error_message`
- `created_at`
- `updated_at`

## 9. UI 页面结构

### Team 管理页

- 顶部：标题、说明、新增 Team。
- 筛选区：搜索、模式、状态。
- 主体：Team 表格或卡片。
- 操作：运行测试、编辑、复制、删除。

### Team 编辑弹窗

- 基础信息 Tab。
- 成员配置 Tab。
- 编排配置 Tab。
- JSON 预览 Tab。

### Team Run 详情

- 总览：状态、耗时、成本、模式。
- Timeline：每个子 Agent step。
- 输入输出：用户输入、最终输出、错误信息。

## 10. 验收标准

MVP 完成标准：

- 用户能创建、编辑、删除 Team。
- Team 可绑定多个现有 Agent。
- Team 定义能保存到后端并刷新恢复。
- Chat 中可以选择 Team 并发送消息。
- `sequential` 和 `parallel` 两种模式可完整执行。
- 每次运行至少记录 run 和 step 摘要。
- 子 Agent 失败时，最终结果包含失败说明，运行记录标记失败步骤。
- 深色/浅色模式在 Team 管理页、编辑器、运行详情中一致。

## 11. 当前实施进度

更新时间：2026-04-25

### 进度汇总

| 模块 | 状态 | 说明 |
| --- | --- | --- |
| Team CRUD / 编辑器 | 已完成 | 支持增删改查、复制、成员配置、编排模式、JSON 预览 |
| Chat Team session | 已完成 | Chat 可进入 Team 会话并发送 Team 消息 |
| sequential / parallel | 已完成 | sequential 顺序传递上下文；parallel 并行执行并汇总 |
| Run / Step 可观测 | 已完成 | 已落库 `team_runs` / `team_run_steps`，前端可查看运行轨迹 |
| 前端组件化 / 昼夜模式 | 已完成 | Team 页面已拆分组件，组件级暗色样式已覆盖 |
| Team Runtime 解耦 | 已完成 | 已抽出 `internal/service/team_runtime.go` |
| coordinator / critic_loop | 基础完成 | 已具备计划优先、生成-评审-修订结构，结构化 JSON 和评分阈值待补 |
| synthesizer Agent 汇总 | 已完成 | 已接入 `synthesizer_agent_id`，由指定 Agent 生成最终回复并记录为 step |
| parallel 部分失败 | 已完成 | 部分失败时继续汇总成功成员结果，run 标记为 `partial_success` |
| timeout / cancel 状态 | 已完成 | 已按 `timeout_seconds` 控制 Team 运行上下文，并记录 `timeout` / `cancelled` 状态 |
| 实时 step event SSE | 基础完成 | 已新增 SSE API，运行轨迹抽屉可实时接收 run/step 事件 |
| Team 模板 | 基础完成 | 已内置顺序协作、并行专家组、生成评审、主控分派模板 |
| 图工作流 schema / 预览 | 基础完成 | definition 已支持 `graph.nodes/edges`，Team 编辑器可生成基础拓扑预览 |
| A2A 协议 | 基础完成 | definition 已支持 `a2a` 配置，Runtime 使用统一 sender/receiver/intent/payload 信封传递上下文 |
| 自适应拓扑选择 | 基础完成 | 已支持 `adaptive` mode，Runtime 基于任务关键词、成员角色和成员数量选择实际拓扑 |

### 已完成

- Team CRUD：后端已支持创建、读取、更新、删除、复制；前端 Team 管理页已接入真实接口。
- Team 编辑器：已支持基础信息、成员 Agent、编排模式、并发上限、说明和 JSON 预览。
- Chat Team session：Chat 页面已支持从 Team 进入会话、创建 Team session、发送 Team 消息。
- sequential runtime：已按成员顺序执行，并将上一个成员输出传递给下一个成员。
- parallel runtime：已支持并行调用多个成员，并汇总为 Team 输出。
- Run / Step 存储：已新增 `team_runs`、`team_run_steps`，每次 Team 执行记录 run 和 step 摘要。
- Team Run 详情：Team 管理页已提供运行轨迹抽屉，可查看 run 概览和子 Agent step。
- 前端组件化：Team 管理页已拆分为 `TeamToolbar`、`TeamCard`、`TeamEditorDialog`、`TeamRunsDialog` 和 `teamUtils`。
- 昼夜模式：Team 管理页、编辑弹窗、运行轨迹抽屉已使用组件级 `is-dark` 样式。
- Team Runtime 解耦：已新增 `internal/service/team_runtime.go`，ChatService 只保留 Team chat 入口转发。
- coordinator：已支持 coordinator 成员优先生成执行计划，再将计划传递给 worker 成员执行。
- critic_loop：已支持 generator 初稿、critic 评审、按 `max_iterations` 触发可选修订。
- synthesizer Agent 汇总：已支持 `synthesizer_agent_id`，由指定 Agent 基于所有成员 step 生成最终回复。
- parallel 部分失败：已允许部分成员失败时继续汇总成功成员结果，失败 step 保留在运行轨迹中。
- timeout / cancel 状态：已按 `timeout_seconds` 为 Team 运行创建超时上下文，run/step 可区分 `timeout`、`cancelled`、`failed`。
- 实时 step event SSE：已新增 `/api/v1/team-run-events`，Team Runtime 在 run 开始/结束、step 完成时发布事件，前端运行轨迹抽屉实时刷新。
- Team 模板：Team 编辑器已支持顺序协作、并行专家组、生成评审、主控分派四个内置模板，一键填充 mode、成员角色、并发上限和评审参数。
- 图工作流 schema / 预览：Team definition 已支持 `graph.nodes`、`graph.edges`、`layout`，保存时自动生成图 schema，编辑器可展示基础节点和边。
- A2A 协议：Team definition 已支持 `a2a.enabled`、`envelope_version`、`message_format`、`include_trace`、`max_payload_chars`，Team Runtime 在成员交接、并行分派、主控分派、生成评审中使用统一 A2A 信封。
- 自适应拓扑选择：已新增 `adaptive` mode，运行时根据任务关键词、成员角色和成员数量选择 `sequential`、`parallel`、`coordinator` 或 `critic_loop`，前端可选择并预览该模式。
- Team Run 成本汇总：`team_runs` / `team_run_steps` 已新增 `cost_micro_usd`，Team Runtime 按子 Agent 模型调用成本写入 step 并汇总到 run，前端运行轨迹可查看本次团队调用和每个子 Agent 的费用。

### 部分完成

- 失败处理：run/step 可记录失败、超时、取消状态，parallel 已支持部分失败继续汇总，但重试策略尚未完善。
- coordinator：已具备计划优先执行结构，但计划仍是自然语言，尚未校验结构化 JSON。
- critic_loop：已具备循环修订结构，但 `score_threshold` 尚未接入结构化评分。
- 实时观测：已完成 SSE 基础链路，但还未加入运行中 step_started、进度百分比、事件回放与断线续传。
- Team 模板：已完成前端内置模板，尚未支持后端模板库、自定义模板保存和模板权限。
- 图工作流：已完成 definition schema 和基础预览，但尚未支持拖拽编辑、条件分支执行和后端按 graph DAG 调度。
- A2A 协议：已完成内部消息信封基础版，尚未支持跨进程 Agent 地址、外部 A2A 协议握手、能力发现和消息持久化。
- 自适应拓扑选择：已完成启发式基础版，尚未接入历史质量、成本、延迟和成功率等指标进行动态调度。

### 待实施

- A2A 外部协议握手、能力发现和消息持久化。
- 图工作流拖拽编辑、条件边和后端 DAG 调度。
- 自适应拓扑指标化调度和策略学习。
- 更完整的团队级模型用量聚合报表。

### 实施计划

第一阶段：Runtime 解耦（已完成）

- 新增 `internal/service/team_runtime.go`，承接 Team definition 解析、成员筛选、拓扑执行、step 生成、run/step 记录、结果汇总。
- `ChatService` 仅负责 session 校验、普通 Agent chat、Team chat 入口转发。
- 运行后保持现有 API 和前端行为不变。

第二阶段：补强 coordinator / critic_loop（已完成基础结构）

- coordinator 不再只拼接提示词，而是按 coordinator 成员优先生成计划，再把计划传给 worker/synthesizer。
- critic_loop 按 generator -> critic -> optional revision 的结构运行，使用 `critic_loop.max_iterations` 控制循环次数。
- run/step 继续复用现有 `team_run_steps`，便于前端运行轨迹展示。

第三阶段：增强失败与汇总策略（已完成基础结构）

- parallel 支持部分失败：记录失败 step，允许成功成员继续汇总。（已完成）
- 支持指定 `synthesizer_agent_id`，由独立 synthesizer Agent 生成最终回复。（已完成）
- `timeout_seconds` 控制 Team 运行超时，并区分 `timeout` / `cancelled` / `failed` 状态。（已完成）
- 为 run 增加成本汇总字段，并与模型用量统计联动。（基础版已完成）

第四阶段：实时观测与高级拓扑（SSE 基础版已完成）

- 增加 step event SSE，前端运行轨迹从“运行后查看”升级为“运行中实时刷新”。（基础版已完成）
- 引入 Team 模板。（内置模板基础版已完成）
- 引入图工作流 definition schema 和基础拓扑预览。（基础版已完成）
- 引入 A2A 协议。（内部信封基础版已完成）
- 引入自适应拓扑选择。（启发式基础版已完成）

## 12. 风险与约束

- 多 Agent 并行会放大模型成本，需要接入现有模型用量统计。
- 子 Agent 上下文传递过长会导致 token 膨胀，MVP 应做摘要传递。
- parallel 模式需要超时和部分失败策略，否则会被单个慢 Agent 阻塞。
- coordinator 模式依赖结构化输出，必须校验 JSON，不可信输出不能直接执行。
- Team Runtime 应复用 Tool / Plugin / Guard 能力，避免绕过现有安全策略。

## 13. 建议优先级

P0：

- Team CRUD。
- Team 编辑器。
- sequential runtime。
- Chat Team session 接入。

P1：

- parallel runtime。
- synthesizer Agent 汇总。
- team_runs / team_run_steps。
- Team Run 详情页。

P2：

- coordinator。
- critic_loop。
- Team 模板。
- 实时 step 事件流。

P3：

- 图工作流。
- A2A 协议集成。
- 自适应拓扑选择。
- 强化学习调度。