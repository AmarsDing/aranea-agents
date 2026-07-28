# ADR-06: 系统 Agent 团队成员身份——过滤点从事件/会话创建上移到分配时

## 状态：已接受（2026-07-28）

## 背景

12:33 会话（skill 安装编排）暴露成员状态管线断裂：Graph 显示所有成员「执行中」，与汇报结果不符。根因排查发现 2026-07-04「问题 5 修复」在**事件/会话创建层**过滤系统 Agent：

1. `biz/spirit_team_usecase.go` AssembleTeam：系统 Agent（`__spirit__`/`__system_admin__`/`__memory__`/`__skills__`）跳过 agent session 创建；
2. `service/spirit_team.go` publishSpiritTeamAssembled：filteredKeys 把系统 Agent 排除在 MemberSession created 事件与 TeamStage.Members 之外。

当时意图：系统 Agent 不参与业务团队，避免其 MemberSession 永远停在 running（无 DB session → completion 搜索不到 → 无 updated 事件）。

但 12:33 场景证明：**Planner 显式把 `__system_admin__` 编入团队定义是合法需求**（安装 skill 只能由系统管家经 `cli_admin_skill_install_from_url` 完成；`system_admin` profile 不含 exec_command，业务 Agent 也无权访问 cli_admin 工具组）。一旦系统 Agent 进入团队定义，它就是该团队的一等成员：参与 graph 执行、产生 steps、需要完整的 MemberSession 生命周期（created → updated）与前端状态显示。在事件/会话层过滤造成「成员在执行但无状态实体」的撕裂：Graph 无卡或卡死 running、成员执行面板无内容、完成事件无法触达。

## 决策

**过滤点上移：从「事件/会话创建时」移到「分配时」。**

1. **删除创建层过滤**：AssembleTeam 不再跳过系统 Agent 的 agent session 创建；publishSpiritTeamAssembled 不再过滤 MemberSession 事件成员。凡进入团队 Definition 的成员（含系统 Agent），一律拥有完整 MemberSession 生命周期。
2. **分配层保持约束**：AgentAllocator 不自动把系统 Agent 分配给普通业务团队（`IsSystemAgentKey` 注释声明的既有意图不变）；Planner prompt 增加显式规则——仅当任务需要 cli_admin 能力（安装/配置/管理类）时才允许指定 `__system_admin__` 为执行成员。
3. **结果导向状态**：成员状态按个体执行证据判定（steps 错误 / 契约 required topic 未产出 → failed），不再随团队主状态全员同刷（详见 11-multi-agent.development.md Phase 11 F10）。

## 后果

**正面**：
- 系统 Agent 成员的状态管线完整：Graph 卡片状态真实、成员执行面板有内容、完成事件必达。
- 「安装 skill / 系统配置」类任务获得一等编排能力，无需绕过团队机制。
- 过滤单一收口在分配层，语义清晰：定义即真相（Definition 中的成员 = 需要完整生命周期的成员）。

**负面 / 风险**：
- 若 Allocator/Planner 约束失守，系统 Agent 可能被编入普通业务团队并出现在 Graph 成员列表（信息噪音）。缓解：Planner prompt 规则 + 审查检查点。
- `__spirit__` 作为 synthesizer 进入定义时也会产生 MemberSession——接受此行为（synthesizer 本就执行汇总工作，状态可见性有益）。

## 替代方案

1. **维持创建层过滤，系统任务不走团队机制**（精灵直接调 cli_admin 工具）：否决——精灵 profile 无 cli_admin 工具；且单步任务绕过团队会失去统一的状态/观测/重试语义。
2. **系统 Agent 创建 MemberSession 但前端隐藏**：否决——状态实体与显示解耦会造成「有数据无展示」的隐性不一致，排查困难；且用户明确要求成员状态可见、以结果为导向。
3. **仅补完成事件兜底（不改创建层）**：否决——治标不治本，成员执行过程（步骤流）仍无挂载点。
