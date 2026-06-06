---
name: aranea-docs-guide
description: Aranea-Agents 项目文档维护规范。当创建、修改、移动 docs/ 目录下的文档时自动触发，提供命名规范、存放规则、合并规则等指导。
triggers:
  - 创建文档
  - 修改文档
  - 移动文档
  - 重命名文档
  - docs 目录
  - 文档规范
  - 文档整理
---

# Aranea-Agents 文档维护规范

> AI 在操作 `docs/` 目录下的任何文件时必须遵守本规范。

---

## 1. docs 目录总览

```
docs/
├── development/     # 项目开发文档（模块需求/设计/开发计划）
├── testing/         # 测试文档（策略/流程/数据/报告）
├── scenarios/       # 专业场景文档（行业/业务场景方案）
├── reports/         # 调研报告文档（分析/审计/调研）
└── notes/           # 个人笔记（用户自己维护）
```

---

## 2. development/ — 开发文档规范

### 2.1 三件套命名

每个模块三类文档，格式 `<编号>-<模块名>.<类型>.md`：

| 类型 | 后缀 | 内容 |
|------|------|------|
| 需求 | `.md` | 用户故事、功能需求、验收标准 |
| 设计 | `.design.md` | 架构、数据模型、API 设计 |
| 开发计划 | `.development.md` | Phase、里程碑、任务清单 |

### 2.2 编号分配

- 新模块使用已有编号后的第一个空位
- 禁止复用已占用的编号
- 禁止同一编号下放置不同主题

### 2.3 子模块合并规则（红线）

**禁止**为子功能创建独立文档文件。子功能内容必须合并到主文档末尾：

```
# 错误 ❌
1-chat.md
1-chat-execution-trace.md

# 正确 ✅
1-chat.md    # 末尾包含 "## 子模块：Chat 执行过程卡片" 章节
```

合并格式：
```markdown
---

## 子模块：<子模块名>

（子文档内容）
```

### 2.4 跨模块参考文档

以下文档为全局参考，不属于任何模块：
- `architecture-blueprint.md`、`module-cross-reference-full.md`
- `backend-layers.md`、`frontend-layers.md`、`frontend-pages.md`
- `logging-framework.md`、`built-in-tools-guide.md`
- `aranea-agents-product-whitepaper.md`

### 2.5 禁止事项

- 禁止 `-development.md` 后缀（必须 `.development.md`）
- 禁止文件名中使用空格
- 禁止保留 SUPERSEDED 文档
- 禁止跳过需求直接写设计

---

## 3. testing/ — 测试文档规范

### 3.1 目录结构

```
testing/
├── test-strategy.md             # 测试策略
├── test-loop-process.md         # 测试闭环流程
├── test-report-template.md      # 报告模板
├── test-data/                   # 测试数据
│   └── sample-<entity>.json     # 统一 sample- 前缀
└── reports/                     # 测试报告
    ├── report-YYYYMMDD-HHMMSS.md
    └── acceptance-YYYY-MM-DD-<topic>.md
```

### 3.2 规则

- test-data/ 下所有 JSON 文件必须以 `sample-` 开头
- 测试报告命名：`report-YYYYMMDD-HHMMSS.md`
- 验收报告命名：`acceptance-YYYY-MM-DD-<topic>.md`
- 禁止在根目录放置专项验收文档
- 禁止在 test-data/ 中放置非 JSON 文件

---

## 4. scenarios/ — 场景文档规范

### 4.1 目录结构

```
scenarios/
└── <scenario-name>/             # 每个场景一个子目录
    ├── README.md                # 场景索引
    ├── <scenario-name>.md       # 需求文档
    ├── <scenario-name>.design.md    # 设计文档
    └── <scenario-name>.development.md  # 开发计划
```

### 4.2 规则

- 目录名和文件名统一使用 kebab-case（连字符）
- 禁止在 scenarios/ 根目录直接放置场景文档
- 禁止使用下划线命名
- 禁止跳过需求文档直接写设计

---

## 5. reports/ — 调研报告规范

### 5.1 命名格式

**必须**使用 `YYYY-MM-DD-<type>-<topic>.md`：

| type | 含义 |
|------|------|
| `analysis` | 分析报告（竞品分析、技术选型） |
| `audit` | 审计报告（代码审计、架构审计） |
| `requirements` | 需求报告（差距需求、调研需求） |
| `review` | 评审报告 |
| `research` | 调研报告 |

### 5.2 规则

- 禁止使用日期后缀命名（如 `topic-2026-05-31.md`）
- 禁止在文件名中使用空格
- 测试报告放 `docs/testing/reports/`，不放此处

---

## 6. notes/ — 个人笔记规范

- AI **不主动**创建、编辑或删除此目录下的文件
- 仅在用户明确要求时才操作
- 笔记内容不作为项目权威文档引用
- 如果笔记内容成熟，AI 可建议迁移到对应目录

---

## 7. 决策树：新文档该放哪？

```
新文档是什么类型？
├── 模块需求/设计/开发计划 → docs/development/<N>-<name>.<type>.md
├── 测试策略/流程/数据 → docs/testing/
├── 测试报告 → docs/testing/reports/
├── 行业/业务场景方案 → docs/scenarios/<name>/
├── 调研/分析/审计报告 → docs/reports/YYYY-MM-DD-<type>-<topic>.md
├── 个人想法/草稿 → docs/notes/
└── 不确定 → 询问用户
```

---

## 8. 自检清单

每次操作 docs/ 目录下的文件后，AI 必须检查：

- [ ] 文件名是否符合对应目录的命名规范？
- [ ] 文件是否放在了正确的目录？
- [ ] 是否存在同模块的子文档需要合并？
- [ ] 编号是否与已有模块冲突？
- [ ] 是否删除了 SUPERSEDED 的旧版本文档？
- [ ] 跨目录引用路径是否已更新？
