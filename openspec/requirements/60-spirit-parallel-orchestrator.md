# M60: Spirit Parallel Orchestrator — 精灵多任务并行编排

> **版本**：2026-06-01
> **读者**：产品、全栈开发、运维
> **关联**：[59-chat-spirit-mode.md](./59-chat-spirit-mode.md) · [59-chat-spirit-mode.design.md](./59-chat-spirit-mode.design.md) · [11-multi-agent.md](./11-multi-agent.md) · [53-team-graph-orchestration.md](./53-team-graph-orchestration.md) · [7-agent-evolution.md](./7-agent-evolution.md) · [39-planner.md](./39-planner.md)
> **技术设计**：[60-spirit-parallel-orchestrator.design.md](./60-spirit-parallel-orchestrator.design.md)
> **开发计划**：[60-spirit-parallel-orchestrator-development.md](./60-spirit-parallel-orchestrator-development.md)

---

## 1. 背景与问题

M59 实现了精灵模式基础骨架：精灵为唯一对话入口，LLM 自主决定是否调用 `assemble_team` 组建团队。但当前存在以下限制：

| 问题 | 用户影响 | 根因 |
|------|----------|------|
| 同一精灵 Session 只能有一个活跃团队 | 无法并行推进多个任务 | `GetActiveTeam()` 短路返回第一个 active 团队 |
| TeamKey 唯一约束冲突 | 同一精灵第二次创建团队失败 | TeamKey 格式为 `"spirit_" + sessionID`，无 UUID 后缀 |
| 并行度硬编码为 2 | 无法根据任务复杂度调整 | `max_concurrency: 2` 写死在 `buildSpiritTeamDefinitionJSON` |
| 无跨团队结果聚合 | 多团队完成后需人工逐个查看 | 缺少 Synthesis Engine |
| 无团队间依赖管理 | 无法表达"团队 B 等团队 A 完成后启动" | 缺少 Task DAG 模型 |
| 编排模式选择全靠 LLM 单次判断 | 选择可能非最优 | 缺少基于历史 DQ Score 的拓扑路由 |
| 任务委派无进化能力 | 同类任务反复选择相同次优编排 | 编排进化闭环未与精灵团队打通 |

**目标**：在 M59 基础上，实现精灵多任务并行编排（Spirit Parallel Orchestrator, SPO），支持同一精灵 Session 下多个团队并行执行、任务依赖调度、结果自动合成，并通过进化闭环持续优化编排策略。

---

## 2. 行业调研与学术参考

### 2.1 AI Coding 工具并行实现

| 工具 | 并行模式 | 隔离策略 | 最大并行度 | 子 Agent 递归 | 结果聚合 |
|------|---------|---------|-----------|-------------|---------|
| Claude Code | Orchestrator-Worker + Agent Teams | Git Worktree | 无硬限制 | 禁止 | 主 Agent 读取子 Agent 输出 |
| OpenAI Codex | Cloud Sandbox + Manager Agent | 云端容器（网络禁用） | 无硬限制 | 支持 `spawn_agent`/`wait_agent` | Manager Agent 汇总 |
| Cursor | Background Agent + Multi-Agents | Git Worktree + Docker | 8 Agent/Prompt | 禁止 | Composer 模型聚合 |
| Trae | @Agent + MCP 路由 | `.trae/rules/` 路径隔离 | 3-5 Agent（社区实践） | 不支持 | MCP 回调聚合 |

**关键发现**：Hub-and-Spoke（中心辐射）模式在 2026 年生产环境占比 66.4%（Landbase Agentic AI Statistics 2025），是主流并行编排模式。

### 2.2 前沿论文

