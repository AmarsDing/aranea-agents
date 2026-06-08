# Memory 系统优化 — 开发计划

> **需求**：[`memory-optimization.md`](./memory-optimization.md) · **设计**：[`memory-optimization.design.md`](./memory-optimization.design.md)

---

## 现状

| 项 | 状态 | 证据 |
|----|------|------|
| L0 Snapshot 写入 | ✅ 已限流 | 最小间隔 300s + ratio delta 0.10 + 阈值穿越 0.80 + 低 ratio 跳过 0.60 |
| segments_json | ✅ 已精简 | 聚合统计替代逐条详情，数据量减少 80%+（字段名保留 segments_json，内容为 summary 格式） |
| Prompt Cache | ❌ 完全失效 | 动态内容与静态内容混在一个 TextBlock |
| 压缩预算 | ⚠️ 硬编码 | reserved_system 不随 Agent 配置变化 |
| Level 1 MicroCompact | ⚠️ 死代码 | `micro_compact.go` 有测试无生产调用 |
| Level 2 Memory Compact | ⚠️ 死代码 | `memory_compact.go` 有测试无生产调用 |
| L1 token_estimate | ❌ 始终为 0 | 调用方从未设置该字段 |
| L1 used_tokens | ❌ 始终为 0 | 聚合从未执行 |
| L1 选择性注入 | ❌ 全量注入 | 无预算硬上限 |
| Episode 结构化生成 | ❌ 全靠 LLM | 70%~90% 可零成本生成 |
| consolidation_status | ⚠️ 三值不一致 | "pending"/"consolidated"/"done" |
| L2ConsolidateWorker | ❌ 空转 | agentID="" bug |
| HMAC 完整性保护 | ❌ ROI 偏低 | 密钥和签名在同一数据库 |
| field_kind 枚举 | ⚠️ 无语义分类 | 缺少 decision/artifact/progress |

---

## 阶段一：L0 压缩与缓存优化（P0）

### Sprint 1A：L0 Snapshot 限流

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 1A-1 | 新增 `L0SnapshotEnabled` 配置字段，解耦 `EvolutionMetricsEnabled` | 无 | ✅ |
| 1A-2 | `segments_json` 精简为 `segments_summary_json`（仅聚合统计） | 无 | ✅ |
| 1A-3 | 实现写入限流——最小间隔 + 变化量阈值 + 阈值穿越强制写入 | 1A-1 | ✅ |
| 1A-4 | session 运行时新增 `lastL0SnapshotWriteAt` / `lastL0SnapshotRatio` 变量 | 1A-1 | ✅ |
| 1A-5 | 前端 `MemorySnapshotDrawer` 改为展示聚合统计 | 1A-2 | ✅ |
| 1A-6 | 删除未实现的 Datadog 指标规划（死规划清理） | 无 | ✅ |
| 1A-7 | 单元测试 | 1A-3, 1A-4 | ✅ |

### Sprint 1B：三层前缀分离 + Prompt Cache

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 1B-1 | 框架层：`BeforeModelHook` 新增 `Layer SystemLayer` 字段 | 无 | ✅ |
| 1B-2 | 框架层：Hook 装配器按 Layer 排序，同 Layer 内按 Priority 排序 | 1B-1 | ✅ |
| 1B-3 | 框架层：`WithSystemCacheStrategy` / `WithCacheSystemPromptDualBreakpoint` Option | 无 | ✅ |
| 1B-4 | Anthropic adapter：改造 `applyCacheControlToSystem`，支持 DualBreakpoint + 断点计数校验 | 1B-3 | ✅ |
| 1B-5 | Anthropic adapter：`convertSystemMessageContent` 先 Content 后 ContentParts | 无 | ✅ |
| 1B-6 | `TimeRequestProcessor`：时间信息写入 ContentParts | 无 | ✅ |
| 1B-7 | Hunyuan adapter：修复 ContentParts 处理 | 无 | ✅ |
| 1B-8 | 应用层：现有 Hook 声明 Layer（staticRuntimeCue=Static, dynamicRuntimeCue=SemiStatic, SkillGuidance=SemiStatic, MemoryInject=Dynamic, KnowledgeCue=Dynamic） | 1B-1 | ✅ |
| 1B-9 | 应用层：`memory_inject.go` 重构 `buildRuntimeMemoryCue()` | 1B-8 | ✅ |
| 1B-10 | 应用层：`runtime_cue_inject.go` 拆分为 staticRuntimeCue + dynamicRuntimeCue | 1B-8 | ✅ |
| 1B-11 | 应用层：`internal/provider/trpc_llm.go` 启用 DualBreakpoint 模式 | 1B-4 | ✅ |
| 1B-12 | 压缩重建后 MemoryInject 选择性重执行 | 1B-9 | ✅ |
| 1B-13 | 单元测试 + 集成测试 | 1B-4~1B-12 | ✅ |

