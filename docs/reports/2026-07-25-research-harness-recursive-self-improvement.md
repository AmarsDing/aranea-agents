# 调研报告：Agent Harness 递归自我改进（抖音图文转档 + 项目启发）

> 类型：调研报告（外部内容转档 + 项目启发分析）。
> 来源：抖音图文作品（31 页轮播图）
> - 作者：CrazyAllen（个人观点，仅供参考）
> - 发布时间：2026-07-19
> - 链接：https://www.douyin.com/note/7664168308161170728 （短链 https://v.douyin.com/oFhxdAC_dOU/ ）
> - 原文案：「一文搞懂：Agent 递归自我改进。Harness 层面的递归自我改进，以及 Harness 和 Model 联合优化。#agent #harness」
> - 配图存档：[assets/harness-rsi/](./assets/harness-rsi/)（img-01.webp ~ img-31.webp，与页码一一对应）
> 背景：本文是对 Lilian Weng《Harness Engineering for Self-Improvement》的解读与补充，补充了原论文细节。

---

## 1. 全文内容整理

### 1.0 关于递归自我改进（RSI）

- **RSI 的重点不在「自我改进」，而在「递归」**。
- **I. J. Good（1965）**《Speculations Concerning the First Ultraintelligent Machine》提出智能爆炸论证：超智能机器能超过人类所有智力活动 → 设计机器本身也是智力活动 → 超智能机器能设计比自己更好的机器 → 更好的机器又能设计更好的机器 → 可能出现「智能爆炸」。这是 RSI 核心因果结构的起源：Good 认为第一台超智能机器可能是人类需要完成的最后一项发明。
- **Eliezer Yudkowsky（2000 年代初）** 系统化提出 Seed AI：初始能力有限，但能够理解自身、改进自身、然后利用改进后的能力继续改进的「种子系统」。并区分了**弱自我改进**与**强递归自我改进**。
- 两个角色定义（后续各论文命名不同但本质相同）：
  - **改进者**：负责发现问题、提出修改计划。
  - **被改进对象**：被修改的东西（代码、模型、知识等）。
- **弱自我改进**：被改进对象不断变好，但改进者、改进机制本身相对固定。例：人类文化知识增长——大脑认知架构基本不变，知识积累让获得更多知识更容易（局部正反馈）。Agent Skill 场景：用 Meta Skill 一轮轮优化工作 Skill，但无法知道 Meta Skill 的改进方案是否足够好。
- **强递归自我改进**：把改进者也纳入改进范畴——改进者→产生改进→改进结果反过来增强改进者→继续产生更好的改进（Meta Skill v1 优化出 v2，v2 再优化出 v3）。

### 1.1 主线：改进什么，以及怎么改

- 通用公式（Vivek Trivedy）：**Agent = Harness + Model**。自我改进 = Harness 层自我改进 + Model 层自我改进。
- Harness 的四个作用面（围绕 Model）：Context Injection（prompts/memory/skills/conversation）、Control（compaction/orchestration/ralph loops）、Action（calls bash/tools/MCPs，结果回 context）、Persist（filesystem/git/progress files）+ Observe & Verify（browser screenshots/test results/logs）。
- **Model 层自我改进** = 模型修改自己的权重、训练流程和部署方式：
  - Synthetic Data（自产数据、筛选验证）、Self-Play（自博弈，如 AlphaGo）、Self-Rewarding（自己当 Reward Model）；再经 DPO/RLVR 更新参数。
  - Test-Time Training（TTT）：执行中基于环境反馈更新参数（区别于 Test-Time Scaling：权重不变只加推理算力）。
  - 例：Karpathy Loop / AutoResearch（2026.3 开源）：Coding Agent 改模型训练代码（优化器、调参、架构），跑实验看指标，决定保留或回滚——属训练流程的自我改进。
- **关键线索：随着 Model 变强，Harness 层优化对象逐层上移**：prompt → context → workflow → harness code → optimizer code，最后到 Harness 与模型权重的联合优化。
- 两条主线：
  - **改进什么**：优化对象的不同层级（context、workflow、harness code、optimizer code）。
  - **怎么改**：人工设计、反思迭代、MCTS、Evolutionary Search 等（也有一条升级路线：人类设计 → Agent 根据轨迹提炼经验 → 搜索与种群进化持续探索候选）。

### 1.2 改进路线一：ACE（Agentic Context Engineering）

