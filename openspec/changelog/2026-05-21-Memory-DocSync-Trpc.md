# Memory 文档对齐与 trpc-agent-go 术语修正（2026-05-21）

## 背景

按 `docs/README.md` 对 Memory 模块（L0–L4）做实现差距 review：代码真相与需求文档存在 ADK 遗留路径、L4/Worker「未实现」误标、双轨（SQLite sessionmemory vs Postgres pgvector）未说明。

## 文档变更

| 文件 | 变更 |
|------|------|
| `docs/需求/12-16 memory-development.md` | 重写 §1–§2：架构分层、代码锚点、现状表（L4/Worker MVP ✅/🟡） |
| `docs/需求/12 memory-L0-sensory.md` | §14 运行时真相；ADK→trpc；`38 memory.md` 链接 |
| `docs/需求/13–16 memory-L*.md` | 心智模型表 trpc 对齐；`38 memory.md` 链接 |
| `docs/需求/12-16 memory.design.md` | §5.1 `RuntimeSet` + `sqlite_adapter` |
| `docs/README.md` | §5.2 Memory L0–L4 索引行 |
| `docs/需求/README-development.md` | Memory 接入度表述 |

## 架构结论（review）

| 原则 | 结论 |
|------|------|
| 单一职责 | `MemoryAdminUsecase`（观测 API）与 `trpcmemory.Service`（Runner）经 `RuntimeSet` 分离 ✅ |
| 依赖方向 | `service` 不 import `pkg/trpc-agent-go`；桥接在 `internal/memory/trpc` ✅ |
| 双轨 | sessionmemory（默认 Chat/L0–L4）与 `MemoryUsecase` pgvector 并行，需在配置层显式选择 |
| 影响域 | L4 注入改 `trpc_build.go`/`l4_prompt.go`；Admin 查询改 `memory.go` + `sessionmemory` |

## 仍待实现（文档已标 ❌/🟡）

- 独立 MemoryWorker（LLM 提取、巩固管道）
- 级联 BFS + Evolution 审核产品闭环
- L3 rerank、全局衰减任务
- Composite `SearchMemory`（SQLite + pgvector）
- `AddSessionToMemory` 与 L2/L3 写入对齐

## 代码

本次 **仅文档**；无行为变更。
