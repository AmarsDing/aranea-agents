# ADR-07: 双压缩系统统一 — 系统 A 降级为确定性紧急截断

## 状态：已接受（2026-07-20）

## 背景

项目历史上并存两套上下文压缩系统：

- **系统 A（BeforeModel Hook，内存态）**：`internal/agent/context_compression_inject.go` 在每次模型调用前判断 token 使用率，超过阈值（`MemoryRuntimePolicy.CompressionThreshold`，默认 0.80）时调用 `biz.ContextCompressor`（实现 `memory.LLMContextCompressor`）对淘汰消息做 LLM 递归摘要，摘要仅存在于 prompt 内存态，不持久化。LLM 由 `MEMORY_COMPRESSOR_PROVIDER`/`MEMORY_COMPRESSOR_MODEL` 环境变量配置。
- **系统 B（Session Compressor，持久化）**：`internal/session/compressor.go` 在轮间执行 L1 MicroCompact → L2 MemoryCompact → L3 LLM 压缩三级级联，摘要持久化到 `session_summaries` 表，配置走 `CompressPolicy`（HardTriggerRatio 默认 0.90）。

两套系统并存导致六个冲突点：

1. **阈值不通**：A 默认 0.80、B 默认 0.90，A 先于 B 触发，B 的压缩窗口被 A 提前吃掉。
2. **开关不通**：A 由 `L0SnapshotMode`/`ContextCompactionEnabled` 控制，B 由 `CompressPolicy` 控制，运维无法统一关停。
3. **双 LLM 成本**：同一批旧消息可能被 A（内存态）和 B（持久化）各摘要一次，双倍 token 开销。
4. **双摘要源不一致**：A 的内存态摘要与 B 的持久化摘要内容可能互相矛盾，LLM 同一 prompt 中看到哪个取决于时序。
5. **存储不通**：A 的摘要不落库，刷新/换端后丢失；B 的摘要落库但 A 不消费。
6. **配置体系分裂**：`MEMORY_COMPRESSOR_*` 环境变量 vs `CompressPolicy`/`agent_runtime_settings` 两套配置入口。

Grok xai-grok-compaction 的对照分析（`docs/reports/2026-07-19-analysis-grok-build-function-by-function-comparison.md`）确认：Grok 只有**一个**共享压缩引擎，压缩风格（code/intra/inter）只是策略差异，不存在双系统。

## 决策

**系统 A 降级为确定性紧急截断（deterministic emergency truncation），系统 B 保留为唯一的 LLM 压缩路径。**

具体改动：

1. **系统 A 重写**（`internal/agent/context_compression_inject.go`）：
   - 移除全部 LLM 调用。超过硬阈值（`HardTriggerRatio`，默认 0.90，镜像 `CompressPolicy`，复制常量以避免 agent → session 包依赖）时，机械删除最旧的非 system 消息（保留最近 30%，`defaultKeepRatio`），在最后一个 system 消息后插入 `<context_truncated>` 标记。
   - tool-call/tool-result 对安全切分（`snapToSafeBoundary`）：切分边界落在 tool-pair 中间时，整对移入保留侧，避免 orphan tool result 导致 LLM API 400。
   - 开关语义镜像 `sessionCompressEnabled`：默认开，`L0SnapshotMode=off` 且未显式 `ContextCompactionEnabled` 时关。
   - L0 快照 `TruncateStrategy` 标识改为 `"emergency_truncation"`。
2. **删除 LLM 链路**：
   - 删除 `internal/memory/context_compressor.go`（`LLMContextCompressor`）及测试。
   - 删除 `internal/biz/memory_compressor.go`（`ContextCompressor` 接口 + `ContextCompressionResult`）。
   - 移除 `runtime.MemorySet.ContextCompressor`、`agent.TRPCMemoryKnowledgeDeps.ContextCompressor` 字段及 5 处装配点（wire.go、team、a2a_endpoint、chat_orch_agent_build、openai_compat）。
   - 删除 wire `provideContextCompressor` 及 `MEMORY_COMPRESSOR_PROVIDER`/`MEMORY_COMPRESSOR_MODEL` 环境变量支持。
3. **系统 B 不动**：`internal/session` 保持轮间持久化 LLM 压缩（含本计划 Phase 1-4 新增的滚动摘要、质量门、失败抑制、缓存）。

## 后果

### 正面

- **消除双 LLM 成本**：每次模型调用前不再有隐藏的第二笔 LLM 开销。
- **消除摘要源不一致**：LLM 看到的压缩产物只可能来自系统 B 的持久化摘要。
- **系统 A 零失败模式**：确定性截断无外部依赖，不存在 LLM 超时/限流/401 导致的 hook 失败路径，也就不需要失败抑制逻辑。
- **配置统一**：阈值/开关语义与 `CompressPolicy` 对齐，删除 `MEMORY_COMPRESSOR_*` 环境变量入口。
- **轮内保护保留**：系统 A 作为 BeforeModel hook 仍是唯一能在**轮内工具循环**（turn 内多次模型调用）中拦截上下文溢出的机制——系统 B 只在轮间运行，无法覆盖该场景。

### 负面

- **被截断消息无摘要留存**：系统 A 截断的旧消息直接丢弃，不产生任何摘要。补偿机制：系统 B 在轮间已对历史做持久化滚动摘要，且触发截断的 0.90 硬阈值远高于系统 B 的常规触发点，正常情况下系统 B 早已压缩过该区间；截断只发生在系统 B 来不及运行的极端长轮内。
- **阈值常量复制**：`HardTriggerRatio` 默认值在 agent 包与 session 包各存一份（有意为之，避免 agent → session 包依赖）；两处注释互相指向，改值需同步。
- **`MemoryRuntimePolicy.CompressionThreshold/CompressionKeepRatio` 成为死配置字段**：hook 改造后不再消费，留待后续死配置清理事项移除。

## 替代方案

- **方案 1（统一阈值，保留双 LLM）**：只解决冲突点 1/2，双 LLM 成本与摘要源不一致依旧存在。否决。
- **方案 3（删除系统 A，只留系统 B）**：轮内工具循环（单轮内几十次模型调用，每次都可能追加大量 tool result）将无任何上下文溢出保护，可能直接打爆 context window 导致 400。否决。
- **方案 4（系统 A 改为调用系统 B 的持久化压缩）**：BeforeModel hook 内执行 DB 事务 + LLM 调用，延迟与失败面都不可接受，且 hook 无法拿到 session 级事务上下文。否决。
