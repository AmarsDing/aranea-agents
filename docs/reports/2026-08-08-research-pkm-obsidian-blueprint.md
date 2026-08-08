# 调研报告：Obsidian 式个人/团队知识库蓝图（学术 × 开源 × 源码剖析）

> 日期：2026-08-08 | 类型：research | 状态：待用户评审
> 触发：用户要求「学习 Obsidian（思路+后端逻辑+图谱逻辑）+ 多模态 + 个人/团队双模知识库；调研前沿论文与开源项目；若无现成探索性项目则自建」。
> 原始笔记（含全部证据锚点）：`test/pkm-research/{A-academic-frontier,B-oss-landscape,C-obsidian-graph-ui,D-siyuan-kernel}.md`

---

## 1. 调研方法

四路并行调研，交叉验证：

| 路 | 内容 | 产出 |
|---|------|------|
| A | 学术前沿（2023-2026，28 篇文献/项目） | CHI/CSCW/NeurIPS/arXiv 论文 + 产品启发 |
| B | 开源全景（25+ 项目 GitHub API 实测） | 选型数据 + License 风险 + 多模态分层 |
| C | Obsidian 架构逆向 + 图谱选型 + UI 趋势 | 内部机制 + 渲染技术对比 + 8 条 UI 特征 |
| D | SiYuan 源码剖析（浅克隆 `F:\pkm-research\siyuan` 本地阅读） | Go 内核数据流 + 图谱管线 + 5 个可借鉴决策 |

## 2. 核心判断（先给结论）

1. **学术与开源均无「个人+团队融合 + 多模态 + 双链图谱 + agent 可读写」的完全体**。空白明确存在，四要素各自有近作背书，属组合创新，风险可控 → **自建**。
2. **Obsidian 图谱不需要 embedding**——边全部来自显式 `[[双链]]` 文本解析。我们的「语义近邻」边是超集（需 embedding），可降级裁剪。
3. **自建骨干 = SiYuan 内核形态（Go 单体内核 + 派生索引）× AFFiNE/Colanode 双模思路 × Anything-LLM 摄取管线**，落在我们现有 Kratos + PG + Ent 栈上。
4. **图谱 = 2D sigma.js（日常检索）+ 3D three.js（展示惊艳）双模式**，布局统一 Worker 跑 ForceAtlas2；Obsidian 四区图谱控制台照搬。
5. **编辑器不自研**：TipTap 或 BlockSuite（均 MIT）。

## 3. 学术前沿要点（路 A 摘要）

- **Collaborative Memory**（arXiv:2505.18279）：双层记忆 private/shared + 片段级动态权限 + 不可变 provenance——个人/团队融合最直接的学术模型。
- **Knowledge Maturing 五阶段**：个人→团队是成熟度光谱（涌现→分发→转化→形式化→制度化），不是二元开关 → 知识条目应有「成熟度」状态机。
- **Zep/Graphiti 时序知识图谱**：事实带有效期、演化而非覆盖（LongMemEval +18.5%）→ 知识条目带时间维。
- **六原子操作框架**（写入/更新/索引/检索/遗忘/压缩）：**遗忘与压缩最被低估** → 记忆衰减机制已有学术背书（与我们 memory 域的 Ebbinghaus 衰减一致）。
- **Video-RAG（NeurIPS 2025）文本代理路线**：OCR/ASR/对象描述转文本即达 GPT-4o 级问答，每问仅 +2K token → **视频入库最低成本方案**。
- **UniversalRAG**：统一 embedding 空间不可取，**模态分库 + 路由器**更优。
- **检索习惯研究**（INTERACT 2025）：搜索型用户不做链接、浏览型用户勤做结构 → 语义搜索与图谱浏览双路径并重，用 AI 自动链接弥合。

## 4. 开源全景要点（路 B 摘要）

活跃度（2026-08-08 实测）：AppFlowy 74.9k⭐、AFFiNE 71.3k、Anything-LLM 64.3k、memos 62.1k、Joplin 55.9k、**SiYuan 45.6k**、Logseq 44.3k。

**License 警示**：SiYuan/Logseq/AppFlowy/Docmost 均 AGPL/GPL——**只读思路，不抄代码**；AFFiNE 服务端 EE 商业许可；可安全深读：memos、Anything-LLM、Cognee（MIT/Apache）。

