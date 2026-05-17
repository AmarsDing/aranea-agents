# Aranea-Agents 执行计划（AI 协作权威基线）

> 版本：v1.0（2026-05-17）
> 编制依据：`docs/README.md`、`docs/需求/*`（86 篇）、`docs/guides/{AI-DEVELOPMENT-SPECIFICATION, master-plan, implementation-plan, task-tracker, plan, trpc-agent-go-framework}.md`、`docs/changelog/*`、`docs/devlog/2026-05-17-optimization-code-audit.md`，以及 `cmd/ internal/ api/ pkg/ web/` 全量代码与配置抽样。
> 文档定位：**给后续 AI 迭代使用的唯一执行基线**。任何 PR / commit 之前，AI 必须按本文 §10 自检。
> 关系：master-plan、implementation-plan、task-tracker、plan 仍保留作为历史参考；本文负责把它们与代码现实重新对齐，并提供新的工作分解。

---

## 0. 阅读路径（先读这一页）

- §1 现状全景：六轴（架构、业务、数据、运行时、前端、可观测）当前状态
- §2 文档与代码的根本性矛盾：必须先解决的元问题
- §3 全域漏洞与不足清单：分类 + 证据 + 优先级
- §4 长效优化路线（M0–M5 里程碑）
- §5 立即可执行 Top-20 工作（每项可一 PR 落地）
- §6 红线与硬约束（在 AI-DEV-SPEC 基础上扩展）
- §7 AI 协作迭代约束（每个 Sprint / PR / Commit 标准动作）
- §8 验收与质量门（机器可检查）
- §9 风险与缓解
- §10 AI 自检清单（每次开工必读）
- §11 文档治理（写在哪、状态如何同步）
- 附录 A：模块状态矩阵
- 附录 B：相关文档索引

---

## 1. 现状全景（事实校准）

### 1.1 已验证可运行项

- `go build ./cmd/admin` 通过（与早期审计的"wire_gen.go 过期"结论已被新版本修正）。
- `internal/data/data.go` 是唯一 `sql.Open` 点，Ent 与 trpc Session/Checkpoint 共用 `RawDB()`。
- `internal/server/ws.go` 已通过 `wsSrv.RegisterOnKratos(srv)` 挂入 Kratos HTTP，不再独立监听。
- `internal/biz/*` 内未 import `pkg/trpc-agent-go` / `internal/*/trpc`，红线 R2/R8 成立。
- Graph builder 已做切片深拷贝避免并发 race；`biz.GraphUsecase` 启动 `gcLoop` 清理执行记录。
- Cron Runner 已支持 `dispatchWithRetry` / dead-letter / Prometheus 指标。
- 前端 `web/src/services/index.ts` 与 `web/src/services/kratos/<24 个域>/v1/index.ts` 已一一对应；`createChatService` 等生成客户端已在 `features/<域>/api.ts` 中使用。
- Pinia store 已覆盖 19 个业务域。

### 1.2 已被代码反超的"待办"

文档里仍标"未实现"或"P2 待启动"，实际代码已经存在或部分存在：

| 文档主张 | 实际代码状态 | 证据 |
|---|---|---|
| `docs/需求/0 系统框图.md` 接入层标"A2A 规划中" | A2A 已注册 HTTP/gRPC Server | `internal/server/http.go`、`grpc.go` |
| `docs/需求/37 knowledge.md` 称"项目无 Knowledge" | Knowledge 已有 proto + service + biz + data + 检索器 | `internal/biz/knowledge.go`、`internal/service/knowledge.go`、`internal/knowledge/*` |
| `docs/需求/27 artifact.md` 称"项目无 Artifact" | Artifact REST 已可用，trpc Service 适配也有 | `api/kratos/artifact/v1/*`、`internal/service/artifact.go`、`internal/artifact/trpc/service.go` |
| `docs/需求/22 plugin.md` 称 Plugin 未注入 Runner | Chat/Team 单聊路径已 `WithPlugins` | `internal/service/trpc_turn.go`、`internal/agent/trpc_runtime.go` |
| `docs/需求/12-16 memory.design.md` 标 Memory RPC 待新增 | `memory.proto` 已含 L0 快照、L1 task/field 等多个 RPC | `api/kratos/memory/v1/memory.proto` |
| `task-tracker.md` 全量 41 个任务 `pending` | 多任务实际已合并（S1 全部、S2-S6 大部分） | `docs/changelog/2026-05-17-S{1..6}-*.md` |

### 1.3 仍然真实缺失或半成品

