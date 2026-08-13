# L0 上下文压缩优化 — 开发计划

> **需求**：[`L0-compression.md`](./L0-compression.md) · **设计**：[`L0-compression.design.md`](./L0-compression.design.md)
>
> **⚠️ 变更通知（2026-07-20）**：L1 MicroCompact 已全链路移除（代码/配置/proto/DB/前端）。移除原因：`loadCompressBody` 仅保留 user/assistant 消息，工具消息过滤逻辑恒不触发，功能从未生效。压缩级联现为两级：L2 Memory Compact → L3 LLM。详见 `docs/superpowers/plans/2026-07-20-session-compression-hardening.md`。

---

## 现状

| 项 | 状态 | 证据 |
|----|------|------|
| SessionCompressor（L3 AutoCompact） | ✅ | `internal/session/compressor.go` |
| CAS + 事务原子性 | ✅ | `TryIncrementCompressVersion` + `CompressSessionInTx` |
| 四种截断策略 | ✅ | summary/drop_oldest/hybrid/drop_tool_results |
| 专用压缩模型 | ✅ | `L0CompressProvider` / `L0CompressModel` |
| 防抖与去重 | ✅ | `inFlight` / `compressDebounceActive` / `SessionSummaryExists` |
| 记忆联动 | ✅ | 压缩后 `resyncSessionMemory` |
| ~~L1 MicroCompact~~ | ❌ 已移除（2026-07-20） | ~~`internal/session/micro_compact.go`~~ 文件已删除；功能从未生效（body 无 tool 消息） |
| L2 Memory Compact | ✅ | `internal/session/memory_compact.go` + `internal/data/memory_fact_reader.go` |
| ~~L1/L2 配置开关~~ | ❌ 部分移除 | ~~`AgentRuntimeSettings.MicroCompactEnabled`~~ 已删；`MemoryCompactEnabled` 保留 + `compress_policy.go` |
| L2 估算升级 | ✅ | `estimateCompactedPromptTokens` + L2→L3 自动升级（原 L1→L2→L3，L1 层已移除） |
| ~~L1 可压缩工具代理~~ | ❌ 已移除 | ~~内容长度 >200 字符作为代理指标~~ 随 L1 一并移除 |
| L2/L3 压缩通知 | ✅ | `publishCompressionNotice` 统一处理 L2/L3（原 L1/L2/L3） |
| 9 章节摘要 | ✅ | `internal/compress/prompt.go`（v1→v2） |
| 工具结果持久化 | ✅ | `internal/biz/tool_result_gate.go` + `internal/data/tool_result_repo.go` + Ent schema |
| ReadToolResult 工具 | ✅ | `internal/tools/custom/read_tool_result.go` + Registry + Seed |
| BeforeModel 回调注入 | ✅ | `internal/agent/tool_result_gate_hook.go`（优先级 3） |
| ToolResultGate 配置开关 | ✅ | `AgentRuntimeSettings.ToolResultGateEnabled` |
| CompressorDeps 拆分 | ✅ | 构造函数直接列出 7 个依赖，消除上帝接口 |
| ToolResultGate 单元测试 | ✅ | `tool_result_gate_test.go`（8 用例）+ `tool_result_gate_hook_test.go`（9 用例） |
| L2 单元测试 | ✅ | ~~`micro_compact_test.go`（8 用例）~~ 已随 L1 删除 + `memory_compact_test.go`（9 用例） |
| LLM 压缩响应缓存 | ✅ | `internal/compress/cache.go`（CompressCache + CachingCompressor 装饰器） |
| 手动压缩 API | ✅ | `POST /v1/sessions:compact` + `biz.ManualCompressor` + 前端压缩按钮 |
| **tail 保留修复** | ✅ | `loadCompressBody` 返回 body+tail 穿透至事务，压缩后快照保留近期轮次（修复 tail 恒为空缺陷） |
| **递归滚动摘要** | ✅ | LLM 压缩传入 `PriorSummary` 吸收合并历史摘要；事务内 `DeleteSessionSummaries` + 写入单行合并摘要，根治无限拼接 |
| 摘要质量门 | ✅ | `compress_quality.go`：退化检测（<200 runes vs ≥1000 runes 原文）+ 减量守卫（≥80% 丢弃）+ 错误分类（deterministic/transient） |
| 压缩失败抑制 | ✅ | `compress_suppress.go`：deterministic sticky（按压缩模型）+ transient minGap 退避；forced 触发绕过 |
| 双锚点 token 校准 | ✅ | `compress/service.go` 压缩成功路径调用 `llmcontext.RecordAuthoritativeUsage`，共享估算器从 2.5 chars/token 默认值校准到真实比率 |
| **TurnNumber 断链修复（评审 F0）** | ✅ | `biz/session/activity_message_adapter.go` `synthesizeTurnNumbers`：activities 只有 TurnID 无数字轮次，按时间序合成稳定序号，修复压缩体恒为空的致命空转 |
| **MemoryCompact ICS 质量门（评审 F1）** | ✅ | `memory_compact.go` `memoryCompactMinICS=0.5`：6 维加权覆盖分，低于门控放弃 L2 防稀疏事实替换对话体 |
| **L3 分块滚动摘要（评审 F2）** | ✅ | `snapshot.go` `splitMessagesForCompress`（`compressChunkMaxRunes=24000`）+ `compressor.go` 逐块 PriorSummary 滚动吸收 |
| **工具消息进 transcript（评审 F3）** | ✅ | `loadCompressBody` 增返回 toolBody；`renderCompressMessage` 渲染 `TOOL(name): body`（≤1000 runes），消息级截断 ≤8000 runes |
| **压缩后估值计入 reserved_system（评审 F4）** | ✅ | `estimateCompactedPromptTokens(..., reservedSystem)`：修复 L2→L3 升级决策系统性偏低 |
| **Section 6 上限 + PromptVersion v3（评审 F5）** | ✅ | `compress/prompt.go` v3：最近 30 条逐字 + 更早压缩为主题列表，防摘要自膨胀触发减量守卫 |
| **ToolsProfile 驱动保留 token（评审 F6）** | ✅ | `compress_policy.go` `CompressProfile.ToolsProfile`：修复 SnapshotMode 误当 profile 致保留 token 恒 8000 |
| **ctx 取消静默中止（复审 G1）** | ✅ | `compressor.go` `llmSummarize`：`fail==none && md==""` 区分 ctx 取消，不记抑制、hybrid 不写兜底标记 |
| **减量守卫计入历史摘要（复审 G2）** | ✅ | `compressor.go`：守卫分母 = totalRunes + priorMerged runes，修复成熟长会话必然误杀 |
| **cacheHit 全块聚合（复审 G3）** | ✅ | `compressor.go`：逐块 `&&` 聚合，仅全块命中才上报 true |
| **options_json 工具名转义（复审 G4）** | ✅ | `activity_message_adapter.go` `buildOptionsJSON` 改 `json.Marshal`，特殊字符不再产出非法 JSON |
| **CAS 冲突/幂等命中不记假成功（复审 G5）** | ✅ | `executeCompression` 返回 `(wrote, err)`；`runCompress` 未写入时保留抑制、不打成功日志 |
| 压缩读取下推（复审 G6） | ❌ 技术债 | `TECH-DEBT(COMPRESS-IO)`：`ListMessagesAfterTurn` 全量 `ListBySession` 无 turn 下推，长会话每次软触发全表读 |
| 记忆操作语义化 | ❌ | 只有 ADD |
| 时间维度 | ❌ | 不存在 |
| 动态链接 | ❌ | 不存在 |
| Agent 自主压缩工具 | ❌ | 不存在 |
| 代码骨架提取 | ❌ | 不存在 |

