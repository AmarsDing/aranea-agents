# Aranea-Agents 全量系统深度审计报告


> 日期：2026-07-15  
> 范围：需求、设计、开发计划、后端、运行时框架适配、数据、前端、测试与 CI  
> 方法：六条并行静态审计线 + 本地可执行门禁验证  
> 说明：本轮只审计并给出整改路线，不修改业务代码。

## 1. 执行结论

Aranea-Agents 已经具备企业级多 Agent 平台的主要骨架：

- Kratos 负责传输与依赖注入，`pkg/trpc-agent-go` 负责 Agent 运行时，边界方向正确。
- Agent、Team、Graph、Spirit、Tool、MCP、Memory、Knowledge、Channel、Evaluation、Monitor 等能力覆盖广。
- Biz 不依赖 tRPC Agent、Service 不直接依赖 Data 两条核心分层红线已由测试验证。
- Run/Session/Team/Graph 等显式状态机、Activity v2 投影、Graph Checkpoint/TimeTravel、工具治理和五层记忆均有实质实现。

但当前不能认定为“生产就绪”。去重后仍有至少 7 项发布阻断风险、16 项关键风险和 20 余项主要风险。最高风险集中在六个根因：

1. **租户与对象授权不是不可绕过的边界**：workspace 由客户端提供，WS、Knowledge、Artifact、Plugin、Tool workspace 等路径存在跨用户或跨 workspace 风险。
2. **编排存在假成功和多事实源**：Spirit 工具可在真正执行前发布完成；TaskPlan、PlanBoard、OrchestrationHandle 使用不同身份；PlanBoard 状态副本导致终态失败。
3. **关键事件与队列不具备崩溃恢复能力**：v2 EventBus、WS normal queue、PendingQueue、Memory 队列及多个后台任务仍以内存为主。
4. **取消、终态与持久化顺序不一致**：RunRegistry 取消后过早释放租约，失败 defer 可覆盖 cancelled，状态持久化错误被吞。
5. **高风险执行面 fail-open**：隔离执行器不可用时可回退 Local；工具目录边界、流式工具预算、MCP 静态凭据均需加固。
6. **质量门禁没有反映文档的绿色状态**：前端 typecheck、单测、分层检查、严格 ESLint、Prettier、Stylelint 均失败；Go race 检出 Sequencer 测试竞态。

综合评级：

| 维度 | 评级 | 判断 |
|---|---|---|
| 架构方向 | B+ | 分层、端口适配、Graph 编排和运行时复用方向合理 |
| 业务正确性 | C | 编排假成功、状态竞态、部分更新和异步一致性存在实质缺陷 |
| 安全与多租户 | D | 身份认证存在，但 workspace、对象级授权和执行沙箱未形成硬边界 |
| 可靠性 | C- | 关键事件、队列、后台任务和多实例调度仍有内存态/单实例假设 |
| 前端与契约 | C- | v1/v2 双轨、类型漂移、重连回补和分层违规同时存在 |
| 测试与门禁 | C- | 后端基础测试较好，但当前仓库不能通过完整预期门禁 |
| 文档可信度 | D+ | 三件套覆盖广，但索引、事件架构、Schema、RPC 和状态标记漂移明显 |
| 生产就绪度 | 45/100 | 可继续研发和内部验证，不建议以当前状态直接多租户生产发布 |

## 2. 审计覆盖

### 2.1 文档与契约

- `docs/development` 约 243 个 Markdown 文件，约 52 个编号模块具有完整三件套。
- 37 个 Kratos Proto 域。
- Ent Schema 代码约 90 个，数据库设计仍记录 82 个。
- 系统入口、模块交叉参考、Chat/Team RPC 表、数据库 Schema 表和 70 号综合升级文档均发现漂移。

### 2.2 代码与产品面

已覆盖：

