# L0 上下文压缩优化 — 设计

> **需求**：[`L0-compression.md`](./L0-compression.md) · **开发计划**：[`L0-compression-development.md`](./L0-compression-development.md)
>
> **⚠️ 变更通知（2026-07-20）**：L1 MicroCompact 已全链路移除（§1.2.2 设计、级联第 5 步、`micro_compact_*` 配置列）。移除原因：`loadCompressBody` 仅保留 user/assistant 消息，工具消息过滤逻辑恒不触发，功能从未生效。压缩级联现为两级：L2 Memory Compact → L3 LLM。下文相关章节仅作历史记录保留。详见 `docs/superpowers/plans/2026-07-20-session-compression-hardening.md`。

---

## 1. 阶段一设计：工程补强

### 1.1 入口管控：工具结果持久化

#### 1.1.1 数据模型

**新增表：`tool_result_blobs`**

```sql
CREATE TABLE IF NOT EXISTS tool_result_blobs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  turn_number INTEGER NOT NULL,
  tool_name TEXT NOT NULL,
  tool_args_summary TEXT NOT NULL DEFAULT '',
  full_content TEXT NOT NULL,
  content_size_chars INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE INDEX idx_tool_result_blobs_session_turn
  ON tool_result_blobs(session_id, turn_number);
```

**新增表：`tool_result_replacements`**（替换决策冻结）

```sql
CREATE TABLE IF NOT EXISTS tool_result_replacements (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  message_id TEXT NOT NULL,
  result_blob_id TEXT NOT NULL,
  preview_text TEXT NOT NULL,
  replaced_at INTEGER NOT NULL,
  FOREIGN KEY (session_id) REFERENCES sessions(id),
  FOREIGN KEY (result_blob_id) REFERENCES tool_result_blobs(id)
);

CREATE UNIQUE INDEX idx_tool_result_replacements_message
  ON tool_result_replacements(session_id, message_id);
```

#### 1.1.2 端口接口

```go
type ToolResultBlobRepository interface {
    SaveBlob(ctx context.Context, blob *biz.ToolResultBlob) error
    GetBlob(ctx context.Context, id string) (*biz.ToolResultBlob, error)
    ListBlobsBySession(ctx context.Context, sessionID string, fromTurn int) ([]*biz.ToolResultBlob, error)
}

type ToolResultReplacementRepository interface {
    SaveReplacement(ctx context.Context, r *biz.ToolResultReplacement) error
    GetReplacementByMessage(ctx context.Context, sessionID, messageID string) (*biz.ToolResultReplacement, error)
}
```

#### 1.1.3 入口管控流程

```
工具执行完成，返回结果
    ↓
ToolResultGate.Check(result)
    ├── 单个结果 > 50K 字符？
    │   ├── 是 → 持久化到 tool_result_blobs
    │   │        写入 tool_result_replacements（冻结预览）
    │   │        返回 preview(2KB) + result_blob_id
    │   └── 否 → 继续
    ↓
    ├── 一轮合计 > 200K 字符？
    │   ├── 是 → 按大小排序，持久化最大的几个直到总量 ≤ 200K
    │   │        每个持久化结果写入 tool_result_replacements
    │   │        返回各结果的 preview + result_blob_id
    │   └── 否 → 全量保留
    ↓
后续轮次注入消息历史时
    ↓
ToolResultReplacementRepository.GetReplacementByMessage()
    ├── 有替换记录 → 用冻结的 preview_text（保证字节级一致）
    └── 无替换记录 → 用原始内容
```

#### 1.1.4 ReadToolResult 工具

```go
type ReadToolResultInput struct {
    ResultBlobID string `json:"result_blob_id" jsonschema:"description=持久化的工具结果ID"`
    Offset       int    `json:"offset,omitempty" jsonschema:"description=起始行号，默认0"`
    Limit        int    `json:"limit,omitempty" jsonschema:"description=最大行数，默认100"`
}

type ReadToolResultOutput struct {
    Content      string `json:"content"`
    TotalLines   int    `json:"total_lines"`
    Truncated    bool   `json:"truncated"`
}
```

工具注册走标准 `ToolRegistration` + `builtin_tools_seed.go` 种子流程。

#### 1.1.5 单轮超限治理（用户输入 / 文本附件）

工具结果之外，**用户输入正文**与**文本附件**同样可能单轮超窗口。复用同一 Gate 与 blob 存储，`source` 区分来源（`ToolResultSourceUserInput` / `ToolResultSourceAttachment`，写入 `blob.ToolName` 以与工具结果区分）：

```go
// ToolResultGate.CheckUserInput — 与 Check 同幂等语义（按 sessionID+messageID 查 replacement）
func (g *ToolResultGate) CheckUserInput(ctx context.Context, sessionID, messageID, source, fullContent string) (ToolResultGateResult, error)
```