- 让 Agent 把每次做任务的经验自动整理成不断更新的「工作手册」（playbook），**改进的是「模型下一次工作时看到的上下文」**——context 层改进。
- 要解决的问题：持续打补丁式上下文膨胀 → 规则冲突、重要信息遗忘、行为漂移。
- 三个角色：**Generator**（用现有手册完成任务，记录执行轨迹、声明用了哪些规则）、**Reflector**（判断每条规则作用，分析有效/失败，总结规律）、**Curator**（把规律合并进结构化手册：新增/修改/合并规则）。
- 每条规则带状态计数：

```text
id: tool-017
content: 调用创建接口后，应重新查询对象确认创建成功
helpful_count: 12   // 有用的次数
harmful_count: 1    // 有害的次数
```

- **核心是一套标准化的更新协议/规则管理机制**：上下文过长时可准确定位删除什么、合并哪些规则、语义去重。每条规则有固定 ID，Curator 只能做**局部 delta 更新**（由 Python 程序执行），禁止整体重写——对抗上下文压缩时的不确定性（整体重写可能丢触发条件、失败模式等关键信息，论文称 **Context Collapse / 上下文塌缩**）。「总结是上下文压缩的手段之一，但不是做治理的手段」。
- 设计优势：**把「发现问题」（Reflector）和「修改规则」（Curator）分开**——单模型一次完成「我失败了所以新增规则 X」常没解决本质问题，或规则太细缺泛化。
- 局限：Reflector 判断不一定正确；上下文仍会膨胀（grow-and-refine：持续增加、定期去重、达上限剪枝，只是更可控）；**只改上下文，不改学习和改进机制本身**（embedding 阈值、helpful/harmful 计数机制、归因方式都是人预设的）→ **ACE 是 context 层的弱自我改进**。

### 1.3 改进路线二：MCE（Meta Context Engineering）

- 出发点：不知道哪种上下文结构和优化方法最好，**应把「优化方法」本身也作为优化对象**（Meta 的含义）。
- 核心机制：**双层优化**——内层优化 Context（被改进对象），外层优化「优化 Context 的方法」（改进者）。
- 结构：Base Level 给任务 Agent 提供 Context Function（= 动态上下文组装机制：user prompt + 从规则/失败案例/业务知识动态拼接的 system prompt）；Meta Level 有一个 Context Engineering Skill（CE Skill），根据训练集结果制定改进方案，告诉 Base Level 如何改 Context；验证分数回流改善 CE Skill。
- 示例（照片分类任务，train 500 张 / test 200 张，评价器 = 准确率）：
  1. 任务 Agent 首轮准确率 68%（证件误判为票据、带人物风景误判人物、食品包装误判美食）。
  2. Meta Agent 产出 CE Skill V1（教 Base Agent **如何从错误案例中提炼类别特征和规则**，而不是直接给具体规则）。
  3. Base Agent 结合 CE Skill 分析错误案例，补出具体规则（如「票据 vs 证件：含金额/交易日期/商户信息优先判票据；含人像/姓名优先判证件」）。
  4. 训练集 68%→88%，测试集 67%→76%，循环迭代，保留测试集更好的方案。
  5. Meta Agent 综合历史每次改进结果进行分析组合（v2 过拟合训练细节、v3 错误聚类+验证后写入更有效、v1 示例检索方式值得保留 → 组合成 skill-v4），论文称 **Crossover**。
- 局限：优化的是 CE Skill 而**不是 Meta Optimizer / Meta Agent 本身**（怎么做改进评测、怎么更有效地 Crossover、Meta Agent 工具权限等未纳入改进范畴）→ 建立了 Meta 层正反馈，但还不是完整递归。

### 1.4 改进路线三：Meta-Harness

- 一句话：MCE 优化的还是「如何生产和组织 Context」，**Meta-Harness 直接优化「Agent 是怎么运行的代码」**。连「调用几次模型、何时检索、是否先做 OCR、如何验证第一次判断、何时写入记忆」等整个程序一起改进。
- 做法：几个初始 Harness 方案作基线 → 每个 Harness 运行并保存**完整 trace**（强调：原始 trace 比短总结或单个分数更有诊断价值——侦探必须现场调查、因果归因，而不是听总结报告）→ 代码、得分、原始执行轨迹都组织保存在文件系统：

```text
history/
├── candidate_001/
│   ├── harness.py
│   ├── scores.json
│   └── traces/
├── candidate_002/ ...
```

