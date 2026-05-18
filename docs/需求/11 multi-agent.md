# Multi-Agent / Team 编排需求文档

## 1. 背景与目标

当前系统已具备 Agent 管理、Agent 分类、Team 基础数据、Session、Tool、Skill、Plugin、模型管理和运行审计等模块。Multi-Agent 基于现有 `Team` 概念扩展为"多智能体编排单元"，让用户可以把多个专业 Agent 组织成一个可运行、可观测、可配置的协作团队。

核心目标：

- 让用户在 UI 中创建 Team，并为 Team 绑定多个 Agent
- 支持五种编排模式：顺序（sequential）、并行（parallel）、主控分派（coordinator）、生成-评审闭环（critic_loop）、群智协作（swarm）
- Team 可以像单个 Agent 一样发起会话，并在 Chat 页面中运行
- 编排过程可追踪：展示每个子 Agent 的输入、输出、状态、耗时、错误和成本
- 支持 Swarm 动态成员管理和 Team 结构导出
- 支持 Critic Loop 结构化评分终止条件

## 2. 范围定义

### 本期已实现

- Team 管理页真实化：列表、搜索、新增、编辑、删除、复制
- Team 编辑器：基础信息、成员 Agent 配置、编排模式、并发上限、超时、JSON 预览
- Chat 页支持选择 Team 发起会话，后端按 Team 定义执行多 Agent 编排
- 五种编排模式全部实现：
  - `sequential`：基于 chainagent 按顺序执行，前一个 Agent 事件传递给下一个
  - `parallel`：基于 parallelagent 并行处理，synthesizer Agent 汇总结果
  - `coordinator`：基于 team.New，协调者 Agent 调度成员作为工具
  - `critic_loop`：基于 cycleagent + escalationFunc，生成-评审循环，支持 ScoreThreshold
  - `swarm`：基于 team.NewSwarm，成员间 transfer_to_agent 传递控制权
- 运行记录落库（team_runs / team_run_steps），支持查看编排轨迹
- SwarmConfig 安全限制：MaxHandoffs / NodeTimeout / RepetitiveHandoff
- CrossRequestTransfer 跨请求转移
- SwarmHandoffInputBuilder 自定义转移输入
- MemberToolConfig 成员工具配置：StreamInner / InnerTextMode / HistoryScope
- 动态成员管理：UpdateSwarmMembers API
- 结构导出：ExportTeamStructure API
- GetTeamRun / CancelTeamRun / RunTeamTest API
- 实时 step event SSE 基础链路
- Team 模板（四个内置模板）
- 图工作流 schema 和基础预览
- A2A 内部信封基础版

### 未来扩展

- A2A 跨框架协议发现与握手
- 强化学习调度、PPO 或自动拓扑训练
- 图工作流拖拽编辑器、条件分支执行、后端 DAG 调度
- 自动生成新 Agent
- 外部 Team 市场或模板商店
- 复杂人工审批流
- Team 模板后端库 / 自定义模板保存
- 实时 SSE 增强：step_started / 进度百分比 / 事件回放与断线续传
- A2A call_agent 工具注入
- Team 运行结果结构化汇总（成员贡献度、工具调用统计）

## 3. 用户场景

### 场景 1：顺序流水线

用户创建"研究报告 Team"，包含：资料收集 Agent → 分析 Agent → 写作 Agent → 校对 Agent。用户在 Chat 中选择该 Team 输入主题，系统按顺序调用成员，最终返回校对后的报告。

### 场景 2：并行评审

用户创建"代码审查 Team"，包含：安全审计 Agent、性能分析 Agent、可维护性审查 Agent、汇总 Agent。系统并行调用前三个 Agent，再由汇总 Agent 输出统一审查报告。

### 场景 3：生成-评审闭环

用户创建"文案优化 Team"，包含生成 Agent 和评审 Agent。生成 Agent 产出初稿，评审 Agent 给出修改意见和评分；系统最多迭代 N 轮，直到评分达到 ScoreThreshold 或轮数耗尽。

### 场景 4：群智协作

用户创建"客服 Team"，包含多个专业客服 Agent。用户问题先由入口 Agent 处理，若需转交则通过 transfer_to_agent 传递给更专业的 Agent，直到问题解决。支持 MaxHandoffs 限制防止无限转移。

### 场景 5：动态成员管理

运维人员在运行期间通过 UpdateSwarmMembers API 动态添加或移除 Swarm Team 的成员 Agent，无需重建 Team。

### 场景 6：结构导出

开发者通过 ExportTeamStructure API 获取 Team 的编排拓扑结构（节点、边、面），用于可视化展示或文档生成。

## 4. 产品需求

### 4.1 Team 管理页

入口：侧栏 `Team 管理`。

页面能力：

- Team 列表：展示名称、Key、编排模式、成员数、启用状态、最近运行状态、创建时间
- 搜索与筛选：按名称、Key、模式、启用状态筛选
- 新增 Team：创建基础信息和编排配置
- 编辑 Team：修改成员、模式、配置 JSON
- 删除 Team：软删除，默认 Team 不允许删除
- 复制 Team：复制定义并生成新 Key
- 运行测试：手动触发 Team 测试运行

### 4.2 Team 编辑器

基础字段：

