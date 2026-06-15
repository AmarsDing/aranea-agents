# Agent 工具执行性能瓶颈——根因分析与架构重构评审报告

> **日期**：2026-06-15
> **类型**：review-performance
> **范围**：trpc-agent-go 工具执行全链路 + 前端消息渲染
> **状态**：已评审，P0 已实施

---

## 一、问题陈述

用户下达"让 agent 每个工具都使用一遍"的命令后，agent 执行速度特别慢。对比 Trae、Cursor 等产品，体验差距明显。本报告从代码逻辑层面深入分析根因，调研业界优秀实践，提出架构级重构方案并评审其可行性。

---

## 二、根因分析

### 2.1 核心矛盾

当前架构的根本矛盾是**执行管线与持久化管线耦合**：

```
当前：LLM调用 → 等持久化(5s) → 工具执行 → 等持久化(5s) → 下一轮
目标：LLM调用 → 工具执行 → 下一轮
                └→ 持久化（异步追赶）
```

### 2.2 瓶颈全景

| # | 瓶颈 | 位置 | 类型 | 严重度 | 单次延迟 |
|---|------|------|------|--------|---------|
| B1 | `emitStartEventAndWait` 同步等待持久化 | `llmflow.go:470-489` | 设计缺陷 | 致命 | 0-5s |
| B2 | `handleFunctionCallsAndSendEvent` 同步等待持久化 | `functioncall.go:302-312` | 设计缺陷 | 致命 | 0-5s |
| B3 | `maybeCompactContextBeforeLLM` 同步摘要+深拷贝 | `llmflow.go:1053-1126` | 设计缺陷 | 高 | 1-10s |
| B4 | `maybeSyncSummaryIntraRun` 同步摘要 | `llmflow.go:446-468` | 设计缺陷 | 高 | 1-5s |
| B5 | 工具默认串行执行 | `functioncall.go:345` | 设计缺陷 | 高 | N×工具耗时 |
| B6 | 无流式工具执行 | 不存在 | 缺失能力 | 高 | LLM流式时间 |
| B7 | 无工具结果截断/摘要 | 不存在 | 缺失能力 | 高 | 上下文膨胀 |
| B8 | `buildSanitizedNameCache` 每次重建 | `functioncall.go:1479-1493` | 实现缺陷 | 中 | O(n) |
| B9 | 并行模式 `Session.Clone()` 深拷贝 | `functioncall.go:959-964` | 实现缺陷 | 中 | O(history) |
| B10 | 串行模式单工具失败终止整批 | `functioncall.go:354-356` | 设计缺陷 | 中 | 后续工具跳过 |
| B11 | 无全局工具执行超时 | 依赖 context deadline | 缺失能力 | 中 | 可能无限阻塞 |
| B12 | 前端每次事件拷贝全量消息数组 | `streamHandlers.ts` 18 处 | 实现缺陷 | 中 | O(n) |
| B13 | 前端 `toolEventFromMessage` 重复 JSON.parse | `envelopeToolCall.ts:224` | 实现缺陷 | 中 | 5-10次/消息 |
| B14 | EventBus `env.Clone()` 每订阅者深拷贝 | `bus.go:85` | 实现缺陷 | 中 | O(subscribers) |
| B15 | 前端 `groupMessagesByTurn` + `useConversationTimeline` 双重重算 | 两个 computed | 设计缺陷 | 中低 | 2×O(n log n) |

### 2.3 量化影响

5 个工具调用的 ReAct 循环（当前）：

```
5轮 × (emitStartEventAndWait 5s + 工具执行 1s + handleFunctionCallsAndSendEvent 5s) = 55s
其中纯等待 = 50s，实际执行 = 5s
等待占比 = 91%
```

---

## 三、业界对标

### 3.1 Claude Code

| 技术 | 实现方式 | 借鉴价值 |
|------|----------|----------|
| **StreamingToolExecutor** | LLM 流式返回 tool_use 时立即执行，不等响应结束 | **极高** |
| **工具安全分类** | `partitionToolCalls()` 将工具分为 concurrent-safe 和 exclusive | **极高** |
| **5 层上下文压缩** | Tool Result Budget → History Snip → Microcompact → Context Collapse → Autocompact | **高** |
| **Tool Result Budget** | 每个工具结果限制最大 token 数 | **高** |
| **Append-Only Session** | 事件只追加不修改，支持回放 | **中** |
| **7 层错误恢复** | 413/Reactive Compact/Max Output Escalation 等 | **中** |

