# Skill 加载优化与可观测性增强 — 开发计划

> 对应需求：[69-skill-loading-optimization.md](./69-skill-loading-optimization.md)
> 设计文档：[69-skill-loading-optimization.design.md](./69-skill-loading-optimization.design.md)
> **状态**：Phase 1-3 已实施，代码审查通过（0 阻断项）

---

## 1. 模块定位

本模块对 Skill 加载链路进行三方向优化：路由命中率可观测性、AI 优化渲染 + 按需注入、Skill 入口可见性。属于 Skill 系统的性能与可观测性增强，不改变 Skill CRUD 与路由的核心架构。

**核心链路**：路由解析 → Invocation State 写入 → 持久化 → 健康指标聚合 → 前端展示；渲染模式扩展 → Full Profile 注入优化；前端类型补全 → 组件可编译。

---

## 2. 代码锚点

| 层级 | 路径 | 说明 |
|------|------|------|
| Schema | `internal/data/ent/schema/skill_invocation.go` | `routed_slugs` / `loaded_slug` 字段 |
| Biz 类型 | `internal/biz/types/skill_health.go` | `RouteHitRate7d/30d` / `RoutedCount/LoadedCount` |
| Biz 接口 | `internal/biz/skill/skill.go` | `InvocationWrite.RoutedSlugs` / `LoadedSlug` |
| Agent 钩子 | `internal/agent/skill_guidance_inject.go` | State Key 常量 / `newSkillLoadCaptureAfterHook` / `extractSlugFromArgs` / `ai_optimized` 渲染 / `skill_load` 提示 |
| Agent 钩子注册 | `internal/agent/callback_chain.go` | `newSkillLoadCaptureAfterHook` 注册点 |
| 调用记录 | `internal/agent/tool_invocation_recorder.go` | 从 Invocation State 读取 `routedSlugs` / `loadedSlug` |
| Data 持久化 | `internal/data/skill.go` | `RecordSkillInvocation` 写入新字段 |
| Data 聚合 | `internal/data/skill_health.go` | `RouteHitRate` 聚合计算 |
| Data 映射 | `internal/data/skill_intelligence.go` | `mapEntSkillInvocationToWrite` 映射新字段 |
| 渲染 | `internal/skill/render/render.go` | `ModeAIOptimized` / `filterDecisionSections` / `isExcludedHeading` |
| 前端类型 | `web/src/features/skills/types.ts` | `route_hit_rate_*` / `routed_count` / `loaded_count` / `SkillCatalogEntry` / `SkillHint` |
| 前端组件 | `web/src/components/skills/SkillHealthCard.vue` | 路由命中率展示 |
| 前端组件 | `web/src/components/chat/ChatSkillCatalogStrip.vue` | Skill 摘要卡片条（待后端事件集成） |
| 前端组件 | `web/src/components/chat/ChatSkillHintBar.vue` | Skill 提示条（待后端事件集成） |

---

## 3. 实施阶段

### Phase 1：路由命中率可观测性 (C1) ✅

| 步骤 | 内容 | 状态 |
|------|------|------|
| 1 | `skill_invocation` Schema 增加 `routed_slugs` / `loaded_slug` 字段 | ✅ |
| 2 | `resolveAndWriteSkillState` 写入 `routed_slugs` 到 Invocation State | ✅ |
| 3 | `newSkillLoadCaptureAfterHook` 捕获 `skill_load` / `skill_run` 的 slug | ✅ |
| 4 | `recordSkillInvocation` 读取并持久化 `routed_slugs` / `loaded_slug` | ✅ |
| 5 | `SkillHealthDetail` 增加 `RouteHitRate7d` / `RouteHitRate30d` | ✅ |
| 6 | `skill_health.go` 聚合计算命中率 | ✅ |
| 7 | 前端 `SkillHealthCard` 展示命中率 + `types.ts` 更新 | ✅ |

### Phase 2：AI 优化渲染 + 按需注入 (B1 + B2) ✅

| 步骤 | 内容 | 状态 |
|------|------|------|
| 1 | `render.go` 增加 `Mode` 字段，支持 `ai_optimized` 模式 | ✅ |
| 2 | `ai_optimized` 过滤非决策段落（精确匹配 + 前缀匹配） | ✅ |
| 3 | UTF-8 安全截断描述至 120 字符 | ✅ |
| 4 | Full Profile 注入改用 `ai_optimized` 渲染 | ✅ |
| 5 | 注入尾部提示"其他可用 Skill 请使用 skill_load 按需加载" | ✅ |

