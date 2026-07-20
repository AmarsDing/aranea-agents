# Session 管理模块 — 实现设计文档

> 对应需求：[10-session.md](./10-session.md) · 开发计划：[10-session.development.md](./10-session.development.md)
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · 运行时边界：[AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)

---

## 〇、分层与单一职责（影响域）

| 层 / 包 | 职责 | 禁止 |
|---------|------|------|
| `api/kratos/session/v1` | 对外 RPC/HTTP 契约 | 业务分支 |
| `internal/service/session.go` | Proto ↔ biz、Audit、**不含**消息发送/Runner Run | import `pkg/trpc-agent-go` 编排 |
| `internal/service/session_batch.go` | 批量预览/归档/删除 RPC | 与 Timeline 聚合混写 |
| `internal/service/session_observability.go` | Export / ListSessionRuns / ListSessionParticipants / ListChildSessions / ListActivities | — |
| `internal/service/session_status_guard.go` | 状态守卫（running 不可删/归档） | — |
| `internal/service/session_context_window.go` | 上下文窗口解析 | — |
| `internal/service/session_projection.go` | Runner 事件投影 | — |
| `internal/service/session_run_durable_worker.go` | Run 持久化 worker | — |
| `internal/service/session_title_llm.go` | LLM 标题生成 | — |
| `internal/biz/session/` | SessionUsecase + 独立子用例（compression/batch/export/pin/participant/timeline/state/turns/messages/metrics/summary/title/status） | import trpc-agent-go |
| `internal/biz/session_batch.go` | cutoff 解析、scope 扫描、批量命中（与 CRUD 分文件） | SQL |
| `internal/data/session_repo*.go` | Ent/SQL 持久化 | 业务规则（running 不可删在 biz 校验） |
| `internal/data/message_search.go` | 消息 FTS/LIKE 检索 | — |
| `internal/agent` + `sessionmemory` | Runner 事件投影、L0 记忆链 | 替代 `SessionUsecase` 做列表 CRUD |

**Chat 发送路径**：`ChatService` → `SessionUsecase.AppendChat*` / `UpdateRunnerSnapshotJSON` / `SessionCompressor`；**不**在 `SessionService` 内调用 Runner。

**批量治理影响域**：`Batch*` RPC → `session_batch.go` (biz) → `session_repo_batch.go` (data)；Monitor Audit `archive.session.batch` / `delete.session.batch`。

---

## 一、模块概述

会话历史存储与编排：Session CRUD、Timeline 时间轴、上下文管理、摘要压缩。逐步向 trpc-agent-go `session.Service` 对齐。

核心能力：
- Session 搜索/创建/删除/归档/恢复/重命名/部分更新/状态转换
- Timeline 时间轴聚合（消息 + 工具调用 + Skill 调用 + MCP 调用）
- 上下文窗口消耗追踪与状态管理
- 异步摘要压缩（SessionCompressor）+ 手动压缩（CompactSession）
- Runner Snapshot 持久化与压缩重写
- Session Summaries 滚动摘要
- Session Turns 对话轮次记录
- Session State KV 状态管理
- 自动标题生成（用户消息截取 + LLM 异步生成）
- 单 Agent / Team 双模式会话
- 会话树（parent/root session_id + agent_depth）
- 编排可观测性（Runs / Participants / Activities）

---

## 二、Proto 层

### 2.1 完整 Proto 定义

文件：`api/kratos/session/v1/session.proto`

> 完整定义以仓库内 `api/kratos/session/v1/session.proto` 为准；以下为关键消息与服务索引。

#### Session 消息

```protobuf
message Session {
  string id = 1;
  string workspace_id = 31;
  string user_id = 32;
  string owner_type = 2;           // "agent" | "team"
  string agent_id = 3;
  string team_id = 4;
  string title = 5;
  string summary = 6;
  string tags_json = 33;
  string dialog_mode = 12;
  string default_provider = 34;
  string default_model = 35;
  int32 default_context_window_tokens = 36;
  string last_provider = 37;
  string last_model = 38;
  int32 last_context_window_tokens = 10;
  string status = 15;              // idle/running/completed/interrupted/awaiting_confirmation/archived/deleted
  string status_reason = 48;
  string status_changed_at = 49;
  string visibility = 39;          // private/team/workspace
  int32 message_count = 16;
  int32 run_count = 17;
  int32 model_call_count = 18;
  int32 tool_call_count = 19;
  int32 skill_call_count = 20;
  int32 mcp_call_count = 21;
  int32 input_tokens = 22;
  int32 output_tokens = 23;
  int32 total_tokens = 24;
  int64 total_cost_micro_usd = 25;
  double avg_latency_ms = 40;
  int32 error_count = 41;
  int32 context_used_tokens = 8;
  double context_used_ratio = 7;
  double max_context_used_ratio = 9;
  string context_status = 11;      // normal/warning/critical/exceeded
  string first_message_at = 42;
  string last_message_at = 26;
  string last_run_at = 43;
  string created_at = 27;
  string updated_at = 28;
  string archived_at = 29;
  string deleted_at = 30;
  string runner_snapshot_json = 44;
  string metadata_json = 45;
  string state_json = 46;
  string pinned_at = 47;
  string parent_session_id = 50;
  string root_session_id = 51;
  int32 agent_depth = 52;
}
```

#### ChatMessageRow 消息

```protobuf
message ChatMessageRow {
  string id = 1;
  string session_id = 2;
  string parent_message_id = 3;
  string turn_id = 4;
  int32 turn_number = 16;
  int32 seq_in_turn = 17;
  string role = 5;
  string content_markdown = 6;
  string model_name = 7;
  int32 token_in = 8;
  int32 token_out = 9;
  int32 latency_ms = 10;
  string status = 11;
  int32 attachments_count = 12;
  string options_json = 13;
  string error_message = 14;
  string created_at = 15;
}
```

#### SessionTurn 消息

```protobuf
message SessionTurn {
  string id = 1;
  string session_id = 2;
  string run_id = 3;
  int32 turn_number = 4;
  string user_message_id = 5;
  string assistant_message_id = 6;
  string owner_type = 7;
  string agent_id = 8;
  string team_id = 9;
  string status = 10;
  string started_at = 11;
  string ended_at = 12;
  int32 duration_ms = 13;
  int32 first_token_ms = 14;
  int32 model_call_count = 15;
  int32 tool_call_count = 16;
  int32 skill_call_count = 17;
  int32 mcp_call_count = 18;
  int32 input_tokens = 19;
  int32 output_tokens = 20;
  int32 total_tokens = 21;
  int64 total_cost_micro_usd = 22;
  string final_provider = 23;
  string final_model = 24;
  string final_content_preview = 25;
  string error_code = 26;
  string error_message = 27;
  string metadata_json = 28;
  string created_at = 29;
  string updated_at = 30;
}
```

#### ListSessionMessages 请求/响应

```protobuf
message ListSessionMessagesRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  int32 limit = 2;
  int32 offset = 3;
  optional int64 after_revision = 4;
}

message ListSessionMessagesResponse {
  repeated ChatMessageRow items = 1;
  int32 total = 2;
  int64 current_revision = 3;
}
```

#### SessionRunRecord 消息（M55 生命周期）

```protobuf
message SessionRunRecord {
  string id = 1;
  string session_id = 2;
  string turn_id = 3;
  string runtime_run_id = 4;
  string source = 5;
  string phase = 6;
  int32 soft_budget_sec = 7;   // Deprecated
  int32 hard_budget_sec = 8;   // Deprecated
  string checkpoint_id = 9;
  string workflow_job_id = 10;
  string agent_id = 11;
  string error_message = 12;
  string started_at = 13;
  string phase_changed_at = 14;
  string finished_at = 15;
  string created_at = 16;
  string updated_at = 17;
}
```

### 2.2 RPC 与 HTTP 路由映射

| RPC | HTTP | 说明 |
|-----|------|------|
| `SearchSessions` | `GET /v1/sessions` | 搜索/列表，支持 owner_type/agent_id/team_id/status/context_status/keyword/user_id/sort_by/sort_order 筛选 + 分页 |
| `CreateSession` | `POST /v1/sessions` | 创建会话，校验 agent_id 或 team_id 存在性 |
| `GetSession` | `GET /v1/sessions/{id}` | 获取单个会话详情 |
| `UpdateSession` | `PATCH /v1/sessions/{id}` | 部分更新会话（title/tags_json/visibility/metadata_json/dialog_mode/default_provider/default_model） |
| `DeleteSession` | `DELETE /v1/sessions/{id}` | 软删除（设置 deleted_at + status=deleted） |
| `DeleteSessionsByAgent` | `DELETE /v1/sessions` | 按 agent_id 批量软删除 |
| `ArchiveSession` | `POST /v1/sessions/{id}/archive` | 归档会话（status=archived, archived_at） |
| `RestoreSession` | `POST /v1/sessions/{id}/restore` | 恢复归档会话（status=idle, 清空 archived_at/deleted_at） |
| `PinSession` | `POST /v1/sessions/{id}/pin` | 置顶会话（设置 pinned_at） |
| `UnpinSession` | `POST /v1/sessions/{id}/unpin` | 取消置顶（清空 pinned_at） |
| `ExportSession` | `GET /v1/sessions/{id}/export` | 导出会话（Markdown/JSON） |
| `GetSessionTimeline` | `GET /v1/sessions/{id}/timeline` | 聚合消息+工具+Skill+MCP 的时间轴，支持 limit/offset/kind_filter/sort_order |
| `ListSessionMessages` | `GET /v1/sessions/{id}/messages` | 获取会话消息列表，支持 limit/offset/after_revision |
| `SearchSessionMessages` | `GET /v1/sessions/messages/search` | 消息全文搜索（FTS5 优先，LIKE 回退） |
| `ListSessionTurns` | `GET /v1/sessions/{session_id}/turns` | 获取会话轮次列表，支持 limit/offset 分页 |
| `ListSessionRuns` | `GET /v1/sessions/{session_id}/runs` | 获取会话 Run 列表（M55 生命周期） |
| `ListSessionParticipants` | `GET /v1/sessions/{session_id}/participants` | 获取会话参与者列表 |
| `CompactSession` | `POST /v1/sessions:compact` | 手动触发上下文压缩 |
| `GetCompressStatus` | `GET /v1/sessions/{session_id}/compress-status` | 查询压缩状态 |
| `ListChildSessions` | `GET /v1/sessions/{parent_session_id}/children` | 查询子会话列表（单层，session tree Tab 用） |
| `GetSessionTree` | `GET /v1/sessions/{spirit_session_id}/tree` | 获取完整递归会话树（一次查询 + 内存构树，详见 §3.6.5） |
| `ListActivities` | `GET /v1/sessions/{session_id}/activities` | 查询活动列表（Activity-First 单一真相源，详见 §3.6.7） |

### 2.3 批量操作 RPC

> 需求来源：会话历史列表批量治理。契约以 `api/kratos/session/v1/session.proto` 为准。

#### 2.3.1 Proto

```protobuf
message SessionBatchScope {
  string owner_type = 1;
  string agent_id = 2;
  string team_id = 3;
  string status = 4;
  string context_status = 5;
  string keyword = 6;
  string user_id = 7;
}

// ids 非空：仅在这些 ID 上应用规则；older_than_days 进一步按 activity 过滤。
// ids 为空：在 scope 匹配集上分页扫描（服务端）；须 older_than_days >= 1。
message BatchPreviewSessionsRequest {
  repeated string ids = 1;
  int32 older_than_days = 2;
  SessionBatchScope scope = 3;
  bool include_archived = 4;
  string mode = 5;  // "archive" | "delete"（REQUIRED）
}

message BatchPreviewSessionsResponse {
  int32 matched = 1;
  int32 skipped_running = 2;
  repeated string sample_ids = 3;  // 最多 5 条
  int32 skipped_not_found = 4;     // ids 模式中不存在的 ID 数
  bool truncated = 5;              // scope 扫描达 SessionBatchMaxScan 上限
}

message BatchArchiveSessionsRequest {
  repeated string ids = 1;
  int32 older_than_days = 2;
  SessionBatchScope scope = 3;
}

message BatchDeleteSessionsRequest {
  repeated string ids = 1;
  int32 older_than_days = 2;
  SessionBatchScope scope = 3;
  bool include_archived = 4;
}

message BatchSessionsResponse {
  int32 matched = 1;
  int32 processed = 2;
  int32 skipped_running = 3;
  repeated string failed_ids = 4;
  int32 skipped_not_found = 5;
  bool truncated = 6;
}
```

> **语义**：`matched` 为 biz 解析命中数；`processed` 为 SQL 实际更新行数。二者差额（扣除 `failed_ids`）表示执行时因状态变化（如变为 running）被 SQL WHERE 跳过，前端 notify 展示为「执行时跳过」。

#### 2.3.2 HTTP 路由

| RPC | HTTP | 说明 |
|-----|------|------|
| `BatchPreviewSessions` | `POST /v1/sessions:batchPreview` | dry-run：matched / skipped_running / skipped_not_found / truncated |
| `BatchArchiveSessions` | `POST /v1/sessions:batchArchive` | 批量归档（ids 或 older_than_days + scope） |
| `BatchDeleteSessions` | `POST /v1/sessions:batchDelete` | 批量软删除 |

