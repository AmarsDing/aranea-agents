# Agent 顶栏与系统提示词 — 实现设计文档

> 对应需求：`8 agent-title.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 详情页顶栏（身份摘要、标签、操作按钮）和系统提示词预览对话框。顶栏组合展示 Agent 核心属性，系统提示词对话框按模式预览运行态渲染后的完整系统提示词。

本模块主要是前端组合展示，后端已有 `GetAgent` + `GetAgentPromptPreview` RPC 支撑，无需新增后端接口。

---

## 二、Proto 与 API 契约

### 2.1 现有 Proto（`api/kratos/agent/v1/agent.proto`）

`Agent` 消息（节选与本模块相关字段）：

```protobuf
message Agent {
  string id = 1;
  string agent_key = 2;
  string display_name = 3;
  string provider = 4;
  string model = 5;
  string status = 6;
  bool is_default = 7;
  bool is_favorite = 8;
  string icon = 9;
  string agent_description = 10;
  string category_position_id = 11;
  string system_prompt_mode = 12;
  int32 context_window = 13;
  int32 budget_monthly_cents = 14;
  string config_json = 15;
  string created_at = 16;
  string updated_at = 17;
  string deleted_at = 18;
  AgentRuntimeSettings settings = 19;
  repeated AgentPromptFile files = 20;
  // agent_kind: "" | "llm" | "a2a_proxy"
  string agent_kind = 21;
  // readonly: system agents cannot be deleted.
  bool readonly = 28;
  string position_key = 29;
  string agent_variant = 30;
  // source: user | system | imported (origin tracking).
  string source = 32;
  // kind: user | system_builtin | ecosystem_preset | marketplace | certified (ownership classification).
  string kind = 33;
  // planner_kind selects the planning strategy: "" | "builtin" | "react" | "a2ui"
  string planner_kind = 100;
}
```

预览请求/响应：

```protobuf
message GetAgentPromptPreviewRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string mode = 2; // "complete" | "task" | "minimized" | "none"
}

message GetAgentPromptPreviewResponse {
  string preview = 1;
  // instruction is the static build-time system instruction (Description + prompt files + RuntimeCapabilityCue).
  string instruction = 2;
  repeated PromptSectionEstimate sections = 3;
  int32 static_total_tokens = 4;
  int32 runtime_overlay_est_tokens = 5;
  string runtime_note = 6;
}

message PromptSectionEstimate {
  string key = 1;
  string label = 2;
  int32 est_tokens = 3;
  // build = included in instruction; runtime = injected per LLM call when enabled.
  string source = 4;
}

service AgentService {
  rpc GetAgent(GetAgentRequest) returns (Agent) {
    option (google.api.http) = {get: "/v1/agents/{id}"};
  }
  rpc UpdateAgent(UpdateAgentRequest) returns (Agent) {
    option (google.api.http) = {patch: "/v1/agents/{id}" body: "agent"};
  }
  rpc DeleteAgent(DeleteAgentRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/agents/{id}"};
  }
  rpc GetAgentPromptPreview(GetAgentPromptPreviewRequest) returns (GetAgentPromptPreviewResponse) {
    option (google.api.http) = {get: "/v1/agents/{id}/system-prompt/preview"};
  }
}
```

### 2.2 API 端点（从需求 §10 迁入）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/agents/:id/system-prompt/preview?mode=complete\|task\|minimized\|none` | 返回渲染后全文 + `instruction` + `sections` + token 估算 |
| GET | `/v1/agents/:id` | 顶栏摘要 + 各 `*_md` 与 `other_config` |
| GET | `/v1/llm-providers` 或 `/v1/llm-provider-models` | 供应商/模型数据；底层 **`llm_provider_models`**（可按 `provider_code` 聚合，见 **`9 provider.md` §9**） |
| GET | `...?provider_code=` 或 `/v1/llm-providers/:code/models` | 级联模型列表：筛选 **`provider_code`** 下各行 |
| POST | `/v1/agents/validate-model` | 校验 `{ provider, model }`（与 **`2 agents-create.md`** 一致） |
| GET | `/v1/channels` | Channel 下拉（§8.2）；数据模型见 **`17 channel.md` §6** |
| GET | `/v1/channels/:channelId/chats` | 级联 Chat ID 列表；`channelId` 与主表 `id` 或 `uuid` 与 API 约定一致 |
| PATCH | `/v1/agents/:id` | 更新 `provider`、`model`、`other_config.messaging` 等 |