### 3.2 Cursor

| 技术 | 实现方式 | 借鉴价值 |
|------|----------|----------|
| **多 Agent 并行** | 最多 8 个 Agent 并行，git worktree 隔离 | **高** |
| **Background Agent** | Agent 在后台运行，不阻塞 IDE | **高** |
| **Subagent 系统** | 独立子 Agent 处理子任务 | **中** |

### 3.3 OpenAI

| 技术 | 实现方式 | 借鉴价值 |
|------|----------|----------|
| **parallel_tool_calls** | API 参数控制是否并行，默认 true | **高** |
| **Truncation Strategy** | 工具结果自动截断策略 | **高** |
| **Structured Outputs** | `strict: true` 保证工具参数格式 | **中** |

---

## 四、方案评审

### 方案 A：双轨管线架构（热轨/冷轨分离）

**目标**：执行管线零 I/O、零等待，持久化管线异步追赶。

#### A1：移除同步等待机制

| 维度 | 评审结论 |
|------|----------|
| **可行性** | **高** — 仅需移除 `RequiresCompletion` + `AddNoticeChannelAndWait`，改动约 5 个文件 |
| **风险** | **低** — 进程崩溃时可能丢失尾部未持久化事件，可接受 |
| **依赖** | 无外部依赖，可独立实施 |
| **预期效果** | 消除 91% 的纯等待延迟，5 工具 ReAct 从 55s → 5s |
| **评审意见** | **通过，最高优先级实施** |

**关键改动**：
- `llmflow.go:470-489`：`emitStartEventAndWait` → `emitStartEvent`（移除等待）
- `functioncall.go:302-312`：移除 `RequiresCompletion = true` + `AddNoticeChannelAndWait`
- `llmflow.go:358`：移除 tool call 事件的 `RequiresCompletion`
- `output.go:173`：移除 StateDelta 事件的 `RequiresCompletion`
- `graph_agent.go:338-347`：Graph barrier 改为异步确认
- `executor.go:2793`：Graph executor barrier 改为异步确认

**风险缓解**：
- Runner 事件循环仍保证持久化顺序（先发先持久化）
- 可增加定期 flush 机制（每 100ms 或每 N 个事件）减少丢失窗口
- Graph barrier 场景可保留可选的同步等待（仅关键路径）

#### A2：Runner 事件循环异步持久化

| 维度 | 评审结论 |
|------|----------|
| **可行性** | **中高** — 需要重构 `processSingleAgentEvent`，引入持久化队列 |
| **风险** | **中** — 事件顺序保证需要仔细设计 |
| **依赖** | 依赖 A1（移除同步等待后才有意义） |
| **预期效果** | 事件循环吞吐量提升 5-10 倍 |
| **评审意见** | **通过，A1 之后实施** |

**关键改动**：
- `runner.go:1026-1103`：`processSingleAgentEvent` 改为先发输出、再入队持久化
- 新增 `persistQueue`：带微批（5ms 窗口，最多 32 个事件）的持久化队列
- 新增 `runPersistWorker` goroutine：独立消费持久化队列

#### A3：Session 批量写入

| 维度 | 评审结论 |
|------|----------|
| **可行性** | **中** — 需要重构 `addEvent` 为 `addEventBatch` |
| **风险** | **中** — 全局写锁持有时间缩短，但批量事务可能更大 |
| **依赖** | 依赖 A2（异步持久化后才有批量写入的基础） |
| **预期效果** | DB 写入次数减少 5-10 倍 |
| **评审意见** | **通过，A2 之后实施** |

#### A4：上下文压缩异步化

| 维度 | 评审结论 |
|------|----------|
| **可行性** | **中高** — `maybeCompactContextBeforeLLM` 改为异步触发 |
| **风险** | **低** — 使用旧摘要可能质量略降，但可接受 |
| **依赖** | 无强依赖，可独立实施 |
| **预期效果** | 消除压缩阻塞，减少 2-10s 延迟 |
| **评审意见** | **通过，可与 A1 并行实施** |

#### A5：EventBus COW 引用共享

| 维度 | 评审结论 |
|------|----------|
| **可行性** | **中** — 需要引入引用计数或 COW 机制 |
| **风险** | **低** — 大多数订阅者只读，共享安全 |
| **依赖** | 无强依赖 |
| **预期效果** | 消除 N 次 Envelope 深拷贝 |
| **评审意见** | **通过，低优先级** |

