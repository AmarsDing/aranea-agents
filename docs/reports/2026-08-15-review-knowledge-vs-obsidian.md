# 评审报告：知识工作台对照 Obsidian 的真实水准（2026-08-15）

> 日期：2026-08-15 | 类型：review | 状态：对照代码 + 本轮已落地的 US-45~US-48 + 真实 Postgres 词法金标
> 对象：模块 37 Knowledge 工作台 / 检索 / 写回，相对 **Obsidian 桌面端日常 PKM**（不是相对 Microsoft GraphRAG）
> 前序：[2026-08-15-review-knowledge-sp7-closeout.md](./2026-08-15-review-knowledge-sp7-closeout.md)

本报告回答的问题只有一个：**现在能不能达到 Obsidian 的标准。**

结论先写：

**不能。** 作为「本地 Markdown 知识库 + 双链 + 反链 + 图谱 + 搜索」的日常驱动，Aranea 有一层能用的壳，距离 Obsidian 的完成度、搜索手感、生态和「文件即真相」纪律还差一整档。作为「Agent 会话沉淀进团队库」的产品，Aranea 走在 Obsidian 核心功能前面——但那不是 Obsidian 的标准，不能拿来冒充已经齐平。

---

## 1. 本轮四条后续实际做了什么

对照收口审查 §7，不是愿望清单：

| 建议 | 落地 | 代码事实 |
|------|------|----------|
| 重建与回填解耦 | 已做 | `RebuildKnowledgeIndex` 只跑块索引。`POST .../autolink-backfill` 才改 Markdown 再重建。⌘K「回填成链并重建」有确认文案。同库互斥门共用，在途 409 |
| 专家 / pending 打写回库 | 已做 | `GET /v1/knowledge/writeback-home` **只解析不创建**。专家与待确认写回打落点库；当前库不一致时提示并可切换。健康度仍统计当前打开的库，并附带「写回在别处」 |
| 金标承担 S-5 | 做了能诚实做的那截 | `TestKnowledgeRepo_SearchChunksBM25_GoldBilingual` 打生产 `SearchChunksBM25`，12 条中英硬断言在本机 `aranea_test`（`postgres:123456`）通过。**不是** 50 条。2 字「灰度」实测 0 命中，已记为已知边界，不假装过了 |
| pending 逐条勾选 | 已做 | `WritebackReviewDialog` 默认全选，提交 `fact_ids`；展示组件不调 API |

未跑：浏览器里点确认弹窗、真实 AutoMemory→团队库、`go test ./internal/service`（包因既有 `internal/tools/twinops` 编译失败，与本轮无关）。

---

## 2. 对照 Obsidian 核心体验（按用户每天会碰到的面）

Obsidian 的标准不是「有双链 API」，而是：**打开库的 200ms 内能搜、能跳、能链、能信文件没被后台改掉。**

| Obsidian 日常能力 | Aranea 现在 | 是否达到 |
|-------------------|-------------|----------|
| 磁盘上的 Markdown 是唯一真相；应用只是透镜 | 双模：local = 文件真相，team = PG 真相。重建索引现已不再改文件；但 ingest/保存仍会**自动**把标题提及包成 `[[wikilink]]`。回填是显式命令，仍会改存量 | **未达到**。Obsidian 核心不会在保存时改写你的未链接提及 |
| 未链接提及：只展示，点了才链 | 反链面板有未链接提及列表（P2-7）；同时写路径会自动成链；另有「编译本页双链」确认 | **部分**。展示有了，默认写行为仍比 Obsidian 激进 |
| `[[wikilink]]`、别名、标题补全 | 有 wikilink、`\|alias` 显示、heading 补全。解析层有 frontmatter title/aliases 键；**成链匹配仍按文件名 basename**，`duty-rota.md` 对不上正文里的「值班制度」 | **未达到** 别名作为一等公民的手感 |
| 反链 / 出链 / 大纲 | 工作台右栏有 | **接近** 单库笔记壳 |
| 图谱 | 3D 星系图 + 局部图，不是 Obsidian 的 2D 力导向默认图。没有 Canvas 白板（i18n 里的 canvas 是 Agent 图画布） | **不同产品**，不能算达到 |
| 即时搜索（含 2 字中文、拼音、文件名） | 全库搜索走 PG 词法（tsvector simple + trigram）。本轮金标：12 条较长中英查询能中；**「灰度」两字短句 0 命中**。没有 Obsidian 那种本地倒排的「边打边出」 | **未达到**。这是对照 Obsidian 最硬的差距之一 |
| 嵌入 / 块引用 / `![[note]]` | 块级双链与晋升是自己的模型，不是 Obsidian embed | **未达到** 同一套用户心智 |
| 插件市场、主题、移动端、官方同步 | 无社区插件；深空主题是产品内置；不是手机上的第二大脑；同步是团队 PG，不是 Obsidian Sync | **未达到** |
| Daily notes / Templates / Quick switcher / ⌘P | 有 ⌘O / ⌘K、日记形态的写回路径（`inbox/writeback-YYYY-MM-DD.md`），不是用户自己的 Daily note 工作流 | **部分** |
| 文件名、文件夹、拖拽整理 | 资源管理器三栏 + 移动冲突对话框 | **接近** 作为库浏览器 |

