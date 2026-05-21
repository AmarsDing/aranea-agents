# Planner 文档与代码对齐（P0–P1）

**日期**：2026-05-21 · **模块**：Planner (39)

## 摘要

按 `docs/README.md` 将 Planner 需求 / 设计 / 开发计划与代码对齐；修复 `planner_kind` 数据层断裂；贯通 `planner_config_json` 全链路；拆分 `internal/agent/planner` 职责（config / build / select）；Biz 层增加 `ValidatePlannerKind` / `ValidatePlannerConfigJSON`。

## 代码变更

| 区域 | 变更 |
|------|------|
| SQL | `docs/sql/02_agent_planner.sql`；`02_agent.sql` 基线列 |
| Ent / Data | `planner_kind`、`planner_config_json` 字段 + `agent_repo` 映射 |
| Proto / Service | `planner_config_json = 102` |
| Biz | `PlannerConfigJSON`、校验、`agent_usecase` 边界 |
| Runtime | `config.go` / `build.go` / `selector.go`；`trpc_build` 传入配置 JSON |
| Web | `types.ts` / `wireNormalize.ts` 字段贯通（无配置 UI） |

## 文档修订

| 文件 | 变更 |
|------|------|
| `39 planner.design.md` | 已实现 vs 待办分层；Proto field 102；包结构 |
| `39-planner-development.md` | 2026-05-21 现状表；Phase 1–2 勾选 |
| `39 planner.md` | §1 现状与验收勾选 |
| `README-development.md` | Planner 接入度列 |

## 仍待办（P2）

- Agent 设置页规划模式 / 参数表单
- Chat ReAct 步骤卡片、A2UI 预览
