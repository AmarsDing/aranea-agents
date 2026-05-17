# Aranea-Agents 任务追踪表

> **⚠️ 本文档自 2026-05-17 起停止维护**。表中显示的"0/41 done"与 `docs/changelog/2026-05-17-S{1..6}-*.md` 宣称"已完成"严重不一致，且已与代码层实际接入度脱钩。
>
> 新的进度真相、模块接入度矩阵与立即可执行清单，请参见 [`docs/guides/execution-plan.md`](execution-plan.md) §3 / §5 / 附录 A。
>
> 仅保留作历史参考。
>
> ---
>
> 用途（原）：S1~S6 全部 41 个任务的统一勾选表。每周站会更新本表，每个 PR 合并时把对应任务 `status` 改为 `done` 并粘贴 PR 链接。
> 配套：[docs/guides/implementation-plan.md](implementation-plan.md) · [sprints/](sprints/)
> 状态枚举：`pending`（待启动） · `in-progress`（进行中） · `review`（PR review 中） · `done`（已合并） · `blocked`（受阻） · `cancelled`（取消）

---

## 1. 总览状态

| Sprint | 任务范围 | 完成数 / 总数 | 状态 |
|--------|----------|----------------|------|
| S1 | T1~T13 | 0 / 13 | pending |
| S2 | T14~T20 | 0 / 7 | pending |
| S3 | T21~T28 | 0 / 8 | pending |
| S4 | T29~T33 | 0 / 5 | pending |
| S5 | T34~T37 | 0 / 4 | pending |
| S6 | T38~T41 | 0 / 4 | pending |
| **合计** | T1~T41 | **0 / 41** | **0%** |

> 进度更新规则：完成 1 个任务（PR 合并 + 验收点全过）记 1；每周三/周五前更新本表。

---

## 2. 任务清单（全量）

> 列说明：`PR` 列写 GitHub PR 链接（`#123`）或本地短链；`Verified` 写验收通过的日期；`Notes` 写阻塞 / 备注。

### S1 — P0 红线 + 数据正确性（[详情](sprints/S1-p0-redlines.md)）

| ID | 任务 | Owner | Status | PR | Verified | Notes |
|----|------|-------|--------|----|----------|-------|
| T1 | 单 SQLite 连接池（Data.RawDB） | — | pending | — | — | 红线 10 / V-4 |
| T2 | WS 接入 Kratos | — | pending | — | — | 红线 6+12 / V-2 V-5 |
| T3 | biz 去框架（envelope→DomainEvent + projection） | — | pending | — | — | 红线 2 / V-1 |
| T4 | biz 去框架（graph→GraphRuntime） | — | pending | — | — | 红线 2+8 / V-1 V-3 |
| T5 | Memory cache 修复（删除 in-process cache） | — | pending | — | — | B-1 |
| T6 | Graph builder race 修复 | — | pending | — | — | B-5 |
| T7 | Graph executions GC + checkpoint 恢复 | — | pending | — | — | B-4 |
| T8 | EventBus 关键事件可靠投递（紧急修复） | — | pending | — | — | B-8 |
| T9 | go func panic recover（pkg/safego） | — | pending | — | — | B-9 / B-10 |
| T10 | loadEffectiveToolKeys ctx 修复 | — | pending | — | — | B-11 |
| T11 | 前端 graph TS 客户端生成 | — | pending | — | — | M-13 |
| T12 | 前端 chat 用生成客户端 | — | pending | — | — | M-14 |
| T13 | 文档同步（plan/README/spec） | — | pending | — | — | D-1~D-9 |

### S2 — P1 架构债（[详情](sprints/S2-architecture-debt.md)）

| ID | 任务 | Owner | Status | PR | Verified | Notes |
|----|------|-------|--------|----|----------|-------|
| T14 | internal/runtime 抽象（取代 runtimedeps） | — | pending | — | — | 大 PR；建议 PR9a/9b |
| T15 | EventBus 完整背压（SubscribeOption） | — | pending | — | — | 替换 S1 T8 临时方案 |
| T16 | Agent 构建 LRU 缓存 | — | pending | — | — | Q-1 |
| T17 | 前端 Pinia store 17 个域补齐 | — | pending | — | — | M-15 |
| T18 | 前端 axios + WS 升级 | — | pending | — | — | F-4 |
| T19 | 文件重命名（errros/legacy/_adk） | — | pending | — | — | R-3 R-5 R-6 |
| T20 | 删除冗余 + 提取公共 filter | — | pending | — | — | R-1 R-2 R-9 |

### S3 — P1 业务可观测 + 测试基线（[详情](sprints/S3-observability.md)）