### 2.3 无需新增

顶栏和预览对话框是纯前端组合，调用已有 API。`GetAgentPromptPreview` 已支持按 mode 预览，并返回分段 token 估算（`sections` 区分 `build` / `runtime` 来源）。

---

## 三、Biz 与 Agent 层（Prompt 组装）

### 3.1 BuildSystemPrompt（`internal/agent/prompt.go`）

```go
// BuildSystemPrompt joins agent description and prompt files, filtered by system_prompt_mode.
//
// PGO-1-AGENT-01: a new optional categoryResponsibility parameter has been added.
// When non-empty and PGO_CATEGORY_RESPONSIBILITY_INJECT is enabled, it is prepended
// as a <role_responsibility source="category"> block BEFORE agent_description.
// Pass "" to preserve the original behaviour (backward-compatible).
func BuildSystemPrompt(agent biz.Agent, files []biz.AgentPromptFile, mode string, categoryResponsibility ...string) string {
    filtered := biz.FilesForMode(files, mode)
    var b strings.Builder

    if len(categoryResponsibility) > 0 {
        if cr := strings.TrimSpace(categoryResponsibility[0]); cr != "" {
            b.WriteString("<role_responsibility source=\"category\">\n")
            b.WriteString(cr)
            b.WriteString("\n</role_responsibility>\n\n")
        }
    }

    if pk := strings.TrimSpace(agent.PositionKey); pk != "" {
        if vd := strings.TrimSpace(agent.VariantDescription); vd != "" {
            b.WriteString("<industry_context>\n")
            fmt.Fprintf(&b, "## 当前定位\n你是本岗位的 %s 方向专家。\n%s\n", agent.AgentVariant, vd)
            b.WriteString("</industry_context>\n\n")
        } else if av := strings.TrimSpace(agent.AgentVariant); av != "" && av != "general" {
            b.WriteString("<industry_context>\n")
            fmt.Fprintf(&b, "## 当前定位\n你是本岗位的 %s 方向专家。\n", av)
            b.WriteString("</industry_context>\n\n")
        }
    }
    if d := strings.TrimSpace(agent.AgentDescription); d != "" {
        b.WriteString(d)
        b.WriteString("\n\n")
    }

    for _, f := range filtered {
        if body := strings.TrimSpace(f.Body); body != "" {
            b.WriteString(fmt.Sprintf("<internal_config name=%q>\n", f.Name))
            b.WriteString(body)
            b.WriteString("\n</internal_config>\n\n")
        }
    }
    return strings.TrimSpace(b.String())
}
```

### 3.2 `<internal_config>` 注入与文件字段

> 从需求 §5 迁入。

运行时将每个 Markdown 文件包在标签内注入：

```xml
<internal_config name="IDENTITY.md">
…
</internal_config>
```

实现位置：`BuildSystemPrompt`（`internal/agent/prompt.go`），每个文件内容用 `fmt.Sprintf("<internal_config name=%q>\n", f.Name)` 包裹。

| `name` 属性 | 典型来源列（见 **`6` §8**） |
|-------------|----------------------------|
| `IDENTITY.md` | `identity_md` |
| `SOUL.md` | `soul_md` |
| `AGENTS_CORE.md` | `agents_core_md`（见 **`6`** AGENTS 拆分） |
| `AGENTS_TASK.md` | `agents_task_md` |
| `CAPABILITIES.md` | `capabilities_md` |

