# Memory 系统优化 — 设计

> **需求**：[`memory-optimization.md`](./memory-optimization.md) · **开发计划**：[`memory-optimization.development.md`](./memory-optimization.development.md)

---

## 1. 阶段一设计：L0 压缩与缓存优化

### 1.1 L0 Snapshot 限流

#### 1.1.1 限流算法

写入决策伪代码：

```
func ShouldWriteL0AssemblySnapshot(mode, usedRatio, lastRatio, lastWriteAt, minInterval, ratioDelta) bool:
    if mode == "off": return false
    if mode == "always" or forceDebug: return true
    if !L0SnapshotEnabled: return false
    if usedRatio < 0.60: return false

    // 阈值穿越强制写入
    if crossedThreshold(lastRatio, usedRatio, 0.80): return true
    // 间隔不足
    if time.Since(lastWriteAt) < minInterval: return false
    // 变化量不足
    if abs(usedRatio - lastRatio) < ratioDelta: return false
    return true
```

#### 1.1.2 segments_summary_json 格式

```json
{
  "system_prompt": { "token_estimate": 8500, "message_count": 1 },
  "l1_memory": { "token_estimate": 1200, "field_count": 5 },
  "l3_recall": { "token_estimate": 800, "fact_count": 3 },
  "l4_graph": { "token_estimate": 300, "entity_count": 2 },
  "session_summary": { "token_estimate": 2000, "from_turn": 1, "to_turn": 15 },
  "history": { "token_estimate": 12000, "turn_count": 8 },
  "tool_results": { "token_estimate": 5000, "result_count": 4 },
  "user_input": { "token_estimate": 150 }
}
```

#### 1.1.3 配置参数

| 配置项 | 字段名 | 默认值 | 说明 |
|--------|--------|--------|------|
| L0 快照开关 | `L0SnapshotEnabled` | true | 新增独立字段 |
| 最小写入间隔 | `l0_snapshot_min_interval` | 300s | session 级变量 |
| Ratio 变化量阈值 | `l0_snapshot_ratio_delta` | 0.10 | session 级变量 |

#### 1.1.4 代码变更

| 文件 | 变更 |
|------|------|
| `ShouldWriteL0AssemblySnapshot` 所在文件 | 增加限流逻辑 |
| session 运行时 | 新增 `lastL0SnapshotWriteAt` / `lastL0SnapshotRatio` 变量 |
| `MemorySnapshotDrawer.vue` | 改为展示聚合统计 |
| 设计文档 | 删除未实现的 Datadog 指标规划（死规划清理） |

---

### 1.2 三层前缀分离 + Prompt Cache

#### 1.2.1 三层前缀结构

| 层 | 内容 | 变化频率 | 预估 Token | 缓存标记 |
|----|------|---------|-----------|---------|
| Layer 1 (静态) | SOUL.md/AGENTS.md + Skill + 角色定义 + staticRuntimeCue | 会话生命周期内不变 | 8K~15K | cache_control |
| Layer 2 (半静态) | SkillGuidance + dynamicRuntimeCue | 任务/轮次切换时变 | 2K~8K | cache_control |
| Layer 3 (动态) | session summary + 历史消息 + 当前输入 + Time + L1 cue (MemoryInject) + L3 facts + L4 graph | 每轮都变 | 动态 | 无 |

Anthropic 请求的 system TextBlock 顺序：

```
[0] Identity+Instr+Skills+Workspace+PostTool+staticRuntimeCue  ← 静态 (RequestProcessor + Hook prepend)
     ↑ breakpoint 1 (静态层末尾)
[1] dynamicRuntimeCue                                        ← 半静态 (Hook prepend)
[2] SkillGuidance                                            ← 半静态 (Hook prepend)
     ↑ breakpoint 2 (半静态层末尾 = TextBlock[2] 末尾)
[3] Time                                                     ← 动态 (ContentParts 分离)
[4] KnowledgeCue                                             ← 动态 (Hook prepend, 含 L3 facts + L4 graph)
[5] MemoryInject                                             ← 动态 (Hook prepend, 含 L1 cue)
```

> **关键约束**：Time 必须放在半静态层之后，否则半静态缓存前缀包含每轮变化的 Time，导致 breakpoint 2 永远无法命中缓存。

