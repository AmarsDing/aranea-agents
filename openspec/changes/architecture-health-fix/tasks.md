## 1. P0: 数据层紧急修复

- [ ] 1.1 确认 `memory_action_log` 实际表名：检查 DDL `memory_chain.sql` 和代码 `memory_shim_action_log.go` 中的 INSERT 语句，确认单复数不一致。DoD: 明确哪个是正确的表名
- [ ] 1.2 修复 `memory_action_log` 表名不一致：以 DDL 为准统一为 `memory_action_log`（单数），修改 `memory_shim_action_log.go` 中所有 INSERT/SELECT 语句的表名。DoD: `grep -r "memory_action_logs" internal/data/` 返回 0 结果
- [ ] 1.3 修复 PostgreSQL 降级：修改 `internal/data/data.go` 的 `initPostgres`，失败时 log warning + 设置 `d.postgres = nil` 而非返回 error。DoD: PostgreSQL 配置错误时系统仍可启动（使用 SQLite 向量存储）
- [ ] 1.4 修复非幂等迁移：修改 `ddlSessionRevisionDataMigration`，添加 `WHERE session_revision IS NULL` 条件。DoD: 迁移重复执行不覆盖已有 revision 值
- [ ] 1.5 验证 P0 修复：`go test ./internal/data/... -count=1` + `make build`。DoD: 测试通过 + 编译成功

## 2. P1: 前后端事件契约对齐

- [ ] 2.1 补齐前端缺失的 EnvelopeType 常量：在 `web/src/realtime/envelope.ts` 添加 `token_usage`、`butler.orchestration.started/completed/failed`、`skill.health_changed`、`skill.evolution_proposed` 共 6 个常量。DoD: 前端 EnvelopeType 常量数与后端一致
- [ ] 2.2 补齐前端 `EnvelopeTokenUsage` 类型：在 `web/src/realtime/envelope.ts` 添加 `EnvelopeTokenUsage` 类型定义（对齐后端 `contract/envelope.go` 的 `EnvelopeTokenUsage` 结构体），并在 `Envelope` 类型中添加 `token_usage` 字段。DoD: TypeScript 编译通过
- [ ] 2.3 删除前端 `EnvelopeUsage.prompt_breakdown` 死代码：移除 `PromptTokenBreakdown` 类型、`prompt_breakdown` 字段、`sessionContextPatch.ts` 中相关逻辑。DoD: `grep -r "prompt_breakdown" web/src/` 返回 0 结果
- [ ] 2.4 为 `mcp.session.reconnect` 和 `mcp.health.alert` 注册默认 handler：在 `features/chat/` 或 `features/mcp/` 的 dispatcher 中添加 `onType` 处理器（日志记录 + toast 通知）。DoD: 事件到达时控制台有日志输出
- [ ] 2.5 为 `alert.notify` 注册 handler：在 `features/monitor/` 的 dispatcher 中添加 `onType` 处理器（显示 toast 通知）。DoD: 告警事件到达时前端显示通知
- [ ] 2.6 为 `user_feedback` 注册 handler：在 `features/chat/` 的 dispatcher 中添加 `onType` 处理器（日志记录）。DoD: 事件到达时控制台有日志输出
- [ ] 2.7 为 `butler.orchestration.*` 3 个事件注册 handler：在 `features/chat/` 或 `features/orchestration/` 的 dispatcher 中添加处理器。DoD: Butler 编排事件到达时前端有响应
- [ ] 2.8 为 `skill.health_changed` 和 `skill.evolution_proposed` 注册 handler：在 `features/skills/` 的 dispatcher 中添加处理器。DoD: 技能事件到达时前端有响应
- [ ] 2.9 创建后端 EnvelopeType 契约测试：新增 `internal/event/contract/envelope_contract_test.go`，提取所有 EnvelopeType 常量值并验证无重复。DoD: `go test ./internal/event/contract/... -run TestEnvelopeContract` 通过
- [ ] 2.10 创建前端 EnvelopeType 契约脚本：新增 `web/scripts/check-envelope-contract.ts`，读取 `envelope.ts` 导出值并与后端期望值比对。DoD: `npx ts-node scripts/check-envelope-contract.ts` 通过
- [ ] 2.11 验证 P1 事件契约修复：`pnpm lint && pnpm build` + `go test ./internal/event/... -count=1`。DoD: 前端编译通过 + 后端测试通过