- `display_name`：Team 名称
- `team_key`：唯一标识，支持小写字母、数字、连字符
- `status`：draft / active / archived
- `adk_app_name`：运行时应用名，可自动生成

成员配置：

- 选择已有 Agent
- 设置成员角色：coordinator / worker / synthesizer / critic / generator
- 设置执行顺序
- 设置是否启用

编排配置：

- `mode`：编排模式（sequential / parallel / coordinator / critic_loop / swarm）
- `max_concurrency`：并行上限
- `timeout_seconds`：单次运行超时
- `synthesizer_agent_id`：汇总 Agent
- `critic_loop.max_iterations`：循环上限
- `critic_loop.score_threshold`：评分阈值

Swarm 配置：

- `swarm.max_handoffs`：最大转移次数
- `swarm.node_timeout_seconds`：节点超时
- `swarm.repetitive_handoff_window`：重复转移检测窗口
- `swarm.repetitive_handoff_min_unique`：重复转移最小唯一数
- `swarm.cross_request_transfer`：跨请求转移

成员工具配置：

- `member_tool_config.stream_inner`：内部流式输出
- `member_tool_config.inner_text_mode`：内部文本模式
- `member_tool_config.skip_summarization`：跳过汇总
- `member_tool_config.history_scope`：历史范围
- `member_tool_config.tool_set_name`：工具集名称

### 4.3 Chat 页面 Team 运行

- Team 列表从 `/api/v1/teams` 获取
- 选择 Team 后，创建 `owner_type=team` 的 session
- 发送消息时携带 `team_id`
- 后端识别 Team 会话并调用 Team 编排运行时
- 前端消息区展示最终答案；详情面板可查看子 Agent 轨迹

### 4.4 编排运行轨迹

每次 Team 运行产生一条 TeamRun：

- run id / team id / session id / message id / mode / status
- started_at / finished_at / duration_ms
- token_in / token_out / cost_micro_usd
- error_message / topology_json

每个子 Agent 调用产生 TeamRunStep：

- step id / run id / agent id / agent_key / agent_name / role
- sort_order / status
- input_preview / output_preview
- token_in / token_out / cost_micro_usd / duration_ms
- error_message / started_at / finished_at

### 4.5 动态成员管理

- UpdateSwarmMembers API：支持添加和移除 Swarm/Adaptive 模式的成员 Agent
- 仅 Swarm 和 Adaptive 模式支持动态成员管理
- 添加的成员默认角色为 worker，启用状态为 true

### 4.6 结构导出

- ExportTeamStructure API：返回 Team 编排拓扑结构
- 包含入口节点、所有成员节点、节点间边关系
- 不同编排模式生成不同的拓扑结构：
  - coordinator：星形拓扑，coordinator 为中心
  - swarm/adaptive：全连接拓扑，成员间可互相转移
  - sequential/parallel/critic_loop：线性拓扑

### 4.7 运行管理

- GetTeamRun API：获取单条运行详情
- CancelTeamRun API：取消正在运行的 Team Run（仅 running/pending 状态可取消）
- RunTeamTest API：手动触发 Team 测试运行

## 5. 后端接口

### 5.1 Team CRUD

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/v1/teams` | 列出所有未删除 Team |
| POST | `/v1/teams` | 创建 Team |
| GET | `/v1/teams/{id}` | 获取单个 Team |
| PATCH | `/v1/teams/{id}` | 更新 Team |
| DELETE | `/v1/teams/{id}` | 软删除 Team |
| POST | `/v1/teams/{id}/duplicate` | 复制 Team |

### 5.2 Team 运行

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/v1/team-runs` | 列出 Team 运行记录 |
| GET | `/v1/team-runs/{id}` | 获取单条运行详情 |
| POST | `/v1/team-runs/{id}/cancel` | 取消正在运行的 Team Run |
| GET | `/v1/team-runs/{run_id}/steps` | 列出运行步骤 |

### 5.3 Team 高级功能

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | `/v1/teams/{team_id}/swarm-members` | Swarm 动态成员管理 |
| GET | `/v1/teams/{team_id}/structure` | 导出 Team 结构快照 |
| POST | `/v1/teams/{id}/run-test` | 手动触发 Team 测试运行 |

## 6. 数据模型

### Team 定义 JSON

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

### 已完成验收

- [x] 用户能创建、编辑、删除 Team
- [x] Team 可绑定多个现有 Agent
- [x] Team 定义能保存到后端并刷新恢复
- [x] Chat 中可以选择 Team 并发送消息
- [x] 五种编排模式可完整执行
- [x] 每次运行至少记录 run 和 step 摘要
- [x] 子 Agent 失败时，最终结果包含失败说明
- [x] SwarmConfig 安全限制已实现
- [x] MemberToolConfig 成员工具配置已实现
- [x] 动态成员管理 API 已实现
- [x] 结构导出 API 已实现
- [x] GetTeamRun / CancelTeamRun API 已实现
- [x] escalationFunc 支持 ScoreThreshold 结构化评分

### 待验收

- [ ] RunTeamTest 端到端测试通过
- [ ] A2A call_agent 工具注入到 Agent 工具集
- [ ] Team 对话发射 member_* SSE 事件
- [ ] Team 运行结果结构化汇总 API
