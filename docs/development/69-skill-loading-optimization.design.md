# Skill 加载优化与可观测性增强 — 设计文档

> 对应需求：[69-skill-loading-optimization.md](./69-skill-loading-optimization.md)
> 来源方案：[2026-06-09-solution-skill-loading-optimization.md](../reports/2026-06-09-solution-skill-loading-optimization.md) 方案评估后裁剪
> 开发计划：[69-skill-loading-optimization.development.md](./69-skill-loading-optimization.development.md)

---

## 一、模块概述

本模块对 Skill 加载链路进行三方向优化：

1. **路由命中率可观测性（C1）** — 量化"Skill 被路由加载后是否真正被使用"，为后续优化提供数据基础
2. **AI 优化渲染 + 按需注入（B1 + B2）** — 将 Skill 内容从"给人看的文档"转为"给 AI 看的极简触发器"，压缩 Token
3. **Skill 入口可见性（A1 + A2）** — 让用户在聊天入口即可看到可用 Skill 摘要并主动加载

**核心链路**：路由解析 → Invocation State 写入 → 持久化 → 健康指标聚合 → 前端展示；渲染模式扩展 → Full Profile 注入优化；前端类型补全 → 组件可编译。

---

## 二、Phase 1：路由命中率可观测性

### 2.1 数据库变更

**Schema 文件**：`internal/data/ent/schema/skill_invocation.go`

`SkillInvocation` 表新增字段：

| 字段 | 类型 | 约束 | 用途 |
|------|------|------|------|
| `routed_slugs` | `field.JSON("routed_slugs", []string{})` | `.Optional()` | 本轮路由到的所有 Skill slug |
| `loaded_slug` | `field.String("loaded_slug")` | `.Default("").Optional().MaxLen(256)` | 本轮实际通过 skill_load 加载的 slug |

表名映射保持 `entsql.Annotation{Table: "skill_invocation"}`，索引无变更。

### 2.2 路由结果写入 Invocation State

**文件**：`internal/agent/skill_guidance_inject.go`

新增/复用 Invocation State Key（常量）：

| Key | 用途 |
|-----|------|
| `skillRoutedSlugsStateKey = "aranea.skill_routed_slugs"` | 路由到的 slug 列表（progressive 模式写入） |
| `skillLoadedSlugStateKey = "aranea.skill_loaded_slug"` | 实际加载的 slug（AfterTool 钩子写入） |
| `skillSelectionReasonStateKey = "aranea.skill_selection_reasons"` | 选择原因（已有，复用） |

`resolveAndWriteSkillState(ctx, runtime, deps, progressive bool)` 行为：

- 调用 `skillruntime.ResolveSkillSlugsDetailed` 解析路由结果
- `progressive=true` 时写入 `skillRoutedSlugsStateKey`（供 SkillsRequestProcessor 标记 `[routed]`，并供 recorder 持久化）
- 始终写入 `skillSelectionReasonStateKey`
- Full Profile 模式（`progressive=false`）当前不写入 `skillRoutedSlugsStateKey`，路由命中率仅 Progressive 模式可观测

### 2.3 Skill 加载捕获

**文件**：`internal/agent/skill_guidance_inject.go`

新增 `newSkillLoadCaptureAfterHook()` 返回 `callbacks.AfterToolHook`：

- 优先级 `0`（早于 tool recorder 的 priority 50）
- 仅处理 `toolName == "skill_load" || toolName == "skill_run"`
- 调用 `extractSlugFromArgs(args.Arguments)` 从工具参数 JSON 提取 slug
  - 支持键 `slug` 与 `skill_slug`
  - 使用 `json.Unmarshal` 到 `map[string]json.RawMessage` 后逐键解析，避免 full unmarshal 整个 args
- 将 slug 写入 `skillLoadedSlugStateKey`

**注册位置**：`internal/agent/callback_chain.go`，在 `callbacks.NewToolRecorderCallback(50, ...)` 之前 `append`。

### 2.4 Skill 调用记录读取路由结果

**文件**：`internal/agent/tool_invocation_recorder.go`

`recordSkillInvocation` 在构造 `SkillInvocationWrite` 时：

