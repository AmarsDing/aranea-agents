# 团队编排（Team）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/team/`
> 项目实现路径：`internal/team/`、`internal/biz/team_*.go`、`internal/service/team*.go`、`internal/data/team_*.go`
> 当前对齐度：★☆☆☆☆

---

## 一、框架能力全景

### 1.1 核心接口

框架 `team.Team` 实现了 `agent.Agent` 接口，可被 `runner.NewRunner` 直接使用，也可嵌套为其他 Team 的成员。

| 接口 | 方法 | 说明 |
|------|------|------|
| `agent.Agent`（Team 实现） | `Run(ctx, *Invocation) (<-chan *event.Event, error)` | 根据 mode 分发到 `runCoordinator` 或 `runSwarm` |
| `agent.Agent` | `Info() agent.Info` | 返回 `{Name, Description}` |
| `agent.Agent` | `Tools() []tool.Tool` | Coordinator 返回 coordinator 的 tools；Swarm 返回 entry member 的 tools |
| `agent.Agent` | `SubAgents() []agent.Agent` | 返回成员列表副本 |
| `agent.Agent` | `FindSubAgent(name string) agent.Agent` | 按 name 查找成员 |
| `structure.Exporter` | `Export(ctx, ChildExporter) (*Snapshot, error)` | 导出 Team 静态结构图（Node/Edge/Surface） |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `Team` | 核心结构体，持有 coordinator/members/swarm 配置，实现 `agent.Agent` |
| `Mode` | 枚举：`ModeCoordinator`（协调者）、`ModeSwarm`（蜂群） |
| `SwarmConfig` | Swarm 安全限制：`MaxHandoffs`/`NodeTimeout`/`RepetitiveHandoffWindow`/`RepetitiveHandoffMinUnique` |
| `MemberToolConfig` | Coordinator 模式成员工具配置：`StreamInner`/`InnerTextMode`/`SkipSummarization`/`HistoryScope` |
| `SwarmHandoffInputBuilder` | `func(ctx, SwarmHandoffInputArgs) (model.Message, error)`，自定义 handoff 输入消息 |
| `SwarmHandoffInputArgs` | Handoff 上下文：`FromAgentName`/`ToAgentName`/`RootInput`/`ParentInput`/`TransferMessage` |
| `HistoryScope` | 成员历史继承方式：`Default`/`Isolated`/`ParentBranch` |
| `InnerTextMode` | 转发时 assistant 文本可见性：`Default`/`Include`/`Exclude` |
| `swarmRuntime` | Swarm 运行时，实现 `agent.TransferController`，控制 handoff 合法性/会话隔离/状态持久化 |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| `agent.TransferController` | `swarmRuntime` 实现此接口，通过 `chainedTransferController` 与业务 TransferController 链式组合 | 自定义 handoff 控制逻辑 |
| `SwarmHandoffInputBuilder` | 函数式扩展，在 handoff 时改写目标成员输入 | 自定义 handoff 消息格式 |
| `agent.SubAgentSetter` | Swarm 成员必须实现此接口（`SetSubAgents`），LLMAgent 默认满足 | 成员间互相发现 |
| `toolSetAdder` | Coordinator 必须实现此接口（`AddToolSet`），LLMAgent 默认满足 | 动态注入成员工具集 |
| 运行时成员增删 | `UpdateSwarmMembers`/`AddSwarmMember`/`RemoveSwarmMember` | Swarm 模式动态调整成员 |

### 1.4 配置选项

