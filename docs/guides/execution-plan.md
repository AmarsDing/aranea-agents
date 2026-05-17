# Aranea-Agents 执行计划（AI 协作权威基线）

> 版本：v1.0（2026-05-17）
> 编制依据：`docs/README.md`、`docs/需求/*`（86 篇）、`docs/guides/{AI-DEVELOPMENT-SPECIFICATION, trpc-agent-go-framework}.md`、`docs/changelog/*`、`docs/devlog/*`，以及 `cmd/ internal/ api/ pkg/ web/` 全量代码与配置抽样。
> 文档定位：**给后续 AI 迭代使用的唯一执行基线**。任何 PR / commit 之前，AI 必须按本文 §10 自检。
---

## 0. 阅读路径（先读这一页）

- §1 现状全景：六轴（架构、业务、数据、运行时、前端、可观测）当前状态
- §2 文档与代码协同失控（已治理）：元问题回顾与后续约束
- §3 全域漏洞与不足清单：分类 + 证据 + 优先级
- §4 长效优化路线（M0–M5 里程碑）
- §5 立即可执行剩余工作（每项可一 PR 落地）
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

### 1.2 仍然真实缺失或半成品

| 主题 | 现实状态 | 关键证据 |
|---|---|---|
| CodeExecutor Docker | `internal/agent/codeexecutor/executor.go` 存在但仅在测试中引用，Skill 仍走 `NewLocalExecutor` | `internal/agent/trpc_build.go` |
| Callback Chain | `internal/agent/callbacks/` 抽象存在但未挂载到 LLMAgent；仅 ToolCallback 接通 | `internal/agent/trpc_build.go:buildToolCallbacks` |
| 多租户 Ent hook | 没有 Ent Hook 强制 `workspace_id` 谓词，`servermw.AssertWorkspace` 定义后从未被调用 | `internal/server/middleware/workspace.go` |
| EvolutionScanner / L4 持久层 | `RunEvolutionScan` / 30min ticker 代码内不存在（需求文档已校准为"未实现"） | `internal/` 无对应 worker |
| 51 消息机制"单 WS 通道" | 文档要求 WS 取代 SSE 为唯一通道；代码里 WS + Chat SSE 并存（文档已统一为"WS 主通道 + Chat SSE 兼容回退"） | `internal/server/ws.go` + `internal/service/chat.go` |
| CLI 产品 | `docs/需求/25 cli.md` 要求 `aranea` 可执行 CLI / REPL；实际只有 `cmd/admin`（服务进程）+ `cmd/araneactl/lint`（lint 工具） | `cmd/` 目录 |

## 3. 全域漏洞与不足清单（按优先级）

> 命名规则：`EP-<域>-<序号>`；优先级 P0=阻塞 / 安全 / 数据正确性；P1=主路径功能；P2=可用性 / 完整性；P3=长期演进。

### 3.2 数据一致性 / 运行时正确性（P1/P2）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-RT-07 | Cron 派发链路不走 Agent Runner 构造函数，Plugin 回调对 Cron 任务可能失效 | P1 | `internal/cronrunner` 不 import plugin runtime | Cron 走统一的 `ChatService.RunNativeTurnUnary`，或在 worker 内复用 plugins |
| EP-RT-08 | 内存 repo 仅服务于测试，未对应生产实现（Artifact / Knowledge / Eval / A2A） | P2 | `internal/biz/biz_coverage_test.go`、`s6_coverage_test.go` | 把内存实现升级为真实存储后端或明确标"骨架未接线" |

### 3.3 业务接入闭环（P1/P2）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-BIZ-02 | CodeExecutor Docker 已实现但只在测试引用 | P1 | `internal/agent/codeexecutor/executor.go` | Skill 执行器加 backend selector，配置驱动；提供 Docker compose 示例 |
| EP-BIZ-05 | Channel 多渠道适配缺失（仅 feishu 真正走端到端） | P2 | `internal/service/channel_ingress.go` + `internal/channel/lark` | 实现 wechat / dingtalk / slack / 邮件入站，或在前端禁用未实现渠道 |
| EP-BIZ-06 | ToolOverride 域只在 proto / 统计语境出现，无 CRUD / Usecase | P2 | proto agent_override_count 字段 + 缺少 biz/data | 补 `biz/tool_override.go` 与 Repo |
| EP-BIZ-07 | EvolutionScanner（L4 持久层）代码不存在 | P2 | `internal/` 无对应 worker（需求文档已校准为"未实现"） | 要么实现 30min ticker scanner，要么保持需求降级为"未实现" |

