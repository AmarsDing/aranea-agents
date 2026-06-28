# Chat UI 统一设计草稿

> **状态**：草稿（待用户审查）
> **目的**：统一 chat UI 的需求和设计，解决现有文档与代码的偏离问题
> **审查后**：分裂为 `1-chat.md`（需求）+ `1-chat.design.md`（设计）+ `1-chat.development.md`（开发计划）

---

## 一、需求修正与确认

### 1.1 核心修正点

| 维度 | 原需求/设计 | 修正后 |
|------|-----------|--------|
| 团队排序 | 按活跃度 `running→pending→interrupted→completed→failed→cancelled` | 按 graph 流程图顺序（后端 WS 指令创建，按对话历史顺序放置） |
| 任务面板 | 团队卡片列表 | 三层结构：任务计划面板 → Graph 流程图 → Team/Agent 任务栏 |
| 面板产生 | 每次 team 组建产生新面板 | 任务计划面板固定位置，不重复产生，通过 WS 更新状态 |
| 计划变更 | 未明确 | 执行中可变更计划，在原面板更新 |
| Team 任务栏 | 仅展示状态/进度 | 完整交互：暂停/重试按钮 + 用户补充输入框 + 树形展开 |
| MaxDepth | 硬编码 = 2 | 由 agent 配置「协作能力 → 最大生成深度」决定 |
| 排序字段 | 全局 Seq 递增 | 用现有 Timestamp 字段，不增加新字段 |
| 异常处理 | 未明确 | 失败：手动重试；卡住：先记录不实现 |

### 1.2 核心架构原则（Agent 统一性）

**所有 agent 本质相同**：
- 精灵（父节点）和子 agent（包裹在 team/子agent 外衣下）都是 agent
- 会话输出内容和展现形式相同（thinking + action + reply）
- 后端交互逻辑相同（ActivityProjector 路径）
- 区别仅在于父子关系（`parentActivityId`）和深度（`agent_depth`）

**推论**：
- ActivityStream 递归渲染是正确方向
- Team 在 UI 上不作为独立层级，而是子 agent 的分组容器
- 父子关系通过 `parentActivityId` + `SpiritSessionID` 表达

### 1.3 精灵的三种 UI 展示路径

| 路径 | 触发条件 | UI 展示 |
|------|---------|---------|
| 简单对话 | 精灵判断为简单任务（StrategyDirect） | thinking + reply（无 plan/graph/team） |
| agent-card | 精灵直接调用子 agent（subagent_spawn） | 简化版卡片，展开后显示 thinking/action/reply |
| team-card | 精灵组建 team | 完整团队卡片（头部+中部+尾部），展开后显示成员列表 |

---

## 二、整体链路设计

### 2.1 单 turn 内的 UI 演进时序

```
用户消息（task）
    ↓
Spirit thinking（思考评估，可折叠）
    ↓
任务计划面板（plan，固定位置，不重复产生）
    ├── 任务拆解列表
    ├── 依赖关系（DAG）
    └── 计划变更时在原面板更新
    ↓
Graph 流程图（graph_stage，独立显示）
    ↓
Team 任务栏 × N（组建几个团队显示几个）
    或 Agent 卡片 × N（直接调用子 agent）
    ↓
Spirit 最终总结（reply，is_final=true）
```

### 2.2 数据流（端到端）

```
[1] 用户发送消息
    ↓ HTTP POST /v1/chat/sessions/{id}/messages
[2] Spirit Orchestrator 接收
    ↓ 调用 LLM 评估
[3] LLM 输出 thinking + plan
    ↓ ActivityProjector 投影为 Activity 事件
    ↓ ActivityEventSequencer 单 publish worker
[4] Bus.Publish → WS fanout + async persist
[5] 前端 WS 接收 → useActivityTimeline.handleActivityEvent
    ↓ 按 parentActivityId 构建 ActivityTree
[6] ActivityStream 渲染：
    ├── task → UserMessageBubble
    ├── thinking → ThinkingBlock（可折叠）
    ├── plan → PlanBlock（任务计划面板）
    ├── graph_stage → GraphStageBlock（流程图）
    ├── team_stage → TeamCard（团队任务栏）
    ├── subagent → AgentCard（子 agent 卡片）
    └── reply(is_final) → ReplyBlock（最终总结）
```

