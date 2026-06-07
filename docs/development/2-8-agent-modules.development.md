# Agent 模块 2–8 — 开发计划

> **版本**：2026-06-06 | **状态**：✅ 迭代 8–11 主项完成；🟡 AGT-15/16/17–22、批量/迁移待补
> **需求/设计**：见各模块 `2 agents-create.md` … `8 agent-title.md` 及对应 `*.design.md`
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **系统待办**：[0-system-development.md](./0-system-development.md) §8.11
> **模块索引**：[2 创建](./2-agents-create.development.md) · [3 列表](./3-agent-list.development.md) · [4 分类](./4-agent-type.development.md) · [5 设置](./5-agent-setting.development.md) · [6 文件](./6-agent-setting-file.development.md) · [7 进化](./7-agent-evolution.development.md) · [8 顶栏](./8-agent-title.development.md)

---

## 1. 目标

在 **不破坏分层红线**（`biz` 不 import `trpc-agent-go`；`server` 不调 Runner）前提下，补齐 Agent 全家桶 **P2 体验与可观测性**，并为 P3（批量、迁移、趋势图）留出接口。

---

## 2. 待优化项清单

| ID | 模块 | 项 | 优先级 | 状态 |
|----|------|-----|--------|------|
| AGT-01 | 运行时 | BuilderDeps 分组 + FlowLog | P2 | ✅ |
| AGT-02 | 5 设置 | config_json PATCH 合并 | P2 | ✅ |
| AGT-03 | 2 创建 | agent_key 查重 | P3 | ✅ |
| AGT-04 | 2 创建 | validate-model | P2 | ✅ |
| AGT-05 | 5 设置 | ToolOverride | P2 | ✅ |
| AGT-06 | 2 创建 | 后端模板 API | P2 | ✅ |
| AGT-07 | 3 列表 | `last_run_status` / `last_run_at` | P2 | ✅ |
| AGT-08 | 5 设置 | `AgentSettingsPage` 拆分 | P2 | ✅（页壳 ~298 行） |
| AGT-09 | 7 进化 | EvolutionScanner | P2 | ✅ |
| AGT-10 | 3 列表 | DuplicateAgent | P3 | ✅ |
| AGT-11 | 6 文件 | AI 编辑（真实 LLM） | P2 | ✅ |
| AGT-12 | 6 文件 | `EstimateTokens` 前端对接 | P2 | ✅ |
| AGT-13 | 4 分类 | 删除前 Agent 计数 | P2 | ✅ data 层已有 |
| AGT-14 | 8 顶栏/列表 | 进化 chip + `pending_evolution_count` | P2 | ✅ |
| AGT-15 | 8 顶栏 | GenerateAgentTitle | P3 | ⏳ |
| AGT-16 | 7 进化 | 指标趋势图 | P3 | ⏳ |
| AGT-17 | 2 创建 | trpc-agent-go §8 对齐（7 项） | P3 | ⏳ |
| AGT-18 | 3 列表 | `BatchUpdateAgents` proto RPC | P3 | ⏳ biz 已有 |
| AGT-19 | 3 列表 | `ReorderAgents` 实现（当前 stub） | P3 | ⏳ |
| AGT-20 | 5 设置 | Debug trace 清理 | P3 | ⏳ |
| AGT-21 | 7 进化 | Learning Loop ↔ Evolution 对齐 | P3 | ⏳ |
| AGT-22 | 6 文件 | PGO V2 文件体系对齐（SOUL/HEARTBEAT deprecated） | P3 | ⏳ |
| — | 3 列表 | 批量 UI / 迁移 | P3 | ⏳ |

---

## 3. 迭代 9 实施顺序（已完成 2026-05-21）

> 变更摘要：[changelog/2026-05-21-Agent-Iteration9.md](../changelog/2026-05-21-Agent-Iteration9.md)

```mermaid
flowchart LR
  A[AGT-07 列表运行态] --> B[AGT-12 EstimateTokens]
  B --> C[AGT-14 进化 chip]
  C --> D[文档 + make api/build]
```

### 3.1 AGT-07 — 列表运行态

| 层 | 变更 |
|----|------|
| proto | `Agent.last_run_status` / `last_run_at` / `pending_evolution_count` |
| data | `ListExtrasForAgents`：最新 session `state_json.runtime.status` + evolution pending 计数 |
| biz | `AgentUsecase.List` 合并 extras |
| web | `formatLastRunStatus`；卡片底栏展示运行态 |

**规则**：无 session → `idle`；有 `runtime.status` → 原样（终态由 `persistRunStatus` 保留，含 `failed`/`cancelled`）；仅有 `last_run_at` 无 status → `idle`。`ListExtrasForAgents` 为批量查询（非 per-agent N+1）。

### 3.2 AGT-12 — Token 估算

| 层 | 变更 |
|----|------|
| 已有 | `EstimateTokens` RPC |
| web | `estimateAgentTokens()`；`AgentFilesPanel` 侧栏用服务端估算 |

### 3.3 AGT-14 — 进化标签

| 层 | 变更 |
|----|------|
| 列表 | `isAgentEvolving(agent)` = `self_evolve && pending_evolution_count > 0` |
| 设置顶栏 | 同上，使用已加载的 suggestions |

---

## 4. 验证（规范要求）

```bash
make api
make build
go build ./internal/agent/ ./internal/biz/ ./internal/data/ ./internal/service/
cd web && pnpm lint && pnpm build
```

---

## 5. 迭代 10（已完成 2026-05-21）

详见 **[devlog/2026-05-21-Agent-Iteration10-Plan.md](../devlog/2026-05-21-Agent-Iteration10-Plan.md)** · [changelog](../changelog/2026-05-21-Agent-Iteration10.md)。审查加固见 changelog §审查修复。

**仍开放**：批量 UI/迁移、Scanner TTL、进化趋势图（AGT-16）、trpc §8 对齐（AGT-17）、PGO V2 对齐（AGT-22）。

---

## 6. 迭代 11 交付（2026-06-06）

> 代码库对齐审计，文档同步至实际实现状态。

| 项 | 说明 |
|----|------|
| 后端 | 22 个 RPC 全部实现；`AgentRuntimeSettings` 123 字段；`Agent` 33 字段（含 `agent_kind`/`source`/`kind`/`position_key`/`agent_variant`） |
| 前端 | 设置页 9 Tab（~298 行页壳）；37 个组件；34 个 features 文件；Learning Loop 全套 |
| 新增系统 | Planner（builtin/react/a2ui）、Ralph Loop、FieldGuide（10 scope）、PGO V2 文件体系、Taxonomy 系统、KindBadge、AIRefineButton |
| 文档 | 模块 2–8 全部 `.development.md` 同步至代码现状 |

---

## 7. LIST-02 交付（2026-05-21）

| 项 | 说明 |
|----|------|
| 库表 | `agents.created_by` + 迁移/索引 `02_agent_created_by.sql` |
| API | `ListAgents.created_by`（`mine` / 用户 id）、`ListAgentCreators`、`ListAgentTemplates` 全字段 |
| 创建 | 服务端写入 `created_by`；模板 `applyTemplate`；Kratos `reason` → inline 错误 |
| 复制 | `Duplicate` 清空 `created_by` 后按当前用户创建 |
| 文档 | [changelog](../changelog/2026-05-21-Agent-CreatedBy-Templates-Errors.md) · `2/3-*-development.md` |
