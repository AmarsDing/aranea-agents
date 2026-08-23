# 精灵三种模式：会话内容分析与修复方案

> 执行时间：2026-08-23 20:41–20:45（CST）  
> 环境：Docker `aranea-admin` HTTP 8810，模型 deepseek-v4-flash，账号 `dev / dev123456`  
> 证据：`f:\myproject\test\spirit-modes-20260823\`（本次）+ `f:\myproject\test\chat-e2e-20260823\`（上午 TASK-B/C）  
> 对照契约：`internal/scenario/system/prompts/DECISION.md`、`CAPABILITIES.md`、`IDENTITY.md`；产品三种模式见 `docs/development/1-chat.md` §1.7  
> 纪律：初稿只分析与给方案。**2026-08-23 晚已按最优方案落地**（SM-01～SM-06 + prompt），复测见同目录 `03-spirit-modes-retest.md`。

---

## 一、测了什么

| 产品模式（1-chat） | 编排 mode（DECISION） | 用例 | 输入意图 | 实测路径 |
|--------------------|----------------------|------|----------|----------|
| A 简单对话 | `direct` | A1 闲聊 | 一句话介绍自己 | 直答，无工具 |
| A 简单对话 | `direct` | A2 事实-时间 | 「现在几点了」 | `datetime` 直答 |
| A 简单对话 | `direct` | A3 事实-天气 | 「北京今天天气」 | **跳过常驻 `web_research`**，两次 `web_fetch` |
| A 简单对话 | `direct` + 澄清 | A4 歧义 | 「帮我做一个应用」 | 先问 3 个问题，未组队 |
| A 简单对话 | `direct` + 记忆 | A5 记住偏好 | 「记住：以后都用中文…」 | **口头答应，未调 `memory_remember`** |
| B 子代理 / 并行 | `parallel` | B1 | 三件独立运维知识题，并行子代理 + 汇总表 | `plan_and_execute(mode=parallel)`，花名册绑定成功 |
| C Team / DAG | `dag` | C1 | 三人运维专家团队写巡检处置说明 | `plan_and_execute(mode=dag)`，**塌成 1 岗 1 队** |
| B（上午） | 降级 `subagents_spawn` | TASK-B2 | 三步 TCP/UDP，每步一个子代理 | 并发确认竞态后精灵**自己答完** |
| C（上午） | 降级 `subagents_spawn` | TASK-C | HTTP/2 vs HTTP/3 三人报告 | `plan_and_execute` 因「研究/调研」名册缺失失败，降级后报告质量高 |

---

## 二、总评

**Mode A 主路径可用**：闲聊身份正确、时间查询干净、歧义会先问。  
**Mode B/C 会选对 mode、也能绑上运维岗**，但会话内容有三类硬伤：

1. **对人说话与真实编排不一致**（说「三人团队已组建」，工具结果只有 1 个 Agent）。
2. **编排引擎往知识问答里塞事故复盘**（用户说「故障诊断思路」就被当成事故闭环）。
3. **根会话过早标 completed**，用户先看到「请稍候」，真正的表/说明要等合成推送，或一直等不到。

上午 TASK-B/C 的「链路 PASS」仍然成立（能降级、能出报告），但**按用户指令对照会话内容，B 没有真的完成三人委派，C 没有走出 Team 编排**。

---

## 三、逐案会话内容

### A1 闲聊 · PASS

- 回复：「我是 Aranea 系统的精灵管家，你的统一对话入口……」
- 无 `plan_and_execute`，符合 DECISION「闲聊自己答」。
- L3 召回了「用户名叫张伟」等记忆，回复里没用上（可接受）。

### A2 时间 · PASS

- 调 `datetime`，最终回复只有 `20:41`，指令遵循好。
- 未组队。

### A3 天气 · 路径偏差（内容可用）

用户要一两句话。精灵先说「我来查一下」，然后：

1. `web_fetch` 百度搜索页 → 「网络不给力」
2. `web_fetch` `wttr.in/Beijing` + 天气网 → 给出 27℃ / 多云有雾

**问题**：DECISION 2b 与 spirit 常驻集规定事实查询走 `datetime` + **`web_research`**；`web_fetch` 在 deferred 目录，应是兜底。本次直接打 `web_fetch`，且先打百度 HTML（注定垃圾）。回复本身事实完整，但多一轮失败、绕开了设计入口。

根因倾向：模型把「查网页」理解成 `web_fetch`；`web_research` 虽常驻，prompt 优先级不够，也没有「事实查询只允许这两把刀」的硬门。

### A4 歧义澄清 · PASS（内容质量高）

「帮我做一个应用」没有组队，先问类型 / 核心功能 / 技术栈，并给了一个可选用的网页工具默认，**停下来等用户**，没有按推荐默认偷偷开工。这是 DECISION 第 1 条（阻塞性歧义先问）的正确行为。

### A5 记忆 · FAIL（会话撒谎）

用户原话同时命中 IDENTITY 三条触发词：「记住：」「以后都」「我的习惯是」。  
回复：「好的，我已记住您的偏好：……」  
Activity：**没有 `memory_remember`**，只有 reply。

下次新会话不会真正记住。这是「对用户声称已写入、实际未落库」。

`memory_remember` 在 `internal/tools/deferred/split.go` 的 spirit 常驻集里，不是工具不可用，是模型没调。

### B1 并行 · 半成功（编排对，交付体验坏）

`plan_and_execute` 参数正确：`mode=parallel`，三个运维子任务分别绑到 `ops_auto_inspection` / `ops_fault_diagnosis` / `ops_doc_generation`。上午 TASK-C 的「名册没有研究岗」在**运维措辞**下不再出现。

但工具结果是 **5 个子任务**，多了：

| 多出来的节点 | 来源 |
|--------------|------|
| 结果汇总表格组 | planner 把「最后汇总成表」又拆成一队（合理） |
| **事故复盘** | 引擎 `appendClosedLoopPostmortem`：用户句里有「故障」二字即追加 |

`closedLoopSignalPattern` 是 `告警|事故|故障|宕机|停机|复盘|…`（`task_planner_impl.go` L1887）。「用三句话说明**故障**诊断的基本思路」被当成事故闭环。复盘 description 还要求「告警分诊、根因定位、修复方案」——这场对话里根本没有事故。

时间线：

| 时刻 | 用户看到的 |
|------|------------|
| +19s | 精灵说「**已完成**：已启动并行编排……请等系统通知」。根会话 `status=completed` |
| +2min | 合成总结推送：三件知识题其实做成了，但整篇总结被「事故复盘失败、无交付物」占据 |

用户要的是一张九行表，得到的是一份五团队事故复盘检讨。合成文案本身诚实（承认复盘失败），但**主交付被噪声淹没**。

### C1 Team/DAG · FAIL（对人撒谎 + 结构塌缩）

精灵调用 `plan_and_execute(mode=dag)`，任务描述里写了三位专家。  
工具返回：`subtask_count=1`，唯一 Agent 是 `ops_auto_inspection`。

紧接着精灵用表格告诉用户：

| 成员 | 职责 |
|------|------|
| 网络巡检专家 | … |
| 故障诊断专家 | … |
| 文档生成专家 | … |

这三行**不是工具结果**，是模型按用户原话编的花名册。根会话约 18s 后 completed。我们等到约 2 分钟，**没有合成说明、没有最终文档**；plan 仍是 `executing`。

`dag` 契约是「每团队 ≥2 成员」。实际是 1 队 1 人，还把三人协作收成「一个文档团队」。

### 上午 TASK-B2 · 会话内容与「PASS」不符

用户明确「每一步交给一个独立子代理」。精灵用英文说要 spawn 三个（L3 已召回「用中文」）。并发 confirm 只有 1 个真正 accepted（即 BUG-02）。5 分钟超时后精灵改口：

> 这些是简单知识题，不需要子代理，我直接答。

内容本身正确，但**违背用户指定的模式 B**。首轮英文 + 放弃委派，是内容层失败；`result.md` 记 PASS 只看了 session 终态。

### 上午 TASK-C · 降级后内容好，模式不是 C

`plan_and_execute` 报 `[SPIRIT/BAD_REQUEST] no roster specialist for 研究/调研`。花名册是运维岗（`ops_*`），没有 `ux_researcher` / `trend_researcher`。`positionDomainPath` 认的是 `network_inspector`，线上岗 key 是 `ops_network_inspection`，`InferDomainPath` 填不出 `domain_path`（settings 里也是空的）。

降级 `subagents_spawn` 后，三位通用子代理把报告写完了，事实与表格质量高。用户要的是 **Team 三层 Activity 树**（graph_stage → team_stage → session），实际是 Mode B 降级。后续「介绍你自己」中文直答正常。

---

## 四、问题清单与方案

### SM-01 根会话在编排未收口时标 completed（P1）

**现象**：B1/C1 在 `plan_and_execute` 返回「orchestrate=running」后，精灵回「请稍候」，turn 结束，session=`completed`。B1 的合成靠 `checkAllTeamsCompleted` → `synthesisSummaryTrigger` 约 2 分钟后才进会话；C1 观察窗口内没有。

**为何伤用户**：列表里会话已是「完成」，主回复却是「还在做」；刷新后容易以为没有下文。

**方案**

1. `plan_and_execute` 成功且 `steps.orchestrate=running` 时，根会话保持 `orchestrating` / `running`，禁止在首轮 reply 后标 completed。
2. 仅在 `checkAllTeamsCompleted` 合成 reply 落地后（或用户取消）才 completed。
3. 首轮话术禁止写「已完成」；只允许「已开工，完成后会推送」。
4. 回归：B1 类用例在合成前 `GET /v1/sessions/{id}` 不得为 completed；合成后 messages 含用户要的表，不只是开工通知。

涉及：`internal/biz/spirit_team_usecase.go` / `internal/service` 里 session runtime.status 的收口；DECISION「idle / orchestrating / ready」与 session.status 对齐。

### SM-02 「故障」二字误触发事故复盘（P1）

**现象**：B1 知识问答被追加「事故复盘」，复盘团队失败，合成被失败分析占满。

**根因**：`appendClosedLoopPostmortem`（`task_planner_impl.go` L1887–1947）用宽正则，且只要 `len(subTasks)>=2`。注释写「不含修复/恢复等泛化词」，但「故障」单独出现在「故障诊断思路」里就会中招。

**方案（推荐收窄，不删闭环）**

1. 正则改为需要**事故现场**信号，例如：`本次事故|故障处置|故障处理|告警处置|宕机|停机|outage|incident`，去掉单独的 `故障`。
2. 增加否定：`故障诊断的(基本)?思路`、`故障排查方法`、`说明故障` 等知识问法不追加。
3. 仅在 `ClassifyTaskGear` 为 medium/heavy，或用户明确「巡检/告警/处置闭环」时追加。
4. 单测：B1 原文不得追加复盘；「机房交换机宕机，按告警处置并复盘」必须追加。

### SM-03 dag 三人团队塌成 1 岗，且回复编造花名册（P1）

**现象**：C1 `mode=dag` + 「三人专家」→ `subtask_count=1` / `ops_auto_inspection`；对用户展示三人表。

**根因（两层）**

- Planner：精灵把三人写成「一个团队协作」，LLM 分解成 1 个 subtask；`shouldForceComplex` 会强制分解，但 1 个节点仍合法。
- 精灵话术：未按工具 JSON 的 `sub_tasks` 复述，按用户愿望编造。

**方案**

1. `mode=dag` 且用户给出 N 个角色时，分解后若 `len(subTasks)<2` 或唯一队成员 `<2`，校验失败并重分解 / 回告「无法按编制组队」，禁止静默 1 人队。
2. 花名册绑定：网络巡检 → `ops_network_inspection`（或 `ops_auto_inspection`），故障诊断 → `ops_fault_diagnosis`，文档 → `ops_doc_generation`；禁止三人职责绑到同一个 `ops_auto_inspection`。
3. Prompt（DECISION / CAPABILITIES）：`plan_and_execute` 返回后，对人复述**只能引用** `sub_tasks[].name/agent_key`，禁止补不存在的成员。
4. 回归：C1 类输入 `subtask_count>=2` 或唯一 Team `members>=2`；最终 messages 有说明文档，不是只有开工表。

### SM-04 明确「记住」不写记忆（P2）

**现象**：A5 口头记住，无 `memory_remember`。

**方案**

1. 短路径：用户命中 `记住：` / `请记住` / `以后都` / `我的习惯是` 时，在 turn 入口**确定性**调用 `memory_remember`（kind=preference/constraint），不靠模型自觉。可挂在已有 `SkipForDirectReply` / `directReplyPatterns` 旁。
2. 若坚持 LLM 调工具：工具失败或本轮未调用则 reply 必须改成「我还没写入记忆」，禁止「已记住」。
3. 单测 + 实跑：A5 后再开新会话问「我要求你怎么回复」，应命中刚写入的 L3。

### SM-05 天气绕开 `web_research`（P2）

**现象**：A3 两次 `web_fetch`（百度失败 → wttr）。

**方案**

1. 事实查询（`LooksLikeFactQuery`）在注入 tools 时**只暴露** `datetime` + `web_research`，本轮不要把 `web_fetch` 放进可调列表（或 tool_load 描述写成「仅当 web_research 明确失败」）。
2. `web_research` 无 key 时返回可行动错误（去设置页配 Tavily），禁止模型改去爬百度首页。
3. 回归：A3 Activity 首个 web 类 action 必须是 `web_research`。

### SM-06 名册缺「研究/调研」且 ops 岗 `domain_path` 为空（P2）

**现象**：TASK-C 路由失败；Agent settings.`domainPath` 为空。`positionDomainPath` 的 key 是 `network_inspector`，种子岗是 `ops_network_inspection`，`InferDomainPath` 对不上。

**方案**

1. 在 `positionDomainPath` / `inferDomainOverride` 补 `ops_*` 与 `__general` 后缀别名（`ops_network_inspection` → `运维/巡检`，`ops_fault_diagnosis` → `运维/诊断`，`ops_doc_generation` → `办公/文档`）。
2. 跑一遍 `SeedRosterIdentity` 回填。
3. 知识型「调研/对比报告」：名册无 `研究/调研` 时，allocator 回退 `办公/文档`（`ops_doc_generation`），**不要** BAD_REQUEST 把整条 Team 路径打断。通用 `subagents_spawn` 只能当二次兜底。
4. 若产品要真·调研岗：种子里加一个 `研究/调研` 专项，而不是让运维包冒充。

### SM-07 中文偏好召回了仍用英文（P3，上午 TASK-B）

L3 已有 “The user prefers … Chinese”，首轮仍英文。  
方案：把召回的语言偏好写成系统硬约束（「必须用该语言回复」），不要只当 notice 扔进时间线。IDENTITY 加一句：用户消息是中文则默认中文，除非用户改口。

### SM-08 委派失败后擅自改成 Mode A（P2，上午 TASK-B）

confirm 超时/竞态后，精灵自己答 TCP/UDP，还说「不需要子代理」。  
方案：用户点名「每个子代理」时，失败应问「要重试委派还是我直接答」，默认不改模式。与 `01-fix-plan-p2-bugs.md` BUG-02 一起做：并行 confirm 先修好，超时话术再改。

### SM-09 pin / 自动标题（已有）

上午 BUG-01 / BUG-03 仍在，本次未重测。不重复方案。

---

## 五、建议实施顺序

| 顺序 | 项 | 为什么先做 |
|------|-----|------------|
| 1 | SM-02 收窄复盘正则 | 改动小、B1 立刻不再长出假复盘队 |
| 2 | SM-01 会话状态与话术 | 用户不再看到「已完成但没交付」 |
| 3 | SM-03 dag 人数校验 + 禁止编造花名册 | Mode C 才能算测过 |
| 4 | SM-04 记住落库 | 一句话偏好现在是假的 |
| 5 | SM-06 名册 domain_path + 研究岗回退 | 解开 TASK-C 的 Team 路径 |
| 6 | SM-05 / SM-07 / SM-08 | 体验与指令遵循 |

上午 BUG-01（自动标题）和 BUG-02（并行 confirm）仍按 `01-fix-plan-p2-bugs.md` 的已确认方案做，与上表不冲突。

---

## 六、判定

- [x] **修复后重测：B1 / C1 / A5 通过；A3 在无 Tavily 环境下条件通过**（见 `03-spirit-modes-retest.md`）
- [ ] 全部通过（配齐 `web_research` Key 后再验 A3 首刀）
- [ ] 严重失败、主链路不能对话
