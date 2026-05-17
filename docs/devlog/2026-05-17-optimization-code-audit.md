# 2026-05-17 优化项代码层审计

> 审计对象：`docs/README.md`、`docs/guides/master-plan.md` 中提到的优化整改项，以及同日 S4/S5/S6 changelog 宣称已完成的能力。
>
> 审计方式：只读检查代码、ProviderSet、Wire、HTTP/gRPC 注册、Agent runtime 调用链、前端客户端与 CI 配置；未修改业务代码。

---

## 1. 总体结论

当前仓库对 `master-plan.md` 中的 P0 红线已经有明显整改：SQLite 单连接池、WebSocket 接入 Kratos、biz 层去 trpc 依赖、Memory 读持久化、Graph builder 拷贝与执行 GC、前端 Graph/Chat 生成客户端等核心问题基本修复。

但 S5/S6 中声明完成的若干模块仍停留在“源码文件存在”或“单元测试可构造”的层面，未形成端到端接入。最关键的问题是 `cmd/admin/wire_gen.go` 已明显过期，仍引用旧包和旧构造函数，导致 `cmd/admin` 当前无法编译；因此不能把 changelog 中的“已完成”直接视为可运行产品能力。

---

## 2. 严重问题

### P0-1 `wire_gen.go` 过期，admin 包无法编译

**状态：未解决**

`cmd/admin/wire.go` 已引入 `event.ProviderSet`、`internal/runtime.PersistenceSet`、`WSServer`、`SkillWatch` 等新注入链路，但 `cmd/admin/wire_gen.go` 仍是旧版本：

- 仍导入不存在的 `aranea-agents/internal/skillimport`。
- 仍构造旧版 `ChatService`，未传入 `ChatServiceDeps`。
- 仍引用旧 `NewSSEServer` / `newApp` 形态。
- `provideCronRunnerDeps` 仍使用旧的 `TeamSSE` 字段，而 `cronrunner.Deps` 已改为 `EventBus`。

验证命令：

```powershell
go test ./cmd/admin
```

结果：

```text
FAIL aranea-agents/cmd/admin [setup failed]
cmd\admin\wire_gen.go:16:2: package aranea-agents/internal/skillimport is not in std
```

**影响：** 当前主程序无法通过编译，所有“已接入 Wire / 可启动”的结论都必须先以重新生成 Wire 并编译通过为前置条件。

---

## 3. P0 红线与核心缺陷整改状态

| 优化项 | 状态 | 代码证据 | 结论 |
|---|---|---|---|
| SQLite 单连接池 | 完成 | `internal/data/data.go`、`internal/session/trpc/sqlite.go`、`internal/graph/trpc/checkpoint.go` | `internal` 内 `sql.Open` 只在 `data.go`；Session/Checkpoint 使用注入的 `*sql.DB`。 |
| WebSocket 接入 Kratos | 完成 | `internal/server/ws.go`、`internal/server/http.go` | 已通过 `wsSrv.RegisterOnKratos(srv)` 挂到 Kratos HTTP，不再独立监听。 |
| biz 层去 trpc 依赖 | 完成 | `internal/biz/*.go` | 未发现 `internal/biz` import `pkg/trpc-agent-go` 或 `internal/*/trpc`。 |
| Memory cache 空读 | 完成 | `internal/memory/trpc/sqlite_adapter.go` | `ReadMemories` 读取持久化 store，不再依赖每 turn 新建实例的空 cache。 |
| 自动记忆任务 | 部分完成 | `internal/memory/trpc/sqlite_adapter.go`、`internal/cronrunner/jobs/auto_memory.go` | 已非空实现，但目前更接近进程内 best-effort 队列；队列满会丢弃，尚非 master-plan 期望的可靠落表。 |
| EventBus 背压 | 部分完成 | `internal/event/bus.go`、`internal/server/ws.go` | Bus 支持 `Reliable` / `DropPolicy`，关键事件偏可靠；但 WS 等订阅方未明确区分 lossy/reliable 策略。 |
| Graph builder race | 完成 | `internal/graph/trpc/builder.go` | 构建时拷贝节点/边等切片，避免修改共享输入。 |
| Graph executions GC | 完成 | `internal/biz/graph.go` | `NewGraphUsecase` 启动 GC loop，按最大存活时间清理执行记录。 |
| 前端 Graph 客户端 | 完成 | `web/src/services/kratos/graph/v1/index.ts`、`web/src/services/index.ts` | Graph TS 客户端已生成并导出。 |
| Chat 使用生成客户端 | 完成 | `web/src/features/chat/api.ts` | 已使用 `createChatService()`。 |