---

## 阶段一：工程补强（P0）

### Sprint 1A：工具结果持久化（入口管控）

| # | 任务 | 依赖 | 估时 | 状态 |
|---|------|------|------|------|
| 1A-1 | Ent schema：`tool_result_blobs` + `tool_result_replacements` 表 | 无 | 0.5d | ✅ |
| 1A-2 | Biz 层：`ToolResultBlobReader/Writer` + `ToolResultReplacementReader/Writer` 端口接口 | 1A-1 | 0.5d | ✅ |
| 1A-3 | Data 层：端口实现 | 1A-2 | 1d | ✅ |
| 1A-4 | `ToolResultGate`：大小检查 + 持久化 + 替换决策冻结逻辑 | 1A-3 | 1.5d | ✅ |
| 1A-5 | BeforeModel 回调集成：工具结果返回路径接入 ToolResultGate | 1A-4 | 1d | ✅ |
| 1A-6 | `ReadToolResult` 工具：注册 + 实现 | 1A-3 | 0.5d | ✅ |
| 1A-7 | BeforeModel 回调：消息历史注入时读取冻结预览 | 1A-4 | 0.5d | ✅（与 1A-5 合并实现） |
| 1A-8 | 单元测试 + 集成测试 | 1A-5, 1A-6, 1A-7 | 1d | ✅ `tool_result_gate_test.go` + `tool_result_gate_hook_test.go` |
| 1A-9 | `agent_runtime_settings` 新增配置字段 + 前端设置面板 | 1A-1 | 0.5d | ✅（后端配置字段已加） |