接入点：

| 接入点 | 位置 | 行为 |
|--------|------|------|
| 用户输入 | `turnPipeline.gateTurnUserInput`（`internal/service/chat_orchestrator_turn_pipeline.go`） | 消息持久化前调用；gate 未配置 / 未超阈值 / 落地失败时透传原文（不阻断对话）；幂等键取 `RootTaskActivityID`（与持久化用户消息 ID 一致），重试复用 replacement |
| 文本附件 | `BuildUserMessageFromArtifacts`（`internal/agent/attachments.go`，`gate UserInputGate` 参数） | 超阈值非 image 附件以 preview 文本 part 替换 File part；image 附件走多模态原路径不拦截 |

落地后 LLM 可通过 `read_tool_result` 按 blob_id 分块读取全文（同工具结果路径）。第二道防线为 LLM 调用前裁剪：`enable_token_tailoring` 在 `context_window_k > 0` 且未显式配置时默认启用（见 [9-provider.design.md §6.3](../9-provider.design.md)）。

#### 1.1.6 可压缩工具白名单

```go
var compactableTools = map[string]bool{
    "read_file":     true,
    "search_files":  true,
    "list_directory": true,
    "execute_command": true,
    "grep":          true,
    "glob":          true,
    "web_search":    true,
    "web_fetch":     true,
}
```

---

### 1.2 三层代价递进压缩

#### 1.2.1 压缩判断链（升级后）

```
runCompress() 判断链
    ├── 1. sessionCompressEnabled? → 否 → 跳过
    ├── 2. contextUsedRatio < threshold? → 是 → 跳过
    ├── 3. 防抖检查 → 跳过/继续
    ├── 4. 消息数量检查 → 不足 → 跳过
    ↓
    ~~【新增】5. L1 MicroCompact~~ ❌ 已移除（2026-07-20，功能从未生效）
    ~~├── 识别可压缩工具的旧结果（age ≥ min_age_turns）~~
    ~~├── 替换为占位符 [Tool result cleared: <tool_name>(<args_summary>)]~~
    ~~├── 估算压缩后 token 数~~
    ~~├── 仍超阈值 → 升级到 L2~~
    ~~└── 未超 → 执行 L1 压缩，更新快照，返回~~
    ↓
    【新增】6. L2 Memory Compact
    ├── 【新增 2026-07-20】摘要行数 ≥ SummaryMaxRows（默认 3）→ 跳过 L2 强制 L3（LLM 吸收合并，行数归一）
    ├── 读取 L1 工作记忆 + 已提取的记忆事实
    ├── 【新增 2026-08-13】ICS 覆盖分 < 0.5 → 放弃 L2（硬触发升 L3 / 软触发 defer）（§1.6.2）
    ├── 组装为摘要（结构化 Markdown）
    ├── 估算压缩后 token 数（含 reserved_system，§1.6.5）
    ├── 仍超阈值 → 升级到 L3
    └── 未超 → 执行 L2 压缩，更新快照，返回
    ↓
    7. L3 AutoCompact（现有，升级摘要结构）
    ├── 【新增 2026-08-13】工具消息渲染进 transcript（§1.6.4）+ 超窗分块滚动摘要（§1.6.3）
    └── 调 LLM 生成 9 章节摘要，CAS + 事务写入
```

#### 1.2.2 ~~L1 MicroCompact 设计~~ ❌ 已移除（2026-07-20）

> **本节仅作历史记录保留。** 实现从未生效：`loadCompressBody` 仅保留 user/assistant 消息，`role=tool` 过滤恒不触发。代码（`micro_compact.go`、proto 字段、DB 列、前端配置）已全链路删除。

```go
type MicroCompactResult struct {
    CompactedMessageIDs []string
    EstimatedTokenSavings int
    StillOverThreshold   bool
}

func (c *Compressor) runMicroCompact(
    ctx context.Context,
    sess *biz.Session,
    messages []biz.ChatMessage,
    thresholdTokens int,
) (*MicroCompactResult, error)
```

**核心逻辑**：

1. 遍历消息历史，找出 `role=tool` 且 `tool_name ∈ compactableTools` 且 `turn_number ≤ currentTurn - minAgeTurns` 的消息
2. 对每条消息：检查 `tool_result_replacements` 是否已有冻结预览
   - 有 → 跳过（已经压缩过）
   - 无 → 替换为 `[Tool result cleared: <tool_name>(<args_summary>)]`，写入 `tool_result_replacements`
3. 估算压缩后 token 数
4. 返回结果

**不调用 LLM，不改变 CAS 版本号，不触发事务**——L1 是轻量级清理，不产生新摘要。

#### 1.2.3 L2 Memory Compact 设计

