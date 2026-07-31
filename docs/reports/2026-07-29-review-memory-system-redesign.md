# 记忆系统整体设计评审与重设计方案

> 日期：2026-07-29
> 类型：review（设计评审 + 重设计提案）
> 触发：精灵助手「使用 3982 次、命中 0、无内容」——排查发现记忆读/写/归档三链路同时断裂（6 个独立 bug），用户要求不急于修 bug，先从系统层面评审设计是否合理，并对照前沿研究给出最合理的方案。
> 关联：`docs/development/70-orchestration-longtask-memory.design.md` §九/§十五、本报告附带的现状排查结论

---

## 零、决策定稿（2026-07-29，拍板）

> 本节为最终决定，替代报告正文中所有「待决策/建议」表述。依据 AS-ADR-01 记录。

| # | 决策 | 结论 | 一句话理由 |
|---|---|---|---|
| D1 | L4 定位 | **降级为 Semantic 的派生图索引**，不再作为独立「记忆层」呈现 | 空层 + 默认不注入的「层」只是认知负担；实体关系由统一写入管线产出 |
| D2 | FR-10 神经元模型 | **推迟至 P4**，以离线评测对比决定去留；需求文档 FR-10.x 状态改记「⏸ 推迟」 | 在 4 条 fact 规模上无可证收益；A-MEM link evolution 以 1/5 复杂度覆盖其约 80% 价值且已验证 |
| D3 | 路线图 | **P0 立即执行**（3 bug + 金丝雀 + 停用虚高指标），P1-P4 按 §6.7 顺序推进，不做并行 | 止血无争议；闭环不通时任何高级机制都是空转 |
| D4 | Profile 常驻卡 | **纳入 P1 且为 P1 最高优先级** | 「用户感到记忆存在」的最短路径（Letta core memory 已验证），成本仅一次蒸馏 + 系统提示注入 |
| D5 | 记忆档位 | **纳入 P1**：off / minimal / standard / full 四档，默认 standard；~20 个细开关退居高级配置，面板不暴露 | 默认开关组合致死（L2 召回关 + minScore 0.55 + brute-force 不算分）证明「组合行为无人审视」是一类系统性风险，必须用预设收敛 |
| D6 | 精灵助手优先 | P0/P1 全部验收场景以精灵助手为准，做到无懈可击后再推广其他 agent | Spirit 是产品门面，其记忆体验即用户对平台智能程度的第一判断 |
| D7 | 记忆中心定位 | 从「数据库表浏览器」改为**健康仪表盘 + 治理台**；L0-L4 工程术语退出 UX | 用户要看的是「系统记住了什么、能否修正、工作正常吗」，不是内部数据结构 |
| D8 | 治理一等公民 | 双时态历史、溯源下钻、手动修正、遗忘操作全部走操作语义进治理台，禁止直接改库 | 可审计/可遗忘/可解释是企业合规卖点，也是修正垃圾记忆的安全阀 |

**状态：已接受（用户授拍板权）。** 后果：FR-10 相关已写代码/文档标记推迟（非删除）；L4 面板并入 Semantic 视图；记忆中心前端按 D7 重构进 P1 范围。

---

## 一、评审结论（TL;DR）

**现有 L0-L4 设计方向不算错，但存在三个根本性问题，且互相叠加导致「整个记忆没有起作用」：**

1. **闭环断裂**：写入、存储、召回、注入、观测五个环节各自独立破碎（本次查出 6 个 bug 分布在其中四个环节），且**没有任何一个端到端校验能发现「记忆从未进入 prompt」**。一个系统级的功能，上线 45 天无人察觉它从未工作——这不是 bug 问题，是设计里根本没有闭环校验。
2. **分层语义混乱**：L0 不是记忆（是 prompt 组装观测快照）、L1 是运行时暂存、L2 默认不召回、L4 默认不注入——五个「层」里只有两个半真正参与记忆职能，但全部以「记忆层」名义呈现在记忆中心，直接造成用户「用了这么多次没有任何内容」的困惑。
3. **复杂度错配**：在 4 条 fact、0 条 episode 的数据规模上，设计了神经元模型/扩散激活/赫布规则（FR-10）；而前沿社区已验证更便宜、更关键的机制（操作语义、降噪闸、闭环观测、混合召回）反而缺失。

