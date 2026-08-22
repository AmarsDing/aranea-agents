# Codex vs Aranea：分项对照、评分与优化方案

> 日期：2026-08-22  
> 类型：analysis  
> Codex 源码快照：`F:\myproject\openai-codex` @ `4f39251a`  
> 深挖正文：[2026-08-22-research-openai-codex-deep-dive.md](./2026-08-22-research-openai-codex-deep-dive.md)  
> Aranea 交叉参考：[65-module-cross-reference-full.md](../development/65-module-cross-reference-full.md)  
> A–D 落地后重估：[2026-08-22-analysis-codex-vs-aranea-post-ad.md](./2026-08-22-analysis-codex-vs-aranea-post-ad.md)

---

## 0. 怎么评

两套系统 **产品目标不同**，不能用「谁更像 Codex」当总分。


|       | Codex CLI              | Aranea                                |
| ----- | ---------------------- | ------------------------------------- |
| 定位    | 本机编码 Agent harness     | 多 Agent 公司操作系统（组织/岗位/团队/知识/语音）        |
| 主用户   | 开发者本人                  | 开发者 + 业务用户 + 编制内专项 Agent              |
| 默认工作区 | 当前 git 仓库              | Workspace + Agent 工作区 + 知识库           |
| 编排    | 单线程 + 可选 spawn 子 Agent | 花名册 / PlanExecutor / Team Graph / 组织链 |


评分规则：

- 每项 1–10，只评 **该项能力的完成度与产品可用性**，不因「Aranea 还会做公司」而给 Codex 扣分。
- **平台分** 单独一行，避免编码 harness 把组织能力比没。
- 差距列只写 **Aranea 可吸收且不违反组织铁律** 的点。M67/M78 编制表、禁止精灵工具箱交差，一律不改。

加权：业务循环 15%、Context 15%、工具 15%、工具调研 10%、Skill 10%、MCP 10%、Prompt 10%、记忆 15%。  
平台能力不进加权总分，单独报告。

---



## 1. 总分


| 维度                     | Codex   | Aranea  | 加权差（Aranea − Codex） |
| ---------------------- | ------- | ------- | ------------------- |
| 业务循环 / Harness         | 9.0     | 7.0     | −0.30               |
| Context                | 8.5     | 7.0     | −0.23               |
| 工具（执行面）                | 9.0     | 7.5     | −0.23               |
| 工具调研（延迟目录）             | 8.5     | 7.0     | −0.15               |
| Skill                  | 8.0     | 7.5     | −0.05               |
| MCP                    | 8.0     | 8.0     | 0.00                |
| Prompt                 | 9.0     | 6.5     | −0.25               |
| 记忆                     | 8.5     | 7.0     | −0.23               |
| **加权总分**               | **8.6** | **7.3** | **−1.3**            |
| 平台 / 组织 / 知识 / 语音（不加权） | 3.5     | 8.5     | —                   |


读法：在「编码 Agent 怎么干活」上 Codex 仍领先约 1.3 分；在「公司怎么运转」上 Aranea 远超。二次源码对照后上调了 Aranea 的工具调研（MCP broker 已是默认 coding profile）和记忆（L0–L4 + profile card + sleep-time **代码已在**，历史断环是运营债不是空白）。优化目标不是追平 8.6，而是把 **编码专项 + 精灵桥接 + 通用 Agent 上下文合同** 拉到 8.0+，同时保住平台分。

---



## 2. 分项对照



### 2.1 业务循环