## 3. P1: 红线违规修复

- [ ] 3.1 修复 Service 层红线 #13：将 `data.NewPackRepoAdapter` 和 `wire.Bind(new(packExporterImporterValidator), new(*data.PackRepoAdapter))` 从 `internal/service/service.go` 移至 `cmd/admin/wire.go`，移除 service.go 中对 `internal/data` 的 import。DoD: `grep "internal/data" internal/service/service.go` 返回 0 结果
- [ ] 3.2 修复 TurnPipeline 的 loggateway.Global()：在 `TurnPipeline` struct 添加 `lg loggateway.Logger` 字段，构造注入，替换 `turn_pipeline.go:44` 的 `loggateway.Global()` 调用。DoD: `grep "loggateway.Global()" internal/service/turn_pipeline.go` 返回 0 结果
- [ ] 3.3 修复 wsTurnExecutorAdapter 的 loggateway.Global()：在 `cmd/admin/wire.go` 的 `wsTurnExecutorAdapter` 中注入 `loggateway.Logger`，替换 `wire.go:695` 的 `loggateway.Global()` 调用。DoD: `grep "loggateway.Global()" cmd/admin/wire.go` 返回 0 结果
- [ ] 3.4 为 6 个无实现 Port 接口添加占位注释：在 `internal/biz/agent_ports.go` 的 `AgentRuntimeBuilder`、`ToolsetAssembler`、`ModelResolverPort`、`AgentBuildRunner`、`AgentPersistTurnRecord`、`AgentProjectRuntimeEvent` 接口添加 `// Placeholder: reserved for future refactoring, not yet implemented` 注释。DoD: 6 个接口均有注释
- [ ] 3.5 删除 4 个 Deprecated CtxFlowLog* 函数：从 `internal/event/flow_context.go` 删除 `CtxFlowLogStart`、`CtxFlowLogDone`、`CtxFlowLogError`、`CtxFlowLog` 函数定义。DoD: `grep "CtxFlowLog" internal/event/flow_context.go` 返回 0 结果
- [ ] 3.6 验证 P1 红线修复：`make wire && make build && make test`。DoD: Wire 生成成功 + 编译通过 + 测试通过

## 4. P2: 前端 Service 客户端补齐

- [ ] 4.1 补齐 AgentCategoryService 前端客户端：在 `web/src/services/index.ts` 添加 `createAgentCategoryService`，创建 `web/src/features/agent-categories/api.ts`。DoD: 前端可调用 AgentCategory CRUD API
- [ ] 4.2 补齐 SkillEvolutionService 前端客户端：在 `web/src/services/index.ts` 添加 `createSkillEvolutionService`，创建 `web/src/features/skill-evolution/api.ts`。DoD: 前端可调用 SkillEvolution API
- [ ] 4.3 补齐 WebhookService 前端客户端：在 `web/src/services/index.ts` 添加 `createWebhookService`，更新 `features/webhooks/api.ts` 使用 proto 客户端。DoD: Webhook 功能使用类型安全客户端
- [ ] 4.4 补齐 PackService 前端客户端：在 `web/src/services/index.ts` 添加 `createPackService`，创建 `web/src/features/pack/api.ts`。DoD: 前端可调用 Pack 导入导出 API
- [ ] 4.5 补齐 PlanService 前端客户端：在 `web/src/services/index.ts` 添加 `createPlanService`，创建 `web/src/features/plan/api.ts`。DoD: 前端可调用 Plan API
- [ ] 4.6 替换 createSpiritService 为 proto 客户端：删除手工 `createSpiritService`，使用 `spirit/v1` proto 生成的客户端。DoD: `grep "createSpiritService" web/src/services/index.ts` 返回 0 结果，Spirit API 使用 proto 客户端
- [ ] 4.7 创建 Service 覆盖率检查脚本：新增 `web/scripts/check-service-coverage.ts`，比对后端 proto Service 与前端 `createXxxService`。DoD: 脚本可运行并输出缺失列表
- [ ] 4.8 验证 P2 Service 补齐：`pnpm lint && pnpm build`。DoD: 前端编译通过

