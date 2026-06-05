## Why

全栈架构审查发现 5 类系统性问题共 18 个具体缺陷：前后端事件契约割裂（6 种后端事件前端缺失）、Service 层与 Feature 域不对齐（5 个后端 Service 无前端客户端）、Biz 层 God Object 与聚合接口膨胀（SessionUsecase 17+ 接口依赖）、数据层隐患（表名不一致/PG 连接失败阻止启动/非幂等迁移）、红线违规与废弃模式残留。这些问题已在生产代码中造成静默功能失效（如 `token_usage` 事件被丢弃）和潜在运行时错误（`memory_action_log` 表名不一致），需立即修复以防止问题扩散。

## What Changes

### P0: 数据层紧急修复
- 修复 `memory_action_log`（DDL）vs `memory_action_logs`（代码）表名不一致，消除潜在运行时 SQL 错误
- PostgreSQL 连接失败时降级为 SQLite-only 模式而非阻止启动
- `ddlSessionRevisionDataMigration` 加幂等保护（`WHERE session_revision IS NULL`）

### P1: 前后端事件契约对齐
- 补齐 6 个前端缺失的 EnvelopeType：`token_usage`、`butler.orchestration.started/completed/failed`、`skill.health_changed`、`skill.evolution_proposed`
- 为 4 个"有类型无处理器"的事件注册 onType handler：`mcp.session.reconnect`、`mcp.health.alert`、`alert.notify`、`user_feedback`
- 删除前端 `EnvelopeUsage.prompt_breakdown` 死代码（后端无此字段）
- 补齐前端 `EnvelopeTokenUsage` 类型定义（后端有 50+ 字段的结构体）

### P1: 红线违规修复
- 将 `data.NewPackRepoAdapter` 和 `wire.Bind` 从 `internal/service/service.go` 移至 `cmd/admin/wire.go`（红线 #13）
- `TurnPipeline` 和 `wsTurnExecutorAdapter` 改为构造注入 `loggateway.Logger`（废弃 `Global()`）
- 确认/清理 6 个无实现的 Port 接口（`agent_ports.go`）
- 删除 4 个 Deprecated `CtxFlowLog*` 函数定义

### P2: 前端 Service 客户端补齐
- 补齐 `AgentCategoryService`、`SkillEvolutionService`、`WebhookService`、`PackService`、`PlanService` 的前端 proto 客户端
- `createSpiritService` 改为 proto 生成的客户端

### P2: Biz 层 God Object 拆分
- `SessionUsecase` 拆分为 SessionCRUDUsecase、SessionMessageUsecase、SessionTimelineUsecase、SessionCompressionUsecase、SessionMetricsUsecase
- 消费者依赖细粒度子接口，逐步废弃 `SessionRepo`/`SessionAdminStore` 聚合接口

### P3: 数据层清理
- 19 个 Repo 补齐编译期接口检查（`var _ biz.Xxx = (*xxx)(nil)`）
- 清理 3 张 DDL 孤儿表（`agent_identity`、`agent_strategy_profile`、`agent_skill_stats`）
- 统一两套 pgvector 实现为 `vector/pgvector.go`
- Memory shim 返回类型化 DTO 替代 `[][]byte`

## Capabilities

### New Capabilities
- `envelope-contract-test`: 前后端 EnvelopeType 契约自动比对 CI 检查，防止新增事件类型时前后端不同步
- `service-coverage-check`: 后端 proto Service 与前端 Service 工厂覆盖率 CI 检查

### Modified Capabilities
- `seed-version-gating`: 迁移幂等性增强（`ddlSessionRevisionDataMigration` 加 WHERE 条件）

## Impact

### 后端
- `internal/data/`：修复表名、PG 降级、编译期检查、Memory DTO 类型化
- `internal/service/`：移除 data 导入、构造注入 Logger
- `internal/biz/session/`：Usecase 拆分、接口拆分
- `internal/biz/`：清理无实现 Port 接口、删除废弃函数
- `internal/event/`：删除 CtxFlowLog* 函数
- `cmd/admin/wire.go`：新增 PackRepoAdapter 绑定、Logger 注入

### 前端
- `realtime/envelope.ts`：补齐 6 个 EnvelopeType + EnvelopeTokenUsage 类型
- `services/index.ts`：补齐 5 个 proto 客户端 + 替换 createSpiritService
- `features/chat/`：注册 4 个事件处理器
- `features/skill-evolution/`、`features/pack/`、`features/plan/`：新增 feature 域

### CI
- 新增 EnvelopeType 契约测试
- 新增 Service 覆盖率检查

### 非目标
- 不重构 SessionUsecase 的所有调用方（渐进式迁移）
- 不修改 trpc-agent-go 框架代码
- 不修改 Proto 定义（仅补齐前端客户端）
- 不涉及 M60 Spirit Parallel Orchestrator 新功能开发
