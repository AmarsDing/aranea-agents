# 评审报告：知识库整体水平与前沿缺口（2026-08-15）

> 日期：2026-08-15 | 类型：review | 状态：已对照代码核验，并落地 Lazy GraphRAG + Retrieve-Then-Generate
> 对象：模块 37 Knowledge（三件套 + `internal/knowledge` + `internal/biz/knowledge` + `internal/tools/knowledge` + `internal/agent/knowledge_inject.go` + 前端 KnowledgePage/Workbench/Graph3D）

---

## 一、真实水平（不要按文档自称来读）

文档曾写「当前处于 Agentic RAG 阶段，正向 GraphRAG 演进」。对照代码，更准确的定位是：

| 层 | 文档自称 | 代码事实 | 用户可感知效果 |
|----|----------|----------|----------------|
| 摄取 | 统一管线 + 多模态 | 文本/Office/图片入库、MD 整理、Vault 同步均存在 | 能存、能预览、能拖拽 |
| Advanced RAG | HyDE / 混合 / 自适应 / CRAG | 组件都在，**查询重写默认关闭**（除非请求带 `rewrite_strategy`），评估无 LLM 时保守标 sufficient | 搜索页能出结果；「高级 RAG」多数时候没开火 |
| Agentic RAG | Plan-Then-Retrieve + reflect | `knowledge_search` / `knowledge_reflect` 工具真实；**Plan-Then-Retrieve 在本轮之前只注入库名目录**，不检索 | Agent 经常不调工具 → 回答不接地 |
| GraphRAG | Phase 3 暂缓 | `graph.go` 是**文档双链 3D 图数据源**，不是实体关系推理 | 星河很好看，检索用不上图 |
| PKM 外壳 | Obsidian 级工作台 | 双链/块/反链/液态玻璃/星系盘：前端资产厚 | 人用笔记体验远强于 Agent 用知识 |

**一句话**：这是一套完成度很高的 **2024 Advanced RAG 底座 + 2025 PKM 可视化外壳**。缺的不是再做一个 Tab，而是「图结构进入召回」和「对话第一轮就带上知识」。

成熟度标尺（相对学术/产品，不是相对本仓库历史）：

```
Naive RAG ── Advanced RAG ── Agentic RAG ── GraphRAG ── Skill Knowledge
    │              │                │              │
    └──────── 本仓库主体 ────────┘              │
                         工具有了、规划是假的      本轮 Lazy 切片
                                                   全量 NER 仍无
```

---

## 二、文档 vs 代码：过称与空洞

| 项 | 问题 | 级别 |
|----|------|------|
| Plan-Then-Retrieve | `knowledge_inject.go` 只 `ListCollections` 拼 markdown 目录，明确告诉模型「去调工具」。这是 catalog cue，不是 retrieve-then-generate | 过称（已在本轮纠正） |
| GraphRAG / `biz/knowledge/graph.go` | 命名像图谱推理，实现是文档节点+双链边给 3D 用 | 误导 |
| 自适应路由 | `classify` 是字数/连接词启发式，不是 LLM 复杂度分类 | 可接受工程简化，但文档写得像学术 Adaptive RAG |
| 查询重写 | 实现完整，Search API **默认 RewriteNone** | 能力闲置（本轮：复杂查询自动 MultiQuery） |
| US-6 AgenticFilter / US-8 多租户 / code_search / SourceSync | 需求里仍列验收，代码明确未做 | 文档债 |
| SP7 知识-记忆同基底 / G2 写回飞轮 | 2026-08-08 已裁决采纳，生产未接线（`knowledge_memory_bridge` 已删） | 路线空白 |
| 知识协同 FR-11 | 开发计划标 ✅ 边界，运行时无写回 | 文档比代码乐观 |

非阻断、但是真的：

- `knowledge_reflect` 依赖模型主动再调一轮；模型不调就没有自校验。
- 联邦 Route 用名称/描述字符串匹配，不是 embedding 路由。
- OCR stub 已废弃，PDF 走 reader 不是视觉 OCR；图片依赖多模态 LLM。
- LinkIndex 单进程内存图（N-1），多副本会散。

---

## 三、代码质量（审查清单摘要）

对照 `aranea-review`：知识库主路径分层大体合规（biz 端口、data 实现、service 传输、`internal/knowledge` 算法）。未发现 biz import proto / trpc-agent-go 红线。

建议项：

- Usecase 字段远超 AS-COG-01（≤15）：历史债，本轮不拆。
- ChunkRepo 未扩 `ListChunksByDocuments`（有意：ISP，避免 mock 连锁）；生产靠动态断言。
- 本机 `aranea_test` 密码失败，data 层 `ListChunksByDocuments` 集成测试未能在本环境跑通；算法单测已绿。

---

## 四、为什么「前沿效果」一直没有

不是 3D 星河不够炫，是三件事同时成立：

1. **图只给人看**：wikilink / 实体边 / 星系盘从不进入 `AdaptiveRouter`。
2. **Agent 必须记得搜**：第一轮 prompt 只有库名，模型经常直接答。
3. **高级 RAG 默认休眠**：HyDE/分解要调 API 参数；复杂问题不会自动分解。

开源 PKM（Obsidian）也不做 2/3；Agent 平台的差异化本应在这里，却被可视化里程碑盖过去了。

---

## 五、本轮已落地（设计见 37-knowledge.design.md §V12.10）

1. **Lazy GraphRAG**：检索后沿已有 `knowledge_links` 一跳扩展（explicit 优先），不新建表、不碰摄取主链路。
2. **Retrieve-Then-Generate**：首轮 user 消息预检索 top4 段落注入；工具循环跳过。
3. **复杂查询自动 MultiQuery**：未指定 `rewrite_strategy` 且判定 complex 时才加 LLM 往返。

## 六、下一步（仍未做，按杠杆排序）

| 优先级 | 项 | 为什么 |
|--------|----|--------|
| P1 | 隐性知识写回飞轮（SP7 G2） | 会话/TeamRun → 带 provenance 的团队库块；这是 Agent 平台独有、开源 PKM 做不到的 |
| P1 | 金标准检索回归 + 引用率看板 | 没有数字，「前沿」无法证明；citation 回采端口已有，缺产品化 |
| P2 | 全量 GraphRAG（§9.6 NER + 社区摘要） | Lazy 之后才有必要；否则继续烧索引成本 |
| P2 | 主动唤回（G6） | 写入自动链接、决策点 JIT 召回 |
| P3 | Skill Knowledge / 多租户 / AgenticFilter | 文档里的旧债，不是当前体验瓶颈 |
