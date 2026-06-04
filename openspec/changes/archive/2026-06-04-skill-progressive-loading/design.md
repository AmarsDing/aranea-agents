# Skill 渐进加载设计

> 日期：2026-06-02
> 状态：Draft
> 范围：Skill Prompt 注入策略从 Eager 全量改为 3 阶段渐进加载

***

## 一、背景与现状分析

### 1.1 当前 Skill Prompt 注入链路

当前系统存在两条并行的 Skill Prompt 注入路径：

**路径 A — 框架层 `SkillsRequestProcessor`**（`pkg/trpc-agent-go/internal/flow/processor/skills.go`）：

| 阶段 | 行为 | 注入位置 | Token 成本 |
|------|------|----------|-----------|
| Overview | `injectOverview`：遍历 `repo.Summaries()`，为每个 Skill 生成 `- name: description` 行 | system prompt 前部 | ~3K / 40 skill |
| Loaded Body | 遍历 session state 中已 `skill_load` 的 Skill，调用 `repo.Get(name)` 获取完整 `Skill.Body` | system prompt 尾部 | ~5-20K / 已加载 skill |
| Docs | 读取 `skill_select_docs` 选中的 doc 文件内容 | system prompt 尾部 | ~2-10K / 选中 docs |

**路径 B — 应用层 `newSkillGuidanceBeforeHook`**（`internal/agent/skill_guidance_inject.go`）：

| 阶段 | 行为 | 注入位置 | Token 成本 |
|------|------|----------|-----------|
| Guidance | `ResolveSkillSlugsDetailed` → `BatchGetSkillGuidance` → `manifest.Parse` + `render.SkillGuidance` 渲染所有被路由 Skill 的完整正文 | system prompt 前部（prepend） | ~15-30K / 被路由 skill |

**核心问题**：路径 B 是最大的 token 浪费源。即使 Agent 在当前 turn 只需 1-2 个 Skill，所有被路由到的 Skill（通常 5-10 个）的完整 guidance 都被注入 system prompt。这本质上是在 L0 阶段就注入了 L1 内容。

### 1.2 当前数据流

```
用户消息
  → ResolveSkillSlugsDetailed (Layer A allow/deny + Layer B intent/tag)
    → BatchGetSkillGuidance (获取所有被路由 Skill 的 SKILL.md 全文)
      → manifest.Parse + render.SkillGuidance (渲染正文)
        → 注入 system prompt (prepend)
  → SkillsRequestProcessor.injectOverview (注入概览)
  → SkillsRequestProcessor.ProcessRequest (注入已加载 body + docs)
  → LLM 调用
```

### 1.3 目标数据流

```
用户消息
  → ResolveSkillSlugsDetailed (Layer A + B 路由，不变)
    → 仅将路由结果写入 session state (不注入正文)
  → SkillsRequestProcessor.injectOverview (注入 L0 manifest，不变)
  → LLM 调用 (仅看到 L0 manifest)
    → LLM 决定使用哪个 Skill → 调用 skill_load
      → skill_load 返回 SKILL.md 正文 (L1 body)
    → LLM 需要参考文件 → 调用 skill_select_docs
      → skill_select_docs 返回 doc 内容 (L2 refs)
```

***

## 二、3 阶段渐进加载模型

### 2.1 三层定义

| 层级 | 名称 | 内容 | 注入方式 | Token 成本 |
|------|------|------|----------|-----------|
| L0 | SkillManifest | name + description + triggers + tags | system prompt（每个请求） | ~3K / 40 skill |
| L1 | SkillBody | SKILL.md 正文（经 manifest.Parse 后的 Body） | `skill_load` 工具返回值 | ~2-8K / skill |
| L2 | SkillRefs | 关联引用文件（doc 文件内容） | `skill_select_docs` 工具返回值 | ~1-5K / doc |

### 2.2 与当前实现的映射