**关键约束**：1B-4（DualBreakpoint）必须与 1B-8~1B-10（三层前缀分离）同步实施，否则压缩场景下缓存完全失效。

### Sprint 1C：压缩预算动态计算

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 1C-1 | 实现 `calculateReservedSystem`（基于 prompt_snapshot section 字段） | 无 | ❌ |
| 1C-2 | 实现 `profileBasedDefault`（ToolsProfile 分级默认值） | 无 | ❌ |
| 1C-3 | 改造压缩触发逻辑，使用 effective_budget 计算三级阈值 | 1C-1 | ❌ |
| 1C-4 | 配置项 `compression_buffer_ratio` 默认值改为 0.15 | 1C-3 | ❌ |
| 1C-5 | 自适应缓冲区策略（监控 token 增量，自动调整 ratio） | 1C-4 | ❌ |
| 1C-6 | 对话模式检测（tool_call_count/turn_count） | 1C-5 | ❌ |
| 1C-7 | hard_trigger 时 UI 显示"正在优化上下文..." | 1C-3 | ❌ |
| 1C-8 | 单元测试 | 1C-1~1C-7 | ❌ |

### Sprint 1D：Level 2 Memory Compact 增强

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 1D-1 | 激活 Level 1 MicroCompact：接入 BeforeModel hook，补充清除逻辑 | 无 | ❌ |
| 1D-2 | 激活 Level 2 Memory Compact Step 1：接入 runCompress，使用 MemoryFactReader | 无 | ❌ |
| 1D-3 | Level 2 Step 2：Compressor 新增 `l1Reader`，Wire 注入更新 | 1D-2 | ❌ |
| 1D-4 | 实现 ICS 评估（6 维分级评分 + 降级规则） | 1D-3 | ❌ |
| 1D-5 | 重写 `tryMemoryCompact` 摘要生成（结构化模板 + L1+L3 数据源） | 1D-4 | ❌ |
| 1D-6 | Level 2 失败后等待 hard_trigger（不立即升级） | 1D-5 | ❌ |
| 1D-7 | 压缩后强制写入 L0 Snapshot（不受限流约束） | 1D-6, 1A-3 | ❌ |
| 1D-8 | 事务安全增强（CAS-事务间隙幂等重入 + 补偿机制） | 1D-6 | ❌ |
| 1D-9 | 压缩进行中标记 + 8 分钟超时自动释放 | 1D-6 | ❌ |
| 1D-10 | 前端上下文指示器（正常/优化中/已优化/正在优化） | 1D-6 | ❌ |
| 1D-11 | 单元测试 | 1D-1~1D-10 | ❌ |

---

## 阶段二：L1 预算与选择性注入（P1）

### Sprint 2A：L1 预算硬上限

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 2A-1 | `UpsertL1Field` 时计算 `token_estimate` | 无 | ❌ |
| 2A-2 | `used_tokens` 同步聚合 + DB 事务行锁 | 2A-1 | ❌ |
| 2A-3 | 事务内预算检查，超预算回滚返回 `ErrL1Overflow` | 2A-2 | ❌ |
| 2A-4 | 三层过滤链（visibility → pin_to_prompt → 相关性 → 预算） | 2A-3 | ❌ |
| 2A-5 | Token 估算精度改进：短期 runeCount/2 | 2A-1 | ❌ |
| 2A-6 | 单元测试 | 2A-1~2A-5 | ❌ |

