# Memory 系统优化 — 需求

> **设计**：[`memory-optimization.design.md`](./memory-optimization.design.md) · **开发计划**：[`memory-optimization.development.md`](./memory-optimization.development.md)
> **调研来源**：`docs/reports/2026-06-08-solution-memory-optimization.md` + 竞品调研（Claude Code / Cursor / Codex CLI / Trae）+ 学术论文（Focus / Memento / A-Mem / Mem0 / LLMLingua-3）

---

## 0. 指导思想

Memory 系统优化的核心矛盾：**上下文窗口是硬约束，但信息消耗是线性增长的**。当前系统已有 L0 压缩基础和 L1-L4 记忆分层，但存在五个结构性缺陷：

1. **L0 写入风暴**：Snapshot 无限流，长会话产生 80~200+ 行写入；`segments_json` 过大
2. **缓存完全失效**：动态内容与静态内容混在一个 TextBlock 中，Prompt Cache 每轮都 miss
3. **L1 预算形同虚设**：`token_estimate` 始终为 0，`used_tokens` 永远为 0，选择性注入退化为全量注入
4. **Episode 生成全靠 LLM**：70%~90% 的 Episode 可通过结构化路径零成本生成
5. **技术债累积**：`consolidation_status` 三值不一致、L2ConsolidateWorker 空转、HMAC ROI 偏低

本需求文档分四个阶段，每阶段独立可交付，渐进升级。

---

## 1. 阶段一：L0 压缩与缓存优化（P0）

> 对标 Claude Code 三层前缀分离 + Prompt Cache 机制，解决写入风暴和缓存失效。

### 1.1 L0 Snapshot 限流

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-1.1-01 | 新增独立 `L0SnapshotEnabled` 字段 | 必须 | 快照写入不再依赖 `EvolutionMetricsEnabled` |
| FR-1.1-02 | `segments_json` 精简为 `segments_summary_json` | 必须 | 仅记录各段聚合统计，数据量减少 80%+ |
| FR-1.1-03 | 前端 `MemorySnapshotDrawer` 展示聚合统计 | 必须 | 替代逐条详情展示 |
| FR-1.1-04 | 写入限流——最小写入间隔（默认 300 秒） | 必须 | session 级变量 `lastL0SnapshotWriteAt`；usedRatio < 0.60 时跳过写入（低负载无需快照） |
| FR-1.1-05 | 写入限流——`usedRatio` 变化量阈值（默认 0.10） | 必须 | session 级变量 `lastL0SnapshotRatio` |
| FR-1.1-06 | 阈值穿越强制写入 | 必须 | `usedRatio` 跨越 0.80 时强制写入 |
| FR-1.1-07 | `always/force` 模式不受限流 | 必须 | 调试场景保留 |
| FR-1.1-08 | 清理未实现的 Datadog 指标规划 | 必须 | 死规划清理，从设计文档中删除 |

**非功能需求**：
- NFR-1.1-01：写入量降幅：中等会话 ~90%、长编码会话 ~95%
- NFR-1.1-02：`segments_summary_json` 数据量减少 80%+

### 1.2 摘要注入模式说明

框架层提供两种摘要注入模式：`SessionSummaryInjectionSystem`（默认，摘要作为 system message 注入）和 `SessionSummaryInjectionUser`（摘要作为 user message 注入，适用于超长对话滑动窗口场景）。当前应用层仅使用 System 模式，User 模式为框架层独立功能，无需改动。若未来出现超长对话场景（数百轮），可按需启用 User 模式。

### 1.3 三层前缀分离 + Prompt Cache

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-1.3-01 | system TextBlock 按三层排列（静态/半静态/动态） | 必须 | Layer 1 静态 8K~15K、Layer 2 半静态 2K~8K、Layer 3 动态 |
| FR-1.3-02 | `TimeRequestProcessor` 时间信息写入 `ContentParts` | 必须 | 分离时间信息，保证静态前缀字节级一致 |
| FR-1.3-03 | Anthropic adapter 支持先 Content 后 ContentParts | 必须 | 静态内容在前，动态内容在后 |
| FR-1.3-04 | DualBreakpoint 缓存策略 | 必须 | 静态层末尾 + 半静态层末尾各一个断点 |
| FR-1.3-05 | 断点优先级淘汰（P1 system 静态 > P2 tools > P3 system 半静态 > P4 messages） | 必须 | 4 断点上限约束 |
| FR-1.3-06 | Hook Layer 声明机制 | 必须 | 注册时声明 `SystemLayer`（Static/SemiStatic/Dynamic），装配器按 Layer 排序 |
| FR-1.3-07 | `WithSystemCacheStrategy` / `WithCacheSystemPromptDualBreakpoint` Option | 必须 | 框架层新增缓存策略选项 |
| FR-1.3-08 | 修复 Hunyuan 适配器 ContentParts 处理 | 必须 | ContentParts 非空时将 Content 作为第一个 ContentPart 追加 |
| FR-1.3-09 | 压缩重建后 MemoryInject 选择性重执行 | 必须 | 确保压缩后注入的记忆内容最新 |
| FR-1.3-10 | RuntimeCue 拆分为 staticRuntimeCue + dynamicRuntimeCue | 必须 | 静态部分入 Layer 1，动态部分入 Layer 2 |

