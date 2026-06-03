# Aranea-Agents — AI Agent Instructions

> Cursor / Claude Code 入口。人类开发者见 [README.md](./README.md)。
> 完整规范在 `.trae/skills/` 下的 SKILLs 中，本文件为精简索引。内容冲突时以 SKILL 为准。

## SKILL 索引

### 编码类

| 任务 | SKILL | 说明 |
|------|-------|------|
| 后端 Go | `aranea-coding-guide` | 架构铁律、分层规范、框架集成（详细版） |
| 后端 Go OOP | `go-oop-guide` | 通用 Go OOP 编程指导 |
| 前端 | `aranea-frontend-guide` | 数据流铁律、分层规范、UX 主题（详细版） |
| 前端 Vue 3 | `vue-frontend-guide` | 通用 Vue 3 编程指导 |

### 审查类

| 任务 | SKILL | 说明 |
|------|-------|------|
| 代码审查 | `aranea-review` | 全栈代码审查（后端 + 前端） |
| Go OOP 审查 | `go-oop-review` | Go OOP 代码审查 |

### 工作流类

| 任务 | SKILL | 说明 |
|------|-------|------|
| 需求探索 | `openspec-explore` | 问题分析、方案对比（只思考不编码） |
| 提案创建 | `openspec-propose` | 生成 proposal+design+tasks |
| 实施执行 | `openspec-apply-change` | 按 tasks.md 逐步实施变更 |
| 归档 | `openspec-archive-change` | 变更完成后归档、同步主规格 |
| 开发纪律 | `superpowers-workflow` | TDD+两阶段审查+验证前置 |
| 测试循环 | `aranea-test-loop` | 运行测试、修复失败、生成报告 |

### 其他

| 任务 | 位置 | 说明 |
|------|------|------|
| Agent 运行时 | `pkg/trpc-agent-go/docs/` | 框架真相源 |

### 日志架构

| 约束 | 说明 |
|------|------|
| 红线 #16 | 禁止 `log/slog`，统一使用 `pkg/loggateway.Logger` |
| Global() deprecated | 新代码必须通过构造注入 `loggateway.Logger`，禁止使用 `loggateway.Global()` |
| CtxFlowLog* | 遗留 API，新代码使用 `loggateway.Logger` + `With()` |
| 运行时日志 | trpc-agent-go 运行时日志已通过 RuntimeLogAdapter 桥接到 Pipeline |

## 模块关联（开发前必读）

> **模块不是孤岛。改任何模块前，必须先读关联文档。**

| 文档 | 路径 | 何时读 |
|------|------|--------|
| 架构蓝图 | `openspec/specs/architecture-blueprint.md` | 了解项目全貌和每个模块的静态职责 |
| 模块交叉参考 | `openspec/specs/module-cross-reference-full.md` | **每次开发前必读**——查找目标模块的所有上游依赖、下游影响、共享契约、事件、数据库、前端对应 |

## 任务执行

- 列假设 → 编码 → 分级验证 → 通过后再扩 scope
- 只改与任务直接相关的文件；不顺带 refactor 相邻模块
- **开发前必读模块交叉参考手册**（`openspec/specs/module-cross-reference-full.md`），确认所有关联影响面
