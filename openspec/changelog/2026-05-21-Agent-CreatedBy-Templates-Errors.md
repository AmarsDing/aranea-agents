# Agent：created_by、创建模板全字段、结构化创建错误

**日期**：2026-05-21  
**范围**：模块 2（创建）+ 模块 3（列表）P2 · LIST-02

## 摘要

- **LIST-02**：`agents.created_by` 列；创建时从 auth 上下文写入；`ListAgents(created_by)` 支持 `mine` 或用户 id；`GET /v1/agents/creators` 供筛选下拉。
- **创建模板**：`ListAgentTemplates` 全字段；前端 `applyTemplate` 填充 display_name / provider / model / description。
- **结构化错误**：`AGENT_KEY_*` / `AGENT` → `kratosError.ts` + 创建弹窗 inline / banner；`CreateAgent` 抑制 4xx 全局 toast。

## 主要改动文件

| 层 | 路径 |
|----|------|
| Proto/SQL | `api/kratos/agent/v1/agent.proto`，`docs/sql/02_agent.sql`，`docs/sql/02_agent_created_by.sql` |
| Biz | `agent_context.go`，`agent_templates.go`，`agent_create_errors.go`，`agent_duplicate.go`，`agent_usecase.go` |
| Data | `internal/data/agent_repo.go`，`ent/schema/agent.go` |
| Service | `internal/service/agent.go` |
| Web | `features/agents/api.ts`，`useAgentsPage.ts`，`AgentsFiltersCard.vue`，`AgentCreateDialog.vue`，`utils/kratosError.ts`，`services/axiosHandler.ts`，`types/axios.d.ts` |

## 迁移

对已有 SQLite 库执行：`docs/sql/02_agent_created_by.sql`（列 + `idx_agents_created_by` 部分索引）。

## 审查修正（同日）

- `Duplicate`：清空 `CreatedBy`，副本归属当前登录用户。
- `ListAgentCreators`：「仅我的」`user_id=mine`，与 `ResolveListCreatedByFilter` 一致。
- 前端：`createAgentService().CreateAgent`；`requestHandler` 对 `CreateAgent` 自动 `skipErrorNotify`。
- 测试：`agent_context_test.go`，`agent_duplicate_test`（created_by），`agent_create_errors_test.go`，`utils/__tests__/kratosError.spec.ts`。

## 文档同步

- [2-agents-create-development.md](../需求/2-agents-create-development.md)
- [3-agent-list-development.md](../需求/3-agent-list-development.md)
- [0-system-development.md](../需求/0-system-development.md) §8.11 AGT-07
- [2-8-agent-modules-development.md](../需求/2-8-agent-modules-development.md)
- [execution-plan.md](../guides/execution-plan.md) I10-LIST-02

## 验证

```bash
go test ./internal/biz/... -count=1
go build ./...
cd web && pnpm test && pnpm build
```
