# System Builtin Agents 设计文档

> 日期：2026-05-31
> 状态：Draft
> 范围：系统内置管家体系 + 编排管家核心能力 + Session 树状模型

---

## 一、背景与动机

### 1.1 现状

当前系统有以下"内置"内容：

| 类别 | 数量 | Kind | 用户可见 | 本质 |
|------|------|------|---------|------|
| System Admin Agent | 1 | `system` | ✅ | CLI 系统管家，管理 Skill/MCP/行业安装 |
| 行业样例 Agent | 76 | `system` | ✅（行业模板库） | YAML 驱动的样例，用户可直接使用 |
| 行业样例 Team | 14 | — | ✅ | YAML 驱动的样例编排方案 |
| 内置平台工具 | 44 | — | ✅ | 全局可用工具 |
| 内置插件 | 若干 | — | ✅ | 全局可用插件 |

**核心问题**：行业 Agent/Team 是"样例"而非"壁垒"。用户仍需自己理解行业、选择 Agent、组建 Team、配置编排。产品缺少一个**智能入口**——用户只需描述任务，系统自动完成从分析到执行到汇报的全流程。

### 1.2 目标

构建**系统内置管家体系**，作为产品的核心壁垒：

1. **总管家（精灵/Spirit）**：用户唯一对话入口，一个精灵控制所有内置管家
2. **编排管家（Orchestrator）**：跨行业自动编排，动态组建团队，执行任务并汇报
3. **CLI 管家（现有）**：管理系统生态（Skill/MCP/行业）
4. **记忆管家（Memory）**：做梦功能，整理记忆系统
5. **技能管家（Skills）**：技能的进化与优化
6. **系统监控管家（Monitor）**：系统健康监控与告警

### 1.3 核心用户故事

> 作为用户，我只想对总管家说"帮我分析某科技公司财报并写一篇自媒体稿"，总管家自动：
> 1. 识别任务涉及金融分析 + 自媒体写作
> 2. 委派给编排管家
> 3. 编排管家从 finance 行业找到分析师 Agent，从 selfmedia 行业找到写手 Agent
> 4. 动态组建跨行业 Team，选择合适的编排模式
> 5. 每个 Agent 在独立 Session 中执行
> 6. 编排管家汇总各 Agent 结果，汇报给总管家
> 7. 总管家将最终结果呈现给用户

---

## 二、架构设计

### 2.1 Agent Kind 扩展

现有 `kind` 枚举：`user` | `system`

新增：`system_builtin`

| Kind | 含义 | 用户可见 | 可编辑 | 可删除 | 示例 |
|------|------|---------|--------|--------|------|
| `user` | 用户自建 | ✅ | ✅ | ✅ | 用户创建的 Agent |
| `system` | 行业样例 | ✅（行业模板库） | ✅（参数可调） | ❌ | finance 分析师 |
| `system_builtin` | 系统内置管家 | ✅（带"系统管家"标签） | ❌ | ❌ | 编排管家、记忆管家 |

**Ent Schema 变更**：

```go
// internal/data/ent/schema/agent.go
field.Enum("kind").Values("user", "system", "system_builtin").Default("user")
```

**前端过滤规则**：Agent 列表页显示 `system_builtin` Agent，但带"系统管家"标签且禁止编辑/删除。

### 2.2 管家层级与协作模型

```
用户 ──对话──→ 总管家（精灵/Spirit）
                  │
                  ├── 编排管家（Orchestrator）
                  │     ├── 行业 Agent A（finance/技术分析师）
                  │     ├── 行业 Agent B（selfmedia/内容写手）
                  │     └── 用户自建 Agent C
                  │
                  ├── CLI 管家（CLI Admin）── 现有 __system_admin__
                  │
                  ├── 记忆管家（Memory）
                  │
                  ├── 技能管家（Skills）
                  │
                  └── 系统监控管家（Monitor）
```

**对齐 trpc-agent-go 框架**：

总管家与各管家之间的关系，映射到框架的 **Team coordinator 模式**：

- 总管家 = Team 的 `coordinator`（LLM Agent）
- 各管家 = Team 的 `members`（各自是独立的 LLM Agent）
- 总管家通过 `AgentTool`（框架的 `team.New()` 内部机制）调用各管家
- 各管家通过专属工具集实现能力

这与框架 `team.Team` 的 `ModeCoordinator` 完全对齐：

```go
// pkg/trpc-agent-go/team/team.go
// Coordinator 模式：coordinator Agent 将成员包装为 AgentTool
// coordinator 自主决定调用哪些成员
```

### 2.3 编排管家的 Agent 发现策略

**核心挑战**：随着行业/岗位/Agent 数量增长，不能让 LLM 遍历所有 Agent（token 消耗巨大）。

**三层检索架构**：

```
第一层：行业级路由（粗筛）
  → 编排管家根据任务描述判断涉及哪些行业
  → 实现：行业关键词表 + LLM 分类，token 消耗极低
  → 工具：classify_industry

第二层：岗位级匹配（细筛）
  → 在目标行业内，根据任务子目标匹配岗位
  → 实现：岗位职责描述的向量相似度搜索（embedding）或关键词匹配
  → 返回 top-K 候选岗位（K=5~10）
  → 工具：search_positions

第三层：Agent 级选择（精选）
  → 在匹配的岗位下查找已有 Agent
  → 优先选择用户自建的（更贴合场景），其次选系统集成的
  → 如果岗位下无 Agent，按模板实例化
  → 工具：find_agents_by_position / instantiate_agent_from_position
```

**关键设计决策**：编排管家不会一次性把所有 Agent 信息喂给 LLM，而是通过工具调用逐步缩小范围，每次只返回必要的摘要信息。这与框架的 `AgentTool` 模式一致——LLM 通过工具调用获取信息，而非在 prompt 中穷举。

### 2.4 编排管家的递归编排与深度控制

**场景**：编排管家收到超复杂任务，需要先拆解为子任务，每个子任务又需要组建子团队。

**对齐框架**：

框架的 `Invocation.Clone()` 机制天然支持递归调用：

```go
// pkg/trpc-agent-go/agent/invocation.go
// Clone 生成新 InvocationID，共享 Session/SessionService
// Branch 拼接: parent.Branch + "/" + newAgentName
```

**深度控制方案**：

在 `Invocation.state` 中记录编排深度：

```go
const (
    stateKeyOrchestrationDepth = "orchestration_depth"
    stateKeyMaxOrchestrationDepth = "max_orchestration_depth"  // 默认 2
)
```

- 编排管家创建子 Team 时，通过 `Invocation.SetState` 传递深度
- 子编排管家启动时检查 `orchestration_depth`，超过 `max_orchestration_depth` 时拒绝嵌套，直接分配给执行 Agent
- Agent 的 system prompt 中自动注入层级信息

**深度示意**：

```
depth=0: 用户 → 总管家（精灵）
depth=1: 总管家 → 编排管家（组建 Team A）
depth=2: 编排管家 → 子编排管家（组建 Team B，处理子任务）
depth=3: 子编排管家 → 执行 Agent（不能再嵌套，直接执行）
```

### 2.5 Session 树状模型

**对齐框架**：

框架的 `session.Session` 使用 `StateMap`（`map[string][]byte`）存储状态，支持 `GetState`/`SetState`/`ApplyEventStateDelta`。Session 之间目前无层级关系。

**扩展方案**：

在 **biz 层** 的 Session 模型中新增层级字段，而非修改框架的 `session.Session`：

```go
// internal/biz/session_types.go 新增字段
type Session struct {
    // ... 现有字段
    ParentSessionID string `json:"parent_session_id,omitempty"`
    RootSessionID   string `json:"root_session_id,omitempty"`
    AgentDepth      int    `json:"agent_depth,omitempty"`
}
```

**Ent Schema 变更**：

```go
// internal/data/ent/schema/session.go 新增字段
field.String("parent_session_id").Optional().Nillable()
field.String("root_session_id").Optional().Nillable()
field.Int("agent_depth").Default(0)
```

**框架 Session State 中存储子 Session 关系**：

编排管家的框架 Session `State` 中存储 `child_session_ids`：

```go
// 编排管家创建子 Agent Session 后，写入自身 Session State
sess.SetState("child_session_ids", json.Marshal([]string{childSessID1, childSessID2}))
```

这与框架的 `session.Service.AppendEvent` + `event.Event.StateDelta` 机制对齐——编排管家通过事件将 `child_session_ids` 写入 Session State。

**Session 树查询**：

```sql
-- 查询某 Session 的所有子 Session
SELECT * FROM sessions WHERE parent_session_id = ?;

-- 查询某根 Session 下的整棵树
SELECT * FROM sessions WHERE root_session_id = ? ORDER BY agent_depth, created_at;
```

---

## 三、编排管家详细设计

### 3.1 核心工作流

```
用户描述任务
  → 总管家分析意图，委派给编排管家
    → 编排管家分析任务（LLM 推理）
      → classify_industry：识别涉及行业
        → search_positions：在行业内搜索匹配岗位
          → find_agents_by_position：查找已有 Agent
            → 场景 A：找到匹配 Agent → 直接使用
            → 场景 B：岗位存在但无 Agent → instantiate_agent_from_position
            → 场景 C：岗位不存在 → 使用通用 Agent 替代 + 建议用户创建
      → estimate_task：评估可行性、成本、推荐方案
        → 用户确认（可选，复杂任务时 HITL）
      → assemble_team：动态创建 Team Definition + 执行
        → 每个 Agent 在独立 Session 中运行
        → 编排管家 Session 记录子 Session ID
      → 收集各 Agent 结果
    → report_task_result：汇总汇报
```

### 3.2 专属工具集

所有工具使用 `function.NewFunctionTool[I, O]` 构建（对齐框架铁律 A5），注册到 `internal/tools/orchestrator/` 包。