#### 2.3.3 Cutoff 与扫描（Biz 统一）

实现：`internal/biz/session/batch.go`

| 常量 | 值 | 说明 |
|------|-----|------|
| `SessionBatchPageSize` | 1000 | scope 分页每页大小 |
| `SessionBatchMaxScan` | 100000 | scope 模式最大扫描行数；超出则 `truncated=true` |

```go
// effectiveActivityAt：last_message_at → updated_at → created_at
// cutoff = now.UTC().AddDate(0, 0, -olderThanDays)
// 命中：effectiveActivityAt(s) < cutoff && deleted_at=="" && status != "running"
// 归档额外：status != "archived"
// 删除额外：include_archived 或 status != "archived"（默认不含已归档）

// resolveBatchOperation：Preview / BatchArchive / BatchDelete 共用（单次 load + resolve）
// loadBatchCandidatesByScope：按 SessionBatchPageSize 循环直到扫完或达 SessionBatchMaxScan
```

#### 2.3.4 Repo 扩展

```go
// internal/biz — SessionReader
ListSessionsForBatch(ctx context.Context, q SessionSearchQuery) ([]Session, error)

// internal/biz — SessionBatchMutator
ArchiveSessionsByIDs(ctx context.Context, ids []string) (processed int, failed []string, err error)
DeleteSessionsByIDs(ctx context.Context, ids []string) (processed int, failed []string, err error)
```

实现要点（`internal/data/session_repo_batch.go`）：

- 查询与 `SearchSessions` 共用 `sessionSearchWheres()` predicate
- **`ListSessionsForBatch` 固定 `ORDER BY id ASC`**，保证 offset 分页稳定
- 批量 `UPDATE` 每批 ≤500；WHERE 含 `deleted_at==""`、`status!='running'`；归档额外 `status!='archived'`
- `failed_ids`：仅 chunk 级 DB 错误；部分行因 WHERE 未更新不计入 failed_ids（由 matched−processed 体现）

#### 2.3.5 Service / Audit

`SessionService.BatchArchiveSessions` / `BatchDeleteSessions`：

1. HTTP 校验：`older_than_days >= 1` 或 `len(ids) > 0`；preview 另校验 `mode` 非空
2. 委托 `SessionUsecase.BatchArchive` / `BatchDelete` / `PreviewBatch`
3. 错误经 `mapSessionErr`；Audit：`archive.session.batch` / `delete.session.batch`（detail 含 matched/processed/skipped/truncated）

单条 `DeleteSession` / `ArchiveSession`：`running`/`awaiting_confirmation` 不可删/归档；已删幂等；列表行操作复用上述 RPC。

---

## 三、Biz 层

### 3.1 领域模型

文件：`internal/biz/session/usecase.go`

```go
type Session struct {
    ID                         string
    WorkspaceID                string
    UserID                     string
    OwnerType                  string   // "agent" | "team"
    AgentID                    string
    TeamID                     string
    Title                      string
    Summary                    string
    TagsJSON                   string
    DialogMode                 string
    DefaultProvider            string
    DefaultModel               string
    DefaultContextWindowTokens int
    LastProvider               string
    LastModel                  string
    LastContextWindowTokens    int
    Status                     string   // idle/running/completed/interrupted/awaiting_confirmation/archived/deleted
    StatusReason               string
    StatusChangedAt            string
    Visibility                 string   // private/team/workspace
    MessageCount               int
    RunCount                   int
    ModelCallCount             int
    ToolCallCount              int
    SkillCallCount             int
    MCPCallCount               int
    InputTokens                int
    OutputTokens               int
    TotalTokens                int
    TotalCostMicroUSD          int64
    AvgLatencyMs               float64
    ErrorCount                 int
    ContextUsedTokens          int
    ContextUsedRatio           float64
    MaxContextUsedRatio        float64
    ContextStatus              string   // normal/warning/critical/exceeded
    FirstMessageAt             string
    LastMessageAt              string
    LastRunAt                  string
    CreatedAt                  string
    UpdatedAt                  string
    ArchivedAt                 string
    DeletedAt                  string
    PinnedAt                   string
    RunnerSnapshotJSON         string
    StateJSON                  string
    MetadataJSON               string
    SessionRevision            int64
    CompressVersion            int64
    ParentSessionID            string
    RootSessionID              string
    AgentDepth                 int
}

type ChatMessage struct {
    ID               string
    SessionID        string
    ParentMessageID  string
    TurnID           string
    TurnNumber       int
    SeqInTurn        int
    Role             string   // user/assistant/system/tool
    ContentMarkdown  string
    ModelName        string
    TokenIn          int
    TokenOut         int
    LatencyMS        int
    Status           string
    AttachmentsCount int
    OptionsJSON      string
    ErrorMessage     string
    CreatedAt        string
}
```

> **⚠️ ChatMessage / `messages` 表已 DELETED（Activity-First 重构，详见 ADR-03）**：
>
> - 后端 `messages` 表已 DROP（DDL 迁移 `20260902`），`message_repo.go` 已删除；`message_usecase.go` 经评估保留为合法子用例（仅作 Activity 适配读取的 Facade，满足 AS-COG-01 复杂度预算）。
> - 上述 `ChatMessage` 结构仅为历史 proto/biz 类型对照，运行时**不再写入 `messages` 表**；所有用户消息、Agent 回复、工具调用、系统通知统一为 `Activity` 行（详见 §3.6 Session 父子树 + Activity 模型）。
> - **角色映射**（LLM API 仅接受 user/assistant/tool/system）：
>   - `task` Activity → `user` 角色
>   - `reply` Activity → `assistant` 角色（含团队成员回复，通过 `agent_key` 标识来源，不改变 role）
>   - `action` Activity → `tool` 角色
>   - `notice` Activity → `system` 角色
> - LLM 上下文构建改走 `BuildLLMContext`（`internal/biz/llm_context_builder.go`），从 `activities` 表查询 session/turn 内的 Activity 行并按上表映射为 LLM 消息；详见 Chat 模块重构方案 §3.5（已归档）。
> - `ListSessionMessages` / `SearchSessionMessages` RPC 当前仍向后兼容（基于 Activity 表实现），proto `ChatMessageRow` 仅作传输载体。

type SessionTurn struct {
    ID                  string
    SessionID           string
    RunID               string
    TurnNumber          int
    UserMessageID       string
    AssistantMessageID  string
    OwnerType           string
    AgentID             string
    TeamID              string
    Status              string
    StartedAt           string
    EndedAt             string
    DurationMs          int
    FirstTokenMs        int
    ModelCallCount      int
    ToolCallCount       int
    SkillCallCount      int
    MCPCallCount        int
    InputTokens         int
    OutputTokens        int
    TotalTokens         int
    TotalCostMicroUSD   int64
    FinalProvider       string
    FinalModel          string
    FinalContentPreview string
    ErrorCode           string
    ErrorMessage        string
    MetadataJSON        string
    CreatedAt           string
    UpdatedAt           string
}
```

### 3.2 Repository 接口

文件：`internal/biz/session/usecase.go`

> `SessionRepo` 聚合接口仅用于 Wire 绑定，消费者应依赖具体子接口。已标记 `Deprecated` + `TECH-DEBT(COG): interface_methods=17`。

```go
// Deprecated: Use fine-grained sub-interfaces instead.
// Stability:evolving
type SessionRepo interface {
    SessionReader
    SessionTreeReader
    SessionWriter
    SessionMutator
    SessionBatchMutator
    MessageReader
    MessageSearchReader
    MessageWriter
    MessageStatusWriter
    TimelineReader
    InvocationReader
    SummaryReader
    SummaryWriter
    StateRepo
    TurnRepo
    ContextUpdater
    CompressRepo
}

// Stability:stable — 会话读取（5 方法）
type SessionReader interface {
    SearchSessions(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
    GetSessionByID(ctx context.Context, id string) (Session, error)
    GetSessionRevision(ctx context.Context, sessionID string) (int64, error)
    ListSessionsForBatch(ctx context.Context, q SessionSearchQuery) ([]Session, error)
    ListSessionsByIDs(ctx context.Context, ids []string) ([]Session, error)
}

// Stability:stable — 会话树读取
type SessionTreeReader interface {
    ListByParentSessionID(ctx context.Context, parentSessionID string) ([]Session, error)
}

// Stability:stable — 会话写入（5 方法）
type SessionWriter interface {
    CreateSession(ctx context.Context, s Session) (Session, error)
    UpdateSessionTitle(ctx context.Context, id, title string) (Session, error)
    UpdateSession(ctx context.Context, id string, fields SessionUpdateFields) (Session, error)
    RestoreSession(ctx context.Context, id string) (Session, error)
    BumpSessionRevision(ctx context.Context, sessionID string) (int64, error)
}

// Stability:stable — 会话变更（5 方法：归档/删除/Agent批量删/置顶/取消置顶）
type SessionMutator interface {
    ArchiveSession(ctx context.Context, id string) (int, error)
    DeleteSession(ctx context.Context, id string) (int, error)
    DeleteSessionsByAgentID(ctx context.Context, agentID string) error
    PinSession(ctx context.Context, id string) (Session, error)
    UnpinSession(ctx context.Context, id string) (Session, error)
}

// Stability:stable — 批量变更（2 方法）
type SessionBatchMutator interface {
    ArchiveSessionsByIDs(ctx context.Context, ids []string) (processed int, failed []string, err error)
    DeleteSessionsByIDs(ctx context.Context, ids []string) (processed int, failed []string, err error)
}

// Stability:stable — 消息读取（5 方法）
type MessageReader interface {
    CountMessagesBySession(ctx context.Context, sessionID string) (int, error)
    ListMessagesBySession(ctx context.Context, sessionID string, limit, offset int) ([]ChatMessage, error)
    ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]ChatMessage, error)
    ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
    ListMessagesByIDs(ctx context.Context, sessionID string, ids []string) ([]ChatMessage, error)
}

// Stability:stable — 消息搜索 + 增量拉取（3 方法）
type MessageSearchReader interface {
    ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]ChatMessage, error)
    SearchMessages(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error)
    ListMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int64) ([]ChatMessage, error)
}

// Stability:stable — 消息写入（4 方法）
type MessageWriter interface {
    AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error
    AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
    UpdateMessageFeedbackJSON(ctx context.Context, sessionID, messageID, rating, comment string) error
    UpsertChatActivityMessage(ctx context.Context, sessionID string, msg ChatMessage) (bool, error)
}

// Stability:stable — Timeline 读取（4 方法）
type TimelineReader interface {
    ListTimelineEventRefsPaged(ctx context.Context, sessionID string, q TimelineQuery) ([]TimelineEventRef, int, error)
    ListToolInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]ToolInvocationView, error)
    ListSkillInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]SkillInvocationView, error)
    LookupAgentDisplayNames(ctx context.Context, agentIDs []string) (map[string]string, error)
}

// Stability:stable — 工具/Skill 调用读取（2 方法）
type InvocationReader interface {
    ListToolInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]ToolInvocationView, error)
    ListSkillInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]SkillInvocationView, error)
}

// Stability:stable — 摘要读取（3 方法）
type SummaryReader interface {
    MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error)
    ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error)
    LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error)
}

// Stability:stable — 摘要写入（4 方法）
type SummaryWriter interface {
    InsertSessionSummary(ctx context.Context, row SessionSummary) error
    DeleteSessionSummaries(ctx context.Context, sessionID string) error // 递归合并时清除被吸收的旧摘要
    UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error
    SessionSummaryExists(ctx context.Context, sessionID string, fromTurn, toTurn int) (bool, error)
}