**Sprint 1A 总估时**：~7d

### Sprint 1B：三层代价递进压缩

| # | 任务 | 依赖 | 估时 | 状态 |
|---|------|------|------|------|
| 1B-1 | ~~L1 MicroCompact：`runMicroCompact` 方法实现~~ ❌ 已移除（2026-07-20） | 1A-5 | 1.5d | ~~✅ `micro_compact.go`~~ 文件已删除 |
| 1B-2 | ~~L1 MicroCompact：集成到 `runCompress` 判断链前置~~ ❌ 已移除（2026-07-20） | 1B-1 | 0.5d | ~~✅ `compressor.go` L1 分支~~ 已删除 |
| 1B-3 | L2 Memory Compact：`runMemoryCompact` 方法实现 | 无 | 1.5d | ✅ `memory_compact.go` + `memory_fact_reader.go` |
| 1B-4 | L2 Memory Compact：集成到 `runCompress` 判断链 | 1B-3 | 0.5d | ✅ `compressor.go` L2 分支 |
| 1B-5 | L1/L2 压缩后的快照更新 + 上下文用量更新 | 1B-2, 1B-4 | 1d | ✅ 复用 L3 事务逻辑 |
| 1B-6 | 压缩层级自动升级逻辑（L1→L2→L3） | 1B-2, 1B-4 | 0.5d | ✅ `estimateCompactedPromptTokens` + 阈值比较 |
| 1B-7 | 单元测试 + 集成测试 | 1B-5, 1B-6 | 1d | ✅ ~~`micro_compact_test.go`~~ 已随 L1 删除 + `memory_compact_test.go` |

**Sprint 1B 总估时**：~6.5d

### Sprint 1C：摘要结构升级 + 手动压缩

| # | 任务 | 依赖 | 估时 | 状态 |
|---|------|------|------|------|
| 1C-1 | 升级 `compress/prompt.go` 系统提示词为 9 章节 | 无 | 0.5d | ✅ v1→v2 |
| 1C-2 | 升级 `compress/prompt.go` 强制保留用户消息原文的指令 | 1C-1 | 0.5d | ✅ 第 6 章强制保留 |
| 1C-3 | Proto 定义：`CompactSession` RPC | 无 | 0.5d | ✅ |
| 1C-4 | Service 层：`CompactSession` 实现 | 1C-3, 1B-6 | 1d | ✅ biz.ManualCompressor + session.Compressor.CompactSession |
| 1C-5 | Server 层：注册 HTTP 路由 | 1C-4 | 0.5d | ✅ POST /v1/sessions:compact |
| 1C-6 | 前端：压缩按钮 + 压缩进度 toast + 自定义保留指令对话框 | 1C-4 | 1.5d | ✅ ChatHeaderUsagePanel 压缩按钮 + toast |
| 1C-7 | 回归测试：9 章节摘要质量 + 手动压缩流程 | 1C-1, 1C-4 | 1d | ✅ |