| 工具名 | 输入 | 输出 | 调用的 Usecase/端口 |
|--------|------|------|-------------------|
| `classify_industry` | `{task_description: string}` | `{industries: [{key, name, relevance_score}]}` | `IndustryUsecase.List` |
| `search_positions` | `{industry_key: string, task_subgoals: []string}` | `{positions: [{key, name, responsibility_summary}]}` | `TaxonomyUsecase.ListByLevel` + embedding 搜索 |
| `find_agents_by_position` | `{position_key: string}` | `{agents: [{agent_key, display_name, capability_tags, kind}]}` | `AgentUsecase.ListByPosition` |
| `instantiate_agent_from_position` | `{position_key: string, variant: string}` | `{agent_key: string}` | `AgentUsecase.Create` + `TaxonomyUsecase.GetPositionPrompt` |
| `estimate_task` | `{task_description: string, selected_agents: []string}` | `{feasible: bool, estimated_tokens: int, estimated_duration_sec: int, recommended_mode: string}` | 内部计算 |
| `assemble_team` | `{agent_keys: []string, mode: string, task_prompt: string}` | `{team_id: string, session_id: string, child_session_ids: []string}` | `TeamUsecase.Create` + `TeamRunner.RunTurnFromInput` |
| `report_task_result` | `{team_run_id: string}` | `{status, agents_results: [...], overall_result, recommendations, token_usage}` | `TeamUsecase.GetTeamRun` + 子 Session 读取 |
| `query_agent_status` | `{session_id: string}` | `{status, progress, current_action}` | `TeamRunUsecase` |

### 3.3 工具注册与注入

**对齐现有 `__system_admin__` 模式**：

```
种子阶段: seed_system_agents.go
  → agents 表: agent_key='__orchestrator__', kind='system_builtin', tools_profile='system_orchestrator'
  → tools 表: orchestrator_* 工具种子

注册阶段: internal/tools/orchestrator/registry.go
  → RegisterAll(deps) → []trpctool.Tool
  → IsOrchestratorAllowed(agentKey) 校验

注入阶段: internal/service/cli_admin_tools.go（扩展为 system_builtin_tools.go）
  → orchestratorTools(ctx, ag) → 仅 __orchestrator__ 返回非 nil
  → 注入到 TRPCBuilderDeps.CustomTools
```

### 3.4 assemble_team 工具的核心逻辑

这是编排管家最复杂的工具，需要：

1. **动态创建 Team**：调用 `TeamUsecase.Create` 创建临时 Team
2. **构建 Team Definition**：根据选定的 Agent 和编排模式生成 JSON
3. **创建 Team Session**：为 Team 创建专属 Session
4. **执行 Team Turn**：调用 `TeamRunner.RunTurnFromInput`
5. **建立 Session 树**：将子 Session 的 `parent_session_id` 指向编排管家的 Session

**伪代码**（关键逻辑，非完整实现）：

```go
func (t *assembleTeamTool) Call(ctx context.Context, input AssembleTeamInput) (*AssembleTeamOutput, error) {
    // 1. 构建 Team Definition
    members := make([]team.MemberDef, 0, len(input.AgentKeys))
    for i, key := range input.AgentKeys {
        members = append(members, team.MemberDef{
            AgentID:    key,
            Role:       "worker",
            Name:       fmt.Sprintf("member_%d", i+1),
            TaskPrompt: input.TaskPrompt,
            Enabled:    ptrBool(true),
            SortOrder:  i,
        })
    }
    def := team.Definition{
        Version:       2,
        Mode:          input.Mode,
        Members:       members,
        RuntimeEngine: "graph",
    }

    // 2. 创建 Team
    teamRow, err := t.teamUC.Create(ctx, biz.Team{
        TeamKey:        fmt.Sprintf("dynamic_%s_%d", input.Mode, time.Now().Unix()),
        DisplayName:    "动态编排团队",
        DefinitionJSON: mustJSON(def),
    })

    // 3. 创建 Team Session（parent_session_id 指向编排管家 Session）
    //    从工具依赖注入中获取编排管家的 Session 信息
    orchestratorSessID := t.deps.SessionID(ctx)
    rootSessID := t.deps.RootSessionID(ctx)
    agentDepth := t.deps.AgentDepth(ctx)

    teamSess, err := t.sessionUC.Create(ctx, biz.Session{
        OwnerType:       "team",
        TeamID:          teamRow.ID,
        ParentSessionID: orchestratorSessID,
        RootSessionID:   rootSessID,
        AgentDepth:      agentDepth + 1,
    })

    // 4. 执行 Team Turn（异步：返回 team_run_id，编排管家通过 query_agent_status 轮询结果）
    teamRun, err := t.teamRunner.RunTurnFromInput(ctx, teamSess, TeamTurnInput{
        TeamID:  teamRow.ID,
        Content: input.TaskPrompt,
    })

    // 5. 将 child_session_ids 写入编排管家 Session State
    //    通过 event.StateDelta 机制异步写入
    childIDs := extractChildSessionIDs(teamRun)
    t.deps.EmitStateDelta(ctx, "child_session_ids", json.Marshal(childIDs))

    return &AssembleTeamOutput{
        TeamID:          teamRow.ID,
        TeamRunID:       teamRun.ID,
        SessionID:       teamSess.ID,
        ChildSessionIDs: childIDs,
    }, nil
}
```

> **执行模式说明**：`assemble_team` 工具本身是同步调用（LLM 工具调用必须返回结果），但 Team 的执行是异步的。工具返回 `team_run_id` 后，编排管家通过 `query_agent_status` 工具轮询执行状态，通过 `report_task_result` 工具获取最终结果。

### 3.5 任务预评估（estimate_task）

在组建团队前，编排管家应先做任务预评估：

- **可行性评估**：当前 Agent 池是否具备完成任务的能力
- **成本预估**：估算所需 Agent 数量、预计 Token 消耗、预计执行时间
- **方案对比**：对于复杂任务，给出 2-3 种编排方案

**实现**：基于 `AgentUsecase.List` 的统计信息 + 历史执行数据的经验值，纯计算无需 LLM 调用。

### 3.6 Agent 能力标签

为 Agent 增加细粒度的能力描述，支持精确匹配：

```go
// internal/data/ent/schema/agent.go 新增字段
field.JSON("capability_tags", []string{}).Optional()    // 如 ["数据分析", "报告撰写", "Python"]
field.JSON("tool_capabilities", []string{}).Optional()  // 如 ["web_research", "code_execution"]
```

**种子数据**：行业 Agent 的 `capability_tags` 从 YAML `AgentSpec` 中推导（基于 `tools_allow` + `position_key`）。

### 3.7 编排结果缓存与复用

- **编排模板缓存**：成功的 Team 编排方案保存为模板，下次相似任务直接复用
- **Agent 池热缓存**：频繁使用的 Agent 组合保持在 `BuildTRPCAgentCached` 的 LRU 缓存中
- **与现有 Team 模板打通**：编排管家创建的临时 Team 可"保存为模板"，出现在 Team 列表中

### 3.8 汇报格式标准化

编排管家的汇报结构化，而非自由文本：

```json
{
  "task_summary": "任务概述",
  "status": "completed|partial|failed",
  "agents_involved": [
    {
      "name": "技术分析师",
      "agent_key": "finance-tech-analyst",
      "session_id": "sess_xxx",
      "status": "success",
      "key_findings": "..."
    }
  ],
  "overall_result": "最终结论",
  "recommendations": ["建议1", "建议2"],
  "token_usage": {
    "total": 15000,
    "by_agent": {"finance-tech-analyst": 8000, "selfmedia-writer": 7000}
  },
  "duration_seconds": 45
}
```

前端渲染为标准化的任务报告卡片。

---

## 四、总管家（精灵）详细设计

### 4.1 定位

总管家是用户的**唯一对话入口**。用户不需要知道有哪些管家、有哪些 Agent，只需描述需求。

### 4.2 工作流

```
用户消息
  → 总管家分析意图（LLM 推理）
    → 系统操作类（安装 Skill/MCP/行业）→ 委派给 CLI 管家
    → 任务执行类（分析/写作/研究）→ 委派给编排管家
    → 记忆管理类（回忆/整理/遗忘）→ 委派给记忆管家
    → 技能进化类（优化/学习/调优）→ 委派给技能管家
    → 系统监控类（状态/告警/日志）→ 委派给系统监控管家
    → 简单问答类 → 总管家直接回答
```

### 4.3 专属工具

| 工具名 | 功能 | 调用方式 |
|--------|------|---------|
| `dispatch_to_butler` | 将任务委派给指定管家 | 通过 `AgentTool` 调用目标管家 |
| `list_butlers` | 列出所有可用管家及其能力描述 | 读取 `system_builtin` Agent 列表 |
| `query_butler_status` | 查询管家当前工作状态 | 读取管家 Session State |

### 4.4 与框架 Team Coordinator 模式的对齐

总管家 + 各管家 = 一个 **系统级 Team**：

```go
// 伪代码：构建系统级 Team
spiritTeam := trpcteam.New(
    "system_spirit_team",
    trpcteam.WithMode(trpcteam.ModeCoordinator),
    trpcteam.WithCoordinator(spiritAgent),       // 总管家 = coordinator
    trpcteam.WithMembers([]agent.Agent{          // 各管家 = members
        orchestratorAgent,
        cliAdminAgent,
        memoryAgent,
        skillsAgent,
        monitorAgent,
    }),
)
```

总管家通过框架的 `AgentTool` 机制调用各管家——这与现有 Team 的 coordinator 模式完全一致，无需新机制。

### 4.5 聊天路由

当用户在聊天界面未指定 Agent 时，默认路由到总管家：

```go
// internal/service/chat.go
// 如果请求未指定 agent_key，默认使用 __spirit__
if agentKey == "" {
    agentKey = "__spirit__"
}
```

---

## 五、各管家概要设计

### 5.1 CLI 管家（现有 `__system_admin__`）

**无需改动**，保持现有实现。从 `kind=system` 迁移到 `kind=system_builtin`。

### 5.2 记忆管家（Memory）

