## Why

Skill Progressive Loading 变更已归档并上线，但代码复检发现两个阻断级问题和多个建议级问题：

1. **P-01（阻断）**：`newProgressiveSkillGuidanceHook` 仍全量调用 `BatchGetSkillGuidance` 获取所有被路由 Skill 的完整 guidance 正文，但仅使用 Name/Description 元数据。这导致每次 turn 仍全量加载 SKILL.md 全文，违背渐进加载"按需获取"的核心设计意图，浪费 I/O 和 CPU。
2. **P-02（阻断）**：`IsProgressiveSkillLoad` 使用精确匹配（`mode == "progressive"`），而框架层 `normalizeSkillLoadMode` 使用 `strings.ToLower(strings.TrimSpace(mode))` 做归一化。大小写变体（如 `"Progressive"`）在 biz 层无法识别，但在框架层可正常归一化，导致行为不一致。

此外还有 8 个建议级问题需要处理：默认值语义不一致、progressive 模式下 skill_run 被禁用未在设计中说明、hook 注入与 processor 概览信息冗余、常量跨层无编译期保障等。

## What Changes

- 将 `newProgressiveSkillGuidanceHook` 改为仅调用轻量接口获取 Skill 元数据（Name+Description），不再调用 `BatchGetSkillGuidance`
- 修复 `IsProgressiveSkillLoad` 为大小写不敏感匹配，与框架层归一化策略保持一致
- 统一 `GetSkillLoadMode` 默认值语义，消除 biz 层 "eager" 与框架层 "turn" 的不一致
- 消除 progressive hook 与 processor `injectOverview` 的信息冗余
- 补充 progressive 模式下 `SkillToolProfileKnowledgeOnly` 限制的设计说明

## Capabilities

### New Capabilities

- `skill-metadata-query`: 轻量 Skill 元数据查询能力，仅返回 Name+Description 而不加载完整 Body

### Modified Capabilities

- `skill-progressive-loading`: 修复阻断级问题——hook 不再全量加载 guidance、大小写匹配一致化、默认值语义统一

## Impact

- **biz 层**：`skill_load_mode.go`（IsProgressiveSkillLoad 修复）、`agent_settings.go`（GetSkillLoadMode 默认值）
- **agent 层**：`skill_guidance_inject.go`（progressive hook 改用轻量接口、消除信息冗余）
- **biz 层**：`skill_usecase.go` 或新增接口（轻量元数据查询方法）
- **框架层**：无变更（`skills.go` 已正确支持）
- **Proto/API**：无变更
- **前端**：无变更