---

## 4. 功能补全模块接入状态

| 模块 | 状态 | 代码证据 | 发现的问题 |
|---|---|---|---|
| Plugin Runtime | 部分完成 | `internal/plugin/trpc/runtime.go`、`internal/service/plugin.go`、`internal/agent/turn_helpers.go` | Plugin runtime 与 CRUD 存在，但 `service/trpc_turn.go`、`internal/team/runner_team_trpc.go` 调 `NewRunnerDepsFromRuntime` 时未传入 plugins，单聊和 Team 路径实际未消费插件。 |
| Skill DB Repository | 未端到端完成 | `internal/skill/trpc/db_repository.go`、`internal/agent/trpc_build.go`、`internal/tools/skillruntime/toolset.go` | DB adapter 已存在，但主构建路径仍使用 `NewFSRepositoryAdapter`。 |
| Planner 多策略 | 完成 | `internal/agent/trpc_build.go`、`internal/agent/planner/selector.go` | 已按 `dialogMode` / `PlannerKind` 接入选择器。 |
| Artifact | 部分完成 | `api/kratos/artifact/v1/artifact.proto`、`internal/biz/artifact.go`、`internal/data/artifactfs/repo.go`、`internal/service/artifact.go` | 代码存在，但未进入 `biz.ProviderSet`、`data.ProviderSet`、`service.ProviderSet`，也未在 `internal/server/http.go` / `grpc.go` 注册。 |
| Cron 重试/死信/指标 | 完成但受 Wire 影响 | `internal/cronrunner/runner.go` | Runner 逻辑已实现；但 `wire_gen.go` 对 `cronrunner.Deps` 仍是旧字段，集成编译前不可验收。 |
| Knowledge | 部分完成 | `api/kratos/knowledge/v1/knowledge.proto`、`internal/biz/knowledge.go`、`internal/data/knowledge.go`、`internal/service/knowledge.go`、`internal/tools/knowledge/tool.go` | 分层文件存在，但未进 ProviderSet/Server 注册；`knowledge_search` 工具未被 Agent 工具注册路径引用。 |
| Evaluation | 部分完成 | `internal/biz/evaluation.go`、`internal/data/evaluation.go`、`internal/evaluation/runner.go`、`internal/service/evaluation.go` | 代码存在，但未进 ProviderSet/Server 注册。 |
| A2A | 部分完成 | `api/kratos/a2a/v1/a2a.proto`、`internal/biz/a2a.go`、`internal/data/a2a.go`、`internal/service/a2a.go`、`internal/a2a/tool.go` | `call_agent` 工具仅定义，未注册到 Agent；服务未进 ProviderSet/Server。 |
| CodeExecutor Docker | 未端到端完成 | `internal/agent/codeexecutor/executor.go`、`internal/agent/trpc_build.go` | Docker executor 存在，但产品路径仍使用 `skilltrpc.NewLocalExecutor`；`internal/agent/codeexecutor` 只被测试引用。 |
| Callback Chain | 部分完成 | `internal/agent/callbacks/*`、`internal/agent/trpc_build.go` | Chain 抽象存在，但构建路径仍主要使用 tool callback，Agent/Model/Plugin 级链路未真正挂载。 |
| RunStatus / Gateway | 部分完成 | `api/kratos/chat/v1/chat.proto`、`internal/service/chat.go`、`web/src/composables/useRunStatus.ts` | Chat 域 `GetRunStatus` 已有；但 master-plan 中 trpc gateway/ManagedRunner 路由未见后端接入。 |

---

## 5. 前端与工程化状态

### 已完成或基本完成