| ID | 任务 | Owner | Status | PR | Verified | Notes |
|----|------|-------|--------|----|----------|-------|
| T21 | RunStatus / AwaitUserReply RPC | — | pending | — | — | M-10 |
| T22 | Callback Chain（Agent/Model/Tool/Plugin） | — | pending | — | — | M-12 |
| T23 | pkg/apierror 统一错误模型 | — | pending | — | — | Q-11 |
| T24 | Workspace middleware + Ent hook | — | pending | — | — | 多租户 |
| T25 | Metrics endpoint（Prometheus） | — | pending | — | — | 可观测 |
| T26 | 跨平台 lint 工具（araneactl） | — | pending | — | — | 替换 ps1 |
| T27 | CI workflows（GitHub Actions） | — | pending | — | — | 含 smoke / proto-clean / wire-clean |
| T28 | 测试覆盖率基线 30% | — | pending | — | — | service/biz/data |

### S4 — P2 功能补全（一）（[详情](sprints/S4-plugin-skill-planner.md)）

| ID | 任务 | Owner | Status | PR | Verified | Notes |
|----|------|-------|--------|----|----------|-------|
| T29 | Plugin 运行时（PluginManager + Callback Points） | — | pending | — | — | M-4；含 AuditLogPlugin sample |
| T30 | Skill repository trpc 适配 + reload | — | pending | — | — | M-3 |
| T31 | Planner 多策略（builtin/react/a2ui） | — | pending | — | — | M-5 |
| T32 | Memory 工具五件套 | — | pending | — | — | M-2 |
| T33 | EnqueueAutoMemoryJob + cron worker | — | pending | — | — | M-1 |

### S5 — P2 功能补全（二）+ 测试矩阵 60%（[详情](sprints/S5-artifact-cron-tests.md)）

| ID | 任务 | Owner | Status | PR | Verified | Notes |
|----|------|-------|--------|----|----------|-------|
| T34 | Artifact 最小实现（proto + storage + UI） | — | pending | — | — | M-6 |
| T35 | Cron 失败重试 + metrics + dead-letter | — | pending | — | — | B-14 / M-18 |
| T36 | AgentRuntimeSettings 拆 8 sub-struct | — | pending | — | — | Q-22 |
| T37 | 测试矩阵 60% + cypress nightly e2e | — | pending | — | — | M-17 升级 |

### S6 — P3 长期能力（[详情](sprints/S6-knowledge-eval-a2a.md)）

| ID | 任务 | Owner | Status | PR | Verified | Notes |
|----|------|-------|--------|----|----------|-------|
| T38 | Knowledge（pgvector pipeline + 工具） | — | pending | — | — | M-7 |
| T39 | Evaluation（runner + 报告） | — | pending | — | — | M-11 |
| T40 | A2A（协议 + service + 工具） | — | pending | — | — | M-9 |
| T41 | CodeExecutor 沙箱（docker / firecracker） | — | pending | — | — | M-8 |

---

## 3. 风险 / 阻塞登记

> 任何被标 `blocked` 的任务必须同步登记原因与解除条件。

| 日期 | 任务 | 阻塞原因 | 解除条件 | 责任人 |
|------|------|-----------|-----------|--------|
| — | — | — | — | — |

---

## 4. PR 索引（按 Sprint）