### 3.5 前端（P2/P3）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-FE-03 | 硬编码颜色违反 token | P2 | `components/sessions/SessionTimelinePanel.vue:173-211`、`components/chat/ChatSessionSidebar.vue:320-359` 等 | stylelint 加规则；批量重构使用 `var(--*)` |
| EP-FE-05 | `heartbeat` 域无 `api.ts` 与生成客户端，仅 composable | P3 | `web/src/features/heartbeat/useServerHeartbeat.ts` | 补 api 层或合并到 system-settings |
| EP-FE-06 | 设计 token：Quasar Material 色（如 `#4caf50`/`#f44336`）出现在 Graph 节点等组件 | P3 | `components/graph/GraphFlowNode.vue` | 用 token 体系替换 |

### 3.8 红线/规范缺口（P2/P3）

| 编号 | 问题 | 优先级 | 证据 | 建议动作 |
|---|---|---|---|---|
| EP-RULE-01 | `pkg/apierror` 实际几乎无人使用，service 层主要用 `kerrors` | P2 | grep `apierror.` 在 `internal/*` 仅 workspace middleware 与测试 | 二选一：扩大 apierror 使用面，或者只在 middleware/边界保留并标注；编辑 §6 红线 |
| EP-RULE-03 | `cmd/araneactl/lint` R3 被刻意跳过，data → biz 接口依赖是否允许未文档化 | P3 | `cmd/araneactl/lint/main.go:85-109` | 在 AI-DEV-SPEC §1.3 / 本计划 §6 写明 |

---

## 4. 长效优化路线（M0–M5）

> 与原 master-plan S1-S6 相比，本路线按"产品里程碑 + 能力闭环"组织，而不是按"红线消除 → 架构债 → 功能补全"的工程顺序；每个里程碑都包含安全、可观测、测试、文档四条平行子线。

### M0 真相同步与基础闭合（1 周）✅

目标：把"文档真相"与"代码真相"对齐，把 P0 安全与运行时正确性补足。
- EP-SEC-01 / EP-SEC-02：JWT 密钥 fail-fast + dev bypass 限定；启动 banner。✅
- EP-RT-01 / EP-RT-02：RunStatus / AwaitUserReply 真正落 store，前端 `useRunStatus` 可见。✅
- EP-OBS-01：暴露 `/metrics`（先 expose，再补点）。✅
- 验收：§3 中 P0 项（EP-SEC-01、EP-SEC-02、EP-RT-01、EP-RT-02）全部消除；`go build ./cmd/admin` + `go test ./...` + `npm test` 全过；`/metrics` 可访问。

### M1 端到端闭合（2 周）

目标：让 changelog 已宣称完成的能力真正接通。

- **EP-BIZ-02** CodeExecutor Docker backend selector。
- **EP-RT-07** Cron 走统一 Runner 入口（或显式插件复用）。
- EP-OBS-02 OTel 接入 Server middleware + OTLP exporter（可仅 dev）。✅
- 验收：`docs/changelog/2026-05-17-S{4,5,6}-*.md` 中每一项 ✅ 都能由附录 A "Runtime 接入"列证明。

### M2 多租户与安全收口（2 週）

- 多租户：在 Ent 添加 Hook 强制 `workspace_id` 谓词；在 biz 层把 `workspace.FromContext` 注入所有查询；`AssertWorkspace` 在写操作中强制执行。
- 审计：所有写操作落 audit_log 表（带 workspace、user、action、subject_id、diff hash）。
- 验收：CI 加 `make tenant-check`（grep + AST）；smoke 用两个 workspace 验证彼此不可见。

### M3 可观测与体验（2 週）

- **EP-FE-03** / **EP-FE-06**：硬编码颜色 → token；stylelint 兜底。
- 51 消息机制：明确"WS 主通道，Chat SSE 仅兼容回退"，统一文档与代码。

### M4 测试矩阵与 CI 真实化（2 週）

- Go line coverage 阶梯：M3=40% → M4=60% → M5=70%。

### M5 长期能力与运维化（开放窗口）

