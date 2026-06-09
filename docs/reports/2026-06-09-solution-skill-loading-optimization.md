# Skill 加载策略优化方案

> **类型**: solution
> **日期**: 2026-06-09
> **来源**: [Claude Code Skill 加载策略面试题分析](https://www.toutiao.com/article/7648721612249776686/) + 项目 Skill 系统现状审计
> **状态**: proposal

---

## 一、背景与问题

### 1.1 外部洞察

一篇广泛传播的文章指出 AI Agent Skill 系统的三个核心问题：

1. **全量加载浪费 Token** — Transformer 注意力"前重后轻"（Lost in the Middle, 2023），200K 窗口中有效注意力仅约 32K，中段内容处于"死亡区"
2. **Skill 当 README 写** — Skill 应该是"AI 看见就知道下一步该调什么工具"的极简触发器，而非给人看的文档
3. **入口可见性** — Skill 的第一设计原则不是"AI 怎么调"，而是"人类怎么知道有这个东西"

### 1.2 项目现状审计

| 维度 | 现状 | 差距 |
|------|------|------|
| 加载策略 | 已有 Progressive 渐进式模式（L0→L1→L2），但传统 Full Profile 模式仍每轮全量注入 | 传统模式无按需机制 |
| 路由 | 已有两层路由（Layer A 策略过滤 + Layer B 意图/标签/Embedding/历史排名） | 无路由命中率可观测性 |
| 内容渲染 | `render.SkillGuidance()` 直接输出 Name+Description+Body 全文 | 无 AI 优化渲染模式 |
| 入口可见性 | 聊天 UI 中 Skill 仅以 `ChatExecutionCard` 活动卡片形式展示（被动），无主动摘要 | 用户不知道有哪些 Skill 可用 |
| Token 量化 | `skill_invocation.token_usage` 已记录 prompt/completion/total | 未用于路由效果分析 |
| 健康指标 | `SkillHealthDetail` 聚合成功率+P95延迟 | 未包含路由命中率、Token 效率 |

---

## 二、解决方案

### 方案 A：Skill 入口可见性增强（前端）

#### A1：会话开始时展示 Skill 摘要卡片

**目标**：用户打开聊天时，即可看到当前 Agent 可用的 Skill 列表。

**实现路径**：

1. **后端**：在 Agent 会话初始化时（`builder_deps.go` 的 Agent 构建流程），将当前 Agent 的 `skill_runtime_json` 中允许的 Skill slug 列表及其摘要（name + description）写入会话元数据（Session State 或专用 key）
2. **WebSocket 事件**：会话创建时发送 `skill_catalog` 事件，payload 为 `{skills: [{slug, name, description, tags}]}`
3. **前端**：新增 `ChatSkillCatalogStrip` 组件，在聊天输入框上方以水平滚动卡片条展示 Skill 摘要，每张卡片仅显示：
   - Skill 名称（slug）
   - 一句话描述（description，截断至 80 字符）
   - 标签（tags，最多 3 个）
4. **交互**：点击卡片 → 前端发送 `skill_load` 工具调用消息 → 后端加载 Skill 内容

**关键约束**：
- 摘要信息从 `skill.SummariesForContext()` 获取，与 `SkillsRequestProcessor.injectOverview()` 同源，不增加额外 DB 查询
- 卡片条高度固定（48px），不占用聊天内容区空间
- 卡片数量超过 8 个时折叠，点击展开

#### A2：用户点击卡片主动触发 Skill 加载

**实现路径**：

1. 点击卡片 → 前端构造一条用户消息，内容为 `请加载 Skill: {slug}`
2. 后端 `skill_load` 工具识别该意图，加载对应 Skill
3. 加载完成后，卡片状态变为"已加载"（视觉反馈：图标变色或加勾）

**备选方案**（更轻量）：点击卡片 → 前端直接在聊天输入框中插入 `/skill {slug}` 文本，用户确认后发送。

#### A3：用户输入匹配 Skill trigger 时给出轻量提示

**实现路径**：

1. **后端**：在 `resolveAndWriteSkillState()` 中，当 Layer B 路由命中某个 Skill 时，将 `result.Reasons` 中匹配的 trigger 关键词写入 Invocation State
2. **WebSocket 事件**：发送 `skill_hint` 事件，payload 为 `{matched_skill: slug, trigger: "关键词", confidence: 0.8}`
3. **前端**：在聊天输入框上方显示一个轻量提示条（类似搜索建议）："检测到可能需要 Skill: {name}，点击加载"
4. **交互**：用户可点击加载，也可忽略（3 秒后自动消失）

**关键约束**：
- 提示仅在 Progressive 模式下触发（Full Profile 模式下 Skill 已全量注入，无需提示）
- 每轮对话最多提示 1 个 Skill，避免干扰
- 提示不自动加载，需用户确认

---

### 方案 B：AI 优化渲染模式（后端）

#### B1：render.SkillGuidance 增加 AI-optimized 输出

**目标**：将 Skill 内容从"给人看的文档"转为"给 AI 看的极简触发器"。

**实现路径**：

1. 在 `render.RenderOptions` 中增加 `Mode string` 字段，支持 `"full"` 和 `"ai_optimized"` 两种模式
2. `ai_optimized` 模式的渲染逻辑：
   - 保留：`Name`、`Description`（截断至 120 字符）、`Triggers`、`Tools`、`Variables`
   - 保留：Body 中以 `##` 开头的决策树段落（如 `## 当用户要求...时`、`## 判断条件`）
   - 过滤：Body 中的"介绍"、"背景"、"完整步骤"、"示例"等段落（按标题关键词匹配）
   - 过滤：超过 3 层嵌套的子标题内容
3. 在 `manifest.Manifest` 中增加 `AiSummary string` 字段（Front Matter 中的 `ai_summary` 键），允许 Skill 作者提供专给 AI 看的极简指令
4. 渲染优先级：`AiSummary` > `ai_optimized` 模式提取 > `full` 模式全文

**段落过滤规则**（`ai_optimized` 模式）：

```go
var aiOptimizedExcludeHeadings = []string{
    "介绍", "背景", "概述", "简介", "overview", "introduction", "background",
    "完整步骤", "详细步骤", "示例", "example", "full steps",
    "changelog", "变更日志", "历史",
}
```

**预期效果**：Skill Guidance 平均 token 数降低 40-60%，从当前平均 2K token 降至 0.8-1.2K token。

#### B2：传统模式按轮次按需注入

**目标**：Full Profile 模式下，不再每轮全量注入所有路由到的 Skill，而是只注入与当前用户意图匹配的 Skill。

**实现路径**：

1. 在 `newSkillGuidanceBeforeHook()` 中，将 `resolveAndWriteSkillState()` 的 `progressive` 参数改为 `true`（即总是执行路由并写入 RoutedSkillsStateKey）
2. 修改注入逻辑：只注入路由结果中 score 最高的前 N 个 Skill（N 由 `maxSkillGuidanceChars` 隐式决定）
3. 注入时使用 `ai_optimized` 渲染模式，进一步压缩 token
4. 在 System Message 中增加提示："其他可用 Skill 请使用 skill_load 工具按需加载"

**关键约束**：
- 这是 Full Profile 模式的行为变更，需通过 `skill_load_mode` 配置项控制，保持向后兼容
- 新增 `SkillLoadModeTurnOptimized = "turn_optimized"` 模式，作为 Full Profile 的优化版本
- 默认仍使用原有行为，用户需显式选择新模式

---

### 方案 C：路由准确率可观测性（后端）

#### C1：路由命中率指标

**目标**：量化"Skill 被路由加载后是否真正被使用"。

**定义**：

```
路由命中率 = Skill 实际被调用次数 / Skill 被路由加载次数
```

**实现路径**：

1. **数据采集**：
   - 在 `resolveAndWriteSkillState()` 中，将路由结果（`result.Slugs` + `result.Reasons`）写入 Invocation State（已有 `skillSelectionReasonStateKey`）
   - 在 `SkillUsageTrackerPlugin.afterTool()` 中，当检测到 `skill_load` / `skill_run` 调用时，记录实际使用的 Skill slug
   - 在 `newTokenUsageAccumulatorAfterHook()` 中，将 token 快照写入 Invocation State（已有 `skillTokenUsageStateKey`）

2. **数据写入**：
   - 在 Invocation 结束时（AfterInvocation hook），读取 Invocation State 中的路由结果和实际使用记录
   - 扩展 `skill_invocation` 表：增加 `routed_slugs` (JSON) 和 `loaded_slug` (String) 字段
     - `routed_slugs`：本轮路由到的所有 Skill slug（可能多个）
     - `loaded_slug`：本轮实际通过 skill_load 加载的 Skill slug（通常 0-1 个）
   - 或更轻量方案：在现有 `selection_reason` JSON 中增加 `routed_slugs` 和 `loaded_slug` 字段

3. **聚合查询**：
   - 扩展 `SkillHealthDetail`：增加 `RouteHitRate7d`、`RouteHitRate30d` 字段
   - 在 `internal/data/skill_health.go` 的聚合查询中，计算：
     - 分子：`loaded_slug` 非空且等于目标 skill_id 的记录数
     - 分母：`routed_slugs` 包含目标 skill_id 的记录数
   - 新增 `DailyMetric.RouteHitRate float64` 字段

4. **前端展示**：
   - 在 `SkillHealthCard` 中增加"路由命中率"指标行
   - 在 `SkillStatsStrip` 中增加命中率小图标

#### C2：Token 效率指标

**目标**：量化 Skill 注入的 Token 是否被有效利用。

**定义**：

```
Token 效率 = Skill 实际被引用的 Token 数 / Skill 注入的 Token 数
```

**实现路径**：

1. **注入 Token 计量**：
   - 在 `newSkillGuidanceBeforeHook()` 注入 Skill Guidance 后，计算注入内容的字符数（已有 `totalChars` 变量）
   - 将字符数写入 Invocation State：`skillInjectedCharsStateKey`
   - 粗略估算：字符数 / 4 ≈ Token 数（英文），字符数 / 2 ≈ Token 数（中文混合）

2. **实际引用计量**：
   - 在 `SkillUsageTrackerPlugin.afterTool()` 中，当 `skill_load` 被调用时，记录加载的 Skill body 长度
   - 当 `skill_run` 被调用时，记录为"Skill 被实际使用"

3. **聚合指标**：
   - 扩展 `SkillHealthDetail`：增加 `AvgInjectedTokens7d`、`AvgInjectedTokens30d` 字段
   - 扩展 `DailyMetric`：增加 `AvgInjectedTokens float64` 字段

---

### 方案 D：渐进式加载效果量化验证（后期验证）

#### D1：Progressive vs Full Profile A/B 对比

**目标**：用数据验证渐进式加载的实际收益。

**验证方法**：

1. **实验设计**：
   - 选取 2 个相似场景（如 `softwaredev` 的两个子场景）
   - 场景 A 使用 `skill_load_mode: "progressive"`
   - 场景 B 使用 `skill_load_mode: "turn"`（传统模式）
   - 运行相同任务集（10-20 个典型任务），记录指标

2. **对比指标**：

| 指标 | 数据来源 | 计算方式 |
|------|----------|----------|
| 任务完成率 | `skill_invocation.outcome` | `outcome=success` 的比例 |
| 平均 Token 消耗 | `skill_invocation.token_usage` | `total` 字段的平均值 |
| Skill 加载命中率 | 方案 C1 新增 | 实际使用 / 路由加载 |
| 平均响应轮次 | Session 事件流 | 从用户消息到最终回复的模型调用次数 |
| 用户满意度 | 主观评分 | 任务完成后用户评分（1-5） |

3. **预期结果**：
   - Progressive 模式：Token 消耗降低 50-70%，任务完成率持平或略降（因多一轮 skill_load）
   - Full Profile 模式：Token 消耗高，但首轮响应更快（无需额外 skill_load 调用）

4. **验证时间线**：标记为后期验证，待方案 A/B/C 落地后执行

#### D2：Lost in the Middle 效应验证

**目标**：验证 Skill 内容在 System Message 中的位置是否影响 LLM 的实际使用率。

**验证方法**：

1. **实验设计**：
   - 在 Full Profile 模式下，将 Skill Guidance 注入位置从 System Message 末尾改为开头
   - 对比两种位置下的 Skill 加载命中率
   - 逐步增加注入 Skill 数量（3/5/8/10），观察命中率变化曲线

2. **关键假设**：
   - 如果 Skill 在 System Message 开头，命中率应更高（Transformer 前重后轻）
   - 当注入 Skill 数量超过 5 个时，命中率应显著下降（注意力衰减）

3. **验证时间线**：标记为后期验证，需积累足够数据后执行

---

## 三、方案评估

### 3.1 可行性评估

| 方案 | 技术可行性 | 依赖 | 复杂度 |
|------|-----------|------|--------|
| A1 Skill 摘要卡片 | 高 | WebSocket 事件扩展、前端新组件 | 中 |
| A2 点击加载 | 高 | 前端交互 + skill_load 工具调用 | 低 |
| A3 Trigger 提示 | 中 | 需要新增 skill_hint 事件类型 | 中 |
| B1 AI 优化渲染 | 高 | render.go 修改，无外部依赖 | 低 |
| B2 按需注入 | 中 | 需新增 load_mode，向后兼容 | 中 |
| C1 路由命中率 | 高 | skill_invocation 扩展字段 | 中 |
| C2 Token 效率 | 中 | 需精确 Token 计量（tiktoken 或 API 返回） | 中 |
| D1 A/B 对比 | 高 | 依赖 C1/C2 数据采集 | 低（纯分析） |
| D2 Lost in Middle | 中 | 需控制注入位置变量 | 低（纯分析） |

### 3.2 成本评估

| 方案 | 后端改动量 | 前端改动量 | 数据库变更 | 运维成本 |
|------|-----------|-----------|-----------|---------|
| A1 | 小（事件发送） | 中（新组件+Store） | 无 | 低 |
| A2 | 无（复用 skill_load） | 小（交互逻辑） | 无 | 低 |
| A3 | 中（新事件类型） | 中（提示条组件） | 无 | 低 |
| B1 | 小（render.go） | 无 | 无 | 低 |
| B2 | 中（新 load_mode） | 无 | 无 | 低 |
| C1 | 中（字段扩展+聚合） | 小（UI 展示） | 小（2 字段） | 低 |
| C2 | 中（Token 计量） | 小（UI 展示） | 无 | 低 |
| D1 | 无（纯分析） | 无 | 无 | 低 |
| D2 | 小（注入位置控制） | 无 | 无 | 低 |

### 3.3 风险评估

| 方案 | 风险 | 缓解措施 |
|------|------|----------|
| A1 | Skill 数量过多时卡片条拥挤 | 折叠机制 + 最多展示 8 个 |
| A2 | 用户误触加载无关 Skill | 加载前确认弹窗（可选） |
| A3 | 提示频繁出现干扰用户 | 每轮最多 1 次 + 3 秒自动消失 + 可关闭 |
| B1 | AI 优化渲染过滤掉关键信息 | 保留决策树段落 + AiSummary 优先级最高 |
| B2 | 新 load_mode 与现有行为不一致 | 默认关闭，显式选择 + 充分测试 |
| C1 | 路由命中率计算需要关联 routed_slugs 和 loaded_slug | 使用 session_id + created_at 关联 |
| C2 | Token 计量精度受模型分词器影响 | 使用 API 返回的 usage 数据而非估算 |
| D1 | A/B 实验样本量不足 | 至少 20 个任务 × 3 次重复 |
| D2 | Lost in Middle 效应可能因模型而异 | 多模型对比（GPT-4o/Claude/Gemini） |

### 3.4 优先级排序

| 优先级 | 方案 | 理由 |
|--------|------|------|
| P0 | B1 AI 优化渲染 | 改动最小、收益最直接、无外部依赖 |
| P0 | C1 路由命中率 | 为后续所有优化提供数据基础 |
| P1 | A1 Skill 摘要卡片 | 解决"入口可见性"核心问题 |
| P1 | B2 按需注入 | 在 B1 基础上进一步压缩 Token |
| P2 | A2 点击加载 | A1 的自然延伸 |
| P2 | C2 Token 效率 | 需要精确计量，依赖 C1 基础设施 |
| P2 | A3 Trigger 提示 | 体验优化，非刚需 |
| P3 | D1 A/B 对比 | 后期验证，依赖 C1/C2 数据 |
| P3 | D2 Lost in Middle | 后期验证，学术性质 |

### 3.5 实施路线图

```
Phase 1（基础优化 + 可观测性）
├── B1: AI 优化渲染模式
├── C1: 路由命中率指标
└── C1 前端展示

Phase 2（入口可见性）
├── A1: Skill 摘要卡片
├── A2: 点击加载
└── B2: 按需注入模式

Phase 3（体验增强 + 深度分析）
├── A3: Trigger 提示
├── C2: Token 效率指标
└── D1: Progressive vs Full Profile 对比

Phase 4（后期验证）
└── D2: Lost in Middle 效应验证
```

---

## 四、关键代码变更索引

| 变更 | 文件 | 说明 |
|------|------|------|
| AI 优化渲染 | `internal/skill/render/render.go` | 增加 `Mode` 字段和过滤逻辑 |
| Manifest 扩展 | `internal/skill/manifest/manifest.go` | 增加 `AiSummary` 字段 |
| 按需注入 | `internal/agent/skill_guidance_inject.go` | 新增 `turn_optimized` 模式 |
| 路由命中记录 | `internal/agent/skill_guidance_inject.go` | 写入 routed_slugs 到 State |
| Invocation 扩展 | `internal/data/ent/schema/skill_invocation.go` | 增加 routed_slugs/loaded_slug |
| 健康指标扩展 | `internal/biz/types/skill_health.go` | 增加 RouteHitRate 字段 |
| 健康聚合扩展 | `internal/data/skill_health.go` | 增加命中率计算 |
| Skill 目录事件 | `internal/agent/builder_deps.go` | 会话初始化时发送 catalog |
| 前端摘要卡片 | `web/src/components/chat/ChatSkillCatalogStrip.vue` | 新组件 |
| 前端提示条 | `web/src/components/chat/ChatSkillHintBar.vue` | 新组件 |
| 前端健康卡片 | `web/src/components/skills/SkillHealthCard.vue` | 增加命中率行 |
| 前端类型 | `web/src/features/skills/types.ts` | 增加新字段类型 |

---

## 五、验收标准

### Phase 1 验收

- [ ] `render.SkillGuidance()` 在 `ai_optimized` 模式下，输出 token 数降低 40%+（对比 `full` 模式）
- [ ] `skill_invocation` 表新增 `routed_slugs` 和 `loaded_slug` 字段，数据正确写入
- [ ] `SkillHealthDetail` 包含 `RouteHitRate7d` 和 `RouteHitRate30d` 字段
- [ ] `SkillHealthCard` 展示路由命中率指标

### Phase 2 验收

- [ ] 聊天界面展示 Skill 摘要卡片条，数据与 `injectOverview()` 同源
- [ ] 点击卡片可触发 `skill_load`，卡片状态变为"已加载"
- [ ] `turn_optimized` 模式下，注入 token 数相比 `turn` 模式降低 50%+
- [ ] 任务完成率不低于 `turn` 模式的 95%

### Phase 3 验收

- [ ] 用户输入匹配 Skill trigger 时，提示条正确出现
- [ ] 提示条 3 秒自动消失，每轮最多 1 次
- [ ] Token 效率指标在 `SkillHealthCard` 中展示

### Phase 4 验收

- [ ] Progressive vs Full Profile 对比报告完成
- [ ] Lost in Middle 效应验证报告完成
