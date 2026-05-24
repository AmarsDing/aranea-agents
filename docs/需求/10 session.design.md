# Session 管理模块 — 实现设计文档

> 对应需求：[10 session.md](./10%20session.md) · 开发计划：[10-session-development.md](./10-session-development.md)
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · 运行时边界：[AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)

---

## 〇、分层与单一职责（影响域）

| 层 / 包 | 职责 | 禁止 |
|---------|------|------|
| `api/kratos/session/v1` | 对外 RPC/HTTP 契约 | 业务分支 |
| `internal/service/session.go` | Proto ↔ biz、Audit、**不含**消息发送/Runner Run | import `pkg/trpc-agent-go` 编排 |
| `internal/service/session_batch.go` | 批量预览/归档/删除 RPC | 与 Timeline 聚合混写 |
| `internal/service/session_compress.go` | 异步上下文压缩、`NativeTurnCompressor` | 直接写 Ent |
| `internal/biz/session_usecase.go` | CRUD、Timeline 聚合、消息追加、Turn/State | import trpc-agent-go |
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
- Session 搜索/创建/删除/归档/恢复/重命名/部分更新
- Timeline 时间轴聚合（消息 + 工具调用 + Skill 调用 + MCP 调用）
- 上下文窗口消耗追踪与状态管理
- 异步摘要压缩（SessionCompressor）
- Runner Snapshot 持久化与压缩重写
- Session Summaries 滚动摘要
- Session Turns 对话轮次记录
- Session State KV 状态管理
- 自动标题生成（用户消息截取 + LLM 异步生成）
- 单 Agent / Team 双模式会话

---

## 二、Proto 层

### 2.1 完整 Proto 定义

文件：`api/kratos/session/v1/session.proto`

```protobuf
syntax = "proto3";

package kratos.session.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";

option go_package = "aranea-agents/api/kratos/session/v1;v1";

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
  string status = 15;              // active/running/completed/failed/archived/deleted
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
}

message SessionTimelineSummary {
  int32 total = 1;
  int32 message_count = 2;
  int32 tool_count = 3;
  int32 skill_count = 4;
  int32 mcp_count = 5;
}

message SessionTimelineItem {
  string id = 1;
  string kind = 2;                 // "message" | "tool" | "skill" | "mcp"
  string side = 3;                 // "left" | "right"
  string title = 4;
  string subtitle = 5;
  string actor_id = 6;
  string actor_name = 7;
  string status = 8;
  string occurred_at = 9;
  int32 duration_ms = 10;
  string content_markdown = 11;
  string preview = 12;
  string detail_json = 13;
  repeated string tags = 14;
}

message SessionTimeline {
  string session_id = 1;
  repeated SessionTimelineItem items = 2;
  SessionTimelineSummary summary = 3;
}

message SearchSessionsRequest {
  string owner_type = 1;
  string agent_id = 2;
  string team_id = 3;
  string status = 4;
  string context_status = 5;
  string keyword = 6;
  int32 limit = 7;
  int32 offset = 8;
  int32 page = 9;
  int32 page_size = 10;
  string user_id = 11;
  string sort_by = 12;
  string sort_order = 13;
}

message SearchSessionsResponse {
  repeated Session items = 1;
  int32 total = 2;
  int32 limit = 3;
  int32 offset = 4;
}

message CreateSessionRequest {
  string owner_type = 1;
  string agent_id = 2;
  string team_id = 3;
  string title = 4 [(google.api.field_behavior) = REQUIRED];
  string dialog_mode = 5;
  string default_provider = 6;
  string default_model = 7;
  string workspace_id = 8;
  string user_id = 9;
  string tags_json = 10;
  string metadata_json = 11;
}

message GetSessionRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message UpdateSessionRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string title = 2;
  string tags_json = 3;
  string visibility = 4;
  string metadata_json = 5;
  string dialog_mode = 6;
  string default_provider = 7;
  string default_model = 8;
}

message DeleteSessionRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message DeleteSessionsByAgentRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ArchiveSessionRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message RestoreSessionRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message GetSessionTimelineRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  int32 limit = 2;
  int32 offset = 3;
  string kind_filter = 4;
  string sort_order = 5;
}

message ChatMessageRow {
  string id = 1;
  string session_id = 2;
  string parent_message_id = 3;
  int32 turn_index = 4;
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

message ListSessionMessagesRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  int32 limit = 2;
  int32 offset = 3;
}

message ListSessionMessagesResponse {
  repeated ChatMessageRow items = 1;
  int32 total = 2;
}

message SessionTurn {
  string id = 1;
  string session_id = 2;
  string run_id = 3;
  int32 turn_index = 4;
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

message ListSessionTurnsRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  int32 limit = 2;
  int32 offset = 3;
}

message ListSessionTurnsResponse {
  repeated SessionTurn items = 1;
  int32 total = 2;
}

service SessionService {
  rpc SearchSessions(SearchSessionsRequest) returns (SearchSessionsResponse) {
    option (google.api.http) = {get: "/v1/sessions"};
  }
  rpc CreateSession(CreateSessionRequest) returns (Session) {
    option (google.api.http) = {post: "/v1/sessions" body: "*"};
  }
  rpc DeleteSessionsByAgent(DeleteSessionsByAgentRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/sessions"};
  }
  rpc GetSession(GetSessionRequest) returns (Session) {
    option (google.api.http) = {get: "/v1/sessions/{id}"};
  }
  rpc UpdateSession(UpdateSessionRequest) returns (Session) {
    option (google.api.http) = {patch: "/v1/sessions/{id}" body: "*"};
  }
  rpc DeleteSession(DeleteSessionRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/sessions/{id}"};
  }
  rpc ArchiveSession(ArchiveSessionRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {post: "/v1/sessions/{id}/archive" body: "*"};
  }
  rpc RestoreSession(RestoreSessionRequest) returns (Session) {
    option (google.api.http) = {post: "/v1/sessions/{id}/restore" body: "*"};
  }
  rpc GetSessionTimeline(GetSessionTimelineRequest) returns (SessionTimeline) {
    option (google.api.http) = {get: "/v1/sessions/{id}/timeline"};
  }
  rpc ListSessionMessages(ListSessionMessagesRequest) returns (ListSessionMessagesResponse) {
    option (google.api.http) = {get: "/v1/sessions/{id}/messages"};
  }
  rpc ListSessionTurns(ListSessionTurnsRequest) returns (ListSessionTurnsResponse) {
    option (google.api.http) = {get: "/v1/sessions/{session_id}/turns"};
  }
  rpc SearchSessionMessages(SearchSessionMessagesRequest) returns (SearchSessionMessagesResponse) {
    option (google.api.http) = {get: "/v1/sessions/messages/search"};
  }
  rpc BatchPreviewSessions(BatchPreviewSessionsRequest) returns (BatchPreviewSessionsResponse) {
    option (google.api.http) = {post: "/v1/sessions:batchPreview" body: "*"};
  }
  rpc BatchArchiveSessions(BatchArchiveSessionsRequest) returns (BatchSessionsResponse) {
    option (google.api.http) = {post: "/v1/sessions:batchArchive" body: "*"};
  }
  rpc BatchDeleteSessions(BatchDeleteSessionsRequest) returns (BatchSessionsResponse) {
    option (google.api.http) = {post: "/v1/sessions:batchDelete" body: "*"};
  }
}
```

