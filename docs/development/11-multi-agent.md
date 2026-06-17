# Multi-Agent / Team 编排需求文档

> **编排融合（M53）**：[53 team-graph-orchestration.md](./53%20team-graph-orchestration.md) — Team 与 Graph 统一拓扑、Agent 状态观测、Kanban 看板。
> **开发计划**：[11-multi-agent.development.md](./11-multi-agent.development.md) · **设计文档**：[11-multi-agent.design.md](./11-multi-agent.design.md)

## 1. 背景与目标

当前系统已具备 Agent 管理、Session、Tool、Skill、Plugin、模型管理和运行审计等模块。Multi-Agent 基于 `Team` 概念扩展为**多智能体编排单元**，让用户把多个专业 Agent 组织成可运行、可观测、可配置的协作团队。

核心目标：

- 用户在 UI 中创建 Team，并为 Team 绑定多个 Agent
- 支持多种编排模式：顺序、并行、主控分派、生成-评审闭环、群智协作（Swarm / Adaptive）
- Team 可像单个 Agent 一样在 Chat 中发起会话并运行
- 编排过程可追踪：展示每个子 Agent 的输入、输出、状态、耗时、错误和成本
- 支持 Swarm 动态成员管理、Team 结构导出、运行测试与取消
- 实时观测经 WebSocket 推送（含子 Agent 流式输出与运行汇总）
- Graph 编排画布可视化编辑与编译预览
- 运行观测台：Agent 工作看板、Timeline、HITL 人工审核、任务看板
- 失败策略（FailurePolicy）与熔断器（CircuitBreaker）容错机制

## 2. 范围定义

### 2.1 功能范围

**Team 管理与配置**

- Team 管理页：列表、搜索、新增、编辑、删除、复制、行业分组、拖拽排序
- Team 编辑器：基础信息、成员 Agent、编排模式、并发/超时、Swarm 配置、运行时引擎、失败策略、A2A 协议、JSON 预览
- 编排画布页（TeamOrchestratePage）：Graph 可视化编辑、节点面板、运行时/容错面板、成员看板
- 编译预览（TeamCompilePreview）：实时编译展示拓扑与校验
- 四个内置模板（顺序 / 并行专家组 / 生成评审 / 主控分派）
- Definition JSON 含 embedded graph 节点（agent / task / review / subgraph / function 五种节点类型）

**编排运行时**

- 五种后端编排模式：`sequential` / `parallel` / `coordinator` / `critic_loop` / `swarm`
- 前端 UI 以 `adaptive` 对应后端 Swarm 运行时（与 `swarm` 共用构建路径）
- Graph 为默认执行路径（Native 应急路径已移除）
- Swarm 安全限制：MaxHandoffs / NodeTimeout / RepetitiveHandoff / CrossRequestTransfer
- MemberToolConfig：StreamInner / InnerTextMode / HistoryScope / SkipSummarization
- Critic Loop 支持 ScoreThreshold 结构化评分终止
- FailurePolicy 容错：重试（RetryPolicy）、熔断器（CircuitBreaker）、节点级覆盖
- Team 成员构建时接入插件与有效工具集（含 `call_agent`，需 Agent 启用 A2A 工具）

**运行记录与运行管理**

- 运行记录持久化（TeamRun / TeamRunStep / OrchestrationStep）
- Team CRUD + 复制 + 归档 + 重试
- 运行列表 / 详情 / 取消 / 步骤列表
- 动态成员管理 / 结构导出
- 运行测试 / 运行汇总 / 运行观测台 / 时间线
- HITL 恢复 / Checkpoint 恢复
- 编译预览
- 死信队列（TaskDeadLetter）
- Spirit 集成：列出 Spirit Team / 结果合成

**Chat 与实时观测**

- Chat 选择 Team 创建 `owner_type=team` 会话
- Channel 路由 `default_team_id` 时同样创建 `owner_type=team` 会话（飞书等 IM 入口）；出站为汇总文本，见 [17-channel-agent-team-integration.md](./17-channel-agent-team-integration.md)
- WebSocket 实时推送：运行状态、步骤事件、子 Agent 流式输出、汇总
- 运行观测台（TeamRunObservatoryPage）：Agent 工作看板 / Timeline / Summary / HITL 审核 / 任务看板
- Monitor EventTimeline 可订阅 Team 相关事件

### 2.2 待补全

- 历史 run 在 `tool_call_count` 字段上线前的步骤工具计数为 0（可选 Usage 回填）
- TeamRuns 自动加载汇总（展开 run 时）
- 拖拽排序后端持久化 API（前端已本地排序）

### 2.3 未来扩展

