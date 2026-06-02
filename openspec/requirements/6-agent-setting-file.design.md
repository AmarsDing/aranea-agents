# Agent 文件 Tab — 实现设计文档

> 对应需求：`6 agent-setting-file.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 设置页「文件」Tab：左侧 Markdown 文件列表 + 右侧编辑器。文件存储在 `agent_prompt_files` 表，与 Agent 为 O2M 关系。每个文件对应一个逻辑 Markdown 分片（如 `IDENTITY.md`、`SOUL.md`、`AGENTS_CORE.md` 等），运行时通过 `<internal_config name="...">` 标签包裹注入系统提示词。

---

## 二、Proto 层

### 2.1 现有 Proto

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
  repeated AgentPromptFile files = 14;
}

message UpdateAgentRequest {
  string id = 1;
  Agent agent = 2;  // agent.files 可提交完整文件列表
}
```

文件通过 `CreateAgentRequest.files` 和 `UpdateAgentRequest.agent.files` 提交，后端使用 `ReplaceAgentPromptFiles` 整体替换策略。

### 2.2 待新增 RPC

| RPC | 路径 | 用途 |
|-----|------|------|
| `CreateAgentPromptFile` | `POST /v1/agents/{agent_id}/files` | 新增单个文件 |
| `UpdateAgentPromptFile` | `PATCH /v1/agents/{agent_id}/files/{id}` | 更新文件内容 |
| `DeleteAgentPromptFile` | `DELETE /v1/agents/{agent_id}/files/{id}` | 删除文件 |
| `EstimateTokens` | `POST /v1/agents/{agent_id}/files/estimate-tokens` | Token 估算 |

