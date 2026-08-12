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
  │  service 层：校验 user_id 非空 → 生成 ingest 批次 ID
  │  ├─ 切块：按 20 条/2000 词切分 messages
  │  ├─ L2：每块沉淀 episode（session_id 分组，保留消息时序与时间戳）
  │  ├─ L3：MemoryConsolidator 提取事实/偏好/规则 → PII 扫描 → 冲突检测 → 写库
  │  └─ 向量：MemoryEmbeddingAdapter（OpenAI 兼容）→ pgvector UpsertFactVector
  │      ‑ Embedding 不可用 → 记 warn 降级，仅关键词索引（契约行为不变）
  └─ 同步返回 {success: true, request_id, timestamp: now}
      ‑ 内部失败可重试部分入异步队列；不可恢复错误返回 5xx（平台重试）

Search(query, user_id, top_k=100, options)
  │  service 层：校验 user_id 非空（空 → 400，不猜测不跨域）
  │  ├─ Embed(query) → L2/L3 混合评分召回（强制 user scope = user_id）
  │  ├─ L4 图谱关联扩展（多跳证据，限 1 跳、封顶条数）
  │  └─ 按 score 排序截断 top_k
  └─ 返回 {data: [{id: 事实/episode ID, content: 记忆原文, score, timestamp: 记忆事件时间}]}
      ‑ 无 LLM 调用、无答案生成（红线 R1）
```

### 5.3 代码落位（T1 实际实施：独立入口，主程序零修改）

| 层 | 文件 | 说明 |
|----|------|------|
| biz | `internal/biz/memory_eval.go`（新增） | `EvalMemoryStore` 窄端口（Stability:internal，2 方法）+ `EvalMessage` / `EvalMemoryItem` |
| data | `internal/data/memory_eval_store.go`（新增） | 委托现有 `l3FactRepo`：`UpsertFactRow`（`(scope_type,scope_id,fingerprint)` 唯一键幂等 + PII gate）+ `RecallL3Facts`（混合评分召回，空 embedding 自动 brute-force 降级） |
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

仓库根 `Dockerfile`：Go 1.23 多阶段构建 → debian-slim；`EXPOSE 8000(HTTP)/9000(gRPC)`；`CMD ["./admin", "-conf", "/data/conf"]`。

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