```go
type MemoryCompactResult struct {
    SummaryMarkdown    string
    EstimatedTokens    int
    StillOverThreshold bool
}

func (c *Compressor) runMemoryCompact(
    ctx context.Context,
    sess *biz.Session,
    messages []biz.ChatMessage,
    thresholdTokens int,
) (*MemoryCompactResult, error)
```

**核心逻辑**：

1. 读取 L1 工作记忆字段（`memory_working_memory` 表中该 session 的活跃条目）
2. 读取已提取的记忆事实（`memory_facts` 表中该 session 的 active 条目）
3. 组装为结构化 Markdown：

```markdown
## Session Memory Summary

### Working Context
{L1 工作记忆字段，按 key-value 列出}

### Key Facts
{记忆事实，按 static/dynamic/episodic 分组列出}

### Recent Decisions
{最近 N 条记忆事实，按时间倒序}
```

4. 估算 token 数
5. 若仍超阈值 → 返回 `StillOverThreshold=true`，升级到 L3

**L2 也不调用 LLM**——复用已有记忆数据，零额外 API 成本。

#### 1.2.4 L3 AutoCompact 升级设计

现有 `compressor.go` 的 `runCompress` 逻辑不变，仅升级 `compress/prompt.go` 的系统提示词：

> **⚠️ 提示词已升级至 v3（2026-08-13）**：Section 6 不再无上限逐字保留，仅保留最近 30 条逐字 + 更早的压缩为主题列表（§1.6.6）。下方摘录为 v2 历史版本。

```go
const UpgradedSystemPrompt = `You consolidate conversation history for downstream LLM turns.
Output Markdown with exactly these 9 sections:

## 1. User Intent & Goals
What the user wants to accomplish.

## 2. Key Technical Concepts
Technologies, frameworks, patterns discussed.

## 3. Files & Code Involved
File paths, function names, key variables, API endpoints.

## 4. Errors & Fixes
Error messages, root causes, solutions applied.

## 5. Problem-Solving Process
Steps taken, approaches tried, what worked and what didn't.

## 6. All User Messages (verbatim)
List EVERY user message verbatim. Do NOT summarize or omit any.

## 7. Constraints & Preferences
Language, style, forbidden actions, naming conventions.

## 8. Pending Tasks & Open Questions
What remains to be done, what needs clarification.

## 9. Current Work State
Last file edited, incomplete changes, immediate next step.

Rules:
- Output Markdown only.
- Do not invent facts. Mark uncertainties as '待澄清'.
- Preserve actionable specifics (file paths, commands, error messages, tool names).
- Section 6 is MANDATORY: every user message must appear verbatim.`
```

---

### 1.3 LLM 压缩响应缓存

#### 1.3.1 问题分析

当前 L3 AutoCompact 调用 `compress.LLMService.Compress()` 执行 LLM 摘要生成，以下场景会产生重复调用：

| 场景 | 原因 | 频率 |
|------|------|------|
| CAS 版本冲突重试 | `TryIncrementCompressVersion` 发现版本已变，放弃当前压缩；但下次触发时消息序列可能未变，LLM 调用重复 | 中 |
| 并发触发压缩 | `inFlight` 阻止同一 session 并发执行，但不同入口（AfterNativeTurn + BeforeDurableTurn）可能先后触发 | 低 |
| L1/L2 升级到 L3 | L1/L2 估算后仍超阈值，升级到 L3 重新调用 LLM；此时消息序列与之前 L1/L2 阶段相同 | 中 |
| 手动压缩 API | 用户手动触发时，可能与自动压缩的消息序列重叠 | 低 |

**核心矛盾**：LLM 调用是压缩流程中最昂贵的操作（token 成本 + 延迟），但项目已有四层缓存（Agent 构建、工具结果、Graph 节点、文件视图），唯独 LLM 调用本身没有缓存。

#### 1.3.2 设计方案：装饰器模式

在 `compress.Compressor` 接口之上增加缓存装饰器，不修改现有 `LLMService` 实现：

```
compressor.go (调用方)
    ↓
CachingCompressor (装饰器)
    ├── 缓存命中 → 直接返回 Result（零 LLM 调用）
    └── 缓存未命中 → 委托给 LLMService.Compress() → 写入缓存 → 返回 Result
```

**选择装饰器而非修改 LLMService 的理由**：
1. 符合开闭原则，不修改已验证的核心压缩逻辑
2. 缓存可独立开关，不影响 LLMService 的行为
3. 未来可替换为分布式缓存实现，只需替换装饰器

#### 1.3.3 缓存键设计

```go
func CompressCacheKey(sessionID string, req compress.Request) string {
    h := sha256.New()
    h.Write([]byte(sessionID))
    h.Write([]byte(req.Transcript))
    h.Write([]byte(req.PriorSummary))
    h.Write([]byte(req.Provider))
    h.Write([]byte(req.Model))
    h.Write([]byte(req.SystemPrompt))
    h.Write([]byte(PromptVersion))
    return hex.EncodeToString(h.Sum(nil))
}
```

