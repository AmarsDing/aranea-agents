# Agent Memory Challenge 2026 — 系统/方法说明 + Add/Search API 封装设计

> **需求**：[agent-memory-challenge.md](./agent-memory-challenge.md) · **计划**：[agent-memory-challenge.development.md](./agent-memory-challenge.development.md)
> 本文既是**学术披露材料**（§1–§4 提交平台），也是**适配层实施设计**（§5–§8 指导 T1 开发）。

---

## 1. 系统架构（参评方法说明）

### 1.1 一句话定位

Aranea-Agents Memory：基于开源 Agent 运行时 trpc-agent-go 构建的 **L0–L4 五层 Agent 长期记忆系统**，以"分层存储 + 混合召回 + 主动治理"覆盖记忆的全生命周期。

### 1.2 五层记忆模型

| 层 | 名称 | 存储 | 写入路径 | 检索方式 |
|----|------|------|----------|----------|
| L0 | Sensory 上下文窗口 | 会话装配快照（Ent） | 每轮对话自动装配/压缩 | 直接注入 prompt（不参与评测检索） |
| L1 | Working 工作记忆 | 任务状态板 task/field（Ent） | Agent 执行中读写 | 按键精确读取 |
| L2 | Episodic 情景记忆 | 会话事件时间线（Ent + 向量） | 会话事件自动沉淀 + 巩固队列 | 向量相似 + 时间线遍历 |
| L3 | Semantic 语义记忆 | 事实/偏好/规则（Ent + pgvector） | LLM 提取（MemoryConsolidator）+ 双写向量 | **混合评分召回**（向量 + 全文 + 衰减 + 置信度） |
| L4 | Persistent 图谱与进化 | 实体关系图谱（Ent） | 从 L3 事实聚合 + 进化事件 | 图谱路径遍历 |

评测 Add/Search 契约主要落在 **L2（情景）+ L3（语义）** 两层；L4 图谱为多跳问题（维度 B）提供关联证据。

### 1.3 检索流水线（Search 内部）

```
query ──► Embedding（OpenAI 兼容 API，可降级）──► 向量召回（pgvector 余弦）
   │                                                    │
   ├──► 全文检索（Postgres to_tsvector + GIN + pg_trgm）──► 混合评分融合
   │                                                    │  score = w1·vector + w2·fts
   ├──► 时间衰减因子（全局衰减模型）                      │        + w3·recency + w4·confidence
   │                                                    │
   └──► scope=user（=评测 user_id）强制过滤 ────────────► top_k 截断 ──► 证据条目
```

降级模式：Embedding 端点未配置/不可用时退化为关键词混合召回（brute-force 路径），契约行为不变。

---

## 2. 评测维度 → 系统能力映射

| 维度 | 评测点 | 对应机制 | 代码锚点 |
|------|--------|----------|----------|
| A 显式事实召回 | 事实/属性/实体 | L3 facts 混合评分召回 | `internal/biz/memory.go`、`internal/data/memory_shim_l3.go` |
| B 关系与多跳组合 | 跨片段证据链 | L4 图谱 + 跨层融合召回（CompositeSearch） | `internal/biz/memory_l4.go` |
| C 时间与事件序列 | 日期/顺序/轨迹 | L2 episode 时间线、turn_index 排序 | `internal/data/memory_shim_l2.go` |
| D 记忆治理 | 更新/冲突/删除/遗忘 | 冲突检测器、反驳/确认、版本回滚、Cascade Saga 删除、全局衰减 | `internal/biz/memory_conflict*.go`、`memory_consolidator.go` |
| E 个性化与关怀 | 偏好/背景 | user scope 偏好事实 + 业务化置信度/强化因子 | `internal/biz/memory_consolidator.go` |
| G 规则与流程执行 | 规则记忆执行 | L3 规则类事实（kind=rule）+ L1 工作记忆 | `internal/data/memory_shim_l1.go` |
| H 安全与隐私 | 拒答边界/最小披露 | PII 扫描脱敏、scope 五级隔离、审计日志 | MEM-OPT-04 PII（`internal/biz/memory_admin*.go`） |