- **Proposer**（Coding Agent）自己在文件系统自主搜索：查看不同版本 Harness 的代码差异、失败案例和失败轨迹，定位原因，写出新的完整 Harness（最复杂实验中 Proposer 每轮中位数读 82 个文件、参考 20+ 历史候选）。Proposer 只写代码，独立评价器验证效果。
- 引入 **Pareto frontier**：现实是多目标取舍（准确率 / Context 长度 / 调用次数 / Token 成本 / 延迟）。例：H006 在所有指标优于 H007 → H007 淘汰（Pareto dominance）；H005（调用少、快）、H006（准确率高）、H008（成本最低）各有优势都保留 → Pareto frontier 构成候选池，下一轮围绕 frontier 做权衡和 crossover。
- 局限：Proposer 仍由一份人写的最小 Skill 指导（写好这份 Proposer Skill 是影响搜索质量的重要因素，建议启动前先短跑调试）；Meta 层 Proposer 本身没有被闭环改进 → 仍是弱自我改进。

### 1.5 改进路线四：Self-Harness

- 更进一步：**自己改自己的系统**——一个 Harness 根据自己的执行失败记录，提出有限、可验证的 Harness 修改，让下一轮表现更好。模型权重和 Evaluator 保持固定（同一套 Harness 放不同模型上会暴露完全不同的问题）。
- 阶段一 **Weakness Mining**：在 Held-in 任务上运行当前 Harness，记录完整轨迹；关键设计是**不把每次失败当独立问题，而是寻找重复出现的失败机制**（表现出来的错误一样，根因不同）。记录 **Failure Signature** 三类信息：
  - **Terminal Cause**：最终为什么没通过（超时、编译失败、文件内容不符）。
  - **Causal Status**：Agent 行为与失败的因果关系（直接导致 / 间接促成 / 仅可疑相关）。
  - **Agent Mechanism**：从具体轨迹抽象出的、可跨任务复用的行为机制（如缺乏失败重试机制、没有强制验证门禁、延迟产物创建）。
  - 三个信息一致的失败聚成 **Failure Cluster**，输出结构化 **Failure Pattern**（出现次数、代表任务、共同轨迹症状、Verifier 证据、Agent 行为机制、是否可能被 Harness 修改解决）——**但不直接规定怎样修改 Harness，让诊断和修改保持分离**。
- 阶段二 **Harness Proposal**：同一固定模型切到 Proposer 角色，看到**受限的 Proposal Context**（允许修改哪些 Harness Surface、反复出现的 Failure Patterns、必须保留的成功行为、历史上已尝试过的修改）。只能修改 Harness Definition File 里预声明的能力，原则是**修改面尽量小、不引入无关部分**；每个 Proposal 必须说明：目标 Failure Pattern、修改哪个 Surface、预期改变什么行为、可能引入什么回归风险。
  - 允许修改的 Surface：System Prompt、Skills、Sub-agent 配置、Failure Recovery Instruction、Runtime Control Policy 等。
  - 不允许修改：Evaluator、Terminal-Bench Verifier、Held-out 数据、Self-Harness 外层循环本身、底层 Agent SDK 核心代码。
- 阶段三 **Proposal Validation**：候选 Harness 分别在 Held-in（判断是否解决了原问题）和 Held-out（轨迹不给 Proposer，判断是否破坏了未观察能力）上运行；**只有在 Held-in 和 Held-out 都不退步、且至少其一有提升才被采纳**。
- 局限：线性改进模式易陷局部最优（忽略了「短期退步但长期收益更大」的可能，没有失败/低分分支保留机制）；高度依赖 Verifier/Evaluator 质量；改动范围人为圈定（问题在不可改区域则无法修复）；**Proposal 的生成机制本身没有被改进**（Proposal Context 构建模板、字段、候选数 K 都是预设）。

### 1.6 改进路线五：STOP（Self-Taught Optimizer）

