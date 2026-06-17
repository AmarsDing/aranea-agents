# Agent 文件 Tab — 实现设计文档

> 对应需求：[6 agent-setting-file.md](./6%20agent-setting-file.md)
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

> **文档边界**：本文件包含架构设计、Proto/API 契约、数据模型、接口定义、技术选型、运行时组装、前端组件设计。用户故事与功能需求见需求文档；模块定位、代码锚点、现状评估与任务清单见 [6-agent-setting-file.development.md](./6-agent-setting-file.development.md)。

---

## 一、模块概述

Agent 设置页「文件」Tab：左侧 Markdown 文件列表 + 右侧编辑器。文件存储在 `agent_prompt_files` 表，与 Agent 为 O2M 关系。每个文件对应一个逻辑 Markdown 分片（如 `IDENTITY.md`、`SOUL.md`、`AGENTS_CORE.md` 等），运行时通过 `<internal_config name="...">` 标签包裹注入系统提示词。

---

## 二、数据模型

### 2.1 Ent Schema

文件：`internal/data/ent/schema/agent_prompt_file.go`

表名：`agent_prompt_files`（通过 `entsql.Annotation{Table: "agent_prompt_files"}` 显式映射）

```go
func (AgentPromptFile) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("agent_id").MaxLen(256),
        field.String("file_name").MaxLen(512),
        field.Text("body").Default(""),
        field.Int("sort_order").Default(0),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}

func (AgentPromptFile) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("agent_id", "sort_order").StorageKey("idx_agent_prompt_files_agent"),
    }
}
```

**字段映射注意**：Ent schema 中字段名为 `file_name`，biz 模型中为 `Name`，转换函数需映射（见 §4.3）。

### 2.2 逻辑文件名与职责

| 逻辑文件名 | 职责 | PGO V2 默认集 |
|------------|------|:---:|
| `AGENTS_CORE.md` | 通用操作规则（语言跟随、内部消息处理、禁止 exec 发消息、须用 write 工具保存等） | 是 |
| `AGENTS_TASK.md` | 任务模式规则（memory 召回/写入路径、MEMORY.md 隐私、cron 约定等） | 是 |
| `IDENTITY.md` | 对外身份、角色名、边界 | 是 |
| `CAPABILITIES.md` | 能力描述、可调用工具说明（与 `tools_config` 互补：一文一配置） | 是 |
| `RULE.md` | 硬性规则、约束清单、禁止项与安全/合规要求；与 `AGENTS.md` 区分：后者偏「如何工作与对话」，本文件偏「必须遵守、不可突破」的边界 | 是 |
| `SOUL.md` | 人格、语气、价值观；与 `self_evolve` 联动时仅允许演化风格相关段落 | 否（Legacy） |
| `USER.md` | Agent 级默认/模板：用户维度的上下文 | 否（Legacy） |
| `USER_PREDEFINED.md` | 预置用户画像、偏好说明 | 否（Legacy） |
| `HEARTBEAT.md` | 心跳周期注入的 Markdown；PGO V2 已将心跳迁移至 Settings | 否（Legacy） |
| `USER_CONTEXT.md` | 用户上下文（替代 Legacy USER + USER_PREDEFINED） | 否（可选添加） |

**NULL 与空串**：应用层统一「未配置 = 空串」语义；UI「空」态显示「空」。

### 2.3 AGENTS 拆分策略

与运行时 **FULL / task / minimal** 组装一致：

| 策略 | 说明 |
|------|------|
| **推荐（PGO V2）** | 使用 **`agents_core_md` + `agents_task_md`** 分离；`task` / `minimized` 模式可只注入对应子集 |
| **单文件** | 仅维护 **`agents_md`**：由服务端按「章节标题」拆成 CORE/TASK，或简化产品只保留一段正文 |
| **迁移** | 由 `agents_md` 拆列写入 `agents_core_md` / `agents_task_md` 后，可清空 `agents_md` 避免双源 |

侧栏若展示 **两个** 文件 `AGENTS_CORE.md` / `AGENTS_TASK.md`，则不再展示合并项 `AGENTS.md`。

---

## 三、Proto 层

### 3.1 现有 Proto