| 主题 | 现实状态 | 关键证据 |
|---|---|---|
| RunStatus / AwaitUserReply 真实回路 | RPC + Service 字段在，但 `setRunStatus(` 全仓无调用，`awaitChans.Store(` 全仓无调用，永远返回 `idle` / `accepted=false` | `internal/service/chat.go:332-388` |
| Skill DB Repository | `internal/skill/trpc/db_repository.go` 存在，主链路仍 `NewFSRepositoryAdapter` | `internal/agent/trpc_build.go:160`、`internal/tools/skillruntime/toolset.go:34` |
| CodeExecutor Docker | `internal/agent/codeexecutor/executor.go` 存在但仅在测试中引用，Skill 仍走 `NewLocalExecutor` | `internal/agent/trpc_build.go` |
| Auto Memory Worker | 队列存在，但 `internal/cronrunner/jobs/auto_memory.go` 的 `extract` 体仍是日志占位 `return nil`，未接 `pkg/trpc-agent-go/memory/extractor` | 同文件 |
| Callback Chain | `internal/agent/callbacks/` 抽象存在但未挂载到 LLMAgent；仅 ToolCallback 接通 | `internal/agent/trpc_build.go:buildToolCallbacks` |
| EventBus 背压差异化 | Bus 自身支持 Reliable / DropPolicy，但 WS、`biz/event_bus_consumer.go` 等订阅方仅传 `BufferSize`，没区分 reliable / lossy | `internal/server/ws.go` SubscribeOptions |
| Metrics 暴露面 | `internal/server/metrics.go` 用 `promauto` 在 init 注册了一堆 `aranea_*` 指标，但 `RegisterMetricsHandler` 全仓无调用，**没有 `/metrics` 端点** | `internal/server/metrics.go` |
| OpenTelemetry | 主进程 admin 日志带 `TraceID/SpanID` 字段，但未挂载 OTel Tracing middleware，也未配置 exporter | `cmd/admin/main.go:72` |
| 多租户 Ent hook | 没有 Ent Hook 强制 `workspace_id` 谓词，`servermw.AssertWorkspace` 定义后从未被调用 | `internal/server/middleware/workspace.go` |
| 鉴权强度 | HTTP 用 cookie+JWT；`KRATOS_AUTH_SECRET` 未设时使用当前时间戳作密钥；`KRATOS_HTTP_AUTH_DISABLED` 可整段绕过；`/webhooks/*` 整体放行；gRPC 无鉴权 | `pkg/auth/middleware.go`、`config.go`、`features.go` |
| 前端 WS 双轨 | `web/src/services/wsClient.ts` + `useWS.ts` 整套封装但全工程无调用；真实 WS 用 `features/chat/ws-transport.ts` | `web/src/services/wsClient.ts` |
| 前端展示组件违反数据流 | 多个 `components/<域>/*.vue`、`composables/useRunStatus.ts` 直接 `import` `features/*/api`，跳过 Pinia store | `components/sessions/SessionTurnsPanel.vue:53` 等十余处 |
| 前端硬编码颜色 | 多个 .vue scoped style 用 `#fff`、`#0f172a`、`#2563eb` 等，违反 §8.2 token 化 | `components/sessions/SessionTimelinePanel.vue:173-211` 等 |
| Cypress E2E | CI `e2e-nightly` job 引用 cypress，但仓库无 `cypress/` 目录、无依赖、`continue-on-error: true`，等同未启用 | `.github/workflows/ci.yml:140-177` |
| 测试矩阵 | Go 测试仅跑白名单 `$TESTABLE_PKGS`，60% 阈值只覆盖白名单；前端 `npm test` 加了 `|| echo` 失败不阻断 | `.github/workflows/ci.yml:47-89` |
| Adaptive Team mode | `biz.TeamUsecase` 允许 `adaptive` 但 `internal/team/trpc_build.go` switch 落到 default，与 coordinator 等价 | `internal/team/trpc_build.go:52-105` |
| Knowledge / A2A / Evaluation 工具未注册 | `internal/tools/knowledge/tool.go`、`internal/a2a/tool.go` 仅定义，没被 BuildToolsets / Registry 引用 | `internal/agent/trpc_build.go` 工具装配链 |
| EvolutionScanner / L4 持久层 | `docs/需求/16 memory-L4-persistent.md` 勾选 `RunEvolutionScan`/30min ticker；代码内不存在 | `internal/` 无对应 worker |
| 51 消息机制"单 WS 通道" | 文档要求 WS 取代 SSE 为唯一通道；`51a 后端消息机制.md` 自身仍描述 `POST /v1/chat/messages/stream` (SSE)；代码里 WS + Chat SSE 并存 | `internal/server/ws.go` + `internal/service/chat.go` |
| CLI 产品 | `docs/需求/25 cli.md` 要求 `aranea` 可执行 CLI / REPL；实际只有 `cmd/admin`（服务进程）+ `cmd/araneactl/lint`（lint 工具） | `cmd/` 目录 |

### 1.4 文档与文档之间的矛盾（必须治理）

- `docs/guides/master-plan.md §4` 状态表 ≠ `docs/guides/plan.md §2` 状态表 ≠ `docs/README.md §6 对齐状态表` ≠ `docs/guides/task-tracker.md §1 总览`。
- task-tracker 显示"0/41 done"；changelog 显示 S1–S6 全部已合。两份是矛盾的。
- `docs/需求/24 telemetry.md` 内容为空，`.design.md` 有完整 OTel 方案。
- `docs/需求/16 memory-L4-persistent.md §1` 实现状态勾选的文件在仓库中不存在。
- `docs/需求/34 event-system.md §1` 称"缺失 StateDelta/Extensions/Branch/Tag"，而 `docs/需求/1 chat.md` 与 `internal/server/ws.go` 已支持。

---

## 2. 元问题：文档/代码协同失控

进度信息分散在四类文档里，每类口径不同：

- **master-plan / plan**：按 M1-M20 模块状态描述；多处过期。
- **implementation-plan / task-tracker**：按 S1-S6 / T1-T41 跟踪；当前全 pending。
- **changelog**：按 sprint 宣称完成；超出代码实际接入度。
- **需求 + 设计文档**：按业务域描述目标；多篇与代码反向（代码已实现，文档仍称待实现）。

> **执行计划首要动作 EP-DOC-01：把"进度真相"集中到本执行计划 §3 / 附录 A 的状态矩阵，其余文档只允许引用本表，不允许独立维护进度信息。**

---

## 3. 全域漏洞与不足清单（按优先级）

> 命名规则：`EP-<域>-<序号>`；优先级 P0=阻塞 / 安全 / 数据正确性；P1=主路径功能；P2=可用性 / 完整性；P3=长期演进。

### 3.1 安全（P0/P1）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-SEC-01 | `pkg/auth/config.go:authSecretFromEnv` 未设 `KRATOS_AUTH_SECRET` 时用 `time.Now().Format(...)` 作为 JWT 签名密钥 | P0 | `pkg/auth/config.go:8-14` | fail-fast：未设密钥则启动失败；或在开发模式下显式日志告警 |
| EP-SEC-02 | `KRATOS_HTTP_AUTH_DISABLED=1` 全站绕过 + 注入 `UserID=1, admin` | P0 | `pkg/auth/middleware.go:15-66` | 限制只在 `DEPLOY_ENV=dev` 时允许；启动期 banner 警告；CI 检查生产构建 |
| EP-SEC-03 | `/webhooks/*` 整体放行鉴权；只靠各 channel 自身 verify | P1 | `pkg/auth/middleware.go` | 在 middleware 中要求 channel 路径前置注册并自带签名校验；未注册路径 401 |
| EP-SEC-04 | gRPC Server 仅 recovery + validate，无鉴权 | P1 | `internal/server/grpc.go:55-60` | 加 grpc auth interceptor，或在文档中明确"gRPC 仅内网" |