---

### 方案 B：StreamingToolExecutor（流式工具执行）

**目标**：LLM 流式返回 tool_calls 时，每解析到一个就立即执行，不等 LLM 响应结束。

| 维度 | 评审结论 |
|------|----------|
| **可行性** | **中高，但有核心难点** |
| **核心难点** | tool_call 参数的增量累积和完整性判断 |
| **风险** | **中高** — 需要修改流式响应处理的核心逻辑 |
| **依赖** | 依赖方案 C（工具安全分类），否则无法安全地即时执行 |
| **预期效果** | 3 工具调用场景延迟减少 30-50% |

#### 可行性验证结果

1. **OpenAI 流式 API**：tool_calls 在流式中逐步构建，但框架默认 `showToolCallDelta=false`，只在最终 response 暴露完整 tool_calls。**需要开启 `showToolCallDelta`** 才能在 chunk 级别获取 tool_call delta。

2. **Anthropic 流式 API**：`content_block_start` 机制在 tool_use 开始时就能确定 ID 和函数名，参数逐步传入。**天然适配流式工具执行**。

3. **参数完整性判断**（核心难点）：
   - **保守策略**：等流结束（等同于当前行为，无收益）
   - **中等策略**：等 `finish_reason="tool_calls"` 的 chunk 到达（OpenAI 在所有 tool_call 参数传完后才发 finish）
   - **激进策略**：收到下一个 tool_call 的 start chunk 时，认为前一个 tool_call 参数已完整

4. **event channel 容量**：当前 256 可能不足，建议增大到 512 或为工具执行事件使用独立 channel。

5. **并行执行复用**：`executeToolCallsInParallel` 的核心逻辑可复用 70%+。

#### 评审意见

**有条件通过**。建议分两步实施：

- **Step 1**（低风险）：在流结束后、postprocess 之前，立即并行执行所有 tool_calls（不等持久化）。这不需要修改流式处理逻辑，只需修改 `ProcessResponse` 的执行策略。
- **Step 2**（高风险）：实现真正的 StreamingToolExecutor，在流式中即时执行。需要开启 `showToolCallDelta`，修改 `processStreamingResponses`，引入 tool_call 累积器。

---

### 方案 C：工具安全分类器（Tool Classifier）

**目标**：将工具分为 ConcurrentSafe（只读，可并行）和 Exclusive（状态修改，需串行），作为默认开启并行执行的前提。

| 维度 | 评审结论 |
|------|----------|
| **可行性** | **高** — 业务层已有 `SupportsConcurrency` 字段 |
| **风险** | **低** — 新增分类不影响现有行为 |
| **依赖** | 无强依赖 |
| **预期效果** | 安全地默认开启并行执行，3 只读工具延迟从 3s → 1s |

#### 关键发现

业务层 `internal/biz/tool/tool.go` 中的 `Tool` 结构体**已有** `SupportsConcurrency` 字段，但该信息在注册到 trpc-agent-go 框架时**丢失了**。只需在桥接层传递此信息即可。

#### 内置工具分类

| 分类 | 工具 | 数量 |
|------|------|------|
| **ConcurrentSafe** | `skill_poll_session`、`skill_list_docs` | 2 |
| **Exclusive** | 其余所有内置工具 | 21 |

**评审注意**：当前内置工具中 ConcurrentSafe 仅 2 个，分类器的直接收益有限。但分类器是 StreamingToolExecutor 和默认并行执行的前提条件，且 MCP/OpenAPI/FunctionTool 等动态工具可能包含大量只读工具。

#### 评审意见

**通过**。建议实施路径：
1. `Declaration` 新增 `Safety` 字段（`json:"-"` 不暴露给 LLM）
2. 桥接层传递 `SupportsConcurrency` → `Safety`
3. `handleFunctionCalls` 改为按 Safety 分组执行（ConcurrentSafe 并行，Exclusive 串行）
4. `WithEnableParallelTools` 升级为三态：Off / All / Classified

---

### 方案 D：工具结果预算制（Tool Result Budget）

**目标**：每个工具结果限制最大 token 数，超出自动截断/摘要。