文件：`api/kratos/agent/v1/agent.proto`

```protobuf
message AgentPromptFile {
  string id = 1;
  string agent_id = 2;
  string name = 3;
  string body = 4;
  int32 sort_order = 5;
  string created_at = 6;
  string updated_at = 7;
}

message Agent {
  // ... 其他字段
  repeated AgentPromptFile files = 20;
}

message CreateAgentRequest {
  // ... 其他字段
  repeated AgentPromptFile files = 13;
}

message UpdateAgentRequest {
  string id = 1;
  Agent agent = 2;  // agent.files 可提交完整文件列表
}
```

文件通过 `CreateAgentRequest.files` 和 `UpdateAgentRequest.agent.files` 提交，后端使用 `ReplaceAgentPromptFiles` 整体替换策略。

### 3.2 PromptFile 专属 RPC（已实现）

| RPC | HTTP 路径 | 用途 |
|-----|-----------|------|
| `CreateAgentPromptFile` | `POST /v1/agents/{agent_id}/files` | 新增单个文件 |
| `UpdateAgentPromptFile` | `PATCH /v1/agents/{agent_id}/files/{id}` | 更新文件内容 |
| `DeleteAgentPromptFile` | `DELETE /v1/agents/{agent_id}/files/{id}` | 删除文件 |
| `EstimateTokens` | `POST /v1/agents/{agent_id}/files/estimate-tokens` | Token 估算 |
| `EditPromptFileByAI` | `POST /v1/agents/{agent_id}/files/{file_id}/ai-edit` | AI 编辑文件 |

### 3.3 Proto Message 定义

```protobuf
message CreateAgentPromptFileRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
  string name = 2 [(google.api.field_behavior) = REQUIRED];
  string body = 3;
  int32 sort_order = 4;
}

message UpdateAgentPromptFileRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
  string id = 2 [(google.api.field_behavior) = REQUIRED];
  string name = 3;
  string body = 4;
  int32 sort_order = 5;
}

message DeleteAgentPromptFileRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
  string id = 2 [(google.api.field_behavior) = REQUIRED];
}

message EstimateTokensRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message EstimateTokensResponse {
  int32 total_tokens = 1;
  repeated FileTokenEstimate file_estimates = 2;
}

message FileTokenEstimate {
  string file_id = 1;
  string file_name = 2;
  int32 estimated_tokens = 3;
}

message EditPromptFileByAIRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
  string file_id = 2 [(google.api.field_behavior) = REQUIRED];
  string instruction = 3 [(google.api.field_behavior) = REQUIRED];
}

message EditPromptFileByAIResponse {
  AgentPromptFile file = 1;
}
```

---

## 四、Biz 层

### 4.1 领域模型

文件：`internal/biz/agent_types.go`

```go
// AgentPromptFile is one row in agent_prompt_files (API name field maps to file_name).
type AgentPromptFile struct {
    ID        string
    AgentID   string
    Name      string
    Body      string
    SortOrder int
    CreatedAt string
    UpdatedAt string
}

// FileTokenEstimate is the token estimate for a single prompt file.
type FileTokenEstimate struct {
    FileID          string
    FileName        string
    EstimatedTokens int
}

// FileTokenEstimates is the aggregate token estimate for all prompt files of an agent.
type FileTokenEstimates struct {
    TotalTokens   int
    FileEstimates []FileTokenEstimate
}
```

### 4.2 Repo 接口

文件：`internal/biz/agent_usecase.go`（`AgentPromptFileRepo` 接口，`Stability:stable`）

```go
type AgentPromptFileRepo interface {
    ListAgentPromptFiles(ctx context.Context, agentID string) ([]AgentPromptFile, error)
    ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []AgentPromptFile) ([]AgentPromptFile, error)
    CreateAgentPromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
    UpdateAgentPromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
    DeleteAgentPromptFile(ctx context.Context, agentID, id string) error
}
```

> 注：Token 估算逻辑在 `AgentUsecase.EstimateTokens` 中实现（基于 `ListAgentPromptFiles` 结果计算），Repo 层不单独提供 `EstimateTokens` 方法。AI 编辑在 Service 层通过 `PromptFileAIEditor` 实现，Biz 层不涉及。

