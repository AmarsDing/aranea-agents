# Aranea-Agents 审计验证与追踪基线

> 日期：2026-07-15  
> 环境：Windows，Go/Node/pnpm 本地环境  
> 原则：环境无法证明的项目标记为未验证，不将其写成通过。

## 1. 可执行验证结果

| 命令 | 结果 | 说明 |
|---|---|---|
| `go run ./cmd/araneactl/lint --root .` | PASS | 0 violations |
| `go vet ./...` | PASS | 无输出 |
| `go run ./cmd/araneactl/fieldguide-lint --root .` | PASS | 5 scopes in sync |
| `go test ./internal/archlint/ -count=1 -v` | PASS with warnings | 9 个宽接口、40+ 个超限 struct 仅记录日志 |
| `go test ./internal/biz/... ./internal/runtime/... ./internal/event/... -count=1` | PASS | 核心普通单测通过 |
| `go test -race ./... -count=1` | FAIL | `internal/agent/v2` 检出 data race；部分包又因系统盘空间不足未完成 |
| `pnpm exec vue-tsc --noEmit` | FAIL | 20 余处类型错误 |
| `pnpm test -- --run` | FAIL | 94/95 test files，551/553 tests 通过 |
| `pnpm build` | PASS | bundle 成功，但不会代替 `vue-tsc` |
| `pnpm check:layer` | FAIL | 9 个 component 直接导入 Pinia store |
| `pnpm exec eslint ... --max-warnings=0` | FAIL | 83 warnings |
| `pnpm exec prettier --check ...` | FAIL | 57 个文件不符合格式 |
| `pnpm exec stylelint ...` | FAIL | 107 errors |

## 2. Go 验证细节

### 2.1 架构门禁的真实语义

`internal/archlint/archlint_test.go` 中只有依赖方向、文件存在类检查会失败：

- Biz 不依赖 `pkg/trpc-agent-go`：`archlint_test.go:14-34`。
- Service 不直接依赖 Data：`archlint_test.go:36-55`。
- 状态机文件存在性：`archlint_test.go:105-126`。

接口与复杂度超限使用 `t.Logf`，不会阻断：

- 接口方法上限：`archlint_test.go:57-103`。
- struct 字段上限：`archlint_test.go:128-178`。

本次实际告警包括：

- 9 个超过 5 方法的端口：`SpiritTeamAssembler`、`LegacyEvolutionSuggestionStore`、`TeamReader`、`TeamRunWriter`、`AgentRepository`、`OrganizationReader`、`SystemSettingRepo`、`TaskPlannerPort`、`ChannelTurnJobReader`。
- 40 余个超过 15 字段的 struct；包括 `AgentRuntimeSettings` 144、`MemoryCfg` 55、`Agent` 36、`GraphDefinition` 22、`Team` 27、`TurnPreviewCoordinator` 22。

结论：当前“archlint 通过”只能证明基本依赖方向，不代表 AS-COG-01 和接口窄化合规。

### 2.2 Race 结果

`go test -race ./...` 明确检测到：

- 读：`internal/agent/v2/sequencer_test.go:380`
- 写：`internal/agent/v2/sequencer_test.go:100`
- 生产调用栈：`internal/agent/v2/sequencer.go:278-319`

受影响测试包括 FIFO、StreamingBatchMerge、ActivityBridge、TaskCreated、DLQ、E2E pipeline 等。

同次执行后续发生系统盘空间不足，导致部分包无法链接。因此结论是：

- Sequencer race：**已验证失败**。
- 全仓其他包 race：**未完成验证**，不能写成通过或失败。

## 3. 前端验证细节

### 3.1 TypeScript

代表性错误：

- `BlockedResult` 未从 `useBlockedStatus` 导出：调用方与定义漂移。
- `window.setTimeout` 返回值与 `Timeout` 类型不兼容：`ChatMessageList.vue:226-235`。
- Spirit 新增 `paused`，两个状态映射未覆盖：`features/spirit/spiritUi.ts:8-18,139-150`。
- `ChatPage.vue` 使用不存在的 `activityTimeline`。
- `useOverviewPage.ts` 将 `string | undefined` 传给必须的 string。
- v2 状态/事件类型测试无法通过 compile-time equality。

结论：前端生成客户端能构建 bundle，不代表应用类型正确；CI 的独立 typecheck 是必要门禁。

### 3.2 Unit

失败用例：

- `ChatMessagePanel.legacy.spec.ts` 两个用例缺少 active Pinia。
- 测试过程还出现未解析 Quasar component、缺失 i18n key 和 setup error 警告。

结论：失败本身主要是 test harness，但它同时证明组件已从纯展示层转为依赖 store，和分层检查失败相互印证。

### 3.3 Layer

直接导入 Pinia store 的组件：