| 维度 | 设计 |
|------|------|
| Agent Key | `__memory__` |
| 核心能力 | 做梦功能（离线记忆整理）、记忆查询、遗忘策略 |
| 专属工具 | `dream_cycle`（触发做梦）、`search_memories`、`consolidate_memories`、`forget_policy` |
| 调用的 Usecase | `MemoryUsecase`、`SessionUsecase` |
| tools_profile | `system_memory` |

> 详细设计待记忆系统实现后补充。

### 5.3 技能管家（Skills）

| 维度 | 设计 |
|------|------|
| Agent Key | `__skills__` |
| 核心能力 | 技能进化、自动调优、技能推荐 |
| 专属工具 | `evolve_skill`、`optimize_skill`、`recommend_skills`、`analyze_skill_usage` |
| 调用的 Usecase | `SkillUsecase` |
| tools_profile | `system_skills` |

> 详细设计待技能进化系统实现后补充。

### 5.4 系统监控管家（Monitor）

| 维度 | 设计 |
|------|------|
| Agent Key | `__monitor__` |
| 核心能力 | 系统健康监控、异常告警、资源使用分析 |
| 专属工具 | `check_system_health`、`get_usage_stats`、`list_alerts`、`get_agent_performance` |
| 调用的 Usecase | `UsageUsecase`、`MonitorUsecase` |
| tools_profile | `system_monitor` |

> 详细设计待监控系统实现后补充。

---

## 六、实现架构（对齐项目分层）

### 6.1 后端改动清单

| 层 | 改动 | 文件 | 优先级 |
|----|------|------|--------|
| **Schema** | Agent kind 新增 `system_builtin`；Agent 新增 `capability_tags`/`tool_capabilities`；Session 新增 `parent_session_id`/`root_session_id`/`agent_depth` | `ent/schema/agent.go`, `ent/schema/session.go` | P0 |
| **Proto** | Session 新增 `parent_session_id`/`root_session_id`/`agent_depth` 字段 | `api/kratos/session/v1/session.proto` | P0 |
| **Biz** | 新增 `SystemAgentUsecase`（编排逻辑）；`AgentUsecase` 增加 `ListByPosition`/`ListByCapability`；`SessionUsecase` 增加树查询 | `internal/biz/system_agent_usecase.go`, `internal/biz/agent_usecase.go`, `internal/biz/session_usecase.go` | P0 |
| **Data** | `seed_system_agents.go` 统一种子；Session 树查询实现；Agent 能力标签查询 | `internal/data/seed_system_agents.go`, `internal/data/session.go`, `internal/data/agent.go` | P0 |
| **Tools** | 注册编排管家专属工具（8 个）；注册总管家专属工具（3 个） | `internal/tools/orchestrator/`, `internal/tools/spirit/` | P0 |
| **Agent** | 编排管家 prompt 文件；总管家 prompt 文件；其他管家 prompt 文件 | `internal/scenario/system/prompts/` | P0 |
| **Service** | 聊天路由默认到总管家；`system_builtin_tools.go` 统一注入 | `internal/service/chat.go`, `internal/service/system_builtin_tools.go` | P0 |
| **Biz** | `TaxonomyUsecase` 增加向量搜索能力（岗位 embedding） | `internal/biz/taxonomy.go` | P1 |
| **Biz** | 编排模板缓存（TeamUsecase 扩展） | `internal/biz/team_usecase.go` | P1 |

### 6.2 前端改动清单

| 改动 | 文件 | 优先级 |
|------|------|--------|
| Agent 列表：显示 `system_builtin` Agent（带"系统管家"标签，禁止编辑/删除） | `AgentsListSection.vue`, `AgentCard.vue` | P0 |
| 聊天界面：默认路由到总管家 | `ChatPage.vue` | P0 |
| 聊天界面：子 Agent 执行过程折叠面板 | `components/chat/TaskProgressPanel.vue`（新增） | P0 |
| Session 详情：新增"子会话"Tab，展示树状关系 | `SessionDetailPage.vue` | P0 |
| 任务报告卡片：编排管家汇报的结构化渲染 | `components/chat/TaskReportCard.vue`（新增） | P1 |

### 6.3 新增文件清单

```
internal/
  tools/
    orchestrator/
      registry.go          # 编排管家工具注册
      classify_industry.go # 行业分类工具
      search_positions.go  # 岗位搜索工具
      find_agents.go       # Agent 查找工具
      instantiate_agent.go # Agent 实例化工具
      estimate_task.go     # 任务评估工具
      assemble_team.go     # 动态组队工具
      report_result.go     # 结果汇报工具
      query_status.go      # 状态查询工具
    spirit/
      registry.go          # 总管家工具注册
      dispatch.go          # 任务委派工具
      list_butlers.go      # 管家列表工具
      query_status.go      # 管家状态工具
  scenario/
    system/
      prompts/
        spirit.md          # 总管家 system prompt
        orchestrator.md    # 编排管家 system prompt
        memory.md          # 记忆管家 system prompt
        skills.md          # 技能管家 system prompt
        monitor.md         # 监控管家 system prompt
  biz/
    system_agent_usecase.go # 系统管家 Usecase
  data/
    seed_system_agents.go   # 统一种子（替代 seed_system_admin.go）
  service/
    system_builtin_tools.go # 系统管家工具注入（扩展自 cli_admin_tools.go）
```

---

## 七、新增系统管家的三步流程

### 步骤 1：注册工具

在 `internal/tools/<管家名>/registry.go` 中注册专属工具到 `Registry()`：

```go
func RegisterAll(deps Deps) []trpctool.Tool {
    return []trpctool.Tool{
        function.NewFunctionTool[InputType, OutputType](
            "tool_name", "tool description", handler,
        ),
    }
}

func IsAllowed(agentKey string) bool {
    return agentKey == "__<管家名>__"
}
```

### 步骤 2：种子数据

在 `seed_system_agents.go` 中添加管家定义：

```go
var systemBuiltinAgents = []systemBuiltinAgentSeed{
    {
        AgentKey:    "__<管家名>__",
        DisplayName: "管家显示名",
        Description: "管家描述",
        Provider:    "openrouter",
        Model:       "gpt-4.1",
        Kind:        "system_builtin",
        ToolsProfile: "system_<管家名>",
        Readonly:    true,
    },
}
```

### 步骤 3：Prompt 文件

在 `internal/scenario/system/prompts/<管家名>.md` 中编写 system prompt。

---

## 八、对齐 trpc-agent-go 框架的关键映射

| 本设计概念 | 框架对应机制 | 对齐方式 |
|-----------|-------------|---------|
| 系统管家 = LLM Agent | `agent.Agent` 接口（5 方法） | 走标准 `BuildTRPCLLMAgent` 构建 |
| 管家专属工具 | `tool.Tool` 接口 + `function.NewFunctionTool[I, O]` | 注册到 `tools.Registry()` + `AssemblyConfig.CustomTools` 注入 |
| 总管家 → 各管家委派 | `team.Team` 的 `ModeCoordinator` | 总管家 = coordinator，各管家 = members |
| 编排管家 → 行业 Agent 编排 | `team.Team` + `TeamRunner.RunTurnFromInput` | 动态创建 Team Definition → Graph 编译 → 执行 |
| 子 Agent Session 隔离 | `session.Service.CreateSession` + `session.Key` | 每个子 Agent 独立 Session，通过 `parent_session_id` 关联 |
| 编排深度控制 | `Invocation.state`（`GetState`/`SetState`） | `orchestration_depth` + `max_orchestration_depth` |
| Agent 工厂动态创建 | `runner.AgentFactory` + `WithAgentFactory` | `BizAgentFactoryOptions` 已有完整机制 |
| 事件流通信 | `agent.Run()` 返回 `<-chan *event.Event` | 编排管家消费子 Team 的事件流，提取结果 |
| 子 Agent 层级感知 | `Invocation.Branch` + `Invocation.state` | Branch 记录调用链路，state 记录深度 |
| 工具注入路径 | `TRPCBuilderDeps.CustomTools` → `tools.Assemble()` | 对齐 `cliAdminTools` 注入模式 |

---

## 九、分期实施计划

### P0：核心骨架（编排管家 + 总管家）

1. Agent Kind 扩展 + Session 树字段
2. `seed_system_agents.go` 统一种子
3. 编排管家专属工具（8 个）
4. 总管家专属工具（3 个）
5. Prompt 文件
6. 聊天路由默认到总管家
7. 前端：系统管家标签 + 子会话 Tab + 折叠面板

### P1：能力增强

1. Agent 能力标签体系
2. 岗位向量搜索（embedding）
3. 编排模板缓存与复用
4. 任务预评估（estimate_task）
5. 前端：任务报告卡片

### P2：其他管家

1. 记忆管家（依赖记忆系统实现）
2. 技能管家（依赖技能进化系统实现）
3. 系统监控管家（依赖监控系统实现）

---

## 十、风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 编排管家 LLM 推理不稳定，选错 Agent | 任务执行失败或结果差 | 任务预评估 + HITL 确认 + 失败回退策略 |
| 三层检索的岗位匹配精度不足 | 找不到合适的 Agent | P1 阶段引入 embedding 向量搜索，提升匹配精度 |
| 递归编排深度失控 | Token 消耗爆炸 / 死循环 | `max_orchestration_depth=2` 硬限制 + Invocation.state 检查 |
| 动态创建的 Team/Agent 数量膨胀 | 数据库膨胀 | 临时 Team 标记 `is_temporary=true`，定期清理 |
| Session 树查询性能 | 大量子 Session 时查询慢 | `root_session_id` 索引 + 限制单次编排的子 Session 数量 |

---

## 十一、AI 落地补充规范

> 本节补充 AI 编码时必须知晓的实现细节，缺失这些内容将导致实现错误。

### 11.1 总管家 Team 的运行时构建方式

**问题**：文档 §4.4 说"总管家+各管家=Team coordinator模式"，但未定义何时/何地构建。

**决策**：总管家 Team **不在** Wire 中静态注册，而是在 `ChatOrchestrator.runSingleAgentViaTRPC` 中**动态构建**。