### 4.3 Usecase 方法

文件：`internal/biz/agent_usecase.go`

```go
func (u *AgentUsecase) CreatePromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
func (u *AgentUsecase) UpdatePromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
func (u *AgentUsecase) DeletePromptFile(ctx context.Context, agentID, id string) error
func (u *AgentUsecase) EstimateTokens(ctx context.Context, agentID string) (FileTokenEstimates, error)
```

每个方法在调用 Repo 前会先 `u.Get(ctx, agentID)` 校验 Agent 存在性，返回 `apierror.NotFound` 当 Agent 不存在。

### 4.4 默认文件集与可选文件

文件：`internal/biz/agent_settings_helpers.go`

```go
// defaultPromptFiles returns the V2 default set (5 core files) when
// PGO_DEFAULT_FILES_V2 is enabled, otherwise returns the legacy 9-file set.
func defaultPromptFiles() []AgentPromptFile

// defaultPromptFilesV2 is the PGO-1 canonical 5-file set.
// SOUL/USER/USER_PREDEFINED are removed; HEARTBEAT moves to Settings.
// USER_CONTEXT.md is available as an optional file via OptionalPromptFileTemplates.
func defaultPromptFilesV2() []AgentPromptFile

// defaultPromptFilesLegacy is the pre-PGO 9-file set, preserved for backward compatibility.
func defaultPromptFilesLegacy() []AgentPromptFile

// OptionalPromptFileTemplates holds optional files that users can add on demand.
// PGO-1-BIZ-01: USER_CONTEXT replaces legacy USER + USER_PREDEFINED.
var OptionalPromptFileTemplates = map[string]AgentPromptFile{
    "USER_CONTEXT.md": { ... },
}
```

**PGO V2 默认集（5 文件）**：`AGENTS_CORE.md` / `AGENTS_TASK.md` / `IDENTITY.md` / `CAPABILITIES.md` / `RULE.md`

**Legacy 兼容集（9 文件）**：上述 5 个 + `SOUL.md` / `USER.md` / `USER_PREDEFINED.md` / `HEARTBEAT.md`

### 4.5 系统提示词模式过滤

文件：`internal/biz/agent_settings_helpers.go`

```go
// FilesForMode filters prompt files by system_prompt_mode.
// PGO-1-BIZ-02: task mode no longer includes HEARTBEAT.md (moved to Settings).
// Whitelist per mode:
//   - complete / "": all files
//   - task:        AGENTS_CORE, IDENTITY, RULE, AGENTS_TASK, CAPABILITIES
//   - minimized:   AGENTS_CORE, RULE
//   - none:        empty
//   - unknown:     AGENTS_CORE, RULE (same as minimized, safe default)
func FilesForMode(files []AgentPromptFile, mode string) []AgentPromptFile
```

**模式映射表**（与代码一致）：

| 逻辑文件名 | `complete` | `task` | `minimized` | `none` | unknown |
|------------|:----------:|:------:|:-----------:|:------:|:-------:|
| `AGENTS_CORE.md` | ✅ | ✅ | ✅ | ❌ | ✅ |
| `AGENTS_TASK.md` | ✅ | ✅ | ❌ | ❌ | ❌ |
| `SOUL.md` | ✅ | ❌ | ❌ | ❌ | ❌ |
| `IDENTITY.md` | ✅ | ✅ | ❌ | ❌ | ❌ |
| `USER.md` | ✅ | ❌ | ❌ | ❌ | ❌ |
| `USER_PREDEFINED.md` | ✅ | ❌ | ❌ | ❌ | ❌ |
| `CAPABILITIES.md` | ✅ | ✅ | ❌ | ❌ | ❌ |
| `RULE.md` | ✅ | ✅ | ✅ | ❌ | ✅ |
| `HEARTBEAT.md` | ✅ | ❌ | ❌ | ❌ | ❌ |

> 注：`minimized` 模式仅包含 `AGENTS_CORE.md` + `RULE.md`（不含 `IDENTITY.md`）。`task` 模式不含 `HEARTBEAT.md`（PGO-1-BIZ-02 已移除）。未知模式回退到 `minimized` 安全默认。

---

## 五、Data 层