| 层级 | 当前实现 | 渐进加载实现 | 变更 |
|------|----------|-------------|------|
| L0 | `SkillsRequestProcessor.injectOverview` | 同左（不变） | 无 |
| L1 | `newSkillGuidanceBeforeHook` 全量注入 + `SkillsRequestProcessor` loaded body | 仅通过 `skill_load` 工具返回 | **移除** `newSkillGuidanceBeforeHook` 的 guidance 注入；**复用** `SkillsRequestProcessor` 的 loaded body 机制 |
| L2 | `SkillsRequestProcessor` docs 注入 | 仅通过 `skill_select_docs` 工具返回 | 无（已按需） |

### 2.3 关键洞察

当前系统中，`skill_load` 工具已经实现了 L1 的按需获取能力。问题在于 `newSkillGuidanceBeforeHook` 在 LLM 调用前就全量注入了所有被路由 Skill 的正文，使得 `skill_load` 变得多余——LLM 已经在 system prompt 中看到了所有内容，不需要再调用工具。

渐进加载的核心变更：**移除 `newSkillGuidanceBeforeHook` 的 guidance 正文注入，让 LLM 必须通过 `skill_load` 工具按需获取 Skill 正文**。

***

## 三、数据结构设计

### 3.1 SkillManifest（L0）

复用框架已有的 `skill.Summary`，在 `injectOverview` 中增强展示：

```go
// 框架已有，无需修改
type Summary struct {
    Name        string
    Description string
}
```

在 `injectOverview` 中，为被路由到的 Skill 添加触发提示，引导 LLM 使用 `skill_load`：

```
Available skills:
- aranea-coding-guide: 后端项目编码指南（详细版） [routed]
- go-oop-guide: 通用 Go OOP 编程指导 [routed]
- vue-frontend-guide: 通用 Vue 3 编程指导

Skill tool availability:
- Use skill_load to load a skill's full instructions before using it.
- Only load skills marked [routed] or relevant to the current task.
```

### 3.2 SkillBody（L1）

复用框架已有的 `skill.Skill.Body`，通过 `skill_load` 工具返回：

```go
// 框架已有，无需修改
type Skill struct {
    Summary Summary
    Body    string
    Docs    []Doc
}
```

### 3.3 SkillRefs（L2）

复用框架已有的 `skill.Skill.Docs`，通过 `skill_select_docs` 工具返回：

```go
// 框架已有，无需修改
type Doc struct {
    Path    string
    Content string
}
```

### 3.4 SkillLoadMode 扩展

在 `biz.Agent` 的 `skill_load_mode` 字段中新增 `progressive` 值：

```go
const (
    SkillLoadModeOnce      = "once"       // 框架已有
    SkillLoadModeTurn      = "turn"       // 框架已有
    SkillLoadModeSession   = "session"    // 框架已有（legacy）
    SkillLoadModeProgressive = "progressive" // 新增：渐进加载模式
)
```

**模式行为对比**：

| 模式 | L0 manifest | L1 guidance 注入 | L1 skill_load | L2 docs |
|------|------------|-----------------|---------------|---------|
| `turn`（当前默认） | system prompt | `newSkillGuidanceBeforeHook` 全量注入 | 可用（但多余） | system prompt |
| `progressive`（新增） | system prompt + `[routed]` 标记 | **不注入** | 可用（必须调用） | tool result |

***

## 四、与现有 skill.Repository 的集成

### 4.1 无需修改框架接口

`skill.Repository` 的三个核心方法完全满足渐进加载需求：

| 方法 | 渐进加载中的用途 | 调用时机 |
|------|----------------|----------|
| `Summaries()` | L0 manifest 注入 | 每个请求（`injectOverview`） |
| `Get(name)` | L1 body 获取 | `skill_load` 工具调用时 |
| `Path(name)` | Skill 目录定位 | `skill_load` / `skill_run` 时 |

### 4.2 FSRepositoryAdapter 无需修改