**对照 2025-2026 前沿共识**（Mem0 / Zep-Graphiti / Letta / LangMem / A-MEM / MemoryOS / Nemori 等，详见 §四）：本系统的 Bi-temporal、Sleep-time、主动召回方向与前沿一致，值得保留；但缺失了社区已固化的四块基石——**写入操作语义（ADD/UPDATE/DELETE/NOOP）、提取降噪纪律、失效而非删除的全表贯通、以及闭环金丝雀**。

**建议：不打翻存储层（表结构大体可用，P0 bug 修完即可恢复数据流），但重排概念分层、重建写入管线与召回融合、补齐观测闭环；FR-10 神经元模型整体推迟。**

---

## 二、现状盘点：设计意图 vs 实现现实

### 2.1 设计意图（70 号文档 §九/§十五）

L0-L4 分层 + 5 项前沿增强 + L4 神经元模型：

| 层 | 设计定位 | 表 |
|---|---|---|
| L0 | 上下文组装快照（prompt 组装观测） | `memory_l0_assembly_snapshots` |
| L1 | 工作记忆（任务级 fields，闲置归档） | `memory_l1_tasks` / `memory_l1_fields` |
| L2 | 情节记忆（session/task 摘要） | `memory_episodes` |
| L3 | 语义事实（双时态、衰减、链接图） | `memory_facts` |
| L4 | 实体图谱（神经元增强：激活值/突触权重/扩散激活） | `memory_entities` / `memory_relations` |

增强项：Bi-temporal（P3-8 ✅）、Ebbinghaus 衰减（P3-9 部分）、Sleep-time 整理（P3-10 未接线）、主动召回（P3-11 ✅）、链接图（P3-12 ✅）、神经元模型（FR-10 设计中）。

### 2.2 实现现实（2026-07-29 生产库实测 + 日志实证）

| 环节 | 状态 | 实证 |
|---|---|---|
| L3 写入（即时） | **断** | 提取出 3 条 fact 后 upsert 报 `42702 字段关联 "version" 是不明确的`（`memory_maintenance_adapter.go:123` 未限定表名），事务回滚。fact 停留在 2026-06-13 的 4 条 |
| L3 写入（cron 批量） | **空转** | auto_memory 每轮 `facts_added:0`（transcript 为空） |
| L3 召回注入 | **断** | fused recall 中 brute-force（fact 数 ≤5000 恒成立）不算分 → `Scores.Total=0` → `minScore=0.55` 过滤全部丢弃 → prompt 从无 L3 段落 |
| 召回计数 | **语义错误** | 每次召回给全部返回 fact 无条件 `use_count++`（4×995≈3982）；`hit_count` 全库无递增代码，死计数器 |
| L1→L2 归档 | **断** | 迁移版本碰撞：`20260802` 被 `session_turn_number_backfill` 抢先占用，`memory_episodes_l1_task_unique` 被静默跳过 → 部分唯一索引不存在 → `ON CONFLICT` 42P10 → 事务回滚。episodes=0 行，20 个 L1 任务全部 cancelled 且未归档（数据丢失） |
| L1 生命周期 | **语义错误** | 闲置 60 分钟即置 `cancelled`（应为 archived/expired），且归档失败后逃出重试集合，失败静默永久化 |
| L2 召回 | **默认关闭** | 内建 agent 全部 `l2_recall_enabled=false` → composite 路径不激活 |
| L4 注入 | **默认关闭** | `l0_inject_l4=false`；每 agent 仅 1 entity、0 relation |
| L0 快照 | **常态空表** | `on_warning` 模式：usedRatio≥0.80 才写，正常对话永不触发（全库仅 1 行） |
| Sleep-time | **未接线** | `MEMORY_SLEEP_TIME_PROVIDER/MODEL` 未配置 → LLM=nil → 每周期 `episode consolidation skipped: nil LLM` |

**结论：除「召回计数器」外，记忆系统的每一条数据通路当前都不通。** 前端记忆中心忠实展示了这些空表。

---

## 三、根因分析：为什么 6 个 bug 能同时存活 45 天

单看每个 bug 都是小错（一行 SQL、一个版本号、一个未填的评分字段）。它们能长期共存的结构性原因：