**非功能需求**：
- NFR-1.3-01：中等对话每轮输入 token 节省 ~52%，长对话节省 ~60%
- NFR-1.3-02：仅修改 Anthropic adapter 的 cache 逻辑和 Hunyuan 适配器的 ContentParts 处理，不影响其他 Provider
- NFR-1.3-03：最多 4 个缓存断点，当前方案刚好用满

### 1.4 压缩预算动态计算

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-1.4-01 | 动态计算 `reserved_system` | 必须 | 基于 prompt_snapshot section 字段，替代硬编码 |
| FR-1.4-02 | 三级触发阈值（soft/hard/emergency） | 必须 | 基于 `effective_budget` 而非 `contextWindow` |
| FR-1.4-03 | `compression_buffer_ratio` 默认 0.15（范围 0.10~0.25） | 必须 | [H.3 修正] 从 0.12 提高到 0.15 |
| FR-1.4-04 | 自适应缓冲区策略 | 推荐 | 监控 token 增量自动调整 ratio |
| FR-1.4-05 | 对话模式检测（编码/聊天） | 推荐 | tool_call_count/turn_count 判断模式 |
| FR-1.4-06 | `reserved_system` 冷启动 fallback | 必须 | 基于 Agent ToolsProfile 分级默认值 |

**非功能需求**：
- NFR-1.4-01：0.15 缓冲区（~19.2K tokens）覆盖 4~9 轮编码场景增量
- NFR-1.4-02：hard_trigger 时 UI 显示"正在优化上下文..."

### 1.5 Level 2 Memory Compact 增强

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-1.5-01 | 激活 Level 1 MicroCompact（死代码→生产） | 必须 | BeforeModel hook 接入，补充清除逻辑 |
| FR-1.5-02 | 激活 Level 2 Memory Compact Step 1 | 必须 | 接入 runCompress，使用已有 MemoryFactReader |
| FR-1.5-03 | Level 2 Step 2：扩展 L1 数据源 | 必须 | Compressor 新增 `l1Reader`，合并 L1+L3 数据 |
| FR-1.5-04 | ICS（信息覆盖度）评估 | 必须 | 6 维分级评分，ICS >= 0.70 且压缩比 <= 60% 时使用 Level 2 |
| FR-1.5-05 | Level 2 失败后等待 hard_trigger | 必须 | 不立即升级到 Level 3 |
| FR-1.5-06 | 压缩后强制写入 L0 Snapshot | 必须 | 不受限流约束 |
| FR-1.5-07 | 前端上下文指示器 | 必须 | 正常/优化中/已优化/正在优化 四种状态；API: GET /api/v1/sessions/{id}/compress-status |
| FR-1.5-08 | 事务安全增强 | 必须 | CAS-事务间隙幂等重入 + 补偿机制 |
| FR-1.5-09 | 压缩进行中标记 + 超时保护 | 必须 | compressing 标记防重复触发，8 分钟超时自动释放 |

**非功能需求**：
- NFR-1.5-01：90%+ 压缩场景零 LLM 成本（Level 1 + Level 2）
- NFR-1.5-02：Level 1 < 1ms，Level 2 5~50ms，Level 3 2~10 秒

---

## 2. 阶段二：L1 预算与选择性注入（P1）

> 解决 L1 预算形同虚设的问题，实现选择性注入和预算硬上限。

### 2.1 L1 预算硬上限

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-2.1-01 | `UpsertL1Field` 时计算 `token_estimate` | 必须 | max(1, runeCount(value_text)/2)（短期改进，原 len()/4 对中文偏差 40-60%） |
| FR-2.1-02 | `used_tokens` 同步聚合 + DB 事务行锁 | 必须 | [H.2 修正] 同一事务内完成字段写入 + 聚合 + 预算检查 |
| FR-2.1-03 | 事务内预算检查，超预算回滚 | 必须 | 返回 `ErrL1Overflow` |
| FR-2.1-04 | 三层过滤链（visibility → pin_to_prompt → 相关性 → 预算） | 必须 | 字段数 > 5 时按 updated_at 降序取 top-K |
| FR-2.1-05 | token 预算硬上限（budget_tokens 的 50%） | 必须 | 超出时按 token_estimate 降序截断 |