| 论文 | 会议/年份 | 核心发现 | 对 M60 的启发 |
|------|----------|---------|--------------|
| **LAMaS** (arXiv:2601.10560) | 2026 | 延迟感知编排，关键路径优化减少 38-46% 延迟；层内并行、层间串行的 DAG 执行模型 | Task DAG 层级调度 |
| **AdaptOrch** (arXiv:2602.16873) | 2026 | 拓扑路由算法 O(\|V\|+\|E\|) 选择最优编排模式；模型能力趋同时编排拓扑成为主导优化变量 | 拓扑路由引擎 |
| **Maestro** (arXiv:2511.06134) | NeurIPS 2025 | 探索-合成分离：并行 Execution Agents 做发散探索，Central Agent 做收敛合成 | Synthesis Engine |
| **M1-Parallel** (arXiv:2507.08944) | ICML 2025 | 多团队并行竞速，最早完成者胜出；2.2× 加速 | 远期参考（Token 成本高） |
| **APWA** (arXiv:2605.15132) | 2026 | Manager-Worker-Executor 三层架构；Data Tables 跨 Agent 数据共享 | Spirit-Team-Agent 三层分离 |
| **DTA-Llama** (arXiv:2501.12432) | 2025 | Divide-Then-Aggregate：树状工具搜索路径转 DAG，同轮并行工具调用 | 任务分解→并行→聚合范式 |
| **Hogwild! Inference** (arXiv:2504.06261) | 2025 | 共享 KV Cache 并行推理，多 LLM 实例实时看到彼此生成进度 | 远期：共享上下文并行 |
| **ParaCook** (arXiv:2510.11608) | AAMAS 2026 | 时间效率感知的并行规划基准；Agent 内并行 + Agent 间并行联合决定规划质量 | 并行度优化策略 |
| **Puppeteer** (NeurIPS 2025, OpenReview L0xZPXT3le) | NeurIPS 2025 | RL 训练的集中式编排器动态调度 Agent；更紧凑的循环推理结构是性能提升关键 | 编排进化：DQ Score 驱动策略优化 |
| **Self-Resource Allocation** (OpenReview 0ZnEzvSLNR) | ICLR 2026 sub. | Planner 模式比 Orchestrator 模式成本效率高 2-3 倍 | Spirit 只做高层规划 |
| **Multi-Agent Coordination Survey** (arXiv:2502.14743) | 2025 | 四维协调框架；混合层级+去中心化协调是未来方向 | 混合编排拓扑 |

### 2.3 权威技术文章

| 文章 | 来源 | 核心观点 |
|------|------|---------|
| Running Parallel AI Agents with Git Worktree | gitworktree.org | 三层架构：Worktree 隔离 → Orchestration Layer → Merge & Review |
| How to Use Git Worktrees for Parallel AI Agent Execution | Augment Code | 四种失败模式：并发写覆盖、上下文污染、基础设施竞争、级联破坏 |
| Multi-Agent Systems: How They Work, When to Use Them | AgentsIndex (2026) | Hub-and-Spoke 主导生产；MAS 在顺序推理任务上性能下降 39-70% |
| AI Coding 让开发效率提升 3 倍的秘密 | 掘金 (2026) | 多工具并行 + Worktree 工作流；简单任务用快模型，复杂任务用强模型 |

---

## 3. 用户故事

### US-01 多团队并行执行

**作为** 用户
**我希望** 可以同时给精灵下达多个任务，每个任务独立组建团队并行执行
**以便** 我可以同时推进多个独立任务，而不需要等一个完成再开始下一个

**验收**：

- 精灵可在同一对话中连续调用 `assemble_team` 组建多个团队
- 每个团队拥有独立的 Session、Runner、状态
- 左侧列表同时展示多个活跃团队卡片
- 并行度可配置（默认最大 3 个并行团队），超出时精灵提示用户等待
- 同一精灵 Session 下的团队列表按状态排序：running → waiting → completed → failed

### US-02 团队进度实时监控

**作为** 用户
**我希望** 在精灵对话中查看所有活跃团队的执行进度
**以便** 我能一目了然了解各任务的推进情况

**验收**：

- 精灵对话中展示并行任务总览卡片，包含所有活跃团队的摘要
- 每个团队显示：名称、状态、进度百分比、耗时
- 团队完成/失败时精灵主动通知
- 支持精灵工具 `check_team_progress` 查询详细进度

### US-03 取消正在执行的团队

**作为** 用户
**我希望** 可以取消不再需要的团队
**以便** 释放资源并保持工作区整洁

**验收**：

- 精灵工具 `cancel_team` 可取消指定团队
- 取消后团队状态变为 `cancelled`
- 取消后精灵通知用户，释放并行度配额
- 已完成的团队不可取消（只能归档）

### US-04 任务依赖调度

**作为** 用户
**我希望** 可以表达任务之间的依赖关系（如"先完成 API 设计，再做前端对接"）
**以便** 有依赖关系的任务按正确顺序执行，无依赖的任务并行推进

**验收**：

- 精灵在分析复杂需求时，自动识别任务间的依赖关系
- 依赖团队在前置团队完成后自动启动（状态从 `waiting` → `running`）
- 无依赖的团队立即并行启动
- 精灵回复中展示任务依赖图（文本形式）

### US-05 编排模式智能选择

**作为** 用户
**我希望** 精灵能根据任务特征自动选择最优编排模式
**以便** 不同类型的任务使用最合适的编排策略

**验收**：