### 2.3 待新增 Proto Message

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
```

---

## 三、Biz 层

### 3.1 领域模型

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

### 3.2 Repo 接口（已定义在 AgentRepository 中）

```go
type AgentRepository interface {
    // ... 其他方法
    ListAgentPromptFiles(ctx context.Context, agentID string) ([]AgentPromptFile, error)
    ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []AgentPromptFile) ([]AgentPromptFile, error)
}
```

### 3.3 待新增独立 Repo 接口

```go
type AgentPromptFileRepository interface {
    ListByAgent(ctx context.Context, agentID string) ([]AgentPromptFile, error)
    GetByID(ctx context.Context, id string) (AgentPromptFile, error)
    Create(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
    Update(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
    Delete(ctx context.Context, id string) error
    Reorder(ctx context.Context, agentID string, ids []string) error
    EstimateTokens(ctx context.Context, agentID string) (TokenEstimate, error)
}

type TokenEstimate struct {
    TotalTokens     int
    FileEstimates   []FileTokenEstimate
}

type FileTokenEstimate struct {
    FileID          string
    FileName        string
    EstimatedTokens int
}
```

### 3.4 Usecase 方法

```go
func (uc *AgentUsecase) ListPromptFiles(ctx context.Context, agentID string) ([]AgentPromptFile, error)
func (uc *AgentUsecase) CreatePromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
func (uc *AgentUsecase) UpdatePromptFile(ctx context.Context, f AgentPromptFile) (AgentPromptFile, error)
func (uc *AgentUsecase) DeletePromptFile(ctx context.Context, agentID, id string) error
func (uc *AgentUsecase) ReorderPromptFiles(ctx context.Context, agentID string, ids []string) error
func (uc *AgentUsecase) EstimateFileTokens(ctx context.Context, agentID string) (TokenEstimate, error)
```

### 3.5 系统提示词模式过滤

```go
func FilesForMode(files []AgentPromptFile, mode string) []AgentPromptFile {
    if mode == "complete" || mode == "" {
        return files
    }
    allowed := map[string]bool{}
    switch mode {
    case "task":
        allowed = map[string]bool{
            "AGENTS_CORE.md": true, "AGENTS_TASK.md": true,
            "IDENTITY.md": true, "CAPABILITIES.md": true,
            "RULE.md": true, "HEARTBEAT.md": true,
        }
    case "minimized":
        allowed = map[string]bool{
            "AGENTS_CORE.md": true, "IDENTITY.md": true, "RULE.md": true,
        }
    case "none":
        return nil
    }
    var out []AgentPromptFile
    for _, f := range files {
        if allowed[f.Name] {
            out = append(out, f)
        }
    }
    return out
}
```

---

## 四、Data 层

### 4.1 Ent Schema（已存在）

文件：`internal/data/ent/schema/agent_prompt_file.go`

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
```

注意：Ent schema 中字段名为 `file_name`，biz 模型中为 `Name`，转换函数需映射。

### 4.2 已有 Data 层实现

文件：`internal/data/agent_repo.go`

```go
func (r *agentRepo) ListAgentPromptFiles(ctx context.Context, agentID string) ([]biz.AgentPromptFile, error) {
    rows, err := r.data.entClient.AgentPromptFile.Query().
        Where(agentpromptfile.AgentIDEQ(agentID)).
        Order(ent.Asc(agentpromptfile.FieldSortOrder)).
        All(ctx)
    if err != nil {
        return nil, err
    }
    out := make([]biz.AgentPromptFile, 0, len(rows))
    for _, row := range rows {
        out = append(out, entPromptFileToBiz(row))
    }
    return out, nil
}

func (r *agentRepo) ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
    tx, _ := r.data.entClient.Tx(ctx)
    tx.AgentPromptFile.Delete().Where(agentpromptfile.AgentIDEQ(agentID)).Exec(ctx)
    for i, file := range files {
        id := file.ID
        if id == "" {
            id = fmt.Sprintf("%s_%s", agentID, sanitizePromptFileID(file.Name))
        }
        sortOrder := file.SortOrder
        if sortOrder == 0 {
            sortOrder = (i + 1) * 10
        }
        tx.AgentPromptFile.Create().
            SetID(id).SetAgentID(agentID).SetFileName(file.Name).
            SetBody(file.Body).SetSortOrder(sortOrder).
            Save(ctx)
    }
    tx.Commit()
    return r.ListAgentPromptFiles(ctx, agentID)
}
```

### 4.3 Ent ↔ Biz 转换函数

```go
func entPromptFileToBiz(row *ent.AgentPromptFile) biz.AgentPromptFile {
    return biz.AgentPromptFile{
        ID:        row.ID,
        AgentID:   row.AgentID,
        Name:      row.FileName,
        Body:      row.Body,
        SortOrder: row.SortOrder,
        CreatedAt: row.CreatedAt,
        UpdatedAt: row.UpdatedAt,
    }
}
```

### 4.4 待新增 Data 层方法

```go
func (r *agentPromptFileRepo) GetByID(ctx context.Context, id string) (biz.AgentPromptFile, error) {
    row, err := r.data.entClient.AgentPromptFile.Get(ctx, id)
    if ent.IsNotFound(err) {
        return biz.AgentPromptFile{}, sql.ErrNoRows
    }
    return entPromptFileToBiz(row), nil
}

func (r *agentPromptFileRepo) Create(ctx context.Context, f biz.AgentPromptFile) (biz.AgentPromptFile, error) {
    id := f.ID
    if id == "" {
        id = fmt.Sprintf("%s_%s", f.AgentID, sanitizePromptFileID(f.Name))
    }
    row, err := r.data.entClient.AgentPromptFile.Create().
        SetID(id).SetAgentID(f.AgentID).SetFileName(f.Name).
        SetBody(f.Body).SetSortOrder(f.SortOrder).
        Save(ctx)
    if err != nil {
        return biz.AgentPromptFile{}, err
    }
    return entPromptFileToBiz(row), nil
}

func (r *agentPromptFileRepo) Update(ctx context.Context, f biz.AgentPromptFile) (biz.AgentPromptFile, error) {
    update := r.data.entClient.AgentPromptFile.UpdateOneID(f.ID)
    if f.Name != "" {
        update.SetFileName(f.Name)
    }
    if f.Body != "" {
        update.SetBody(f.Body)
    }
    update.SetSortOrder(f.SortOrder)
    row, err := update.Save(ctx)
    if ent.IsNotFound(err) {
        return biz.AgentPromptFile{}, sql.ErrNoRows
    }
    return entPromptFileToBiz(row), nil
}

func (r *agentPromptFileRepo) Delete(ctx context.Context, id string) error {
    return r.data.entClient.AgentPromptFile.DeleteOneID(id).Exec(ctx)
}

func (r *agentPromptFileRepo) Reorder(ctx context.Context, agentID string, ids []string) error {
    for i, id := range ids {
        r.data.entClient.AgentPromptFile.UpdateOneID(id).
            SetSortOrder((i + 1) * 10).
            Save(ctx)
    }
    return nil
}

func (r *agentPromptFileRepo) EstimateTokens(ctx context.Context, agentID string) (biz.TokenEstimate, error) {
    files, err := r.ListByAgent(ctx, agentID)
    if err != nil {
        return biz.TokenEstimate{}, err
    }
    total := 0
    estimates := make([]biz.FileTokenEstimate, 0, len(files))
    for _, f := range files {
        tokens := len([]rune(f.Body)) / 4
        total += tokens
        estimates = append(estimates, biz.FileTokenEstimate{
            FileID:          f.ID,
            FileName:        f.Name,
            EstimatedTokens: tokens,
        })
    }
    return biz.TokenEstimate{TotalTokens: total, FileEstimates: estimates}, nil
}
```

---

## 五、Service 层

### 5.1 已有 Service 方法

文件：`internal/service/agent.go`

```go
func (s *AgentService) CreateAgent(ctx context.Context, req *v1.CreateAgentRequest) (*v1.Agent, error) {
    // ... req.Files 通过 toProtoAgent/fromProtoAgent 转换
}

func (s *AgentService) UpdateAgent(ctx context.Context, req *v1.UpdateAgentRequest) (*v1.Agent, error) {
    // ... req.Agent.Files 通过 ReplaceAgentPromptFiles 整体替换
}
```

### 5.2 类型转换函数

```go
func toProtoPromptFile(f biz.AgentPromptFile) *v1.AgentPromptFile {
    return &v1.AgentPromptFile{
        Id:        f.ID,
        AgentId:   f.AgentID,
        Name:      f.Name,
        Body:      f.Body,
        SortOrder: int32(f.SortOrder),
        CreatedAt: f.CreatedAt,
        UpdatedAt: f.UpdatedAt,
    }
}

func fromProtoPromptFile(f *v1.AgentPromptFile) biz.AgentPromptFile {
    return biz.AgentPromptFile{
        ID:        f.GetId(),
        AgentID:   f.GetAgentId(),
        Name:      f.GetName(),
        Body:      f.GetBody(),
        SortOrder: int(f.GetSortOrder()),
        CreatedAt: f.GetCreatedAt(),
        UpdatedAt: f.GetUpdatedAt(),
    }
}
```

### 5.3 待新增 Service 方法

```go
func (s *AgentService) CreateAgentPromptFile(ctx context.Context, req *v1.CreateAgentPromptFileRequest) (*v1.AgentPromptFile, error) {
    f := biz.AgentPromptFile{
        AgentID:   req.GetAgentId(),
        Name:      req.GetName(),
        Body:      req.GetBody(),
        SortOrder: int(req.GetSortOrder()),
    }
    out, err := s.uc.CreatePromptFile(ctx, f)
    if err != nil {
        return nil, err
    }
    return toProtoPromptFile(out), nil
}

func (s *AgentService) UpdateAgentPromptFile(ctx context.Context, req *v1.UpdateAgentPromptFileRequest) (*v1.AgentPromptFile, error) {
    f := biz.AgentPromptFile{
        ID:        req.GetId(),
        AgentID:   req.GetAgentId(),
        Name:      req.GetName(),
        Body:      req.GetBody(),
        SortOrder: int(req.GetSortOrder()),
    }
    out, err := s.uc.UpdatePromptFile(ctx, f)
    if stderrors.Is(err, sql.ErrNoRows) {
        return nil, kerrors.NotFound("AGENT_PROMPT_FILE", "file not found")
    }
    return toProtoPromptFile(out), nil
}

func (s *AgentService) DeleteAgentPromptFile(ctx context.Context, req *v1.DeleteAgentPromptFileRequest) (*emptypb.Empty, error) {
    err := s.uc.DeletePromptFile(ctx, req.GetAgentId(), req.GetId())
    if err != nil {
        return nil, err
    }
    return &emptypb.Empty{}, nil
}

func (s *AgentService) EstimateTokens(ctx context.Context, req *v1.EstimateTokensRequest) (*v1.EstimateTokensResponse, error) {
    est, err := s.uc.EstimateFileTokens(ctx, req.GetAgentId())
    if err != nil {
        return nil, err
    }
    resp := &v1.EstimateTokensResponse{TotalTokens: int32(est.TotalTokens)}
    for _, fe := range est.FileEstimates {
        resp.FileEstimates = append(resp.FileEstimates, &v1.FileTokenEstimate{
            FileId:          fe.FileID,
            FileName:        fe.FileName,
            EstimatedTokens: int32(fe.EstimatedTokens),
        })
    }
    return resp, nil
}
```

---

## 六、Wire 注入

已有（通过 `AgentService` + `AgentUsecase` + `agentRepo`），无需新增 Provider。

若新增独立 `AgentPromptFileRepository`，需在 `data.ProviderSet` 添加 `NewAgentPromptFileRepo`，在 `biz.ProviderSet` 更新 `NewAgentUsecase` 参数。

---

## 七、运行时层

### 7.1 系统提示词组装

文件：`internal/agent/prompt.go`

```go
func BuildSystemPrompt(agent biz.Agent, files []biz.AgentPromptFile, mode string) string {
    var b strings.Builder
    if agent.AgentDescription != "" {
        b.WriteString(agent.AgentDescription)
        b.WriteString("\n\n")
    }
    filtered := biz.FilesForMode(files, mode)
    for _, f := range filtered {
        fmt.Fprintf(&b, "<internal_config name=%q>\n", f.Name)
        b.WriteString(f.Body)
        b.WriteString("\n</internal_config>\n\n")
    }
    return b.String()
}
```

### 7.2 文件名与模式映射

| 逻辑文件名 | `complete` | `task` | `minimized` | `none` |
|------------|:----------:|:------:|:-----------:|:------:|
| `AGENTS_CORE.md` | ✅ | ✅ | ✅ | ❌ |
| `AGENTS_TASK.md` | ✅ | ✅ | ❌ | ❌ |
| `SOUL.md` | ✅ | ❌ | ❌ | ❌ |
| `IDENTITY.md` | ✅ | ✅ | ✅ | ❌ |
| `USER.md` | ✅ | ❌ | ❌ | ❌ |
| `USER_PREDEFINED.md` | ✅ | ❌ | ❌ | ❌ |
| `CAPABILITIES.md` | ✅ | ✅ | ❌ | ❌ |
| `RULE.md` | ✅ | ✅ | ✅ | ❌ |
| `HEARTBEAT.md` | ✅ | ✅ | ❌ | ❌ |

### 7.3 运行时注入顺序

`IDENTITY.md` → `SOUL.md` → `USER_PREDEFINED.md` → `USER.md` → `CAPABILITIES.md` → `AGENTS_CORE.md` → `AGENTS_TASK.md` → `RULE.md`

`HEARTBEAT.md` 仅在心跳任务注入，不并入普通轮次系统提示。

---

## 八、Web 前端设计

### 8.1 TypeScript 类型（已存在）

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

### 8.2 API 调用（已存在部分）

文件：`web/src/features/agents/api.ts`

```typescript
export async function updateAgent(id: string, payload: Partial<Agent>): Promise<Agent> {
  const svc = createAgentService();
  const data = await svc.UpdateAgent({
    id,
    agent: partialAgentToWire(payload)
  });
  return normalizeAgentFromService(data);
}
```

### 8.3 待新增 API 调用

```typescript
export async function createPromptFile(
  agentId: string,
  req: { name: string; body?: string; sort_order?: number }
): Promise<AgentPromptFile> {
  const svc = createAgentService();
  const data = await svc.CreateAgentPromptFile({
    agentId,
    name: req.name,
    body: req.body ?? "",
    sortOrder: req.sort_order ?? 0
  });
  return normalizePromptFileFromWire(data);
}

export async function updatePromptFile(
  agentId: string,
  id: string,
  req: { name?: string; body?: string; sort_order?: number }
): Promise<AgentPromptFile> {
  const svc = createAgentService();
  const data = await svc.UpdateAgentPromptFile({
    agentId,
    id,
    name: req.name,
    body: req.body,
    sortOrder: req.sort_order
  });
  return normalizePromptFileFromWire(data);
}

export async function deletePromptFile(agentId: string, id: string): Promise<void> {
  const svc = createAgentService();
  await svc.DeleteAgentPromptFile({ agentId, id });
}

export async function estimateFileTokens(agentId: string): Promise<{
  total_tokens: number;
  file_estimates: { file_id: string; file_name: string; estimated_tokens: number }[];
}> {
  const svc = createAgentService();
  const res = await svc.EstimateTokens({ agentId });
  return {
    total_tokens: res.totalTokens ?? 0,
    file_estimates: (res.fileEstimates ?? []).map((e) => ({
      file_id: e.fileId ?? "",
      file_name: e.fileName ?? "",
      estimated_tokens: e.estimatedTokens ?? 0
    }))
  };
}
```

### 8.4 Wire Normalize（已存在）

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

### 8.5 Vue 组件设计

#### AgentFilesTab.vue

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
          <QBtn flat label="AI 编辑" icon="auto_fix_high" @click="showAiEdit = true" />
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
      <div v-else class="column items-center justify-center" style="height: 100%">
        <QIcon name="description" size="48px" color="grey-6" />
        <div class="text-grey-6 q-mt-sm">选择左侧文件开始编辑</div>
      </div>
    </template>
  </QSplitter>

  <QDialog v-model="showAiEdit">
    <QCard style="min-width: 480px">
      <QCardSection class="row items-center">
        <QIcon name="auto_fix_high" class="q-mr-sm" />
        <span class="text-h6">AI 编辑</span>
        <QSpace />
        <QBtn flat round dense icon="close" @click="showAiEdit = false" />
      </QCardSection>
      <QCardSection>
        <div class="text-caption q-mb-sm">
          描述您想要更改的内容。AI 将读取当前文件并相应更新。
        </div>
        <QInput v-model="aiInstruction" type="textarea" filled rows="4"
          placeholder="使 Agent 更正式、添加中文支持、将名称改为 Luna…" />
      </QCardSection>
      <QCardActions align="right">
        <QBtn flat label="取消" @click="showAiEdit = false" />
        <QBtn unelevated label="重新生成" color="primary" @click="onAiEdit" />
      </QCardActions>
    </QCard>
  </QDialog>
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
  "HEARTBEAT.md": "心跳注入清单"
};

function fileSubtitle(name: string): string {
  return FILE_SUBTITLES[name] ?? "";
}
```

#### 交互逻辑

```typescript
const props = defineProps<{ agent: Agent }>();
const emit = defineEmits<{ (e: "update", agent: Agent): void }>();

const sortedFiles = computed(() =>
  [...(props.agent.files ?? [])].sort((a, b) => a.sort_order - b.sort_order)
);
const selectedFile = ref<AgentPromptFile | null>(null);
const editBody = ref("");
const originalBody = ref("");
const isDirty = computed(() => editBody.value !== originalBody.value);
const showAiEdit = ref(false);
const aiInstruction = ref("");

function selectFile(file: AgentPromptFile) {
  if (isDirty.value) {
    confirmUnsaved();
  }
  selectedFile.value = file;
  editBody.value = file.body;
  originalBody.value = file.body;
}

async function onSave() {
  if (!selectedFile.value || !isDirty.value) return;
  const updatedFiles = (props.agent.files ?? []).map((f) =>
    f.name === selectedFile.value!.name ? { ...f, body: editBody.value } : f
  );
  const updated = await updateAgent(props.agent.id, { files: updatedFiles });
  emit("update", updated);
  originalBody.value = editBody.value;
}

async function onRecall() {
  const fresh = await getAgent(props.agent.id);
  emit("update", fresh);
  if (selectedFile.value) {
    const reloaded = fresh.files?.find((f) => f.name === selectedFile.value!.name);
    if (reloaded) {
      selectedFile.value = reloaded;
      editBody.value = reloaded.body;
      originalBody.value = reloaded.body;
    }
  }
}

async function onAiEdit() {
  // TODO: 调用后端 AI 编辑接口，返回修订稿写入编辑器
}

function estimateFor(file: AgentPromptFile): number {
  return Math.ceil((file.body?.length ?? 0) / 4);
}
```

---

## 九、验收要点

- [ ] 左侧文件列表按 `sort_order` 排序，显示逻辑文件名 + Token 估算
- [ ] 点击文件 → 右侧加载内容，编辑后脏状态标记，保存按钮启用
- [ ] 保存通过 `UpdateAgent` 整体提交 `files` 列表，后端 `ReplaceAgentPromptFiles` 替换
- [ ] 「重新召唤」从服务端重新拉取最新值，未保存时有确认提示
- [ ] AI 编辑弹窗：输入指令 → 后端返回修订稿 → 写入编辑器
- [ ] Token 估算：前端近似（字符/4）或调用 `EstimateTokens` API
- [ ] `HEARTBEAT.md` 与 Agent 页心跳卡片同源字段，无两套存储
- [ ] 系统提示词模式过滤（`complete`/`task`/`minimized`/`none`）与运行时 `FilesForMode` 一致
- [ ] 运行时注入顺序与侧栏展示顺序一致
