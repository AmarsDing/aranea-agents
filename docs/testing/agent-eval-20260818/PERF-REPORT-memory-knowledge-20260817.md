# 记忆与知识库提取性能评测报告（2026-08-17）

> 范围：会话记忆存储/加载、记忆召回（L2/L3/复合）、知识库 RAG 检索
> 被测：Docker 部署版 aranea-admin（HTTP :8810），PG/Redis 同栈，ollama bge-m3（宿主机 :11434）
> 方法：API 端对端计时（n≥8 取 p50/p95）+ 生产链路日志证据 + LLM relay 抓包 + DB 直查 + A/B 对照
> 数据证据：`02-memory-recall/evidence/perf-baseline.json`、容器日志 `agent.memory_cue.build`、`llm-capture.jsonl`

---

## 1. 结论速览

| 链路 | p50 | p95 | 判定 | 说明 |
|------|-----|-----|------|------|
| 会话历史读取（消息列表） | 32~44ms | ≤65ms | **PASS，快** | 含 665 事件大会话，limit=200 |
| 记忆召回（复合 L2+L3，DB 段） | 49ms | 60ms | **PASS，快** | 目标 <500ms；优于 Mem0(549ms)/Zep(~200ms) |
| 记忆召回 debug（L2/L3 拆分） | 54ms | 67ms | **PASS，快** | 同上 |
| chat 内记忆 cue 构建（fresh） | ~150ms | ≤283ms | **PASS** | 含 profile+L1+召回+embed；turn 内缓存命中 0ms |
| 知识库检索 dense/RRF/联邦 | 1540~1619ms | 1962~2258ms | **DEGRADED** | 瓶颈非检索本身，见 §4 |
| 知识库检索 dense（摘除评估器 A/B） | **149ms** | 400ms | （对照组） | 纯 embed+pgvector 段 |
| 知识库词法降级（BM25/tsvector） | 70ms | 90ms | **PASS，快** | 无 embed 路径 |

**核心发现**：记忆存取全链路是快的（30~70ms 量级）；知识库检索 p50≈1.5s 的表象下，**~90% 延迟来自每次搜索默认携带的「检索质量评估」远程 LLM 调用（RetrievalEvaluator→deepseek），而非向量检索本身**——向量检索段（ollama embed + pgvector）实测仅 ~150ms。

---

## 2. 会话记忆：存储与加载性能

### 2.1 会话历史读取（GET /v1/sessions/{id}/messages?limit=200，n=8）

| 会话 | 规模 | p50 | min | max |
|------|------|-----|-----|-----|
| big665 | 665 事件（全库最大，trpc_session_events 共 22274 行/59MB/854 会话） | 44ms | 40ms | 65ms |
| plant（评测植入会话） | 121 消息 | 32ms | 30ms | 37ms |
| recent | 30 消息 | 40ms | 33ms | 51ms |

判定：**快**。历史装配（DB→API→JSON）在 200 条消息内稳定 ≤65ms，随消息数增长平缓。

### 2.2 chat 链路记忆 cue 构建（生产日志证据，`agent.memory_cue.build`）

容器日志 15:44~15:47 连续 15 条样本：

| 指标 | fresh 轮（真实召回） | 缓存轮（工具循环重进 hook） |
|------|---------------------|---------------------------|
| duration_ms | 133~283（典型 133~173） | **0** |
| cue_chars | 3000~3300 | 同左 |
| recall_hits | 8 | 8 |

判定：**快且已被缓存保护**。每个 turn 首次 LLM 调用前构建 cue 增加 ~150ms（含多次 DB 读 + 一次 embed），同一 turn 的工具循环后续轮次经 invocation-state 缓存复用为 0ms，设计生效。

---

## 3. 记忆召回性能（检索段，不经 LLM；n=20，`perf-baseline.json`）

| 端点 | min | p50 | avg | p95 | max | 命中 |
|------|-----|-----|-----|-----|-----|------|
| /v1/memory/search/composite（L2+L3 复合） | 43 | 49 | 50.8 | 60 | 61 | 5 |
| /v1/memory/recall/debug（L2/L3 拆分） | 43 | 54 | 54.8 | 67 | 79 | 5 |

测试 agent 含 110 条 facts（75 条带 embedding）。判定：**PASS**——p50≈50ms，较目标 <500ms 有一个数量级余量，亦优于业界参考（Mem0 p50≈549ms、Zep≈200ms，口径：云服务 API）。

---

## 4. 知识库 RAG 检索性能与瓶颈定位

### 4.1 端对端基线（n=20，UX验证库 bge-m3 语义层，3 chunks）

| 模式 | min | p50 | avg | p95 | max |
|------|-----|-----|-----|-----|-----|
| dense | 1303 | 1540 | 1617 | 1962 | 2214 |
| rrf | 1336 | 1615 | 1756 | 2258 | 2642 |
| federated（全库路由） | 1002 | 1619 | 1658 | 2195 | 2521 |
| lexical（BM25 降级库） | 63 | 70 | 73 | 90 | 92 |