---

## 三、数据模型

### 3.1 Activity 模型（核心字段）

```go
type Activity struct {
    ID                string    // 唯一标识
    SessionID         string    // 当前 session
    SpiritSessionID   string    // 根 spirit session（跨 session 聚合）
    ParentActivityID  string    // 父 activity（构建树）
    TurnID            string    // 所属 turn
    Kind              string    // activity 类型
    Status            string    // activity 状态
    Timestamp         time.Time // 事件产生时间（纳秒精度，用于排序）
    
    // 业务字段
    AgentKey          string    // 当前 agent 标识
    TeamID            string    // 所属 team ID（可空）
    Stage             string    // 阶段标识（team_stage/graph_stage 用）
    
    // 工具字段（kind=action）
    ToolName          string
    ToolCallID        string
    ToolArguments     json.RawMessage
    ToolResult        json.RawMessage
    
    // 内容字段
    Content           string    // 文本内容
    Meta              map[string]any  // 元数据（is_final, members, progress 等）
}
```

**关键约束**：
- **不使用全局 Seq**——用 `Timestamp` 排序（纳秒精度，单 publish worker 保证单调递增）
- **所有 direct-publish 事件必须填 `SpiritSessionID`**（修复 Team/Graph 事件当前未填的问题）

### 3.2 ActivityKind 枚举（10 种）

| Kind | 用途 | 持久化 | UI 组件 |
|------|------|--------|---------|
| `task` | 用户消息/turn 容器 | ✅ | UserMessageBubble |
| `thinking` | 推理过程 | ✅ | ThinkingBlock（可折叠） |
| `action` | 工具调用 | ✅ | ActionBlock（可折叠） |
| `reply` | 回复内容 | ✅ | ReplyBlock（始终展开） |
| `plan` | 任务计划面板 | ✅ | PlanBlock（固定位置） |
| `graph_stage` | Graph 流程图 | ✅ | GraphStageBlock |
| `team_stage` | Team 阶段 | ✅ | TeamCard |
| `session` | 子 session 创建 | ✅ | AgentCard（subagent_spawn） |
| `notice` | 系统通知 | ✅ | NoticeBlock |
| `confirm` | 待确认 | ✅ | ConfirmBlock |

**移除**：
- ~~`sub_task_board`~~（Phase 3 已移除，改用 `parentActivityId` 递归）
- ~~`error`~~（统一通过 `task.failed` 表达）
- ~~`end`~~（统一通过 `team_stage.completed` / `reply.completed` 表达）
- ~~全局 `Seq` 字段~~（用 `Timestamp` 排序）

### 3.3 排序规则

**核心原则**：用现有 `Timestamp` 字段排序，不增加任何新字段。

**排序逻辑**（前端 + 后端一致）：

```
1. 按 TurnID 分组（每个 turn 独立）
2. 在 turn 内，按 ParentActivityID 构建树
3. 同一父节点下的子节点，按 Timestamp ASC 排序
4. 特殊规则：
   - kind=task（用户消息）必排第一
   - kind=reply && meta.is_final=true 必排最后
```

**后端保证**：
- 同一 session 内的事件由单 publish worker 串行处理，Timestamp 单调递增
- Timestamp 在事件产生时设置（不是发送时）
- 持久化时保留 Timestamp

**DB 查询**：

```sql
-- 替代原来的 ORDER BY seq ASC, timestamp ASC
SELECT * FROM activities 
WHERE spirit_session_id = ? 
ORDER BY turn_id ASC, parent_activity_id ASC, timestamp ASC;
```

**前端处理**：
- WS 推送：按 Timestamp 排序插入（不依赖到达顺序）
- 历史加载：按 (TurnID, ParentActivityID, Timestamp) 排序