#### 1.2.2 Hook Layer 声明机制

```go
type SystemLayer int
const (
    LayerStatic    SystemLayer = iota  // Layer 1
    LayerSemiStatic                     // Layer 2
    LayerDynamic                        // Layer 3
)

type BeforeModelHook struct {
    Priority int
    Layer    SystemLayer  // 新增
    Fn       func(...) error
}
```

现有 Hook 的 Layer 声明：

| Hook | Priority | Layer | 说明 |
|------|----------|-------|------|
| ToolResultGate | 3 | -- | 不注入系统消息 |
| staticRuntimeCue | 4 | LayerStatic | 运行时静态信息（模型名、Provider 等），注入 TextBlock[0] |
| dynamicRuntimeCue | 4 | LayerSemiStatic | 运行时动态信息（工具策略等），注入 TextBlock[1] |
| SkillGuidance | 5 | LayerSemiStatic | Skill 内容半静态 |
| MemoryInject | 5 | LayerDynamic | L1 cue 每轮变化 |
| KnowledgeCue | 6 | LayerDynamic | L3 facts + L4 graph 每轮变化 |

未声明 Layer 的 Hook 默认为 `LayerDynamic`（向后兼容）。

#### 1.2.3 DualBreakpoint 设计

断点预算池（4 个断点上限）：

| 优先级 | 断点位置 | 说明 |
|--------|---------|------|
| P1 (必保) | system 静态层 TextBlock[0] 末尾 | 覆盖 8K~15K tokens |
| P2 (必保) | 最后一个 tool 定义 | 覆盖 2K~8K tokens |
| P3 (重要) | system 半静态层末尾 | 覆盖 2K~8K tokens |
| P4 (可选) | 倒数第二个 assistant message | 覆盖对话历史 |

分配规则：可用断点不足时从 P4 开始淘汰。

框架层新增 Option：

```go
type SystemCacheStrategy int
const (
    CacheLastBlock SystemCacheStrategy = iota
    CacheFirstBlock
    CacheAtBlockIndex
)

func WithSystemCacheStrategy(strategy SystemCacheStrategy, blockIndex int) Option
func WithCacheSystemPromptDualBreakpoint(secondBlockIndex int) Option
```

#### 1.2.4 TimeRequestProcessor 改造

时间信息从 `message.Content` 移到 `message.ContentParts`：

```go
// 修改前：
req.Messages[systemMsgIndex].Content += "\n\n" + timeContent

// 修改后：
req.Messages[systemMsgIndex].ContentParts = append(
    req.Messages[systemMsgIndex].ContentParts,
    model.ContentPart{Type: model.ContentTypeText, Text: &timeContent},
)
```

#### 1.2.5 Anthropic adapter 改造

`convertSystemMessageContent` 改为先输出 Content 为 TextBlock，再输出 ContentParts：

```go
// 修改后：
if msg.Content != "" {
    blocks = append(blocks, &anthropic.TextBlockParam{Text: msg.Content})
}
for _, part := range msg.ContentParts {
    if part.Type == model.ContentTypeText && part.Text != nil {
        blocks = append(blocks, &anthropic.TextBlockParam{Text: *part.Text})
    }
}
```

`applyCacheControlToSystem` 改为按语义标记定位断点：

```go
// 遍历找到 LayerStatic 最后一个 TextBlock = breakpoint 1
// 遍历找到 LayerSemiStatic 最后一个 TextBlock = breakpoint 2
// 不再依赖位置索引
```

#### 1.2.6 Hunyuan 适配器修复

```go
// 修改前：
if len(hMsg.Contents) > 0 {
    hMsg.Content = ""  // 丢弃 Content
}

// 修改后：
if len(hMsg.Contents) > 0 && msg.Content != "" {
    firstPart := ChatCompletionMessageContentParam{
        Type: "text",
        Text: msg.Content,
    }
    hMsg.Contents = append([]ChatCompletionMessageContentParam{firstPart}, hMsg.Contents...)
    hMsg.Content = ""
}
```

#### 1.2.7 压缩重建后 MemoryInject 重执行

在 `rebuildRequestForContextCompaction` 末尾增加 `postRebuildHooks`：