## 5. P2: Biz 层 God Object 拆分（Phase 1）

- [ ] 5.1 提取 SessionMetricsUsecase：从 `SessionUsecase` 提取指标刷新 + delta 聚合逻辑到新 `SessionMetricsUsecase` struct。DoD: `SessionMetricsUsecase` 独立文件 `internal/biz/session/metrics.go`
- [ ] 5.2 提取 SessionCompressionUsecase：从 `SessionUsecase` 提取上下文压缩逻辑到新 `SessionCompressionUsecase` struct。DoD: `SessionCompressionUsecase` 独立文件 `internal/biz/session/compression.go`
- [ ] 5.3 SessionUsecase Facade 委托：`SessionUsecase` 持有 `SessionMetricsUsecase` 和 `SessionCompressionUsecase` 引用，旧方法委托调用。DoD: 旧调用方无需修改
- [ ] 5.4 更新 Wire 绑定：在 `cmd/admin/wire.go` 添加 `SessionMetricsUsecase` 和 `SessionCompressionUsecase` 的 ProviderSet 和绑定。DoD: `make wire && make build` 通过
- [ ] 5.5 标记聚合接口 Deprecated：在 `SessionRepo` 和 `SessionAdminStore` 添加 `// Deprecated: Use fine-grained sub-interfaces` 注释。DoD: IDE 中使用聚合接口时显示 Deprecated 警告
- [ ] 5.6 验证 P2 Biz 拆分：`go test ./internal/biz/session/... -count=1 && make wire && make build`。DoD: 测试通过 + 编译成功

## 6. P3: 数据层清理

- [ ] 6.1 补齐 19 个 Repo 编译期接口检查：为 `SkillIntelligenceRepo`、`obsRepo`、`patternRepo`、`proposalRepo`、`evalRepo`、`ecosystemRepo`、`cronRepo`、`monitorRepo`、`knowledgeRepo`、`avatarRepo`、`backgroundJobRepo`、`pluginRepo`、`pluginCostGuardUsageRepo`、`pluginRunRepo`、`usageRepo`、`sessionStateRepo`、`sessionTurnRepo`、`sessionMessageRepo`、`sessionMetricsRepo` 添加 `var _ biz.Xxx = (*xxx)(nil)` 断言。DoD: `make build` 通过（编译期检查生效）
- [ ] 6.2 清理 3 张 DDL 孤儿表：在 `memory_chain.sql` 中为 `agent_identity`、`agent_strategy_profile`、`agent_skill_stats` 添加 `-- RESERVED: Not yet implemented` 注释。DoD: DDL 中孤儿表有明确标注
- [ ] 6.3 统一 pgvector 实现：确认 `internal/data/pgvector/store.go` 无调用方后标记 Deprecated，新增注释指向 `internal/data/vector/pgvector.go`。DoD: 旧实现有 Deprecated 标注
- [ ] 6.4 验证 P3 数据层清理：`go test ./internal/data/... -count=1 && make build`。DoD: 测试通过 + 编译成功

## 7. 全量验证

- [ ] 7.1 后端全量验证：`make api && make wire && make build && make test && make lint`。DoD: 全部通过
- [ ] 7.2 前端全量验证：`cd web && pnpm lint && pnpm test && pnpm build`。DoD: 全部通过
- [ ] 7.3 更新架构蓝图数据：修正 `architecture-blueprint.md` 中 EnvelopeType 数量（51 后端/49 前端）和 Repo 数量（78 个）。DoD: 蓝图数字与代码一致