**缓存键组成**：

| 字段 | 理由 |
|------|------|
| `sessionID` | 隔离不同会话的缓存 |
| `Transcript` | 消息序列是摘要的核心输入 |
| `PriorSummary` | 前次摘要影响输出 |
| `Provider + Model` | 不同模型生成不同摘要 |
| `SystemPrompt` | 提示词变更导致输出不同 |
| `PromptVersion` | 提示词版本升级时自动失效 |

**不包含 `compressVersion`**：`compressVersion` 是 CAS 乐观锁版本号，每次压缩事务成功后递增。缓存键基于输入确定性，版本推进后旧条目通过 TTL 自然淘汰，无需在键中编码版本号。

#### 1.3.4 缓存存储设计

进程内 LRU + TTL 双淘汰，与现有 `internal/agent/cache.go`（Agent 构建缓存）模式一致：

```go
type CompressCache struct {
    mu      sync.Mutex
    cap     int
    ttl     time.Duration
    items   map[string]*compressCacheEntry
    lruList *list.List
}

type compressCacheEntry struct {
    key       string
    result    compress.Result
    createdAt time.Time
    element   *list.Element
}
```

**参数**：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `cap` | 256 | 最大缓存条目数 |
| `ttl` | 10 分钟 | 缓存过期时间 |

**淘汰策略**：
1. 读取时检查 TTL，过期则删除并返回未命中
2. 写入时若超容量，先淘汰过期条目，再淘汰 LRU 尾部
3. `compressVersion` 推进时无需主动清理（TTL 自然淘汰）

#### 1.3.5 装饰器实现

```go
type CachingCompressor struct {
    inner compress.Compressor
    cache *CompressCache
    lg    loggateway.Logger
}

func NewCachingCompressor(inner compress.Compressor, cache *CompressCache, lg loggateway.Logger) *CachingCompressor {
    return &CachingCompressor{inner: inner, cache: cache, lg: lg}
}

func (c *CachingCompressor) Compress(ctx context.Context, req compress.Request) (compress.Result, error) {
    sessionID := SessionIDFromCtx(ctx)
    key := CompressCacheKey(sessionID, req)
    if hit, ok := c.cache.Get(key); ok {
        c.lg.Info("L0 压缩缓存命中", loggateway.StepID("compress.cache_hit"), loggateway.SessionID(sessionID))
        return hit, nil
    }
    result, err := c.inner.Compress(ctx, req)
    if err != nil {
        return result, err
    }
    c.cache.Put(key, result)
    return result, nil
}
```

**sessionID 传递**：通过 `context.Context` 传递，在 `compressor.go` 的 `runCompress` 中注入：

```go
ctx = context.WithValue(ctx, compressCtxSessionIDKey, sessionID)
```

#### 1.3.6 与压缩流程的集成

在 `compressor.go` 的 `runCompress` 方法中，L3 分支调用 `c.Compress.Compress()` 时，`c.Compress` 已被 `CachingCompressor` 装饰，无需修改调用方代码。

**Wire 装配变更**：

```go
// 之前
wire.Bind(new(compress.Compressor), new(*compress.LLMService))

// 之后
func provideCompressor(svc *compress.LLMService, cache *compress.CompressCache, lg loggateway.Logger) compress.Compressor {
    return compress.NewCachingCompressor(svc, cache, lg)
}
```

#### 1.3.7 缓存失效场景

| 场景 | 处理方式 |
|------|---------|
| PromptVersion 升级 | 缓存键包含 PromptVersion，自动失效 |
| Provider/Model 变更 | 缓存键包含 Provider+Model，自动失效 |
| 消息序列变化 | 缓存键包含 Transcript 哈希，自动失效 |
| 压缩版本推进 | TTL 自然淘汰（10 分钟） |
| 手动压缩带 preserveInstruction | SystemPrompt 不同，缓存键不同，不命中 |

#### 1.3.8 可观测性

| 事件 | 日志 | StepID |
|------|------|--------|
| 缓存命中 | `lg.Info("L0 压缩缓存命中")` | `compress.cache_hit` |
| 缓存未命中 | `lg.Info("L0 压缩缓存未命中")` | `compress.cache_miss` |
| 缓存淘汰 | `lg.Debug("L0 压缩缓存淘汰")` | `compress.cache_evict` |

压缩完成通知（`publishCompressionNotice`）的 metadata 中增加 `cache_hit: bool` 字段。

---

### 1.4 手动压缩 API

#### 1.4.1 Proto 定义