- A2A 跨框架协议发现与握手（见 [26 a2a-protocol.md](./26%20a2a-protocol.md)）
- Agent Kind `a2a_proxy` 与 LLM Agent A2A Endpoint 设置页
- Team 模板后端库 / 自定义模板保存
- 强化学习调度、自动拓扑训练
- 外部 Team 市场或模板商店
- 复杂人工审批流

## 3. 用户场景

### 场景 1：顺序流水线

用户创建「研究报告 Team」：资料收集 → 分析 → 写作 → 校对。Chat 中选择该 Team 输入主题，系统按顺序调用成员，返回最终报告。

### 场景 2：并行评审

用户创建「代码审查 Team」：安全 / 性能 / 可维护性 Agent 并行执行，汇总 Agent 输出统一审查报告。

### 场景 3：生成-评审闭环

用户创建「文案优化 Team」：生成 Agent 产出初稿，评审 Agent 给出评分与修改意见；迭代至 ScoreThreshold 或轮数耗尽。

### 场景 4：群智协作（Adaptive / Swarm）

用户创建「客服 Team」：入口 Agent 处理问题，必要时 `transfer_to_agent` 转交；MaxHandoffs 防止无限转移。前端选择 `adaptive` 模式即启用 Swarm 运行时。

### 场景 5：动态成员管理

运维通过 UpdateSwarmMembers API 在 Swarm / Adaptive 模式下动态增删成员，无需重建 Team。

### 场景 6：结构导出

开发者通过 ExportTeamStructure API 获取 Team 拓扑（节点、边、面），用于可视化或文档生成。

### 场景 7：运行测试

用户在 Team 管理页触发「运行测试」，系统发送测试消息并返回 TeamRun 与助手回复，无需进入 Chat。

### 场景 8：编排画布编辑

用户在编排画布页（TeamOrchestratePage）可视化编辑 Team 拓扑：拖拽节点、调整连线、配置节点属性。编译预览实时展示拓扑校验结果与编译问题。

### 场景 9：运行观测台

用户在运行观测台（TeamRunObservatoryPage）查看 Team 运行实时状态：Agent 工作看板展示各成员状态，Timeline 展示执行时间线，Summary 展示汇总统计。

### 场景 10：HITL 人工审核

Team 运行中遇到 task/review 节点时暂停等待人工审核；审核人在观测台 approve/reject/fallback 后，运行通过 ResumeTeamRunExecution 恢复。

### 场景 11：归档与重试

用户对已完成/失败/取消的 Team 执行归档（ArchiveTeam）或重试（RetryTeam），重试会重置状态并重新启动。

### 场景 12：Spirit 集成

Spirit 会话中列出关联 Team（ListSpiritTeams），并将多个 Team 的结果合成为统一输出（SynthesizeResults）。

## 4. 产品需求

### 4.1 Team 管理页

入口：侧栏 **Team 管理**（`/team`）。

页面能力：

- Team 列表：名称、Key、编排模式、成员数、状态、创建时间
- 搜索与筛选：名称、Key、模式、状态、行业
- 新增 / 编辑 / 删除（默认 Team 不可删）/ 复制 / 归档 / 重试
- 行业分组与拖拽排序
- 运行记录对话框（TeamRunsDialog）：历史 Run、Steps、实时 WS 事件
- 运行测试（TeamTestDialog）：调用 RunTeamTest
- 导航至编排画布页 / 运行观测台

### 4.2 Team 编辑器

基础字段：

- `display_name`、`team_key`（小写字母/数字/连字符）
- `status`：draft / active / archived
- `adk_app_name`：运行时应用名

成员配置：

- 选择已有 Agent；角色 coordinator / worker / synthesizer / critic / generator
- 执行顺序、启用开关

编排配置：

- `mode`：sequential / parallel / coordinator / critic_loop / **adaptive**（UI）↔ swarm 运行时
- `max_concurrency`、`timeout_seconds`
- `synthesizer_agent_id`（parallel）
- `critic_loop.max_iterations`、`critic_loop.score_threshold`
- `intent_anchor_agent_id`（意图锚点成员）

运行时配置：

- `runtime_engine`：graph（默认，唯一执行路径）
- `failure_policy`：重试（RetryPolicy）、熔断器（CircuitBreaker）、节点级覆盖
- `enable_checkpoint`：长任务 checkpoint 支持

Swarm 配置（adaptive / swarm 模式）：

- `swarm.max_handoffs`、`node_timeout_seconds`
- `swarm.repetitive_handoff_window`、`repetitive_handoff_min_unique`
- `swarm.cross_request_transfer`

成员工具配置（coordinator 模式）：

- `member_tool_config.*`：stream_inner、inner_text_mode、skip_summarization、history_scope、tool_set_name

