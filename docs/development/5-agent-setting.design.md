# Agent 设置模块 — 实现设计文档

> 对应需求：[5 agent-setting.md](./5%20agent-setting.md)
> 开发计划：[5 agent-setting.development.md](./5%20agent-setting.development.md)

---

## 一、模块概述

Agent 详情/设置页，包含身份、模型、系统提示、能力、记忆、进化、钩子、文件、权限、A2A 等 Tab。复用 `AgentService` RPC，数据源为 `agents` 主表 + `agent_runtime_settings` O2O + `agent_prompt_files` O2M。

**架构分层**：

```
api/kratos/agent/v1/agent.proto          ← Proto 契约（AgentService）
internal/service/agent.go                ← Service 层（RPC 实现 + Proto/Biz 转换）
internal/biz/agent_usecase.go            ← Biz 层（AgentUsecase + 窄接口）
internal/biz/agent_types.go              ← Biz 领域模型（Agent / AgentRuntimeSettings）
internal/biz/agent_settings.go           ← Biz 领域视图（IdentityCfg / MemoryCfg / ToolsCfg 等）
internal/biz/agent_effective_tools.go    ← Effective Tools 计算（9 tool groups, 8 profiles）
internal/biz/agent_settings_helpers.go   ← helpers（withSettingDefaults, settingsFromLegacyConfig 等）
internal/data/agent_repo.go              ← Data 层 Repo 实现
internal/data/ent/schema/agent_runtime_setting.go  ← Ent Schema
internal/agent/trpc_build.go             ← Agent 装配链（BuildTRPCLLMAgent）
internal/agent/prompt.go                 ← 系统提示构建（BuildSystemPrompt）
web/src/pages/AgentSettingsPage.vue      ← 前端主页面
web/src/features/agents/                 ← 前端 Composable + 类型 + API
web/src/components/agents/               ← 前端组件
```

---

## 二、Proto 层

### 2.1 Proto 文件

文件：`api/kratos/agent/v1/agent.proto`

#### Agent 消息

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
  string position_id = 11;
  string system_prompt_mode = 12;
  int32 context_window = 13;
  int32 budget_monthly_cents = 14;
  string config_json = 15;
  string created_at = 16;
  string updated_at = 17;
  string deleted_at = 18;
  AgentRuntimeSettings settings = 19;
  repeated AgentPromptFile files = 20;
  string agent_kind = 21;           // "" | "llm" | "a2a_proxy"
  A2AProxyConfig a2a_proxy_config = 22;
  bool a2a_endpoint_enabled = 23;
  string last_run_status = 24;
  string last_run_at = 25;
  int32 pending_evolution_count = 26;
  string created_by = 27;
  bool readonly = 28;
  string position_key = 29;
  string agent_variant = 30;
  string variant_description = 31;
  string source = 32;               // user | system | imported
  string kind = 33;                 // user | system_builtin | ecosystem_preset | marketplace | certified
}

message A2AProxyConfig {
  string remote_url = 1;
  string agent_card_url = 2;
  bool enable_streaming = 3;
  string auth_type = 4;
  string auth_config_json = 5;
  int32 timeout_seconds = 6;
}