```protobuf
message CompactSessionRequest {
  string session_id = 1;
  string user_id = 2;
  string preserve_instruction = 3; // 可选：自定义保留指令
}

message CompactSessionResponse {
  bool compacted = 1;
  int32 from_turn = 2;
  int32 to_turn = 3;
  int32 estimated_tokens_before = 4;
  int32 estimated_tokens_after = 5;
  string compression_level = 6; // "micro" / "memory" / "auto"
}
```

#### 1.4.2 Service 层映射

```go
func (s *ChatService) CompactSession(ctx context.Context, req *pb.CompactSessionRequest) (*pb.CompactSessionResponse, error) {
    // 1. 鉴权 + 获取 session
    // 2. 调用 Compressor.RunManualCompact(ctx, sessionID, preserveInstruction)
    // 3. 返回压缩结果
}
```

#### 1.4.3 前端交互

- 聊天工具栏增加"压缩会话"按钮（仅在 `context_used_ratio > 0.4` 时显示）
- 点击后弹出对话框，可选填"重点保留"指令
- 压缩过程中显示 toast "正在压缩会话上下文..."
- 压缩完成后 toast "会话上下文已压缩，节省 XX% token"
- 现有 `system.session.compress` Envelope 已支持推送，前端需增加 toast 渲染

---

### 1.5 配置扩展