### 5.1 已有 Data 层实现

文件：`internal/data/agent_repo.go`

```go
func (r *agentRepo) ListAgentPromptFiles(ctx context.Context, agentID string) ([]biz.AgentPromptFile, error) {
    rows, err := r.data.RW().Read(ctx).AgentPromptFile.Query().
        Where(agentpromptfile.AgentIDEQ(agentID)).
        Order(agentpromptfile.BySortOrder(), agentpromptfile.ByFileName()).
        All(ctx)
    if err != nil {
        return nil, err
    }
    out := make([]biz.AgentPromptFile, 0, len(rows))
    for _, row := range rows {
        out = append(out, entPromptToBiz(row))
    }
    return out, nil
}

func (r *agentRepo) ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
    if agentID == "" {
        return nil, apierror.BadRequest("AGENT", "agent id is required")
    }
    err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
        if _, err := r.data.RW().Write(txCtx).AgentPromptFile.Delete().Where(agentpromptfile.AgentIDEQ(agentID)).Exec(txCtx); err != nil {
            return err
        }
        now := nowRFC3339()
        builders := make([]*ent.AgentPromptFileCreate, 0, len(files))
        for i, file := range files {
            if strings.TrimSpace(file.Name) == "" {
                continue
            }
            id := file.ID
            if id == "" {
                id = fmt.Sprintf("%s_%s", agentID, sanitizePromptFileID(file.Name))
            }
            sortOrder := file.SortOrder
            if sortOrder == 0 {
                sortOrder = (i + 1) * 10
            }
            builders = append(builders, r.data.RW().Write(txCtx).AgentPromptFile.Create().
                SetID(id).SetAgentID(agentID).SetFileName(strings.TrimSpace(file.Name)).
                SetBody(file.Body).SetSortOrder(sortOrder).
                SetCreatedAt(now).SetUpdatedAt(now))
        }
        if len(builders) > 0 {
            if _, err := r.data.RW().Write(txCtx).AgentPromptFile.CreateBulk(builders...).Save(txCtx); err != nil {
                return err
            }
        }
        return nil
    })
    if err != nil {
        return nil, err
    }
    return r.ListAgentPromptFiles(ctx, agentID)
}
```

**访问器规范**：使用 `r.data.RW().Read(ctx)` / `r.data.RW().Write(ctx)`（事务感知读写分离），禁止使用已废弃的 `r.data.entClient` 直连。

**事务管理**：`ReplaceAgentPromptFiles` 通过 `r.data.ExecInTx` 包裹删除+创建，保证原子性。

### 5.2 Ent ↔ Biz 转换函数

文件：`internal/data/agent_repo.go`

```go
func entPromptToBiz(e *ent.AgentPromptFile) biz.AgentPromptFile {
    return biz.AgentPromptFile{
        ID:        e.ID,
        AgentID:   e.AgentID,
        Name:      e.FileName,
        Body:      e.Body,
        SortOrder: e.SortOrder,
        CreatedAt: e.CreatedAt,
        UpdatedAt: e.UpdatedAt,
    }
}
```

### 5.3 ID 生成

文件：`internal/data/agent_repo.go`

```go
func sanitizePromptFileID(value string) string
```

当 `file.ID` 为空时，按 `{agentID}_{sanitizePromptFileID(file.Name)}` 规则生成确定性 ID。

---

## 六、Service 层

### 6.1 已有 Service 方法

文件：`internal/service/agent.go`

```go
func (s *AgentService) CreateAgentPromptFile(ctx context.Context, req *v1.CreateAgentPromptFileRequest) (*v1.AgentPromptFile, error)
func (s *AgentService) UpdateAgentPromptFile(ctx context.Context, req *v1.UpdateAgentPromptFileRequest) (*v1.AgentPromptFile, error)
func (s *AgentService) DeleteAgentPromptFile(ctx context.Context, req *v1.DeleteAgentPromptFileRequest) (*emptypb.Empty, error)
func (s *AgentService) EstimateTokens(ctx context.Context, req *v1.EstimateTokensRequest) (*v1.EstimateTokensResponse, error)
func (s *AgentService) EditPromptFileByAI(ctx context.Context, req *v1.EditPromptFileByAIRequest) (*v1.EditPromptFileByAIResponse, error)
```