// Stability:stable — KV 状态（3 方法）
type StateRepo interface {
    GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
    SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
    PatchSessionState(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error
}

// Stability:stable — Turn 读写（4 方法）
type TurnRepo interface {
    CreateSessionTurn(ctx context.Context, turn SessionTurn) (SessionTurn, error)
    UpdateSessionTurn(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error)
    ListSessionTurns(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error)
    GetSessionTurn(ctx context.Context, id string) (SessionTurn, error)
}

// Stability:evolving — 上下文更新（5 方法）
type ContextUpdater interface {
    UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error
    UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error
    UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error
    IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error
    ApplyMetricsDelta(ctx context.Context, d *SessionMetricsDelta) error
}

// Stability:stable — 压缩 CAS + 事务（2 方法）
type CompressRepo interface {
    TryIncrementCompressVersion(ctx context.Context, sessionID string) (oldVersion int64, err error)
    CompressSessionInTx(ctx context.Context, sessionID string, fn func(ctx context.Context) error) error
}
```

> **运行时拆分接口**：`SessionRuntimeWriter`（TransitionSessionStatus 等）与 `SessionMetricsReader`/`SessionMetricsWriter` 在 `internal/biz/session/metrics_repo.go` 等文件中定义，用于高频 metrics 字段与运行时快照字段的独立读写。

### 3.3 Usecase 结构

```go
// SessionUsecase handles session CRUD + timeline.
// TECH-DEBT(COG): struct_fields=14, limit=15 (AS-COG-01 biz layer); resolved via sub-usecase decomposition
type SessionUsecase struct {
    sessionReader       SessionReader
    sessionTreeReader   SessionTreeReader
    sessionWriter       SessionWriter
    sessionMutator      SessionMutator
    sessionBatchMutator SessionBatchMutator
    runtimeWriter       SessionRuntimeWriter
    agents              AgentLookup
    teams               TeamLookup
    lg                  loggateway.Logger
    statusPublisher     SessionStatusPublisher

    // Sub-usecases (Facade pattern — old callers delegate through these).
    metricsUsecase     *SessionMetricsUsecase
    compressionUsecase *SessionCompressionUsecase
    timelineUsecase    *SessionTimelineUsecase
    messageUsecase     *SessionMessageUsecase
}

func NewSessionUsecase(
    sessions SessionRepo,
    agents AgentLookup,
    teams TeamLookup,
    titleGenerator SessionTitleGenerator,
    participants SessionParticipantRepository,
    statusPublisher SessionStatusPublisher,
    metricsUsecase *SessionMetricsUsecase,
    runtimeWriter SessionRuntimeWriter,
    lg loggateway.Logger,
) *SessionUsecase
```

主要方法（详见 `internal/biz/session/usecase.go`）：

- `Search` / `Get` / `Create` / `Rename` / `Update` / `Restore` / `Archive` / `Delete` / `DeleteByAgent`
- `TransitionStatus` — 状态机转换（经 `SessionStatusMachine`）
- `ListChildSessions` / `GetRootSession` — 会话树
- `Timeline` / `SearchMessages` / `ListMessages` / `ListMessagesPaged`
- `AppendChatTurn` / `AppendChatMessage` / `UpdateRunnerSnapshotJSON`
- `UpdateSessionContextFromLLMUsage` / `UpdateSessionContextAfterCompression`
- `InsertSessionSummary` / `ListSessionSummaries` / `MaxSessionSummaryToTurn`
- `GetSessionState` / `SaveSessionState` / `ApplyStateDelta`
- `CreateTurn` / `UpdateTurn` / `ListTurns`
- `PreviewBatch` / `BatchArchive` / `BatchDelete`
- `IncrementInvocationCounts`

### 3.4 关键业务逻辑

**Create 校验**：
- `owner_type=agent` 时，`agent_id` 必填且 agent 必须存在
- `owner_type=team` 时，`team_id` 必填且 team 必须存在
- 自动生成 `uuid.NewString()` 作为 ID
- 默认 `status=idle`, `context_status=normal`

**Timeline 聚合**：
1. 查询 `messages` → `kind=message`, `side=left`
2. 查询 `tool_invocations` → `kind=tool` 或 `kind=mcp`（当 `source="mcp"` 或 `tool_key` 包含 "mcp" 时）, `side=right`
3. 查询 `skill_invocations` → `kind=skill`, `side=right`
4. 合并所有 items 按 `occurred_at` 排序（支持 `sort_order=desc` 倒序）
5. 支持 `kind_filter` 过滤
6. 支持 `limit/offset` 分页
7. 统计 summary 各类型计数

**消息 Timeline 映射**：
- `role=user` → `title="用户消息"`, `tags=["User"]`
- `role=system` → `title="系统消息"`, `tags=["System"]`
- `role=assistant` + `options_json.team_member.name` → `title=成员名`, `tags=["Team"]`
- `role=assistant` + `options_json.agent.display_name` → `title=Agent名`, `tags=["Agent"]`

**上下文状态计算**：
```go
func contextStatusForRatio(ratio float64) string {
    switch {
    case ratio >= 0.95: return "exceeded"
    case ratio >= 0.8:  return "critical"
    case ratio >= 0.6:  return "warning"
    default:            return "normal"
    }
}
```

**自动命名**：
- 当 session 标题为空、"untitled"、"新会话"、"未命名" 等占位符时，从首条用户消息截取前 56 字符自动命名
- 同时异步调用 `SessionTitleGenerator`（LLM）生成更精确的标题

**SessionTitleGenerator 接口**：
```go
type SessionTitleGenerator interface {
    Generate(ctx context.Context, content string) (string, error)
}
```
- `LLMSessionTitleGenerator`：使用轻量模型生成标题
- `NoopSessionTitleGenerator`：空实现，用于测试

**Session State KV**：
- `GetSessionState`/`SaveSessionState`：读写 `sessions.state_json`（JSON 序列化的 `map[string]string`）
- `PatchSessionState`：支持 `sets`/`deletes` 的增量更新
- `ApplyStateDelta`：支持 `set`/`append`/`delete` 操作的增量更新

**Session Turns**：
- `CreateTurn`：创建对话轮次，自动生成 UUID 和时间戳
- `UpdateTurn`：部分更新轮次（status/duration/token/counts 等）
- `ListTurns`：按 turn_number 升序分页查询

**NativeTurnCompressor 接口**：
```go
type NativeTurnCompressor interface {
    AfterNativeTurn(ctx context.Context, sessionID string, agent Agent)
}
```

### 3.5 Session 状态机

文件：`internal/biz/session/status_machine.go` + `status.go`

状态枚举：

```go
const (
    SessionStatusIdle                 SessionStatus = "idle"
    SessionStatusRunning              SessionStatus = "running"
    SessionStatusCompleted            SessionStatus = "completed"
    SessionStatusInterrupted          SessionStatus = "interrupted"
    SessionStatusAwaitingConfirmation SessionStatus = "awaiting_confirmation"
)
```

合法转换表：

| From | To |
|------|-----|
| `idle` | `running` |
| `running` | `completed` / `interrupted` / `awaiting_confirmation` |
| `completed` | `running` |
| `interrupted` | `running` |
| `awaiting_confirmation` | `running` / `interrupted` |

状态原因（`SessionStatusReason`）：

| 原因 | 说明 |
|------|------|
| `user_cancelled` | 用户取消 |
| `timeout` | 超时 |
| `user_escalated` | 用户升级 |
| `error` | 错误 |
| `context_overflow` | 上下文溢出 |
| `server_shutdown` | 服务器关闭 |
| `unexpected_shutdown` | 异常关闭 |
| `confirmation_timeout` | 确认超时 |
| `tool_confirmation` | 工具确认 |
| `agent_awaiting_reply` | Agent 等待回复 |
| `manual_override` | 手动覆盖 |

**受保护状态**（`IsProtectedStatus`）：`running` / `awaiting_confirmation` — 不可删除/归档。

> 注：`archived` / `deleted` 是通过 `ArchiveSession` / `DeleteSession` 单独设置的状态，不参与状态机转换。

---

## 3.6 Session 父子树 + Activity 模型（Activity-First 重构核心）

> **本节为 Activity-First 重构后的核心设计**，详见 ADR-02（活动事件持久化）与 ADR-03（统一总线架构）。

### 3.6.1 SessionType 枚举（父子树角色）

```go
// internal/biz/session/types.go（或 status.go 同包）
type SessionType string

const (
    // SessionTypeSpirit：根会话，用户与平台的入口（一个 Spirit 一次对话根）
    SessionTypeSpirit     SessionType = "spirit"
    // SessionTypeTeam：Team 编排产生的会话节点
    SessionTypeTeam       SessionType = "team"
    // SessionTypeAgent：单个 Agent 执行产生的会话节点（含 Team member 与 sub-agent）
    SessionTypeAgent      SessionType = "agent"
    // SessionTypeStandalone：独立会话（无父，默认）
    SessionTypeStandalone SessionType = "standalone"
)
```

> **DELETED**：原 `member` 类型已删除，统一并入 `agent`（Team member 与 sub-agent 共用 `agent` 类型，通过 `member_agent_key` + `member_role` 区分来源）。

### 3.6.2 父子树字段（Session 表新增）

参见 §4.2 Ent Schema — Session 主表（字段 50–59）：

| 字段 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `parent_session_id` | string(256) | `""` | 父会话 ID；spirit/standalone 为空 |
| `root_session_id` | string(256) | `""` | 根会话 ID（spirit ID）；自身为 spirit 时填自身 ID |
| `agent_depth` | int | `0` | Agent 层相对深度（spirit=0，team=1，team member=2，sub-agent 递增） |
| `session_type` | string(32) | `"standalone"` | 父子树角色枚举（见 §3.6.1） |
| `member_agent_key` | string | `""` | agent 类型 Session 标识执行 Agent 的 key |
| `member_role` | string | `""` | Agent 在 Team 中的角色（coordinator/worker/critic 等） |
| `execution_stage` | string | `""` | 当前阶段：`idle`/`planning`/`allocating`/`executing`/`completed`/`failed` |
| `completed_steps` | int | `0` | 已完成步骤数 |
| `total_steps` | int | `0` | 总步骤数 |
| `progress_pct` | float | `0.0` | 进度百分比（0–100） |

### 3.6.3 树构建规则

1. **Spirit 为根**：用户首次发起对话时创建 `session_type=spirit` 的根 Session，`root_session_id` 填自身 ID，`agent_depth=0`，`parent_session_id` 为空。
2. **Team 编派**：Team Run 启动时创建 `session_type=team` 的 Session，`parent_session_id=spirit_id`，`root_session_id=spirit_id`，`agent_depth=1`。
3. **Member 执行**：Team 成员执行时创建 `session_type=agent` 的 Session，`parent_session_id=team_session_id`，`root_session_id=spirit_id`，`agent_depth=2`，并填 `member_agent_key` + `member_role`。
4. **Sub-Agent 递归**：Agent 通过 AgentTool 调用 sub-agent 时创建 `session_type=agent` 的 Session，`parent_session_id=caller_agent_session_id`，`agent_depth=caller_depth+1`。
5. **Standalone**：未挂载到 Spirit 树的独立 Session（如 `RunTeamTest` 临时会话），`session_type=standalone`，`parent_session_id` 为空。

### 3.6.4 深度校验 `validateDepth`

```go
// internal/biz/session/tree_validate.go（伪代码）
func validateDepth(parent *Session, agentConfig AgentRuntimeSetting) error {
    // Agent 级别：相对深度（subagents_max_generation_depth 来自 Agent 配置）
    maxGen := agentConfig.SubagentsMaxGenerationDepth
    if parent.AgentDepth+1 > maxGen {
        return ErrMaxGenerationDepthExceeded
    }
    // Spirit 级别：绝对深度（max_session_depth 来自 Spirit 全局配置）
    spirit, ok := lookupSpirit(parent.RootSessionID)
    if ok && parent.AgentDepth+1 > spirit.MaxSessionDepth {
        return ErrMaxSessionDepthExceeded
    }
    return nil
}
```

**双层深度控制**：
- `subagents_max_generation_depth`（Agent 级别，相对）：单个 Agent 最多递归派生多少代 sub-agent。
- `max_session_depth`（Spirit 级别，绝对）：一棵 Spirit 树允许的最大 agent_depth，整树封顶。

### 3.6.5 GetSessionTree RPC 设计

```protobuf
message GetSessionTreeRequest {
  string root_session_id = 1;  // Spirit 根 ID
}
message SessionTreeNode {
  Session session = 1;
  repeated SessionTreeNode children = 2;
}
message GetSessionTreeResponse {
  SessionTreeNode root = 1;  // 完整树
}
```

**实现策略（一次查询 + 内存构树，任意深度）**：

```go
// 伪代码：避免 N+1 查询
func (u *SessionUsecase) GetSessionTree(ctx, rootSessionID) (*SessionTreeNode, error) {
    // 单次查询：按 root_session_id 取回整棵树的所有节点
    all, err := u.sessionRepo.ListByRootSessionID(ctx, rootSessionID)
    if err != nil { return nil, err }
    // 内存中按 parent_session_id 构建多叉树
    nodes := make(map[string]*SessionTreeNode, len(all))
    for _, s := range all {
        nodes[s.ID] = &SessionTreeNode{Session: s}
    }
    var root *SessionTreeNode
    for _, s := range all {
        node := nodes[s.ID]
        if s.ParentSessionID == "" {
            root = node  // Spirit 根
        } else if parent, ok := nodes[s.ParentSessionID]; ok {
            parent.Children = append(parent.Children, node)
        }
    }
    return root, nil
}
```

**辅助 RPC**：
- `ListChildSessions(parent_session_id)`：单层子节点列表（详情页 Tab 用）。
- `ListTeamAgentSessions(team_session_id)`：Team 下所有 agent member session。

### 3.6.6 SpiritSessionID 传播

**问题背景**：原实现中 SpiritSessionID 在 Team/Graph 多层嵌套下会丢失，导致跨会话聚合（如 `ListSpiritTeams`、`SynthesizeResults`）失效。

**解决方案**：通过 `ProjectMeta` 显式传播 Spirit 会话身份。

```go
// ProjectMeta 字段（trpc-agent-go 运行时元数据）
type ProjectMeta struct {
    SpiritSessionID string  // Spirit 根会话 ID（贯穿整树）
    ParentSessionID string  // 父会话 ID（用于挂载新 session）
    RootSessionID   string  // = SpiritSessionID（冗余便于查询）
    // ... 其他元数据
}
```

**传播规则**：
- Spirit 创建 Session 时：`SpiritSessionID = self.ID`，写入 `ProjectMeta`。
- Team Run 启动：从 Spirit 的 `ProjectMeta` 继承 `SpiritSessionID`，新建 team Session 时设 `parent_session_id = spirit_id`。
- Member/Sub-Agent 执行：继承上游 `ProjectMeta.SpiritSessionID`，新建 agent Session 时设 `parent_session_id = caller_session_id`。

**`buildTeamProjectMeta`**（跨会话聚合）：

```go
// internal/team/runner_team_trpc.go（伪代码）
func buildTeamProjectMeta(spiritSessionID, parentSessionID string) ProjectMeta {
    return ProjectMeta{
        SpiritSessionID: spiritSessionID,
        ParentSessionID: parentSessionID,
        RootSessionID:   spiritSessionID,
    }
}
```

Team Runner 启动时调用 `buildTeamProjectMeta` 填充下游 Agent 的 `ProjectMeta`，保证子 Agent 创建 Session 时 `root_session_id` / `parent_session_id` 正确。

### 3.6.7 Activity 模型（单一真相源）

> `messages` 表已 DELETED（详见 §4.5），所有用户消息、Agent 回复、工具调用、系统通知统一为 `Activity` 行。

**ActivityKind（10 种，无 error）**：

```go
type ActivityKind string
const (
    ActivityKindTask        ActivityKind = "task"        // 用户输入任务
    ActivityKindThinking    ActivityKind = "thinking"    // 思考/推理过程
    ActivityKindAction      ActivityKind = "action"      // 工具/Skill 调用
    ActivityKindReply       ActivityKind = "reply"       // Agent 回复（含 Team member 回复）
    ActivityKindPlan        ActivityKind = "plan"        // 计划
    ActivityKindConfirm     ActivityKind = "confirm"     // 需用户确认
    ActivityKindNotice      ActivityKind = "notice"      // 系统通知
    ActivityKindSession     ActivityKind = "session"     // 会话级事件（创建/压缩等）
    ActivityKindTeamStage   ActivityKind = "team_stage"  // Team 编排阶段事件
    ActivityKindGraphStage  ActivityKind = "graph_stage" // Graph 执行阶段事件
)
```

> **DELETED**：原 `error` / `SubTaskBoard` / `Delegate` 等 ActivityKind 已删除（详见 ADR-02 D3/D4）。失败语义改为 `ActivityEventType=failed` + `ActivityKind=task`。

**ActivityEventType（7 种）**：

```go
type ActivityEventType string
const (
    ActivityEventCreated       ActivityEventType = "created"
    ActivityEventStreaming     ActivityEventType = "streaming"
    ActivityEventUpdated       ActivityEventType = "updated"
    ActivityEventCompleted     ActivityEventType = "completed"
    ActivityEventFailed        ActivityEventType = "failed"
    ActivityEventCancelled     ActivityEventType = "cancelled"
    ActivityEventChildCreated  ActivityEventType = "child_created"
)
```

**ActivityEvent 结构**：

```go
type ActivityEvent struct {
    Event    ActivityEventType  // 7 种事件类型
    Activity Activity           // Activity 行（kind/content/agent_key/session_id 等）
    Domain   ActivityDomain     // chat（持久化）| system（仅 WS 推送）
}
```

**LLM 角色映射**（`BuildLLMContext`）：

| ActivityKind | LLM Role |
|--------------|----------|
| `task` | `user` |
| `reply` | `assistant`（含 Team member 回复，通过 `agent_key` 标识来源） |
| `action` | `tool` |
| `notice` | `system` |

### 3.6.8 双总线架构（ActivityEventBus + MonitorEventBus）

> 详见 ADR-03。**已删除**：`SessionBus`、基于 Envelope 的 `MonitorBus`、`event_projector.go`、`activity_publish.go`、`activity_persist.go`。

| 总线 | 事件类型 | 持久化 | 用途 |
|------|---------|--------|------|
| `ActivityEventBus` | `biz.ActivityEvent` | chat 域持久化到 `activities` 表；system 域仅 WS | Chat 域业务事件（task/reply/action 等） |
| `MonitorEventBus` | `contract.MonitorEvent` | 不持久化 | 系统监控事件（FlowLog/TokenUsage 等） |

**ActivityEventBus 持久化策略**（ADR-02 D1）：
- 并行异步：`persistChan` fire-and-forget + 同步发布到总线
- 三级补偿：重试预算（5 次/3100ms）→ 死信环形缓冲（512 容量，FIFO 驱逐，activityID 去重）→ API Backfill（最终一致兜底）
- OnError 语义：根 Task 转 `failed`；无根场景创建最小失败 Task

### 3.6.9 替换的旧组件

| 旧组件 | 状态 | 新组件 |
|--------|------|--------|
| `messages` 表 | DELETED | `activities` 表 |
| `message_repo.go` / `session_message_repo.go` | DELETED | `activity_repo.go` |
| `message_usecase.go` | 保留为合法子用例（Facade） | — |
| `event_projector.go` | DELETED | `activity_projector.go`（ActivityProjector） |
| `activity_publish.go` / `activity_persist.go` | DELETED | `ActivityEventBus` 内置持久化 |
| `StatusProjector` | 替换 | `ActivityProjector` |
| `SessionBus` + Envelope `MonitorBus` | DELETED | `ActivityEventBus` + `MonitorEventBus` |
| `event_store` 表 | DELETED | `activities` 表 + 死信缓冲 |

---

## 四、数据层

### 4.1 数据模型总览

Session 数据分为四层：

| 层级 | 表 | 说明 |
|------|----|------|
| 会话主表 | `sessions` | 一条 session 的归属、标题、状态、时间、上下文消耗摘要、父子树字段（§3.6） |
| 运行时拆分表 | `session_runtime` | 高频运行时字段拆分（session_revision/state_json/runner_snapshot_json/compress_version） |
| 指标拆分表 | `session_metrics` | 高频更新 metrics 字段拆分（message_count/token/cost/context_*） |
| **活动层（内容真相源）** | **`activities`** | **Activity-First 单一真相源**：用户消息、Agent 回复、工具调用、系统通知、Team/Graph 阶段事件统一为 Activity 行（详见 §3.6.7） |
| 编排层 | `session_runs`、`session_run_checkpoints`、`session_participants` | Run 生命周期、检查点、参与者 |
| 摘要层 | `session_summaries` | 滚动摘要（原生 SQL 表） |

> **拆分表策略**：`session_runtime` 与 `session_metrics` 将高频更新字段从 `sessions` 主表拆出，减少写放大。`sessions` 主表保留查询列表所需字段，拆分表通过 `session_id` 一对一关联。
>
> **⚠️ `messages` 表已 DELETED**（DDL 迁移 `20260902`，详见 §4.5）：内容层改由 `activities` 表承载，`session_turns` / `chat_attachments` 保留。`event_store` 表亦已 DELETED，事件持久化改由 `ActivityEventBus` 内置并行异步机制 + 死信缓冲 + API Backfill 完成（详见 ADR-02）。

### 4.2 Ent Schema — Session 主表

文件：`internal/data/ent/schema/session.go`

```go
type Session struct {
    ent.Schema
}

func (Session) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "sessions"},
    }
}