**原因**：
- 框架 `team.New(coordinator, members)` 需要已构建好的 `agent.Agent` 实例
- 管家 Agent 通过 `BuildTRPCAgentCached` 按需构建，无法在 Wire 阶段获取
- 现有 `__system_admin__` 也是运行时动态构建的（通过 `BizAgentFactoryOptions`）

**实现路径**：

```
1. 用户发起 Chat 请求，agent_key="__spirit__"
2. ChatOrchestrator.runSingleAgentViaTRPC() 识别 __spirit__
3. 构建 spirit Agent（BuildTRPCAgentCached）
4. 构建 member 管家 Agents（遍历 system_builtin Agent 列表，逐个 BuildTRPCAgentCached）
5. 调用 team.New(spiritAgent, memberAgents) 构建 Team
6. 用 Team 作为 root Agent 创建 Runner
7. Runner.Run() 启动对话
```

**关键代码位置**：`internal/service/chat_orchestrator_turn.go` 行 478-597，在 `CustomTools` 注入之后、`BuildTRPCAgentCached` 调用之前，增加判断逻辑：

```go
// 伪代码：在 runSingleAgentViaTRPC 中
if ag.AgentKey == "__spirit__" {
    root, err = o.buildSpiritTeam(ctx, ag, deps)  // 构建 Team
} else {
    root, err = chatagent.BuildTRPCAgentCached(ctx, ag, deps)
}
```

**`buildSpiritTeam` 方法**：

```go
func (o *ChatOrchestrator) buildSpiritTeam(
    ctx context.Context, spiritAg biz.Agent, deps chatagent.TRPCBuilderDeps,
) (agent.Agent, error) {
    // 1. 构建 spirit coordinator Agent
    spiritAgent, err := chatagent.BuildTRPCAgentCached(ctx, spiritAg, deps)
    if err != nil { return nil, err }

    // 2. 加载所有 system_builtin Agent（排除 __spirit__ 自身）
    butlers, err := o.td.Catalog.Agents.SearchAgents(ctx, biz.AgentListQuery{
        Kind:  "system_builtin",
        Limit: 50,
    })
    if err != nil { return nil, err }

    // 3. 逐个构建 member Agent
    var members []agent.Agent
    for _, b := range butlers.Items {
        if b.AgentKey == "__spirit__" { continue }
        memberDeps := deps  // 共享依赖
        memberDeps.CustomTools = o.systemBuiltinTools(ctx, b)  // 注入管家专属工具
        member, err := chatagent.BuildTRPCAgentCached(ctx, b, memberDeps)
        if err != nil { continue }  // 单个管家构建失败不影响整体
        members = append(members, member)
    }

    // 4. 构建 Team（coordinator 模式）
    team, err := trpcteam.New(spiritAgent, members)
    if err != nil { return nil, err }
    return team, nil
}
```

### 11.2 `dispatch_to_butler` 工具的实现方式

**问题**：文档 §4.3 说"通过 AgentTool 调用目标管家"，但未说明如何获取 AgentTool。

**决策**：`dispatch_to_butler` **不需要单独实现**。

**原因**：框架 `team.New(coordinator, members)` 内部自动将每个 member 包装为 `AgentTool`，注入到 coordinator 的 ToolSet 中。coordinator（总管家）的 LLM 可以直接通过工具调用各管家，工具名格式为成员 Agent 的名称。

**总管家实际拥有的工具**：
- 框架自动注入的 `AgentTool`（每个管家一个，工具名=管家 AgentKey）
- `list_butlers`（自定义工具，列出管家能力描述）
- `query_butler_status`（自定义工具，查询管家状态）

**总管家 Prompt 中需说明**："你可以通过工具调用各管家。可用的管家工具有：`__orchestrator__`（编排管家）、`__system_admin__`（CLI管家）、`__memory__`（记忆管家）、`__skills__`（技能管家）、`__monitor__`（监控管家）。根据用户意图选择合适的管家。"

### 11.3 聊天路由修改的精确位置

**问题**：文档 §4.5 说"默认路由到总管家"，但未指定修改位置。

**现状分析**：
- **原生 Chat 层**（`chat_orchestrator_turn.go:136-141`）：`sess.AgentID == ""` 时直接报错
- **OpenAI 兼容层**（`openai_compat.go:106-111`）：通过 `conf.DefaultAgentKey` 配置
- **Cron 层**（`cronrunner/runner.go:341-358`）：优先取 `IsDefault=true` 的 Agent

**决策**：在**创建 Session 时**自动设置 `agent_id` 为总管家，而非在路由层拦截。

**修改位置**：

1. **`internal/service/chat_orchestrator_session.go`**（Session 创建逻辑）：当用户未指定 agent_key 时，自动查找 `__spirit__` Agent 并设置到 Session

```go
// 创建 Session 时
if agentKey == "" {
    spiritAg, err := o.td.Catalog.Agents.GetAgentByAgentKey(ctx, "__spirit__")
    if err == nil {
        agentID = spiritAg.ID
    }
}
```

2. **`configs/configs.yaml`**：设置 `default_agent_key: "__spirit__"`，使 OpenAI 兼容层也默认路由到总管家

3. **CLI 入口**（`cmd/aranea/main.go:86`）：将默认 AgentKey 从 `"__system_admin__"` 改为 `"__spirit__"`

### 11.4 `assemble_team` 工具的依赖注入路径

**问题**：TeamRunner 在 `internal/team/runner.go`，但工具在 `internal/tools/orchestrator/`，如何获取 TeamRunner？

**决策**：通过 `service.ChatOrchestratorDeps` 中已有的 `TeamOrchestrationDeps` 间接获取。

**现有依赖链**：

```
service.ChatOrchestratorDeps
  → service.TeamOrchestrationDeps（包含 TeamRunner）
    → internal/team/Runner
```

**工具 Deps 定义**：

```go
type Deps struct {
    // Usecase 层依赖
    IndustryUC  *biz.IndustryUsecase
    TaxonomyUC  *biz.TaxonomyUsecase
    AgentUC     *biz.AgentUsecase
    SessionUC   *biz.SessionUsecase
    TeamUC      biz.TeamRepository

    // 运行时依赖（通过 ChatOrchestratorDeps 传递）
    TeamRunner  func() *team.Runner  // 延迟获取，避免循环依赖

    // Session 上下文
    SessionIDFunc       func(ctx context.Context) string
    RootSessionIDFunc   func(ctx context.Context) string
    AgentDepthFunc      func(ctx context.Context) int
    EmitStateDeltaFunc  func(ctx context.Context, key string, value []byte)
}
```

**注入方式**（在 `system_builtin_tools.go` 中）：

```go
func (o *ChatOrchestrator) orchestratorTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
    if ag.AgentKey != "__orchestrator__" { return nil }
    return orchestrator.RegisterAll(orchestrator.Deps{
        IndustryUC:  o.td.Catalog.IndustryUC,
        TaxonomyUC:  o.td.Catalog.TaxonomyUC,
        AgentUC:     o.td.Catalog.AgentsUC,
        SessionUC:   o.td.Sessions,
        TeamUC:      o.td.Persist.Teams,
        TeamRunner:  func() *team.Runner { return o.td.TeamDeps.Runner },
        // ... Session 上下文函数
    })
}
```

### 11.5 Session 树字段的写入时机

**问题**：`parent_session_id`/`root_session_id`/`agent_depth` 何时写入？

**决策**：在 `sessionUC.Create` 时一次性写入，不事后补填。

**写入场景**：

| 场景 | parent_session_id | root_session_id | agent_depth |
|------|-------------------|-----------------|-------------|
| 用户直接对话（无编排） | null | 自身 ID | 0 |
| 总管家 Session | null | 自身 ID | 0 |
| 编排管家创建 Team Session | 编排管家的 Session ID | 编排管家的 root_session_id | 编排管家的 agent_depth + 1 |
| Team 内 Agent Session | Team Session ID | Team Session 的 root_session_id | Team Session 的 agent_depth + 1 |

**关键实现**：编排管家的 `assemble_team` 工具通过 Deps 中的 `SessionIDFunc`/`RootSessionIDFunc`/`AgentDepthFunc` 获取当前 Session 信息，创建子 Session 时传入。

**Session ID 传递机制**：工具的 Deps 函数从 `ctx` 中提取 Session 信息。在 `runSingleAgentViaTRPC` 中，将当前 Session 信息注入 ctx：

```go
ctx = context.WithValue(ctx, ctxKeySessionID, sess.ID)
ctx = context.WithValue(ctx, ctxKeyRootSessionID, sess.RootSessionID)
ctx = context.WithValue(ctx, ctxKeyAgentDepth, sess.AgentDepth)
```

### 11.6 Wire DI 配置变更

**新增 ProviderSet**：

```go
// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    // ... 现有
    NewExperienceAnalyticsUsecase,  // 新增
)
```

**修改 `provideChatServiceDeps`**（`cmd/admin/wire.go`）：

新增参数：
- `expAnalytics *biz.ExperienceAnalyticsUsecase`

**修改 `ChatOrchestratorDeps`**（`internal/service/chat.go`）：

新增字段：
- `ExperienceAnalytics *biz.ExperienceAnalyticsUsecase`

### 11.7 工具 Input/Output Go struct 定义

编排管家工具（`internal/tools/orchestrator/`）：