- CLI 产品（`docs/需求/25 cli.md`）。
- Knowledge 多租户隔离（先单用户） + 知识图谱 L4。
- A2A 注册中心 / Discover 流程。
- Evaluation 可视化平台 + A/B。
- pkg/trpc-agent-go 版本治理：每月一次同步评估；适配层版本兼容测试。
- 文档站点化（mkdocs / docusaurus 等），废弃零散 `.md` 维护方式。

---

## 5. 剩余可执行工作（每项可一 PR 落地）

> 2026-05-17 已批量完成 EP-SEC-01/02/03/04、EP-RT-01/02/03/04/05/06、EP-BIZ-01/03/04、EP-OBS-01/02/03/04/05、EP-FE-01/02/04、EP-ENG-01/02/03/04/05/07、EP-RULE-02/04，共 26 项。
> 2026-05-17 二批完成 EP-RT-07、EP-BIZ-02/05/07、EP-FE-03/05/06、EP-RULE-01/03，共 9 项。以下为**仍未完成**的工作：

| ID | 动作 | 优先级 | 主要证据 / 起点 |
|---|---|---|---|
| EP-RT-08 | Artifact / Knowledge / Eval / A2A 生产实现；内存 repo 降为 test-only | P2 | `internal/biz/*_coverage_test.go` |
| EP-BIZ-06 | 补 `biz/tool_override.go` + Repo + CRUD service + 前端页面 | P2 | proto agent_override_count 字段 |

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
| **R-FE1**（新增） | `web/src/components/**/*.vue` 中不得 runtime import `features/*/api`；type-only import 豁免 | araneactl lint R-FE1 |
| **R10 强化** | `sql.Open` 唯一例外仍是 `data.go`；新增 Postgres 池也走 `Data` 字段；任何子包不得自行 `sql.Open` | araneactl lint |
| **R-ERR**（EP-RULE-01） | `pkg/apierror` 仅在 HTTP/gRPC 边界（middleware、server handler）使用；biz/service 层统一使用 `github.com/go-kratos/kratos/v2/errors`（`kerrors`）；禁止 `fmt.Errorf` 跨层传递错误类型 | araneactl lint R-ERR |
| **R-LAYER**（EP-RULE-03） | `data` 层只能 import `biz` 的 Repo **接口**（`biz.XxxRepo`）和 domain model，禁止 import `biz.XxxUsecase`；araneactl lint R3 已刻意放行 `data → biz interface` 依赖，但不允许反向 `biz → data` | araneactl lint R3（已放行）|

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
7. 任何新 metric → 在 `internal/metrics/vars.go` 定义，且在生成点采样。

### 7.3 提交前

1. `make lint`（araneactl 红线 + go vet + fmtcheck）通过。
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
- 修改工具生成的代码

---

## 8. 验收与质量门（机器可检查）

> CI 每条规则都映射到一个具体命令；失败即阻断合并。