`agent_runtime_settings` 新增字段：

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN tool_result_max_size_chars INTEGER NOT NULL DEFAULT 50000;
ALTER TABLE agent_runtime_settings ADD COLUMN tool_result_max_per_message_chars INTEGER NOT NULL DEFAULT 200000;
ALTER TABLE agent_runtime_settings ADD COLUMN tool_result_preview_size_chars INTEGER NOT NULL DEFAULT 2000;
-- ❌ 已移除（2026-07-20）：micro_compact_enabled / micro_compact_min_age_turns
--    列已由迁移 20261107_drop_micro_compact 删除
ALTER TABLE agent_runtime_settings ADD COLUMN memory_compact_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN memory_compact_min_tokens INTEGER NOT NULL DEFAULT 10000;
ALTER TABLE agent_runtime_settings ADD COLUMN memory_compact_max_tokens INTEGER NOT NULL DEFAULT 40000;
ALTER TABLE agent_runtime_settings ADD COLUMN compress_llm_cache_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN compress_llm_cache_max_entries INTEGER NOT NULL DEFAULT 256;
ALTER TABLE agent_runtime_settings ADD COLUMN compress_llm_cache_ttl_sec INTEGER NOT NULL DEFAULT 600;
```

---

### 1.6 系统评审加固（2026-08-13）

> 背景：对压缩子系统做系统层面深入评审（对标业界产品 + 前沿论文）后发现 1 个致命断链 + 5 个质量/精度缺陷。本节记录最终方案的设计；任务清单见 `L0-compression-development.md`「评审修复轮」。

#### 1.6.1 F0：TurnNumber 断链修复（致命）

**问题**：`messages` 表删除后，活动流适配器（`ActivityMessageReader`）只携带 `TurnID`、不填 `TurnNumber`，所有消息 `TurnNumber=0`。压缩体过滤器 `m.TurnNumber > maxSummarized` 恒不匹配 → **压缩体恒为空，整个压缩子系统在生产空转**。

**方案**：`internal/biz/session/activity_message_adapter.go` 新增 `synthesizeTurnNumbers`——按时间序对 `TurnID` 首次出现顺序分配递增序号（同 TurnID 共享同一序号）。activities 是 append-only，序号分配在多次压缩间稳定：已摘要前缀的轮次号永不错位。无 `TurnID` 的行保持 0，沿用既有行为排除出压缩体。

#### 1.6.2 F1：MemoryCompact ICS 质量门

**问题**：`tryMemoryCompact` 只要有事实就用记忆事实替换整个对话体——稀疏事实集（如仅 1 条 intent）会生成近乎空的摘要并持久化，造成**不可恢复的上下文丢失**。

**方案**：引入信息覆盖分（ICS）门控，`memory_compact.go`：

- `compactCoverage` 6 维加权：intent 0.25 / state 0.20 / decisions(≥2) 0.20 / files(≥2) 0.15 / facts(≥3) 0.10 / pending 0.10（计数维半分给 1 条）
- 阈值 `memoryCompactMinICS = 0.5`：低于门控 → 放弃 L2，硬触发升级 L3 / 软触发 defer，并打 Info 日志（含 ics/min_ics/fact_count）

#### 1.6.3 F2：L3 分块滚动摘要

**问题**：长会话 transcript 可能超出压缩模型自身上下文窗口——压缩调用因 context overflow 失败（确定性错误还会触发 sticky 抑制，永久锁死）。

**方案**：`snapshot.go` 新增 `splitMessagesForCompress`，按渲染后 rune 数切分（`compressChunkMaxRunes = 24000`，包级 var 供测试覆盖）；`compressor.go` `llmSummarize` 逐块调用 LLM，每块把上一块产出的摘要作为 `PriorSummary` 滚动吸收，最终单块产出覆盖全区间的合并摘要。任一块失败即走既有错误分类/抑制路径，不写部分结果。

#### 1.6.4 F3：工具消息进入 transcript

**问题**：`loadCompressBody` 只捞 user/assistant，压缩模型完全看不到工具调用与结果——摘要丢失「做了什么、查到什么」的关键决策依据（原 L1 MicroCompact 即因此从未生效而移除）。

**方案**：

- `loadCompressBody` 增返回 `toolBody`：压缩轮次范围内 `(maxSummarized, cutoffTurn]` 的 `role=tool` 消息
- `snapshot.go` `renderCompressMessage` 将工具消息渲染为 `TOOL(name): body` 并入时间线；仅 `summary`（默认）策略渲染，`hybrid`/`drop_tool_results`/`drop_oldest` 与 `filterMessagesForTruncateStrategy` 过滤语义保持一致（`compressStrategyRendersToolResults`）
- 截断防溢出：工具结果单条 ≤ `compressToolResultMaxRunes`(1000) runes，user/assistant/system 单条 ≤ `compressMessageMaxRunes`(8000) runes，截断追加 `…[truncated]` 标记

#### 1.6.5 F4：压缩后估值计入保留系统 token

**问题**：`estimateCompactedPromptTokens` 只算「摘要 + tail」，漏掉 reserved_system（coding profile 下 15000 tokens）→ 压缩后估值系统性偏低，L2→L3 升级决策失真（该升的没升）。

**方案**：`estimateCompactedPromptTokens(mergedSummary, tail, reservedSystem)` 增加 `reservedSystem` 参数，调用方传入 `calculateReservedSystemPolicy(p, p.Profile.ToolsProfile)` 的估值。

#### 1.6.6 F5：Section 6 用户消息上限 + PromptVersion v3

**问题**：9 章节提示词 v2 要求 Section 6 逐字保留**每一条**用户消息——长会话中摘要本身膨胀，减量守卫（≥80% 丢弃）反而更容易丢弃结果。

**方案**：`compress/prompt.go` 升级 `PromptVersion = "v3"`：Section 6 仅逐字保留**最近 30 条**用户消息，更早的压缩为编号主题列表（禁止编造）。缓存键含 PromptVersion，升级自动失效旧缓存。

#### 1.6.7 F6：ToolsProfile 驱动保留 token（SnapshotMode-as-profile bug）

**问题**：`softTriggerTokensPolicy` 等策略函数把 `SnapshotMode`（如 `"summary"`）当 profile 传给 `profileBasedDefaultPolicy` → switch 恒落 default → 保留 token 恒为 8000：coding agent（应 15000）被低估→压缩触发偏晚，chat_only agent（应 4000）被高估→触发偏早。

**方案**：`CompressProfile` 新增 `ToolsProfile` 字段，`CompressPolicyFromAgent` 从 `ag.Settings.ToolsProfile` 填充；全部 `*Policy` 触发阈值函数改用 `p.Profile.ToolsProfile`。`SnapshotMode` 保留在 `CompressModelConfig` 仅作开关语义（`"off"` 且未显式启用 compaction 时禁用压缩）。

---

### 1.7 复审加固（2026-08-13，第二轮深入检查）

> 背景：F0-F6 落地后对压缩子系统做第二轮深入复审，发现 5 个实现级缺陷（G1-G5）+ 1 项 I/O 技术债（G6）。全部按 TDD 实施，任务清单见 `L0-compression-development.md`「复审加固轮」。

#### 1.7.1 G1：ctx 取消静默中止

**问题**：`llmCallWithRetry` 对 ctx 取消显式返回 `fail=none`（"ctx 取消不是压缩失败"），但 `llmSummarize` 分块循环把 `md==""` 一律按瞬态失败处理——进程关闭/压缩超时（8 分钟 detach ctx）会被误记入失败抑制；hybrid 策略下更糟：取消时仍写兜底标记并推进 `to_turn`（写入事务在 detach ctx 上照常提交），**未摘要内容被永久跳过**。

**方案**：分块循环显式区分 `fail==none && md==""`（唯一对应 ctx 取消）→ 直接返回 `level=none, fail=none`：不记抑制、不写兜底标记、不推进覆盖。

#### 1.7.2 G2：减量守卫分母计入被吸收的历史摘要

**问题**：滚动吸收场景下 LLM 产出覆盖 `(prior + body)`，但减量守卫只对比本次 body——成熟长会话（大历史摘要 + 小增量 body）必然误杀：丢弃有效结果 + 误记瞬态失败，上下文无限增长直至硬截断。

**方案**：守卫分母改为 `EstimateTokensFromChars(totalRunes + runes(priorMerged))`（估算器为线性 chars→tokens，合并 rune 数与分别估算等价）。

#### 1.7.3 G3：cacheHit 聚合全块

**问题**：分块循环 `cacheHit = hit` 只保留末块结果——首块未命中、后续块命中时上报 `cache_hit=true`，元数据谎称整次压缩零 LLM 调用，污染监控/计费口径。

**方案**：`cacheHit` 初值 `true`，逐块 `cacheHit = cacheHit && hit` 聚合，仅全块命中才为 true；hybrid 兜底标记路径显式置 false（兜底标记非真实缓存结果）。

#### 1.7.4 G4：options_json 工具名转义

**问题**：`buildOptionsJSON` 用字符串拼接 `{"tool_name":"<name>"}`——ToolName 含引号/反斜杠/控制字符时产出非法 JSON，前端解析失败静默丢失工具卡片。

**方案**：改 `json.Marshal(map[string]string{"tool_name": name})`，失败返回空串（与无工具名语义一致）。

#### 1.7.5 G5：CAS 冲突/幂等命中不得记假成功

**问题**：`executeCompression` 在 CAS 冲突（并发压缩已抢先写入）与幂等命中（同区间摘要已存在）时返回 `nil`，`runCompress` 据此 `suppress.clear` + 打「压缩完成」流程日志——未真实写入却按成功处理，误清既有失败退避记录，误导后续抑制决策。

**方案**：`executeCompression` 改返回 `(wrote bool, err error)`，CAS 冲突/幂等命中均 `wrote=false`；`runCompress` 在 `!wrote` 时直接返回 nil（保留抑制、不打成功日志），仅真实写入才 `suppress.clear` + LogDone。

#### 1.7.6 G6（技术债）：压缩读取路径全量加载 Activity

**问题**：`ActivityMessageReader.ListMessagesAfterTurn` 无 `afterTurn` 下推——每次压缩（含每次软触发评估）都 `ListBySession` 全量读取并转换会话全部 Activity，长会话成本随历史线性增长。

**处置**：本轮只登记不改行为（`TECH-DEBT(COMPRESS-IO)` 注释 + 开发计划差距清单）。修复方向：`ActivityLister` 增加按 turn 范围/游标的分页查询（需 TurnID 单调或可排序映射），或 `ListBySession` 内部服务端过滤。

---

## 2. 阶段二设计：记忆演化

### 2.1 记忆操作语义化

#### 2.1.1 数据模型扩展

`memory_facts` 表新增字段：

```sql
ALTER TABLE memory_facts ADD COLUMN operation_type TEXT NOT NULL DEFAULT 'add';
  -- add / update / delete / merge / noop