> 完整定义以仓库内 `api/kratos/session/v1/session.proto` 为准；上文为设计索引，非逐字拷贝。

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
| `RestoreSession` | `POST /v1/sessions/{id}/restore` | 恢复归档会话（status=active, 清空 archived_at/deleted_at） |
| `GetSessionTimeline` | `GET /v1/sessions/{id}/timeline` | 聚合消息+工具+Skill+MCP 的时间轴，支持 limit/offset/kind_filter/sort_order |
| `ListSessionMessages` | `GET /v1/sessions/{id}/messages` | 获取会话消息列表，支持 limit/offset 分页 |
| `ListSessionTurns` | `GET /v1/sessions/{session_id}/turns` | 获取会话轮次列表，支持 limit/offset 分页 |

### 2.3 批量操作 RPC（Phase 1b — 已实现）

> 需求来源：会话历史列表批量治理（2026-05-20）。契约以 `api/kratos/session/v1/session.proto` 为准。

#### 2.3.1 Proto（现行）

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

实现：`internal/biz/session_batch.go`

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
// internal/biz — SessionRepository
ListSessionsForBatch(ctx context.Context, q SessionSearchQuery) ([]Session, error)
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

单条 `DeleteSession` / `ArchiveSession`：`running` 不可删/归档；已删幂等；列表行操作复用上述 RPC。

---

## 三、Biz 层

### 3.1 领域模型

文件：`internal/biz/session_usecase.go`

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
    Status                     string   // active/running/completed/failed/archived/deleted
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
    RunnerSnapshotJSON         string
    StateJSON                  string
    MetadataJSON               string
}

type SessionSearchQuery struct {
    OwnerType     string
    AgentID       string
    TeamID        string
    Status        string
    ContextStatus string
    Keyword       string
    UserID        string
    Limit         int
    Offset        int
    Page          int
    PageSize      int
    SortBy        string
    SortOrder     string
}

type SessionListResult struct {
    Items  []Session
    Total  int
    Limit  int
    Offset int
}

type ChatMessage struct {
    ID               string
    SessionID        string
    ParentMessageID  string
    TurnIndex        int
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

type ToolInvocationView struct {
    ID               string
    ToolKey          string
    ToolDisplayName  string
    AgentID          string
    AgentDisplayName string
    SessionID        string
    Source           string   // "" | "mcp"
    Status           string
    StartedAt        string
    EndedAt          string
    DurationMS       int
    InputPreview     string
    OutputPreview    string
    ErrorCode        string
    ErrorMessage     string
    MetadataJSON     string
    CreatedAt        string
}

type SkillInvocationView struct {
    ID               string
    SkillID          string
    SkillName        string
    SkillVersion     string
    AgentID          string
    AgentDisplayName string
    SessionID        string
    Status           string
    DurationMS       int
    StartedAt        string
    EndedAt          string
    InputPreview     string
    OutputPreview    string
    ErrorCode        string
    ErrorMessage     string
}

type SessionTimelineItem struct {
    ID              string
    Kind            string   // "message" | "tool" | "skill" | "mcp"
    Side            string   // "left" | "right"
    Title           string
    Subtitle        string
    ActorID         string
    ActorName       string
    Status          string
    OccurredAt      string
    DurationMS      int
    ContentMarkdown string
    Preview         string
    DetailJSON      string
    Tags            []string
}

type SessionTimelineSummary struct {
    Total        int
    MessageCount int
    ToolCount    int
    SkillCount   int
    MCPCount     int
}

type SessionTimeline struct {
    SessionID string
    Items     []SessionTimelineItem
    Summary   SessionTimelineSummary
}

type SessionSummary struct {
    ID              string
    SessionID       string
    SummaryMarkdown string
    FromTurn        int
    ToTurn          int
    TokenEstimate   int
    CreatedAt       string
}
```

### 3.2 Repository 接口

文件：`internal/biz/session_usecase.go`

```go
type SessionRepository interface {
    SearchSessions(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
    CreateSession(ctx context.Context, s Session) (Session, error)
    GetSessionByID(ctx context.Context, id string) (Session, error)
    UpdateSessionTitle(ctx context.Context, id, title string) (Session, error)
    UpdateSession(ctx context.Context, id string, fields SessionUpdateFields) (Session, error)
    RestoreSession(ctx context.Context, id string) (Session, error)
    ArchiveSession(ctx context.Context, id string) error
    DeleteSession(ctx context.Context, id string) error
    DeleteSessionsByAgentID(ctx context.Context, agentID string) error
    CountMessagesBySession(ctx context.Context, sessionID string) (int, error)
    ListMessagesBySession(ctx context.Context, sessionID string, limit, offset int) ([]ChatMessage, error)
    ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]ChatMessage, error)
    ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]ChatMessage, error)
    ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
    ListToolInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]ToolInvocationView, error)
    ListSkillInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]SkillInvocationView, error)
    AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error
    AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
    UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error
    UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error
    UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error
    InsertSessionSummary(ctx context.Context, row SessionSummary) error
    MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error)
    ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error)
    LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error)
    UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error
    GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
    SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
    CreateSessionTurn(ctx context.Context, turn SessionTurn) (SessionTurn, error)
    UpdateSessionTurn(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error)
    ListSessionTurns(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error)
    GetSessionTurn(ctx context.Context, id string) (SessionTurn, error)
    SearchMessages(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error)
    IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error
    ListSessionsForBatch(ctx context.Context, q SessionSearchQuery) ([]Session, error)
    ArchiveSessionsByIDs(ctx context.Context, ids []string) (processed int, failed []string, err error)
    DeleteSessionsByIDs(ctx context.Context, ids []string) (processed int, failed []string, err error)
}
```

### 3.3 Usecase 方法

```go
type SessionUsecase struct {
    sessions       SessionRepository
    agents         AgentRepository
    teams          TeamRepository
    titleGenerator SessionTitleGenerator
}