**为什么不需要全局 Seq**：
- 同一 session 内的事件是顺序产生的（单 publish worker 保证）
- 不同 session 的事件通过 ParentActivityID 分到不同子树，互不干扰
- Timestamp 纳秒精度足够区分同一 session 内的事件
- 跨 session 聚合时，通过树形结构组织，不需要全局排序

### 3.4 Session 树模型

```
Spirit Session (root, AgentDepth=0)
├── Team Session A (AgentDepth=1)
│   ├── SubAgent Session A.1 (AgentDepth=2)
│   └── SubAgent Session A.2 (AgentDepth=2)
└── Agent Session B (AgentDepth=1, subagent_spawn)
```

**字段**：
- `parent_session_id`：父 session ID
- `root_session_id`：根 spirit session ID
- `agent_depth`：当前深度
- `session_type`：spirit / team / agent / standalone
- `member_agent_key`：成员 agent 标识
- `max_agent_depth`：从 agent 配置读取（协作能力 → 最大生成深度）

**MaxDepth 配置**：
- 字段位置：`AgentRuntimeSetting.MaxSessionDepth`（agent 配置 → 协作能力 → 最大生成深度）
- 读取方式：从 agent 配置读取，每个 agent 可独立配置
- 默认值：2（Spirit → Team → Member）
- 超出限制：返回明确错误，不静默失败

---

## 四、UI 组件设计

### 4.1 UI 层级（修正后）

```
Spirit (agent, depth=0)
├── thinking / action / reply（精灵自己的会话）
├── plan（任务计划面板，固定位置）
├── graph_stage（流程图，独立显示）
│
├── team-card（团队容器，depth=1）
│   ├── team 标签（头部+中部+尾部）
│   └── 团队成员（子 agent, depth=1）
│       ├── member 头部（avatar + name + status）
│       └── 展开后：
│           ├── thinking（折叠）
│           ├── action（折叠）
│           └── reply（展开）
│
└── agent-card（精灵直接调用子 agent，depth=1）
    ├── agent 标签（简化版：agent 名称 + 状态 + 时间）
    └── 展开后：
        ├── thinking（折叠）
        ├── action（折叠）
        └── reply（展开）
```

**关键修正**：
- team-card 是**容器**，包含 team 标签 + 成员列表
- 成员（子 agent）在 team-card 内部
- 成员展开后是 thinking/action/reply 序列
- 如果成员还调用子 agent（depth=2），递归 team-card 或 agent-card（受 MaxDepth 限制）

### 4.2 team-card 布局设计

```
┌──────────────────────────────────────────────────────────────────────┐
│ team-card 长条                                                        │
│ ┌─────────────┬──────────────────────────────┬────────────────────┐  │
│ │   头部 20%    │        中部 60%              │     尾部 20%        │  │
│ ├─────────────┼──────────────────────────────┼────────────────────┤  │
│ │             │                              │                    │  │
│ │  团队名称    │  ┌────────────────────────┐ │  ┌──────────────┐  │  │
│ │  ───────    │  │ 成员头像+名称 (1/3)     │ │  │ [对话框...]   │  │  │
│ │  任务名称    │  │ [G1][G2][G3]           │ │  │      [发送]   │  │  │
│ │  ───────    │  └────────────────────────┘ │  │              │  │  │
│ │  创建时间    │  ┌────────────────────────┐ │  │ [⏸停止/▶恢复] │  │  │
│ │  10:30:00   │  │ 进度条│状态│耗时 (2/3)  │ │  │              │  │  │
│ │             │  │ [███░]│运行中│2m30s    │ │  │              │  │  │
│ │             │  │  3:1:1 比例            │ │  │              │  │  │
│ │             │  └────────────────────────┘ │  │              │  │  │
│ └─────────────┴──────────────────────────────┴────────────────────┘  │
│                                                                       │
│ ── 展开后（点击 team-card 或尾部箭头）────────────────────────────── │
│                                                                       │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ [G1] 成员1（avatar + name + status）                            │ │
│ │   ├─ 🧠 thinking（折叠）                                       │ │
│ │   ├─ ⚡ action: file_read（折叠）                              │ │
│ │   └─ 💬 reply（展开）                                          │ │
│ │                                                                │ │
│ │ [G2] 成员2（avatar + name + status）                          │ │
│ │   └─ ...                                                       │ │
│ │                                                                │ │
│ │ [G3] 成员3（avatar + name + status）                          │ │
│ │   └─ ...                                                       │ │
│ └────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────┘
```