```go
type ClassifyIndustryInput struct {
    TaskDescription string `json:"task_description" jsonschema:"description=用户任务描述"`
}
type ClassifyIndustryOutput struct {
    Industries []IndustryMatch `json:"industries"`
}
type IndustryMatch struct {
    Key            string  `json:"key"`
    Name           string  `json:"name"`
    RelevanceScore float64 `json:"relevance_score"`
}

type SearchPositionsInput struct {
    IndustryKey  string   `json:"industry_key" jsonschema:"description=行业key"`
    TaskSubgoals []string `json:"task_subgoals" jsonschema:"description=任务子目标列表"`
}
type SearchPositionsOutput struct {
    Positions []PositionMatch `json:"positions"`
}
type PositionMatch struct {
    Key                   string `json:"key"`
    Name                  string `json:"name"`
    ResponsibilitySummary string `json:"responsibility_summary"`
}

type FindAgentsInput struct {
    PositionKey string `json:"position_key" jsonschema:"description=岗位key"`
}
type FindAgentsOutput struct {
    Agents []AgentMatch `json:"agents"`
}
type AgentMatch struct {
    AgentKey       string   `json:"agent_key"`
    DisplayName    string   `json:"display_name"`
    CapabilityTags []string `json:"capability_tags"`
    Kind           string   `json:"kind"`
}

type InstantiateAgentInput struct {
    PositionKey string `json:"position_key" jsonschema:"description=岗位key"`
    Variant     string `json:"variant" jsonschema:"description=岗位变体,默认general"`
}
type InstantiateAgentOutput struct {
    AgentKey string `json:"agent_key"`
}

type EstimateTaskInput struct {
    TaskDescription string   `json:"task_description"`
    SelectedAgents  []string `json:"selected_agents"`
}
type EstimateTaskOutput struct {
    Feasible              bool   `json:"feasible"`
    EstimatedTokens       int    `json:"estimated_tokens"`
    EstimatedDurationSec  int    `json:"estimated_duration_sec"`
    RecommendedMode       string `json:"recommended_mode"`
}

type AssembleTeamInput struct {
    AgentKeys  []string `json:"agent_keys" jsonschema:"description=选定的Agent key列表"`
    Mode       string   `json:"mode" jsonschema:"description=编排模式:sequential/parallel/coordinator/swarm/adaptive"`
    TaskPrompt string   `json:"task_prompt" jsonschema:"description=任务提示词"`
}
type AssembleTeamOutput struct {
    TeamID          string   `json:"team_id"`
    TeamRunID       string   `json:"team_run_id"`
    SessionID       string   `json:"session_id"`
    ChildSessionIDs []string `json:"child_session_ids"`
}

type ReportTaskResultInput struct {
    TeamRunID string `json:"team_run_id"`
}
type ReportTaskResultOutput struct {
    Status        string         `json:"status"`
    AgentsResults []AgentResult  `json:"agents_results"`
    OverallResult string         `json:"overall_result"`
    Recommendations []string     `json:"recommendations"`
    TokenUsage    TokenUsageInfo `json:"token_usage"`
}
type AgentResult struct {
    Name        string `json:"name"`
    AgentKey    string `json:"agent_key"`
    SessionID   string `json:"session_id"`
    Status      string `json:"status"`
    KeyFindings string `json:"key_findings"`
}
type TokenUsageInfo struct {
    Total   int               `json:"total"`
    ByAgent map[string]int    `json:"by_agent"`
}

type QueryAgentStatusInput struct {
    SessionID string `json:"session_id"`
}
type QueryAgentStatusOutput struct {
    Status        string `json:"status"`
    Progress      string `json:"progress"`
    CurrentAction string `json:"current_action"`
}
```

总管家工具（`internal/tools/spirit/`）：

```go
type ListButlersInput struct{}
type ListButlersOutput struct {
    Butlers []ButlerInfo `json:"butlers"`
}
type ButlerInfo struct {
    AgentKey    string `json:"agent_key"`
    DisplayName string `json:"display_name"`
    Description string `json:"description"`
}

type QueryButlerStatusInput struct {
    AgentKey string `json:"agent_key"`
}
type QueryButlerStatusOutput struct {
    Status       string `json:"status"`
    ActiveTask   string `json:"active_task,omitempty"`
    LastActiveAt string `json:"last_active_at,omitempty"`
}
```

### 11.8 `classify_industry` 的行业关键词表

**数据结构**：

```go
// internal/tools/orchestrator/industry_keywords.go
var industryKeywordMap = map[string][]string{
    "finance":     {"金融", "股票", "投资", "财报", "基金", "风控", "量化", "交易", "银行", "保险"},
    "selfmedia":   {"自媒体", "写作", "视频", "直播", "内容", "粉丝", "网文", "小说", "运营", "变现"},
    "softwaredev": {"软件", "开发", "编程", "代码", "后端", "前端", "API", "架构", "测试", "部署"},
}
```

**实现方式**：工具内部用关键词匹配 + LLM 分类双保险。先做关键词匹配（零 Token 消耗），匹配不到再让 LLM 分类。

### 11.9 错误处理约定

所有管家工具的错误处理遵循项目规范：

```go
// 工具内部错误用 kerrors
kerrors.NotFound("ORCHESTRATOR", "agent not found by position_key: %s", input.PositionKey)
kerrors.BadRequest("ORCHESTRATOR", "invalid mode: %s", input.Mode)
kerrors.InternalServer("ORCHESTRATOR", "failed to create team: %v", err)

// 工具返回值中不包含 error 详情，只包含业务状态
// error 由框架自动转换为 LLM 可读的错误消息
```

### 11.10 `__system_admin__` 迁移策略

**迁移 SQL**（在 `seed_system_agents.go` 中执行）：

```sql
UPDATE agents SET kind = 'system_builtin' WHERE agent_key = '__system_admin__';
```

**前端兼容**：`kind=system_builtin` 的 Agent 在前端显示"系统管家"标签，与 `kind=system`（行业样例）的"行业模板"标签区分。

### 11.11 Prompt 文件核心指令要点

**总管家（spirit.md）核心指令**：

```
你是 Aranea 系统的总管家（精灵），用户的唯一对话入口。

## 你的职责
1. 分析用户意图，判断任务类型
2. 将任务委派给合适的管家
3. 汇总各管家结果，呈现给用户

## 可用管家
- __orchestrator__（编排管家）：跨行业任务编排，动态组建团队
- __system_admin__（CLI管家）：管理系统生态（Skill/MCP/行业安装）
- __memory__（记忆管家）：记忆整理、选择性记忆、遗忘策略
- __skills__（技能管家）：技能进化/消亡、工具权重优化
- __monitor__（监控管家）：系统健康监控

## 决策规则
- 简单问答 → 直接回答，不委派
- 任务执行类 → 委派给 __orchestrator__
- 系统操作类 → 委派给 __system_admin__
- 记忆管理类 → 委派给 __memory__
- 技能进化类 → 委派给 __skills__
- 系统监控类 → 委派给 __monitor__
```

**编排管家（orchestrator.md）核心指令**：

```
你是 Aranea 系统的编排管家，专注于跨行业任务编排。

## 工作流程
1. 用 classify_industry 识别涉及行业
2. 用 search_positions 搜索匹配岗位
3. 用 find_agents_by_position 查找可用 Agent
4. 用 estimate_task 评估可行性
5. 用 assemble_team 动态组队执行
6. 用 report_task_result 汇总汇报

## 约束
- 最大编排深度为 2，不可无限嵌套
- 优先复用已有 Agent，避免重复创建
- 复杂任务需先做 estimate_task 评估
```

### 11.12 前端数据流

**子 Agent 执行进度**：

```
WebSocket Envelope（type: tool_call / agent_status）
  → stores/chat/index.ts（事件分发）
    → features/chat/conversationEventDispatcher.ts
      → TaskProgressPanel.vue（渲染折叠面板）
```

**Session 树展示**：

```
Session Detail API（GET /v1/sessions/{id}，返回 parent_session_id/root_session_id/agent_depth）
  → features/session/api.ts
    → stores/session/index.ts
      → SessionDetailPage.vue（子会话 Tab）
        → SessionTreePanel.vue（新增组件，展示树状关系）
```

**新增 Proto 字段**：

```protobuf
// api/kratos/session/v1/session.proto
message Session {
    // ... 现有字段
    string parent_session_id = N;
    string root_session_id = N+1;
    int32 agent_depth = N+2;
}
```

---

## 十二、代码验证勘误与补充（第二轮）

> 本节基于代码库交叉验证，修正 §1~§11 中与实际 API 不符的内容，补充缺失的实现细节。

### 12.1 `team.New()` API 签名修正

**§4.4 伪代码有误**。实际签名：

```go
// pkg/trpc-agent-go/team/team.go
func New(coordinator agent.Agent, members []agent.Agent, opts ...Option) (*Team, error)
```

**修正后的总管家 Team 构建**：

```go
spiritTeam, err := trpcteam.New(spiritAgent, memberAgents)
// 不需要 WithMode/WithCoordinator/WithMembers 等 Option
// coordinator 模式是 team.New 的默认行为
```

**关键约束**：`coordinator` 参数必须实现 `toolSetAdder` 接口（即 `AddToolSet` 方法），所有通过 `BuildTRPCLLMAgent` 构建的 Agent 都满足此条件。

### 12.2 `RunTurnFromInput` 返回值修正

**§3.4 伪代码有误**。实际签名：

```go
// internal/team/runner.go
func (r *Runner) RunTurnFromInput(
    ctx context.Context, sess biz.Session, input biz.TurnInput,
) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error)
```

返回 3 个值，不是 `(teamRun, err)`。`assemble_team` 工具需要从 `assistantMsg` 中提取执行结果。

### 12.3 `buildSpiritTeam` 查询修正——`AgentListQuery` 无 `Kind` 字段

**§11.1 伪代码有误**。`biz.AgentListQuery` 当前字段：

```go
type AgentListQuery struct {
    Keyword    string
    Status     string
    Provider   string
    CategoryID string
    CreatedBy  string
    Role       string
    Limit      int
    Offset     int
}
```

**没有 `Kind` 字段**。两种修正方案：

**方案 A（推荐）：新增 `Kind` 字段到 `AgentListQuery`**

```go
// internal/biz/agent_types.go
type AgentListQuery struct {
    // ... 现有字段
    Kind string  // 新增：按 Agent Kind 过滤
}
```

Data 层实现中增加 WHERE 条件：`if q.Kind != "" { where = where.Where(kind, q.Kind) }`。

**方案 B：使用 `AgentReader.GetAgentByAgentKey` 逐个查询**