- Pinia store 已补齐主要业务域，`web/src/stores/index.ts` 导出 admin、agents、avatar、channels、chat、cron、graph、heartbeat、mcp、memory、monitor、platform、plugins、session、skills、system-settings、teams、tools、usage。
- `web/src/services/wsClient.ts` 与 `web/src/composables/useWS.ts` 已形成统一 WebSocket 客户端和重连封装。
- `web/src/services/axiosHandler.ts` 已集中处理 HTTP 错误、认证与 Quasar Notify。
- `web/package.json`、`web/vitest.config.ts` 和若干 `*.spec.ts` 已建立前端测试基础。
- TS 生成客户端目录已包含 graph、artifact、a2a、knowledge、evaluation 等新 proto。

### 未完全符合 master-plan

- `Makefile` 有 `wire-admin`，但没有文档要求的 `make wire` 别名。
- `make lint` 运行 `cmd/araneactl/lint` 和 `go vet`，未包含 `golangci-lint`。
- `make test` 只跑 Go 测试，未包含前端 vitest。
- CI 中前端 `npm test` 失败不会阻断；E2E job 为夜间/手动且 `continue-on-error`，仓库内也未看到完整 Cypress 工程。
- `web/src/services/index.ts` 尚未封装导出 artifact、knowledge、evaluation、a2a 的 `create*Service()`。
- `master-plan.md` 仍有“仅 2 个 Pinia store”“Graph 客户端缺失”等旧描述，与当前代码不一致。

---

## 6. 文档与代码不一致

| 文档声明 | 当前代码情况 | 建议 |
|---|---|---|
| README 当前状态仍标记 Memory、Plugin、Artifact、Knowledge、A2A、Evaluation 多项未实现 | 代码已有不同程度实现，但多数未端到端接线 | 状态表应拆成“源码实现 / DI 接入 / HTTP/gRPC 注册 / runtime 接入 / 测试验证”五列。 |
| S5 changelog 宣称 Artifact Minimal Implementation | Artifact 业务/服务文件存在，但未注册 ProviderSet 与 Server | 修改 changelog 或补齐接线后再标为完成。 |
| S6 changelog 将 Knowledge/Evaluation/A2A/CodeExecutor 标为 ✅ | 多数仅文件存在，未进 Wire/Server/Agent runtime | 降级为“代码骨架完成，端到端接入待完成”。 |
| master-plan §1.3 仍称前端仅 2 个 store | 当前已补齐大量 stores | 更新 master-plan 的现状描述。 |
| master-plan §9 验收要求 `go test ./...`、`pnpm build`、`pnpm test` | 当前 CI 使用 npm，Go 测试为包白名单，前端测试失败不阻断 | 统一文档与实际工具链，或按文档增强 CI。 |

---

## 7. 建议修复优先级

### 立即处理

1. 重新生成 `cmd/admin/wire_gen.go`，确保 `go test ./cmd/admin` 通过。
2. 将 `Artifact/Knowledge/Evaluation/A2A` 的 Usecase、Repo、Service 纳入 ProviderSet，并在 HTTP/gRPC server 注册。
3. 修复 `Plugin Runtime` 传入 Agent/Team Runner 的链路。
4. 为 `Knowledge` / `A2A` 工具加入 Agent 工具注册路径，或明确文档标注“仅 API 能力，暂不作为 Agent tool”。

### 一个迭代内处理

1. 将 Skill DB Repository 接入 `buildSkillDeps` 或明确保留 FS 作为主实现。
2. 将 Docker CodeExecutor 接入 skill/code execution 路径，并提供配置开关。
3. 完成 Callback Chain 到 Agent/Model/Plugin 级回调的挂载。
4. 补齐 `web/src/services/index.ts` 对新增 proto client 的封装导出。
5. 修正 `Makefile` / CI，使 `lint`、`test`、`ci` 与验收标准一致。

### 文档同步

1. 更新 `docs/README.md` 模块状态表，避免“未实现”和“已部分接线”的状态混用。
2. 更新 `docs/guides/master-plan.md` 中过时的前端 store、Graph 客户端、模块状态描述。
3. 对 S5/S6 changelog 增加限制说明：哪些是源码骨架、哪些已端到端可用。

---

## 8. 本次验证记录

已执行：

```powershell
go test ./cmd/admin
```

结果：失败，原因是 `cmd/admin/wire_gen.go` 仍引用旧路径 `internal/skillimport`。

未执行全量 `go test ./...` 和前端 `npm test`，因为主入口包已在编译阶段失败，继续跑全量测试无法作为有效验收依据。