**预览对话框**可高亮块头 **或** 与「文件」Tab 联动跳转（选中同名逻辑文件）。

### 3.3 AGENTS 双文件存储

> 从需求 §6 迁入。

部分运行时将操作规则拆为：

| 逻辑文件 | 职责摘要 |
|----------|----------|
| **AGENTS_CORE.md** | 通用：语言跟随、`[System Message]` 处理、保存须 `write_file`/`edit`、禁止用 exec 发消息等 |
| **AGENTS_TASK.md** | 任务向：memory 召回/写入路径、MEMORY.md 隐私、cron 使用约定等 |

存储上建议 **`agents_core_md` + `agents_task_md`** 两列；若产品仅提供单一 **`agents_md`**，可由服务端 **按标题拆段** 或 **任务模式只取一段**。详见 **`6 agent-setting-file.md` §8.3**。

### 3.4 运行时生成块（非「文件」Tab）

> 从需求 §7 迁入。

下列 **不必** 在「文件」侧栏以可编辑文件出现（或仅只读展示）：

| 块 | 说明 |
|----|------|
| **Tooling** | 来自己册 + `tools_config` 过滤后的工具列表 |
| **Workspace** | `workspace` 路径文案；团队共享路径来自租户/团队配置 |
| **`<system_context name="TEAM.md">`** | 团队与成员、委派规则；数据来自 **团队/成员 API**，非 Agent 表长文本 |
| **Current date** | 动态 |
| **Runtime** | 如 `Runtime: agent=… \| id=…` |

### 3.5 缓存边界（cache boundary）

> 从需求 §3 迁入。

运行时在完整提示中可插入标记（示例）：

```text
── cache boundary ── stable above · dynamic below
```

| 区段 | 含义 |
|------|------|
| **stable above** | 相对稳态：人格块、工具说明、AGENTS 分片、CAPABILITIES 等（仍可能随 PATCH 变，但同会话内多次请求可缓存复用） |
| **dynamic below** | 每轮或高频变化：**当前日期**、`<system_context name="TEAM.md">`、**Runtime** 行、会话任务片段等 |

UI 可在预览中 **弱化展示** 该分隔线，或仅调试模式显示。

### 3.6 FilesForMode（`internal/biz/agent_settings_helpers.go`）

> 实现位置：`internal/biz/agent_settings_helpers.go`（**非** `agent_defaults.go`）。

```go
// FilesForMode filters prompt files based on the agent's system_prompt_mode.
// PGO-1-BIZ-02: task mode no longer includes HEARTBEAT.md (moved to Settings).
// Whitelist per mode:
//   - complete / "": all files
//   - task:        AGENTS_CORE, IDENTITY, RULE, AGENTS_TASK, CAPABILITIES
//   - minimized:   AGENTS_CORE, RULE
//   - none:        empty
//   - unknown:     AGENTS_CORE, RULE (same as minimized, safe default)
func FilesForMode(files []AgentPromptFile, mode string) []AgentPromptFile {
    mode = strings.ToLower(strings.TrimSpace(mode))
    if mode == "" || mode == "complete" {
        return files
    }
    if mode == "none" {
        return nil
    }
    allowed := map[string]bool{}
    switch mode {
    case "minimized":
        allowed["AGENTS_CORE.md"] = true
        allowed["RULE.md"] = true
    case "task":
        allowed["AGENTS_CORE.md"] = true
        allowed["IDENTITY.md"] = true
        allowed["RULE.md"] = true
        allowed["AGENTS_TASK.md"] = true
        allowed["CAPABILITIES.md"] = true
        // HEARTBEAT.md intentionally removed: heartbeat is now a runtime
        // Settings concern, not a static prompt file. PGO-1-BIZ-02.
    default:
        // Unknown modes fall back to minimized core rules to avoid leaking full prompt files.
        allowed["AGENTS_CORE.md"] = true
        allowed["RULE.md"] = true
    }
    result := []AgentPromptFile{}
    for _, file := range files {
        if allowed[file.Name] {
            result = append(result, file)
        }
    }
    return result
}
```

