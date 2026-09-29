# Agent Memory Challenge 2026 参评说明

> Aranea-Agents 参加 [Agent Memory Leaderboard](https://agentmemories.ai/competition/) Cycle 2 学术/开源方法榜（Open-source Methods）。
> 本文是平台要求的**方法披露**与**部署说明**，随仓库公开发布。

| 项 | 内容 |
|----|------|
| 参评类别 | Open-source Methods |
| 系统名 | Aranea-Agents Memory |
| 版本 | `amc-2026.09`（git tag `amc-2026.09`） |
| 参评实现 | `cmd/memoryeval/`（独立入口，**主程序零修改**） |
| 适配层端口 | `8910` |

---

## 1. 系统架构

Aranea-Agents Memory 是基于开源 Agent 运行时 trpc-agent-go 构建的 **L0–L4 五层 Agent 长期记忆系统**，以"分层存储 + 混合召回 + 主动治理"覆盖记忆全生命周期。五层模型详见 [05 五层记忆系统](./manual/05-memory.md)。

| 层 | 名称 | 存储 | 检索方式 |
|----|------|------|----------|
| L0 | 会话上下文窗口 | 会话装配快照 | 直接注入 prompt（不参与评测检索） |
| L1 | 工作记忆 | 任务状态板 | 按键精确读取 |
| L2 | 情景记忆 | 会话事件时间线 + 向量 | 向量相似 + 时间线遍历 |
| L3 | 语义记忆 | 事实/偏好/规则 + pgvector | 混合评分召回 |
| L4 | 持久图谱 | 实体关系图谱 | 图谱路径遍历 |

评测 Add/Search 契约主要落在 **L2（情景）+ L3（语义）** 两层；L4 图谱为多跳问题提供关联证据。

### 检索流水线（Search 内部）

```
query ──► Embedding（OpenAI 兼容 API，可降级）──► 向量召回（pgvector 余弦）
   │                                                    │
   ├──► 全文检索（Postgres to_tsvector + GIN + pg_trgm）──► 混合评分融合
   │                                                    │  score = w1·vector + w2·fts
   ├──► 时间衰减因子                                      │        + w3·recency + w4·confidence
   │                                                    │
   └──► scope=user 强制过滤 ────────────────────────────► top_k 截断 ──► 证据条目
```

降级模式：Embedding 端点未配置/不可用时退化为关键词混合召回，契约行为不变。

### 评测维度 → 系统能力映射

| 维度 | 评测点 | 对应机制 |
|------|--------|----------|
| A 显式事实召回 | 事实/属性/实体 | L3 facts 混合评分召回 |
| B 关系与多跳组合 | 跨片段证据链 | L4 图谱 + 跨层融合召回 |
| C 时间与事件序列 | 日期/顺序/轨迹 | L2 episode 时间线、turn_index 排序 |
| D 记忆治理 | 更新/冲突/删除/遗忘 | 冲突检测、反驳/确认、版本回滚、级联删除、全局衰减 |
| E 个性化与关怀 | 偏好/背景 | user scope 偏好事实 + 置信度/强化因子 |
| G 规则与流程执行 | 规则记忆执行 | L3 规则类事实 + L1 工作记忆 |
| H 安全与隐私 | 拒答边界/最小披露 | PII 扫描脱敏、scope 五级隔离、审计日志 |

---

## 2. 原始工作引用（披露）

| 引用 | 作者/来源 | 用途 | 许可 |
|------|-----------|------|------|
| trpc-agent-go | Tencent（项目内嵌 `pkg/trpc-agent-go`） | Agent 运行时内核；`memory.Service` 接口定义 | Apache-2.0（以仓库 LICENSE 为准） |
| pgvector | pgvector/pgvector | Postgres 向量相似度检索 | PostgreSQL License |
| mem0 | mem0ai/mem0 | 记忆管理范式参考（提取-巩固-召回）；**未复用其代码** | Apache-2.0 |
| 分层记忆理论 | 认知科学感觉/工作/情景/语义记忆模型 | L0–L4 分层理论基础 | — |
| Kratos v2 | go-kratos | 传输壳层（HTTP/gRPC），非记忆方法本身 | MIT |

---

## 3. 相对 trpc-agent-go 的方法改动与创新点

框架原生记忆为扁平 KV（`memory.Service` + `UserKey→[]Entry`）。本项目的全部改动如下：

| # | 改动 | 说明 |
|---|------|------|
| C1 | **L0–L4 五层分层模型** | 各层独立表结构、读写接口与维护 Job |
| C2 | **混合评分召回** | 未使用框架 BM25+RRF；自建 向量 + Postgres FTS + 时间衰减 + 置信度强化 的多因子评分 |
| C3 | **记忆治理套件** | 冲突检测、反驳/确认工作流、版本历史与回滚、级联删除、全局衰减 + 业务化置信度模型 |
| C4 | **PII 安全管线** | 写入前 PII 扫描与脱敏标记，审计留痕 |
| C5 | **提取协议与队列** | 未启用框架 Auto 模式；自建三优先级队列 + Cron Worker + LLM 提取器 |
| C6 | **五级 scope 隔离** | user / agent / team / workspace / global 作用域权限模型；评测中 `user_id` 直接映射 `scope=user` |
| C7 | **Add/Search 评测适配层** | 平台契约 → 内部 L2/L3 读写的协议桥接，不含任何评测数据特化逻辑 |

---

## 4. Add/Search API 契约

| 端点 | 方法 | 说明 |
|------|------|------|
| `/v1/memory/add` | POST | 写入记忆，同步返回 `200 {success, request_id, user_id, session_id, timestamp}` |
| `/v1/memory/search` | POST | 检索记忆证据，返回 `{data: [{id, content, score, created_at}]}`；无结果返回 `data: []` |
| `/healthz` | GET | 免鉴权存活探针，返回 `{"status":"ok"}` |

- **鉴权**：`Authorization: Bearer <Memory System Key>` 或 `X-Api-Key` 头；key 由我方签发并经申请表提交，不进仓库。
- **隔离**：`user_id` 是唯一检索边界，Add 与 Search 必须使用同一值；禁止跨 `user_id` 检索。
- **Search 不生成答案**：Search 路径无任何 LLM 调用，返回内容为库存记忆原文。
- **Add/Search 模型规则**：开源组 Add 阶段若使用 LLM 须为 `gpt-4o-mini`。本系统 **Add 路径不调用任何 LLM**——事实切分与槽位分类均为词法/规则实现（`internal/biz/memory_eval_classify.go`），故该约束自动满足。
- **Embedding**：OpenAI 兼容端点（如 `qwen3.7-text-embedding`，1024 维）；未配置时降级为关键词混合召回，契约保持可用。
- **重试语义**：可重试错误按 408/409/425/429/500/502/503/504 返回，供平台重试；`request_id` 重放幂等。

---

## 5. 部署方式

```bash
# 构建 + 启动（app + pgvector）
docker compose -f docker-compose.eval.yml build
EVAL_MEMORY_TOKEN=<memory-system-key> \
EVAL_VECTOR_DIM=1024 \
EMBEDDING_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode \
EMBEDDING_API_KEY=<key> \
EMBEDDING_MODEL=qwen3.7-text-embedding \
EMBEDDING_DIM=1024 \
docker compose -f docker-compose.eval.yml up -d

# 健康检查
curl http://localhost:8910/healthz

# 契约自测（7 项：healthz / 401 / 400×2 / 双用户隔离 / 幂等重放 / 空结果 []）
./test/agent-memory-challenge/smoke.sh http://localhost:8910 "$EVAL_MEMORY_TOKEN"
```

| 变量 | 用途 | 必填 |
|------|------|------|
| `EVAL_MEMORY_TOKEN` | 适配层 Bearer Key（Memory System Key） | 是 |
| `EVAL_VECTOR_DIM` | pgvector 列维度，须与 embedding 模型输出一致（1024 / 1536 / …）；不匹配会导致向量索引被跳过 | 建议 |
| `EVAL_PG_SOURCE` | Postgres DSN（compose 已内置默认值） | 是 |
| `EMBEDDING_*` | OpenAI 兼容 Embedding 端点 | 否（缺省降级关键词召回） |
| `KRATOS_AUTH_SECRET` | 框架 `pkg/auth` 的初始化守卫（评测端点不涉及 JWT）；compose 已提供占位默认值 | 否 |

> 仓库根 `Dockerfile` 用 Go 1.26 多阶段构建（与 `go.mod` 一致），评测镜像通过 `GO_BUILD_TAGS=pgvector` 构建参数启用 pgvector 向量存储。
> 运行形态为 **app + pgvector 双容器**：`NewData` 硬编码 Postgres，SQLite 仅存在于遗留离线迁移工具，故单容器形态不可行。

---

## 6. 容量、超时与限流

| 项 | 口径 |
|----|------|
| 容量 | 单容器默认配置支撑评测规模；PG 模式 16 写 / 32 读连接池 |
| Add 延迟 | 同步确认 < 1s（向量索引异步，不阻塞 200 响应） |
| Search 延迟 | P95 < 3s（向量 + FTS 双路召回） |
| 超时建议 | Add 60s / Search 30s |
| 限流 | 评测端点默认不限流；异常按 5xx 返回供平台重试 |

---

## 7. 合规声明

| 红线 | 保证 |
|------|------|
| user_id 隔离 | 适配层强制校验 `user_id`；所有召回 SQL 带 `scope='user' AND scope_id=:user_id` 谓词；无"全局记忆"兜底路径参与评测检索；单测覆盖跨 user_id 污染用例 |
| Search 不生成答案 | Search 路径无 LLM 调用（代码审查 + 单测断言）；返回 content 为库存记忆原文 |
| 来源披露 | 本文 §2/§3 即披露内容，随仓库发布 |
| 不作弊 | 适配层无语料匹配/硬编码分支；评测数据只经 Add 写入；版本 tag 绑定 commit |