message AgentPromptFile {
  string id = 1;
  string agent_id = 2;
  string name = 3;
  string body = 4;
  int32 sort_order = 5;
  string created_at = 6;
  string updated_at = 7;
}
```

#### AgentRuntimeSettings 消息

字段众多（130+），按域分组。完整定义见 `api/kratos/agent/v1/agent.proto`。主要域：

| 域 | 代表字段 | 说明 |
|----|---------|------|
| Identity | `agent_id`, `channel_id`, `chat_id`, `workspace`, `variables_json`, `model_instructions_json` | 身份与上下文 |
| Reasoning | `reasoning_mode`, `reasoning_level` | 推理模式 |
| Memory | `memory_enabled`, `memory_max_*`, `heartbeat_*`, `l0_*` ~ `l4_*` | 记忆 L0–L4 + 心跳 |
| Tools | `tools_enabled`, `tools_profile`, `tools_tool_call_prefix`, `tools_allow_json`, `tools_deny_json`, `tools_concurrent_allow_json`, `tools_retry_*`, `tools_parallel_enabled`, `tools_streaming_enabled`, `tools_circuit_breaker_*`, `tools_deferred_json`, `tools_command_safety_enabled`, `tools_execution_timeout_sec` | 工具策略 + 重试 + 并行 + 熔断 |
| Skills | `skill_runtime_json`, `intent_pass_enabled`, `skill_load_mode`, `code_executor_type` | 技能运行时 |
| Evolution | `self_evolve`, `subagents_*`, `evolution_*`, `guardrail_*`, `evo_*` | 进化 + 子 Agent + 守卫 |
| Context | `context_compaction_enabled`, `memory_compact_enabled`, `tool_result_gate_enabled`, `compress_llm_cache_*`, `compression_buffer_ratio`, `soft_trigger_ratio`, `hard_trigger_ratio`, `session_summary_enabled`, `output_schema_json`, `model_selector` | 上下文压缩 |
| Planner | `planner_kind`, `planner_config_json` | 规划器 |
| Ralph Loop | `ralph_loop_max_iterations`, `ralph_loop_completion_promise`, `ralph_loop_verify_command`, `ralph_loop_verify_timeout_seconds`, `ralph_loop_promise_tag_open/close`, `ralph_loop_verify_work_dir` | Ralph 循环 |

### 2.2 AgentService RPC

文件：`api/kratos/agent/v1/agent.proto`

| RPC | HTTP 方法 | 路径 | 用途 |
|-----|-----------|------|------|
| `ListAgents` | GET | `/v1/agents` | 列表 |
| `CreateAgent` | POST | `/v1/agents` | 创建 |
| `GetAgent` | GET | `/v1/agents/{id}` | 详情（含 Settings + Files） |
| `UpdateAgent` | PATCH | `/v1/agents/{id}` | 部分更新（body: agent） |
| `DeleteAgent` | DELETE | `/v1/agents/{id}` | 软删 |
| `ToggleFavorite` | PATCH | `/v1/agents/{id}/favorite` | 收藏切换 |
| `GetAgentPromptPreview` | GET | `/v1/agents/{id}/system-prompt/preview` | 系统提示预览 |
| `GetAgentEffectiveTools` | GET | `/v1/agents/{agent_id}/tools/effective` | 有效工具视图 |
| `UpdateAgentToolPolicy` | PUT | `/v1/agents/{agent_id}/tools/policy` | 工具策略更新 |
| `CreateAgentPromptFile` | POST | `/v1/agents/{agent_id}/files` | 创建提示文件 |
| `UpdateAgentPromptFile` | PATCH | `/v1/agents/{agent_id}/files/{id}` | 更新提示文件 |
| `DeleteAgentPromptFile` | DELETE | `/v1/agents/{agent_id}/files/{id}` | 删除提示文件 |
| `EstimateTokens` | POST | `/v1/agents/{agent_id}/files/estimate-tokens` | Token 估算 |
| `EditPromptFileByAI` | POST | `/v1/agents/{agent_id}/files/{file_id}/ai-edit` | AI 编辑提示文件 |
| `ListAgentTemplates` | GET | `/v1/agent-templates` | 模板列表 |
| `ListAgentCreators` | GET | `/v1/agents/creators` | 创建者列表 |
| `DuplicateAgent` | POST | `/v1/agents/{id}/duplicate` | 复制 Agent |
| `CheckAgentKey` | GET | `/v1/agent-keys/check` | Agent Key 可用性检查 |
| `GetAgentEvolutionMetrics` | GET | `/v1/agents/{agent_id}/evolution/metrics` | 进化指标 |
| `GetAgentEvolutionSuggestions` | GET | `/v1/agents/{agent_id}/evolution/suggestions` | 进化建议列表 |
| `ApplyEvolutionSuggestion` | POST | `/v1/agents/{agent_id}/evolution/suggestions/{suggestion_id}/apply` | 应用进化建议 |
| `RejectEvolutionSuggestion` | POST | `/v1/agents/{agent_id}/evolution/suggestions/{suggestion_id}/reject` | 拒绝进化建议 |

---

## 三、Biz 层

### 3.1 领域模型

文件：`internal/biz/agent_types.go`

#### Agent 结构体

```go
type Agent struct {
    ID                 string
    AgentKey           string
    DisplayName        string
    Provider           string
    Model              string
    Status             string
    IsDefault          *bool  // nil = not set; explicit true/false for merge
    IsFavorite         *bool
    Icon               string
    AgentDescription   string
    PositionID         string
    PositionKey        string
    AgentVariant       string
    VariantDescription string
    SystemPromptMode   string
    ContextWindow      int
    BudgetMonthlyCents int
    ConfigJSON         string
    MetadataJSON       string
    Roles              []string
    Kind               string // user | system_builtin | ecosystem_preset | marketplace | certified
    AgentKind          string // llm | a2a_proxy
    A2AProxy           *A2AProxyConfig
    A2AEndpointEnabled bool
    LastRunStatus      string
    LastRunAt          string
    PendingEvolutionCount int
    CreatedBy          string
    Readonly           bool
    Source             string // user | system | imported
    CreatedAt          string
    UpdatedAt          string
    DeletedAt          string
    Settings           *AgentRuntimeSettings
    Files              []AgentPromptFile
    CategoryResponsibilityPreview string // transient, not persisted
}
```

#### AgentRuntimeSettings 结构体

字段众多（130+），按域分组。完整定义见 `internal/biz/agent_types.go`。Biz 层提供领域视图访问器（`GetIdentity()` / `GetReasoning()` / `GetMemory()` / `GetTools()` / `GetSkills()` / `GetEvolution()` / `GetContext()`），定义于 `internal/biz/agent_settings.go`。

#### AgentPromptFile 结构体

```go
type AgentPromptFile struct {
    ID        string
    AgentID   string
    Name      string
    Body      string
    SortOrder int
    CreatedAt string
    UpdatedAt string
}
```

### 3.2 窄接口（Repo Port）

文件：`internal/biz/agent_usecase.go`

```go
// Stability:stable
type AgentReader interface {
    SearchAgents(ctx context.Context, q AgentListQuery) (AgentListResult, error)
    GetAgentByID(ctx context.Context, id string) (Agent, error)
    GetAgentByAgentKey(ctx context.Context, agentKey string) (Agent, error)
    ListExtrasForAgents(ctx context.Context, agentIDs []string) (map[string]AgentListExtras, error)
}

