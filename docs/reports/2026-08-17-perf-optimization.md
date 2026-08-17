# 性能问题深入分析与优化方案

> 日期：2026-08-17  
> 依据：[report-20260817-144800](../testing/reports/report-20260817-144800.md) 实测 + 代码锚点  
> 性质：调研/优化方案（不含实施进度）

---

## 0. 问题全景

本轮 k6 证明 **读路径服务端本身很快**（health P95 963µs、agents 47ms、0/77 万失败）。真正拖后腿的是少数「聚合/远程/全表扫描」接口，不是整体吞吐不够。

| # | 问题 | 实测 | 主导成本类型 | 优先级 |
|---|------|------|--------------|--------|
| OPT-1 | 模型目录 providers | P95 **747ms** | 188 次 Docker 卷 `os.Stat` | P2（管理页） |
| OPT-2 | 知识检索 | P95 **3.3s** | 远程 embedding + 顺序 RRF + 向量索引被 recency 表达式打掉 | **P1** |
| OPT-3 | 工具运行列表 | P95 **3.1s** | 租户 COUNT 相关子查询，无可用索引 | P2 |
| OPT-4 | 会话时间线 | P95 **1.7s** | `limit=0` 拉全量 UNION + 多次 hydrate | P2 |
| OPT-5 | Team 列表 | 371KB / page=5 仍全量 | **接口无分页** + definition 双份序列化 | P2 |
| OPT-6 | Prompt 预览 | P95 **958ms** | 预览路径调用 `GetEffectiveTools` + 全量拼装 | P3 |
| OPT-7 | 工具目录 | k6 P95 **422ms** / 56KB | 列表字段过肥 | P3 |

已修且本轮回归通过（无需再作为性能项）：BUG-01 部分唯一索引、BUG-02 cascade 改 `turns_v2`。

---

## OPT-1：`GET /v1/model-catalog/providers` ~750ms

### 现状

目录 JSON 已做 mtime 缓存（`current.json` 2.88MB 不再每请求反序列化）：

```176:191:internal/modelregistry/store.go
// LoadDirectory 加载模型目录（mtime+size 失效缓存：os.Stat 每请求 µs 级，
```

缓存之后仍稳定 590–764ms。组装循环对 **每个 provider** 调 `HasProviderLogo`：

```61:73:internal/service/model_catalog.go
		logoURL := s.uc.ProviderLogoURL(p.ID)
		out = append(out, &v1.CatalogProviderSummary{
			...
			LogoCached: s.uc.HasProviderLogo(ctx, p.ID),
```

```121:128:internal/modelregistry/store.go
func (s *Store) HasProviderLogo(providerID string) bool {
	path := s.ProviderLogoPath(providerID)
	_, err := os.Stat(path)
	return err == nil
}
```

188 家 × Docker Desktop 绑定卷 `Stat`（Windows 常见 2–4ms）≈ **400–750ms**，与实测吻合。`len(p.Models)` 在缓存命中时只是内存计数，不是主因。

### 方案

**推荐（半日，收益最大）**

1. 在 `LoadDirectory` 成功时扫描 `logos/` 一次，把存在的 providerID 放进 `map[string]struct{}`，随目录缓存一起失效。
2. `HasProviderLogo` 只查内存 set，禁止列表热路径 `os.Stat`。
3. 列表默认 `limit=50`；前端分页。188 家一次吐 52KB 无必要。

**备选**

- 列表不再返回 `logo_cached`，前端按 `logo_url` 拉，404 即无图。
- 进程内 TTL 10s 的完整 `ListCatalogProvidersResponse` 缓存（同步任务 `SaveDirectory` 时主动 `cacheOK=false`）。

**预期**：P95 **<80ms**（k6 同口径）。  
**验证**：`k6` catalog 场景阈值改为 `p(95)<100`；单测「缺 logo / 有 logo / 目录更新后 set 失效」。

---

## OPT-2：`POST /v1/knowledge/search` P95 3.3s（P1）