**与需求 §4 模式表的差异**：实现以代码白名单为准——`task` 模式不含 `HEARTBEAT.md`（已迁移至运行时 Settings）；`minimized` 仅含 `AGENTS_CORE.md` + `RULE.md`；未知模式回退到 `minimized` 安全默认。

### 3.7 BuildPreviewReport（`internal/agent/prompt_preview.go`）

Service 层预览入口实际调用 `chatagent.BuildPreviewReport`，而非直接调用 `BuildSystemPrompt`。`PreviewReport` 包含 `Summary`、`Instruction`、`Sections`、`StaticTotalTokens`、`RuntimeOverlayEst`、`RuntimeNote`，对应 Proto 响应字段。

### 3.8 行业上下文与运行时能力提示（`internal/agent/prompt.go`）

| 函数 | 职责 |
|------|------|
| `BuildIndustryContext(ctx, d, ag)` | 基于 `position_key` + 组织树构建 `<industry_context>` 行业/部门/岗位描述 |
| `StaticRuntimeCapabilityCue(ctx, d, ag)` | 静态运行时能力提示（子代理开关、有效工具策略）——构建期注入 |
| `DynamicRuntimeCapabilityCue(ctx, d, ag)` | 动态运行时能力提示——按 LLM 调用注入 |
| `RuntimeCapabilityCue(ctx, d, ag)` | **Deprecated**：等价于 `Static` + `Dynamic`，新代码应使用拆分后的两个函数 |

---

## 四、Data 层

无需新增。`GetAgent` 已返回完整 Agent（含 settings + files），`GetAgentPromptPreview` 在 Service 层调用 `chatagent.BuildPreviewReport` 组装。

### 4.1 供应商与模型数据模型

> 从需求 §8.1 迁入。

| 数据关联 | 说明 |
|----------|------|
| **`llm_provider_models`** | **单表**：Provider 与 Model **不拆表**；每行含 `provider_code`、连接字段、`model_api_id`、分类、评级等；级联下拉 **唯一数据源**，详见 **`9 provider.md` §5** |

**持久化**：`agents.provider`、`agents.model`（**字符串**，与表中 **`provider_code` / `model_api_id`** 对齐，见 **`前端.md`**）。

### 4.2 通道与 Chat 数据模型

> 从需求 §8.2 迁入。

#### 与 `17 channel.md` 的对应关系

| 概念 | 来源 | 说明 |
|------|------|------|
| **Channel 记录** | 表 **`channel`**（见 **`17` §6.1**） | 一行 = 一个已配置的接入（飞书/Lark、微信、Telegram 等）；在 **通道管理** 中新增/编辑，**非** Agent 详情内嵌创建（Agent 仅「选择已有通道」） |
| **下拉展示字段** | `channel.name`、`channel.channel_type`（及微信时 `wechat_subtype`） | `QSelect` 选项 label 建议：`{name}` + 副文案 `「飞书」` / `「微信-公众号」` 等，与 **`17` §1.1.1** 列表列一致，便于运营识别 |
| **选项值（绑定键）** | 建议使用 **`channel.id`**（或 API 统一暴露 **`channel.uuid`**，二选一全项目一致） | Agent 持久化里存 **与 `channel` 主键一致** 的外键，禁止自造与 `channel` 表无关的字符串 |
| **Chat** | 关联表 **`channel_chats`**（或 `conversations` 等，见下） | 外部平台的 `chat_id` / `thread_id` + 展示名；**级联条件**：`channel_id = 当前选中的 channel` |

#### 数据关联