```go
// 压缩重建后，重执行 MemoryInject Hook
for _, hook := range postRebuildHooks {
    if hook.Name == "MemoryInject" {
        // 读取最新 L1/L3/L4 数据
        // 替换快照中旧的 MemoryInject TextBlock
    }
}
```

仅重执行 MemoryInject，不影响其他 Hook 的缓存断点位置。

#### 1.2.8 代码变更

| 文件 | 变更 |
|------|------|
| `pkg/trpc-agent-go/model/anthropic/options.go` | 新增 `WithSystemCacheStrategy` / `WithCacheSystemPromptDualBreakpoint` |
| `pkg/trpc-agent-go/model/anthropic/anthropic.go` | 改造 `applyCacheControlToSystem`，支持 DualBreakpoint + 断点计数校验；改造 `convertSystemMessageContent` 先 Content 后 ContentParts |
| `pkg/trpc-agent-go/model/hunyuan/hunyuan.go` | 修复 ContentParts 处理 |
| `pkg/trpc-agent-go/internal/flow/processor/time.go` | 时间信息写入 ContentParts |
| `pkg/trpc-agent-go/internal/flow/hook.go` | `BeforeModelHook` 新增 `Layer SystemLayer` 字段；Hook 装配器按 Layer 排序 |
| `internal/provider/trpc_llm.go` | 启用 DualBreakpoint 模式 |
| `internal/agent/memory_inject.go` | 重构 `buildRuntimeMemoryCue()`；压缩重建后 MemoryInject 选择性重执行 |
| `internal/agent/runtime_cue_inject.go` | 拆分为 staticRuntimeCue + dynamicRuntimeCue；现有 Hook 声明 Layer |

---

### 1.3 压缩预算动态计算

#### 1.3.1 预算公式

```
reserved_system = identity_tokens + instruction_tokens + runtime_cue_tokens + skills_tokens + tools_tokens
compression_buffer = contextWindow * compression_buffer_ratio  (默认 0.15)
effective_budget = contextWindow - reserved_system - compression_buffer
```

示例（coding profile, 256K 产品窗口）：
- reserved_system = 15K
- compression_buffer = 256K * 0.15 = 38.4K
- effective_budget = 256K - 15K - 38.4K = 202.6K

#### 1.3.2 三级触发阈值

| 阈值 | 公式 | 256K 示例 | 行为 | 现有机制 |
|------|------|----------|------|---------|
| soft_trigger | effective_budget * 0.70 | 141.8K | 后台异步压缩 | AfterNativeTurn + safego.Go |
| hard_trigger | effective_budget * 0.90 | 182.3K | 同步压缩 | BeforeDurableTurn 扩展 |
| emergency | effective_budget | 202.6K | 截断最老消息 | atFullContextUsage |

#### 1.3.3 reserved_system 冷启动 fallback

| ToolsProfile | 默认值 | 理由 |
|-------------|--------|------|
| coding/full | 15000 | 系统提示词+工具 11K~18K |
| research | 12000 | 研究工具集中等 |
| chat_only/minimal | 4000 | 最小工具集 |
| 其他 | 8000 | 通用 fallback |

#### 1.3.4 自适应缓冲区策略

```
监控 soft_trigger 到压缩完成期间的 token 增量：
  - 增量 > compression_buffer * 0.70 → ratio += 0.02（上限 0.25）
  - 连续 5 次增量 < compression_buffer * 0.30 → ratio -= 0.01（下限 0.10）

对话模式检测：
  - tool_call_count / turn_count > 2.0 → 编码模式
  - tool_call_count / turn_count < 0.5 → 聊天模式
```

#### 1.3.5 配置参数

| 配置项 | 字段名 | 默认值 | 范围 |
|--------|--------|--------|------|
| 压缩缓冲区比例 | `compression_buffer_ratio` | 0.15 | 0.10~0.25 |
| 自适应缓冲区 | `compression_buffer_adaptive` | true | -- |
| 软触发比例 | `soft_trigger_ratio` | 0.70 | 0.50~0.80 |
| 硬触发比例 | `hard_trigger_ratio` | 0.90 | 0.80~0.95 |
| 保留尾部轮次 | `compress_keep_turns` | 4 | 2~10 |
| 最小压缩间隔 | `compress_min_gap_sec` | 600 | 60~1800 |