**头部（20%）**：上中下三部分（1:1:1）
- 上：团队名称
- 中：任务名称
- 下：任务创建时间

**中部（60%）**：上下两部分（1:2）
- 上：团队成员头像和名称
- 下：进度条:状态:耗时 = 3:1:1

**尾部（20%）**：
- 对话框（收缩状态，点击横向展开，展开后显示发送按钮）
- 停止/恢复按钮

**尾部对话框交互**：
- 默认：收缩状态，显示"💬 补充信息..."提示文字
- 点击：横向展开，显示完整输入框
- 展开后：输入框后面显示"发送"按钮
- 发送后：清空输入框，保持展开状态

**停止/恢复按钮**：
- running 状态：显示"⏸ 停止"
- interrupted 状态：显示"▶ 恢复"
- completed/failed 状态：隐藏

**展开/折叠**：
- 点击 team-card 头部或中部区域 → 展开/折叠成员列表
- running 状态默认展开
- 终态默认折叠

### 4.3 agent-card 布局设计（简化版，含补充输入框）

```
┌──────────────────────────────────────────────────────────────────────┐
│ agent-card 简化版                                                     │
│ ┌──────────────────────────────────────┬────────────────────────────┐ │
│ │  头部 80%                             │  尾部 20%                   │ │
│ ├──────────────────────────────────────┼────────────────────────────┤ │
│ │  [avatar] Agent 名称  [status badge] │  [⏸停止/▶恢复]              │ │
│ │  创建时间 10:30:00                    │  [💬 补充信息...] [发送]     │ │
│ └──────────────────────────────────────┴────────────────────────────┘ │
│                                                                       │
│ ── 展开后 ────────────────────────────────────────────────────────── │
│                                                                       │
│ ├─ 🧠 thinking（折叠）                                              │
│ ├─ ⚡ action: tool_name（折叠）                                      │
│ └─ 💬 reply（展开）                                                  │
└──────────────────────────────────────────────────────────────────────┘
```

**与 team-card 的区别**：
- 无团队信息（团队名称、成员列表）
- 无进度条（单个 agent，直接显示状态）
- **保留用户补充输入框**（与 team-card 一致的交互：收缩状态 → 点击横向展开 → 显示发送按钮）
- 头部显示 agent 名称 + 状态 + 时间
- 展开后直接显示 thinking/action/reply

**尾部对话框交互**（与 team-card 一致）：
- 默认：收缩状态，显示"💬 补充信息..."提示文字
- 点击：横向展开，显示完整输入框 + 发送按钮
- 发送后：清空输入框，保持展开状态
- 触发 `POST /v1/agents/{id}/inject` 携带 `{message: "..."}`

### 4.4 任务计划面板（PlanBlock）

**作用**：
1. **为 agent 执行提供清晰的任务执行指导**——在 session 对话记忆中保持任务的明确和方向
2. **为用户指明任务的进度**——提高可观测性

**作用范围**：每个 turn 独立一个任务计划面板

**固定语义**：
- 同一 turn 内，只产生一个任务计划面板
- 后续 plan 更新事件在原面板更新，不产生新面板
- 通过 `plan.id` 或 `parentActivityId` 关联到原面板

**折叠行为**：
- **支持折叠**（用户可手动折叠/展开）
- 默认展开（任务进行中）
- 任务全部完成后，可自动折叠为摘要（显示"✅ N 项任务已完成"）
- 折叠状态下，显示进度摘要（X/N 已完成）
- 展开后显示完整任务列表 + 状态 + 依赖关系

**状态更新机制**（A 选项：由执行者发出）：
- 每个 plan item 对应一个 team 或 agent
- team_stage / agent 执行状态变化时，更新对应 plan item 状态
- WS 推送 `team_stage.updated` 事件携带 `team_id` + `status`
- 前端按 `team_id` 匹配更新 plan item 状态

