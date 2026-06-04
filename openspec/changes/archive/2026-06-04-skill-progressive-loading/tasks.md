# Skill 渐进加载 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 将 Skill Prompt 注入策略从 Eager 全量注入改为 3 阶段渐进加载（L0 manifest → L1 body → L2 refs），通过 `skill_load_mode=progressive` 配置切换，目标降低 Skill 相关 token 消耗 50-80%。

**Architecture:** 在 `newSkillGuidanceBeforeHook` 中增加 progressive 模式判断，跳过 guidance 全量注入；在 `SkillsRequestProcessor` 中增加 `[routed]` 标记能力；在 Agent 构建时根据 `skill_load_mode` 自动启用 `toolResultMode`。

**Tech Stack:** Go + trpc-agent-go 框架 + Kratos v2 + Wire DI

---

## File Structure

| 操作 | 文件 | 职责 |
|------|------|------|
| Modify | `internal/agent/skill_guidance_inject.go` | progressive 模式下跳过 guidance 注入 |
| Modify | `pkg/trpc-agent-go/internal/flow/processor/skills.go` | injectOverview 增加 `[routed]` 标记 |
| Modify | `pkg/trpc-agent-go/internal/flow/processor/skills_test.go` | `[routed]` 标记单测 |
| Modify | `internal/agent/trpc_build.go` | progressive 模式自动启用 toolResultMode + 传入 routed skills |
| Modify | `internal/agent/prompt_mode.go` | 新增 `IsProgressiveSkillLoad` 辅助函数 |
| Modify | `api/kratos/agent/v1/agent.proto` | skill_load_mode 注释更新，新增 progressive 值说明 |
| Modify | `internal/tools/skillruntime/toolset.go` | RuntimeSettings 接口增加 GetSkillLoadMode |

---

### Task 1: RuntimeSettings 接口扩展 — 新增 GetSkillLoadMode

**Files:**
- Modify: `internal/tools/skillruntime/toolset.go`

- [x] **Step 1: 在 RuntimeSettings 接口中新增 GetSkillLoadMode 方法**

在 `RuntimeSettings` 接口中新增：

```go
GetSkillLoadMode() string
```

- [x] **Step 2: 在所有实现 RuntimeSettings 的类型中添加 GetSkillLoadMode 方法**

搜索所有实现 `RuntimeSettings` 接口的类型，为每个类型添加：

```go
func (s *XXX) GetSkillLoadMode() string {
    if s == nil { return "" }
    return s.SkillLoadMode
}
```

具体字段名取决于各实现类型的结构。需要读取代码确认。

- [x] **Step 3: 验证编译通过**

Run: `go build ./internal/tools/skillruntime/...`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add internal/tools/skillruntime/
git commit -m "feat(skill): add GetSkillLoadMode to RuntimeSettings interface"
```

**DoD:**
- `RuntimeSettings` 接口包含 `GetSkillLoadMode() string` 方法
- 所有实现类型均实现该方法
- `go build ./internal/tools/skillruntime/...` 通过

---

### Task 2: prompt_mode.go — 新增 IsProgressiveSkillLoad 辅助函数

**Files:**
- Modify: `internal/agent/prompt_mode.go`

- [x] **Step 1: 新增 IsProgressiveSkillLoad 函数**

```go
func IsProgressiveSkillLoad(mode string) bool {
    return strings.EqualFold(strings.TrimSpace(mode), "progressive")
}
```

- [x] **Step 2: 验证编译通过**

Run: `go build ./internal/agent/...`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add internal/agent/prompt_mode.go
git commit -m "feat(skill): add IsProgressiveSkillLoad helper"
```

**DoD:**
- `IsProgressiveSkillLoad` 函数存在且正确判断 `"progressive"` 值
- `go build ./internal/agent/...` 通过

---

### Task 3: skill_guidance_inject.go — progressive 模式跳过 guidance 注入

**Files:**
- Modify: `internal/agent/skill_guidance_inject.go`

- [x] **Step 1: 在 newSkillGuidanceBeforeHook 中增加 progressive 判断**

在 `SkillsUseFullProfile` 判断之后，新增 progressive 模式判断：