#### 1.3.6 代码变更

| 文件 | 变更 |
|------|------|
| `internal/session/compressor.go` | 新增 `calculateReservedSystem` + `profileBasedDefault` |
| 压缩触发逻辑 | 改用 effective_budget 计算三级阈值 |

---

### 1.4 Level 2 Memory Compact 增强

#### 1.4.1 ICS 评分模型

| 维度 | 权重 | 覆盖源 | 评分规则 |
|------|------|--------|---------|
| 用户意图 | 0.25 | task_goal | 非空=1.0, 空=0.0 |
| 当前状态 | 0.20 | status + fields | 有 status=1.0, 无=0.0 |
| 关键决策 | 0.20 | decision/choice 字段 | >=2=1.0, 1=0.5, 0=0.0 |
| 文件变更 | 0.15 | file/artifact 字段 | >=2=1.0, 1=0.5, 0=0.0 |
| 长期事实 | 0.10 | L3 recall facts | >=3=1.0, 1~2=0.5, 0=0.0 |
| 待办事项 | 0.10 | pending/todo 字段 | >=1=1.0, 0=0.0 |

降级规则：
- ICS >= 0.70 且 压缩比 <= 60% → Level 2
- ICS < 0.70 → 降级到 Level 3
- 压缩比 > 60% → 即使 ICS >= 0.70 也降级

#### 1.4.2 死代码激活

| 层级 | 现有代码 | 操作 |
|------|---------|------|
| Level 1 | `tryMicroCompact` (micro_compact.go) | 激活：接入 BeforeModel hook，补充清除逻辑 |
| Level 2 | `tryMemoryCompact` (memory_compact.go) | 激活+增强：接入 runCompress，分两步扩展数据源 |
| Level 3 | `runCompress` default | 增强：使用默认保留策略（保留意图/决策/文件/错误/待办） |

#### 1.4.3 Level 2 集成

**Step 1**（~30-50 行）：激活现有 L3 版本
- 集成点：`runCompress` 中 strategy 判断之前
- Compressor 已持有 `memoryReader biz.MemoryFactReader`

**Step 2**（~80-120 行）：扩展 L1 数据源
- Compressor 新增 `l1Reader biz.L1AdminReader` 字段
- Wire 注入点更新
- 新增 L1 数据格式化函数

#### 1.4.4 接口变更

```go
// memory_compact.go 新增
type compactCoverage struct {
    HasIntent      bool
    HasState       bool
    DecisionCount  int
    FileCount      int
    FactCount      int
    HasPending     bool
}

func (c compactCoverage) ICS() float64
func gradedScore(count, threshold int) float64
// ~~func shouldUseStructuredCompact(...) bool~~ ❌ 已删除（2026-07-20）：门控从未接线，死代码清理
```

#### 1.4.5 事务安全增强

CAS-事务间隙幂等重入：
- 事务内版本校验：`compressVersion` 在事务内再次检查
- `SyncRunnerSnapshot` 失败后：`snapshot_sync_status` 标记 + BeforeModel 重试
- `AppendChatMessage` 失败降级：事件通知
- `EventBus.Publish` 失败降级：前端轮询

#### 1.4.6 代码变更

| 文件 | 变更 |
|------|------|
| `internal/session/micro_compact.go` | 返回 clearableMsgIDs，补充清除逻辑 |
| `internal/session/memory_compact.go` | 重写 tryMemoryCompact，合并 L1+L3 数据源，增加 ICS 评估 |
| `internal/session/compressor.go` | Level 2 失败后等待 hard_trigger；压缩后强制写入 L0 Snapshot；新增 compressing 标记 + 8 分钟超时自动释放 |
| `internal/compress/prompt.go` | Level 3 使用默认保留策略（用户自定义 preserve_instruction 为阶段四功能） |

#### 1.4.7 压缩进行中标记 + 超时保护

