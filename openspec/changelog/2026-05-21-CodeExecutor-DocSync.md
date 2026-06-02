# CodeExecutor 文档与代码对齐

**日期**：2026-05-21 · **模块**：CodeExecutor (32)

## 摘要

按 `docs/README.md` 文档边界，将 CodeExecutor 需求 / 设计 / 开发计划与代码现状对齐；修正产出物、双 Local、metrics 覆盖、默认 backend 等偏差；更新 Skill / Artifact / execution-plan 交叉引用。

## 文档修订

| 文件 | 变更 |
|------|------|
| `32 codeexecutor.md` | 增加 `container` 后端；验收 `[x]`/`[ ]` 与代码对齐；标注 Skill 门控 |
| `32 codeexecutor.design.md` | §2.1 双 Local + artifact 包装；§4.2 Registry 与框架后端；§6.2 回退待实现；§7.1 metrics 缺口；§九 文件清单 |
| `32-codeexecutor-development.md` | 2026-05-21 现状表；产出物 🟡；Phase 任务去空 adapter；验收总览 |
| `README-development.md` | CodeExecutor 接入度列 |
| `execution-plan.md` | 模块表 + 迭代 11 任务板 |
| `0-system-development.md` | Artifact 行 CodeExecutor 产出物 🟡 |
| `20 skill.design.md` / `20-skill-development.md` | `artifact_executor.go` 锚点 |
| `27-artifact-development.md` | CodeExecutor 产出物 ✅→🟡（Docker 路径） |

## 代码真相（未改代码）

- Skill 默认：**框架 `trpclocal`**，非项目 `LocalExecutor`
- 配置面：仅 `CODE_EXECUTOR_BACKEND` 等环境变量；无 `CodeExecutorType`
- 产出物：`WrapWithArtifactSave` + Docker `collectDockerOutputFiles`（部分）
- Prometheus：仅项目 `internal/agent/codeexecutor` Docker/Local 路径

## 待实现（Phase 1+）

见 [32-codeexecutor-development.md](../需求/32-codeexecutor-development.md) §3–§4。