---

## 3. 原始工作引用（学术披露 · 强制）

| 引用 | 作者/来源 | 用途 | 许可 |
|------|-----------|------|------|
| trpc-agent-go | Tencent（项目内嵌 `pkg/trpc-agent-go`） | Agent 运行时内核；`memory.Service` 接口定义 | Apache-2.0（以仓库 LICENSE 为准） |
| pgvector | pgvector/pgvector | Postgres 向量相似度检索 | PostgreSQL License |
| mem0 | mem0ai/mem0 | 记忆管理范式参考（提取-巩固-召回）；**未复用其代码** | Apache-2.0 |
| 分层记忆理论 | 认知科学感觉/工作/情景/语义记忆模型（`docs/development/memory/theory.md`） | L0–L4 分层理论基础 | — |
| Kratos v2 | go-kratos | 传输壳层（HTTP/gRPC），非记忆方法本身 | MIT |

## 4. 方法改动与创新点（学术披露 · 强制）

相对 trpc-agent-go 框架扁平 KV 记忆（`memory.Service` + 9 种后端）的全部方法改动：

| # | 改动 | 说明 |
|---|------|------|
| C1 | **L0–L4 五层分层模型** | 框架为扁平 `UserKey→[]Entry`；本项目扩展为五层产品模型，各层独立表结构、读写接口与维护 Job |
| C2 | **混合评分召回** | 未使用框架 BM25+RRF；自建 向量 + Postgres FTS + 时间衰减 + 置信度强化 的多因子评分 |
| C3 | **记忆治理套件** | 冲突检测（新增 vs 已有事实）、反驳/确认工作流、版本历史与回滚、Cascade Saga 级联删除、全局衰减 + 业务化置信度模型（MEM-OPT-02/05/06） |
| C4 | **PII 安全管线** | 写入前 PII 扫描与脱敏标记（MEM-OPT-04），审计留痕 |
| C5 | **提取协议与队列** | 未启用框架 Auto 模式；自建三优先级队列 + Cron Worker + LLM 提取器（EnhancedTextExtractor） |
| C6 | **五级 scope 隔离** | user / agent / team / workspace / global 作用域权限模型，评测中 user_id 直接映射 `scope=user` |
| C7 | **Add/Search 评测适配层** | 本次参赛新增：平台契约 → 内部 L2/L3 读写的协议桥接（§5），不含任何评测数据特化逻辑 |

---

## 5. Add/Search API 封装方案

> ⚠️ 契约字段提取自平台 https://agentmemories.ai/docs （SPA 渲染内容）；**提交前须与官方 API Guide 最新版逐字段核对**，以官方为准。

### 5.1 平台契约（我方须实现）

**Add — 写入记忆**（同步返回，内部可异步）

| 项 | 内容 |
|----|------|
| 方法/路径 | `POST`（适配层路径 `/v1/memory/add`） |
| 请求体 | `request_id`（字符串）、`messages`（消息数组）、`user_id`（字符串，**隔离边界**）、`session_id`（字符串，仅会话分组） |
| 响应体 | `success`（bool）、`request_id`（回显）、`timestamp`；HTTP 200 同步返回 |
| 语义 | 接收记忆并完成内部处理；摄取可内部异步，但响应必须同步确认 |

**Search — 检索证据**（同步）

| 项 | 内容 |
|----|------|
| 方法/路径 | `POST`（适配层路径 `/v1/memory/search`） |
| 请求体 | `query`（字符串）、`user_id`（隔离边界）、`top_k`（默认 100）、`options`（可选） |
| 响应体 | `data` 数组，每项 `id` / `content` / `score` / `timestamp` |
| 语义 | **只返回记忆证据**，不得生成最终答案 |

**公共约束**

