# Aranea-Agents 项目开发规则

> AI 每次开发必须遵守。本文件为**索引 + 全局约束**，详细规范见各 SKILL（冲突时以 SKILL 为准）。
> 本文件**不重复** SKILL 中的红线、编程规范、分层规范、决策树等内容——只告诉你「去哪个 SKILL 找」。

---

## 一、项目概览

Aranea-Agents 是基于 trpc-agent-go 的多智能体编排平台。以 Kratos v2 为传输壳层、trpc-agent-go 为运行时内核。

**技术栈**：Go + Kratos v2（HTTP/gRPC/WebSocket）| trpc-agent-go（Agent 运行时）| Vue 3 + Quasar + Pinia + TypeScript | SQLite（Ent ORM）| Wire（编译期 DI）

**双框架分工**：
- Kratos v2：传输层（HTTP/gRPC/WebSocket）、配置、鉴权、中间件、Wire DI
- trpc-agent-go：Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team）

---

## 二、SKILL 体系（按任务选读）

> 以下 SKILL 为各领域的权威规范。本文件不再重复其内容。

### 编码类

| SKILL | 定位 | 触发场景 |
|-------|------|----------|
| `aranea-coding-guide` | 后端项目编码指南（详细版） | 编写 Go 后端代码 |
| `go-oop-guide` | 通用 Go OOP 编程指导 | struct/接口/组合/工厂设计 |
| `aranea-frontend-guide` | 前端项目编码指南（详细版） | 编写 Vue 3/Quasar/Pinia/TS 代码 |
| `vue-frontend-guide` | 通用 Vue 3 编程指导 | 组件/Composable/TypeScript 设计 |

### 审查类

| SKILL | 定位 | 触发场景 |
|-------|------|----------|
| `aranea-review` | 全栈代码审查（后端 + 前端） | 审查代码的架构/分层/数据流/OOP/UX 合规 |
| `go-oop-review` | Go OOP 代码审查 | 审查 Go 代码的 OOP 合规 |

### 工作流类

| SKILL | 定位 | 触发场景 |
|-------|------|----------|
| `sddflow` | OpenSpec + Superpowers 编排器 | 统一入口，自动路由阶段 |
| `sddflow-brainstorming` | 需求探索 + 设计 | `/sddflow brainstorming` |
| `sddflow-spec` | 生成规格 + 工程翻译 | `/sddflow spec` |
| `sddflow-build` | TDD 实施 + 子代理执行 | `/sddflow build` |
| `sddflow-amend` | 需求变更回退 | `/sddflow amend` |
| `sddflow-close` | 验证 + 归档 | `/sddflow close` |
| `aranea-test-loop` | 自动化测试循环 | 运行测试、修复失败、生成报告 |

### Superpowers 技能（sddflow build 阶段自动激活）

| SKILL | 定位 | 触发场景 |
|-------|------|----------|
| `brainstorming` | 协作式设计探索 | 任何创造性工作前 |
| `writing-plans` | 细粒度实施计划 | 有规格后、编码前 |
| `subagent-driven-development` | 子代理驱动开发 | 执行实施计划 |
| `test-driven-development` | TDD 红绿重构 | 实现任何功能/修复 |
| `verification-before-completion` | 完成前验证 | 声明完成前必须提供证据 |
| `finishing-a-development-branch` | 分支收尾 | 实施完成后 |
| `executing-plans` | 顺序执行计划 | 无子代理时的替代方案 |
| `systematic-debugging` | 系统调试 | 遇到 bug/测试失败 |
| `requesting-code-review` | 请求代码审查 | 任务/功能完成后 |
| `receiving-code-review` | 接收审查反馈 | 收到审查意见时 |

### 各 SKILL 覆盖范围速查