```go
// compressor.go 新增
type Compressor struct {
    // ...
    compressing  atomic.Bool   // 压缩进行中标记
    compressStart time.Time    // 压缩开始时间
    compressTimeout time.Duration // 默认 8 分钟
}

func (c *Compressor) tryStartCompress() bool {
    // 超时自动释放：防止异常中断后标记永远为 true
    if c.compressing.Load() && time.Since(c.compressStart) > c.compressTimeout {
        c.compressing.Store(false)
    }
    if c.compressing.Load() {
        return false // 已有压缩进行中，跳过
    }
    if c.compressing.CompareAndSwap(false, true) {
        c.compressStart = time.Now()
        return true
    }
    return false
}

func (c *Compressor) finishCompress() {
    c.compressing.Store(false)
}
```

#### 1.4.8 前端上下文指示器

状态机：

```
┌─────────┐  soft_trigger  ┌──────────┐  压缩开始  ┌──────────┐
│  正常   │ ──────────────→│ 优化中   │ ──────────→│ 正在优化 │
└─────────┘                └──────────┘            └──────────┘
     ↑                                                  │
     └──────────── 压缩完成 ────────────────┌───────────┘
                                           │
                                     ┌──────────┐
                                     │  已优化  │
                                     └──────────┘
                                           │
                                     下一轮开始 → 正常
```

API：`GET /api/v1/sessions/{id}/compress-status` 返回 `{ status: "normal"|"optimizing"|"optimized"|"compressing" }`

前端组件：`ContextIndicator.vue`，显示在对话输入框上方，状态对应颜色：正常(绿)→优化中(黄)→正在优化(橙)→已优化(蓝)

---

## 2. 阶段二设计：L1 预算与选择性注入

### 2.1 L1 预算硬上限

#### 2.1.1 同步聚合 + DB 事务行锁

```go
func (r *l1FieldRepo) UpsertL1Field(ctx context.Context, in biz.L1FieldInsert) error {
    // 1. 计算 token_estimate
    if in.TokenEstimate == 0 && in.ValueText != "" {
        in.TokenEstimate = max(1, utf8.RuneCountInString(in.ValueText)/2)
    }
    // 2. 开启事务
    tx, err := r.data.RW().Write(ctx).BeginTx(ctx, nil)
    // 3. INSERT/UPDATE 字段（含 token_estimate）
    // 4. 同步聚合 used_tokens
    //    UPDATE memory_l1_tasks SET used_tokens = (
    //      SELECT COALESCE(SUM(token_estimate), 0)
    //      FROM memory_l1_fields WHERE task_id = ? AND pin_to_prompt = 1 AND visibility != 'internal'
    //    ) WHERE id = ?
    // 5. 预算检查
    //    injectLimit = budgetTokens * L1InjectBudgetRatio
    //    if usedTokens > injectLimit → ROLLBACK, return ErrL1Overflow
    return tx.Commit()
}
```

#### 2.1.2 三层过滤链

```
1. visibility 过滤：visibility != 'internal'
2. pin_to_prompt 过滤：pin_to_prompt = true
3. 相关性过滤：
   ├── 字段数 <= 5：全量注入
   └── 字段数 > 5：pinned 始终注入，其余按 updated_at 降序取 top-K (K=min(非pinned, 5))
4. token 预算硬上限：
   ├── L1 注入总 token 不超过 budget_tokens 的 50%
   └── 超出时按 token_estimate 降序截断
```

#### 2.1.3 配置参数

| 配置项 | 字段名 | 默认值 | 说明 |
|--------|--------|--------|------|
| L1 注入预算比例 | `L1InjectBudgetRatio` | 0.50 | budget_tokens 的 N% |
| L1 最大注入字段数 | `L1MaxInjectFields` | 10 | 非 pinned 字段上限 |

### 2.2 field_kind 枚举增强

扩展 `field_kind` 为语义枚举：

```
string / number / boolean / json / reference
/ decision / artifact / progress / constraint
```

`working_memory.write` 工具参数增加 enum 约束：

```go
FieldKind string `jsonschema:"description=Categorize this field,enum=string,enum=number,enum=boolean,enum=json,enum=reference,enum=decision,enum=artifact,enum=progress,enum=constraint"`
```

### 2.3 L1 选择性注入