**多模态分层**（行业现状）：
- L0 仅附件（BookStack）→ L1 嵌入播放（AFFiNE/Trilium）→ L2 深度交互（SiYuan PDF 标注即双链块）→ L3 内容理解可检索（Anything-LLM/Khoj）
- **结论：摄取管线学 Anything-LLM（每格式一个 collector loader），语义挂接学 SiYuan（解析成片段落成「块」进双链+向量体系）**。Office 解析可借 docconv/excelize/pdfium 思路（SiYuan 同款），音视频走 Whisper ASR + OCR 文本代理。

**半成品优等生**：AFFiNE（本地+云双模已跑通、Edgeless 画布、但无双链图谱）；Colanode（双模架构纯正但停滞）；Anytype（object-based + Spaces 协作、许可非 OSI）。

## 5. Obsidian 架构剖析（路 C 摘要）

**数据流（最值得我们抄的部分）**：
```
磁盘 .md → fs.watch → Lezer 增量重解析
        → MetadataCache（内存唯一真相源：links/backlinks/tags/frontmatter）
        → 持久化 IndexedDB → 所有视图只从 cache 消费
```
纪律：**单一元数据索引，视图不各自拉数**。映射到我们：Go 端建统一链接索引，WS 推增量，前端各视图订阅。

**编辑器**：CodeMirror 6 + Live Preview（源文本始终在底层，光标进入才揭示语法标记）——所见即所得但不锁数据。

**图谱**：渲染 PixiJS（WebGL）+ 物理 d3-force 分离；控制台四区 **Filters / Groups / Display / Forces**（Center 0–1、Repel 0–20、Link force 0–1、Link distance 0–500，滑块→力强度二次映射）；局部图谱 1–5 跳；时间轴=按时间过滤逐步显示。

## 6. SiYuan 内核剖析（路 D 摘要）

- **文件系统 .sy（AST JSON）是唯一权威源，SQLite 只是可删除重建的派生索引**——核心哲学。
- 一切皆块（Block）：`blocks` 树形表 + `refs` 引用边表 + `attributes`。
- 双链管线：前端事务 → lute 改 AST → 写文件 → **异步队列（磁盘重放防崩溃）** → 批量 flush 时**块级 hash diff** + **引用边按文档整体删了重插**（永不留孤儿边）。
- 图谱后端 `BuildGraph` 只产扁平 `{nodes, links}`（size ∝ log2 被引数、minRefs 剪枝、局部图 16 层 BFS），**零布局零渲染**；前端 vis-network + forceAtlas2Based。
- 资源：`data/assets/` + docconv/pdfium/excelize 提取正文入 FTS。
- **无团队能力**（单用户 + 只读发布）。
- 搜索：SQLite FTS5 + 自研 CJK 分词器；另有块级向量语义搜索。

**我们直接借鉴的 5 决策**：
1. 派生索引与源数据分离、可全量重建（暴露 rebuildIndex RPC）
2. AST 一趟遍历产出全部索引行（纯函数 `tree→[]Row`）
3. 引用边整文档重建 + 队列 op 合并 + 磁盘重放
4. 图谱后端只出参数化数据（size/minRefs/BFS 参数可直抄）
5. 派生索引与主行同事务生命周期（PG 下 tsvector 生成列 + 同事务 refs）

**落地差异**：PG+Ent 替代 SQLite/文件、Kratos+Proto 替代 gin、多租户图谱需权限过滤、中文分词用 pg_jieba/zhparser。

## 7. 图谱技术选型（路 C 摘要）

| 规模 | 方案 |
|------|------|
| ≤1k | 任意 |
| 1k–10k | WebGL（sigma.js / three.js） |
| 10k–100k | WebGL + Worker 布局 + 聚合 |
| >100k | 服务端预布局/社区折叠 |

- **2D 万级最优：sigma.js + graphology**（FA2 布局跑 Worker，WASM 版提速 10×）
- **3D 保留为「惊艳模式」**：three.js InstancedMesh 实测 10 万节点 60fps；我们已有 GPU 纹理管线底子
- 排除：G6 v5（万级性能回退 issue 多）、cytoscape（Canvas2D 万级瓶颈）

