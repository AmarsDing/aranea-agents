## Why

当前数据访问层存在 6 大结构性问题，严重制约项目的扩展性和可维护性：

1. **Session 超级表写入放大**：sessions 表（52 字段）承载了冷元数据、热聚合、运行时状态、版本控制 4 类不同变更频率的数据，一次 Chat Turn 同步写入 7~14 次 UPDATE
2. **sessionmemory.Store 架构旁路**：96 方法的 God Object，直接满足 22 个 biz 接口（隐式耦合），绕过 Ent ORM、绕过统一事务管理、返回 `[][]byte` JSON 而非领域类型
3. **双轨 Schema 管理**：Ent Schema（57 张表）vs memory_chain.sql 野生表（34 张纯野生 + 23 张重叠），新增字段开发周期 3~5x
4. **读写分离合规率仅 3%**：59 个 Repo 中仅 2 个正确实现 readClient + txClient，~10 个 Repo 写操作绕过 txClient（35+ 处调用），~13 个 Repo 读操作走写连接（56 处调用）
5. **向量存储双写混乱**：SQLite embedding_blob + Postgres pgvector 并存，一致性无保障
6. **错误处理/可观测性缺失**：3 种错误翻译风格不统一，data 层零 DB 指标、零链路追踪

Phase 1-3 的治理已将单 Turn 写入从 8~17 次降至 ~3 次，但上述结构性问题无法通过渐进式修补解决，需要系统性重构。

## What Changes

### 架构重构

- **BREAKING**：sessions 表拆分为 `sessions`（冷元数据）+ `session_metrics`（热聚合）+ `session_runtime`（运行时状态），同步写入从 ~3 次降至 2~3 次
- **BREAKING**：sessionmemory.Store 拆分为 6 个独立 Repo（L0SnapshotRepo / L1WorkingMemoryRepo / L2EpisodeRepo / L3FactRepo / L4EntityRepo / CascadeRepo），每个 ≤16 方法（当前 Store 96 方法按层级分配）
- **BREAKING**：消除 Store 直接满足 biz 接口的隐式耦合，改为 data 层显式 adapter
- 引入 `ReadWriteClient` 自动路由抽象，统一读写分离和事务感知，替代每个 Repo 手动实现 readClient/txClient
- 引入 `VectorStore` 策略模式接口，统一向量存储（SQLite json_set / Postgres pgvector 可切换）
- 引入 `entErrToBizErr()` 统一错误翻译函数
- 引入 data 层 DB 指标（query latency histogram / slow query counter / connection pool gauge）

### Schema 管理

- 野生表渐进式纳入 Ent Schema，目标纯野生表从 34 张降至 0
- memory_chain.sql 缩减为仅含 Memory 专属表（删除与 Ent 重叠的 23 张表定义）
- 增强自研迁移系统（支持 SQL 文件版本化、自动生成 patch），替代散落的 `*_patch.go`

### 接口合规

- monitor.Repo（19 方法）→ 4 子接口、a2a.Repo（14 方法）→ 3 子接口
- wire 适配器从 `cmd/admin/wire_memory.go` 归位到 `internal/data/`
- Store 方法参数去 biz 依赖（6 个 biz DTO → data 层自有 DTO）
- 移除 Store.Client() 后门

### 实时更新

- 新增 `EnvelopeTypeMetricsUpdated` 事件，session_metrics 异步写入后通知前端
- Session 列表查询引入 metrics 读缓存，避免拆表后每次穿透 DB

## Capabilities

### New Capabilities

- `session-table-split`：Session 表冷热拆分（sessions + session_metrics + session_runtime），含迁移策略、API 兼容层、前端事件通知
- `memory-store-decomposition`：sessionmemory.Store 拆分为 6 个独立 Repo，消除架构旁路，所有数据访问走 biz 接口
- `readwrite-client-abstract`：ReadWriteClient 自动路由抽象，统一读写分离和事务感知
- `vector-store-strategy`：VectorStore 策略模式，统一向量存储引擎（SQLite / Postgres 可切换）
- `data-layer-observability`：data 层可观测性（统一错误翻译、DB 指标、慢查询日志）
- `wild-table-ent-migration`：野生表渐进式纳入 Ent Schema + 迁移系统增强

### Modified Capabilities

- `session-repo-interfaces`：Session 相关 biz 接口新增 session_metrics / session_runtime 读写端口
- `memory-admin-interfaces`：SessionAdminStore Deprecated 组合接口迁移到独立子接口

## Impact

### 受影响层

| 层 | 影响 |
|----|------|
| **internal/data/** | 重构核心：表拆分、Store 拆分、读写分离统一、错误翻译、指标 |
| **internal/biz/** | 新增端口接口、Usecase 适配新接口、Delta 安全阀 |
| **internal/service/** | toProtoSession 适配拆表后的聚合查询、WebSocket 事件处理 |
| **api/v1/** | Proto 无变更（API 契约不变），但 service 层映射逻辑变更 |
| **cmd/admin/** | wire_memory.go 适配器归位、Wire 绑定更新 |
| **web/** | 前端处理 MetricsUpdated 事件、reconcilePatchFromServer 修复 |

### 受影响模块（交叉参考）

- Session 子系统（session_repo / session_message_repo / session_state_repo / session_run_repo）
- Memory 子系统（sessionmemory 全部 24 文件）
- Usage 子系统（usage_write / usage_quota）
- Monitor 子系统（monitor / alert）
- Wire DI 图（200+ 节点中的 ~60 个 data 层绑定）

### 风险

- **API 契约风险**：proto `v1.Session` 包含 16 个 metrics 字段，拆表后需聚合两数据源
- **迁移风险**：sessions 表拆分需双写过渡期 + feature flag
- **性能风险**：session 列表查询拆表后需 JOIN 或二次查询，需缓存层缓冲
- **测试风险**：当前无共享 test fixture，迁移脚本需在所有测试中执行

## Non-goals

- 不更换 SQLite 为 Postgres 作为主数据库（保持 SQLite 单机部署优势）
- 不重写前端（仅适配新事件和 API 响应格式）
- 不引入分布式缓存（Redis 等），仅进程内缓存
- 不修改 Proto 定义（API 契约保持向后兼容）
- 不在本次变更中实施 Session 冷热分离的完整方案（仅做表拆分基础）