| Option | 适用模式 | 说明 | 默认值 |
|--------|---------|------|--------|
| `WithDescription(desc)` | 通用 | Team 描述 | 空 |
| `WithMemberToolSetName(name)` | Coordinator | 成员工具集名称 | `team-members-<teamName>` |
| `WithMemberToolStreamInner(bool)` | Coordinator | 转发成员流式事件 | false |
| `WithMemberToolInnerTextMode(mode)` | Coordinator | 转发时 assistant 文本可见性 | `InnerTextModeDefault` |
| `WithMemberToolConfig(MemberToolConfig)` | Coordinator | 一次性配置所有成员工具选项 | `HistoryScopeParentBranch` |
| `WithSwarmConfig(SwarmConfig)` | Swarm | Swarm 安全限制 | `MaxHandoffs=20, Window=8, MinUnique=3` |
| `WithCrossRequestTransfer(bool)` | Swarm | 跨请求 transfer（下次从上次活跃 Agent 开始） | false |
| `WithSwarmIndependentAgents()` | Swarm | 成员独立 session（历史隔离） | 共享 root session |
| `WithSwarmHandoffInputBuilder(builder)` | Swarm | 自定义 handoff 目标成员输入 | 使用 transfer_to_agent 消息 |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| Coordinator Team | `team.New()` | 协调者模式：coordinator 将 members 作为 AgentTool 调用 |
| Swarm Team | `team.NewSwarm()` | 蜂群模式：成员间通过 `transfer_to_agent` 自主交接 |
| `swarmRuntime` | `team/runtime.go` | TransferController 实现：handoff 安全检查 + session 隔离 + 事件路由 |
| `chainedTransferController` | `team/runtime.go` | TransferController 链式组合机制 |
| 结构导出 | `team/structure_export.go` | 导出 Team 静态结构图（Coordinator 星型 / Swarm 全连接） |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `team.New()` Coordinator 模式 | ❌ 未使用 | ❌ | 项目未使用框架 Coordinator Team |
| `team.NewSwarm()` Swarm 模式 | ❌ 未使用 | ❌ | 项目未使用框架 Swarm Team |
| `MemberToolConfig` | ❌ 未使用 | ❌ | 项目无 Coordinator 模式，无需成员工具配置 |
| `SwarmConfig` | ❌ 未使用 | ❌ | 项目 Swarm/Adaptive 通过 Graph 实现，不走框架 Swarm |
| `WithCrossRequestTransfer` | ❌ 未使用 | ❌ | 项目无跨请求 transfer 需求 |
| `WithSwarmIndependentAgents` | ❌ 未使用 | ❌ | 项目成员 session 由 Graph 运行时管理 |
| `WithSwarmHandoffInputBuilder` | ❌ 未使用 | ❌ | 项目 handoff 由 Graph 条件边控制 |
| `UpdateSwarmMembers`/`AddSwarmMember`/`RemoveSwarmMember` | ⚠️ 部分对齐 | ⚠️ | 项目 `TeamUsecase.UpdateSwarmMembers` 功能类似但实现不同 |
| `Export()` 结构导出 | ⚠️ 部分对齐 | ⚠️ | 项目 `ExportStructure` 功能类似但输出格式不同 |
| `agent.Agent` 接口（Team 实现） | ❌ 未使用 | ❌ | 项目 Team 不是 `agent.Agent`，而是业务实体 |
| `TransferController` 链式组合 | ❌ 未使用 | ❌ | 项目 handoff 由 Graph 条件边实现 |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| 6 种编排模式（sequential/parallel/coordinator/critic_loop/swarm/adaptive） | `internal/team/template_registry.go`、`internal/team/graph_compile.go` | 框架仅 Coordinator + Swarm 两种 | 框架功能不足，无法覆盖 6 种模式 |
| Team Definition 解析 | `internal/team/definition.go` | 框架无此概念 | 框架 Team 是纯代码构造，项目需要 JSON 定义 |
| Team 编译为 Graph | `internal/team/runner_team_compiler.go`、`internal/team/graph_compile.go` | 框架无此概念 | 项目选择 Graph 作为统一执行引擎 |
| TeamRunner 执行引擎 | `internal/team/runner.go`、`internal/team/runner_team_trpc.go` | 框架 `team.Team.Run()` | 项目通过 Graph 运行时执行，不走框架 Team.Run |
| TeamRunMediator（中介者） | `internal/team/runner_mediator.go` | 框架无此概念 | 打破 Runner <-> Coordinator 循环依赖 |
| TeamGraphRunCoordinator | `internal/team/team_graph_run_coordinator.go` | 框架无此概念 | 管理 Graph 执行注册、HITL 延迟、Task 恢复 |
| Team 状态机（7 种状态） | `internal/biz/team_state_machine.go` | 框架无此概念 | 框架 Team 无持久化状态 |
| TeamRun 状态机（6 种状态） | `internal/biz/team_run_state_machine.go` | 框架无此概念 | 框架 Team 无运行持久化 |
| SpiritTeamUsecase（三域架构） | `internal/biz/spirit_team_usecase.go` | 框架无此概念 | 精灵团队 DAG 编排，框架无对应能力 |
| FallbackPolicy（引擎级回退） | `internal/biz/team_fallback.go` | 框架无此概念 | Graph vs Native 运行时回退 |
| TeamFailurePolicy（节点级失败策略） | `internal/biz/failure_policy.go` | 框架无此概念 | retry_then_block/skip/fail_fast/continue/abort/halt |
| TeamRun 持久化 | `internal/data/team_repo.go` | 框架无此概念 | 框架 Team 无数据库持久化 |
| TeamRunStep 持久化 | `internal/data/team_repo.go` | 框架无此概念 | 框架无步骤级追踪 |
| TeamGraphSession 追踪 | `internal/data/team_graph_session_repo.go` | 框架无此概念 | Graph 运行时会话追踪 |
| CompiledTeam 缓存 | `internal/data/compiled_team_repo.go` | 框架无此概念 | 编译产物持久化 + 哈希缓存失效 |
| 动态成员管理 | `internal/biz/team_usecase.go`（`UpdateSwarmMembers`） | `team.UpdateSwarmMembers` | 项目通过数据库更新，框架通过内存操作 |
| 结构导出 | `internal/biz/team_usecase.go`（`ExportStructure`） | `team.Export()` | 项目导出业务结构，框架导出 Agent 结构 |
| TeamRunSummary 聚合 | `internal/biz/team_summary.go` | 框架无此概念 | 运行摘要数据聚合 |
| TeamRunObserver | `internal/team/runner_team_observer.go` | 框架无此概念 | 运行时生命周期事件观察 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `team.New()` Coordinator 模式 | 项目 coordinator 模式通过 Graph dispatch 模板实现，功能更强 | 否，Graph 实现更灵活 |
| `team.NewSwarm()` Swarm 模式 | 项目 swarm/adaptive 通过 Graph adaptive 模板实现 | 否，Graph 实现更灵活 |
| `MemberToolConfig` | 项目无 Coordinator 模式 AgentTool 包装 | 否 |
| `SwarmConfig` 安全限制 | 项目通过 Graph 条件边 + TeamFailurePolicy 实现类似安全控制 | 评估中，部分概念可借鉴 |
| `WithCrossRequestTransfer` | 项目通过 Session 状态 + Graph 恢复实现跨请求续跑 | 评估中，概念类似但实现路径不同 |
| `WithSwarmIndependentAgents` | 项目成员 session 由 Graph 运行时管理 | 评估中，概念类似但实现路径不同 |
| `SwarmHandoffInputBuilder` | 项目 handoff 由 Graph 条件边控制 | 否，Graph 条件边更灵活 |
| `TransferController` 链式组合 | 项目无 TransferController 使用场景 | 否 |
| `Export()` 结构导出 | 项目有独立的 `ExportStructure` 实现 | 评估中，可考虑适配框架 Export |
| 运行时成员增删 | 项目通过数据库 + 重新编译实现 | 评估中，可借鉴框架的内存操作模式 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | **Team 是 `agent.Agent`**：可直接被 Runner 使用，可嵌套为其他 Team 的成员 | 项目 Team 是业务实体（`biz.Team`），不是 `agent.Agent`，无法直接被框架 Runner 使用 | 若项目 Team 实现框架 `agent.Agent`，可复用框架 Runner/Session/Memory 生态 |
| 2 | **Coordinator 模式 AgentTool 包装**：成员自动包装为 `agenttool.NewTool`，coordinator 像调用工具一样调用成员 | 项目 coordinator 模式通过 Graph dispatch 模板实现，成员作为 Graph 节点执行 | 框架方式更轻量，但项目 Graph 方式更灵活（支持条件边、子图） |
| 3 | **Swarm 安全默认值**：`MaxHandoffs=20`、循环检测（`Window=8, MinUnique=3`） | 项目通过 `TeamFailurePolicy` 的 retry/fail_fast 等策略控制，无专门的 handoff 循环检测 | 可借鉴框架的循环检测机制，增强 adaptive 模式的安全性 |
| 4 | **Session 隔离机制**：`WithSwarmIndependentAgents()` 为非 entry 成员创建独立 session，事件路由到对应 session | 项目成员 session 由 Graph 运行时管理，通过 `TeamGraphSession` 追踪 | 框架的 session 隔离设计更规范，可参考其 session ID 派生规则 |
| 5 | **TransferController 链式组合**：`swarmRuntime` 与业务 TransferController 互不干扰 | 项目无 TransferController 概念 | 若未来需要自定义 handoff 控制，可复用此机制 |
| 6 | **结构导出**：`Export()` 导出 Node/Edge/Surface，Coordinator 星型 / Swarm 全连接 | 项目 `ExportStructure` 导出业务结构（含成员角色、依赖关系等） | 框架导出更标准化，可考虑适配 |
| 7 | **嵌套 Team**：Team 实现了 `agent.Agent`，可作为其他 Team 的成员 | 项目不支持 Team 嵌套 | 框架天然支持，项目若实现 `agent.Agent` 可免费获得 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | **6 种编排模式**：sequential/parallel/coordinator/critic_loop/swarm/adaptive | 框架仅 Coordinator + Swarm 两种 | 贡献回框架（sequential/parallel/critic_loop 作为 Graph 模板） |
| 2 | **Graph 统一执行引擎**：所有模式编译为 Graph 拓扑，支持条件边、子图、中断/恢复 | 框架 Team 直接执行 Agent，无 Graph 编译 | 贡献回框架（Graph 编译层） |
| 3 | **Team Definition JSON**：声明式定义 Team 编排，支持动态修改 | 框架 Team 是纯代码构造（`New`/`NewSwarm`），不支持运行时修改定义 | 贡献回框架（Definition 解析层） |
| 4 | **Team/TeamRun 状态机**：7 种 Team 状态 + 6 种 TeamRun 状态，支持中断/恢复/重试 | 框架 Team 无持久化状态 | 保持自建（业务特有需求） |
| 5 | **HITL（Human-in-the-Loop）**：`TeamGraphRunCoordinator` 支持 HITL 延迟、恢复 | 框架无 HITL 支持 | 贡献回框架 |
| 6 | **Spirit 团队 DAG 编排**：`SpiritTeamUsecase` 三域架构（Assembly/Orchestration/Delivery） | 框架无此概念 | 保持自建（业务特有需求） |
| 7 | **FallbackPolicy**：Graph vs Native 运行时回退 + 金丝雀灰度 | 框架无回退机制 | 保持自建（过渡期需要） |
| 8 | **TeamFailurePolicy**：节点级失败策略（retry/skip/fail_fast/continue/abort/halt） | 框架仅有 SwarmConfig 的 MaxHandoffs/循环检测 | 贡献回框架 |
| 9 | **TeamRunStep 持久化**：记录每个成员 Agent 的执行步骤 | 框架无步骤级追踪 | 保持自建（业务特有需求） |
| 10 | **CompiledTeam 缓存**：编译产物持久化 + 哈希缓存失效 | 框架无编译缓存 | 保持自建（性能优化） |
| 11 | **动态成员管理（持久化）**：通过数据库更新成员，重新编译 | 框架 `UpdateSwarmMembers` 仅内存操作，重启丢失 | 贡献回框架（持久化扩展） |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| 项目完全未使用框架 `team.Team` | **架构决策**：项目选择 Graph 作为统一执行引擎，所有模式编译为 Graph 拓扑，而非使用框架的 AgentTool/Transfer 机制 | 全局：Team 运行时、编译、执行 |
| 项目 6 种模式 vs 框架 2 种模式 | **功能缺失**：框架仅 Coordinator + Swarm，无法覆盖 sequential/parallel/critic_loop/adaptive | 编译层：`template_registry.go`、`graph_compile.go` |
| 项目 Team 是业务实体而非 `agent.Agent` | **架构决策**：项目 Team 需要持久化（SQLite）、状态机、CRUD API，与框架 `agent.Agent` 的无状态设计不匹配 | Biz/Data 层：`team_types.go`、`team_repo.go` |
| 项目 Team 编译为 Graph | **架构决策**：Graph 提供条件边、子图、中断/恢复、检查点等高级能力，框架 Team 的 AgentTool/Transfer 机制无法满足 | 运行时：`runner_team_compiler.go`、`team_graph_run_coordinator.go` |
| 项目 HITL 支持 | **功能缺失**：框架无 Human-in-the-Loop 支持 | 运行时：`team_graph_run_coordinator.go` |
| 项目 Spirit 团队 DAG | **业务特有**：精灵会话下的多 Team DAG 编排，框架无此概念 | Biz 层：`spirit_team_usecase.go` |
| 项目 FallbackPolicy | **过渡期需要**：Graph 运行时成熟前的安全网 | 运行时：`team_fallback.go` |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 适配框架 `Export()` 结构导出 | 新增适配层 | P2 | `internal/biz/team_usecase.go` | 统一结构导出格式，与框架生态兼容 |
| 2 | 借鉴框架 Swarm 安全机制增强 adaptive 模式 | 启用框架功能 | P2 | `internal/team/graph_compile.go` | 增强 adaptive 模式的 handoff 安全性 |
| 3 | 借鉴框架 Session 隔离机制优化成员 session 管理 | 启用框架功能 | P2 | `internal/team/team_graph_run_coordinator.go` | 规范化成员 session ID 派生和事件路由 |
| 4 | 贡献 sequential/parallel/critic_loop 模式回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/team/` | 减少自建维护负担，丰富框架能力 |
| 5 | 贡献 Graph 编译层回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/team/` | 减少自建维护负担，使框架 Team 支持 Graph 执行 |
| 6 | 贡献 TeamFailurePolicy 回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/team/` | 减少自建维护负担，增强框架 Team 的失败处理 |
| 7 | 贡献 HITL 支持回框架 | 贡献回框架 | P3 | `pkg/trpc-agent-go/team/` | 减少自建维护负担，使框架 Team 支持 HITL |
| 8 | 评估项目 Team 实现框架 `agent.Agent` 接口的可行性 | 评估中 | P3 | `internal/team/` | 若可行，可复用框架 Runner/Session/Memory 生态 |

### 4.2 对齐项详情

#### 对齐项 #1：适配框架 `Export()` 结构导出

**类型**：新增适配层

**现状**：
- 项目当前实现：`TeamUsecase.ExportStructure` 导出业务结构（含成员角色、依赖关系、DAG 拓扑等），输出为自定义 JSON 格式
- 框架提供能力：`team.Team.Export()` 导出 `structure.Snapshot`（Node/Edge/Surface），Coordinator 星型 / Swarm 全连接

**对齐方案**：
1. 在 `internal/team/` 中新增 `structure_adapter.go`，将项目的 `ExportStructure` 输出适配为框架 `structure.Snapshot` 格式
2. 保留项目现有的 `ExportStructure` 业务接口不变，适配层作为可选输出格式
3. 前端可按需选择使用业务格式或框架标准格式

**代码变更范围**：
- 新增：`internal/team/structure_adapter.go`（约 80 行）
- 修改：`internal/biz/team_usecase.go`（新增 `ExportStructureFramework` 方法）
- 修改：`internal/service/team.go`（新增 gRPC 端点或扩展现有端点）

**兼容性风险**：
- 低：新增适配层，不影响现有 `ExportStructure` 功能

**回退方案**：
- 删除适配层代码，恢复原有 `ExportStructure` 即可

**验证方法**：
- 单元测试：验证适配层输出的 `structure.Snapshot` 与框架 `Export()` 格式一致
- 集成测试：对比项目导出与框架导出的结构图

**预期收益**：
- 代码减少：0 行（新增约 80 行适配代码）
- 性能影响：可忽略
- 维护成本：与框架结构导出格式对齐，未来框架可视化工具可直接使用
- 功能增强：获得框架结构导出生态兼容性

---

#### 对齐项 #2：借鉴框架 Swarm 安全机制增强 adaptive 模式

**类型**：启用框架功能

**现状**：
- 项目当前实现：adaptive 模式通过 Graph 条件边 + `TeamFailurePolicy` 控制，无专门的 handoff 循环检测
- 框架提供能力：`SwarmConfig` 提供 `MaxHandoffs`（最大交接次数）、`RepetitiveHandoffWindow`（循环检测窗口）、`RepetitiveHandoffMinUnique`（窗口内最少不同 Agent 数）

**对齐方案**：
1. 在 `RunnerConfig` 或 `TeamFailurePolicy` 中新增 `MaxHandoffs` 和循环检测参数
2. 在 `compileTeamRuntime` 编译 adaptive 模式时，将安全参数注入 Graph 节点的条件函数
3. 运行时在 handoff 决策点检查 handoff 次数和循环模式

**代码变更范围**：
- 修改：`internal/biz/failure_policy.go`（新增 `MaxHandoffs`/`RepetitiveHandoffWindow`/`RepetitiveHandoffMinUnique` 字段）
- 修改：`internal/team/runner_team_compiler.go`（编译时注入安全参数）
- 修改：`internal/team/graph_compile.go`（adaptive 模式条件函数增加安全检查）
- 新增：`internal/team/handoff_guard.go`（handoff 安全检查逻辑，约 60 行）

**兼容性风险**：
- 中：新增安全限制可能影响现有 adaptive 模式的行为，需提供开关和默认值
- 缓解：默认关闭，通过配置启用

**回退方案**：
- 关闭安全检查开关即可恢复原有行为

**验证方法**：
- 单元测试：验证循环检测逻辑（连续 handoff 到同一 Agent 被拦截）
- 集成测试：adaptive 模式下 handoff 次数超限时的行为

**预期收益**：
- 代码减少：0 行（新增约 60 行）
- 性能影响：可忽略（仅计数和窗口检查）
- 维护成本：减少因无限 handoff 循环导致的运行时问题
- 功能增强：adaptive 模式获得循环检测和 handoff 限制能力

---

#### 对齐项 #3：借鉴框架 Session 隔离机制优化成员 session 管理

**类型**：启用框架功能

**现状**：
- 项目当前实现：成员 session 由 Graph 运行时通过 `TeamGraphSession` 追踪，session ID 格式为数据库自增 ID
- 框架提供能力：`WithSwarmIndependentAgents()` 为非 entry 成员创建独立 session，ID 格式为 `<parentID>/<teamName>/<memberName>`，事件通过 `RouteEvent` 路由到对应 session

**对齐方案**：
1. 评估项目 `TeamGraphSession` 的 session ID 派生规则是否可对齐框架的 `<parentID>/<teamName>/<memberName>` 格式
2. 若可行，在 `TeamGraphRunCoordinator` 中引入框架的 `RouteEvent` 机制，将成员事件路由到对应 session
3. 保留项目现有的 `TeamGraphSession` 数据库追踪，框架 session ID 作为逻辑映射

**代码变更范围**：
- 修改：`internal/team/team_graph_run_coordinator.go`（session ID 派生规则对齐）
- 修改：`internal/team/team_graph_run_context.go`（事件路由逻辑）
- 修改：`internal/data/team_graph_session_repo.go`（session ID 格式适配）

**兼容性风险**：
- 高：session ID 格式变更影响数据库中已有记录和前端展示
- 缓解：渐进式迁移，新 TeamRun 使用新格式，旧记录保持不变

**回退方案**：
- 恢复原有 session ID 格式

**验证方法**：
- 单元测试：验证新 session ID 派生规则与框架一致
- 集成测试：验证成员事件正确路由到对应 session
- 回归测试：验证旧格式记录仍可正常读取

**预期收益**：
- 代码减少：0 行
- 性能影响：可忽略
- 维护成本：与框架 session 管理机制对齐，减少自建 session 追踪逻辑
- 功能增强：获得框架 session 隔离的标准化能力

---

#### 对齐项 #4：贡献 sequential/parallel/critic_loop 模式回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：6 种编排模式通过 `template_registry.go` 和 `graph_compile.go` 编译为 Graph 拓扑
- 框架提供能力：仅 Coordinator + Swarm 两种模式

**对齐方案**：
1. 将项目的 `sequential`/`parallel`/`critic_loop` 模式抽象为框架 `team` 包的扩展
2. 设计框架层的模式注册机制（`team.RegisterMode`），允许第三方注册新的编排模式
3. 将项目的 Graph 模板编译逻辑提取为框架可用的模式实现
4. 项目通过注册机制使用自建模式，而非完全自建

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/team/mode_registry.go`（模式注册机制，约 100 行）
- 新增：`pkg/trpc-agent-go/team/mode_sequential.go`（约 80 行）
- 新增：`pkg/trpc-agent-go/team/mode_parallel.go`（约 80 行）
- 新增：`pkg/trpc-agent-go/team/mode_critic_loop.go`（约 100 行）
- 修改：`pkg/trpc-agent-go/team/team.go`（`Run` 方法支持注册模式分发）
- 修改：`internal/team/template_registry.go`（改为使用框架注册机制）