- 核心链：Server → Service → Biz → Data，Chat → Runner → Event → WS → Web。
- 编排：Agent、Team、Graph、Spirit、PlanExecutor、Checkpoint/HITL/Resume。
- 能力：Tools、MCP、Skill、Plugin、Memory、Knowledge、Artifact、CodeExecutor、Model Catalog。
- 平台：Channel、Cron、A2A、Evaluation、Evolution、Learning Loop、Monitor、Usage、Auth、Workspace、数据库。
- 前端：Proto client、feature、store、composable、page、WS transport、测试与 CI。

## 3. 已验证的关键事实

### 3.1 正向事实

- `go run ./cmd/araneactl/lint --root .`：通过，0 violations。
- `go vet ./...`：通过。
- `go run ./cmd/araneactl/fieldguide-lint --root .`：通过，5 scopes in sync。
- 核心 Biz/Runtime/Event 包普通测试：通过。
- 架构测试确认 Biz 不依赖 tRPC Agent、Service 不直接依赖 Data。
- 前端生产 bundle 可以构建，但该构建不会执行完整 TypeScript 类型检查。

### 3.2 当前失败门禁

| 验证项 | 结果 | 关键证据 |
|---|---|---|
| `vue-tsc --noEmit` | 失败 | 20 余处类型错误，包括缺失导出、状态枚举漂移、ChatPage 属性缺失 |
| Vitest | 失败 | 94/95 文件通过，2/553 用例失败，缺少 active Pinia |
| 前端分层检查 | 失败 | 9 个 chat/v2 component 直接导入 Pinia store |
| 严格 ESLint | 失败 | 83 warnings，CI 使用 `--max-warnings=0` |
| Prettier | 失败 | 57 个文件格式不一致 |
| Stylelint | 失败 | 107 errors |
| Go `-race` | 失败 | `internal/agent/v2` Sequencer 测试检测到 data race；其余部分又受系统盘空间不足影响 |

完整命令和限制见 `2026-07-15-audit-aranea-verification.md`。

## 4. 最高优先级风险

### 4.1 安全与隔离

- Workspace 中间件信任 `X-Workspace-ID`/query 值，未与 JWT membership 绑定：`internal/server/middleware/workspace.go:13-45`。
- WS 仅认证身份，未验证 session 所有权；全局 `session_id=*` 缺少明确管理员守卫：`internal/server/ws.go:175-210`、`internal/server/ws_message_handler.go:109-146`。
- Knowledge 的 workspace 创建、列表和点查链路不完整：`internal/service/knowledge.go:103-133`、`internal/data/knowledge.go:131-168`。
- Artifact Repo 忽略 context，按客户端 ID/session 访问本地对象：`internal/service/artifact.go:36-109`、`internal/data/artifactfs/repo.go:113-176`。
- CodeExecutor 在隔离后端不可用时可回退本地执行：`internal/agent/codeexecutor/factory.go:149-185`。

### 4.2 核心运行时

- Session lock sweep 可删除仍被持有的锁并创建第二把锁：`internal/biz/session_lock.go:61-106`。
- RunRegistry Cancel 立即删除 active entry，旧 run 未退出时新 run 可进入：`internal/runtime/run_registry.go:171-219`。
- 用户取消可能被后续 failed/interrupted 事件覆盖：`internal/service/chat_orchestrator_turn.go:436-451`。
- Run 状态持久化错误不向调用方传播：`internal/service/chat_orch_run_status.go:97-145`。
- `IdempotencyKey` 没有数据库字段、唯一约束和冲突返回语义：`internal/service/turn_service_persistent.go:29-68`、`internal/data/ent/schema/session_turn.go:21-63`。

### 4.3 编排与长任务