func NewSessionUsecase(sessions SessionRepository, agents AgentRepository, teams TeamRepository, titleGenerator SessionTitleGenerator) *SessionUsecase

func (uc *SessionUsecase) Search(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
func (uc *SessionUsecase) Get(ctx context.Context, id string) (Session, error)
func (uc *SessionUsecase) Create(ctx context.Context, in Session) (Session, error)
func (uc *SessionUsecase) Rename(ctx context.Context, id, title string) (Session, error)
func (uc *SessionUsecase) Update(ctx context.Context, id string, fields SessionUpdateFields) (Session, error)
func (uc *SessionUsecase) Restore(ctx context.Context, id string) (Session, error)
func (uc *SessionUsecase) Archive(ctx context.Context, id string) error
func (uc *SessionUsecase) Delete(ctx context.Context, id string) error
func (uc *SessionUsecase) DeleteByAgent(ctx context.Context, agentID string) error
func (uc *SessionUsecase) ListMessages(ctx context.Context, sessionID string) ([]ChatMessage, error)
func (uc *SessionUsecase) ListMessagesPaged(ctx context.Context, sessionID string, limit, offset int) (MessageListResult, error)
func (uc *SessionUsecase) ListMessagesAfterTurn / ListMessagesByStatus / ListMessagesRecent(...)
func (uc *SessionUsecase) AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error
func (uc *SessionUsecase) AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
func (uc *SessionUsecase) UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error
func (uc *SessionUsecase) UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error
func (uc *SessionUsecase) UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error
func (uc *SessionUsecase) InsertSessionSummary(ctx context.Context, row SessionSummary) error
func (uc *SessionUsecase) MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error)
func (uc *SessionUsecase) ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error)
func (uc *SessionUsecase) LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error)
func (uc *SessionUsecase) UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error
func (uc *SessionUsecase) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
func (uc *SessionUsecase) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
func (uc *SessionUsecase) ApplyStateDelta(ctx context.Context, sessionID string, delta DomainStateDelta) error
func (uc *SessionUsecase) CreateTurn(ctx context.Context, turn SessionTurn) (SessionTurn, error)
func (uc *SessionUsecase) UpdateTurn(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error)
func (uc *SessionUsecase) ListTurns(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error)
func (uc *SessionUsecase) Timeline(ctx context.Context, id string, q TimelineQuery) (SessionTimeline, error)
func (uc *SessionUsecase) SearchMessages(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error)
func (uc *SessionUsecase) PreviewBatch / BatchArchive / BatchDelete(...)  // 见 session_batch.go
func (uc *SessionUsecase) IncrementInvocationCounts(...)  // 工具/MCP/Skill 计数回填 sessions
```

### 3.4 关键业务逻辑

**Create 校验**：
- `owner_type=agent` 时，`agent_id` 必填且 agent 必须存在
- `owner_type=team` 时，`team_id` 必填且 team 必须存在
- 自动生成 `uuid.NewString()` 作为 ID
- 默认 `status=active`, `context_status=normal`

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
- `LLMSessionTitleGenerator`：使用轻量模型（如 gpt-4o-mini）生成标题
- `NoopSessionTitleGenerator`：空实现，用于测试

**Session State KV**：
- `GetSessionState`/`SaveSessionState`：读写 `sessions.state_json`（JSON 序列化的 `map[string]string`）
- `ApplyStateDelta`：支持 `set`/`append`/`delete` 操作的增量更新

**DomainStateDelta**：
```go
type DomainStateDelta struct {
    Path      string
    ValueJSON string
    Operation string  // "set" | "append" | "delete"
}
```

**Session Turns**：
- `CreateTurn`：创建对话轮次，自动生成 UUID 和时间戳
- `UpdateTurn`：部分更新轮次（status/duration/token/counts 等）
- `ListTurns`：按 turn_index 升序分页查询

**NativeTurnCompressor 接口**：
```go
type NativeTurnCompressor interface {
    AfterNativeTurn(ctx context.Context, sessionID string, agent Agent)
}
```

---

## 四、Data 层

### 4.1 Ent Schema

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
        field.String("status").Default("active"),
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
        field.Float("avg_latency_ms").Default(0),
        field.Int("error_count").Default(0),
        field.Int("context_used_tokens").Default(0),
        field.Float("context_used_ratio").Default(0),
        field.Float("max_context_used_ratio").Default(0),
        field.String("context_status").Default("normal"),
        field.String("first_message_at").Default(""),
        field.String("last_message_at").Default(""),
        field.String("last_run_at").Default(""),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("archived_at").Default(""),
        field.String("deleted_at").Default(""),
        field.Text("runner_snapshot_json").Default(""),
        field.Text("state_json").Default("{}"),
        field.Text("metadata_json").Default("{}"),
    }
}
```

### 4.2 Ent Schema — SessionTurn

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
        field.Int("turn_index").Default(0),
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
        index.Fields("session_id", "turn_index"),
        index.Fields("status", "started_at"),
    }
}
```

### 4.3 session_summaries DDL

`session_summaries` 表通过 raw SQL 创建（非 Ent Schema），DDL：

```sql
CREATE TABLE IF NOT EXISTS session_summaries (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  summary_markdown TEXT NOT NULL DEFAULT '',
  from_turn INTEGER NOT NULL DEFAULT 0,
  to_turn INTEGER NOT NULL DEFAULT 0,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_session_summaries_session
  ON session_summaries(session_id, created_at);
```

### 4.4 Ent Schema — Message

文件：`internal/data/ent/schema/message.go`

```go
type Message struct {
    ent.Schema
}

func (Message) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "messages"},
    }
}