| 你要查的内容 | 去哪个 SKILL |
|-------------|-------------|
| 后端 19 条红线 | `aranea-coding-guide` §2 |
| 前端 14 条红线 | `aranea-frontend-guide` §1 |
| 后端编程规范 CS-B1~B17 | `aranea-coding-guide` §14 |
| 前端编程规范 CS-F1~F8 | `aranea-frontend-guide` §13 |
| 依赖方向 / 分层规范 | `aranea-coding-guide` §1+§5 |
| Agent 运行时规范 | `aranea-coding-guide` §6 |
| 前端数据流 / 分层 | `aranea-frontend-guide` §3+§4 |
| 聊天消息分组 | `aranea-frontend-guide` §5 |
| UX 主题 / Dialog 规范 | `aranea-frontend-guide` §6+§7 |
| 数据库编码规范 | `aranea-coding-guide` §5.4（Schema/访问模式/Repo/事务/读写分离/迁移） |
| Go OOP 设计模式 | `go-oop-guide` |
| Vue 3 组件/Composable 模式 | `vue-frontend-guide` |
| 代码审查清单 | `aranea-review`（全栈）、`go-oop-review`（Go OOP） |
| 决策树（代码该放哪） | `aranea-coding-guide` §4、`aranea-frontend-guide` §2 |
| AI 编码自检清单 | `aranea-coding-guide` §11、`aranea-frontend-guide` §10 |

---

## 日志架构约束

- **红线 #16**：禁止 `log/slog`，统一使用 `pkg/loggateway.Logger`
- **Global() deprecated**：`loggateway.Global()` 已废弃，新代码必须通过构造注入 `loggateway.Logger`
- **CtxFlowLog\***：`internal/event/flow_context.go` 中的 CtxFlowLog* 函数为遗留 API，新代码应使用 `loggateway.Logger` + `With()` 预设字段
- **RuntimeLogAdapter**：trpc-agent-go 运行时日志已桥接到 loggateway Pipeline，无需额外处理

---

## 三、验证命令

| 改动类型 | 最小验证 |
|----------|----------|
| 仅 Service + 单测 | `go test ./internal/service/... -run TestXxx -count=1` |
| 仅 Biz / Data | `go test ./internal/biz/... ./internal/data/... -count=1` |
| Proto 变更 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 前端 | `cd web && pnpm lint && pnpm test && pnpm build` |
| **提交前（全量）** | 后端：`make api && make wire && make build && make test && make lint`；前端：`cd web && pnpm lint && pnpm test && pnpm build` |

---

## 四、代码审查纪律

- 代码审查**必须使用项目 SKILL**（`aranea-review` / `go-oop-review`），不可仅依赖内置通用审查
- 通用审查（如 `TRAE-code-review`）只能作为补充，项目红线和业务规则检查以 SKILL 为准
- **维度审查**：按变更范围动态加载审查维度，详见 `docs/review-dimension-checklists.md`
  - 所有变更：维度 1（架构）、2（质量）、3（正确性）、8（错误处理）
  - 涉及 DB：+ 维度 4（性能）
  - 涉及外部输入/API：+ 维度 5（安全）
  - 涉及 Usecase：+ 维度 6（可测试性）、11（业务逻辑）
  - 涉及跨模块：+ 维度 7（可维护性）、12（文档同步）

---

## 五、任务执行纪律

- 有任务 ID 时：只读对应 development.md / blueprint 中该 ID 块
- 列假设 → 编码 → 分级验证 → 通过后再扩 scope
- 只改与任务直接相关的文件；不顺带 refactor 相邻模块

### 5.1 sddflow 工作流（OpenSpec + Superpowers 编排）

**新变更必须走 sddflow 流程**，由 sddflow 自动编排 OpenSpec 规格管理和 Superpowers 实施纪律：

```
/sddflow brainstorming  → 探索需求，生成 proposal.md
/sddflow spec           → 生成 specs + plan-ready.md + superpowers plan
/sddflow build          → TDD 实施，子代理执行（两阶段审查）
/sddflow amend          → 需求变更时回退（不直接改代码）
/sddflow close          → 验证 + 归档
```

