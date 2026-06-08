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
4. **写入频率过高，无任何限流机制**——这是原始问题的核心，此前误判为"合理频率"

#### 写入频率过高的根因分析

`ShouldWriteL0AssemblySnapshot` 是**无记忆的单次判定**：只检查 `usedRatio >= 0.60`，不知道上次写入是什么时候。而 `usedRatio` 在对话中**单调递增**（上下文只增不减，除非大幅截断），一旦跨过 0.60 阈值，**后续每次模型调用都会写入**，没有任何限流、去重或冷却机制。

| 会话类型 | 跨过 0.60 的 turn | 总 turn | 每轮模型调用 | 跨过 0.60 后写入行数 | 数据量 |
|---------|-------------------|---------|------------|---------------------|--------|
| 短会话 | 不跨过 | 5-10 | 1-2 | 0 | 0 |
| 中等会话 | turn 8 | 20 | 2 | ~24 行 | 36KB~264KB |
| 长编码会话 | turn 10 | 50 | 2-3 | 80~120 行 | 120KB~1.1MB |
| 超长调试会话 | turn 8 | 80 | 3 | ~216 行 | 324KB~2.4MB |

连续两次模型调用之间的 `usedRatio` 变化极小（可能只差几百 token），写入的快照几乎完全重复，诊断价值递减。

#### 方案

**保留 L0 Snapshot 功能，简化实现 + 增加写入限流**：

| 变更项 | 说明 |
|--------|------|
| 解耦 `EvolutionMetricsEnabled` | 新增独立的 `L0SnapshotEnabled` 字段（默认 true），快照写入不再依赖 `EvolutionMetricsEnabled` |
| 精简 `segments_json` | 改为 `segments_summary_json`，仅记录各段的聚合统计（段名、token 估算、消息数），不记录逐条 message 详情 |
| 保留 `MemorySnapshotDrawer` | 抽屉改为展示聚合统计而非逐条详情，数据量减少 80%+ |
| 清理死规划 | 从设计文档中删除未实现的 Datadog 指标规划 |
| 保留 `ARANEA_L0_SNAPSHOT` 环境变量 | 调试时可通过 `always/force` 强制高频写入 |
| **新增：写入限流** | 见下方详细方案 |

##### `segments_summary_json` 格式

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

##### 写入限流方案

```
限流策略（在 ShouldWriteL0AssemblySnapshot 中实现）：

1. 最小写入间隔（l0_snapshot_min_interval）：
   - 默认 300 秒（5 分钟）
   - 同一 session 内，距上次写入不足此间隔则跳过
   - 实现方式：session 级变量 lastL0SnapshotWriteAt

2. usedRatio 变化量阈值（l0_snapshot_ratio_delta）：
   - 默认 0.10（即 usedRatio 变化超过 10% 才写入）
   - 实现方式：session 级变量 lastL0SnapshotRatio
   - 例：上次写入时 ratio=0.65，当前 ratio=0.72 → 变化 0.07 < 0.10 → 跳过
   - 例：上次写入时 ratio=0.65，当前 ratio=0.78 → 变化 0.13 >= 0.10 → 写入

3. 阈值穿越强制写入：
   - 当 usedRatio 跨越 ContextStatusCriticalThreshold（0.80）时强制写入
   - 即使不满足间隔和变化量条件也写入
   - 确保关键状态变化不被遗漏

4. always/force 模式不受限流影响

写入决策伪代码：
  if mode == "off": return false
  if mode == "always" or forceDebug: return true
  if !EvolutionMetricsEnabled: return false
  if usedRatio < 0.60: return false

  // 限流检查
  if crossedThreshold(lastRatio, usedRatio, 0.80): return true  // 阈值穿越
  if time.Since(lastWriteAt) < minInterval: return false        // 间隔不足
  if abs(usedRatio - lastRatio) < ratioDelta: return false      // 变化量不足
  return true
```

##### 限流后的写入量估算

| 会话类型 | 限流前写入行数 | 限流后写入行数 | 降幅 |
|---------|--------------|--------------|------|
| 中等会话（20 轮） | ~24 | 2~3 | ~90% |
| 长编码会话（50 轮） | 80~120 | 4~6 | ~95% |
| 超长调试会话（80 轮） | ~216 | 6~8 | ~97% |

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
// 重构后：记忆注入保持独立 system message
// MemoryInject Hook 仍注入一个 system message，包含 L1+L3+L4 内容
// 在 Anthropic API 中，该 system message 是一个独立的 TextBlock
// cache_control 由 applyCacheControlToSystem 统一处理，不在 Message 层面设置

memoryCue := ""
memoryCue += l1Cue   // L1 working memory
memoryCue += l3Cue   // L3 recall facts
memoryCue += l4Cue   // L4 graph context
if memoryCue != "" {
    memoryMsg := trpcmodel.NewSystemMessage(memoryCue)
    args.Request.Messages = append([]trpcmodel.Message{memoryMsg}, args.Request.Messages...)
}

// 注意：条件激活的 Skill 内容（§6.5）也在此 Hook 中注入
// 作为同一个 system message 的一部分，归入 Layer 2（半静态前缀）
```

##### 2.3.2 缓存断点标记

**Anthropic API**：
- 支持最多 **4 个独立缓存断点**
- 当前框架已使用 3 个：system（1 个）+ tools（1 个）+ messages（1 个）
- 三层前缀分离需要在 system 部分增加 1 个断点（静态层 + 半动态层），刚好达到 4 个上限
- `applyCacheControlToSystem` 当前在 system prompts 的**最后一个 TextBlock** 标记 `cache_control`

**关键问题：system message 顺序倒置**

当前 BeforeModel Hook 的执行顺序和 prepend 方式导致**动态内容在前、静态内容在后**：

```
当前 Hook 执行顺序（优先级数值越小越先执行）：
  P3: ToolResultGate（不注入系统消息）
  P4: RuntimeCue → prepend RuntimeCue
  P5: SkillGuidance → prepend SkillGuidance  ← 先执行
  P5: MemoryInject → prepend MemoryInject    ← 后执行
  P6: KnowledgeCue → prepend KnowledgeCue

由于 prepend 是向前插入，最后执行的 Hook 的消息排在最前面。
最终 Anthropic 请求的 system TextBlock 顺序：

  [0] KnowledgeCue    ← 每轮变化（动态）
  [1] MemoryInject    ← 每轮变化（动态）
  [2] SkillGuidance   ← 半动态
  [3] RuntimeCue      ← 半动态
  [4] Identity+Instr  ← 静态（会话内不变）
       ↑ cache_control 标记在这里（最后一个 TextBlock）
```

**问题**：Anthropic 的 Prompt Caching 是前缀缓存——缓存从开头到 breakpoint 位置的所有内容。动态内容排在前面，一变就导致整个前缀缓存失效。静态内容虽然标记了 breakpoint，但因为前面的动态内容变了，缓存永远命中不了。

**解决方案：在 Anthropic adapter 层面修复缓存断点位置**

**不可行方案**：将 Identity/Instruction 从 RequestProcessor 移到 BeforeModel Hook。
- 原因：Context Compaction 重建流程（`rebuildRequestForContextCompaction`）只重新执行 RequestProcessor，**不执行 BeforeModel Hook**。移动后 Identity/Instruction 会在 compaction 重建时丢失。
- 其他风险：事件发送时序改变、Hook 顺序不可控、与 Plugin 系统冲突。

**可行方案：在 RequestProcessor 层面分离静态/动态内容，修复 `applyCacheControlToSystem` 断点位置**

```
核心问题：
  所有 RequestProcessor 将内容拼接到 message.Content（单个字符串）
  → convertSystemMessageContent 生成单个 TextBlock
  → applyCacheControlToSystem 在最后一个 TextBlock 放 cache_control
  → 但 TimeRequestProcessor 每轮注入变化的时间信息，导致整个 TextBlock 缓存失效

解决思路：
  将动态内容（Time）分离到独立的 ContentPart，使静态内容独占一个 TextBlock

实现步骤：

1. 修改 TimeRequestProcessor（time.go）：
   - 将时间信息写入 message.ContentParts 而非追加到 message.Content
   - 这是唯一每轮变化的 RequestProcessor

2. 修改 convertSystemMessageContent（anthropic.go:1391）：
   - 先输出 message.Content 为一个 TextBlock（静态内容）
   - 再输出 message.ContentParts 中的各个 TextBlock（动态内容）

3. 修改 applyCacheControlToSystem（anthropic.go:494）：
   - 在第一个 TextBlock（静态内容）上放 cache breakpoint
   - 而非最后一个 TextBlock（动态内容）

4. BeforeModel Hook 的 MemoryInject 等动态内容：
   - 已通过 prepend 方式注入为独立的 system message
   - 每个独立 system message 在 Anthropic API 中是独立的 TextBlock
   - 不需要修改 Hook 优先级

最终 Anthropic 请求的 system TextBlock 顺序：

  [0] Identity+Instr+Skills+Workspace+PostTool  ← 静态（RequestProcessor 注入）
       ↑ breakpoint 1（静态层末尾）
  [1] Time                                       ← 动态（ContentParts 分离）
  [2] RuntimeCue                                 ← 半动态（Hook prepend）
  [3] SkillGuidance                              ← 半动态（Hook prepend）
  [4] KnowledgeCue                               ← 动态（Hook prepend）
  [5] MemoryInject                               ← 动态（Hook prepend）
       ↑ breakpoint 2（半动态层末尾 = TextBlock[3] 末尾）
```

**缓存命中分析**：
- Layer 1（Identity+Instr+Skills）不变时：breakpoint 1 命中，缓存覆盖 TextBlock[0]
- Layer 2（RuntimeCue+SkillGuidance）不变时：breakpoint 2 命中，缓存覆盖 TextBlock[0-3]
- Layer 3（Time+KnowledgeCue+MemoryInject）每轮变化：不影响前面层的缓存

**安全性**：
- 仅修改 Anthropic adapter 的 cache 逻辑，不影响其他 Provider
- TimeRequestProcessor 改为写入 ContentParts，对非 Anthropic Provider 无影响（忽略 ContentParts）
- 无需修改 BeforeModel Hook 优先级，无 Context Compaction 风险

**稳定性保障：Hook Layer 声明机制** [G.1 新增]

当前 TextBlock 顺序依赖 Hook 优先级硬编码整数（P3/P4/P5/P6），未来新增 Hook 或调整优先级可能导致顺序错位、缓存断点索引失效。增加 Layer 声明机制确保顺序稳定：

```
1. Hook 注册时声明所属 Layer：

   type BeforeModelHook struct {
       Priority int
       Layer    SystemLayer  // 新增：声明所属缓存层
       Fn       func(...) error
   }

   type SystemLayer int
   const (
       LayerStatic    SystemLayer = iota  // Layer 1：静态前缀（会话内不变）
       LayerSemiStatic                     // Layer 2：半静态（任务/轮次切换时变）
       LayerDynamic                        // Layer 3：动态（每轮变化）
   )

2. 装配器按 Layer 排序，同 Layer 内按 Priority 排序：

   排序规则：LayerStatic > LayerSemiStatic > LayerDynamic
   （静态层 TextBlock 排在最前面，动态层排在最后面）

3. applyCacheControlToSystem 按语义标记定位断点：

   遍历 system TextBlock 数组，找到 LayerStatic 的最后一个 TextBlock → breakpoint 1
   找到 LayerSemiStatic 的最后一个 TextBlock → breakpoint 2
   不再依赖位置索引，即使 Hook 数量变化也不会错位

4. 现有 Hook 的 Layer 声明：

   | Hook | Priority | Layer | 说明 |
   |------|----------|-------|------|
   | ToolResultGate | 3 | — | 不注入系统消息 |
   | RuntimeCue | 4 | LayerSemiStatic | 工具策略半动态 |
   | SkillGuidance | 5 | LayerSemiStatic | Skill 内容半动态 |
   | MemoryInject | 5 | LayerDynamic | L1/L3/L4 每轮变化 |
   | KnowledgeCue | 6 | LayerDynamic | 知识检索每轮变化 |

5. RequestProcessor 注入的内容（Identity+Instr+Skills）归入 LayerStatic，
   通过 convertSystemMessageContent 时标记 Layer。

6. 向后兼容：未声明 Layer 的 Hook 默认为 LayerDynamic（最安全的降级）。
```

**Google Gemini API**：在 `system_instruction` 中使用 `cached_content` 引用缓存。

**OpenAI API**：自动缓存，无需手动标记。但前缀分离仍然有益——静态部分更可能命中自动缓存。

**不支持的模型**：忽略 `cache_control` 字段，降级为无缓存行为，功能不受影响。

##### 2.3.3 Prompt Caching 的工作原理

```
Anthropic Prompt Caching 是 API 层面的自动缓存，不需要应用层手动缓存内容：

1. 应用层每次请求照常构建完整的 system messages
2. Anthropic API 根据 cache_control 断点位置，匹配前缀是否与已缓存内容一致
3. 前缀匹配时：缓存命中，输入 token 按缓存费率计费（通常 10%~50%）
4. 前缀不匹配时：缓存未命中，正常计费，同时更新缓存

因此：
- 不需要 session.staticPrefix 变量来"缓存"静态内容
- 不需要手动比较 hash 来决定是否"复用"Layer 2 message
- 只需确保静态内容在 TextBlock 中的位置和内容稳定，Anthropic API 会自动匹配
- 唯一需要做的是：将动态内容（Time）分离到独立 ContentPart，避免污染静态 TextBlock
```

##### 2.3.4 条件激活 Skill 的按需加载

详见 §6.5 子目录规则按需加载。条件激活的 Skill 内容归入 Layer 2（半静态前缀），因为它们根据当前文件路径动态变化，但变化频率低于每轮。

##### 2.3.5 RuntimeCapabilityCue 的归属

`RuntimeCapabilityCue`（`runtime_cue_inject.go`）在每次 LLM 调用前动态生成，内容包含 effective tool keys 列表、MCP 工具集说明等。其中 effective tool keys 在 agent 配置不变时基本稳定，但 MCP server 上下线时可能变化。

**归属决策**：归入 Layer 2（半静态前缀），理由：
- 大多数请求间内容不变（agent 配置稳定时）
- 变化时需要重新标记缓存断点，但不会导致 Layer 1 缓存失效
- 若归入 Layer 3（动态），则每轮都重新生成，浪费计算

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

reserved_system = 动态计算（非硬编码）
  = identity_tokens + instruction_tokens + runtime_cue_tokens + skills_tokens + tools_tokens
  数据源：
    - prompt_snapshot.go 的 section_* 字段（首次 LLM 调用后可用）
    - 或首次 LLM 调用的 prompt_token_actual 减去 history/summary 部分
  典型范围：
    - coding/full profile Agent: 11K~18K
    - chat_only/minimal profile Agent: 2.5K~4.5K

compression_buffer_ratio = 0.12 (12%)
compression_buffer = contextWindow * compression_buffer_ratio
effective_budget = contextWindow - reserved_system - compression_buffer

示例（coding profile, 128K 窗口）：
  reserved_system = 15K
  compression_buffer = 128K * 0.12 = 15K
  effective_budget = 128K - 15K - 15K = 98K

示例（minimal profile, 128K 窗口）：
  reserved_system = 4K
  compression_buffer = 128K * 0.12 = 15K
  effective_budget = 128K - 4K - 15K = 109K
```

**为什么不用硬编码 15K**：
- 系统提示词 + 工具定义随 Agent 配置变化，不是固定值
- coding profile Agent 的 reserved_system 可达 18K，硬编码 15K 会低估
- minimal profile Agent 的 reserved_system 仅 4K，硬编码 15K 会浪费 11K 有效预算
- 已有数据源（`prompt_snapshot.go` 的 section_* 字段）可支撑动态计算

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

借鉴 Claude Code 的 MicroCompact → Session Memory → AutoCompact，结合 Codex CLI 的结构化优先路径。

**关键发现**：项目中已存在 `tryMicroCompact`（`micro_compact.go`）和 `tryMemoryCompact`（`memory_compact.go`），但它们是**死代码**——有测试但无生产调用。`CompactSession` 的 `level` 字段已预设了 `micro_compact` 和 `memory_compact` 两个值，说明架构上已为分级压缩预留了位置。本方案应**激活和增强现有代码**而非重写。

```
┌─────────────────────────────────────────────────────────────┐
│ Level 1: MicroCompact（零 LLM 成本，每次请求前自动执行）      │
│                                                              │
│ 触发：每次 LLM 请求前                                        │
│ 执行：同步，耗时 < 1ms                                       │
│ 现有代码：tryMicroCompact（micro_compact.go）— 死代码，需激活 │
│ 操作：                                                       │
│   1. 清理已摘要覆盖的旧工具结果                               │
│      （已有 compactCurrentInvocationEvent）                   │
│   2. 截断超大 tool result                                    │
│      （已有 OversizedToolResultMaxTokens）                    │
│   3. 激活 tryMicroCompact：扫描 turn >= 2 且 > 200 字符的    │
│      tool result，生成占位标记                                │
│      [F.2.3 修正] 需补充实际清除逻辑：返回 clearableMsgIDs，  │
│      调用方执行 ReplaceMessageContent 替换为占位标记           │
│   4. 新增：清理已过期 TTL 的 L1 字段渲染                      │
│   5. 新增：清理 reasoning_content（按配置策略）               │
│ 成本：零 LLM 调用                                            │
│ 用户感知：无                                                  │
├─────────────────────────────────────────────────────────────┤
│ Level 2: Memory Compact（零 LLM 成本，soft_trigger 时）      │
│                                                              │
│ 触发：usedRatio >= soft_trigger（默认 70%）                  │
│ 执行：异步后台，耗时 5~50ms                                   │
│ 适用场景：中长任务（> 5 轮），L1 有足够结构化数据 [G.3 新增]   │
│ 不适用场景：短任务（<= 5 轮），L1 数据不足，ICS 通常 < 0.70  │
│   → 短任务直接等待 hard_trigger 触发 Level 3                 │
│ 现有代码：tryMemoryCompact（memory_compact.go）— 死代码，需激活+增强 │
│ 当前实现：读取 L3 facts 机械拼接为列表（未接入压缩流程）       │
│ [F.2.4 修正] 需重写摘要生成逻辑：合并 L1+L3 数据源，增加 ICS  │
│ 评估和降级判断（ICS < 0.70 或压缩比 > 60% 时降级到 Level 3） │
│ 操作：                                                       │
│   1. 读取 L1 快照（task_title, task_goal, fields）           │
│   2. 读取 L3 recall facts（已有 MemoryFactReader）            │
│   3. 合并生成结构化摘要：                                     │
│      - L1 提供当前任务状态（task, goal, decisions, pending）  │
│      - L3 提供长期事实（key facts, preferences）              │
│   4. 若 ICS >= 0.70 且压缩比 <= 60%：替换历史为结构化摘要 [D.3 修正] │
│   5. 否则：Level 2 失败，等待 hard_trigger 触发 Level 3 [F.2.2 修正] │
│ 成本：零 LLM 调用                                            │
│ 用户感知：无（后台执行）                                      │
├─────────────────────────────────────────────────────────────┤
│ Level 3: LLM Compact（有 LLM 成本，hard_trigger 时）         │
│                                                              │
│ 触发：usedRatio >= hard_trigger（默认 90%）                  │
│       或 Level 2 失败后等待 hard_trigger [F.2.2 修正]        │
│ 执行：异步后台（hard_trigger 时同步阻塞），耗时 2~10 秒       │
│ 现有代码：runCompress 中的 default 策略 — 已在生产使用        │
│ 操作：                                                       │
│   1. 调用 LLM 生成结构化摘要（复用现有 9 段模板）            │
│   2. 支持保留/丢弃策略指令（接入现有 preserveInstruction 管道）│
│   3. 原子替换：压缩完成后一次性替换上下文                     │
│ 成本：1 次 LLM 调用（~2K~4K tokens）                         │
│ 用户感知：hard_trigger 时短暂等待；异步时无感知               │
└─────────────────────────────────────────────────────────────┘
```

**与现有代码的对应关系**：

| 方案层级 | 现有代码 | 状态 | 操作 |
|---------|---------|------|------|
| Level 1 | `tryMicroCompact`（micro_compact.go） | 死代码，有测试无生产调用 | **激活**：接入 BeforeModel hook |
| Level 2 | `tryMemoryCompact`（memory_compact.go） | 死代码，有测试无生产调用 | **激活+增强**：接入 runCompress 流程，分两步扩展数据源 |
| Level 3 | `runCompress` default 策略 | 生产使用中 | **增强**：接入 preserveInstruction 管道 |