**Plan item 状态机**：
```
pending → running → completed
              ↘ → failed
              ↘ → skipped
              ↘ → cancelled
```

**计划变更处理**：
- 不替换整个面板，而是 diff 更新
- 新增的 task item 标记为"➕ 新增"
- 删除的 task item 标记为"⊘ 已移除"（保留可见，灰显）
- 修改的 task item 标记为"✏️ 已变更"

### 4.5 Graph 流程图（GraphStageBlock）

**位置**：在 plan 之后、team 之前独立显示

**时机**：Spirit 完成 team 分配后，发送 `graph_stage.created` 事件时创建

**多 graph 处理**：
- 单 turn 内通常一个 graph（team 依赖关系）
- 如果 turn 内有多次重新分配，更新原 graph
- 跨 turn 的 graph 各自独立

**Graph 节点状态**：
| 节点状态 | 视觉 | WS 事件 |
|---------|------|---------|
| idle | 灰色 | graph_node_start 未触发 |
| running | 蓝色 + pulse | `graph_stage.updated` (stage=running) |
| success | 绿色 ✓ | `graph_stage.completed` |
| error | 红色 ✗ | `graph_stage.failed` |
| interrupted | 黄色 ⏸ | `team_stage.updated` (stage=interrupted) |

### 4.6 折叠规则（按需求 §6.2.3）

| 节点类型 | 折叠行为 |
|---------|---------|
| thinking | 完成后自动折叠，进行中展开 |
| action | 完成后自动折叠，进行中展开 |
| task | 始终展开 |
| reply | 始终展开 |
| team-card | running 默认展开，终态默认折叠，可手动切换 |
| agent-card | 同 team-card |
| plan | **支持折叠**：进行中默认展开，全部完成后自动折叠为摘要（X/N），可手动切换 |
| graph_stage | 始终展开 |

---

## 五、Team 任务栏交互设计

### 5.1 进度计算（简单实现）

**维度**：子任务完成数 X/N

**计算方式**：
- total = team 成员总数
- completed = 已完成成员数（status=completed）
- progress = completed / total * 100%

**后端字段**（team_stage.updated 事件携带）：
```json
{
  "kind": "team_stage",
  "stage": "executing",
  "meta": {
    "team_id": "team_xxx",
    "members": [
      {"agent_key": "g1", "status": "completed"},
      {"agent_key": "g2", "status": "running"},
      {"agent_key": "g3", "status": "pending"}
    ],
    "completed_count": 1,
    "total_count": 3,
    "progress_pct": 33
  }
}
```

### 5.2 暂停/恢复

**暂停按钮**：
- 触发 `POST /v1/teams/{id}/pause`
- Team 状态 → `interrupted`
- WS 推送 `team_stage.updated` (stage=interrupted)
- 按钮变为"▶ 恢复"

**恢复按钮**：
- 触发 `POST /v1/teams/{id}/resume`
- Team 状态 → `running`
- WS 推送 `team_stage.updated` (stage=running)
- 按钮变为"⏸ 暂停"

### 5.3 重试

**触发条件**：team 处于 `failed` / `interrupted` 状态

**行为**：
- 触发 `POST /v1/teams/{id}/retry`
- 重新启动 team，保留原 plan
- WS 推送 `team_stage.updated` (stage=running)

### 5.4 用户补充信息

**交互**：
- 用户在 team-card 尾部输入框输入文本
- 点击发送按钮
- 触发 `POST /v1/teams/{id}/inject` 携带 `{message: "..."}`
- Team 接收后融入执行上下文
- WS 推送 `team_stage.updated` (meta.last_inject="用户补充: ...")

---

## 六、异常处理设计

### 6.1 Team 失败

**触发**：
- Team 执行过程中发生错误
- WS 推送 `team_stage.failed` 携带 `error_message`

**UI 表现**：
- Team 任务栏状态 → ❌ 失败（红色）
- 显示错误信息（可展开查看详情）
- 显示"🔄 重试"按钮（手动重试）

**处理策略**：
- **手动重试**（不自动重试，避免无限循环）
- 由用户决定重试/跳过/取消