### 3.2 数据一致性 / 运行时正确性（P0/P1）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-RT-01 | `setRunStatus` 全仓无调用 → `GetRunStatus` 恒为 `idle` | P0 | `internal/service/chat.go:332-388` | 在 `trpc_turn.go` 的 `runSingleAgentViaTRPC` / `runTeamTRPC` 开始/结束/出错时调用 |
| EP-RT-02 | `awaitChans.Store` 全仓无调用 → `AwaitUserReply` 永远 accepted=false | P0 | 同上 | 当 Agent 走 await_user_reply tool 时落 channel；前端走 `useRunStatus` 才能闭环 |
| EP-RT-03 | Auto Memory `extract` 体为 `return nil` 占位 | P1 | `internal/cronrunner/jobs/auto_memory.go` | 接 `pkg/trpc-agent-go/memory/extractor` 或自研 LLM extractor，写入 `session_memory` 表 |
| EP-RT-04 | `internal/team/trpc_build.go` adaptive 模式落到 default，与 coordinator 不可区分 | P1 | `internal/team/trpc_build.go:52-105` | 实现 adaptive 编排（按上下文动态选 coord / swarm），或在 biz 校验中拒绝 adaptive |
| EP-RT-05 | Memory tool（`memorytool.DefaultTools`）依赖 Runner 的 `WithMemoryService` 注入，但 Settings 默认未必启用；需明确开关与回退策略 | P1 | `internal/agent/trpc_build.go:117-123` | 在 Agent Settings UI / 默认值中显式控制；缺失 MemoryService 时禁用工具集 |
| EP-RT-06 | EventBus 订阅方未区分 reliable/lossy；WS 默认丢老事件可能丢 `tool_result` | P1 | `internal/server/ws.go` SubscribeOptions | WS 订阅按事件类型分通道，关键事件 Reliable=true，文本 delta lossy |
| EP-RT-07 | Cron 派发链路不走 Agent Runner 构造函数，Plugin 回调对 Cron 任务可能失效 | P1 | `internal/cronrunner` 不 import plugin runtime | Cron 走统一的 `ChatService.RunNativeTurnUnary`，或在 worker 内复用 plugins |
| EP-RT-08 | `internal/biz/biz_coverage_test.go` 等多个 unused import / 内存 repo 仅服务于测试，未对应生产实现（Artifact / Knowledge / Eval / A2A） | P2 | `internal/biz/biz_coverage_test.go`、`s6_coverage_test.go` | 把内存实现升级为真实存储后端或明确标"骨架未接线" |

### 3.3 业务接入闭环（P1）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-BIZ-01 | Skill DB Repository 已有但未接入主链路 | P1 | `internal/skill/trpc/db_repository.go`、`internal/agent/trpc_build.go:160` | 把 `buildSkillDeps` 改为按配置选择 FS / DB，默认 DB；保留 FS 作 dev fallback |
| EP-BIZ-02 | CodeExecutor Docker 已实现但只在测试引用 | P1 | `internal/agent/codeexecutor/executor.go` | Skill 执行器加 backend selector，配置驱动；提供 Docker compose 示例 |
| EP-BIZ-03 | Knowledge / A2A 工具未注册到 Agent 工具装配链 | P1 | `internal/tools/knowledge/tool.go`、`internal/a2a/tool.go` | 在 `tools/registry.go` 注册 + `buildToolsetsForAgent` 开关 |
| EP-BIZ-04 | Evaluation Runner 在 `internal/service/wire_providers.go` 用 `nil, nil` 构造 | P1 | `internal/service/wire_providers.go`（按子代理引用） | 真实注入 AgentRunner + Judge，或在文档中明确"仅评估元数据，不跑用例" |
| EP-BIZ-05 | Channel 多渠道适配缺失（仅 feishu 真正走端到端） | P2 | `internal/service/channel_ingress.go` + `internal/channel/lark` | 实现 wechat / dingtalk / slack / 邮件入站，或在前端禁用未实现渠道 |
| EP-BIZ-06 | ToolOverride 域只在 proto / 统计语境出现，无 CRUD / Usecase | P2 | proto agent_override_count 字段 + 缺少 biz/data | 补 `biz/tool_override.go` 与 Repo |
| EP-BIZ-07 | EvolutionScanner（L4 持久层）需求文档勾选，但代码不存在 | P2 | `docs/需求/16 memory-L4-persistent.md` vs `internal/` | 要么实现 30min ticker scanner，要么把需求降级为"未实现" |

### 3.4 可观测 / 运维（P1）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-OBS-01 | `/metrics` 端点未挂载，预定义指标基本未采样 | P1 | `internal/server/metrics.go` `RegisterMetricsHandler` 无调用 | 在 `NewHTTPServer` 末尾挂 `srv.Route("/").GET("/metrics", ...)`；同时在 ChatTurn / Tool / Memory 关键点采样 |
| EP-OBS-02 | OpenTelemetry 未在 Server 中间件层接入 | P1 | `cmd/admin/main.go:72` + Server 文件 | 注入 Kratos tracing.Server middleware + OTLP exporter；提供 `OTEL_EXPORTER_OTLP_ENDPOINT` 环境变量 |
| EP-OBS-03 | WSServer 未纳入 `kratos.App` Server 列表，优雅退出可能不发 `broadcastShutdown` | P2 | `cmd/admin/main.go` 与 `internal/server/ws.go` | 把 WSServer 注册为 transport.Server，或在 main `AfterStop` 显式 Stop |
| EP-OBS-04 | 大量定义但未采样的 metrics（ChatTurnDuration、EventBusPublished 等） | P2 | `internal/server/metrics.go` 中变量 | 在生成点 `inc()`/`observe()`；删除长期不打点的指标 |
| EP-OBS-05 | `docs/observability/grafana-aranea.json` 仪表与代码采样字段不匹配（指标未上报） | P2 | Grafana JSON 引用 `aranea_chat_turn_duration_seconds` 等 | 与 EP-OBS-01 联动修复 |

