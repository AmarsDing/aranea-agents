# 开源知识库工程后端业务逻辑调研：SiYuan 深度专题 + Logseq/AFFiNE/Joplin 对比

> **类型**：research | **日期**：2026-08-11 | **触发**：用户要求「深入分析后端业务逻辑能否支撑需求，调研可靠开源工程学习」
> **调研对象**：SiYuan 思源笔记（Go 内核，重点）、Logseq、AFFiNE、Joplin、Outline（可选参照）
> **验证状态**：所有结论均来自实际阅读的 GitHub 源码/官方文档，读不到的细节标注「未能确认」。本轮阅读的源码缓存于 `test/oss-research-kb/`。

---

## A. 各工程后端核心设计摘要

### A.1 SiYuan 思源笔记（github.com/siyuan-note/siyuan，Go）

深度专题见 B 节，此处为速览：

- **数据模型**：文件系统为真相源，每个文档是一个 `.sy` 文件（JSON 化的 Kramdown AST 树），块 ID 写入 AST 节点的 IAL 属性（`id`/`updated`），块与文件的关系是"文档=根块，子块挂在树下"。出处：`kernel/treenode/tree.go`、`kernel/model/file.go`。
- **索引**：单 SQLite 库（`sqlite3_extended` 驱动，注册 `regexp` 函数），核心表 `blocks/refs/attributes/spans/assets/block_embeddings`，另有独立 blocktree 库存内存映射。出处：`kernel/sql/database.go`。
- **同步/增量**：编辑产生的变更进入**持久化索引队列**（落盘，崩溃后 `recoverIndexQueue` 恢复），批量 flush；启动时校验数据库结构版本号决定是否全量重建。出处：`kernel/sql/database.go:88-128`。
- **链接解析**：**写入时物化**——解析 AST 时把块引（`TextMarkBlockRefID`）写入 `refs` 表；反链=按 `def_block_id` 查 `refs` 表，无查询时遍历。出处：`kernel/model/index.go IndexRefs`、`kernel/model/backlink.go`。
- **搜索**：FTS5 虚表 + `regexp` + 关键词过滤 + 类型过滤的多段管线；`[[` 补全按"最近引用时间"排序。出处：`kernel/model/search.go SearchRefBlockInBox`。
- **语义检索**：`block_embeddings` 表存 BLOB 向量，Go 侧算余弦 + min-heap 取 TopN；带 fail_count 退避熔断。出处：`kernel/model/embedding.go`。

### A.2 Logseq（ClojureScript + Datascript）

- **数据模型**：内存 Datascript 图库（EAV 三元组），文件（Markdown/Org）是真相源，启动时经 graph-parser 解析后整体 transact 进 Datascript。新版 DB 图 schema：`:block/uuid`（unique identity）、`:block/parent`（ref+index）、`:block/page`（ref+index）、`:block/refs`（**ref, cardinality many**）、`:block/alias`、`:block/name`（小写索引）、`:block/title`、`:block/journal-day`、`:file/path`（unique）等。出处：`deps/db/src/logseq/db/frontend/schema.cljs`（schema version 65.33）。
- **链接解析时机**：**提取时（extract/transact 时）物化**。`extract-pages-and-blocks` 解析 AST 产出 pages+blocks 后，用 `with-block-refs` 以正则 `block-ref-pattern` 匹配 `((uuid))` 提取块引用写入 `:block/refs`；页面引用 `[[wikilink]]` 与属性中的引用在属性提取阶段一并处理。出处：`deps/graph-parser/src/logseq/graph_parser/extract.cljc`、`block.cljs`。
- **反链表达力**：Datascript 对 ref 类型属性自动维护反向索引（VAET），反链即对 `:block/refs` 的反向 Datalog 查询；Datalog 的递归规则对 outliner 树和图遍历表达力极强。具体反链查询函数所在文件未能确认，但 schema 层面的反向索引能力已确认。
- **同步**：文件监听变更→重解析单文件→增量 transact（`:file/path` 定位实体）；无服务端，同步依赖外部（iCloud/Git 等）。
- **对我们的启示**：图查询表达力来自"引用作为一等实体物化 + 双向索引"，而非 Datalog 本身；这套语义在 PG 里用一张 links 表 + 两个方向的 B-tree 索引即可等价实现。

### A.3 AFFiNE（TypeScript + BlockSuite/Yjs + NestJS + Postgres）

