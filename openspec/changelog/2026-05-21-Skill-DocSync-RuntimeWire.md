# Skill 文档同步 + 运行时 Layer A/B 接通

**日期**：2026-05-21  
**模块**：Skill (20)

## 摘要

- 接通 `skillruntime` 与 Agent 构建：`NewAgentVisibilityFilter` + `RunOptionWithTurnQuery`。
- 四份 Skill 文档与代码对齐（18 RPC、前端目录、trpc-agent-go 术语、Import proto 状态）。
- `20-skill-development.md` 作为实现差距真相源更新。

## 代码

| 文件 | 变更 |
|------|------|
| `internal/tools/skillruntime/filter.go` | `AgentVisibilityFilter` — 按 invocation 解析 Layer A/B |
| `internal/tools/skillruntime/runtime.go` | `RunOptionWithTurnQuery` / `TurnQueryFromContext` |
| `internal/tools/skillruntime/resolve.go` | 导出 `ResolveSkillSlugs` |
| `internal/agent/trpc_build.go` | `buildSkillDeps(ctx, ag, deps)` 使用 visibility filter |
| `internal/service/trpc_turn.go` | turn 注入 skill query |
| `internal/team/runner_team_trpc.go` | Team turn 注入 skill query |

## 文档

- `docs/需求/20-skill-development.md`
- `docs/需求/20 skill.md`
- `docs/需求/20 skill.design.md`
- `docs/需求/20 skill struct design.md`
- `docs/需求/README-development.md`

## 验证

```bash
go test ./internal/tools/skillruntime/... ./internal/agent/... -count=1
```