- 精灵根据任务 DAG 结构自动选择编排拓扑：
  - 所有任务独立 → `parallel`
  - 任务有严格依赖链 → `sequential`
  - 部分独立部分依赖 → `hybrid`
  - 需要协调者调度 → `coordinator`
- 选择依据包含历史 DQ Score 数据（如有）
- 精灵回复中说明选择该编排模式的理由

### US-06 多团队结果合成

**作为** 用户
**我希望** 所有并行团队完成后，精灵自动合成各团队结果
**以便** 我能一次性看到所有任务的汇总结果，而不需要逐个查看

**验收**：

- 所有活跃团队完成后，精灵自动调用 Synthesis Engine 合成结果
- 合成结果包含：每个团队的任务摘要、执行状态、关键产出
- 精灵生成综合回复，整合所有团队的结果
- 部分团队失败时，合成结果标注失败团队及原因
- 用户可通过精灵工具 `synthesize_results` 主动触发合成

### US-07 编排策略进化

**作为** 用户
**我希望** 精灵的编排策略能从历史执行中学习优化
**以便** 同类任务越做越好，减少次优编排

**验收**：

- 每次团队执行完成后计算 DQ Score
- DQ Score > 0.7 的编排拓扑被缓存，相似任务优先复用
- DQ Score < 0.5 时，进化系统生成编排优化建议
- 精灵在 `assemble_team` 时参考历史 DQ Score 选择编排模式
- 进化护栏确保策略变更幅度可控（`GuardrailMaxChangePerPeriod`）

---

## 4. 功能规格

### 4.1 并行团队管理

**并行度控制**：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `MaxConcurrentTeams` | 3 | 同一精灵 Session 最大并行团队数 |
| `MaxTeamConcurrency` | 2 | 单团队内最大并发成员数 |
| `TeamTimeout` | 10min | 单团队超时 |
| `AutoArchiveAfter` | 1h | 完成后自动归档时间 |
| `MaxSessionDepth` | 2 | Session 树最大深度 |

**团队生命周期扩展**：

```
assembling → running → completed → archived
                     → failed → retrying → running
                     → waiting_human → running
                     → cancelled
         → waiting_deps → running  (新增：等待依赖团队完成)
```

### 4.2 Task DAG 模型

```
TaskNode:
  id: string
  task_prompt: string
  agent_ids: []string
  mode: string              // parallel / sequential / coordinator / hybrid
  dependencies: []string    // 依赖的 TaskNode ID
  priority: int             // 优先级（0=最高）

TaskDAG:
  nodes: []TaskNode
  spirit_session_id: string
```

**拓扑路由规则**（对齐 AdaptOrch）：

| DAG 特征 | 路由结果 | 说明 |
|----------|---------|------|
| 单节点 | `sequential` | 只有一个任务 |
| 所有节点无依赖 | `parallel` | 完全独立，全部并行 |
| 存在依赖但宽度 > 1 | `hybrid` | 部分并行 + 部分串行 |
| 依赖链深度 > 3 | `coordinator` | 需要协调者管理复杂流程 |

### 4.3 Synthesis Engine

**合成策略**：

| 场景 | 策略 | 说明 |
|------|------|------|
| 全部成功 | 完整合成 | 汇总所有团队产出，生成综合摘要 |
| 部分失败 | 部分合成 | 汇总成功团队，标注失败团队 |
| 全部失败 | 失败报告 | 列出每个团队的失败原因 |
| 依赖链中断 | 级联标注 | 标注被失败团队阻塞的下游团队 |

### 4.4 精灵工具扩展

| 工具 | 功能 | Phase |
|------|------|-------|
| `assemble_team` | 组建团队（改造：支持多团队） | P1 |
| `list_butlers` | 列出可用管家 | P1（不变） |
| `query_butler_status` | 查询管家状态 | P1（不变） |
| `check_team_progress` | 查询所有活跃团队进度 | P1 |
| `cancel_team` | 取消指定团队 | P1 |
| `synthesize_results` | 合成已完成团队的结果 | P2 |

### 4.5 事件驱动模型

精灵 Session 注册为 Event Observer，订阅子团队完成/失败事件：

```
Team A completed → spirit_team_completed → Spirit Observer
Team B failed    → spirit_team_failed    → Spirit Observer
Team C completed → spirit_team_completed → Spirit Observer
                                              │
                                              ▼
                                    检查：所有 Team 都完成？
                                    ├── YES → 触发 Synthesis
                                    └── NO  → 继续等待
```

新增 EnvelopeType：