1. **链路无人端到端拥有**：写入由 cron/即时 hook 负责、召回由 prompt 组装负责、计数由 data 层负责——没有一处代码断言「写入的 fact 必须出现在后续 prompt 中」。
2. **失败全部静默降级**：upsert 失败 → Warn 日志；episode 创建失败 → Warn 日志；fused 过滤全丢 → 返回空字符串（合法行为）；LLM 未接线 → Warn 后 skip。**每一环的接口契约都允许「空」作为正常返回值**，错误被逐层吞掉。
3. **观测指标定义错误**：面板展示的是 `use_count`（召回即加，与是否注入无关）——它唯一在增长，给人「系统在运行」的假象。真正该监控的「注入率」不存在。
4. **测试缺失分层**：单测用 mock 填满 `Scores`，集成测试覆盖不到「fused recall + 真实 brute-force + 真实 minScore」组合；没有 e2e 金丝雀。

---

## 四、前沿调研要点（2025-2026，附来源）

### 4.1 已成事实标准的设计

| 共识 | 代表 | 对本项目的意义 |
|---|---|---|
| 纯向量 RAG ≠ 记忆（无时间/无矛盾处理/无实体解析/无关系） | Zep 论文核心论证 | 现有 hybrid 评分方向正确，但 FTS 未接入、融合在热路径断裂 |
| 认知三分：semantic / episodic / procedural | LangGraph/LangMem 文档 | L0-L4 数字分层应重组为认知分层；procedural = 平台已有的 Skill 体系，不必重建 |
| **失效而非删除**（append-only + invalidation） | Graphiti 边失效、Mem0 2026 转向 ADD-only、Letta git 版本化——三条独立路线殊途同归 | 现有 fact 双时态列方向正确，但未贯通到 relation/episode，且写入路径根本没走到 |
| 写入异步化 + 后台反思（reflection/sleeptime/memify/Auto Dream） | LangMem、Letta、Cognee、Claude Code | Sleep-time 设计方向对，但 LLM 未接线成死代码 |
| 提取管线 = LLM + 结构化 schema，规则式已放弃 | Mem0、Graphiti | 现有提取器已符合 |
| scope/namespace 隔离是一等公民 | 全部 | 现有 agent_id+user_id 双 scope 基础可用，缺 team/workspace |
| **token 预算是一等指标**（新算法卖点是「<7k token 达到同等准确率」而非「更准」） | Mem0 2026 新算法 | 现有 L0 有预算概念，但未作为记忆召回的一等约束 |

### 4.2 被证伪/退潮的设计

1. **「更大上下文窗口解决记忆」被证伪**（成本/延迟线性膨胀 + lost-in-the-middle）。
2. **LLM 对账式 UPDATE/DELETE 被 Mem0 自己放弃**（2026）：「对账是上下文被销毁的地方」——但注意，Mem0 转向的是 ADD-only + 双时态并存，**不是放弃操作语义，而是把冲突解决从写入时移到读取时**。
3. **无过滤提取被生产打脸**：Mem0 issue #4573，32 天 10134 条记忆 97.8% 是垃圾（系统提示复读 52.7%、心跳噪声、幻觉画像、身份混淆、敏感信息泄漏）；换强模型仅降到 ~90%。**提取侧必须有降噪纪律**。
4. **全自动自主记忆管理（Letta 式）未被主流采纳**：token 贵、不可预测；OpenAI/Claude 均选「记忆是显式工具，框架不做自动魔法」。**混合式（小预算自动注入 + 工具按需召回）是当前最优解**。
5. **厂商 benchmark 数字互掐不可尽信**：按场景选型，必须有自己的评测集。

### 4.3 值得关注的前沿机制