| 点       | Codex                                              | Aranea                                                                                                                                                                     |
| ------- | -------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 主循环     | `submission_loop` 统一 Op：输入/审批/压缩/挂起/子 Agent/MCP 刷新 | ChatOrchestrator 分 phase + Runner；审批/澄清/编排各有入口                                                                                                                             |
| HITL    | Exec/Patch/Permissions/UserInput 都是 Op 回传          | 工具确认门 + clarify；编码桥 **M2 审批中继已落地**（卡片 + 本任务 always + 超时）；claude/codex adapter 仍待                                                                                                                                            |
| 恢复      | RecoverTurn、SuspendTurn、resume/fork thread         | Durable Resume、Graph checkpoint；chat 域无「挂起未完成根 turn」                                                                                                                       |
| 子 Agent | 模型工具 spawn/wait/interrupt                          | Team Graph / 组织链 / PlanExecutor；**不是**模型随手 spawn                                                                                                                           |
| 沙箱      | OS 级 Seatbelt / bwrap / Windows token              | 工作区路径约束 + 确认门；无 OS sandbox                                                                                                                                                 |
| 编码桥     | 自己就是编码 Agent                                       | M76：**派发/查询/取消工具 + ACP 客户端 + API 已落地**（M1-1~12/15 ✅，默认 `enabled=false`）；M1 冒烟与 M2 审批中继未收口。另有 `trpc-agent-go/agent/codex` 的 `codex exec --json` 与 registry `claudecode` 内置桥 |


**评分：Codex 9.0 / Aranea 7.0**

Aranea 的编排与状态机（Team/Graph/Run）在平台侧更强；缺的是 **编码场景的 OS 沙箱 + 统一控制面 + 未完成 turn 挂起**。  
**禁止** 把 Team 改成 Codex 式 spawn_agent。组织铁律：编排绑花名册，不把员工做成精灵分身。

**可做：**

- 给编码专项 / computer-use / shell 补「工作区沙箱策略」一等配置（只读 / 工作区写 / 全盘），先产品合同后 OS 落地。
- M76 按既定 ACP 方案做完审批中继（不要再开第四套协议）。
- Chat 域补「turn suspend」语义时对齐 Codex 的 durable writer-close 再退出，避免丢磁带。

---



### 2.2 Context


| 点    | Codex                                          | Aranea                                     |
| ---- | ---------------------------------------------- | ------------------------------------------ |
| 项目手册 | `AGENTS.md` 链运行时注入，未信任项目跳过                     | 仓库有 `AGENTS.md`，**运行时不按目录树注入到 Agent**      |
| 前缀稳定 | base + developer 分层，动态 cue 靠后                  | 三层前缀 + insertAfterLastSystem；方向正确          |
| 压缩   | LLM 交接摘要（checkpoint compaction）                | 默认确定性截断（不调 LLM）；另有 session compressor / L0 |
| 预算可见 | `get_context_remaining` / `new_context_window` | context_budget 台账 + 指标；模型不可自助开新窗           |
| 工具输出 | exit/时长/行数 + 截断                                | 多数工具摘要/截断，格式不统一                            |
| 知识   | 不预注入代码 chunk，靠 rg                              | 知识库 JIT 工具 + 目录 cue（已对标正确）                 |


**评分：Codex 8.5 / Aranea 7.0**

2026-08-13 上下文报告已经指出：方向对，闭环和度量不够。Codex 多出来的是 **项目文档运行时化** 和 **压缩=交接**。

**可做：**

1. **P0** 给「编码/仓库型 Agent」实现 `AGENTS.md` 加载器（可复用 trpc-agent-go 已引用的 Codex `project_doc` 思路）：cwd→root，override 优先，字节封顶，未信任根不读。
2. **P1** 在硬截断之前增加可选「交接摘要」路径（小模型，CAS 守卫，写入 L0/会话状态），prompt 用 Codex compact 合同：进度/决策/约束/下一步。
3. **P1** 工具输出统一信封：`exit` / `duration` / `truncated` / `total_lines`。
4. 不把整本知识库预注入——两边都已否定这条路。

---



### 2.3 工具（执行面）


| 点         | Codex                          | Aranea                              |
| --------- | ------------------------------ | ----------------------------------- |
| 核心编辑      | `apply_patch` 一等工具             | `diff_edit` / `patch_file` 已有，种子已启用 |
| Shell     | unified exec + PTY stdin + 多环境 | shell 工作区约束；无跨环境 environment_id     |
| 计划        | `update_plan` 渲染给用户            | 编排规划在平台侧；单 Agent 少「可见 TODO」         |
| 并行        | registry + MCP 声明              | 工具并行已有 ADR；文件 worktree 隔离在 70 号规划里  |
| 安全        | 沙箱 + execpolicy + 审批           | 风险级 + confirm_intent + 脱敏审计         |
| Code Mode | 进程内脚本调工具                       | 无对等物（不急）                            |


