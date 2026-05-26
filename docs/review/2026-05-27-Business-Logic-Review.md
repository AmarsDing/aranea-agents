# Aranea-Agents 业务逻辑 Review 报告

> 日期：2026-05-27 | 范围：全项目业务逻辑 | 审查人：AI Code Reviewer

---

## 一、项目总体评价

Aranea-Agents 是一个**架构成熟度较高**的多智能体编排平台。Kratos v2 传输壳层 + trpc-agent-go 运行时内核的双框架分工清晰，分层约束（service → biz → data）执行到位。以下按模块逐一分析业务逻辑的亮点与风险。

---

## 二、核心业务模块 Review

### 1. Chat / Turn 生命周期（M55 核心）

**架构亮点：**
- `turn_executor.go` 定义了统一的 `TurnExecutor` 接口，Web/WS/Channel/Cron/A2A 所有入口共享同一 Turn 路径，消除了历史遗留的多路径分歧
- `turn_gateway.go` 将 ChatService 拆分为 `TurnGateway` / `TurnControlGateway` / `PendingMessageGateway` 三个窄接口，遵循接口隔离原则
- `TurnAdmissionDecider` / `SessionLocker` / `RunRegistry` 等 Hook 抽象，让 Agent Turn 和 Team Turn 共享准入/锁/持久化基础设施

**风险点：**
- **ChatUsecase.awaitChans 使用 `sync.Map` + `interface{}`**：类型安全弱，`LoadAwaitChannel` 返回 `interface{}` 需要调用方做类型断言。建议改为 `sync.Map` value 为 `chan awaitReplyCh`，消除运行时 panic 风险
- **GC 协程的 await channel 最大 30 分钟硬编码**：长任务场景（如 Durable Phase）可能超过 30 分钟，channel 被清理后 `AwaitUserReply` 将永久失败。应与 `SessionRunBudget.HardBudgetSec` 对齐
- **EnqueueUserMessage 的 reject 逻辑分散**：service 层先做了一次 `EvaluateIngressPolicy`，然后 `orch.EnqueueUserMessage` 内部又做了一次 `ChatUsecase.EnqueueUserMessage` 的准入检查。双重检查逻辑不一致时会产生行为差异

### 2. Agent 目录管理

**架构亮点：**
- `AgentUsecase` 的 Create/Update 严格遵循"写三表 + syncConfigJSON"模式，确保 `agents` / `agent_runtime_settings` / `agent_prompt_files` / `config_json` 四者一致性
- Legacy 迁移路径（`migrateLegacySettings` / `migrateLegacyFiles`）保证旧数据平滑升级
- `AgentKind` 不可变约束（Update 时检查 Kind 变更拒绝）是正确的设计

**风险点：**
- **syncConfigJSON 存在冗余写入**：Create 流程中先 `CreateAgent` → `UpsertAgentRuntimeSettings` → `ReplaceAgentPromptFiles` → `syncConfigJSON`，而 `syncConfigJSON` 又会 `GetAgentByID` + `UpdateAgent`，导致一次 Create 操作至少 5 次数据库写入
- **BatchUpdateAgents 事务内逐行操作**：事务内循环 `GetAgentByID` + `UpdateAgent`，批量 100 个 Agent 时事务持有时间长
- **Agent 的 ConfigJSON 双向同步**：Settings/Files 变更后写回 ConfigJSON，ConfigJSON 变更后解析出 Settings/Files。这种双向映射容易产生不一致

### 3. Team 编排

**架构亮点：**
- `validateTeamDefinition` 对编排模式与角色兼容性做了严格校验
- Update 时检查 `HasActiveRun`，防止运行中修改编排定义
- `ExportStructure` 按 mode 生成不同的拓扑结构，可视化友好

**风险点：**
- **TeamUsecase 直接依赖 AgentRepository**：`validateTeamMembersExist` 跨模块查询 Agent，违反了 biz 层模块间应通过接口解耦的原则。应使用已有的 `AgentExistenceCheckerFunc`
- **Duplicate 生成随机后缀**：`newAgentCatalogID()` 生成 24 字符 hex 后截取前 6 位，碰撞概率不低。应使用完整 ID 或 UUID
- **HasActiveRun 查询最近 50 条 Run**：如果活跃 Run 不在最近 50 条内，会误判为无活跃 Run。应改为按状态索引查询

### 4. Graph 工作流

**架构亮点：**
- `GraphBuildConfig` 作为 biz 层纯数据结构，与 trpc-agent-go 运行时解耦
- 内存缓存 `defs` map + 持久化 repo 双层设计
- 版本历史 + Rollback 机制完善

**风险点：**
- **内存 executions map 无上限**：大量并发 Graph 执行时 map 可能膨胀。建议加 cap 或改用 LRU
- **GC 协程标记 expired 后直接 delete**：将运行中超时的执行标记为 expired 但不持久化状态到 repo，进程重启后状态丢失。应先 `UpdateRun` 持久化再清理内存

### 5. Channel 外部接入

**架构亮点：**
- Channel 抽象了多种 IM 平台，通过 Catalog + Schema 实现插件化
- 凭据加密存储 + 脱敏返回
- `RunHealthChecks` 并发检查所有启用的 Channel 健康状态