| 项 | 内容 |
|----|------|
| 鉴权 | `Authorization: Bearer <Memory System Key>` 或 `X-Api-Key` 头（Key 由我方签发并经申请表提交，不进仓库） |
| 隔离 | user_id 是唯一检索隔离边界；Add 与 Search 必须使用相同值；禁止跨 user_id 检索 |
| 可重试错误码 | 408 / 409 / 425 / 429 / 500 / 502 / 503 / 504（平台按码重试） |
| 消息切块 | 建议 20 条消息或 2000 词为一切块单位 |
| 轮询 | 无；两端点均为同步接口 |

### 5.2 契约 → 内部实现映射

```
Add(request_id, messages[], user_id, session_id)
  │  适配层：校验 user_id 非空
  │  ├─ 每条消息浅分类 kind（preference / identity / constraint / event）
  │  ├─ 同槽位偏好更新 → InvalidateFact（valid_until）后写入新行
  │  ├─ L3 UpsertFactRow（SkipPIIRedact：打标不改检索正文；CreatedAt/ValidFrom=消息时间）
  │  ├─ 同批 sibling 写入 links（Search 一跳）
  │  ├─ L2 episode 按 session 幂等更新（agent_id=user_id）
  │  └─ 向量索引 safego 异步（不阻塞 200）
  └─ 同步返回 {success: true, request_id, timestamp: now, stored}

Search(query, user_id, top_k=100, options)
  │  校验 user_id / query 非空
  │  ├─ Embed(query) 可选；失败降级关键词
  │  ├─ L3 RecallL3Facts：小集合（≤5000）全量打分；大集合无向量走 FTS-OR 主召回
  │  ├─ 会话 hop（top hit 的 source_session_id）+ links 一跳
  │  ├─ L2 RecallL2Episodes（agent_id=user_id）
  │  └─ MMR（λ=0.7）截断 top_k
  └─ 返回 {data: [{id, content, score, timestamp}]}  — 无 LLM、无答案生成
```

### 5.3 代码落位（T1 实际实施：独立入口，主程序零修改）

| 层 | 文件 | 说明 |
|----|------|------|
| biz | `internal/biz/memory_eval.go`（新增） | `EvalMemoryStore` 窄端口（Stability:internal，2 方法）+ `EvalMessage` / `EvalMemoryItem` |
| data | `internal/data/memory_eval_store.go`（新增） | L3 浅分类写入 + 矛盾 supersede + SkipPIIRedact；L2 session episode；Search = L3 全量/FTS-OR + session/links 一跳 + L2 + MMR；embed 异步 |
| cmd | `cmd/memoryeval/main.go`、`handler.go`（新增） | 独立 HTTP 入口；net/http 标准库；Bearer/X-Api-Key 鉴权；`make build` 自动产出 `bin/memoryeval`（`go build -o ./bin/ ./...`） |
| 主程序 | **零修改** | 不动 api proto / cmd/admin wire / internal/service / data 现有文件 |

**鉴权**：handler 层中间件校验 `Authorization: Bearer` / `X-Api-Key`，仅保护评测端点；token 由环境变量 `EVAL_MEMORY_TOKEN` 注入，不进仓库。

---

## 6. 样本隔离与合规设计

| 红线 | 设计保证 |
|------|----------|
| R2 user_id 隔离 | ① 适配层强制校验 user_id；② 所有召回 SQL 带 `scope='user' AND scope_id=:user_id` 谓词；③ 无"全局记忆"兜底路径参与评测检索；④ 单测覆盖跨 user_id 污染用例 |
| R1 Search 不生成答案 | Search 路径无任何 LLM 调用（代码审查 + 单测断言）；返回 content 为库存记忆原文 |
| R3 来源披露 | §3/§4 即披露内容，随仓库发布 |
| R4 不作弊 | 适配层无语料匹配/硬编码分支；评测数据只经 Add 写入；版本 tag 绑定 commit |

## 7. 容量、超时与限流（提交说明口径）