| 数据关联 | 说明 |
|----------|------|
| **`channel`** | 字段语义与枚举见 **`17` §6.1**（`channel_type`、`wechat_subtype`、`uuid`、`webhook_path` 等）；Agent **不重复存** AppSecret 等，只引用 `channel_id` |
| **`channel_chats`**（或等价名） | `channel_id`（FK → `channel.id`）+ 平台侧 `chat_id` + `title`/`name`；第二级下拉 **仅** `WHERE channel_id = :选中` |

#### Agent 侧持久化

| 字段路径 | 类型 | 说明 |
|----------|------|------|
| `other_config.messaging` | object | 推荐：`{ "channel_id": <bigint 或 uuid 与 API 一致>, "chat_id": "<string>" }` |
| 或独立列 | | **`agents.channel_id`**（FK → `channel.id`）+ **`agents.default_chat_id`**（TEXT，平台会话 id）— 与 `other_config.messaging` **二选一**，避免双源 |

**约束**：`channel_id` 必须在 `channel` 表中存在；切换租户或删除通道时，后端应校验或级联提示（见 **`17`** 删除确认文案）。

### 4.3 扩展思考数据模型

> 从需求 §8.4 迁入。

建议路径：`other_config.reasoning`：`{ "mode": "provider_default"|"custom", "level": "off"|"low"|"medium"|"high" }`。

---

## 五、Service 层

### 5.1 GetAgentPromptPreview（`internal/service/agent.go`）

```go
// GetAgentPromptPreview implements GET /v1/agents/{id}/system-prompt/preview.
func (s *AgentService) GetAgentPromptPreview(ctx context.Context, req *v1.GetAgentPromptPreviewRequest) (*v1.GetAgentPromptPreviewResponse, error) {
    a, err := s.uc.Get(ctx, req.GetId())
    if err != nil {
        if apierror.IsCode(err, apierror.CodeNotFound) {
            return nil, apierror.NotFound("AGENT", "agent not found")
        }
        return nil, err
    }
    mode := strings.TrimSpace(req.GetMode())
    report := chatagent.BuildPreviewReport(ctx, a, mode, chatagent.Deps{AgentUC: s.uc, LG: s.lg})
    sections := make([]*v1.PromptSectionEstimate, 0, len(report.Sections))
    for _, sec := range report.Sections {
        sections = append(sections, &v1.PromptSectionEstimate{
            Key:       sec.Key,
            Label:     sec.Label,
            EstTokens: int32(sec.EstTokens),
            Source:    sec.Source,
        })
    }
    return &v1.GetAgentPromptPreviewResponse{
        Preview:                 report.Summary,
        Instruction:             report.Instruction,
        Sections:                sections,
        StaticTotalTokens:       int32(report.StaticTotalTokens),
        RuntimeOverlayEstTokens: int32(report.RuntimeOverlayEst),
        RuntimeNote:             report.RuntimeNote,
    }, nil
}
```

---

## 六、Wire 注入

已有，无需新增。

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/
├── components/agents/
│   ├── AgentSettingsHeader.vue        ← 顶栏组件（设置页头部）
│   ├── AgentAdvancedDialog.vue        ← 「高级」模态（通道/工作区/Reasoning/压缩/沙箱）
│   ├── AgentPromptPreviewDialog.vue   ← 系统提示词预览对话框
│   ├── KindBadge.vue                  ← ownership 类型徽章
│   ├── AgentCard.vue                  ← 列表卡片（含「进化中」chip）
│   └── agentUi.ts                     ← promptModeLabel / statusLabel / promptModes
├── features/agents/
│   ├── types.ts                       ← Agent / AgentPromptPreview 类型
│   └── useAgentPromptPreview.ts       ← preview composable
├── stores/agents/detail.ts             ← useAgentDetailStore（fetchPromptPreview）
└── pages/AgentSettingsPage.vue        ← 组装顶栏 + 高级模态 + 预览对话框
```

### 7.2 TypeScript 类型（`web/src/features/agents/types.ts`）

```typescript
// 预览响应（对应 Proto GetAgentPromptPreviewResponse）
export type AgentPromptPreview = {
  summary: string;
  instruction: string;
  sections: AgentPromptSection[];
  static_total_tokens: number;
  runtime_overlay_est_tokens: number;
  runtime_note: string;
};