- 从 `inv.GetState(skillRoutedSlugsStateKey)` 读取 `[]string`，写入 `SkillInvocationWrite.RoutedSlugs`
- 从 `inv.GetState(skillLoadedSlugStateKey)` 读取 `string`，写入 `SkillInvocationWrite.LoadedSlug`

**Biz 层类型**：`internal/biz/skill/skill.go` 的 `InvocationWrite` 结构体新增字段：

```go
RoutedSlugs []string // slugs routed by Layer A+B for this turn
LoadedSlug  string   // slug actually loaded via skill_load/skill_run
```

### 2.5 健康指标扩展

**Biz 类型文件**：`internal/biz/types/skill_health.go`

`SkillHealthDetail` 新增字段：

| 字段 | 类型 | JSON Tag |
|------|------|----------|
| `RouteHitRate7d` | `float64` | `route_hit_rate_7d` |
| `RouteHitRate30d` | `float64` | `route_hit_rate_30d` |

`DailyMetric` 新增字段：

| 字段 | 类型 | JSON Tag |
|------|------|----------|
| `RoutedCount` | `int` | `routed_count` |
| `LoadedCount` | `int` | `loaded_count` |

**聚合实现**：`internal/data/skill_health.go`

聚合按 `skill_id` 过滤（`skillinvocation.SkillIDEQ(skillID)`），在 7d / 30d 窗口内统计：

- 分子：`loaded_slug` 非空的记录数（`loaded7d` / `loaded30d`）
- 分母：`routed_slugs` 非空的记录数（`routed7d` / `routed30d`）
- 计算公式：`RouteHitRate = types.SafeRate(loaded, routed)`
- `DailyMetric.RoutedCount` / `LoadedCount` 同理按日桶聚合

> 注：当前聚合粒度为"该 Skill 的调用记录中，有多少轮路由到了任意 Skill 且加载了任意 Skill"，而非精确匹配"路由到该 Skill 且加载了该 Skill"。这是已知简化，后续如需精确匹配需扩展聚合逻辑。

### 2.6 前端展示

**类型文件**：`web/src/features/skills/types.ts`

`SkillHealthMetric` 新增字段：

```ts
route_hit_rate_7d: number;
route_hit_rate_30d: number;
```

`SkillHealthDailyMetric` 新增字段：

```ts
routed_count: number;
loaded_count: number;
```

**组件文件**：`web/src/components/skills/SkillHealthCard.vue`

- 增加 7d / 30d 路由命中率指标行
- `rate > 0` 时显示百分比与"加载/路由"标签
- `rate == 0` 时显示"-"与"暂无路由数据"（避免"0%"语义矛盾）

---

## 三、Phase 2：AI 优化渲染 + 按需注入

### 3.1 渲染模式

**文件**：`internal/skill/render/render.go`

`RenderOptions` 新增 `Mode string` 字段，支持两种模式：

| 常量 | 值 | 行为 |
|------|----|------|
| `ModeFull` | `"full"` | 默认，渲染完整 Name + Description + Body |
| `ModeAIOptimized` | `"ai_optimized"` | 压缩渲染，过滤非决策段落 |

`SkillGuidance(m, opts)` 根据 `opts.Mode` 分派到 `skillGuidanceFull` 或 `skillGuidanceAIOptimized`。

**`ai_optimized` 模式渲染规则**：

1. 保留 `Name`（`## {Name}` 标题）
2. 保留 `Description`，UTF-8 安全截断至 120 字符（`[]rune` 切片，超长取前 117 + `"..."`）
3. 保留 `Triggers`（`Triggers: a, b, c`）
4. 保留 `Tools`（`Tools: x, y, z`）
5. Body 仅保留 `## ` 开头的决策树段落，过滤介绍/背景/示例等段落

**段落过滤实现**：

- `filterDecisionSections(body)` 逐行扫描：
  - `## ` 开头：判断是否在排除列表，决定是否进入 `skipSection`
  - `# ` 开头：跳过（顶层标题，Name 已渲染）
  - 首个 `## ` 之前的行：丢弃（典型为介绍文本）
  - `skipSection=true` 时跳过该段所有行