func (Session) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("agent_id", "deleted_at", "updated_at").StorageKey("idx_sessions_agent"),
        index.Fields("team_id", "deleted_at", "updated_at").StorageKey("idx_sessions_team"),
        index.Fields("last_message_at").StorageKey("idx_sessions_last_message"),
        index.Fields("deleted_at", "user_id"),
        index.Fields("deleted_at", "status"),
        index.Fields("parent_session_id").StorageKey("idx_sessions_parent"),
        index.Fields("root_session_id").StorageKey("idx_sessions_root"),
    }
}

func (Session) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("workspace_id").Default(""),
        field.String("user_id").Default(""),
        field.String("owner_type").Default("agent"),
        field.String("agent_id").Default(""),
        field.String("team_id").Default(""),
        field.String("title"),
        field.Text("summary").Default(""),
        field.Text("tags_json").Default("[]"),
        field.String("dialog_mode").Default(""),
        field.String("default_provider").Default(""),
        field.String("default_model").Default(""),
        field.Int("default_context_window_tokens").Default(0),
        field.String("last_provider").Default(""),
        field.String("last_model").Default(""),
        field.Int("last_context_window_tokens").Default(0),
        field.String("status").Default("idle"),
        field.String("status_reason").Default(""),
        field.String("status_changed_at").Default(""),
        field.String("visibility").Default("private"),
        field.Int("message_count").Default(0),
        field.Int("run_count").Default(0),
        field.Int("model_call_count").Default(0),
        field.Int("tool_call_count").Default(0),
        field.Int("skill_call_count").Default(0),
        field.Int("mcp_call_count").Default(0),
        field.Int("input_tokens").Default(0),
        field.Int("output_tokens").Default(0),
        field.Int("total_tokens").Default(0),
        field.Int64("total_cost_micro_usd").Default(0),
        field.Float("avg_latency_ms").Default(0.0),
        field.Int("error_count").Default(0),
        field.Int("context_used_tokens").Default(0),
        field.Float("context_used_ratio").Default(0.0),
        field.Float("max_context_used_ratio").Default(0.0),
        field.String("context_status").Default("normal"),
        field.String("first_message_at").Default(""),
        field.String("last_message_at").Default(""),
        field.String("last_run_at").Default(""),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("archived_at").Default(""),
        field.String("deleted_at").Default(""),
        field.String("pinned_at").Default(""),
        field.Text("runner_snapshot_json").Default(""),
        field.Text("state_json").Default("{}"),
        field.Text("metadata_json").Default("{}"),
        field.Int64("session_revision").Default(0),
        field.Int64("compress_version").Default(0),
        field.String("parent_session_id").Default("").MaxLen(256),
        field.String("root_session_id").Default("").MaxLen(256),
        field.Int("agent_depth").Default(0),
        // —— Session 父子树字段（Phase D 补全，详见 §3.6） ——
        // session_type: spirit (root) | team | agent (member 或 sub-agent) | standalone
        field.String("session_type").MaxLen(32).Default("standalone").Comment("spirit/team/agent/standalone"),
        // member_agent_key: agent 类型 Session 标识执行 Agent 的 key
        field.String("member_agent_key").Default("").Comment("Agent key for agent-type sessions"),
        // member_role: Agent 在 Team 中的角色（coordinator/worker 等）
        field.String("member_role").Default("").Comment("Agent role within team"),
        // execution_stage: idle/planning/allocating/executing/completed/failed
        field.String("execution_stage").Default("").Comment("Current stage"),
        field.Int("completed_steps").Default(0),
        field.Int("total_steps").Default(0),
        field.Float("progress_pct").Default(0.0),
    }
}
```

### 4.3 Ent Schema — SessionRuntime（运行时拆分表）

文件：`internal/data/ent/schema/session_runtime.go`

```go
type SessionRuntime struct {
    ent.Schema
}

func (SessionRuntime) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "session_runtime"},
    }
}

func (SessionRuntime) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").StorageKey("session_id").Unique().Immutable().MaxLen(256),
        field.Int("session_revision").Default(0),
        field.Text("state_json").Default("{}"),
        field.Text("runner_snapshot_json").Default(""),
        field.Text("metadata_json").Default("{}"),
        field.Int("compress_version").Default(0),
        field.String("updated_at").Default(""),
    }
}
```

### 4.4 Ent Schema — SessionMetrics（指标拆分表）

文件：`internal/data/ent/schema/session_metrics.go`

```go
type SessionMetrics struct {
    ent.Schema
}

func (SessionMetrics) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "session_metrics"},
    }
}

func (SessionMetrics) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").StorageKey("session_id").Unique().Immutable().MaxLen(256),
        field.Int("message_count").Default(0),
        field.Int("run_count").Default(0),
        field.Int("model_call_count").Default(0),
        field.Int("tool_call_count").Default(0),
        field.Int("skill_call_count").Default(0),
        field.Int("mcp_call_count").Default(0),
        field.Int("input_tokens").Default(0),
        field.Int("output_tokens").Default(0),
        field.Int("total_tokens").Default(0),
        field.Int64("total_cost_micro_usd").Default(0),
        field.Float("avg_latency_ms").Default(0.0),
        field.Int("error_count").Default(0),
        field.Int("context_used_tokens").Default(0),
        field.Float("context_used_ratio").Default(0.0),
        field.Float("max_context_used_ratio").Default(0.0),
        field.String("context_status").Default(""),
        field.String("last_message_at").Default(""),
        field.String("updated_at").Default(""),
    }
}
```

### 4.5 Ent Schema — Message（DEPRECATED · 已 DELETED）

> **⚠️ 本节 Schema 已 DELETED（Activity-First 重构，详见 ADR-03）**：
>
> - 后端 `messages` 表已 DROP（DDL 迁移 `20260902`），`internal/data/ent/schema/message.go` 与 `internal/data/session_message_repo.go` / `internal/data/message_repo.go` 均已删除。
> - 所有用户消息、Agent 回复、工具调用、系统通知统一为 **`Activity` 行**，单一真相源位于 `activities` 表（Schema 定义见 §4.13，运行时模型见 §3.6.7 与 Chat 模块重构方案 §3.4（已归档））。
> - `ListSessionMessages` / `SearchSessionMessages` RPC 当前仍向后兼容（基于 Activity 表实现），proto `ChatMessageRow` 仅作传输载体。
> - 下表保留仅作历史对照，**禁止再据此 Schema 创建/迁移 `messages` 表**。
>
> **历史 Schema（仅供回溯）**：
>
> ```go
> // 已删除：internal/data/ent/schema/message.go
> type Message struct { ent.Schema }
> // 索引：session_id / (session_id, turn_id) / (session_id, turn_number) / (session_id, status)
> // 字段：id, session_id, parent_message_id, turn_id, turn_number, seq_in_turn,
> //       role, content_markdown, model_name, token_in, token_out, latency_ms,
> //       status, attachments_count, options_json, error_message, created_at
> ```
>
> **替代方案**：所有写入/查询走 `Activity` 模型（`internal/biz/activity.go` + `internal/data/activity_repo.go`），LLM 上下文构建走 `BuildLLMContext`（`internal/biz/llm_context_builder.go`）。

### 4.6 Ent Schema — SessionTurn

文件：`internal/data/ent/schema/session_turn.go`

```go
type SessionTurn struct {
    ent.Schema
}