**评分：Codex 9.0 / Aranea 7.5**

Aranea 管理面（列表、schema、审计、在线测试、allow/deny）比 Codex 完整。执行面差在 **补丁工具的模型合同** 和 **沙箱**。

**可做：**

- 把 `apply_patch` 语义写进编码专项的 system prompt（只准这一种编辑，失败不要再读文件）。
- 编码专项默认开 `update_plan` 类可见步骤（复用现有 plan 工具或轻量 TODO），交互审批模式下测/lint 先问。
- OS 沙箱列为专项，不阻塞其他项。

---



### 2.4 工具调研（延迟目录）


| 点      | Codex                                  | Aranea                                                 |
| ------ | -------------------------------------- | ------------------------------------------------------ |
| 发现工具   | `tool_search` + BM25 + `defer_loading` | `tool_search` + `tool_load`（`internal/tools/deferred`） |
| 索引     | name+description 字段化；cache 按 registry  | token 打分 `scoreEntryAgainstQuery`；有漏斗指标                |
| 协议     | Responses API `defer_loading`          | 产品侧 DeferredToolManager + Filter，框架无原生 defer           |
| 预选     | 与 Code Mode / omit_tools_from 联动       | 分片装配 + 延迟名单；意图尚未驱动 topK（08-13 P0）                      |
| MCP 长尾 | 默认可进 Deferred                          | MCP 常随 profile 全量挂上                                    |


**评分：Codex 8.5 / Aranea 7.0**

Aranea **已经有** search/load 双工具，且 coding profile 默认走 **MCP broker**（`mcp_list_`* / `mcp_call`，schema >16K 自动降级），这是和 Codex Deferred 同方向、实现不同的第二条路。差距是：检索质量（BM25 vs 子串打分）、直连 MCP 仍可能全量 schema、意图预选未闭环。

**可做：**

1. 延迟目录检索升级为字段化 BM25（或复用知识库 BM25 基础设施），与 Codex 一样只索引 name/description。
2. MCP 工具默认 Deferred：常驻核心 3–5 + `tool_search`；`required` server 可例外 Direct。
3. 兑现 08-13 方案：intent 一次输出工具 topK，避免「付了意图钱只用一半」。
4. Broker 与 Deferred **二选一做默认**，不要两套同时全开把 cue 再灌回去。
5. 不要为了对齐去改 trpc-agent-go 的 Responses `defer_loading`，除非编码桥/OpenAI 兼容真需要。

---



### 2.5 Skill


| 点    | Codex                     | Aranea                                                    |
| ---- | ------------------------- | --------------------------------------------------------- |
| 管理面  | 文件 + plugin marketplace   | 完整 CRUD / 版本 / 冲突 / 炼化 / 运行记录                             |
| 加载   | 三级披露；`$skill`；隐式调用        | progressive overview + `skill_load`/`skill_run`；routed 标记 |
| 选择实验 | shadow 选择器族（BM25/LRU/RRF） | 路由命中率可观测（M69）                                             |
| 依赖   | skill 可声明 MCP             | Skill 不直接拉起 MCP（边界正确）                                     |
| 产出   | 记忆巩固可写出 skill             | 无「从会话长出 skill」管线                                          |


**评分：Codex 8.0 / Aranea 7.5**

管理面 Aranea 胜；运行时合同 Codex 更干净。两边都已接受 progressive disclosure，不要退回 Full Profile 默认。

**可做：**

- 支持 `$skill-name` / 结构化 mention，与 routed 列表合流。
- 可选扫仓库 `.agents/skills` / `.codex/skills`（Codex / Cursor 同构），作为 FS 后端补充，不替代 DB 管理面。
- 隐式调用只做 **观测**（Codex 的 implicit telemetry），默认不自动灌全文。
- Skill 正文继续「触发器短、references 按需」；skill-creator 的三条原则可写进 Aranea 的 skill 编写规范。
- 「从 rollout 生成 skill」放到记忆巩固之后，禁止热路径 Factory 交差。

---



### 2.6 MCP