### Phase 3：Skill 入口可见性 (A1 + A2) 🟡 (前端类型补全，集成待后端事件)

| 步骤 | 内容 | 状态 |
|------|------|------|
| 1 | 前端补全 `SkillCatalogEntry` / `SkillHint` 类型 | ✅ |
| 2 | `ChatSkillCatalogStrip.vue` 类型引用已修复 | ✅ |
| 3 | `ChatSkillHintBar.vue` 类型引用已修复 | ✅ |
| 4 | 后端 `skill_catalog` WebSocket 事件 | ⏳ 待实施 |
| 5 | 聊天界面集成 Skill 摘要卡片条 | ⏳ 待后端事件 |

---

## 4. 现状评估（2026-06-17）

| 项 | 状态 | 证据 |
|----|------|------|
| `skill_invocation` 表新字段 | ✅ | `routed_slugs` JSON + `loaded_slug` String(256) 已落地 |
| Invocation State 写入 | ✅ | `skillRoutedSlugsStateKey` / `skillLoadedSlugStateKey` 常量已定义 |
| AfterTool 钩子捕获 slug | ✅ | `newSkillLoadCaptureAfterHook` 在 `callback_chain.go` 注册（priority 0 < 50） |
| Recorder 读取 State | ✅ | `tool_invocation_recorder.go:257,267` 读取并写入 `SkillInvocationWrite` |
| Biz 层 `InvocationWrite` 扩展 | ✅ | `RoutedSlugs []string` / `LoadedSlug string` 字段已添加 |
| 健康指标扩展 | ✅ | `SkillHealthDetail.RouteHitRate7d/30d` + `DailyMetric.RoutedCount/LoadedCount` |
| 聚合计算 | ✅ | `skill_health.go` 使用 `types.SafeRate(loaded, routed)` |
| 前端类型扩展 | ✅ | `SkillHealthMetric.route_hit_rate_*` + `SkillHealthDailyMetric.routed_count/loaded_count` |
| 前端命中率展示 | ✅ | `SkillHealthCard.vue` rate=0 时显示"-"与"暂无路由数据" |
| AI 优化渲染模式 | ✅ | `ModeAIOptimized` + `filterDecisionSections` + `isExcludedHeading` |
| UTF-8 安全截断 | ✅ | `[]rune` 切片截断至 120 字符 |
| Full Profile 注入优化 | ✅ | `newSkillGuidanceBeforeHook` 使用 `ModeAIOptimized` 渲染 + 尾部提示 |
| 前端 Catalog 类型 | ✅ | `SkillCatalogEntry` / `SkillHint` 已定义 |
| 前端 Catalog 组件 | ✅ | `ChatSkillCatalogStrip.vue` / `ChatSkillHintBar.vue` 可编译 |
| 后端 `skill_catalog` 事件 | ⏳ | 代码中未找到 `skill_catalog` WebSocket 事件实现 |
| 聊天界面集成 | ⏳ | 待后端事件实施后集成 |

---

## 5. 差距与优化

### 5.1 已完成

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1 | 路由命中率可观测性全链路（Schema → State → 持久化 → 聚合 → 前端） | ✅ |
| Phase 2 | AI 优化渲染 + Full Profile 注入优化 | ✅ |
| Phase 3 前端 | 类型补全 + 组件可编译 | ✅ |

### 5.2 待后续迭代

| 项 | 说明 | 优先级 |
|----|------|--------|
| 后端 `skill_catalog` WebSocket 事件 | 会话初始化时发送 Skill 摘要 payload | 中 |
| 聊天界面集成 Skill 摘要卡片条 | 依赖后端事件 | 中 |
| `route_hit_rate=0` 语义区分 | 当前无法区分"无数据"与"0%命中率"，需后端暴露 `routed_count` 聚合字段 | 低 |
| Token 效率指标（C2） | 需精确 Token 计量，依赖 C1 数据后再评估 | 低 |
| A/B 对比验证（D1/D2） | 依赖 C1/C2 数据积累 | 低 |

---

## 6. 代码审查记录