| 维度 | 评审结论 |
|------|----------|
| **可行性** | **高** — 在 `buildDefaultToolMessage` 中增加截断逻辑 |
| **风险** | **低** — 截断可能丢失信息，但比上下文溢出好 |
| **依赖** | 无强依赖 |
| **预期效果** | 减少 50-70% 上下文消耗，降低压缩触发频率 |

#### 评审意见

**通过，高优先级**。这是投入产出比最高的方案之一：
- 实现简单（约 50 行代码）
- 效果显著（直接减少上下文消耗）
- 无破坏性（可配置截断阈值）

建议配置：
- `read_file`：10K tokens
- `search`：5K tokens
- `shell`：20K tokens
- 默认：10K tokens
- 截断模式：tail（保留开头，丢弃尾部）

---

### 方案 E：确定性工具缓存

**目标**：相同工具名 + 相同参数 → 直接返回缓存结果。

| 维度 | 评审结论 |
|------|----------|
| **可行性** | **高** — `toolcache` 包已有局部实现 |
| **风险** | **低** — 仅缓存 ConcurrentSafe 工具 |
| **依赖** | 依赖方案 C（需要 Safety 分类判断哪些工具可缓存） |
| **预期效果** | 重复工具调用 <1ms |

#### 评审意见

**通过，中优先级**。ReAct 循环中 LLM 经常重复读取同一文件，缓存可消除这些重复执行。但需注意：
- 缓存生命周期与 Invocation 绑定
- Exclusive 工具执行后需失效相关缓存
- 缓存 key 应包含工具名 + 参数 hash

---

### 方案 F：前端增量状态架构

**目标**：MessageStore 从数组+全量替换改为 Map+增量 Patch。

| 维度 | 评审结论 |
|------|----------|
| **可行性** | **中** — 需要重构 messageStore 和 streamHandlers |
| **风险** | **中** — 涉及前端核心数据结构变更 |
| **依赖** | 无强依赖 |
| **预期效果** | 消除 18 处数组拷贝，减少 GC 压力 |

#### 评审意见

**有条件通过**。建议分步实施：
1. **先做低风险优化**：`toolEventFromMessage` 缓存、合并 `groupMessagesByTurn` + `useConversationTimeline`
2. **再做高风险重构**：MessageStore Map + 增量 Patch

---

### 方案 G：实现缺陷快速修复

| 修复 | 风险 | 评审意见 |
|------|------|----------|
| `buildSanitizedNameCache` 缓存到 Processor | 低 | **通过** |
| 串行模式单工具失败不终止整批 | 低 | **通过** |
| 全局工具执行超时（默认 60s） | 低 | **通过** |
| `enableParallelTools` 默认改为 true | 中 | **有条件通过** — 需先实施方案 C（安全分类） |
| Session.Clone() 改为 COW | 中 | **通过，低优先级** |
| 并行 StateDelta 冲突记录 warning | 低 | **通过** |

---

## 五、方案依赖关系与实施路线

```
Phase 1（1-2 天，低风险，立即实施）
├── G: 实现缺陷快速修复（缓存/超时/失败不终止）
├── A1: 移除同步等待机制
└── A4: 上下文压缩异步化

Phase 2（1-2 周，中风险）
├── C: 工具安全分类器
├── D: 工具结果预算制
├── A2: Runner 事件循环异步持久化
└── G: enableParallelTools 默认开启（依赖 C）

Phase 3（2-4 周，中高风险）
├── B-Step1: 流结束后立即并行执行（低风险版 StreamingToolExecutor）
├── E: 确定性工具缓存（依赖 C）
├── A3: Session 批量写入（依赖 A2）
└── F-Step1: 前端低风险优化

Phase 4（4-8 周，高风险）
├── B-Step2: 真正的 StreamingToolExecutor
├── A5: EventBus COW
├── F-Step2: 前端增量状态架构
└── Session.Clone() COW
```

---

## 六、预期效果汇总

| 场景 | 当前 | Phase 1 后 | Phase 2 后 | Phase 3 后 | Phase 4 后 |
|------|------|-----------|-----------|-----------|-----------|
| 5 工具 ReAct 循环 | 30-60s | 5-10s | 3-7s | 2-5s | 1.5-3s |
| 3 只读工具并行 | 6-9s | 3-5s | 1-2s | 0.8-1.5s | 0.5-1s |
| 单轮对话（无工具） | 2-5s | 1.5-4s | 1-3s | 1-3s | 0.5-2s |
| 上下文压缩触发频率 | 每 5-8 轮 | 每 5-8 轮 | 每 15-20 轮 | 每 15-20 轮 | 每 20-30 轮 |
| 重复工具调用 | 1s/次 | 1s/次 | 1s/次 | <1ms/次 | <1ms/次 |
| 流式文本渲染延迟 | 100-300ms | 100-300ms | 80-200ms | 50-150ms | 30-80ms |

