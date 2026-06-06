# Development 项目开发文档

> 每个模块的三类文档（需求/设计/开发计划）统一存放于此。AI 新增或修改文档时必须遵守以下规范。

---

## 目录结构

```
docs/development/
├── README.md                              # 本说明文件
├── <N>-<module>.md                        # 需求文档
├── <N>-<module>.design.md                 # 设计文档
├── <N>-<module>.development.md            # 开发计划
├── memory/                                # 记忆系统子模块
├── phase1-补齐框架能力缺口/               # 阶段规划
├── phase2-增强自主性/
├── phase3-进化能力/
├── phase4-生产级增强/
├── phase5-差异化创新/
├── phase6-记忆系统增强/
└── <cross-cutting-docs>                   # 跨模块参考文档
```

## 命名规范

### 模块文档（三件套）

每个模块必须有三类文档，命名格式为 `<编号>-<模块名>.<类型>.md`：

| 类型 | 后缀 | 内容 |
|------|------|------|
| 需求文档 | `.md` | 用户故事、功能需求、验收标准、非功能需求 |
| 设计文档 | `.design.md` | 架构设计、数据模型、API 设计、技术选型 |
| 开发计划 | `.development.md` | Phase 划分、里程碑、任务清单、实施进度 |

**示例**：
- `1-chat.md` — Chat 对话模块需求
- `1-chat.design.md` — Chat 对话模块设计
- `1-chat.development.md` — Chat 对话模块开发计划

### 编号规则

| 编号范围 | 用途 |
|----------|------|
| 0 | 系统级文档（总览、架构图） |
| 1-11 | 核心业务模块（Chat、Agent、Provider 等） |
| 12-38 | 扩展功能模块（Model Catalog、Admin Auth 等） |
| 39-49 | 运行时与编排（Planner、Runner 等） |
| 50-63 | 高级功能（Avatar、Message、Skill、Spirit 等） |

**新模块编号**：使用已有编号后的第一个空位。禁止复用已占用的编号。

### 子模块内容合并规则

同一模块的子需求/子设计/子开发计划必须**合并到主文档**中，而非创建独立文件：

```
# 错误 ❌
1-chat.md
1-chat-execution-trace.md          # 子需求独立文件

# 正确 ✅
1-chat.md                          # 包含 execution-trace 子需求章节
```

合并格式：在主文档末尾追加 `---` 分隔线 + `## 子模块：<子模块名>` 标题 + 子文档内容。

### 跨模块参考文档

以下文档不属于任何单一模块，作为全局参考：

| 文档 | 用途 |
|------|------|
| `architecture-blueprint.md` | 项目总体架构蓝图 |
| `module-cross-reference-full.md` | 模块交叉参考手册 |
| `backend-layers.md` | 后端分层规范 |
| `frontend-layers.md` | 前端分层规范 |
| `frontend-pages.md` | 前端页面梳理 |
| `logging-framework.md` | 日志框架规范 |
| `built-in-tools-guide.md` | 内置工具指南 |
| `aranea-agents-product-whitepaper.md` | 产品白皮书 |

### 子目录

| 目录 | 用途 | 命名规范 |
|------|------|----------|
| `memory/` | 记忆系统子模块文档 | `L0.md` / `L0.design.md` / `L0-development.md` |
| `phase<N>-<名称>/` | 阶段规划文档 | `<序号>-<主题>.md` + `实施进度.md` |

## AI 存放规则

### 新增模块文档
1. 确定模块编号（查找空位）
2. 创建三件套：`<N>-<name>.md`、`<N>-<name>.design.md`、`<N>-<name>.development.md`
3. 每个文件头部标注版本号和日期

### 新增子功能
1. **禁止**创建独立的子功能文档文件
2. 将子功能内容合并到主文档对应类型的末尾
3. 使用 `## 子模块：<子功能名>` 标题分隔

### 修改现有文档
1. 保持文档的三件套结构不变
2. 新增内容追加到对应章节末尾
3. 过时内容标记 `> ⚠️ 已废弃` 而非直接删除

### 禁止事项
- 禁止创建 `-development.md` 后缀（必须用 `.development.md` 点号分隔）
- 禁止文件名中使用空格（使用连字符 `-`）
- 禁止同一编号下放置不同主题的模块
- 禁止跳过需求文档直接写设计或开发计划
- 禁止保留 SUPERSEDED 文档（应删除旧版，保留新版）