**兼容性风险**：
- 中：框架 API 变更需要上游审核
- 缓解：先以独立扩展包形式贡献，不修改框架核心 API

**回退方案**：
- 项目继续使用自建模式注册

**验证方法**：
- 框架单元测试：验证新模式的 Graph 拓扑生成正确
- 项目集成测试：验证注册模式与自建模式行为一致

**预期收益**：
- 代码减少：约 200 行（`template_registry.go` 中可删除的模式定义）
- 性能影响：无
- 维护成本：模式逻辑由框架维护，项目仅需注册使用
- 功能增强：框架用户可获得更多编排模式

---

#### 对齐项 #5：贡献 Graph 编译层回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`compileTeamRuntime` 将 Team Definition 编译为 `GraphBuildConfig`，再通过 `BuildStateGraphWithRegistryAndLogger` 编译为框架 `Graph`，包装为 `GraphAgent`
- 框架提供能力：Team 直接执行 Agent（Coordinator 通过 AgentTool，Swarm 通过 TransferController），无 Graph 编译

**对齐方案**：
1. 在框架 `team` 包中新增 `team.WithGraphExecution()` Option，启用 Graph 执行模式
2. 将项目的 `compileTeamRuntime` 核心逻辑提取为框架的 Graph 编译器
3. 框架 Team 在 `Run()` 时根据配置选择直接执行或 Graph 执行

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/team/graph_compiler.go`（约 300 行）
- 修改：`pkg/trpc-agent-go/team/team.go`（`Run` 方法支持 Graph 执行路径）
- 修改：`pkg/trpc-agent-go/team/options.go`（新增 `WithGraphExecution` Option）
- 修改：`internal/team/runner_team_compiler.go`（改为使用框架 Graph 编译器）

**兼容性风险**：
- 高：框架核心 API 变更，需上游审核
- 缓解：以 Option 形式提供，默认不启用，完全向后兼容

**回退方案**：
- 项目继续使用自建 Graph 编译

**验证方法**：
- 框架单元测试：验证 Graph 编译输出与项目一致
- 项目集成测试：验证框架 Graph 编译器与自建编译器行为一致

**预期收益**：
- 代码减少：约 300 行（`runner_team_compiler.go` 可大幅简化）
- 性能影响：无
- 维护成本：Graph 编译逻辑由框架维护
- 功能增强：框架 Team 获得 Graph 执行能力（条件边、子图、检查点）

---

#### 对齐项 #6：贡献 TeamFailurePolicy 回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`TeamFailurePolicy` 支持 retry_then_block/skip/fail_fast/continue/abort/halt，含重试策略、节点覆盖、熔断器、并行失败策略
- 框架提供能力：仅有 `SwarmConfig.MaxHandoffs` 和循环检测

**对齐方案**：
1. 将项目的 `TeamFailurePolicy` 抽象为框架 `team` 包的扩展
2. 设计框架层的 `team.WithFailurePolicy(FailurePolicy)` Option
3. 在框架 Team 的 `Run()` 中集成失败策略检查

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/team/failure_policy.go`（约 200 行）
- 修改：`pkg/trpc-agent-go/team/options.go`（新增 `WithFailurePolicy` Option）
- 修改：`pkg/trpc-agent-go/team/runtime.go`（集成失败策略到 TransferController）