// Stability:stable
type AgentWriter interface {
    CreateAgent(ctx context.Context, a Agent) (Agent, error)
    UpdateAgent(ctx context.Context, a Agent) (Agent, error)
    DeleteAgent(ctx context.Context, id string) error
    ToggleFavorite(ctx context.Context, id string) (Agent, error)
}

// Stability:stable
type AgentRuntimeSettingsRepo interface {
    GetAgentRuntimeSettings(ctx context.Context, agentID string) (AgentRuntimeSettings, error)
    UpsertAgentRuntimeSettings(ctx context.Context, v AgentRuntimeSettings) (AgentRuntimeSettings, error)
}

// Stability:stable
type AgentPromptFileRepo interface {
    ListAgentPromptFiles(ctx context.Context, agentID string) ([]AgentPromptFile, error)
    ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []AgentPromptFile) ([]AgentPromptFile, error)
    CreateAgentPromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
    UpdateAgentPromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
    DeleteAgentPromptFile(ctx context.Context, agentID, id string) error
}

// Stability:evolving
type AgentAtomicWriter interface {
    CreateAgentAtomic(ctx context.Context, a Agent, files []AgentPromptFile, settings AgentRuntimeSettings) (Agent, error)
    UpdateAgentAtomic(ctx context.Context, a Agent, files []AgentPromptFile, settings *AgentRuntimeSettings) (Agent, error)
}

// Stability:stable
type AgentPositionRepo interface {
    ListAgentCreators(ctx context.Context) ([]AgentCreator, error)
    ReorderAgents(ctx context.Context, ids []string) error
    ClearPositionByDepartment(ctx context.Context, deptID string) (int, error)
}