dense/RRF/联邦 ~1.5s vs 词法 70ms，差 20 倍。embed 是唯一显性差异项，但实测 embed 远小于该差值 → 逐段定位如下。

### 4.2 分段定位证据链

**① ollama embed 段（bge-m3, dim=1024）**
- 宿主机直连：p50=146ms，avg=167ms（n=15，min=133/max=339）
- 容器内→host.docker.internal：30 次共 3s ≈ **100ms/次**（含 wget 进程开销），响应 20237B 真实向量
- 结论：embed ≈ 100~150ms，**不是主瓶颈**。

**② pgvector 纯检索段**
- 目标库仅 3 chunks（表 512KB，HNSW 索引 4MB），检索为毫秒级；④的 A/B 差值反推 DB 段 ≈ 10~20ms。

**③ 抓包实锤（llm-capture.jsonl）**
15:44:30 发起一次 dense 搜索（耗时 1924ms），同一秒 relay 抓到一条 `deepseek-v4-flash` 请求，system prompt 为「**你是一名检索质量评估助手**。给定用户问题和检索到的文本片段，评…」（`RetrievalEvaluator` 的 prompt），messages=2。
- 代码路径：[knowledge.go](file:///f:/myproject/aranea-agents/internal/service/knowledge.go#L888-L895) → `SearchWithEvaluation` → [retrieval_evaluator.go](file:///f:/myproject/aranea-agents/internal/knowledge/retrieval_evaluator.go#L74-L79) → 远程 LLM。
- 装配：`wire_gen.go:331` 无条件构造（dynamicLLMCaller 恒非 nil）；模型解析 `refine_llm=deepseek/deepseek-v4-flash`（system_settings），清空后还回退到模型目录首个启用项 → **任何配置下都会发起远程评估调用**。

**④ A/B 对照（决定性）**
临时禁用全部 4 个启用模型使评估器解析失败（走降级策略 2 直接判 sufficient、不发 LLM），同库同查询 dense 检索：

| 组 | p50 | p95 | min | max | avg |
|----|-----|-----|-----|-----|-----|
| 含评估器（基线） | 1540ms | 1962ms | 1303 | 2214 | 1617 |
| **无评估器（A/B）** | **149ms** | 400ms | 147 | 400 | 174 |

**差值 ≈1390ms/次（占总延迟 ~90%）即评估 LLM 调用成本。**（A/B 后已恢复 DB 原状：4 模型 enabled、refine_llm 还原。）

**⑤ 长尾放大器**：评估判 insufficient 时触发二次补充检索（`search_helpers.go:37` 再一轮 embed+DB）。实测单发最高 4672ms（15:40），与基线 max 2642ms 的长尾一致。

### 4.3 结论

- 知识**检索本身是快的**（dense 全段 ~150ms：embed ~130 + pgvector ~15），与词法路径（70ms）同数量级。
- 慢的是**默认开启的检索质量评估（CRAG 式 LLM self-check）**：每次搜索一次远程 deepseek 调用 +1.3~1.5s，insufficient 时再叠加补充检索。RRF/联邦路径同样经过该评估器，故 latency 同水平。

---

## 5. 优化建议（按收益排序）

| 优先级 | 措施 | 预期收益 | 改动面 |
|--------|------|---------|--------|
| P1 | `RetrievalEvaluator` 改为**显式开启**（SearchRequest 加 `use_eval` 开关，默认关；或 system_settings 加 `knowledge_search_eval_enabled` 默认 false） | dense/RRF/联邦 p50 1540→**~150ms（-90%）** | service 一处 + proto 一个字段 |
| P1 | 评估调用**异步化**：先返回检索结果，评估结果用于后台补充检索/质量日志 | 同上，且保留 CRAG 能力 | search_helpers 改异步 |
| P2 | 评估模型换本地小模型（ollama qwen2.5vl:7b 已在目录），或结果按 query+chunks hash 缓存 | 评估从 ~1.4s 降至 ~200ms/趋 0 | 配置/缓存层 |
| P2 | 补充检索加预算护栏（每查询最多 1 次补充，总时延上限） | 收敛 2.6~4.7s 长尾 | search_helpers |
| P3 | embed 冷启动首击 ~400ms 已有 prewarm 覆盖（60s 窗口），容器内 wget 开销非生产路径，无需处理 | — | — |

## 6. 附：测试过程 DB 变更与恢复记录

| 时间 | 操作 | 状态 |
|------|------|------|
| 15:41 | `system_settings.refine_llm_*` 清空（验证缓存/回退路径） | 已恢复 deepseek/deepseek-v4-flash |
| 15:45 | `llm_provider_models` 4 行 enabled→false（A/B 对照） | 已恢复 enabled=true（llama3.1:8b 保持原状 false） |

恢复后已 SELECT 复核（15:52），与变更前一致。