```go
var butlerKeys = []string{"__orchestrator__", "__system_admin__", "__memory__", "__skills__", "__monitor__"}
var members []agent.Agent
for _, key := range butlerKeys {
    b, err := o.td.Catalog.Agents.GetAgentByAgentKey(ctx, key)
    if err != nil { continue }
    // ... 构建 member
}
```

方案 B 更简单但硬编码管家列表，方案 A 更通用。**P0 阶段用方案 B 快速落地，P1 阶段迁移到方案 A**。

### 12.4 `IndustryUsecase` 不存在——行业逻辑由 `TaxonomyUsecase` 承担

**§3.2 和 §11.4 引用的 `IndustryUsecase.List` 不存在**。

行业数据存储在 `TaxonomyNode` 树中，`level="industry"` 的节点即为行业。修正：

| 原引用 | 修正 |
|--------|------|
| `IndustryUsecase.List` | `TaxonomyUsecase.ListByLevel(ctx, "industry")` |
| `IndustryUsecase` 字段 | 改为 `TaxonomyUC *biz.TaxonomyUsecase` |

`TaxonomyUsecase` 关键方法：

```go
ListByLevel(ctx, level string) ([]TaxonomyNode, error)     // 按层级查询
GetByKey(ctx, key string) (TaxonomyNode, error)             // 按 key 查询
ListByParentID(ctx, parentID string) ([]TaxonomyNode, error) // 查子节点
GetPositionPrompt(ctx, industryKey, positionKey, variant string) (PositionPromptResult, error)
```

### 12.5 `assemble_team` 输入类型修正——`biz.TurnInput`

**§3.4 伪代码使用 `TeamTurnInput`，实际是 `biz.TurnInput`**：

```go
type TurnInput struct {
    SessionID   string
    Content     string
    AgentKey    string
    TeamID      string
    Options     TurnOptions
    Timeouts    TurnTimeouts
    EntryConfig TurnEntryPointConfig
}
```

修正后的 `assemble_team` 核心调用：

```go
userMsg, assistantMsg, err := t.deps.TeamRunner().RunTurnFromInput(ctx, teamSess, biz.TurnInput{
    SessionID: teamSess.ID,
    Content:   input.TaskPrompt,
    TeamID:    teamRow.ID,
    EntryConfig: biz.TurnEntryPointConfig{
        EntryPoint: "web",
    },
})
```

### 12.6 `AgentUsecase` 需新增的方法

**§3.2 引用的 `AgentUsecase.ListByPosition` / `ListByCapability` 不存在**，需新增：

```go
// internal/biz/agent_usecase.go 新增方法
func (uc *AgentUsecase) ListByPosition(ctx context.Context, positionKey string) ([]Agent, error)
func (uc *AgentUsecase) ListByCapability(ctx context.Context, capabilityTags []string) ([]Agent, error)
```

**Data 层实现**：

- `ListByPosition`：查询 `agents` 表中 `capability_tags` JSON 包含 `positionKey` 的记录
- `ListByCapability`：查询 `agents` 表中 `capability_tags` JSON 与输入标签有交集的记录

**Repo 接口扩展**：在 `AgentReader` 中新增：

```go
type AgentReader interface {
    // ... 现有方法
    SearchAgentsByPosition(ctx context.Context, positionKey string) ([]Agent, error)
    SearchAgentsByCapability(ctx context.Context, tags []string) ([]Agent, error)
}
```

### 12.7 `biz.Session` 结构体当前字段与修改方案

**当前 `biz.Session` 无 `ParentSessionID`/`RootSessionID`/`AgentDepth`**。完整修改：

**Ent Schema 新增**（`internal/data/ent/schema/session.go`）：

```go
field.String("parent_session_id").Optional().Nillable(),
field.String("root_session_id").Optional().Nillable(),
field.Int("agent_depth").Default(0),
```

**Biz 层新增**（`internal/biz/session/usecase.go`）：

```go
type Session struct {
    // ... 现有 40+ 字段
    ParentSessionID string `json:"parent_session_id,omitempty"`  // 新增
    RootSessionID   string `json:"root_session_id,omitempty"`    // 新增
    AgentDepth      int    `json:"agent_depth,omitempty"`        // 新增
}
```

**Data 层转换函数**：在 `entSessionToBiz` / `bizSessionToEnt` 中增加新字段的映射。

### 12.8 Spirit Team 事件流传播机制

**问题**：Spirit（coordinator）委派给管家后，管家的事件如何传播回用户 WebSocket？

**框架机制**：`team.New()` 构建的 Team，其 `Run()` 方法返回 `<-chan *event.Event`，所有成员的事件都会合并到此通道。`ChatOrchestrator` 已有完整的事件转发逻辑：

```
Runner.Run() → event channel
  → ChatOrchestrator 逐个读取事件
    → 通过 WebSocket 发送给前端
```

**关键**：Spirit Team 作为 root Agent 被 Runner 管理，Team 内部所有成员的事件（`tool_call`/`text_delta`/`member_delta` 等）都会通过 Team 的事件通道向上传播。

**前端已有对应处理**：框架定义了 `EnvelopeTypeMemberMessageStart`/`EnvelopeTypeMemberDelta`/`EnvelopeTypeMemberMessageDone` 等事件类型，前端 `conversationEventDispatcher.ts` 已处理这些事件。

**无需额外开发**：Spirit Team 的事件流与现有 Team 编排的事件流完全一致，前端已有的 Team 编排进度面板可直接复用。

### 12.9 `cliAdminTools` 与 `system_builtin_tools.go` 的关系

**决策**：两者**共存**，`system_builtin_tools.go` 是 `cli_admin_tools.go` 的扩展。

**架构**：

```go
// internal/service/system_builtin_tools.go
func (o *ChatOrchestrator) systemBuiltinTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
    var tools []trpctool.Tool

    // 1. CLI Admin 工具（现有逻辑，__system_admin__ 专用）
    if adminTools := o.cliAdminTools(ctx, ag); len(adminTools) > 0 {
        tools = append(tools, adminTools...)
    }

    // 2. 编排管家工具
    if orchTools := o.orchestratorTools(ctx, ag); len(orchTools) > 0 {
        tools = append(tools, orchTools...)
    }

    // 3. 记忆管家工具
    if memTools := o.memoryButlerTools(ctx, ag); len(memTools) > 0 {
        tools = append(tools, memTools...)
    }

    // 4. 技能管家工具
    if skillTools := o.skillsButlerTools(ctx, ag); len(skillTools) > 0 {
        tools = append(tools, skillTools...)
    }

    return tools
}
```

**注入点修改**（`chat_orchestrator_turn.go`）：

```go
// 原来：
CustomTools: o.cliAdminTools(ctx, ag),
// 改为：
CustomTools: o.systemBuiltinTools(ctx, ag),
```

**`cli_admin_tools.go` 保持不变**，作为 `systemBuiltinTools` 的子调用。

### 12.10 `assemble_team` 错误处理与重试策略

**Team 执行失败场景**：

| 场景 | 处理方式 |
|------|---------|
| Team 创建失败 | 返回错误给编排管家 LLM，LLM 决定是否换方案 |
| Team 执行超时 | `TurnTimeouts.TurnTimeout` 控制超时，超时后 `RunTurnFromInput` 返回 error |
| 部分成员失败 | Team 的 coordinator 模式下，单成员失败不影响其他成员 |
| 全部失败 | 编排管家通过 `report_task_result` 获取失败信息，向总管家汇报 |

**重试策略**：`assemble_team` 工具本身不重试。编排管家 LLM 根据失败信息决定是否重新组队（更换 Agent 或调整模式）。

### 12.11 `buildSpiritTeam` 并发安全与缓存

**问题**：多用户同时与 Spirit 对话时，每次都重新构建 Team 效率低。

**方案**：利用现有的 `BuildTRPCAgentCached` 缓存机制。`BuildTRPCLLMAgentCached` 内部使用 `globalBuildCache`（LRU 缓存），缓存 key 包含 `ToolVersionHash`/`SkillVersionHash`/`MCPVersionHash`。

**Spirit Team 缓存策略**：

```go
func (o *ChatOrchestrator) buildSpiritTeam(...) (agent.Agent, error) {
    // 1. coordinator 和每个 member 都走 BuildTRPCAgentCached
    //    → 单个 Agent 构建结果会被缓存
    // 2. team.New() 本身很轻量（只是组装 AgentTool），无需额外缓存
    // 3. 当工具/技能/MCP 版本变化时，缓存自动失效
}
```

**无需额外缓存层**：现有 `globalBuildCache` 已覆盖 Agent 级别的缓存，`team.New()` 只是组装操作，开销可忽略。

### 12.12 临时 Team 清理机制

**问题**：编排管家动态创建的 Team 会持续累积。

**方案**：

1. **标记临时 Team**：`assemble_team` 创建的 Team 在 `MetadataJSON` 中标记：

```go
teamRow, err := t.deps.TeamUC.CreateTeam(ctx, biz.Team{
    TeamKey:        fmt.Sprintf("dynamic_%s_%d", input.Mode, time.Now().Unix()),
    DisplayName:    "动态编排团队",
    DefinitionJSON: mustJSON(def),
    MetadataJSON:   `{"is_temporary": true, "created_by": "__orchestrator__"}`,
})
```

2. **定时清理**：在 `cronrunner` 中新增清理任务，删除超过 7 天的临时 Team：

```go
// 清理逻辑
func cleanTemporaryTeams(ctx context.Context, teamRepo biz.TeamRepository) {
    teams, _ := teamRepo.ListTeams(ctx)
    for _, t := range teams {
        if isTemporary(t) && isOlderThan(t, 7*24*time.Hour) {
            teamRepo.DeleteTeam(ctx, t.ID)
        }
    }
}
```

3. **保留选项**：用户可将临时 Team "保存为永久"，清除 `is_temporary` 标记。

### 12.13 `classify_industry` 行业关键词表的动态更新

**问题**：§11.8 的 `industryKeywordMap` 是静态硬编码，新增行业时需改代码。

**方案**：P0 阶段使用静态表 + 数据库补充。P1 阶段从 `TaxonomyNode` 自动生成。