**兼容性风险**：
- 中：框架 API 扩展，需上游审核
- 缓解：以 Option 形式提供，默认不启用

**回退方案**：
- 项目继续使用自建 `TeamFailurePolicy`

**验证方法**：
- 框架单元测试：验证各失败策略行为
- 项目集成测试：验证框架失败策略与自建策略行为一致

**预期收益**：
- 代码减少：约 150 行（`failure_policy.go` 可简化为框架调用）
- 性能影响：无
- 维护成本：失败策略由框架维护
- 功能增强：框架 Team 获得节点级失败处理能力

---

#### 对齐项 #7：贡献 HITL 支持回框架

**类型**：贡献回框架

**现状**：
- 项目当前实现：`TeamGraphRunCoordinator` 支持 HITL 延迟（`DeferTeamRunSuccessIfHITL`）、恢复（`HandleTeamGraphTaskCompleted`）、超时管理（`WatchTimeout`/`HITLSLATimeout`）
- 框架提供能力：无 HITL 支持

**对齐方案**：
1. 在框架 `team` 包中新增 `team.WithHITL(HITLConfig)` Option
2. 将项目的 HITL 核心逻辑（延迟完成、恢复、超时）提取为框架扩展
3. 框架 Team 在 Swarm/Coordinator 模式下均支持 HITL 中断点