### 3.5 前端（P1/P2）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-FE-01 | 展示组件直 import `features/*/api`，违反 §7.1/§7.2 | P1 | `components/sessions/*.vue`、`components/monitor/*.vue`、`components/tools/ToolDetailContent.vue`、`composables/useRunStatus.ts` 等 | 抽 store action，组件改用 store；CI 加 lint：`components/**/*.vue` 禁止 `from .*features/.*/api` |
| EP-FE-02 | `wsClient.ts` + `useWS.ts` 全工程未引用，与真实 chat WS 双轨 | P2 | `web/src/services/wsClient.ts` 全仓无 import | 二选一：把 chat ws-transport 收敛到 `wsClient`；或删除 `wsClient.ts` / `useWS.ts` |
| EP-FE-03 | 硬编码颜色违反 token | P2 | `components/sessions/SessionTimelinePanel.vue:173-211`、`components/chat/ChatSessionSidebar.vue:320-359` 等 | stylelint 加规则；批量重构使用 `var(--*)` |
| EP-FE-04 | Cypress E2E 配置缺失但 CI 仍尝试运行 | P2 | `.github/workflows/ci.yml:140-177` + 无 `web/cypress` | 立项：要么真接入 Cypress（含 fixtures、CI 必跑），要么把 e2e job 删除 |
| EP-FE-05 | `heartbeat` 域无 `api.ts` 与生成客户端，仅 composable | P3 | `web/src/features/heartbeat/useServerHeartbeat.ts` | 补 api 层或合并到 system-settings |
| EP-FE-06 | 设计 token：Quasar Material 色（如 `#4caf50`/`#f44336`）出现在 Graph 节点等组件 | P3 | `components/graph/GraphFlowNode.vue` | 用 token 体系替换 |

### 3.6 工程化与 CI（P1）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-ENG-01 | Go test 跑白名单包，60% 阈值不真实 | P1 | `.github/workflows/ci.yml:47-57` | 改 `go test -coverprofile=cov.out ./...`；可保留 race subset；阈值按 Sprint 分阶段提升 |
| EP-ENG-02 | 前端 `npm test` `|| echo` 失败不阻断 | P1 | `.github/workflows/ci.yml:85-89` | 去掉 echo，失败即 fail；先把现有 3 个 spec 保稳 |
| EP-ENG-03 | `make wire` 别名缺失，且 `wire_gen.go` 与 `wire.go` 之间不强制一致性检查 | P1 | `Makefile` + 无 `make wire-clean` | 加 `make wire`（运行 wire 后 git diff 为空才通过）；CI `wire-clean` job |
| EP-ENG-04 | `make api` 提交检查缺失（生成产物可能与 proto 不同步） | P2 | `Makefile` | 加 `proto-clean` job：跑 `make api` 后 git diff 必须为空 |
| EP-ENG-05 | `golangci-lint` 未集成 | P2 | Makefile/CI 仅 `go vet` + araneactl lint | 引入 golangci-lint，配置最小集（govet、ineffassign、staticcheck、errcheck） |
| EP-ENG-06 | `task-tracker.md` 与 changelog 严重不一致，且未自动校验 | P1 | `docs/guides/task-tracker.md` 总览 0/41 | 弃用 task-tracker；以本执行计划 §3 + 附录 A 为唯一进度源；新增 `araneactl docs-check` |
| EP-ENG-07 | `Makefile` lint 不包含 `gofmt`/`goimports` 检查 | P3 | Makefile | 加 fmt check |

### 3.7 文档治理（P0）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-DOC-01 | 进度信息四处分散且彼此矛盾 | P0 | 见 §1.4 / §2 | 本计划 §3 + 附录 A 为唯一进度真相 |
| EP-DOC-02 | `docs/需求/24 telemetry.md` 空文件 | P1 | 文件大小 | 写明状态：`参见 24 telemetry.design.md` |
| EP-DOC-03 | `docs/需求/16 memory-L4-persistent.md` 描述代码不存在 | P1 | 与代码核对 | 重写为"未实现"或先实现再标 |
| EP-DOC-04 | `docs/需求/27 artifact.md`、`37 knowledge.md` 称"无能力"但代码已具备 | P2 | 与代码核对 | 更新现状段，区分"REST 已通"和"Runner 注入待完成" |
| EP-DOC-05 | `docs/需求/34 event-system.md` 描述与 chat SSE 实际能力矛盾 | P2 | 与 `1 chat.md` 对比 | 标历史草稿或重写 |
| EP-DOC-06 | `docs/需求/31 memery.md` 文件名拼写错误 | P3 | 文件 | rename 为 `31 memory.md` |
| EP-DOC-07 | `docs/需求/51 消息机制.md` 与 `51a 后端消息机制.md` 自相矛盾（WS 唯一通道 vs 仍写 SSE） | P2 | 两文件本身 | 统一为"WS 主通道 + Chat SSE 兼容回退" |