- **块模型与 CRDT**：BlockSuite 的 `@blocksuite/store` 以 Yjs 为唯一真相源：块树 = `Y.Doc`，块属性 = `Y.Map`，子块顺序 = `Y.Array`，富文本 = `Y.Text`；块由 schema 定义（`flavour`/`role`/`parent`/`children` 约束）。Page/Edgeless 双模式共享同一 Y.Doc。出处：blocksuite.io 官方 Framework Overview。
- **服务端存储（关键设计）**：服务端把 Yjs 文档当**不透明更新日志**存 Postgres——`PgWorkspaceDocStorageAdapter` 追加 updates 行，定时任务 `DocServiceCronJob.mergePendingDocUpdates` 将多条 update squash 成 snapshot（`Y.mergeUpdates`），随后删除已合并 updates；历史版本由 `HistoryModel` 按 `historyMinIntervalMs/historyMaxAgeSeconds` 约束生成。出处：PR toeverything/AFFiNE#15282、issue #15283。
- **索引与搜索分层**：**事件驱动索引**——snapshot 合并后 EventBus 派发 `doc.snapshot.updated`，`IndexerEvent.indexWorkspace` 监听并重建搜索索引，`DocEventsListener.markDocContentCacheStale` 失效缓存；AI 侧另有 `copilot.embedding` 任务与 `cleanupTrashedDocEmbeddings` 清理孤儿向量。即：内容索引发生在 snapshot 物化点，而非每次 CRDT update。
- **悬挂引用**：删除被引文档后，源文档中的 `affine-reference` 节点保留但 `refMeta` 为空（dangling 不删源内容、渲染态降级）。出处：PR #15282 `editor-semantics.spec.ts`。
- **同步**：Yjs 生态 provider（IndexedDB 本地 + WebSocket 远端），天然 CRDT 冲突合并；服务端不解析内容语义，只做存储/广播/压缩。

### A.4 Joplin（TypeScript，SQLite 客户端 + 多后端同步）

- **同步引擎**：以"item"（note/folder/tag/resource）为同步单位。`Synchronizer.ts` 泛型主流程，`SyncTarget*`/`file-api-driver-*` 适配各后端。**变更即传**（改动几秒内上传以减少冲突）+ 定期轮询下载。删除会传播到服务端再到其他设备。出处：`readme/dev/spec/sync.md`。
- **同步状态表**：`sync_items` 表按 `(item_id, sync_target)` 记录 `sync_time`，据此判定待同步集合；`sync_disabled` 处理被云端拒绝的条目；`info.json` 存同步目标级共享属性，每个属性带 `updatedTime` 做 LWW 冲突裁决。
- **版本历史（后端设计精华）**：`RevisionService` 从 `item_changes` 表按 `lastProcessedChangeId` **游标批量消费**变更（跳过来自同步/解密的变更避免回声），为笔记生成**差分修订链**——`title_diff`/`body_diff` 用 diff-match-patch 补丁、`metadata_diff` 用对象补丁，经 `parent_id` 串成链；`intervalBetweenRevisions` 节流；对"老笔记"先补基线快照；删除笔记时用 `before_change_item` 存最终版本；TTL 清理；恢复=`mergeDiffs` 沿链合并后导入"Restored Notes"文件夹。出处：`packages/lib/services/RevisionService.ts`（已读全文）。

### A.5 Outline（Postgres 系参照，可选）

- 栈确认为 Node + Sequelize + Postgres，协作文档+集合+修订（Revision 模型）。
- 其全文搜索的具体实现本轮**未能确认**——`Document.ts` 过大被截断，未读到 `searchForTeam` 方法体；不作为依据。
- 结论：我们项目的 PG 侧实现仍以自身已有的 `to_tsvector + GIN + pg_trgm` 规范为准。

---

## B. SiYuan 深度专题：块级双链与索引机制

SiYuan 与我们最像：Go 内核、文件系统真相源、块级双链。以下全部基于本轮实际阅读的 `kernel/` 源码。

### B.1 块模型：ID 与内容寻址双轨