**代码变更范围**：
- 新增：`pkg/trpc-agent-go/team/hitl.go`（约 200 行）
- 修改：`pkg/trpc-agent-go/team/options.go`（新增 `WithHITL` Option）
- 修改：`pkg/trpc-agent-go/team/runtime.go`（集成 HITL 到 TransferController）

**兼容性风险**：
- 中：框架 API 扩展，需上游审核
- 缓解：以 Option 形式提供，默认不启用

**回退方案**：
- 项目继续使用自建 HITL 机制

**验证方法**：
- 框架单元测试：验证 HITL 延迟/恢复/超时行为
- 项目集成测试：验证框架 HITL 与自建 HITL 行为一致

**预期收益**：
- 代码减少：约 150 行（`team_graph_run_coordinator.go` 中 HITL 相关代码可简化）
- 性能影响：无
- 维护成本：HITL 逻辑由框架维护
- 功能增强：框架 Team 获得 Human-in-the-Loop 能力

---

#### 对齐项 #8：评估项目 Team 实现框架 `agent.Agent` 接口的可行性

**类型**：评估中

**现状**：
- 项目当前实现：`biz.Team` 是业务实体（23 个字段，含 `TeamKey`/`Status`/`DefinitionJSON` 等），不实现 `agent.Agent`
- 框架提供能力：`team.Team` 实现 `agent.Agent`，可被 `runner.NewRunner` 直接使用