| 点   | Codex                               | Aranea                             |
| --- | ----------------------------------- | ---------------------------------- |
| 传输  | stdio + streamable HTTP             | stdio + SSE + streamable HTTP      |
| 治理  | toml 分层、required、omit 面、per-tool 审批 | CRUD、健康灯、SSRF、OAuth、用户凭据、告警、Broker |
| 资源  | list/read resource 一等工具             | 以 Tool 为主，Resource 面弱              |
| 反向  | Codex 可当 MCP server                 | Agent 暴露为 MCP 弱                    |
| 热更新 | `RefreshMcpServers`                 | 分片缓存 + 健康探活                        |


**评分：Codex 8.0 / Aranea 8.0**

这是唯一打平的项。Aranea 管理面更深；Codex 运行时开关（required / omit_tools_from / parallel 声明）更利落。

**可做：**

- 补 MCP Resource 三件套（有 server 才挂），避免把资源硬折成 tool。
- 配置对齐：`required`、`supports_parallel_tool_calls`、`omit_tools_from`。
- 「Aranea Agent → MCP server」作为独立专项，服务编码桥/外部 IDE，不塞进日常 chat。

---



### 2.7 Prompt


| 点            | Codex                            | Aranea                                   |
| ------------ | -------------------------------- | ---------------------------------------- |
| 资产           | 独立 crate + md 模板，业务只渲染           | `BuildSystemPrompt` + 多处 BeforeModel cue |
| 编码合同         | preamble / plan / 验证时机 / 最终格式写死 | 角色 + 岗位 + 记忆自标记 + runtime cue            |
| 权限说明         | sandbox×approval 模板随配置变          | 确认门在运行时，很少用自然语言告诉模型「你现在的权限是什么」           |
| 压缩/记忆/review | 各有专用模板                           | 压缩无交接合同；记忆 cue 分散                        |


**评分：Codex 9.0 / Aranea 6.5**

这是 **ROI 最高** 的差距：不改架构，改合同。

**可做（编码专项 / 精灵 / 通用可分层启用）：**

1. 抽出 `prompts` 资产目录（不要继续把长文堆在 `prompt.go`）。
2. 编码专项 system 增加：preamble 纪律、`update_plan` 何时用、`never` vs 交互审批下的测试时机、`apply_patch` only、`rg` 优先。
3. 每轮注入 **当前权限一段话**（只读 / 工作区写 / 要审批），对标 `permissions_instructions`。
4. 记忆自标记指令继续压缩（08-13 已建议 ~150 token），与 sleep-time 抽取单轨。

组织/岗位 `<role_responsibility>` / `<industry_context>` **保留**，这是 Aranea 的产品，Codex 没有。

---



### 2.8 记忆


| 点     | Codex                                 | Aranea                                      |
| ----- | ------------------------------------- | ------------------------------------------- |
| 读模型   | summary 常驻 + MEMORY.md 按需 + citation  | L1/L2/L3/L4 cue + composite recall + 主动召回   |
| 写模型   | 启动 Phase1 抽 rollout → Phase2 巩固 Agent | 即时 extractor + consolidator；sleep-time 曾未接线 |
| 污染控制  | 无高信号则空；禁止模型改手册                        | 操作语义 ADD/UPDATE/NOOP 在 07-29 重设计里；实现曾多次断环   |
| 跨会话任务 | rollout 磁带 + 巩固                       | Graph checkpoint / 交付物信封强；chat 域缺任务状态表      |
| 形态    | 文件工作区，对编码模型极友好                        | DB 分层，对治理/合规更友好                             |


**评分：Codex 8.5 / Aranea 7.0**

Aranea 的分层与治理目标 **更正确**（企业要审计、遗忘、双时态）。代码上 L0–L4、profile card、pinned prefs、sleep-time、mid-run（每 100 events）、working memory 归档 **都已接线**。Codex 的读合同仍更干净（summary → grep 手册 → citation；另有 `memories.`* 工具）。07-29 断环是运营/正确性债，不是「没实现」；和 Codex「打开就能用」的差距在 **默认是否真正进 prompt、计数是否诚实**。

**可做（在现有 L0–L4 上叠一层，不推倒）：**