- `plan_and_execute` 在异步 DAG 尚未执行前发布完成：`internal/tools/spirit_tools.go:183-197,285-340`。
- PlanBoard `planning → executing` 只修改副本，后续终态仍从 planning 转换：`internal/service/plan_executor.go:182-225,405-449`。
- 无根或循环 DAG 可零执行即成功：`internal/service/plan_executor.go:351-389`。
- TaskPlan、PlanBoard、OrchestrationHandle 三套 ID 破坏进度、取消与恢复关联。
- RuntimeReplanner 只记录决策，不改变执行：`internal/graph/adapter/runtime_adapter.go:355-407`。
- Synthesis prompt/hybrid 返回“让模型综合的提示词”，没有调用模型得到综合结果：`internal/biz/spirit_synthesis.go:138-175,226-236`。

### 4.4 事件与实时前端

- v2 EventBus subscriber 和 WS normal queue 都可丢弃关键终态：`internal/event/bus_v2.go:68-107`、`internal/server/ws_priority.go:120-126`。
- `last_event_id` 被客户端发送，但服务端没有执行重放。
- 重连不主动重新水合 v2 snapshot，历史子请求错误又被吞为空数组。
- v1 Activity 与 v2 Entity 通过布尔值整体切换，部分 v2 数据可遮蔽更完整的 legacy 数据。
- Step delta 早于 StepCreated 时直接丢弃。

### 4.5 数据与运维

- 大部分租户实体没有统一 `workspace_id`；Repo 无法普遍施加租户谓词。
- Cron 排他仅为进程内锁，多副本会重复执行。
- DDL migration 无 Postgres advisory lock，执行与版本登记不在统一事务。
- Usage rollup 无 outbox/consumer 去重；Postgres purge 使用 SQLite 日期语法。
- Monitor alert 以 `*sql.DB` 执行 `"BEGIN"`/`"COMMIT"`，不是 `*sql.Tx`。

## 5. 架构合理性判断

### 建议保留

1. 保留 Kratos + tRPC Agent 的“双层框架”职责分工。
2. 保留 Server/Service/Biz/Data 主分层及 Biz port 模式。
3. Team 继续以 Graph runtime 作为主要执行路径，避免恢复 Native 双引擎。
4. 保留显式状态机、Activity v2 实体化和 Graph Checkpoint/TimeTravel。
5. 保留 `loggateway`、`safego`、FieldGuide、架构 lint 等治理设施。

### 必须调整

1. 安全边界必须下沉为认证 principal + tenant scope + Repo predicate/RLS，而不是依赖 Service 约定。
2. 所有长任务采用 durable command/inbox/outbox/lease，不再用裸 goroutine + 内存队列表达业务完成。
3. 编排只保留一个 execution identity 和一个权威状态源。
4. Critical 事件先持久化再发布；重连以 sequence + snapshot/replay 收敛。
5. CodeExecutor、FunctionResolver、Graph 编译和权限检查默认 fail-closed。
6. 架构 lint 中的接口与复杂度超限应逐步变为失败门禁，而不是仅 `t.Logf`。

## 6. 行业定位

当前系统更接近“功能覆盖广的单实例 Agent 控制面”，尚未达到“多租户、可恢复、可水平扩展的 Agent 运行平台”。下一阶段不应优先增加更多编排名词，而应完成：

1. Tenant-aware control plane。
2. Durable orchestration execution。
3. Reliable event projection and replay。
4. Policy-governed tools and sandbox。
5. Versioned memory/RAG provenance。
6. Evaluation and cost ledger as release gates。

模块级行业方向和评分见 `2026-07-15-audit-aranea-module-scorecards.md`。

## 7. 建议决策

- **当前发布决策**：不建议多租户生产发布；可以继续内部单租户开发验证。
- **第一修复批**：workspace/WS/Artifact/Knowledge 授权、CodeExecutor fail-closed、PlanExecutor 假成功、关键事件终态可靠性。
- **第二修复批**：取消与终态一致性、幂等、durable queue/outbox、Cron/迁移多实例安全。
- **第三修复批**：前端 v2 收敛、契约生成、质量门禁、文档真理库重建。

详细问题、收益和验收方式见 `2026-07-15-audit-aranea-issue-register.md`；实施顺序见 `2026-07-15-audit-aranea-remediation-roadmap.md`。