**对齐方案**：
1. 评估在 `internal/team/` 中新增 `team_agent.go`，将 `Runner` 包装为 `agent.Agent` 的可行性
2. 包装后的 `TeamAgent` 实现 `Run()`/`Info()`/`Tools()`/`SubAgents()`/`FindSubAgent()` 方法
3. `Run()` 内部调用 `runTeamTRPCFromInput`，将事件流转换为框架事件格式
4. 评估是否可将 `TeamAgent` 直接传给框架 `runner.NewRunner`，替代项目自建的 `TurnRunner`

**代码变更范围**：
- 新增：`internal/team/team_agent.go`（约 150 行）
- 修改：`internal/team/runner.go`（构造函数新增 `TeamAgent` 创建逻辑）
- 修改：Wire 注入（`TeamAgent` 替代部分 `TurnRunner` 使用场景）

**兼容性风险**：
- 高：`agent.Agent` 接口的 `Run()` 方法签名与项目 Team 执行流程差异较大
  - 框架 `Run()` 接收 `*agent.Invocation`，项目 `runTeamTRPCFromInput` 接收业务参数（sessionID/messageID/mode 等）
  - 框架 `Run()` 返回 `<-chan *event.Event`，项目通过 EventBus + WebSocket 推送事件
  - 需要适配层将业务参数映射到 `agent.Invocation`，将 EventBus 事件转换为框架事件流