func (SessionTurn) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "session_turns"},
    }
}

func (SessionTurn) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("session_id").MaxLen(256),
        field.String("run_id").Default(""),
        field.Int("turn_number").Default(0),
        field.String("user_message_id").Default(""),
        field.String("assistant_message_id").Default(""),
        field.String("owner_type").Default("agent"),
        field.String("agent_id").Default(""),
        field.String("team_id").Default(""),
        field.String("status").Default("running"),
        field.String("started_at").Default(""),
        field.String("ended_at").Default(""),
        field.Int("duration_ms").Default(0),
        field.Int("first_token_ms").Default(0),
        field.Int("model_call_count").Default(0),
        field.Int("tool_call_count").Default(0),
        field.Int("skill_call_count").Default(0),
        field.Int("mcp_call_count").Default(0),
        field.Int("input_tokens").Default(0),
        field.Int("output_tokens").Default(0),
        field.Int("total_tokens").Default(0),
        field.Int64("total_cost_micro_usd").Default(0),
        field.String("final_provider").Default(""),
        field.String("final_model").Default(""),
        field.Text("final_content_preview").Default(""),
        field.String("error_code").Default(""),
        field.Text("error_message").Default(""),
        field.Text("metadata_json").Default("{}"),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}

func (SessionTurn) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("session_id", "turn_number"),
        index.Fields("status", "started_at"),
        index.Fields("run_id").StorageKey("idx_session_turns_run_id"),
        index.Fields("agent_id").StorageKey("idx_session_turns_agent_id"),
        index.Fields("team_id").StorageKey("idx_session_turns_team_id"),
    }
}
```

### 4.7 Ent Schema — SessionRun（M55 生命周期）

文件：`internal/data/ent/schema/session_run.go`

> 注：当前 `session_runs` 表为 M55 Run 生命周期模型（phase/budget/checkpoint），与早期设计文档中的编排 runs（run_type/trigger_type/plan_json）字段不同。扩展前需 schema 决策。

```go
type SessionRun struct {
    ent.Schema
}

func (SessionRun) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "session_runs"},
    }
}

func (SessionRun) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("session_id"),
        index.Fields("phase", "finished_at"),
    }
}

func (SessionRun) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("session_id").MaxLen(256),
        field.String("turn_id").Default(""),
        field.String("runtime_run_id").Default(""),
        field.String("source").Default(""),
        field.String("phase").Default("interactive"),
        field.Int("soft_budget_sec").Default(0),  // Deprecated: budget mechanism removed
        field.Int("hard_budget_sec").Default(0),  // Deprecated: budget mechanism removed
        field.String("checkpoint_id").Default(""),
        field.String("workflow_job_id").Default(""),
        field.String("agent_id").Default(""),
        field.Text("error_message").Default(""),
        field.String("started_at").Default(""),
        field.String("phase_changed_at").Default(""),
        field.String("finished_at").Default(""),
        field.String("resume_started_at").Default(""),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}
```

### 4.8 Ent Schema — SessionRunCheckpoint

文件：`internal/data/ent/schema/session_run_checkpoint.go`

```go
type SessionRunCheckpoint struct {
    ent.Schema
}

func (SessionRunCheckpoint) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "session_run_checkpoints"},
    }
}

func (SessionRunCheckpoint) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("session_run_id"),
        index.Fields("session_id"),
    }
}

func (SessionRunCheckpoint) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("session_run_id").Default(""),
        field.String("session_id").Default(""),
        field.String("turn_id").Default(""),
        field.String("agent_id").Default(""),
        field.Text("payload_json").Default(""),
        field.String("created_at").Default(""),
    }
}
```

### 4.9 Ent Schema — SessionParticipant

文件：`internal/data/ent/schema/session_participant.go`

```go
type SessionParticipant struct {
    ent.Schema
}

func (SessionParticipant) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "session_participants"},
    }
}

func (SessionParticipant) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("session_id"),
        index.Fields("participant_id").StorageKey("idx_session_participants_participant"),
    }
}

func (SessionParticipant) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("session_id").MaxLen(256),
        field.String("participant_type").Default(""),
        field.String("participant_id").Default(""),
        field.String("display_name").Default(""),
        field.String("role_in_session").Default(""),
        field.String("status").Default("active"),
        field.String("first_active_at").Default(""),
        field.String("last_active_at").Default(""),
        field.Int("message_count").Default(0),
        field.Int("run_step_count").Default(0),
        field.Int("input_tokens").Default(0),
        field.Int("output_tokens").Default(0),
        field.Float("context_used_ratio").Default(0.0),
        field.Text("metadata_json").Default("{}"),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}
```

### 4.10 session_summaries 表（原生 SQL）

通过 `data.EnsureSessionMemorySchema`（`internal/data/sql/memory_chain.sql`）创建，不使用 Ent Schema：

```sql
CREATE TABLE IF NOT EXISTS session_summaries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    summary_markdown TEXT NOT NULL,
    from_turn INTEGER NOT NULL,
    to_turn INTEGER NOT NULL,
    token_estimate INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_summaries_session
  ON session_summaries(session_id, created_at);
```

### 4.11 关键 Data 层实现

**SearchSessions** — Ent ORM 查询（`internal/data/session_repo.go`）：

```go
func (r *sessionRepo) SearchSessions(ctx context.Context, q biz.SessionSearchQuery) (biz.SessionListResult, error) {
    c := r.data.entClient
    limit := clampSessionLimit(q.Limit)
    offset := clampOffset(q.Offset)

    wheres := []predicate.Session{entsession.DeletedAtEQ("")}
    if q.OwnerType != "" {
        wheres = append(wheres, entsession.OwnerTypeEQ(q.OwnerType))
    }
    // ... 其他筛选条件

    wherePred := entsession.And(wheres...)
    total, err := c.Session.Query().Where(wherePred).Count(ctx)
    // ... 分页查询 + 转换
}
```

**AppendChatTurn** — 事务写入用户+助手消息对（`internal/data/session_message_repo.go`）：

```go
func (r *sessionRepo) AppendChatTurn(ctx context.Context, sessionID string, user, assistant biz.ChatMessage) error {
    tx, err := r.data.entClient.Tx(ctx)
    // ... 事务内写入 user + assistant 消息，更新 session 聚合字段
}
```

**UpdateSessionContextFromLLMUsage** — 上下文消耗更新（`internal/data/session_repo.go`）：

```go
func (r *sessionRepo) UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, _ int, contextWindow int) error {
    cur, err := r.GetSessionByID(ctx, sessionID)
    // ... 计算 ratio = promptTokens / contextWindow，更新 context_used_ratio/max/context_status
}
```

**Session Summaries CRUD** — 原生 SQL（`internal/data/session_repo_summaries.go`）：

```go
func (r *sessionRepo) InsertSessionSummary(ctx context.Context, row biz.SessionSummary) error {
    q := `INSERT INTO session_summaries (id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at)
          VALUES (?,?,?,?,?,?,?)`
    // ...
}
```

### 4.12 类型转换

```go
func entSessionToBiz(e *ent.Session) biz.Session {
    if e == nil { return biz.Session{} }
    return biz.Session{
        ID: e.ID, WorkspaceID: e.WorkspaceID, UserID: e.UserID,
        OwnerType: e.OwnerType, AgentID: e.AgentID, TeamID: e.TeamID,
        // ... 全字段映射
        PinnedAt: e.PinnedAt,
        SessionRevision: e.SessionRevision, CompressVersion: e.CompressVersion,
        // —— 父子树字段（Phase D 补全，详见 §3.6） ——
        ParentSessionID: e.ParentSessionID, RootSessionID: e.RootSessionID,
        AgentDepth: e.AgentDepth, SessionType: e.SessionType,
        MemberAgentKey: e.MemberAgentKey, MemberRole: e.MemberRole,
        ExecutionStage: e.ExecutionStage, CompletedSteps: e.CompletedSteps,
        TotalSteps: e.TotalSteps, ProgressPct: e.ProgressPct,
    }
}
```

### 4.13 Ent Schema — Activity（Activity-First 单一真相源）

文件：`internal/data/ent/schema/activity.go`

> Activity-First 架构核心表，承载原 `messages` 表删除后的全部内容（用户消息/Agent 回复/工具调用/系统通知/Team/Graph 阶段事件）。运行时模型见 §3.6.7。

```go
type Activity struct {
    ent.Schema
}

func (Activity) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "activities"},
    }
}

func (Activity) Fields() []ent.Field {
    return []ent.Field{
        // === 主键 ===
        field.String("id").MaxLen(64).Unique().Immutable(),
        // === 分类 ===
        field.String("kind").MaxLen(32).Comment("ActivityKind: task/thinking/action/reply/notice/confirm/plan/session/team_stage/graph_stage"),
        field.String("status").MaxLen(32).Default("pending").Comment("ActivityStatus"),
        // === 归属 ===
        field.String("session_id").MaxLen(128).Default(""),
        field.String("turn_id").MaxLen(128).Default(""),
        field.String("parent_activity_id").MaxLen(64).Default("").Comment("父 Activity，树形嵌套"),
        // === 时序 ===
        field.String("timestamp").Default(""),
        field.Int64("duration_ms").Default(0),
        field.Int64("seq").Default(0).Comment("全局发射序号，前端稳定排序"),
        // === Token 用量（仅 kind=task 根 Activity） ===
        field.Int64("prompt_tokens").Default(0),
        field.Int64("completion_tokens").Default(0),
        // === 内容字段（按 kind） ===
        field.Text("content").Default("").Comment("task/reply/notice 文本"),
        field.Text("reasoning").Default("").Comment("thinking 推理内容"),
        // === 工具字段（kind=action） ===
        field.String("tool_name").MaxLen(128).Default(""),
        field.String("tool_category").MaxLen(32).Default(""),
        field.String("tool_call_id").MaxLen(128).Default(""),
        field.Text("tool_arguments").Default("").Sensitive(),
        field.Text("tool_result").Default("").Sensitive(),
        field.Int64("tool_duration_ms").Default(0),
        field.String("tool_error_code").MaxLen(64).Default(""),
        // === 阶段（kind=session/team_stage/graph_stage） ===
        field.String("stage").MaxLen(64).Default(""),
        // === Sub-task board（kind=sub_task_board，遗留） ===
        field.String("child_board_id").MaxLen(64).Default(""),
        // === Spirit 扩展 ===
        field.String("spirit_session_id").MaxLen(128).Default("").Comment("Spirit Session ID"),
        field.String("team_id").MaxLen(128).Default(""),
        field.String("dag_node_id").MaxLen(128).Default(""),
        field.JSON("depends_on", []string{}).Optional(),
        // === Agent 信息 ===
        field.String("agent_key").MaxLen(128).Default(""),
        field.String("agent_name").MaxLen(128).Default(""),
        // === 展示提示 ===
        field.Bool("collapsed").Default(false),
        field.String("label").MaxLen(128).Default("").Comment("自定义标签如 规划/推理/重规划"),
        // === Kind 特定元数据 ===
        field.JSON("meta", map[string]any{}).Optional(),
    }
}

func (Activity) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("session_id", "turn_id").StorageKey("idx_activities_session_turn"),
        index.Fields("parent_activity_id").StorageKey("idx_activities_parent"),
        index.Fields("spirit_session_id").StorageKey("idx_activities_spirit_session"),
        index.Fields("team_id").StorageKey("idx_activities_team"),
    }
}
```

> **持久化机制**：由 `ActivityEventBus` 内置并行异步机制写入（`persistChan` fire-and-forget + 同步发布），三级补偿（重试预算 → 死信环形缓冲 512 容量 FIFO 驱逐 activityID 去重 → API Backfill）。详见 ADR-02 D1。

---

## 五、Service 层

### 5.1 SessionService

文件：`internal/service/session.go`

```go
type SessionService struct {
    v1.UnimplementedSessionServiceServer
    uc *biz.SessionUsecase
}

func NewSessionService(uc *biz.SessionUsecase) *SessionService {
    return &SessionService{uc: uc}
}
```

### 5.2 类型转换函数

```go
func toProtoSession(s biz.Session) *v1.Session {
    return &v1.Session{
        Id: s.ID, WorkspaceId: s.WorkspaceID, UserId: s.UserID,
        OwnerType: s.OwnerType, AgentId: s.AgentID, TeamId: s.TeamID,
        // ... 全字段映射
        PinnedAt: s.PinnedAt,
        ParentSessionId: s.ParentSessionID, RootSessionId: s.RootSessionID,
        AgentDepth: int32(s.AgentDepth),
    }
}

