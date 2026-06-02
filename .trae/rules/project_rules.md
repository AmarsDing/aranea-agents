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
| `openspec-explore` | OpenSpec 探索模式 | 需求探索、问题分析、方案对比 |
| `openspec-propose` | OpenSpec 提案创建 | 新增变更提案（proposal+design+tasks） |
| `openspec-apply-change` | OpenSpec 实施执行 | 按 tasks.md 逐步实施变更 |
| `openspec-archive-change` | OpenSpec 归档 | 变更完成后归档、同步主规格 |
| `superpowers-workflow` | 开发纪律强制 | 实施阶段：TDD+两阶段审查+验证前置 |
| `aranea-test-loop` | 自动化测试循环 | 运行测试、修复失败、生成报告 |

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

### 5.1 OpenSpec + Superpowers 工作流（推荐）

**新变更必须走 OpenSpec 流程**，开发纪律由 Superpowers 强制执行：

```
1. EXPLORE  → openspec-explore（需求探索，只思考不编码）
2. PROPOSE  → openspec-propose（生成 proposal+design+tasks）
3. APPLY    → openspec-apply-change + superpowers-workflow（TDD+审查+验证）
4. ARCHIVE  → openspec-archive-change（归档、同步主规格）
```

**目录约定**：活跃变更 `openspec/changes/<name>/`，主规格库 `openspec/specs/`，已归档 `openspec/changes/archive/`

**Superpowers 纪律**（实施阶段强制）：
1. **TDD 强制**：先写失败测试 → 最小实现 → 重构（hotfix/typo/CSS 除外）
2. **两阶段审查**：先过规格合规，再过代码质量
3. **验证前置**：测试通过 + lint 通过 + build 通过 + 无红线违反 = 才能声明完成

---

## 六、模块关联强制读取（违反即停）

> **任何模块开发前必须先读关联文档。** 模块不是孤岛，改一处必知影响面。

| 文档 | 路径 | 定位 |
|------|------|------|
| **架构蓝图** | `docs/architecture-blueprint.md` | "每个模块是什么"（静态结构、全貌） |
| **模块交叉参考** | `docs/module-cross-reference.md` | "改模块 X 时必须注意谁"（动态关联、影响面） |

**开发任何模块时**：
1. 定位目标模块 → 读蓝图对应章节
2. 读交叉参考手册 → 找到目标模块卡片
3. 查变更影响表 → 确定需要同步修改的文件清单
4. 按依赖方向逐层修改 → 验证时覆盖所有影响面