### 3.8 红线/规范缺口（P1）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-RULE-01 | `pkg/apierror` 实际几乎无人使用，service 层主要用 `kerrors` | P2 | grep `apierror.` 在 `internal/*` 仅 workspace middleware 与测试 | 二选一：扩大 apierror 使用面，或者只在 middleware/边界保留并标注；编辑 §6 红线 |
| EP-RULE-02 | `cmd/araneactl/lint` 未检查"展示组件 import features/api"前端约束 | P2 | `cmd/araneactl/lint/main.go` 规则集 | 新增 web lint rule（用 eslint plugin 或 araneactl 内 web 子检查） |
| EP-RULE-03 | `cmd/araneactl/lint` R3 被刻意跳过，data → biz 接口依赖是否允许未文档化 | P3 | `cmd/araneactl/lint/main.go:85-109` | 在 AI-DEV-SPEC §1.3 / 本计划 §6 写明 |
| EP-RULE-04 | `pkg/safego` 已存在但仅部分 goroutine 使用 recover | P2 | grep `safego.Go` | 全量梳理 `go func()`，强制 `safego.Go`；lint 检查 |

---

## 4. 长效优化路线（M0–M5）

> 与原 master-plan S1-S6 相比，本路线按"产品里程碑 + 能力闭环"组织，而不是按"红线消除 → 架构债 → 功能补全"的工程顺序；每个里程碑都包含安全、可观测、测试、文档四条平行子线。

### M0 真相同步与基础闭合（1 周）

目标：把"文档真相"与"代码真相"对齐，把 P0 安全与运行时正确性补足。

- EP-DOC-01：本计划替代 task-tracker；其余文档 §2/§3 收编为"现状已知问题"，删除/降级旧 sprint 信息。
- EP-SEC-01 / EP-SEC-02：JWT 密钥 fail-fast + dev bypass 限定；启动 banner。
- EP-RT-01 / EP-RT-02：RunStatus / AwaitUserReply 真正落 store，前端 `useRunStatus` 可见。
- EP-OBS-01：暴露 `/metrics`（先 expose，再补点）。
- 验收：本 §1.3 P0 行全部消除；`go build ./cmd/admin` + `go test ./...` + `npm test` 全过；`/metrics` 可访问。

### M1 端到端闭合（2 周）

目标：让 changelog 已宣称完成的能力真正接通。

- EP-BIZ-01 Skill DB Repo 接入主链路。
- EP-BIZ-02 CodeExecutor Docker backend selector。
- EP-BIZ-03 Knowledge / A2A 工具注册到 Agent。
- EP-BIZ-04 Evaluation Runner 真实 Agent + Judge 注入。
- EP-RT-03 Auto Memory extractor 落地。
- EP-RT-07 Cron 走统一 Runner 入口（或显式插件复用）。
- EP-OBS-02 OTel 接入 Server middleware + OTLP exporter（可仅 dev）。
- 验收：`docs/changelog/2026-05-17-S{4,5,6}-*.md` 中每一项 ✅ 都能由附录 A "Runtime 接入"列证明。

### M2 多租户与安全收口（2 周）

- EP-SEC-03 / EP-SEC-04 webhook 与 gRPC 鉴权。
- EP-RT-06 EventBus 订阅方按事件类型分通道。
- 多租户：在 Ent 添加 Hook 强制 `workspace_id` 谓词；在 biz 层把 `workspace.FromContext` 注入所有查询；`AssertWorkspace` 在写操作中强制执行。
- 审计：所有写操作落 audit_log 表（带 workspace、user、action、subject_id、diff hash）。
- 验收：CI 加 `make tenant-check`（grep + AST）；smoke 用两个 workspace 验证彼此不可见。

### M3 可观测与体验（2 周）

- EP-FE-01 / EP-FE-02：前端数据流回归 §7.1；WS 双轨二选一。
- EP-FE-03 / EP-FE-06：硬编码颜色 → token；stylelint 兜底。
- EP-OBS-04 / EP-OBS-05：补齐 metrics 采样 + Grafana 联调。
- 51 消息机制：明确"WS 主通道，Chat SSE 仅兼容回退"，统一文档与代码。

### M4 测试矩阵与 CI 真实化（2 周）

- EP-ENG-01 全量 `./...` 测试 + race subset。
- EP-ENG-02 前端 `npm test` 失败必 fail。
- EP-ENG-03 / EP-ENG-04 `make wire` / `make api` 一致性检查。
- EP-ENG-05 接入 golangci-lint。
- EP-FE-04 Cypress 决策（接入或删 e2e job）。
- Go line coverage 阶梯：M3=40% → M4=60% → M5=70%。

### M5 长期能力与运维化（开放窗口）

- CLI 产品（`docs/需求/25 cli.md`）。
- Knowledge 多租户隔离 + 知识图谱 L4。
- A2A 注册中心 / Discover 流程。
- Evaluation 可视化平台 + A/B。
- pkg/trpc-agent-go 版本治理：每月一次同步评估；适配层版本兼容测试。
- 文档站点化（mkdocs / docusaurus 等），废弃零散 `.md` 维护方式。

---

## 5. 立即可执行 Top-20 工作清单

> 每条都对应一个 PR 级别动作，已带证据指针；AI 接到任务时直接读取对应文件作为起点。

