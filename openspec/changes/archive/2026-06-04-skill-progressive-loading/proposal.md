# Skill 渐进加载（Progressive Loading）

## Why

当前 Skill 加载采用 Eager 全量注入策略，存在严重的 token 浪费问题：

1. **L0 概览层**：`SkillsRequestProcessor.injectOverview` 在每个请求的 system prompt 中注入所有 Skill 的 name + description 概览行。40 个 Skill 约 ~3K token，这部分是合理的。
2. **L1 正文层（问题核心）**：`skill_guidance_inject.go` 的 `newSkillGuidanceBeforeHook` 在每次模型调用前，将所有已路由 Skill 的完整 guidance（经 `manifest.Parse` + `render.SkillGuidance` 渲染后的 SKILL.md 全文）注入 system prompt。即使 Agent 只需其中 1-2 个 Skill，所有被路由到的 Skill 正文都被全量注入，单次可达 ~15-30K token。
3. **L2 引用层**：`skill_load` 工具加载后，`SkillsRequestProcessor` 将 SKILL.md body + docs 全文追加到 system prompt。已加载的 Skill 在后续 turn 中持续占用上下文窗口（直到 load mode 触发卸载）。

**量化影响**：
- 40 个 Skill 的 L0 概览：~3K token/turn（合理）
- 被路由到的 5-10 个 Skill 的 L1 正文：~15-30K token/turn（浪费）
- 已加载 Skill 的 L2 body + docs：~5-20K token/turn（持续占用）

**参考**：Hermes 的 3 阶段渐进加载模型证明，按需加载可将 Skill 相关 token 消耗降低 50-80%。

## Goals

- 实现 Skill 3 阶段渐进加载：L0 manifest → L1 body → L2 refs
- L0：仅注入 Skill 名字 + 描述的紧凑 manifest（~3K token / 40 skill），保持当前行为
- L1：按 turn 查询特定 SKILL.md 正文，替代当前的全量 guidance 注入；LLM 通过 `skill_load` 工具按需获取
- L2：关联引用文件按需加载，替代当前的自动全量 docs 注入
- 目标：Skill 相关 token 消耗降低 50-80%
- 保持向后兼容：`skill_load` / `skill_select_docs` 工具行为不变
- 支持渐进式迁移：通过 `skill_load_mode` 配置切换 eager/progressive 模式

## Non-goals

- 不改变 `skill.Repository` 框架接口（`Summaries()` / `Get()` / `Path()`）
- 不改变 Skill 的 CRUD / 发布 / 启用管理流程
- 不实现 Skill 正文的 embedding 索引（L1 路由仍基于现有 intent + tag 机制）
- 不改变 `skill_load` / `skill_select_docs` 工具的对外协议
- 不涉及前端 UI 变更（本变更纯后端运行时优化）