**P0 阶段**：

```go
// 静态表作为 fallback
var industryKeywordMap = map[string][]string{...}

// 优先从 TaxonomyNode 读取行业关键词
func (t *classifyIndustryTool) buildIndustryMap(ctx context.Context) map[string][]string {
    nodes, err := t.deps.TaxonomyUC.ListByLevel(ctx, "industry")
    if err != nil { return industryKeywordMap }  // fallback

    m := make(map[string][]string)
    for _, n := range nodes {
        m[n.Key] = extractKeywords(n)  // 从 TaxonomyNode 的 description/tags 提取
    }
    return m
}
```

**P1 阶段**：完全从数据库动态生成，移除静态表。

### 12.14 编排管家工具 Deps 修正版

综合以上修正，编排管家工具的完整 Deps 定义：

```go
// internal/tools/orchestrator/registry.go
type Deps struct {
    TaxonomyUC  *biz.TaxonomyUsecase    // 修正：原 IndustryUC
    AgentUC     *biz.AgentUsecase
    SessionUC   *biz.SessionUsecase
    TeamUC      biz.TeamRepository

    TeamRunner  func() *team.Runner

    SessionIDFunc       func(ctx context.Context) string
    RootSessionIDFunc   func(ctx context.Context) string
    AgentDepthFunc      func(ctx context.Context) int
    EmitStateDeltaFunc  func(ctx context.Context, key string, value []byte)
}
```

**注入方式修正**（`system_builtin_tools.go`）：

```go
func (o *ChatOrchestrator) orchestratorTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
    if ag.AgentKey != "__orchestrator__" { return nil }
    return orchestrator.RegisterAll(orchestrator.Deps{
        TaxonomyUC:  o.td.Catalog.TaxonomyUC,     // 修正
        AgentUC:     o.td.Catalog.AgentsUC,
        SessionUC:   o.td.Sessions,
        TeamUC:      o.td.Persist.Teams,
        TeamRunner:  func() *team.Runner { return o.td.Team.TeamsNative },
        // ... Session 上下文函数
    })
}
```

### 12.15 `ChatOrchestratorDeps` 实际结构与注入路径

**当前结构**（`internal/service/chat_orchestrator.go`）：

```go
type ChatOrchestratorDeps struct {
    rt.TurnDeps                        // 嵌入：包含 Catalog/Agents/Sessions 等
    Runs         *rt.RunRegistry
    PendingQueue *rt.PendingMessageQueue
    RT           RuntimeTooling
    Team         TeamOrchestrationDeps  // Team 相关依赖
    ChTurn       ChannelTurnDeps
    Usage        *biz.UsageUsecase
    Monitor      *biz.MonitorUsecase
    Artifacts    *biz.ArtifactUsecase
    A2AUC        *biz.A2AUsecase
    MCPServers   *biz.MCPServerUsecase
}

type TeamOrchestrationDeps struct {
    Teams          biz.TeamRepository
    TeamsNative    *team.Runner          // ← TeamRunner 在这里
    GraphFactory   biz.GraphBuilderFactory
    Graphs         *biz.GraphUsecase
    Tasks          *biz.TaskUsecase
    TeamGraphCoord *team.TeamGraphRunCoordinator
}
```

**获取 TeamRunner 的正确路径**：`o.td.Team.TeamsNative`（不是 `o.td.TeamDeps.Runner`）。

**获取 TaxonomyUsecase 的路径**：`o.td.Catalog.TaxonomyUC`（通过 `rt.TurnDeps` 嵌入）。

### 12.16 `biz.Team` 和 `biz.TeamRun` 当前结构

**`biz.Team`**：

```go
type Team struct {
    ID                  string
    TeamKey             string
    DisplayName         string
    Status              string
    IsDefault           bool
    DefinitionJSON      string
    ADKAppName          string
    CategoryIndustryID  string
    SortOrder           int
    CreatedAt           string
    UpdatedAt           string
    DeletedAt           string
}
```

**`biz.TeamRun`**：

```go
type TeamRun struct {
    ID                     string
    TeamID                 string
    SessionID              string
    MessageID              string
    Mode                   string
    Status                 string
    InputPreview           string
    OutputPreview          string
    TokenIn                int
    TokenOut               int
    CostMicroUSD           int64
    DurationMS             int
    ErrorMessage           string
    TopologyJSON           string
    GraphExecutionID       string
    DefinitionSnapshotJSON string
    TraceID                string
    StartedAt              string
    FinishedAt             string
    CreatedAt              string
    UpdatedAt              string
}
```

`assemble_team` 工具创建 Team 后，通过 `TeamRun` 的 `Status`/`OutputPreview`/`TopologyJSON` 获取执行结果。

### 12.17 集成测试计划

**端到端测试场景**：

| # | 场景 | 测试步骤 | 预期结果 |
|---|------|---------|---------|
| E2E-1 | 总管家委派给编排管家 | 1. 向 `__spirit__` 发送"帮我分析某公司财报" 2. 验证 Spirit 调用 `__orchestrator__` AgentTool 3. 验证编排管家调用 `classify_industry` → `search_positions` → `find_agents_by_position` → `assemble_team` | Team 创建成功，子 Session 树正确 |
| E2E-2 | 编排管家动态组队 | 1. 直接向 `__orchestrator__` 发送任务 2. 验证 Team 创建 3. 验证 `report_task_result` 返回结构化结果 | 编排结果格式符合 §3.8 标准 |
| E2E-3 | Session 树正确性 | 1. 执行 E2E-1 2. 查询总管家 Session 3. 验证 `root_session_id`/`agent_depth` 4. 查询子 Session 的 `parent_session_id` | Session 树层级正确 |
| E2E-4 | 编排深度控制 | 1. 设置 `max_orchestration_depth=1` 2. 发送超复杂任务 3. 验证编排管家不嵌套组队 | 深度超限时直接分配给执行 Agent |
| E2E-5 | 记忆管家 dream_cycle | 1. 向 `__memory__` 发送"整理记忆" 2. 验证 `analyze_memory_quality` → `forget_low_quality` → `deduplicate_memories` 调用链 3. 验证 HealthScore 提升 | dream_cycle 正确执行，HealthScore 改善 |
| E2E-6 | 技能管家 analyze_skill_health | 1. 向 `__skills__` 发送"分析 Skill 健康度" 2. 验证返回 `SkillHealth` 列表 3. 验证健康度判定规则 | 健康度分类正确 |
| E2E-7 | CLI 管家兼容性 | 1. 向 `__system_admin__` 发送"列出所有 Skill" 2. 验证 CLI Admin 工具正常工作 | 现有功能不受影响 |
| E2E-8 | 默认路由到总管家 | 1. 不指定 agent_key 发送消息 2. 验证 Session 自动绑定 `__spirit__` | 默认路由正确 |

**单元测试重点**：

| 模块 | 测试文件 | 关键断言 |
|------|---------|---------|
| `classify_industry` | `orchestrator/classify_industry_test.go` | 关键词匹配 + TaxonomyNode 回退 |
| `assemble_team` | `orchestrator/assemble_team_test.go` | Team 创建、Session 树、TurnInput 构造 |
| `selective_remember` | `memory_butler/selective_remember_test.go` | embedding 相似度阈值、冗余判断 |
| `forget_low_quality` | `memory_butler/forget_low_quality_test.go` | misaligned 检测、批量删除 |
| `evolve_skill` | `skills_butler/evolve_skill_test.go` | LLM 调用 mock、新版本创建 |
| `ExperienceAnalytics` | `biz/experience_analytics_test.go` | 各分析方法返回值格式 |

### 12.18 管家监控与告警机制

**管家健康检查**：在系统监控管家（`__monitor__`）中新增管家专用检查：

```go
type ButlerHealthCheck struct {
    ButlerKey     string    `json:"butler_key"`
    Status        string    `json:"status"`         // "healthy" | "degraded" | "down"
    LastActiveAt  string    `json:"last_active_at"`
    ErrorCount24h int       `json:"error_count_24h"`
    AvgLatencyMs  float64   `json:"avg_latency_ms"`
}
```

**告警规则**：

| 条件 | 告警级别 | 通知方式 |
|------|---------|---------|
| 管家 24h 内错误 > 10 | Warning | `EnvelopeTypeAlertNotify` |
| 管家连续 3 次调用失败 | Critical | `EnvelopeTypeAlertNotify` + 日志 |
| `HealthScore < 0.4` | Critical | 自动触发 dream_cycle |
| `DQ Score < 0.3` | Warning | 技能管家生成优化建议 |

**指标采集**：管家工具调用通过现有 `tool_invocations` 表自动记录，无需额外埋点。`SkillUsageTrackerPlugin` 的 BeforeTool/AfterTool 钩子已覆盖 Skill 调用统计。

### 12.19 两份文档的交叉引用关系

| 本文档章节 | 记忆管家文档章节 | 关联内容 |
|-----------|---------------|---------|
| §2.2 管家层级 | §三 记忆管家 / §四 技能管家 | 管家在 Team 中的角色定义 |
| §3.3 工具注册与注入 | §六.3 工具注入路径 | `system_builtin_tools.go` 统一注入 |
| §5.2 记忆管家概要 | §三 记忆管家详细设计 | 概要 → 详细的引用 |
| §5.3 技能管家概要 | §四 技能管家详细设计 | 概要 → 详细的引用 |
| §6.1 后端改动清单 | §六 实现架构 | 新增文件和改动清单 |
| §9 分期实施 | §七 分期实施 | P0/P1/P2 阶段对齐 |
| §11.6 Wire DI 配置 | §9.1 / §10.15 Wire 绑定 | `ExperienceAnalyticsUsecase` 注册 |
| §12.9 systemBuiltinTools | §六.3 / §10.14 Deps 注入 | 工具注入的统一入口 |

**实施顺序依赖**：

```
Doc1 P0（核心骨架）
  → Doc2 P0（经验分析引擎 + 基础工具）
    → Doc1 P1（能力增强）
      → Doc2 P1（高级能力）
        → Doc2 P2（策略化与自适应）
```