**Sprint 1C 总估时**：~5.5d

### Sprint 1D：LLM 压缩响应缓存

| # | 任务 | 依赖 | 估时 | 状态 |
|---|------|------|------|------|
| 1D-1 | `internal/compress/cache.go`：`CompressCache` 实现（LRU + TTL 双淘汰，sha256 缓存键） | 无 | 1d | ✅ |
| 1D-2 | `internal/compress/cache.go`：`CachingCompressor` 装饰器（实现 `compress.Compressor` 接口） | 1D-1 | 0.5d | ✅ |
| 1D-3 | `internal/session/compressor.go`：`runCompress` 中注入 sessionID 到 context | 1D-2 | 0.5d | ✅ |
| 1D-4 | `agent_runtime_settings` 新增配置字段 + Ent schema | 无 | 0.5d | ✅ |
| 1D-5 | Wire 装配变更：`provideCompressor` 替代直接 Bind | 1D-2 | 0.5d | ✅ |
| 1D-6 | `publishCompressionNotice` 增加 `cache_hit` metadata 字段 | 1D-2 | 0.5d | ✅ |
| 1D-7 | 单元测试：缓存命中/未命中/过期/淘汰/并发安全 | 1D-1, 1D-2 | 1d | ✅ `cache_test.go`（15 用例） |

**Sprint 1D 总估时**：~4.5d

### 阶段一总估时

| Sprint | 估时 | 可并行 |
|--------|------|--------|
| 1A | 7d | - |
| 1B | 6.5d | 依赖 1A-5，1B-1/1B-3 可并行 |
| 1C | 5.5d | 1C-1/1C-3 可与 1B 并行 |
| 1D | 4.5d | 1D-1/1D-4 可与 1B/1C 并行，1D-5 依赖 1B-6 |
| **总计** | **~23.5d** | 1B+1C+1D 部分并行后约 **16d** |

---

## 评审修复轮（2026-08-13）

> 系统层面深入评审（对标业界实现 + 前沿论文调研）发现 1 个致命断链 + 5 个质量/精度缺陷。设计详见 `L0-compression.design.md` §1.6。全部按 TDD 实施（失败测试先行），验证：`go build ./cmd/... ./internal/... ./api/... ./pkg/...` ✅ + `go test ./internal/session/... ./internal/compress/...` 全绿（含 `internal/session/trpc` PG 集成测试）。

| # | 任务 | 严重度 | 改动文件 | 状态 |
|---|------|--------|----------|------|
| F0 | TurnNumber 断链修复：activities 适配器合成稳定轮次序号 | 致命 | `internal/biz/session/activity_message_adapter.go` | ✅ |
| F1 | MemoryCompact ICS 质量门（≥0.5 才接受 L2） | 高 | `internal/session/memory_compact.go` | ✅ |
| F2 | L3 分块滚动摘要（24000 runes/块，PriorSummary 逐块吸收） | 高 | `internal/session/snapshot.go`、`internal/session/compressor.go` | ✅ |
| F3 | 工具消息渲染进 L3 transcript + 消息级截断 | 高 | `internal/session/compressor.go`、`internal/session/snapshot.go`、`internal/session/compress_policy.go` | ✅ |
| F4 | 压缩后估值计入 reserved_system | 中 | `internal/session/token_estimate.go` | ✅ |
| F5 | Section 6 用户消息上限 30 条 + PromptVersion v3 | 中 | `internal/compress/prompt.go` | ✅ |
| F6 | ToolsProfile 驱动保留 token（修 SnapshotMode-as-profile bug） | 中 | `internal/session/compress_policy.go` | ✅ |

**测试**：`internal/session/compress_pipeline_review_test.go`（F2-F6 失败测试先行）+ `compressor_test.go` / `memory_compact_test.go` 存量用例适配（ICS 门控后 stub 事实需覆盖 intent/state/decision 维度）。

---

## 复审加固轮（2026-08-13，第二轮深入检查）