1. **Profile / Summary 卡**（07-29 D4；`memory_inject.go` 已有 profile card）：把 Spirit 默认档打满并加金丝雀——「冷启动能说出 3 条稳定偏好」，避免再出现「用了几千次命中 0」。
2. **手册层**：L3/治理台导出一份可 grep 的 `MEMORY.md` 视图（或知识库一篇），模型用现有知识/记忆工具查，不预注入全文。
3. **巩固 Agent**：sleep-time 用隔离子 Agent（无网络、无 collab、只写记忆工作区），prompt 采用 Codex Phase2 的高信号门 + no-op。
4. **Citation**：召回被用到才 `use_count++`（07-29 已点名的虚高计数）。
5. Chat 域任务状态表继续走 08-16 跨会话记忆结论，不要改存全量对话。

---



## 3. 不该抄的东西


| Codex 做法                    | 为什么不要原样搬                               |
| --------------------------- | -------------------------------------- |
| 模型 `spawn_agent` 当主编排       | 违反组织铁律；Aranea 已有花名册 / Graph            |
| 记忆只放 `~/.codex/memories` 文件 | 企业要治理台、租户、遗忘；文件只能当 **导出视图**            |
| 去掉知识库改纯 rg                  | 业务 Agent 不是只改一个 git 仓库                 |
| Code Mode 优先                | 成本高、和现有 tool schema 生态重复               |
| 放弃 MCP SSE                  | Aranea 已有客户依赖                          |
| 全员共用一套编码 prompt             | 岗位专项各自 mission；只给 coding profile / 精灵桥 |


---



## 4. 优化计划（按依赖排序）

原则：先合同与度量，再运行时行为，最后才碰 OS 沙箱 / 新协议。每项都要能在现有模块落地，不新开根目录、不写新模块编号（必要时并入 20/23/19/70/76）。

### 阶段 A — 两周内，零架构风险（Prompt + 文档合同）


| ID  | 项                                                                     | 落点                                      | 验收                                |
| --- | --------------------------------------------------------------------- | --------------------------------------- | --------------------------------- |
| A1  | 编码专项 / 精灵编程桥 增加 Codex 风格工作法 prompt（preamble、plan、验证时机、apply_patch、rg） | `internal/agent` prompt 资产 + 编码 profile | 同源任务：模型先短 preamble 再工具；不复述整份 plan |
| A2  | 每轮注入当前权限/沙箱一段话                                                        | 动态 cue，插在稳定前缀之后                         | 只读 Agent 不再声称「我可以直接改文件」           |
| A3  | 工具输出统一截断信封                                                            | shell / exec / MCP 适配                   | 截断时模型看见 total_lines               |
| A4  | Skill 编写规范吸收三级披露 + 「只写会改变决策的内容」                                       | `20-skill` 设计附录 + 内置 skill-creator 说明   | 新建 skill 描述可区分，正文 < 阈值            |




### 阶段 B — 一个月，Context / 工具调研闭环


| ID  | 项                                    | 落点                                 | 验收                                          |
| --- | ------------------------------------ | ---------------------------------- | ------------------------------------------- |
| B1  | 仓库型 Agent 运行时加载 `AGENTS.md` 链        | agent 构建 / workspace               | 嵌套目录覆盖生效；超字节截断；未信任根跳过                       |
| B2  | 可选交接压缩（小模型）                          | compress + L0                      | 硬截断前若启用，下一轮能复述「已做/未做/约束」                    |
| B3  | 延迟目录 BM25 + MCP 默认 Deferred          | `internal/tools/deferred` + MCP 装配 | MCP≥20 时首轮 schema token 明显下降；tool_load 后可调用 |
| B4  | Intent → 工具 topK（兑现 08-13 P0-1/P0-2） | intent + tool assembly             | 意图产物含 tool slugs；主模型不再默认全量 MCP schema       |
| B5  | `$skill` mention + 与 routed 合流       | skill_guidance_inject              | 用户打 `$foo` 即 load，不靠再搜                      |




### 阶段 C — 一季度，记忆读合同 + 巩固