- 终于开始**改进 Optimizer（改进者）**，论文里叫 Improver。评价一个 Improver 不能只看它改好一个程序（可能偶然）→ 准备一组下游任务，计算 Improver 在任务上的平均表现，称 **Meta-Utility**。
- 初始 Seed Improver 很朴素：Best-of-N 搜索（看当前代码 → 生成多候选 → 逐个运行 → Meta-Utility 评分 → 返回最高分）——特意设计得简单，看它能否迭代出更好的自己。
- 结果：模型自发探索出很多机制——**Beam Search**（保留高分候选群避免过早锁死）、**Genetic Algorithm**（候选种群 + Mutation/Crossover）、**分解优化**（拆函数分别改进再组合）、**Simulated Annealing**（早期高温激进探索、后期低温保守修改）。
- 局限：相比真正的 Harness 简易得多（无文件系统记忆、Subagent 管理、工具权限、长期状态）——**在简易脚手架上实现了强递归自我改进**，证明了模型外部的认知支架不是只能由人类设计、支架本身可以成为模型驱动的优化对象。

### 1.7 改进路线六：DGM-H（Darwin Gödel Machine with Hyperagent）

- 最接近强递归自我改进。**Hyperagent** = 同时包含 Task Agent（被改进对象）和 Meta Agent（改进者）、**且两者都可以被修改**的统一程序；允许 Meta Agent G 修改整个 H，论文称**元认知自我修改（Metacognitive self-modification）**。

```text
Hyperagent H
├── Task Agent T   — 负责解决目标任务
└── Meta Agent G   — 负责修改 Task Agent 和 Meta Agent
```

- **DGM = Gödel Machine（递归自我改进）+ Darwin（进化）**。Darwin 的作用：放弃线性改进（Self-Harness 的缺陷：短期退步但通往更好方案的中间步骤会被提前淘汰），改为维护 **Agent Archive**——保存很多不同版本，每个版本都可以成为以后演化的 parent（child = parent.modify(parent)）。
- Parent 选择策略：**在「表现好」和「尚未充分探索」之间平衡**——分高的 parent 被选概率大；已产生很多子代的概率降低；低分 Agent 仍有非零概率被选（避免一直围绕最高分做小修小补）。
- 循环：选 parent → Meta-Agent 读取改进上下文（自己完整代码仓库、历代任务分数与评估反馈、Archive 历史 Agent、剩余迭代预算）→ 元认知自我修改（改任务 Agent、也改 Meta-Agent 自己的 prompt/工具/记忆/工作流）→ 生成 Child Hyperagent → 执行任务并评估（代码审查、论文评审、机器人奖励设计等）→ 有效性检查与入库（可运行/有分数入 Archive，无效丢弃；**child 不必立即超过 parent，低分版本也可能成为未来突破的踏脚石**）。
- 巧妙设计——如何证明 Meta-Agent「更会改进了」：**跨领域迁移实验**（训练时 Task Agent 做论文评审，新任务做奥数题评分；Task Agent 一开始不会，但 Meta-Agent 已学会如何更快改良出有效 Task Agent）。用 **improvement@k**（最多 k 次修改机会能把初始 Task Agent 提升多少）衡量，迁移后 50 次修改取得约 0.63 提升。
- 局限：能改内部 Meta-Agent，但还不能改「Archive 怎么维护」「哪个 Parent 被选中」「如何评分」「停止条件」这些外层机制（即整个外层 DGM-H 循环）。
- **重要立场：100% 意义的强递归自我改进并非目标，很可能只是构想且有难以估量的风险。关键在于「受控」——必须给 Agent 设定边界。**

### 1.8 改进路线七：SIA（Self Improving AI with Harness & Weight Updates，2026.5）

- 打破 Harness / Model 边界：**同时改 Harness 和模型权重**（Two levers, one loop）。
- 判断依据：过去两者像孤岛。Harness 改进可解释、敏捷、易回滚，但易停留在软件工程层面（输出解析更稳、工具调用更可靠、搜索重试更完善）；Model 层改进（TTT 等）把任务经验内化到参数，但承载训练执行的 Harness 不随任务结构变化。
- 本质区别：**Harness 层改进是提高「如何利用已有能力」的效率；Model 层改进是提高「已有能力本身」的质量、改造模型认知倾向**。例：基础模型单次答对 30%，Harness 可生成 10 次+验证+修改提到 80%，但单次能力仍 30%；权重更新可 30%→70%，同套 Harness 下所有节点（生成器、Reviewer）都受益。
- 机制：**Feedback-Agent** 判断当前瓶颈类别，决定下一步是 Harness Update（生成新 Harness：prompts/tools/retries/parsing）还是 Weight Update（RL 更新 rank 32 LoRA Adapter），可自由交错（H→H→W→H→W→W）。两者互相改变对方优化空间（更好的 Harness 产生更好的训练数据，更好的模型改变最佳 Harness）→ 1+1>2 的 Joint Optimization。
- 现状：目前切换是较粗粒度的阶段性切换（持续改 Harness → 得分平台期 → 切 Weight Update），未来希望更细粒度来回交错，建立深度共同演化系统。