| 机制 | 来源 | 价值 |
|---|---|---|
| 双时态四时间戳（valid_at/invalid_at/created_at/expired_at）+ point-in-time 查询 | Zep/Graphiti | 事实的时间语义完整方案，PG 即可实现 |
| 写入操作语义 {ADD/UPDATE/DELETE/NOOP}，甚至用 RL 学（152 样本即学会 UPDATE vs DELETE+ADD） | Mem0 / Memory-R1 | 解决「记忆只增不改」的脏数据积累 |
| Zettelkasten note + link evolution（新记忆触发旧记忆关键词/标签演化） | A-MEM (NeurIPS 2025) | 本系统链接图（P3-12）的完整版，时序 F1 39.41 vs MemGPT 17.29 |
| 预测误差驱动写入边界（对话分段不按轮次按「意外度」） | Nemori | 替代固定窗口/固定轮次的 episode 切分，token 省 88% 且超 full-context |
| 写入时推理前移（存储前就完成关系推理，读取零成本） | PREMem | ≤4B 小模型可比大模型，降低热路径成本 |
| 热度/生命周期取代固定遗忘曲线；**衰减仅作召回降权，不做删除** | MemoryOS / MemoryBank / LightMem | 修正 Ebbinghaus worker 的定位 |
| 多智能体记忆 = 二部图动态访问控制 | Collaborative Memory | 编排平台多 agent 共享记忆的权限模型 |
| 评测基准：LongMemEval（五能力框架）、LOCOMO、MemoryBench（程序性记忆 SOTA 不及格）、MemBench | 各基准论文 | 自建金丝雀 + 离线评测集的依据 |

---

## 五、设计评审：合理之处 vs 根本漏洞

### 5.1 值得保留的

- **Bi-temporal fact 列**（valid_from/valid_until）——与 Zep 同向，已落库。
- **Sleep-time 异步整理**的架构意图——与 Letta/LangMem/Cognee 同向。
- **主动召回触发器**（ProactiveRecall，P3-11）——实体提及触发，方向正确。
- **链接图列**（links/keywords/tags，P3-12）——A-MEM 的雏形。
- **混合评分召回**（semantic/recency/importance/keyword 加权 + decay 融合）——公式合理，只是热路径断了。
- **agent 自助记忆工具**（`memory_search`/`memory_delete` 已装配）——proactive 路线的底子已有。
- **表结构总体可用**——不需要打翻重来，需要的是概念重排 + 管线重建。

### 5.2 根本性漏洞（按严重度排序）

**V1. 无闭环校验（致命）**。系统设计里不存在「写进去的记忆必须能被读出来」的不变量。6 个 bug 分布在写、存、读、注四个环节，任何一个 e2e 金丝雀都能全部抓住，但这样的金丝雀不存在。→ **记忆系统第一需求不是「先进」，是「可验证地工作」。**

**V2. 分层语义混乱（架构级）**。
- L0 是 prompt 组装的观测快照，不是记忆——却占着「记忆 L0」的名字和面板位置，且 `on_warning` 模式注定常态空表。
- L1 是运行时 scratchpad，生命周期管理（60min cancel）与归档链（→L2）双双破碎后，它实际上是一条**数据销毁管道**。
- L2 默认不召回（`l2_recall_enabled=false`）、L4 默认不注入（`l0_inject_l4=false`）——两个「层」在默认配置下是纯写不读（实际上连写也是断的）。**层数多 ≠ 能力强；五个层里两个半在干活，用户看到的却是五个空表。**

**V3. 写入管线缺三块基石（功能级）**。
- 无操作语义：只有 append + key 冲突失效，没有 {ADD/UPDATE/DELETE/NOOP} 裁决，记忆只会淤积不会演化。
- 无降噪闸：无 fact 类型白名单、无置信度门槛、无 embedding 去重——Mem0 #4573 证明这是必踩的坑（97.8% 垃圾率），我们的 cron 提取 0 条反而「因祸得福」。
- 无溯源：fact 不携带 source_episode/source_message 引用，面板无法下钻「这条记忆从哪来的」，治理无从谈起。

**V4. 观测指标定义错误（观测级）**。`use_count`=召回即加（虚高 3982）、`hit_count`=死代码（恒 0）、「注入率」「引用率」不存在。错误的指标比没有指标更糟——它让所有人以为系统在工作。

**V5. 复杂度错配（决策级）**。FR-10 神经元模型（激活值/赫布规则/扩散激活/元记忆）学术上有趣，但：①它建立在 L4 之上，而 L4 当前空且默认不注入；②在 4 条 fact 的规模上，扩散激活相对简单混合检索没有可证明的收益；③A-MEM 的 link evolution 以 1/5 的复杂度覆盖其约 80% 的价值，且已在 LoCoMo 上验证（时序 F1 39.41 vs MemGPT 17.29）。**先用经过验证的简单机制把闭环跑通，把神经元模型留到数据规模与评测体系到位之后。**