| EnvelopeType | 触发时机 | Metadata |
|-------------|---------|----------|
| `spirit_team_progress` | 团队进度更新 | team_id, progress_pct, current_step |
| `spirit_teams_all_completed` | 所有并行团队完成 | spirit_session_id, team_ids, summary |
| `spirit_synthesis_completed` | 合成完成 | spirit_session_id, synthesis_summary |

### 4.6 编排进化闭环

```
用户需求 → 精灵判断 → 组建团队（基于历史 DQ Score 选择编排模式）
    → 团队执行 → Session 执行轨迹记录
        ├──→ DQ Score 计算 → 编排策略优化（下次选择更优模式）
        ├──→ 工具调用模式检测 → Skill 提议 → 能力自动增长
        ├──→ Agent 能力画像更新 → tool_weight 调整 → 工具选择优化
        └──→ 记忆提取 → dream_cycle → 记忆质量提升
```

**DQ Score 驱动的编排缓存**：

| DQ Score | 动作 |
|----------|------|
| > 0.7 | 缓存当前编排拓扑，相似任务优先复用 |
| 0.5 ~ 0.7 | 记录但不操作 |
| 0.3 ~ 0.5 | 生成编排优化建议 |
| < 0.3 | 生成优化建议 + 告警 |

---

## 5. 非功能需求

| 项 | 要求 |
|----|------|
| 架构 | `internal/biz` 不 import `pkg/trpc-agent-go`；精灵构建仅在 `internal/service` |
| 性能 | 并行 3 团队首屏 < 800ms；WS 状态更新 < 200ms；Synthesis 合成 < 3s |
| 并发 | `MaxConcurrentTeams` 可配置，默认 3；超出拒绝并提示 |
| 进化 | 编排策略变更受 `GuardrailMaxChangePerPeriod` 约束；DQ Score < 0.3 触发回滚 |
| 兼容 | M59 精灵模式基础功能不受影响；新工具通过 CustomTools 注入 |
| 前端 | 遵循 UX 规范 token；复用 `ChatExecutionCard` / `SessionStatusBadge` |

---

## 6. 模块边界

| 模块 | 本需求中的职责 |
|------|----------------|
| Chat (1) | 并行团队总览卡片、合成结果卡片、进度面板 |
| Team (11) | 多团队并行创建、TeamKey UUID、依赖调度 |
| Orchestration (53) | Task DAG 拓扑路由、依赖感知调度 |
| Session (10) | Session 树深度限制（MaxSessionDepth=2） |
| Agent (2-8) | 精灵工具扩展（check_team_progress / cancel_team / synthesize_results） |
| Evolution (7) | DQ Score 驱动编排缓存、编排策略进化 |
| Memory (L0-L4) | Synthesis Engine 读取团队执行轨迹 |

**不在范围**：Git Worktree 文件隔离、多团队竞速模式、自适应并行度调整（远期方向）。

---

## 7. 开放问题

| # | 问题 | 选项 | 建议 |
|---|------|------|------|
| 1 | 依赖团队自动启动还是精灵确认后启动？ | A: 自动启动 B: 精灵确认 C: 可配置 | C（默认自动，关键任务需确认） |
| 2 | Synthesis Engine 用 LLM 合成还是模板合成？ | A: LLM 合成 B: 模板合成 C: 混合 | C（简单场景模板，复杂场景 LLM） |
| 3 | 编排拓扑缓存存储位置？ | A: AgentRuntimeSettings JSON B: 独立表 C: Memory L4 | A（利用现有字段） |
| 4 | 并行团队间是否共享 Memory L3/L4？ | A: 共享 B: 隔离 C: 可配置 | A（与 M59 Agent 复用隔离策略一致） |
| 5 | 团队超时后是否自动重试？ | A: 自动重试 B: 仅通知精灵 C: 可配置 | B（与 M59 开放问题 #2 一致） |

---

## 8. 验收标准索引

| ID | 摘要 | 阶段 |
|----|------|------|
| SPO-01 | 同一精灵 Session 支持多团队并行 | P1 |
| SPO-02 | 并行度可配置，超限拒绝 | P1 |
| SPO-03 | 团队进度实时监控 + 精灵主动通知 | P1 |
| SPO-04 | 取消团队 + 释放配额 | P1 |
| SPO-05 | Task DAG 依赖调度 | P2 |
| SPO-06 | 拓扑路由自动选择编排模式 | P2 |
| SPO-07 | Synthesis Engine 结果合成 | P2 |
| SPO-08 | DQ Score 驱动编排缓存 | P2 |
| SPO-09 | 编排策略进化闭环 | P2 |

完整任务拆分见 [开发计划](./60-spirit-parallel-orchestrator-development.md)。