| 项 | 口径 |
|----|------|
| 容量 | 单容器默认配置支撑评测规模（≈150M 字符写入、≈5K 查询）；PG 模式 16 写 / 32 读连接池 |
| Add 延迟 | 同步确认 < 1s（不含 LLM 提取的完整摄取为异步；若平台要求摄取完成后才可检索，以 Smoke 实测调整同步/异步边界） |
| Search 延迟 | P95 < 3s（向量 + FTS 双路召回，PG 模式） |
| 超时建议 | Add 60s / Search 30s |
| 限流 | 适配层默认不限流；Panic/异常按 5xx 返回供平台重试 |

## 8. Docker 部署要求

### 8.1 现状

仓库根 `Dockerfile`：Go 1.26 多阶段构建（与 go.mod 一致）→ debian-slim；`EXPOSE 8000(HTTP)/9000(gRPC)`；`CMD ["./admin", "-conf", "/data/conf"]`。

### 8.2 参赛运行形态（已定论）

**`NewData` 硬编码 Postgres（pgvector），SQLite 仅存在于遗留迁移工具——单容器 SQLite 形态不可行。唯一形态为 docker-compose（app + pgvector）**，见仓库根 `docker-compose.eval.yml`：

| 服务 | 镜像/构建 | 说明 |
|------|-----------|------|
| `db` | `pgvector/pgvector:pg16` | Postgres + pgvector，健康检查后启动 app |
| `memoryeval` | 根 `Dockerfile` 构建（`make build` 自动含 `bin/memoryeval`），`command: ["./memoryeval"]` 覆盖 CMD | 评测适配层服务，暴露 8910 |

### 8.3 环境变量（运行说明必须包含）

| 变量 | 用途 | 必填 |
|------|------|------|
| `EVAL_MEMORY_TOKEN` | 适配层 Bearer Key（Memory System Key） | 是 |
| `EMBEDDING_BASE_URL` / `EMBEDDING_API_KEY` / `EMBEDDING_MODEL` / `EMBEDDING_DIM` | OpenAI 兼容 Embedding 端点 | 否（缺省降级关键词召回） |
| `EVAL_PG_SOURCE` | Postgres DSN（compose 已内置默认值） | 是 |

### 8.4 仓库须补充（T3）

README 参赛章节：构建命令 → 启动命令 → 健康检查 → Add/Search curl 示例 → Smoke 自测脚本用法 → 外部依赖与降级说明。

---

## 9. Cycle 3 质量升级（2026-10-22 立项；**未参评、未发版，披露口径以 §1–§4 的 r2 为准**）

> 触发：Cycle 2 Full（teval_e176051a414de4db）暴露的真实质量缺口（10-20 trgm 搜索风暴、embedding 异步索引无补偿、中文 FTS 薄弱、治理/时序/聚合/门控短板），全案见 `docs/reports/2026-10-22-analysis-amc-full-eval-failure-and-scoring-plan.md` §5。
> 纪律不变：全部修复为**确定性规则/检索结构，零新 LLM 调用**；Add/Search HTTP 契约零变更（19 契约测试原样通过）；user_id 唯一隔离边界；Add 返回 `stored` 仍=消息条数（原子化不改契约形状）。

### 9.1 召回结构（维度 I/A/全体稳定性）

