# Memory 记忆 — 开发计划

> **版本**：2026-05-21 | **状态**：🟢 L0–L3 + 运行时双轨已落地；🟡 L4 / Worker / AutoMemory 为 MVP；❌ 完整 MemoryWorker / 级联审核 / L3 rerank  
> **需求**：[12 L0](./12%20memory-L0-sensory.md) · [13 L1](./13%20memory-L1-working.md) · [14 L2](./14%20memory-L2-episodic.md) · [15 L3](./15%20memory-L3-semantic.md) · [16 L4](./16%20memory-L4-persistent.md) · [38 知识体系](./38%20memory.md) · [UX](./12-16%20memory.md)  
> **设计**：[12-16 memory.design.md](./12-16%20memory.design.md)  
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · [0-system-development.md](./0-system-development.md) §8.6  
> **运行时边界**：[AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)

---

## 1. 模块定位

Agent 记忆系统：**五层产品模型（L0–L4）** + **trpc-agent-go `memory.Service` 适配轨** + **Aranea 管理/观测 API**。Runner 侧记忆走 `pkg/trpc-agent-go/memory`；产品层 L0–L4 走 `internal/data/sessionmemory` + `MemoryAdminUsecase`。

### 1.1 架构分层（Kratos × trpc-agent-go）

| 层级 | 职责 | 记忆相关包 |
|------|------|------------|
| `api/kratos/memory/v1` | 对外契约：L0/L1/L3/L4 查询与治理 | `memory.proto` |
| `internal/service` | proto ↔ biz；**不** import `pkg/trpc-agent-go` | `memory.go`、`session_compress.go` |
| `internal/biz` | 领域：`MemoryUsecase`（pgvector）、`MemoryAdminUsecase`、`L4GraphUsecase`、`RuntimeSet` | `memory.go`、`memory_admin_*.go`、`memory_l4*.go` |
| `internal/agent` | Runner 装配：`BuildTRPCLLMAgent`、`L4MemoryCue` | `trpc_build.go`、`l4_prompt.go` |
| `internal/memory/trpc` | 框架 `memory.Service` → SQLite | `sqlite_adapter.go` |
| `internal/data/sessionmemory` | L0–L4 表读写（`memory_chain.sql`）；**是 `MemoryAdminUsecase` 的 data 层实现，也是 `memory.Service` SQLite 适配器的底层存储**，非旁路或遗留代码 | `store*.go` |
| `pkg/trpc-agent-go/memory` | 框架真相源（接口语义） | 仅 agent/memory 桥接引用 |

**主从关系（已定稿）**：

- **Runner 运行时**：`biz.RuntimeSet.TRPC` = `trpcmem.NewSQLiteMemoryService(sessionmemory.Store)`（有 Store 时）。
- **产品观测 / 治理**：`biz.RuntimeSet.Admin` = `SessionAdminStore`；`MemoryService` gRPC 暴露 L0/L1/facts/entities 等。
- **可选第二轨**：`biz.MemoryUsecase` = Postgres `agent_memory` pgvector（`Remember`/`Recall`），**不**在默认 Chat Turn 自动挂载，与 SQLite 链并行。

**禁止**：`internal/service` / `internal/biz` 直接 import `pkg/trpc-agent-go`（红线 #2）；经 `internal/agent`、`internal/memory/trpc` 桥接。

### 1.2 代码锚点

| 能力 | 路径 |
|------|------|
| L0 压缩 | `internal/service/session_compress.go` → `SessionCompressor.AfterNativeTurn` |
| L0/L1/L3/L4 查询 API | `internal/service/memory.go` → `MemoryAdminUsecase` |
| L1–L4 存储 | `internal/data/sessionmemory` + `internal/data/sql/memory_chain.sql` |
| 框架 Memory | `internal/memory/trpc/sqlite_adapter.go` |
| L4 图写入/冲突元数据 | `internal/biz/memory_l4_usecase.go`、`internal/data/memory_l4.go` |
| L4 Prompt 注入 | `internal/agent/l4_prompt.go` |
| Turn 后提取调度 | `internal/biz/memory_worker.go` → `memtrpc.EnqueueAutoMemory` |
| 周期 AutoMemory | `internal/cronrunner/jobs/auto_memory.go`（启发式 + L4） |
| Agent 默认开关 | `internal/biz/agent_types.go` · `agent_runtime_settings` |

---

## 2. 现状评估（代码真相，2026-05-21）

