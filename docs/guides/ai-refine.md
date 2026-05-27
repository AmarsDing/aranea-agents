# AI Refine 服务（PGO-3）

> **对应任务**：PGO-3-DOC-01  
> **服务路径**：`POST /v1/ai/refine`  
> **协议定义**：`api/kratos/ai_refine/v1/ai_refine.proto`

---

## 1. 概述

AI Refine 服务为平台所有可编辑文本字段提供统一的 AI 润色能力，替代原先分散的 `EditPromptFileByAI` 端点。核心特性：

- **统一端点**：单一 `POST /v1/ai/refine` 处理全部 6 类 scope
- **模型分级选取**：Agent 专属模型 → 平台默认 Refine LLM → LLM Catalog 排序第一 → 环境变量
- **字符限制**：input `5000` 字符（`spec_extract` scope 例外：`50000` 字符）
- **速率限制**：全局 20 QPS burst；每用户 10 次/5 分钟
- **审计**：每次调用记录 `provider`、`model`、`source`，供 Datadog 看板消费

---

## 2. 请求格式

```json
{
  "scope": 5,
  "resource_id": "agent-uuid",
  "file_name": "IDENTITY.md",
  "original_text": "# IDENTITY\n我是企业级 Agent...",
  "user_hint": "更专业一些，补充边界说明",
  "target_mode": "complete"
}
```

### RefineScope 枚举

| 值 | 名称 | 说明 |
|----|------|------|
| 0 | UNSPECIFIED | 无效，拒绝处理 |
| 1 | CATEGORY_INDUSTRY | 行业说明 |
| 2 | CATEGORY_DEPT | 部门职责 |
| 3 | CATEGORY_POSITION | 岗位职责 |
| 4 | AGENT_DESCRIPTION | Agent 摘要/能力描述 |
| 5 | AGENT_FILE | Agent Prompt 文件（需填 `file_name`） |
| 6 | SPEC_EXTRACT | Markdown → YAML 组织规格（CLI Import 专用） |

### 响应格式

```json
{
  "refined": "优化后的文本",
  "diff": "--- original\n+++ refined\n...",
  "tokens_before": 120,
  "tokens_after": 180,
  "provider": "openai",
  "model": "gpt-4o",
  "source": "agent_model"
}
```

`source` 可能值：`agent_model` | `system_default` | `catalog_first`

---

## 3. 前端集成

### AIRefineButton 组件

```vue
<ai-refine-button
  scope="agent.file"
  file-name="IDENTITY.md"
  :resource-id="agentId"
  :text="currentFileBody"
  @apply="(refined) => updateFileBody(refined)"
/>
```

**Props**

| 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `scope` | `FieldScope` | ✅ | 对应 Go `FieldScope` |
| `fileName` | `string` | 仅 `agent.file` | 文件名，如 `IDENTITY.md` |
| `resourceId` | `string` | 建议填 | Agent/Category ID，用于审计和模型选取 |
| `text` | `string` | ✅ | 当前字段内容 |
| `targetMode` | `string` | 否 | `complete` / `task` / `minimized`，默认 `complete` |

**事件**

| 名称 | 参数 | 说明 |
|------|------|------|
| `apply` | `refined: string` | 用户点击「应用」后触发，携带优化结果 |

### 挂载点

- **分类页**：`AgentCategoriesPage.vue` — 行业/部门/岗位描述字段底部
- **Agent 设置 Agent Tab**：`AgentSettingsAgentTab.vue` — 专业摘要/能力描述字段底部
- **Agent 设置文件 Tab**：`AgentFilesPanel.vue` — 编辑器顶部工具栏

---

## 4. 后端扩展

### 新增 scope

1. 在 `api/kratos/ai_refine/v1/ai_refine.proto` 的 `RefineScope` 枚举中添加新值
2. 运行 `make api` 重新生成代码
3. 在 `internal/biz/field_guides.go` 的 `init()` 中注册对应 `FieldGuide`
4. 在 `internal/service/prompt_refine.go` 的 `scopeMap` 中添加映射
5. 在 `web/src/features/agents/fieldGuides.ts` 的 `FieldScope` 类型和注册表中同步

运行 `make fieldguide-lint` 校验前后端 scope 一致性。

### 配置平台默认 Refine LLM

通过管理后台系统设置：

```
PATCH /v1/system-settings/refine-llm
{
  "provider": "openai",
  "model": "gpt-4o-mini",
  "base_url": "",
  "api_key": "sk-..."
}
```

或通过 `biz.SystemSettingUsecase.UpdateRefineLLM` 调用。

---

## 5. 速率限制错误码

| HTTP 状态 | Kratos 错误原因 | 说明 |
|-----------|----------------|------|
| 429 | `REFINE_RATE_LIMIT` | 全局 QPS 达上限 |
| 429 | `REFINE_RATE_LIMIT_USER` | 当前用户超 10 次/5 分钟 |
| 400 | `REFINE_INPUT_TOO_LONG` | 输入超字符限制 |
| 400 | `REFINE_NO_LLM` | 无可用 LLM 配置 |
| 400 | `REFINE_UNKNOWN_SCOPE` | Scope 未注册 |

---

## 6. 相关文件

| 文件 | 说明 |
|------|------|
| `api/kratos/ai_refine/v1/ai_refine.proto` | 协议定义 |
| `internal/biz/prompt_refiner.go` | 核心 Refine 业务逻辑 |
| `internal/biz/field_guides.go` | FieldGuide 注册表 |
| `internal/biz/llm_caller.go` | LLMCaller 接口 |
| `internal/agent/llm_caller_impl.go` | LLM 调用实现（DynamicLLMCaller） |
| `internal/service/prompt_refine.go` | HTTP 服务层 + 速率限制 |
| `web/src/components/agents/AIRefineButton.vue` | 前端按钮组件 |
| `web/src/features/agents/aiRefine.ts` | 前端 API client |
| `web/src/features/agents/fieldGuides.ts` | TS FieldGuide 注册表 |