**风险点：**
- **ChannelService.TestChannel 中平台特定逻辑硬编码**：飞书/Slack/Telegram 的测试逻辑直接写在 service 层，违反了 Channel 插件化设计
- **RunHealthChecks 并发无限制**：每个 Channel 一个 goroutine，无 worker pool 限制
- **Update 时 `firstNonEmpty` 合并策略**：`Enabled: false` 这样的零值无法通过 patch 关闭

### 6. Memory 系统

**架构亮点：**
- `MemoryUsecase` 简洁地协调 embedding + vector repo
- 支持 agent + user 双维度分区
- 优雅降级设计

**风险点：**
- L0-L4 五层记忆的编排逻辑分散在 agent 层，biz 层的 `MemoryUsecase` 只管向量存储/检索
- EmbeddingService 单模型绑定，不同层可能需要不同维度/模型的向量

### 7. Session Run 生命周期

**架构亮点：**
- Interactive → Escalating → Durable → Completed/Failed 五阶段模型设计合理
- `TryClaimDurableResume` 原子化恢复锁
- 防止死锁的 stale claim 机制

**风险点：**
- **SessionRunUsecase 大量 nil 检查静默返回**：`if u == nil || u.repo == nil { return ... }` 出现 7 次，静默吞错比 panic 更危险
- **StartInteractive 在参数为空时静默返回空**：调用方无法区分"成功但无记录"和"参数错误"

### 8. Event Bus

**架构亮点：**
- `criticalTypes()` 标记关键事件类型，保证不丢失
- `BlockUpTo` + `DropOldest` 组合策略
- 灵活的订阅者过滤

**风险点：**
- Bus 是纯内存实现，进程重启后所有 in-flight 事件丢失
- subscriber channel buffer 上限 512，高频场景可能不够

---

## 三、跨模块架构问题

### 1. 依赖方向违规风险
biz 层的 `TeamUsecase` 直接依赖 `AgentRepository`，违反了"biz 模块间通过接口解耦"的原则。

### 2. ConfigJSON 双向同步反模式
Agent 的 `ConfigJSON` 与 `AgentRuntimeSettings` + `AgentPromptFiles` 存在双向映射，增加不一致风险。

### 3. Service 层 Proto 映射代码膨胀
手工映射极易遗漏字段，建议使用代码生成。

### 4. 错误处理不一致
- biz 层部分方法返回 `kerrors.BadRequest`，部分返回 `errors.New`
- `SessionRunUsecase` 对 nil receiver 静默返回零值
- `GraphUsecase.gc()` 标记 expired 不持久化

### 5. 并发安全
- `GraphUsecase` 的 `executions` map 所有 Graph 执行共享一把锁
- Channel 健康检查无并发限制

---

## 四、评分

| 维度 | 评分 | 说明 |
|------|------|------|
| 分层架构 | **A** | service/biz/data 三层边界清晰，Wire DI 编译期校验 |
| 接口设计 | **A-** | TurnGateway/GraphExecutor 等窄接口设计优秀，但部分接口存在耦合 |
| 业务完整性 | **A** | Chat/Agent/Team/Graph/Channel/Memory/SessionRun 全链路闭环 |
| 错误处理 | **B+** | 关键路径有覆盖，但 nil receiver 静默返回和错误码不一致是隐患 |
| 并发安全 | **B+** | 核心路径有锁保护，但 Graph executions 全局锁和 Channel 健康检查无并发限制 |
| 可测试性 | **B** | biz 层接口抽象好，但 agent 层记忆编排逻辑与运行时耦合较深 |
| 可扩展性 | **B+** | Channel 插件化设计好，但平台特定逻辑仍有硬编码 |

---

## 五、修复清单

| # | 优先级 | 问题 | 修复方案 | 状态 |
|---|--------|------|----------|------|
| 1 | P0 | Graph GC expired 状态不持久化 | GC 标记 expired 前先 UpdateRun 持久化 | ✅ |
| 2 | P0 | ChatUsecase.awaitChans 类型安全 | 将 sync.Map value 改为具体类型 `chan awaitReplyCh` | ✅ |
| 3 | P0 | TeamUsecase 直接依赖 AgentRepository | 改用 `AgentExistenceCheckerFunc` 接口 | ✅ |
| 4 | P1 | ChatService 双重 IngressPolicy 检查 | 移除 service 层冗余检查，统一在 orchestrator 内处理 | ✅ |
| 5 | P1 | SessionRunUsecase nil receiver 静默返回 | 关键方法改为返回 error | ✅ |
| 6 | P1 | ChannelService 平台特定测试逻辑硬编码 | 抽象 ChannelTester 接口 | ⏳ 延后（改动面大） |
| 7 | P1 | GraphUsecase executions map 无上限 | 添加 maxExecutions cap | ✅ |
| 8 | P1 | TeamUsecase.HasActiveRun 查询最近50条误判 | 添加 HasActiveTeamRunByStatus repo 方法 | ✅ |
| 9 | P2 | Channel RunHealthChecks 并发无限制 | 添加 worker pool 限制 | ✅ |
| 10 | P2 | TeamUsecase.Duplicate 后缀碰撞风险 | 使用完整 UUID 后缀 | ✅ |