| # | ID | 动作 | 主要证据 / 起点 |
|---|---|---|---|
| 1 | EP-DOC-01 | 用本计划替代 task-tracker，并把 master-plan §4 + plan §2 状态表改为"参见 docs/guides/execution-plan.md 附录 A" | `docs/guides/master-plan.md`、`plan.md`、`task-tracker.md` |
| 2 | EP-SEC-01 | JWT 密钥 fail-fast | `pkg/auth/config.go:8-14` |
| 3 | EP-SEC-02 | Bypass 限定 dev，启动 banner | `pkg/auth/middleware.go:15-66`、`features.go` |
| 4 | EP-RT-01 | `runSingleAgentViaTRPC` / `runTeamTRPC` 起止处调用 `setRunStatus` | `internal/service/chat.go:332-388` + `internal/service/trpc_turn.go` |
| 5 | EP-RT-02 | await_user_reply 工具触发时 `awaitChans.Store`；AwaitUserReply 处 select 消费 | `internal/service/chat.go:332-388` + 工具回调 |
| 6 | EP-OBS-01 | `NewHTTPServer` 末尾挂 `/metrics`；同时在 ChatService、Bus、Tool 关键点采样 | `internal/server/metrics.go` + `internal/server/http.go` |
| 7 | EP-BIZ-01 | `buildSkillDeps` 增加 backend selector：DB（默认）/ FS（dev） | `internal/agent/trpc_build.go:160`、`internal/skill/trpc/db_repository.go` |
| 8 | EP-BIZ-03 | 在 `internal/tools/toolset.go:Registry()` 注册 knowledge_search 与 call_agent；`AssemblyConfig` 增加开关 | `internal/tools/knowledge/tool.go`、`internal/a2a/tool.go` |
| 9 | EP-RT-03 | `auto_memory.go` `extract` 调用 `pkg/trpc-agent-go/memory/extractor`，写入 `session_memory` 表 | `internal/cronrunner/jobs/auto_memory.go` |
| 10 | EP-BIZ-04 | `wire_providers.go` 给 Evaluation Runner 注入真实 Agent + Judge | `internal/service/wire_providers.go` |
| 11 | EP-RT-06 | WS subscribeOptions 按事件类型设 Reliable / DropPolicy | `internal/server/ws.go` |
| 12 | EP-FE-01 | 重写 SessionTurnsPanel / SessionTimelinePanel / Tools / Monitor 系列展示组件，使用 store | `web/src/components/sessions/*.vue` 等 |
| 13 | EP-FE-02 | 删除 `wsClient.ts` + `useWS.ts` 或将 chat ws-transport 重构到统一客户端 | `web/src/services/wsClient.ts` |
| 14 | EP-ENG-01 | CI `go test ./...` + race subset；保留覆盖率 60% 但口径全量 | `.github/workflows/ci.yml:47-57` |
| 15 | EP-ENG-02 | 去掉前端 `|| echo`；修稳 3 个 spec 后接通 | `.github/workflows/ci.yml:85-89` |
| 16 | EP-ENG-03 | 新增 `make wire`、`make wire-clean`；CI job 跑后 git diff 必空 | `Makefile` + 新 CI job |
| 17 | EP-ENG-04 | 新增 `proto-clean` job；`make api` 后 git diff 必空 | `.github/workflows/ci.yml` + Makefile |
| 18 | EP-OBS-02 | OTel：Kratos tracing.Server middleware + OTLP；环境变量 `OTEL_EXPORTER_OTLP_ENDPOINT` | `internal/server/http.go`、`grpc.go` + new `internal/server/telemetry.go` |
| 19 | EP-RULE-04 | `pkg/safego.Go` 替代所有 `go func()`；araneactl lint 加规则 | grep `go func\(` 全仓 |
| 20 | EP-DOC-04/05/07 | 更新 `27 artifact.md` `37 knowledge.md` `22 plugin.md` `34 event-system.md` `51` 系列现状段 | 对应需求文件 |

---

## 6. 红线（在 AI-DEV-SPEC 基础上扩展）

> 这里把 AI-DEV-SPEC §"红线（违反即停）" 中 12 条原文保留，本节追加 6 条 / 强化 1 条，全部由 `cmd/araneactl/lint` 在 CI 强制。

| # | 红线 | 检查方式 |
|---|---|---|
| 既有 R1-R12 | 参见 `docs/guides/AI-DEVELOPMENT-SPECIFICATION.md` 速查卡 | araneactl lint |
| **R13**（新增） | 生产构建中禁止启用 `KRATOS_HTTP_AUTH_DISABLED`；`KRATOS_AUTH_SECRET` 必须为强密钥 | startup fail-fast + ci grep |
| **R14**（新增） | 所有 `go func()` 必须用 `pkg/safego.Go` 包装，禁止裸 goroutine | araneactl lint |
| **R15**（新增） | 任何写操作必须能通过 workspace context 隔离；`internal/data/*` 写路径在 M2 后必须经 Ent Hook 注入 `workspace_id` | araneactl lint + ent hook |
| **R16**（新增） | 进度信息只允许写在 `docs/guides/execution-plan.md` 附录 A；其它文档引用本表 | docs-check 子命令 |
| **R17**（新增） | `wire_gen.go` 与 `wire.go` 必须一致；CI 每个 PR 跑 `make wire-clean` | CI job |
| **R18**（新增） | `*.pb.go` / `*.ts` 生成物必须与 proto 一致；CI 跑 `make api` 后 git diff 必空 | CI job |
| **R10 强化** | `sql.Open` 唯一例外仍是 `data.go`；新增 Postgres 池也走 `Data` 字段；任何子包不得自行 `sql.Open` | araneactl lint |

---

## 7. AI 协作迭代约束

> 给在 Cursor 中工作的 AI agent 的硬约束。每个 PR 都要按本节自查。

### 7.1 接到任务时

1. 读本文件 §1.3 + §3 找到任务对应 EP 编号；如果新任务，登记到 §3 表格。
2. 读 `docs/README.md`、`AI-DEVELOPMENT-SPECIFICATION.md` 速查卡。
3. 如果任务涉及 trpc-agent-go 框架，再读 `trpc-agent-go-framework.md` 对应章节。
4. 如果任务涉及业务域，读对应 `docs/需求/<编号>.md` + `.design.md`，并对照本计划 §1.3 看现状。
5. 写一个 ≤ 5 条的"假设清单"，列出"任务依赖的现状假设"，先 grep 验证再写代码。

### 7.2 写代码时

1. 决策树定位代码归属（AI-DEV-SPEC §速查卡）；跨层禁止。
2. 任何 `go func()` → `safego.Go`。
3. 任何跨层错误 → `kerrors` 或 `apierror`，禁止 `fmt.Errorf` 跨层。
4. 任何新 RPC → proto 先行；`make api` 生成；service `Unimplemented*` 嵌入；server 注册；wire 注入。
5. 任何写库操作 → 显式接受 / 验证 `workspace_id`（M2 完成后通过 Ent Hook 强制）。
6. 任何新事件类型 → 在 `internal/event/bus.go` 显式声明 reliable vs lossy；订阅侧明确 DropPolicy。
7. 任何新 metric → 在 `internal/server/metrics.go` 定义，且在生成点采样。