| ID  | 项                               | 落点                      | 验收                         |
| --- | ------------------------------- | ----------------------- | -------------------------- |
| C1  | Agent 级 `memory_summary` 常驻（D4） | memory 读路径 + prompt     | ✅ 无卡回退钉住偏好；`<memory_summary>` ≤800 token |
| C2  | 可检索手册层 + 用到才计数                  | L3 导出 / 知识一文 + citation | ✅ L4 召回不再 `use_count++`；handbook index |
| C3  | Sleep-time 巩固子 Agent（Phase2 合同） | memory write / 隔离 prompt | ✅ 无高信号 no-op；密钥脱敏+拒写；不 spawn |
| C4  | Chat 域任务状态表（08-16）              | session / L1 cue        | ✅ 本会话无 L1 时注入上一会话 `task_board` |




### 阶段 D — 专项，编码桥与沙箱


| ID  | 项                                            | 落点                   | 验收                                    |
| --- | -------------------------------------------- | -------------------- | ------------------------------------- |
| D1  | M76 M2 审批中继                                  | coding bridge + 确认卡片 | ✅ ACP permission → 确认卡 + `coding_task_approval`；超时 cancelled；`allow_always` 仅本任务。adapter 仍待 |
| D2  | 工作区沙箱策略（先 Windows 受限 token / 路径策略，再评估 bwrap） | computer-use / shell | ✅ 路径策略 BeforeTool：只读写工具拒绝并回传原因；越界 `path`/`file`/`cwd` 拒绝。OS token/bwrap 未做 |
| D3  | 可选：Agent 暴露为 MCP server                      | 19-mcp 子能力           | 📋 仅评估（见 19-mcp.design.md 子模块），不实现 |


---



## 5. 优化方案（怎么做，避免做成第二套 Codex）



### 5.1 Prompt 资产化（A1/A2）

把「长字符串拼在 `BuildSystemPrompt`」拆成可测试模板：

```
<role_responsibility>     # 岗位，Aranea 已有
<industry_context>        # 变体，Aranea 已有
<working_contract>        # 新增：preamble / plan / 验证 / 编辑纪律（按 profile）
<permission_state>        # 新增：本轮只读|工作区写|需审批
<internal_config>         # 已有 prompt files
<memory_summary>          # C1，短
```

`working_contract` 只挂在 `coding` / `computer_use` / 精灵编程意图上，不污染财务/客服专项。

### 5.2 `AGENTS.md` 运行时（B1）

算法直接移植 Codex `agents_md.rs` 的纯函数部分（无 Seatbelt 也可）：

1. 从 Agent `workspace_root` 找 `.git`
2. root→cwd 每层 `AGENTS.override.md` > `AGENTS.md` > 可选 `CLAUDE.md` fallback
3. 总预算默认 32KiB，超出截断并打日志字段 `agents_md_truncated`
4. 注入位置：稳定前缀之后、动态 cue 之前，便于 cache

未纳入信任的外部目录（编码桥未注册项目）不读。

### 5.3 交接压缩（B2）

保持现有硬截断当最后防线。在 `usedRatio` 过软阈值时：

1. 调小模型，prompt = Codex compact 四段
2. 结果写入 session 状态 + L0，带版本 CAS（已有 compressor 守卫）
3. 下一轮只带摘要 + 最近 K 轮原文

失败则落回确定性截断。禁止压缩失败阻塞 turn。

### 5.4 延迟工具（B3/B4）

保持 `tool_search` / `tool_load` 双工具（Codex 是 search 后直接 loadable spec，Aranea 显式 load 更安全，**保留**）。

改三处：

1. `scoreEntryAgainstQuery` → 字段化 BM25（name 权重大于 description）
2. MCP shard 默认进 deferred catalog；`mcp.required` 或 Agent allow 白名单除外
3. Intent 输出 `tool_hints[]`，装配期把 hints 预 `tool_load`（仍记 metrics）



### 5.5 记忆读合同（C1–C3）

```
每轮：
  memory_summary（≤800 token，常驻）
  模型需要时：
    memory_search / knowledge → 手册条目
    命中再拉 fact 正文 / episode
写：
  热路径：现有 extractor（修闭环，不换存储）
  冷路径：巩固 Agent 只看 rollout/会话摘要，产出 summary + 手册 diff
```