- `components/chat/ChatMessageList.vue`
- `components/chat/v2/GraphStageBlock.vue`
- `MemberSessionPanel.vue`
- `PlanBoardCard.vue`
- `TaskCard.vue`
- `TaskList.vue`
- `TeamRunCard.vue`
- `TeamStagePanel.vue`
- `TurnContainer.vue`

### 3.4 CI 对齐

`.github/workflows/ci.yml` 的前端 ESLint 使用 `--max-warnings=0`，因此本地普通 `pnpm lint` 虽退出 0，精确 CI 命令会因 83 warnings 失败。

其他门禁事实：

- Go coverage 注释声称 biz/service/data 分包阈值，但脚本只执行 total ≥50%：`ci.yml:293-299`。
- Trivy 配置 `exit-code: '1'`，同时又 `continue-on-error: true`：`ci.yml:303-316`。
- 根模块 `go test ./...` 不会进入嵌套的 `pkg/trpc-agent-go` modules。
- 前端无 coverage threshold。
- E2E 主要是 nightly，不阻断普通 PR。

## 4. 文档—契约—代码追踪基线

### 4.1 覆盖摘要

| 链路 | 评价 | 证据 |
|---|---|---|
| 需求→设计→开发计划 | 较完整 | 约 52 个编号模块有三件套 |
| 开发计划→代码锚点 | 中等 | 多数存在，CLI/Token/Graph UI 等缺正式锚点节 |
| 设计→Proto | 较差 | Chat、Team 端点表落后 |
| 设计→Schema | 较差 | 已 DROP 表和 v2 Schema 未同步 |
| 代码→测试 | 较差 | 只有少数 parity test 被文档明确引用 |
| 后端→前端 | 中等偏差 | 生成 client 广，但 feature/page 和 enum/mapper 不完整 |
| 全局索引→模块 | 断裂 | 多个被声明为真理源的文件不存在 |

### 4.2 已验证的失效真理源

- `docs/README.md` 不存在，但 `0-system-diagram.md:3-8` 要求优先阅读。
- `docs/guides/execution-plan.md` 不存在，但多份 development 文档声明其为全局进度真理源。
- `docs/development/README.md:71-84` 列出的多个跨模块文件不存在或已改名。
- `0-system.development.md` 引用不存在的 `README-development.md` 和 38 号计划。

### 4.3 事件文档漂移

- 34 号事件文档和迁移 SQL表明 legacy EventStore/WAL 已下线。
- 70 号长任务文档仍把 EventWAL、Postgres replay 等标为完成。
- `internal/server/ws_event.go:31-34` 明确 replay 已移除，改用 `ListActivities`。
- 65 号模块交叉参考仍引用旧 Envelope/旧前端文件。

### 4.4 Proto 漂移

Chat 设计表未列出的现有 RPC：

- SubmitChatMessage
- RetrySession
- PauseSession
- ResumeSession
- ConfirmPlan
- ListPlans
- GetPlan

证据：`api/kratos/chat/v1/chat.proto:264-390`，对照 `1-chat.design.md:142-158`。

Team 设计表未列出的现有 RPC：

- PauseTeamRun
- UnpauseTeamRun
- InjectTeamMessage

证据：`api/kratos/team/v1/team.proto:739-759`，对照 `11-multi-agent.design.md:124-151`。

### 4.5 Schema 漂移

`66-database-architecture.design.md:386-430`：

- 记录约 82 个 Schema。
- 仍列 Message 和 EventStore。
- 未列 `session_v2`、`turn_v2`、`task_v2`、`step_v2`、`plan_board_v2`、`plan_step_v2`、`team_run_v2`、`team_stage_v2`、`member_session_v2`、`graph_stage_v2`、`graph_node_v2`。

代码和迁移：

- Message/EventStore schema 已不存在。
- `20260901_drop_event_store_subsystem.sql`、`20260902_drop_messages_subsystem.sql` 明确删除旧子系统。

### 4.6 数据库需求冲突

`66-database-architecture.md` 同时包含：

- PostgreSQL 为唯一主库、不允许降级。
- SQLite 可作为降级/单机模式。

而当前 `internal/data/data.go` 已强制 PostgreSQL/pgvector。建议将 SQLite 只保留为测试适配器口径。

## 5. 未验证项

以下需要外部环境或更充足本机资源：

- 真实 PostgreSQL 迁移并发、RLS、FK rollout。
- 真实 Provider/LLM/MCP/A2A 外部服务 E2E。
- Docker/E2B/Container 恶意代码沙箱测试。
- 24h soak、进程 kill/restart、跨实例 lease/outbox。
- GitHub branch protection required-check 配置。
- 全部嵌套 `pkg/trpc-agent-go` module 的 race/coverage。

这些项目在报告中均未标记为通过。