一句话：把 Aranea 知识页当 Obsidian 用，能记、能链、能看图，但搜索、别名、不乱改文件、生态这四件事会每天绊脚。

---

## 3. Aranea 明显强过 Obsidian 核心的地方（不要用来对冲上面的「未达到」）

这些是真的，但它们定义的是**另一类产品**：

- 会话自动记忆过门后写入团队日记（provenance：`fact_id` / `session_id` / `agent_id`）。
- 0.60–0.84 进 pending，人可以逐条放行。
- Agent 活动记忆投影为 `agents/{id}.md`。
- 检索可走向量 + 词法 + 一跳 `knowledge_links`（explicit×3 > entity×2 > semantic×1）。
- 团队库与个人库分后端，晋升到 team。

Obsidian 要靠社区插件（Dataview、Smart Connections、Copilot）才能靠近这些。Aranea 把它们做进了主产品，代价是 PKM 主路径没有 Obsidian 干净。

---

## 4. 检索：不要用「有 BM25」三个字对齐 Obsidian

本轮在真实 Postgres 上跑了生产路径，不是 Markdown 里写「应该能搜到」。

- **过了**：`双人值守`、`MQTT`、`secondary oncall`、`必须走灰度`、`回滚开关` 等 12 条；负例不误中。
- **没过（已知）**：短查询 `灰度` 对含「走灰度」的短句 0 命中。Obsidian 即时搜索会中。
- **没做**：50 条金标、拼音、文件名加权、拼写容错、本地毫秒级倒排。

所以 S-5「BM25 召回纳入回归」现在的诚实状态是：**有一条可跑的生产路径金标，覆盖面是 12 条，中文两字短查询仍是洞。** 不能对外说检索已达 Obsidian。

---

## 5. 成熟度标尺（PKM，不是 RAG）

```
记事本 ── 能链的库 ── 日常可依赖的 PKM ── Obsidian 级（生态+手感+文件纪律）
              │                │
              └─ Aranea 在这 ──┘  还没跨过「每天愿意把人生笔记放进来」
```

相对今早的 SP7 收口：误改文件的重建路径已经拆开；写回入口不再假装当前个人库就是收件箱；pending 可以少确认一条。这些让产品**少踩坑**，没有让它变成 Obsidian。

---

## 6. 若要以 Obsidian 为合格线，下一步只值得做的三件事

按杠杆，仍然不是愿望清单：

1. **保存默认不成链**（未链接提及保持展示；成链必须确认或明确开关）。没有这一条，文件纪律永远达不到 Obsidian。
2. **别名成为成链与跳转的一等键**（frontmatter `aliases` 真正参与 basename 之外的匹配）。否则真实库的文件名习惯会让一跳检索空转。
3. **中文短查询与边打边出**：至少让 2 字专名稳定命中；搜索不必先等 PG 往返才出第一屏。做不到就不要对用户说「搜索已经齐了」。

插件市场、Canvas、官方同步、移动端：在上面三条没做之前开工，是在装修，不是在达标。