### 1.9 优化对象上移全景 + 真正的瓶颈

- 上移路线：**Context（ACE、MCE）→ Workflow（ADAS、AFlow）→ Harness Code（Meta-Harness、Self-Harness）→ Optimizer Code（STOP、DGM with Hyperagents）→ Harness + Model（SIA）**。
- **瓶颈一：评估**。所有改进方案的基础假设是系统能准确判断什么叫「更好」（预设的 Evaluator）。代码、数学这类可验证任务相对容易；大量现实任务评价标准非常模糊（你怎么判断一篇文章是否真正有洞察？）。**Agent 自我改进的上限主要受制于它的判断系统，而不是修改能力**——如果 Evaluator 不可靠，自我改进就是「一个不可靠的系统，根据一个不可靠的裁判，不断修改自己」。Lilian Weng 把 weak and fuzzy evaluators 放在未来挑战首位：现有方法更适合快速、客观、精确指标的任务，**品味、创新性和长期价值仍然很难度量**。
- **瓶颈二：治理**。Context 和 Memory 不可避免地不断膨胀、腐化。Lilian 称 Context and memory lifecycle：随 Agent 自主性和任务周期增长，如何维护长期记忆将逐渐成为智能本身的一部分——**建立一套不会随运行不断腐化的知识系统**（对抗系统熵增）。
- **瓶颈三：可控**。人类不应简单退出循环，而应**向更高抽象层移动**：定义目标和不可违反的边界、设计评价系统、审核重要 Failure Mode 等。未来需要更多研究「人类应该在什么时机、以什么抽象层级介入」。
- **瓶颈四：衡量长期价值**。自我改进很容易陷入局部最优和多样性坍缩（人类做事也一样）；尤其在开放式研究中，真正有突破性的方向早期表现可能并不好。

---

## 2. 对本项目（Aranea-Agents）的启发

> 本项目是「多智能体编排平台」——我们本身就是用户 Agent 的 Harness 提供方。这篇文章的价值在于：它给出了一张**自我改进能力分级地图**和一套**已被论文验证的机制清单**，可以直接对照我们的模块找差距。

### 2.1 我们在这张地图上的位置

| 层级 | 代表工作 | 本项目现状 | 差距 |
|------|---------|-----------|------|
| Context 改进 | ACE | SKILL/AGENTS.md 人工维护 + session memory 自动沉淀 | **无规则状态计数、无 delta 更新协议、无 Reflector/Curator 分工** |
| Meta-Context 改进 | MCE | 无 | 无「优化方法本身」的优化层 |
| Workflow 改进 | ADAS/AFlow | 团队/Graph 定义由精灵（spirit）一次性生成 | 生成后**不根据运行反馈迭代定义** |
| Harness Code 改进 | Meta/Self-Harness | 无（Harness 即平台自身代码，只有人类改） | 无失败签名挖掘、无受限提案机制 |
| Optimizer 改进 | STOP/DGM-H | 无 | — |
| 联合优化 | SIA | 不适用（平台不拥有模型权重） | —（但 Harness 侧仍可做） |

现实定位：**平台整体处于「Context 层弱自我改进」之前**。但我们有几个独特优势可以直接跳到 ACE/MCE 级别的机制落地：

### 2.2 可直接借鉴的机制（按落地成本排序）

**启发 1：给 Skill/规则加状态计数与 delta 更新协议（ACE），对应瓶颈二「治理」**
- 现状：Skill 文件（.trae/skills、Agent 的 skill 包）只有人工增删；session memory 有沉淀但无「这条规则有用/有害」的反馈回路。
- 借鉴 ACE：每条规则/记忆条目带 `id + helpful_count + harmful_count`；更新只能走 delta 操作（add/update/merge by id），**禁止整体重写**——这正好回应我们知识库 Vault 重设计评审中担心的「LLM 重写丢关键信息」（即 ACE 论文说的 Context Collapse）。
- 关联模块：skill 系统、memory（L3 recall）、knowledge vault（37-knowledge）。

