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
| CtxFlowLog* / FlowLogger 别名 | **已删除**（`WithFlowLogger`/`FlowLoggerFromContext`/`NewFlowLogger`）。进程日志用 `loggateway.Logger` + `With()`；流程日志 ctx 传播用 `WithTraceEmitter` / `TraceEmitterFromContext`，创建用 `NewTraceEmitterForRun` |
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

## 根目录规范（红线）

> **禁止随意新建根目录。** 所有产物/中间文件必须归入以下既有目录；新增根目录必须在本文档登记。

| 目录 | 用途 | 规则 |
|------|------|------|
| `bin/` | 所有生成的 exe/二进制 + 运行日志 | `go build` 等一切编译产物一律输出到此；运行日志写入 `bin/logs/`（`configs/*.yaml` output_dir）；禁止散落到仓库根或其他目录 |
| `build/` | 编译目录：编译脚本 + 编译产物 | 打包/发布脚本（`build/*.ps1`）、NSIS 脚本（`build/installer/`）、staging/deps 中间产物、安装包输出（`build/release/`） |
| `docs/` | 所有文档 | 规范见下文「docs 目录规范」 |
| `test/` | 测试中间文件 | 按测试名称自建子目录 `test/<test-name>/`；禁止在仓库根或 `_temp/`、`tmp/` 等临时目录堆放排查脚本/中间产物 |
| `blender/` | Blender 3D 资产与预览 Demo | 机房/机柜/服务器/交换机/UPS/PDU/显示器等 3D 模型（.blend/.glb/.fbx）与 Three.js 预览页；`start.bat` 一键启动本地预览服务（http://localhost:8930/） |
| `docker/` | Docker 部署资产（2026-08-13 新增） | 全量 Docker 化（参照 TwinMonitor 部署方案）：`Dockerfile.runtime`（alpine 薄镜像）、`gen-config.ps1`（overlay 配置生成，`config/` 为生成物入 git）、`build-runtime.ps1`（本地交叉编译+镜像）、`migrate-data.ps1`（本地 PG→容器迁移）、`dev-up.ps1`（一键构建部署冒烟）、`register-monitor.ps1`（登记进 TwinMonitor 监控：设备 ICMP + 线路 HTTP/TCP 探测，幂等）；`volumes/` 为运行时产物（logs/data/migrate），不入库 |

- 一次性调试/排查脚本、日志、抓包等中间产物：放 `test/<test-name>/`，用完可整目录删除
- 安装包、zip 等发布产物：放 `build/release/`
- 历史遗留目录（`_temp/`、`_tmp_dbcheck/`、`tmp/`、`release/`、`scripts/`、根部 `installer/`、根部 `sql/`）已于 2026-08-09 清理：有价值小文件归档至 `test/legacy-*/`，根部 `sql/`（`internal/data/sql/migrations/` 的残留副本）与重复 `installer/`（正本在 `build/installer/`）已 git rm；禁止再建
- Go 编译缓存（GOCACHE）一律使用 `F:\gocache`（用户级环境变量），禁止在工程目录内设置 `.gocache*` 等本地缓存

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