---

## 七、风险登记

| # | 风险 | 影响 | 概率 | 缓解措施 |
|---|------|------|------|----------|
| R1 | 移除同步等待后进程崩溃丢失事件 | 数据丢失 | 低 | 定期 flush + WAL 重放 |
| R2 | StreamingToolExecutor 参数完整性误判 | 工具执行失败 | 中 | 保守策略 + 重试 |
| R3 | 工具安全分类错误导致并行冲突 | 数据不一致 | 低 | 默认 Exclusive + 渐进开放 |
| R4 | 工具结果截断丢失关键信息 | LLM 判断错误 | 中 | 可配置阈值 + 摘要模式 |
| R5 | 前端增量状态重构引入渲染 bug | UI 异常 | 中 | 分步实施 + 充分测试 |
| R6 | 异步持久化导致事件顺序错乱 | 状态不一致 | 低 | 持久化队列保持 FIFO |

---

## 八、决策建议

### 必须实施（P0）— ✅ 已实施

1. **A1：移除同步等待机制** — ✅ 已实施。移除 `RequiresCompletion` + `AddNoticeChannelAndWait`，5 个文件
2. **G：实现缺陷快速修复** — ✅ 已实施。缓存 `sanitizedNameCache`、串行失败不终止、全局工具超时 60s
3. **D：工具结果预算制** — ✅ 已实施。新增 `ResultBudget` 类型 + `truncateResult` JSON 安全截断 + `WithResultBudget` 选项

### 强烈建议（P1）

4. **C：工具安全分类器** — 默认并行执行的前提
5. **A4：上下文压缩异步化** — 消除压缩阻塞
6. **A2：Runner 事件循环异步持久化** — 事件循环吞吐量提升

### 建议实施（P2）

7. **B-Step1：流结束后立即并行执行** — StreamingToolExecutor 的低风险版本
8. **E：确定性工具缓存** — 消除重复执行
9. **F-Step1：前端低风险优化** — toolEventFromMessage 缓存等

### 远期规划（P3）

10. **B-Step2：真正的 StreamingToolExecutor** — 核心难点需充分验证
11. **A3/A5：批量写入 + COW** — 进一步优化
12. **F-Step2：前端增量状态架构** — 高风险重构

---

## 九、附录

### A. 关键代码位置索引

| 组件 | 文件 | 关键行号 |
|------|------|----------|
| emitStartEventAndWait | `pkg/trpc-agent-go/internal/flow/llmflow/llmflow.go` | 470-489 |
| handleFunctionCallsAndSendEvent | `pkg/trpc-agent-go/internal/flow/processor/functioncall.go` | 268-325 |
| maybeCompactContextBeforeLLM | `pkg/trpc-agent-go/internal/flow/llmflow/llmflow.go` | 1053-1126 |
| maybeSyncSummaryIntraRun | `pkg/trpc-agent-go/internal/flow/llmflow/llmflow.go` | 446-468 |
| handleFunctionCalls（并行/串行分支） | `pkg/trpc-agent-go/internal/flow/processor/functioncall.go` | 335-381 |
| executeToolCallsInParallel | `pkg/trpc-agent-go/internal/flow/processor/functioncall.go` | 511-568 |
| buildSanitizedNameCache | `pkg/trpc-agent-go/internal/flow/processor/functioncall.go` | 1479-1493 |
| Session.Clone() | `pkg/trpc-agent-go/internal/flow/processor/functioncall.go` | 959-964 |
| processStreamingResponses | `pkg/trpc-agent-go/internal/flow/llmflow/llmflow.go` | 616-767 |
| handleEventPersistence | `pkg/trpc-agent-go/runner/runner.go` | 1326-1404 |
| addEvent（DB 写入） | `pkg/trpc-agent-go/session/sqlite/service_helper.go` | 285-399 |
| EventBus.Publish | `internal/event/bus.go` | 57-82 |
| 前端 streamHandlers | `web/src/features/chat/streamHandlers.ts` | 全文 |
| 前端 toolEventFromMessage | `web/src/features/chat/envelopeToolCall.ts` | 224-238 |
| 前端 groupMessagesByTurn | `web/src/features/chat/groupMessagesByTurn.ts` | 87-168 |
| 业务层 Tool.SupportsConcurrency | `internal/biz/tool/tool.go` | 已有字段 |
| 桥接层 trpc_build | `internal/agent/trpc_build.go` | 166-218 |

