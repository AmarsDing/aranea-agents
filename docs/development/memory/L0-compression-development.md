# L0 上下文压缩优化 — 开发计划

> **需求**：[`L0-compression.md`](./L0-compression.md) · **设计**：[`L0-compression.design.md`](./L0-compression.design.md)

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
| L1 MicroCompact | ✅ | `internal/session/micro_compact.go` |
| L2 Memory Compact | ✅ | `internal/session/memory_compact.go` + `internal/data/memory_fact_reader.go` |
| L1/L2 配置开关 | ✅ | `AgentRuntimeSettings.MicroCompactEnabled` / `MemoryCompactEnabled` + `compress_policy.go` |
| L1/L2 估算升级 | ✅ | `estimateCompactedPromptTokens` + L1→L2→L3 自动升级 |
| L1 可压缩工具代理 | ✅ | 内容长度 >200 字符作为代理指标 |
| L1/L2 压缩通知 | ✅ | `publishCompressionNotice` 统一处理 L1/L2/L3 |
| 9 章节摘要 | ✅ | `internal/compress/prompt.go`（v1→v2） |
| 工具结果持久化 | ✅ | `internal/biz/tool_result_gate.go` + `internal/data/tool_result_repo.go` + Ent schema |
| ReadToolResult 工具 | ✅ | `internal/tools/custom/read_tool_result.go` + Registry + Seed |
| BeforeModel 回调注入 | ✅ | `internal/agent/tool_result_gate_hook.go`（优先级 3） |
| ToolResultGate 配置开关 | ✅ | `AgentRuntimeSettings.ToolResultGateEnabled` |
| CompressorDeps 拆分 | ✅ | 构造函数直接列出 7 个依赖，消除上帝接口 |
| ToolResultGate 单元测试 | ✅ | `tool_result_gate_test.go`（8 用例）+ `tool_result_gate_hook_test.go`（9 用例） |
| L1/L2 单元测试 | ✅ | `micro_compact_test.go`（8 用例）+ `memory_compact_test.go`（9 用例） |
| LLM 压缩响应缓存 | ✅ | `internal/compress/cache.go`（CompressCache + CachingCompressor 装饰器） |
| 手动压缩 API | ✅ | `POST /v1/sessions:compact` + `biz.ManualCompressor` + 前端压缩按钮 |
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
| 1B-1 | L1 MicroCompact：`runMicroCompact` 方法实现 | 1A-5 | 1.5d | ✅ `micro_compact.go` |
| 1B-2 | L1 MicroCompact：集成到 `runCompress` 判断链前置 | 1B-1 | 0.5d | ✅ `compressor.go` L1 分支 |
| 1B-3 | L2 Memory Compact：`runMemoryCompact` 方法实现 | 无 | 1.5d | ✅ `memory_compact.go` + `memory_fact_reader.go` |
| 1B-4 | L2 Memory Compact：集成到 `runCompress` 判断链 | 1B-3 | 0.5d | ✅ `compressor.go` L2 分支 |
| 1B-5 | L1/L2 压缩后的快照更新 + 上下文用量更新 | 1B-2, 1B-4 | 1d | ✅ 复用 L3 事务逻辑 |
| 1B-6 | 压缩层级自动升级逻辑（L1→L2→L3） | 1B-2, 1B-4 | 0.5d | ✅ `estimateCompactedPromptTokens` + 阈值比较 |
| 1B-7 | 单元测试 + 集成测试 | 1B-5, 1B-6 | 1d | ✅ `micro_compact_test.go` + `memory_compact_test.go` |

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
| 3B-3 | MicroCompact 代码感知：L1 对代码类工具结果自动替换为骨架 | 3B-2, 1B-1 | 1.5d |
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

- `internal/session/compressor.go` — 压缩判断链 + L1/L2 分支 + 缓存 sessionID 注入
- `internal/session/compress_policy.go` — 压缩策略
- `internal/compress/prompt.go` — 摘要系统提示词（升级为 9 章节）
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
| L1 MicroCompact | 验证旧工具结果被清理，近期结果保留 |
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
| L1 MicroCompact 误清理仍在使用的工具结果 | 低 | Agent 丢失关键上下文 | `min_age_turns` 保护 + 可压缩工具白名单 |
| L2 Memory Compact 摘要质量不足 | 中 | 压缩后信息丢失 | 自动升级到 L3 + 质量评估 |
| 9 章节摘要 token 开销增加 | 低 | 摘要本身占更多 token | 摘要 token 预算控制 + 章节可配置 |
| LLM 压缩响应缓存一致性 | 低 | PromptVersion 升级后旧缓存返回过时摘要 | 缓存键包含 PromptVersion，自动失效；TTL 兜底 |
| LLM 压缩响应缓存内存占用 | 低 | 大量会话并发压缩时缓存条目过多 | LRU 淘汰 + maxEntries 上限（默认 256） |
| 记忆操作语义化 LLM 判断不准 | 中 | 误 UPDATE/DELETE | 审计日志 + 用户确认流程 |
| tree-sitter 依赖引入复杂度 | 中 | 构建和维护成本 | 阶段三可选，不影响阶段一/二 |