```go
func newSkillGuidanceBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
    if ag.Settings == nil || deps.SkillUC == nil {
        return nil
    }
    if !SkillsUseFullProfile(ag.SystemPromptMode) {
        return nil
    }
    if IsProgressiveSkillLoad(ag.Settings.GetSkillLoadMode()) {
        return nil
    }
    return callbacks.NewBeforeModelHook(5, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
        // ... 原有逻辑不变
    })
}
```

- [x] **Step 2: 编写单测验证 progressive 模式返回 nil**

```go
func TestNewSkillGuidanceBeforeHook_ProgressiveMode(t *testing.T) {
    ag := biz.Agent{
        SystemPromptMode: "complete",
        Settings: &mockRuntimeSettings{skillLoadMode: "progressive"},
    }
    hook := newSkillGuidanceBeforeHook(ag, mockDeps)
    if hook != nil {
        t.Error("progressive mode should return nil hook")
    }
}
```

> mock 类型和 deps 结构需要根据实际代码调整。

- [x] **Step 3: 验证编译通过**

Run: `go build ./internal/agent/...`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add internal/agent/skill_guidance_inject.go
git commit -m "feat(skill): skip guidance injection in progressive mode"
```

**DoD:**
- `newSkillGuidanceBeforeHook` 在 `skill_load_mode == "progressive"` 时返回 nil
- 非 progressive 模式行为不变
- 单测验证 progressive 模式返回 nil
- `go build ./internal/agent/...` 通过

---

### Task 4: SkillsRequestProcessor — 增加 `[routed]` 标记能力

**Files:**
- Modify: `pkg/trpc-agent-go/internal/flow/processor/skills.go`
- Modify: `pkg/trpc-agent-go/internal/flow/processor/skills_test.go`

- [x] **Step 1: 在 skillsRequestProcessorOptions 中新增 routedSkills 字段**

```go
type skillsRequestProcessorOptions struct {
    // ...existing fields
    routedSkills []string
}
```

- [x] **Step 2: 新增 WithRoutedSkills option**

```go
func WithRoutedSkills(names []string) SkillsRequestProcessorOption {
    return func(o *skillsRequestProcessorOptions) {
        o.routedSkills = names
    }
}
```

- [x] **Step 3: 在 SkillsRequestProcessor struct 中新增 routedSkills 字段**

```go
type SkillsRequestProcessor struct {
    // ...existing fields
    routedSkills []string
}
```

在 `NewSkillsRequestProcessor` 构造函数中赋值。

- [x] **Step 4: 修改 injectOverview 中的 Skill 行生成逻辑**

在生成每个 Skill 概览行时，检查是否在 routedSkills 中：

```go
routedSet := make(map[string]struct{}, len(p.routedSkills))
for _, name := range p.routedSkills {
    routedSet[name] = struct{}{}
}
// ...
for _, s := range sums {
    suffix := p.skillOverviewSuffix(ctx, repo, s.Name)
    routedMark := ""
    if _, ok := routedSet[s.Name]; ok {
        routedMark = " [routed]"
    }
    line := fmt.Sprintf("- %s: %s%s%s\n", s.Name, s.Description, suffix, routedMark)
    b.WriteString(line)
}
```

- [x] **Step 5: 编写单测验证 `[routed]` 标记**

```go
func TestSkillsRequestProcessor_RoutedMark(t *testing.T) {
    // 创建包含 2 个 Skill 的 repo
    // 配置 WithRoutedSkills([]string{"skill-a"})
    // 验证 injectOverview 输出中 skill-a 行包含 [routed]
    // 验证 skill-b 行不包含 [routed]
}
```

- [x] **Step 6: 验证编译通过**

Run: `cd pkg/trpc-agent-go && go build ./...`
Expected: PASS

- [x] **Step 7: Commit**

```bash
git add pkg/trpc-agent-go/internal/flow/processor/
git commit -m "feat(skill): add [routed] mark support in SkillsRequestProcessor"
```

**DoD:**
- `WithRoutedSkills` option 可用
- `injectOverview` 为 routed Skill 添加 `[routed]` 标记
- 非 routed Skill 不受影响
- 单测验证 `[routed]` 标记正确
- `cd pkg/trpc-agent-go && go build ./...` 通过

---

### Task 5: Agent 构建集成 — progressive 模式自动启用 toolResultMode + 传入 routed skills

**Files:**
- Modify: `internal/agent/trpc_build.go`

- [x] **Step 1: 找到 Agent 构建中 SkillsRequestProcessor 的 option 配置位置**

搜索 `WithSkills` / `WithSkillLoadMode` / `SkillsRequestProcessor` 相关的构建代码。

- [x] **Step 2: 在构建时根据 skill_load_mode 传入 WithRoutedSkills**

在 Agent 构建流程中，当 `skill_load_mode == "progressive"` 时：
1. 调用 `ResolveSkillSlugsDetailed` 获取被路由 Skill 列表
2. 将结果通过 `WithRoutedSkills` 传入 `SkillsRequestProcessor`

> 注意：`ResolveSkillSlugsDetailed` 需要 `SkillResolver` 和 `SkillToolsetOptions`，需在构建时或首次请求时获取。如果构建时无法获取路由结果，可改为在 `SkillsRequestProcessor` 中通过 `repoResolver` + invocation context 延迟解析。

- [x] **Step 3: 在构建时根据 skill_load_mode 启用 toolResultMode**

当 `skill_load_mode == "progressive"` 时，自动添加 `WithSkillsLoadedContentInToolResults(true)`：

```go
if IsProgressiveSkillLoad(loadMode) {
    skillsOpts = append(skillsOpts, processor.WithSkillsLoadedContentInToolResults(true))
}
```

- [x] **Step 4: 验证编译通过**

Run: `go build ./internal/agent/...`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add internal/agent/trpc_build.go
git commit -m "feat(skill): auto-enable toolResultMode and routed skills in progressive mode"
```