| # | 改动 | 说明 | 锚点 |
|---|------|------|------|
| Q3a | **trgm scope 先行 CTE** | 原 `statement %> $1 ORDER BY word_similarity` 在 38 万行/8.2GB 表上全量候选扫描（32 并发下 10–16min/条，10-20 搜索风暴根因）；改为 `MATERIALIZED` CTE 先按 scope_type/scope_id/user_id/active 限定 ≤2000 行子集再算 `%>`，风暴类查询从分钟级降到毫秒级 | `internal/data/memory_l3_trgm.go` |
| Q3b | **CJK bigram FTS 通道** | PG 内建 `to_tsvector` 不切分中文，中文召回只剩 trgm/向量两单点；新增 `bigram_text()` IMMUTABLE SQL 函数 + 函数式 GIN 索引（`to_tsvector('simple', bigram_text(statement))`；对抗审查 #9：弃用 STORED 生成列——PG≤17 建列即全表重写且启动期阻塞写），`ftsCandidateIDs` 增 CJK 通道与英文通道合并排序，42703/42883（列/函数不存在）优雅降级 | `internal/data/memory_l3_fts.go`、`sql/migrations/20261292_memory_facts_cjk_bigram_fts.sql` |
| Q2 | **embedding 索引补偿 sweeper** | 原 `scheduleEmbed` fire-and-forget，失败事实向量永久缺失（Full 期间 `eval fact index sync failed` ×1047）；`MemoryFactIndexSweeper` 每 30s 拾取 `embedding_status IN (stale,pending,failed)` 且 `index_attempts<8` 行，SQL 侧 `2^index_attempts` 分钟退避封顶 60min，串行批 50、每行 30s ctx，复用 `SyncFactIndexFromRow` 同构路径；对抗审查 #7 补 migration 20261293 部分索引 `idx_memory_facts_embedding_due`（due 行拾取由 38 万行 seq scan 降索引扫） | `internal/data/memory_fact_index_sweeper.go`、`cmd/memoryeval/main.go`、`sql/migrations/20261293_memory_facts_embedding_due_idx.sql` |

### 9.2 评测写路径增强（维度 A/C/D/F1/G/H）

| # | 改动 | 说明 | 锚点 |
|---|------|------|------|
| Q4 | **消息原子化入库** | 多事实长句整句入库稀释目标属性；`SplitEvalStatements` 按 。！？；\n + 英文句点拆原子（≤8 枚/消息，保留 role 前缀；对抗审查 #5 英文句点拆分带缩写词干守卫——Dr./St./etc. 前不砍句），metadata 记 `message_index`/`atom_index`，同批原子互设 links，Add 返回 `stored` 仍=消息条数 | `internal/biz/memory_eval_classify.go`、`internal/data/memory_eval_store.go` |
| Q5 | **中英相对时间解析** | "上周三/明天/in 2 weeks" 以消息时间戳为 anchor 逐原子解析为绝对日期（对抗审查 #3：不再复用首原子结果，同消息各原子的"昨天/前天"各归其位），注解进 `metadata.event_dates`（间隔/排序题证据可用） | `internal/biz/memory_eval_time.go` |
| Q6 | **查询模式** | `AnalyzeEvalQuery`：Chronological（先后/顺序/what order）→ 按事件日期升序直出、跳过 MMR；Range（between/since/until）→ 按日期过滤；Aggregate（总体/一共/总结）→ 门控通过后追加最近 5 条 L2 episodes | `internal/biz/memory_eval_query.go`、`internal/data/memory_eval_store.go` |
| Q7a | **规则归类拓宽** | 条件句式（如果/每当/只要/规定/规则/whenever/if…then/policy；对抗审查 #2 移出裸「若/按照」防若干/按照计划误伤）→ kind=constraint（importance 0.9，evergreen）；假设句（如果…就/if…then/would）经 `IsHypotheticalStatement` 归类保留但绝不 supersede 在册事实 | `internal/biz/fact_kind.go` |
| Q7b | **槽位扩展** | 搬到/搬去/moved to→live；换工作/入职/新工作→job；手机号/电话号码→phone | `internal/biz/fact_kind.go` |
| Q7c | **矛盾近邻失效** | 新事实含纠错 cue（改成/改为/换成/不再是/不喜欢了/其实/不对/错了/不是…而是/actually/correction；对抗审查 #8 移出裸「改」）时，取其 embedding 对全索引 top50 近邻 scope 过滤（#6 抗多租户共享索引饿死；同批守卫仅作用于本向量通道——#1 保证同请求 [旧值, 纠正] 槽位纠错照常失效）cosine≥0.82 即 InvalidateFact（valid_until 置位），旧值写入 metadata.previous_values 留痕；事件时序守卫（#0/#4）保证平台整请求重放既不复活墓碑、不清空审计，也不反噬更新事实 | `internal/data/memory_eval_store.go`（supersedeContradictions）、`internal/data/memory_shim_l3_write.go`（UpsertFactRow CASE 守卫） |
| Q8a | **L2 全量分片** | eval 单批 4000 字符截断→32768（`evalEpisodeMaxChars`+`L1ArchiveEpisodeInsert.MaxMergeChars`），聚合类问题可带全转录 | `internal/biz/memory_admin_store.go` |
| Q9a | **证据门控** | eval Search 接 `AdaptiveRecallMinScore`（top1 自适应下限，空结果返回空数组而非兜底凑数），抑制低相关噪声挤进 top_k | `internal/data/memory_eval_store.go` |
| Q9b | **敏感信息掩码** | 证据正文对身份证（中 8 位）/手机号（中 4 位）/银行卡（中 6 位）确定性掩码；query 明确询问该类别时放行（同 user 范围内合规，"问身份证"豁免 18 位形态不误伤） | `internal/data/memory_eval_store.go`（MaskEvalEvidence） |
| Q9c | **证据溯源前缀** | 证据 content 前缀 `[YYYY-MM-DD · sess-<id>]`，来源回忆题可直接引用 | `internal/data/memory_eval_store.go` |