**V6. 多智能体记忆缺位（平台差异化缺失）**。本系统是编排平台（spirit/team/graph），但记忆 scope 只有 agent+user：团队成员的集体经验（「哪类任务哪个成员做得好」）无处可存——而这恰恰是编排器做分配决策最需要的记忆。Collaborative Memory 的二部图访问控制模型可直接借鉴。

**V7. 配置面碎片化（工程级）**。记忆行为由 ~20 个 runtime setting 开关控制（l0_snapshot_mode、l0_inject_l1/3/4、l2_recall_enabled、l2_recall_max、l3_recall_min_score…），默认值组合出来的实际行为无人审视（本次就是「默认关 L2 召回 + 默认 minScore 0.55 + brute-force 不算分」三者叠加致死）。需要一个「记忆档位」抽象（off/minimal/standard/full）把开关组合收敛为可理解的几个预设。

---

## 六、重设计方案

### 6.1 设计原则（按优先级）

1. **闭环优先**：任何记忆能力上线前，必须配一只自动化金丝雀断言「写入→召回→注入」贯通。没有金丝雀的功能不许进面板。
2. **先工作，再先进**：机制选型顺序 = 已被多系统验证的简单机制 > 单系统验证的复杂机制 > 论文新机制。
3. **失效而非删除**：所有记忆实体（fact/episode/relation）统一双时态，删除仅限治理操作且留审计。
4. **写入重、读取轻**：所有 LLM 重处理（提取/合并/反思/推理）挪出对话热路径；热路径只做检索 + 预算内注入（Zep「热路径无 LLM」原则）。
5. **衰减只降权不删除**；**token 预算是一等约束**；**scope 隔离是一等公民**。
6. **平台差异化**：记忆必须服务于编排（团队经验→分配决策），而不是一个通用 RAG 附属品。

### 6.2 目标概念模型：认知三层 + 运行时暂存 + 观测快照

```
┌─────────────────────────────────────────────────────────────┐
│ 对话热路径（每次 LLM 调用前）                                  │
│  ① Profile 常驻卡（每个 agent 一张，sleep-time 维护，必注入）   │
│  ② 混合召回 top-k（预算内，默认 ≤800 tokens）                  │
│  ③ agent 自助工具（memory_search，按需大召回，已有）            │
└─────────────────────────────────────────────────────────────┘
        ▲ 读取                          写入 ▲
┌───────┴───────────────────────────────────┴────────────────┐
│ 记忆存储（Postgres，全部双时态，scope=user/agent/team）        │
│                                                             │
│  Semantic Store（原 L3 升级）                                │
│    memory_facts + 操作语义 + 溯源引用 + FTS 索引              │
│    └─ Graph Index（原 L4 降级为派生索引）                     │
│       memory_entities/relations 由写入管线同步产出，           │
│       不再是独立「层」，而是 fact 的实体视图                    │
│                                                             │
│  Episodic Store（原 L2 修复）                                │
│    memory_episodes：任务/会话情节，预测误差或任务边界切分，      │
│    embedding + 时间序，含 outcome（成功/失败）                 │
│                                                             │
│  Procedural（不新建）= 平台已有 Skill/Playbook 体系            │
└─────────────────────────────────────────────────────────────┘
        ▲ 归档                        ┌──────────────────────┐
┌───────┴──────────────┐             │ Sleep-time 整理管线    │
│ Scratchpad（原 L1）   │────────────►│ 提取→操作裁决→降噪闸→  │
│ 任务级暂存 fields，    │  任务结束    │ 落库 + profile 卡维护  │
│ 任务结束即归档进       │             │ + 链接演化（A-MEM 式） │
│ episode，不再 60min   │             └──────────────────────┘
│ 强制 cancel           │
└──────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ 观测（移出记忆中心，归入 tracing）                              │
│  L0 快照（原样保留，改名 prompt_assembly_snapshots）            │
└─────────────────────────────────────────────────────────────┘
```

关键变化：