ALTER TABLE memory_facts ADD COLUMN scope TEXT NOT NULL DEFAULT 'dynamic';
  -- static / dynamic / episodic
ALTER TABLE memory_facts ADD COLUMN valid_from INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memory_facts ADD COLUMN valid_until INTEGER;
  -- NULL = 永久有效
ALTER TABLE memory_facts ADD COLUMN decay_rate REAL NOT NULL DEFAULT 0.0;
  -- dynamic 事实的衰减速率
ALTER TABLE memory_facts ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE memory_facts ADD COLUMN prev_version_id TEXT;
  -- 指向上一版本 fact 的 ID，形成版本链
ALTER TABLE memory_facts ADD COLUMN merged_from_ids TEXT;
  -- JSON 数组，MERGE 操作的来源 fact IDs
ALTER TABLE memory_facts ADD COLUMN audit_reason TEXT;
  -- 操作原因
```

**新增表：`memory_fact_audit_log`**

```sql
CREATE TABLE IF NOT EXISTS memory_fact_audit_log (
  id TEXT PRIMARY KEY,
  fact_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  operation_type TEXT NOT NULL,
  before_value TEXT,
  after_value TEXT,
  trigger_source TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
```

#### 2.1.2 双阶段提取提示词升级

在 `compress/memory_extract.go` 的 V2 提示词基础上，增加操作类型判断阶段：

```
Stage 1: Operation Classification
Given the new conversation excerpt and existing memory facts, classify each fact extraction as one of:
- ADD: New information not in memory
- UPDATE: Information that contradicts or refines an existing fact
- DELETE: Information that invalidates an existing fact
- MERGE: Multiple existing facts can be combined into a more concise one
- NOOP: No memory change needed

Output JSON: {"operations": [{"type": "UPDATE", "target_fact_id": "...", "new_statement": "...", "reason": "..."}]}

Stage 2: Execute Operations (existing logic, extended)
```

#### 2.1.3 作用域分类规则

| 作用域 | 判断规则 | 衰减策略 |
|--------|---------|---------|
| `static` | 用户偏好、项目约定、技术栈选择、团队规范 | 不衰减，仅 UPDATE/DELETE 时变更 |
| `dynamic` | 当前任务、调试状态、临时决策、进行中的工作 | 按 `decay_rate` 衰减，半衰期默认 24h |
| `episodic` | 具体事件、某次操作的结果、某次对话的结论 | 保留 7 天后自动归档 |

LLM 在提取时自动判断作用域，规则嵌入提示词。

### 2.2 动态链接

#### 2.2.1 数据模型

`memory_facts` 表新增字段：

```sql
ALTER TABLE memory_facts ADD COLUMN link_ids TEXT;
  -- JSON 数组: [{"id": "fact_xxx", "relation": "elaborates"}, ...]
```

#### 2.2.2 链接建立流程

```
新记忆写入
    ↓
embedding 检索 top-5 最相关的已有记忆
    ↓
LLM 判断关系类型
    ├── contradicts → 标记为矛盾，提示 UPDATE/DELETE
    ├── elaborates  → 建立双向链接
    ├── depends_on  → 建立双向链接
    └── unrelated   → 不建链接
    ↓
写入 link_ids（两端都更新）
```

#### 2.2.3 矛盾处理

```
contradicts 关系建立
    ↓
自动生成 UPDATE/DELETE 候选
    ↓
需要用户确认（通过 Memory Center 或 Cascade 流程）
    ↓
确认后执行 UPDATE/DELETE，记录审计日志
```

---

## 3. 阶段三设计：Agent 自主压缩

### 3.1 CompactContext 工具

```go
type CompactContextInput struct {
    Reason       string   `json:"reason" jsonschema:"description=Why you want to compact the context"`
    PreserveKeys []string `json:"preserve_keys" jsonschema:"description=Key information categories to preserve in the summary,enum=user_intent,enum=file_changes,enum=errors,enum=decisions,enum=constraints"`
}

type CompactContextOutput struct {
    Compacted    bool   `json:"compacted"`
    TokensBefore int    `json:"tokens_before"`
    TokensAfter  int    `json:"tokens_after"`
    Summary      string `json:"summary_preview"`
}
```

**执行逻辑**：

1. 验证调用者是当前 session 的活跃 Agent
2. 将 `preserve_keys` 注入压缩提示词的"重点保留"段
3. 复用现有 `Compressor.runCompress` 逻辑，但由 Agent 主动触发
4. 完整对话记录保留在 `session_messages` 表中，仅从 Runner 快照中移除
5. 返回压缩结果预览

### 3.2 RecallDetail 工具

```go
type RecallDetailInput struct {
    Query string `json:"query" jsonschema:"description=What information you want to recall from compressed history"`
}

type RecallDetailOutput struct {
    Found    bool   `json:"found"`
    Content  string `json:"content"`
    TurnRange string `json:"turn_range"`
}
```

**执行逻辑**：

1. 从 `session_messages` 表中检索被压缩的消息
2. 用 embedding 做语义匹配，找到与 query 最相关的片段
3. 返回匹配内容 + 原始轮次范围

### 3.3 代码骨架提取

```go
type CodeSkeletonInput struct {
    FilePath string `json:"file_path" jsonschema:"description=Path to the source file"`
}

type CodeSkeletonOutput struct {
    Skeleton    string `json:"skeleton"`
    OriginalTokens int `json:"original_tokens"`
    SkeletonTokens int `json:"skeleton_tokens"`
}
```

**依赖**：tree-sitter Go 绑定（`github.com/tree-sitter/go-tree-sitter`）

**骨架保留**：函数签名、类定义、接口定义、类型声明、import 列表、结构体字段定义

**骨架丢弃**：函数体、方法体、注释、空行

---

## 4. 模块关联

### 4.1 受影响的现有模块

| 模块 | 变更类型 | 阶段 |
|------|---------|------|
| `internal/session/compressor.go` | 修改（判断链前置 L1/L2 + 缓存 sessionID 注入） | 一 |
| `internal/compress/prompt.go` | 修改（升级系统提示词） | 一 |
| `internal/compress/cache.go` | 新增（CompressCache + CachingCompressor） | 一 |
| `internal/compress/memory_extract.go` | 修改（双阶段提取） | 二 |
| `internal/tools` | 新增工具 + 修改结果返回路径 | 一/三 |
| `internal/data` | 新增表 + Ent schema | 一/二 |
| `internal/biz` | 新增端口接口 + 模型扩展 | 一/二 |
| `internal/service` | 新增 CompactSession API + Wire 装配变更 | 一 |
| `internal/memory` | 记忆操作语义化 + 动态链接 | 二 |
| `api/` | 新增 proto 定义 | 一 |

### 4.2 依赖方向合规

所有变更遵循 `api → service → biz → data` 依赖方向：

- 新端口接口定义在 `internal/biz`
- 端口实现在 `internal/data`
- Proto 映射在 `internal/service`
- 工具注册在 `internal/tools`
- 框架交互通过 `internal/agent` 桥接
