# 记忆中心召回链路评审与升级终稿（P0-P2）

> 类型：review（评审 + 终稿方案 + 实施计划）
> 日期：2026-08-08
> 范围：五层记忆系统（L0-L4）读路径「快而准」+ 写路径可靠性
> 前置：2026-07-29-review-memory-system-redesign.md（结构性重设计）、本次 embedder 配置落地

---

## 一、本次已完成的配置项（embedder 落地）

| 项 | 状态 | 证据 |
|----|------|------|
| Embedder 选型 | ✅ Ollama 本地 `bge-m3`（1024 维，中文语义质量好） | `system_settings.knowledge_embed_*` |
| 向量维度切换 | ✅ 1536→1024（`configs/config.yaml` `vector_dim` + `vector_embeddings` 列） | DB 探查 |
| pgvector 构建标签 | ✅ 发现 `go run` 未带 `-tags pgvector` 导致向量存储静默 nil，已带标签重启 | 见 P0-2 |
| 存量索引回填 | ✅ 195/197 facts fresh（agent_memory_1024 195 行）+ 39/39 episodes embedded | 启动 reconciler 日志 synced=73+122, failed=0 |
| 前端 limit 解析错误 | ✅ `listConflictingFacts` 参数错位修复 + `memory.proto` 补 `agent_id` | api.ts / memory.proto |

回填过程中新发现两条 P0 级缺陷（P0-1/P0-2），已纳入下方终稿。

---

## 二、五层功能代码级验证结论

| 层 | 数据现状 | 读路径 | 结论 |
|----|---------|--------|------|
| L0 上下文组装 | 按设计低频 | 正常 | ✅ 无需动作 |
| L1 工作记忆 | 22 tasks / 9 fields | 注入链路存在（`L1MemoryCue`），默认开 | 🟡 字段稀疏，P2 排查写入侧 |
| L2 情景记忆 | 39 episodes，全部已嵌入 | 向量+关键词+重要性+会话加权评分正常 | ✅ embedder 修复后恢复 |
| L3 语义事实 | 195 facts fresh | 向量 RRF + FTS + Go 关键词三路融合结构完整；Bug A（Total=0 被 minScore 全杀）已修复 | ⚠️ 中文关键词/FTS 通道弱、minScore 配置两极化 |
| L4 实体图谱 | 5 entities，稀疏 | 注入代码存在但有 0.3 confidence 门控 | ❌ `l0_inject_l4` 存量 55/55 全为 false（默认 false）→ 从未注入 |

---

## 三、P0 修复清单（阶段一）

> **实施状态（2026-08-08）：P0-1 ~ P0-4 全部 ✅ 已实施**，代码/迁移/测试/文档已落地，详见各节「实施记录」与 `70-orchestration-longtask-memory.development.md` 阶段一完成记录。

### P0-1 reconciler 不捞 `pending` 状态事实（索引死信）

- **现象**：122 条事实 `embedding_status='pending'` 滞留，无任何重试路径；本次靠手工 SQL flip 成 stale 才完成回填。
- **根因**：[memory_maintenance_adapter.go](../../internal/data/memory_maintenance_adapter.go) `ListStaleIndexFacts` 只查 `embedding_status IN ('stale','failed')`；而 insert 默认 `'pending'`，只有写入时同步失败才会转 `'stale'`。写入时同步未执行（进程崩溃窗口、canary 插入、历史数据）即永久滞留。
- **修法**：查询条件加入 `'pending'`，保留 `index_attempts` 上限（≥5 次转 disabled）防死循环。
- **验证**：单测覆盖三状态；运行时构造 pending 事实观察 reconciler 一轮内转 fresh。
- **实施记录** ✅：`ListStaleIndexFacts` 查询条件改 `IN ('stale','failed','pending')`；配套修复 `index_attempts` 列断链（`sqlFactSelect` 补列、`scanFactRowJSON` 补解析、`syncEmbeddingBlob` 重置 0、`markFactIndexStale` 自增）；测试 `TestListStaleIndexFacts_IncludesPending` 等覆盖三状态。

### P0-2 pgvector build tag 静默降级

- **现象**：不带 `-tags pgvector` 启动时 `NewPgVectorFactStore` 返回 error → `MemoryRepo=nil` → 所有向量读写返回 `ErrMemoryUnavailable`，仅散落 warn 日志，进程照常运行（本次事故即因此：embedder 已配对但向量写入全灭）。
- **修法**：启动 canary 增加向量存储可用性探针（`vectorStore != nil` 且 `IsPgvector()`），不可用时 **启动日志 Error 级显著告警**（不阻断启动，读写分离降级是设计意图，但必须可观测）。同时在 `README.md` 运行说明中强调 `-tags pgvector`。
- **验证**：不带 tag 启动可见 Error 日志；带 tag 启动 canary 通过。
- **实施记录** ✅：[data.go](../../internal/data/data.go) `ensurePostgresSchemas` 两处降级分支（tag 缺失 / EnsureSchema 失败）Info/Warn 升级为 Error 显著告警；`README.md` 三条启动命令全部补 `-tags pgvector` 并加警告块。

### P0-3 L4 注入默认关闭且存量全关