**非功能需求**：
- NFR-2.1-01：每轮节省 2K~8K tokens
- NFR-2.1-02：预算检查在写入路径上原子完成，无窗口期

### 2.2 field_kind 枚举增强

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-2.2-01 | `field_kind` 新增 decision/artifact/progress/constraint 枚举值 | 必须 | [H.4] 语义化字段分类 |
| FR-2.2-02 | `working_memory.write` 工具增加 field_kind enum 约束 | 必须 | [H.4] LLM 遵循率 > 90% |
| FR-2.2-03 | Agent system prompt 加入推荐字段名列表 | 推荐 | [H.4 第二层] Prompt 约定 |
| FR-2.2-04 | Schema 约束可选启用 | 推荐 | [H.4 第三层] 利用已有 memory_l1_schemas 表 |

### 2.3 L1 选择性注入

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-2.3-01 | `memory_l1_field_history` 降为可选 | 必须 | `L1HistoryEnabled` 默认 false |
| FR-2.3-02 | `memory_l1_schemas` 降为可选 | 必须 | 仅 Agent 配置 `L1DefaultSchemaID` 时激活 |
| FR-2.3-03 | Token 估算精度改进 | 推荐 | 短期 runeCount/2，中期 tiktoken，长期 API usage 校准 |

---

## 3. 阶段三：Episode 结构化与记忆统一（P1）

> 解决 Episode 生成全靠 LLM 的问题，统一 consolidation 管道。

### 3.1 结构化 Episode 双路径

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-3.1-01 | Path A 零成本 Episode 生成 | 必须 | L1 任务归档时触发，数据源为 L1 快照，episode_kind = "l1_archive_structured" |
| FR-3.1-02 | Path A key_decisions 分层 fallback（4 层） | 必须 | Layer 0: field_kind="decision" → Layer 1: 模式匹配 → Layer 2: pin_to_prompt=true 且 visibility="prompt" 的前 3 个 → Layer 3: 全量兜底（最近 5 个 visibility="prompt" 字段） |
| FR-3.1-03 | Path A key_artifacts 分层 fallback | 必须 | Layer 0: field_kind="artifact"/"reference" → Layer 1: field_path 模式匹配 + field_kind="reference" → Layer 2: field_kind="reference" |
| FR-3.1-04 | Path B LLM 增强路径 | 必须 | 满足任一条件即触发（importance/critic/tool_count/duration/user_mark） |
| FR-3.1-05 | Path B 综合评分公式 | 推荐 | critic_score 存在时 5 维加权，缺失时权重重分配 |

**非功能需求**：
- NFR-3.1-01：70%~90% Episode 生成零 LLM 成本
- NFR-3.1-02：field_kind 枚举 LLM 遵循率 > 90%

### 3.2 Embedding 单写 + 按需索引

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-3.2-01 | 删除 `memory_l2_index_meta` 表 | 必须 | 索引元数据不再独立维护 |
| FR-3.2-02 | 暴力搜索阈值（默认 5000 条） | 必须 | 低于阈值时线性扫描，高于阈值时触发 pgvector 查询 |
| FR-3.2-03 | pgvector 增量同步协议 | 必须 | 写入时同步生成 embedding，无需后台索引任务 |
| FR-3.2-04 | `embedding_version` 字段 | 必须 | 标记 embedding 模型版本，支持增量重建 |

### 3.3 consolidation_status 统一

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-3.3-01 | 统一 `consolidation_status` 为 `"consolidated"` | 必须 | 修改 3 处 SQL 硬编码 |
| FR-3.3-02 | 删除 `MarkEpisodeConsolidated` 方法 | 必须 | 仅 L2ConsolidateWorker 使用 |
| FR-3.3-03 | 删除 L2ConsolidateWorker 全文件 | 必须 | 因 agentID="" bug 完全空转 |
| FR-3.3-04 | 数据迁移 | 必须 | `UPDATE ... SET consolidation_status = 'consolidated' WHERE ... IN ('pending', 'done')` |
| FR-3.3-05 | 统一巩固管道 | 推荐 | AutoMemoryWorker 消费消息 → Extract → 写入 L3/L4 → 生成 Episode |
| FR-3.3-06 | `memory_facts` 新增 `source_episode_id` | 推荐 | Episode-Fact 关联 |

### 3.4 L1 与框架 Memory 职责厘清

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-3.4-01 | 复用 `ToolsDenyJSON` 实现记忆工具互斥 | 必须 | 不新增 `memory_tool_mode` 字段 |
| FR-3.4-02 | 渐进迁移（Phase 1 文档 → Phase 2 默认 → Phase 3 提示 → Phase 4 自动） | 必须 | 向后兼容 |

