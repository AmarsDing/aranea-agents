# Skill Progressive Loading 复检修复 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 修复 Skill Progressive Loading 代码复检发现的 2 个阻断级问题和 8 个建议级问题，确保渐进加载实现与设计文档一致。

**Architecture:** `newProgressiveSkillGuidanceHook` 改为仅写状态不注入 system message（不需要 BatchGetSkillSummaries）；`IsProgressiveSkillLoad` 改为大小写不敏感；`GetSkillLoadMode` 默认值统一为空字符串。

**Tech Stack:** Go + trpc-agent-go 框架 + Kratos v2 + Wire DI

**Non-goals:**
- 不改变 `skill.Repository` 框架接口
- 不改变 `skill_load` / `skill_select_docs` 工具的对外协议
- 不改变 progressive 模式下 `SkillToolProfileKnowledgeOnly` 的行为
- 不涉及前端 UI 变更

---

## File Structure

| 操作 | 文件 | 职责 |
|------|------|------|
| Modify | `internal/biz/skill_load_mode.go` | IsProgressiveSkillLoad 改为大小写不敏感；删除 SkillLoadModeEager |
| Modify | `internal/biz/agent_settings.go` | GetSkillLoadMode 默认值改为空字符串 |
| Modify | `internal/agent/skill_guidance_inject.go` | progressive hook 不再调用 BatchGetSkillGuidance，不再注入 system message |
| Add | `internal/agent/skill_guidance_inject_test.go` | progressive hook 行为单测 |

---

## 1. biz 层修复 — IsProgressiveSkillLoad + GetSkillLoadMode

- [x] 1.1 修改 `IsProgressiveSkillLoad` 为大小写不敏感匹配

在 `internal/biz/skill_load_mode.go` 中：
- 删除 `SkillLoadModeEager = "eager"` 常量
- 修改 `IsProgressiveSkillLoad` 为 `strings.EqualFold(strings.TrimSpace(mode), SkillLoadModeProgressive)`
- 添加 `import "strings"`

**DoD:** `IsProgressiveSkillLoad("Progressive")` 返回 `true`；`IsProgressiveSkillLoad(" progressive ")` 返回 `true`

- [x] 1.2 修改 `GetSkillLoadMode` 默认值为空字符串

在 `internal/biz/agent_settings.go` 中：
- `GetSkillLoadMode()` 在 `SkillLoadMode` 为空或 nil 时返回 `""` 而非 `SkillLoadModeEager`
- 删除对 `SkillLoadModeEager` 的引用

**DoD:** `GetSkillLoadMode()` 在空值时返回 `""`；框架层 `normalizeSkillLoadMode("")` 归一化为 `"turn"`

- [x] 1.3 验证编译通过

Run: `go build ./internal/biz/...`

---

## 2. ~~biz 层新增 — BatchGetSkillSummaries~~ → 已跳过

> **设计决策 D4 变更**：progressive hook 不再注入 system message（仅写 routed slugs 到 state），因此不需要 Name+Description 元数据，`BatchGetSkillSummaries` 方法不再需要。`injectOverview` 已有完整的 Skill 概览展示逻辑和 `[routed]` 标记。

- [x] ~~2.1 定义 `SkillSummary` biz 模型~~ → 跳过
- [x] ~~2.2 在 `SkillUsecase` 新增 `BatchGetSkillSummaries` 方法~~ → 跳过
- [x] ~~2.3 编写 `BatchGetSkillSummaries` 单测~~ → 跳过
- [x] ~~2.4 验证编译通过~~ → 跳过

---

## 3. agent 层修复 — progressive hook 简化

- [x] 3.1 修改 `newProgressiveSkillGuidanceHook` 不再调用 `BatchGetSkillGuidance`

在 `internal/agent/skill_guidance_inject.go` 中，将 `newProgressiveSkillGuidanceHook` 简化为：

```go
func newProgressiveSkillGuidanceHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
    return callbacks.NewBeforeModelHook(5, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
        if args == nil || args.Request == nil {
            return &trpcmodel.BeforeModelResult{Context: ctx}, nil
        }
        runtime := ag.Settings
        opts := &skillruntime.SkillToolsetOptions{Runtime: runtime, UserQuery: skillruntime.TurnQueryFromContext(ctx)}
        result, err := skillruntime.ResolveSkillSlugsDetailed(ctx, deps.SkillUC, opts, deps.Logger())
        if err != nil || len(result.Slugs) == 0 {
            return &trpcmodel.BeforeModelResult{Context: ctx}, nil
        }
        // Store routed skill names in invocation state so the
        // SkillsRequestProcessor can mark them as [routed].
        if inv, ok := trpcagent.InvocationFromContext(ctx); ok {
            inv.SetState(trpcllmagent.RoutedSkillsStateKey, result.Slugs)
            inv.SetState(skillSelectionReasonStateKey, result.Reasons)
        }
        return &trpcmodel.BeforeModelResult{Context: ctx}, nil
    })
}
```

关键变更：
- 移除 `BatchGetSkillGuidance` 调用
- 移除 `manifest.Parse` / `render.SkillGuidance` 调用
- 移除 system message 注入逻辑
- 仅保留 routed slugs + selection reasons 写入 invocation state

- [x] 3.2 编写 progressive hook 单测

覆盖场景：
- progressive 模式下 hook 不注入 system message
- progressive 模式下 hook 将 routed slugs 写入 invocation state
- progressive 模式下 hook 不调用 `BatchGetSkillGuidance`
- 非 progressive 模式行为不变

- [x] 3.3 验证编译通过

Run: `go build ./internal/agent/...`

---

## 4. 全量验证

- [x] 4.1 后端全量验证

Run: `make api && make wire && make build && make test && make lint`

- [x] 4.2 重点回归测试

1. 创建 Agent，设置 `skill_load_mode = "progressive"`
2. 发送消息，验证 system prompt 中不包含 Skill guidance 正文
3. 验证 system prompt 中 Skill 概览包含 `[routed]` 标记
4. 验证 LLM 调用 `skill_load` 后能获取 Skill 正文
5. 将 `skill_load_mode` 改回 `turn`，验证 guidance 全量注入恢复
6. 测试 `skill_load_mode = "Progressive"`（大写）是否正确识别为 progressive 模式

**DoD:**
- `make api && make wire && make build && make test && make lint` 全部通过
- progressive 模式下 `BatchGetSkillGuidance` 不被调用
- progressive 模式下 hook 不注入 system message
- `IsProgressiveSkillLoad("Progressive")` 返回 `true`
- `GetSkillLoadMode()` 空值时返回 `""`
- 非 progressive 模式行为不变
