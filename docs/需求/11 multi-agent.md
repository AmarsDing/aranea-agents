# Multi-Agent / Team 编排需求文档

## 1. 背景与目标

当前系统已具备 Agent 管理、Session、Tool、Skill、Plugin、模型管理和运行审计等模块。Multi-Agent 基于 `Team` 概念扩展为**多智能体编排单元**，让用户把多个专业 Agent 组织成可运行、可观测、可配置的协作团队。

核心目标：

- 用户在 UI 中创建 Team，并为 Team 绑定多个 Agent
- 支持多种编排模式：顺序、并行、主控分派、生成-评审闭环、群智协作（Swarm / Adaptive）
- Team 可像单个 Agent 一样在 Chat 中发起会话并运行
- 编排过程可追踪：展示每个子 Agent 的输入、输出、状态、耗时、错误和成本
- 支持 Swarm 动态成员管理、Team 结构导出、运行测试与取消
- 实时观测经 WebSocket Envelope 推送（含子 Agent 流式输出与运行汇总）

## 2. 范围定义

### 2.1 本期已实现

**Team 管理与配置**

- Team 管理页：列表、搜索、新增、编辑、删除、复制
- Team 编辑器：基础信息、成员 Agent、编排模式、并发/超时、Swarm 配置、JSON 预览
- 四个内置模板（顺序 / 并行专家组 / 生成评审 / 主控分派）
- Definition JSON 含 graph 预览节点（前端 schema，非独立 Graph 执行引擎）

**编排运行时**

- 五种后端编排模式：`sequential` / `parallel` / `coordinator` / `critic_loop` / `swarm`
- 前端 UI 以 `adaptive` 对应后端 Swarm 运行时（与 `swarm` 共用 `NewSwarm` 构建）
- Swarm 安全限制：MaxHandoffs / NodeTimeout / RepetitiveHandoff / CrossRequestTransfer
- MemberToolConfig：StreamInner / InnerTextMode / HistoryScope / SkipSummarization
- Critic Loop 支持 ScoreThreshold 结构化评分终止
- Team 成员构建时接入 `PluginsForAgent` 与有效工具集（含 `call_agent`，需 Agent 启用 A2A 工具）

**运行记录与 API**

- `team_runs` / `team_run_steps` 持久化
- Team CRUD + DuplicateTeam
- ListTeamRuns / GetTeamRun / CancelTeamRun / ListTeamRunSteps
- UpdateSwarmMembers / ExportTeamStructure
- RunTeamTest 后端 · CancelTeamRun · GetTeamRunSummary RPC
- team_step_started / team_summary WS · tool_call_count 落库
- Team 管理页 RunTeamTest UI · TeamRuns 汇总与 WS 回放
- Chat Team 成员 strip（`ChatTeamMemberStrip`）

**Chat 与实时观测**

- Chat 选择 Team 创建 `owner_type=team` 会话
- Channel 路由 `default_team_id` 时同样创建 `owner_type=team` 会话（飞书等 IM 入口）；出站为汇总文本，见 [17-channel-agent-team-integration.md](./17-channel-agent-team-integration.md)
- WS Envelope：`team_run_*` / `team_step_started` / `team_step_finished` / `team_summary` / `member_message_*` / `member_delta` / `intent_pass` / `transfer`
- Monitor EventTimeline 可订阅 Team 相关 Envelope

### 2.2 待补全（见开发计划）

- 历史 run 在 `tool_call_count` 字段上线前的步骤工具计数为 0（可选 Usage 回填，TEAM-11）

### 2.3 未来扩展

- A2A 跨框架协议发现与握手（见 [26 a2a-protocol.md](./26%20a2a-protocol.md)）
- Agent Kind `a2a_proxy` 与 LLM Agent A2A Endpoint 设置页
- 图工作流拖拽编辑器、条件分支、后端 DAG 调度
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

## 4. 产品需求

### 4.1 Team 管理页

入口：侧栏 **Team 管理**（`/team`）。

页面能力：

- Team 列表：名称、Key、编排模式、成员数、状态、创建时间
- 搜索与筛选：名称、Key、模式、状态
- 新增 / 编辑 / 删除（默认 Team 不可删）/ 复制
- 运行记录对话框（TeamRunsDialog）：历史 Run、Steps、实时 WS 事件
- **运行测试**（待 UI 接线）：调用 RunTeamTest RPC

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

Swarm 配置（adaptive / swarm 模式）：