func (Message) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("session_id").MaxLen(256),
        field.String("parent_message_id").Default(""),
        field.Int("turn_index").Default(0),
        field.String("role"),
        field.Text("content_markdown").Default(""),
        field.String("model_name").Default(""),
        field.Int("token_in").Default(0),
        field.Int("token_out").Default(0),
        field.Int("latency_ms").Default(0),
        field.String("status").Default("ok"),
        field.Int("attachments_count").Default(0),
        field.Text("options_json").Default(""),
        field.Text("error_message").Default(""),
        field.String("created_at"),
    }
}
```

### 4.3 Session Summaries 表（原生 SQL）

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
```

### 4.4 关键 Data 层实现

**SearchSessions** — Ent ORM 查询：
```go
func (r *sessionRepo) SearchSessions(ctx context.Context, q biz.SessionSearchQuery) (biz.SessionListResult, error) {
    c := r.data.entClient
    limit := clampSessionLimit(q.Limit)
    offset := clampOffset(q.Offset)

    wheres := []predicate.Session{entsession.DeletedAtEQ("")}
    if q.OwnerType != "" {
        wheres = append(wheres, entsession.OwnerTypeEQ(q.OwnerType))
    }
    if q.AgentID != "" {
        wheres = append(wheres, entsession.AgentIDEQ(q.AgentID))
    }
    if q.TeamID != "" {
        wheres = append(wheres, entsession.TeamIDEQ(q.TeamID))
    }
    if q.Status != "" {
        wheres = append(wheres, entsession.StatusEQ(q.Status))
    }
    if q.ContextStatus != "" {
        wheres = append(wheres, entsession.ContextStatusEQ(q.ContextStatus))
    }
    if kw := strings.TrimSpace(q.Keyword); kw != "" {
        wheres = append(wheres, entsession.Or(
            entsession.TitleContainsFold(kw),
            entsession.SummaryContainsFold(kw),
            entsession.IDContainsFold(kw),
        ))
    }

    wherePred := entsession.And(wheres...)
    total, err := c.Session.Query().Where(wherePred).Count(ctx)
    if err != nil {
        return biz.SessionListResult{}, err
    }

    rows, err := c.Session.Query().
        Where(wherePred).
        Order(
            entsession.ByLastMessageAt(entsql.OrderDesc()),
            entsession.ByUpdatedAt(entsql.OrderDesc()),
        ).
        Limit(limit).
        Offset(offset).
        All(ctx)
    if err != nil {
        return biz.SessionListResult{}, err
    }
    items := make([]biz.Session, 0, len(rows))
    for _, row := range rows {
        items = append(items, entSessionToBiz(row))
    }
    return biz.SessionListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}
```

**AppendChatTurn** — 事务写入用户+助手消息对：
```go
func (r *sessionRepo) AppendChatTurn(ctx context.Context, sessionID string, user, assistant biz.ChatMessage) error {
    tx, err := r.data.entClient.Tx(ctx)
    if err != nil {
        return err
    }
    rollback := func(e error) error { _ = tx.Rollback(); return e }

    if _, err = tx.Session.Query().Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).Only(ctx); err != nil {
        return rollback(err)
    }
    maxTurn, err := r.maxMessageTurnTx(ctx, tx, sessionID)
    if err != nil {
        return rollback(err)
    }
    user.TurnIndex = maxTurn + 1
    assistant.TurnIndex = maxTurn + 2
    if err = r.insertMessageTx(ctx, tx, user); err != nil {
        return rollback(err)
    }
    if err = r.insertMessageTx(ctx, tx, assistant); err != nil {
        return rollback(err)
    }
    upd := tx.Session.UpdateOneID(sessionID).
        AddMessageCount(2).
        SetLastMessageAt(assistant.CreatedAt).
        SetUpdatedAt(nowRFC3339()).
        AddModelCallCount(1)
    if tin, tout := assistant.TokenIn, assistant.TokenOut; tin > 0 || tout > 0 {
        upd = upd.AddInputTokens(tin).AddOutputTokens(tout).AddTotalTokens(tin+tout).AddContextUsedTokens(tin+tout)
    }
    if _, err = upd.Save(ctx); err != nil {
        return rollback(err)
    }
    return tx.Commit()
}
```

**UpdateSessionContextFromLLMUsage** — 上下文消耗更新：
```go
func (r *sessionRepo) UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, _ int, contextWindow int) error {
    cur, err := r.GetSessionByID(ctx, sessionID)
    if err != nil {
        return err
    }
    ratio := cur.ContextUsedRatio
    if contextWindow > 0 && promptTokens > 0 {
        ratio = float64(promptTokens) / float64(contextWindow)
        if ratio > 1 { ratio = 1 }
    }
    maxR := cur.MaxContextUsedRatio
    if ratio > maxR { maxR = ratio }
    upd := r.data.entClient.Session.Update().
        Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
        SetContextUsedRatio(ratio).
        SetMaxContextUsedRatio(maxR).
        SetContextStatus(contextStatusForRatio(ratio)).
        SetUpdatedAt(nowRFC3339())
    if contextWindow > 0 { upd = upd.SetLastContextWindowTokens(contextWindow) }
    if promptTokens > 0 { upd = upd.SetContextUsedTokens(promptTokens) }
    _, err = upd.Save(ctx)
    return err
}
```