### 9.3 验证与遗留

- **带外提交态验证**（`git archive a0cdc026c` → `/d/temp/amc-c3-commit-verify` 干净树，避开主树并行会话 WIP）：编译绿 + `cmd/memoryeval` 19 契约绿 + `internal/biz`/`internal/data`（含 PG 集成）套绿；5 项失败经基线（721a28beb）带外复现证明为并行会话存量（TestSkillMerge_FuseAndApply / TestSkillDedup_DetectDuplicateGroups / TestEventDeliveryOutbox_InsertAndListAfter / TestMCPServerRepo_UpdateWritesOnlyAdminColumns / TestEntRuntimeToBizFieldCoverage），与本改动无关。
- **对抗审查**：合并态 diff 经三视角（契约合规 / 正确性并发 / 性能）45 智能体审查 + 每发现三票反驳，14 发现 → **10 项定案**（4 major + 6 minor；4 项驳回），全部修复于 `a0cdc026c`（12 文件 +487/-77），每项定案均有定向绿测试钉死：
  - **#0/#4（major）** UpsertFactRow ON CONFLICT 裸覆写 —— eval 域 CASE 守卫（重放不复活 superseded 行、不清空 previous_values 审计）+ supersede 双通道事件时序守卫（重放旧请求不反噬更新事实）。比较基准取 valid_from（事件时）而非 valid_until（Invalidate 写墙钟，混用会永久锁死真情改回）；事件时严格更新者照常复活。
  - **#1（major）** 同批守卫曾屏蔽槽位通道 —— 收窄至仅向量近邻通道，同请求 [旧值, 纠正] 恢复失效。
  - **#3（major）** event_dates 曾全原子复用 atoms[0] 解析结果 —— 逐原子 anchor 解析。
  - **#9（major）** 20261292 STORED 生成列（PG≤17 全表重写 + 启动期阻塞写）—— 改函数式 GIN，免列免重写。
  - **#2（minor）** constraint cue 裸 若/按照 过宽 + 假设句可 supersede 活事实 —— cue 移出，`IsHypotheticalStatement` 永久否绝 supersede（仍归类 constraint 供 G 维度检索）。
  - **#5（minor）** "I met Dr. Lee yesterday." 被英文句点规则砍半 —— 缩写词干守卫。
  - **#6（minor→perf）** 向量纠错 topK=5 在多租户共享索引下饿死 —— top50 后 scope 过滤。
  - **#7（perf）** sweeper due 行拾取 38 万行 seq scan —— migration 20261293 部分索引。
  - **#8（minor）** 纠错 cue 裸「改」误伤 改变/改善/修改 —— 白名单收紧（我修改了/我改变了主意的「改了/变成」式真情纠错仍在册）。
- **已落 dev（2026-10-22，merge+FF 至 ee29fca85）**：A11 属主会话提交 20261291（e7cdbb23a）解锁后，merge dev 做 `ddl_migration_table_late.go` 版本序联合（291<292<293）→ FF；合并态 build + Migration 全局版本唯一守卫 + 25 条 PG 红线测绿；**未推远端**。原「previous_values 重放清空」遗留已由 #0/#4 消除——无其他未决项。