### 7.3 提交前

1. `make lint`（araneactl 红线 + go vet）通过。
2. `go test ./...` 通过；如改了 race-sensitive 代码，跑 `go test -race ./internal/<改动包>`。
3. `make wire-clean`（如改了 wire 输入）。
4. `make api` 后 git diff 空（如改了 proto）。
5. `pnpm build && npm test`（如改了 web）。
6. 更新本计划 §3 表格状态、附录 A 状态矩阵、相应需求/设计文档现状段。
7. `docs/changelog/<日期>-<topic>.md` 单文件 + 链接到本计划 EP 编号。

### 7.4 不应该做的事

- 新建零散的 sprint/plan 文档，绕过本执行计划。
- 在 changelog 中宣称"已完成"，但代码层未接入 wire/server/runtime。
- 大重构（>500 行）单 PR；必须拆分。
- 在 `pkg/trpc-agent-go` 中改动；任何需求都走 `internal/*/trpc` 适配层。

---

## 8. 验收与质量门（机器可检查）

> CI 每条规则都映射到一个具体命令；失败即阻断合并。

| 门 | 命令 / 检查 | 期望 |
|---|---|---|
| 红线 | `go run ./cmd/araneactl/lint --root .` | 退出码 0 |
| 单元测试 | `go test -coverprofile=cov.out ./...` | 通过；M3 阈值 40%，M4 60%，M5 70% |
| race | `go test -race ./internal/event/... ./internal/graph/... ./internal/service/...` | 通过 |
| 编译 | `go build ./...` | 通过 |
| Wire 同步 | `go run -mod=mod github.com/google/wire/cmd/wire ./cmd/admin && git diff --exit-code` | 无 diff |
| Proto 同步 | `make api && git diff --exit-code` | 无 diff |
| 前端构建 | `npm --prefix web run build` | 通过 |
| 前端单测 | `npm --prefix web test` | 通过（不允许 `|| echo`） |
| stylelint（M3+） | `npm --prefix web run stylelint` | 通过 |
| docs 一致性 | `go run ./cmd/araneactl docs-check`（新增） | 进度真相只在本计划 |
| smoke（M2+ 强制） | `make smoke` | 启动 + chat + tool + memory + graph 零错 |

---

## 9. 风险与缓解

| 风险 | 触发条件 | 缓解 |
|---|---|---|
| 路线图与既有 sprint 文档双轨 | 旧文档继续被引用 | 把 master-plan / plan / task-tracker 顶部加 `已废弃，参见 execution-plan.md` |
| 安全 P0 修复影响开发体验 | 强制密钥与限定 bypass | dev 配置文件预置一个明确的 dev secret；启动文档说明 |
| 全量 `./...` 测试初期失败多 | 现有覆盖白名单只跑通的包 | 分阶段：先 allowlist 阻断不稳定包；逐包修绿 |
| Ent Hook 多租户改造影响所有 Repo | 一次性大改 | 按域分批：admin → agent → session → memory → tool，每域一 PR |
| OTel 增加进程依赖 | go.mod 体积 + 启动时间 | 仅在 `OTEL_EXPORTER_OTLP_ENDPOINT` 非空时初始化 |
| 前端展示组件重构涉及 ≥ 20 文件 | 一次性 PR 过大 | 按域分批：sessions → monitor → tools → chat → agents |
| upstream `pkg/trpc-agent-go` API 变 | 适配层频繁返工 | 锁定 go.mod；每个适配层 `_test.go` 覆盖核心 API |

---

## 10. AI 自检清单（开工前必读）

```
□ 我是否在 docs/guides/execution-plan.md §3 中找到了对应 EP 编号？
□ 我是否读了 AI-DEVELOPMENT-SPECIFICATION 红线速查与本计划 §6 扩展红线？
□ 我是否对照 §1.3 "真实缺失或半成品"确认了任务前置假设？
□ 我修改的代码是否落在决策树正确的层？
□ 我新增的 go func 是否使用 safego.Go？
□ 我新增的写库操作是否接受 / 校验 workspace_id？
□ 我新增的事件订阅是否明确 reliable vs lossy？
□ 我新增的 metric 是否在生成点 inc/observe？
□ 我新增的 RPC 是否走了 proto-first + make api + wire 流程？
□ 我提交时是否同时更新本计划 §3 表格、附录 A 矩阵、相应需求文档现状段？
□ 我是否避免在 changelog 中宣称未端到端接通的"完成"？
```

---

## 11. 文档治理

| 文档 | 用途 | 维护策略 |
|---|---|---|
| `docs/guides/execution-plan.md`（本文） | 唯一进度真相 + AI 协作约束 | 每个 PR 同步 §3 / 附录 A；季度 review |
| `docs/guides/AI-DEVELOPMENT-SPECIFICATION.md` | 规范红线（不变层） | 仅在 §6 红线变化时改 |
| `docs/guides/trpc-agent-go-framework.md` | 框架工程化解读 | 框架升级时同步 |
| `docs/需求/<n> <模块>.md` | 纯需求（用户故事 / 验收） | 需求变更时改 |
| `docs/需求/<n> <模块>.design.md` | 纯设计（接口 / 数据模型 / 选型） | 设计调整时改 |
| `docs/changelog/<date>-<topic>.md` | 变更摘要 + EP 引用 | 每个 PR 1 篇 |
| `docs/devlog/<date>-<topic>.md` | 实施过程 / 调试 / 走查记录 | 自由 |
| `docs/guides/master-plan.md` | **已废弃**，保留作历史；顶部需加废弃声明 | 不维护 |
| `docs/guides/plan.md` | **已废弃**，保留作历史；顶部需加废弃声明 | 不维护 |
| `docs/guides/implementation-plan.md` | **已废弃**，保留作历史 | 不维护 |
| `docs/guides/task-tracker.md` | **已废弃**，保留作历史 | 不维护 |
| `docs/guides/sprints/*` | **已废弃**，保留作历史 | 不维护 |
| `docs/devlog/2026-05-17-optimization-code-audit.md` | 旧审计快照 | 附录引用，不再更新 |

