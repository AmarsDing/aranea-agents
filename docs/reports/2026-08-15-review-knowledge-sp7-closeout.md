# 评审报告：知识模块 SP7 收口后的整体深度审查（2026-08-15）

> 日期：2026-08-15 | 类型：review | 状态：对照代码核验，不以三件套自称当事实
> 对象：模块 37 Knowledge（L1 成链 / L2 一跳 / L3 Agent 注入 / L4 写回 + 本轮 US-38~US-44）
> 前序：[2026-08-15-review-knowledge-frontier-gap.md](./2026-08-15-review-knowledge-frontier-gap.md)、[2026-08-15-research-knowledge-synthesis.md](./2026-08-15-research-knowledge-synthesis.md)

---

## 1. 先给结论

本轮把综合方案 §6 列出的剩余切片都接到了代码上：**历史成链回填、合成金标、G1 投影、G7 专家、G8 健康度、低置信写回确认、本页确认成链、搜索引用来源条**。

它仍然不是 Agentic RAG，也不是 Microsoft LazyGraphRAG / HippoRAG 2 的 PPR 或社区摘要。更准确的一句话：

**写时把标题提及编成 `[[wikilink]]`，查时沿 explicit 边走一跳，会话高置信事实进团队日记，中置信进 pending，Agent 的 L3 活动事实覆盖写成一篇只读 Markdown。**

用户可感知的增益取决于两件事：库里是否已经有可匹配的**文件名标题**，以及你是否打开的是**写回所在的团队库**（个人 local vault 上看不到 G1/G7）。

---

## 2. 对照「剩余任务」：声称 vs 代码

| 任务 | 文档故事 | 代码是否真接线 | 诚实边界 |
|------|----------|----------------|----------|
| 历史回填 | US-38 | `RebuildKnowledgeIndex` 异步 goroutine 里**先** `BackfillOutgoingAutolinks` 再 `RebuildCollectionBlockIndex` | 全量重建函数本身仍 `allowBackfill=false`；回填是重建 RPC 的附加写路径。点重建会改存量 Markdown。Watcher / 外部编辑器仍不成链 |
| 确认成链 | US-39 | custom GET/POST + 反链面板按钮 + ⌘K | ingest/保存路径**仍然自动成链**，没有确认。确认只覆盖「本页编译」 |
| 检索金标 | US-40 | `TestGoldRecall_AutolinkPlusOneHopBeatsHybridSeed` 12 条中英查询，测试已绿 | **不是** V6 S-5 的 50 条 BM25×Postgres。语料内嵌、假 `GraphExpander`。证明的是「成链 → 一跳」，不是混合检索质量 |
| citation 面板 | 综合方案曾写「金标 + citation」 | 全库搜索浮层顶部列出命中文档名（去重） | **不是** 对话里的引用角标，也不是 memory citation backfill。Agent 侧仍靠 `## Retrieved Knowledge` |
| G1 投影 | US-41 | `AgentMemoryProjector` 独立 struct；`AutoMemoryWorker.maybeProjectMemory`；覆盖 `agents/{id}.md` | 写进「团队知识收件箱」解析出的 team 库，不是当前打开的 local vault。无事实时仍写空投影页。不改 L3 内核 |
| G7 专家 | US-42 | 扫 `inbox/writeback-*.md` 与 `agents/*.md` 的 provenance 行 | 没有专家表。当前库若不是写回库，列表为空。聚合精度等于 Markdown 解析 |
| G8 健康度 | US-43 | `CollectionHealthSnapshot`：图边、孤立率、dangling、写回日记路径 | 命令面板一条 notify，**不是**仪表盘。GET **不**物化 `meta/health.md` |
| 写回确认 | US-44 | 0.60–0.84 白名单进 `inbox/writeback-pending.md`；确认后追加当日日记且**不再套 0.85 门** | UI 是「预览若干条 + 全部确认」，API 虽接受 `fact_ids` 但前端没做逐条勾选 |

未做且不应写成已做：入库期 NER、PPR、社区摘要、视频 RAG、时间 KG、SP5 成熟度 FSM、SP6 JITAI、Plan-Then-Retrieve（已是 Retrieve-Then-Generate 切片，不是规划器）。