> F0-F6 落地后第二轮系统复审发现 5 个实现级缺陷（G1-G5）+ 1 项 I/O 技术债（G6）。设计详见 `L0-compression.design.md` §1.7。全部按 TDD 实施（失败测试先行），验证：`go test ./internal/session/ ./internal/biz/session/ -count=1` 全绿。

| # | 任务 | 严重度 | 改动文件 | 状态 |
|---|------|--------|----------|------|
| G1 | ctx 取消静默中止（不记抑制、hybrid 不写兜底标记） | 高 | `internal/session/compressor.go` | ✅ |
| G2 | 减量守卫分母计入被吸收的历史摘要 | 高 | `internal/session/compressor.go` | ✅ |
| G3 | cacheHit 逐块聚合（部分命中不得谎称整次零调用） | 中 | `internal/session/compressor.go` | ✅ |
| G4 | `buildOptionsJSON` 改 `json.Marshal` 转义工具名 | 中 | `internal/biz/session/activity_message_adapter.go` | ✅ |
| G5 | CAS 冲突/幂等命中上报 wrote=false，不记假成功 | 中 | `internal/session/compressor.go`、`compressor_test.go`（4 处调用适配） | ✅ |
| G6 | 压缩读取路径全量加载 Activity（无 turn 下推） | 低（技术债） | `internal/biz/session/activity_message_adapter.go`（仅 TECH-DEBT 注释登记） | 📋 已登记 |

**测试**：`compress_pipeline_review_test.go` 新增 G1/G2/G3/G5 用例（ctx 取消双策略、守卫计入 prior、部分/全量缓存命中、executeCompression CAS 冲突、runCompress 级抑制保留）；`activity_message_adapter_turn_test.go` 新增 G4 转义往返用例。

---

## 阶段二：记忆演化（P2）

### Sprint 2A：记忆操作语义化

| # | 任务 | 依赖 | 估时 |
|---|------|------|------|
| 2A-1 | `memory_facts` 表新增字段（operation_type/scope/valid_from/valid_until/decay_rate/version/prev_version_id/merged_from_ids/audit_reason） | 无 | 0.5d |
| 2A-2 | `memory_fact_audit_log` 表 + Ent schema | 无 | 0.5d |
| 2A-3 | 升级 `compress/memory_extract.go` V2 提示词：双阶段提取（操作分类 + 执行） | 无 | 1d |
| 2A-4 | `auto_memory_queue` 扩展：支持 UPDATE/DELETE/MERGE 操作处理 | 2A-1, 2A-3 | 2d |
| 2A-5 | 作用域分类逻辑：static/dynamic/episodic 衰减策略 | 2A-1 | 1d |
| 2A-6 | 审计日志写入 | 2A-2, 2A-4 | 0.5d |
| 2A-7 | 单元测试 + 集成测试 | 2A-4, 2A-5 | 1.5d |

**Sprint 2A 总估时**：~7d

### Sprint 2B：动态链接

| # | 任务 | 依赖 | 估时 |
|---|------|------|------|
| 2B-1 | `memory_facts` 表新增 `link_ids` 字段 | 无 | 0.5d |
| 2B-2 | 链接建立逻辑：新记忆写入时 embedding 检索 + LLM 关系判断 | 2B-1 | 2d |
| 2B-3 | 双向链接维护：两端 link_ids 同步更新 | 2B-2 | 1d |
| 2B-4 | 矛盾检测：contradicts 关系触发 UPDATE/DELETE 候选 | 2B-2 | 1d |
| 2B-5 | 前端 Memory Center：链接展示 + 矛盾提示 | 2B-3, 2B-4 | 1.5d |
| 2B-6 | 单元测试 + 集成测试 | 2B-3, 2B-4 | 1d |

**Sprint 2B 总估时**：~7d

### 阶段二总估时

| Sprint | 估时 | 可并行 |
|--------|------|--------|
| 2A | 7d | - |
| 2B | 7d | 依赖 2A-1，2B-1/2B-2 可与 2A-4 并行 |
| **总计** | **~14d** | 部分并行后约 **10d** |