| 原概念 | 新归属 | 理由 |
|---|---|---|
| L0 快照 | 移出记忆中心 → tracing/可观测 | 它不是记忆，是 prompt 组装的观测产物；`on_warning` 常态化空表不再误导 |
| L1 fields | Scratchpad（任务级暂存） | 生命周期绑定任务结束而非 60min 定时 cancel；归档即转 episode，链路必须原子 |
| L2 episodes | Episodic Store | 修复归档链后启用召回（默认开），承载「我做过什么」 |
| L3 facts | Semantic Store | 修复写入/召回后启用，承载「用户是谁/偏好什么」 |
| L4 entities/relations | Semantic 的 Graph Index | 降为派生索引：由同一写入管线从 fact/episode 产出，供下钻与图扩展召回；不再单独占据一个「层」的认知负担 |
| Profile 卡 | **新增** | Letta core memory 已验证：一张 100% 注入的小卡片是「用户能感到记忆存在」的最直接来源；由 sleep-time 从 facts 蒸馏维护，约 300-500 tokens |
| 神经元模型 FR-10 | **推迟** | 数据规模与评测体系到位后再评估；link evolution 先行覆盖其大部分价值 |

### 6.3 写入管线（重建，单一管道）

所有来源（即时 hook、auto-memory cron、sleep-time、L1 归档、agent memory_save 工具）汇入**同一条管线**，杜绝多条写入路径各自破碎：

```
SessionEnd / TaskEnd / 显式"记住这个"
        │
        ▼
① Episode 落库（含原文引用 message_ids、outcome、scope）
        │
        ▼
② 异步提取（单次 LLM 调用，Zep v0.28 模式：facts + entities + relations 一次出）
   - 操作语义：每条候选 fact 裁决 ADD / UPDATE / DELETE / NOOP
     （UPDATE/DELETE = 旧记录 invalid_at=now，新记录 valid_from=now，永不物理删）
   - 时间解析：相对时间以 episode reference_time 为锚
        │
        ▼
③ 降噪三闸（Mem0 #4573 教训，缺一则垃圾淤积）
   - 类型白名单：preference / profile / goal / constraint / decision / relationship
   - 置信度门槛：confidence < 0.6 丢弃
   - 去重：与存量 fact embedding 余弦 > 0.92 → 合并（access 计数+1）而非新增
        │
        ▼
④ 落库（双时态四列贯通 fact + relation；每行带 source_episode_id + source_message_ids）
        │
        ▼
⑤ 派生维护（异步）：graph index 更新、link evolution（新 fact 触发近邻 keywords/tags 演化）、
   profile 卡重蒸馏（阈值触发：新增 ≥N 条或每 24h）
```

配套修正：

- **Scratchpad→Episode 归档原子化**：修复迁移版本碰撞（Bug C），任务结束触发（不再 60min cancel）；归档失败必须可重试（留在扫描集合内 + 死信告警），消灭「静默永久失败」。
- **计数器语义重建**（详见 §6.5）。
- **Sleep-time 接线**：LLM 走 ModelCatalog 默认模型解析（与 MemoryLLMExtractor 一致），淘汰 `MEMORY_SLEEP_TIME_PROVIDER` 环境变量方案； consolidator 纳入 cronrunner 调度（当前是死代码）+ 失败重试 + 死信。

### 6.4 读取路径（修复 + 升级）

```
触发：before-model hook（每次 LLM 调用前）
  │
  ├─ ① Profile 常驻卡 → 必注入（无召回，零延迟）
  │
  ├─ ② 混合召回（semantic + episodic 并行）：
  │     query = 最近用户消息（+ 提及实体）
  │     信号：pgvector 语义 + PG FTS（to_tsvector，基础设施已有）
  │           + recency + importance + decay_score
  │     融合：RRF → minScore 过滤（修 Scores.Total 贯通，Bug A）
  │     预算：默认 ≤800 tokens（可配档位），超出按分截断
  │
  └─ ③ agent 自助：memory_search 工具（已有，按需大召回，不占自动预算）

冲突呈现：召回命中 valid_until 并存的新旧 fact 时，优先当前有效；
          潜在矛盾（关键词重叠 + 语义冲突）时在注入文本中并列标注，交 LLM 澄清（ProactiveRecall 已有雏形）
```

- **brute-force 路径必须算分**：小数据量（<阈值）时更要把分算全，minScore 才有意义；或者显式标记 unscored 并豁免 minScore（二选一，推荐前者）。
- **记忆档位预设**（治 V7）：`off / minimal(仅 profile 卡) / standard(profile+语义召回) / full(标准+情节+图扩展)`，默认 standard；底层细开关保留但面板不暴露。

### 6.5 观测与评估（新建，闭环之本）