- `isExcludedHeading(heading)`：
  - 精确匹配（小写比较）`aiOptimizedExcludeHeadings`
  - 前缀匹配：heading 以排除关键词开头且后跟分隔符（`空格` / `:` / `：` / `—` / `-`）

**排除标题列表**：

```go
var aiOptimizedExcludeHeadings = []string{
    "介绍", "背景", "概述", "简介", "overview", "introduction", "background",
    "完整步骤", "详细步骤", "示例", "example", "full steps",
    "changelog", "变更日志", "历史",
}
```

### 3.2 按需注入

**文件**：`internal/agent/skill_guidance_inject.go`

`newSkillGuidanceBeforeHook`（Full Profile 模式分支）行为调整：

1. 调用 `resolveAndWriteSkillState(ctx, ag.Settings, deps, false)` 解析路由
2. `BatchGetSkillGuidance` 批量获取路由到的 Skill guidance
3. 渲染时使用 `render.SkillGuidance(m, render.RenderOptions{Mode: render.ModeAIOptimized})`
4. 累计字符数，超过 `maxSkillGuidanceChars = 4000` 时截断并提示"... and N more skills (truncated)"
5. 当 `written < len(entries)` 时，注入尾部提示：
   > `Other available skills can be loaded on demand using the skill_load tool.`
6. 最终通过 `truncateAtMarkdownBoundary` 在 Markdown 边界（`\n### ` / `\n---` / `\n\n`）安全截断
7. 作为 `trpcmodel.NewSystemMessage` 插入到 `args.Request.Messages` 开头

**注意**：本模块未新增 `turn_optimized` load mode（原方案 B2 精简版），直接优化 Full Profile 注入逻辑，保持向后兼容。

---

## 四、Phase 3：Skill 入口可见性

### 4.1 前端类型补全

**文件**：`web/src/features/skills/types.ts`

新增类型（`Skill Catalog (Chat Integration)` 区段）：

```ts
export type SkillCatalogEntry = {
  slug: string;
  name: string;
  description: string;
  tags: string[];
};

export type SkillHint = {
  matched_skill: string;
  trigger: string;
  confidence: number;
};
```

### 4.2 组件状态

| 组件 | 路径 | 状态 |
|------|------|------|
| `ChatSkillCatalogStrip.vue` | `web/src/components/chat/ChatSkillCatalogStrip.vue` | 类型引用已修复，可编译；待后端事件集成 |
| `ChatSkillHintBar.vue` | `web/src/components/chat/ChatSkillHintBar.vue` | 类型引用已修复，可编译；待后端事件集成 |

**`ChatSkillCatalogStrip.vue` 设计**：

- Props：`skills: SkillCatalogEntry[]`
- 水平滚动卡片条，最多展示 `maxVisible = 8` 个，超出折叠
- 每张卡片显示：Skill 名称（slug）、描述（截断 80 字符）、标签（最多 3 个）
- 点击卡片 → `onSkillClick(skill)` 触发加载
- 已加载卡片视觉反馈：图标变为 `check_circle`，添加 `--loaded` class

**`ChatSkillHintBar.vue` 设计**：

- Props：`hint: SkillHint | null`
- 检测到 Skill trigger 时显示提示条："检测到可能需要 Skill: {matched_skill}（匹配: {trigger}）"
- 提供"加载"按钮与"关闭"按钮
- 使用 `hint-fade` transition

### 4.3 后端事件契约（待实施）

**`skill_catalog` WebSocket 事件**（未实施）：

- 触发时机：Agent 会话初始化时
- Payload：`{skills: [{slug, name, description, tags}]}`
- 数据来源：`skill.SummariesForContext()`，与 `SkillsRequestProcessor.injectOverview()` 同源

**`skill_hint` WebSocket 事件**（未实施，原方案 A3 已裁剪）：

- 触发时机：Layer B 路由命中 Skill 时
- Payload：`{matched_skill: slug, trigger: "关键词", confidence: 0.8}`

> 当前 Phase 3 仅完成前端类型与组件骨架，后端 WebSocket 事件待后续迭代实施。