### 6.2 Member 失败

**触发**：
- 成员执行过程中发生错误
- WS 推送 `action.failed` 或 `reply.failed`

**UI 表现**：
- 成员子任务面板标记失败节点
- Team 自治决策：跳过该成员 / 重新分配 / 标记 team 失败

**处理策略**：
- **不自动重试 member**
- 由 Team 自治决策

### 6.3 卡住场景（先记录，不实现）

**卡住定义**（待实现）：
- Team 超过 N 秒（默认 120s）无任何 WS 事件
- 或 Member 超过 M 秒（默认 60s）无 thinking/action/reply
- **主要卡在工具执行上**

**本期处理**：
- 仅记录场景，不做具体实现
- 后续迭代再设计心跳检测 + 卡住告警

---

## 七、历史加载设计

### 7.1 加载策略

**只加载 spirit 根 session 事件，子 session 事件按需懒加载**

**流程**：
1. 用户进入 spirit session
2. 调用 `ListBySession(spiritSessionID)` 加载 spirit 根 session 的所有 activity
3. 按 `parentActivityId` 构建 ActivityTree
4. 按 ActivityStream 渲染规则恢复 UI
5. 已完成 team 默认折叠，进行中 team 默认展开

### 7.2 子 session 懒加载

**触发**：
- 用户点击 team-card 展开成员列表
- 或用户点击 agent-card 展开子 agent 会话

**流程**：
1. 前端检测到 team-card / agent-card 展开
2. 检查该 team/agent 的子 session activity 是否已加载
3. 未加载 → 调用 `ListBySession(teamSessionID)` 或 `ListBySession(agentSessionID)`
4. 加载完成后，合并到 ActivityTree
5. 渲染成员/子 agent 的 thinking/action/reply

### 7.3 后端修复要求

**direct-publish 事件必须填 `SpiritSessionID`**：

| 事件来源 | 当前状态 | 修复要求 |
|---------|---------|---------|
| Spirit (spirit_team.go) 生成 | SessionID 空 → Bus 兜底规范化 | ✅ 已正确 |
| **Team (runner_helpers.go) 生成** | **SessionID=team session ID, SpiritSessionID 空** | ❌ 必须修复 |
| **Graph (event_bridge.go) 生成** | **SessionID=graph session ID, SpiritSessionID 空** | ❌ 必须修复 |
| Projector agent 事件 | SessionID=worker session, SpiritSessionID 填充 | ✅ 已正确 |

**修复方式**：
- Team 事件生成时，从 `run.SpiritSessionID` 回填 `SpiritSessionID`
- Graph 事件生成时，从 `graph.SpiritSessionID` 回填 `SpiritSessionID`

---

## 八、WS 协议流

### 8.1 事件类型矩阵

| 阶段 | WS 事件 | Activity Kind | 持久化 | UI 更新 |
|------|---------|--------------|--------|---------|
| 用户消息 | `task.created` | task | ✅ | UserMessageBubble |
| Spirit 思考 | `thinking.streaming/done` | thinking | ✅ | ThinkingBlock |
| 任务计划 | `plan.created/updated` | plan | ✅ | PlanBlock（固定面板，更新状态） |
| 计划变更 | `plan.updated` | plan | ✅ | 原面板更新（不产生新面板） |
| Graph 创建 | `graph_stage.created` | graph_stage | ✅ | GraphStageBlock |
| Graph 节点 | `graph_stage.updated` | graph_stage | ✅ | 流程图节点状态更新 |
| Team 组建 | `team_stage.created` (stage=assembled) | team_stage | ✅ | TeamCard 出现 |
| Team 进度 | `team_stage.updated` | team_stage | ✅ | 进度条/状态更新 |
| 成员执行 | `thinking/action/reply` (member) | thinking/action/reply | ✅ | 树形展开后显示 |
| Team 完成 | `team_stage.completed` | team_stage | ✅ | TeamCard 标记完成 |
| 子 agent 创建 | `session.created` (subagent_spawn) | session | ✅ | AgentCard 出现 |
| 最终总结 | `reply.completed` (is_final=true) | reply | ✅ | ReplyBlock（必排最后） |