### B. 业界参考来源

- Claude Code 架构分析：[Dive into Claude Code](https://zhiqiangshen.com/projects/Claude_Code_Report/Claude_Code_Report.pdf)
- Claude Code Agent Loop 深度分析：[blog.vincentqiao.com](https://blog.vincentqiao.com/en/posts/claude-code-agent-loop/)
- Claude Code 源码泄露分析：[bits-bytes-nn.github.io](https://bits-bytes-nn.github.io/insights/agentic-ai/2026/03/31/claude-code-architecture-analysis.html)
- OpenAI Parallel Tool Calling 评估：[callsphere.ai](https://www.callsphere.ai/blog/parallel-tool-calling-openai-agents-sdk-eval)
- Cursor 2.0 Changelog：[cursor.com/changelog/2-0](https://cursor.com/changelog/2-0)
- OpenAI Function Calling 指南：[platform.openai.com/docs](https://platform.openai.com/docs/guides/function-calling)

---

## 十、P0 实施记录（2026-06-15）

### 变更文件清单

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `pkg/trpc-agent-go/internal/flow/llmflow/llmflow.go` | 修改 | P0-A1: 移除 `emitStartEventAndWait` 中的 `RequiresCompletion` 和 `AddNoticeChannelAndWait` |
| `pkg/trpc-agent-go/internal/flow/processor/functioncall.go` | 修改 | P0-A1: 移除 `handleFunctionCallsAndSendEvent` 中的同步等待；P0-G: 缓存 `sanitizedNameCache`、串行失败不终止、全局工具超时；P0-D: `ResultBudget` 集成、`truncateResult` JSON 安全截断 |
| `pkg/trpc-agent-go/internal/flow/processor/output.go` | 修改 | P0-A1: 移除 StateDelta 事件的 `RequiresCompletion` 和 `AddNoticeChannelAndWait` |
| `pkg/trpc-agent-go/agent/graphagent/graph_agent.go` | 修改 | P0-A1: 移除 Graph barrier 的同步等待 |
| `pkg/trpc-agent-go/graph/executor.go` | 修改 | P0-A1: 移除 Graph executor barrier 的同步等待 |
| `pkg/trpc-agent-go/tool/tool.go` | 修改 | P0-D: 新增 `ResultBudget` 类型和 `Declaration.ResultBudget` 字段 |
| `pkg/trpc-agent-go/internal/flow/processor/functioncall_test.go` | 修改 | 更新 4 个测试以反映新行为 |
| `pkg/trpc-agent-go/agent/graphagent/graph_agent_test.go` | 修改 | 更新 2 个测试以反映新行为 |

### aranea-review 审查结论

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 |
|------|---------|---------|---------|
| 架构合规 | 0 | 0 | 0 |
| 质量 | 0 | 1（缺单元测试） | 0 |
| 正确性 | 1（已修复） | 0 | 0 |
| 可维护性 | 0 | 1（死代码待清理） | 0 |
| 错误处理 | 0 | 0 | 0 |
| 文档同步 | 0 | 1（框架文档待更新） | 0 |

**审查发现的严重问题（已修复）**：
- `truncateResult` 直接字节切片导致 JSON 格式损坏 → 已修复为 JSON envelope 包装
- `ensureToolTimeout` goroutine 残留 → 已修复为返回 `CancelFunc` + `defer cancel()`

**遗留项（后续迭代处理）**：
- M-1: P0-G/P0-D 新增功能缺少专门单元测试
- L-1: `maybeConsumeQueuedUserMessages` 仍保留 `RequiresCompletion`，需补充注释说明保留原因

**已完成的遗留项**：
- ~~M-3: `completeSuppressedGraphAgentBarrier` 和 `notifySuppressedBarrierCompletion` 为死代码~~ → ✅ 已清理（graph_agent.go 和 executor.go）
- ~~L-4: 框架文档需同步更新~~ → ✅ 已更新（en/zh tool.md + event.md，新增 ResultBudget 和 Tool Execution Timeout 文档，更新 RequiresCompletion 语义变更说明）