- **文件格式**：文档 = `.sy` 文件，内容是 JSON 序列化的 AST 树（根为 Document 节点）。块属性存于 Kramdown IAL（`{: id="..." updated="..."}`）。读不到属性的文件被视为损坏并**移入 `workspace/corrupted/` 隔离**而非报错中断（加密笔记本解密失败时豁免，防止误丢数据）。出处：`kernel/model/file.go:108-154`。
- **块 ID 生成**：`ast.NewNodeID()` = **14 位时间戳 + 7 位随机字符**（如 `20260101120000-ab3x9kz`）。ID 前 14 位即创建时间，`util.TimeFromID(id)` 直接从 ID 解析时间回填 `updated` 属性。**ID 是固定身份，不是内容寻址**：块改名/改内容 ID 不变，引用永不失效。
- **块哈希（变更检测用）**：`NodeHash()` = 对 `box + path + hpath + IAL(排序后) + markdown + parentID` 取 sha256 的前 7 位十六进制。哈希用于索引时判断块是否变化（`blocks` 表有 `hash` 列，复合索引 `idx_blocks_root_id_id_hash(root_id, id, hash)`），**"固定 ID 管引用、内容 hash 管增量"是双轨制核心**。
- **块与文件关系**：文档=根块（root_id 指向文档 ID），所有子块 `blocks.root_id = 文档 ID`、`parent_id` 指向父块；文件名必须是块 ID（否则拒收）。

### B.2 索引架构：SQLite 表设计 + 持久化队列 + 重建机制

**表结构**（`kernel/sql/database.go initDBTables`，已逐行确认）：

| 表 | 列 | 说明 |
|---|---|---|
| `blocks` | id, parent_id, root_id, **hash**, box, path, hpath, name, alias, memo, tag, content, fcontent, markdown, length, type, subtype, ial, sort, created, updated | 块主表；索引：id / parent_id / root_id / (root_id,id,hash) |
| `refs` | id, **def_block_id**, def_block_parent_id, **def_block_root_id**, def_block_path, **block_id**, **root_id**, box, path, content, markdown, type | 双链表 |
| `attributes` | id, name, value, type, block_id, root_id, box, path | 块属性 |
| `spans` | id, block_id, root_id, box, path, content, markdown, type, ial | 行内元素 |
| `assets` | id, block_id, root_id, box, docpath, path, name, title, hash | 资源引用 |
| `block_embeddings` | id PK, root_id, box, path, **embedding BLOB**, model, content_len, updated, **fail_count, last_tried, ignored_type** | 向量+熔断字段 |
| `file_annotation_refs` | file_path, annotation_id, block_id, root_id... | PDF 标注引用 |
| `stat` | key, value | 含数据库结构版本号 |

另有 FTS5 虚表（content/markdown 全文）。`regexp` SQL 函数通过 sqlite3 驱动 ConnectHook 注册。

**写入路径（双库）**：解析出的树同时进两处——`treenode.IndexBlockTree(tree)` 写内存块树库（支撑 ID→(box,path) 快速定位），`sql.IndexTreeQueue(tree)` 进 SQLite 索引队列。

**队列与重建**：
- 索引写入走**持久化队列**：`initIndexQueue()` 初始化、`recoverIndexQueue()` 在启动时恢复未消费的条目——崩溃不丢索引任务。
- 数据库结构版本号存 `stat` 表；版本不一致→整库 DROP 重建；版本一致但缺新列→幂等列迁移（明确注释"不升版本号避免全库重建丢失已嵌入向量"）。
- 全量重建 `indexBox()`：列出全部 `.sy` → ants 协程池（`min(NumCPU, 4)`）并行 `LoadTree` → 逐树索引 → 批间推进度条。

### B.3 双链实现（核心）

**引用解析时机 = 写入时物化，查询时零解析**：