// Agent.system_prompt_mode 在 types.ts 中为 string；
// 联合字面量类型 PromptMode 见 agentUi.ts（§7.3）。
```

### 7.3 Prompt 模式与标签工具（`web/src/components/agents/agentUi.ts`）

```typescript
export type PromptMode = 'complete' | 'task' | 'minimized' | 'none';

export const promptModes = [
  { value: 'complete', label: '完整', caption: '交互聊天 + 完整人格类能力', tokens: '~8K tokens' },
  { value: 'task', label: '任务', caption: '企业自动化、记忆、进化', tokens: '~5K tokens' },
  { value: 'minimized', label: '最小化', caption: '后台任务、核心规则、仅观察', tokens: '~2K tokens' },
  { value: 'none', label: '无', caption: '纯工具调用自动化', tokens: '~2K tokens' },
];

export function promptModeLabel(value: string): string {
  return promptModes.find((mode) => mode.value === value)?.label ?? '完整';
}
```

### 7.4 API 调用（`web/src/features/agents/useAgentPromptPreview.ts`）

预览通过 `useAgentDetailStore().fetchPromptPreview(agentId, mode)` 发起，store 内部调用 `GET /v1/agents/:id/system-prompt/preview?mode=...`，返回 `AgentPromptPreview`。

```typescript
export function useAgentPromptPreview(agentId: Ref<string>, systemPromptMode: Ref<string>) {
  const detailStore = useAgentDetailStore();
  const promptDialog = ref(false);
  const previewMode = ref<PromptMode>('complete');
  const promptPreview = ref<AgentPromptPreview>(emptyPromptPreview());

  async function loadPromptPreview() {
    const id = agentId.value.trim();
    if (!id) return;
    promptPreview.value = await detailStore.fetchPromptPreview(id, previewMode.value);
  }

  function syncPreviewModeFromAgent(mode?: string) {
    previewMode.value = (String(mode ?? '').trim() as PromptMode) || 'complete';
  }

  watch(previewMode, () => void loadPromptPreview());
  // ...
}
```

### 7.5 Vue 组件 — AgentSettingsHeader.vue（顶栏）

> 实际组件为 `AgentSettingsHeader.vue`（非 `AgentHeader.vue`）。

关键结构：
- 头像（`AgentAvatarQ`）+ 显示名 + `KindBadge` + 状态 badge + 模式 chip + 「进化中」chip（`showEvolving` prop）
- 副标题 `metaCaption`：`{agent_key} · {provider} / {model}`，过滤误写入的相对路径式 `agent_key`
- 操作按钮：「系统提示词」（`@open-prompt`）、「高级」（`@open-advanced`）、收藏星标（`@toggle-favorite`）、「保存设置」（`@save`）

```vue
<template>
  <q-card-section class="agent-settings-header settings-header">
    <div class="row items-center q-gutter-md no-wrap">
      <q-btn flat round icon="arrow_back" class="header-icon-btn" @click="$emit('back')" />
      <div class="settings-header__avatar-wrap cursor-pointer" @click="$emit('change-avatar')">
        <agent-avatar-q :icon="agent.icon" :alt="agent.display_name || 'Agent 设置'" size="64px" />
      </div>
      <div class="min-width-0">
        <div class="row items-center q-gutter-sm">
          <div class="text-h5 text-weight-bold ellipsis">{{ agent.display_name || 'Agent 设置' }}</div>
          <KindBadge :kind="agent.kind" />
          <q-badge rounded :class="['settings-status', agent.status === 'active' ? 'is-active' : '']">{{
            statusLabel(agent.status)
          }}</q-badge>
          <q-chip dense square class="settings-chip">{{ promptModeLabel(agent.system_prompt_mode) }}</q-chip>
          <q-chip v-if="showEvolving" dense square class="settings-chip is-evolving" icon="auto_awesome">进化中</q-chip>
        </div>
        <div v-if="metaCaption" class="text-caption text-grey-7">{{ metaCaption }}</div>
      </div>
    </div>
    <!-- 操作按钮：系统提示词 / 高级 / 收藏 / 保存 -->
  </q-card-section>
