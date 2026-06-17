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
| 测试循环 | `aranea-test-loop` | 运行测试、修复失败、生成报告 |

### Superpowers 辅助（按需使用）

`brainstorming`、`writing-plans`、`subagent-driven-development`、`test-driven-development`、`verification-before-completion`、`systematic-debugging`、`executing-plans`、`requesting-code-review`、`receiving-code-review`、`finishing-a-development-branch`

### 其他

| 任务 | 位置 | 说明 |
|------|------|------|
| Agent 运行时 | `pkg/trpc-agent-go/docs/` | 框架真相源 |

### 日志架构

| 约束 | 说明 |
|------|------|
| 红线 #16 | 禁止 `log/slog`，统一使用 `pkg/loggateway.Logger` |
| Global() deprecated | 新代码必须通过构造注入 `loggateway.Logger`，禁止使用 `loggateway.Global()` |
| CtxFlowLog* | 遗留 API（`WithFlowLogger`/`FlowLoggerFromContext`/`NewFlowLogger`），新代码使用 `loggateway.Logger` + `With()` |
| 运行时日志 | trpc-agent-go 运行时日志已通过 RuntimeLogAdapter 桥接到 Pipeline |
| 构造注入 | struct 通过 `lg loggateway.Logger` 参数注入，用 `lg.With()` 预设字段 |
| 结构化字段 | 使用 `loggateway.StepID`/`SessionID`/`RunID`/`Err` 等，禁止拼接字符串到 msg |
| 错误记录 | 使用 `loggateway.Err(err)`（自动解包错误链），不要用 `Str("error", err.Error())` |
| 测试 Logger | 优先使用 `loggateway.NewNoop()`，避免 `loggateway.Global()` |
| 前端日志 | 无框架，仅 `console.warn/info`，禁止 `console.log` |

> 详细规则见 `project_rules.md` §日志架构约束

## 模块关联（开发前必读）

> **模块不是孤岛。改任何模块前，必须先读关联文档。**

| 文档 | 路径 | 何时读 |
|------|------|--------|
| 模块交叉参考 | `docs/development/65-module-cross-reference-full.md` | **每次开发前必读**——查找目标模块的所有上游依赖、下游影响、共享契约、事件、数据库、前端对应 |
| 系统架构总览 | `docs/development/0-system-diagram.md` | 了解项目全貌和每个模块的静态职责 |
| 数据库架构 | `docs/development/66-database-architecture.md` | 数据库设计与访问模式 |

## 架构评判标准（AS 系列）

> 建设性指引，补充"禁止性红线"的不足。详细方案见 `docs/reports/2026-06-11-review-architecture-runtime-pain-points.md`。

| 编号 | 标准 | 核心要求 |
|------|------|---------|
| AS-ADR-01 | 架构决策记录 | 跨模块决策必须记录 ADR（背景/决策/后果/替代方案） |
| AS-COG-01 | 认知复杂度量化 | struct 字段 ≤15、biz 依赖 ≤8、sync.Map = 0（应提取） |
| AS-FSM-01 | 状态机显式化 | >3 种状态的实体必须定义显式状态机 |
| AS-STA-01 | 接口稳定性分级 | biz port 接口标注 Stable/Evolving/Internal |
| AS-FIT-01 | 架构 Fitness Function | 依赖方向/分层隔离/接口窄化/状态机覆盖自动验证 |
| AS-EVT-01 | 事件可靠性分级 | Critical=WBPF+重试（ToolResult/Error/RunnerCompletion/Checkpoint）、Important=BlockUpTo+异步持久化（StateDelta/TokenUsage/RunStatus/SessionStatusChanged/GraphNodeEnd/TeamRunFinished）、Informational=尽力而为 |

## 任务执行

- **新变更推荐流程**：需求探索 → 规格设计 → TDD 实施 → 验证归档
- 需求变更时回退到设计阶段，禁止直接改代码适应新需求
- 只改与任务直接相关的文件；不顺带 refactor 相邻模块
- **开发前必读模块交叉参考手册**（`docs/development/65-module-cross-reference-full.md`），确认所有关联影响面

## docs 目录规范

> **操作 docs/ 目录下的任何文件前，必须先读 `aranea-docs-guide` SKILL。**

| 目录 | 用途 | 命名规范 |
|------|------|----------|
| `docs/development/` | 模块开发文档 | `<N>-<name>.md` / `.design.md` / `.development.md` |
| `docs/testing/` | 测试文档 | 见各子目录 README.md |
| `docs/scenarios/` | 专业场景文档 | 每场景一个子目录，kebab-case |
| `docs/reports/` | 调研报告 | `YYYY-MM-DD-<type>-<topic>.md` |
| `docs/notes/` | 个人笔记 | 用户自维护，AI 不主动修改 |

### 三件套内容边界（强制）

每个模块的三件套必须严格按边界组织内容：

| 文档 | 后缀 | 只允许的内容 |
|------|------|-------------|
| 需求文档 | `.md` | 用户故事、功能需求清单、验收标准、非功能需求、交互规格（用户视角） |
| 设计文档 | `.design.md` | 架构设计、代码分层、Proto/API 契约、数据模型、技术选型、状态机、前端组件设计、UX 规范 |
| 开发计划 | `.development.md` | 模块定位、代码锚点、现状评估、任务清单（含状态）、验收标准、改动文件清单 |

### 文档同步纪律（红线）

**代码改动必须同步对应文档**，文档与代码偏差视为技术债务。

| 规则 | 说明 |
|------|------|
| DOC-SYNC-1 | 影响模块行为/接口/数据结构的代码改动，必须同 PR 同步更新三件套 |
| DOC-SYNC-2 | 需求文档只列功能需求；架构/协议/实现迁移到设计文档 |
| DOC-SYNC-3 | 设计文档体现设计与实现；不含需求清单和开发进度 |
| DOC-SYNC-4 | 开发计划记录进度；不含需求和架构设计 |
| DOC-SYNC-5 | 状态标记（✅/⏳/🟡/📋）必须反映代码真实状态 |
| DOC-SYNC-6 | 代码锚点引用的文件路径必须真实存在 |
| DOC-SYNC-7 | API 端点表必须与 `api/kratos/` Proto 定义一致 |
| DOC-SYNC-8 | 数据表结构必须与 `internal/data/ent/schema/` 一致 |

**触发同步的条件**：新增/删除/重命名 RPC、Schema、核心文件；完成待办任务；新增模块功能。
**豁免**：纯 bug 修复、不改变行为的重构、测试代码、配置调参。

> 详细规则见 `project_rules.md` §六「文档同步纪律」