**三段计数（替换现有 use_count/hit_count）**：

| 指标 | 定义 | 落点 |
|---|---|---|
| `recalled_count` | 进入召回结果集 | data 层（=现 use_count 语义，改名） |
| `injected_count` | 通过过滤+预算，真正写入 prompt | prompt 组装处递增（这是唯一该向用户展示的「使用次数」） |
| `cited_count` | LLM 回复显式引用了该记忆（启发式/抽样 LLM 判定） | sleep-time 抽样回填 |

**闭环金丝雀（最重要的一项新增）**：

```
每 30 分钟（可配）：
  1. 合成会话发送「金丝雀记忆测试：我的代号是 FALCON-<rand>」
  2. 断言 60s 内 semantic store 出现该 fact（写入链路）
  3. 新会话发送「我的代号是什么？」
  4. 断言 prompt 组装日志中该 fact 被注入（召回链路）
  5. 任一断言失败 → 告警（EventBus + 面板红条）+ 指标 canary_failed_total
本次 6 个 bug（42702 / 版本碰撞 / Total=0 / nil LLM / 死计数器 / 归档丢失）会被这只金丝雀全部抓获。
```

**离线评测集**：LongMemEval 五能力子集（信息抽取/多跳/时序/知识更新/拒答）本地化 50-100 条，每个记忆相关 PR 跑一遍防回归；定期抽样注入内容做人审垃圾率评估（目标 <5%）。

**面板重构（记忆中心）**：
- Panorama → **闭环健康仪表盘**：写入成功率、注入率、金丝雀状态、各 scope 数据量趋势、垃圾率抽样——替代当前的空表罗列。
- Browse → 按 Semantic / Episodic / Scratchpad 三视图组织，fact 可下钻 source episode/message。
- Graph → 保留（graph index 可视化），但入口从属于 semantic。
- Governance → 双时态历史、失效记录、合并审计、手动修正（修正也走操作语义，不直接改库）。

### 6.6 多智能体记忆（平台差异化，Phase 3）

- **scope 四级**：`user / agent / team / workspace`，默认严格隔离；共享走显式规则（二部图授权：memory_scope × consumer_scope，参考 Collaborative Memory）。
- **Team 经验记忆（新增，编排平台核心价值）**：团队任务完成后，除 episode 外沉淀 `team_skill_outcome`（任务类型 × 成员 ×  outcome × 耗时），供编排器分配决策与 Agent Factory 匹配评分使用——记忆反哺编排，这是通用记忆系统给不了、本平台独有的回路。
- **Spirit 记忆总管**：跨 agent 的记忆路由（「这条偏好该给哪个成员可见」）+ 隐私边界（用户级 fact 不下放到 team scope，除非显式授权）。

### 6.7 分期路线图

| 阶段 | 内容 | 验证标准 |
|---|---|---|
| **P0 止血** | 修 Bug F（version 歧义）/ Bug C（迁移版本碰撞重编号）/ Bug A（Scores 贯通） + **金丝雀上线** | 金丝雀绿；发一条「我喜欢 X」下一轮能问出来 |
| **P1 闭环重构** | 统一写入管线（操作语义 + 降噪三闸 + 溯源）；计数器三段化；profile 常驻卡；scratchpad 归档原子化；sleep-time 接线 | 注入率 >0；7 天垃圾率抽样 <5%；无静默失败告警积压 |
| **P2 召回升级** | FTS 接入 + RRF 融合；token 预算档位；L2 召回默认开；离线评测集 50 条 | 评测集通过率基线建档；p95 召回延迟 <200ms |
| **P3 多智能体** | 四级 scope + 共享授权；team 经验记忆反哺编排；面板治理完善 | 团队任务分配命中率有可测提升 |
| **P4 前沿实验（可选）** | A-MEM link evolution 完整版；Nemori 预测误差切分；**届时再评估神经元模型** | 与 P2 基线对比有统计显著提升才保留 |

### 6.8 明确不做 / 推迟