## 8. UI 趋势（路 C 摘要，8 条可落地）

1. **图谱即主页**：全屏深空图谱启动 + time-lapse 入场动画 + 2D/3D pill 切换
2. **Obsidian 四区控制台**做成液态玻璃浮层（blur 20px + 高光描边）
3. **节点视觉编码**：大小=度数、hover 邻居提亮/其余降透明、3D 连线粒子流
4. **局部图谱侧栏**（480px 可拖宽、深度滑块、随当前笔记联动）
5. ⌘K 命令面板，图谱过滤复用搜索表达式语法
6. **AI 对话检索**：回答内联笔记卡 + 图谱脉冲高亮定位
7. **空间画布视图**（DOM+CSS transform，学 Obsidian Canvas/Heptabase 白板）
8. 全站液态玻璃 + spring 微交互 + CSS 变量设计令牌 + `prefers-reduced-motion`

配色基调：`#0a0e1a` 暗底 + 霓虹分组色 + bloom 微光晕。

## 9. 产品定位与骨架推荐

**定位**：「Obsidian 式界面 × 片段级权限 × LLM 维护 × 多模态入库」四位一体——学术空白点，且与我们 agent 平台（已有 LLM/记忆/工具链）天然协同。

**骨架（推荐方案 B）**：
- **PG 为权威源**（多租户原生），块/引用/FTS 为派生索引，同事务维护 + 可全量重建（SiYuan 哲学的事务化版本）
- 文件资产双轨存储（学 Joplin resource 模型 + SiYuan assets）
- 编辑器 TipTap/BlockSuite（MIT）+ 自研 `[[ ]]` 扩展
- 图谱 sigma.js 2D + three.js 3D 双模式，Worker FA2
- 团队模式优先（PG 权威），个人=单租户降级；CRDT 本地副本后置

**替代方案**：
- A. 照搬 SiYuan 文件系统权威源 → 与多租户 PG 栈冲突，放弃
- C. CRDT 本地优先（AFFiNE 式）→ 工程量大、与现有后端割裂，后置为离线增强

## 10. 子项目分解提案

| # | 子项目 | 内容 | 依赖 |
|---|--------|------|------|
| SP1 | **知识内核重构** | 块模型 + `[[双链]]` 解析管线 + refs 物化边表 + 派生索引可重建 + rebuildIndex RPC | 基于现有 37-knowledge 演进 |
| SP2 | **编辑器与笔记体验** | TipTap 集成 + Live Preview 双链语法 + 反链面板 + 大纲 | SP1 |
| SP3 | **图谱 2.0** | sigma 2D + 3D 双模式 + Obsidian 四区控制台 + 局部图谱 + 时间轴 | SP1（边表） |
| SP4 | **多模态摄取管线** | collector（Office/PDF/视频 ASR+OCR/音频 Whisper）→ 片段成块入库入图谱 | SP1 |
| SP5 | **个人/团队双模** | 片段级权限（Collaborative Memory 模型）+ 成熟度状态机 + 共享空间 | SP1 |
| SP6 | **AI 知识伙伴** | 自动链接/自动标签/主动唤回/对话检索（复用现有 agent 平台） | SP1，贯穿全程 |

**建设顺序**：SP1 为一切基础 → 其后 SP2/SP3/SP4 可并行 → SP5/SP6 渐进。

## 11. 风险与注意

1. **License**：GPL/AGPL 项目（SiYuan/Logseq/AppFlowy）只借鉴思路不抄代码；MIT 项目（TipTap/BlockSuite/Anything-LLM/memgraph 工具）可借鉴实现。
2. **范围**：本报告覆盖「整个 Obsidian 式系统」蓝图画法，单个子项目需各自走 需求→设计→TDD 流程。
3. **现有 G5 深空图谱**：按用户决策推倒重做——但其 GPU 纹理管线/Worker 物理积累可在 SP3 的 3D 模式中复用，非全废。
4. **embedding 非必需**：图谱边以双链解析为主，语义近邻作为可选增强。

---

*附录：四份原始调研笔记（含全部论文链接、GitHub 实测数据、SiYuan 文件/函数证据锚点表）见 `test/pkm-research/`。*