所有写操作（Create/Update/Delete/AIEdit）完成后调用 `invalidateAgentBuildCache(req.GetAgentId())` 失效构建缓存。

错误处理：`apierror.IsCode(err, apierror.CodeNotFound)` 时返回 `apierror.NotFound("AGENT_FILE", ...)`。

### 6.2 类型转换函数

文件：`internal/service/agent.go`

```go
func fromProtoFile(pb *v1.AgentPromptFile) biz.AgentPromptFile {
    if pb == nil {
        return biz.AgentPromptFile{}
    }
    return biz.AgentPromptFile{
        ID:        pb.GetId(),
        AgentID:   pb.GetAgentId(),
        Name:      pb.GetName(),
        Body:      pb.GetBody(),
        SortOrder: int(pb.GetSortOrder()),
        CreatedAt: pb.GetCreatedAt(),
        UpdatedAt: pb.GetUpdatedAt(),
    }
}

func toProtoFile(b biz.AgentPromptFile) *v1.AgentPromptFile {
    return &v1.AgentPromptFile{
        Id:        b.ID,
        AgentId:   b.AgentID,
        Name:      b.Name,
        Body:      b.Body,
        SortOrder: int32(b.SortOrder),
        CreatedAt: b.CreatedAt,
        UpdatedAt: b.UpdatedAt,
    }
}
```

### 6.3 AI 编辑实现

文件：`internal/service/agent_prompt_ai.go`

```go
type PromptFileAIEditor struct {
    catalog *biz.LlmProviderModelUsecase
    rt      *provider.RoundTrip
    lg      loggateway.Logger
}

func (e *PromptFileAIEditor) Revise(ctx context.Context, providerName, modelName, fileName, currentBody, instruction string) (string, error)
```

**AI 编辑流程**（`AgentService.EditPromptFileByAI`）：
1. 校验 `agent_id` / `file_id` / `instruction` 非空
2. `s.uc.Get(ctx, agentID)` 加载 Agent
3. 在 `a.Files` 中查找 `target`（按 `fileID`）
4. `s.promptAI.Revise(ctx, a.Provider, a.Model, target.Name, target.Body, instruction)` 调用 LLM 修订
5. `s.uc.UpdatePromptFile(ctx, *target)` 持久化修订结果
6. `invalidateAgentBuildCache(agentID)` 失效缓存
7. 日志：`agent.prompt.ai_edit` FlowLog

**LLM 调用**：`PromptFileAIEditor.resolveModel` 从 catalog 解析 provider/model，90s 超时，流式接收响应并去除 markdown 代码围栏。

---

## 七、Wire 注入

已有（通过 `AgentService` + `AgentUsecase` + `agentRepo`），无需新增 Provider。

`PromptFileAIEditor` 通过 `NewPromptFileAIEditor(catalog, rt, lg)` 注入到 `AgentService.promptAI` 字段。

---

## 八、运行时层

### 8.1 系统提示词组装

文件：`internal/agent/prompt.go`

```go
// BuildSystemPrompt joins agent description and prompt files, filtered by system_prompt_mode.
// PGO-1-AGENT-01: optional categoryResponsibility parameter prepended as
// <role_responsibility source="category"> block when non-empty.
func BuildSystemPrompt(agent biz.Agent, files []biz.AgentPromptFile, mode string, categoryResponsibility ...string) string {
    filtered := biz.FilesForMode(files, mode)
    var b strings.Builder

    // 1. 可选：categoryResponsibility 块（PGO-1-AGENT-01）
    if len(categoryResponsibility) > 0 {
        if cr := strings.TrimSpace(categoryResponsibility[0]); cr != "" {
            b.WriteString("<role_responsibility source=\"category\">\n")
            b.WriteString(cr)
            b.WriteString("\n</role_responsibility>\n\n")
        }
    }

    // 2. 可选：行业上下文（PositionKey + VariantDescription）
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

    // 3. Agent 描述
    if d := strings.TrimSpace(agent.AgentDescription); d != "" {
        b.WriteString(d)
        b.WriteString("\n\n")
    }

    // 4. 过滤后的提示文件，每个用 <internal_config> 包裹
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

**`<internal_config>` 标签包裹**：每个文件内容包裹在 `<internal_config name="{Name}">` 标签中，便于 LLM 区分不同配置块，也支持 Prompt Cache 优化：

```xml
<internal_config name="IDENTITY.md">
身份与对外设定内容...
</internal_config>