1. **编辑/索引时**：解析 AST 时识别块引节点——`TextMark` 类型为 `block-ref`（带 `TextMarkBlockRefID`）或 `a` 且 href 为 `siyuan://blocks/<id>`，upsert 进 `refs` 表：源块 `block_id`、目标块 `def_block_id`，并**冗余存储** `def_block_root_id/def_block_path/root_id/content/markdown`，使反链查询无需回表。
2. **启动全量引用解析 `IndexRefs()`**：遍历所有 `.sy` 原始字节，先做**字节级预检** `bytes.Contains(data, "TextMarkBlockRefID")`——不含引用特征串的文件直接跳过解析（大库启动的关键优化）；命中才解析、AST Walk 收集含块引的文档，再逐文档重建其引用边。
3. **反链计算 = 纯索引查询**：`GetBacklinkDoc(defID)` → 查 refs 表 → 加载源文档树渲染 DOM（含面包屑、按文档内顺序排序）。无缓存表——每次实时查 refs 表（SQLite 索引足够快）；**引用计数**则异步维护（延迟去重刷新）。
4. **级联更新 `RefreshBacklink(id)`**（编辑/改名/移动后调用）→ 查该 defID 的全部引用 → 收集源 rootIDs 去重 → 只重索引这些树的引用行 + 异步刷新计数。即"以被引块为轴心，反向刷新所有引用源文档的 refs 行"，级联范围精确到受影响的文档集。
5. **Unlinked mentions**：`buildTreeBackmention()`——用关键词全文搜索找候选提及块，排除已是正式引用的块，取上下文前后 12 字符，命中词高亮。即"提及=搜索候选 − 已链接"。
6. **悬空引用**：`ListInvalidBlockRefs()` 全库扫描对比 refBlockMap 与现存块，找出目标已不存在的引用（dangling）供用户治理。
7. **`[[` 补全排序**：空关键词时按"最近引用优先"——内存中 `refUsed`（defBlockID→最近使用时间戳）精确排序，截断 32 条。

### B.4 搜索管线

多段组合：FTS5 关键词候选 → `regexp` 过滤 → 类型/子类型过滤器 → 大小写敏感选项 → 路径/笔记本范围裁剪。文档搜索单独走 name/alias/memo 字段 + 精确匹配优先排序。嵌入块（query_embed）允许直接执行用户 SQL（vitess sqlparser 校验），把"搜索即查询"开放给高级用户。

### B.5 向量嵌入与熔断

- 后台批处理（每批 100，`ORDER BY fail_count ASC, updated DESC`——**失败少的优先**）。
- **精确到块的退避**：按失败次数指数退避，SQL 粗筛 + Go 侧按每块 fail_count 精确判定；失败即 fail_count+1、写空向量、本轮熔断并提示用户。
- 成功整行重写并复位熔断字段；`RetryFailedEmbedding` 只清失败块不动已成功向量。
- 相似度：向量以 `[]float32` 编码 BLOB 存储，查询时全量解码 + 余弦 + min-heap 取 TopN（单机库量级可接受，但这是我们用 pgvector 要规避的点）。

### B.6 文件同步、历史与隔离

- **文件监听**：SiYuan 假定自己是 `.sy` 唯一写入方（编辑器经 API 写入后内核主动标脏重索引），未见 fsnotify 对 .sy 数据目录的监听。外部挂载与启动索引兜底外部变更。
- **历史**：**文件级快照**（非 diff 链）——定时为"近期修改的文档"生成历史副本，按保留策略清理；支持历史全文搜索、四类回滚，且显式防路径穿越。
- **加密笔记本隔离**：boxID→独立 *sql.DB 路由，**fail-closed**：未解锁时绝不回退全局明文库，避免索引污染；所有查询都有按 box 路由的变体。
- **损坏数据**：解析失败文件复制后移入 `workspace/corrupted/<时间戳>/`。

### B.7 关键文件清单

| 主题 | 文件 |
|---|---|
| 表结构/队列/版本 | `kernel/sql/database.go` |
| 块树内存库/NodeHash | `kernel/treenode/tree.go`、`blocktree.go` |
| 索引/引用解析 | `kernel/model/index.go` |
| 反链/提及 | `kernel/model/backlink.go` |
| 搜索/悬空引用/补全 | `kernel/model/search.go` |
| 向量 | `kernel/model/embedding.go` |
| 历史 | `kernel/model/history.go` |
| 文件/损坏隔离 | `kernel/model/file.go` |

---

## C. 可借鉴清单（映射到 Go + Postgres + Ent + pgvector + 文件系统真相源）