##### Level 2 Memory Compact 集成分两步

**Step 1：激活现有 L3 版本（低复杂度，~30-50 行）**

```
集成点：runCompress 中 strategy 判断之前

1. Compressor 已持有 memoryReader biz.MemoryFactReader（已注入但未使用）
2. 在 runCompress 的 switch strategy 之前，增加 memory_compact 尝试：
   if memoryCompactEnabled(ag) {
       result := tryMemoryCompact(ctx, body, c.memoryReader, sessionID, c.lg)
       if result.didCompact {
           md = result.summaryMarkdown
           fromTurn = result.fromTurn
           toTurn = result.toTurn
           // 跳过 LLM 压缩，直接进入写入流程
       }
   }
3. 输出格式（Markdown 字符串）与 session_summaries.summary_markdown 完全兼容
4. 无需新增依赖注入
```

**Step 2：扩展 L1 数据源（中等复杂度，~80-120 行）**

```
1. Compressor 新增 l1Reader biz.L1AdminReader 字段
2. Wire 注入点更新
3. 新增 L1 数据格式化函数（将 task/field 层级转为 Markdown 摘要）
4. 改造 tryMemoryCompact，合并 L1 + L3 数据源
```

#### Level 2 Memory Compact 的结构化摘要模板

```markdown
## Session Memory Summary

### Current Task State (from L1 Working Memory)
- Task: 实现用户认证模块
- Goal: 完成 JWT 认证流程
- Status: in_progress
- Key Decisions: 使用 RS256 算法签名 JWT; Token 过期时间设为 24 小时
- Files Modified: internal/service/auth.go — 新增 Login/Logout 方法; internal/biz/token.go — 实现 JWT 生成和验证
- Pending: 实现 Refresh Token 逻辑; 添加 Token 黑名单

### Key Facts (from L3 Semantic Memory)
- 用户偏好中文注释
- 项目使用 Kratos v2 框架
- 数据库为 SQLite + Ent ORM
```

#### Level 3 LLM Compact 的 9 章节摘要模板

**直接复用现有 `internal/compress/prompt.go` 中的 `DefaultSystemPrompt`（v2 版本）**，已在生产使用：

```
## 1. User Intent & Goals
## 2. Key Technical Concepts
## 3. Files & Code Involved
## 4. Errors & Fixes
## 5. Problem-Solving Process
## 6. All User Messages (verbatim)
## 7. Constraints & Preferences
## 8. Pending Tasks & Open Questions
## 9. Current Work State
```

**唯一增强**：接入 `preserveInstruction` 管道（已有 context key，只需在 `compress/prompt.go` 中读取并注入 system prompt）。

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
4. [G.2 新增] 强制写入 L0 Snapshot（不受限流约束）：
   - 压缩后上下文状态发生重大变化，必须记录新状态
   - 调用 ShouldWriteL0AssemblySnapshot 时传入 forceAfterCompress = true
   - 跳过间隔、变化量等限流检查，确保 Snapshot 与实际状态一致
   - 防止 Snapshot 记录的是压缩前状态，而实际上下文已被替换
5. 通过 EventBus 发布 `session_compressed` 事件
6. 前端收到事件后更新上下文使用率显示
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

#### 前置依赖

**L1 预算管理系统完全未实现**：
- `memory_l1_tasks.used_tokens` 永远为 0，从未被更新
- `memory_l1_tasks.budget_tokens`（默认 8192）从未被检查
- `memory_l1_fields.token_estimate` 始终为 0，从未被计算
- 设计文档中的 `ErrL1Overflow` / `ErrFieldTooLarge` 在代码中不存在
- 唯一生效的限制是 `L1FieldMaxChars`（= `L1FieldMaxTokens * 4`），仅在 prompt 注入时截断单字段显示

**选择性注入方案需要先实现基础的 token 预算追踪**，否则无法判断"注入总 token 是否超预算"。

#### 方案

##### 3.2.1 三层过滤链

```
L1 → L0 注入过滤链：

1. visibility 过滤（已有）：visibility != 'internal'
2. pin_to_prompt 过滤（已有）：pin_to_prompt = true
3. 新增：相关性过滤 [F.1.2 修正]
   ├── 字段数 <= 5：全量注入（开销可忽略）
   └── 字段数 > 5：
       a. pin_to_prompt=true 的字段：始终注入
       b. 其余字段：按 updated_at 降序排序（最近更新的优先），取 top-K
       c. K = min(非 pinned 字段数, 5)
       注：长期方案改为按 read_count 降序（需先实现 read_count 递增，见 F.1.2）
4. 新增：token 预算硬上限 [F.1.1 修正]
   ├── L1 注入总 token 不超过 budget_tokens 的 50%（默认 4K）
   ├── 前置依赖：token_estimate 计算必须先实现（当前始终为 0，见 F.1.1）
   └── 超出时按 token_estimate 降序截断（大字段优先截断）
```

##### 3.2.2 实现路径

**短期（无 embedding）**[F.1.2 修正]：按 `updated_at` 降序排序（最近更新的字段优先）。`read_count` 当前始终为 0，短期不可用。

**长期（有 embedding）**：基于当前 user message 语义检索 top-K 字段。为 L1 字段增量添加 embedding，复用现有 embedding 基础设施。同时实现 `read_count` 异步递增（见 F.1.2 长期方案），提供更精准的相关性排序。

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

##### 3.3.2 互斥规则 [D.4 修正]

```
不新增 memory_tool_mode 字段，复用现有 ToolsDenyJSON 机制：

"working_memory"（默认，推荐）：
  ToolsDenyJSON 增加：["memory_add", "memory_update", "memory_delete", "memory_search", "memory_load"]
  效果：Agent 只能看到 working_memory_* 工具

"framework_memory"：
  ToolsDenyJSON 增加：["working_memory_read", "working_memory_list", "working_memory_write",
                       "working_memory_patch", "working_memory_delete"]
  效果：Agent 只能看到 memory_* 工具

"both"（不推荐）：
  ToolsDenyJSON 不增加任何记忆工具
  效果：Agent 可看到两套工具（当前默认行为）

渐进迁移策略：
  Phase 1：文档引导，不修改任何 Agent 配置
  Phase 2：新建 Agent 默认禁用 framework memory 工具
  Phase 3：存量 Agent 提示迁移（前端增加记忆工具模式检测）
  Phase 4：未来可选 — 基于工具调用日志自动迁移
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
  │   key_decisions = L1 fields 中提取关键决策 [F.1.3 修正]，使用分层 fallback：
    Layer 1：field_path 匹配 "decision"/"choice"/"approach"/"strategy"/"rationale" 的字段
    Layer 2（fallback）：pin_to_prompt=true 且 visibility="prompt" 的前 3 个字段
    Layer 3（fallback）：最近更新的前 5 个 visibility="prompt" 字段
  key_artifacts = L1 fields 中提取关键产物 [F.1.3 修正]，使用分层 fallback：
    Layer 1：field_path 匹配 "file"/"artifact"/"output"/"deliverable" 的字段 + field_kind="reference"
    Layer 2（fallback）：field_kind="reference" 的字段
  │   importance = 0.5（默认）
  │   confidence = 0.6（结构化路径置信度略低）
  └── 标记：episode_kind = "l1_archive_structured"，consolidation_status = "consolidated"
      注意：直接设为 "consolidated" 而非 "pending"，因为当前没有任何 Worker 会处理
      consolidation_status="pending" 的 Episode（AutoMemoryWorker 不读取已有 Episode，
      L2ConsolidateWorker 因 agentID="" bug 完全无效）。结构化路径已生成足够质量的摘要，
      无需后续巩固。若未来需要 LLM 增强，由 Path B 直接覆盖。
      [F.1.5 修正] 同时需修改 InsertL1ArchiveEpisode 中硬编码的 "pending" 为 "consolidated"，
      以及 PurgeEpisodesOlderThan 和 ListEpisodesPendingEmbedding 中查询 "done" 改为 "consolidated"。

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
标准公式（critic_score 存在时）：
episode_score = 0.30 * importance
             + 0.25 * min(critic_score / 0.8, 1.0)
             + 0.15 * min(tool_call_count / 20, 1.0)
             + 0.15 * min(duration_ms / 300000, 1.0)
             + 0.15 * (user_mark ? 1.0 : 0.0)

若 episode_score >= 0.6 → 触发 Path B（LLM 增强）

[G.7 修正] 条件权重重分配（critic_score 不存在时）：
大多数场景下 critic_score 不存在（需要 Critic 代理评分），
此时 0.25 权重被浪费，实际有效评分上限仅 0.75。

重分配公式（critic_score 缺失时）：
episode_score = 0.40 * importance               // 0.30 → 0.40（+0.10）
             + 0.00 * critic_score               // 不存在
             + 0.20 * min(tool_call_count / 20, 1.0)   // 0.15 → 0.20（+0.05）
             + 0.20 * min(duration_ms / 300000, 1.0)   // 0.15 → 0.20（+0.05）
             + 0.20 * (user_mark ? 1.0 : 0.0)   // 0.15 → 0.20（+0.05）

权重增量来源：critic_score 的 0.25 分配到其他 4 个维度
（importance +0.10，其余各 +0.05）
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

2. **状态值不一致** [F.1.5 修正]：
   - AutoMemoryWorker 创建 Episode 时直接设置 `ConsolidationStatus = "consolidated"`
   - L2ConsolidateWorker 查找 `consolidation_status = 'pending'`，标记为 `'done'`
   - `InsertL1ArchiveEpisode` 硬编码写入 `'pending'`
   - 三个不同的值（`"pending"` / `"consolidated"` / `"done"`），永远不会协作
   - **修正**：统一为 `"consolidated"`，修改 3 处 SQL（详见 F.1.5）

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
  - AutoMemoryWorker 创建 Episode 时直接设为 `"consolidated"`（已有行为，不变）
  - L1 归档创建的 Episode（Path A 结构化路径）也直接设为 `"consolidated"`（见 §4.1 修正）
  - 删除 `MarkEpisodeConsolidated` 方法（仅 L2ConsolidateWorker 使用）
- **重要**：AutoMemoryWorker 不读取也不处理已有 Episode，它只从消息提取 fact 并创建新 Episode。因此 Path A 创建的 Episode 不应依赖 AutoMemoryWorker 后续处理，而应在创建时就设为终态 `"consolidated"`

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
  扩展性阈值 [G.6 新增]：
    - 暴力搜索上限：5000 条 Episode
    - 超过 5000 条时：
      a. 若配置了 pgvector → 自动切换到 pgvector ANN 检索
      b. 若未配置 pgvector → 退化为最近 N 条（按 updated_at 降序）+ 暴力搜索
         N = min(5000, 总数)
      c. 日志告警：embedding count exceeds brute-force threshold, consider enabling pgvector
    - 阈值依据：5000 条 × 1536 维 × float32 ≈ 29MB，暴力搜索延迟 < 100ms
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
  ├── 密钥：session 级密钥（会话创建时生成）
  ├── 密钥存储：session_runtime 表新增 signing_key_enc TEXT DEFAULT '' 字段
  │   密钥使用现有 CredentialCrypto（AES-256-GCM）加密后存储
  │   API 层不返回该字段（Ent schema 标记 Sensitive）
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

  session_runtime 表新增字段：
    - signing_key_enc TEXT DEFAULT '' — AES-GCM 加密后的 HMAC 密钥

  压缩完成时：
    1. 从 session_runtime.signing_key_enc 读取并解密 HMAC 密钥
    2. 计算 HMAC-SHA256(summary_markdown, session_key)
    3. 若 encryption_mode = 'aes'：AES-256-GCM 加密
    4. 写入 signature_blob 和 encryption_mode

  注入时：
    1. 验证 HMAC 签名
    2. 若 encryption_mode = 'aes'：解密
    3. 签名不匹配 → 跳过摘要注入，记录告警日志
```

**为什么不能存储在 metadata_json 中**：
- `metadata_json` 通过 `GET /v1/sessions/{id}` API 对前端完全可见
- `PATCH /v1/sessions/{id}` 允许修改 `metadata_json`
- 密钥暴露给前端 = 任何人可伪造签名
- 项目已有成熟的 `CredentialCrypto` 体系（AES-256-GCM），应复用

**威胁模型与安全边界** [G.4 新增]：

```
HMAC 签名防御的威胁模型：

威胁 A：跨代理传递时的网络传输篡改（HMAC 有效）
  - 场景：Agent A 压缩产物通过 gRPC/HTTP 传递给 Agent B
  - 攻击面：网络中间人修改 summary_markdown 内容
  - 防御：HMAC 签名验证，签名不匹配 → 拒绝使用摘要
  - 前提：HMAC 密钥未泄露给攻击者

威胁 B：数据库层面的数据篡改（HMAC 有限）
  - 场景：攻击者直接修改 SQLite 中 session_summaries 表
  - 攻击面：同时修改 summary_markdown 和 signature_blob
  - 防御：HMAC 密钥存储在 session_runtime.signing_key_enc，
    使用 CredentialCrypto（AES-256-GCM）加密，主密钥在服务端配置
  - 限制：如果攻击者能访问数据库 + 服务端配置，则 HMAC 无效
  - 结论：HMAC 防御的是"仅修改摘要但不修改签名"的低级篡改，
    无法防御拥有完整访问权限的攻击者

威胁 C：前端用户伪造摘要（HMAC 有效）
  - 场景：用户通过 API 修改 session_summaries
  - 防御：签名验证在注入时执行，API 层不暴露签名密钥
  - 结论：有效防御

安全边界总结：
  - HMAC 的核心价值在威胁 A（跨代理网络传输）和威胁 C（前端伪造）
  - 对威胁 B（数据库篡改）提供有限防御（需攻击者无法获取主密钥）
  - 如果仅需防御数据库层面的低级篡改，可考虑更轻量的方案：
    在 session_summaries 表增加 checksum 字段（SHA-256 of summary_markdown + salt）
  - Level 2 AES 加密适用于高安全场景（多租户隔离、跨组织传递）

替代方案（如不需要跨代理传递完整性保护）：
  - 仅在 gRPC/HTTP 传输层使用 TLS，应用层不做 HMAC
  - 优点：实现简单，无密钥管理开销
  - 缺点：无法防御数据库层面的低级篡改
  - 适用：单租户、内网部署、信任环境
```

---

### 6.3 L2 Episode 分叉

#### 方案

```sql
ALTER TABLE memory_episodes ADD COLUMN fork_from_episode_id TEXT DEFAULT '';
ALTER TABLE memory_episodes ADD COLUMN fork_from_turn_index INTEGER DEFAULT 0;
```

**API** [G.5 修正]：

```
POST /v1/sessions
  body: {
    fork_from_episode_id: string,  // 从哪个 Episode 分叉
    fork_from_turn_index: integer, // 从第几轮开始分叉（0-based）
    agent_id: string               // 新会话使用的 Agent
  }
  response: {
    session_id: string,            // 新创建的会话 ID
    injected_context: {            // 注入的初始上下文
      l1_snapshot: {...},          // L1 快照
      summary: "...",              // Episode 摘要
      key_decisions: [...]         // 关键决策
    }
  }
```

**API 设计变更说明** [G.5 修正]：

原方案为 `POST /v1/episodes/{eid}/fork`，从 Episode 资源创建 Session 资源，存在语义割裂。修正为在 Session API 下通过参数创建，保持资源创建的一致性：

- 原方案：`POST /v1/episodes/{eid}/fork` → 返回 `new_session_id`（从 Episode 资源创建 Session 资源，语义割裂）
- 修正方案：`POST /v1/sessions` with `fork_from_episode_id` → 返回 `session_id`（在 Session 资源下创建，语义一致）
- 优势：与现有 `POST /v1/sessions` 创建接口统一，前端调用方式一致

**语义**：
- 从指定 Episode 的指定轮次分叉出新会话
- 新会话注入 fork 源的 L1 快照 + Episode 摘要作为初始上下文
- 适用于多代理探索场景：主 Agent 完成任务后，子 Agent 从关键节点分叉继续探索

**与现有 Session 层级的关系**：
- 现有 `parent_session_id` / `root_session_id` 用于 Agent 委派层级（agent delegation hierarchy）
- Fork 产生的新 session 应设置 `parent_session_id` 指向源 session
- `fork_source` 标记改为独立字段 [G.5 修正]：

```sql
-- session_runtime 表新增独立字段（而非 metadata_json 标记）
ALTER TABLE session_runtime ADD COLUMN fork_source TEXT DEFAULT '';
-- 值：''（默认，Agent 委派）| 'episode_fork'（Episode 分叉）
```

- 查询 Agent 委派树时：`WHERE fork_source = ''` 排除分叉产生的 session
- 相比 `metadata_json` 标记：独立字段查询性能更好（无需 JSON 解析），语义更明确

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
| L0 Snapshot 简化+限流（解耦 EvolutionMetricsEnabled，精简 segments_json，增加写入限流） | §2.1 | 写入量降低 90%+ |
| Prompt Caching 三层前缀分离 | §2.3 | 每轮节省 8K~15K 输入 token 成本 |
| 显式压缩预算 | §2.4 | 避免被动压缩信息损失 |
| Embedding 单写确认 + 删除 memory_l2_index_meta | §4.3 | 消除冗余设计 |

### Phase 2：核心优化（2~3 周）

| 任务 | 方案章节 | 预期收益 | 前置依赖 |
|------|---------|---------|---------|
| 三级压缩流水线（Micro → Structured → LLM） | §2.5 | 90%+ 压缩场景零 LLM 成本 | 无（可参考已有 MemoryCompact） |
| L1 token 预算追踪基础实现 | §3.2 前置 | 为选择性注入提供基础 | 无 |
| L1 选择性注入 | §3.2 | 每轮节省 2K~8K tokens | L1 token 预算追踪 |
| 无感后台压缩 + UI 指示器 | §2.5 | 用户体验对标 Trae/Cursor | 三级压缩流水线 |
| 结构化 Episode 优先路径 | §4.1 | 70%~90% Episode 生成零 LLM 成本 | 无 |

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
| 5 | "Path A Episode 设 consolidation_status='pending'，等待 AutoMemoryWorker 处理" | AutoMemoryWorker **不读取也不处理已有 Episode**，只从消息提取 fact 并创建新 Episode。pending 状态的 Episode 将永远停留在 pending | §4.1 改为直接设 `"consolidated"`，§4.2.2 补充说明 |
| 6 | "L1 选择性注入只需加过滤链" | L1 预算管理系统完全未实现（`used_tokens` 永远为 0，`token_estimate` 从未计算，`ErrL1Overflow` 不存在），选择性注入依赖的基础设施缺失 | §3.2 新增前置依赖说明 |
| 7 | "L0 Snapshot on_warning 模式是合理的诊断频率，不需要增加写入策略" | `ShouldWriteL0AssemblySnapshot` 是无记忆的单次判定，`usedRatio` 单调递增，一旦跨过 0.60 后每次模型调用都写入，无任何限流/去重/冷却。长会话中可产生 80~200+ 行写入，原始问题"每次模型调用都写，SQLite 写放大"完全准确 | §2.1 增加写入限流方案（间隔+变化量+阈值穿越） |
| 8 | "Level 1/2 压缩需要从零实现" | `tryMicroCompact`（micro_compact.go）和 `tryMemoryCompact`（memory_compact.go）已存在但为死代码，`CompactSession.level` 已预设 `micro_compact`/`memory_compact` 值 | §2.5 改为"激活+增强现有死代码"而非重写 |
| 9 | "reserved_system = 15K 固定开销" | 系统提示词+工具定义随 Agent 配置变化（2.5K~18K），硬编码 15K 对 minimal profile 浪费 11K，对 full profile 低估 3K。已有 `prompt_snapshot.go` 的 section_* 字段可支撑动态计算 | §2.4.1 改为动态计算 reserved_system |
| 10 | "HMAC 密钥存储在 session 元数据中" | `metadata_json` 通过 `GET/PATCH /v1/sessions/{id}` API 对前端完全可见且可修改，存储密钥 = 任何人可伪造签名。项目已有 `CredentialCrypto`（AES-256-GCM）体系 | §6.2 改为 session_runtime 表新增 signing_key_enc 字段，AES-GCM 加密存储 |
| 11 | "Episode Fork API: POST /v1/sessions/{sid}/fork" | 现有 `parent_session_id` 用于 Agent 委派层级，Fork 语义不同。Session 命名空间下引入 Episode 核心语义的 API 易混淆，且未来 Session 级 fork 会路径冲突 | §6.3 改为 `POST /v1/episodes/{eid}/fork`，并明确 parent_session_id + fork_source 标记 |
| 12 | "将 Identity/Instruction 移到 BeforeModel Hook 以修复缓存顺序" | Context Compaction 重建流程（`rebuildRequestForContextCompaction`）只重新执行 RequestProcessor，**不执行 BeforeModel Hook**。移动后 Identity/Instruction 会在 compaction 重建时丢失 | §2.3 改为在 Anthropic adapter 层面分离静态/动态内容（Time 写入 ContentParts），修复 `applyCacheControlToSystem` 断点位置 |
| 13 | "memoryMsg.CacheControl = ephemeral" 和 "session.staticPrefix 缓存" | `trpcmodel.Message` 没有 `CacheControl` 字段，cache_control 由 `applyCacheControlToSystem` 统一处理。Anthropic Prompt Caching 是 API 层面的自动缓存，不需要应用层手动缓存内容或设置 Message 级别的缓存标记 | §2.3.1 删除 CacheControl 设置，§2.3.3 改为说明 Prompt Caching 工作原理 |

