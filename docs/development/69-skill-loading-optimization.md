# 69-Skill 加载优化与可观测性增强

> 来源：[2026-06-09-solution-skill-loading-optimization.md](../reports/2026-06-09-solution-skill-loading-optimization.md) 方案评估后裁剪
> 设计文档：[69-skill-loading-optimization.design.md](./69-skill-loading-optimization.design.md)
> 开发计划：[69-skill-loading-optimization.development.md](./69-skill-loading-optimization.development.md)

---

## 0. 需求结论

### 0.1 裁剪决策

| 原方案 | 决策 | 理由 |
|--------|------|------|
| A1 Skill 摘要卡片 | **做** | 解决入口可见性核心问题，前端组件已有骨架 |
| A2 点击加载 | **做**（随 A1） | A1 自然延伸，复用 skill_load |
| A3 Trigger 提示 | **不做** | 投入产出比低，Triggers 字段路由未激活 |
| B1 AI 优化渲染 | **做** | 改动最小收益直接，Token 压缩 |
| B2 按需注入 | **做**（精简版） | 不新增 mode，直接优化 Full Profile 注入逻辑 |
| C1 路由命中率 | **做** | 所有优化的数据基础，最优先 |
| C2 Token 效率 | **不做** | 需精确计量，依赖 C1 数据后再评估 |
| D1/D2 后期验证 | **不做** | 依赖 C1/C2 数据积累 |

---

## 1. 用户故事

### 1.1 路由命中率可观测性（C1）

**作为** 平台运维人员 / Skill 作者，
**我希望** 看到 Skill 路由命中率指标（7d / 30d），
**以便** 评估路由准确性，识别"路由到但未被实际加载"的 Skill，优化路由策略。

### 1.2 AI 优化渲染（B1）

**作为** Agent 运行时，
**我希望** 注入到 System Message 的 Skill Guidance 采用 AI 优化渲染模式（过滤介绍/背景/示例段落，仅保留决策树），
**以便** 压缩 Token 消耗，让模型注意力集中在决策关键信息上。

### 1.3 按需注入提示（B2）

**作为** Agent 运行时，
**我希望** Full Profile 模式下注入 Skill Guidance 时，尾部提示"其他可用 Skill 请使用 skill_load 按需加载"，
**以便** 引导模型在路由结果不全时主动加载所需 Skill。

### 1.4 Skill 入口可见性（A1 + A2）

**作为** 聊天用户，
**我希望** 打开聊天时即可看到当前 Agent 可用的 Skill 摘要卡片条，并点击卡片主动加载 Skill，
**以便** 知道有哪些 Skill 可用，无需在对话中盲目试探。

---

## 2. 功能需求

### 2.1 路由命中率可观测性

| # | 需求 | 优先级 |
|---|------|--------|
| FR-1 | `skill_invocation` 记录本轮路由到的所有 Skill slug（`routed_slugs`） | P0 |
| FR-2 | `skill_invocation` 记录本轮实际通过 `skill_load` / `skill_run` 加载的 slug（`loaded_slug`） | P0 |
| FR-3 | Progressive 模式下路由结果写入 Invocation State，供调用记录器持久化 | P0 |
| FR-4 | AfterTool 钩子捕获 `skill_load` / `skill_run` 工具调用的 slug，写入 Invocation State | P0 |
| FR-5 | `SkillHealthDetail` 提供 7d / 30d 路由命中率指标 | P0 |
| FR-6 | `DailyMetric` 提供每日路由次数与加载次数 | P0 |
| FR-7 | 前端 `SkillHealthCard` 展示路由命中率，rate=0 时显示"暂无路由数据" | P0 |

### 2.2 AI 优化渲染 + 按需注入

| # | 需求 | 优先级 |
|---|------|--------|
| FR-8 | `render.SkillGuidance()` 支持 `ai_optimized` 渲染模式 | P0 |
| FR-9 | `ai_optimized` 模式保留 Name、Description（UTF-8 安全截断 120 字符）、Triggers、Tools | P0 |
| FR-10 | `ai_optimized` 模式过滤介绍/背景/示例等非决策段落，仅保留 `##` 决策树段落 | P0 |
| FR-11 | Full Profile 注入使用 `ai_optimized` 渲染模式 | P0 |
| FR-12 | Full Profile 注入尾部提示"其他可用 Skill 请使用 skill_load 按需加载" | P0 |

### 2.3 Skill 入口可见性

| # | 需求 | 优先级 |
|---|------|--------|
| FR-13 | 前端定义 `SkillCatalogEntry` 类型（slug / name / description / tags） | P0 |
| FR-14 | 前端定义 `SkillHint` 类型（matched_skill / trigger / confidence） | P0 |
| FR-15 | `ChatSkillCatalogStrip.vue` 组件可编译，展示 Skill 摘要卡片条 | P0 |
| FR-16 | `ChatSkillHintBar.vue` 组件可编译，展示 Skill 提示条 | P0 |
| FR-17 | 后端 `skill_catalog` WebSocket 事件（会话初始化时发送 Skill 摘要） | P1（待实施） |
| FR-18 | 聊天界面集成 Skill 摘要卡片条 | P1（待实施） |

---

## 3. 非功能需求

| # | 需求 | 说明 |
|---|------|------|
| NFR-1 | UTF-8 安全 | 描述截断必须基于 `[]rune`，不得破坏多字节字符 |
| NFR-2 | 向后兼容 | 不新增 `turn_optimized` load mode，直接优化 Full Profile 注入逻辑，保持现有配置兼容 |
| NFR-3 | 性能 | AfterTool 钩子优先级 0（早于 tool recorder 的 50），确保 slug 写入在持久化之前完成 |
| NFR-4 | UI 语义 | `route_hit_rate=0` 时显示"-"而非"0%"，避免"无数据"与"0%命中率"语义混淆 |
| NFR-5 | 渲染准确性 | 段落过滤使用精确匹配 + 前缀匹配（带分隔符校验），避免误排除决策树段落 |

---

## 4. 验收标准

### 4.1 Phase 1：路由命中率可观测性

- `skill_invocation` 表新增 `routed_slugs` 和 `loaded_slug` 字段
- Progressive 模式下路由结果正确写入 Invocation State（供 recorder 持久化）
- AfterTool 钩子捕获 `skill_load` / `skill_run` 的 slug 并写入 Invocation State
- `SkillHealthDetail` 包含 `RouteHitRate7d` 和 `RouteHitRate30d`
- `SkillHealthCard` 展示路由命中率指标

### 4.2 Phase 2：AI 优化渲染 + 按需注入

- `render.SkillGuidance()` 支持 `ai_optimized` 模式（过滤非决策段落）
- Full Profile 注入使用 `ai_optimized` 渲染
- 注入尾部包含 `skill_load` 提示

### 4.3 Phase 3：Skill 入口可见性

- `SkillCatalogEntry` / `SkillHint` 类型已定义
- `ChatSkillCatalogStrip.vue` / `ChatSkillHintBar.vue` 可编译
- 后端 `skill_catalog` WebSocket 事件（待实施）
- 聊天界面集成 Skill 摘要卡片条（待后端事件）

> 实施进度与状态标记详见 [开发计划](./69-skill-loading-optimization.development.md) §3 与 §7。