记忆管家和技能管家的 P0 阶段依赖 Doc1 P0 完成后的管家体系骨架（Agent Kind 扩展、种子数据、工具注入机制）。

---

## 十三、决策定稿（结合路线图验证）

> 本节记录经代码库验证 + 路线图（Phase 1~5）兼容性分析后的最终决策。所有决策均遵循"数据驱动 + 配置化 + 最小框架侵入"原则。

### 13.1 决策 1：Agent 归属查询——`Ownership` 新字段

**问题**：`biz.Agent.Kind` 已被运行时类型占用（`llm`/`a2a_proxy`/`chain`/`cycle`/`parallel`），DB `kind` 字段（`user`/`system`）未映射到 biz 层，`AgentListQuery` 无归属过滤能力。

**决策**：新增 `Ownership` 字段映射 DB `kind`，值域预留 Phase 5 扩展。

```go
// internal/biz/agent_types.go
type Agent struct {
    // ... 现有字段
    Kind      string  // 运行时类型：llm | a2a_proxy | chain | cycle | parallel（不变）
    Ownership string  // 归属类型：user | system | system_builtin | industry_template | marketplace | certified（新增）
}

type AgentListQuery struct {
    // ... 现有字段
    Ownership string  // 新增：按归属类型过滤
}
```

**Ent Schema 修改**：

```go
// internal/data/ent/schema/agent.go
// 原来：field.Enum("kind").Values("user", "system")
// 改为：
field.Enum("kind").Values("user", "system", "system_builtin", "industry_template", "marketplace", "certified")
```

**Data 层映射**：

```go
// internal/data/agent_repo.go entAgentToBiz 新增映射
func entAgentToBiz(a *ent.Agent) biz.Agent {
    return biz.Agent{
        // ... 现有映射
        Kind:      hydrateAgentKind(a),  // 运行时类型（从 ConfigJSON 解析）
        Ownership: string(a.Kind),       // 归属类型（从 DB kind 字段映射）
    }
}
```

**路线图兼容性**：

| Phase | Ownership 值 | 来源 |
|-------|-------------|------|
| P0 | `system_builtin` | 种子数据 |
| Phase 5.1 | `industry_template` | IndustryDeployer 部署 |
| Phase 5.3 | `marketplace` | 工作流市场部署 |
| Phase 5.5 | `certified` | 评估认证通过 |

**分期实施**：P0 新增字段 + `system_builtin` 值 + `AgentListQuery.Ownership` 过滤；P1 前端按 Ownership 分组显示；Phase 5.1 新增 `industry_template` 值。

### 13.2 决策 2：记忆删除——`L3FactWriter` 子接口 + 保护锁

**问题**：`MemoryAdminUsecase` 无 `DeleteFact`/`ListFacts` 方法，`L3FactAdminStore` 接口不完整（缺 Delete）。

**决策**：拆分 `L3FactAdminStore` 为 `L3FactReader` + `L3FactWriter`，新增删除方法 + 保护锁。

```go
// internal/biz/memory_admin_store.go

type L3FactReader interface {
    ListFactRows(ctx, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error)
    ListFactRowsForUser(ctx, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error)
    RecallL3Facts(ctx, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error)
}

type L3FactWriter interface {
    UpsertFactRow(ctx, in FactUpsert) ([]byte, error)
    DeleteFactRow(ctx, factID string) error
    DeleteFactRowsByIDs(ctx, factIDs []string) (int, error)
}

// SessionAdminStore 修改为组合 L3FactReader（而非 L3FactAdminStore）
type SessionAdminStore interface {
    L0AdminStore
    L1AdminReader
    L2RecallStore
    L3FactReader   // 改为 Reader
    L4GraphAdminStore
}
```

**保护锁**：`MemoryAdminUsecase.DeleteFactRow` 删除前检查 observations 引用：

```go
func (uc *MemoryAdminUsecase) DeleteFactRow(ctx context.Context, factID string) error {
    obsCount, _ := uc.countObservationsForFact(ctx, factID)
    if obsCount > 0 {
        lg.Warn(ctx, "deleting fact with active observations",
            lg.String("fact_id", factID),
            lg.Int("obs_count", obsCount))
    }
    return uc.factWriter.DeleteFactRow(ctx, factID)
}
```

**路线图兼容性**：Phase 4.2 pgvector 切换只需替换 Data 层实现，接口不变。Phase 3.1 学习闭环的 observations 不被误删。

### 13.3 决策 3：工具调用明细查询——复用现有 `biz.ToolInvocationReader` 接口

**问题**：`EvolutionMetricsRepo` 只有 4 个聚合方法，无法提供工具调用明细。技能管家和 Phase 3.2 技能自创建共享同一数据源。

**代码库验证结果**：`internal/biz/tool/tool.go` 已存在 `ToolInvocationReader` 接口，具备完整的工具调用查询能力：

```go
// internal/biz/tool/tool.go（现有代码，无需修改）

type ToolInvocationReader interface {
    SearchToolInvocations(ctx context.Context, q ToolRunQuery) ([]ToolInvocation, error)
    GetToolInvocationParams(ctx context.Context, invocationID string) ([]ToolInvocationParam, error)
}

type ToolRunQuery struct {
    ToolKey    string
    AgentID    string
    SessionID  string
    Status     string
    From       time.Time
    To         time.Time
    HasError   *bool
    Limit      int
    Offset     int
}

type ToolInvocation struct {
    ToolKey       string
    AgentID       string
    Status        string
    DurationMS    int
    InputPreview  string
    OutputPreview string
    // ... 更多字段
}
```

**决策**：**不新增接口**，直接复用 `biz.ToolInvocationReader`。该接口已通过 `internal/biz/tool_reexport.go` 在 biz 包顶层可见，Data 层实现已存在。

**消费者注入方式**：

```go
// 技能管家 Usecase 注入
type SkillButlerUsecase struct {
    toolReader biz.ToolInvocationReader  // 复用现有接口
    // ...
}
```

**路线图兼容性**：Phase 3.2 `SkillEvolutionUsecase.DetectAndPropose` 同样注入 `biz.ToolInvocationReader`，无需重复定义。

### 13.4 决策 4：工具权重/排序——`agent_runtime_settings` + Prompt 策略

**问题**：`tools.Assemble()` 无排序逻辑，技能管家需要按权重调整工具优先级。

**决策**：不修改 `Assemble()`，通过 `agent_runtime_settings.tool_weight_json` + Prompt 策略 + 移除 disabled 工具实现。

```go
// internal/data/ent/schema/agent_runtime_setting.go 新增字段
field.String("tool_weight_json").Default("{}"),
```

**权重 JSON 结构**：

```json
{
    "high_priority": ["web_search", "file_read"],
    "low_priority": ["code_execute"],
    "disabled": ["shell_exec"],
    "weights": {
        "web_search": 0.95,
        "file_read": 0.9,
        "code_execute": 0.3,
        "shell_exec": 0.0
    }
}
```

**注入方式**（在 `ChatOrchestrator` 构建 `TRPCBuilderDeps` 时）：

1. 从 `agent_runtime_settings` 读取权重配置
2. 过滤 `disabled` 工具（从 `EnabledTools` 移除）
3. 在 system prompt 中注入优先级提示

**不修改 `Assemble()` 的理由**：

1. 框架哲学：trpc-agent-go 的 `Assemble()` 是"组装工具集"，排序是 LLM 的决策
2. Phase 5.1 兼容：行业模板的 `IndustryDeployer` 部署时只需设置 `tool_weight_json`
3. 可观测性：权重配置存储在数据库中，用户可在前端查看和调整

### 13.5 决策 5：事件通知类型——复用 `EnvelopeTypeAlertNotify` + `severity` 分级

**问题**：`retire_skill` 等通知需要事件类型，新增 `EnvelopeType` 会导致枚举膨胀。

**决策**：复用 `EnvelopeTypeAlertNotify`，通过 `alert_type` 二级分类 + `severity` 分级。

```go
t.deps.EventBus.Publish(ctx, event.Envelope{
    Type: event.EnvelopeTypeAlertNotify,
    Payload: mustJSON(map[string]interface{}{
        "alert_type": "skill_retired",
        "severity":   "warning",
        "skill_id":   skillID,
        "skill_name": skillName,
        "reason":     reason,
        "message":    fmt.Sprintf("Skill %s 已退役：%s", skillName, reason),
    }),
})
```

**前端统一处理**：

```typescript
case 'alert.notify':
    const { alert_type, severity, message } = payload
    switch (severity) {
        case 'critical': showCriticalAlert(message); break
        case 'warning':  showWarningToast(message);  break
        case 'info':     showInfoToast(message);      break
    }
    if (alert_type === 'skill_retired') refreshSkillList()
    break
```

**路线图兼容性**：

| Phase | alert_type | severity |
|-------|-----------|----------|
| P0 | `skill_retired` | warning |
| Phase 3.2 | `skill_proposal_created` | info |
| Phase 5.1 | `industry_deployed` | info |
| Phase 5.4 | `federation_trust_changed` | warning |
| Phase 5.5 | `certification_completed` | info |

### 13.6 决策汇总

| # | 决策点 | 最终方案 | 路线图兼容性 |
|---|--------|---------|------------|
| 1 | Agent 归属查询 | `Ownership` 新字段，映射 DB `kind` | Phase 5.1/5.3 天然兼容 |
| 2 | 记忆删除 | `L3FactWriter` 子接口 + 保护锁 | Phase 3.1/4.2 兼容 |
| 3 | 工具调用明细 | 复用现有 `biz.ToolInvocationReader`（无需新增接口） | Phase 3.2 共享接口 |
| 4 | 工具权重 | `agent_runtime_settings.tool_weight_json` + Prompt 策略 | Phase 5.1 行业模板自带权重 |
| 5 | 事件通知 | 复用 `EnvelopeTypeAlertNotify` + `alert_type` + `severity` | Phase 3.2~5.5 无限扩展 |
