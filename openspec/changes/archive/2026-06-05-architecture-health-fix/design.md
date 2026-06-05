## Context

全栈架构审查发现 5 类系统性问题，涉及后端 4 层（data/biz/service/server）和前端 4 层（services/stores/composables/pages）。当前状态：

- **事件契约**：后端 51 种 EnvelopeType，前端 49 种，6 种后端事件前端完全缺失
- **Service 覆盖**：5 个后端 Service 无前端 proto 客户端，createSpiritService 为手工 API
- **Biz 层**：SessionUsecase 持有 17+ 接口字段（God Object），SessionRepo 聚合 30+ 方法
- **数据层**：memory_action_log 表名不一致、PG 连接失败阻止启动、19 个 Repo 缺少编译期检查
- **红线**：service.go 导入 data 包、2 处使用 loggateway.Global()、6 个 Port 接口无实现

## Goals / Non-Goals

**Goals:**
- 修复所有 P0 级运行时风险（表名不一致、PG 启动失败、非幂等迁移）
- 补齐前后端事件契约，消除静默功能失效
- 修复红线违规，确保架构合规
- 建立契约测试 CI 防止问题复发
- 渐进式拆分 SessionUsecase God Object

**Non-Goals:**
- 不重构 SessionUsecase 的所有调用方（渐进式迁移，旧接口标记 Deprecated）
- 不修改 trpc-agent-go 框架代码
- 不修改 Proto 定义（仅补齐前端客户端）
- 不涉及 M60 Spirit Parallel Orchestrator 新功能开发
- 不修改 Ent 生成的代码
- 不做 Memory shim `[][]byte` → DTO 的全面重构（仅 P3 探索）

## Decisions

### D1: memory_action_log 表名修复策略

**决策**：以 DDL 为准，统一使用 `memory_action_log`（单数），修改代码中的 INSERT 语句。

**理由**：DDL 是 schema 真相源，且所有其他 memory 表均使用单数命名（`memory_facts` 除外，但那是历史遗留）。修改代码比修改 DDL 风险更低（无需数据迁移）。

**替代方案**：修改 DDL 为复数 → 需要数据迁移 + 更新所有引用，风险更高。

### D2: PostgreSQL 降级策略

**决策**：`initPostgres` 失败时 log warning + 降级为 SQLite-only，不阻止 `NewData` 返回。

**实现**：将 `initPostgres` 改为 best-effort，失败时设置 `d.postgres = nil`，VectorStore 选择逻辑已有 fallback。

**理由**：PostgreSQL 在架构中是可选的（仅用于 pgvector 向量存储），配置了但连不上不应阻止系统启动。

**替代方案**：添加配置开关 `pg_required: true/false` → 增加配置复杂度，当前无场景需要强制 PG。

### D3: 前后端事件契约测试

**决策**：在 CI 中添加 Go 测试，从 `contract/envelope.go` 提取所有 `EnvelopeType` 常量值，与 `envelope.ts` 中的常量值比对，不一致则失败。

**实现**：
1. 新增 `internal/event/contract/envelope_contract_test.go`
2. 测试读取后端所有 `EnvelopeType` 常量，生成期望值列表
3. 前端契约测试通过 `ts-node` 脚本读取 `envelope.ts` 导出值
4. CI 步骤：`go test ./internal/event/contract/... -run TestEnvelopeContract` + `npx ts-node scripts/check-envelope-contract.ts`

**替代方案**：Proto-first 定义 EnvelopeType → 需要大量重构，当前不现实。

### D4: SessionUsecase 拆分策略

**决策**：渐进式拆分，保留 `SessionUsecase` 作为 Facade，内部委托给子 Usecase。

**Phase 1**（本次变更）：
- 提取 `SessionMetricsUsecase`（指标刷新 + delta 聚合）——最独立的职责
- 提取 `SessionCompressionUsecase`（上下文压缩）——与压缩模块交互
- `SessionUsecase` 持有子 Usecase 引用，旧方法委托调用

**Phase 2**（后续变更）：
- 提取 `SessionMessageUsecase`、`SessionTimelineUsecase`
- 消费者逐步迁移到子 Usecase
- `SessionUsecase` 标记 Deprecated 方法

**理由**：一次性拆分所有调用方风险太高，Facade 模式允许渐进迁移。

**替代方案**：一次性拆分 → 修改 30+ 调用方，变更风险不可控。

### D5: Service 层红线 #13 修复

**决策**：将 `data.NewPackRepoAdapter` 和 `wire.Bind(new(packExporterImporterValidator), new(*data.PackRepoAdapter))` 移至 `cmd/admin/wire.go`。

**理由**：Wire 绑定属于 DI 组装层，`cmd/admin/wire.go` 是唯一允许同时 import service 和 data 的位置。

### D6: 无实现 Port 接口处理

**决策**：为 6 个无实现的 Port 接口添加 `// Placeholder: reserved for future refactoring, not yet implemented` 注释。不删除——它们定义了 biz 与 agent/tools/provider 层的未来解耦方向。

**理由**：这些接口（`AgentRuntimeBuilder`、`ToolsetAssembler` 等）是架构占位符，删除会丢失设计意图。

### D7: 前端 Spirit Service proto 化

**决策**：使用现有 `spirit/v1` proto 生成客户端替换手工 `createSpiritService`。

**理由**：proto 生成的客户端有类型安全、自动同步 proto 变更、支持所有 RPC 方法。

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|---------|
| [P0] memory_action_log 表名修复可能影响已有数据 | 确认实际运行中的表名（SQLite CREATE TABLE IF NOT EXISTS），以实际表名为准 |
| [P1] PG 降级可能导致向量搜索功能静默失效 | 降级时 log warning + readiness probe 标记 vector search 不可用 |
| [P1] 补齐前端 EnvelopeType 可能引入新 bug | 每个新处理器先做 toast 通知，验证事件到达后再做业务逻辑 |
| [P2] SessionUsecase 拆分可能破坏现有调用方 | Facade 委托模式保证旧调用方无需修改 |
| [P2] 前端 proto 客户端补齐需要 make api | 仅新增前端代码，后端 proto 已存在 |
| [P3] 编译期检查补齐可能暴露隐藏的接口不匹配 | 逐个添加，发现不匹配时先修复接口再添加检查 |

## Migration Plan

1. **P0 修复**：直接修改，无需迁移——表名修复、PG 降级、迁移幂等化
2. **P1 修复**：增量添加——前端类型/处理器、红线修复、废弃代码删除
3. **P2 修复**：渐进式——SessionUsecase Facade 拆分、前端 Service 补齐
4. **P3 修复**：低优先级——编译期检查、DDL 清理、pgvector 统一

**回滚策略**：每个 P 级别独立提交，可独立回滚。P0 修复需验证 `make test` 通过。

## Open Questions

1. `memory_action_log` 实际运行中的表名是单数还是复数？需确认生产环境 SQLite 文件
2. `AgentCategoryService` 蓝图中有路由但代码不存在——是蓝图过时还是功能缺失？
3. SessionUsecase 拆分 Phase 2 的时间表？