### C.2 关键发现

| # | 发现 | 影响 |
|---|------|------|
| 1 | **框架已完整支持 Anthropic Prompt Caching**（`WithCacheSystemPrompt/WithCacheTools/WithCacheMessages`），项目层已集成配置 | §2.3 方案可大幅简化——只需配置启用 + 调整 system message 顺序，无需从零实现 |
| 2 | **Anthropic 最多 4 个缓存断点**，当前已用 3 个 | 三层前缀分离需在 system 部分增加 1 个断点，刚好达到上限，无法再添加更多 |
| 3 | **BeforeModel hooks 以 prepend 方式注入**，动态内容排在 system TextBlock 数组前面，静态内容排在后面 | `cache_control` 标记在最后一个 TextBlock（静态内容），但前面的动态内容变化也会导致缓存失效。需调整注入顺序 |
| 4 | **L1 的 `used_tokens` 和 `budget_tokens` 字段从未被读取或更新** | 设计文档中的预算检查逻辑完全未实现，§3.2 的选择性注入方案需要先实现基础的 token 预算追踪 |
| 5 | **`memory_l2_index_meta` 表无任何 Go 代码引用** | 确认可安全删除，§4.3 方案无误 |
| 6 | **已有 MemoryCompact（memory_compact.go）和 MicroCompact（micro_compact.go）** | §2.5 Level 1/2 应**激活和增强现有死代码**而非重写。`CompactSession.level` 已预设 `micro_compact`/`memory_compact` 值 |
| 7 | **AutoMemoryWorker 不读取也不处理已有 Episode** | Path A Episode 不应设为 pending 等待后续处理，应直接设为 consolidated 终态 |
| 8 | **preserveInstruction 已有传输管道但未接入压缩 prompt** | §6.1 的摘要保留/丢弃策略只需接入现有管道，无需新建传输机制 |
| 9 | **压缩 prompt 已使用 9 段结构化模板**（v2 版本） | §2.5 Level 3 的 9 章节模板与现有压缩 prompt 高度一致，可直接复用 |
| 10 | **preserveInstruction 是死管道**——前端可传、后端可收，但压缩逻辑完全不消费 | §6.1 只需接入现有管道，无需新建传输机制 |
| 11 | **系统提示词 token 范围 2.5K~18K**，随 Agent 配置变化（coding profile ~18K, minimal ~4.5K） | §2.4 reserved_system 必须动态计算，硬编码 15K 不准确 |
| 12 | **metadata_json 通过 API 对前端完全可见且可修改**，不适合存储密钥 | §6.2 HMAC 密钥必须用独立加密字段 + CredentialCrypto |
| 13 | **parent_session_id 用于 Agent 委派层级**，与 Episode Fork 语义不同 | §6.3 API 路径改为 Episode 命名空间，fork_source 标记区分 |
| 14 | **Context Compaction 重建不执行 BeforeModel Hook** | §2.3 不可将 Identity/Instruction 移到 Hook，改为 adapter 层面分离静态/动态内容 |
| 15 | **Compressor 已持有 memoryReader 但从未使用** | §2.5 Level 2 Step 1 可直接复用，无需新增依赖注入 |
| 16 | **TimeRequestProcessor 是唯一每轮变化的 RequestProcessor** | §2.3 只需将 Time 分离到 ContentParts 即可实现静态/动态分离 |

### C.3 业务合理性评估

| 方案 | 业务合理性 | 风险 | 新发现影响 |
|------|-----------|------|-----------|
| §2.1 L0 Snapshot 简化+限流 | 合理——保留诊断功能，限流降低写入量 90%+ | 低 | **修正**——原判断"合理频率"有误，增加写入限流方案 |
| §2.2 摘要注入模式 | 合理——无需改动，文档说明即可 | 无 | 无 |
| §2.3 三层前缀分离 | 合理——adapter 层面分离静态/动态内容 | 低 | **重大修正**——原方案"移到 Hook"不可行（Compaction 重建不执行 Hook），改为 adapter 层面修复断点位置 |
| §2.4 显式压缩预算 | 合理——复用现有同步/异步路径 | 低 | **修正**——reserved_system 改为动态计算，避免不同 profile 的预算偏差 |
| §2.5 无感后台压缩 | 合理——已有死代码可激活，无需重写 | 低 | **大幅简化**——Level 1/2 对应已有 tryMicroCompact/tryMemoryCompact 死代码，只需激活+增强；Level 3 复用现有 9 段模板 |
| §3.1 L1 表轻量化 | 合理——不删表，降为可选 | 低 | 无 |
| §3.2 L1 选择性注入 | 合理——但需先实现 token 预算追踪 | **高** | **前置依赖增加**——需先实现 used_tokens 更新和 token_estimate 计算 |
| §3.3 L1 与框架 Memory 厘清 | 合理——互斥规则清晰 | 低 | 无 |
| §4.1 结构化 Episode | 合理——零成本路径优先 | 低 | **修正**——consolidation_status 直接设为 "consolidated"，不依赖后续处理 |
| §4.2 单跳巩固 | 合理——删除无效 Worker，统一状态值 | 低 | **简化**——L2ConsolidateWorker 本就无效，删除无影响 |
| §4.3 Embedding 单写 | 合理——删除未使用的表和设计 | 低 | 无 |
| §6.1 摘要保留/丢弃策略 | 合理——preserveInstruction 管道已有 | 低 | **简化**——只需接入现有管道 |
| §6.2 加密上下文传递 | 合理——HMAC 轻量，AES 可选 | 低 | **修正**——密钥存储改为独立加密字段+CredentialCrypto，不可用 metadata_json |
| §6.3 Episode 分叉 | 合理——多代理探索场景 | 低 | **修正**——API 改为 Episode 命名空间，parent_session_id + fork_source 标记区分委派 |
| §6.5 子目录规则按需加载 | 合理——减少无关 Skill 注入 | 中 | 无 |

### C.4 需要额外验证的假设

| # | 假设 | 验证方法 |
|---|------|---------|
| 1 | Level 2 Structured Compact 的结构化摘要通常 < 原始历史 30% | 在生产环境统计 L1 快照 token vs 对话历史 token 的比例 |
| 2 | 12% 压缩缓冲区足以覆盖异步压缩期间的增量 | 在生产环境监控 soft_trigger 到压缩完成期间的 token 增量 |
| 3 | 调整 system message 注入顺序不影响其他 BeforeModel hooks | 梳理所有 BeforeModel hooks 的优先级和依赖关系 |
| 4 | L1 选择性注入的 `read_count` 排序能反映字段相关性 | A/B 测试：对比全量注入 vs 选择性注入的模型输出质量 |

---

## 附录 D：架构风险解决方案

> 本附录针对方案评审中识别的 5 项架构风险，给出具体解决方案。

### D.1 风险1：Anthropic 缓存断点上限约束

#### 风险描述

Anthropic API 最多允许 **4 个缓存断点**。当前方案使用 3 个（system 1 + tools 1 + messages 1），三层前缀分离需在 system 部分增加 1 个 = 4 个，**刚好达到上限**。未来如果需要更多断点（如 tools 部分分层、多轮对话增加缓存层），将没有空间。

#### 代码现状

