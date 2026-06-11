# 69-Skill 加载优化与可观测性增强

> 来源：[2026-06-09-solution-skill-loading-optimization.md](../reports/2026-06-09-solution-skill-loading-optimization.md) 方案评估后裁剪
> **状态**: Phase 1-3 已实施，代码审查通过（0 阻断项）

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

### 0.2 实施阶段

```
Phase 1: 路由命中率可观测性 (C1) ✅
├── skill_invocation 增加 routed_slugs / loaded_slug 字段
├── resolveAndWriteSkillState 写入 routed_slugs 到 State
├── newSkillLoadCaptureAfterHook 捕获 skill_load/skill_run 的 slug
├── recordSkillInvocation 读取并持久化
├── SkillHealthDetail 增加 RouteHitRate7d / RouteHitRate30d
├── skill_health.go 聚合计算命中率
└── 前端 SkillHealthCard 展示命中率 + types.ts 更新

Phase 2: AI 优化渲染 + 按需注入 (B1 + B2) ✅
├── render.go 增加 Mode 字段，支持 ai_optimized 模式
├── ai_optimized 过滤非决策段落（精确匹配 + 前缀匹配）
├── UTF-8 安全截断描述至 120 字符
├── Full Profile 注入改用 ai_optimized 渲染
└── 注入尾部提示"其他可用 Skill 请使用 skill_load 按需加载"

Phase 3: Skill 入口可见性 (A1 + A2) ✅ (前端类型补全，集成待后端事件)
├── 前端补全 SkillCatalogEntry / SkillHint 类型
├── ChatSkillCatalogStrip.vue 类型引用已修复
└── 后端 skill_catalog WebSocket 事件待实施
```

---

## 1. Phase 1: 路由命中率可观测性

### 1.1 数据库变更

**文件**: `internal/data/ent/schema/skill_invocation.go`

新增字段：
- `routed_slugs` JSON(`[]string`) — 本轮路由到的所有 Skill slug
- `loaded_slug` String(256) — 本轮实际通过 skill_load 加载的 slug

### 1.2 路由结果写入 Invocation State

**文件**: `internal/agent/skill_guidance_inject.go`

- `resolveAndWriteSkillState` 中，将 `result.Slugs` 写入 `skillRoutedSlugsStateKey`
- Full Profile 模式也写入（当前仅 Progressive 模式写入 RoutedSkillsStateKey）
- 双 state key 注释说明兼容原因

### 1.3 Skill 加载捕获

**文件**: `internal/agent/skill_guidance_inject.go`

- 新增 `newSkillLoadCaptureAfterHook`：AfterTool 钩子，捕获 skill_load/skill_run 的 slug
- `extractSlugFromArgs`：从工具参数 JSON 中提取 slug（轻量字符串搜索，避免 full unmarshal）
- 新增 `skillLoadedSlugStateKey` 常量
- 钩子在 callback_chain.go 中注册于 tool recorder 之前（priority 0 < 50）

### 1.4 Skill 调用记录读取路由结果

**文件**: `internal/agent/tool_invocation_recorder.go`

- `recordSkillInvocation` 读取 `skillRoutedSlugsStateKey` 写入 `SkillInvocationWrite.RoutedSlugs`
- 读取 `skillLoadedSlugStateKey` 写入 `SkillInvocationWrite.LoadedSlug`

### 1.5 健康指标扩展

**文件**: `internal/biz/types/skill_health.go`

- `SkillHealthDetail` 增加 `RouteHitRate7d float64` 和 `RouteHitRate30d float64`
- `DailyMetric` 增加 `RoutedCount int` 和 `LoadedCount int`

**文件**: `internal/data/skill_health.go`

- 聚合计算：RouteHitRate = SafeRate(loaded, routed)

### 1.6 前端展示

**文件**: `web/src/features/skills/types.ts`

- `SkillHealthMetric` 增加 `route_hit_rate_7d` / `route_hit_rate_30d`
- `SkillHealthDailyMetric` 增加 `routed_count` / `loaded_count`

**文件**: `web/src/components/skills/SkillHealthCard.vue`

- 增加路由命中率指标行，rate=0 时显示"-"和"暂无路由数据"

---

## 2. Phase 2: AI 优化渲染 + 按需注入

### 2.1 渲染模式

**文件**: `internal/skill/render/render.go`