---

## 3. 架构与红线抽查

| 检查 | 结果 |
|------|------|
| AS-COG-01 | G1 **没有**给 `Usecase` 加字段；投影器单独构造。Usecase 原有可选端口数量未增加 |
| ISP | `ListChunksByDocuments` 仍不在 `ChunkRepo` 上 |
| 日志 | 新路径用 `loggateway.Logger` + `StepID` + `Err(err)`，未见 `log/slog` / `Global()` |
| Proto | 本轮零 `make api`；custom route 与 `/asset` 同 JWT 过滤器 |
| 失败隔离 | 回填失败只 Warn 后继续重建；写回 / pending / 投影失败不阻断 AutoMemory |
| 前端 | 新 HTTP 在 `features/knowledge/api.ts`；`PanelBacklinks` 只 emit；工作台容器调 API（与既有 rebuild 模式一致） |
| 测试产物 | 金标说明在 `test/knowledge-gold/`，可执行测试在 `internal/knowledge/` |

---

## 4. 行为风险（审查意见，不是已修 bug）

1. **重建会改用户正文。** 以前「重建索引」只修 blocks/refs。现在会把未链接标题包成 `[[wikilink]]` 再重建。对 local vault 会走 CAS 写文件。这是产品语义变化，命令文案仍叫「重建当前库索引」，没有单独的「回填成链」开关。
2. **G1/G7 与当前库错位。** 投影和写回日记落在 team 收件箱集合。用户在个人库看「谁懂这个」会得到空列表——实现正确，产品易误解。
3. **标题=文件名。** 正文提到「值班制度」但笔记叫 `duty-rota.md` 时不成链。frontmatter alias 仍未做。金标语料特意让 title=提及词，所以金标比真实库乐观。
4. **确认成链与自动成链双轨。** 保存仍会静默成链；「编译本页双链」只是多了一条确认入口。Obsidian 插件那种「默认只展示」并没有变成默认。
5. **`go build ./cmd/admin` 当前失败在既有 `internal/service/computeruse.go` 与 Proto 字段不一致（`ScreenshotRef` / `Degraded`），不是本轮知识改动。** Wire 侧已手改 `provideAutoMemoryWorker` 注入投影器。

---

## 5. 测试证据（本轮跑过）

```
go test ./internal/biz/knowledge/ ./internal/knowledge/ ./internal/cronrunner/jobs/ ./internal/service/ ./internal/event/ ./internal/server/
```

均通过。其中 `TestGoldRecall_AutolinkPlusOneHopBeatsHybridSeed` 要求：12 条查询在种子 chunks 上**不得**已含邻文档事实，扩展后必须命中。

未跑：浏览器工作台确认弹窗、真实 AutoMemory→团队库、Postgres 集成。

---

## 6. 成熟度标尺（更新，仍相对业界）

```
Naive RAG ── Advanced RAG ── Agentic RAG ── GraphRAG ── Skill Knowledge
    │              │                │              │
    └──────── 底座仍在这里 ────────┘              │
              工具 + 首轮预检索段落                 一跳 + 写时成链
                                                   NER/PPR 仍无
```

相对今早的缺口评审：图稀疏问题有了**写路径供粮**（成链 + 回填 + 写回边）；Agent 首轮仍是 Retrieve-Then-Generate，不是规划器。SP7 的 G1/G2/G7/G8 在工程上闭环，产品深度是「Markdown 投影 + 聚合」，不是记忆内核与笔记内核合并。

---

## 7. 建议的后续（按杠杆，不是愿望清单）

1. 重建与回填解耦：命令拆成「只重建索引」/「回填成链并重建」，避免误改存量文件。
2. 专家/投影入口指向写回集合，或在当前库为空时提示「去团队收件箱」。
3. 金标若要承担 S-5，需要真实 BM25 语料与 PG，而不是再加 Markdown 愿望清单。
4. 待确认写回若常用，再做逐条勾选；现在 API 已留 `fact_ids`。