| # | 借鉴自 | 模式 | 解决我们什么问题 | 落地模块 |
|---|--------|------|------------------|----------|
| 1 | SiYuan `refs` 表 | 引用边物化表：`(def_block_id, block_id, root_id)` + 冗余 `content/root_path`，双向各建索引 | 块级双链的反链查询变为纯 SQL 索引查询，dangling 检测 = `LEFT JOIN blocks IS NULL`；避免查询时解析 markdown | data 层（新增 links/refs Ent Schema）+ 解析管线物化 |
| 2 | SiYuan ID/hash 双轨 | 块 ID 固定（时间戳+随机，ID 即创建时间可解析），内容 sha256 短哈希做变更检测 | "移动识别保留身份"：移动只改 path 不改 ID，引用不断；hash 决定是否需要重建索引/重生成摘要卡 | data 层（blocks 表加 content_hash 列）+ 同步管线 |
| 3 | SiYuan 持久化索引队列 | 索引任务落盘队列 + 启动 recover + 批量延迟 flush | 文件监听事件风暴下的写放大削峰；进程崩溃不丢待索引集合 | 管线（ingest worker）+ data 层队列表（PG 表替代磁盘队列） |
| 4 | SiYuan 变更日志 + Joplin 游标 | 变更日志表 + 每消费者 `lastProcessedChangeId` 游标 | 文件增删改→索引/摘要卡/embedding/链接物化多个下游各自独立消费、互不阻塞、可独立重放；与项目 AS-EVT-01 事件分级天然契合 | data 层 change_log 表 + 管线各 worker |
| 5 | SiYuan `IndexRefs` 字节预检 | 全量扫描时先 `bytes.Contains` 特征串再决定是否解析 | 挂载大库的启动/全量重建成本数量级下降 | 管线（file scan 阶段） |
| 6 | SiYuan `RefreshBacklink` | 以被引块为轴心反向刷新：查 defID 全部引用→去重源文档集→只重索引受影响文档 | 文档改名/删除时引用级联更新的影响面最小化，避免全库重扫 | biz（关联服务）+ 管线 |
| 7 | SiYuan unlinked mentions | 提及 = 全文搜索候选 − 已链接集合，带上下文截断 | 我们的 unlinked mentions 需求可直接复用此定义与算法（已实现 P2-7，算法同源） | 已实现（2026-08-11 增强轮） |
| 8 | SiYuan `[[` 补全排序 | 空查询按"最近引用时间"排序候选（refUsed 时间戳） | 编辑器 wikilink 补全体验；实现只需一张 `link_used(def_id, last_used_at)` 小表 | data + biz |
| 9 | SiYuan `block_embeddings` 熔断 | `fail_count/last_tried` 按块指数退避 + 失败写空向量 + 成功复位 + 只重试失败块 | embedding/LLM 摘要卡生成失败的退避与治理，防止坏块反复打爆外部 API | data（embeddings 表加熔断列）+ 管线 |
| 10 | SiYuan 损坏隔离 | 解析失败文件移入 `corrupted/` 目录而非报错中断或删除 | 挂载目录存在乱码/非 md 文件时的鲁棒性；配合回收站语义 | 管线 |
| 11 | Joplin RevisionService | 修订 = diff-patch 链（parent_id 串联）+ 间隔节流 + 老文档基线快照 + 删除前留终版 + TTL 清理 + 恢复到独立位置 | 回收站/版本历史实现路线：文档级历史用 diff 链省空间；"删除进回收站不物理删"= 删除时存终版 + 索引剔除 + tombstone | data（revisions 表）+ biz |
| 12 | Joplin `sync_items` | `(item_id, target, sync_time, sync_disabled)` 幂等同步状态 | "文件增删改→索引跟进"的断点续传与幂等：记录 path+hash+indexed_at，重复事件去重 | data + 同步管线 |
| 13 | AFFiNE 事件驱动索引 | snapshot 物化点派发事件→Indexer 监听重建，而非每次操作都重建 | 我们的索引刷新挂在事件总线上（Important 级），与文件真相源的"保存点"对齐，避免高频中间态重复索引 | 管线（接 event.Bus） |
| 14 | AFFiNE dangling 处理 | 引用目标删除后源内容保留、渲染态降级 | dangling link 的 UX 语义：不删源文本，查询/渲染时 join 失败即标记 dangling | biz（渲染/查询层） |
| 15 | Logseq 数据模型 | 引用在写入时物化为 cardinality-many 的 ref 属性；反链=反向索引查找 | 与 #1 互相印证"写入时物化"是主流答案；同时警示：内存图库不适合我们服务端多租户场景，PG 表是正确替代 | 架构决策记录（ADR） |
| 16 | SiYuan 加密盒 fail-closed 路由 | 按 boxID 路由独立连接，未解锁绝不回退全局库 | 若未来做租户/密级隔离知识库，这是经过生产验证的隔离范式 | data（连接路由，远期） |