**DoD:**
- progressive 模式下 `WithSkillsLoadedContentInToolResults(true)` 自动启用
- progressive 模式下 `WithRoutedSkills` 传入被路由 Skill 列表
- 非 progressive 模式行为不变
- `go build ./internal/agent/...` 通过

---

### Task 6: Proto 注释更新 — skill_load_mode 新增 progressive 值说明

**Files:**
- Modify: `api/kratos/agent/v1/agent.proto`

- [x] **Step 1: 更新 skill_load_mode 字段注释**

在 `skill_load_mode` 字段的注释中新增 `progressive` 值说明：

```protobuf
// skill_load_mode controls how skill content is loaded:
//   "once"      - load for one model request, then offload
//   "turn"      - load for the current invocation, offload on next (default)
//   "session"   - keep loaded across invocations (legacy)
//   "progressive" - 3-phase progressive loading: L0 manifest only in prompt,
//                   L1 body via skill_load tool, L2 refs via skill_select_docs
string skill_load_mode = 84;
```

- [x] **Step 2: 运行 make api 重新生成 proto 代码**

Run: `make api`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add api/
git commit -m "docs(skill): add progressive mode to skill_load_mode proto comment"
```

**DoD:**
- `skill_load_mode` 字段注释包含 `progressive` 值说明
- `make api` 通过

---

### Task 7: 全量验证

- [x] **Step 1: 后端全量验证**

Run: `make api && make wire && make build && make test && make lint`
Expected: ALL PASS

- [x] **Step 2: 手动集成测试**

1. 创建 Agent，设置 `skill_load_mode = "progressive"`
2. 发送消息，验证 system prompt 中不包含 Skill guidance 正文
3. 验证 system prompt 中 Skill 概览包含 `[routed]` 标记
4. 验证 LLM 调用 `skill_load` 后能获取 Skill 正文
5. 验证 `skill_select_docs` 正常工作
6. 将 `skill_load_mode` 改回 `turn`，验证 guidance 全量注入恢复

- [x] **Step 3: Token 节省基准测试**

1. 记录 `turn` 模式下 system prompt 的 token 数
2. 记录 `progressive` 模式下 system prompt 的 token 数
3. 计算节省比例，目标 ≥ 50%

- [x] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat(skill): complete progressive loading implementation"
```

**DoD:**
- `make api && make wire && make build && make test && make lint` 全部通过
- progressive 模式下 guidance 不注入 system prompt
- progressive 模式下 `[routed]` 标记正确显示
- `skill_load` / `skill_select_docs` 工具正常工作
- 回退到 `turn` 模式后行为正常
- Token 节省 ≥ 50%