---

## 阶段三：Agent 自主压缩（P3）

### Sprint 3A：Agent 自主压缩工具

| # | 任务 | 依赖 | 估时 |
|---|------|------|------|
| 3A-1 | `CompactContext` 工具：注册 + 实现 | 1B-6 | 2d |
| 3A-2 | `RecallDetail` 工具：注册 + 实现 | 无 | 1.5d |
| 3A-3 | 压缩后完整记录保留逻辑（不删除 session_messages，仅移除快照） | 3A-1 | 1d |
| 3A-4 | Agent 构建时注入自主压缩工具 | 3A-1, 3A-2 | 0.5d |
| 3A-5 | 单元测试 + 集成测试 | 3A-3, 3A-4 | 1d |

**Sprint 3A 总估时**：~6d

### Sprint 3B：代码骨架提取

| # | 任务 | 依赖 | 估时 |
|---|------|------|------|
| 3B-1 | 引入 tree-sitter Go 绑定依赖 | 无 | 0.5d |
| 3B-2 | `CodeSkeleton` 工具：注册 + 实现 | 3B-1 | 2d |
| 3B-3 | ~~MicroCompact 代码感知：L1 对代码类工具结果自动替换为骨架~~ ❌ 已取消（L1 已移除，2026-07-20） | 3B-2, 1B-1 | 1.5d |
| 3B-4 | 单元测试 + 集成测试 | 3B-2, 3B-3 | 1d |

**Sprint 3B 总估时**：~5d

### 阶段三总估时

| Sprint | 估时 | 可并行 |
|--------|------|--------|
| 3A | 6d | - |
| 3B | 5d | 依赖 3B-1 + 1B-1，可与 3A 并行 |
| **总计** | **~11d** | 并行后约 **7d** |

---

## 全局时间线

```
Week 1-2:  Sprint 1A（工具结果持久化）
Week 2-3:  Sprint 1B（三层压缩） + Sprint 1C（摘要升级 + 手动压缩） + Sprint 1D（LLM 缓存）并行
Week 4:    阶段一集成测试 + 验收
           ↓
Week 5-6:  Sprint 2A（记忆操作语义化）
Week 6-7:  Sprint 2B（动态链接）
Week 8:    阶段二集成测试 + 验收
           ↓
Week 9-10: Sprint 3A（Agent 自主压缩） + Sprint 3B（代码骨架）并行
Week 10:   阶段三集成测试 + 验收
```

---

## 代码锚点

### 阶段一

- `internal/session/compressor.go` — 压缩判断链 + L1/L2 分支 + 缓存 sessionID 注入 + 分块滚动摘要（F2）+ toolBody 穿透（F3）+ ctx 取消静默/守卫分母/cacheHit 聚合/CAS wrote（G1/G2/G3/G5）
- `internal/session/snapshot.go` — transcript 渲染截断 + `splitMessagesForCompress` 分块（F2/F3）
- `internal/session/compress_policy.go` — 压缩策略 + `CompressProfile.ToolsProfile`（F6）+ 工具渲染策略判定（F3）
- `internal/session/token_estimate.go` — 压缩后估值含 reserved_system（F4）
- `internal/session/memory_compact.go` — L2 + ICS 质量门（F1）
- `internal/biz/session/activity_message_adapter.go` — `synthesizeTurnNumbers` 轮次序号合成（F0）+ options_json 转义（G4）+ `TECH-DEBT(COMPRESS-IO)` 登记（G6）
- `internal/compress/prompt.go` — 摘要系统提示词（9 章节，v3：Section 6 上限 30 条，F5）
- `internal/compress/service.go` — LLM 摘要服务
- `internal/compress/cache.go` — LLM 压缩响应缓存（CompressCache + CachingCompressor）
- `internal/compress/memory_extract.go` — 记忆提取提示词
- `internal/tools/` — 工具结果返回路径 + 新工具注册
- `internal/biz/` — 新端口接口
- `internal/data/` — 新表 + 端口实现
- `internal/service/` — CompactSession API
- `api/` — Proto 定义