## 10. 文本评分增量设计与 ADR-AMC-EVIDENCE-20261022

**背景**：C3 的确定性升级仍存在英文连续分句偏移、短样例丢弃、选项未检索、墙钟影响历史查询、宽槽位误失效和跨层评分基数不同等问题。相似度不等同于矛盾，检索分数中的重要度也不等同于相关证据。

**决策**：在既有评测端口内实现下列策略，保留外部 JSON 契约及现有生产记忆模块边界，不新增模型、数据库表或迁移。

1. `EvalMemoryStore.SearchMemories` 以可选字符串参数传递全部选项；原问题独立召回，另以问题与选项的并集召回。模式、时间和隐私策略只分析原问题；选项不得成为输出正文。
2. 分句保留非空短片段，英文分隔不额外推进字节游标。写入以来源会话、消息标识、时间与原子位置构造稳定 occurrence 指纹，重放先识别既有 occurrence；不同时间的同文事实保留各自来源。
3. 同 user_id 的 Add 使用 PostgreSQL 事务级 advisory lock 串行提交，事实、失效、links、L2 同事务。向量索引仍在提交后补偿；同步词法检索保证立即可用。
4. 纠错要求主体、槽位和显式语义相容；向量只负责发现候选，不能单独使事实失效。假设及不同说话人的自述不能互相覆盖。历史查询允许读取带失效标注的原始旧值，普通现状查询排除失效项。
5. 来源时间锚定相对查询；日期解析区分点与周/月区间，绝对日期校验日历合法性。多日期证据与范围按区间相交判断。先按相关性选取有界候选，再按用户要求排列事件时间与来源顺序。
6. 召回门控使用词法/向量相关性，排除只有重要度、质量或新鲜度的噪声；规则请求增加用户范围内的规则候选。长历史聚合按主题及历史覆盖取证，L2 按相关窗口摘录，避免整段长转录挤占上下文。
7. 保留电话/身份证/银行卡掩码，补充国际电话、邮箱、街道住址、SSN 与凭据脱敏；英文类别按词边界匹配，整段脱敏后再摘录，避免剪掉标签后泄露值。所有辅助读取和证据扩展沿用 user_id 隔离。

**后果**：旧库存仍可读，新写入保留更完整的 occurrence；历史证据与当前状态可明确区分。额外候选读取和选项召回均有数量上限，正式分数与并发容量必须在独立新版本部署后另行验证。

**替代方案**：不采用 Search 生成答案、特定题库匹配或单凭余弦失效；不通过扩大整段转录和无限增加 top_k 解决证据覆盖。

### 10.1 第二轮：历史区间与来源证据单元

同主体同槽位使用确定性治理键存入既有 metadata，候选优先匹配该键并兼容旧库存；写入修补前驱结束时间与后继边界。明确过去状态作为历史证据保存，不参与当前状态覆盖。

日期解析补齐粒度与明确日期标题继承，来源时间和摄取时间分别处理。时间范围在用户范围内下推候选筛选；L2 会话创建日期不用于代替各行事件时间。

规则/归纳与聚合模式以源消息为证据单元，拼接的内容只来自原始库存原子，保留顺序及来源；有界分层覆盖减少单会话和重复转录占用。摘录使用有效词的窗口评分和 rune 位置；隐私策略在摘录前执行。

源消息分组键包含来源时间，复用 message_id 的不同时间不合并。单字母函数调用保留函数名锚点（含空格写法），仅据此召回原始示例。完整历史总结包含过去状态；泛化当前概况继续遵守当前状态过滤。

规范化 UTC 时间用于比较，来源原始偏移用于相对日历解析和查询锚定；无时间戳时只解析不随锚点变化的绝对日期。正文预算 36000 rune，规则证据卡最多 6000，普通条目最多 1800，预算分配为后续来源留出空间。

### 10.2 Full 增量：生命周期与证据关联