### 8.2 direct-publish 事件持久化

**路径**：
- Spirit 生成的事件 → Bus.Publish → 异步 UpsertActivity
- Team/Graph 生成的事件 → Bus.Publish → 异步 UpsertActivity（必须填 SpiritSessionID）

**Bus 层规范化**（保留现有逻辑）：
- `SessionID` 为空时，用 `SpiritSessionID` 兜底
- chat 域事件异步持久化（无重试，无 dead-letter）

---

## 九、待实现项与已知问题

### 9.1 待实现项

| 项 | 说明 | 优先级 |
|----|------|--------|
| 卡住检测 | 工具执行卡住场景，先记录不实现 | P2（后续迭代） |
| MaxDepth 配置 | agent 配置 → 协作能力 → 最大生成深度 | P0 |
| Team/Graph 事件 SpiritSessionID 填充 | 修复跨 session 聚合盲区 | P0 |
| 全局 Seq 移除 | 用 Timestamp 排序替代 | P1（兼容期） |

### 9.2 已知问题

| 问题 | 影响 | 解决方案 |
|------|------|---------|
| 文档 59-chat-ui-optimization.md L592、L733 引用不存在的 TaskBoard/SubTaskBoard | 违反 DOC-SYNC-6 | 同步更新为 ActivityStream + TeamCard/AgentCard |
| project_memory.md "sub_task_board 递归 2 层"描述过时 | 误导开发 | 更新为 parentActivityId 递归 + MaxDepth 配置 |
| 历史 sub_task_board Activity 数据 | 显示为错误框 | 提供 legacy 兼容渲染或迁移 |

### 9.3 待确认问题

**已全部确认**（2026-06-28）：

| 问题 | 确认结果 |
|------|---------|
| MaxDepth 具体字段名 | ✅ 使用现有 `AgentRuntimeSetting.MaxSessionDepth` |
| agent-card 是否需要用户补充输入框 | ✅ 保留补充输入框（与 team-card 一致交互） |
| plan 面板是否支持折叠 | ✅ 支持折叠（进行中展开，全部完成后自动折叠为摘要） |

**无待确认问题**，可进入文档分裂阶段。

---

## 十、下一步计划

### 10.1 文档分裂

审查通过后，本草稿分裂为三个正式文档：

1. **`1-chat.md`**（需求）— 用户故事 + 功能需求 + 验收标准
2. **`1-chat.design.md`**（设计）— 架构 + 协议 + 数据模型 + UI 组件 + 排序规则
3. **`1-chat.development.md`**（开发计划）— 代码锚点 + 任务清单 + 验收标准

### 10.2 旧文档处理

- `1-chat.md` / `1-chat.design.md` / `1-chat.development.md`：替换为本草稿内容
- `59-chat-ui-optimization.md` / `.design.md` / `.development.md`：标记为 SUPERSEDED 或合并到 1-chat 三件套
- `51-message-mechanism.md`：保留，作为消息机制的底层参考

### 10.3 代码改动清单（审查通过后）

**后端**：
1. 移除 `internal/biz/activity_seq.go` GlobalSeqAllocator
2. 修复 Team/Graph direct-publish 事件的 `SpiritSessionID` 填充
3. DB schema 移除 `seq` 字段（兼容期保留，不再用于排序）
4. 查询排序改为 `ORDER BY turn_id ASC, parent_activity_id ASC, timestamp ASC`

**前端**：
1. 新增 `TeamCard.vue` 组件（按 4.2 设计）
2. 新增 `AgentCard.vue` 组件（按 4.3 设计）
3. ActivityStream 增加 `team_stage` → TeamCard 分支
4. ActivityStream 增加 `session`（subagent_spawn）→ AgentCard 分支
5. 移除 `compareActivities` 中的 seq 排序逻辑
6. 实现 plan 状态由 team_stage 事件驱动更新
7. 实现子 session 懒加载

**文档**：
1. 修复 59-chat-ui-optimization.md 的失效引用
2. 更新 project_memory.md 的过时描述