**明确不借鉴的**：
- Logseq 全量内存 Datascript——服务端多租户下内存不可控，我们用 PG 等价实现其图语义。
- SiYuan 的 Go 侧全量余弦计算（B.5）——库量级一大即崩，我们用 pgvector ivfflat 索引（且已有维度对账 `reconcileEmbeddingDim` 机制）。
- AFFiNE 服务端不透明 CRDT 日志存储——导致服务端索引必须等 snapshot 才能解析内容，复杂度高；我们文件真相源直接可读，无此问题。其 update-log + 定期 squash 模式仅在做协同编辑时才需回看。
- Outline 搜索实现——本轮未能从源码确认细节，不作为依据。

**验证状态声明**：以上 SiYuan 结论全部基于本轮实际阅读的 `kernel/` 源码（含建表 SQL、反链/索引/嵌入/历史函数级逻辑）；Logseq 基于实际阅读的 schema.cljs、extract.cljc、block.cljs；AFFiNE 基于实际阅读的 PR #15282 diff、issue #15283 服务日志及 BlockSuite 官方文档；Joplin 基于实际阅读的 sync.md 官方规格与 RevisionService.ts 全文。标注「未能确认」处为 Logseq 反链查询函数具体位置、文件图版 schema 现状、Outline 搜索实现细节三处。

---

## D. 差距矩阵（16 条借鉴清单 × 现状核对，2026-08-11 完成）

核对方法：逐条读现状代码确认（非凭印象）。关键锚点：`internal/data/sql/migrations/20261203_knowledge_blocks.sql`、`internal/biz/knowledge/{link_index,block_pipeline,rebuild_index,sync_engine,link_resolver,backlink,vault_filer,mention}.go`、`internal/knowledge/{vault_sync,vault_sync_runner}.go`、`web/src/features/knowledge/wikilink.ts`。

### D.1 已实现 / 语义等价（7 条，无需动作）

| # | 模式 | 现状证据 | 判定 |
|---|------|---------|------|
| 1 | refs 引用边物化表 | `knowledge_block_refs`（src_block_id/dst_doc_id/dst_block_id/raw_target/context/ambiguous）+ dst_block/dst_doc/raw_target/src_block 四个索引；另有进程内 LinkIndex 五索引内存图（bySrc/byDstBlk/incoming/bySrcDoc/danglingByColl）+ WS GraphDelta 增量推送 | ✅ **已实现且超越**（内存图 + 实时增量是 SiYuan 没有的） |
| 2 | ID/hash 双轨 | 文档级：`HashContent`(sha1) + mtime 预筛复用 + `ChangeMoved` 凭 hash 识别移动保留身份（`sync_engine.go`）；块级：`knowledge_blocks.content_hash` + anchor 部分唯一索引 + 惰性锚点回填 | ✅ 等价实现。设计差异：块 ID 是派生（整文档删了重插），非 SiYuan 式固定身份——因我们的块是派生索引而非用户可引用的一等实体，稳定锚点由 `^anchor` 承担 |
| 3 | 持久化索引队列 | 无队列，但可靠性契约等价：`content_hash` 只在 chunks 索引成功后落库，失败则下轮 Scan+Diff 自动重判变更（`vault_sync.go` 契约注释）；prev 快照失败不保存；30s 轮询天然合并事件风暴 | ✅ 语义等价（轮询模型与 SiYuan 编辑事件模型不同，崩溃自愈成立） |
| 6 | RefreshBacklink 级联 | refs 表冗余字段全部是**源侧**信息（raw_target/alias/context），dst 侧仅 ID 引用——目标改名/改内容不影响已物化边，无需反向刷新；删除由 FK `ON DELETE SET NULL/CASCADE` 处理 | ✅ 设计上规避（SiYuan 需要它是因为冗余了 dst 侧 path/content） |
| 7 | unlinked mentions | `mention.go` 已实现：全文候选 − 已链接集合 + 上下文窗口截断，与 SiYuan `buildTreeBackmention` 算法同源 | ✅ 已实现 |
| 12 | sync_items 幂等状态 | `documents.content_hash` + `status` + 「索引成功才落 hash」契约 + 幂等短路（hash 一致且 indexed 直接跳过） | ✅ 等价实现（PG 镜像表即同步状态表，无需独立 sync_items） |
| 14 | dangling 渲染降级 | dangling 边保留 raw_target（复活线索）+ `danglingByColl` 聚合「未创建笔记」视图 + 目标创建并索引后自动复活；源内容从不删除 | ✅ 已实现且超越（聚合视图是 AFFiNE 没有的） |

### D.2 真实差距（4 条，建议排期）

