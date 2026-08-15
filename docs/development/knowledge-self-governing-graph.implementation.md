# 自治理知识图谱 — 开发实施文档（M0–M4）

> 对应设计：`knowledge-self-governing-graph.design.md`（评审稿）
> 对应需求：`37-knowledge.md` · `memory/memory.md`
> 状态：实施稿（2026-08-15）—— 每个环节细化到端口/装配点/DDL/测试，可直接拆任务开发
> 铁律：不引图数据库（PG 单库）；检索零 LLM；增量+失效不删除+版本化；受控涌现。

---

## 总览：五层架构与里程碑映射

| 层 | 职责 | 里程碑 | 关键改造文件 |
|---|---|---|---|
| L0 摄取编译 | 任意格式文件夹 → Markdown | **M0** | `sync_engine.go` / `vault_sync.go` / `extractor.go` |
| L1 统计联想 | base-level 打分 + Hebbian 边 + 扩散激活 | **M1** | `data/knowledge.go` / `graph_expander.go` / 新增 `access_log` |
| L2 语义关系 | 两步 LLM 抽取 typed edges | **M2** | 新增 `relation_extract.go` / 接线 `SetEntityHook` |
| L3 演化时序 | 双时态边 + supersedes 版本链 | **M1(地基)/M3(链)** | `data/knowledge.go`(DDL) / `writeback.go` |
| L4 自治理 | dream_cycle 接管词条治理 | **M4** | `memory_butler/` 新增 `knowledge_curate.go` |

依赖关系：M0（摄取）与 M1（检索+存储地基）可并行；M2 依赖 M1 的边表；M3 依赖 M1 的双时态；M4 依赖 M1/M2/M3 全部。

---

## M0 — 摄取编译层（丢任意格式文件夹）

### 目标
vault 文件夹同步从「只收 .md」扩为「按扩展名路由到多模态抽取器」，统一产出 Markdown 进索引。

