# Session 管理模块 — 实现设计文档

> 对应需求：`10 session.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

会话历史存储与编排：Session CRUD、Timeline 时间轴、上下文管理、摘要压缩。逐步向 trpc-agent-go `session.Service` 对齐。

核心能力：
- Session 搜索/创建/删除/归档/重命名
- Timeline 时间轴聚合（消息 + 工具调用 + Skill 调用 + MCP 调用）
- 上下文窗口消耗追踪与状态管理
- 异步摘要压缩（SessionCompressor）
- Runner Snapshot 持久化与压缩重写
- Session Summaries 滚动摘要
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
}

message GetSessionRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message UpdateSessionRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string title = 2 [(google.api.field_behavior) = REQUIRED];
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

message GetSessionTimelineRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
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
}

message ListSessionMessagesResponse {
  repeated ChatMessageRow items = 1;
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
  rpc GetSessionTimeline(GetSessionTimelineRequest) returns (SessionTimeline) {
    option (google.api.http) = {get: "/v1/sessions/{id}/timeline"};
  }
  rpc ListSessionMessages(ListSessionMessagesRequest) returns (ListSessionMessagesResponse) {
    option (google.api.http) = {get: "/v1/sessions/{id}/messages"};
  }
}
```

### 2.2 RPC 与 HTTP 路由映射

| RPC | HTTP | 说明 |
|-----|------|------|
| `SearchSessions` | `GET /v1/sessions` | 搜索/列表，支持 owner_type/agent_id/team_id/status/context_status/keyword 筛选 + 分页 |
| `CreateSession` | `POST /v1/sessions` | 创建会话，校验 agent_id 或 team_id 存在性 |
| `GetSession` | `GET /v1/sessions/{id}` | 获取单个会话详情 |
| `UpdateSession` | `PATCH /v1/sessions/{id}` | 重命名会话标题 |
| `DeleteSession` | `DELETE /v1/sessions/{id}` | 软删除（设置 deleted_at + status=deleted） |
| `DeleteSessionsByAgent` | `DELETE /v1/sessions` | 按 agent_id 批量软删除 |
| `ArchiveSession` | `POST /v1/sessions/{id}/archive` | 归档会话（status=archived, archived_at） |
| `GetSessionTimeline` | `GET /v1/sessions/{id}/timeline` | 聚合消息+工具+Skill+MCP 的时间轴 |
| `ListSessionMessages` | `GET /v1/sessions/{id}/messages` | 获取会话消息列表 |

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
    MetadataJSON               string
}

type SessionSearchQuery struct {
    OwnerType     string
    AgentID       string
    TeamID        string
    Status        string
    ContextStatus string
    Keyword       string
    Limit         int
    Offset        int
    Page          int
    PageSize      int
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
    ArchiveSession(ctx context.Context, id string) error
    DeleteSession(ctx context.Context, id string) error
    DeleteSessionsByAgentID(ctx context.Context, agentID string) error
    ListMessagesBySession(ctx context.Context, sessionID string) ([]ChatMessage, error)
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
}
```

### 3.3 Usecase 方法

```go
type SessionUsecase struct {
    sessions SessionRepository
    agents   AgentRepository
    teams    TeamRepository
}

func NewSessionUsecase(sessions SessionRepository, agents AgentRepository, teams TeamRepository) *SessionUsecase

func (uc *SessionUsecase) Search(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
func (uc *SessionUsecase) Get(ctx context.Context, id string) (Session, error)
func (uc *SessionUsecase) Create(ctx context.Context, in Session) (Session, error)
func (uc *SessionUsecase) Rename(ctx context.Context, id, title string) (Session, error)
func (uc *SessionUsecase) Archive(ctx context.Context, id string) error
func (uc *SessionUsecase) Delete(ctx context.Context, id string) error
func (uc *SessionUsecase) DeleteByAgent(ctx context.Context, agentID string) error
func (uc *SessionUsecase) ListMessages(ctx context.Context, sessionID string) ([]ChatMessage, error)
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
func (uc *SessionUsecase) Timeline(ctx context.Context, id string) (SessionTimeline, error)
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
4. 合并所有 items 按 `occurred_at` 升序排序
5. 统计 summary 各类型计数

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
        field.Text("metadata_json").Default("{}"),
    }
}
```

### 4.2 Ent Schema — Message

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

通过 `sessionmemory.EnsureSchema` 创建，不使用 Ent Schema：

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
    out, err := s.uc.Rename(ctx, req.GetId(), req.GetTitle())
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

文件：`web/src/pages/SessionsPage.vue`

布局：
```
┌──────────────────────────────────────────────────┐
│ Session 管理                          [刷新]     │  ← SessionsPageHero
├──────────────────────────────────────────────────┤
│ [当前页会话: 20] [活跃: 15] [平均上下文: 42%]    │  ← SessionsSummaryCards
│ [Token: 125,000]                                 │
├──────────────────────────────────────────────────┤
│ 关键词[___] 类型[▼] 状态[▼] 上下文[▼] [重置][搜索]│  ← SessionsFilterBar
├──────────────────────────────────────────────────┤
│ [选中详情卡片]                                    │  ← SessionsSelectedDetail
│  标题 · Agent · active                           │
│  Context 进度条 · 消息数 · 模型调用 · Token · 费用│
│  [继续会话] [归档]                                │
├──────────────────────────────────────────────────┤
│ 会话 │ 类型/归属 │ 上下文 │ 消耗 │ 时间 │ 状态 │操作│  ← SessionsTableSection
│ ─── │ ──────── │ ────── │ ──── │ ──── │ ──── │── │
│ ... │ Agent    │ ████░░ │ 1.2K │ 4/26 │active│👁📦│
├──────────────────────────────────────────────────┤
│ 共 50 个 Session          [20/页 ▼] < 1 2 3 >   │  ← 分页
└──────────────────────────────────────────────────┘
```

筛选选项：
- `owner_type`: Agent / Team
- `status`: active / running / completed / failed / archived
- `context_status`: normal / warning / critical / exceeded

表格列定义：
| 列 | 字段 | 内容 |
|----|------|------|
| 会话 | title | 标题 + summary/id |
| 类型/归属 | owner_type | Agent/Team chip + ID |
| 上下文 | context_used_ratio | QLinearProgress + 百分比 + context_status |
| 消耗 | total_tokens | Token 数 + model/tool/skill/mcp 调用数 |
| 时间 | last_message_at | 最后活跃 + 创建时间 |
| 状态 | status | QBadge |
| 操作 | id | 查看详情 + 归档 |

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
| M5-4 | 桥接 trpc `session.Service` 接口 | 待实现 |
| M5-5 | 多后端支持（Redis/PG） | 待实现 |
| M5-6 | 内置压缩迁移到 trpc 框架 | 待实现 |
