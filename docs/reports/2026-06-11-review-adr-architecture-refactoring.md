# ADR-01: ChatOrchestrator 拆分为子管理器 + 通用状态机 + 事件可靠性分级

## 状态：已接受

## 背景

ChatOrchestrator 是 service 层的核心编排器，承担了会话运行生命周期、等待/恢复协调、运行状态追踪、待处理队列管理、Agent 依赖构建等职责。随着业务增长，该 struct 出现以下问题：

1. **God Object**：字段数 ~30，4 个 `sync.Map`，认知复杂度远超 AS-COG-01 上限
2. **EventBus 无持久化**：Critical 事件（ToolResult/Error/RunnerCompletion/Checkpoint）丢失后无法恢复
3. **缺少显式状态机**：Run（5 种状态）、SessionRunPhase（5 种阶段）仅有字符串常量，无转换校验
4. **架构不变量无自动验证**：依赖方向、分层隔离等规则仅靠人工审查

## 决策

### D1: ChatOrchestrator 子管理器提取

将 ChatOrchestrator 的职责拆分为 5 个子管理器，每个子管理器定义窄接口（方法数 ≤5）：

| 子管理器 | 接口 | 职责 |
|---------|------|------|
| RunStatusTracker | 5 个子接口组合 | 运行状态追踪、绑定管理、等待元数据 |
| PendingQueueManager | 3 个子接口组合 | 待处理队列、合并后续管理 |
| AwaitCoordinator | 4 个子接口组合 | 等待/恢复协调、通道注册 |
| SessionRunLifecycle | 4 方法 | 会话运行生命周期（开始/完成/升级/配置） |
| AgentBuildDirector | 1 方法 | TRPCBuilderDeps 构建 |

**sync.Map 消除**：引入 `TypedSyncMap[K, V]` 泛型包装，替代原始 `sync.Map` + `timestampedEntry{value: any}` 模式。ChatOrchestrator 的 sync.Map 数量从 4 降至 0。

**参数聚合**：超过 5 个参数的接口方法使用 Option struct 聚合（`SessionRunStartParams`、`AgentBuildParams`、`chatSessionRunLifecycleDeps`）。

### D2: 通用状态机框架

创建 `StateMachine[S ~string, E ~string]` 泛型接口，支持：
- `Transition(from, event) (to, error)` — 带守卫条件的转换
- `CanTransition(from, event) bool` — 可转换性检查
- `ValidTargets(from) []S` — 合法目标状态枚举

实现 `GenericStateMachine[S, E]`，使用双索引（`fromEventIndex` + `fromToIndex`）O(1) 查找。

已迁移的状态机：
- `RunStateMachine`：6 种状态、8 条转换规则
- `SessionRunPhaseMachine`：5 种阶段、8 条转换规则

### D3: 事件可靠性三级分级（AS-EVT-01）

| 级别 | 事件类型 | 可靠性保证 | 持久化 |
|------|---------|-----------|--------|
| Critical | ToolResult / Error / RunnerCompletion / Checkpoint | WBPF（先写后发）+ 重试 | SQLite WAL |
| Important | StateDelta / TokenUsage / RunStatus / SessionStatusChanged / GraphNodeEnd / TeamRunFinished / UserFeedback | BlockUpTo + 异步持久化 | SQLite EventStore |
| Informational | TextDelta / FlowLog / Log / MemberDelta | 尽力而为 | 不持久化 |

**EventWAL 实现**：Critical 事件先写入 `event_wal` SQLite 表，发布成功后标记 `published=1`，启动时 `Recover` 未发布事件。

### D4: 架构 Fitness Function（AS-FIT-01 P0）

实现两个自动化测试：
- `TestBizNotDependOnTrpcAgentGo`：验证 biz 层不依赖 `pkg/trpc-agent-go`
- `TestServiceNotDirectlyAccessData`：验证 service 层不直接访问 data 层

## 后果

### 正面
- ChatOrchestrator 认知复杂度大幅降低，sync.Map 归零
- 状态转换有编译期校验，非法转换在运行时被拦截
- Critical 事件不再丢失，WBPF 保证至少一次投递
- 架构不变量有自动化测试守护

### 负面
- 子管理器增加了间接层，调试时需要跨文件跳转
- EventWAL 增加了 SQLite 写入开销（仅 Critical 事件）
- 状态机泛型约束 `~string` 限制了未来扩展到整数状态枚举

## 替代方案

1. **不拆分，仅添加注释**：无法解决认知复杂度问题，违反 AS-COG-01
2. **使用有限状态机库（如 github.com/looplab/fsm）**：引入外部依赖，且不支持泛型守卫条件
3. **EventBus 全量持久化**：性能开销过大，TextDelta/FlowLog 等高频事件不适合持久化
4. **使用代码生成替代泛型状态机**：增加构建复杂度，泛型方案更简洁