### 6.1 审查结果

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 |
|------|---------|---------|---------|
| 后端架构合规 | 0 | 0 | 0 |
| 后端分层合规 | 0 | 0 | 0 |
| 后端 OOP | 0 | 0 | 0 |
| Agent 运行时 | 0 | 0 | 0 |
| 并发安全 | 0 | 0 | 0 |
| 错误处理 | 0 | 0 | 0 |
| 前端数据流 | 0 | 0 | 0 |
| 前端 UX | 0 | 1 | 0 |
| 编程规范 | 0 | 0 | 3 |

### 6.2 已修复的问题

| 问题 | 修复 |
|------|------|
| `loadedSlug` 推导逻辑永远返回空字符串 | 新增 `newSkillLoadCaptureAfterHook` 从工具参数提取 slug 写入 invocation state |
| 描述截断可能破坏多字节 UTF-8 | 改用 `[]rune` 截断 |
| `isExcludedHeading` 使用 `strings.Contains` 过于宽泛 | 改为精确匹配 + 前缀匹配（带分隔符校验） |
| `route_hit_rate=0` 时 UI 语义矛盾 | rate=0 时显示"-"而非"0%" |

### 6.3 待后续迭代

- 🟡 `route_hit_rate=0` 时无法区分"无数据"和"0%命中率"，需后端暴露 `routed_count` 聚合字段

---

## 7. 验收标准

### Phase 1 ✅

- [x] `skill_invocation` 表新增 `routed_slugs` 和 `loaded_slug` 字段
- [x] Progressive 模式下路由结果正确写入 Invocation State（供 recorder 持久化）
- [x] AfterTool 钩子捕获 `skill_load` / `skill_run` 的 slug 并写入 Invocation State
- [x] `SkillHealthDetail` 包含 `RouteHitRate7d` 和 `RouteHitRate30d`
- [x] `SkillHealthCard` 展示路由命中率指标

### Phase 2 ✅

- [x] `render.SkillGuidance()` 支持 `ai_optimized` 模式（过滤非决策段落）
- [x] Full Profile 注入使用 `ai_optimized` 渲染
- [x] 注入尾部包含 `skill_load` 提示

### Phase 3（部分完成）

- [x] `SkillCatalogEntry` / `SkillHint` 类型已定义
- [x] `ChatSkillCatalogStrip.vue` / `ChatSkillHintBar.vue` 可编译
- [ ] 后端 `skill_catalog` WebSocket 事件（待实施）
- [ ] 聊天界面集成 Skill 摘要卡片条（待后端事件）

---

## 8. 变更文件索引

| 文件 | 变更说明 |
|------|----------|
| `internal/data/ent/schema/skill_invocation.go` | 新增 `routed_slugs` / `loaded_slug` 字段 |
| `internal/biz/skill/skill.go` | `InvocationWrite` 新增 `RoutedSlugs` / `LoadedSlug` |
| `internal/biz/types/skill_health.go` | `SkillHealthDetail` 增加 `RouteHitRate`；`DailyMetric` 增加 `RoutedCount` / `LoadedCount` |
| `internal/agent/skill_guidance_inject.go` | 新增 `skillRoutedSlugsStateKey` / `skillLoadedSlugStateKey`；`newSkillLoadCaptureAfterHook`；`extractSlugFromArgs`；`ai_optimized` 渲染；`skill_load` 提示 |
| `internal/agent/tool_invocation_recorder.go` | 从 invocation state 读取 `routedSlugs` / `loadedSlug` |
| `internal/agent/callback_chain.go` | 注册 `newSkillLoadCaptureAfterHook` |
| `internal/data/skill.go` | `RecordSkillInvocation` 持久化新字段 |
| `internal/data/skill_health.go` | 路由命中率聚合计算 |
| `internal/data/skill_intelligence.go` | `mapEntSkillInvocationToWrite` 映射新字段 |
| `internal/skill/render/render.go` | AI 优化渲染模式（`ModeAIOptimized` / `filterDecisionSections` / `isExcludedHeading`） |
| `web/src/features/skills/types.ts` | 新增 `route_hit_rate` / `routed_count` / `loaded_count` / `SkillCatalogEntry` / `SkillHint` |
| `web/src/components/skills/SkillHealthCard.vue` | 路由命中率展示 |
