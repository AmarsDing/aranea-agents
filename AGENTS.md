# Aranea-Agents — AI Agent Instructions

> Cursor / Claude Code 入口。人类开发者见 [README.md](./README.md)。

## 编码规范（按任务选读）

> 详细规范在 `.trae/skills/` 下的 SKILLs 中，本文件为精简索引。内容冲突时以 SKILL 为准。

| 任务 | SKILL | 说明 |
|------|-------|------|
| 后端 Go | `aranea-coding-guide` | 架构铁律、分层规范、框架集成（详细版） |
| 后端 Go OOP | `go-oop-guide` | 通用 Go OOP 编程指导 |
| 后端 Go 审查 | `go-oop-review` | Go OOP 代码审查 |
| 前端 | `aranea-frontend-guide` | 数据流铁律、分层规范、UX 主题（详细版） |
| 前端 Vue 3 | `vue-frontend-guide` | 通用 Vue 3 编程指导 |
| 前端审查 | `aranea-frontend-review` | 前端代码审查（含业务检查） |
| Agent 运行时 | `.cursor/rules/trpc-agent-framework-first.mdc` | 框架真相源 |

## 红线（违反即停）

完整后端 19 条红线见 `aranea-coding-guide` SKILL；完整前端 14 条红线见 `aranea-frontend-guide` SKILL。

## 任务执行

- 列假设 → 编码 → 分级验证 → 通过后再扩 scope
- 只改与任务直接相关的文件；不顺带 refactor 相邻模块