**ListToolInvocationsBySession** — 工具调用查询 + Agent 名称解析：
```go
func (r *sessionRepo) ListToolInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]biz.ToolInvocationView, error) {
    if limit <= 0 || limit > 100 { limit = 100 }
    rows, err := r.data.entClient.ToolInvocation.Query().
        Where(toolinvocationpkg.SessionIDEQ(sessionID)).
        Order(toolinvocationpkg.ByStartedAt(entsql.OrderDesc()), toolinvocationpkg.ByCreatedAt(entsql.OrderDesc())).
        Limit(limit).
        All(ctx)
    if err != nil { return nil, err }
    if len(rows) == 0 { return nil, nil }
    // 批量查 Agent 名称
    agentNames := map[string]string{}
    agentIDs := dedupeStrings(extractAgentIDs(rows))
    if len(agentIDs) > 0 {
        agents, _ := r.data.entClient.Agent.Query().Where(agent.IDIn(agentIDs...), agent.DeletedAtEQ("")).All(ctx)
        for _, a := range agents { agentNames[a.ID] = a.DisplayName }
    }
    // 转换
    out := make([]biz.ToolInvocationView, 0, len(rows))
    for _, row := range rows {
        out = append(out, biz.ToolInvocationView{
            ID: row.ID, ToolKey: row.ToolKey, ToolDisplayName: row.ToolKey,
            AgentID: row.AgentID, AgentDisplayName: agentNames[row.AgentID],
            SessionID: row.SessionID, Source: row.Source, Status: row.Status,
            StartedAt: row.StartedAt, EndedAt: row.EndedAt, DurationMS: row.DurationMs,
            InputPreview: row.InputPreview, OutputPreview: row.OutputPreview,
            ErrorCode: row.ErrorCode, ErrorMessage: row.ErrorMessage,
            MetadataJSON: row.MetadataJSON, CreatedAt: row.CreatedAt,
        })
    }
    return out, nil
}
```

**Session Summaries CRUD** — 原生 SQL：
```go
func (r *sessionRepo) InsertSessionSummary(ctx context.Context, row biz.SessionSummary) error {
    q := `INSERT INTO session_summaries (id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at)
          VALUES (?,?,?,?,?,?,?)`
    _, err := r.data.entClient.ExecContext(ctx, q,
        row.ID, row.SessionID, row.SummaryMarkdown, row.FromTurn, row.ToTurn, row.TokenEstimate, row.CreatedAt)
    return err
}

func (r *sessionRepo) MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error) {
    var max int
    err := entQueryRowScan(r.data.entClient, ctx,
        `SELECT COALESCE(MAX(to_turn), 0) FROM session_summaries WHERE session_id = ?`,
        []any{sessionID}, &max)
    return max, err
}

func (r *sessionRepo) ListSessionSummaries(ctx context.Context, sessionID string) ([]biz.SessionSummary, error) {
    rows, err := r.data.entClient.QueryContext(ctx,
        `SELECT id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at
         FROM session_summaries WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
    if err != nil { return nil, err }
    defer rows.Close()
    var out []biz.SessionSummary
    for rows.Next() {
        var s biz.SessionSummary
        if err := rows.Scan(&s.ID, &s.SessionID, &s.SummaryMarkdown, &s.FromTurn, &s.ToTurn, &s.TokenEstimate, &s.CreatedAt); err != nil {
            return nil, err
        }
        out = append(out, s)
    }
    return out, rows.Err()
}
```

### 4.5 类型转换

```go
func entSessionToBiz(e *ent.Session) biz.Session {
    if e == nil { return biz.Session{} }
    return biz.Session{
        ID: e.ID, WorkspaceID: e.WorkspaceID, UserID: e.UserID,
        OwnerType: e.OwnerType, AgentID: e.AgentID, TeamID: e.TeamID,
        Title: e.Title, Summary: e.Summary, TagsJSON: e.TagsJSON,
        DialogMode: e.DialogMode, DefaultProvider: e.DefaultProvider,
        DefaultModel: e.DefaultModel, DefaultContextWindowTokens: e.DefaultContextWindowTokens,
        LastProvider: e.LastProvider, LastModel: e.LastModel,
        LastContextWindowTokens: e.LastContextWindowTokens,
        Status: e.Status, Visibility: e.Visibility,
        MessageCount: e.MessageCount, RunCount: e.RunCount,
        ModelCallCount: e.ModelCallCount, ToolCallCount: e.ToolCallCount,
        SkillCallCount: e.SkillCallCount, MCPCallCount: e.McpCallCount,
        InputTokens: e.InputTokens, OutputTokens: e.OutputTokens,
        TotalTokens: e.TotalTokens, TotalCostMicroUSD: e.TotalCostMicroUsd,
        AvgLatencyMs: e.AvgLatencyMs, ErrorCount: e.ErrorCount,
        ContextUsedTokens: e.ContextUsedTokens, ContextUsedRatio: e.ContextUsedRatio,
        MaxContextUsedRatio: e.MaxContextUsedRatio, ContextStatus: e.ContextStatus,
        FirstMessageAt: e.FirstMessageAt, LastMessageAt: e.LastMessageAt,
        LastRunAt: e.LastRunAt, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
        ArchivedAt: e.ArchivedAt, DeletedAt: e.DeletedAt,
        RunnerSnapshotJSON: e.RunnerSnapshotJSON, MetadataJSON: e.MetadataJSON,
    }
}
```

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
        Title: s.Title, Summary: s.Summary, TagsJson: s.TagsJSON,
        DialogMode: s.DialogMode, DefaultProvider: s.DefaultProvider,
        DefaultModel: s.DefaultModel,
        DefaultContextWindowTokens: int32(s.DefaultContextWindowTokens),
        LastProvider: s.LastProvider, LastModel: s.LastModel,
        LastContextWindowTokens: int32(s.LastContextWindowTokens),
        Status: s.Status, Visibility: s.Visibility,
        MessageCount: int32(s.MessageCount), RunCount: int32(s.RunCount),
        ModelCallCount: int32(s.ModelCallCount), ToolCallCount: int32(s.ToolCallCount),
        SkillCallCount: int32(s.SkillCallCount), McpCallCount: int32(s.MCPCallCount),
        InputTokens: int32(s.InputTokens), OutputTokens: int32(s.OutputTokens),
        TotalTokens: int32(s.TotalTokens), TotalCostMicroUsd: s.TotalCostMicroUSD,
        AvgLatencyMs: s.AvgLatencyMs, ErrorCount: int32(s.ErrorCount),
        ContextUsedTokens: int32(s.ContextUsedTokens),
        ContextUsedRatio: s.ContextUsedRatio,
        MaxContextUsedRatio: s.MaxContextUsedRatio,
        ContextStatus: s.ContextStatus,
        FirstMessageAt: s.FirstMessageAt, LastMessageAt: s.LastMessageAt,
        LastRunAt: s.LastRunAt, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
        ArchivedAt: s.ArchivedAt, DeletedAt: s.DeletedAt,
        RunnerSnapshotJson: s.RunnerSnapshotJSON, MetadataJson: s.MetadataJSON,
    }
}

func toProtoTimeline(t biz.SessionTimeline) *v1.SessionTimeline {
    items := make([]*v1.SessionTimelineItem, 0, len(t.Items))
    for i := range t.Items {
        items = append(items, toProtoTimelineItem(t.Items[i]))
    }
    return &v1.SessionTimeline{
        SessionId: t.SessionID, Items: items,
        Summary: &v1.SessionTimelineSummary{
            Total: int32(t.Summary.Total), MessageCount: int32(t.Summary.MessageCount),
            ToolCount: int32(t.Summary.ToolCount), SkillCount: int32(t.Summary.SkillCount),
            McpCount: int32(t.Summary.MCPCount),
        },
    }
}

func toProtoTimelineItem(it biz.SessionTimelineItem) *v1.SessionTimelineItem {
    tags := it.Tags
    if tags == nil { tags = []string{} }
    return &v1.SessionTimelineItem{
        Id: it.ID, Kind: it.Kind, Side: it.Side,
        Title: it.Title, Subtitle: it.Subtitle,
        ActorId: it.ActorID, ActorName: it.ActorName,
        Status: it.Status, OccurredAt: it.OccurredAt,
        DurationMs: int32(it.DurationMS),
        ContentMarkdown: it.ContentMarkdown, Preview: it.Preview,
        DetailJson: it.DetailJSON, Tags: tags,
    }
}

func toProtoChatMessageRow(m biz.ChatMessage) *v1.ChatMessageRow {
    return &v1.ChatMessageRow{
        Id: m.ID, SessionId: m.SessionID, ParentMessageId: m.ParentMessageID,
        TurnIndex: int32(m.TurnIndex), Role: m.Role,
        ContentMarkdown: m.ContentMarkdown, ModelName: m.ModelName,
        TokenIn: int32(m.TokenIn), TokenOut: int32(m.TokenOut),
        LatencyMs: int32(m.LatencyMS), Status: m.Status,
        AttachmentsCount: int32(m.AttachmentsCount),
        OptionsJson: m.OptionsJSON, ErrorMessage: m.ErrorMessage,
        CreatedAt: m.CreatedAt,
    }
}
```