`internal/skill/trpc/repository.go` 的 `FSRepositoryAdapter` 仅是框架 `FSRepository` 的薄包装，所有方法委托给 `delegate`，无需任何变更。

### 4.3 应用层 SkillUsecase 集成

`BatchGetSkillGuidance` 方法当前被 `newSkillGuidanceBeforeHook` 调用。渐进加载模式下，此方法不再被调用（L1 body 改由 `skill_load` 工具触发 `repo.Get()` 获取）。

但 `BatchGetSkillGuidance` 仍需保留，因为：
- 非 progressive 模式（`turn`/`once`/`session`）仍需全量注入
- 前端 Skill 预览功能可能依赖此方法

***

## 五、Prompt 注入策略变更

### 5.1 变更点 1：移除 `newSkillGuidanceBeforeHook` 的 guidance 注入

**当前**（`internal/agent/skill_guidance_inject.go`）：

```go
func newSkillGuidanceBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
    // ...
    return callbacks.NewBeforeModelHook(5, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
        // 1. ResolveSkillSlugsDetailed → 获取被路由 Skill 列表
        // 2. BatchGetSkillGuidance → 获取所有被路由 Skill 的完整正文
        // 3. manifest.Parse + render.SkillGuidance → 渲染正文
        // 4. 将所有正文 prepend 到 system prompt
    })
}
```

**渐进加载模式**：

```go
func newSkillGuidanceBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
    if ag.Settings == nil || deps.SkillUC == nil {
        return nil
    }
    if !SkillsUseFullProfile(ag.SystemPromptMode) {
        return nil
    }
    // progressive 模式下不注入 guidance，LLM 通过 skill_load 按需获取
    if ag.Settings.GetSkillLoadMode() == "progressive" {
        return nil
    }
    // 非 progressive 模式保持原有行为
    return callbacks.NewBeforeModelHook(5, func(...) { ... })
}
```

### 5.2 变更点 2：`injectOverview` 增加 `[routed]` 标记

**当前**（`SkillsRequestProcessor.injectOverview`）：

```
Available skills:
- aranea-coding-guide: 后端项目编码指南（详细版）
- go-oop-guide: 通用 Go OOP 编程指导
```

**渐进加载模式**：

```
Available skills:
- aranea-coding-guide: 后端项目编码指南（详细版） [routed]
- go-oop-guide: 通用 Go OOP 编程指导 [routed]
- vue-frontend-guide: 通用 Vue 3 编程指导
```

`[routed]` 标记告诉 LLM 哪些 Skill 与当前 turn 最相关，引导其优先加载这些 Skill。

**实现方式**：在 `SkillsRequestProcessor` 中新增 `routedSkills` 选项，通过 `WithRoutedSkills` 注入。应用层在构建 Agent 时，将 `ResolveSkillSlugsDetailed` 的结果传入。

### 5.3 变更点 3：`skill_load` 工具返回值增强

当前 `skill_load` 返回 SKILL.md 正文。渐进加载模式下，`skill_load` 是 LLM 获取 Skill 正文的唯一途径，需确保返回内容完整且包含足够的上下文引导。

**无需修改**：当前 `skill_load` 已返回完整 SKILL.md body + 可选 docs，满足 L1 + L2 需求。

### 5.4 变更点 4：`SkillsRequestProcessor` 的 `toolResultMode` 联动

框架已有 `WithSkillsLoadedContentInToolResults` 选项，将 loaded body/docs 放入 tool result 而非 system prompt。渐进加载模式应默认启用此选项：

- **原因**：避免 loaded body 再次被注入 system prompt（否则 progressive 模式退化为 eager 模式）
- **实现**：在构建 Agent 时，当 `skill_load_mode == "progressive"` 时，自动设置 `WithSkillsLoadedContentInToolResults(true)`

***

## 六、渐进迁移策略

### 6.1 配置驱动

通过 Agent 的 `skill_load_mode` 字段控制行为：