func toProtoTimeline(t biz.SessionTimeline) *v1.SessionTimeline { /* ... */ }
func toProtoTimelineItem(it biz.SessionTimelineItem) *v1.SessionTimelineItem { /* ... */ }
func toProtoChatMessageRow(m biz.ChatMessage) *v1.ChatMessageRow { /* ... */ }
```

### 5.3 RPC 实现

```go
func (s *SessionService) SearchSessions(ctx context.Context, req *v1.SearchSessionsRequest) (*v1.SearchSessionsResponse, error) { /* ... */ }
func (s *SessionService) CreateSession(ctx context.Context, req *v1.CreateSessionRequest) (*v1.Session, error) { /* ... */ }
func (s *SessionService) GetSession(ctx context.Context, req *v1.GetSessionRequest) (*v1.Session, error) { /* ... */ }
func (s *SessionService) UpdateSession(ctx context.Context, req *v1.UpdateSessionRequest) (*v1.Session, error) { /* ... */ }
func (s *SessionService) DeleteSession(ctx context.Context, req *v1.DeleteSessionRequest) (*emptypb.Empty, error) { /* ... */ }
func (s *SessionService) ArchiveSession(ctx context.Context, req *v1.ArchiveSessionRequest) (*emptypb.Empty, error) { /* ... */ }
func (s *SessionService) RestoreSession(ctx context.Context, req *v1.RestoreSessionRequest) (*v1.Session, error) { /* ... */ }
func (s *SessionService) PinSession(ctx context.Context, req *v1.PinSessionRequest) (*v1.Session, error) { /* ... */ }
func (s *SessionService) UnpinSession(ctx context.Context, req *v1.UnpinSessionRequest) (*v1.Session, error) { /* ... */ }
func (s *SessionService) GetSessionTimeline(ctx context.Context, req *v1.GetSessionTimelineRequest) (*v1.SessionTimeline, error) { /* ... */ }
func (s *SessionService) ListSessionMessages(ctx context.Context, req *v1.ListSessionMessagesRequest) (*v1.ListSessionMessagesResponse, error) { /* ... */ }
```

### 5.4 SessionCompressionUsecase

文件：`internal/biz/session/compression.go`

```go
type SessionCompressionUsecase struct {
    compressRepo   CompressRepo
    contextUpdater ContextUpdater
    summaryReader  SummaryReader
    summaryWriter  SummaryWriter
}

func NewSessionCompressionUsecase(
    compressRepo CompressRepo,
    contextUpdater ContextUpdater,
    summaryReader SummaryReader,
    summaryWriter SummaryWriter,
) *SessionCompressionUsecase
```

> **O8 变更**：`SessionCompressor`（service 层）的 `CompressorDeps` 已从 `biz.AgentRepository`（54 方法）收窄为 `AgentKeyLookup` 窄接口；压缩核心逻辑提取到 `SessionCompressionUsecase`（biz 层），依赖 4 个窄接口。

**压缩流程**：
1. 检查 `context_used_ratio` 是否超过 agent 阈值（默认 0.6）
2. 100% 满窗时立即压缩，否则检查距上次压缩间隔 ≥ 10 分钟（`L0CompressMinGapSec` 可配置）
3. 获取消息列表，计算需要压缩的 turn 范围（保留最近 `keepTurns` 轮）
4. 调用 `compress.Compressor.Compress()` 生成摘要
5. 写入 `session_summaries` 表
6. 合并所有摘要，重写 `runner_snapshot_json`（摘要事件 + 尾部消息事件）
7. 更新 `context_used_ratio` 为压缩后估算值（`llmcontext.ResolveWindow` 解析分母；`llmcontext.ContextRatio` / `ContextStatusForRatio` 与前端 `contextMetrics.ts` 共用阈值 0.6/0.8/0.95）
8. WS 推送 `text_done`（`metadata.kind=system.session.compress`，含 `context_used_ratio` / `context_used_tokens` / `context_status`），前端乐观 patch；DB 更新失败写 session 系统日志
9. 更新 `sessions.summary` 为首行摘要
10. 清理 SessionMemory 中的旧事件实体

**压缩模型选择**：
- 优先使用 `agent.settings.L0CompressProvider` + `L0CompressModel`
- 回退到 `session.DefaultProvider` + `DefaultModel`
- 最终回退到 `agent.Provider` + `agent.Model`

---

## 六、上下文压缩设计

### 6.1 问题与目标

**问题**：单会话消息与 Runner 事件随轮次增长，反复把完整历史送入模型导致 prompt token 线性增长、成本与延迟上升，更易触碰上下文上限。

**目标**：

1. 当历史达到一定规模，将一段可追溯的对话区间交给「压缩模型」梳理为结构化摘要，要求不遗漏对后续决策关键的事实。
2. 后续用户继续对话时，模型侧上下文改为：**压缩摘要 + 最近若干轮原文 + 本轮输入**。
3. **账本不变**：`messages`（及必要的 trace）仍保留完整原文；压缩改变的是**送入模型的装配结果**，默认不物理删除历史消息。

**非目标**：不替代 L3/L4 长期记忆检索；不把「压缩」作为唯一降耗手段；首版不要求用户编辑摘要正文。

### 6.2 核心概念

| 术语 | 含义 |
|------|------|
| 滚动摘要（Rolling Summary） | 覆盖区间 `[from_turn, to_turn]` 的 Markdown 文本，存于 `session_summaries` |
| 当前有效摘要（Active Summary） | 某 session 在装配时刻用于头部的摘要集合 |
| 滑动窗口（Tail） | 摘要区间之后、尚未被摘要覆盖的最近 K 轮或最近 T tokens 的原始对话 |
| 压缩模型（Compressor Model） | 执行摘要生成的 LLM 调用；可与对话模型同厂商或降级为更小规格 |

### 6.3 触发条件

建议可组合配置（Agent / Team / Session 级覆盖），默认启用「soft + hard」双层：

| 策略 | 条件 | 说明 |
|------|------|------|
| 比例触发 | `context_used_ratio ≥ summary_threshold` | 与现有 `sessions.context_*` 字段对齐 |
| 轮次触发 | 自上次摘要以来新增 `Δturn ≥ compress_every_n_turns` | 防窗口很大但比例尚未告警时长对话不摘要 |
| Token 估算触发 | 未摘要前缀估算 token ≥ `compress_prefix_token_budget` | 与滑动窗口预算联动 |
| 手动触发 | UI「生成会话摘要」或 `CompactSession` API | 便于调试与关键节点强制固化 |

**防抖**：同一 session 短时间窗口内（如 5～10 分钟，`L0CompressMinGapSec` 可配置）最多触发 N 次摘要任务。

**并发**：若上一轮摘要尚未完成，后续触发应合并区间或排队，避免交错写入两条重叠 `from_turn/to_turn`。

### 6.4 压缩任务的输入与输出

**输入（发给压缩模型）**：对选定区间，序列化可追溯的对话——角色与时间序、工具调用（工具名、参数摘要、结果摘要或截断后的关键字段）、锚点（session_id、区间、上轮摘要）。

**输出（摘要 schema）**——推荐固定章节：

1. 用户意图与目标
2. 已确认事实 / 结论（含数字、版本、路径、API 名等硬信息）
3. 约束与偏好（语言、风格、禁止项）
4. 未完成事项 / 待澄清问题
5. 重要工具结果摘录（表格或列表）
6. 术语与别名

输出写入 `session_summaries.summary_markdown`，并填写 `from_turn`、`to_turn`、`token_estimate`、`created_at`。可同时更新 `sessions.summary` 为列表页/会话卡片用的一句话摘要。

**提示词原则**：明确后续对话仅能看到本摘要 + 最近几轮，要求在无损前提下最大化密度；不得编造；保留可执行细节。

### 6.5 后续轮次装配顺序

在单次模型调用前，L0 装配器按段拼装：

1. 系统 / 开发者固定段（SOUL、策略等）
2. **`session_summaries` 合并摘要**（标记 `source: session_summaries:<id>`）
3. L1 工作记忆字段（若有）
4. **滑动窗口内原始 messages**（摘要区间之后）
5. L3/L4 检索段（若有）
6. 本轮 user 输入

被摘要覆盖的旧消息不再重复进入 prompt，但仍可从 DB 读取用于 UI 与合规。

### 6.6 多条 session_summaries 合并策略

- **A. 区间链式（已废弃）**：~~保留多条记录，装配时按 `from_turn` 排序拼接~~。已被 B 替代（2026-07-20，Grok 借鉴 Phase 2）。
- **B. 单条滚动（已实现 ✅）**：LLM 压缩时传入 `PriorSummary` 吸收合并历史摘要，产出一条覆盖 `[earliest_from, current_to]` 的新摘要；事务内 `DeleteSessionSummaries` 删除旧摘要行、写入单行合并行。防止摘要无限拼接增长。
  - **吸收条件**：仅当 LLM 真实产出摘要（`llmSucceeded`）才置 `absorbedPriors=true`；hybrid 策略 LLM 失败落兜底标记（`[Earlier turns trimmed per hybrid policy]`）时不吸收、不删旧行（标记不含历史内容，删除会丢数据）。

### 6.6.1 摘要质量门（已实现 ✅）

移植自 Grok `code_compaction/config.rs`，三道防线均为零副作用纯函数（`internal/session/compress_quality.go`）：

| 防线 | 机制 | 参数 |
|------|------|------|
| 退化检测 | 摘要 rune 数 < 200 且原文 ≥ 1000 runes → 判退化，重试 | `minSummarySeedChars=200`, `minTranscriptCharsForGuard=1000` |
| 减量守卫 | 摘要 est tokens ≥ 原文 80% → 丢弃结果 | `maxSummaryReductionRatio=0.8` |
| 错误分类 | 确定性（上下文溢出/鉴权/参数错误）→ 不重试；瞬态 → 重试 ≤2 次 | `llmCompressMaxAttempts=2` |

### 6.6.2 压缩失败抑制（已实现 ✅）

移植自 Grok `auto_compact_suppressed`（`internal/session/compress_suppress.go`）：

| 失败类型 | 抑制策略 | 解除条件 |
|----------|----------|----------|
| 确定性（deterministic） | sticky 抑制（不受 minGap 影响） | 压缩模型切换自动解除 |
| 瞬态（transient） | minGap 退避 | 超过 minGap 后放行 |

手动 `/compact` 与 durable turn（`forced=true`）绕过抑制。抑制为进程内内存态，重启后重新尝试一次再抑制。

### 6.6.3 双锚点 token 估算校准（已实现 ✅）

压缩 LLM 成功返回后，用权威 `prompt_tokens` 校准共享估算器（`internal/llmcontext/token_estimator.go`）：

- **默认比率**：2.5 chars/token（CJK/英文混合）
- **校准路径**：`compress/service.go` → `llmcontext.RecordAuthoritativeUsage(ptok, chars)`
- **注意**：共享估算器为进程级单例，多模型混用时比率漂移（与 Grok 同语义，接受近似）

### 6.7 与 Runner 会话持久态的关系

| 方案 | 做法 | 优点 | 风险 |
|------|------|------|------|
| **装配层优先（首版推荐）** | 不改变 `runner_snapshot_json` 内全量事件；仅在构造发往 LLM 的 messages 时应用摘要 + tail | 实现集中、可逆、与现有 messages 账本一致 | Runner 内部若独立推算上下文，需确认走同一装配入口 |
| 快照裁剪（可选） | 在摘要固化后，对 snapshot 中早于 `to_turn` 的模型事件做归档或删除 | 持久态更小 | 回放 Runner 历史不完整，需额外归档存储 |

### 6.8 一致性、失败与重试

- **幂等键**：`(session_id, from_turn, to_turn, prompt_hash)`；重复任务返回已有记录。
- **失败**：摘要失败时不阻断用户发消息；回退为「仅滑动窗口截断」并打 `context_status`/告警日志。
- **观测**：写入 `memory_l0_assembly_snapshots` 中的 `summarized_turn_from/to`、`summary_token_estimate`、`segments_json` 段来源。

### 6.9 Team 会话

- **隔离**：每个子 Agent 应有独立的摘要区间与 `session_summaries` 维度，避免 Host 与子 Agent 上下文串扰。
- **Host 摘要**：可仅摘要「路由级」对话；专家会话单独滚动摘要。

### 6.10 API / 配置 / UX

| 层次 | 建议 |
|------|------|
| 配置 | 扩展 `agent_runtime_settings`：`summary_threshold`、`compress_every_n_turns`、`recent_window_turns`、`recent_window_tokens`、`compressor_model_profile`；另设 `l0_compress_provider` / `l0_compress_model`（可选），指定专用压缩调用 |
| API | `CompactSession` RPC 手动触发；`GetCompressStatus` 查询状态 |
| UI | 会话详情展示「已摘要至第 N 轮」标签；可选展示摘要正文（只读）；手动触发按钮 |

### 6.11 Context Window 计算

核心公式：

```text
context_used_ratio = prompt_tokens / context_window_tokens
```

如果 provider 返回精确 token，使用返回值；否则使用本地估算。由于一个 session 可以切换多个模型，`context_window_tokens` 必须按「本次调用的实际模型」计算，而不是只看 session 主表。优先级：

1. 本次调用模型配置中的 `context_window_k * 1000`
2. 本次消息 options 指定模型的 context window
3. session 创建时保存的 `default_context_window_tokens`
4. agent 配置中的 `context_window`
5. provider preset 的默认值（128000）

**实现**：`llmcontext.ResolveWindow`（`internal/llmcontext/window.go`）在每次 native turn 与 `runner_completion` 投影时解析分母；`context_used_tokens` **仅**由 `UpdateSessionContextFromLLMUsage` 写入本次 LLM 的 `prompt_tokens`（ReAct 多步取 **turn 内最大 prompt**），消息落库时不再累加。

**WS 契约**：`context_usage` 在 ReAct 多步 LLM 每次 prompt 峰值上升时推送（仅更新 context 条，不累加 session total）；`runner_completion.usage` 携带 `context_prompt_tokens`、`max_tokens`、`turn_total_tokens`。

**状态阈值单一来源**：Go `internal/llmcontext/metrics.go` 与前端 `web/src/features/session/contextMetrics.ts` 保持同步（0.6 / 0.8 / 0.95）。

**失败可观测**：`UpdateSessionContextFromLLMUsage` / 压缩后 `UpdateSessionContextAfterCompression` 失败时写入 session 系统日志（`context.usage` / `system.session.compress`），不再静默 `_ =` 丢弃。

**100% 后重新计数（Cursor 式）**：

1. **同 turn 内**：tRPC ContextCompaction（Agent 开启 `context_compaction_enabled`）在 LLM 调用前压缩历史，API 返回的 `prompt_tokens` 已是压缩后值。
2. **turn 后异步**：L0 `SessionCompressor` 生成摘要并重写 snapshot，调用 `UpdateSessionContextAfterCompression` 将 ratio 重置为压缩后估算值。
3. **实时 UI**：压缩完成 WS 推送 `text_done`（`metadata.kind=system.session.compress`，携带 `context_used_ratio` / `context_used_tokens` / `context_status`）；前端 `sessionContextPatch` 立即 patch store，无需等待 HTTP 刷新。

状态阈值：

| 状态 | 条件 | UI |
|------|------|----|
| `normal` | `< 60%` | 绿色 |
| `warning` | `60% - 80%` | 橙色 |
| `critical` | `80% - 95%` | 红色 |
| `exceeded` | `>= 95%` 或模型报 context length exceeded | 紫/红 + 建议新建 session 或摘要 |

Team session 的 context 有两个口径：

| 口径 | 说明 |
|------|------|
| Team 总消耗 | 本 session 下所有模型调用的最大 `used_ratio` 或最近 run 的聚合值 |
| Agent 局部消耗 | 每个 participant / step 自己的 context ratio |

前端列表展示 Team 总消耗；详情页展示每个 Agent 的局部消耗。

### 6.12 聚合更新策略

写入消息、usage、step 后，统一调用聚合函数更新 `sessions` / `session_metrics` 表。高频流式场景不要每个 delta 更新 session，只在以下时机更新：

| 时机 | 是否更新 session 聚合 | 前端 context % |
|------|----------------------|----------------|
| 用户消息落库 | 创建 turn 和 `user_message` span，更新 `message_count`、`last_message_at` | 不变 |
| assistant 最终消息落库 | 写入 `ai_response` span，更新消息、时间、最终内容预览 | 不变 |
| 工具/Skill/MCP 调用完成 | 更新 span 状态、耗时、输入输出和错误；必要时增量更新 turn 统计 | 不变 |
| 模型 usage 完成（turn 结束） | 更新 token、费用、context（DB） | **`runner_completion.usage` 乐观 patch**（`web/src/features/chat/sessionContextPatch.ts`） |
| L0 压缩完成 | 更新 `context_used_*`（DB） | **`text_done` compress notice 乐观 patch** |
| run 结束 | 更新 run_count、状态、耗时 | HTTP `loadAgentSessions` 与 WS patch 合并校正 |

---

## 七、trpc-agent-go 对齐路径

> 实现状态与任务 ID 详见 [10-session.development.md §8 待优化清单](./10-session.development.md#8-待优化清单全部)

| 阶段 | 内容 |
|------|------|
| M5-1 | Ent + SQLite Session CRUD + Timeline |
| M5-2 | 上下文追踪 + 摘要压缩 |
| M5-3 | Runner Snapshot 持久化 |
| M5-3a | Session Turns 对话轮次 |
| M5-3b | Session State KV + ApplyStateDelta |
| M5-3c | RestoreSession / UpdateSession 部分更新 |
| M5-3d | 自动标题生成（LLM + 截取双策略） |
| M5-4 | Session 置顶功能（pinned_at + PinSession RPC） |
| M5-5 | Session 导出功能（Markdown/JSON） |
| M5-6 | 消息搜索功能（全文检索） |
| M5-7 | session_runs / session_run_steps 编排记录 |
| M5-8 | session_participants Team 参与者 |
| M5-9 | session_trace_spans 完整追踪链路 |
| M5-10 | session_context_snapshots Context 趋势 |
| M5-11 | session_model_summaries 多模型分布 |
| M5-12 | 桥接 trpc `session.Service` 接口 |
| M5-13 | 多后端支持（Redis/PG） |
| M5-14 | 内置压缩迁移到 trpc 框架 |

### 7.1 trpc session.Service 集成

**trpc 框架**：`session.Service` 提供统一的 Session 存储接口，支持多后端。

**设计**：
- 新建 `internal/session/trpc/service.go`，桥接 Ent session 到 trpc `session.Service` 接口
- 先实现 SQLite 后端（项目已有）
- 后续增加 Redis 后端用于生产环境
- 最终支持 PostgreSQL/MySQL/ClickHouse 后端

**涉及文件**：`internal/session/trpc/service.go`、`internal/session/trpc/sqlite.go`、`internal/session/trpc/redis.go`

### 7.2 Event 分页

**trpc 框架**：`session.Service.ListEvents` 支持分页查询 Session 事件。

**设计**：
- 实现 `ListEvents(sessionID, pageSize, pageToken)` 方法
- 支持按时间正序/倒序
- 支持按事件类型过滤

### 7.3 Session Track

**trpc 框架**：`session.Service` 支持 Track 操作，记录 Session 级别的元数据。

**设计**：
- 实现 `Track(sessionID, key, value)` 方法
- Track 数据存储在 `sessions.metadata_json` 中
- 前端可查询 Track 数据

### 7.4 Session Ingestor

**trpc 框架**：`session.Ingestor` 接口，Session 完成后自动摄入到外部平台。

**设计**：
- 新建 `internal/session/trpc/ingestor.go`
- 实现 `session.Ingestor` 接口
- 可对接 Mem0 等外部记忆平台
- Runner 完成后自动调用 Ingestor

### 7.5 多后端支持

**trpc 框架**：`session/sqlite`、`session/redis`、`session/pg`、`session/mysql`、`session/clickhouse` 多后端。

**设计**：
- 配置文件增加 `session.backend` 字段
- 可选值：`sqlite`（默认）、`redis`、`postgresql`、`mysql`、`clickhouse`
- 按配置动态选择后端
- 提供迁移工具

### 7.6 Runner Snapshot 集成

**trpc 框架**：Runner 执行状态序列化为 `runner_snapshot_json`，用于 Runner 恢复。

**设计**：
- Runner 每轮执行后更新 `sessions.runner_snapshot_json`
- Runner 恢复时从 `runner_snapshot_json` 加载状态
- 摘要压缩后同步更新 snapshot

**涉及文件**：`internal/agent/trpc_runtime.go`、`internal/service/chat_native.go`

### 7.7 运行时层桥接

```go
// internal/agent/adksvc/session.go
type BizSessionService struct {
    store *sessionmemory.Store
}