| 表 | 策略 |
|----|------|
| `memory_l1_field_history` | 可选，`L1HistoryEnabled` 默认 false |
| `memory_l1_schemas` | 可选，仅 Agent 配置 `L1DefaultSchemaID` 时激活 |

---

## 3. 阶段三设计：Episode 结构化与记忆统一

### 3.1 结构化 Episode 双路径

#### 3.1.1 Path A 零成本生成规则

```
title = task_title
goal = task_goal
outcome = status 描述 + 最后一轮 assistant 消息截断 200 字
outcome_summary = status 枚举值映射
  - "completed" → "任务已完成"
  - "cancelled" → "任务被取消（空闲超时）"
  - "failed" → "任务失败"
  - "timeout" → "任务超时"
key_decisions = 分层 fallback（4 层）：
  Layer 0: field_kind = "decision" 的字段
  Layer 1: field_path 模式匹配
  Layer 2: pin_to_prompt=true 且 visibility="prompt" 的前 3 个
  Layer 3: 最近更新的前 5 个 visibility="prompt" 字段（全量兜底）
key_artifacts = 分层 fallback：
  Layer 0: field_kind = "artifact"/"reference"
  Layer 1: field_path 模式匹配 + field_kind="reference"
  Layer 2: field_kind = "reference"
importance = 0.5
confidence = 0.6
episode_kind = "l1_archive_structured"
consolidation_status = "consolidated"
```

#### 3.1.2 Path B LLM 增强触发条件

简化规则（P0）：满足任一即触发

| 信号 | 阈值 |
|------|------|
| importance | >= 0.7 |
| critic_score | >= 0.8 |
| tool_call_count | >= 20 |
| duration_ms | >= 300000 |
| user_mark | star/consolidate |

综合评分公式（P1）：

```
// critic_score 存在时
episode_score = 0.30*importance + 0.25*min(critic/0.8,1) + 0.15*min(tools/20,1) + 0.15*min(duration/300000,1) + 0.15*(user_mark?1:0)

// critic_score 缺失时（权重重分配）
episode_score = 0.40*importance + 0.20*min(tools/20,1) + 0.20*min(duration/300000,1) + 0.20*(user_mark?1:0)
```

#### 3.1.3 代码变更

| 文件 | 变更 |
|------|------|
| 新增 `internal/biz/l1_field_extraction.go` | `ExtractKeyDecisions` / `ExtractKeyArtifacts` |
| `internal/data/memory_shim_l2.go` | 替换 key_decisions 提取逻辑 |
| `internal/tools/working_memory/tools.go` | field_kind 参数增加 enum 约束 |

### 3.2 Embedding 单写 + 按需索引

#### 3.2.1 删除 memory_l2_index_meta 表

```sql
DROP TABLE IF EXISTS memory_l2_index_meta;
```

索引元数据不再独立维护，embedding 在写入时同步生成。

#### 3.2.2 暴力搜索阈值

```
func searchFacts(query string, facts []Fact, threshold int) []Fact:
    if len(facts) <= threshold:  // 默认 5000
        // 线性扫描：遍历所有 facts，计算余弦相似度
        return linearScan(query, facts)
    else:
        // pgvector 查询
        return pgvectorSearch(query)
```

#### 3.2.3 pgvector 增量同步协议

写入 fact 时同步生成 embedding：

```go
func (r *factRepo) CreateFact(ctx context.Context, fact Fact) error {
    // 1. 写入 fact 记录
    // 2. 同步生成 embedding
    embedding, err := r.embedder.Embed(ctx, fact.Content)
    // 3. 更新 embedding 列
    _, err = r.db.ExecContext(ctx,
        "UPDATE memory_facts SET embedding = $1, embedding_version = $2 WHERE id = $3",
        embedding, r.embedder.Version(), fact.ID)
    return err
}
```

#### 3.2.4 embedding_version 字段

```sql
ALTER TABLE memory_facts ADD COLUMN embedding_version INTEGER NOT NULL DEFAULT 0;
```

增量重建逻辑：

```
func rebuildEmbeddings(ctx, currentVersion int):
    // 查询所有 embedding_version < currentVersion 的记录
    // 批量重新生成 embedding
    // UPDATE memory_facts SET embedding = $1, embedding_version = currentVersion WHERE id = $2
```