### Sprint 2B：field_kind 枚举增强

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 2B-1 | `field_kind` 新增 decision/artifact/progress/constraint 枚举值 | 无 | ❌ |
| 2B-2 | `working_memory.write` 工具增加 field_kind enum 约束 | 2B-1 | ❌ |
| 2B-3 | Agent system prompt 加入推荐字段名列表 | 2B-2 | ❌ |
| 2B-4 | Schema 约束可选启用（利用已有 memory_l1_schemas 表） | 2B-1 | ❌ |
| 2B-5 | 单元测试 | 2B-1~2B-4 | ❌ |

### Sprint 2C：L1 选择性注入

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 2C-1 | `memory_l1_field_history` 降为可选（`L1HistoryEnabled` 默认 false） | 无 | ❌ |
| 2C-2 | `memory_l1_schemas` 降为可选（仅 Agent 配置 `L1DefaultSchemaID` 时激活） | 无 | ❌ |
| 2C-3 | 单元测试 | 2C-1, 2C-2 | ❌ |

---

## 阶段三：Episode 结构化与记忆统一（P1）

### Sprint 3A：结构化 Episode 双路径

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 3A-1 | 新增 `internal/biz/l1_field_extraction.go`（ExtractKeyDecisions / ExtractKeyArtifacts） | 2B-1 | ❌ |
| 3A-2 | Path A 零成本 Episode 生成（L1 归档触发） | 3A-1 | ❌ |
| 3A-3 | Path B LLM 增强路径（简化规则：满足任一条件即触发） | 3A-2 | ❌ |
| 3A-4 | Path B 综合评分公式（P1） | 3A-3 | ❌ |
| 3A-5 | 单元测试 | 3A-1~3A-4 | ❌ |

### Sprint 3B：consolidation_status 统一

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 3B-1 | 统一 `consolidation_status` 为 `"consolidated"`（修改 3 处 SQL） | 无 | ❌ |
| 3B-2 | 删除 `MarkEpisodeConsolidated` 方法 | 3B-1 | ❌ |
| 3B-3 | 删除 `memory_l2_consolidate.go` 全文件 + 移除注册 | 3B-2 | ❌ |
| 3B-4 | 数据迁移脚本 | 3B-1 | ❌ |
| 3B-5 | 删除 `memory_l2_index_meta` 表 | 3B-3 | ❌ |
| 3B-5a | 实现暴力搜索阈值（默认 5000 条，低于阈值线性扫描） | 3B-5 | ❌ |
| 3B-5b | pgvector 增量同步协议（写入时同步生成 embedding） | 3B-5 | ❌ |
| 3B-5c | `memory_facts` 新增 `embedding_version` 字段 + 增量重建逻辑 | 3B-5b | ❌ |
| 3B-6 | `memory_facts` 新增 `source_episode_id` 列 | 无 | ❌ |
| 3B-7 | 统一巩固管道（AutoMemoryWorker 消费 → Extract → L3/L4 → Episode） | 3A-2, 3B-6 | ❌ |
| 3B-8 | 单元测试 | 3B-1~3B-7 | ❌ |

### Sprint 3C：L1 与框架 Memory 职责厘清

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 3C-1 | Phase 1：文档引导（无代码变更） | 无 | ❌ |
| 3C-2 | Phase 2：新建 Agent 默认禁用 framework memory 工具 | 3C-1 | ❌ |
| 3C-3 | Phase 3：前端 Agent 列表增加"记忆工具模式"列 | 3C-2 | ❌ |
| 3C-4 | 单元测试 | 3C-2 | ❌ |

### Sprint 3D：L3 Recall 去重

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 3D-1 | fingerprint 去重（基于内容哈希） | 无 | ❌ |
| 3D-2 | 语义去重（embedding 余弦相似度 > 0.95） | 3B-5b | ❌ |
| 3D-3 | 跨层去重（L3 recall 与 L1 已注入字段去重） | 3D-1 | ❌ |
| 3D-4 | 单元测试 | 3D-1~3D-3 | ❌ |