// Stability:stable
type AgentTxRepo interface {
    ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// AgentRepository 聚合所有窄接口，仅用于 Wire 绑定
type AgentRepository interface {
    AgentReader
    AgentWriter
    AgentAtomicWriter
    AgentRuntimeSettingsRepo
    AgentPromptFileRepo
    AgentReferenceChecker
    AgentPositionRepo
    AgentTxRepo
}
```

### 3.3 Usecase

文件：`internal/biz/agent_usecase.go`

```go
type AgentUsecaseDeps struct {
    Reader             AgentReader
    Writer             AgentWriter
    Settings           AgentRuntimeSettingsRepo
    Files              AgentPromptFileRepo
    Position           AgentPositionRepo
    Tx                 AgentTxRepo
    Tools              ToolRegistryReader
    Sys                SystemSettingRepo
    WebResearchChecker WebResearchReadinessChecker
    ProviderValidator  ProviderModelPairValidator
    Lg                 loggateway.Logger
}

type AgentUsecase struct {
    reader, writer, settings, files, position, tx, tools, sys,
    webResearchChecker, providerValidator, lg, agentSM
}

func NewAgentUsecase(d AgentUsecaseDeps) *AgentUsecase
```

**关键方法**：

| 方法 | 行为 |
|------|------|
| `Get(ctx, id)` | 获取 Agent 后自动 hydrate Settings 和 Files；若不存在则从 `config_json` 迁移并 Upsert |
| `Update(ctx, id, patch)` | merge patch 到 current，同步更新 Agent 主表、Settings、Files、config_json |
| `PromptPreview(ctx, id, mode)` | 按指定模式生成系统提示预览 |
| `ToggleFavorite(ctx, id)` | 切换收藏标记 |
| `CreateAgentAtomic` / `UpdateAgentAtomic` | 事务化创建/更新（Pack 导入场景） |

#### 持久化边界校验（`validateAgentSettings`）

Create/Update 共用的设置校验链（`internal/biz/agent_usecase_validate.go`）依次执行：provider/model 存在性 → `ValidateCodeExecutorType` → `ValidatePlannerKind` / `ValidatePlannerConfigJSON` → `ValidateRalphLoopSettings` → `ValidateSafetyLimitCoupling` → tools allow/deny JSON 格式。

**安全限额联动规则**（`internal/biz/safety_limit.go`）：当 `max_llm_calls` 与 `max_tool_iterations` 同时配置（>0）时，要求 `max_llm_calls >= max_tool_iterations + 2`（`SafetyLimitGracefulHeadroom`）。余量语义来自框架优雅收尾路径：工具预算耗尽后框架摘掉工具声明引导模型产出总结（+1 次 LLM 调用）；若模型仍发工具调用，框架合成拒绝结果再循环一次（再 +1）。不满足时校验拒绝写入。存量违规数据由 `biz.CoupledSafetyLimits` 在 Agent 构建时（`SafetyLimitAdapter`）防御性抬升 `max_llm_calls` 并打 Warn 进程日志。MaxLLMCalls 硬停（`stop_agent_error` + "max LLM calls … exceeded"）时 v2 projector 发射兜底 final reply step（`emitSafetyLimitFallbackReply`）+ 流程日志 `chat.turn.safety_limit_stop`，保证用户看到优雅收尾而非裸硬停。

---

## 四、Data 层

### 4.1 Ent Schema

#### Agent 主表

文件：`internal/data/ent/schema/agent.go`

表名：`agents`。主要字段：`id`, `agent_key`, `display_name`, `provider`, `model`, `status`, `is_default`, `is_favorite`, `icon`, `agent_description`, `position_id`, `system_prompt_mode`, `context_window`, `budget_monthly_cents`, `config_json`, `metadata_json`, `roles`, `kind`, `source`, `created_by`, `created_at`, `updated_at`, `deleted_at`。

#### AgentRuntimeSetting 表

文件：`internal/data/ent/schema/agent_runtime_setting.go`

表名：`agent_runtime_settings`（通过 `entsql.Annotation{Table: "agent_runtime_settings"}` 显式映射）。

主键：`id` 字段，StorageKey 为 `agent_id`，与 Agent O2O。

字段众多（140+），与 Proto `AgentRuntimeSettings` 消息一一对应。主要域分组见 §二。完整字段清单见 Schema 文件。

**已知技术债务**：`AgentRuntimeSetting` Schema 约 140 个字段，严重超标（DB-DEBT-01）。后续应考虑拆分为多表或 JSON 列。

#### AgentPromptFile 表

文件：`internal/data/ent/schema/agent_prompt_file.go`

表名：`agent_prompt_files`。字段：`id`, `agent_id`, `name`, `body`, `sort_order`, `created_at`, `updated_at`。

### 4.2 Repo 实现

文件：`internal/data/agent_repo.go`

**Ent→Biz 转换函数**：

```go
func entAgentToBiz(a *ent.Agent) biz.Agent
func entRuntimeToBiz(e *ent.AgentRuntimeSetting) biz.AgentRuntimeSettings
func entPromptToBiz(e *ent.AgentPromptFile) biz.AgentPromptFile
```

**关键方法**：

- `GetAgentByID` — 按 ID 查询，未找到返回 `shared.ErrNotFound`
- `UpsertAgentRuntimeSettings` — 使用 `OnConflict` + `ResolveWithNewValues` 实现 Upsert
- `ReplaceAgentPromptFiles` — 事务内删除旧文件 + 创建新文件
- 所有错误经 `entErrToBizErr(err, domain)` 翻译为 Biz 错误码

**读写分离**：读用 `r.data.RW().Read(ctx)`，写用 `r.data.RW().Write(ctx)`。

---

## 五、Service 层

文件：`internal/service/agent.go`

### 5.1 Service 结构体

```go
type AgentService struct {
    v1.UnimplementedAgentServiceServer
    uc              *biz.AgentUsecase
    evoUC           *biz.EvolutionUsecase
    mon             *biz.MonitorUsecase
    a2aUC           *biz.A2AUsecase
    promptAI        *PromptFileAIEditor
    agentTemplateUC *biz.AgentTemplateUsecase
    lg              loggateway.Logger
}

func NewAgentService(
    uc *biz.AgentUsecase,
    evoUC *biz.EvolutionUsecase,
    mon *biz.MonitorUsecase,
    a2aUC *biz.A2AUsecase,
    promptAI *PromptFileAIEditor,
    agentTemplateUC *biz.AgentTemplateUsecase,
    lg loggateway.Logger,
) *AgentService
```

### 5.2 类型转换函数

```go
func fromProtoRuntime(pb *v1.AgentRuntimeSettings) *biz.AgentRuntimeSettings
func toProtoRuntime(b *biz.AgentRuntimeSettings) *v1.AgentRuntimeSettings
func fromProtoFile(pb *v1.AgentPromptFile) biz.AgentPromptFile
func toProtoFile(b biz.AgentPromptFile) *v1.AgentPromptFile
func fromProtoAgent(pb *v1.Agent) biz.Agent
func toProtoAgent(b biz.Agent) *v1.Agent
func fromProtoCreate(req *v1.CreateAgentRequest) biz.Agent
```

### 5.3 RPC 实现要点

- `GetAgent` — 调用 `uc.Get`，未找到返回 `NotFound`
- `UpdateAgent` — 调用 `uc.Update`，body 为空返回 `BadRequest`
- `GetAgentPromptPreview` — 调用 `uc.PromptPreview`
- `GetAgentEffectiveTools` — 调用 `uc.GetEffectiveTools`
- `UpdateAgentToolPolicy` — 调用 `uc.UpdateAgentToolPolicy`
- `ToggleFavorite` — 调用 `uc.ToggleFavorite`
- A2A 相关：通过 `a2aUC` 处理 AgentCard CRUD 与远程发现

---

## 六、Wire 注入

已有，无需新增：

```
data.ProviderSet → NewAgentRepo
biz.ProviderSet → NewAgentUsecase
service.ProviderSet → NewAgentService
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/
├── pages/
│   ├── AgentSettingsPage.vue              ← 主页面（QTabs + QTabPanels，9 Tab）
│   └── agent-settings/
│       ├── AgentSettingsAgentTab.vue      ← Agent 属性 Tab（身份/模型/提示模式/能力/心跳）
│       ├── AgentSettingsMemoryTab.vue     ← 记忆 Tab（L0-L4 分层）
│       ├── AgentSettingsSkillsTab.vue     ← Skill / 工具 Tab
│       ├── AgentSettingsPromptSection.vue ← 系统提示模式分区
│       └── AgentChannelRefsSection.vue    ← Channel 引用分区
├── components/agents/
│   ├── AgentSettingsHeader.vue            ← 顶栏（头像/名称/状态/标签/操作）
│   ├── AgentSettingsA2ATab.vue            ← A2A Tab（Endpoint / Proxy）
│   ├── AgentSettingsA2AEndpointTab.vue    ← A2A Endpoint 子组件
│   ├── AgentAdvancedDialog.vue            ← 高级设置对话框
│   ├── AgentEvolutionPanel.vue            ← 进化面板
│   ├── AgentFilesPanel.vue                ← 文件面板（见 6-agent-setting-file）
│   ├── AgentHooksPanel.vue                ← 钩子面板
│   ├── AgentPlannerSection.vue            ← Planner 分区
│   ├── AgentToolsSection.vue              ← 工具分区
│   ├── AgentUsageQuotaPanel.vue           ← 用量配额面板
│   ├── AgentLearningLoopPanel.vue         ← 学习闭环面板
│   ├── LearningPatternList.vue            ← 学习模式列表
│   ├── MemoryLevelSection.vue             ← 记忆分层折叠（L0-L4）
│   ├── AIRefineButton.vue                 ← AI 优化按钮
│   ├── FieldGuideHint.vue                 ← 字段引导提示
│   └── KindBadge.vue                      ← Agent 类型徽章
├── features/agents/
│   ├── types.ts                           ← TypeScript 类型定义
│   ├── api.ts                             ← API 调用函数
│   ├── api.learning.ts                    ← 学习闭环 API
│   ├── wireNormalize.ts                   ← Wire 数据规范化
│   ├── useAgentSettingsPage.ts            ← 设置页主 Composable
│   ├── useAgentRuntimeConfig.ts           ← 运行时配置表单
│   ├── agentRuntimeConfig.ts              ← 默认值 + 选项
│   ├── agentRuntimeConfigHydrate.ts       ← 表单填充
│   ├── agentRuntimeConfigSerialize.ts     ← 表单序列化
│   ├── useAgentA2AEndpointTab.ts          ← A2A Endpoint Composable
│   ├── useAgentA2AProxyTab.ts             ← A2A Proxy Composable
│   ├── useAgentEvolutionPanel.ts          ← 进化面板 Composable
│   ├── useAgentHooksPanel.ts              ← 钩子面板 Composable
│   ├── useAgentPlannerForm.ts             ← Planner 表单 Composable
│   ├── useAgentRalphLoopForm.ts           ← Ralph Loop 表单 Composable
│   ├── useAgentPromptFiles.ts             ← 提示文件 Composable
│   ├── useAgentPromptPreview.ts           ← 提示预览 Composable
│   ├── useAgentSkillCatalog.ts            ← 技能目录 Composable
│   ├── useAgentToolsCatalog.ts            ← 工具目录 Composable
│   ├── useAgentToolOverrides.ts           ← 工具覆盖 Composable
│   ├── useAgentChannelRefs.ts             ← Channel 引用 Composable
│   ├── useAgentAvatarIcon.ts              ← 头像图标 Composable
│   ├── useLearningLoopPanel.ts            ← 学习闭环面板 Composable
│   ├── fieldGuides.ts                     ← FieldGuide 注册表（10 scopes）
│   ├── plannerConfig.ts                   ← Planner 配置
│   ├── ralphLoopConfig.ts                 ← Ralph Loop 配置
│   ├── aiRefine.ts                        ← AI 优化逻辑
│   ├── learning.types.ts                  ← 学习闭环类型
│   ├── learning.utils.ts                  ← 学习闭环工具函数
│   └── agentUtils.ts                      ← Agent 工具函数
```

### 7.2 TypeScript 类型

文件：`web/src/features/agents/types.ts`

```typescript
export type AgentKind = '' | 'llm' | 'a2a_proxy';
export type AgentOwnership = '' | 'user' | 'system_builtin' | 'ecosystem_preset' | 'marketplace' | 'certified';

export type A2AProxyConfig = {
  remote_url: string;
  agent_card_url?: string;
  enable_streaming?: boolean;
  auth_type?: string;
  auth_config_json?: string;
  timeout_seconds?: number;
};

export type Agent = {
  id: string;
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  agent_kind?: AgentKind;
  kind?: AgentOwnership;
  a2a_proxy_config?: A2AProxyConfig;
  a2a_endpoint_enabled?: boolean;
  // ... 其余字段与 Proto Agent 消息对应
  settings?: AgentRuntimeSettings;
  files?: AgentPromptFile[];
};

export type AgentRuntimeSettings = {
  agent_id?: string;
  // ... 130+ 字段，与 Proto AgentRuntimeSettings 消息对应
};

export type AgentPromptFile = {
  id?: string;
  agent_id?: string;
  name: string;
  body: string;
  sort_order: number;
  created_at?: string;
  updated_at?: string;
};
```

### 7.3 API 函数

文件：`web/src/features/agents/api.ts`

```typescript
export async function getAgent(id: string): Promise<Agent>
export async function updateAgent(id: string, payload: Partial<Agent>): Promise<Agent>
export async function getAgentPromptPreview(id: string, mode?: string): Promise<string>
export async function deleteAgent(id: string): Promise<void>
export async function toggleFavorite(id: string): Promise<Agent>
export async function getAgentEffectiveTools(agentId: string): Promise<AgentEffectiveToolsView>
export async function updateAgentToolPolicy(agentId: string, payload: ToolPolicyInput): Promise<AgentEffectiveToolsView>
```

### 7.4 自动保存策略

| 字段类型 | 保存方式 | 实现 |
|----------|----------|------|
| 文本字段 | `debounce(500ms)` + `PATCH /v1/agents/{id}` | `QInput @blur` 触发 |
| Toggle 字段 | `@update:model-value` 立即 PATCH | `QToggle` 直接触发 |
| Settings 字段 | 通过 `updateAgent` 整体 PATCH | Settings 变更合并到 Agent 对象 |
| 工具策略 | `PUT /v1/agents/{id}/tools/policy` | `UpdateAgentToolPolicy` 独立 RPC |

### 7.5 数据规范化

文件：`web/src/features/agents/wireNormalize.ts`

核心函数：
- `normalizeAgentFromService(raw: unknown): Agent` — 将 Wire 响应规范化为 snake_case 类型
- `normalizeRuntimeSettingsFromWire(raw: unknown): AgentRuntimeSettings | undefined`
- `normalizePromptFileFromWire(raw: unknown): AgentPromptFile`
- `runtimeSettingsToWire(s: AgentRuntimeSettings): KratosRuntimeWire` — snake_case → camelCase
- `promptFileToWire(f: AgentPromptFile): KratosFileWire`
- `partialAgentToWire(payload: Partial<Agent>): KratosAgentWire` — 部分更新映射

### 7.6 A2A Tab 设计

> 需求：[5 agent-setting.md](./5%20agent-setting.md) §13 · A2A 架构：[26 a2a-protocol.design.md](./26%20a2a-protocol.design.md) §11.6

**API 复用**：
- `GET/PUT /v1/a2a/agents/{agent_id}/card` — AgentCard CRUD
- `GET /v1/a2a/discover` — Discover 预览
- Proxy 连接测试：可选 `POST /v1/a2a/invoke` 或专用 `DiscoverRemoteAgent`

**Composable**：`useAgentA2AEndpointTab.ts` / `useAgentA2AProxyTab.ts`

| 分支 | 主要状态 | 操作 |
|------|----------|------|
| `agent_kind=llm` | `card.enabled`, `capabilities[]`, `streaming` | `updateCard`；Toggle 启用 Endpoint |
| `agent_kind=a2a_proxy` | `a2a_proxy_config`, 远程 `card`（只读） | PATCH Agent + 重新发现 Card |

---

## 八、运行时映射（system_prompt_mode → FilesForMode）

`system_prompt_mode` 在 `BuildTRPCLLMAgent` 中控制哪些 `AgentPromptFile` 注入到系统提示：

| 模式 | 注入的文件 | 代码实现 |
|------|-----------|---------|
| `complete` | 全部文件（AGENTS_CORE + AGENTS_TASK + SOUL + IDENTITY + USER + USER_PREDEFINED + CAPABILITIES + RULE + HEARTBEAT） | `FilesForMode(files, "complete")` → 返回全部 |
| `task` | AGENTS_CORE + AGENTS_TASK + IDENTITY + CAPABILITIES + RULE + HEARTBEAT | `FilesForMode(files, "task")` → allowed 集合 |
| `minimized` | AGENTS_CORE + IDENTITY + RULE | `FilesForMode(files, "minimized")` → allowed 集合 |
| `none` | 无文件注入 | `FilesForMode(files, "none")` → 空集 |

**代码实现路径**：

- `BuildTRPCLLMAgent`（`internal/agent/trpc_build.go`）读取 `ag.SystemPromptMode` 并传递给 `BuildSystemPrompt`
- `BuildSystemPrompt`（`internal/agent/prompt.go`）接收 mode 参数，调用 `biz.FilesForMode` 过滤文件
- `FilesForMode`（`internal/biz/agent_settings_helpers.go`）已导出，根据模式返回允许的文件子集
- 每个文件内容用 `<internal_config name="{Name}">` 标签包裹，便于 LLM 区分配置块

---

## 九、字段映射汇总（UI ↔ 数据模型）

| UI 区域 | 数据表 / 字段 |
|---------|-----------|
| 顶栏 / 个性 | `agents.display_name`、`agents.agent_key`、`agents.icon`、`agents.status`、`agents.is_default`、`agents.agent_description` |
| 模型与预算 | `agents.provider`、`agents.model`、`agents.context_window`、`agent_runtime_settings.max_tool_iterations`（或 config_json）、`agents.budget_monthly_cents` |
| 系统提示模式 | `agents.system_prompt_mode` |
| TTS | `agents.config_json` 内 TTS 配置 |
| 能力 - 子 Agent | `agent_runtime_settings.subagents_enabled`、`subagents_max_concurrency`、`subagents_max_generation_depth`、`subagents_max_children_per_agent`、`subagents_archive_after_minutes`、`subagents_max_retries`、`subagents_model_override` |
| 能力 - 工具策略 | `agent_runtime_settings.tools_enabled`、`tools_profile`、`tools_tool_call_prefix`、`tools_allow_json`、`tools_deny_json`、`tools_concurrent_allow_json` |
| 记忆 | `agent_runtime_settings.memory_enabled`、`memory_max_chunk_length`、`memory_max_results`、`memory_min_score`、`l0_*` ~ `l4_*` |
| 进化 Tab | `agent_runtime_settings.evolution_self_evolve`、`evolution_skill_evolve`、`evolution_metrics_enabled`、`evolution_suggestions_enabled` |
| 心跳 | `agent_runtime_settings.heartbeat_enabled`、`heartbeat_interval_minutes`、HEARTBEAT.MD 正文（存于 `agent_prompt_files` 或 config_json） |
| 钩子 | `hooks` 表（Agent 关联经 `config_json.condition.agent_id`，见 `28-callback.design.md`） |
| 技能/编排 | `agent_runtime_settings.skill_runtime_json`；编排见 `subagents_*` |
| 分类 | `agents.position_id`、`agents.position_key`（见 `4.agent-type.md`） |
| A2A | `agents.agent_kind`、`agents.a2a_proxy_config`（config_json）；AgentCard 见 `a2a_agent_cards` 表 |

---

## 十、错误处理规格

| 场景 | HTTP 状态码 | 错误码 | 前端行为 |
|------|------------|--------|----------|
| ToolOverride 引用不存在的工具 | 400 Bad Request | `TOOL_NOT_FOUND` | inline error：工具不存在 |
| `config_json` JSON 格式错误 | 400 Bad Request | `CONFIG_JSON_INVALID` | Toast：配置格式错误 |
| RuntimeSettings 字段越界 | 400 Bad Request | `SETTING_OUT_OF_RANGE` | 对应字段 inline error |
| 并发更新冲突 | 409 Conflict | `VERSION_CONFLICT` | Toast：数据已被修改，请刷新 |
| Agent 不存在 | 404 Not Found | `AGENT_NOT_FOUND` | 跳转列表页 |
| agent_key 格式无效 | 400 Bad Request | `AGENT_KEY_INVALID` | inline error：仅允许小写字母、数字、连字符 |
| agent_key 已被占用 | 409 Conflict | `AGENT_KEY_CONFLICT` | inline error：标识已被使用 |

**错误翻译路径**：Data 层错误经 `entErrToBizErr(err, domain)` 翻译为 `*apierror.Error`，Service 层透传，前端通过 `axiosHandler` 解析错误码并展示。

---

*文档版本：与 `api/kratos/agent/v1/agent.proto`、`internal/biz/agent_types.go`、`internal/data/ent/schema/agent_runtime_setting.go` 对齐。需求见 [5 agent-setting.md](./5%20agent-setting.md)，开发计划见 [5 agent-setting.development.md](./5%20agent-setting.development.md)。*