### 3.3 consolidation_status 统一

#### 3.3.1 数据模型变更

| 表 | 变更 | 说明 |
|---|------|------|
| `memory_episodes` | 数据迁移 | `UPDATE ... SET consolidation_status = 'consolidated' WHERE ... IN ('pending', 'done')` |
| `memory_facts` | 新增列 | `ALTER TABLE memory_facts ADD COLUMN source_episode_id TEXT DEFAULT ''` |
| `memory_l2_index_meta` | 删除表 | `DROP TABLE IF EXISTS memory_l2_index_meta` |

#### 3.3.2 代码变更

| 文件 | 变更 |
|------|------|
| `internal/data/memory_shim_l2.go` | `'pending'` → `'consolidated'`；删除 `MarkEpisodeConsolidated` |
| `internal/data/memory_maintenance_adapter.go` | `'done'` → `'consolidated'` |
| `internal/cronrunner/jobs/memory_l2_consolidate.go` | **删除整个文件** |
| `internal/cronrunner/` 注册处 | 移除 L2ConsolidateWorker 注册 |

### 3.4 L1 与框架 Memory 职责厘清

互斥规则映射（复用 `ToolsDenyJSON`）：

| 模式 | ToolsDenyJSON 增加值 | 效果 |
|------|---------------------|------|
| `working_memory` | `["memory_add", "memory_update", "memory_delete", "memory_search", "memory_load"]` | Agent 只见 working_memory_* |
| `framework_memory` | `["working_memory_read", "working_memory_list", "working_memory_write", "working_memory_patch", "working_memory_delete"]` | Agent 只见 memory_* |
| `both` | 不增加 | 当前默认行为 |

---

## 4. 阶段四设计：安全与完整性

### 4.1 SHA-256 校验和（单租户推荐）

```sql
ALTER TABLE session_summaries ADD COLUMN content_hash TEXT DEFAULT '';
ALTER TABLE session_summaries ADD COLUMN trust_source TEXT NOT NULL DEFAULT 'self';
```

```go
// 压缩完成时
contentHash := sha256.Sum256([]byte(summary.SummaryMarkdown + salt))
summary.ContentHash = hex.EncodeToString(contentHash[:])

// 注入时验证
func verifySummaryIntegrity(summary SessionSummary, salt string) error {
    if summary.TrustSource == "episode_fork" || summary.TrustSource == "agent_delegation" {
        return nil // 跨 session 来源，跳过校验
    }
    expected := sha256.Sum256([]byte(summary.SummaryMarkdown + salt))
    if hex.EncodeToString(expected[:]) != summary.ContentHash {
        return fmt.Errorf("content hash mismatch")
    }
    return nil
}
```

分层策略：
- Tier 0: TLS + 数据库约束（默认，零额外代码）— 验证项：TLS 连接强制启用、session_summaries.summary_markdown NOT NULL、trust_source NOT NULL DEFAULT 'self'
- Tier 1: SHA-256 校验和（推荐替代 HMAC）
- Tier 2: HMAC-SHA256（多租户升级）
- Tier 3: AES-256-GCM 加密（高安全场景）

### 4.2 Episode Fork

```sql
ALTER TABLE memory_episodes ADD COLUMN fork_from_episode_id TEXT DEFAULT '';
ALTER TABLE memory_episodes ADD COLUMN fork_from_turn_index INTEGER DEFAULT 0;
ALTER TABLE session_runtime ADD COLUMN fork_source TEXT DEFAULT '';
```

Fork API：

```
POST /v1/sessions
  body: { fork_from_episode_id, fork_from_turn_index, agent_id }
  response: { session_id, injected_context: { l1_snapshot, summary, key_decisions } }
```

### 4.3 摘要保留/丢弃策略

默认保留：用户原始意图、关键决策及理由、修改的文件及原因、遇到的错误及解决方案、待办事项和当前进度。

默认丢弃：冗长工具输出、重复尝试、中间调试过程。

用户指定：`preserve_instruction` 参数，接入现有 `preserveInstruction` 传输管道。

### 4.4 子目录规则按需加载

#### 4.4.1 activate_on_glob 字段

规则配置新增 glob 模式，匹配当前操作文件路径时才激活：