func (s *BizSessionService) GetSession(ctx context.Context, id string) (*session.Session, error)
func (s *BizSessionService) SaveSession(ctx context.Context, sess *session.Session) error
```

桥接路径：
1. `internal/agent/adksvc` — 将 Ent session 映射到 trpc `session.Service` 接口
2. 后续可扩展 Redis/PG/MySQL 后端

### 7.8 Runner Snapshot 结构

`runner_snapshot_json` 存储 trpc-agent-go Runner 会话状态：

```json
{
  "events": [
    {
      "author": "user",
      "timestamp": "2026-04-26T09:00:00Z",
      "content": "[Conversation summary — earlier turns compressed]\n\n摘要内容...",
      "role": "system"
    },
    {
      "author": "agent",
      "timestamp": "2026-04-26T09:01:00Z",
      "content": "最新回复内容",
      "role": "assistant"
    }
  ],
  "updated_at": "2026-04-26T09:01:00Z"
}
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/session/
├── api.ts                          ← Kratos API 封装 + 类型定义
├── types.ts                        ← 类型定义（BatchPreviewResult, BulkProgress 等）
├── useSessionsPage.ts              ← 会话列表页 composable
├── useSessionDetailPage.ts         ← 会话详情页 composable
├── useSessionTimelinePanel.ts      ← Timeline 面板 composable（服务端分页）
├── useSessionTimelineInspector.ts  ← Timeline 检查器 composable
├── useSessionTurnsPanel.ts         ← Turn 面板 composable
├── useSessionRunsPanel.ts          ← Run 面板 composable
├── useSessionParticipantsPanel.ts  ← 参与者面板 composable
├── useSessionMessagesPanel.ts      ← 消息面板 composable
├── timelineHelpers.ts              ← Timeline 辅助函数
├── sessionSort.ts                  ← 排序逻辑
├── downloadExport.ts               ← 导出下载
├── contextMetrics.ts               ← 上下文指标（阈值与 Go llmcontext/metrics.go 同步）
├── batchNotify.ts                  ← 批量操作通知
web/src/components/chat/
├── ChatSessionSidebar.vue          ← Chat 页右侧 Session 列表
├── SessionTimelineDialog.vue       ← 历史追踪弹窗（服务端分页）
├── SessionEventInspectorPanel.vue  ← 事件检查器面板
web/src/components/sessions/
├── sessionUi.ts                    ← 工具函数（格式化、颜色、列定义）
├── SessionsPageHero.vue            ← 页面标题
├── SessionsSummaryCards.vue        ← 摘要卡片
├── SessionsFilterBar.vue           ← 筛选栏
├── SessionsErrorBanner.vue         ← 错误提示
├── SessionsSelectedDetail.vue      ← 选中详情
├── SessionsTableSection.vue        ← 表格+分页
├── SessionsBulkToolbar.vue         ← 批量选择 toggle、按天数按钮
├── SessionsBulkSelectionBar.vue    ← 已选 N + 归档/删除
├── SessionsBulkProgressBar.vue     ← 批量进度条
├── SessionDeleteConfirmDialog.vue  ← 单条/批量删除确认
├── SessionRetentionDialog.vue      ← 保留天数 + preview + 归档/删除确认
├── SessionStatusBadge.vue          ← 状态徽章
├── SessionRunsPanel.vue            ← Runs 面板
├── SessionTurnsPanel.vue           ← Turns 面板
├── SessionParticipantsPanel.vue    ← 参与者面板
├── SessionMessagesPanel.vue        ← 消息面板
├── SessionTimelinePanel.vue        ← Timeline 面板
├── SessionTimelineEntry.vue        ← Timeline 条目
├── SessionTimelineStats.vue        ← Timeline 统计
├── ContextIndicator.vue            ← 上下文指示器
web/src/pages/
├── SessionsPage.vue                ← Session 管理页面
```

### 8.2 API 层

文件：`web/src/features/session/api.ts`

```typescript
export type Session = {
  id: string;
  owner_type: string;
  agent_id: string;
  team_id: string;
  title: string;
  summary: string;
  context_used_ratio: number;
  max_context_used_ratio: number;
  context_status: string;
  dialog_mode: string;
  default_provider: string;
  default_model: string;
  last_provider: string;
  last_model: string;
  status: string;
  pinned_at: string;
  parent_session_id: string;
  root_session_id: string;
  message_count: number;
  run_count: number;
  model_call_count: number;
  tool_call_count: number;
  skill_call_count: number;
  mcp_call_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost_micro_usd: number;
  last_message_at: string;
  created_at: string;
  updated_at: string;
  archived_at: string;
  deleted_at: string;
  context_used_tokens?: number;
  last_context_window_tokens?: number;
};

export type SessionSearchQuery = {
  owner_type?: string;
  agent_id?: string;
  team_id?: string;
  status?: string;
  context_status?: string;
  keyword?: string;
  limit?: number;
  offset?: number;
  page?: number;
  page_size?: number;
  sort_by?: string;
  sort_order?: string;
};