记忆生命周期由 Add 的明确用户指令驱动，删除、抑制、恢复与普通事实写入共享用户事务锁及事务。逻辑删除覆盖同用户的当前及历史事实；L2 从剩余有效事实重建，防止转录回流。持续抑制以既有 memory_facts 内的独立控制来源保存，仅存不可直接回显的选择器摘要和元数据；Search 与索引写入不将其作为证据。引文、假设、第三方消息只作为内容。

结构化输入保留原始单元；已通过相关性门控的种子可以通过存储实体名扩展有界中间证据，附带来源，仍执行用户、有效期、删除及规则范围约束。个人背景触发线索只用于召回，不能生成或预置答案。工作流和格式按源范围归类，历史有效期使用实际时间边界。

控制选择器的属性模式仅识别属性名词短语，其他目标使用规范化字面摘要匹配，避免把“某次电话”扩大为所有号码。控制要求独立、单条、明确的用户消息；多行文档和多句混合内容不执行。恢复仅允许后续新事实，不恢复已删 occurrence。未知来源时间继续保持未知，不伪造回溯截止日期。

实体关联最多两跳，每跳查询最多 64 行，最多 8 个种子行/12 个字面实体；不进行生成式关系抽取。支持行标记父来源 `linked_source`；top_k 内需要容纳其父证据链，无法完整容纳的分支被省略。结构化原文不按属性槽位覆盖；有范围的格式指令按说话人、显式范围、指令属性治理，并在 L3 和 L2 证据读取时过滤不适用范围。

历史状态区间使用 `[from,until)`，查询日转换为来源日历的 `[当日零点,次日零点)`；日期事件仍按明确事件日/区间匹配。SQL 候选与最终过滤保持相同区间规则。综合来源覆盖后再补尚未覆盖的证据类型，继续受 top_k、候选数量与 36000 rune 正文预算约束。

### 10.3 ADR-AMC-P3-INTEGRATION-20261023：派生证据与既有治理契约

**背景**：P3 分支引入向量意图路由、档案摘要、槽位历史与数值摘要；现有 dev 已有跨进程事务锁、生命周期、来源日历和证据父链。直接采用任一整份冲突文件会丢失另一侧行为。

**决策**：保留既有事务写链，P3 功能拆入 `internal/data/memory_eval_derived.go`，纯领域实现保留在 `memory_eval_{intent,rollup,numdigest}.go`。向量路由对确定性模式作增量补充；显式更正的语义候选仍需主体/属性相容和时间守卫，语义相似不能授权删除。明确遗忘继续走独立用户控制通道，文档和第三方内容不执行。

派生行使用独立 `source_kind=agent_memory_challenge_derived` 与生成指纹，`source_timestamp` 明确为空，不进入来源时间锚和 L2 转录。原子事实、失效及派生写入同事务；数据库错误向上传递并回滚。profile 只读取有效的事实性内容，slot history 对已失效源值明确标注；无日期数值保留源文但不生成虚构日期的 series digest。数值更新查找使用字面转义后的系列键，并保留过期回放守卫。

生命周期删除先处理源事实，再淘汰该用户全部派生代际，数值统计从未删除源事实重建，档案和槽位历史在同次 Add 重建。不同系列的有效观察不因删除另一系列而丢失。派生证据只在对应查询模式且无日期窗口时参加召回，失效派生行不作历史源事实返回；实体关联 SQL 和种子排除派生行，防止摘要把无关实体拼成桥接。总结分配预算时先保留来源覆盖，余量再放派生条目。

**后果**：保留离线兼容与生命周期一致性，Add 增加有界派生重建与可选语义嵌入成本。当前重建扫描最多 2048 行、派生候选最多 64 行；超长历史完整性、真实嵌入路由及 Full 并发容量仍需独立验收。源码合并不等于比赛分数提升或线上部署完成。

**替代方案**：不采用纯向量路由关闭离线模式、不恢复分支中的非事务写法、不将派生摘要当作关系边或用处理时间填充未知来源日期。
