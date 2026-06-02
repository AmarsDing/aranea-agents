# OpenSpec Specs

> Aranea-Agents 项目的稳定、权威规范库。

---

## 定位

本目录包含项目核心规范的**精简参考文档**，聚焦规则、约束与结构，去除叙述性内容。

- **不是教程**：不解释"为什么"，只记录"是什么"
- **自包含**：每份文档可独立阅读，无需其他上下文
- **权威源**：与 SKILLs 和项目规则冲突时，以 SKILLs 为准

---

## 文档索引

| 文档 | 内容 | 定位 |
|------|------|------|
| [architecture-blueprint.md](./architecture-blueprint.md) | 架构真相源：模块结构、业务流程、数据库 Schema、Wire 注入、路由表、开发决策树 | 全局上下文参考 |
| [backend-layers.md](./backend-layers.md) | 后端分层规则、Agent 运行时铁律、横切约束、验证命令 | 后端编码规则速查 |
| [frontend-layers.md](./frontend-layers.md) | 前端分层规则、数据流约束、CSS 主题、消息分组 | 前端编码规则速查 |
| [module-cross-reference.md](./module-cross-reference.md) | 模块交叉参考：改模块 X 时必须注意谁（8 维度卡片） | 跨模块影响分析 |
| [review-dimension-checklists.md](./review-dimension-checklists.md) | 代码审查 12 维度 × 双面卡片（A 面=编码预防，B 面=Review 检查） | 代码审查清单 |
| [built-in-tools-guide.md](./built-in-tools-guide.md) | 内置工具清单 + 竞品对标 + 知识图谱可视化建设指南 | 工具建设参考 |

---

## 文档间关系

```
architecture-blueprint.md    ← 架构真相源（全局上下文）
  ├── backend-layers.md      ← 引用 blueprint §三（后端上下文），只保留规则
  ├── frontend-layers.md     ← 引用 blueprint §四（前端上下文），只保留规则
  └── module-cross-reference.md ← 与蓝图互补：蓝图说"是什么"，交叉参考说"改了影响谁"
```

**去重原则**：结构描述和数据表只在 `architecture-blueprint.md` 出现一次，layer docs 通过引用避免重复。

---

## 与其他规范的关系

| 规范来源 | 定位 | 与本目录关系 |
|---------|------|-------------|
| `.trae/rules/project_rules.md` | 项目级速查规则 | 本目录从中提取分层规则，展开为独立文档 |
| `.trae/skills/aranea-coding-guide` | 后端编码完整规范 | 本目录为精简版，SKILL 为权威源 |
| `.trae/skills/aranea-frontend-guide` | 前端编码完整规范 | 本目录为精简版，SKILL 为权威源 |

**冲突处理**：内容冲突时以 SKILLs 为准，本目录文档为快速参考用途。