| 项 | 处置 | 理由 |
|---|---|---|
| FR-10 神经元模型（激活值/赫布/扩散激活/元记忆） | **推迟至 P4 再评估** | 建立在空置的 L4 之上；无评测无法证明收益；link evolution 先行 |
| Ebbinghaus 固定曲线删除策略 | **取消删除语义**，decay_score 仅作召回降权 | 社区共识：衰减不用于删除 |
| L0 快照常驻记录 | 维持 `on_warning`，但移出记忆中心 | 它是 tracing 产物，常态记录性价比低 |
| 图数据库（Neo4j 等） | **不引入** | PG 递归 CTE + pgvector 足够；运维成本不匹配当前规模 |
| Letta 式全自动自主记忆管理 | 不采纳 | token 成本与不可预测性；保留 agent 工具 + 自动注入混合式 |

---

## 七、迁移映射（现有资产处置）

| 现有资产 | 处置 |
|---|---|
| `memory_facts`（双时态/链接图列） | 保留 = Semantic Store；补 source 引用列 + FTS 索引；修 P0 bug 即通 |
| `memory_episodes` | 保留 = Episodic Store；修迁移碰撞后重建归档流 |
| `memory_entities/relations` | 保留 = Graph Index（派生）；写入改由统一管线产出；FR-10 新列暂缓 |
| `memory_l1_tasks/fields` | 保留 = Scratchpad；生命周期改绑任务结束 |
| `memory_l0_assembly_snapshots` | 保留表，前端入口移至 tracing/监控 |
| `use_count/hit_count` | 改名/废弃，替换为 recalled/injected/cited 三段计数（历史数据可保留不再维护） |
| Sleep-time/consolidator 代码 | 接线修复（LLM 来源、调度、重试），逻辑沿用 |
| ProactiveRecall / 混合评分 / 记忆工具 | 沿用，接入新融合层 |
| 前端四 Tab | 按 §6.5 重构（Panorama 改健康仪表盘为最大变化） |

**结论：表不用推、概念必须推。** 存储层约 80% 可复用；概念分层、写入管线、召回融合、观测闭环四件事重做。

---

## 八、附录：前沿系统横向对比（调研摘要）

| 维度 | Mem0 | Zep/Graphiti | Letta | Cognee | LangMem | 本系统现状 | 本方案目标 |
|---|---|---|---|---|---|---|---|
| 核心抽象 | 原子事实(+图) | 双时态 KG | 自编辑分层 | 图+向量知识层 | 认知三分 | L0-L4 五层 | 认知三层+暂存+观测 |
| 冲突解决 | 2026 转 ADD-only 并存 | 边 invalid_at 失效 | git 版本化 | 节点版本化 | manager 裁决 | 仅 fact key 冲突失效（且断） | 全实体双时态失效 |
| 写入操作语义 | 旧版 ADD/UPD/DEL/NOOP | 抽取管线内解析 | agent 自主 | cognify 管线 | hot/background 双模 | **无** | ADD/UPD/DEL/NOOP + 降噪三闸 |
| 降噪 | 教训级反面教材(#4573) | 实体解析去重 | agent 自律 | 本体校验 | schema 约束 | **无** | 白名单+置信度+去重 |
| 读取 | 向量+图+多层融合 | 向量+BM25+图遍历+RRF | 工具主动+core 常驻 | 向量定位+图扩展 | 键/语义/过滤 | 混合评分（断） | FTS+向量+RRF+预算+profile 常驻 |
| 注入 | reactive | reactive | proactive+常驻 | 均可 | 均可 | reactive（断）+工具 | 混合：常驻卡+预算注入+工具 |
| 后台整理 | 无（新算法写入即完成） | 增量管线 | sleeptime | memify | background reflection | 有（未接线） | 接线+重试+死信+金丝雀 |
| 观测闭环 | 无公开 | 无公开 | 无公开 | pipeline 元数据 | 无公开 | **错误指标** | 三段计数+金丝雀+评测集 |
| 多 agent | scope 隔离 | scope 隔离 | 单 agent | dataset 隔离 | namespace | agent+user | 四级 scope+授权+team 经验 |

主要来源：Mem0(arXiv:2504.19413 + 2026 算法博客 + issue#4573)、Zep(arXiv:2501.13956)、A-MEM(NeurIPS 2025)、MemoryOS、Nemori、Memory-R1、MemoryBank、LangMem 发布博客(2025-02)、Letta 仓库、OpenAI Agents SDK 文档、Claude Code 文档、LongMemEval(ICLR 2025)、Collaborative Memory。完整 URL 清单见调研工作记录。
