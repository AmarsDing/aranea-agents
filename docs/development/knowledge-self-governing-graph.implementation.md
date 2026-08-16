# 自治理知识图谱 — 开发实施文档（M0–M4）

> 对应设计：`knowledge-self-governing-graph.design.md`（评审稿）
> 对应需求：`37-knowledge.md` · `memory/memory.md`
> 状态：实施稿（2026-08-15）—— 每个环节细化到端口/装配点/DDL/测试，可直接拆任务开发
> 进度：**M0 ✅ / M1 ✅ / M2 ✅ / M3 ✅ / M4 ✅（2026-08-16 完成，测试全绿）——五层全部落地**
> 铁律：不引图数据库（PG 单库）；检索零 LLM；增量+失效不删除+版本化；受控涌现。

---

## M4 落地纪要（实施偏差记录）

- **M4.1 提案表零 DDL**：`knowledge_governance_proposal` 随 M3 迁移 `20261222_knowledge_fact_version.sql` 已建，M4 直接复用；fresh 形态由 `TestEnsureKnowledgeSchema_M3EvolutionFreshShape` 守卫。提案 kind/risk/status 常量集中在 biz `evolution.go`。
- **M4.2 数据端口 + biz 用例**：`KnowledgeCurateRepo` 窄接口（8 方法）定义在 biz [curate.go](file:///f:/myproject/aranea-agents/internal/biz/knowledge/curate.go)，data 层 `knowledge_curate.go` 实现；装配走 `NewKnowledgeUsecase` 内 repo 类型断言（`biz/knowledge.go`，与 M3 `SetEvolutionRepos` 同模式，断言失败 `CurateKnowledge` 显式报不可用）。`CurateKnowledge` 编排五类任务：低风险自动——decay（co_activated `weight_f *= 0.9`，跌破 0.05 置 `valid_to` 关闭留痕）、relation_promote（candidate `use_count≥3` → promoted，幂等）、stale（出向 semantic 边关闭比例 ≥0.5 且 30 天未检索，`status=applied` 留痕即标记）；高风险仅产 pending 提案——conflict（active contradicts 边）、orphan（度=0 且 30 天未检索）。**dedup_key 去重防周期风暴**（`stale:<doc>`/`conflict:<a>→<b>`/`orphan:<doc>`，探测失败保守放行不丢信号）；单任务失败 Warn 降级继续；每类提案单轮上限 50。dry_run：decay 走 COUNT 预估，promote/提案不落库。人工二审闭环 `ResolveGovernanceProposal`（pending→applied/rejected，其他状态非法）。
- **M4.2 工具**：`memory_butler_knowledge_curate`（function 工具模式，非 CallableTool——与 butler 既有 7 工具一致）定义在 `internal/tools/memory_butler/knowledge_curate.go`；`RegisterAll` 内 `deps.Knowledge != nil` 才挂载，`cli_admin_tools.go` 经 `o.rt().Knowledge.Usecase` 注入，未装配时工具消失、dream 步骤跳过（解耦降级）。
- **M4.3 dream_cycle 接入**：`dream_cycle.go` Step 5.5 在 L3 治理后调 `CurateKnowledge`——**dream 的 dry_run 传导**（dry_run 时知识治理走只读预估路径），非 dry_run 真治理；失败 Warn 不中断梦境主流程，产出计入 `KnowledgeProposals`/`KnowledgeActions`，actions 追加 `curate_knowledge`。
- **distill 落地（2026-08-16 补齐）**：`runDistillTask` 高频词条反向蒸馏——`HotDocumentLister`（热度窗口内命中 ≥ 阈值的文档，data `ListHotDocuments`）取热词条，摘要卡蒸馏成 L3 轻量事实（scope=workspace 全工作区注入）；**仅团队库且有 workspace**，local vault 静默跳过；**幂等双保险**——fingerprint（`kdistill:<doc>`）冲突键 upsert + dedup_key 带摘要 hash 截 12 位（摘要变更自然重蒸馏，去重只看 applied）；low risk 自动 applied 留痕，dry_run 只探测。生产装配 `provideKnowledgeDistillWiring`（wire.go/wire_gen.go 同源）：knowledgeRepo 断言 `HotDocumentLister`，`MemoryAdminUsecase` 适配 `DistillFactWriter`，`SetDistillRepos` 注入（未接线返回 0 静默跳过）。
- **moc_emerge 落地（2026-08-16 补齐）**：`runMOCEmergeTask` hub 簇涌现——data `ListHubClusters` 纯 SQL 检测（entries↔entries active 边无向度数 ≥ `MOCMinDegree`，inbox 流水与 closed 边排除，度数降序 limit 50，附 1 跳 DISTINCT entries 邻居）；规模 ≥ `MOCMinMember` 且密度（`CountActiveEdgesWithin` 簇内有向边 / n(n-1)/2，反向并存计两条）≥ `MinMOCDensity` → **high risk pending 提案**（payload 含 members/density/suggested_title/suggested_path=moc/<title>.md），**人工二审后手动建 MOC 词条，不自动落地**（「MOC 是否参与默认检索」仍是设计开放问题）；去重三态全含（pending+applied+rejected——人工已建或否决的同 hub 不再提案）。
- 测试：`curate_test.go`（18 例：未接线/全产出轮/dry_run 零写入/提案去重三态语义/提案 repo 未接线降级/落点解析失败/落点选中团队库/单任务失败降级/Resolve/List 二审闭环 + distill 5 例：蒸馏写入+提案留痕/applied 去重/dry_run 零写入/未接线跳过/local vault 跳过 + moc_emerge 3 例：提案产出/规模密度阈值闸/三态去重）+ `knowledge_curate_test.go`（PG 集成 9 例：衰减 dry-run 预估与实际关闭且 semantic 边不受波及/谓词提升幂等/孤儿扫描/陈旧扫描/contradicts 扫描/提案去重+Resolve 闭环/提案列表过滤投影 + hub 簇检测（度数阈值、entries-only、closed 排除、排序截断、邻居无 inbox 泄漏）+ 簇内边计数（反向并存计两条、closed 不计、空集恒 0））。PG 测试需 `ARANEA_TEST_PG_DSN`。

### M4 补丁纪要（2026-08-16 深度检查修复）

深度检查知识库/记忆管家/L0–L4 业务逻辑后发现 2 缺陷 + 1 风险已修，1 风险调研排除：

- **缺陷1 提案死信（只写不读）**：pending 提案无出口，人工二审不可达。修复三层贯通——data `ListGovernanceProposals`（collection/status 过滤 + id DESC 分页）；biz `ListGovernanceProposals`/`ResolveGovernanceProposal`（status 白名单校验、limit 默认 50/上限 200）；出口两路：①记忆管家工具 `memory_butler_governance_proposals`/`memory_butler_governance_resolve`（与 butler 既有工具同 function 模式，`deps.Knowledge != nil` 才挂载共 10 工具；resolve 描述明确「仅用户明确指示时调用，禁自主批量二审」）；②HTTP API `GET /v1/knowledge/governance-proposals`、`POST /v1/knowledge/governance-proposals/{id}:resolve`（list 指定 collection 时校验读权限，空=平台管理视图；resolve 流程日志 step `knowledge.governance.resolve`）。
- **缺陷2 框架原生检索漏排日记流水**：`knowledge_adapter.go toBizQuery` 补 `ExcludePathPrefixes=[inbox/writeback-]`，与 knowledge 工具/cue 预检索同规（此前框架原生 `knowledge_search` 路径流水会进 Agent 上下文）。
- **风险3 拒绝不沉默**：conflict/orphan 去重 statuses 只查 pending → 已否决提案每轮 dream 重复骚扰。修复：去重状态集加 `rejected`（拒绝即沉默），`TestCurateKnowledge_ProposalDedup` 断言三类 kind 状态集语义（conflict/orphan=pending+rejected，stale=pending+applied）。
- **风险5 cue 路径学习信号——调研排除**：cue 预检索（`knowledge_inject.go retrieveCueChunks`）与 knowledge 工具同走 FederatedRetriever→AdaptiveRouter 三件套；`AdaptiveRouter.Search` 末端统一挂 `applyBaseLevelBoost`（写 `knowledge_access_log`，生产唯一写入方）+ `triggerHebbian`（`StrengthenCoActivations` 同批召回两两 +0.1，异步）；生产装配 `NewKnowledgeAdaptiveRouter` 接线 `SetAccessLog(0.1)`/`SetCoActivation(0.1)`。强化（+0.1/批）与衰减（×0.9/轮，<0.05 关闭）闭环自洽；cue 高频召回正常记 access_log，不会误标 stale/orphan。**结论：已闭环，无需修复**（裸 `ret.Search` 兜底仅 router 为 nil 时触发，生产不可能）。

### 深度检查补丁纪要（2026-08-16 第二轮）

对知识库/记忆管家/L0–L4 业务逻辑做第二轮深度巡检，发现 2 缺陷 + 1 风险 + 1 过期注释，全部修复：

- **P1-1+P2-1 写回收口实体/关系冷启动**：`writeback.go` 创建词条页后，新增 `writeBackGraphHook` 同步触发 `EntityPipeline.ProcessDoc`（M2.1 实体共现）+ `RelationExtractor.ExtractDoc`（M2.2 typed edges），新词条页即刻获得语义关联，不再等热文档阈值。生产装配 `provideKnowledgeWriteBackGraphHook` 注入 `NewKnowledgeService`。
- **P1-2 知识检索 workspace 隔离回填（C-01）**：`knowledge_inject.go` cue 预检索、`knowledge_search` 工具、`KnowledgeService.Search` 三处全库枚举入口统一加 `workspace.ReadableFilterID(ctx)` 过滤——system 调用方见全部，其余按调用方 workspace 过滤（共享行仍可见），杜绝跨租户集合名/ID 泄露。
- **P2-2 ImmediateFactWriter embedding 新鲜度**：`ImmediateFactWriter` 新增 `indexSync MemoryFactIndexSyncer` 字段，`writeFactsSync` 写入成功后逐行 `SyncFactIndexFromRow`（对齐 auto_memory 回采范式），即时事实 embedding 不再等 reconciler cron 兜底。`ChatInfraDeps` 同步加 `FactIndexSync`/`SkillEmbedder` 两字段。
- **P2-3 selective_remember 置信度修正 + 语义去重**：①默认 confidence=0.8 / importance=0.7（此前零值导致召回侧阈值永久过滤）；②新增 `semanticDuplicate` 余弦判重（阈值 0.92，对齐 `FactWriteMergeScore`），Embedder 生产已接线 `MultiProviderEmbedder`，nil/失败降级字符串判重不阻断。
- **P3-3 graph_expander 过期注释清理**：`linkTypePriority` 注释从「SetEntityHook 全仓无生产调用者/死代码」更新为「entity/semantic 边由 M2.1 SetEntityHook 生产装配触发，当前已接线」。

测试：新增 `immediate_fact_writer_test.go`（4 例：索引同步/ nil 降级/同步失败不阻断/写入失败跳过）+ `selective_remember_test.go`（5 例：精确重复/语义重复/新颖默认值/nil Embedder 降级/Embed 失败降级）+ 全量构建 `go build ./cmd/... ./internal/...` + `go build -tags wireinject` + `go vet` 全绿。回归：biz/knowledge（18 例）、knowledge（全量）、memory_butler（11 例）、service Knowledge（全量）、workspace（全量）均通过。

### 深度检查补丁纪要（2026-08-16 第三轮）

第三轮整体梳理（两个巡检代理线索 + 逐条亲验代码）——代理报告的 6 条线索亲验后 **5 条为误报**（附核实依据防再报），确认 1 盲区 + 2 语义项，全部修复：

- **P2 dream 治理覆盖盲区（修复，选案 A 多库枚举）**：`CurateKnowledge` 单轮只治理解析出的单集合（CollectionID 空 → `LookupWriteBackHome` 默认写回落点），dream 调用不传 workspace/collection → 多团队库/多 workspace 场景非落点集合永不被自动治理（六类任务单集合，仅 relation_promote 全库）。修复：biz 新增 `CurateAllTeamKnowledge`——枚举 `ListCollections` 过滤 `VaultBackendTeam` 逐库 `CurateKnowledge`（指定 CollectionID 退化单库保工具语义；单库失败 Warn 降级其余库继续；无团队库返回同单库版 NotFound；relation_promote 全库口径第二库起自然空幂等）。dream_cycle Step 5.5 dry_run/正式两分支均改调多库入口，报告经 `aggregateCurateReports` 合并（pending 提案求和、任务名首现去重）。
- **P3-1 moc 密度口径上偏（修复）**：`CountActiveEdgesWithin` 原按有向边计数（A→B/B→A 并存计 2），分母却是无向完全图 n(n-1)/2 → 密度上限 2.0、阈值 0.3 偏松。修复：SQL 改 `LEAST/GREATEST` 规范化按文档对 DISTINCT（反向并存/同对多类型计 1 对），密度口径上限 1.0，biz 接口注释同步。
- **P3-2 hub 统计自环边未防御（修复）**：`ListHubClusters` entry_edges CTE 无自环过滤——M2 抽取若落自指边，UNION ALL 两端使一条自环计 2 度且邻居混入自身。修复：CTE 加 `l.doc_id <> l.target_doc_id`。
- **误报排除（亲验依据，防重复上报）**：①workspace scope 蒸馏事实非死信——`UpsertFactRow` 落库即 `syncFactIndexBestEffort` 异步同步 L4 向量索引，L3 召回默认 scopes `["agent","user","team","workspace"]` 且 scope_id 与集合 workspace 同源（同经 LookupWriteBackHome）；②dream dry_run 传导正确（两分支分别传 `DryRun: true/false`）；③Hebbian 无震荡——复活语义 `valid_to=NULL`+残留权重+0.1 构成滞后环；④orphan 度含 co_activated 自洽——decay 关闭长期不用的共激活边，临时关联不永久豁免；⑤ListHotDocuments 不过滤 inbox 无碍——access_log 唯一写入方在检索召回后（inbox 已被检索层排除不产生 log），runDistillTask 另有 `entries/` 前缀二次过滤兜底。

测试：biz `curate_test.go` 新增 4 例（多库枚举过滤/无团队库 NotFound/指定集合退化单库不查列表/未接线 Unavailable）+ data `knowledge_curate_test.go` 2 例改写（无向边对：反向并存 4→3、自环 seed 断言度数与邻居不受污染）。回归：全量编译 + biz/knowledge（22 例）+ memory_butler（11 例）+ data Knowledge PG 集成（9 例）全绿。

### 深度检查补丁纪要（2026-08-16 第四轮）

第四轮换角度审查（生命周期级联 / cron 编排与异步竞态 / L3 注入链实证），代理 9 条线索亲验后确认 1 缺陷 + 1 低风险，全部修复；另实证闭环第三轮遗留推断：

- **P2 docMove 治理归属错乱（治理黑洞，修复）**：`MoveDocument` 事务原只更新 documents/chunks 的 collection_id + 两集合计数器，5 张附表滞留旧集合——links 滞留使 decay（按 links.collection_id 过滤）归属错乱、moc 簇归属错乱；access_log 滞留使 ListHotDocuments 旧集合蒸馏已移走文档、事实 scope 写错 workspace；doc_entities/relation_state/fact_version 同滞留。修复：事务内补 5 条 UPDATE——**links 仅迁源端**（`doc_id = 本文档`；边 collection_id 语义即「源文档集合」，入边 target 端不动，跨集合边保留在源集合治理域），access_log/doc_entities/relation_state/fact_version 按 doc_id 随迁。
- **低风险 access_log 无 FK 残留（修复）**：`knowledge_access_log` 是唯一无 FK CASCADE 的附表，文档删除后 log 残留垃圾行（INNER JOIN 过滤无功能影响但长期累积）。修复：`DeleteDocument` 事务内显式 `DELETE FROM knowledge_access_log WHERE doc_id = $1`。
- **L3 注入链实证闭环（第三轮推断确认）**：召回侧 `RecallL3Fused → L3ScopeTargets(rt.Workspace) → RecallL3Hits(scope_id=session.Workspace)`（memory_l3_fused_recall.go），写入侧 distill `ScopeID=homeCol.Workspace`——同 workspace 闭合；集合 workspace 空时 distill 跳过防御（curate.go）杜绝空 scope 死信。**distill 事实可召回，链路成立**。
- **误报排除（亲验依据）**：①删除级联缺失——links/chunks/doc_entities/relation_state/fact_version 全部 FK `ON DELETE CASCADE`（knowledge.go 建表语句）；②cron 无单实例锁——lease 机制（cron.lease_skip）+ dead_letter/retry/panic recover；③异步 ctx 请求取消——syncFactIndexBestEffort 用 `context.WithoutCancel` detached + Safego recover + reconciler 兜底；④Hebbian 丢更新——`weight_f = knowledge_links.weight_f + EXCLUDED.weight_f` 原子 SQL；⑤reconciler vs distill 踩踏——同键幂等 upsert 无踩踏语义。

测试：data `knowledge_curate_test.go` 新增 2 例（`TestKnowledgeRepo_MoveDocument_AttachedTablesFollow`：documents/chunks/links 源端/access_log/doc_entities/relation_state/fact_version 随迁 c2 + links 入边留 c1 + 两集合计数器 0/0→1/2；`TestKnowledgeRepo_DeleteDocument_AccessLogPurged`：log 清除 0 残留）。回归：data Knowledge PG 集成全量（11 例）全绿。

---

## M3 落地纪要（实施偏差记录）

- **M3.1 supersedes 版本链（增量叠加，不改主流程）**：迁移 `20261222_knowledge_fact_version.sql` 建 `knowledge_fact_version`（旧段快照，fact_id 可空）+ `knowledge_governance_proposal`（治理提案，status 默认 pending）两表，fresh 形态同步 `EnsureKnowledgeSchema`。biz 端口 `FactVersionRepo`/`GovernanceProposalRepo` 定义在 `evolution.go`，经 `NewKnowledgeUsecase` 内 repo 类型断言接线（`SetEvolutionRepos`，断言失败保持未接线降级）。留痕点在 `upsertEntryDoc`：fact_id 整段替换生效（`nb != body`，幂等重放不留痕）时收集 `versions`，**正文持久化成功后**才 best-effort 落库（`recordFactVersion` 失败仅 Warn 不回滚）；`oldBody == newBody` 与空 oldBody 双保险跳过。
- **M3.2 写入时冲突检测**：`internal/knowledge/writeback_arbiter.go` 实现 `WriteBackArbiter`（LLM 批量仲裁，单次调用、60s 超时、existing≤20/news≤10/段 300 符文截断控 prompt）。触发条件从严：仅存量页 + 有待追加事实 + 页内抽取到带 fact_id 段（`extractFactBlocks`）才仲裁；新建页不仲裁。裁决分两档置信度门槛——`supersedes ≥0.8` 走版本链顶替目标段（新事实不再追加）；`contradicts ≥0.7` 旧段不覆盖、新事实仍追加留痕 + 高风险提案（kind=conflict/risk=high，payload 含新旧 fact_id/陈述/理由）待人工二审。仲裁器未接线/LLM 失败/低置信一律降级原追加行为。生产装配 `provideKnowledgeWriteBackArbiter`（Set 回注 Usecase，与 curator SetGate 同模式），以 `provideAutoMemoryWorker` 末位锚点形参保证 wire 图到达；环境开关 `KNOWLEDGE_WRITEBACK_ARBITER_DISABLED=1`。**wire_gen.go 手工同步**：provider 定义 + 锚点形参两处补齐（wire.go 整文件 wireinject tag，普通构建不可见）。
- 测试：`writeback_evolution_test.go`（9 例：版本链留痕/幂等跳过/未接线降级/supersedes 顶替+低置信追加/contradicts 提案+低置信跳过/仲裁失败降级/新建页不仲裁/未接线 Legacy 行为）+ `writeback_arbiter_test.go`（7 例：裁决解析/代码围栏容忍/空输入零 LLM/LLM 错误上抛/坏 JSON/未接线/候选截断）+ `knowledge_evolution_test.go`（PG 集成 4 例：fresh 形态/迁移幂等/版本链往返含 NULL fact_id/提案 JSONB 往返+默认 risk）。PG 测试需 `ARANEA_TEST_PG_DSN`。

---

## M2 落地纪要（实施偏差记录）

- **M2.1 entity 轨复活**：新增 `internal/knowledge/entity_pipeline.go`——`EntityPipeline.ProcessDoc` 复用 M2.2 Step1 的 `llmExtractEntities`（重构为包级共享函数）→ `ReplaceDocEntities`（name_norm 归一化/别名路由）→ `FindEntityCooccurrences`（R-3 频次过滤 `entityMaxDocFreq=50`）→ `ReplaceEntityLinks`（context=共享实体名、weight=共享数、自环跳过）。幂等状态落 `knowledge_relation_state.entities_extracted_at`（与关系轨同表分列；`UpsertRelationState` 零值时间 `COALESCE` 不动既有列，双轨互不踩踏）。生产接线在 `provideVaultSyncSupervisor`：`SetEntityHook` + `safego.Go(appctx.Ctx(), "knowledge.entity_pipeline", …)` 异步（不阻塞索引主路径），环境开关 `KNOWLEDGE_ENTITY_PIPELINE_DISABLED=1`。
- **M0 欠账补清**：同一 provider 内 `applier.SetCompiler(knowledge.NewBodyCompiler(registry))`，registry 复用 `service.NewKnowledgeExtractorRegistry`（wire_gen 已有实例直接传参）；此前 `SetCompiler` 全仓无生产调用者，office/图片在 vault 同步链恒降级 error。
- **M2.2 关系抽取**：`relation_extract.go` 两步 LLM（实体→三元组），谓词归一化核心闭集、词表外落 `vocab candidate`；宾语经 `docKeyIndex`（basename/title/aliases 多键、歧义跳过）解析为同库文档；confidence<0.7 边写入即关闭（`valid_to=now` 留痕）。后台 worker `KnowledgeRelationExtractWorker` 只对热文档（`knowledge_access_log` 命中 ≥ 阈值）抽取，按 `content_hash` 幂等。
- **M2.3 DDL**：迁移 `20261221_knowledge_relation_vocab.sql`（vocab + relation_state 两表，预置 core 谓词 ON CONFLICT DO NOTHING）；fresh 形态同步 `EnsureKnowledgeSchema`。
- 测试：`entity_pipeline_test.go`（6 例：写边/幂等跳过/变更重抽/空正文/LLM 失败/未接线）+ `relation_extract_test.go` + `knowledge_relation_test.go`（PG 集成）+ worker 单测。PG 测试需 `ARANEA_TEST_PG_DSN`。

---

## M1 落地纪要（实施偏差记录）

- **M1.1 DDL**：迁移 `20261220_knowledge_links_bitemporal.sql`（SQL 文件式，非 Func 式）；`relation` 定 `NOT NULL DEFAULT ''`（表达式唯一索引会破坏既有 `ON CONFLICT` 列推断，弃用 COALESCE 方案）；`valid_from/recorded_at` 先加可空列回填 `created_at` 再 `SET NOT NULL`（PG11+ 带 DEFAULT 的 ADD COLUMN 直接填充存量行，`WHERE IS NULL` 回填永不命中）。fresh 形态同步进 `EnsureKnowledgeSchema`。
- **M1.2 base-level**：端口 `bizknowledge.AccessLogRepo`（`access_log.go`）；注入点定在 `AdaptiveRouter.Search` 返回前（service 与 tools 两条生产检索路径的唯一收敛点）；先算历史加成再记本次命中（防循环自激）；β=0.1 wire 接线。
- **M1.3 Hebbian**：端口 `bizknowledge.CoActivationRepo`；无向边规范化 `doc_id<target_doc_id` 单行，`ON CONFLICT weight_f += η` 且复活 `valid_to`；ghost 端点跳过防 FK 拖垮整批；router 内 `safego.Go` + `context.WithoutCancel` 异步。
- **M1.4 扩散激活**：`GraphExpander.SetActiveLinks` 接线后走 2 跳 BFS（能量 ×0.5/跳，类型权 explicit1.0/semantic0.9/entity0.7/co_activated0.4，侧抑制 top-10），未接线保持旧 1 跳；`ListActiveLinks` 只读 `valid_to IS NULL`。
- 测试：`knowledge_bitemporal_test.go` / `knowledge_access_log_test.go`（PG 集成）+ `access_boost_test.go` / `graph_expander_spread_test.go`（单测）。PG 测试需 `ARANEA_TEST_PG_DSN`（本机 `postgres://postgres:123456@127.0.0.1:5432/aranea_test`）。

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