**阶段门控（红线）**：
1. **brainstorming 阶段禁止写代码** — 只能读文件、搜索、讨论
2. **spec 阶段禁止写代码** — 只能生成规格文档和计划
3. **build 阶段必须 TDD** — 先写失败测试，再写最小实现
4. **close 阶段必须验证** — 全量测试 + build + lint 通过才能归档
5. **需求变更必须走 amend** — 禁止在 build 阶段直接改代码适应新需求

**sddflow 生成的关键文件**：
- `openspec/changes/<name>/proposal.md` — 做什么和为什么
- `openspec/changes/<name>/design.md` — 怎么做
- `openspec/changes/<name>/specs/` — 规格增量
- `openspec/changes/<name>/tasks.md` — 实施清单
- `openspec/changes/<name>/plan-ready.md` — 工程翻译层
- `docs/superpowers/plans/YYYY-MM-DD-<name>.md` — Superpowers 详细计划

**三文档同步校验**：`tasks.md` + `plan-ready.md` + superpowers plan 的任务号必须一致

**目录约定**：活跃变更 `openspec/changes/<name>/`，主规格库 `openspec/specs/`，已归档 `openspec/changes/archive/`

**文档维护纪律**（红线）：
1. **OpenSpec 文档必须通过 OpenSpec 命令维护**，禁止手动创建、编辑、移动、删除、重命名 `openspec/` 目录下的任何文件（包括 `openspec/specs/`、`openspec/changes/`、`openspec/changes/archive/` 下的所有文件）
2. 归档变更：`openspec archive <change-name>`（自动同步 delta specs 到主规格库）
3. 更新指令文件：`openspec update`（更新 OpenSpec 自身指令，非业务文档）
4. 主规格库 `openspec/specs/` 的更新只能通过 `openspec archive` 同步，禁止直接编辑主规格文件
5. 如需清理或重组文档结构，必须先创建 change 提案，经审批后再通过 OpenSpec 命令执行
6. **如需修复主规格库中的格式问题（如 Purpose:TBD 未填写、格式不一致等），必须通过 OpenSpec 命令操作，禁止直接编辑 `openspec/specs/` 下的 spec.md 文件**
7. **唯一例外**：用户明确要求手动操作时，须在操作前确认并记录原因
8. **`openspec archive` 会自动完成两件事**：(a) 将变更目录移入 `openspec/changes/archive/`；(b) 自动创建 `openspec/specs/<capability>/` 目录并同步 delta specs。**禁止在归档前手动创建 specs 目录或手动复制 spec 文件**——这是 archive 命令的职责，提前手动操作会导致归档时合并冲突或重复

**Superpowers 纪律**（build 阶段自动激活）：
1. **TDD 铁律**：无失败测试不写生产代码。先写代码后补测试 = 删掉重来
2. **两阶段审查**：规格合规审查优先，代码质量审查其次
3. **验证前置**：无新鲜验证证据不做完成声明。证据先于断言，永远
4. **子代理驱动**：每个任务派遣独立子代理，隔离上下文，连续执行不暂停
5. **YAGNI**：不添加未请求的功能，不过度工程

---

## 六、模块关联强制读取（违反即停）

> **任何模块开发前必须先读关联文档。** 模块不是孤岛，改一处必知影响面。

| 文档 | 路径 | 定位 |
|------|------|------|
| **架构蓝图** | `openspec/specs/architecture-blueprint.md` | "每个模块是什么"（静态结构、全貌） |
| **模块交叉参考** | `openspec/specs/module-cross-reference.md` | "改模块 X 时必须注意谁"（动态关联、影响面） |

**开发任何模块时**：
1. 定位目标模块 → 读蓝图对应章节
2. 读交叉参考手册 → 找到目标模块卡片
3. 查变更影响表 → 确定需要同步修改的文件清单
4. 按依赖方向逐层修改 → 验证时覆盖所有影响面
