# Agent 列表 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[3 agent-list.md](./3%20agent-list.md) · **设计**：[3 agent-list.design.md](./3%20agent-list.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Agent 管理列表页：展示所有 Agent，支持搜索、筛选、排序、分页，以及快捷操作（编辑、复制、删除、运行测试）。

**代码锚点**：
- `api/kratos/agent/v1/agent.proto` — ListAgents RPC
- `internal/service/agent.go` — ListAgents
- `internal/biz/agent_usecase.go` — AgentUsecase.List

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| Agent 列表 | ✅ | `ListAgents` RPC + 分页 |
| 搜索/筛选 | ✅ | search / status / agent_type 参数 |
| 排序 | ✅ | sort_order 字段 |
| 软删除 | ✅ | `deleted_at` 字段 |
| 复制 Agent | ✅ | `DuplicateAgent` RPC |
| 收藏切换 | ✅ | `ToggleFavorite` RPC |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 列表页网格/列表视图 | 🟡 待验证 | 需确认 `AgentListPage.vue` 是否已实现 |
| 搜索框 | 🟡 待验证 | 需确认防抖搜索是否已实现 |
| 类型筛选 | 🟡 待验证 | `agent_type` 筛选下拉 |
| 业务分类筛选 | 🟡 待验证 | `category_position_id` 筛选（依赖模块4） |
| 创建者筛选 | ❌ 未实现 | 需 `agents.created_by` 字段（当前表无此列） |
| 视图切换（网格/列表） | 🟡 待验证 | 需确认 localStorage 持久化 |
| 最近运行状态 | ❌ 未实现 | 需聚合 session 数据，后端无 `last_run_status` 字段 |
| 批量操作 | ❌ 未实现 | 无 `BatchUpdateAgents` RPC，前端无多选 |
| Agent 迁移按钮 | ❌ 未实现 | 需求 §3.2 提到"Agent迁移"，需单独 PRD |

---

## 3. 差距与优化

1. **P2**：列表中"最近运行状态"列需聚合 session 数据，当前前端为静态展示。
2. **P3**：批量操作（批量启用/停用/删除）未实现。
3. **P2**：`agents` 表缺少 `created_by` 列，创建者筛选无法实现。
4. **P3**：列表空态和加载态未设计（需求文档未描述空列表和加载中的 UI）。
5. **P3**：大量 Agent 时列表性能未优化（无虚拟滚动）。

---

## 4. 开发阶段

- **Phase 1**：Agent 列表增加最近运行状态聚合字段 + `created_by` 列
- **Phase 2**：批量操作 API + 前端多选
- **Phase 3**：列表性能优化（虚拟滚动）

---

## 5. 任务清单

| # | 任务 | 层 | 优先级 | EP | 需求回溯 |
|---|------|-----|--------|-----|----------|
| 1 | `agents` 表增加 `created_by` 列 + Ent schema | 后端 | P2 | — | 需求 §3.7 |
| 2 | Agent 列表响应增加 `last_run_status` 聚合字段 | 后端 | P2 | — | 需求 §3.9 |
| 3 | 批量操作 RPC（`BatchUpdateAgents`） | 后端 | P3 | — | 需求 §3.9 |
| 4 | 前端列表页：创建者筛选下拉 | 前端 | P2 | — | 需求 §3.7 |
| 5 | 前端列表页：最近运行状态展示 | 前端 | P2 | — | 需求 §3.9 |
| 6 | 前端列表页：批量操作多选 + 操作栏 | 前端 | P3 | — | — |
| 7 | 前端列表页：空态/加载态设计 | 前端 | P3 | — | 需求补充 |
| 8 | 前端列表页：虚拟滚动优化 | 前端 | P3 | — | 性能优化 |

---

## 6. 验收标准

- [ ] Agent 列表可显示最近一次运行状态
- [ ] 创建者筛选可正常工作
- [ ] 批量操作可正常执行
- [ ] 空列表和加载中有合理的 UI 展示
- [ ] 100+ Agent 列表滚动流畅

---

## 7. 依赖与风险

### 7.1 跨模块依赖

| 依赖模块 | 依赖项 | 说明 |
|----------|--------|------|
| 模块2 Agent创建 | 创建成功后列表刷新 | 创建弹窗关闭后需通知列表页 |
| 模块4 Agent分类 | 分类树数据 | 业务分类筛选下拉数据源 |
| 模块10 Session | `last_run_status` | 最近运行状态需查询 sessions 表 |
| 模块50 Avatar | 头像缩略图 | 卡片头像 `GET /avatar-assets/{id}/thumbnail` |

### 7.2 风险

- 最近运行状态聚合依赖 Session 表查询，需注意性能（可加缓存或物化列）
- `created_by` 列新增需数据迁移（已有 Agent 的 created_by 为 NULL）
- 批量删除需考虑级联影响（关联的 RuntimeSettings、PromptFile 等）

---

## 8. 错误处理规格

| 场景 | HTTP 状态码 | 错误码 | 前端行为 |
|------|------------|--------|----------|
| 列表查询失败 | 500 Internal | `LIST_QUERY_FAILED` | 空态 + 重试按钮 |
| 删除 Agent 失败（有关联 Session） | 409 Conflict | `AGENT_HAS_SESSIONS` | 确认对话框提示 |
| 批量操作部分失败 | 207 Multi-Status | `BATCH_PARTIAL_FAILURE` | 展示成功/失败计数 |
| 搜索超时 | 504 Gateway Timeout | `SEARCH_TIMEOUT` | Toast 提示 + 缩小范围建议 |