| # | 模式 | 现状缺口 | 影响 | 建议落地 |
|---|------|---------|------|---------|
| 9 | embedding 熔断退避 | `buildChunks` Embed 失败 → 整文档 `markError` → 30s 后整文档重试，**无 fail_count / 无退避 / 无写空向量跳过**。embedder 持续故障（key 失效/额度尽）时每个轮询周期对全部失败文档重打 API | **运维风险最高**：故障期间持续打爆外部 API，日志刷错误，且阻塞该文档的词法索引（chunks 与 embedding 同事务，embed 失败 = 词法也不落库） | **P0**。knowledge_documents 加 `embed_fail_count`/`embed_last_tried` 两列；embed 失败时降级写 NULL 向量让词法索引先落库（对齐 NFR-15 无语义层可用）；指数退避跳过；恢复后首轮全量重试。参考 SiYuan `ORDER BY fail_count ASC` |
| 4 | 变更日志+游标多消费者 | 下游是同步链 + hook：块索引失败仅 Warn 降级而 content_hash 已落库 → **下轮不再重试**，只能靠手动 `RebuildCollectionBlockIndex` 收敛；实体 hook 同理 | 块级双链/实体在失败后**长期静默滞后**，用户无感知 | **P1**。不建完整 change_log（YAGNI），做轻量收敛：sync 轮询增加「docs with missing block index」检测（`documents.status='indexed' AND blocks 缺失` 的廉价 LEFT JOIN 校验，抽样或低频执行），或对 rebuildBlockIndex 失败文档落 `index_degraded` 标记下轮重试 |
| 5 | IndexRefs 字节预检 | `RebuildCollectionBlockIndex` 每篇都 GetDocument + 全量 Parse（容错解析成本全付），无特征串预检 | 大库全量重建耗时线性偏长（解析是无引用文档的纯浪费） | **P2**。重建前 `strings.Contains(body, "[[")` 预检，无引用文档只刷块不跑 Resolver；一行改动 |
| 8 | `[[` 补全最近引用排序 | wikilink 候选按路径/字母序（前端 `wikilink.ts` 无 recency 逻辑），无 `link_used` 表 | 高频链接文档每次都要重新输入过滤，体验差距 | **P2**。`knowledge_link_used(doc_id, last_used_at)` 小表，wikilink 落链时 upsert；补全空查询时按最近使用排序（前端 candidates 排序键） |

### D.3 有意不借鉴 / 需求外（5 条）

| # | 模式 | 结论 |
|---|------|------|
| 10 | corrupted 隔离 | **有意不借鉴**。SiYuan 移 corrupted 因它是 `.sy` 唯一写入方；我们是只读挂载哲学（不动用户文件），降级 = 跳过 + 下轮重试 + 删除抢救进 trash，已够 |
| 13 | 事件驱动索引 | **部分采用**。物化挂在同步点（embedding 同步执行 = 检索立即可用的设计选择；摘要/实体已 hook 异步化）；WS GraphDelta/ingest 事件/流程日志已有。同步 embedding 的阻塞问题由 D.2#9 的熔断降级覆盖，无需改事件总线 |
| 11 | Joplin diff 修订链 | **需求外**。V2 需求无版本历史（NFR-17 只要求 trash 不丢数据）；未来做版本历史时按 Joplin RevisionService 全套（diff 链 + 游标 + TTL）实施 |
| 15 | 写入时物化 ADR | **文档任务**。「写入时物化 + 内存图」vs「Logseq 内存图库」的选型已在代码注释和设计中论证，缺一份正式 ADR；随下个实施项顺带补 |
| 16 | fail-closed 租户路由 | **远期**。US-8 多租户未实现；实施时按 SiYuan boxID 路由范式（独立连接、未解锁绝不回退全局库） |

### D.4 结论

**后端业务逻辑整体判断：能支撑已定需求，且架构方向与标杆一致**。「文件系统真相源 + 写入时物化 + 派生索引可重放」三大主线与 SiYuan/Logseq/AFFiNE 的生产验证答案同源；双链/反链/dangling/unlinked mentions/移动保留身份/删除救援等核心语义已全部落地。缺口集中在**失败治理**（embedding 熔断 #9、下游自动收敛 #4）而非功能缺失——这正是 SiYuan 在 `block_embeddings` 熔断设计上投入最重的地方，也是我们下一迭代最值得抄的作业。