A2A 协议配置：

- `a2a.*`：跨框架协议相关设置

编译预览（TeamCompilePreview）：

- 实时调用 CompileTeamGraph API
- 展示编译后拓扑（节点/边）与校验问题列表

### 4.3 编排画布页

入口：Team 管理页卡片 → 编排画布按钮（`/teams/:teamId/orchestrate`）。

页面能力：

- Graph 可视化编辑器（GraphEditorCanvas）
- 节点详情面板（TeamOrchestrateNodePanel）：名称/角色/输入/做什么/输出
- 运行时与容错面板（TeamOrchestrateRuntimePanel）
- 成员看板（TeamMemberKanban）：按角色分列展示编译节点
- 实时运行模式：有活跃 Run 时自动切换只读 + 实时流

### 4.4 运行观测台

入口：Team 管理页卡片 → 观测台按钮 / TeamRunsDialog → 观测台链接（`/teams/:teamId/runs/:runId/observatory`）。

页面能力：

- Agent 工作看板（OrchestrationKanban）：各成员实时状态
- Timeline（OrchestrationActivityTimeline）：执行时间线
- Summary：运行汇总统计
- HITL 审核（OrchestrationHitlReviewDialog）：approve / reject / fallback
- 任务看板（GraphTaskKanban）：task/review 节点创建的 Task
- ResumeTeamRunExecution：恢复暂停的运行

### 4.5 Chat 页面 Team 运行

- Team 列表来自 `/v1/teams`
- 选择 Team 后创建 `owner_type=team` 的 Session
- 发送消息携带 `team_id`；后端调用 Team Runner
- 消息区展示最终答案；可查看子 Agent 轨迹（WS `member_*` / `team_step_*`）

### 4.6 编排运行轨迹

**TeamRun**：run id、team id、session id、message id、mode、status、时间、token、成本、topology、error_message。

**TeamRunStep**：step id、agent 信息、role、sort_order、status、input/output preview、token、耗时、error。

### 4.7 动态成员管理

- UpdateSwarmMembers：仅 `swarm` / `adaptive` 模式
- 新增成员默认 worker、enabled=true

### 4.8 结构导出

- ExportTeamStructure：入口节点、成员节点、边、面
- coordinator → 星形；swarm/adaptive → 全连接；其他 → 线性

### 4.9 运行管理

- GetTeamRun / CancelTeamRun（running/pending 可取消，经 RunRegistry 中断运行时）
- RunTeamTest：手动测试运行
- 运行结束推送 `team_summary` 事件（成员 token/耗时汇总）

### 4.10 Spirit 集成

- ListSpiritTeams：列出 Spirit 会话关联的 Team
- SynthesizeResults：将多个 Team 的结果合成为统一输出
- ArchiveTeam：归档已完成/失败/取消的 Team
- RetryTeam：重试失败/取消的 Team

## 5. 验收标准

### Team 管理与配置

- 创建、编辑、删除、复制 Team
- Team 绑定多个 Agent，Definition 持久化
- 行业分组与拖拽排序
- 归档与重试 Team

### 编排运行

- 五种后端编排模式可执行（含 adaptive ↔ Swarm）
- 每次运行记录 run 与 step 摘要
- 子 Agent 失败时最终结果含失败说明
- SwarmConfig / MemberToolConfig / CriticLoop ScoreThreshold
- Graph 为唯一执行路径
- embedded graph 五种节点类型（agent / task / review / subgraph / function）
- FailurePolicy 容错（RetryPolicy / CircuitBreaker / 节点级覆盖）

### 运行管理 API

- UpdateSwarmMembers / ExportTeamStructure
- GetTeamRun / CancelTeamRun（含 RunRegistry 取消）
- RunTeamTest 后端端到端
- GetTeamRunSummary 结构化汇总（含 tool_call_count）
- ListSpiritTeams / SynthesizeResults / ArchiveTeam / RetryTeam

### Chat 与实时观测

- Chat 选择 Team 并发送消息
- `call_agent` 工具在 Agent 有效工具集启用时可注入
- Team 管理页 RunTeamTest UI
- Chat Team 成员 strip
- WS 推送 `member_message_*` / `team_summary` / `team_step_started` / `team_step_finished`
- 编排画布页（TeamOrchestratePage）可视化编辑
- 编译预览（CompileTeamGraph + TeamCompilePreview）
- 运行观测台（TeamRunObservatoryPage）
- HITL 人工审核 + ResumeTeamRunExecution
- 死信队列（TaskDeadLetter）

### 待验收

- 历史 run 工具调用数回填（可选）
- TeamRuns 自动加载汇总（展开 run 时）
- 拖拽排序后端持久化 API