</template>
```

### 7.6 Vue 组件 — AgentPromptPreviewDialog.vue（预览对话框）

> 实际组件为 `AgentPromptPreviewDialog.vue`（非 `PromptPreviewDialog.vue`）。

关键结构：
- 顶部：构建期 token + 运行时追加 token 估算
- 模式 Tab：`complete` / `task` / `minimized` / `none`（来自 `promptModes`）
- 「Token 分解（估算）」展开表：按 `sections`（`source: build|runtime`）展示
- 正文区：`preview`（summary）只读展示

```vue
<template>
  <q-dialog :model-value="open" @update:model-value="emit('update:open', $event)">
    <!-- 顶部 token 估算：构建期约 {{ staticTokens }} tokens · 运行时追加约 {{ runtimeTokens }} tokens -->
    <!-- 模式 Tab：complete / task / minimized / none -->
    <!-- Token 分解表：sections（source: build|runtime） -->
    <!-- 正文：preview summary -->
  </q-dialog>
</template>
```

### 7.7 Vue 组件 — AgentAdvancedDialog.vue（高级模态）

> 实际组件为 `AgentAdvancedDialog.vue`，内部已包含通道绑定、工作区、扩展思考、上下文压缩、沙箱等区块（**非** 独立的 `AgentChannelRefsSection.vue`）。

区块结构：

| 区块 | 字段/控件 | 对应需求 |
|------|-----------|----------|
| **通道绑定** | `QSelect` Channel + `QInput` Chat ID（级联） | §8.2 |
| **工作区** | `QInput` workspace 路径 | §8.3 |
| **扩展思考（Reasoning）** | `QSelect` 策略 + `QSelect` 思考级别（`custom` 时启用） | §8.4 |
| **上下文压缩** | `QToggle` 启用压缩 + `QToggle` 会话摘要 | §8.5 |
| **沙箱** | 预留 `QBanner`（后续版本支持） | §8.5 |

保存时通过 `@save` emit `{ channel_id, chat_id, workspace, reasoning_mode, reasoning_level, compaction_enabled, session_summary_enabled }`，由父组件统一 PATCH。

### 7.8 高级设置区块设计文档索引

| 区块 | 设计文档 |
|------|----------|
| 供应商与模型级联 | `9 provider.design.md` |
| 通道与 Chat 级联 | `17 channel.design.md` |
| 工作区 | `5 agent-setting.design.md` |
| 扩展思考（Reasoning） | `5 agent-setting.design.md` |
| 压缩/裁剪/沙箱 | `5 agent-setting.design.md` |

---

## 八、验收要点

- [ ] 顶栏完整 / 任务 / 最小化 / 无 标签与 `system_prompt_mode` 一致
- [ ] 「进化中」标签与 `evolution_self_evolve` 策略一致
- [ ] 系统提示词对话框 Token 估算与四子 Tab 预览与后端渲染一致
- [ ] `AGENTS_CORE` / `AGENTS_TASK` 在完整/任务/最小化模式下出现规则与 `6 agent-setting-file.md` 一致（以 `FilesForMode` 代码白名单为准）
- [ ] 收藏星标切换正确调用 `UpdateAgent` 并更新 UI
- [ ] 删除按钮弹出确认对话框，确认后调用 `DeleteAgent`
- [ ] 高级中供应商 → 模型级联与 `9 provider.design.md` 一致
- [ ] 高级中 Channel → Chat ID 级联与 `17 channel.design.md` 一致