```go
type SkillRule struct {
    // ...现有字段
    ActivateOnGlob []string `json:"activate_on_glob"` // 新增：glob 模式列表
}
```

示例配置：

```yaml
- name: "react-patterns"
  activate_on_glob:
    - "src/components/**/*.tsx"
    - "src/hooks/**/*.ts"
  content: "遵循 React 函数组件模式..."
```

#### 4.4.2 文件路径提取

从工具调用参数中提取当前操作文件路径：

```go
func extractFilePaths(toolCalls []ToolCall, paramMapping map[string]string) []string {
    var paths []string
    for _, tc := range toolCalls {
        for paramName, pathValue := range paramMapping {
            if v, ok := tc.Params[paramName]; ok {
                paths = append(paths, v.(string))
            }
        }
    }
    return paths
}
```

#### 4.4.3 参数名映射表

将工具参数名映射到文件路径：

| 工具 | 参数名 | 说明 |
|------|--------|------|
| file_read / file_write | file_path | 文件读写路径 |
| file_edit | file_path | 文件编辑路径 |
| shell | command | 从命令中提取路径（正则） |
| search | path | 搜索目录 |

匹配逻辑：遍历当前轮次所有工具调用，提取文件路径，与 `activate_on_glob` 做 glob 匹配，仅注入匹配的规则。

---

## 5. 跨层优化设计

### 5.1 L3 Recall 去重

#### 5.1.1 fingerprint 去重

基于内容哈希去重，防止重复写入相同事实：

```go
func (r *factRepo) CreateFact(ctx context.Context, fact Fact) error {
    fingerprint := sha256.Sum256([]byte(fact.Content))
    fp := hex.EncodeToString(fingerprint[:])
    // 检查是否已存在相同 fingerprint
    exists, _ := r.db.QueryContext(ctx,
        "SELECT 1 FROM memory_facts WHERE fingerprint = $1 AND agent_id = $2 LIMIT 1",
        fp, fact.AgentID)
    if exists {
        return nil // 已存在，跳过
    }
    fact.Fingerprint = fp
    // 正常写入
}
```

#### 5.1.2 语义去重

embedding 余弦相似度 > 0.95 时判定为重复：

```go
func isSemanticDuplicate(ctx context.Context, embedding []float64, existingFacts []Fact, threshold float64) bool {
    for _, f := range existingFacts {
        sim := cosineSimilarity(embedding, f.Embedding)
        if sim > threshold { // 默认 0.95
            return true
        }
    }
    return false
}
```

#### 5.1.3 跨层去重

L3 recall 与 L1 已注入字段去重，避免信息冗余：

```go
func dedupL3WithL1(l3Facts []Fact, l1Fields []L1Field) []Fact {
    l1ContentSet := make(map[string]bool)
    for _, f := range l1Fields {
        l1ContentSet[normalize(f.ValueText)] = true
    }
    var deduped []Fact
    for _, fact := range l3Facts {
        if !l1ContentSet[normalize(fact.Content)] {
            deduped = append(deduped, fact)
        }
    }
    return deduped
}
```

### 5.2 L4 实体提取增强

#### 5.2.1 P0：修复现有实体提取 bug

确保基础提取链路正常，修复已知 bug（空指针、字段缺失等）。

#### 5.2.2 P1：与 Path B 合并执行

LLM 增强路径同时提取实体和 Episode，减少 LLM 调用次数：

```go
type EnhancedExtractionResult struct {
    Episode     Episode
    Entities    []Entity
    Relations   []Relation
}

func extractWithLLM(ctx context.Context, input ExtractionInput) (*EnhancedExtractionResult, error) {
    // 单次 LLM 调用同时提取 Episode + 实体 + 关系
    prompt := buildEnhancedExtractionPrompt(input)
    result, err := callLLM(ctx, prompt)
    // 解析结果
    return parseEnhancedResult(result), nil
}
```

#### 5.2.3 P2：实体关系推理

基于提取的实体构建关系图（可选，后续迭代）：

```go
type EntityRelation struct {
    SourceEntityID string
    TargetEntityID string
    RelationType   string  // "depends_on" / "implements" / "references" / "contains"
    Confidence     float64
}
```
