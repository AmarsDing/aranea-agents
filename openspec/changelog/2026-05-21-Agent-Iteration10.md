# Agent 迭代 10 — 设置页拆分 · Scanner · AI 编辑 · 模板 · 复制

> **日期**：2026-05-21  
> **计划**：[devlog/2026-05-21-Agent-Iteration10-Plan.md](../devlog/2026-05-21-Agent-Iteration10-Plan.md)

## 摘要

按 AGT-08 → AGT-09 → AGT-11 → AGT-06/10 顺序交付：设置页 Tab 组件化、进化扫描 worker、Prompt AI 编辑 RPC、模板列表 API、DuplicateAgent。

## 变更

### AGT-08 前端

- `AgentSettingsAgentTab` / `AgentSettingsMemoryTab` / `AgentSettingsSkillsTab`
- `AgentSettingsPage.vue` 约 488 行（页壳 + dialogs + 样式）

### AGT-09 后端

- `internal/biz/evolution_scan.go` — `ScanAll` / `ScanAgent`
- `internal/cronrunner/jobs/evolution_scanner.go` — 30min ticker
- `cmd/admin` Wire + `main` 启动（需 `make wire` 成功后生效）

### AGT-11 / AGT-06 / AGT-10

- Proto：`EditPromptFileByAI`、`ListAgentTemplates`、`DuplicateAgent`
- `internal/service/agent_prompt_ai.go`、`biz/agent_templates.go`、`biz/agent_duplicate.go`
- 前端：`editPromptFileByAI`、`listAgentTemplates`、`duplicateAgent`；列表「复制」；AI 编辑对话框

## 审查修复（P0–P2）

- **P0**：`make config` + `make wire`；`wire_gen` 含 `promptAI` / `EvolutionScanner`
- **P1**：`ListExtrasForAgents` 批量查询；终态 run 保留 `runtime.status`；`ApplySuggestion(prompt)` 写入 AGENTS_*；`Duplicate` 深拷贝文件 + `CheckAgentKey`；`ScanAll` 聚合错误 + scanner 日志
- **P2**：AI 编辑独立 `aiEditing` loading；`mapPromptFileAIError`；FlowLog；单测

## 文档同步（README §4 步骤 8）

| 文档 | 修订 |
|------|------|
| `需求/2-agents-create-development.md` | 模板 API ✅ |
| `需求/3-agent-list-development.md` | `DuplicateAgent`、批量 ListExtras |
| `需求/5-agent-setting-development.md` | Tab 子组件拆分 |
| `需求/6-agent-setting-file-development.md` | AI 编辑 RPC ✅ |
| `需求/7-agent-evolution-development.md` | Scanner + Apply prompt |
| `需求/README-development.md` | 接入度表 + 迭代 10 快照 |
| `需求/2-8-agent-modules-development.md` | 迭代 10 ✅ |
| `devlog/2026-05-21-Agent-Iteration10-Plan.md` | 验收 + §9 审查 |
| `guides/execution-plan.md` | I10-REVIEW、I10-LIST-02 |
| `changelog/2026-05-21-Agent-CreatedBy-Templates-Errors.md` | LIST-02 专章（后续交付） |
| `需求/frontend-pages.md` | 复制 / AI 编辑 |

## 验证

```bash
make api
make config
make wire
cd web && pnpm build
go build ./...
go test ./internal/biz/... ./internal/data/... ./internal/service/... -count=1
```