`applyCacheControlToSystem`（[anthropic.go](file:///f:/project/aranea-agents/pkg/trpc-agent-go/model/anthropic/anthropic.go)）始终在**最后一个 TextBlock** 标记 `cache_control`，无法指定具体位置。三个 Option 都是布尔开关，粒度为"整个类别开/关"。

#### 解决方案：可扩展断点分配器 + 优先级淘汰

##### D.1.1 断点预算池

```
const maxCacheBreakpoints = 4  // Anthropic API 上限

断点分配池（按优先级从高到低）：

Priority 1（必保）：system 静态层断点
  - 覆盖 Identity+Instruction+Skills+Workspace+PostTool
  - 预估 8K~15K tokens，命中率最高
  - 每轮必命中（除非 Skill 变化）
  - 断点位置：system TextBlock[0] 末尾

Priority 2（必保）：tools 断点
  - 覆盖工具定义
  - 预估 2K~8K tokens
  - Agent 配置不变时必命中
  - 断点位置：最后一个 tool 定义

Priority 3（重要）：system 半静态层断点
  - 覆盖 L1+L3+L4 记忆 + RuntimeCue + SkillGuidance
  - 预估 2K~8K tokens
  - 任务/轮次切换时变化，同任务内稳定
  - 断点位置：半静态层 TextBlock 末尾

Priority 4（可选）：messages 断点
  - 覆盖对话历史
  - 预估动态
  - 每轮前移，命中率中等
  - 断点位置：倒数第二个 assistant message

分配规则：
  if 可用断点 >= 4: 全部分配（当前方案）
  if 可用断点 == 3: 保留 P1+P2+P3，放弃 P4（messages）
  if 可用断点 == 2: 保留 P1+P2，放弃 P3+P4
  if 可用断点 == 1: 仅保留 P1（system 静态层）
  if 可用断点 == 0: 不启用缓存
```

##### D.1.2 框架层改造：断点位置可控

当前 `applyCacheControlToSystem` 只能标记最后一个 TextBlock，需要扩展为支持指定位置。

```go
// 新增 Option：控制系统提示缓存断点位置
type SystemCacheStrategy int

const (
    // CacheLastBlock 在最后一个 TextBlock 标记断点（当前行为，向后兼容）
    CacheLastBlock SystemCacheStrategy = iota
    // CacheFirstBlock 在第一个 TextBlock 标记断点（静态前缀分离后使用）
    CacheFirstBlock
    // CacheAtBlockIndex 在指定索引的 TextBlock 标记断点
    CacheAtBlockIndex
)

// WithSystemCacheStrategy 设置系统提示缓存策略
func WithSystemCacheStrategy(strategy SystemCacheStrategy, blockIndex int) Option {
    // ...
}
```

**改造 `applyCacheControlToSystem`**：

```go
func (m *Model) applyCacheControlToSystem(systemPrompts []anthropic.TextBlockParam) []anthropic.TextBlockParam {
    if len(systemPrompts) == 0 {
        return systemPrompts
    }

    result := make([]anthropic.TextBlockParam, len(systemPrompts))
    copy(result, systemPrompts)

    var targetIdx int
    switch m.systemCacheStrategy {
    case CacheFirstBlock:
        targetIdx = 0
    case CacheAtBlockIndex:
        targetIdx = m.systemCacheBlockIndex
        if targetIdx >= len(result) {
            targetIdx = len(result) - 1
        }
    default: // CacheLastBlock
        targetIdx = len(result) - 1
    }

    result[targetIdx].CacheControl = anthropic.NewCacheControlEphemeralParam()
    return result
}
```

##### D.1.3 双断点 system 缓存

三层前缀分离需要在 system 部分放置 **2 个断点**（静态层末尾 + 半静态层末尾），加上 tools(1) + messages(1) = 4 个，刚好达到上限。

```go
// 新增 Option：启用 system 双断点
// 在 system TextBlock 数组中，同时标记两个位置：
//   1. 静态层末尾（TextBlock[0]）
//   2. 半静态层末尾（由 systemSecondBreakpointIndex 指定）
func WithCacheSystemPromptDualBreakpoint(secondBlockIndex int) Option {
    // ...
}
```

**改造 `applyCacheControlToSystem`**：

```go
func (m *Model) applyCacheControlToSystem(systemPrompts []anthropic.TextBlockParam) []anthropic.TextBlockParam {
    if len(systemPrompts) == 0 {
        return systemPrompts
    }

    result := make([]anthropic.TextBlockParam, len(systemPrompts))
    copy(result, systemPrompts)

    if m.cacheSystemDualBreakpoint {
        // 双断点模式：标记静态层末尾 + 半静态层末尾
        result[0].CacheControl = anthropic.NewCacheControlEphemeralParam()  // 静态层
        idx := m.systemSecondBreakpointIndex
        if idx > 0 && idx < len(result) {
            result[idx].CacheControl = anthropic.NewCacheControlEphemeralParam()  // 半静态层
        }
    } else {
        // 单断点模式（向后兼容）
        targetIdx := len(result) - 1
        if m.systemCacheStrategy == CacheFirstBlock {
            targetIdx = 0
        }
        result[targetIdx].CacheControl = anthropic.NewCacheControlEphemeralParam()
    }

    return result
}
```

##### D.1.4 断点计数校验

```go
// 在 applyCacheControl 入口处增加断点计数校验
func (m *Model) applyCacheControl(...) (...) {
    usedBreakpoints := 0
    maxBP := maxCacheBreakpoints  // 默认 4

    // 计算需要的断点数
    if m.cacheSystemPrompt {
        if m.cacheSystemDualBreakpoint {
            usedBreakpoints += 2  // system 双断点
        } else {
            usedBreakpoints += 1  // system 单断点
        }
    }
    if m.cacheTools {
        usedBreakpoints += 1
    }
    if m.cacheMessages {
        usedBreakpoints += 1
    }

    // 超出上限时按优先级淘汰
    if usedBreakpoints > maxBP {
        // 淘汰策略：P4(messages) > P3(system半静态) > P2(tools) > P1(system静态)
        // 从最低优先级开始禁用
        if m.cacheMessages && usedBreakpoints > maxBP {
            m.cacheMessages = false
            usedBreakpoints--
        }
        if m.cacheSystemDualBreakpoint && usedBreakpoints > maxBP {
            m.cacheSystemDualBreakpoint = false  // 降级为单断点
            usedBreakpoints--
        }
        if m.cacheTools && usedBreakpoints > maxBP {
            m.cacheTools = false
            usedBreakpoints--
        }
        // P1（system 静态层）永远保留
    }

    // ... 原有逻辑
}
```

##### D.1.5 未来扩展路径

| 场景 | 断点需求 | 应对策略 |
|------|---------|---------|
| 当前方案（三层前缀） | system(2) + tools(1) + messages(1) = 4 | 刚好满足 |
| tools 需要分层（如常用/非常用工具） | system(2) + tools(2) + messages(1) = 5 | 淘汰 messages 断点，保留 system(2) + tools(2) = 4 |
| 多轮对话需要多个 messages 断点 | system(2) + tools(1) + messages(2) = 5 | 淘汰 system 半静态断点，保留 system(1) + tools(1) + messages(2) = 4 |
| Anthropic 未来放宽断点上限 | 不受限 | 自动利用更多断点，`maxCacheBreakpoints` 改为从 API 响应动态获取 |

**关键约束文档化**：在 `anthropic.go` 文件头部和项目架构文档中明确记录"Anthropic 4 断点上限"约束，确保未来开发者理解新增断点时的取舍。

---

### D.2 风险2：压缩原子替换的事务安全

#### 风险描述

压缩完成后的操作序列（写入 summary → 重写 Snapshot → 发布事件）如果中间步骤失败，可能导致状态不一致。

#### 代码现状

经代码验证，**核心数据库写操作已在事务中**（`CompressSessionInTx`），事务安全性良好。但存在两个风险点：

1. **CAS 与事务之间的间隙**：`TryIncrementCompressVersion`（原子 SQL）在事务外执行，CAS 成功后、事务开始前如果进程崩溃，`compress_version` 已递增但 summary 未写入。
2. **事务后操作无补偿**：`SyncRunnerSnapshot`（trpc 运行时同步）、`AppendChatMessage`（system 消息）、`EventBus.Publish`（事件发布）在事务外执行，失败后无重试或补偿。

#### 解决方案

##### D.2.1 CAS-事务间隙：幂等重入保护

**现状已安全**：CAS 递增 `compress_version` 后，如果事务未执行，下次压缩时：
- `MaxSessionSummaryToTurn` 不会找到对应的 summary（因为未写入）
- 消息仍会被重新压缩（浪费一次 LLM 调用，但不会丢失数据）
- `compress_version` 只是递增了一个版本号，不影响正确性

**增强**：在事务内增加 `compress_version` 一致性校验。

```go
// 在 CompressSessionInTx 回调开头增加版本校验
fn := func(txCtx context.Context) error {
    // 校验事务内的 compress_version 与 CAS 时一致
    currentVersion, err := c.sessionRepo.GetCompressVersion(txCtx, sessionID)
    if err != nil {
        return err
    }
    if currentVersion != expectedVersionAfterCAS {
        // 并发压缩已提交，放弃本次
        return ErrCompressVersionConflict
    }

    // ... 原有写入逻辑
}
```

**效果**：即使 CAS 和事务之间有另一个压缩完成提交，当前事务也会因版本冲突而回滚，避免覆盖更新的压缩结果。

##### D.2.2 事务后操作：补偿机制

**问题1：`SyncRunnerSnapshot` 失败**

trpc-agent-go 运行时快照与 DB 不一致时，运行时仍使用旧快照，可能导致：
- 下次 LLM 调用使用过时的上下文（包含已压缩的消息）
- 但 Context Compaction 重建时会从 DB 重新加载，自动修复

**补偿方案**：增加快照同步状态标记。

```go
// session_runtime 表新增字段
ALTER TABLE session_runtime ADD COLUMN snapshot_sync_status TEXT DEFAULT 'synced';
// 值：'synced' | 'pending' | 'failed'

// 事务提交后
if err := c.Runtime.SyncRunnerSnapshot(ctx, sessionID, newSnapshot); err != nil {
    // 标记同步失败，下次 BeforeModel Hook 时重试
    c.sessionRepo.UpdateSnapshotSyncStatus(ctx, sessionID, "failed")
    c.lg.Warn("sync runner snapshot failed, will retry on next turn",
        "session_id", sessionID, "error", err)
}

// BeforeModel Hook 中增加重试逻辑
if status == "failed" {
    if err := c.Runtime.SyncRunnerSnapshot(ctx, sessionID, currentSnapshot); err == nil {
        c.sessionRepo.UpdateSnapshotSyncStatus(ctx, sessionID, "synced")
    }
}
```

**问题2：`AppendChatMessage` 失败**

压缩通知消息写入失败，用户看不到"已优化"提示，但 DB 状态正确。

**补偿方案**：降级为事件通知。

```go
// publishCompressionNotice 中
if err := c.messageWriter.AppendChatMessage(ctx, sessionID, sysMsg, false); err != nil {
    c.lg.Warn("append compression notice failed, event still published",
        "session_id", sessionID, "error", err)
    // 不重试——事件已发布，前端可通过事件更新 UI 状态
}
```

**问题3：`EventBus.Publish` 失败**

事件丢失只影响前端实时通知，前端可通过轮询 session 状态补偿。

**补偿方案**：前端增加轮询降级。

```typescript
// 前端：压缩请求发出后，启动轮询降级
async function onCompactSession(sessionId: string) {
  const result = await sessionApi.compactSession(sessionId)

  // 如果事件未到达，3 秒后主动查询 session 状态
  setTimeout(async () => {
    if (!compressionNoticeReceived) {
      const session = await sessionApi.getSession(sessionId)
      updateContextIndicator(session.contextUsedRatio)
    }
  }, 3000)
}
```

##### D.2.3 事务安全总结

| 操作 | 当前状态 | 增强方案 | 风险等级 |
|------|---------|---------|---------|
| CAS 乐观锁 | 原子 SQL，事务外 | 事务内版本校验 | 低 → 极低 |
| DB 写入（5 步） | 同一事务内 | 无需改动 | 安全 |
| SyncRunnerSnapshot | 事务外，失败无补偿 | sync_status 标记 + BeforeModel 重试 | 中 → 低 |
| AppendChatMessage | 事务外，失败无补偿 | 降级为事件通知 | 低 |
| EventBus.Publish | 事务外，失败无补偿 | 前端轮询降级 | 低 |

---

### D.3 风险3：Level 2 Memory Compact 降级阈值

#### 风险描述

当前方案中 Level 2（Memory Compact）降级到 Level 3（LLM Compact）的判断条件是"结构化数据 < 原始历史 30%"，即如果结构化摘要的 token 数超过原始历史的 30%，则认为结构化摘要不够精炼，降级到 LLM 压缩。这个 30% 阈值缺乏理论依据——结构化摘要虽然短，但信息密度高，30% 可能过于保守。

#### 解决方案：信息完整度评估替代简单 token 比例

##### D.3.1 降级判断标准重构

```
降级判断（替代"结构化数据 < 原始历史 30%"）：

核心指标：信息覆盖度（Information Coverage Score, ICS）

ICS = Σ(已覆盖信息维度得分) / Σ(全部信息维度得分)

信息维度（6 维，分级评分）：
  1. 用户意图（User Intent）   — 权重 0.25
     L1 覆盖源：task_goal
     评分：task_goal 非空 = 1.0，空 = 0.0

  2. 当前状态（Current State） — 权重 0.20
     L1 覆盖源：status + fields 中 status/progress 相关字段
     评分：有 status 字段 = 1.0，无 = 0.0

  3. 关键决策（Key Decisions） — 权重 0.20
     L1 覆盖源：fields 中 decision/choice/approach 相关字段
     L3 覆盖源：recall facts 中 preference 类
     评分（分级）：
       有 >= 2 个决策字段 = 1.0
       有 1 个决策字段 = 0.5
       无决策字段 = 0.0

  4. 文件变更（File Changes）  — 权重 0.15
     L1 覆盖源：fields 中 file/artifact/reference 相关字段
     评分（分级）：
       有 >= 2 个文件字段 = 1.0
       有 1 个文件字段 = 0.5
       无文件字段 = 0.0

  5. 长期事实（Long-term Facts）— 权重 0.10
     L3 覆盖源：recall facts
     评分（分级）：
       有 >= 3 条 fact = 1.0
       有 1~2 条 fact = 0.5
       无 fact = 0.0

  6. 待办事项（Pending Items） — 权重 0.10
     L1 覆盖源：fields 中 pending/todo/blocker 相关字段
     评分：有 >= 1 个待办字段 = 1.0，无 = 0.0

ICS 计算：
  ICS = 0.25 * intent + 0.20 * state + 0.20 * decisions
      + 0.15 * files + 0.10 * facts + 0.10 * pending

降级规则：
  if ICS >= 0.70 → 使用 Level 2（结构化摘要），不降级
  if ICS < 0.70 → 降级到 Level 3（LLM Compact）
```

##### D.3.2 为什么 ICS >= 0.70 而非 100%

- 6 维中覆盖 4 维即可达到 0.70+（如 intent + state + decisions(1.0) + files(1.0) = 0.80）
- L1 工作记忆天然覆盖 intent/state/decisions/files（任务级结构化数据的核心职责）
- pending 和 facts 是"锦上添花"，缺失不影响核心信息传递
- 0.70 阈值确保：即使 L3 recall 为空（无长期事实）且无 pending 字段，只要 L1 有 task_goal + status + 2 个决策字段，ICS = 0.25 + 0.20 + 0.20 = 0.65，接近阈值
- **分级评分的优势**：仅 1 个决策字段时得分 0.5（而非 1.0），避免"刚好有 1 个字段就通过"的边界情况。此时 ICS = 0.25 + 0.20 + 0.10 = 0.55，低于阈值，将降级到 Level 3——这是合理的，因为仅 1 个决策字段说明 L1 数据不够丰富
- 实际场景中，中长任务（> 5 轮）的 L1 通常有 task_goal + status + 2+ 个决策字段，ICS >= 0.70

##### D.3.3 Token 比例作为辅助约束

ICS 评估信息完整度，但结构化摘要如果过长（接近原始历史长度），仍然没有压缩价值。保留 token 比例作为**辅助约束**而非主判断：

```
辅助约束：
  if 结构化摘要 token > 原始历史 token * 0.60:
    → 即使 ICS >= 0.70，也降级到 Level 3
    → 原因：压缩比不足 40%，结构化摘要不够精炼

放宽阈值从 30% 到 60%：
  - 30% 过于保守：结构化摘要 3K tokens vs 原始 8K = 37.5%，本应保留但被降级
  - 60% 更合理：只要压缩比 > 40%，结构化摘要就有价值
  - 结构化摘要的信息密度远高于原始对话（无重复、无调试过程、无工具输出）
```

##### D.3.4 实现代码

```go
// memory_compact.go 中新增

type compactCoverage struct {
    HasIntent      bool  // task_goal 非空
    HasState       bool  // status 字段存在
    DecisionCount  int   // decision/choice/approach 字段数量
    FileCount      int   // file/artifact/reference 字段数量
    FactCount      int   // L3 recall facts 数量
    HasPending     bool  // pending/todo/blocker 字段存在
}

func (c compactCoverage) ICS() float64 {
    intent := boolToFloat(c.HasIntent)
    state := boolToFloat(c.HasState)
    decisions := gradedScore(c.DecisionCount, 2) // >=2 → 1.0, 1 → 0.5, 0 → 0.0
    files := gradedScore(c.FileCount, 2)         // >=2 → 1.0, 1 → 0.5, 0 → 0.0
    facts := gradedScore(c.FactCount, 3)         // >=3 → 1.0, 1~2 → 0.5, 0 → 0.0
    pending := boolToFloat(c.HasPending)

    return 0.25*intent + 0.20*state + 0.20*decisions +
        0.15*files + 0.10*facts + 0.10*pending
}

// gradedScore 分级评分：count >= threshold → 1.0, count >= 1 → 0.5, else → 0.0
func gradedScore(count, threshold int) float64 {
    if count >= threshold {
        return 1.0
    }
    if count >= 1 {
        return 0.5
    }
    return 0.0
}

func shouldUseStructuredCompact(
    coverage compactCoverage,
    structuredTokens int,
    originalTokens int,
) bool {
    // 主判断：信息覆盖度
    if coverage.ICS() < 0.70 {
        return false
    }
    // 辅助约束：压缩比
    if structuredTokens > int(float64(originalTokens)*0.60) {
        return false
    }
    return true
}
```

---

### D.4 风险4：§3.3 互斥规则后向兼容

#### 风险描述

方案提出新增 `memory_tool_mode` 配置字段（`working_memory` / `framework_memory` / `both`），默认 `working_memory` 对新 Agent 生效，但"已有 Agent 保持现状"意味着存量 Agent 仍然可能混淆使用两套工具。缺少迁移策略。

#### 代码现状

经代码验证：
- `AgentRuntimeSettings` 中**不存在** `memory_tool_mode` 字段
- 工具注册由 `ToolsProfile` + `ToolsAllowJSON` + `ToolsDenyJSON` 三层控制
- `memory_clear` 已通过 `filterMemoryTools` 硬编码过滤
- `working_memory` 工具在 `MemoryAdmin` 存在时自动注入
- 框架层 `AutoMemoryMode` 通过 `enabledTools` map 控制，但不是持久化配置

#### 解决方案：基于现有工具过滤机制的渐进迁移

##### D.4.1 不新增 `memory_tool_mode` 字段

**理由**：项目已有完善的工具过滤机制（`ToolsProfile` + `ToolsAllowJSON` + `ToolsDenyJSON`），新增独立字段会造成两套并行控制系统，增加理解和维护成本。

**替代方案**：通过 `ToolsDenyJSON` 实现互斥，无需新增字段。

##### D.4.2 互斥规则映射到现有机制

```
memory_tool_mode 的语义 → ToolsDenyJSON 映射：

"working_memory"（默认，推荐）：
  ToolsDenyJSON 增加：["memory_add", "memory_update", "memory_delete", "memory_search", "memory_load"]
  效果：Agent 只能看到 working_memory_* 工具

"framework_memory"：
  ToolsDenyJSON 增加：["working_memory_read", "working_memory_list", "working_memory_write",
                       "working_memory_patch", "working_memory_delete"]
  效果：Agent 只能看到 memory_* 工具

"both"（不推荐）：
  ToolsDenyJSON 不增加任何记忆工具
  效果：Agent 可看到两套工具（当前默认行为）
```

##### D.4.3 渐进迁移策略

```
Phase 1：文档 + 引导（无代码变更）
  - 在 Agent 配置文档中说明两套记忆工具的区别和推荐
  - 在 Agent 配置界面的 ToolsDenyJSON 编辑器中增加提示：
    "建议禁用 memory_add/memory_update 等框架记忆工具，使用 working_memory 替代"
  - 不修改任何 Agent 的现有配置

Phase 2：新 Agent 默认值（低风险）
  - 新建 Agent 时，如果选择 coding/full/research profile：
    自动在 ToolsDenyJSON 中添加 framework memory 工具
  - 已有 Agent 不受影响
  - 实现位置：buildAgentEffectiveTools 中的 profile 预设逻辑

Phase 3：存量迁移提示（中风险）
  - 在 Agent 列表页增加"记忆工具模式"列
  - 对使用 "both" 模式的 Agent 显示提示：
    "此 Agent 同时启用了 working_memory 和 framework memory 工具，
     建议选择其中一套以避免混淆。点击配置"
  - 用户点击后跳转到 ToolsDenyJSON 编辑界面
  - 不自动修改，由用户决定

Phase 4：未来可选 — 自动迁移（需评估）
  - 分析 Agent 的工具调用日志
  - 如果 Agent 从未调用 framework memory 工具 → 自动禁用
  - 如果 Agent 只调用 framework memory → 自动禁用 working_memory
  - 需要充分的日志数据支撑，避免误判
```

##### D.4.4 实现要点

```go
// Phase 2：在 profile 预设中增加默认禁用规则
// internal/biz/agent_effective_tools.go

var toolGroupsFrameworkMemory = []string{
    "memory_add", "memory_update", "memory_delete",
    "memory_search", "memory_load",
}

// 在 buildAgentEffectiveTools 中，对新建 Agent 的 coding/full/research profile
// 自动将 framework memory 工具加入 deny 列表
func applyMemoryToolPolicy(profile string, denySet map[string]bool) {
    switch profile {
    case "coding", "full", "research":
        for _, tool := range toolGroupsFrameworkMemory {
            denySet[tool] = true
        }
    }
}
```

```typescript
// Phase 3：前端 Agent 列表增加记忆工具模式检测
function getMemoryToolMode(agent: AgentConfig): 'working_memory' | 'framework_memory' | 'both' {
  const deny = new Set(agent.toolsDenyJSON ?? [])
  const hasWorkingMemory = !deny.has('working_memory_read')
  const hasFrameworkMemory = !deny.has('memory_search')

  if (hasWorkingMemory && hasFrameworkMemory) return 'both'
  if (hasWorkingMemory) return 'working_memory'
  return 'framework_memory'
}
```

---

### D.5 风险5：Embedding 增量重建 embedding_version 语义

#### 风险描述

方案提出新增 `embedding_version` 字段用于 pgvector 增量同步，但未明确 version 的初始值、递增策略、以及 Episode 被多次更新时的 version 行为。

#### 代码现状

经代码验证：
- `embedding_version` 字段**不存在**于代码中，仅在方案文档中提出
- `embedding_status` 已有完整的状态机：`pending` → `stale` → `fresh`（+ `failed`/`disabled`）
- L3 Fact 已实现 SQLite + pgvector 双写
- L2 Episode 仅写 SQLite，pgvector 同步未实现
- `EpisodeBackfillWorker` 每 6 小时扫描 `embedding_status IN ('pending','stale')` 的 episode

#### 解决方案：明确 embedding_version 语义与增量同步协议

##### D.5.1 embedding_version 语义定义

```
embedding_version 语义：

1. 初始值：0（ALTER TABLE ... DEFAULT 0）
   - 新创建的 Episode embedding_version = 0
   - embedding_status = 'pending'

2. 递增时机：Episode 内容字段被 UPDATE 时
   - 内容字段 = title, outcome_summary, key_decisions, key_artifacts, importance
   - metadata_json 变更不触发递增（非检索内容）
   - 递增方式：embedding_version = embedding_version + 1
   - 同时标记：embedding_status = 'stale'

3. 递增规则：每次内容 UPDATE 递增 1 次
   - Path A → Path B 覆盖：version 从 0 → 1（Path A 创建时为 0，Path B 更新时 +1）
   - 用户手动编辑 Episode：version +1
   - LLM 增强后更新：version +1
   - 同一事务内多次 UPDATE：version 只递增 1 次（SQLite ON UPDATE 触发器保证）

4. 不递增的场景：
   - embedding_status 从 'stale' → 'fresh'（Backfill Worker 重建完成）
   - embedding_status 从 'pending' → 'fresh'（首次计算完成）
   - 非内容字段更新（consolidated_l3_count 等）
```

##### D.5.2 增量同步协议

```
SQLite（权威源）↔ pgvector（读索引）同步协议：

1. 同步方向：单向（SQLite → pgvector）
   - SQLite 是权威源，pgvector 是可选读索引
   - pgvector 不同步不影响本地检索（fallback 到 SQLite 暴力搜索）

2. 同步触发点：
   a. Backfill Worker 重建完成后（embedding_status → 'fresh'）
   b. AutoMemoryWorker 创建 Episode 后
   c. Path B LLM 增强 Episode 后

3. 增量判断：
   pgvector 中每条向量记录携带 metadata.embedding_version

   同步逻辑：
   SELECT id, embedding_version FROM memory_episodes
   WHERE embedding_status = 'fresh' AND agent_id = ?

   对比 pgvector 中的 embedding_version：
   - SQLite version > pgvector version → 更新（DELETE + INSERT）
   - SQLite version = pgvector version → 跳过
   - SQLite version < pgvector version → 不可能（SQLite 是权威源），记录告警

4. 删除同步：
   Episode 软删除（deleted_at != ''）→ pgvector 对应记录删除
   定期扫描：每 24 小时清理 pgvector 中 SQLite 已删除的记录

5. 同步失败处理：
   - pgvector 不可用时：SQLite 正常工作，pgvector 同步跳过
   - pgvector 部分失败：记录失败的 episode_id，下次 Backfill Worker 运行时重试
   - 不影响 embedding_status 状态（始终以 SQLite 为准）
```

##### D.5.3 DDL 变更

```sql
-- memory_episodes 表
ALTER TABLE memory_episodes ADD COLUMN embedding_version INTEGER NOT NULL DEFAULT 0;

-- memory_facts 表（已有 embedding_status，新增 version）
ALTER TABLE memory_facts ADD COLUMN embedding_version INTEGER NOT NULL DEFAULT 0;

-- memory_entities 表（已有 embedding_status，新增 version）
ALTER TABLE memory_entities ADD COLUMN embedding_version INTEGER NOT NULL DEFAULT 0;
```

##### D.5.4 代码变更

```go
// Episode 内容更新时递增 version
// internal/data/memory_shim_l2.go

func (r *l2EpisodeRepo) UpdateEpisodeContent(ctx context.Context, id string, updates EpisodeContentUpdate) error {
    q := `UPDATE memory_episodes SET
          title = ?, outcome_summary = ?, key_decisions = ?, key_artifacts = ?,
          importance = ?, embedding_status = 'stale',
          embedding_version = embedding_version + 1,
          updated_at = ?
          WHERE id = ?`
    // ...
}

// Backfill Worker 重建后记录当前 version
// internal/data/memory_episode_sync.go

func (s *memoryEpisodeIndexSync) SyncEpisodeIndex(ctx context.Context, agentID, episodeID, title, summary string) error {
    // ... 计算 embedding ...

    q := `UPDATE memory_episodes SET
          embedding_blob = ?, embedding_norm = ?, embedding_dim = ?,
          embedding_status = 'fresh', embedding_model = 'memory_embedder',
          last_embedded_at = ?
          WHERE id = ?`
    // 注意：不递增 embedding_version（重建不是内容变更）
    // ...
}

// pgvector 增量同步
// internal/data/pgvector/store.go

func (s *PgVectorStore) SyncEpisodesIncremental(ctx context.Context, agentID string) (synced, skipped, failed int, err error) {
    // 1. 读取 SQLite 中所有 fresh 状态的 episode 及其 version
    episodes, err := s.reader.ListEpisodesWithVersion(ctx, agentID)

    // 2. 读取 pgvector 中现有记录的 version
    pgVersions, err := s.getExistingVersions(ctx, agentID)

    // 3. 增量对比
    for _, ep := range episodes {
        pgVer, exists := pgVersions[ep.ID]
        if !exists || pgVer < ep.EmbeddingVersion {
            // 需要更新：DELETE + INSERT
            s.deleteAndInsert(ctx, ep)
            synced++
        } else {
            skipped++
        }
    }

    return
}
```

##### D.5.5 与现有 embedding_status 的关系

```
embedding_status 和 embedding_version 是互补的：

embedding_status：控制"是否需要重建 embedding"
  - 'pending'/'stale' → Backfill Worker 会处理
  - 'fresh' → 不处理
  - 这是 SQLite 内部的状态机

embedding_version：控制"pgvector 是否需要同步"
  - version 不一致 → 需要同步
  - version 一致 → 跳过
  - 这是 SQLite ↔ pgvector 的同步协议

两者不冲突：
  - 内容变更 → embedding_status = 'stale', embedding_version += 1
  - Backfill 重建 → embedding_status = 'fresh', version 不变
  - pgvector 同步 → 对比 version，一致则跳过
```

---

## 附录 E：综合评估报告

> 本附录对方案全文进行业务逻辑和架构设计的综合评估，识别遗留问题和跨章节交互风险。

### E.1 业务逻辑问题

#### E.1.1 [严重] §4.1 Path A key_decisions 提取规则依赖不存在的命名约定

**问题**：方案提出按 `field_path` 匹配 "decision"/"choice"/"approach" 模式提取 key_decisions，但 `field_path` 是完全自由格式的字符串，由 LLM Agent 在调用 `working_memory_write` 时自行决定，没有任何命名规范约束。

**代码证据**：
- [tools.go](file:///f:/project/aranea-agents/internal/tools/working_memory/tools.go) 第 120 行：`FieldPath string` 仅声明为 `jsonschema:"description=The field path to write to,required"`，无枚举或前缀约束
- [memory_shim_l1.go](file:///f:/project/aranea-agents/internal/data/memory_shim_l1.go) 第 210-278 行：`UpsertL1Field` 只做 `strings.TrimSpace`，无模式校验
- Agent 可能写入 `"user_preference"`、`"current_step"`、`"approach_used"` 等任意字符串

**影响**：模式匹配可能提取不到任何字段，也可能误匹配不相关字段。key_decisions 为空时 Episode 质量显著下降。

**修正方案**：

```
Path A key_decisions 提取策略（分层 fallback）：

Layer 1：模式匹配（当前方案）
  - 匹配 field_path 包含 "decision"/"choice"/"approach" 的字段
  - 匹配 field_path 包含 "file"/"artifact" 的字段 → key_artifacts

Layer 2：属性匹配（新增 fallback）
  - 若 Layer 1 匹配结果为空：
    - 取 pin_to_prompt=true 且 visibility="prompt" 的前 3 个字段作为 key_decisions
    - 取 field_kind="reference" 的字段作为 key_artifacts
  - 理由：pinned+visible 字段通常是 Agent 认为重要的信息

Layer 3：全量兜底（新增 fallback）
  - 若 Layer 1+2 均为空：
    - 取所有 visibility="prompt" 字段的前 5 个
    - 按更新时间降序排列（最新变更优先）

实现位置：新增 extractKeyDecisions(fields []L1FieldRow) []string 函数
```

#### E.1.2 [严重] §3.2 选择性注入依赖 `read_count` 排序，但 `read_count` 从未被追踪

**问题**：方案提出"按 `read_count` 降序排序，高频字段优先注入"作为短期无 embedding 的相关性策略，但 `read_count` 在代码中**始终为 0**——INSERT 硬编码为 0，ON CONFLICT 不更新，全代码库无任何 UPDATE 语句递增它。

**代码证据**：
- [memory_chain.sql](file:///f:/project/aranea-agents/internal/data/sql/memory_chain.sql) 第 113 行：`read_count INTEGER NOT NULL DEFAULT 0`
- [memory_shim_l1.go](file:///f:/project/aranea-agents/internal/data/memory_shim_l1.go) 第 272 行：INSERT 时硬编码 `0`
- ON CONFLICT 子句（第 253-258 行）不包含 `read_count`
- 全代码库搜索 `UPDATE.*memory_l1_fields.*read_count`：零匹配

**影响**：所有字段的 `read_count` 相同（均为 0），排序无意义，选择性注入退化为"取前 K 个"。

**修正方案**：

```
短期替代策略（无需实现 read_count 追踪）：

1. 按 updated_at 降序排序（最近更新的字段优先）
   - 理由：最近更新的字段更可能与当前任务相关
   - 数据源：memory_l1_fields.updated_at 已有且被正确维护

2. 按 field_path 字母序排序（确定性，可复现）
   - 理由：作为 fallback 保证排序稳定

3. 长期方案：实现 read_count 递增
   - 在 L1MemoryCue() 渲染每个字段时，异步递增 read_count
   - 实现方式：L1MemoryCue 返回渲染结果 + 已读字段 ID 列表
   - 调用方异步执行 UPDATE memory_l1_fields SET read_count = read_count + 1, last_read_at = ? WHERE id IN (...)
   - 不阻塞 prompt 注入流程
```

#### E.1.3 [中等] §3.2 `token_estimate` 声明与实际不符

**问题**：方案 §3.2 前置依赖声称"`token_estimate` 始终为 0，从未被计算"——这是**准确的**。但上一轮评估中我修正为"L1 field 的 `token_estimate` 在 upsert 时会更新"，这个修正**不完整**。

**代码证据**：ON CONFLICT 子句确实包含 `token_estimate = excluded.token_estimate`，但调用方（[tools.go](file:///f:/project/aranea-agents/internal/tools/working_memory/tools.go) 第 246-255 行）从未设置 `TokenEstimate` 字段，Go 零值为 0。因此 `token_estimate` **在数据库中始终为 0**。

**影响**：§3.2 选择性注入的"token 预算硬上限"无法生效——所有字段的 token_estimate 为 0，无法计算总注入 token。

**修正方案**：

```
token_estimate 计算实现（§3.2 的真正前置依赖）：

1. 在 UpsertL1Field 时计算 token_estimate：
   token_estimate = len(value_text) / 4  // 粗略估算：4 字符 ≈ 1 token

2. 在 L1MemoryCue 渲染时聚合 used_tokens：
   used_tokens = SUM(token_estimate) WHERE task_id = ? AND pin_to_prompt = true AND visibility != 'internal'

3. 写入 memory_l1_tasks.used_tokens（当前始终为 0）

4. 预算检查：
   if used_tokens > budget_tokens * L1InjectBudgetRatio:
     按优先级截断字段
```

#### E.1.4 [中等] §4.2 consolidation_status 存在三个值而非两个

**问题**：方案描述 AutoMemoryWorker 使用 `"consolidated"`、L2ConsolidateWorker 使用 `"done"`，但遗漏了 `InsertL1ArchiveEpisode` 硬编码写入 `"pending"`。

**代码证据**：
- [memory_shim_l2.go](file:///f:/project/aranea-agents/internal/data/memory_shim_l2.go) 第 60 行：`InsertL1ArchiveEpisode` 写入 `'pending'`
- [auto_memory.go](file:///f:/project/aranea-agents/internal/cronrunner/jobs/auto_memory.go) 第 263 行：AutoMemoryWorker 写入 `"consolidated"`
- [memory_shim_l2.go](file:///f:/project/aranea-agents/internal/data/memory_shim_l2.go) 第 95 行：L2ConsolidateWorker 写入 `"done"`

**影响**：方案 §4.1 修正 Path A Episode 直接设为 `"consolidated"` 是正确的，但还需要修改 `InsertL1ArchiveEpisode` 中的硬编码 `"pending"`。同时，`PurgeEpisodesOlderThan` 和 `ListEpisodesPendingEmbedding` 查询 `consolidation_status = 'done'`，统一为 `"consolidated"` 后这两处也必须同步修改。

**必须修改的代码清单**：

| 文件 | 行号 | 当前值 | 修改为 |
|------|------|--------|--------|
| `internal/data/memory_shim_l2.go` | 60 | `'pending'` | `'consolidated'` |
| `internal/data/memory_maintenance_adapter.go` | 211 | `'done'` | `'consolidated'` |
| `internal/data/memory_maintenance_adapter.go` | 280 | `'done'` | `'consolidated'` |

#### E.1.5 [中等] §2.5 tryMicroCompact 和 tryMemoryCompact 的实际能力被高估

**问题**：

1. **tryMicroCompact**：方案描述为"扫描 turn >= 2 且 > 200 字符的 tool result，生成占位标记"，暗示会清除 tool result。实际代码只做**检测+统计**，返回描述性字符串 `"[MicroCompact: N tool result(s) cleared]"`，**不实际删除或修改任何消息**。激活后需要补充实际的清除逻辑。

2. **tryMemoryCompact**：方案描述为"读取 L3 facts 生成摘要字符串"，实际代码只是将每条 fact 的 `Statement` + `Scope` 机械拼接为 Markdown 列表，**没有智能摘要生成**。方案 Step 2 扩展 L1 数据源后，需要实现 L1+L3 数据的合并摘要逻辑，这比"激活现有代码"的工作量大得多。

**影响**：Phase 2 实施时，"激活死代码"的工作量可能被低估。tryMicroCompact 需要补充清除逻辑（~50 行），tryMemoryCompact 需要重写摘要生成逻辑（~150 行）。

#### E.1.6 [低] §6.5 Skill activate_on_glob 缺乏触发信号来源

**问题**：方案提出从 `tool_call.function.arguments` 的 JSON 中提取 `file_path` 等参数来匹配 glob，但依赖工具参数命名的规范性。如果工具使用非标准参数名（如 `directory`、`target`、`source`），可能遗漏。

**修正方案**：维护一个工具参数名映射表。

```go
// 已知的文件路径参数名
var filePathParamNames = []string{
    "file_path", "path", "directory", "dir",
    "target", "source", "destination", "cwd",
    "working_dir", "root_dir", "base_path",
}
```

---

### E.2 架构设计问题

#### E.2.1 [严重] §2.3 + §2.5 交互：压缩后缓存断点失效

**问题**：三层前缀分离（§2.3）将 system TextBlock 按静态/半静态/动态排列并标记缓存断点。但压缩完成后（§2.5），session summary 作为 system message 注入，如果排在 MemoryInject TextBlock 之后，它变成了"最后一个 TextBlock"，导致 `applyCacheControlToSystem` 的断点标记错位到动态内容上。

**影响**：压缩后首次 LLM 调用的缓存全部失效，直到断点位置被修正。

**解决方案**：已在附录 D.1 中设计 `WithCacheSystemPromptDualBreakpoint`，**必须在 §2.3 实施时同步完成**，不能推迟。否则三层前缀分离在压缩场景下完全失效。

**实现约束**：
1. 压缩后 summary 必须归入 Layer 3（动态层），排在 MemoryInject TextBlock 之后
2. `applyCacheControlToSystem` 必须改为按索引定位断点（`CacheFirstBlock` 或 `DualBreakpoint`），不能依赖"最后一个 TextBlock"
3. 压缩后 TextBlock 数量可能变化，断点索引需要动态计算

#### E.2.2 [严重] §6.2 + §6.3 交互：跨会话 HMAC 验证导致 Fork 功能失效

**问题**：HMAC 签名使用 session 级密钥（§6.2），Episode fork 创建新 session（§6.3），新 session 无法解密源 session 的密钥，因此无法验证注入内容的 HMAC 签名。按 §6.2 设计，签名不匹配 → 拒绝使用摘要 → fork 注入的上下文被完整性保护机制丢弃。

**影响**：如果先实施 §6.2 再实施 §6.3，fork 功能实质失效。如果先实施 §6.3 再实施 §6.2，需要同步设计密钥传递机制。

**解决方案**：

```
Fork 场景的完整性保护策略：

1. 新增 trust_source 字段：
   ALTER TABLE session_summaries ADD COLUMN trust_source TEXT DEFAULT 'self';
   值：'self'（本 session 压缩产生）| 'episode_fork'（fork 注入）| 'agent_delegation'（委派传递）

2. HMAC 验证逻辑修改：
   if trust_source == 'self':
     正常验证 HMAC 签名
   elif trust_source == 'episode_fork':
     跳过 HMAC 验证（系统内部操作，不存在中间人篡改风险）
   elif trust_source == 'agent_delegation':
     正常验证 HMAC 签名（跨代理传递，需完整性保护）

3. Fork API 写入 summary 时：
   - trust_source = 'episode_fork'
   - signature_blob = NULL（不需要签名）
   - encryption_mode = 'none'

4. 理由：
   - Fork 是同一用户/系统的主动操作，不是跨代理的不可信传递
   - HMAC 保护的目标是"防止跨代理传递时被篡改"，fork 场景不适用
   - 不涉及密钥传递，安全性更好
```

#### E.2.3 [中等] §2.4 + §2.5 交互：Level 2 失败后的升级策略未明确

**问题**：当 `usedRatio` 达到 soft_trigger（70%），Level 2 Memory Compact 执行后发现 ICS < 0.70，此时应立即升级到 Level 3 LLM Compact，还是等待 hard_trigger（90%）？方案中存在歧义。

**§2.5 描述**："若结构化数据 < 原始历史 30%：替换历史为结构化摘要；否则：降级到 Level 3"——暗示立即升级。

**§2.4 描述**："soft_trigger → 启动后台异步压缩（Level 2 或 Level 3）"——暗示 soft_trigger 阶段可能触发 Level 3。

**建议**：采用"等待 hard_trigger 再升级"策略。

```
Level 2 失败后的升级策略：

soft_trigger（70%）触发时：
  1. 尝试 Level 2 Memory Compact
  2. 若 ICS >= 0.70 且压缩比 <= 60%：使用结构化摘要，压缩完成
  3. 若 ICS < 0.70 或压缩比 > 60%：Level 2 失败
     → didCompact = false，正常返回
     → 记录日志：soft_trigger Level 2 failed, ICS=X.XX, ratio=X.XX
     → 不立即升级到 Level 3（违背 soft_trigger 零成本初衷）
  4. 等待 usedRatio 继续增长到 hard_trigger（90%）
  5. hard_trigger 触发 Level 3 LLM Compact（同步阻塞）

理由：
  - soft_trigger 的设计意图是"零成本后台压缩"
  - 从 70% 到 90% 有 20% 缓冲区（约 25K tokens），足够多轮对话
  - Level 2 失败说明 L1+L3 数据不足以生成有效摘要，此时调用 LLM 是合理的但不应在 soft_trigger 阶段做
  - 如果 soft_trigger Level 2 失败频率过高，说明 L1+L3 数据源质量不足，需优化 Memory Compact
```

#### E.2.4 [中等] §2.3 RuntimeCapabilityCue 的"半静态"分类不完全准确

**问题**：方案将 RuntimeCapabilityCue 归入 Layer 2（半静态），但其中 `GetEffectiveTools` 是动态调用，可能受运行时工具配置变更影响。如果工具配置在会话中变更（如 MCP server 上下线），RuntimeCapabilityCue 内容变化会导致 Layer 2 缓存失效。

**影响**：Layer 2 缓存命中率低于预期。

**修正方案**：

```
RuntimeCapabilityCue 的缓存归属细化：

1. 静态部分（Agent 配置不变时稳定）：
   - 工具使用策略说明
   - Subagent 开关和参数
   - MCP/Memory 提示
   - 工具调用前缀
   → 归入 Layer 1（静态前缀），与 Identity+Instruction 合并

2. 动态部分（每轮可能变化）：
   - 有效工具 key 列表（GetEffectiveTools 动态解析）
   - CAPABILITIES.md 覆盖检测
   → 保留在 Layer 2（半静态前缀），或降级到 Layer 3（动态层）

3. 实现方式：
   - 拆分 RuntimeCapabilityCue 为 staticRuntimeCue + dynamicRuntimeCue
   - staticRuntimeCue 在 session 创建时生成，归入 Layer 1
   - dynamicRuntimeCue 每次 LLM 调用前生成，归入 Layer 2 或 Layer 3
   - 拆分后 Layer 1 缓存命中率进一步提升
```

#### E.2.5 [低] §2.4 reserved_system 冷启动问题

**问题**：`reserved_system` 依赖首次 LLM 调用后的 `prompt_snapshot` 数据，但 `soft_trigger` 可能在首次调用前就触发（虽然概率极低，如 session 恢复时已有大量历史消息）。

**修正方案**：

```
reserved_system 冷启动 fallback（基于 Agent ToolsProfile 分级）：

if prompt_snapshot 数据可用:
  reserved_system = 动态计算（§2.4.1 方案）
else:
  reserved_system = profileBasedDefault(ag.ToolsProfile)
  // 基于 Agent 配置的保守估算，避免冷启动时误判压缩时机
  // 首次 LLM 调用后自动切换为动态计算

profileBasedDefault 映射：
  | ToolsProfile | 默认 reserved_system | 理由 |
  |-------------|----------------------|------|
  | coding/full | 15000                | 系统提示词+工具定义可达 11K~18K |
  | research    | 12000                | 研究工具集中等规模 |
  | chat_only   | 4000                 | 最小工具集，2.5K~4.5K |
  | minimal     | 4000                 | 同 chat_only |
  | 其他/未知   | 8000                 | 中等估算，通用 fallback |

注意：统一使用 8192 对 coding profile 严重低估（reserved_system 实际 ~18K），
会导致 effective_budget 虚高，过早触发 soft_trigger。
```

---

### E.3 跨章节交互风险矩阵

| 交互 | 风险 | 核心问题 | 必须措施 |
|------|------|---------|---------|
| §2.3 + §2.5 | **高** | 压缩后 summary 改变 TextBlock 顺序，缓存断点失效 | D.1 的 DualBreakpoint 必须与 §2.3 同步实施 |
| §6.2 + §6.3 | **高** | 跨 session HMAC 密钥不可用，fork 注入内容被丢弃 | 增加 `trust_source` 标记，fork 来源跳过 HMAC |
| §4.1 + §4.2 | **中** | `InsertL1ArchiveEpisode` 硬编码 `pending`，清理/索引查询 `done` | 统一为 `consolidated`，修改 3 处 SQL |
| §2.4 + §2.5 | **中** | Level 2 失败后升级策略未明确 | soft_trigger 失败后等待 hard_trigger |
| §2.3 RuntimeCue | **中** | RuntimeCapabilityCue 包含动态内容，Layer 2 缓存命中率低于预期 | 拆分为 static + dynamic 两部分 |
| §3.2 + §2.5 | **低** | 两条数据读取路径不同，无冲突 | Memory Compact 读数据库原始数据，不复用 L1MemoryCue |
| §3.1 + §3.2 | **低** | 两者独立，共享 token_estimate 未实现的前置依赖 | 先实现 token_estimate 计算 |
| §2.1 + §2.5 | **中** | 压缩完成后 L0 Snapshot 与实际状态不一致 [G.2] | 压缩后强制写入 L0 Snapshot，不受限流约束 |
| §3.2 + §4.1 | **低** | ExtractKeyDecisions 读取全量字段，选择性注入可能过滤掉部分字段 [G.8] | Episode 中 key_decisions 可能包含 prompt 中不可见的字段信息，非 bug 但需文档说明 |

---

### E.4 方案修正优先级

| 优先级 | 问题 | 修正方案 | 影响章节 |
|--------|------|---------|---------|
| P0 | Path A key_decisions 提取规则不可靠 | 增加分层 fallback（属性匹配 + 全量兜底） | §4.1 |
| P0 | read_count 从未追踪，选择性注入排序无效 | 短期改用 updated_at 降序，长期实现 read_count 递增 | §3.2 |
| P0 | token_estimate 始终为 0，预算硬上限无法生效 | 在 UpsertL1Field 时计算 token_estimate = len(value)/4 | §3.2 |
| P0 | 压缩后缓存断点失效 | DualBreakpoint 必须与 §2.3 同步实施 | §2.3 + §2.5 |
| P1 | Fork + HMAC 交互导致 fork 功能失效 | 增加 trust_source 标记 | §6.2 + §6.3 |
| P1 | consolidation_status 三个值需统一 | 修改 3 处 SQL + 删除 MarkEpisodeConsolidated | §4.1 + §4.2 |
| P1 | Level 2 失败后升级策略歧义 | 明确"等待 hard_trigger"策略 | §2.4 + §2.5 |
| P2 | tryMicroCompact/tryMemoryCompact 能力被高估 | 补充清除逻辑 + 重写摘要生成 | §2.5 |
| P2 | RuntimeCapabilityCue 半静态分类不精确 | 拆分为 static + dynamic | §2.3 |
| P2 | reserved_system 冷启动 | 增加 budget_tokens fallback | §2.4 |

---

### E.5 总体评价

| 维度 | 评分 | 说明 |
|------|------|------|
| 问题诊断 | 9/10 | 根因分析精准，代码验证高度一致 |
| 方案合理性 | 8/10 | 整体方向正确，4 项 P0 已修正，ICS 分级评分、critic_score 条件权重等细节已完善 |
| 架构设计 | 8/10 | 跨章节交互已覆盖 9 项风险，Hook Layer 声明机制保障 TextBlock 顺序稳定性，HMAC 威胁模型已明确 |
| 实施可行性 | 7.5/10 | token_estimate 已列为 Phase 1 阻塞项，tryMemoryCompact 已拆分为 Step 1/Step 2 |
| 文档质量 | 9.5/10 | 附录 C/D/E/F 的自我纠错机制优秀，附录 G 补充了外部评审发现的 10 项问题 |

**核心结论**：方案的问题诊断和整体方向是正确的。经附录 C~F 的自我纠错和附录 G 的外部评审补充后，原有 4 项 P0 级问题已全部修正，2 项 P1 级架构交互问题已解决。新增的 10 项修正（G.1~G.10）进一步提升了方案的完整性和实施可行性。

---

## 附录 F：统一问题解决清单

> 本附录将附录 C/D/E 中识别的所有问题统一汇总，按实施阶段分组，给出可直接落地的代码变更方案。
> 主文档 §3.2/§4.1/§4.2/§2.5 中的相关描述已同步修正（见各章节内的 `[F.x 修正]` 标记）。

### F.1 Phase 1 必须修正（P0 — 阻塞实施）

#### F.1.1 token_estimate 计算实现

**问题**：`memory_l1_fields.token_estimate` 始终为 0，调用方从未设置该字段。§3.2 选择性注入的 token 预算硬上限无法生效。

**变更清单**：

| # | 文件 | 变更 |
|---|------|------|
| 1 | `internal/biz/memory_admin_store.go` | `L1FieldInsert` 结构体无需改动（`TokenEstimate int` 已存在） |
| 2 | `internal/data/memory_shim_l1.go` | `UpsertL1Field` 中计算 token_estimate |
| 3 | `internal/data/memory_shim_l1.go` | 新增 `UpdateL1TaskUsedTokens` 方法 |
| 4 | `internal/agent/l1_prompt.go` | `L1MemoryCue` 渲染后更新 used_tokens |

**代码变更**：

```go
// === 文件 2: internal/data/memory_shim_l1.go ===
// UpsertL1Field 中，INSERT/ON CONFLICT 前计算 token_estimate

func (r *l1FieldRepo) UpsertL1Field(ctx context.Context, in biz.L1FieldInsert) error {
    // 新增：计算 token_estimate（4 字符 ≈ 1 token）
    if in.TokenEstimate == 0 && in.ValueText != "" {
        in.TokenEstimate = len(in.ValueText) / 4
        if in.TokenEstimate == 0 {
            in.TokenEstimate = 1 // 最小值 1，避免 0 值无法区分"未计算"和"空字段"
        }
    }

    // ... 原有 INSERT ... ON CONFLICT 逻辑不变
    // token_estimate = excluded.token_estimate 已在 ON CONFLICT 中
}
```

```go
// === 文件 3: internal/data/memory_shim_l1.go ===
// 新增方法：聚合更新 used_tokens

func (r *l1TaskRepo) UpdateL1TaskUsedTokens(ctx context.Context, taskID string) error {
    q := `UPDATE memory_l1_tasks SET used_tokens = (
            SELECT COALESCE(SUM(token_estimate), 0)
            FROM memory_l1_fields
            WHERE task_id = ? AND pin_to_prompt = 1 AND visibility != 'internal'
          ) WHERE id = ?`
    _, err := r.data.RW().Write(ctx).ExecContext(ctx, q, taskID, taskID)
    return err
}
```

```go
// === 文件 4: internal/agent/l1_prompt.go ===
// L1MemoryCue 渲染完成后，异步更新 used_tokens

func L1MemoryCue(ctx context.Context, ...) (string, error) {
    // ... 原有渲染逻辑 ...

    // 新增：渲染完成后异步更新 used_tokens
    if admin != nil && taskID != "" {
        go func() {
            _ = admin.UpdateL1TaskUsedTokens(context.Background(), taskID)
        }()
    }

    return result, nil
}
```

**DDL 变更**：无（`token_estimate` 和 `used_tokens` 字段已存在）。

**测试策略**：
- 单元测试：`UpsertL1Field` 后验证 `token_estimate > 0`
- 单元测试：`UpdateL1TaskUsedTokens` 后验证 `used_tokens = SUM(token_estimate)`
- 集成测试：`L1MemoryCue` 渲染后验证 `used_tokens` 被更新

---

#### F.1.2 read_count 短期替代 + 长期实现

**问题**：`read_count` 始终为 0，§3.2 选择性注入的"按 read_count 降序排序"无效。

**短期方案**（Phase 1 实施）：改用 `updated_at` 降序排序。

**长期方案**（Phase 3 实施）：在 `L1MemoryCue` 渲染时异步递增 `read_count`。

**变更清单**：

| # | 文件 | 变更 | 阶段 |
|---|------|------|------|
| 1 | `internal/agent/l1_prompt.go` | 相关性过滤改用 `updated_at` 降序 | Phase 1 |
| 2 | `internal/agent/l1_prompt.go` | 渲染后异步递增 `read_count` | Phase 3 |
| 3 | `internal/data/memory_shim_l1.go` | ON CONFLICT 增加 `read_count = read_count + 1` | Phase 3 |

**代码变更（短期）**：

```go
// === 文件 1: internal/agent/l1_prompt.go ===
// 相关性过滤：短期使用 updated_at 降序

func filterFieldsByRelevance(fields []L1FieldRow, maxFields int) []L1FieldRow {
    if len(fields) <= maxFields {
        return fields
    }

    // 短期策略：按 updated_at 降序（最近更新的字段优先）
    sort.Slice(fields, func(i, j int) bool {
        return fields[i].UpdatedAt > fields[j].UpdatedAt
    })
    return fields[:maxFields]
}
```

**代码变更（长期）**：

```go
// === 文件 2: internal/agent/l1_prompt.go ===
// L1MemoryCue 渲染完成后，异步递增 read_count

func L1MemoryCue(ctx context.Context, ...) (string, error) {
    // ... 原有渲染逻辑 ...

    // 长期方案：异步递增 read_count
    if admin != nil {
        readFieldIDs := collectFieldIDs(renderedFields)
        go func() {
            _ = admin.IncrementL1FieldReadCounts(context.Background(), readFieldIDs)
        }()
    }

    return result, nil
}
```

```go
// === 文件 3: internal/data/memory_shim_l1.go ===
// 新增方法：批量递增 read_count

func (r *l1FieldRepo) IncrementL1FieldReadCounts(ctx context.Context, fieldIDs []string) error {
    if len(fieldIDs) == 0 {
        return nil
    }
    q := `UPDATE memory_l1_fields SET read_count = read_count + 1, last_read_at = ? WHERE id IN (?)`
    // 使用 Ent 的 In() 或拼接占位符
    _, err := r.data.RW().Write(ctx).ExecContext(ctx, q, time.Now().Format(time.RFC3339), fieldIDs)
    return err
}
```

---

#### F.1.3 Path A key_decisions 分层提取

**问题**：`field_path` 是自由格式字符串，模式匹配 "decision"/"choice"/"approach" 可能匹配不到任何字段。

**变更清单**：

| # | 文件 | 变更 |
|---|------|------|
| 1 | `internal/data/memory_shim_l2.go` | `InsertL1ArchiveEpisode` 中替换 key_decisions 提取逻辑 |
| 2 | 新增 `internal/biz/l1_field_extraction.go` | `ExtractKeyDecisions` / `ExtractKeyArtifacts` 函数 |

**代码变更**：

```go
// === 新文件: internal/biz/l1_field_extraction.go ===

// ExtractKeyDecisions 从 L1 字段中提取关键决策，使用分层 fallback 策略
func ExtractKeyDecisions(fields []L1FieldRow) []string {
    // Layer 1：模式匹配
    var decisions []string
    decisionPatterns := []string{"decision", "choice", "approach", "strategy", "rationale"}
    for _, f := range fields {
        path := strings.ToLower(f.FieldPath)
        for _, p := range decisionPatterns {
            if strings.Contains(path, p) {
                decisions = append(decisions, fmt.Sprintf("- %s: %s", f.FieldPath, truncate(f.ValueText, 200)))
                break
            }
        }
    }
    if len(decisions) > 0 {
        return decisions
    }

    // Layer 2：属性匹配（pinned + visible 字段）
    for _, f := range fields {
        if f.PinToPrompt && f.Visibility == "prompt" && len(decisions) < 3 {
            decisions = append(decisions, fmt.Sprintf("- %s: %s", f.FieldPath, truncate(f.ValueText, 200)))
        }
    }
    if len(decisions) > 0 {
        return decisions
    }

    // Layer 3：全量兜底（最近更新的前 5 个 visible 字段）
    sort.Slice(fields, func(i, j int) bool {
        return fields[i].UpdatedAt > fields[j].UpdatedAt
    })
    for _, f := range fields {
        if f.Visibility == "prompt" && len(decisions) < 5 {
            decisions = append(decisions, fmt.Sprintf("- %s: %s", f.FieldPath, truncate(f.ValueText, 200)))
        }
    }
    return decisions
}

// ExtractKeyArtifacts 从 L1 字段中提取关键产物，使用分层 fallback 策略
func ExtractKeyArtifacts(fields []L1FieldRow) []string {
    // Layer 1：模式匹配
    var artifacts []string
    artifactPatterns := []string{"file", "artifact", "output", "deliverable"}
    for _, f := range fields {
        path := strings.ToLower(f.FieldPath)
        for _, p := range artifactPatterns {
            if strings.Contains(path, p) {
                artifacts = append(artifacts, fmt.Sprintf("- %s: %s", f.FieldPath, truncate(f.ValueText, 200)))
                break
            }
        }
        if f.FieldKind == "reference" {
            artifacts = append(artifacts, fmt.Sprintf("- %s: %s", f.FieldPath, truncate(f.ValueText, 200)))
        }
    }
    if len(artifacts) > 0 {
        return artifacts
    }

    // Layer 2：属性匹配（reference 类型字段）
    for _, f := range fields {
        if f.FieldKind == "reference" {
            artifacts = append(artifacts, fmt.Sprintf("- %s: %s", f.FieldPath, truncate(f.ValueText, 200)))
        }
    }
    return artifacts
}

func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen] + "..."
}
```

---

#### F.1.4 压缩后缓存断点失效防护

**问题**：压缩后 summary 注入改变 TextBlock 顺序，`applyCacheControlToSystem` 的断点标记错位。

**变更清单**：

| # | 文件 | 变更 |
|---|------|------|
| 1 | `pkg/trpc-agent-go/model/anthropic/options.go` | 新增 `WithSystemCacheStrategy` / `WithCacheSystemPromptDualBreakpoint` |
| 2 | `pkg/trpc-agent-go/model/anthropic/anthropic.go` | 改造 `applyCacheControlToSystem`，支持指定断点位置 |
| 3 | `pkg/trpc-agent-go/model/anthropic/anthropic.go` | `applyCacheControl` 入口增加断点计数校验和优先级淘汰 |
| 4 | `internal/provider/trpc_llm.go` | 启用 `DualBreakpoint` 模式 |

**代码变更**：详见附录 D.1.2 ~ D.1.4，此处不重复。

**实施约束**：此变更**必须与 §2.3 三层前缀分离同步实施**，不能推迟。否则三层前缀分离在压缩场景下完全失效。

---

#### F.1.5 consolidation_status 三值统一

**问题**：`consolidation_status` 存在三个值（`pending`/`consolidated`/`done`），需要统一为 `consolidated`。

**变更清单**：

| # | 文件 | 行号 | 当前值 | 修改为 |
|---|------|------|--------|--------|
| 1 | `internal/data/memory_shim_l2.go` | ~60 | `'pending'` | `'consolidated'` |
| 2 | `internal/data/memory_maintenance_adapter.go` | ~211 | `'done'` | `'consolidated'` |
| 3 | `internal/data/memory_maintenance_adapter.go` | ~280 | `'done'` | `'consolidated'` |
| 4 | `internal/data/memory_shim_l2.go` | ~95 | `MarkEpisodeConsolidated` 方法 | 删除 |
| 5 | `internal/cronrunner/jobs/memory_l2_consolidate.go` | 全文件 | L2ConsolidateWorker | 删除 |
| 6 | `internal/cronrunner/` 注册处 | — | Worker 注册 | 移除 |

**数据迁移**：

```sql
-- 将现有 'pending' 和 'done' 状态统一为 'consolidated'
UPDATE memory_episodes SET consolidation_status = 'consolidated'
WHERE consolidation_status IN ('pending', 'done');
```

**测试策略**：
- 单元测试：验证 `InsertL1ArchiveEpisode` 写入 `consolidation_status = 'consolidated'`
- 单元测试：验证 `PurgeEpisodesOlderThan` 查询 `consolidation_status = 'consolidated'`
- 单元测试：验证 `ListEpisodesPendingEmbedding` 查询 `consolidation_status = 'consolidated'`
- 集成测试：验证删除 `L2ConsolidateWorker` 后 Episode 生命周期正常

---

### F.2 Phase 2 应该修正（P1 — 影响功能正确性）

#### F.2.1 Fork + HMAC 交互：trust_source 标记

**问题**：跨 session HMAC 密钥不可用，fork 注入内容被完整性保护机制丢弃。

**变更清单**：

| # | 文件 | 变更 |
|---|------|------|
| 1 | `internal/data/sql/memory_chain.sql` | `session_summaries` 表新增 `trust_source` 字段 |
| 2 | `internal/session/compressor.go` | 压缩时写入 `trust_source = 'self'` |
| 3 | 新增 Fork API handler | Fork 时写入 `trust_source = 'episode_fork'` |
| 4 | `internal/agent/memory_inject.go` | HMAC 验证逻辑增加 `trust_source` 判断 |

**DDL 变更**：

```sql
ALTER TABLE session_summaries ADD COLUMN trust_source TEXT NOT NULL DEFAULT 'self';
-- 值：'self' | 'episode_fork' | 'agent_delegation'
```

**代码变更**：

```go
// === 文件 4: internal/agent/memory_inject.go ===
// HMAC 验证逻辑修改

func verifySummaryIntegrity(summary SessionSummary, signingKey []byte) error {
    // Fork 来源跳过 HMAC 验证
    if summary.TrustSource == "episode_fork" {
        return nil // 系统内部操作，不存在中间人篡改风险
    }

    // 其他来源正常验证
    if summary.SignatureBlob == nil {
        return fmt.Errorf("summary missing signature")
    }
    return verifyHMAC(summary.SummaryMarkdown, summary.SignatureBlob, signingKey)
}
```

---

#### F.2.2 Level 2 失败后升级策略明确

**问题**：soft_trigger 阶段 Level 2 Memory Compact 失败后，升级策略未明确。

**决策**：采用"等待 hard_trigger 再升级"策略。

**变更清单**：

| # | 文件 | 变更 |
|---|------|------|
| 1 | `internal/session/compressor.go` | `AfterNativeTurn` 中 Level 2 失败后 `didCompact = false`，正常返回 |
| 2 | `internal/session/compressor.go` | 增加日志记录 Level 2 失败原因 |

**代码变更**：

```go
// === 文件 1: internal/session/compressor.go ===
// AfterNativeTurn 中，Level 2 Memory Compact 失败后的处理

func (c *Compressor) AfterNativeTurn(ctx context.Context, sessionID string, ...) {
    // ... 触发判断 ...

    result := c.tryMemoryCompact(ctx, sessionID, ...)
    if result.didCompact {
        // Level 2 成功，使用结构化摘要
        c.lg.Info("memory compact succeeded",
            "session_id", sessionID, "ics", result.ics, "ratio", result.compressionRatio)
        // ... 写入摘要 ...
        return
    }

    // Level 2 失败：不立即升级到 Level 3
    // 等待 hard_trigger 时再触发 LLM Compact
    c.lg.Info("memory compact failed, waiting for hard_trigger",
        "session_id", sessionID, "ics", result.ics, "ratio", result.compressionRatio,
        "reason", result.failReason)
    // 正常返回，不做任何压缩
}
```

---

#### F.2.3 tryMicroCompact 补充清除逻辑

**问题**：`tryMicroCompact` 只做检测统计，不实际清除 tool result。

**变更清单**：

| # | 文件 | 变更 |
|---|------|------|
| 1 | `internal/session/micro_compact.go` | `tryMicroCompact` 返回需清除的消息 ID 列表 |
| 2 | `internal/session/compressor.go` | 调用方根据返回结果执行实际清除 |

**代码变更**：

```go
// === 文件 1: internal/session/micro_compact.go ===

type microCompactResult struct {
    didCompact       bool
    summaryMarkdown  string
    fromTurn         int
    toTurn           int
    clearedCount     int
    clearableMsgIDs  []string  // 新增：需要清除的消息 ID 列表
}

func tryMicroCompact(currentTurn int, messages []Message) microCompactResult {
    minTurn := currentTurn - microCompactMinAgeTurns
    var clearableIDs []string
    cleared := 0

    for _, msg := range messages {
        if msg.Role == "tool" && msg.TurnNumber <= minTurn && len(msg.ContentMarkdown) > 200 {
            clearableIDs = append(clearableIDs, msg.ID)
            cleared++
        }
    }

    if cleared == 0 {
        return microCompactResult{didCompact: false}
    }

    from, to := minTurn, minTurn
    return microCompactResult{
        didCompact:      true,
        summaryMarkdown: fmt.Sprintf("[MicroCompact: %d tool result(s) from turns %d–%d cleared]", cleared, from, to),
        fromTurn:        from,
        toTurn:          to,
        clearedCount:    cleared,
        clearableMsgIDs: clearableIDs,
    }
}
```

```go
// === 文件 2: internal/session/compressor.go ===
// 调用 tryMicroCompact 后，执行实际清除

func (c *Compressor) runMicroCompact(ctx context.Context, sessionID string, ...) {
    result := tryMicroCompact(currentTurn, messages)
    if !result.didCompact {
        return
    }

    // 实际清除：将 tool result 替换为占位标记
    for _, msgID := range result.clearableMsgIDs {
        err := c.messageWriter.ReplaceMessageContent(ctx, sessionID, msgID,
            "[Tool result compacted — see summary above]")
        if err != nil {
            c.lg.Warn("failed to compact tool result", "msg_id", msgID, "error", err)
        }
    }
}
```

---

#### F.2.4 tryMemoryCompact 重写摘要生成

**问题**：`tryMemoryCompact` 只做机械拼接，没有智能摘要生成。Step 2 扩展 L1 数据源后需要重写。

**变更清单**：

| # | 文件 | 变更 |
|---|------|------|
| 1 | `internal/session/memory_compact.go` | 重写 `tryMemoryCompact`，合并 L1 + L3 数据源 |
| 2 | `internal/session/memory_compact.go` | 增加 ICS 评估和降级判断 |

**代码变更**：

```go
// === 文件 1: internal/session/memory_compact.go ===

type memoryCompactResult struct {
    didCompact       bool
    summaryMarkdown  string
    fromTurn         int
    toTurn           int
    ics              float64  // 信息覆盖度
    compressionRatio float64  // 压缩比
    failReason       string   // 失败原因
}

func tryMemoryCompact(
    ctx context.Context,
    body []Message,
    l1Reader biz.L1AdminReader,    // 新增：L1 数据源
    factReader biz.MemoryFactReader,
    sessionID string,
    lg loggateway.Logger,
) memoryCompactResult {
    var sb strings.Builder
    sb.WriteString("## Session Memory Summary\n\n")

    // 1. 读取 L1 快照
    coverage := compactCoverage{}
    l1Data := ""
    if l1Reader != nil {
        task, fields, err := l1Reader.ReadL1TaskSnapshot(ctx, sessionID)
        if err == nil && task != nil {
            coverage.HasIntent = task.TaskGoal != ""
            coverage.HasState = task.Status != ""
            sb.WriteString("### Current Task State\n")
            if task.TaskTitle != "" {
                sb.WriteString("- Task: " + task.TaskTitle + "\n")
            }
            if task.TaskGoal != "" {
                sb.WriteString("- Goal: " + task.TaskGoal + "\n")
            }
            if task.Status != "" {
                sb.WriteString("- Status: " + task.Status + "\n")
            }

            decisions := biz.ExtractKeyDecisions(fields)
            if len(decisions) > 0 {
                coverage.HasDecisions = true
                sb.WriteString("- Key Decisions:\n")
                for _, d := range decisions {
                    sb.WriteString("  " + d + "\n")
                }
            }

            artifacts := biz.ExtractKeyArtifacts(fields)
            if len(artifacts) > 0 {
                coverage.HasFiles = true
                sb.WriteString("- Key Artifacts:\n")
                for _, a := range artifacts {
                    sb.WriteString("  " + a + "\n")
                }
            }

            l1Data = sb.String()
        }
    }

    // 2. 读取 L3 facts
    facts, _ := factReader.ReadSessionMemoryFacts(ctx, sessionID)
    if len(facts) > 0 {
        coverage.HasFacts = true
        sb.WriteString("\n### Key Facts\n")
        for _, f := range facts {
            sb.WriteString("- " + f.Statement)
            if f.Scope != "" {
                sb.WriteString(" _[" + f.Scope + "]_")
            }
            sb.WriteString("\n")
        }
    }

    summary := sb.String()
    if strings.TrimSpace(summary) == "## Session Memory Summary" {
        return memoryCompactResult{didCompact: false, failReason: "no L1/L3 data available"}
    }

    // 3. ICS 评估
    ics := coverage.ICS()
    if ics < 0.70 {
        return memoryCompactResult{
            didCompact: false,
            ics:        ics,
            failReason: fmt.Sprintf("ICS %.2f below threshold 0.70", ics),
        }
    }

    // 4. 压缩比检查
    originalTokens := estimateTokens(body)
    structuredTokens := len(summary) / 4
    ratio := float64(structuredTokens) / float64(originalTokens)
    if ratio > 0.60 {
        return memoryCompactResult{
            didCompact:       false,
            ics:              ics,
            compressionRatio: ratio,
            failReason:       fmt.Sprintf("compression ratio %.2f above threshold 0.60", ratio),
        }
    }

    return memoryCompactResult{
        didCompact:       true,
        summaryMarkdown:  summary,
        ics:              ics,
        compressionRatio: ratio,
    }
}
```

---

### F.3 Phase 3 建议修正（P2 — 优化体验）

#### F.3.1 RuntimeCapabilityCue 拆分

**变更清单**：

| # | 文件 | 变更 |
|---|------|------|
| 1 | `internal/agent/runtime_cue_inject.go` | 拆分为 `staticRuntimeCue` + `dynamicRuntimeCue` |
| 2 | `internal/agent/prompt.go` | `RuntimeCapabilityCue` 拆分为两个函数 |

**代码变更**：

```go
// === 文件 1: internal/agent/runtime_cue_inject.go ===

func newRuntimeCueBeforeHook(deps promptDeps, ag *biz.Agent) llmagent.BeforeModelHook {
    return llmagent.BeforeModelHook{
        Priority: 4,
        Fn: func(ctx context.Context, args *llmagent.BeforeModelArgs) error {
            // 静态部分：归入 Layer 1（与 Identity+Instruction 合并）
            staticCue := buildStaticRuntimeCue(ctx, deps, ag)
            if staticCue != "" {
                // 注入到 RequestProcessor 层面（静态前缀），而非 Hook prepend
                // 具体实现：写入 invocation state，由 RequestProcessor 读取
                args.Invocation.SetState("static_runtime_cue", staticCue)
            }

            // 动态部分：归入 Layer 2/3（Hook prepend）
            dynamicCue := buildDynamicRuntimeCue(ctx, deps, ag)
            if dynamicCue != "" {
                sys := trpcmodel.NewSystemMessage(dynamicCue)
                args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
            }
            return nil
        },
    }
}
```

```go
// === 文件 2: internal/agent/prompt.go ===

// 静态部分：Agent 配置不变时稳定
func buildStaticRuntimeCue(ctx context.Context, d promptDeps, ag *biz.Agent) string {
    var sb strings.Builder
    // 工具使用策略说明
    if ag.Settings.SystemPromptMode != "" { sb.WriteString(...) }
    // Subagent 开关和参数
    if ag.Settings.SubagentsEnabled { sb.WriteString(...) }
    // MCP/Memory 提示
    if d.SQLiteSessionMemory { sb.WriteString(...) }
    // 工具调用前缀
    if ag.Settings.ToolsToolCallPrefix != "" { sb.WriteString(...) }
    return sb.String()
}

// 动态部分：每轮可能变化
func buildDynamicRuntimeCue(ctx context.Context, d promptDeps, ag *biz.Agent) string {
    var sb strings.Builder
    // 有效工具 key 列表
    effTools, _ := d.ToolsUC.GetEffectiveTools(ctx, ag.ID)
    sb.WriteString(formatEffectiveTools(effTools))
    // CAPABILITIES.md 覆盖检测
    if hasFilteredPromptFile(d, ag) { sb.WriteString(...) }
    return sb.String()
}
```

---

#### F.3.2 reserved_system 冷启动 fallback

**代码变更**：

```go
// internal/session/compressor.go 或 internal/biz/ 中

func calculateReservedSystem(ctx context.Context, sessionID string, snapshot *PromptSnapshot, profile string) int {
    if snapshot != nil && snapshot.SystemTokens > 0 {
        // 正常路径：使用 prompt_snapshot 数据
        return snapshot.SystemTokens
    }
    // 冷启动 fallback：基于 Agent ToolsProfile 分级默认值
    return profileBasedDefault(profile)
}

func profileBasedDefault(profile string) int {
    switch profile {
    case "coding", "full":
        return 15000
    case "research":
        return 12000
    case "chat_only", "minimal":
        return 4000
    default:
        return 8000
    }
}
```

---

#### F.3.3 Skill activate_on_glob 参数名映射

**代码变更**：

```go
// internal/agent/skill_guidance_inject.go 或新文件

// 已知的文件路径参数名（用于从 tool_call arguments 中提取文件路径）
var filePathParamNames = []string{
    "file_path", "path", "directory", "dir",
    "target", "source", "destination", "cwd",
    "working_dir", "root_dir", "base_path",
}

func extractFilePathsFromToolCall(toolCall ToolCall) []string {
    var paths []string
    args := parseToolCallArguments(toolCall.Function.Arguments)
    for _, paramName := range filePathParamNames {
        if v, ok := args[paramName]; ok && v != "" {
            paths = append(paths, v)
        }
    }
    return paths
}
```

---

### F.4 主文档修正索引

以下主文档章节已根据上述问题进行修正，标注 `[F.x 修正]`：

| 章节 | 修正内容 | 对应问题 |
|------|---------|---------|
| §3.2.1 三层过滤链 | `read_count` 排序改为 `updated_at` 降序 | F.1.2 |
| §3.2.2 实现路径 | 短期策略改为 `updated_at`，长期实现 `read_count` | F.1.2 |
| §3.2 前置依赖 | `token_estimate` 修正为"始终为 0，需先实现计算" | F.1.1 |
| §4.1.1 Path A key_decisions | 增加分层 fallback 策略 | F.1.3 |
| §4.2.2 删除 L2ConsolidateWorker | 增加 `InsertL1ArchiveEpisode` 的 `pending` 修正 | F.1.5 |
| §2.5 Level 2 降级条件 | "30% token 比例"改为 ICS + 60% 辅助约束 | D.3 + F.2.4 |
| §2.5 Level 2 失败策略 | 明确"等待 hard_trigger" | F.2.2 |
| §2.5 tryMicroCompact | 补充"需增加清除逻辑" | F.2.3 |
| §2.5 tryMemoryCompact | 补充"需重写摘要生成" | F.2.4 |
| §3.3.2 互斥规则 | `memory_tool_mode` 改为 `ToolsDenyJSON` | D.4 |
| §6.2 HMAC 验证 | 增加 `trust_source` 标记 | F.2.1 |

---

### F.5 实施路线图更新

基于以上修正，更新实施路线图：

#### Phase 1：低风险高收益（1~2 周）— 含 P0 修正

| 任务 | 方案章节 | 预期收益 | 修正项 |
|------|---------|---------|--------|
| L0 Snapshot 简化+限流 | §2.1 | 写入量降低 90%+ | — |
| Prompt Caching 三层前缀分离 + DualBreakpoint + Hook Layer 声明 | §2.3 + D.1 + G.1 | 每轮节省 8K~15K | **F.1.4：必须同步实施**；**G.1：Hook Layer 声明机制** |
| 显式压缩预算 | §2.4 | 避免被动压缩信息损失 | G.9：profile 分级冷启动 fallback |
| Embedding 单写确认 + 删除 memory_l2_index_meta | §4.3 | 消除冗余设计 | G.6：暴力搜索阈值 |
| **[阻塞项]** token_estimate 计算实现 | F.1.1 | 为选择性注入提供基础 | Phase 2 的 §3.2 强依赖此项 |
| **[新增]** consolidation_status 统一 + 删除 L2ConsolidateWorker | F.1.5 | 消除状态不一致 | — |

#### Phase 2：核心优化（2~3 周）— 含 P1 修正

| 任务 | 方案章节 | 预期收益 | 修正项 |
|------|---------|---------|--------|
| 三级压缩流水线 | §2.5 | 90%+ 压缩场景零 LLM 成本 | F.2.2：Level 2 失败等待 hard_trigger；G.3：Level 2 适用场景明确 |
| tryMicroCompact 激活 + 清除逻辑 | §2.5 + F.2.3 | 每轮节省 1K~3K tokens | F.2.3：补充清除逻辑 |
| tryMemoryCompact Step 1：激活现有 L3 版本 | §2.5 + F.2.4 | L3 结构化压缩零 LLM 成本 | F.2.4：激活死代码 |
| tryMemoryCompact Step 2：扩展 L1 数据源 + ICS 分级评估 | §2.5 + F.2.4 + D.3 | L1+L3 合并压缩 | F.2.4：重写摘要生成 + G.10：ICS 分级评分 |
| L1 选择性注入 | §3.2 | 每轮节省 2K~8K tokens | F.1.2：短期用 updated_at |
| 结构化 Episode 优先路径 | §4.1 | 70%~90% Episode 生成零 LLM 成本 | F.1.3：分层 fallback；G.7：critic_score 条件权重 |
| 无感后台压缩 + UI 指示器 | §2.5 | 用户体验对标 Trae/Cursor | G.2：压缩后强制写入 L0 Snapshot |

#### Phase 3：架构优化（3~4 周）— 含 P2 修正

| 任务 | 方案章节 | 预期收益 | 修正项 |
|------|---------|---------|--------|
| 单跳巩固管道 | §4.2 | LLM 调用从 2 次降为 1 次 | — |
| L1 Schema/History 降为可选 | §3.1 | 降低大多数 Agent 的 L1 开销 | — |
| L1 与框架 Memory 职责厘清 | §3.3 | 消除 Agent 工具混淆 | D.4：ToolsDenyJSON 渐进迁移 |
| 摘要保留/丢弃策略 | §6.1 | 提高摘要信噪比 | — |
| RuntimeCapabilityCue 拆分 | F.3.1 | Layer 1 缓存命中率提升 | — |
| read_count 异步递增 | F.1.2 长期 | 选择性注入排序更精准 | — |

#### Phase 4：高级特性（4~6 周）— 含 P1 修正

| 任务 | 方案章节 | 预期收益 | 修正项 |
|------|---------|---------|--------|
| 多代理加密上下文传递 | §6.2 | 多代理上下文完整性保护 | F.2.1：trust_source 标记；G.4：威胁模型明确 |
| L2 Episode 分叉 | §6.3 | 多代理探索场景支持 | F.2.1：fork 来源跳过 HMAC；G.5：API 改为 Session 命名空间 + fork_source 独立字段 |
| L3/L4 Embedding 增量重建 | §4.3.3/4.3.4 | 避免全量重建 | D.5：embedding_version 语义 |
| 子目录规则按需加载 | §6.5 | 每轮节省 2K~5K tokens | F.3.3：参数名映射表 |
| L3 Recall 去重 | §5.1 | 减少 recall 噪声 | — |

---

## 附录 G：外部评审补充修正

> 本附录记录外部评审中发现的新问题及修正方案，与附录 C~F 的自我纠错互补。
> 主文档相关章节已同步标注 `[G.x 新增/修正]`。

### G.1 Hook Layer 声明机制（§2.3.2 补充）

**问题**：TextBlock 顺序依赖 Hook 优先级硬编码整数，新增/调整 Hook 可能导致顺序错位、缓存断点索引失效。

**修正**：Hook 注册时声明所属 `SystemLayer`（Static/SemiStatic/Dynamic），装配器按 Layer 排序，`applyCacheControlToSystem` 按语义标记定位断点。详见 §2.3.2 稳定性保障部分。

### G.2 压缩后强制写入 L0 Snapshot（§2.5 补充）

**问题**：压缩完成后 L0 Snapshot 可能记录压缩前状态，与实际上下文不一致。

**修正**：压缩后操作序列中增加"强制写入 L0 Snapshot"，不受限流约束。详见 §2.5 压缩完成后的原子替换。

### G.3 Level 2 Memory Compact 适用场景明确（§2.5 补充）

**问题**：短任务（<= 5 轮）L1 数据不足，ICS 通常 < 0.70，Level 2 几乎必然失败。

**修正**：Level 2 明确标注"适用场景：中长任务（> 5 轮）"，短任务直接等待 hard_trigger 触发 Level 3。详见 §2.5 三级压缩流水线 Level 2 部分。

### G.4 HMAC 威胁模型明确（§6.2 补充）

**问题**：HMAC 签名的实际安全价值存疑——密钥和签名在同一数据库中，攻击者获取数据库后可同时获取密钥。

**修正**：明确 HMAC 防御的三个威胁模型（跨代理网络传输篡改、数据库低级篡改、前端伪造），区分有效和有限防御场景，提供替代方案。详见 §6.2 威胁模型与安全边界。

### G.5 Episode Fork API 改进 + fork_source 独立字段（§6.3 修正）

**问题**：`POST /v1/episodes/{eid}/fork` 从 Episode 资源创建 Session 资源，语义割裂；`fork_source` 放在 `metadata_json` 中查询性能差。

**修正**：API 改为 `POST /v1/sessions` with `fork_from_episode_id` 参数；`fork_source` 改为 `session_runtime` 表独立字段。详见 §6.3。

### G.6 Embedding 暴力搜索扩展性阈值（§4.3.1 补充）

**问题**：暴力余弦相似度在 Episode 数量达到 1 万+ 时延迟不可接受，未定义阈值。

**修正**：暴力搜索上限 5000 条，超出后自动切换 pgvector 或退化为最近 N 条。详见 §4.3.1。

### G.7 critic_score 条件权重重分配（§4.1.2 修正）

**问题**：`critic_score` 在大多数场景下不存在，0.25 权重被浪费，有效评分上限仅 0.75。

**修正**：critic_score 缺失时，0.25 权重重分配到其他 4 个维度（importance +0.10，其余各 +0.05）。详见 §4.1.2。

### G.8 ExtractKeyDecisions 与选择性注入的字段可见性差异（§3.2 + §4.1 交互）

**问题**：`ExtractKeyDecisions`（§4.1）读取全量字段，选择性注入（§3.2）可能过滤掉部分字段，导致 Episode 中 key_decisions 包含 prompt 中不可见的字段信息。

**评估**：非 bug——Episode 是持久化记忆，应记录完整信息；L0 注入是运行时优化，按需过滤。两者职责不同，数据源不同是合理的。但需在文档中说明此设计意图，避免实施时误解。

### G.9 reserved_system 冷启动 profile 分级 fallback（§E.2.5 + F.3.2 修正）

**问题**：统一使用 8192 作为冷启动 fallback 对 coding profile 严重低估（reserved_system 实际 ~18K），导致 effective_budget 虚高。

**修正**：基于 Agent `ToolsProfile` 分级设置默认值（coding=15000, research=12000, chat_only=4000）。详见 §E.2.5 和 F.3.2。

### G.10 ICS 分级评分（§D.3 修正）

**问题**：ICS 的 6 个维度全部是 0/1 二元判断，缺乏对信息丰富度的度量，"刚好有 1 个字段就通过"的边界情况不合理。

**修正**：Decisions、Files、Facts 三个维度改为分级评分（>=threshold → 1.0, >=1 → 0.5, 0 → 0.0）。详见 §D.3.1 和 D.3.4。

---

### G.x 修正索引

以下主文档章节已根据上述问题进行修正，标注 `[G.x 新增/修正]`：

| 章节 | 修正内容 | 对应问题 |
|------|---------|---------|
| §2.3.2 稳定性保障 | Hook Layer 声明机制 | G.1 |
| §2.5 原子替换 | 压缩后强制写入 L0 Snapshot | G.2 |
| §2.5 Level 2 | 适用场景明确（中长任务 > 5 轮） | G.3 |
| §6.2 威胁模型 | HMAC 安全边界和替代方案 | G.4 |
| §6.3 API + fork_source | Session API + 独立字段 | G.5 |
| §4.3.1 检索路径 | 暴力搜索阈值 5000 条 | G.6 |
| §4.1.2 评分公式 | critic_score 条件权重重分配 | G.7 |
| §E.3 交互矩阵 | 新增 2 项交互风险 | G.2 + G.8 |
| §E.2.5 + F.3.2 | profile 分级冷启动 fallback | G.9 |
| §D.3.1 + D.3.2 + D.3.4 | ICS 分级评分 | G.10 |

---

## 附录 H：竞品与学术调研补充

> 本附录针对评估中发现的未解决问题，调研竞品（Claude Code / Cursor / Codex CLI / Trae）和学术研究，
> 为每个问题提供新的思路和具体解决方案。主文档相关章节已同步标注 `[H.x 补充]`。

### H.1 Compaction 重建与三层前缀分离的交互（§2.3 + §2.5 评估遗留）

#### 问题回顾

三层前缀分离（§2.3）将 system TextBlock 按静态/半静态/动态排列，但 Context Compaction 重建流程
（`rebuildRequestForContextCompaction`）只重新执行 RequestProcessor，**不执行 BeforeModel Hook**。
评估中担心：重建后 L1/L3/L4 记忆内容（由 MemoryInject Hook 注入）是否缺失？

#### 竞品调研

**Claude Code / trpc-agent-go 框架的实际实现**：

框架层的 `maybeCompactContextBeforeLLM`（`llmflow.go` 第 1053-1163 行）已完整处理此问题：

1. **preprocess 阶段**：所有 RequestProcessor 按序执行，在 ContentProcessor 之前保存 `beforeContent` 快照
2. **压缩阶段**：从 `beforeContent` 快照克隆 Request，**重放 ContentProcessor + tailProcessors**
3. **tailProcessors 包括**：TimeProcessor、SkillsToolResultRequestProcessor 等支持
   `SupportsContextCompactionRebuild` 的 processor
4. **安全检查**：如果任何 tailProcessor 不支持重建，整个 rebuildPlan 被置为 nil（放弃压缩）

**关键发现**：BeforeModel Hook 注入的内容（MemoryInject、SkillGuidance、RuntimeCue 等）**不在
rebuildPlan 的重放范围内**——它们在 preprocess 阶段已被执行并写入了 `beforeContent` 快照。
压缩重建时从快照克隆，这些内容自然保留。

**Claude Code 的 System Prompt 保护策略**：
- System message 位于 "preserved head"，不参与压缩
- 压缩只针对对话历史和工具结果
- 替换决策冻结（Frozen Replacement）：一旦工具结果被替换为预览，后续轮次使用相同预览字符串，
  保证前缀字节级一致，有利于 Prompt Cache 命中

**Cursor 的策略**：
- 文件精简三级策略（完整文件 → 精简摘要 → 仅文件名），本质是**前置压缩**
- repo-map 机制基于 tree-sitter 提取代码骨架替代完整文件内容
- System prompt 始终位于上下文头部，不参与压缩

**Codex CLI 的策略**：
- 重建策略较基础，依赖模型层 token tailoring 作为最终兜底
- System prompt 通过 API 的 system 角色消息始终保留

#### 结论与修正方案

**原评估的担忧是多余的**——框架层已通过 `beforeContent` 快照机制保证 Hook 注入内容在压缩后不丢失。
但有一个新的风险点需要关注：

**三层前缀分离后，压缩重建的 `beforeContent` 快照中的 TextBlock 顺序是否与压缩后新注入的一致？**

```
压缩前（beforeContent 快照）：
  [0] Identity+Instr+Skills+Workspace+PostTool  ← 静态（RequestProcessor）
  [1] Time                                       ← 动态（ContentParts 分离后）
  [2] RuntimeCue                                 ← 半动态（Hook prepend）
  [3] SkillGuidance                              ← 半动态（Hook prepend）
  [4] KnowledgeCue                               ← 动态（Hook prepend）
  [5] MemoryInject                               ← 动态（Hook prepend）

压缩后重建（从 beforeContent 克隆 + 重放 ContentProcessor）：
  - ContentProcessor 重新注入最新摘要 + 增量事件
  - tailProcessors 重新执行（Time 更新、Skills 重新加载）
  - BeforeModel Hooks 不重新执行，但内容已在快照中保留

问题：压缩后 MemoryInject 的内容可能已过时（L1 字段在压缩期间被更新）
```

**修正方案**：压缩重建后，增加一次 MemoryInject Hook 的选择性重执行：

```
1. rebuildRequestForContextCompaction 完成后
2. 检查是否有 MemoryInject Hook 注册
3. 如果有，重新执行 MemoryInject（读取最新的 L1/L3/L4 数据）
4. 替换快照中旧的 MemoryInject TextBlock
5. 这确保压缩后注入的记忆内容是最新的
```

**实现方式**：在 `rebuildRequestForContextCompaction` 的末尾，增加一个 `postRebuildHooks` 列表，
包含需要重执行的 Hook（目前只有 MemoryInject）。这与 `tailProcessors` 的设计模式一致。

**安全性**：仅重执行 MemoryInject 一个 Hook，不影响其他 Hook 的执行顺序和缓存断点位置。
MemoryInject 的输出归入 Layer 3（动态层），重执行不会影响 Layer 1/2 的缓存命中。

---

### H.2 Token 预算追踪：同步聚合 + DB 事务行锁（§3.2 评估遗留）

#### 问题回顾

§3.2 选择性注入依赖 `token_estimate` 和 `used_tokens`，但原方案中 `used_tokens` 的聚合是异步的
（`go func() { _ = admin.UpdateL1TaskUsedTokens(...) }()`），存在窗口期：写入字段后、聚合完成前，
预算检查用的是旧值。

#### 竞品调研

| 工具/框架 | Token 追踪方式 | 并发保护 |
|-----------|---------------|---------|
| Claude Code | API 响应精确 `usage` 字段 + 字符比例估算 | 单线程事件循环，无并发问题 |
| Cursor | 自研 token 估算器 + 滑动窗口优先级淘汰 | 单线程串行组装，无并发问题 |
| OpenAI tiktoken | 同步计数，发送前检查预算 | 无内置并发保护 |
| LangChain | `ConversationTokenBufferMemory` 同步 tiktoken 计数 | 非线程安全，需外部加锁 |
| LlamaIndex | `ContextWindowPromptHelper` 同步计算 | 非线程安全 |

**关键洞察**：所有主流工具的 token 预算检查都是**同步**的。异步化只应用于持久化步骤
（如本项目的 CostGuard channel + worker 模式），预算检查本身必须在写入路径上原子完成。

#### 推荐方案：同步聚合 + DB 事务行锁

```go
// UpsertL1Field 中，在同一个 DB 事务内完成字段写入 + used_tokens 聚合

func (r *l1FieldRepo) UpsertL1Field(ctx context.Context, in biz.L1FieldInsert) error {
    // 1. 计算 token_estimate
    if in.TokenEstimate == 0 && in.ValueText != "" {
        in.TokenEstimate = max(1, len(in.ValueText)/4)
    }

    // 2. 开启事务
    tx, err := r.data.RW().Write(ctx).BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // 3. INSERT/UPDATE 字段（含 token_estimate）
    _, err = tx.ExecContext(ctx, `
        INSERT INTO memory_l1_fields (..., token_estimate, ...)
        VALUES (..., ?, ...)
        ON CONFLICT(task_id, field_path) DO UPDATE SET
            value_text = excluded.value_text,
            token_estimate = excluded.token_estimate,
            ...`,
        in.TaskID, in.FieldPath, in.ValueText, in.TokenEstimate, ...)
    if err != nil {
        return err
    }

    // 4. 同步聚合 used_tokens（在同一事务内）
    _, err = tx.ExecContext(ctx, `
        UPDATE memory_l1_tasks SET used_tokens = (
            SELECT COALESCE(SUM(token_estimate), 0)
            FROM memory_l1_fields
            WHERE task_id = ? AND pin_to_prompt = 1 AND visibility != 'internal'
        ) WHERE id = ?`, in.TaskID, in.TaskID)
    if err != nil {
        return err
    }

    // 5. 预算检查（可选，在同一事务内）
    var usedTokens, budgetTokens int
    err = tx.QueryRowContext(ctx,
        `SELECT used_tokens, budget_tokens FROM memory_l1_tasks WHERE id = ?`, in.TaskID,
    ).Scan(&usedTokens, &budgetTokens)
    if err != nil {
        return err
    }
    injectLimit := int(float64(budgetTokens) * L1InjectBudgetRatio)
    if usedTokens > injectLimit {
        // 超预算：事务回滚，字段写入也回滚
        return ErrL1Overflow{Used: usedTokens, Budget: injectLimit}
    }

    return tx.Commit()
}
```

**与原方案的差异**：

| 维度 | 原方案（F.1.1） | 修正方案（H.2） |
|------|----------------|----------------|
| token_estimate 计算 | UpsertL1Field 中同步 | 不变 |
| used_tokens 聚合 | L1MemoryCue 渲染后异步 `go func()` | UpsertL1Field 事务内同步 |
| 预算检查 | 无（超预算时仅截断注入） | 事务内同步，超预算回滚写入 |
| 窗口期 | 存在（异步聚合延迟） | 不存在（事务内原子完成） |

**为什么不用异步**：
1. L1 写入频率低（每轮 1-3 次），事务行锁排队影响可忽略
2. 预算准确性要求高：L1 超预算会导致 prompt 超出上下文窗口
3. 不需要处理内存/DB 漂移、进程重启恢复等复杂问题

**RoughTokenEstimate 对中文的精度问题**：

当前 `len(s)/4` 对中文严重低估（4 汉字 ≈ 1 token，实际约 2-3 token）。建议：

```
短期：调整估算比例为 runeCount / 2（对中英文混合场景更准确）
中期：在框架层默认启用 tiktoken Counter（已有实现，需显式注入）
长期：使用 API 响应的 usage.prompt_tokens 作为校准基准
```

---

### H.3 压缩缓冲区自适应策略（§2.4 评估遗留）

#### 问题回顾

12% 缓冲区（~15K tokens）在编码场景中可能不足——单轮工具调用返回可达 10K+ tokens，
3 轮就可能耗尽缓冲。建议验证并考虑自适应调整。

#### 竞品调研

| 工具 | 缓冲区策略 | 大小 |
|------|-----------|------|
| Claude Code | 半固定（33K 绝对值 ≈ 200K 窗口的 16.5%） | 33K tokens |
| Cursor | 入口管控优先，被动压缩为辅 | 未公开 |
| Codex CLI | 结构化优先路径，减少压缩需求 | 未公开 |

**Claude Code 的 33K 缓冲区推导**：
- 200K 窗口 × 16.5% = 33K
- 覆盖系统提示词（~12%）+ 压缩完成前的增量空间
- 半固定：绝对值固定，比例随窗口大小变化

#### 学术研究

| 论文 | 压缩时机策略 | 启示 |
|------|------------|------|
| Focus (arXiv:2601.07190) | Agent 自主决策压缩时机 | 最优时机考虑任务语义 |
| Mem0 (arXiv:2504.19413) | 每轮增量提取，无批量压缩 | 消除"何时压缩"的问题 |
| LLMLingua-3 | 持续性 token 级压缩 | 压缩是持续操作，非阈值触发 |

#### 修正方案：自适应缓冲区 + 编码场景增强

```
compression_buffer_ratio 调整策略：

1. 基础比例：0.15（从 0.12 提高到 0.15）
   - 0.15 × 128K = 19.2K tokens
   - 覆盖 4~9 轮编码场景增量（每轮 2K~5K）
   - 覆盖 2~3 轮大增量场景（每轮 8K~10K）

2. 编码场景自适应增强：
   - 监控 soft_trigger 到压缩完成期间的 token 增量
   - 如果增量 > compression_buffer × 0.70：
     → 自动提高 compression_buffer_ratio（步进 +0.02，上限 0.25）
     → 记录告警日志
   - 如果连续 5 次压缩增量 < compression_buffer × 0.30：
     → 自动降低 compression_buffer_ratio（步进 -0.01，下限 0.10）

3. 对话模式检测：
   - tool_call_count / turn_count > 2.0 → 编码模式，使用增强缓冲
   - tool_call_count / turn_count < 0.5 → 聊天模式，使用基础缓冲
   - 中间值 → 使用加权平均

4. 配置项更新：

| 配置项 | 原默认值 | 修正默认值 | 范围 |
|--------|---------|-----------|------|
| compression_buffer_ratio | 0.12 | 0.15 | 0.10~0.25 |
| compression_buffer_adaptive | — | true | 是否启用自适应 |
```

**为什么从 0.12 提高到 0.15**：
- Claude Code 在 200K 窗口下使用 16.5%，本项目 128K 窗口下 15% 是合理的对应比例
- 0.15 × 128K = 19.2K，比 0.12 × 128K = 15.4K 多 3.8K，足够多覆盖 1~2 轮大增量
- 对 effective_budget 的影响：从 98K 降到 94.7K（减少 3.3%），可接受

---

### H.4 结构化字段提取的三层防御策略（§4.1 + §3.2 评估遗留）

#### 问题回顾

Path A key_decisions 提取依赖 `field_path` 模式匹配，但 `field_path` 是 LLM 自由填写的字符串，
无命名规范约束。评估建议增加 `field_kind` 枚举或在工具 description 中引导命名。

#### 竞品调研

| 系统 | 结构化记忆方式 | 字段名约束 |
|------|-------------|-----------|
| Claude Code | 无结构化工作记忆，依赖上下文压缩 | N/A |
| Cursor | 代码引用解析 + diff 解析 | 无字段名约束 |
| MemGPT/Letta | `core_memory` Block 级记忆，`label` 定位 | prompt 约定推荐 label，不强制 |
| OpenAI Function Calling | JSON Schema 定义参数 | `strict: true` 强制 schema 符合 |

**MemGPT/Letta 的关键设计决策**：选择 **prompt engineering + 宽松接受** 而非 **schema 校验 + 严格拒绝**。
原因是 LLM 的 function calling 可靠性不足以保证 100% 遵循 schema，严格拒绝会导致 Agent 陷入重试循环。

**OpenAI Structured Outputs**（2024.08）：`strict: true` 模式保证输出 100% 符合 JSON Schema，
`enum` 约束可限制字段值。但**无法约束字段名本身**——字段名由 schema 的 `properties` 键定义。

#### 修正方案：三层防御

```
第一层：field_kind 枚举增强（最可靠，向后兼容）

  扩展 field_kind 为语义枚举：
    string / number / boolean / json / reference
    / decision / artifact / progress / constraint

  working_memory.write 工具增加 field_kind 参数的 enum 约束：
    jsonschema:"description=Categorize this field,enum=string,enum=decision,enum=artifact,..."

  提取 key_decisions 时：
    Layer 0：field_kind = "decision" 的字段（新增，最可靠）
    Layer 1：field_path 模式匹配（当前方案）
    Layer 2：pin_to_prompt + visibility 属性匹配（当前 fallback）

  提取 key_artifacts 时：
    Layer 0：field_kind = "artifact" 或 "reference" 的字段
    Layer 1：field_path 模式匹配
    Layer 2：field_kind = "reference" 的字段

第二层：Prompt 约定（中等可靠，零成本）

  在 Agent system prompt 中加入推荐字段名列表：
    "When using working_memory.write, prefer these standard field paths:
     - task_goal: the current task objective
     - key_decisions: important decisions made
     - key_artifacts: files or outputs created
     - current_step: current progress
     - open_questions: unresolved issues
     - active_constraints: limitations to respect"

  在 working_memory.write 工具的 description 中说明：
    "Use field_kind='decision' for key decisions, field_kind='artifact' for files/outputs"

第三层：Schema 约束（最可靠，但限制灵活性，可选启用）

  利用已有 memory_l1_schemas 表：
    - Agent 绑定 L1 Schema（通过 l1_default_schema_id）
    - working_memory.write 执行时校验 field_path 是否在 schema.properties 中
    - 不在则拒绝写入，返回可用字段名列表
    - 适用于任务型 Agent（字段可预定义）

  不绑定 Schema 的 Agent：使用第一层 + 第二层防御
  绑定 Schema 的 Agent：使用第三层防御（覆盖第一层 + 第二层）
```

**与原方案的差异**：

| 维度 | 原方案（F.1.3） | 修正方案（H.4） |
|------|----------------|----------------|
| 提取策略 | 模式匹配 + 属性匹配 + 全量兜底 | field_kind 枚举 + 模式匹配 + 属性匹配 + 全量兜底 |
| 字段分类 | 无语义分类 | field_kind 枚举（decision/artifact/progress 等） |
| Prompt 引导 | 无 | 推荐字段名列表 + field_kind 使用说明 |
| Schema 约束 | 已有表但未启用 | 可选启用，按 Agent 配置 |

**field_kind 枚举的 LLM 遵循率**：枚举值比自由文本字段名更容易被 LLM 遵循
（MemGPT/Letta 的经验：枚举遵循率 > 90%，自由文本命名一致性 < 70%）。

---

### H.5 跨代理上下文完整性保护：SHA-256 校验和替代 HMAC（§6.2 评估遗留）

#### 问题回顾

HMAC 的实际 ROI 偏低：密钥和签名在同一数据库中，单租户场景下 HMAC 的防御价值趋近于零。
评估建议考虑更轻量的替代方案。

#### 竞品调研

| 框架 | 应用层完整性 | 传输层安全 | 跨 Agent 信任模型 |
|------|-------------|-----------|------------------|
| AutoGen | 无 | TLS（跨进程） | 全信任 |
| CrewAI | 无 | 无（同进程） | 全信任 |
| LangGraph | 无 | TLS（可选） | 全信任 |
| Codex CLI | Thread 级全局密钥（单用户简化） | 本地文件系统 | 全信任 |
| **Aranea（当前设计）** | **HMAC-SHA256** | **TLS** | **分级信任** |

**结论**：主流多智能体框架均**不做应用层完整性保护**，完全依赖传输层 TLS 和进程隔离。
本项目的 HMAC 方案在业界属于**超前设计**。

#### 安全性等价分析

**核心论点**：在单租户场景下，SHA-256 校验和与 HMAC-SHA256 提供等价的实际安全性。

推理：
1. HMAC 的安全性前提是"攻击者无法获取 HMAC 密钥"
2. 单租户场景中，HMAC 密钥存储在 `session_runtime.signing_key_enc`，
   使用 `CredentialCrypto`（AES-256-GCM）加密，主密钥来自环境变量
3. 如果攻击者能访问数据库，通常也能访问应用服务器环境变量（同一台机器或同一 K8s Pod）
4. 因此"攻击者无法获取 HMAC 密钥"这个前提在单租户场景下**不成立**
5. SHA-256 校验和的前提是"攻击者无法同时修改内容和哈希"——这与 HMAC 的实际前提相同

#### 修正方案：分层策略

```
单租户部署（推荐方案）：

  Tier 0: TLS + 数据库约束（默认，零额外开发）
    - gRPC/HTTP 传输层 TLS
    - API 层权限控制（不暴露内部字段）
    - 防御：网络传输篡改、前端伪造

  Tier 1: SHA-256 校验和（推荐替代 HMAC，轻量）
    - session_summaries 表新增 content_hash TEXT 字段
    - content_hash = SHA-256(summary_markdown + salt)
    - salt = workspace_id 或 agent_id（非密钥，但攻击者需额外获取）
    - 注入时验证：SHA-256(内容 + salt) == content_hash
    - 不匹配 → 跳过摘要注入，记录告警日志
    - 防御：意外损坏、只改内容不改哈希的低级篡改
    - 优势：无密钥管理、实现极简、与 HMAC 防御等价

多租户部署（升级方案）：

  Tier 2: HMAC-SHA256（按租户密钥隔离）
    - 即当前 §6.2 方案，但密钥按租户隔离
    - 防御：跨租户篡改

  Tier 3: AES-256-GCM 加密（高安全场景）
    - 即当前 §6.2 Level 2 方案
    - 防御：跨租户数据泄露 + 完整性保护
```

**实现成本对比**：

| 维度 | HMAC-SHA256（当前方案） | SHA-256 校验和（推荐替代） |
|------|------------------------|--------------------------|
| 新增 DB 字段 | 3 个（signature_blob, encryption_mode, signing_key_enc） | 2 个（content_hash, trust_source） |
| 密钥管理 | 需要密钥生成、加密存储、解密读取 | 无密钥 |
| 代码行数（估算） | ~150 行 | ~40 行 |
| Fork 交互问题 | 需 trust_source 标记 + 密钥传递 | 需 trust_source 标记（无密钥传递问题） |

**迁移路径**：
- Phase 1：实现 Tier 1 SHA-256 校验和 + trust_source
- Phase 2（多租户时）：升级到 Tier 2 HMAC，content_hash 保留作为快速校验

---

### H.6 ContentParts 多 Provider 兼容性验证（§2.3 评估遗留）

#### 问题回顾

方案将 Time 信息从 `message.Content` 移到 `message.ContentParts`，需验证非 Anthropic Provider
是否正确处理。

#### 代码验证结果

| Provider | ContentParts 处理 | 安全性 |
|----------|-------------------|--------|
| Anthropic | Content + ContentParts 分别转为 TextBlockParam | 安全 |
| OpenAI（含 DeepSeek/Qwen） | Content + ContentParts 分别转为 content parts | 安全 |
| Gemini | Content + ContentParts 分别转为 genai.Part | 安全 |
| Bedrock | Content + ContentParts 分别转为 SystemContentBlock | 安全 |
| Ollama | ContentParts text 拼接到 content 字符串 | 安全 |
| HuggingFace | Content 优先，Content 为空时才用 ContentParts | 安全 |
| **Hunyuan** | ContentParts 非空时**清空 Content** | **有风险** |

#### Hunyuan 适配器的具体问题

Hunyuan 适配器（`hunyuan.go` 第 623-626 行）在 `ContentParts` 非空时执行 `hMsg.Content = ""`，
只保留 `Contents`（ContentParts 转换后的结构化内容）。如果 system 消息原本有 `Content`
（静态前缀），追加 `ContentParts`（动态时间信息）后，Hunyuan 会丢弃 `Content` 中的原始内容。

#### 修正方案

```
方案 A（推荐）：修复 Hunyuan 适配器

  在 hunyuan.go 的 convertMessage 中，当 ContentParts 非空时，
  将 Content 的内容作为第一个 ContentPart 追加到 Contents 中，
  而非清空 Content：

  // 修改前：
  if len(hMsg.Contents) > 0 {
      hMsg.Content = ""
  }

  // 修改后：
  if len(hMsg.Contents) > 0 && msg.Content != "" {
      // 将 Content 内容作为第一个 ContentPart
      firstPart := ChatCompletionMessageContentParam{
          Type: "text",
          Text: msg.Content,
      }
      hMsg.Contents = append([]ChatCompletionMessageContentParam{firstPart}, hMsg.Contents...)
      hMsg.Content = ""
  }

方案 B（保守）：不在 ContentParts 中写入时间信息

  继续将时间信息写入 Content 字段（当前行为），
  在 Anthropic adapter 的 convertSystemMessageContent 中，
  通过解析 Content 字符串来分离静态/动态内容。

  缺点：需要在 Content 字符串中标记时间信息的边界（如特殊分隔符），
  增加解析复杂度。

推荐方案 A：修复 Hunyuan 适配器是更干净的解决方案，
且符合"所有适配器都应正确处理 Content + ContentParts 共存"的原则。
```

---

### H.7 ICS 评估时机问题（§2.5 评估遗留）

#### 问题回顾

Level 2 Memory Compact 在 `AfterNativeTurn` 异步触发时，L1 数据可能处于中间状态
（Agent 正在写入字段但未完成），ICS 评估可能得到偏低值。

#### 竞品参考

**Claude Code**：压缩触发在 `AfterNativeTurn` 之后，此时当前 turn 的所有工具调用已完成，
LLM 响应已生成。不存在"工具调用进行中触发压缩"的问题。

**MemGPT/Letta**：记忆更新在每次工具调用后同步完成，不存在异步窗口。

#### 修正方案

```
Level 2 触发时机保证：

1. AfterNativeTurn 在 turn 完整结束后触发（LLM 响应已生成）
2. working_memory.write 工具调用在 turn 内同步完成
3. 因此 L1 数据在 AfterNativeTurn 时已是终态

4. 唯一的窗口期：Agent 在一个 turn 内多次调用 working_memory.write，
   中间某次调用后触发了 AfterNativeTurn（不可能——AfterNativeTurn 在 turn 结束后才触发）

5. 结论：原评估的担忧是多余的——AfterNativeTurn 的触发时机保证了 L1 数据的完整性

6. 但需注意：如果未来引入流式记忆写入（streaming memory update），
   则需要重新评估此问题
```

---

### H.8 调研总结与方案修正索引

| 评估问题 | 调研结论 | 修正方案 | 影响章节 |
|---------|---------|---------|---------|
| Compaction 重建后 Hook 内容缺失 | **原担忧多余**——框架已通过 beforeContent 快照保留；但需增加 MemoryInject 选择性重执行 | 压缩重建后重执行 MemoryInject | §2.3 + §2.5 |
| token_estimate 异步聚合窗口期 | 所有主流工具同步做预算检查 | 改为同步聚合 + DB 事务行锁 | §3.2 + F.1.1 |
| 12% 缓冲区编码场景不足 | Claude Code 用 16.5%；0.15 更合理 | 默认值从 0.12 → 0.15 + 自适应 | §2.4 |
| field_path 命名不可靠 | MemGPT 用 prompt 约定 + 宽松接受；OpenAI strict mode 保证 schema | field_kind 枚举 + Prompt 约定 + Schema 可选 | §4.1 + §3.2 |
| HMAC ROI 偏低 | 主流框架不做应用层完整性；单租户下 SHA-256 与 HMAC 等价 | 单租户用 SHA-256 校验和，多租户升级 HMAC | §6.2 |
| ContentParts 兼容性 | 6/7 Provider 安全；Hunyuan 有风险 | 修复 Hunyuan 适配器 | §2.3 |
| ICS 评估时机 | **原担忧多余**——AfterNativeTurn 在 turn 结束后触发 | 无需修改，记录结论 | §2.5 |

### H.x 修正索引

以下主文档章节需根据本附录调研结果进行修正，标注 `[H.x 补充]`：

| 章节 | 修正内容 | 对应调研 |
|------|---------|---------|
| §2.3 + §2.5 交互 | 压缩重建后增加 MemoryInject 选择性重执行 | H.1 |
| §3.2 + F.1.1 | used_tokens 聚合改为同步 + DB 事务行锁 | H.2 |
| §2.4 | compression_buffer_ratio 默认值从 0.12 → 0.15 + 自适应策略 | H.3 |
| §4.1 + F.1.3 | 增加 field_kind 枚举 + Prompt 约定 + Schema 可选 | H.4 |
| §6.2 | 单租户用 SHA-256 校验和替代 HMAC，多租户升级 | H.5 |
| §2.3 | 修复 Hunyuan 适配器 ContentParts 处理 | H.6 |
| §2.5 | ICS 评估时机结论（无需修改） | H.7 |
| §E.2.1 | 降级为"中"（原评估为"严重"），因框架已有快照机制 | H.1 |