| PR | Sprint / 任务 | Title | Status | Link |
|----|---------------|-------|--------|------|
| PR1 | S1 / T1 | data: introduce RawDB() and reuse pool for trpc adapters | — | — |
| PR2 | S1 / T2 | server: mount WebSocket through Kratos HTTP server | — | — |
| PR3 | S1 / T3+T4 | biz: decouple from trpc-agent-go via DomainEvent and GraphRuntime | — | — |
| PR4 | S1 / T5 | memory: drop in-memory cache, route reads to store | — | — |
| PR5 | S1 / T6+T7+T9+T10 | graph/agent: race-safe builder, executions GC, recover, ctx fix | — | — |
| PR6 | S1 / T8 | event: reliable delivery for critical envelope types | — | — |
| PR7 | S1 / T11+T12 | web: generate graph client and switch chat to typed client | — | — |
| PR8 | S1 / T13 | docs: sync plan/README/spec with current code state | — | — |
| PR9 | S2 / T14 | runtime: introduce RuntimeKernel and migrate service layer | — | — |
| PR10 | S2 / T15 | event: per-subscriber backpressure with drop policies | — | — |
| PR11 | S2 / T16 | agent: cache trpc LLMAgent builds by config hash | — | — |
| PR12 | S2 / T17+T18 | web: pinia stores for 17 domains + axios/ws upgrade | — | — |
| PR13 | S2 / T19+T20 | cleanup: rename legacy/_adk files and remove redundant helpers | — | — |
| PR14 | S3 / T21 | chat: expose RunStatus and AwaitUserReply RPCs | — | — |
| PR15 | S3 / T22 | agent: callback chain for agent/model/tool/plugin points | — | — |
| PR16 | S3 / T23 | apierror: unified error model for biz/data/service | — | — |
| PR17 | S3 / T24 | server: workspace middleware + ent hook | — | — |
| PR18 | S3 / T25+T26+T27 | obs: metrics endpoint, cross-platform lint, CI workflows | — | — |
| PR19 | S3 / T28 | tests: baseline coverage 30% for service/biz/data | — | — |
| PR20 | S4 / T29 | plugin: runtime + manager + AuditLogPlugin sample | — | — |
| PR21 | S4 / T30 | skill: trpc repository + db/file reload | — | — |
| PR22 | S4 / T31 | planner: builtin/react/a2ui selector | — | — |
| PR23 | S4 / T32 | tools/memory: add/update/load/search/delete | — | — |
| PR24 | S4 / T33 | cron: auto_memory job worker with retry | — | — |
| PR25 | S5 / T34 | artifact: minimum service + storage + UI | — | — |
| PR26 | S5 / T35 | cron: retry policy + metrics + dead-letter | — | — |
| PR27 | S5 / T36 | biz: split AgentRuntimeSettings into 8 sub-structs | — | — |
| PR28 | S5 / T37 | tests: coverage to 60% + cypress e2e nightly | — | — |
| PR29a | S6 / T38 | knowledge: pgvector pipeline + chunker + retriever + tool | — | — |
| PR29b | S6 / T38 | web: knowledge management UI | — | — |
| PR30a | S6 / T39 | evaluation: runner + metrics + async report | — | — |
| PR30b | S6 / T39 | web: evaluation dataset + report UI | — | — |
| PR31a | S6 / T40 | a2a: protocol + service + tool + audit | — | — |
| PR31b | S6 / T40 | web: a2a capability config UI | — | — |
| PR32a | S6 / T41 | codeexec: docker backend with resource limits | — | — |
| PR32b | S6 / T41 | codeexec: sandboxed backend (firecracker/nsjail) | — | — |

---

## 5. 周报模板

> 每周五（或 Sprint 内最后一个工作日）由 Tech Lead 在此粘贴一段周报，同步更新表格。
> 周报同时归档到 `docs/changelog/YYYY-MM-DD-weekly.md`。

### 模板

```markdown
## Week N (YYYY-MM-DD ~ YYYY-MM-DD)

### 本周完成
- [x] T{m} - <一句话总结> (PR#xx)
- [x] T{m}
- ...

### 本周进行中
- [ ] T{m} - <进展>，预计 D{n} 完成
- ...

### 本周阻塞
- T{m}：<阻塞原因> — 已记入 §3；预计 YYYY-MM-DD 解除

### 本周变更（计划外）
- <例：T{m} 拆为 T{m}.1 / T{m}.2；理由 ...>

### 下周计划
- T{m} / T{m+1}：<重点>
- 风险关注：<例：CI 派发；upstream API 变动观察>

### 度量快照
- 红线违反：<0>
- `go test -cover ./...`：<x%>
- `pnpm -C web build`：<pass/fail>
- smoke：<pass/fail>
```

### 历史周报

> 在此粘贴每周条目（最新在上）。

---

## 6. 收尾流程（Sprint 末必做）

每个 Sprint 结束日：

1. 本表 §1 总览状态 + 任务 Status 全量校对
2. 把本 Sprint 所有 PR 合并入 `docs/changelog/2026-MM-DD-S{n}-<topic>.md`
3. 更新 [docs/guides/master-plan.md](master-plan.md) §4 状态表（M-x 标记 ✅）
4. 在 [docs/guides/implementation-plan.md](implementation-plan.md) §6 索引把当前 Sprint 标记 `done` / 下个 Sprint `进行中`
5. 在 §5 历史周报区追加一份 Sprint Retro（含数据、亮点、改进）

---

## 7. 备注

- 本表是 **唯一权威进度源**。如出现 Sprint 文档与本表不一致，以本表 + 已合并 PR 为准，反推回 Sprint 文档修正。
- 任何"计划外"的临时任务，必须在 §4 PR 索引补一行 PR 号 + Sprint 归属 + 一段简短说明，确保审计链完整。
- 严禁出现 PR 已合并但任务仍 `pending` 的状态滞后；CI 每周检查表与 git log 一致性（S3 T27 落地后由 `araneactl tracker-check` 子命令自动校验）。