### Sprint 3E：L4 实体提取增强

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 3E-1 | P0：修复现有实体提取 bug | 无 | ❌ |
| 3E-2 | P1：与 Path B 合并执行（单次 LLM 调用同时提取实体和 Episode） | 3A-3, 3E-1 | ❌ |
| 3E-3 | P2：实体关系推理（待定，后续迭代） | 3E-2 | ⏳ |
| 3E-4 | 单元测试 | 3E-1, 3E-2 | ❌ |

---

## 阶段四：安全与完整性（P2）

### Sprint 4A：摘要完整性保护

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 4A-1 | `session_summaries` 新增 `content_hash` + `trust_source` 列 | 无 | ❌ |
| 4A-1a | Tier 0 验证：确认 TLS 连接强制启用 + DB NOT NULL 约束已就位 | 4A-1 | ❌ |
| 4A-2 | 压缩时计算 SHA-256 校验和 | 4A-1 | ❌ |
| 4A-3 | 注入时验证校验和 | 4A-2 | ❌ |
| 4A-4 | 单元测试 | 4A-1~4A-3 | ❌ |

### Sprint 4B：Episode Fork

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 4B-1 | `memory_episodes` 新增 `fork_from_episode_id` + `fork_from_turn_index` | 无 | ❌ |
| 4B-2 | `session_runtime` 新增 `fork_source` | 无 | ❌ |
| 4B-3 | Fork API（`POST /v1/sessions` with fork 参数） | 4B-1, 4B-2 | ❌ |
| 4B-4 | 单元测试 | 4B-1~4B-3 | ❌ |

### Sprint 4C：摘要保留/丢弃策略

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 4C-1 | 默认压缩保留策略（保留意图/决策/文件/错误/待办） | 无 | ❌ |
| 4C-2 | `CompactSession` API 新增 `preserve_instruction` 参数 | 4C-1 | ❌ |
| 4C-3 | 接入现有 `preserveInstruction` 传输管道 | 4C-2 | ❌ |
| 4C-4 | 前端"保留重点"输入框 | 4C-3 | ❌ |
| 4C-5 | 单元测试 | 4C-1~4C-4 | ❌ |

### Sprint 4D：子目录规则按需加载

| # | 任务 | 依赖 | 状态 |
|---|------|------|------|
| 4D-1 | 规则配置新增 `activate_on_glob` 字段 | 无 | ❌ |
| 4D-2 | 文件路径提取（从工具调用参数中提取当前操作文件路径） | 无 | ❌ |
| 4D-3 | 参数名映射表（工具参数名 → 文件路径） | 4D-2 | ❌ |
| 4D-4 | 按需加载匹配逻辑（glob 匹配 + 仅注入匹配规则） | 4D-1, 4D-3 | ❌ |
| 4D-5 | 单元测试 | 4D-1~4D-4 | ❌ |

---

## 依赖关系图

```
阶段一（P0）：
  1A (限流) ──────────────────────────────────────────┐
  1B (前缀分离+Cache) ── 1D-7 依赖 1A-3 ─────────────┤
  1C (压缩预算) ──────────────────────────────────────┤
  1D (L2 Compact) ── 1D-7 依赖 1A-3, 1B 必须同步 ───┘

阶段二（P1）：
  2A (L1 预算) ── 2A-1 为 2A-2 前置
  2B (field_kind) ── 2B-1 为 3A-1 前置
  2C (L1 选择性注入)

阶段三（P1）：
  3A (Episode 双路径) ── 依赖 2B-1 (field_kind 枚举)
  3B (consolidation 统一) ── 3B-7 依赖 3A-2, 3B-5b 为 3D-2 前置
  3C (Memory 职责厘清)
  3D (L3 Recall 去重) ── 3D-2 依赖 3B-5b (pgvector)
  3E (L4 实体提取增强) ── 3E-2 依赖 3A-3 (Path B)

阶段四（P2）：
  4A (完整性保护)
  4B (Episode Fork) ── 依赖 4A (trust_source)
  4C (保留策略)
  4D (子目录按需加载)
```

---

## 验收标准

