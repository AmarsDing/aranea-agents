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
| [architecture-blueprint.md](./architecture-blueprint.md) | 架构真相源：模块结构（38 Service / 36 Usecase / ~60 Repo）、业务流程、数据库 Schema（~40 表）、Wire 注入、路由表（44 条）、开发决策树 | 全局上下文参考 |
| [backend-layers.md](./backend-layers.md) | 后端分层规则、Agent 运行时铁律（A1-A7）、横切约束、验证命令 | 后端编码规则速查 |
| [frontend-layers.md](./frontend-layers.md) | 前端分层规则（31 Service / 43 Store）、数据流约束、CSS 主题、消息分组 | 前端编码规则速查 |
| [module-cross-reference.md](./module-cross-reference.md) | 模块交叉参考：26 个后端模块卡片 + 6 个前端域卡片（8 维度） | 跨模块影响分析 |
| [review-dimension-checklists.md](./review-dimension-checklists.md) | 代码审查 12 维度 × 双面卡片（A 面=编码预防，B 面=Review 检查） | 代码审查清单 |
| [built-in-tools-guide.md](./built-in-tools-guide.md) | 内置工具清单（28 注册 + ~37 运行时注入）+ 竞品对标 + 知识图谱可视化建设指南 | 工具建设参考 |
| [logging-framework.md](./logging-framework.md) | 日志框架双轨制架构、Pipeline/Sink/EventBus、红线约束、v2 增量（单写路径/断路器/TTL/Trace 拆分） | 日志架构参考 |
| [module-cross-reference-full.md](./module-cross-reference-full.md) | 模块交叉参考完整版（含日志架构模块卡片 + Pack 导入导出增量） | 跨模块影响分析（完整版） |
| [data-layer-observability.md](./data-layer-observability.md) | 数据层可观测性（Ent 错误翻译/DB 延迟指标/慢查询）+ 读写分离抽象（ReadWriteClient/ReadWriteDB） | 数据层基础设施 |
| [vector-store-strategy.md](./vector-store-strategy.md) | VectorStore 接口抽象、SQLite/Postgres 双实现策略、配置驱动选择 | 向量存储策略 |
| [pack-import-export.md](./pack-import-export.md) | Pack 导入导出（Proto/API/格式/校验/冲突策略/种子迁移） | Pack 交换规范 |
| [memory-skills-butler.md](./memory-skills-butler.md) | 记忆管家 + 技能管家 + 经验分析引擎（工具权重/技能健康/记忆质量/编排效率） | 记忆技能管家 |
| [monitor-self-healing.md](./monitor-self-healing.md) | 自愈监控（诊断包/根因分析/运行时修复/自愈观察者/断路器） | 自愈监控体系 |
| [monitor-selfcheck-repair.md](./monitor-selfcheck-repair.md) | 自检修复（SelfCheckRepairer/内置修复动作/定期调度/手动触发） | 自检修复体系 |
| [self-iteration-engine.md](./self-iteration-engine.md) | 自迭代引擎（CI 流水线/AutoFix/发布/E2E/Lint/文档同步/仪表盘） | 自迭代引擎 |
| [team-graph-optimization.md](./team-graph-optimization.md) | Team-Graph 编译模型（CompiledTeam/RoleManifest/TeamMediator） | Team 运行时优化 |

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