<internal_config name="SOUL.md">
人格/语调核心内容...
</internal_config>
```

### 8.2 构建时文件加载

文件：`internal/agent/trpc_build.go`

`BuildTRPCLLMAgent` 在构建 Agent 时：
1. 优先使用 `ag.Files`（内存中的文件）
2. 若为空且 `deps.Agents != nil`，调用 `deps.Agents.ListAgentPromptFiles(ctx, ag.ID)` 从持久层加载
3. 调用 `BuildSystemPrompt(ag, files, ag.SystemPromptMode, catResp)` 组装系统提示词

### 8.3 运行时注入顺序

`IDENTITY.md` → `SOUL.md` → `USER_PREDEFINED.md` → `USER.md` → `CAPABILITIES.md` → `AGENTS_CORE.md` → `AGENTS_TASK.md` → `RULE.md`

`HEARTBEAT.md` 仅在心跳任务注入，不并入普通轮次系统提示。

具体分隔符与标题由服务端统一，注入顺序受 `FilesForMode` 过滤结果影响（见 §4.5）。

---

## 九、Web 前端设计

### 9.1 TypeScript 类型

文件：`web/src/features/agents/types.ts`

```typescript
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

### 9.2 API 调用

文件：`web/src/features/agents/api.ts`

已有：
- `updateAgent(id, payload)` — 通过 `UpdateAgent` 整体提交 `files` 列表
- `getAgent(id)` — 详情包含 `files`

PromptFile 专属 API 通过 `useAgentPromptFiles` composable + `detailStore` 调用：
- `detailStore.estimateTokens(id)` — Token 估算
- `detailStore.editPromptFile(formId, fileId, instruction)` — AI 编辑

### 9.3 Wire Normalize

文件：`web/src/features/agents/wireNormalize.ts`

```typescript
export function normalizePromptFileFromWire(raw: unknown): AgentPromptFile {
  const w = asWireRecord(raw);
  return {
    id: pickStrOpt(w, "id", "id"),
    agent_id: pickStrOpt(w, "agentId", "agent_id"),
    name: pickStr(w, "name", "name"),
    body: pickStr(w, "body", "body"),
    sort_order: pickNum(w, "sortOrder", "sort_order", 0),
    created_at: pickStrOpt(w, "createdAt", "created_at"),
    updated_at: pickStrOpt(w, "updatedAt", "updated_at")
  };
}

export function promptFileToWire(f: AgentPromptFile): KratosFileWire {
  return {
    id: f.id,
    agentId: f.agent_id,
    name: f.name,
    body: f.body,
    sortOrder: f.sort_order,
    createdAt: f.created_at,
    updatedAt: f.updated_at
  };
}
```

### 9.4 Composable 设计

文件：`web/src/features/agents/useAgentPromptFiles.ts`

核心状态与方法：
- `fileSplitter` / `activeFile` / `initialFileBodies` — 编辑器状态
- `aiEditOpen` / `aiEditing` / `aiInstruction` — AI 编辑弹窗状态
- `fileTokenByName` — Token 估算缓存
- `files` — 响应式文件列表（基于 `coreAgentFiles` 初始化）
- `availableOptionalFiles` / `addOptionalFile(name)` — 可选文件管理
- `hydrateFiles(savedFiles)` — 从后端数据填充
- `refreshFileTokenEstimates(formId)` — 调用 `EstimateTokens` API
- `applyAiEdit(formId)` — 调用 `EditPromptFileByAI` API
- `filesForSave()` — 生成保存负载

文件：`web/src/features/agents/useAgentPromptPreview.ts` — 提示词预览 composable
文件：`web/src/features/agents/aiRefine.ts` — AI Refine 逻辑（diff preview）
文件：`web/src/features/agents/fieldGuides.ts` — FieldGuide（6 file scopes）

