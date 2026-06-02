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

| 文档 | 内容 | 来源 |
|------|------|------|
| [architecture.md](./architecture.md) | 模块结构与职责、数据库 Schema、Wire 注入、开发决策树（精简版） | `architecture-blueprint.md` |
| [architecture-blueprint.md](./architecture-blueprint.md) | 架构蓝图完整版（含叙述性内容） | 原 `docs/architecture-blueprint.md` |
| [module-cross-reference.md](./module-cross-reference.md) | 指针文件 → `module-cross-reference-full.md`（模块关联 8 维度卡片） | — |
| [module-cross-reference-full.md](./module-cross-reference-full.md) | 模块交叉参考完整版 | 原 `docs/module-cross-reference.md` |
| [backend-layers.md](./backend-layers.md) | 后端分层规则、依赖方向、Agent 运行时铁律、横切约束 | 项目规则 + `aranea-coding-guide` SKILL |
| [frontend-layers.md](./frontend-layers.md) | 前端分层规则、数据流、组件约束、CSS 主题、消息分组 | 项目规则 + `aranea-frontend-guide` SKILL |
| [review-dimension-checklists.md](./review-dimension-checklists.md) | 代码审查 12 维度 × 双面卡片（A 面=编码预防，B 面=Review 检查） | 原 `docs/review-dimension-checklists.md` |
| [built-in-tools-guide.md](./built-in-tools-guide.md) | 内置工具清单 + 可视化配置指南 | 原 `docs/built-in-tools-and-visualization-guide-2026-05-31.md` |

---

## 与其他规范的关系

| 规范来源 | 定位 | 与本目录关系 |
|---------|------|-------------|
| `.trae/rules/project_rules.md` | 项目级速查规则 | 本目录从中提取分层规则，展开为独立文档 |
| `.trae/skills/aranea-coding-guide` | 后端编码完整规范 | 本目录为精简版，SKILL 为权威源 |
| `.trae/skills/aranea-frontend-guide` | 前端编码完整规范 | 本目录为精简版，SKILL 为权威源 |
| `architecture-blueprint.md` | 架构蓝图（含叙述） | `architecture.md` 提取结构事实，去除叙述 |
| `module-cross-reference-full.md` | 模块交叉参考（完整） | `module-cross-reference.md` 为指针文件，指向完整版 |

**冲突处理**：内容冲突时以 SKILLs 为准，本目录文档为快速参考用途。