### 3.5 L3 Recall 去重

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-3.5-01 | fingerprint 去重 | 必须 | 基于内容哈希去重，防止重复写入相同事实 |
| FR-3.5-02 | 语义去重 | 推荐 | embedding 余弦相似度 > 0.95 时判定为重复 |
| FR-3.5-03 | 跨层去重 | 推荐 | L3 recall 与 L1 已注入字段去重，避免信息冗余 |

### 3.6 L4 实体提取增强

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-3.6-01 | P0：修复现有实体提取 bug | 必须 | 确保基础提取链路正常 |
| FR-3.6-02 | P1：与 Path B 合并执行 | 推荐 | LLM 增强路径同时提取实体和 Episode，减少 LLM 调用 |
| FR-3.6-03 | P2：实体关系推理 | 可选 | 基于提取的实体构建关系图 |

---

## 4. 阶段四：安全与完整性（P2）

> 解决多代理上下文完整性保护和 Episode 分叉需求。

### 4.1 摘要完整性保护

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-4.1-01 | Tier 0：TLS + 数据库约束（默认） | 必须 | 传输层加密 + 数据库 NOT NULL 约束，零额外代码 |
| FR-4.1-02 | Tier 1：SHA-256 校验和（单租户推荐） | 必须 | [H.5] content_hash = SHA-256(summary + salt)，~40 行代码 |
| FR-4.1-03 | `trust_source` 标记 | 必须 | 'self' / 'episode_fork' / 'agent_delegation' |
| FR-4.1-04 | Tier 2：HMAC-SHA256（多租户升级） | 可选 | 按租户密钥隔离，~150 行代码 |
| FR-4.1-05 | Tier 3：AES-256-GCM 加密（高安全场景） | 可选 | 端到端加密 |

### 4.2 Episode Fork

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-4.2-01 | `memory_episodes` 新增 `fork_from_episode_id` + `fork_from_turn_index` | 必须 | 分叉源标记 |
| FR-4.2-02 | Fork API | 必须 | `POST /v1/sessions` with fork 参数 |
| FR-4.2-03 | `session_runtime` 新增 `fork_source` | 必须 | 区分 Agent 委派和 Episode 分叉 |

### 4.3 摘要保留/丢弃策略

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-4.3-01 | 默认压缩保留策略 | 必须 | 保留意图/决策/文件/错误/待办，丢弃冗长输出/重复/调试 |
| FR-4.3-02 | 用户指定保留策略 | 推荐 | `preserve_instruction` 参数，接入现有管道 |
| FR-4.3-03 | 前端"保留重点"输入框 | 推荐 | 手动压缩时可选 |

### 4.4 子目录规则按需加载

| # | 需求 | 必要性 | 说明 |
|---|------|--------|------|
| FR-4.4-01 | `activate_on_glob` 字段 | 必须 | 规则配置新增 glob 模式，匹配时才激活 |
| FR-4.4-02 | 文件路径提取 | 必须 | 从工具调用参数中提取当前操作文件路径 |
| FR-4.4-03 | 参数名映射表 | 必须 | 将工具参数名映射到文件路径（如 file_path → 实际路径） |
| FR-4.4-04 | 按需加载评估 | 推荐 | 每轮节省 2K~5K tokens（未匹配的子目录规则不注入） |

---

## 5. 跨章节交互风险

| 交互 | 风险 | 必须措施 |
|------|------|---------|
| §1.3 + §1.5 | 压缩后 summary 改变 TextBlock 顺序，缓存断点失效 | DualBreakpoint 必须与三层前缀分离同步实施 |
| §1.3 RuntimeCue | RuntimeCue 包含动态内容（工具策略），Layer 2 缓存命中率低于预期 | 拆分为 staticRuntimeCue + dynamicRuntimeCue |
| §3.2 + §4.1 | ExtractKeyDecisions 读取全量字段，但选择性注入可能过滤掉部分字段 | 非 bug，需文档说明 Episode 可包含 prompt 中不可见的字段 |
| §4.1 + §4.2 | 跨 session 密钥不可用，fork 注入内容被丢弃 | 增加 trust_source 标记 |
| §3.1 + §3.3 | consolidation_status 三值不一致 | 统一为 consolidated |
| §1.4 + §1.5 | Level 2 失败后升级策略 | soft_trigger 失败后等待 hard_trigger |
| §1.1 + §1.5 | 压缩后 L0 Snapshot 与实际状态不一致 | 压缩后强制写入 L0 Snapshot |