**回退方案**：
- 不实现 `agent.Agent`，保持现有执行流程

**验证方法**：
- 概念验证（PoC）：创建最小 `TeamAgent` 实现，验证能否被框架 `runner.NewRunner` 使用
- 性能测试：对比 `TeamAgent` + 框架 Runner vs 自建 `TurnRunner` 的性能

**预期收益**：
- 代码减少：约 200 行（若 `TurnRunner` 可被框架 Runner 替代）
- 性能影响：需评估，框架 Runner 可能引入额外开销
- 维护成本：若可行，可复用框架 Runner/Session/Memory 生态，减少自建
- 功能增强：获得框架 Runner 的所有能力（自动 Session 管理、Memory 集成等）

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #1（Export 适配）、#2（Swarm 安全机制） | 无 | 中 |
| Phase 2 | #3（Session 隔离优化） | Phase 1 | 中 |
| Phase 3 | #8（agent.Agent 可行性评估） | Phase 2 | 中（评估 + PoC） |
| Phase 4 | #4（贡献模式）、#5（贡献 Graph 编译）、#6（贡献 FailurePolicy）、#7（贡献 HITL） | Phase 3 评估结论 | 大（需框架上游审核） |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架上游拒绝贡献（API 设计分歧） | 中 | 高 | 先以独立扩展包形式贡献，不修改框架核心 API；与框架团队提前沟通设计 |
| Session ID 格式变更导致数据不兼容 | 中 | 高 | 渐进式迁移，新 TeamRun 使用新格式，旧记录保持不变；提供迁移脚本 |
| `agent.Agent` 适配层性能开销过大 | 低 | 中 | PoC 阶段进行性能基准测试，若开销过大则放弃此对齐项 |
| Graph 编译逻辑提取后项目行为不一致 | 中 | 高 | 提取前先补全项目 Team 编译的集成测试，提取后对比测试 |
| HITL 贡献回框架后项目需适配新 API | 低 | 低 | 项目保持自建 HITL 作为 fallback，渐进切换 |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| Coordinator Team | `examples/team/coord/` | `team.New(coordinator, members, WithMemberToolConfig(...))` | 代码构造：创建 coordinator Agent + member Agents → `team.New()` → `runner.NewRunner()` | 项目 coordinator 模式通过 Graph dispatch 模板实现，不使用 `team.New()` |
| Swarm Team | `examples/team/swarm/` | `team.NewSwarm(name, entryName, members, WithCrossRequestTransfer(true))` | 代码构造：创建 member Agents → `team.NewSwarm()` → `runner.NewRunner()` | 项目 swarm/adaptive 通过 Graph adaptive 模板实现，不使用 `team.NewSwarm()` |
| 嵌套 Team | `examples/team/README.md` | 内层 `team.New()` → 外层 `team.New(outerCoord, []agent.Agent{innerTeam, ...})` | Team 作为 `agent.Agent` 嵌套 | 项目不支持 Team 嵌套 |
| Chat Helper | `examples/team/internal/chat/` | `chat.New()` 辅助函数 | 封装 Runner + Session 创建 | 项目有 `TurnRunner` 封装，但实现完全不同 |

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| Team 中文文档 | `docs/mkdocs/zh/team.md` |
| Team 英文文档 | `docs/mkdocs/en/team.md` |
| Coordinator 示例 README | `examples/team/coord/README.md` |
| Swarm 示例 README | `examples/team/swarm/README.md` |
| Team 示例总览 README | `examples/team/README.md` |