- **现象**：`agent_runtime_settings.l0_inject_l4` schema 默认 `false`；存量 55 个 memory_enabled agent 全部为 false → L4 实体图谱从未注入 prompt。
- **修法**：schema 默认改 `true`（下游有 0.3 confidence 门控 + maxPaths=8 限制兜底，风险可控）；DDL 迁移将存量 `l4_enabled=true AND l0_inject_l4=false` 的行置 true。
- **验证**：迁移后探查 55/55 为 true；构造含 ≥0.3 confidence 实体会话，prompt 出现 L4 块。
- **实施记录** ✅：三处默认同步翻转——[agent_defaults.go](../../internal/biz/agent_defaults.go) `DefaultAgentRuntimeSettings`、Ent schema `l0_inject_l4.Default(true)`、前端 `agentRuntimeConfig.ts` `inject_l4: true`；迁移 20261130 `memory_recall_defaults_fix` 值守卫 UPDATE（`l4_enabled=TRUE AND l0_inject_l4=FALSE` → true）；测试 `TestDefaultAgentRuntimeSettings_L0InjectL4` + `TestMemoryRecallDefaultsFixMigration`（含幂等重跑）。

### P0-4 L3 minScore 配置两极化

- **现象**：存量分布 `0.55×60 行 / 0.00×239 行`。0.55 按权重分布（keyword 0.25 + vector 0.30 + importance 0.20 + recency 0.15 + quality 0.10）实测相关命中典型 Total≈0.4-0.5 → 被误杀；0.00 则完全不过滤（代码 `minScore>0` 才过滤）。
- **修法**：默认 0.55→0.35；DDL 迁移存量 0.55→0.35；0.00 行不动（用户显式关过滤）。根本解在 P1-3 自适应阈值。
- **验证**：迁移后无 0.55 行；典型中文查询能召回相关事实。
- **实施记录** ✅：三处默认同步翻转（同 P0-3 三文件，`recall_min_score: 0.35`）；同一迁移 20261130 值守卫 UPDATE（`= 0.55` → 0.35，0.00 不动）；测试 `TestDefaultAgentRuntimeSettings_L3RecallMinScore` + 迁移测试。**注意**：金丝雀 `MemoryCanaryMinScore` 与 eval set `min_score` 刻意保留 0.55 严格门（质量哨兵，非生产默认，注释已声明）。

---

## 四、P1 读路径升级清单（阶段二）— 「快而准」

### P1-1 中文 bigram 分词（Go 关键词通道 + 词法 reranker）

- **根因**：`tokenizeQuery`（memory_helpers.go）按空格/标点切分，连续中文成单 token；`keywordOverlapScore` 退化为整句子串匹配，命中率极低。`CrossEncoderReranker` 的 `bigramJaccard` 基于 `strings.Fields`，对中文同样失效。
- **修法**：CJK run 切重叠 bigram（「我想吃火锅」→{我想，想吃，吃火，火锅}），与拉丁 token 混合计分；reranker 的 bigram 同步改为 rune 级。纯 Go 改动，无外部依赖，收益立现。
- **验证**：中文查询关键词分 >0；单测覆盖混合中英文查询。

### P1-2 FTS 中文通道（pg_trgm）

- **背景**：FTS 'simple' 配置对 CJK 不分词（代码注释已声明的已知限制）。
- **修法**：`memory_facts.statement` 建 pg_trgm GIN 索引，中文/短查询走 `similarity()` 通道作为第三路候选注入 RRF 融合（knowledge 模块已有 pg_trgm 先例，37-knowledge.design.md）。
- **验证**：中文 FTS 候选非空；RRF 融合后中文相关事实进入候选池。

### P1-3 自适应 minScore

- **修法**：候选集评分后按分布定阈值：`max(floor=0.25, top1×0.6)`，替代静态值；静态 `l3_recall_min_score` 作为 floor 下限（用户显式配置仍生效为下限）。
- **验证**：高分集不误杀、低分集不泛滥；单测覆盖分布形态。

### P1-4（可选）真实 Cross-Encoder reranker

- Ollama `bge-reranker-v2-m3` HTTP 实现 `biz.Reranker` 接口；当前词法 proxy 作为 fallback。视 P1-1/P1-2 上线后效果决定是否启动。

---

## 五、P2 结构增强清单（阶段三）

| # | 项 | 内容 |
|---|----|------|
| P2-1 | L1 写入侧排查 | 22 tasks 仅 9 fields：定位是模型不调 memory 工具还是工具写入字段不全；让 L1 真正承载「当前任务草稿」 |
| P2-2 | L4 增产 | 实体仅 5 条：检查 L4 graph extraction 触发条件与产量；P0-3 打开注入后需要有货可注 |
| P2-3 | 热度换页 | L2→L3 consolidation 已在；评估冷 L3 降档/热事实置顶（艾宾浩斯 + injected_count 已有信号） |

---

## 六、实施计划与门控

| 阶段 | 内容 | 完成判据 | 门控 |
|------|------|---------|------|
| 一 | P0-1 ~ P0-4 | 全量测试 + build + lint 绿；运行时验证 reconciler 捞 pending、L4 注入出现、中文查询有召回 | review 后进入阶段二 |
| 二 | P1-1 ~ P1-3（P1-4 可选） | 中文关键词/FTS 通道实测有分；自适应阈值生效；测试全绿 | review 后进入阶段三 |
| 三 | P2-1 ~ P2-3 | L1 字段增长链路打通；L4 实体增产；评审报告 | 全方位深入终审 |

**纪律**：每阶段 TDD（先失败测试后实现）；每阶段完成后 review；全部完成后按 `aranea-review` 做记忆中心全方位终审。