| 门 | 命令 / 检查 | 期望 |
|---|---|---|
| 红线 | `go run ./cmd/araneactl/lint --root .` | 退出码 0 |
| 格式检查 | `go run ./cmd/araneactl/fmtcheck --root .` | 退出码 0 |
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
| 路线图与既有 sprint 文档双轨 | 旧文档继续被引用 | 已迁移至 `_deprecated/guides/`；顶部加废弃声明 |
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
| `docs/需求/<n> <模块>.md` | 需求 + 运维指南（§6/§7） | 需求变更或运维要点变更时改 |
| `docs/需求/<n> <模块>.design.md` | 纯设计（接口 / 数据模型 / 选型） | 设计调整时改 |
| `docs/changelog/README.md` | 变更记录索引 | 新增 changelog 时同步 |
| `docs/changelog/<date>-<topic>.md` | 变更摘要 + EP 引用 | 每个 PR 1 篇 |
| `docs/devlog/README.md` | 开发日志索引 | 新增 devlog 时同步 |
| `docs/devlog/<date>-<topic>.md` | 实施过程 / 调试 / 走查记录 | 自由 |
| `docs/devlog/2026-05-17-optimization-code-audit.md` | 旧审计快照 | 附录引用，不再更新 |
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
| Admin / Auth | ✅ | ✅ | ✅ | ✅ | ✅ | n/a | ✅ | EP-SEC-01 ✅, EP-SEC-02 ✅, EP-SEC-03 ✅, EP-SEC-04 ✅ |
| Avatar | ✅ | ✅ | ✅ | ✅ | ✅ | n/a | ✅ | — |
| Agent / RuntimeSettings | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | EP-RT-05 ✅ |
| AgentCategory | ✅ | ✅ | ✅ | ✅ | ✅ | n/a | ✅ | — |
| AgentPromptFile | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| AgentEvolution | ✅ | 🟡 | 🟡 | ✅ | ✅ | 🟡 | ✅ | EP-BIZ-07 ✅（降级为未实现；EvolutionScanner 需求已标注 N/A）|
| LlmProviderModel | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Session（CRUD/Turns/Restore/Archive/压缩） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Chat（SendMessage/Stream/Cancel） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Chat RunStatus / AwaitUserReply | ✅ | n/a | n/a | ✅ | ✅ | ✅ | 🟡 | EP-RT-01 ✅, EP-RT-02 ✅ |
| Team（5 种模式 + transfer） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | EP-RT-04 ✅ |
| Tool 基础 / Invocation / Override | ✅ | 🟡 | ✅ | ✅ | ✅ | ✅ | ✅ | EP-BIZ-06 |
| Skill 运行时（FS） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Skill DB Repo 适配 | n/a | ✅ | ✅ | n/a | n/a | ✅ | n/a | EP-BIZ-01 ✅ |
| MCP Server | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Channel（Feishu） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | EP-BIZ-05 |
| Channel（其他渠道） | 📄 | 🟡 | 🟡 | 🟡 | 🟡 | ❌ | ✅ | EP-BIZ-05 ✅（前端禁用未实现渠道按钮）|
| Cron（CRUD + Runner + 重试 + DLQ） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | EP-RT-07 ✅ |
| Plugin（CRUD + Runtime） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | EP-RT-07 ✅ |
| Memory 基础（L0-L4 表） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Memory Auto Extract | n/a | ✅ | ✅ | n/a | n/a | ✅ | n/a | EP-RT-03 ✅ |
| Memory Tools（5 件套） | n/a | n/a | n/a | n/a | n/a | 🟡 | n/a | EP-RT-05 ✅ |
| Knowledge | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | 🟡 | EP-BIZ-03 ✅（工具已注册）|
| Artifact REST | ✅ | ✅ | ✅ | ✅ | ❌ | ⛔ | 🟡 | EP-BIZ-03（间接）|
| A2A | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | 🟡 | EP-BIZ-03 ✅（call_agent 工具已注册）|
| Evaluation | ✅ | ✅ | ✅ | ✅ | ❌ | 🟡 | 🟡 | EP-BIZ-04 ✅ |
| CodeExecutor（Local） | n/a | n/a | n/a | n/a | n/a | ✅ | n/a | — |
| CodeExecutor（Docker） | n/a | n/a | n/a | n/a | n/a | ✅ | n/a | EP-BIZ-02 ✅ |
| Graph 工作流 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Monitor / Usage / SystemSetting | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| Event Bus | n/a | n/a | n/a | n/a | n/a | ✅ | ✅ | EP-RT-06 ✅, EP-FE-02 ✅ |
| WebSocket Gateway | n/a | n/a | n/a | n/a | ✅ | ✅ | ✅ | EP-OBS-03 ✅ |
| Metrics / OTel | n/a | n/a | n/a | n/a | ✅ | ✅ | n/a | EP-OBS-01 ✅, EP-OBS-02 ✅, EP-OBS-04 ✅, EP-OBS-05 ✅ |
| Workspace 多租户 | n/a | 🟡 | ❌ | n/a | 🟡 | n/a | 🟡 | M2 |
| Audit Log | n/a | ❌ | ❌ | n/a | n/a | n/a | n/a | M2 |
| CLI 产品 | n/a | n/a | n/a | n/a | n/a | n/a | n/a | M5 |

---

## 附录 B：相关文档索引

- 权威规范：`docs/guides/AI-DEVELOPMENT-SPECIFICATION.md`
- 框架解读：`docs/guides/trpc-agent-go-framework.md`
- 系统架构：`docs/需求/0 系统框图.md`
- 需求合集：`docs/需求/*`（含运维指南 §6/§7）
- 变更记录索引：`docs/changelog/README.md`
- 开发日志索引：`docs/devlog/README.md`
- 历史审计：`docs/devlog/2026-05-17-optimization-code-audit.md`（结论已被本计划 §1 修正）