- `RenderOptions` 增加 `Mode string`（`"full"` / `"ai_optimized"`）
- `ai_optimized` 模式：保留 Name、Description（UTF-8 安全截断 120 字符）、Triggers、Tools
- Body 中仅保留 `##` 开头的决策树段落，过滤介绍/背景/示例等段落
- `isExcludedHeading` 使用精确匹配 + 前缀匹配（带分隔符校验），避免误排除

### 2.2 按需注入

**文件**: `internal/agent/skill_guidance_inject.go`

- Full Profile 注入改用 `ai_optimized` 渲染模式
- 注入尾部增加提示："Other available skills can be loaded on demand using the skill_load tool."

---

## 3. Phase 3: Skill 入口可见性

### 3.1 前端类型补全

**文件**: `web/src/features/skills/types.ts`

- 新增 `SkillCatalogEntry` 类型：`{slug, name, description, tags: string[]}`
- 新增 `SkillHint` 类型：`{matched_skill, trigger, confidence}`

### 3.2 组件状态

- `ChatSkillCatalogStrip.vue` 类型引用已修复，可编译
- `ChatSkillHintBar.vue` 类型引用已修复，可编译
- 后端 `skill_catalog` WebSocket 事件待实施后集成到聊天界面

---

## 4. 验收标准

### Phase 1 ✅

- [x] `skill_invocation` 表新增 `routed_slugs` 和 `loaded_slug` 字段
- [x] Full Profile 模式下路由结果正确写入 Invocation State
- [x] `SkillHealthDetail` 包含 `RouteHitRate7d` 和 `RouteHitRate30d`
- [x] `SkillHealthCard` 展示路由命中率指标

### Phase 2 ✅

- [x] `render.SkillGuidance()` 支持 `ai_optimized` 模式（过滤非决策段落）
- [x] Full Profile 注入使用 ai_optimized 渲染
- [x] 注入尾部包含 skill_load 提示

### Phase 3 (部分完成)

- [x] `SkillCatalogEntry` / `SkillHint` 类型已定义
- [x] `ChatSkillCatalogStrip.vue` / `ChatSkillHintBar.vue` 可编译
- [ ] 后端 skill_catalog WebSocket 事件（待实施）
- [ ] 聊天界面集成 Skill 摘要卡片条（待后端事件）

---

## 5. 代码审查记录

### 审查结果

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

### 已修复的问题

| 问题 | 修复 |
|------|------|
| `loadedSlug` 推导逻辑永远返回空字符串 | 新增 `newSkillLoadCaptureAfterHook` 从工具参数提取 slug 写入 invocation state |
| 描述截断可能破坏多字节 UTF-8 | 改用 `[]rune` 截断 |
| `isExcludedHeading` 使用 `strings.Contains` 过于宽泛 | 改为精确匹配 + 前缀匹配（带分隔符校验） |
| route_hit_rate=0 时 UI 语义矛盾 | rate=0 时显示"-"而非"0%" |

### 待后续迭代

- 🟡 route_hit_rate=0 时无法区分"无数据"和"0%命中率"，需后端暴露 routed_count 聚合字段

---

## 6. 变更文件索引

| 文件 | 变更说明 |
|------|----------|
| `internal/data/ent/schema/skill_invocation.go` | 新增 routed_slugs / loaded_slug 字段 |
| `internal/biz/skill/skill.go` | InvocationWrite 新增 RoutedSlugs / LoadedSlug |
| `internal/biz/types/skill_health.go` | SkillHealthDetail 增加 RouteHitRate; DailyMetric 增加 RoutedCount/LoadedCount |
| `internal/agent/skill_guidance_inject.go` | 新增 skillRoutedSlugsStateKey / skillLoadedSlugStateKey; newSkillLoadCaptureAfterHook; extractSlugFromArgs; ai_optimized 渲染; skill_load 提示 |
| `internal/agent/tool_invocation_recorder.go` | 从 invocation state 读取 routedSlugs / loadedSlug |
| `internal/agent/callback_chain.go` | 注册 newSkillLoadCaptureAfterHook |
| `internal/data/skill.go` | RecordSkillInvocation 持久化新字段 |
| `internal/data/skill_health.go` | 路由命中率聚合计算 |
| `internal/data/skill_intelligence.go` | mapEntSkillInvocationToWrite 映射新字段 |
| `internal/skill/render/render.go` | AI 优化渲染模式（ModeAIOptimized / filterDecisionSections / isExcludedHeading） |
| `web/src/features/skills/types.ts` | 新增 route_hit_rate / routed_count / loaded_count / SkillCatalogEntry / SkillHint |
| `web/src/components/skills/SkillHealthCard.vue` | 路由命中率展示 |
