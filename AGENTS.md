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

### 文档类

| 任务 | SKILL | 说明 |
|------|-------|------|
| docs 文档维护 | `aranea-docs-guide` | docs 目录命名规范、存放规则、合并规则 |

### 审查类

| 任务 | SKILL | 说明 |
|------|-------|------|
| 代码审查 | `aranea-review` | 全栈代码审查（后端 + 前端） |
| Go OOP 审查 | `go-oop-review` | Go OOP 代码审查 |

### 工作流类

| 任务 | SKILL | 说明 |
|------|-------|------|
| 统一入口 | `sddflow` | OpenSpec + Superpowers 编排器，自动路由阶段 |
| 需求探索 | `sddflow-brainstorming` | `/sddflow brainstorming` — 探索需求+设计 |
| 生成规格 | `sddflow-spec` | `/sddflow spec` — specs + plan-ready.md |
| TDD 实施 | `sddflow-build` | `/sddflow build` — 子代理执行+两阶段审查 |
| 需求变更 | `sddflow-amend` | `/sddflow amend` — 回退修改规格 |
| 验证归档 | `sddflow-close` | `/sddflow close` — 全量验证+归档 |
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

- **新变更必须走 sddflow 流程**：`/sddflow brainstorming` → `/sddflow spec` → `/sddflow build` → `/sddflow close`
- 需求变更时用 `/sddflow amend`，禁止直接改代码
- 只改与任务直接相关的文件；不顺带 refactor 相邻模块
- **开发前必读模块交叉参考手册**（`openspec/specs/module-cross-reference-full.md`），确认所有关联影响面

## docs 目录规范

> **操作 docs/ 目录下的任何文件前，必须先读 `aranea-docs-guide` SKILL。**

| 目录 | 用途 | 命名规范 |
|------|------|----------|
| `docs/development/` | 模块开发文档 | `<N>-<name>.md` / `.design.md` / `.development.md` |
| `docs/testing/` | 测试文档 | 见各子目录 README.md |
| `docs/scenarios/` | 专业场景文档 | 每场景一个子目录，kebab-case |
| `docs/reports/` | 调研报告 | `YYYY-MM-DD-<type>-<topic>.md` |
| `docs/notes/` | 个人笔记 | 用户自维护，AI 不主动修改 |

## OpenSpec 文档维护纪律（红线）

- **OpenSpec 文档必须通过 OpenSpec 命令维护**，禁止手动创建、编辑、移动、删除、重命名 `openspec/` 目录下的任何文件
- 主规格库 `openspec/specs/` 的更新只能通过 `openspec archive` 同步，禁止直接编辑
- **`openspec archive` 会自动创建 specs 目录并同步 delta specs，禁止在归档前手动创建目录或复制文件**
- 如需修复格式问题，必须通过 OpenSpec 命令操作
- **唯一例外**：用户明确要求手动操作时，须在操作前确认并记录原因