### 调用链（默认路径）

未带 `collection_id` → `FederatedRetriever.SearchAll`（最多 3 库 Route）→ `AdaptiveRouter`（简单查询不重写）→ `HybridRetriever` **Auto = RRF**：

```132:137:internal/knowledge/hybrid_retriever.go
func (h *HybridRetriever) selectMode(q biz.KnowledgeSearchQuery) HybridSearchMode {
	if h.sparse == nil {
		return HybridDense
	}
	return HybridRRF
}
```

RRF **串行**三步：`EmbedSingle` → dense ANN → BM25（trigram **再** tsvector）：

```164:210:internal/knowledge/hybrid_retriever.go
	vec, err := h.embedder.EmbedSingle(ctx, q.Query)
	denseChunks, err := h.dense.SearchChunks(ctx, denseQ, vec)
	sparseChunks, err := h.sparse.SearchChunksBM25(ctx, sparseQ)
```

```1091:1092:internal/data/knowledge.go
	trgmResults, trgmErr := r.searchChunksTrigram(ctx, q, extraClauses, extraArgs)
	tsResults, tsErr := r.searchChunksTsvector(ctx, q, extraClauses, extraArgs)
```

向量 SQL 把距离包进 recency 表达式再 `ORDER BY score DESC`，**IVFFlat 无法使用**：

```1016:1056:internal/data/knowledge.go
func recencyScoreSQL(base string) string {
	return fmt.Sprintf(`(%s) * EXP(-%g * GREATEST(...)) * CASE WHEN d.stale_at IS NULL ...`, base, recencyDecayLambda)
}
// ...
ORDER BY score DESC
```

`knowledge_chunks_embedding_idx` 是 `ivfflat (embedding vector_cosine_ops)`。pgvector 要求 `ORDER BY embedding <=> $1` 才能走索引。当前形态对 6000+ chunk 的库是 **JOIN documents + 表达式排序全表扫描**。

分段成本估计（与 2.2s 均值对齐）：

| 段 | 估计 | 依据 |
|----|------|------|
| 查询 embedding（远程） | 0.8–2.0s | 与 LLM 同网关，一次 HTTP |
| dense（无索引） | 0.2–0.8s | 6000 chunk cosine + JOIN |
| sparse trigram+tsvector | 0.1–0.4s | 两次独立查询 |
| 联邦 Route / 多库 | 0–1s | 空 collection 时最多 3 库重复上述 |

### 方案（按性价比）

**P0-a 恢复向量索引（1 天）**

1. 先按 `embedding <=> $1` 取 `topK*N`（例如 50），**再在应用层或子查询**乘 recency/stale。
2. `EXPLAIN ANALYZE` 确认 `Index Scan using knowledge_chunks_embedding_idx`。
3. chunk 过万后把 IVFFlat 换成 **HNSW**（构建慢、查询稳）。

**P0-b 查询向量缓存（0.5 天）**

- key = `embedder_id + model + sha256(query)`，TTL 10–30min，进程内 LRU（上限 2k）。
- 相同「告警」「故障清除」运维热词第二次应 <200ms。

**P1-c RRF 并行（0.5 天）**

- `embed` 完成后 `errgroup` 并行 dense / BM25。
- BM25：tsvector 优先，trigram 仅当 ts 命中不足再补（或并行但限制 `LIMIT`）。

**P1-d 默认模式**

- 列表页/工具调用默认 `hybrid_search=dense` 或 `sparse`；RRF 留给「高级检索」勾选。
- 空 `collection_id` 的联邦广播在管理 API 上打 span：`embed_ms / dense_ms / sparse_ms / collections`。

**P2-e 预计算 recency**

- dream_cycle 把 `exp(-λ·age)*stale` 写成 `knowledge_documents.rank_boost`，检索只读一列，避免每行 `now()-updated_at`。

**预期**：热查询 P95 **<400ms**（embedding 缓存命中 **<150ms**）；冷查询受 embedding 网关限制，目标 **<1s**。  
**验证**：Prometheus 已有 `knowledgeSearchDuration`，按 label 拆段；30 条 `sample-knowledge-qa.json` 回归命中率不得下降。