| `skill_load_mode` | L0 | L1 guidance hook | L1 skill_load | toolResultMode |
|-------------------|----|--------------------|---------------|----------------|
| `turn`（默认） | system prompt | 全量注入 | 可用 | false |
| `once` | system prompt | 全量注入 | 可用 | false |
| `session` | system prompt | 全量注入 | 可用 | false |
| `progressive` | system prompt + `[routed]` | **不注入** | 可用（必须） | true |

### 6.2 迁移路径

1. **Phase 1**：实现 `progressive` 模式，默认仍为 `turn`
2. **Phase 2**：新创建的 Agent 默认使用 `progressive`
3. **Phase 3**：将 `progressive` 设为全局默认，`turn`/`once`/`session` 降级为 legacy

### 6.3 回退方案

如果 progressive 模式导致 LLM 无法正确选择 Skill（因为看不到正文），可通过将 `skill_load_mode` 改回 `turn` 立即回退到 eager 模式。

***

## 七、边界场景

| 场景 | 处理方式 |
|------|----------|
| 无 Skill 被路由 | `[routed]` 标记为空，LLM 看到普通概览列表，不影响行为 |
| 所有 Skill 都被路由 | 所有 Skill 标记 `[routed]`，LLM 需根据任务选择加载哪些 |
| LLM 不调用 `skill_load` 直接使用 Skill | LLM 只能看到 L0 manifest 中的 name + description，缺少正文指导，可能产生幻觉。通过 capability guidance 提示 LLM 必须先 `skill_load` |
| `skill_load` 后 body 过长 | 复用现有 `maxLoadedSkills` + `SkillLoadMode` 卸载机制 |
| 多 Agent / Team 场景 | 每个 Agent 独立维护自己的 loaded skill state，互不影响（已有 `StateKeyLoadedByAgentPrefix` 隔离） |
| Session summary 恢复 | `SkipSkillsFallbackOnSessionSummary` 已处理：当 session summary 存在时，跳过 loaded skill 的 system prompt fallback，避免重复注入 |
| 非 full profile 模式 | `SkillsUseFullProfile` 返回 false 时，`newSkillGuidanceBeforeHook` 已返回 nil，不受影响 |

***

## 八、变更影响速查

| 修改内容 | 影响范围 | 需同步更新 |
|----------|----------|-----------|
| `newSkillGuidanceBeforeHook` 增加 progressive 判断 | `internal/agent/skill_guidance_inject.go` | 无 |
| `SkillsRequestProcessor` 增加 `[routed]` 标记 | `pkg/trpc-agent-go/internal/flow/processor/skills.go` | 框架层变更 |
| Agent 构建时传入 routed skills | `internal/agent/trpc_build.go` | Wire 装配 |
| `skill_load_mode` 新增 `progressive` 值 | `api/kratos/agent/v1/agent.proto` | Proto 生成 |
| progressive 模式自动启用 `toolResultMode` | `internal/agent/trpc_build.go` | Agent 构建逻辑 |
| `RuntimeSettings.GetSkillLoadMode()` | `internal/tools/skillruntime/toolset.go` | 接口扩展 |

***

## 九、验证计划

| 验证项 | 方法 |
|--------|------|
| progressive 模式下 guidance 不注入 | 单测：验证 `newSkillGuidanceBeforeHook` 返回 nil |
| `[routed]` 标记正确显示 | 单测：验证 `injectOverview` 输出包含 `[routed]` |
| `skill_load` 正常返回 body | 集成测试：progressive 模式下调用 `skill_load` |
| token 节省效果 | 基准测试：对比 turn vs progressive 模式的 system prompt token 数 |
| LLM 行为正确性 | 手动测试：progressive 模式下 LLM 能正确选择并加载 Skill |
| 回退到 eager 模式 | 手动测试：将 `skill_load_mode` 改回 `turn` 后行为正常 |
| 全量 | `make api && make wire && make build && make test && make lint` |