### 5.3 RPC 实现

```go
func (s *SessionService) SearchSessions(ctx context.Context, req *v1.SearchSessionsRequest) (*v1.SearchSessionsResponse, error) {
    q := searchQueryFromProto(req)
    res, err := s.uc.Search(ctx, q)
    if err != nil { return nil, err }
    out := &v1.SearchSessionsResponse{
        Total: int32(res.Total), Limit: int32(res.Limit), Offset: int32(res.Offset),
        Items: make([]*v1.Session, 0, len(res.Items)),
    }
    for i := range res.Items {
        out.Items = append(out.Items, toProtoSession(res.Items[i]))
    }
    return out, nil
}

func (s *SessionService) CreateSession(ctx context.Context, req *v1.CreateSessionRequest) (*v1.Session, error) {
    in := biz.Session{
        WorkspaceID: req.GetWorkspaceId(), UserID: req.GetUserId(),
        OwnerType: req.GetOwnerType(), AgentID: req.GetAgentId(), TeamID: req.GetTeamId(),
        Title: req.GetTitle(), DialogMode: req.GetDialogMode(),
        DefaultProvider: req.GetDefaultProvider(), DefaultModel: req.GetDefaultModel(),
    }
    created, err := s.uc.Create(ctx, in)
    if err != nil { return nil, err }
    return toProtoSession(created), nil
}

func (s *SessionService) GetSession(ctx context.Context, req *v1.GetSessionRequest) (*v1.Session, error) {
    out, err := s.uc.Get(ctx, req.GetId())
    if err != nil { return nil, mapSessionErr(err) }
    return toProtoSession(out), nil
}

func (s *SessionService) UpdateSession(ctx context.Context, req *v1.UpdateSessionRequest) (*v1.Session, error) {
    // 映射 SessionUpdateFields：title / tags_json / visibility / metadata / dialog_mode / default_provider / default_model
    out, err := s.uc.Update(ctx, req.GetId(), fields)
    if err != nil { return nil, mapSessionErr(err) }
    return toProtoSession(out), nil
}

func (s *SessionService) DeleteSession(ctx context.Context, req *v1.DeleteSessionRequest) (*emptypb.Empty, error) {
    if err := s.uc.Delete(ctx, req.GetId()); err != nil { return nil, mapSessionErr(err) }
    return &emptypb.Empty{}, nil
}

func (s *SessionService) DeleteSessionsByAgent(ctx context.Context, req *v1.DeleteSessionsByAgentRequest) (*emptypb.Empty, error) {
    if err := s.uc.DeleteByAgent(ctx, req.GetAgentId()); err != nil { return nil, err }
    return &emptypb.Empty{}, nil
}

func (s *SessionService) ArchiveSession(ctx context.Context, req *v1.ArchiveSessionRequest) (*emptypb.Empty, error) {
    if err := s.uc.Archive(ctx, req.GetId()); err != nil { return nil, mapSessionErr(err) }
    return &emptypb.Empty{}, nil
}

func (s *SessionService) GetSessionTimeline(ctx context.Context, req *v1.GetSessionTimelineRequest) (*v1.SessionTimeline, error) {
    out, err := s.uc.Timeline(ctx, req.GetId())
    if err != nil { return nil, mapSessionErr(err) }
    return toProtoTimeline(out), nil
}

func (s *SessionService) ListSessionMessages(ctx context.Context, req *v1.ListSessionMessagesRequest) (*v1.ListSessionMessagesResponse, error) {
    rows, err := s.uc.ListMessages(ctx, req.GetId())
    if err != nil { return nil, mapSessionErr(err) }
    out := make([]*v1.ChatMessageRow, 0, len(rows))
    for i := range rows { out = append(out, toProtoChatMessageRow(rows[i])) }
    return &v1.ListSessionMessagesResponse{Items: out}, nil
}
```

