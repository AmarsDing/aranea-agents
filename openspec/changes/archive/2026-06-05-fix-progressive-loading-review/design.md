## Context

Skill Progressive Loading 变更（归档于 `openspec/changes/archive/2026-06-04-skill-progressive-loading/`）已实现并上线。代码复检发现实现偏离了设计文档，存在两个阻断级问题和多个建议级问题。

**当前状态**：

1. `newProgressiveSkillGuidanceHook`（`internal/agent/skill_guidance_inject.go:91-133`）在 progressive 模式下仍调用 `BatchGetSkillGuidance` 全量获取所有被路由 Skill 的完整 guidance 正文，但仅使用 `manifest.Parse` 提取 Name/Description。design.md 明确说"BatchGetSkillGuidance 方法不再被调用"。
2. `IsProgressiveSkillLoad`（`internal/biz/skill_load_mode.go:10`）使用精确匹配 `mode == "progressive"`，而框架层 `normalizeSkillLoadMode` 使用 `strings.ToLower(strings.TrimSpace(mode))` 做归一化。
3. `GetSkillLoadMode` 默认返回 `"eager"`，但框架层不识别 "eager"，会 fallback 到 "turn"。
4. progressive hook 注入的 system message（Name+Description 列表）与 `SkillsRequestProcessor.injectOverview` 已注入的 L0 概览信息冗余。
5. progressive 模式下 `skillProfile` 被强制设为 `SkillToolProfileKnowledgeOnly`，禁用了 `skill_run`/`skill_exec`，但 design.md 未说明此限制。

## Goals / Non-Goals

**Goals:**

- 修复 P-01：progressive hook 不再全量调用 `BatchGetSkillGuidance`，改用轻量接口仅获取 Skill 元数据
- 修复 P-02：`IsProgressiveSkillLoad` 改为大小写不敏感匹配，与框架层归一化策略一致
- 统一 `GetSkillLoadMode` 默认值语义，消除 "eager" vs "turn" 不一致
- 消除 progressive hook 与 processor `injectOverview` 的信息冗余
- 补充 progressive 模式下工具集限制的设计说明

**Non-Goals:**

- 不改变 `skill.Repository` 框架接口
- 不改变 `skill_load` / `skill_select_docs` 工具的对外协议
- 不实现 Skill 正文的 embedding 索引
- 不涉及前端 UI 变更
- 不改变 progressive 模式下 `SkillToolProfileKnowledgeOnly` 的行为（仅补充文档说明）

## Decisions

### D1: 轻量元数据查询 — 复用 `skill.Repository.Summaries()` 而非新增 biz 接口

**选择**：在 `newProgressiveSkillGuidanceHook` 中，通过 `deps.SkillUC.ListEnabledPublishedSkillKeys` 获取已启用 Skill 列表，再结合 `ResolveSkillSlugsDetailed` 的路由结果，直接从 `skill.Repository.Summaries()` 获取 Name+Description。

**理由**：
- `skill.Repository.Summaries()` 已返回 `[]skill.Summary{Name, Description}`，正是所需元数据
- 无需新增 biz 层接口，减少变更面
- `BatchGetSkillGuidance` 加载完整 Body + `manifest.Parse` 的开销被完全消除

**替代方案**：
- 在 `SkillUsecase` 新增 `GetSkillSummaries(slugs)` 方法 → 过度设计，`Summaries()` 已满足需求
- 在 hook 中直接调用 `repo.Summaries()` → hook 在 agent 层，不应直接访问框架 Repository，应通过 SkillUC 间接获取

**最终方案**：在 `SkillUsecase` 新增 `BatchGetSkillSummaries(ctx, slugs) []SkillSummary` 方法，内部调用 `repo.Summaries()` 并按 slugs 过滤。返回纯 biz 层模型 `SkillSummary{Name, Description}`，不暴露框架类型。

### D2: `IsProgressiveSkillLoad` 改为大小写不敏感

**选择**：改为 `strings.EqualFold(strings.TrimSpace(mode), SkillLoadModeProgressive)`

**理由**：与框架层 `normalizeSkillLoadMode` 的 `strings.ToLower(strings.TrimSpace(mode))` 策略一致，确保 `"Progressive"` / `"PROGRESSIVE"` 等变体在 biz 层也能正确识别。

### D3: `GetSkillLoadMode` 默认值统一为空字符串

**选择**：`GetSkillLoadMode` 在 `SkillLoadMode` 为空时返回 `""` 而非 `"eager"`，由框架层 `normalizeSkillLoadMode` 归一化为 `"turn"`

**理由**：
- 删除 `SkillLoadModeEager` 常量，消除 biz 层与框架层默认值语义不一致
- 框架层已有完善的归一化逻辑，biz 层无需定义自己的默认值
- 空字符串表示"未设置"，由框架决定默认行为，更符合分层原则

### D4: 消除 progressive hook 与 processor 的信息冗余

**选择**：`newProgressiveSkillGuidanceHook` 不再注入包含 Name+Description 列表的 system message，仅将 routed slugs 写入 invocation state（已有 `RoutedSkillsStateKey`）。由 `SkillsRequestProcessor.injectOverview` 统一负责展示 Skill 概览（含 `[routed]` 标记）。

**理由**：
- `injectOverview` 已有完整的 Skill 概览展示逻辑和 `[routed]` 标记
- hook 注入的额外 system message 与 processor 注入的概览信息重复
- 简化 progressive hook 为纯状态写入，职责更清晰

### D5: progressive 模式工具集限制 — 仅补充文档说明

**选择**：不改变 `SkillToolProfileKnowledgeOnly` 行为，在 design 文档中补充说明 progressive 模式下 `skill_run`/`skill_exec` 不可用的限制。

**理由**：
- progressive 模式的核心目标是减少 token 消耗，知识型工具集（load/doc）已满足此目标
- 如需运行型工具，用户可回退到 `turn` 模式
- 此限制为有意的架构决策，非 bug

## Risks / Trade-offs

- **[Risk] `BatchGetSkillSummaries` 新方法可能被其他调用方误用** → 方法名明确表达"仅返回摘要"语义，且返回 biz 层模型而非框架类型
- **[Risk] 删除 `SkillLoadModeEager` 可能影响已有代码引用** → 搜索确认仅有 `GetSkillLoadMode` 内部使用，影响面可控
- **[Risk] progressive hook 不注入 system message 后，LLM 可能无法感知哪些 Skill 可用** → `injectOverview` 已注入完整概览 + `[routed]` 标记 + capability guidance 提示 `skill_load`，信息充分
- **[Trade-off] `BatchGetSkillSummaries` 通过 `repo.Summaries()` 获取全量摘要再过滤，仍有全量读取开销** → `Summaries()` 返回的数据量远小于 `BatchGetSkillGuidance`（仅 Name+Description vs 完整 Body），且结果可缓存，开销可接受