### 现状边界（代码实证）
- [sync_engine.go:92](file:///f:/myproject/aranea-agents/internal/biz/knowledge/sync_engine.go#L92)：`!strings.HasSuffix(strings.ToLower(name), ".md") → return nil`，非 .md 静默跳过。
- 多模态抽取器已存在但只服务 Chat 附件：
  - [document_extract.go:15-20](file:///f:/myproject/aranea-agents/internal/knowledge/document_extract.go#L15)：`supportedExtractExts` 含 txt/md/json/csv/yaml/xml/html/pdf/doc/docx/xlsx/pptx
  - [vision_extractor.go:33-39](file:///f:/myproject/aranea-agents/internal/knowledge/vision_extractor.go#L33)：png/jpg/jpeg/webp 经视觉 LLM → Markdown
  - [extractor.go:76-82](file:///f:/myproject/aranea-agents/internal/knowledge/extractor.go#L76)：`ExtractorRegistry` 按 ext 路由

### 设计

**改动点 1 — SyncEngine 扫描放宽**（`biz/knowledge/sync_engine.go`）

把 [第 92 行](file:///f:/myproject/aranea-agents/internal/biz/knowledge/sync_engine.go#L92) 的后缀过滤改为「可索引扩展名集合」判定：
```go
// 原：只收 .md
if strings.HasPrefix(name, ".") || !strings.HasSuffix(strings.ToLower(name), ".md") { return nil }
// 改：收可索引集合（文本类直读 + 二进制走抽取）
if strings.HasPrefix(name, ".") || !IsIngestibleExt(name) { return nil }
```
`IsIngestibleExt` 新增于 `biz/knowledge`（纯函数，含 `.md` 直读集合 ∪ 抽取器支持集合）。注意：抽取器在 `internal/knowledge`（infra 层），`biz/knowledge` 不依赖 infra —— 故 `IsIngestibleExt` 只判扩展名白名单，不 import 抽取器；真正的抽取在 applier 层。

**改动点 2 — SnapshotDoc 支持编译**（`biz/knowledge/vault_filer.go:498`）

`FileSnapshot` 当前只带 `RelPath/ModTime/Size/Hash`，正文由 applier 读文件直取。需扩展：
- 文本类（.md/.txt/...）：维持直读，零成本。
- 二进制类（.pdf/.docx/图片）：applier 在 `applyOne` 里检测扩展名 → 调抽取器 → 得 Markdown 作为 `body` 进 chunk/autolink。

**改动点 3 — 抽取器注入 applier**（`internal/knowledge/vault_sync.go`）

`VaultSyncApplier` 新增 `extractor *ExtractorRegistry` 字段（nil = 仅文本直读，向后兼容）。在 `applyOne` 内：
```go
body, compileMeta, err := a.compileBody(ctx, vault, ev.RelPath)
// compileMeta: {extractor: "text|vision|doc", confidence, source_hash}
```
- `compileBody`：文本类 → `os.ReadFile` 直读；二进制 → `extractor.Extract(...)` 路由。
- 抽取失败 → `status=error`，**不阻断**其他文件（沿用现有单事件失败隔离）。

**改动点 4 — 异步与限流（成本闸门）**

二进制抽取走 LLM，必须限流：
- 新增 vault 级配置 `compile_daily_quota`（默认 50 文件/日），超额文件标记 `pending_compile` 次日续。
- 抽取经 `safego.Go` 异步（沿用 summaryHook/entityHook 的异步模式），首轮扫描不阻塞。

**改动点 5 — provenance 留痕**

`documents` 表已有 `source`/`mime_type`。新增 metadata 记录编译来源：`compiled_from=原始扩展名`、`compiler=extractor名`、`source_hash=原文件hash`。低置信（图片 OCR/扫描件 PDF）置 `needs_review=true`，进治理队列。

### 风险与对策
| 风险 | 对策 |
|---|---|
| 大文件夹首轮全量编译 LLM 爆量 | 每日配额限流 + 异步 + .md 直读零成本优先 |
| 扫描件 OCR 错乱污染下游 | provenance 置信度 + needs_review 进治理队列 |
| biz 层依赖 infra 破坏分层 | `IsIngestibleExt` 纯函数判白名单，抽取在 infra applier |

### 测试（TDD）
- `sync_engine_test.go`：混合文件夹扫描，.md/.pdf/.png 入选，`.tmp`/隐藏文件跳过。
- `vault_sync_test.go`：pdf 事件 → 调 mock extractor → body 为返回的 Markdown；extractor 失败 → status=error 不阻断。
- `vault_sync_test.go`：配额超限时第二个 pdf 标 `pending_compile`。

---

## M1 — 统计联想层 + 时态地基（检索质变）

### 目标
检索打分加 base-level 激活分、Hebbian 共激活边、扩散激活 2 跳扩展；铺双时态边表地基。

### 设计

**M1.1 — 双时态边表 DDL**（`data/ddl_migration_registry.go` 末尾新增版本）

```go
{Version: 20260xxx, Name: "knowledge_links_bitemporal", Func: ddlKnowledgeLinksBitemporal},
```
DDL（幂等）：
```sql
ALTER TABLE knowledge_links
  ADD COLUMN IF NOT EXISTS relation     TEXT,
  ADD COLUMN IF NOT EXISTS weight_f     DOUBLE PRECISION NOT NULL DEFAULT 1.0,
  ADD COLUMN IF NOT EXISTS confidence   DOUBLE PRECISION NOT NULL DEFAULT 1.0,
  ADD COLUMN IF NOT EXISTS valid_from   TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS valid_to     TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS recorded_at  TIMESTAMPTZ NOT NULL DEFAULT now();

DROP INDEX IF EXISTS knowledge_links_unique;
CREATE UNIQUE INDEX IF NOT EXISTS knowledge_links_unique
  ON knowledge_links (doc_id, target_doc_id, link_type, COALESCE(relation,''));
CREATE INDEX IF NOT EXISTS knowledge_links_valid_idx
  ON knowledge_links USING GIST (tstzrange(valid_from, COALESCE(valid_to,'infinity')));
CREATE INDEX IF NOT EXISTS knowledge_links_active_idx
  ON knowledge_links (collection_id, doc_id) WHERE valid_to IS NULL;
```
存量回填：`valid_from=created_at, valid_to=NULL`（UPDATE WHERE valid_from IS NULL，幂等）。

> 版本号须递增且过 `TestMigrationVersionsGloballyUnique`（项目铁律）。

**M1.2 — `knowledge_access_log` 表**（同迁移）
```sql
CREATE TABLE IF NOT EXISTS knowledge_access_log (
  id BIGSERIAL PRIMARY KEY,
  collection_id TEXT NOT NULL,
  doc_id TEXT NOT NULL,
  accessed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  query_hash TEXT,
  session_id TEXT
);
CREATE INDEX ON knowledge_access_log (collection_id, doc_id, accessed_at DESC);
```

**M1.3 — base-level 激活分并入检索**

落点：检索结果融合处。`Usecase.Search`（[knowledge.go:491](file:///f:/myproject/aranea-agents/internal/biz/knowledge/knowledge.go#L491)）委托 `chunks.SearchChunks`。混合检索的 RRF 融合在 service/检索编排层（`knowledge_search_federated.go` 等）。在融合后、返回前注入：
```go
finalScore = rrfScore + beta * baseLevel(docID, accessLog)
// baseLevel = ln( Σ_t (now - access_t)^-0.5 )，ACT-R base-level，d≈0.5
```
- 新增端口 `AccessLogRepo`（biz/knowledge）：`LogAccess(batch)` / `BaseLevelScores(ctx, collectionID, docIDs) map[docID]float64`。
- 实现于 data 层，`BaseLevelScores` 用一条聚合 SQL：`SELECT doc_id, ln(sum(power(extract(epoch from now()-accessed_at)/86400, -0.5))) ... GROUP BY doc_id`。
- `beta` 默认 0.1，vault 可调。**纯词法库**（无向量）同样适用（与语义层无关）。

**M1.4 — Hebbian 共激活边**

- 检索返回 top-k 后，同批词条（同 `query_hash`）两两写 `co_activated` 边 `weight_f += η`（η=0.1）。
- 落点：检索结果消费处异步（`safego.Go`），不阻塞返回。
- `co_activated` 为独立 `link_type`，`relation=NULL`。
- 周期衰减在 M4 dream_cycle：`weight_f *= 0.9`，< 0.05 置 `valid_to`。

**M1.5 — 受限扩散激活 2 跳扩展**（改造 `graph_expander.go`）

现状：`graph_expander.go` 做查询时一跳 Lazy 扩展（explicit/entity×2/semantic×1）。改造：
- 种子 = 混合检索 top-k 的 docID。
- 沿 active 边（`valid_to IS NULL`）传播 2 跳：explicit/entity/semantic/co_activated。
- 每跳能量 ×0.5 衰减；不同类型边权重：explicit=1.0, semantic=0.9, entity=0.7, co_activated=0.4（Hebbian 弱信号压低）。
- 侧抑制：只保留激活值 top-N（默认 10）。
- 激活值作为扩展 chunk 的加分项并入最终排序。**零 LLM**。
- 修正 [graph_expander.go:204](file:///f:/myproject/aranea-agents/internal/knowledge/graph_expander.go#L204) 的 1/2/1 硬编码，改为按 link_type 取权。

### 测试（TDD）
- `knowledge_access_log` 写入与 `BaseLevelScores` 聚合正确性（构造多次访问，断言分数递增）。
- Hebbian：同批 3 词条检索 → 3 条 co_activated 边；衰减 job 后 weight_f 降。
- 扩散激活：A→B(explicit)→C(explicit)，查 A 能 2 跳召回 C（无词重叠）；co_activated 边权重低于 explicit。
- 双时态：as-of 查询 `WHERE valid_from<=t AND (valid_to IS NULL OR valid_to>t)` 命中正确边。

---

## M2 — 语义关系层（懂关系）

### 目标
两步 LLM 抽取 typed edges（is-a/part-of/depends-on/causes/…），激活 entity/semantic 死代码。

### 设计

**M2.1 — 接线 entity 共现轨（死代码复活）**

现状：[vault_sync.go:236](file:///f:/myproject/aranea-agents/internal/knowledge/vault_sync.go#L236) `a.entityHook` 为 nil 时跳过；`SetEntityHook` 全仓无生产调用者。
- 生产装配（wire/service 层）：构造 `VaultSyncApplier` 后调 `SetEntityHook(...)`，hook 内部按 `docID+contentHash` 幂等触发实体共现抽取（`FindEntityCooccurrences` → `ReplaceEntityLinks`，`link_type=entity`）。
- 落点：`cli_admin_tools.go` / wire provider（查 `NewVaultSyncApplier` 生产装配处）。

**M2.2 — 两步 LLM 关系抽取**（新增 `internal/knowledge/relation_extract.go`）

```
词条 body
 → [Step1 实体抽取] LLM 抽实体清单（名词短语）
 → [Step2 三元组抽取] 基于实体清单抽 (主语, 谓词, 宾语, confidence)
 → [归一化] 嵌入召回 top-k 已有同义实体/谓词 → LLM 判重合并（KGGen 方法）
 → 写 knowledge_links (link_type=semantic, relation=谓词, confidence)
```
- 谓词归一化到核心闭集（is-a/part-of/depends-on/causes/applies-to/contradicts/supersedes/evolves-from）；词表外关系 → 写 `knowledge_relation_vocab` `tier=candidate`。
- **只对高价值词条抽取**（被检索频次 > 阈值，数据源 `knowledge_access_log`），控成本。
- confidence < 0.7 的边 `valid_to=now()`（不进主图谱，仅留痕）。
- 用 mini 级模型（`ResolveVisionLLM` 类似的 LLM 解析机制，配 `relation_extract` 用途）。

**M2.3 — `knowledge_relation_vocab` 表**（DDL 随 M2 迁移）
```sql
CREATE TABLE IF NOT EXISTS knowledge_relation_vocab (
  relation TEXT PRIMARY KEY,
  tier TEXT NOT NULL,           -- core/candidate/promoted
  proposed_by TEXT,
  use_count INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 预置 core 谓词（ON CONFLICT DO NOTHING 幂等）
```

### 测试（TDD）
- mock LLM 返回实体清单+三元组，断言 typed 边写入正确 relation。
- 同义实体（"PostgreSQL"/"PG"）判重合并为同一节点。
- confidence<0.7 的边 valid_to 已关闭；词表外谓词进 candidate。
- 低价值词条（access 频次低）不触发抽取。

---

## M3 — 演化时序层（生长留痕）

### 目标
supersedes 版本链（增量叠加，不改主流程）+ 写入时冲突检测。

### 设计

**M3.1 — supersedes 版本链（增量叠加）**

现状：`replaceH2BlockContaining`（[writeback.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback.go)）整段顶替旧段，演化轨迹丢失。
- **不改主流程**（规避回归面）：替换成功后，**额外**把旧段快照 + 新段写一条 `supersedes` 边（旧段 → 新段），旧段版本 `valid_to=now()`。
- 新增 `knowledge_fact_version` 表存旧段快照（避免污染 links 表）：
```sql
CREATE TABLE IF NOT EXISTS knowledge_fact_version (
  id BIGSERIAL PRIMARY KEY,
  collection_id TEXT NOT NULL,
  doc_id TEXT NOT NULL,
  fact_id TEXT,
  old_body TEXT NOT NULL,
  new_body TEXT,
  superseded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**M3.2 — 写入时冲突检测**

- 新 fact 写入前，语义检索召回同词条/同实体的候选旧 fact → LLM 仲裁（contradicts / supersedes / 无关）。
- `contradicts` → 建边 + 进治理提案（高风险，人工二审）。
- `supersedes` → 走 M3.1 版本链。
- 落点：`writeBackFacts` 写词条路径（[writeback_entry.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/writeback_entry.go) `upsertEntryDoc`），复用 M2 的 LLM 仲裁器。

### 测试（TDD）
- 同 fact_id 二次写入 → 产生 supersedes 版本记录，旧 body 留痕，主文档为新 body。
- 矛盾 fact 写入 → 建 contradicts 边 + 高风险提案，旧段不被覆盖。
- writeback 既有全量测试保持绿（确认增量叠加不破坏主流程）。

---

## M4 — 自治理层（记忆管家接管词条）

### 目标
dream_cycle 从只管 memory_fact 扩到治理知识库词条，输出治理提案闭环。

### 设计

**M4.1 — `knowledge_governance_proposal` 表**（DDL 随 M4）
```sql
CREATE TABLE IF NOT EXISTS knowledge_governance_proposal (
  id BIGSERIAL PRIMARY KEY,
  collection_id TEXT NOT NULL,
  kind TEXT NOT NULL,        -- conflict/stale/orphan/decay/merge/moc_emerge/relation_promote/distill
  payload JSONB NOT NULL,
  risk TEXT NOT NULL,        -- low/high
  status TEXT NOT NULL DEFAULT 'pending',  -- pending/applied/rejected
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ
);
```

**M4.2 — 新增 `memory_butler_knowledge_curate` 工具**（`internal/tools/memory_butler/knowledge_curate.go`）

实现 `trpctool.CallableTool`（沿用 butler 工具模式，经 `cli_admin_tools.go` 在 `AgentKey==__memory__` 时挂载）。扫描词条库健康，产出提案：

| 任务 | kind | risk | 应用 | 数据源 |
|---|---|---|---|---|
| Hebbian 弱边衰减 | decay | low | 自动 | knowledge_links co_activated |
| 陈旧词条标记 | stale | low | 自动 | valid_to 关闭比例 + access_log 距今天数 |
| 候选谓词归并提升 | relation_promote | low | 自动 | relation_vocab candidate use_count |
| 高频词条反向蒸馏成 memory_fact | distill | low | 自动 | access_log 高频 + L3 注入 |
| 冲突仲裁（contradicts 边） | conflict | high | 人工二审 | knowledge_links contradicts |
| 孤儿词条（度=0 且超 N 天未检索） | orphan | high | 人工二审 | links 度统计 |
| hub 簇蒸馏成 MOC 词条 | moc_emerge | high | 人工二审 | 图密度聚类 |

**M4.3 — 接入 dream_cycle**

- dream_cycle 工具（[dream_cycle.go](file:///f:/myproject/aranea-agents/internal/tools/memory_butler/dream_cycle.go)）增加 `curate_knowledge` 步骤，在现有 L3 治理后执行。
- 高风险提案写 `knowledge_governance_proposal` `status=pending`，复用 writeback pending HITL 链人工确认。
- cron 触发处（[seed_system_admin.go:443](file:///f:/myproject/aranea-agents/internal/data/seed_system_admin.go#L443)）当前 `dry_run=true`——M4 上线后知识治理部分可对低风险项转 `dry_run=false`，高风险仍提案制。

### 测试（TDD）
- 造孤儿词条 → curate 产出 orphan 提案（risk=high, status=pending）。
- co_activated 边衰减：weight_f<0.05 的边被置 valid_to。
- candidate 谓词 use_count 超阈值 → 提升 promoted。
- 高风险提案不入主流程，人工 apply 后才生效。

---

## 全局约束与落地纪律

1. **不引图数据库**：全部 PG 单库（tstzrange+GiST 支撑时态）。
2. **检索零 LLM**：LLM 只在 M0 编译 / M2 抽取 / M3 仲裁 / M4 治理（写入与离线路径）。
3. **增量+失效不删除+版本化**：M1 双时态 + M3 supersedes，替代覆盖式修订。
4. **受控涌现**：core 谓词硬编码 + LLM 提议 candidate + M4 治理归并 promoted。
5. **DDL 迁移铁律**：版本号递增 + `TestMigrationVersionsGloballyUnique`；种子幂等 `ON CONFLICT DO NOTHING`。
6. **工具注册铁律**：新工具 `internal/tools/<pkg>/` 实现 `CallableTool` + `builtin_tools_seed.go` 加 seed + 存量库加 reseed 迁移 + `tool_key` 与 declaration `Name` 一致。
7. **每期独立可验证、可回滚**，符合小步快跑。

## 开发顺序建议

- **第一波（可并行）**：M0（摄取编译）+ M1（统计联想+时态地基）。
- **第二波**：M2（语义关系，依赖 M1 边表）。
- **第三波**：M3（演化链，依赖 M1 双时态）。
- **第四波**：M4（自治理，依赖 M1/M2/M3）。

> 本实施稿待确认后，按里程碑拆任务、逐环节 TDD 落地。M0/M1 为首发。