### 5.4 SessionCompressor

文件：`internal/service/session_compress.go`

```go
type SessionCompressor struct {
    Sessions    *biz.SessionUsecase
    Agents      biz.AgentRepository
    Compress    compress.Compressor
    RT          *runtimedeps.Runtime
    MonitorLogs *biz.MonitorLogBroker
    inFlight    sync.Map
}

func NewSessionCompressor(
    sessions *biz.SessionUsecase,
    agents biz.AgentRepository,
    rt *runtimedeps.Runtime,
    comp compress.Compressor,
    monitorLogs *biz.MonitorLogBroker,
) *SessionCompressor
```

**压缩流程**：
1. 检查 `context_used_ratio` 是否超过 agent 阈值（默认 0.6）
2. 100% 满窗时立即压缩，否则检查距上次压缩间隔 ≥ 10 分钟
3. 获取消息列表，计算需要压缩的 turn 范围（保留最近 `keepTurns` 轮）
4. 调用 `compress.Compressor.Compress()` 生成摘要
5. 写入 `session_summaries` 表
6. 合并所有摘要，重写 `runner_snapshot_json`（摘要事件 + 尾部消息事件）
7. 更新 `context_used_ratio` 为压缩后估算值
8. 更新 `sessions.summary` 为首行摘要
9. 清理 SessionMemory 中的旧事件实体

**压缩模型选择**：
- 优先使用 `agent.settings.L0CompressProvider` + `L0CompressModel`
- 回退到 `session.DefaultProvider` + `DefaultModel`
- 最终回退到 `agent.Provider` + `agent.Model`

---

## 六、运行时层

### 6.1 trpc session.Service 桥接（待实现）

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

### 6.2 Runner Snapshot 结构

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

## 七、Wire 注入