巩固 Agent 的权限必须抄 Codex Phase2：**无网络、无团队委派、只写记忆目录/表**。否则会变成「又一次全库闲聊」。

### 5.6 编码桥（D1）

继续 76 号选定方案：ACP 统一，不把 App Server / `codex exec` 再做成第三套生产协议。  
`codex exec --json` 仅作无审批派发的降级，文档里已写明无法中途 HITL。

---



## 6. 建议排期与负责人向


| 优先级 | ID              | 预估      | 依赖        |
| --- | --------------- | ------- | --------- |
| P0  | A1 A2 A3        | 3–5 日   | 无         |
| P0  | B4（intent→topK） | 5–8 日   | 08-13 已设计 |
| P1  | B1 B3 B5        | 1–2 周   | A1        |
| P1  | C1 C2           | 1–2 周   | 记忆中心信任环   |
| P2  | B2 C3 C4        | 2–3 周   | C1        |
| P2  | D1              | 按 76 M2 | ✅ 中继已落地；adapter 仍待 |
| P3  | D2 D3           | 专项评估    | D2 路径策略 ✅；D3 仅评估不实现 |


---



## 7. 成功标准（怎么算「学到了」而不是「抄了皮」）

1. 编码专项同源任务：首轮工具 schema token 相对全量 MCP 下降 ≥50%（B3/B4）。
2. 仓库型任务：不靠用户粘贴，模型遵守本仓 `AGENTS.md`（B1）。
3. 超长会话：压缩后下一轮能复述未完成步骤，而不是从头 ls（B2）。
4. Spirit 隔日会话：无需用户重申 3 条以上稳定偏好（C1）。
5. 记忆 `use_count` 与「prompt 里真正出现的条目」相关系数转正（C2）。
6. 组织/岗位/花名册行为零回归（自动化：现有 org invariant 测试）。

---



## 7.1 二次源码对照补记（2026-08-22）

并行深挖（[Explore Codex architecture](8f718f10-29c3-4933-bf84-8a1aad732319)、[Explore Codex tools skills MCP](0581be8e-c2d4-4407-bea8-64f7891a5906)、[Map Aranea counterpart modules](019ed9c9-d0cb-415f-9648-36ffafa0c10a)）补上这些、已写入两份报告正文：

- Codex **World State diff** 是一等 prompt 层，不只是「再拼一段 AGENTS.md」。
- Codex Skill/记忆都有 **工具面**（`skills.read`、`memories.search`），不只是注入。
- Codex 压缩有第三条路：开新窗、不摘要。
- Thread / Session / Rollout 三层：`SessionId` 父子共享，`ThreadId` 各对话独立，rollout 是 JSONL 磁带。
- Aranea coding profile 已默认 MCP broker；M76 派发链路已在代码里，缺的是冒烟与审批中继，不是从零开工。
- Aranea 已有 `subagents_*` 工具，不要再为「对齐 spawn_agent」新开一条热路径。

---



## 8. 源码与文档索引


| 用途       | 路径                                                                                                                     |
| -------- | ---------------------------------------------------------------------------------------------------------------------- |
| Codex 克隆 | `F:\myproject\openai-codex`（仓库外，不入库）                                                                                   |
| 深挖       | [2026-08-22-research-openai-codex-deep-dive.md](./2026-08-22-research-openai-codex-deep-dive.md)                       |
| 上下文旧案    | [2026-08-13-research-llm-context-pipeline-optimization.md](./2026-08-13-research-llm-context-pipeline-optimization.md) |
| 记忆重设计    | [2026-07-29-review-memory-system-redesign.md](./2026-07-29-review-memory-system-redesign.md)                           |
| 跨会话任务    | [2026-08-16-research-cross-session-task-memory.md](./2026-08-16-research-cross-session-task-memory.md)                 |
| 编码桥      | [76-coding-agent-bridge.md](../development/76-coding-agent-bridge.md)                                                  |
| Skill 渐进 | [69-skill-loading-optimization.design.md](../development/69-skill-loading-optimization.design.md)                      |