> 文档治理动作 EP-DOC-01 完成后，废弃文档须加顶部声明：
>
> ```
> > 本文档自 2026-05-17 起停止维护，进度信息以 docs/guides/execution-plan.md 为准。
> ```

---

## 附录 A：模块状态矩阵（唯一进度真相）

> 状态枚举：✅ 完成 / 🟡 部分 / ❌ 未实现 / ⛔ 已实现但未接入主链路 / 📄 仅文档
>
> 列含义：
> - Proto：API 契约是否完整
> - Biz：业务模型 + Usecase + Repo 接口
> - Data：Repo 实现 / Schema
> - Service：Kratos service 注册（含 `Unimplemented*` 嵌入 + ProviderSet）
> - Server：HTTP + gRPC 路由注册
> - Runtime：进入 Agent/Team Runner 或后台 worker 的调用链
> - 前端：features/api + store + page 闭环
> - 风险/EP：本计划 §3 中关联编号

| 模块 | Proto | Biz | Data | Service | Server | Runtime | 前端 | 关联 EP |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|---|
| Admin / Auth | ✅ | ✅ | ✅ | ✅ | ✅ | n/a | ✅ | EP-SEC-01..04 |
| Avatar | ✅ | ✅ | ✅ | ✅ | ✅ | n/a | ✅ | — |
| Agent / RuntimeSettings | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | EP-RT-05 |
| AgentCategory | ✅ | ✅ | ✅ | ✅ | ✅ | n/a | ✅ | — |
| AgentPromptFile | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| AgentEvolution | ✅ | 🟡 | 🟡 | ✅ | ✅ | 🟡 | ✅ | EP-BIZ-07 |
| LlmProviderModel | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Session（CRUD/Turns/Restore/Archive/压缩） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Chat（SendMessage/Stream/Cancel） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Chat RunStatus / AwaitUserReply | ✅ | n/a | n/a | 🟡 | ✅ | ❌ | 🟡 | EP-RT-01, EP-RT-02 |
| Team（5 种模式 + transfer） | ✅ | ✅ | ✅ | ✅ | ✅ | 🟡 | ✅ | EP-RT-04 |
| Tool 基础 / Invocation / Override | ✅ | 🟡 | ✅ | ✅ | ✅ | ✅ | ✅ | EP-BIZ-06 |
| Skill 运行时（FS） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Skill DB Repo 适配 | n/a | ✅ | ✅ | n/a | n/a | ⛔ | n/a | EP-BIZ-01 |
| MCP Server | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Channel（Feishu） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | EP-BIZ-05 |
| Channel（其他渠道） | 📄 | 🟡 | 🟡 | 🟡 | 🟡 | ❌ | 🟡 | EP-BIZ-05 |
| Cron（CRUD + Runner + 重试 + DLQ） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | EP-RT-07 |
| Plugin（CRUD + Runtime） | ✅ | ✅ | ✅ | ✅ | ✅ | 🟡 | ✅ | EP-RT-07 |
| Memory 基础（L0-L4 表） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Memory Auto Extract | n/a | 🟡 | 🟡 | n/a | n/a | ❌ | n/a | EP-RT-03 |
| Memory Tools（5 件套） | n/a | n/a | n/a | n/a | n/a | 🟡 | n/a | EP-RT-05 |
| Knowledge | ✅ | ✅ | ✅ | ✅ | ❌ | ⛔ | 🟡 | EP-BIZ-03 |
| Artifact REST | ✅ | ✅ | ✅ | ✅ | ❌ | ⛔ | 🟡 | EP-BIZ-03（间接）|
| A2A | ✅ | ✅ | ✅ | ✅ | ✅ | ⛔ | 🟡 | EP-BIZ-03 |
| Evaluation | ✅ | ✅ | ✅ | ✅ | ❌ | ⛔ | 🟡 | EP-BIZ-04 |
| CodeExecutor（Local） | n/a | n/a | n/a | n/a | n/a | ✅ | n/a | — |
| CodeExecutor（Docker） | n/a | n/a | n/a | n/a | n/a | ⛔ | n/a | EP-BIZ-02 |
| Graph 工作流 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Monitor / Usage / SystemSetting | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Event Bus | n/a | n/a | n/a | n/a | n/a | ✅ | 🟡 | EP-RT-06, EP-FE-02 |
| WebSocket Gateway | n/a | n/a | n/a | n/a | ✅ | ✅ | ✅ | EP-OBS-03 |
| Metrics / OTel | n/a | n/a | n/a | n/a | ⛔ | ❌ | n/a | EP-OBS-01, EP-OBS-02 |
| Workspace 多租户 | n/a | 🟡 | ❌ | n/a | 🟡 | n/a | 🟡 | EP-SEC-03, M2 |
| Audit Log | n/a | ❌ | ❌ | n/a | n/a | n/a | n/a | M2 |
| CLI 产品 | n/a | n/a | n/a | n/a | n/a | n/a | n/a | M5 |

---

## 附录 B：相关文档索引

- 权威规范：`docs/guides/AI-DEVELOPMENT-SPECIFICATION.md`
- 框架解读：`docs/guides/trpc-agent-go-framework.md`
- 系统架构：`docs/需求/0 系统框图.md`
- 需求合集：`docs/需求/*`
- 旧规划（已废弃）：`docs/guides/master-plan.md`、`plan.md`、`implementation-plan.md`、`task-tracker.md`、`sprints/*`
- 历史审计：`docs/devlog/2026-05-17-optimization-code-audit.md`（结论已被本计划 §1 修正）
- 变更记录：`docs/changelog/*`