### 9.5 Vue 组件设计

#### AgentFilesPanel.vue

文件：`web/src/components/agents/AgentFilesPanel.vue`

```vue
<template>
  <QSplitter :model-value="280" unit="px" :limits="[220, 400]">
    <template #before>
      <div class="file-sidebar q-pa-sm">
        <div class="row items-center q-mb-sm">
          <span class="text-subtitle2">文件</span>
          <QSpace />
          <QBtn flat round dense icon="add" @click="onCreateFile">
            <QTooltip>新增文件</QTooltip>
          </QBtn>
        </div>
        <QList separator dense>
          <QItem
            v-for="file in sortedFiles"
            :key="file.id ?? file.name"
            :active="selectedFile?.name === file.name"
            clickable
            @click="selectFile(file)"
          >
            <QItemSection>
              <QItemLabel>{{ file.name }}</QItemLabel>
              <QItemLabel caption>
                <template v-if="!file.body">空</template>
                <template v-else>估计 {{ estimateFor(file) }} token</template>
              </QItemLabel>
            </QItemSection>
          </QItem>
        </QList>
      </div>
    </template>

    <template #after>
      <div v-if="selectedFile" class="file-editor q-pa-md">
        <div class="row items-center q-mb-md">
          <div>
            <div class="text-h6">{{ selectedFile.name }}</div>
            <div class="text-caption text-grey">{{ fileSubtitle(selectedFile.name) }}</div>
          </div>
          <QSpace />
          <QBtn flat label="重新召唤" icon="refresh" @click="onRecall" />
          <AIRefineButton ... />
          <QBtn
            unelevated
            label="保存"
            color="primary"
            :disable="!isDirty"
            @click="onSave"
          />
        </div>

        <QInput
          v-model="editBody"
          type="textarea"
          filled
          autogrow
          class="editor-textarea"
        />
      </div>
    </template>
  </QSplitter>
</template>
```

#### 文件副标题映射

```typescript
const FILE_SUBTITLES: Record<string, string> = {
  "AGENTS_CORE.md": "通用操作规则（语言、系统消息、保存工具约束等）",
  "AGENTS_TASK.md": "任务向规则（memory 路径、cron、MEMORY.md 隐私等）",
  "SOUL.md": "人格/语调核心",
  "IDENTITY.md": "身份与对外设定",
  "USER.md": "用户侧上下文（每用户）",
  "USER_PREDEFINED.md": "预置用户相关说明",
  "CAPABILITIES.md": "能力边界与工具使用说明",
  "RULE.md": "硬性规则、约束与禁止项",
  "HEARTBEAT.md": "心跳注入清单",
  "USER_CONTEXT.md": "用户上下文（可选）"
};
```

#### 关联组件

- `web/src/components/agents/AIRefineButton.vue` — AI Refine 按钮（带 diff preview）
- `web/src/components/agents/MemoryOptionalFilesSection.vue` — 可选文件添加区
- `web/src/pages/AgentSettingsPage.vue` — `files` Tab 宿主

---

## 十、设计验收要点

- [ ] 左侧文件列表按 `sort_order` 排序，显示逻辑文件名 + Token 估算
- [ ] 点击文件 → 右侧加载内容，编辑后脏状态标记，保存按钮启用
- [ ] 保存通过 `UpdateAgent` 整体提交 `files` 列表，后端 `ReplaceAgentPromptFiles` 替换
- [ ] 「重新召唤」从服务端重新拉取最新值，未保存时有确认提示
- [ ] AI 编辑弹窗：输入指令 → 后端 `EditPromptFileByAI` 返回修订稿 → 写入编辑器
- [ ] Token 估算：前端近似（字符/4）或调用 `EstimateTokens` API
- [ ] `HEARTBEAT.md` 与 Agent 页心跳卡片同源字段，无两套存储（PGO V2 已迁移至 Settings）
- [ ] 系统提示词模式过滤（`complete`/`task`/`minimized`/`none`）与运行时 `FilesForMode` 一致
- [ ] 运行时注入顺序与侧栏展示顺序一致
- [ ] `BuildSystemPrompt` 正确包裹 `<internal_config>` 标签，空 body 文件跳过