**启发 2：把「发现问题」和「修改规则」拆成两个角色（ACE Reflector/Curator 分离）**
- 现状：critic_loop 团队里 critic 既诊断又（通过回复）影响生成；没有独立的「归因」产物。
- 借鉴：诊断输出结构化 **Failure Pattern**（terminal_cause / causal_status / agent_mechanism 三段式），与修改提案分离——先聚类重复失败机制，再针对机制改，而不是一次失败加一条规则。
- 关联模块：critic_loop 团队模式、eval 域、evolution_suggestion（演进建议）。

**启发 3：完整 trace 是第一公民，比分数和总结更有诊断价值（Meta-Harness）**
- 现状：平台已有 activities 表 + trace 域 + 事件可靠性分级（AS-EVT-01），数据基础比论文场景好。
- 缺口：**没有面向「改进」的 trace 组织方式**（按 candidate/版本归档 harness 配置 + scores + traces 三元组）。若要支持任何自我改进实验，需要先定义「Harness 版本 + 评测得分 + 完整轨迹」的统一归档结构。
- 关联模块：trace 域、activities、eval 域。

**启发 4：改进提案必须是「受限 + 小面 + 可回归验证」的（Self-Harness）**
- 可声明的 Harness Surface（对照平台实体）：System Prompt、Skills、Sub-agent 配置（团队定义）、Failure Recovery Instruction、Runtime Control Policy（≈ 我们的 AgentRuntimeSetting、团队 failure_policy）。
- 提案门禁：**Held-in / Held-out 双集回归，两边不退步且至少一边提升才采纳**——可直接作为 eval 域做「团队定义自动调优」实验时的验收协议。
- 提案必须写明：目标 Failure Pattern、修改哪个 Surface、预期行为变化、回归风险——这正是我们 ADR/评审纪律的运行时版本。

**启发 5：线性改进会陷局部最优，要保留「失败但有潜力」的分支（DGM Archive）**
- 若未来做团队定义/workflow 的自动演化：不要只保留当前最优配置，维护一个 **Archive**（多版本共存，低分版本有非零概率被选作 parent）——避免围绕最高分小修小补、多样性坍缩。
- 选择策略：分数高概率大 + 已产生子代多概率降 + 低分非零概率。

**启发 6：评估器质量是自我改进的上限（瓶颈一）——投入优先级最高**
- 原文核心论断：「Agent 自我改进的上限主要受制于它的判断系统，而不是修改能力。」
- 对本项目：eval 域（LLM Judge、可验证任务评测）是任何自动改进机制的**前置依赖**。在 Evaluator 不可靠的领域（文案、品味、研究类），**不要**做全自动改进，只做「Agent 提案 + 人类采纳」（对应瓶颈三：人类向更高抽象层移动——定义边界、设计评价系统、审核重要 Failure Mode）。
- 这与我们现有的 confirm 机制（awaiting_confirmation、人工审核）方向一致，应保留为改进回路的硬门禁。

**启发 7：「改进什么」逐层上移的路径图可作平台演进路线图参考**
- prompt → context → workflow → harness code → optimizer code。平台当前能做到 context 层（知识/记忆/skill 注入优化）已是合理的第一步；workflow 层（团队定义根据运行反馈迭代）是第二步的自然候选；harness code 层（Agent 改平台自身配置/代码）必须慎之又慎，只能在受限 Surface + 双集回归 + 人工门禁下试点。

### 2.3 不建议现在做的

| 项 | 原因 |
|----|------|
| Optimizer 自我改进（STOP/DGM-H 完整版） | 依赖可靠 Evaluator + 成熟 trace 归档，前置条件不具备 |
| Harness + 权重联合优化（SIA） | 平台不拥有模型权重，超出边界 |
| 无人工门禁的全自动 Harness 修改 | 原文明确立场：强 RSI 非目标，「受控」是关键；且我们 Evaluator 尚不可靠 |

---

## 3. 参考链接

- Lilian Weng, *Harness Engineering for Self-Improvement*（原文综述，本文为其解读与补充）
- ACE（Agentic Context Engineering）/ MCE（Meta Context Engineering）/ Meta-Harness / Self-Harness / STOP（Self-Taught Optimizer）/ DGM with Hyperagent（Darwin Gödel Machine）/ SIA（Self Improving AI with Harness & Weight Updates, 2026.5）各论文
- Karpathy Loop（AutoResearch，2026.3 开源）
- 项目关联文档：[37-knowledge.design.md](../development/37-knowledge.design.md)、[2026-07-25-review-knowledge-vault-redesign.md](./2026-07-25-review-knowledge-vault-redesign.md)