### 阶段二

- `internal/compress/memory_extract.go` — 双阶段提取
- `internal/memory/trpc/auto_memory_queue.go` — 操作类型扩展
- `internal/biz/` — MemoryFact 模型扩展
- `internal/data/` — Ent schema 扩展

### 阶段三

- `internal/tools/` — CompactContext / RecallDetail / CodeSkeleton 工具
- `internal/agent/` — 工具注入

---

## 验证策略

### 阶段一验证

| 验证项 | 方法 |
|--------|------|
| 工具结果持久化 | 构造 > 50K 字符的工具结果，验证自动持久化 + 预览保留 |
| 替换决策冻结 | 多轮对话后验证同一工具结果的预览字符串字节级一致 |
| ReadToolResult | Agent 通过工具读取持久化内容，验证内容完整 |
| ~~L1 MicroCompact~~ ❌ 已移除 | ~~验证旧工具结果被清理，近期结果保留~~ |
| L2 Memory Compact | 验证复用记忆生成摘要，零额外 API 调用 |
| L3 摘要结构 | 验证 9 章节输出，用户消息原文逐条保留 |
| 手动压缩 API | 调用 CompactSession，验证压缩触发 + 结果返回 |
| 压缩层级升级 | 构造不同 token 量级，验证 L1→L2→L3 自动升级 |
| LLM 压缩响应缓存命中 | 构造相同消息序列的重复压缩请求，验证缓存命中（零 LLM 调用） |
| LLM 压缩响应缓存失效 | 修改 PromptVersion 或 Provider/Model，验证缓存未命中 |
| LLM 压缩响应缓存 TTL | 等待 TTL 过期后验证缓存未命中 |
| LLM 压缩响应缓存淘汰 | 超过 maxEntries 后验证 LRU 淘汰 |

### 阶段二验证

| 验证项 | 方法 |
|--------|------|
| UPDATE 操作 | 对话中修正之前的事实，验证 UPDATE 而非 ADD |
| MERGE 操作 | 多条相关事实存在时，验证合并为一条 |
| 作用域分类 | 验证 static/dynamic/episodic 不同衰减策略 |
| 动态链接 | 新事实写入后验证自动建立链接 |
| 矛盾检测 | 对话中否定之前的事实，验证 contradicts 关系 |

### 阶段三验证

| 验证项 | 方法 |
|--------|------|
| Agent 自主压缩 | Agent 在完成子任务后主动调用 CompactContext |
| RecallDetail | Agent 通过工具恢复被压缩的细节 |
| 代码骨架 | 验证骨架保留签名/定义，丢弃函数体 |
| 压缩后任务连续性 | 压缩后 Agent 仍能正确继续当前任务 |

---

## 风险与缓解

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| 工具结果持久化影响消息历史前缀一致性 | 中 | Prompt Cache 失效 | 替换决策冻结 + 预览字符串字节级一致 |
| ~~L1 MicroCompact 误清理仍在使用的工具结果~~ ❌ 已移除 | — | — | ~~`min_age_turns` 保护 + 可压缩工具白名单~~ |
| L2 Memory Compact 摘要质量不足 | 中 | 压缩后信息丢失 | 自动升级到 L3 + 质量评估 |
| 9 章节摘要 token 开销增加 | 低 | 摘要本身占更多 token | 摘要 token 预算控制 + 章节可配置 |
| LLM 压缩响应缓存一致性 | 低 | PromptVersion 升级后旧缓存返回过时摘要 | 缓存键包含 PromptVersion，自动失效；TTL 兜底 |
| LLM 压缩响应缓存内存占用 | 低 | 大量会话并发压缩时缓存条目过多 | LRU 淘汰 + maxEntries 上限（默认 256） |
| 记忆操作语义化 LLM 判断不准 | 中 | 误 UPDATE/DELETE | 审计日志 + 用户确认流程 |
| tree-sitter 依赖引入复杂度 | 中 | 构建和维护成本 | 阶段三可选，不影响阶段一/二 |