```go
// internal/data/data.go
var ProviderSet = wire.NewSet(
    NewSessionRepo,
    // ... 其他 repo
)

// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    NewSessionUsecase,
    // ... 其他 usecase
)

// internal/service/service.go
var ProviderSet = wire.NewSet(
    NewSessionService,
    NewSessionCompressor,
    // ... 其他 service
)
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/session/
├── api.ts                          ← Kratos API 封装 + 类型定义
web/src/components/chat/
├── ChatSessionSidebar.vue          ← Chat 页右侧 Session 列表
├── SessionTimelineDialog.vue       ← 历史追踪弹窗
web/src/components/sessions/
├── sessionUi.ts                    ← 工具函数（格式化、颜色、列定义）
├── SessionsPageHero.vue            ← 页面标题
├── SessionsSummaryCards.vue        ← 摘要卡片
├── SessionsFilterBar.vue           ← 筛选栏
├── SessionsErrorBanner.vue         ← 错误提示
├── SessionsSelectedDetail.vue      ← 选中详情
├── SessionsTableSection.vue        ← 表格+分页
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
  provider: string;
  model: string;
  status: string;
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
};

export type SessionListResult = {
  items: Session[];
  total: number;
  limit: number;
  offset: number;
};

export type SessionTimelineItem = {
  id: string;
  kind: "message" | "tool" | "skill" | "mcp" | string;
  side: "left" | "right" | string;
  title: string;
  subtitle: string;
  actor_id: string;
  actor_name: string;
  status: string;
  occurred_at: string;
  duration_ms: number;
  content_markdown: string;
  preview: string;
  detail_json: string;
  tags: string[];
};

export type SessionTimelineSummary = {
  total: number;
  message_count: number;
  tool_count: number;
  skill_count: number;
  mcp_count: number;
};

export type SessionTimeline = {
  session_id: string;
  items: SessionTimelineItem[];
  summary: SessionTimelineSummary;
};

export async function listSessions(agentID: string): Promise<Session[]>
export async function listTeamSessions(teamID: string): Promise<Session[]>
export async function searchSessions(query?: SessionSearchQuery): Promise<SessionListResult>
export async function getSession(id: string): Promise<Session>
export async function getSessionTimeline(id: string): Promise<SessionTimeline>
export async function createSession(payload: { owner_type?; agent_id?; team_id?; title; dialog_mode?; default_provider?; default_model?; workspace_id?; user_id? }): Promise<Session>
export async function deleteSession(id: string): Promise<void>
export async function archiveSession(id: string): Promise<void>
export async function updateSessionTitle(id: string, title: string): Promise<Session>
export async function clearAgentSessions(agentID: string): Promise<void>
export async function listSessionChatMessages(sessionID: string): Promise<Message[]>
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
| Skill | deep-purple |
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

> **2026-05-20 增量**：行删除、批量选择、按保留天数归档/删除。组件拆分见 §8.5.1。

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

#### 8.5.1 前端文件结构（新增/变更）

```
web/src/
├── pages/SessionsPage.vue                    ← 挂载 composable，不含裸 API
├── features/session/
│   ├── api.ts                                ← batchPreview/batchArchive/batchDelete/deleteSession
│   ├── types.ts                              ← BatchPreviewResult, BulkProgress
│   └── useSessionsPage.ts                    ← ★ 列表加载、selection、bulk 进度、dialog 状态
├── components/sessions/
│   ├── SessionsTableSection.vue              ← checkbox 列、行删除、emit selection
│   ├── SessionsBulkToolbar.vue               ← 批量选择 toggle、按天数按钮
│   ├── SessionsBulkSelectionBar.vue          ← 已选 N + 归档/删除
│   ├── SessionDeleteConfirmDialog.vue        ← 单条/批量删除确认
│   ├── SessionRetentionDialog.vue            ← 保留天数 + preview + 归档/删除确认
│   └── sessionUi.ts                          ← 列定义（含 selection 列）
└── stores/session/index.ts                   ← 可选：bulk 进度全局态（或 composable 内 ref）
```

**分层纪律**（`frontend-guide.md`）：

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
export function ownerChipColor(value: string): string    // "team" → "deep-purple", else → "primary"
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

---

## 九、trpc-agent-go 对齐路径

| 阶段 | 内容 | 状态 |
|------|------|------|
| M5-1 | Ent + SQLite Session CRUD + Timeline | ✅ 已实现 |
| M5-2 | 上下文追踪 + 摘要压缩 | ✅ 已实现 |
| M5-3 | Runner Snapshot 持久化 | ✅ 已实现 |
| M5-3a | Session Turns 对话轮次 | ✅ 已实现 |
| M5-3b | Session State KV + ApplyStateDelta | ✅ 已实现 |
| M5-3c | RestoreSession / UpdateSession 部分更新 | ✅ 已实现 |
| M5-3d | 自动标题生成（LLM + 截取双策略） | ✅ 已实现 |
| M5-4 | Session 置顶功能（pinned_at + PinSession RPC） | ✅ 2026-05-24 |
| M5-5 | Session 导出功能（Markdown/JSON） | ✅ 2026-05-24 |
| M5-6 | 消息搜索功能（全文检索） | ✅ |
| M5-7 | session_runs / session_run_steps 编排记录 | 🟡 M55 runs ✅ · steps ❌ |
| M5-8 | session_participants Team 参与者 | 🟡 读时聚合 + List RPC + Team Tab |
| M5-9 | session_trace_spans 完整追踪链路 | 待实现 |
| M5-10 | session_context_snapshots Context 趋势 | 待实现 |
| M5-11 | session_model_summaries 多模型分布 | 待实现 |
| M5-12 | 桥接 trpc `session.Service` 接口 | 待实现 |
| M5-13 | 多后端支持（Redis/PG） | 待实现 |
| M5-14 | 内置压缩迁移到 trpc 框架 | 待实现 |

---

## 十、优化内容与后期开发

### 10.1 代码优化（当前可做）

| # | 优化项 | 说明 | 优先级 |
|---|--------|------|--------|
| O1 | `session_repo_summaries.go` 错误处理 | 将 `errors.New` 替换为 `kerrors.BadRequest`/`kerrors.InternalServer`，对齐 §10 原则 10 | ✅ 2026-05-21 |
| P1 | 消息加载 | `ListMessagesBySession(limit,offset)` + `CountMessagesBySession`；Timeline `ListMessagesRecent`（cap `TimelineMessageMaxFetch`）；压缩 `ListMessagesAfterTurn`；取消 `ListMessagesByStatus` | ✅ 2026-05-21 |
| P3 | 批量 ids | `ListSessionsByIDs` + biz `loadBatchCandidates` 一次查询 | ✅ 2026-05-21 |
| O2 | Timeline 超长会话 | SQL UNION 分页；全量按 COUNT 无 2000 cap | ✅ 2026-05-24 |
| O3 | Timeline 工具/Skill 拉取上限 | `timelineInvocationLimit(q)`：默认 100、最大 500 | ✅ 2026-05-21 |
| O4 | AppendChatTurn 事务内两次查询 | `maxMessageTurnTx` + session 查询可合并为一次 | P3 |
| O5 | 压缩防抖策略可配置化 | Agent `L0CompressMinGapSec` · `compress_policy.go` | ✅ 2026-05-24 |
| O6 | SessionCompressor 压缩模型选择 | 当前 fallback 逻辑分散，应统一为策略模式 | P3 |

### 10.2 功能开发（按优先级排序）

| # | 功能 | 需求文档 | 设计要点 | 优先级 |
|---|------|----------|----------|--------|
| F1 | Session 置顶 | §9 Phase 1 | `pinned_at` + Pin/Unpin + 前端 | ✅ |
| F2 | Session 导出 | §9 Phase 2 | ExportSession RPC Markdown/JSON | ✅ |
| F3 | 消息搜索 | §9 Phase 3 | SearchSessionMessages FTS/LIKE | ✅ |
| F4 | session_runs 编排记录 | §4.3 | M55 生命周期 + ListSessionRuns；编排 schema 待对齐 | 🟡 |
| F5 | session_run_steps 步骤记录 | §4.4 | 新表 + step 写入 + List RPC | 待办 |
| F6 | session_participants | §4.2 | 读时 Sync + Team Tab；增量写待办 | 🟡 |
| F7 | session_trace_spans | §4.6 | 完整追踪链路 + parent_span_id 树 + Trace API | P3 |
| F8 | session_context_snapshots | §5.4 | Context ratio 趋势数据 + 快照 API | P3 |
| F9 | session_model_summaries | §4.7 | 多模型分布汇总 + 模型切换历史 | P3 |
| F10 | trpc session.Service 适配器 | §12.1 | `internal/session/trpc/service.go` 桥接 | P3 |
| F11 | Event 分页 | §12.2 | ListEvents 分页查询 | P3 |
| F12 | Session Track | §12.3 | Track(sessionID, key, value) | P3 |
| F13 | Session Ingestor | §12.4 | Session 完成后自动摄入外部记忆平台 | P4 |
| F14 | 多后端支持 | §12.5 | Redis/PG/MySQL/ClickHouse | P4 |
| F15 | 前端 Trace 链路页 | §7.4 | 树形/瀑布视图 | P3 |
| F16 | 前端 Context 趋势线 | §7.6 | context ratio 趋势可视化 | P3 |
| F17 | 前端 Team Session 专属展示 | §7.5 | Participants Panel ✅ · Handoff Badge 待办 | 🟡 |

### 10.3 开发阶段建议

**Phase 1（近期优化）** — ✅ 已完成：
- O1 · O5 · F1 · F2 · O2 Timeline UNION

**Phase 2（编排增强）** — 🟡 进行中：
- F4: session_runs 列表 ✅（M55）
- F5: session_run_steps 待办
- F6: session_participants 部分 ✅
- F17: Team UI 部分 ✅

**Phase 3（可观测性）**：
- F7: session_trace_spans
- F8: session_context_snapshots
- F9: session_model_summaries
- F15: 前端 Trace 链路页
- F16: 前端 Context 趋势线

**Phase 4（导出与搜索）**：
- F2: Session 导出
- F3: 消息搜索

**Phase 5（框架对齐）**：
- F10-F14: trpc session.Service 适配器 + 多后端