### 阶段一验收

| # | 标准 | 验证方式 |
|---|------|---------|
| V1-1 | L0 Snapshot 写入量降幅 >= 90%（中等会话），>= 95%（长编码会话） | 对比限流前后写入次数 |
| V1-2 | `segments_summary_json` 数据量减少 >= 80% | 对比 JSON 大小 |
| V1-3 | Anthropic Prompt Cache 命中率 >= 50%（中等对话） | 日志统计 cache_hit |
| V1-3a | 中等对话每轮输入 token 节省 >= 52%，长对话节省 >= 60% | 对比缓存前后输入 token 数 |
| V1-3b | 非 Anthropic Provider 功能回归测试通过 | 各 Provider 专项测试 |
| V1-3c | 断点计数校验通过（不超过 4 个） | Anthropic 请求日志 |
| V1-4 | 压缩预算动态计算准确（误差 < 10%） | 对比 reserved_system 估算与实际 |
| V1-5 | Level 1 MicroCompact 执行耗时 < 1ms | 基准测试 |
| V1-6 | Level 2 Memory Compact 执行耗时 < 50ms | 基准测试 |
| V1-7 | 90%+ 压缩场景零 LLM 成本 | 统计 Level 1/2 命中率 |

### 阶段二验收

| # | 标准 | 验证方式 |
|---|------|---------|
| V2-1 | `token_estimate` 非零（所有新写入字段） | 数据库查询 |
| V2-2 | `used_tokens` 准确（与 SUM(token_estimate) 一致） | 数据库查询 |
| V2-3 | 预算超限时写入被拒绝 | 测试 ErrL1Overflow |
| V2-4 | L1 注入 token 节省 2K~8K/轮 | 对比注入前后 token 数 |
| V2-5 | field_kind 枚举 LLM 遵循率 >= 90% | 统计工具调用参数 |

### 阶段三验收

| # | 标准 | 验证方式 |
|---|------|---------|
| V3-1 | 70%~90% Episode 零 LLM 成本生成 | 统计 Path A/Path B 比例 |
| V3-2 | `consolidation_status` 统一为 "consolidated" | 数据库查询无 pending/done |
| V3-3 | L2ConsolidateWorker 已删除 | 代码搜索无引用 |
| V3-4 | 新建 Agent 默认禁用 framework memory 工具 | 功能测试 |
| V3-5 | `memory_l2_index_meta` 表已删除 | 数据库查询 |
| V3-6 | 暴力搜索阈值生效（<=5000 条线性扫描，>5000 条 pgvector） | 基准测试 |
| V3-7 | embedding_version 字段正确标记 | 数据库查询 |
| V3-8 | fingerprint 去重生效，无重复事实写入 | 插入相同内容验证跳过 |
| V3-9 | L4 实体提取基础链路正常 | 功能测试 |

### 阶段四验收

| # | 标准 | 验证方式 |
|---|------|---------|
| V4-1 | SHA-256 校验和验证通过 | 注入时无 hash mismatch |
| V4-2 | Fork API 返回正确 injected_context | 功能测试 |
| V4-3 | `preserve_instruction` 参数生效 | 手动压缩测试 |
| V4-4 | `activate_on_glob` 按需加载生效，未匹配规则不注入 | 对比注入前后 token 数 |
| V4-5 | 文件路径提取准确率 >= 95% | 工具调用参数测试 |

---

## 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| DualBreakpoint 与三层前缀分离不同步 | 压缩场景缓存完全失效 | Sprint 1B 强制同步实施 |
| token_estimate 精度不足 | 预算检查误判 | 短期 runeCount/2，中期 tiktoken |
| ICS 评分阈值不合理 | Level 2 过度降级/过度使用 | 可配置阈值 + 日志可观测 |
| Hunyuan 适配器修复引入新问题 | Hunyuan 用户功能异常 | 修复后增加专项测试 |
| L2ConsolidateWorker 删除影响现有功能 | 删除后无替代 | Worker 本就空转，零风险 |
| 自适应缓冲区策略过度调整 | ratio 震荡 | 步进限制 + 冷却期 |