- `swarm.max_handoffs`、`node_timeout_seconds`
- `swarm.repetitive_handoff_window`、`repetitive_handoff_min_unique`
- `swarm.cross_request_transfer`

成员工具配置（coordinator 模式）：

- `member_tool_config.*`：stream_inner、inner_text_mode、skip_summarization、history_scope、tool_set_name

### 4.3 Chat 页面 Team 运行

- Team 列表来自 `/v1/teams`
- 选择 Team 后创建 `owner_type=team` 的 Session
- 发送消息携带 `team_id`；后端调用 Team Runner
- 消息区展示最终答案；可查看子 Agent 轨迹（WS `member_*` / `team_step_*`）

### 4.4 编排运行轨迹

**TeamRun**：run id、team id、session id、message id、mode、status、时间、token、成本、topology_json、error_message。

**TeamRunStep**：step id、agent 信息、role、sort_order、status、input/output preview、token、耗时、error。

### 4.5 动态成员管理

- UpdateSwarmMembers：仅 `swarm` / `adaptive` 模式
- 新增成员默认 worker、enabled=true

### 4.6 结构导出

- ExportTeamStructure：入口节点、成员节点、边、面
- coordinator → 星形；swarm/adaptive → 全连接；其他 → 线性

### 4.7 运行管理

- GetTeamRun / CancelTeamRun（running/pending 可取消，经 RunRegistry 中断运行时）
- RunTeamTest：手动测试运行
- 运行结束推送 `team_summary` Envelope（成员 token/耗时汇总）

## 5. 后端接口

### 5.1 Team CRUD

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/v1/teams` | 列出未删除 Team |
| POST | `/v1/teams` | 创建 |
| GET | `/v1/teams/{id}` | 获取 |
| PATCH | `/v1/teams/{id}` | 更新 |
| DELETE | `/v1/teams/{id}` | 软删除 |
| POST | `/v1/teams/{id}/duplicate` | 复制 |

### 5.2 Team 运行

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/v1/team-runs` | 列出运行记录 |
| GET | `/v1/team-runs/{id}` | 运行详情 |
| POST | `/v1/team-runs/{id}/cancel` | 取消运行 |
| GET | `/v1/team-runs/{run_id}/steps` | 运行步骤 |

### 5.3 Team 高级功能

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | `/v1/teams/{team_id}/swarm-members` | Swarm 动态成员 |
| GET | `/v1/teams/{team_id}/structure` | 结构导出 |
| POST | `/v1/teams/{id}/run-test` | 运行测试 |
| GET | `/v1/team-runs/{id}/summary` | 运行结构化汇总 |

## 6. 数据模型

### Team Definition JSON（示例）

```json
{
  "version": 1,
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
  },
  "swarm": {
    "max_handoffs": 10,
    "node_timeout_seconds": 120,
    "repetitive_handoff_window": 3,
    "repetitive_handoff_min_unique": 2,
    "cross_request_transfer": true
  },
  "member_tool_config": {
    "stream_inner": false,
    "inner_text_mode": "default",
    "skip_summarization": false,
    "history_scope": "default",
    "tool_set_name": ""
  }
}
```

## 7. 验收标准

### 已完成

- [x] 创建、编辑、删除、复制 Team
- [x] Team 绑定多个 Agent，Definition 持久化
- [x] Chat 选择 Team 并发送消息
- [x] 五种后端编排模式可执行（含 adaptive ↔ Swarm）
- [x] 每次运行记录 run 与 step 摘要
- [x] 子 Agent 失败时最终结果含失败说明
- [x] SwarmConfig / MemberToolConfig / CriticLoop ScoreThreshold
- [x] UpdateSwarmMembers / ExportTeamStructure
- [x] GetTeamRun / CancelTeamRun（含 RunRegistry 取消）
- [x] RunTeamTest 后端端到端
- [x] WS 推送 `member_message_*` / `team_summary` / `team_step_finished`
- [x] `call_agent` 工具在 Agent 有效工具集启用时可注入

### 待验收

- [ ] 历史 run 工具调用数回填（可选）

### 已完成（Phase 3 — 2026-05-21）

- [x] Team 管理页 RunTeamTest UI
- [x] team_step_started Envelope
- [x] GetTeamRunSummary REST RPC
- [x] 汇总含 tool_call_count
- [x] Chat Team 成员 strip
- [x] Runner persistStep 单测
