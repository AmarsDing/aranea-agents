# Memory L0-L4 优化解决方案

> 日期：2026-06-08
> 状态：方案批准，待实施
> 范围：L0 上下文装配、L1 工作记忆、L2 情景记忆、L3 语义记忆、L4 持久记忆

---

## 目录

1. [问题总览与优先级](#1-问题总览与优先级)
2. [L0 层优化](#2-l0-层优化)
3. [L1 层优化](#3-l1-层优化)
4. [L2 层优化](#4-l2-层优化)
5. [L3/L4 层优化](#5-l3l4-层优化)
6. [跨层优化](#6-跨层优化)
7. [实施路线图](#7-实施路线图)

---

## 1. 问题总览与优先级

| # | 问题 | 严重度 | Token 影响 | 方案章节 |
|---|------|--------|-----------|---------|
| P1 | L0 Snapshot 写入与 EvolutionMetricsEnabled 耦合 | 低 | 无直接影响 | §2.1 |
| P2 | 摘要注入 User 模式未使用但代码存在 | 低 | 无 | §2.2 |
| P3 | L1 Schema/History 表大多数 Agent 未使用 | 中 | 间接 | §3.1 |
| P4 | L1→L0 全量渲染，字段多时 token 浪费 | 高 | 每轮浪费 2K~8K | §3.2 |
| P5 | L1 与框架 Memory 职责重叠 | 中 | 无 | §3.3 |
| P6 | Episode 生成需要额外 LLM 调用 | 高 | 每任务 ~2K | §4.1 |
| P7 | L2→L3/L4 两跳 LLM | 高 | 每巩固 ~4K | §4.2 |
| P8 | Embedding 双写设计冗余 | 低 | 无 | §4.3 |
| P9 | 缺少结构化替代路径（零成本压缩） | 高 | 可省 50%+ 压缩成本 | §2.5 |
| P10 | 缺少 Prompt Caching 前缀分离 | 高 | 每轮浪费 8K~15K | §2.3 |
| P11 | 缺少显式压缩预算 | 高 | 被动压缩信息损失 | §2.4 |
| P12 | 压缩用户可感知 | 中 | 无 | §2.5 |
| P13 | 缺少摘要保留/丢弃策略 | 中 | 摘要信噪比低 | §6.1 |
| P14 | 多代理间压缩上下文无完整性保护 | 中 | 无 | §6.2 |
| P15 | L2 Episode 不支持分叉 | 低 | 无 | §6.3 |
| P16 | L3/L4 Embedding 全量重建 | 中 | 重建成本高 | §6.4 |
| P17 | Skill 文件全量加载 | 中 | 每轮浪费 2K~5K | §6.5 |

---

## 2. L0 层优化

### 2.1 L0 Snapshot 简化

#### 现状

- `memory_l0_assembly_snapshots` 表记录每次 prompt 装配的元数据快照
- 写入门控：`EvolutionMetricsEnabled = true`（默认开启）+ `L0SnapshotMode != "off"`
- 默认 `on_warning` 模式：`usedRatio >= 0.60` 时写入
- 前端记忆中心有完整的展示链路（`MemorySessionsPanel.vue` + `MemorySnapshotDrawer.vue`）
- `EvolutionMetricsEnabled` 控制的「进化指标采集」逻辑**从未实现**，目前唯一作用是门控 L0 快照写入

#### 问题

1. `EvolutionMetricsEnabled` 语义不匹配——它本意是控制进化指标采集，但实际只控制快照写入
2. `segments_json` 是快照中最大的字段（记录每条 message 的 section/role/source/tokens/preview），前端抽屉使用率极低
3. 设计文档中规划的 Datadog 指标（`aranea.memory.l0.*`）从未实现，属于死规划

#### 方案

**保留 L0 Snapshot 功能，简化实现**：

| 变更项 | 说明 |
|--------|------|
| 解耦 `EvolutionMetricsEnabled` | 新增独立的 `L0SnapshotEnabled` 字段（默认 true），快照写入不再依赖 `EvolutionMetricsEnabled` |
| 精简 `segments_json` | 改为 `segments_summary_json`，仅记录各段的聚合统计（段名、token 估算、消息数），不记录逐条 message 详情 |
| 保留 `MemorySnapshotDrawer` | 抽屉改为展示聚合统计而非逐条详情，数据量减少 80%+ |
| 清理死规划 | 从设计文档中删除未实现的 Datadog 指标规划 |
| 保留 `ARANEA_L0_SNAPSHOT` 环境变量 | 调试时可通过 `always/force` 强制高频写入 |

#### `segments_summary_json` 格式

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

#### 写入频率

保持现状：默认 `on_warning` 模式（`usedRatio >= 0.60` 时写入）。这是合理的诊断频率，不需要增加三级写入策略。

---

### 2.2 摘要注入模式说明

#### 现状

框架层（`trpc-agent-go`）提供两种摘要注入模式：

| 模式 | 行为 | 适用场景 |
|------|------|---------|
| `SessionSummaryInjectionSystem`（默认） | 摘要作为 system message 注入，位于 preserved head，不受 token trimming | 通用场景 |
| `SessionSummaryInjectionUser` | 摘要作为 user message 注入，参与 token trimming，可被滑动窗口淘汰 | 超长对话（数百轮）的滑动窗口场景 |

#### 事实

- User 模式**不是 workaround**，而是为超长对话滑动窗口设计的独立功能
- Aranea-Agents 应用层**完全没有使用 User 模式**——`internal/` 中没有任何对 `WithSessionSummaryInjectionMode` 的调用
- 框架层 User 模式有独立设计价值，不应删除

#### 方案

**无需改动**。在 Agent 配置文档中说明当前仅使用 System 模式。若未来出现超长对话场景（数百轮），可按需启用 User 模式。

---

### 2.3 Prompt Caching 友好的三层前缀分离

#### 背景

当前 `memory_inject.go` 中 `buildRuntimeMemoryCue()` 将 L1/L3/L4 所有记忆段拼成一个 system message，与 SOUL.md / Skill 文件混在一起。每次请求内容可能变化，导致 Prompt Cache 频繁失效。

#### 框架已有 Prompt Caching 支持

**关键发现**：trpc-agent-go 框架已完整支持 Anthropic Prompt Caching，且项目层已集成配置。

框架层（`pkg/trpc-agent-go/model/anthropic/options.go`）提供三个独立缓存选项：

| 选项 | 作用 | 默认值 |
|------|------|--------|
| `WithCacheSystemPrompt(true)` | 在 system prompt 最后一个 TextBlock 标记 `cache_control: ephemeral` | false |
| `WithCacheTools(true)` | 在最后一个 tool 定义标记 `cache_control` | false |
| `WithCacheMessages(true)` | 在倒数第二个 assistant message 标记 `cache_control` | false |

项目层（`internal/provider/trpc_llm.go`）已将配置映射到框架：

```go
if cfg.CacheSystemPrompt {
    providerOpts = append(providerOpts, trpcanthropic.WithCacheSystemPrompt(true))
}
```

**只需在数据库的模型配置 JSON 中设置 `cache_system_prompt: true`、`cache_tools: true`、`cache_messages: true`，即可启用基本 Prompt Caching，无需改代码。**

#### 目标

在已有缓存支持基础上，通过三层前缀分离最大化缓存命中率。

#### 三层前缀设计

```
Messages 布局（从左到右，缓存前缀从长到短）：

┌─────────────────────────────────────────────────────┐
│ Layer 1: 静态前缀 — 缓存命中率最高                    │
│ system message:                                     │
│   - SOUL.md / AGENTS.md 内容                        │
│   - Skill 文件内容（条件激活的 Skill 除外）            │
│   - Agent 角色定义、约束、安全规则                     │
│ 变化频率：会话生命周期内不变                           │
│ 预估 token：8K~15K                                   │
│ 缓存标记：cache_control = "ephemeral"                │
├─────────────────────────────────────────────────────┤
│ Layer 2: 半静态前缀 — 缓存命中率中等                  │
│ system message:                                     │
│   - L1 working memory cue（任务内低频变化）           │
│   - L3 recall facts（跨轮低频变化）                   │
│   - L4 graph context（跨轮低频变化）                  │
│   - 条件激活的 Skill 内容（按文件路径按需加载）        │
│ 变化频率：任务/轮次切换时才变                          │
│ 预估 token：2K~8K                                    │
│ 缓存标记：cache_control = "ephemeral"                │
├─────────────────────────────────────────────────────┤
│ Layer 3: 动态内容 — 无缓存                           │
│ system message:                                     │
│   - session summary（压缩后变化）                     │
│ user/assistant:                                     │
│   - 历史消息                                         │
│ user:                                               │
│   - 当前输入                                         │
│ 变化频率：每轮都变                                    │
│ 预估 token：动态                                      │
│ 缓存标记：无                                         │
└─────────────────────────────────────────────────────┘
```

#### 实现细节

##### 2.3.1 `memory_inject.go` 重构

**当前代码**（`buildRuntimeMemoryCue`）：

```go
// 当前：所有记忆段拼成一个 system message
cue := ""
cue += l1Cue   // L1 working memory
cue += l2Cue   // L2 episodic
cue += l3Cue   // L3 recall
cue += l4Cue   // L4 graph
sys := trpcmodel.NewSystemMessage(cue)
args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
```

**重构后**：

```go
// 重构后：三层 system message 分离
// Layer 1: 静态前缀（由 ContentRequestProcessor 在会话首次请求时构建，后续复用）
//   → 已有的 system prompt 注入逻辑不变

// Layer 2: 半静态记忆前缀
memoryCue := ""
memoryCue += l1Cue   // L1 working memory
memoryCue += l3Cue   // L3 recall facts
memoryCue += l4Cue   // L4 graph context
memoryCue += conditionalSkillCue  // 条件激活的 Skill
if memoryCue != "" {
    memoryMsg := trpcmodel.NewSystemMessage(memoryCue)
    // 标记缓存断点（Anthropic API）
    memoryMsg.CacheControl = "ephemeral"
    args.Request.Messages = append([]trpcmodel.Message{memoryMsg}, args.Request.Messages...)
}

// Layer 3: 动态内容（session summary + 历史消息 + 当前输入）
//   → 已有的 summary 注入和历史消息逻辑不变
```

##### 2.3.2 缓存断点标记

**Anthropic API**：
- 支持最多 **4 个独立缓存断点**
- 当前框架已使用 3 个：system（1 个）+ tools（1 个）+ messages（1 个）
- 三层前缀分离需要在 system 部分增加 1 个断点（静态层 + 动态层），刚好达到 4 个上限
- `applyCacheControlToSystem` 当前在 system prompts 的**最后一个 TextBlock** 标记 `cache_control`
- **关键问题**：BeforeModel hooks 以 prepend 方式注入 system message，动态内容（L2/L3/L4 记忆）排在前面，静态内容（Instruction/Skills）排在后面。`cache_control` 标记在最后一个 TextBlock（静态内容），但前面的动态内容变化也会导致缓存失效
- **解决方案**：调整 system message 注入顺序——静态内容在前，动态内容在后；或修改 `applyCacheControlToSystem` 支持在指定位置标记断点

**Google Gemini API**：在 `system_instruction` 中使用 `cached_content` 引用缓存。

**OpenAI API**：自动缓存，无需手动标记。但前缀分离仍然有益——静态部分更可能命中自动缓存。

**不支持的模型**：忽略 `cache_control` 字段，降级为无缓存行为，功能不受影响。

##### 2.3.3 Layer 1 静态前缀的构建时机

```
会话首次请求：
  1. 读取 SOUL.md / AGENTS.md / Skill 文件
  2. 构建 Layer 1 system message
  3. 缓存到 session 级变量（session.staticPrefix）
  4. 标记 cache_control = "ephemeral"

后续请求：
  1. 直接复用 session.staticPrefix（不重新构建）
  2. 若 Skill 列表变化（工具调用导致新 Skill 激活），标记 staticPrefixDirty
  3. 下次请求时重建 staticPrefix
```

##### 2.3.4 Layer 2 半静态前缀的变化检测

```
每次请求前：
  1. 计算 Layer 2 的内容 hash（L1 fields + L3 recall + L4 graph）
  2. 与 session 级缓存 hash 比较
  3. 若相同：复用缓存的 Layer 2 message
  4. 若不同：重建 Layer 2 message，标记 cache_control = "ephemeral"
```

##### 2.3.5 条件激活 Skill 的按需加载

详见 §6.5 子目录规则按需加载。条件激活的 Skill 内容归入 Layer 2（半静态前缀），因为它们根据当前文件路径动态变化，但变化频率低于每轮。

#### Token 节省估算

| 场景 | 当前每轮输入 token | 优化后每轮输入 token | 节省 |
|------|-------------------|---------------------|------|
| 短对话（5 轮内） | ~15K | ~15K（无缓存收益） | 0% |
| 中等对话（20 轮） | ~25K | ~12K（Layer 1+2 缓存命中） | ~52% |
| 长对话（50 轮+） | ~50K | ~20K（Layer 1+2 缓存命中 + summary 压缩） | ~60% |

注：节省的是输入 token 成本（缓存命中按缓存读取费率计费，通常为正常价格的 10%~50%），不是 token 数量本身。

#### 兼容性

| 模型厂商 | 缓存支持 | 行为 |
|---------|---------|------|
| Anthropic | `cache_control` API | 完全支持，三层前缀均标记缓存断点 |
| Google Gemini | `cached_content` API | 支持，需适配 API 格式 |
| OpenAI | 自动缓存 | 前缀分离有助于提高自动缓存命中率 |
| 其他 | 不支持 | 降级为无缓存，功能不受影响 |

---

### 2.4 显式压缩预算

#### 现状

- 当前压缩触发完全基于 `ContextUsedRatio`（`promptTokens / contextWindow`）与阈值（默认 0.6）的比较，**不存在 `contextWindow * 0.85` 的预算计算**——这是此前方案的错误描述
- 压缩触发路径有三条：
  1. `AfterNativeTurn`：异步触发（`safego.Go`），不阻塞用户
  2. `BeforeDurableTurn`：**同步触发**，在 durable turn 恢复前阻塞执行，跳过 minGap 防抖
  3. `CompactSession`：手动触发的同步 API，支持 `preserveInstruction` 参数
- 上下文满载时（`ContextUsedRatio >= 1.0` 或 `ContextUsedTokens >= Window`），`atFullContextUsage` 返回 true，绕过 minGap 防抖
- **已有同步压缩机制**（`BeforeDurableTurn` 和 `CompactSession`），但缺少基于压缩预算的分级触发策略

#### 方案

##### 2.4.1 压缩预算模型

```
contextWindow = 128K (示例)

reserved_system = 15K (系统提示词 + 工具定义，固定开销)
compression_buffer_ratio = 0.12 (12%，约 15K)
effective_budget = contextWindow - reserved_system - compression_buffer
                 = 128K - 15K - 15K = 98K
```

##### 2.4.2 为什么是 12% 而非 15%

- 15% 是总窗口的 15%，但系统提示词已占 ~12%，实际可用窗口只剩 ~88%
- 在 88% 可用窗口中再留 15% 缓冲，等于总窗口的 13.2%，过于保守
- 压缩延迟（异步 2~5 秒）期间，每轮增量约 2K~5K tokens
- 12% 缓冲（~15K tokens）足以覆盖 3~7 轮的增量，足够压缩完成
- 用户可通过 `compression_buffer_ratio` 调整（0.08~0.20 范围）

##### 2.4.3 三级触发阈值

```
effective_budget = contextWindow * (1 - compression_buffer_ratio)

soft_trigger  = effective_budget * 0.70
  → 启动后台异步压缩（Level 2 Structured Compact 或 Level 3 LLM Compact）
  → 复用现有 AfterNativeTurn 异步路径
  → 用户无感知，继续正常对话

hard_trigger  = effective_budget * 0.90
  → 同步压缩（阻塞下一轮用户请求）
  → 复用现有 BeforeDurableTurn 同步路径，扩展为通用 hard trigger
  → UI 显示"正在优化上下文..."

emergency     = effective_budget
  → 截断最老消息（drop_oldest 策略）
  → 复用现有 atFullContextUsage 机制
  → 最后手段，保证不超出上下文窗口
```

以 128K 窗口为例：

| 阈值 | Token 值 | 行为 | 现有机制 |
|------|---------|------|---------|
| soft_trigger | 98K * 0.70 = 68.6K | 后台异步压缩 | AfterNativeTurn + safego.Go |
| hard_trigger | 98K * 0.90 = 88.2K | 同步压缩 | BeforeDurableTurn 扩展 |
| emergency | 98K | 截断最老消息 | atFullContextUsage |

##### 2.4.4 配置项

| 配置项 | 字段名 | 默认值 | 范围 | 说明 |
|--------|--------|--------|------|------|
| 压缩缓冲区比例 | `compression_buffer_ratio` | 0.12 | 0.08~0.20 | 总窗口的 N% 作为压缩缓冲 |
| 软触发比例 | `soft_trigger_ratio` | 0.70 | 0.50~0.80 | effective_budget 的 N% 触发异步压缩 |
| 硬触发比例 | `hard_trigger_ratio` | 0.90 | 0.80~0.95 | effective_budget 的 N% 触发同步压缩 |
| 保留尾部轮次 | `compress_keep_turns` | 4 | 2~10 | 压缩时保留最近 N 轮不压缩 |
| 最小压缩间隔 | `compress_min_gap_sec` | 600 | 60~1800 | 两次异步压缩的最小间隔（秒） |

---

### 2.5 用户无感后台压缩

#### 目标

压缩过程对用户透明——不阻塞对话、不增加等待时间，但用户可在界面上观察到后台压缩的状态。

#### 三级压缩流水线

借鉴 Claude Code 的 MicroCompact → Session Memory → AutoCompact，结合 Codex CLI 的结构化优先路径：

```
┌─────────────────────────────────────────────────────────────┐
│ Level 1: MicroCompact（零 LLM 成本，每次请求前自动执行）      │
│                                                              │
│ 触发：每次 LLM 请求前                                        │
│ 执行：同步，耗时 < 1ms                                       │
│ 操作：                                                       │
│   1. 清理已摘要覆盖的旧工具结果                               │
│      （已有 compactCurrentInvocationEvent）                   │
│   2. 截断超大 tool result                                    │
│      （已有 OversizedToolResultMaxTokens）                    │
│   3. 新增：清理已过期 TTL 的 L1 字段渲染                      │
│   4. 新增：清理 reasoning_content（按配置策略）               │
│ 成本：零 LLM 调用                                            │
│ 用户感知：无                                                  │
├─────────────────────────────────────────────────────────────┤
│ Level 2: Structured Compact（零 LLM 成本，soft_trigger 时）  │
│                                                              │
│ 触发：usedRatio >= soft_trigger（默认 70%）                  │
│ 执行：异步后台，耗时 5~50ms                                   │
│ 操作：                                                       │
│   1. 读取 L1 快照（task_title, task_goal, fields）           │
│   2. 读取关键决策（key_decisions from L1 fields）             │
│   3. 读取待办任务（pending items from L1 fields）             │
│   4. 构建结构化摘要 JSON：                                    │
│      {                                                       │
│        "task": "...", "goal": "...",                         │
│        "decisions": [...], "pending": [...],                 │
│        "files_modified": [...], "errors_resolved": [...]     │
│      }                                                       │
│   5. 若结构化数据 < 原始历史 30%：替换历史为结构化摘要        │
│   6. 否则：降级到 Level 3                                    │
│ 成本：零 LLM 调用                                            │
│ 用户感知：无（后台执行）                                      │
├─────────────────────────────────────────────────────────────┤
│ Level 3: LLM Compact（有 LLM 成本，hard_trigger 时）         │
│                                                              │
│ 触发：usedRatio >= hard_trigger（默认 90%）                  │
│       或 Level 2 降级                                        │
│ 执行：异步后台（hard_trigger 时同步阻塞），耗时 2~10 秒       │
│ 操作：                                                       │
│   1. 调用 LLM 生成结构化摘要（支持保留/丢弃策略指令）         │
│   2. 摘要格式：9 章节结构化模板                               │
│   3. 原子替换：压缩完成后一次性替换上下文                     │
│ 成本：1 次 LLM 调用（~2K~4K tokens）                         │
│ 用户感知：hard_trigger 时短暂等待；异步时无感知               │
└─────────────────────────────────────────────────────────────┘
```

#### Level 2 Structured Compact 的结构化摘要模板

```json
{
  "task": {
    "title": "实现用户认证模块",
    "goal": "完成 JWT 认证流程",
    "status": "in_progress"
  },
  "decisions": [
    "使用 RS256 算法签名 JWT",
    "Token 过期时间设为 24 小时"
  ],
  "files_modified": [
    "internal/service/auth.go — 新增 Login/Logout 方法",
    "internal/biz/token.go — 实现 JWT 生成和验证"
  ],
  "errors_resolved": [
    "Token 验证失败 → 修复了密钥加载顺序问题"
  ],
  "pending": [
    "实现 Refresh Token 逻辑",
    "添加 Token 黑名单"
  ],
  "current_focus": "正在实现 Refresh Token 逻辑"
}
```

#### Level 3 LLM Compact 的 9 章节摘要模板

```
# Session Summary

## 1. User Intent
{用户的核心意图和需求}

## 2. Key Technical Decisions
{关键技术决策及其理由}

## 3. Files Modified
{修改的文件及原因}

## 4. Errors Encountered
{遇到的错误及修复方式}

## 5. Pending Tasks
{待完成任务及当前进度}

## 6. Current Focus
{当前正在处理的内容}

## 7. Architecture Decisions
{架构层面的决策}

## 8. Test Results
{测试结果和覆盖率}

## 9. Next Steps
{下一步行动}
```

#### 用户界面设计

```
对话界面底部的上下文指示器：

正常状态：
┌──────────────────────────────────────────┐
│ 上下文: ████████░░ 78%                    │
└──────────────────────────────────────────┘

后台压缩中：
┌──────────────────────────────────────────┐
│ 上下文: ████████░░ 78%  🔄 优化中...      │
└──────────────────────────────────────────┘

压缩完成：
┌──────────────────────────────────────────┐
│ 上下文: ███░░░░░░░ 32%  ✓ 已优化         │
└──────────────────────────────────────────┘

同步压缩中（hard_trigger）：
┌──────────────────────────────────────────┐
│ ⏳ 正在优化上下文，请稍候...              │
└──────────────────────────────────────────┘
```

#### 压缩完成后的原子替换

```
压缩完成后的操作序列：

1. 生成新摘要（Level 2 结构化 或 Level 3 LLM）
2. 写入 session_summaries 表
3. 重写 Runner Snapshot：
   - 保留最近 N 轮消息（compress_keep_turns）
   - 前置摘要 system message
   - 更新 ContextUsedTokens 和 ContextUsedRatio
4. 通过 EventBus 发布 `session_compressed` 事件
5. 前端收到事件后更新上下文使用率显示
```

#### 防抖与并发控制

```
并发控制：

1. CAS 乐观锁：TryIncrementCompressVersion（已有）
2. 最小间隔：compress_min_gap_sec（默认 600 秒）
3. 满载跳过防抖：atFullContextUsage 时跳过最小间隔检查（已有）
4. 新增：压缩进行中标记（compressing = true），防止重复触发
5. 新增：压缩超时（默认 8 分钟），超时后释放标记
```

---

## 3. L1 层优化

### 3.1 L1 表结构轻量化

#### 现状

4 张表：`memory_l1_tasks` + `memory_l1_fields` + `memory_l1_field_history` + `memory_l1_schemas`

大多数 Agent 不使用 Schema 声明和字段版本历史。

#### 方案

**不删表，降为可选**：

| 表 | 策略 | 说明 |
|----|------|------|
| `memory_l1_tasks` | 必需 | 任务容器，核心表 |
| `memory_l1_fields` | 必需 | 结构化字段，核心表 |
| `memory_l1_field_history` | 可选 | 新增 `L1HistoryEnabled` 配置（默认 false），关闭时 UpsertL1Field 跳过历史写入 |
| `memory_l1_schemas` | 可选 | 仅在 Agent 配置了 `L1DefaultSchemaID` 时激活 Schema 校验，无 Schema 时字段自由写入 |

#### 配置项

| 配置项 | 字段名 | 默认值 | 说明 |
|--------|--------|--------|------|
| L1 历史记录 | `L1HistoryEnabled` | false | 是否记录字段版本历史 |
| L1 Schema 校验 | `L1DefaultSchemaID` | "" | 绑定的 Schema ID，空则不校验 |

---

### 3.2 L1 选择性注入

#### 现状

`L1MemoryCue()` 渲染时遍历所有 `pin_to_prompt=true` 且 `visibility!=internal` 的字段，全量注入 system message。字段多时每轮浪费 2K~8K tokens。

#### 方案

##### 3.2.1 三层过滤链

```
L1 → L0 注入过滤链：

1. visibility 过滤（已有）：visibility != 'internal'
2. pin_to_prompt 过滤（已有）：pin_to_prompt = true
3. 新增：相关性过滤
   ├── 字段数 <= 5：全量注入（开销可忽略）
   └── 字段数 > 5：
       a. pin_to_prompt=true 的字段：始终注入
       b. 其余字段：按 read_count 降序排序，取 top-K
       c. K = min(非 pinned 字段数, 5)
4. 新增：token 预算硬上限
   ├── L1 注入总 token 不超过 budget_tokens 的 50%（默认 4K）
   └── 超出时按 token_estimate 降序截断（大字段优先截断）
```

##### 3.2.2 实现路径

**短期（无 embedding）**：按 `read_count` 降序排序，高频字段优先注入。

**长期（有 embedding）**：基于当前 user message 语义检索 top-K 字段。为 L1 字段增量添加 embedding，复用现有 embedding 基础设施。

##### 3.2.3 配置项

| 配置项 | 字段名 | 默认值 | 说明 |
|--------|--------|--------|------|
| L1 注入预算比例 | `L1InjectBudgetRatio` | 0.50 | budget_tokens 的 N% 作为注入上限 |
| L1 最大注入字段数 | `L1MaxInjectFields` | 10 | 非 pinned 字段最多注入 N 个 |

---

### 3.3 L1 与框架 Memory 职责厘清

#### 现状

trpc-agent-go 框架层的 `memory_add/update/search` 工具与 L1 的 `working_memory_write/read` 功能重叠，Agent 可能混淆使用。

#### 方案

##### 3.3.1 分工定义

| 维度 | 框架层 Memory（trpc-agent-go/memory/） | 产品层 L1 Working Memory |
|------|---------------------------------------|-------------------------|
| 定位 | 通用键值记忆，扁平结构 | 任务级结构化决策态 |
| 隔离 | `<appName, userID>` | `<session, agent, task>` |
| 结构 | 扁平 key-value | 字段有类型/可见性/TTL/版本 |
| 适用 | 简单偏好/事实存储，跨 Agent 共享 | 复杂任务的 goal/constraints/decisions |
| 工具 | memory_add, memory_update, memory_search, memory_load | working_memory_read/write/patch/delete |

##### 3.3.2 互斥规则

```
Agent 配置新增 memory_tool_mode：
  - "working_memory"（默认）：仅注册 working_memory 工具集
  - "framework_memory"：仅注册框架 memory 工具
  - "both"（不推荐）：同时注册两套，Agent 自行区分

默认行为：
  - 新 Agent：仅 working_memory
  - 已有 Agent：保持现状（向后兼容）
  - 禁止同一 Agent 同时启用两套工具（除非显式配置 both）
```

##### 3.3.3 数据桥接

```
框架 Memory → L1 的桥接（未来可选）：
  - 框架 memory 中与当前任务相关的 fact 可通过 L3 recall 注入 L0
  - 不需要显式数据迁移，两者通过 L0 装配层自然桥接
```

---

## 4. L2 层优化

### 4.1 结构化 Episode 优先路径

#### 现状

Episode 生成依赖 AutoMemoryWorker 的 `Extract()` 方法（LLM 或正则提取），每次任务结束可能触发 LLM 调用。

#### 方案

##### 4.1.1 双路径 Episode 生成

```
Episode 生成路径：

Path A: 结构化路径（零 LLM 成本）— 优先
  ├── 触发：L1 任务归档时
  ├── 数据源：L1 快照（task_title, task_goal, status, fields）
  ├── 生成规则：
  │   title = task_title（已有）
  │   goal = task_goal（已有）
  │   outcome = status 描述 + 最后一轮 assistant 消息截断 200 字
  │   outcome_summary = status 枚举值映射为自然语言
  │     - "completed" → "任务已完成"
  │     - "cancelled" → "任务被取消（空闲超时）"
  │     - "failed" → "任务失败"
  │     - "timeout" → "任务超时"
  │   key_decisions = L1 fields 中 field_path 匹配以下模式的值：
  │     - 包含 "decision" 的 field_path
  │     - 包含 "choice" 的 field_path
  │     - 包含 "approach" 的 field_path
  │     - visibility = "prompt" 且 pin_to_prompt = true 的字段
  │   key_artifacts = L1 fields 中 field_path 匹配：
  │     - 包含 "file" 的 field_path
  │     - 包含 "artifact" 的 field_path
  │     - field_kind = "reference" 的字段
  │   importance = 0.5（默认）
  │   confidence = 0.6（结构化路径置信度略低）
  └── 标记：episode_kind = "l1_archive_structured"，consolidation_status = "pending"

Path B: LLM 增强路径（有 LLM 成本）— 按需
  ├── 触发条件（满足任一即触发）：
  │   1. episode.importance >= 0.7
  │   2. 用户标记（memory_event_marks 中 mark_type = "star" 或 "consolidate"）
  │   3. critic_score >= 0.8
  │   4. tool_call_count >= 20（复杂任务）
  │   5. duration_ms >= 300000（5 分钟以上的长任务）
  ├── 数据源：完整消息历史 + L1 快照
  ├── 生成：调用 LLM 生成高质量摘要
  └── 覆盖 Path A 的低质量 Episode
```

##### 4.1.2 高重要性判断标准

| 信号 | 权重 | 阈值 | 说明 |
|------|------|------|------|
| `importance` 字段 | 0.30 | >= 0.7 | L1 任务或 Episode 自身标记的重要性 |
| `critic_score` | 0.25 | >= 0.8 | Critic 代理评分 |
| `tool_call_count` | 0.15 | >= 20 | 工具调用次数多 = 任务复杂 |
| `duration_ms` | 0.15 | >= 300000 | 持续时间长 = 投入大 |
| `user_mark` | 0.15 | star/consolidate | 用户显式标记 |

**综合评分公式**：

```
episode_score = 0.30 * importance
             + 0.25 * min(critic_score / 0.8, 1.0)
             + 0.15 * min(tool_call_count / 20, 1.0)
             + 0.15 * min(duration_ms / 300000, 1.0)
             + 0.15 * (user_mark ? 1.0 : 0.0)

若 episode_score >= 0.6 → 触发 Path B（LLM 增强）
```

**简化规则（P0 实现）**：满足任一条件即触发 Path B，不计算综合评分。综合评分作为 P1 优化。

##### 4.1.3 Path B 的 LLM 增强流程

```
1. 读取 Episode 关联的消息历史（最近 40 条）
2. 构建 LLM prompt：
   "Based on the following conversation, generate a structured episode summary:
    - Title: concise description
    - Goal: what was the user trying to achieve
    - Outcome: what was accomplished
    - Key Decisions: important choices made and why
    - Key Artifacts: files/code created or modified
    - Errors: problems encountered and solutions"
3. 调用 LLM（使用轻量模型，如 gpt-4o-mini）
4. 解析 LLM 输出，更新 Episode 字段
5. 标记 episode_kind = "l1_archive_enhanced"，consolidation_status = "consolidated"
6. confidence = 0.9（LLM 增强路径置信度高）
```

##### 4.1.4 效果估算

| 场景 | 当前 LLM 调用次数 | 优化后 LLM 调用次数 | 节省 |
|------|-------------------|---------------------|------|
| 10 个任务/会话 | 10 次 | 1~3 次（仅高重要性） | 70%~90% |
| 50 个任务/会话 | 50 次 | 5~10 次 | 80%~90% |

---

### 4.2 单跳巩固

#### 现状

巩固管道分两条路径：
- `L2ConsolidateWorker`：每 10 分钟运行，仅标记 `consolidation_status = "done"`，不执行实际 LLM 抽取
- `AutoMemoryWorker`：从消息直接提取 Facts/Entities，同时创建 Episode

实际数据流是：`Messages → LLM → Facts/Entities`（单跳），但设计文档描述的是 `Messages → LLM → Episode → LLM → Facts/Entities`（两跳）。

当前实现已经是单跳，但存在以下问题：

1. **L2ConsolidateWorker 是完全无效的 Worker**：
   - 它调用 `ListPendingConsolidationEpisodes(ctx, "", 20)` 传入 `agentID=""`
   - SQL 查询 `WHERE consolidation_status = 'pending' AND agent_id = ''` 几乎不可能匹配到任何记录（正常 episode 都有 agent_id）
   - 因此该 Worker **连 episode 都查不到**，不仅是"不做 LLM 提取"，而是完全空转

2. **状态值不一致**：
   - AutoMemoryWorker 创建 Episode 时直接设置 `ConsolidationStatus = "consolidated"`
   - L2ConsolidateWorker 查找 `consolidation_status = 'pending'`，标记为 `'done'`
   - 两个 Worker 使用不同的状态值（`"consolidated"` vs `"done"`），永远不会协作

3. **Episode 和 Fact 的生成是割裂的**——AutoMemoryWorker 生成 Fact 后才创建 Episode

4. **设计文档与实现不一致**，容易误导

#### 方案

##### 4.2.1 统一巩固管道

```
新巩固管道（单跳）：

AutoMemoryWorker 消费消息时：
  1. 读取最近 40 条消息
  2. 调用 Consolidator.Extract() 提取 MemoryProposal
     ├── ChainConsolidator（LLM 提取，优先）
     └── HeuristicConsolidator（正则提取，fallback）
  3. 写入 L3 facts（若 memoryPolicy.WriteL3Facts）
  4. 写入 L4 entities/relations（若 memoryPolicy.WriteL4Graph）
  5. 用结构化路径生成 Episode（零 LLM 成本，见 §4.1）
  6. 通过 episode_id 关联 Facts 和 Episode
```

##### 4.2.2 删除 L2ConsolidateWorker + 统一状态值

`L2ConsolidateWorker` 由于 `agentID=""` bug 完全无效，且与 AutoMemoryWorker 的状态值不一致，应删除并统一状态。

**操作**：
- 删除 `internal/cronrunner/jobs/memory_l2_consolidate.go`
- 从 cronrunner 注册中移除该 Worker
- 统一 `consolidation_status` 状态值：
  - 保留 AutoMemoryWorker 使用的 `"consolidated"`（而非 L2ConsolidateWorker 的 `"done"`）
  - SQL DDL 中 `consolidation_status` 的默认值从 `'pending'` 改为 `'pending'`（不变）
  - AutoMemoryWorker 创建 Episode 时直接设为 `"consolidated"`（已有行为，不变）
  - 删除 `MarkEpisodeConsolidated` 方法（仅 L2ConsolidateWorker 使用）
- L1 归档创建的 Episode（`episode_kind = "l1_archive_structured"`）保持 `consolidation_status = "pending"`，等待 AutoMemoryWorker 下次消费时处理

##### 4.2.3 Episode-Fact 关联

```
新增关联机制：

memory_facts 表新增 episode_id 字段：
  ALTER TABLE memory_facts ADD COLUMN source_episode_id TEXT DEFAULT '';

写入 Fact 时记录来源 Episode：
  - AutoMemoryWorker 提取 Fact 时，关联当前消费的 Episode
  - 查询 Fact 时可通过 episode_id 回溯到原始对话上下文

Episode 的 consolidated_l3_count / consolidated_l4_count：
  - 由 AutoMemoryWorker 写入 Fact/Entity 后更新
  - 不再由 L2ConsolidateWorker 维护
```

##### 4.2.4 更新设计文档

将设计文档中的两跳流程描述更新为单跳，与实现保持一致。

---

### 4.3 Embedding 单写 + 按需索引

#### 现状

- 设计文档要求 `memory_episodes.embedding_blob` + `memory_l2_index_meta` 双写
- 实际实现只写了 `memory_episodes.embedding_blob`
- `memory_l2_index_meta` 表已创建但未使用
- `memory_l2_index_fts`（FTS5 虚拟表）未创建

#### 方案

##### 4.3.1 确认单写策略

```
Embedding 存储策略（确认）：

SQLite（权威源）：
  memory_episodes.embedding_blob — 唯一写入点
  memory_l2_index_meta — 删除此表（当前未使用，删除无风险）

pgvector（可选读索引）：
  仅在配置了 pgvector 时同步
  同步逻辑：episode 写入后异步 best-effort 同步

检索路径：
  SQLite 本地：用 embedding_blob 做暴力余弦相似度（小规模足够）
  pgvector：用向量索引做 ANN 检索（大规模场景）
```

##### 4.3.2 删除 `memory_l2_index_meta`

```sql
DROP TABLE IF EXISTS memory_l2_index_meta;
```

当前无代码写入此表，删除零风险。

##### 4.3.3 增量 Embedding 重建

```
当前：EpisodeBackfillWorker 每 6 小时扫描 embedding_status IN ('pending','stale') 的 episode，全量补算

优化：增量重建

1. 新增 embedding_version 字段：
   ALTER TABLE memory_episodes ADD COLUMN embedding_version INTEGER DEFAULT 0;

2. 变更时标记 dirty：
   - Episode 内容更新 → embedding_status = 'stale', embedding_version += 1
   - 仅标记，不立即重建

3. Backfill Worker 优化：
   - 只处理 embedding_status = 'stale' 的记录
   - 批量处理，每批 100 条
   - 重建后标记 embedding_status = 'fresh'
   - 记录本次重建的 embedding_version

4. pgvector 同步：
   - 对比 embedding_version，仅同步版本不一致的记录
   - 删除旧向量 + 插入新向量（单条）
   - 避免全量重建
```

##### 4.3.4 L3/L4 同理

```
memory_facts：
  - 已有 index_status 字段（fresh/stale/rebuilding/disabled）
  - 增量重建逻辑已实现（SyncFactIndexFromRow）
  - 新增 embedding_version 字段，支持 pgvector 增量同步

memory_entities：
  - 新增 index_status 字段（fresh/stale/rebuilding/disabled）
  - 新增 embedding_version 字段
  - 复用与 memory_facts 相同的增量重建逻辑
```

---

## 5. L3/L4 层优化

### 5.1 L3 Recall 去重

#### 现状

L3 recall 可能返回同一事实的多个版本或高度相似的事实，浪费 token。

#### 方案

```
Recall 去重策略：

1. fingerprint 去重（已有）：memory_facts 表有 fingerprint 字段，UNIQUE 约束
2. 新增：语义去重
   - recall 结果中，若两条 fact 的向量余弦相似度 > 0.95，视为重复
   - 保留 importance 更高的那条
3. 新增：跨层去重
   - L3 recall 结果与 L1 当前字段比较
   - 若 L1 已有相同信息，跳过 L3 recall 中的重复条目
```

### 5.2 L4 实体提取增强

#### 现状

L4 当前仅支持正则提取（中英文姓名/偏好），能力有限。

#### 方案（P1 阶段，不在 P0 范围内）

```
L4 实体提取增强路径：

P0（当前）：纯正则提取
  - 人名、偏好
  - 零 LLM 成本

P1：LLM 辅助提取（仅对高重要性 Episode）
  - 触发条件：Episode 的 episode_score >= 0.6（见 §4.1.2）
  - 使用轻量模型提取实体和关系
  - 与 §4.1 的 Path B LLM 增强合并执行（一次 LLM 调用同时生成 Episode 摘要 + 实体关系）

P2：模型微调
  - 训练专用 NER 模型
  - 本地推理，零 API 成本
```

---

## 6. 跨层优化

### 6.1 摘要保留/丢弃策略

#### 方案

```
压缩 prompt 模板：

默认模板（自动压缩时使用）：
  "Summarize the conversation, preserving:
   1. User's original intent and requirements
   2. Key technical decisions and their rationale
   3. Files modified and why
   4. Errors encountered and how they were resolved
   5. Pending tasks and current progress
   Discard: verbose tool outputs, repeated attempts, intermediate debugging"

用户指定策略（手动压缩时）：
  "Summarize the conversation, focusing on: {user_instruction}
   Preserve everything related to {focus_area}
   Discard: topics unrelated to {focus_area}"
```

**实现**：
- `CompactSession` API 新增 `preserve_instruction` 参数
- 自动压缩时使用默认模板
- 手动压缩时支持用户指定保留策略
- 前端压缩按钮旁增加"保留重点"输入框（可选）

---

### 6.2 多代理加密上下文传递

#### 方案

```
多代理上下文传递安全模型：

Level 1: HMAC 签名（默认，轻量）
  ├── 压缩产物附加 HMAC-SHA256 签名
  ├── 密钥：session 级密钥（会话创建时生成，存储在 session 元数据中）
  ├── 验证：接收代理验证签名完整性
  └── 防篡改：签名不匹配 → 拒绝使用摘要，降级为不使用摘要

Level 2: AES 加密（可选，高安全场景）
  ├── 压缩产物 AES-256-GCM 加密
  ├── 密钥：agent 级密钥（Agent 配置中预设）
  └── 解密：仅目标代理可解密

实现：
  session_summaries 表新增字段：
    - signature_blob BLOB DEFAULT NULL  — HMAC 签名
    - encryption_mode TEXT DEFAULT 'hmac' — hmac / aes / none

  压缩完成时：
    1. 计算 HMAC-SHA256(summary_markdown, session_key)
    2. 若 encryption_mode = 'aes'：AES-256-GCM 加密
    3. 写入 signature_blob 和 encryption_mode

  注入时：
    1. 验证 HMAC 签名
    2. 若 encryption_mode = 'aes'：解密
    3. 签名不匹配 → 跳过摘要注入，记录告警日志
```

---

### 6.3 L2 Episode 分叉

#### 方案

```sql
ALTER TABLE memory_episodes ADD COLUMN fork_from_episode_id TEXT DEFAULT '';
ALTER TABLE memory_episodes ADD COLUMN fork_from_turn_index INTEGER DEFAULT 0;
```

**API**：

```
POST /v1/sessions/{sid}/fork
  body: {
    episode_id: string,       // 分叉源 Episode
    turn_index: integer,      // 从第几轮开始分叉（0-based）
    agent_id: string          // 新会话使用的 Agent
  }
  response: {
    new_session_id: string,   // 新创建的会话 ID
    injected_context: {       // 注入的初始上下文
      l1_snapshot: {...},     // L1 快照
      summary: "...",         // Episode 摘要
      key_decisions: [...]    // 关键决策
    }
  }
```

**语义**：
- 从指定 Episode 的指定轮次分叉出新会话
- 新会话注入 fork 源的 L1 快照 + Episode 摘要作为初始上下文
- 适用于多代理探索场景：主 Agent 完成任务后，子 Agent 从关键节点分叉继续探索

---

### 6.4 L3/L4 Embedding 增量重建

（详见 §4.3.3 和 §4.3.4，已合并到 Embedding 单写方案中）

---

### 6.5 子目录规则按需加载

#### 现状

Skill 文件在会话启动时全量加载到 system prompt，即使当前任务不涉及某些 Skill 的领域。

#### 方案

```
Skill 条件激活机制：

1. Skill 配置新增 activate_on_glob 字段：
   示例：
   - activate_on_glob: "internal/data/**" → 仅当操作 data 层文件时激活
   - activate_on_glob: "web/src/**" → 仅当操作前端代码时激活
   - activate_on_glob: "" → 始终激活（默认，向后兼容）

2. L0 装配时检查当前请求涉及的文件路径：
   - 从 user message 中提取文件路径（正则匹配）
   - 从 tool_call 参数中提取文件路径（file_path、path 等参数）
   - 匹配 activate_on_glob
   - 仅注入匹配的 Skill 内容

3. 归入 Layer 2（半静态前缀）：
   - 条件激活的 Skill 内容注入 Layer 2 system message
   - 变化时标记 Layer 2 dirty，下次请求重建

4. 无 activate_on_glob 的 Skill：始终注入 Layer 1（静态前缀），向后兼容
```

**实现要点**：
- Skill 配置存储在 Agent 的 `skills` 列表中，新增 `activate_on_glob` 字段
- 使用 `filepath.Match` 或 `doublestar` 库进行 glob 匹配
- 文件路径提取：从 `tool_call.function.arguments` 的 JSON 中提取 `file_path`、`path`、`directory` 等参数

---

## 7. 实施路线图

### Phase 1：低风险高收益（1~2 周）

| 任务 | 方案章节 | 预期收益 |
|------|---------|---------|
| L0 Snapshot 简化（解耦 EvolutionMetricsEnabled，精简 segments_json） | §2.1 | 降低维护复杂度 |
| Prompt Caching 三层前缀分离 | §2.3 | 每轮节省 8K~15K 输入 token 成本 |
| 显式压缩预算 | §2.4 | 避免被动压缩信息损失 |
| Embedding 单写确认 + 删除 memory_l2_index_meta | §4.3 | 消除冗余设计 |

### Phase 2：核心优化（2~3 周）

| 任务 | 方案章节 | 预期收益 |
|------|---------|---------|
| 三级压缩流水线（Micro → Structured → LLM） | §2.5 | 90%+ 压缩场景零 LLM 成本 |
| L1 选择性注入 | §3.2 | 每轮节省 2K~8K tokens |
| 无感后台压缩 + UI 指示器 | §2.5 | 用户体验对标 Trae/Cursor |
| 结构化 Episode 优先路径 | §4.1 | 70%~90% Episode 生成零 LLM 成本 |

### Phase 3：架构优化（3~4 周）

| 任务 | 方案章节 | 预期收益 |
|------|---------|---------|
| 单跳巩固管道 + 删除 L2ConsolidateWorker | §4.2 | LLM 调用从 2 次降为 1 次 |
| L1 Schema/History 降为可选 | §3.1 | 降低大多数 Agent 的 L1 开销 |
| L1 与框架 Memory 职责厘清 | §3.3 | 消除 Agent 工具混淆 |
| 摘要保留/丢弃策略 | §6.1 | 提高摘要信噪比 |

### Phase 4：高级特性（4~6 周）

| 任务 | 方案章节 | 预期收益 |
|------|---------|---------|
| 多代理加密上下文传递 | §6.2 | 多代理上下文完整性保护 |
| L2 Episode 分叉 | §6.3 | 多代理探索场景支持 |
| L3/L4 Embedding 增量重建 | §4.3.3/4.3.4 | 避免全量重建 |
| 子目录规则按需加载 | §6.5 | 每轮节省 2K~5K tokens |
| L3 Recall 去重 | §5.1 | 减少 recall 噪声 |

---

## 附录 A：竞品参考

| 借鉴来源 | 借鉴内容 | 对应方案章节 |
|---------|---------|-------------|
| Codex CLI | 结构化 Session Memory 优先路径（零成本压缩） | §2.5 Level 2 |
| Claude Code | Prompt Caching 前缀分离 | §2.3 |
| Claude Code | 显式压缩预算（33K 缓冲区） | §2.4 |
| Claude Code | MicroCompact → AutoCompact 三级压缩 | §2.5 |
| Claude Code | `/compact` 带指令 | §6.1 |
| Trae | 后台无感压缩 + UI 指示器 | §2.5 |
| Cursor | Dynamic Context Discovery（按需加载） | §6.5 |
| Codex CLI | 加密压缩（防篡改） | §6.2 |
| Codex CLI | Thread 分叉/回滚 | §6.3 |
| Cursor | Merkle 树增量索引 | §4.3.3 |

## 附录 B：Token 效率对比

| 维度 | 优化前 | 优化后 | 改善 |
|------|--------|--------|------|
| 每轮系统提示词 token | 8K~15K（无缓存） | 8K~15K（缓存命中率 80%） | 输入成本降 50%~80% |
| L1 注入 token | 2K~8K（全量） | 0.5K~4K（选择性） | 降低 50%~75% |
| Episode 生成 LLM 调用 | 每任务 1 次 | 10%~30% 任务 1 次 | 降低 70%~90% |
| 巩固 LLM 调用 | 2 次/Episode | 1 次/消息批次 | 降低 50% |
| 压缩 LLM 调用 | 每次压缩 1 次 | 90%+ 场景零调用 | 降低 90%+ |
| Skill 文件 token | 全量 2K~5K | 按需 0.5K~2K | 降低 50%~75% |

---

## 附录 C：勘误与代码验证

> 本附录记录方案文档经代码验证后发现的错误和修正。

### C.1 已修正的错误

| # | 原始描述 | 实际情况 | 修正 |
|---|---------|---------|------|
| 1 | "当前 `budget = contextWindow * 0.85`" | 代码中不存在此计算，压缩触发完全基于 `ContextUsedRatio` 与阈值比较 | §2.4 现状已修正 |
| 2 | "无 hard trigger 同步压缩机制" | 代码已有 `BeforeDurableTurn`（同步）和 `CompactSession`（同步），以及 `atFullContextUsage`（绕过防抖） | §2.4 现状已修正，§2.4.3 三级触发阈值复用现有机制 |
| 3 | "L2ConsolidateWorker 仅标记 done 不做 LLM 提取" | 该 Worker 因 `agentID=""` bug 完全查不到 episode，是彻底的空转；且与 AutoMemoryWorker 的状态值不一致（`"done"` vs `"consolidated"`） | §4.2 现状已补充 |
| 4 | "AutoMemoryWorker 创建 kind='consolidation' 的 episode" | AutoMemoryWorker 设置的是 `ConsolidationStatus="consolidated"`，`episode_kind` 仍为默认的 `"task"` | §4.1 已修正 |

### C.2 关键发现

| # | 发现 | 影响 |
|---|------|------|
| 1 | **框架已完整支持 Anthropic Prompt Caching**（`WithCacheSystemPrompt/WithCacheTools/WithCacheMessages`），项目层已集成配置 | §2.3 方案可大幅简化——只需配置启用 + 调整 system message 顺序，无需从零实现 |
| 2 | **Anthropic 最多 4 个缓存断点**，当前已用 3 个 | 三层前缀分离需在 system 部分增加 1 个断点，刚好达到上限，无法再添加更多 |
| 3 | **BeforeModel hooks 以 prepend 方式注入**，动态内容排在 system TextBlock 数组前面，静态内容排在后面 | `cache_control` 标记在最后一个 TextBlock（静态内容），但前面的动态内容变化也会导致缓存失效。需调整注入顺序 |
| 4 | **L1 的 `used_tokens` 和 `budget_tokens` 字段从未被读取或更新** | 设计文档中的预算检查逻辑完全未实现，§3.2 的选择性注入方案需要先实现基础的 token 预算追踪 |
| 5 | **`memory_l2_index_meta` 表无任何 Go 代码引用** | 确认可安全删除，§4.3 方案无误 |

### C.3 业务合理性评估

| 方案 | 业务合理性 | 风险 |
|------|-----------|------|
| §2.1 L0 Snapshot 简化 | 合理——保留诊断功能，降低存储开销 | 低——仅精简字段，不删功能 |
| §2.2 摘要注入模式 | 合理——无需改动，文档说明即可 | 无 |
| §2.3 三层前缀分离 | 合理——框架已支持缓存，只需调整顺序 | 中——需调整 system message 注入顺序，可能影响其他 BeforeModel hooks |
| §2.4 显式压缩预算 | 合理——复用现有同步/异步路径，增加分级触发 | 低——在现有机制上扩展 |
| §2.5 无感后台压缩 | 合理——三级流水线从零成本到有成本渐进 | 中——Level 2 Structured Compact 的"结构化数据 < 原始历史 30%"阈值需实际验证 |
| §3.1 L1 表轻量化 | 合理——不删表，降为可选 | 低 |
| §3.2 L1 选择性注入 | 合理——但需先实现 `used_tokens` 追踪 | 中——依赖未实现的基础设施 |
| §3.3 L1 与框架 Memory 厘清 | 合理——互斥规则清晰 | 低 |
| §4.1 结构化 Episode | 合理——零成本路径优先，LLM 增强按需 | 低——高重要性判断标准可调 |
| §4.2 单跳巩固 | 合理——删除无效 Worker，统一状态值 | 低——L2ConsolidateWorker 本就无效，删除无影响 |
| §4.3 Embedding 单写 | 合理——删除未使用的表和设计 | 低 |
| §6.1 摘要保留/丢弃策略 | 合理——提高摘要信噪比 | 低 |
| §6.2 加密上下文传递 | 合理——HMAC 轻量，AES 可选 | 低 |
| §6.3 Episode 分叉 | 合理——多代理探索场景 | 低 |
| §6.5 子目录规则按需加载 | 合理——减少无关 Skill 注入 | 中——文件路径提取依赖正则，可能遗漏 |

### C.4 需要额外验证的假设

| # | 假设 | 验证方法 |
|---|------|---------|
| 1 | Level 2 Structured Compact 的结构化摘要通常 < 原始历史 30% | 在生产环境统计 L1 快照 token vs 对话历史 token 的比例 |
| 2 | 12% 压缩缓冲区足以覆盖异步压缩期间的增量 | 在生产环境监控 soft_trigger 到压缩完成期间的 token 增量 |
| 3 | 调整 system message 注入顺序不影响其他 BeforeModel hooks | 梳理所有 BeforeModel hooks 的优先级和依赖关系 |
| 4 | L1 选择性注入的 `read_count` 排序能反映字段相关性 | A/B 测试：对比全量注入 vs 选择性注入的模型输出质量 |