| 项 | 状态 | 证据 |
|----|------|------|
| `memory.RuntimeSet` 端口 | ✅ | `internal/biz/memory_runtime_set.go`；`runtime.Persist.Memory` |
| L0 上下文压缩 | ✅ | `SessionCompressor`；`agent_runtime_settings` L0 字段 |
| L1 工作记忆（SQLite） | ✅ | `memory_l1_*` 表；Admin List API |
| L2/L3 事实与事件（SQLite） | ✅ | `memory_facts` / event entities；Admin API |
| L3 pgvector（Postgres） | 🟡 可选 | `biz.MemoryUsecase`；需配置 Postgres，非默认 Chat 路径 |
| L4 实体/关系存储 | ✅ | `memory_entities` / `memory_relations`；`L4GraphRepo` |
| L4 Prompt 注入 | ✅ | `L4MemoryCue` in `trpc_build.go` |
| L4 启发式写入 | 🟡 MVP | `L4GraphUsecase.WriteFromUserText`（regex）；`AutoMemoryWorker` |
| L4 冲突/衰减元数据 | 🟡 MVP | `l4ConflictMeta` / `ApplyConfidenceDecay`；无完整审核 UI 流 |
| trpc `memory.Service` SQLite | ✅ | `sqlite_adapter` 注入 Runner（有 Store 时） |
| `MemoryService` HTTP/gRPC | ✅ | ListL0/L1/Facts/Entities/Neighborhood/Evolution… |
| TurnMemoryWorker | 🟡 MVP | `OnRunnerCompletion` → 入队 AutoMemory；**非**独立 goroutine 全量 EP-MEM-01 |
| MemoryWorker（独立单元） | ❌ | 无 `turn.completed` 订阅 + LLM 提取管道 |
| 记忆级联审核 / Proposal 闭环 | ❌ | Evolution API 部分存在；级联 BFS + 审核流未产品化 |
| L3 rerank / 遗忘策略 | ❌ | 无 Cross-Encoder；无统一 decay 任务 |
| 前端 Memory Center | 🟡 | 部分页面；见 `12-16 memory.md` |

---

## 3. 差距与优化（优先级）

| 优先级 | 项 | 说明 |
|--------|-----|------|
| P1 | 文档与术语 | 需求文内「ADK」统一为 **trpc-agent-go**；`memory.md` 链接指向 `38 memory.md` |
| P1 | 双轨说明 | 明确 pgvector vs sessionmemory 的使用场景，避免实现者误接 Runner |
| P2 | MemoryWorker | 将 AutoMemory 升级为 EventBus `runner_completion` 后异步 LLM 提取（替代纯 regex） |
| P2 | L4 治理 | 冲突检测 UI、EvolutionProposal 审核、级联 BFS（`memory_l4_usecase` 已有元数据钩子） |
| P2 | `AddSessionToMemory` | 框架会话同步与 L2/L3 写入对齐（若产品需要） |
| P3 | L3 rerank、全局衰减、PII | 见 Phase 4–5 |

---

## 4. 开发阶段

（保留原 Phase 1–5 路线图；**Phase 1 基础表与注入已实现**，后续侧重治理与 Worker。）

### Phase 1：L4 基础 — 🟡 MVP 已落地

- ✅ Schema + Repo + `L4MemoryCue` + 启发式 `WriteFromUserText`
- ❌ 完整 GraphRAG、Proposal 审核台、EvolutionScanner 产品闭环

### Phase 2：MemoryWorker — 🟡 部分

- ✅ `TurnMemoryWorker` 入队 + `AutoMemoryWorker` 周期任务
- ❌ LLM fact/episode 提取、巩固管道、可配置 `memory_worker_*` 设置项

### Phase 3–5

- 级联更新、L3 rerank、衰减与遗忘（见下文任务表）

---

## 5. 任务清单（更新后）

### Phase 1：L4 图谱（剩余）

| # | 任务 | 状态 |
|---|------|------|
| 1.1–1.4 | Schema / Repo / 注入 / 启发式写入 | ✅ |
| 1.5–1.6 | Identity/Strategy/Evolution 全链路审核 UI | ❌ |

### Phase 2：MemoryWorker（剩余）

| # | 任务 | 状态 |
|---|------|------|
| 2.1 | Runner 完成后入队 | ✅ `TurnMemoryWorker` |
| 2.2 | LLM 提取 fact/episode | ❌ |
| 2.3–2.5 | 冲突检测 / 巩固 / 配置项 | ❌ / ❌ / ❌ |

---

## 6. 验收标准

与 Phase 状态对齐：Phase 1 基础能力可验收；Phase 2–5 仍按原验收条目，**未勾选项表示未实现**。

---

## 7. 依赖与风险

（保留原文档 §7；补充 **Wire**：`MemoryUsecase` 依赖 `EmbeddingService`，需与 `provideMemoryService` 一并注入。）

---

## 8. MemoryWorker 设计要点

（保留 §8.1–8.4 产品设想；**实现现状**以 §2 为准：`TurnMemoryWorker` + `AutoMemoryWorker`，非独立常驻 Worker 进程。）