export async function listSessions(agentID: string): Promise<Session[]>
export async function listTeamSessions(teamID: string): Promise<Session[]>
export async function searchSessions(query?: SessionSearchQuery): Promise<SessionListResult>
export async function getSession(id: string): Promise<Session>
export async function getSessionTimeline(id: string): Promise<SessionTimeline>
export async function createSession(payload: CreateSessionPayload): Promise<Session>
export async function deleteSession(id: string): Promise<void>
export async function archiveSession(id: string): Promise<void>
export async function restoreSession(id: string): Promise<Session>
export async function pinSession(id: string): Promise<Session>
export async function unpinSession(id: string): Promise<Session>
export async function exportSession(id: string, format: "markdown" | "json"): Promise<ExportResult>
export async function updateSessionTitle(id: string, title: string): Promise<Session>
export async function clearAgentSessions(agentID: string): Promise<void>
export async function listSessionChatMessages(sessionID: string): Promise<Message[]>
export async function batchPreview(req: BatchPreviewRequest): Promise<BatchPreviewResult>
export async function batchArchive(req: BatchArchiveRequest): Promise<BatchResult>
export async function batchDelete(req: BatchDeleteRequest): Promise<BatchResult>
```

### 8.3 ChatSessionSidebar 组件

文件：`web/src/components/chat/ChatSessionSidebar.vue`

Props：
- `open: boolean` — 侧边栏是否展开
- `sessions: SessionView[]` — 会话列表
- `selectedSessionId?: string | null` — 当前选中
- `isDark: boolean` — 暗色模式

Events：
- `select(id: string)` — 选中会话
- `new-session()` — 新建会话
- `rename({ id, title })` — 重命名
- `delete(kind: DeleteKind, id: string)` — 删除
- `trace(id: string)` — 打开历史追踪

布局：
```
┌─────────────────────────┐
│ Session          [count]│  ← 标题 + 数量徽章
├─────────────────────────┤
│ ▸ 置顶会话              │  ← pinned 分组
│   [圆环] 会话标题  时间  │
│          [★] [···]      │
│ ▸ 今天                  │  ← 时间分组
│   [圆环] 会话标题  时间  │
│ ▸ 昨天                  │
│ ▸ 近7天                 │
│ ▸ 近30天                │
│ ▸ 更早                  │
├─────────────────────────┤
│ [新建会话] [清空历史]    │  ← 操作按钮
└─────────────────────────┘
```

每个会话项：
- 左侧：`QCircularProgress` 显示 `context_used_ratio * 100` 百分比
- 中间：标题（2行截断）+ 时间徽章 + 收藏星标
- 右侧：更多菜单（重命名/置顶/收藏/历史追踪/删除）

时间分组逻辑：
- 置顶 → 今天 → 昨天 → 近7天 → 近30天 → 更早

Pin/Favorite 存储：`localStorage` 键 `chat:pinned-sessions` / `chat:favorite-sessions`

### 8.4 SessionTimelineDialog 组件

文件：`web/src/components/chat/SessionTimelineDialog.vue`

> Timeline 弹窗已实现服务端分页（`useSessionTimelinePanel`，PAGE_SIZE=100，offset 翻页），与详情页 Timeline Panel 对齐。

Props：
- `modelValue: boolean` — 弹窗开关
- `sessionId?: string | null` — 会话 ID
- `sessionTitle?: string` — 会话标题

布局：
```
┌──────────────────────────────────────────────────┐
│ Session Trace                               [×]  │
│ 会话标题                                         │
│ N events · conversation + tools + skills + MCP   │
├──────────────────────────────────────────────────┤
│ [Messages: 4] [Tools: 2] [Skills: 1] [MCP: 1]  │  ← 统计卡片
├──────────────────────────────────────────────────┤
│ ● 用户消息          [User]                        │  ← 左侧消息
│   12:30:05                                       │
│   ▸ 消息内容预览...                               │
│                                                   │
│                    ● 读取文件 [Tool]    │  ← 右侧工具
│                      12:30:06 · 耗时 34ms         │
│                      ▸ {"path":"README.md"}       │
│                                                   │
│ ● Agent 消息       [Agent]                       │
│   12:30:08                                       │
│   ▸ 回复内容预览...                               │
└──────────────────────────────────────────────────┘
```

标签颜色映射：
| Tag | 颜色 |
|-----|------|
| User | grey-7 |
| Agent | primary |
| Team | orange |
| Tool | info |
| Skill | teal |
| MCP | teal |

图标映射：
| kind | icon |
|------|------|
| tool | build |
| skill | auto_awesome |
| mcp | hub |
| User tag | person |
| 默认 | smart_toy |

每个条目使用 `QExpansionItem`，展开后显示：
- Actor 名称 + Source
- `content_markdown` 或 `detail_json`（格式化 JSON）

### 8.5 SessionsPage 管理页面

文件：`web/src/pages/SessionsPage.vue`（编排） + `features/session/useSessionsPage.ts`（状态/composable）

布局：
```
┌──────────────────────────────────────────────────────────────────┐
│ Session 管理                                    [刷新]           │
├──────────────────────────────────────────────────────────────────┤
│ [KPI 卡片 ×4]                                                    │
├──────────────────────────────────────────────────────────────────┤
│ 关键词[___] 类型[▼] 状态[▼] 上下文[▼] [重置][搜索]               │
│ [批量选择] [按天数归档] [按天数删除]                              │  ← SessionsBulkToolbar
├──────────────────────────────────────────────────────────────────┤
│ ████████████░░░░  批量归档中 12/40                               │  ← 仅 bulk 进行时显示
├──────────────────────────────────────────────────────────────────┤
│ [选中详情卡片]                                                    │
├──────────────────────────────────────────────────────────────────┤
│ ☐ │ 会话 │ 类型 │ 上下文 │ 消耗 │ 时间 │ 状态 │ 操作            │
│ ☐ │ ...  │      │        │      │      │      │ 👁 📦 🗑       │
├──────────────────────────────────────────────────────────────────┤
│ 共 N 个 Session                         [20/页 ▼] < 1 2 3 >      │
└──────────────────────────────────────────────────────────────────┘
```

**批量选择模式激活时**，筛选栏下方追加：

```
已选 3 项    [归档] [删除] [取消选择]
```

| 操作 | 确认 | 进度 / 反馈 |
|------|------|-------------|
| 行内删除 | `QDialog` 永久删除 | notify |
| 批量删除（勾选） | `QDialog` | `QLinearProgress` + notify |
| 批量归档（勾选） | **无** | `QLinearProgress` + notify「归档成功」 |
| 按天数归档 | `SessionRetentionDialog` 预览+确认 | 进度条 + notify |
| 按天数删除 | `SessionRetentionDialog` 预览+确认 | 进度条 + notify |

#### 8.5.1 分层纪律

| 层 | 职责 |
|----|------|
| `SessionsPage.vue` | 组合子组件、绑定 composable |
| `useSessionsPage.ts` | selection、bulk 分块、进度、调 `features/session/api` |
| `components/sessions/*` | 纯展示 + `emit`；**不** import api/store |
| `features/session/api.ts` | HTTP 封装 |

#### 8.5.2 批量进度实现

勾选模式与按天数模式共用 `runBulkOperation`：

```typescript
async function runBulkOperation(
  ids: string[],
  op: "archive" | "delete",
  onProgress: (done: number, total: number) => void
): Promise<{ processed: number; failed: string[] }>
```

- 优先调用 `batchArchive` / `batchDelete`（单次 RPC，服务端批处理）
- 若后端未就绪，fallback：每批 10 个 id 串行 `archiveSession` / `deleteSession`
- `onProgress` 驱动 `QLinearProgress`（`:value="done/total"`）
- 完成后 `$q.notify({ type: 'positive', message: '已归档 N 个会话' })`

#### 8.5.3 SessionRetentionDialog

| 字段 | 组件 |
|------|------|
| 保留天数 | `QInput` type=number，min=1，default=30 |
| 预览 | 调用 `batchPreview` 后展示 matched / skipped_running |
| 模式 archive | 文案：「将归档 cutoff 之前的 **X** 个会话，**保留最近 N 天**」 |
| 模式 delete | 文案 + `QCheckbox`「包含已归档会话」 |
| 按钮 | 取消 / 确认归档 / 确认删除（destructive color） |

表格列定义：
| 列 | 字段 | 内容 |
|----|------|------|
| 会话 | title | 标题 + summary/id |
| 类型/归属 | owner_type | Agent/Team chip + ID |
| 上下文 | context_used_ratio | QLinearProgress + 百分比 + context_status |
| 消耗 | total_tokens | Token 数 + model/tool/skill/mcp 调用数 |
| 时间 | last_message_at | 最后活跃 + 创建时间 |
| 状态 | status | QBadge |
| 操作 | id | 查看详情 + 归档 + **删除** |
| 选择 | — | 批量模式首列 QCheckbox（仅 selectionMode=true） |

### 8.6 工具函数

文件：`web/src/components/sessions/sessionUi.ts`

```typescript
export function ownerLabel(value: string): string        // "team" → "Team", else → "Agent"
export function ownerChipColor(value: string): string    // "team" → "teal", else → "primary"
export function statusBadgeColor(value: string): string  // failed→negative, archived→grey, running→primary, else→positive
export function contextProgressColor(value: string): string  // exceeded→purple, critical→negative, warning→warning, else→positive
export function ratioValue(value: number): number        // clamp(0, 1)
export function formatPercent(value: number): string     // "42%"
export function formatNumber(value: number): string      // 千分位
export function formatCostMicroUsd(value: number): string // "$0.0012"
export function formatSessionDate(value: string): string  // 本地时间格式
export function buildSessionsSummaryCards(rows: Session[], total: number): SessionsSummaryCard[]
```

### 8.7 上下文进度颜色阈值

| context_used_ratio | context_status | 颜色 |
|--------------------|----------------|------|
| < 0.6 | normal | positive (绿) |
| 0.6 - 0.8 | warning | warning (黄) |
| 0.8 - 0.95 | critical | negative (红) |
| ≥ 0.95 | exceeded | purple (紫) |

### 8.8 Session 详情页

详情页建议左右结构：

| 区域 | 内容 |
|------|------|
| 顶部 Header | title、类型 chip、状态、创建时间、最后活跃、继续会话按钮 |
| 左侧主区 | 消息流 / 对话轮次 / Trace 链路 / 编排 Timeline Tab |
| 右侧属性栏 | 会话属性、参与 Agent、上下文消耗、Token/费用、模型信息 |
| 底部或 Tab | Turn traces、Run steps、Context snapshots、Usage events、附件 |

Tab 建议：

| Tab | 内容 |
|-----|------|
| 消息 | 用户可见消息，内部消息默认折叠 |
| 对话轮次 | 每轮对话的耗时、token、状态、模型/工具/Skill/MCP 调用数 |
| Trace 链路 | 按树或瀑布图展示 `session_trace_spans` |
| 编排 | `session_runs` + `session_run_steps` 时间线 |
| 上下文 | context ratio 趋势、摘要/裁剪事件 |
| 消耗 | 模型调用明细、Token、费用、延迟、工具/Skill/MCP 耗时 |
| 附件 | 上传文件、工具产物 |

### 8.9 Team Session 专属展示

Team session 需要突出编排结构，而不是只显示聊天气泡。

| 组件 | 内容 |
|------|------|
| Participants Panel | 每个 Agent 的头像、角色、状态、Token、context ratio |
| Timeline | planner → executor → tool → reviewer → final response |
| Step Drawer | 点击 step 查看输入、输出、错误、模型和工具参数 |
| Handoff Badge | 展示 Agent A 交给 Agent B 的原因和上下文摘要 |
| Internal Message Toggle | 「显示内部消息」开关，默认关闭 |

Timeline 节点颜色：

| 状态 | 颜色 |
|------|------|
| success | positive |
| running | primary + spinner |
| failed | negative |
| skipped | grey |
| cancelled | warning |

---

## 九、关键设计原则

1. **Session 是事实关联中心，不是所有事实本身**：消息、usage、run、step 各自独立落库，通过 `session_id` 关联。
2. **明细不可变，聚合可重算**：`model_token_usage_events`、`session_run_steps` 是事实源；`sessions` 上的 token、费用、context 是查询优化字段。
3. **Team 与 Agent 共用一套 session 模型**：通过 `owner_type` 区分，不要拆成两套历史系统。
4. **内部编排消息默认可折叠**：Team session 既要可复盘，也不能让用户被内部 step 淹没。
5. **Context ratio 必须可解释**：列表显示比例，详情能追到哪个 run/step/message 导致消耗升高。
6. **软删除优先**：session 删除不应破坏成本统计和审计链路。
7. **模型配置保存快照，但不假设唯一模型**：历史 session 保存默认模型和最近模型；每次实际调用的 provider/model/context window 以 usage 与 step 明细为准。
8. **框架对齐优先**：先查 trpc-agent-go 框架 API 再实现，不在 biz 重写运行时；`runner_snapshot_json` 是 Runner 状态的唯一持久化格式。
9. **分层铁律不可违反**：`internal/biz` 不得 import `pkg/trpc-agent-go`；框架运行时交互只在 `internal/service` 和 `internal/agent` 层进行。
10. **错误处理统一**：biz 层使用 `apierror.BadRequest`/`apierror.NotFound`/`apierror.Conflict`/`apierror.InternalServer`，不用 `fmt.Errorf` 或 `errors.New`。
11. **高频字段拆表**：`session_runtime` 与 `session_metrics` 将高频更新字段从 `sessions` 主表拆出，减少写放大。

---

## 十、Wire 注入

```go
// internal/data/data.go
var ProviderSet = wire.NewSet(
    NewSessionRepo,
    // ... 其他 repo
)

// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    NewSessionUsecase,
    NewSessionMetricsUsecase,
    // ... 其他 usecase
)

// internal/service/service.go
var ProviderSet = wire.NewSet(
    NewSessionService,
    NewSessionCompressor,
    // ... 其他 service
)
```