---

## OPT-3：`GET /v1/tools/runs` P95 3.1s

### 根因

```596:626:internal/data/tool.go
		where = append(where, `(EXISTS (SELECT 1 FROM sessions ws WHERE ws.id = ti.session_id AND ws.workspace_id = ?)
			OR (ti.session_id = '' AND EXISTS (SELECT 1 FROM agents wa WHERE wa.id = ti.agent_id AND wa.workspace_id = ?)))`)
	...
	SELECT COUNT(1) FROM tool_invocations ti WHERE `+whereSQL
	...
		FROM tool_invocations ti
		LEFT JOIN tools t ON t.tool_key = ti.tool_key
		LEFT JOIN agents a ON a.id = ti.agent_id
		WHERE `+whereSQL+`
		ORDER BY ti.started_at DESC
```

租户隔离用 **相关 EXISTS**，COUNT 必须扫调用表。现有索引是 `(tool_key, started_at)` / `(agent_id, started_at)` / `session_id`，**没有 workspace 维**。调用量随会话增长后 COUNT 先爆。

列表还 `LEFT JOIN` 整表取 display_name，再带 `input_preview/output_preview`（热列表不需要全文预览）。

### 方案

1. **冗余 `workspace_id` 到 `tool_invocations`**（写路径从 session/agent 填），索引 `(workspace_id, started_at DESC)`。COUNT 变 `Index Only Scan`。  
   过渡期可用物化：触发器或写入时双写。
2. 列表 API 默认不返回 preview；详情/params 接口再取。`SELECT` 列裁到 id/key/status/duration/started_at。
3. COUNT 可延迟：第一页只 `LIMIT n+1` 判断 has_more，total 异步或近似。
4. `EXPLAIN` 现网：若 COUNT >500ms 立刻上 1。

**预期**：P95 **<150ms**。  
**验证**：造 10 万 invocation 夹具；租户 A 不可见租户 B。

---

## OPT-4：`GET /v1/sessions/{id}/timeline` P95 1.7s

### 根因

`GetSessionTimeline` 把 proto `limit` 原样传入。`limit<=0` 时：

```68:74:internal/data/session_timeline.go
func clampTimelinePageLimit(limit, total int) int {
	if limit <= 0 {
		if total > 0 {
			return total  // 全量！
		}
```

测试未传 limit → **先 `COUNT(*)` 包三表 UNION，再取出全部行，再 3 次 IN 查询 hydrate**（message / tool / skill）+ agent 显示名。巡检会话工具事件多，11.5KB 仍要 1.7s，成本在 COUNT+UNION 不在 JSON。

消息-only 路径有默认 100，UNION 路径没有。

### 方案

1. `limit<=0` 改为 `MessageListDefaultLimit`（100），与消息列表一致。**禁止 limit=total**。
2. COUNT：`reltuples` 估算或「has_more」；精确 total 用独立轻量计数表。
3. UNION 三路改为 **按 occurred_at 归并游标**（merge 已排序的三条索引扫描），避免物化整个 UNION。
4. hydrate 保持现有 IN 批量，但 preview 截断 256 字符。

**预期**：默认页 P95 **<200ms**。  
**验证**：千事件会话分页稳定；`kind_filter=message` 回归。

---

## OPT-5：`GET /v1/teams` 371KB（page_size 无效）

### 根因

`ListTeams` **没有 page/page_size 字段**（team.proto 列表 RPC 无分页）。`ListByWorkspace` 一次取出全部 169 个 Team。

```189:197:internal/service/team.go
func toProtoTeam(t biz.Team) *v1.Team {
	return &v1.Team{
		DefinitionJson:    t.DefinitionJSON,
		OrchestrationSpec: toProtoOrchestrationSpec(t.DefinitionJSON),
```

同一份编排 JSON **原文 + 解析后的 spec 各传一次**。Spirit 自动建队多，单页 371KB，k6 混合读里 teams 会拖带宽（本轮 mixed 已 2.0GB received，health 为主，但 teams 一旦进混合权重会立刻放大）。

### 方案

1. proto 增加 `page/page_size`（默认 20，最大 50）；SQL `LIMIT/OFFSET`。兼容：缺省仍全量会破坏前端，**必须同时改 Web 列表**。
2. 列表 DTO：`id, team_key, display_name, status, member_count, has_active_run, updated_at`。`definition_json` / `orchestration_spec` 只走 `GET /v1/teams/{id}`。
3. 已有 `CountOnly` 可给徽章用，列表不要再 `List`+`Count` 打两枪（可 `COUNT(*) OVER()` 一次）。

**预期**：列表 <20KB、P95 <50ms。  
**验证**：前端团队页骨架；Spirit 建队后列表仍可滚动加载。

---

## OPT-6：`GET /v1/agents/{id}/system-prompt/preview` P95 958ms

### 根因

```31:57:internal/agent/prompt_preview.go
	sys := BuildSystemPrompt(...)
	if cue := RuntimeCapabilityCue(ctx, deps, agPreview); cue != "" {
```

`RuntimeCapabilityCue` → `GetEffectiveTools`（本轮单独测 **137ms**）。complete 模式再拼全部 prompt 文件（响应 18KB）。预览接口为设置页，却走了「近似真实装配」。

### 方案

1. 预览默认 `mode=minimized` 或跳过 `GetEffectiveTools`，工具段显示「启用 N 个（点开详情）」。
2. 设置未变时按 `agent.updated_at + files hash` 缓存预览 60s。
3. 设置页用骨架屏，不阻塞表单。

**预期**：P95 **<150ms**。非热路径，P3。

---

## OPT-7：`GET /v1/tools` k6 P95 422ms

### 根因

分页默认 20，但测试 `page_size=50` 打出 56KB：`bizToolToProto` 含 schema/config/binding 等宽字段。k6 贴 500ms 阈值，50 VU 混合时会先成为拐点。

### 方案

列表只返 key/name/category/enabled/risk；schema 走详情。默认 page_size=20。与 OPT-3 同一「列表瘦身」原则。

**预期**：P95 **<80ms**。

---

## 配置类（非代码性能，但拖观测与稳定性）

| 项 | 现象 | 方案 |
|----|------|------|
| MCP 6/6 error | 宿主 exe / `host.docker.internal:895x` 容器内不可达 | 容器内 MCP 改 compose 服务名；或 `enabled=false` 停探活 |
| ISSUE-K1 vault | Windows 路径每 30s Warn | `root_path` 改卷内路径或停用 sync |
| 图 visualize 400 | 残缺 function 节点缺 Func | 保持 400（已优于上午 500）；列表过滤 `valid=false` 或可视化占位节点 |

---

## 建议实施顺序

```
第 1 天  OPT-2a 向量 ORDER BY 恢复索引 + 检索分段 metrics
第 1 天  OPT-1  logo set 缓存（半日）
第 2 天  OPT-2b/c embedding LRU + RRF 并行
第 2 天  OPT-4 timeline 默认 limit
第 3 天  OPT-5 Team 列表 DTO + 分页（含前端）
第 4 天  OPT-3 tool_invocations.workspace_id + COUNT 索引
第 5 天  OPT-6/7 预览与工具列表瘦身
```

**不要先加机器**。k6 已证明 CPU/内存不是瓶颈；优化集中在「远程 embed、打掉的 ANN 索引、N 次 Stat、无分页肥 JSON、相关子查询 COUNT」。

### 观测缺口（批准后做）

- admin 开 `pprof`（内网端口、默认关）——验证 OPT-1 Stat 占比。
- PG `pg_stat_statements` + `log_min_duration_statement=200`——验证 OPT-2/3 计划。
- 检索 span：`knowledge.search.embed_ms` / `dense_ms` / `sparse_ms`。
