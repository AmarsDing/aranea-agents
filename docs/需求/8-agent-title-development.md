# Agent 标题 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[8 agent-title.md](./8%20agent-title.md) · **设计**：[8 agent-title.design.md](./8%20agent-title.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Agent 详情顶栏（身份摘要、标签、操作按钮）和系统提示词预览对话框。顶栏组合展示 Agent 核心属性，系统提示词对话框按模式预览运行态渲染后的完整系统提示词。

**代码锚点**：
- `internal/service/session_title_llm.go` — LLMSessionTitleGenerator
- `internal/service/agent.go` — GetAgentPromptPreview
- `internal/agent/prompt.go` — BuildSystemPrompt
- `internal/biz/agent_defaults.go` — FilesForMode

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| LLM 标题生成 | ✅ | `LLMSessionTitleGenerator.Generate` |
| Session 标题 | ✅ | 首次对话后自动生成 |
| Agent 名称 | ✅ | 创建时用户手动输入 |
| Prompt 预览 | ✅ | `GetAgentPromptPreview` RPC |
| 系统提示模式过滤 | ✅ | `FilesForMode` 按 mode 过滤 |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 顶栏组件 | 🟡 待验证 | 需确认 `AgentHeader.vue` 是否已实现 |
| 标签 chips（模式/进化状态） | 🟡 待验证 | 需确认标签渲染逻辑 |
| 系统提示词预览对话框 | 🟡 待验证 | 需确认 `PromptPreviewDialog.vue` 是否已实现 |
| 预览子 Tab（完整/任务/最小化/无） | 🟡 待验证 | 需确认四个模式切换预览 |
| 高级设置模态 | ❌ 未实现 | 需求 §8 描述的高级设置抽屉 |
| Agent 标题自动生成 | ❌ 未实现 | 需求提到"根据描述自动生成"但未实现 |

---

## 3. 差距与优化

1. **P3**：Agent 标题（非 Session 标题）无自动生成逻辑，用户需手动输入。需求文档提到"根据描述自动生成"但未实现。
2. **P3**：系统提示词预览对话框缺少性能优化设计（大文本渲染可能导致卡顿）。
3. **P3**：高级设置模态（§8）未实现，包含供应商/模型级联、通道绑定、工作区配置等。
4. **P2**：顶栏标签 chips 的推导逻辑未明确（如"进化中"标签的判断条件：`self_evolve === true` 且有 pending 建议？）。

---

## 4. 开发阶段

- **Phase 1**：顶栏标签 chips 推导逻辑明确化 + 前端实现
- **Phase 2**：Agent 创建后可选自动生成标题
- **Phase 3**：高级设置模态实现

---

## 5. 任务清单

| # | 任务 | 层 | 优先级 | EP | 需求回溯 |
|---|------|-----|--------|-----|----------|
| 1 | 顶栏标签 chips 推导逻辑文档化 + 前端实现 | 前端 | P2 | — | 需求 §1 |
| 2 | Agent 创建后可选自动生成标题（复用 LLMSessionTitleGenerator） | 后端 | P3 | — | 需求 §1 |
| 3 | 前端标题自动生成开关 + loading 状态 | 前端 | P3 | — | — |
| 4 | 系统提示词预览性能优化（虚拟滚动/分段渲染） | 前端 | P3 | — | 需求 §2 |
| 5 | 高级设置模态：供应商/模型级联 | 前端 | P3 | — | 需求 §8.1 |
| 6 | 高级设置模态：通道/Chat 级联 | 前端 | P3 | — | 需求 §8.2 |
| 7 | 高级设置模态：工作区配置 | 前端 | P3 | — | 需求 §8.3 |

---

## 6. 顶栏标签 chips 推导规则

| 标签 | 推导条件 | 数据来源 |
|------|---------|---------|
| **完整** / **任务** / **最小化** / **无** | `system_prompt_mode` 值映射 | `agents.system_prompt_mode` |
| **进化中** | `self_evolve === true` 且（`evolution_suggestions_enabled === true` 或有 pending 建议） | `AgentRuntimeSettings` + `evolution_suggestions` |
| **V3** 等版本标签 | `agent_type` 或产品版本字段 | `agents.agent_type` / `other_config` |
| **活跃** / **停用** | `status` 值映射 | `agents.status` |

---

## 7. Agent 标题自动生成方案

### 7.1 触发时机

- Agent 创建成功后，若 `display_name` 为空或用户选择"自动生成"
- Agent 描述变更后，可选重新生成

### 7.2 实现方案

复用 `LLMSessionTitleGenerator`，将 `agent_description` + `provider/model` 作为输入：

```go
func (uc *AgentUsecase) GenerateAgentTitle(ctx context.Context, agentID string) (string, error) {
    ag, err := uc.repo.GetAgentByID(ctx, agentID)
    if err != nil {
        return "", err
    }
    prompt := fmt.Sprintf("Generate a short, concise title (max 50 chars) for an AI Agent with the following description: %s. Provider: %s, Model: %s",
        ag.AgentDescription, ag.Provider, ag.Model)
    title, err := uc.titleGenerator.Generate(ctx, prompt)
    if err != nil {
        return "", err
    }
    return title, nil
}
```

### 7.3 新增 RPC

```protobuf
rpc GenerateAgentTitle(GenerateAgentTitleRequest) returns (GenerateAgentTitleResponse) {
  option (google.api.http) = { post: "/v1/agents/{id}/generate-title" body: "*" };
}
```

---

## 8. 验收标准

- [ ] 顶栏标签 chips 正确展示系统提示模式、进化状态、Agent 状态
- [ ] Agent 创建时可选择自动生成标题
- [ ] 系统提示词预览在 10K+ token 内容下渲染流畅
- [ ] 高级设置模态可正常打开和保存

---

## 9. 依赖与风险

### 9.1 跨模块依赖

| 依赖模块 | 依赖项 | 说明 |
|----------|--------|------|
| 模块5 Agent设置 | 系统提示模式 | 标签 chips 需读取 `system_prompt_mode` |
| 模块7 Agent进化 | 进化状态 | "进化中"标签需读取进化开关和建议状态 |
| 模块9 Provider | Provider/Model 列表 | 高级设置供应商/模型级联 |
| 模块17 Channel | 通道列表 | 高级设置通道/Chat 级联 |
| 模块50 Avatar | 头像组件 | 顶栏头像展示 |

### 9.2 风险

- 标题自动生成依赖 LLM 调用，需考虑延迟和配额
- 高级设置模态内容较多，需合理分组避免信息过载
- 系统提示词预览大文本渲染需性能优化

---

## 10. 错误处理规格

| 场景 | HTTP 状态码 | 错误码 | 前端行为 |
|------|------------|--------|----------|
| 标题生成 LLM 调用失败 | 502 Bad Gateway | `TITLE_GENERATION_FAILED` | Toast：标题生成失败，请手动输入 |
| Prompt 预览 Agent 无文件 | 200 OK | — | 展示空预览 + 提示"暂无提示文件" |
| 高级设置保存失败 | 500 Internal | `SETTINGS_SAVE_FAILED` | Toast：保存失败，请重试 |
| Agent 不存在 | 404 Not Found | `AGENT_NOT_FOUND` | 跳转列表页 |
