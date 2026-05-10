# Agent 代码库检索与上下文工程——本仓库实施手册

> **文档地位**：指导 **AI 与人类** 对本项目（aranea-agents）做可验证的检索/上下文优化；含**代码地图**、**分阶段工单**、**文件级改动清单**与**验收标准**。  
> **编排层（意图梳理 / 规划 / 并行委派）** 的总体设计见 **[`agent-orchestration-total-design.md`](./agent-orchestration-total-design.md)**；本文专注 **工具与上下文**。
> **读者**：负责本仓库 Agent/Team 工具链的贡献者或自动化助手。  
> **用法**：按需执行 §5 中某一 **P0/P1/P2** 工单；每完成一单，跑一次 §7.3 的命令集做回归。

**必读预期**：请先读 **§1.5**。商业产品里的「Cursor 级」通常同时包含：**IDE 宿主**、**后台索引与增量更新**、**模型路由**、**编辑器内自动装配上下文**；本文档只覆盖其中的 **服务端 Agent：检索与工作区上下文工具**。按工单落实后能改善「找对代码」，但无法单靠本文复刻宿主侧体验。

**对齐**：[`docs/需求/23 tools.md`](../需求/23 tools.md)、[`docs/design/session-context-compression.md`](./session-context-compression.md)、[`docs/design/agent-orchestration-total-design.md`](./agent-orchestration-total-design.md)、[`docs/需求/12 memory-L0-sensory.md`](../需求/12 memory-L0-sensory.md)、[`docs/需求/15 memory-L3-semantic.md`](../需求/15 memory-L3-semantic.md)、[`docs/AGENT_RUNTIME_BOUNDARY.md`](../AGENT_RUNTIME_BOUNDARY.md)。

---

## 1. 目标与可度量指标

将「少一些 list_files、多一些高信号上下文」落成工程目标：

| 指标 | 含义 | 建议度量方式 |
|------|------|----------------|
| **Precision@k（工具）** | 前 k 次工具调用中，对用户最终任务有直接帮助的比例 | 固定任务脚本 + 人工或 LLM-as-judge 标注 |
| **工具返回 token 体积** | 单次 list/search 的平均 JSON 体积 | 日志里对工具 result 长度采样 |
| **到达结论步数** | 从首轮 user 到「可编译/通过约定测试」的 tool+assistant 轮次 | CI 评测集 |

**低效 `list_files` 的根因（在本仓库）**：工作区 filesystem 工具仅有 `read_file` / `list_files` / `write_file` / `edit_file`（见 [`internal/tools/registry/workspace.go`](../../internal/tools/registry/workspace.go)），**缺少受预算约束的字面/workspace 级搜索工具**；模型只能反复列目录或直接读大文件摸索。

---

### 1.5 复盘：能否对齐「Cursor 级」——快速索引、准确定位、准确分析、快速作答

先做**术语对齐**，避免误判「文档不够 = 做不好」：

| 「Cursor 级」体验的常见片段 | 典型依赖 | **本文工单是否直接覆盖** |
|----------------------------|---------|---------------------------|
| 毫秒～百毫秒级的全仓字面检索 | **类 ripgrep 的索引/引擎**或与 IDE **共用**索引层 | §5 **P0-WS**：**默认生产路径应为「检测到 `rg` 则用 `rg`，否则 WalkDir」**；纯 WalkDir 在大仓仍为 **O(文件数)**。要进一步逼近 IDE，可加 **P1-BGIDX**（后台摘要/文件表）|
| 一次命中「定义 / 引用 / 导出符号」 | **LSP、gopls、静态图** | **P1-SYM** |
| 「登录态在哪儿」这类弱关键词概念查询 | **向量召回 + hybrid 字面重排** | **P2-RAG** |
| **准确分析**、结论真的对 | 检索只管**材料是否相关**；分析依赖任务可判定性、模型、**构建/测试守门** | 见本节表下「分析回路」|

#### 坦率结论

1. **仅靠当前文档所列 P0，且后端弱化为纯 WalkDir**：可达「少 token、少无用 list」；**不能保证**「与 Cursor 同量级的检索延迟」。  
2. **P0-WS（生产路径优先 `rg`）+ P0-LF/RF + P1-SYM +（可选）P1-BGIDX + （可选）P2-RAG + 明示的分析/测试回路**：在 **服务端 Agent** 形态下可逼近 **「快速定位代码 + 在不少任务上较快给出可靠答案」**；是否「总快于人用 Cursor」仍取决于宿主、网络与模型。  
3. **宿主侧独享能力**（当前打开文件、诊断、光标选区自动进上下文）若未通过产品协议注入 Agent，体验和 Cursor **天然有差距**，这不是本篇能单独补完的。

#### 分析与「正确答案」的工程回路（建议写进 Team/Runbook）

| 顺序 | 内容 |
|------|------|
| 1 | 小证据：`workspace_search` 命中 → **按行切片** `read_file` |
| 2 | 静态信号：**P1-SYM** 或语言级只读检查（可后续专有工具，`shell_exec` 仅高风险 profile） |
| 3 | 行为验证：**对相关包运行 `go test` / 仓库约定脚本**后再做最终论断 |
| 4 | 若失败：用报错串 **再走一轮 L0 字面检索**（报错栈是比模糊语义检索更高精的锚） |

若在 Runner 中强制自动跑测试或长时间命令，须与 [`docs/AGENT_RUNTIME_BOUNDARY.md`](../AGENT_RUNTIME_BOUNDARY.md) 及用户对延迟/确认的期望一致。

---

## 2. 本仓库代码地图（改哪里）

读下面表格后再动工，避免重复造轮子或与 effective-tools/policy 脱节。

| 主题 | 路径 | 说明 |
|------|------|------|
| **工作区根与路径校验** | [`internal/tools/workspace/sandbox.go`](../../internal/tools/workspace/sandbox.go) | `workspace.Root()` / `workspace.ResolvePath`；一切文件访问须落在此 sandbox 内 |
| **工作区四类文件工具（ADK）** | [`internal/tools/registry/workspace.go`](../../internal/tools/registry/workspace.go)；各工具 `internal/tools/{read_file,list_files,write_file,edit_file}` | `WorkspaceToolNames` 顺序即挂载顺序 |
| **Effective tools → ADK Tool** | [`internal/tools/tools.go`](../../internal/tools/tools.go) `ToolsFromAgentEffective` → [`registry/adk_enabled.go`](../../internal/tools/registry/adk_enabled.go) `ADKToolsFromEnabled` | 新 builtins 须在 **platform catalog + biz policy + registry 挂载** 三处贯通 |
| **OpenAI legacy 原生工具回路** | [`internal/agent/native_tools.go`](../../internal/agent/native_tools.go) | **当前仅挂载 `WorkspaceOpenAISpecs`**（与 ADK 路径可能不一致）；若产品仍使用该回路，新增工具要记得同步 specs + invoke |
| **Legacy invoke（仅四类 FS）** | [`internal/tools/registry/invoke.go`](../../internal/tools/registry/invoke.go) `InvokeWorkspaceJSON` | 新方法：扩展 switch 或拆 `InvokeFilesystemJSON` + 外层分发 |
| **运行时能力提示（system cue）** | [`internal/agent/prompt.go`](../../internal/agent/prompt.go) `RuntimeCapabilityCue` | 已含 `list_files` 单次路径约束；可在此追加**探索策略**（优先 search→read→edit） |
| **平台工具种子（SQLite）** | [`internal/data/builtin_tools_seed.go`](../../internal/data/builtin_tools_seed.go) | 新工具要增 `builtinPlatformToolSeeds` 行，`ON CONFLICT DO NOTHING` |
| **聊天侧工具结果 Markdown** | [`web/src/features/chat/toolEventMarkdown.ts`](../../web/src/features/chat/toolEventMarkdown.ts) | 未特判的工具会走通用 JSON 摘要；若 `workspace_search` 结果大而丑，可加与 `list_files` 同级分支（截断表格/列表） |
| **Profile 预设** | [`internal/biz/agent_effective_tools.go`](../../internal/biz/agent_effective_tools.go) `toolProfiles` / `toolGroupsFilesystem` | `read_only`/`coding` 等是否默认带上新检索工具 |
| **Shell（高风险）** | [`internal/tools/shell_exec/tool.go`](../../internal/tools/shell_exec/tool.go) | 可用 `rg`/`find` **但不应用作默认检索路径**：默认应走只读、`max_results` 可控的专用工具 |

---

## 3. 「混合检索」在本项目中的具体含义（对应旧版 §1.2-(1)）

**原则**：不向模型承诺「和 Cursor 闭源实现一致」，只落地**可复制**的组合：

| 层级 | 能力 | **在本仓库的落点（现状 / 要做）** |
|------|------|-----------------------------------|
| **L0 字面检索** | 子串或安全子集正则、`glob`、按文件名 | **缺口**：工单 **P0-WS**（新工具，见 §5） |
| **L1 Git/变更锚点** | `git status` / `diff`（可选） | **缺口**：可先通过受控 `shell_exec`（full profile）或由专用只读工具包装；优先级低于 P0-WS |
| **L2 符号/LSP** | 跳转到定义、引用 | **中长期**：工单 **P1-SYM** |
| **L3 语义/向量** | hybrid：向量候选 + 字面重排 | **长期**：工单 **P2-RAG**；与会话/L3 memory 管线对齐 |

**单行结论**：先做 **带硬预算的 workspace 字面搜索（默认 `rg` + WalkDir 回退）**，再考虑 **P1-BGIDX** → Git/LSP（P1-SYM）→ RAG。

---

## 4. 设计约束（所有实现必须遵守）

1. **沙箱**：只允许访问 `workspace.ResolvePath` 可解析的路径；禁止把绝对路径或可穿越 `..` 的原始字符串直接交给操作系统 API。  
2. **输出预算**：任何「扫描类」工具必须有 `max_results`（默认如 40～80）、单行/单文件 excerpt 上限、总输出字节上限。  
3. **DoS**：正则搜索须限复杂度（可选用 RE2、`regexp` 编译失败即拒；或只允许 `Substring`/`FixedString` 首版）。  
4. **忽略噪音**：可选实现「尊重 `.gitignore`」（若嵌入调用 `rg` 则开箱即有）。  
5. **双通路一致**：若维护 OpenAI-native 回路，新增工具须同时更新 **`WorkspaceOpenAISpecs`/`Invoke*`** 中与 ADK 等价的契约（见 §2）。  
6. **Catalog 一致性**：新增 `tool_key` 必须与 `biz`/`data`/`web`（若有硬编码列表）对齐；参考现有 `list_files` 行。

---

## 5. 分阶段工单（AI 可按单实施）

以下为**独立可复制**工单；每项含「改哪些文件」与「完成定义」。

---

### P0-WS：`workspace_search`（或 `grep_workspace`）字面检索工具

**目标**：给模型一个不依赖 shell 的、**默认 `max_results` 截断** 的工作区搜索能力（子串或可配置 regex + 可选路径 glob）。

**前置**：无。

**建议实现骨架**：

1. **新建包** `internal/tools/workspace_search/`（名称与 `tool_key` 一致即可），export：
   - `Run(ctx, args map[string]any) (map[string]any, error)`
   - `New() (tool.Tool, error)`（functiontool，`Name` = catalog key）
   - `OpenAIFunctionSpec() map[string]any`

2. **参数（推荐）**：  
   `query`（必填）；`mode`：`substring`|`regex`；`path_prefix`（可选）；`glob`（可选文件名 glob）；`max_results`（可选 int，默认 `50`，硬顶如 `500`）；`max_matches_per_file`（可选）；`context_lines`（可选，每条匹配前后行数）。

3. **返回（推荐）**：`matches`: `[{ "path","line","column","snippet" }]`，若超过预算在顶层加 `truncated: true`。  

4. **后端选型（与「Cursor 级延迟」直接相关——请按优先级实现）**：  
   - **首推（对齐商业 IDE 字面检索体感）**：在 **只读、`PATH` 上存在 `rg`** 且沙箱 cwd 定于 `workspace.Root()` 的前提下，封装一次 `rg` 调用：`--follow`（按需）、`-S`（smart case）、`-n`/`--column`、`--glob`（与参数的 `glob` 对齐）、`--max-count`（映射 `max_matches_per_file`）、`--max-filesize`（防巨型单文件）、`--json`（便于解析）、**硬性 wall-clock 超时**（如 10～30s，可配置）。结果仍受 `max_results` 裁剪。  
   - **必选回退**：无 `rg` 或调用失败 → `filepath.WalkDir` + **默认跳过清单**（与 §7.4 一致）+ UTF-8 文本启发式（忽略明显二进制）；大仓必须通过 `path_prefix` 收窄。  
   - **不推荐**：默认把字面检索唯一路径绑在 `shell_exec` 上调 `rg`（模型与高权限工具耦合、难审计）。

5. **验收补充（ latency ）**：在 **与本仓库体量相当**的工作区、`path_prefix="."`、常见查询下，单次 `workspace_search` P95 耗时：有 `rg` 时应 **远低于**纯 WalkDir 全仓首轮（可把两种后端分别打 Bench 记在 PR）。

6. **接入 registry**：  
   - [`internal/tools/registry/keys.go`](../../internal/tools/registry/keys.go) 新增常量例如 `WorkspaceSearch = "workspace_search"`。  
   - [`registry/adk_enabled.go`](../../internal/tools/registry/adk_enabled.go)：在 `WorkspaceADKTools` 之后**追加**该工具调用 `workspace_search.New()`（不要塞进 `WorkspaceToolNames` 以免破坏四类 FS 的既有假设，除非你愿意全面回归 UI 排序语义）。  

7. **Catalog / policy**：  
   - [`builtin_tools_seed.go`](../../internal/data/builtin_tools_seed.go) 增加一行（`category: filesystem` 或新建 `filesystem_search`，与 UI 归类约定一致）。  
   - [`biz/agent_effective_tools.go`](../../internal/biz/agent_effective_tools.go)：`toolProfiles` 中至少 `coding`、`research`、`full` 应包含该 key（或通过 `expandToolGroup` 增加 `group:filesystem_search`）；`read_only` 是否要给由产品决定（建议 **给**：只读且风险低）。

8. **Legacy OpenAI**：若仍使用原生工具列表，[`internal/agent/native_tools.go`](../../internal/agent/native_tools.go) 需挂载该 spec，`executeNativeFilesystemTool` 或并列函数能 dispatch。

9. **描述文案**：tool `Description` 中写明「**先 search 缩小范围再 read_file；不要为用 search 而用 search——有明确路径时直接 read**」。

10. **（可选 UI）**：在 [`toolEventMarkdown.ts`](../../web/src/features/chat/toolEventMarkdown.ts) 为 `workspace_search` 增加与 `list_files` 类似的截断渲染，便于人类读日志与回放。

**验收**：  
- `go test ./internal/tools/workspace_search/...`（含边界：越权路径、`max_results` 截断）。  
- 启用该工具的 Agent 在一次「找字符串 X」任务中，`list_files` 调用次数 **≤** 人工基线或未实现前日志对比。

---

### P0-LF：收紧 `list_files` 的输出预算（可选但与 P0-WS 强互补）

**目标**：防爆目录 Listing。

**改 [`internal/tools/list_files/tool.go`](../../internal/tools/list_files/tool.go)**：  
新增可选参数：`max_entries`（默认如无则返回全量——为兼容可默认 `0`=不截断）；或默认 `max_entries=200`。截断时在结果中加 `truncated: true`。  

**可选**：`depth`/`sort`/`dirs_only` 按复杂度迭代。

**验收**：对大型 `node_modules`（若 sandbox 内含）仍能快速返回且不撑爆上下文。

---

### P0-RF：`read_file` 部分读取（中高价值）

**改 [`internal/tools/read_file/tool.go`](../../internal/tools/read_file/tool.go)**：  
增加可选 `offset` + `limit` **或**「按行区间」`(start_line,end_line)`，避免全文读入。  

**验收**：对大文件仍能返回可读片段并可与 P0-WS 的 snippet 衔接。

---

### P0-PROMPT：系统提示与 Runtime cue

**改 [`internal/agent/prompt.go`](../../internal/agent/prompt.go)** `RuntimeCapabilityCue`：追加短规则（中英文均可，与用户语言一致为佳），例如：

- 有 `workspace_search` 时：**探索顺序** `workspace_search → read_file → edit/write`；无明确关键词才 `list_files` 且每层只列一次。
- **禁止**为用工具而用工具：`list_files("")` / 根目录仅作最后手段。
- **`read_image`/`memory_search`/`docs/`**（若启用）：需求分析类任务优先读文档与记忆。

**验收**：抽样 3 条真实会话，模型首轮行为符合上述顺序（可由人在 UI 或通过日志判别）。

---

### P1-SYM：Go 符号级导航（按需）

**目标**：减少「找函数定义」所需的轮次。

**候选实现**（选一）：  

- 包装 `go doc`/`go list`/`gopls query`（仅只读）；或  
- 预生成 `.json` outline（离线 job）存入 `docs/gen/`（注意 git ignore 策略）。

**接入**：仍为独立 `tool_key`（如 `go_outline`），或作为 `workspace_search` 的特殊 `mode`。  

**验收**：跨包跳转某导出符号的任务，工具调用次数下降。

---

### P1-BGIDX：（可选）工作区后台轻量索引 / 摘要

**目标**：在 **不捆绑 IDE 宿主** 的前提下，降低「从零认识大仓」的固定成本——比单纯「每次 rg 全盘」更进一步的是维护**可失效的摘要状态**。

**示例形态**（择一演进，不要求首版全开）：

1. **文件级 manifest**：mtime + hash + lang + LOC；存放在进程内 LRU 或服务端 SQLite/Bolt（见你们现有 infra 选型）。Watcher 可选用 fsnotify，或会话首次 `workspace_search` 时 lazy 补齐。  
2. **Trigram/BM25 侧车**：独立进程或对 `rg`/zoekt 的包一层 HTTP —— Agent 进程只调用稳定 API。

**与本项目衔接**：若在 Runner 常驻，需定义生命周期（何时重建索引、多 workspace 租户隔离）。

**验收**：同等查询下第二轮及以后延迟显著低于冷启动（可记录 Prometheus 或直接日志 P50/P95）。

---

### P2-RAG：项目向量检索 + hybrid（长期）

**目标**：自然语言「登录态在哪处理」类问题不靠纯字面。

**约束**：必须与 [`docs/需求/15 memory-L3-semantic.md`](../需求/15 memory-L3-semantic.md) 对齐，避免再造一套「孤岛向量库」。  

**建议**：chunk 边界用 **目录 + exported API + 文件名**；检索流程 **向量召回 topK → same-file 字面重排 → excerpt**。  

**验收**：对比无向量基线的「概念查询」precision@10。

---

### P3-TEAM：Team 运行时黑板（与 §6 提纲一致）

**改**：[`internal/team/runner_team_adk.go`](../../internal/team/runner_team_adk.go) / planner member 的系统提示：`working_memory`/共享字段记录「已搜路径 / 已知模块」，减少并行成员重复扫荡。  

详见 [`docs/需求/team.md`](../需求/team.md)。

---

## 6. 「规划-执行」在本仓库的注意点（纠偏）

- **不要盲目先拆 Planner**：在未落地 P0-WS/P0-RF 时，Planner 往往只能产出「去读某目录」类空泛子任务。  
- **Planner 输出的子任务应带来可观测成功判据**（例如：「找到 `grep`/`workspace_search` 唯一命中路径 ≤2」）。  
- **Worker 配额**：可对 `list_files` 深度与次数设团队级默认值（需在 Team definition 或 settings 文档化）。

---

## 7. 安全、守门与自检

### 7.1 安全

- 新检索工具必须为 **readonly**；与 `shell_exec` 严格分离。  
- 正则与时间上限防 ReDoS。  
- 不在工具结果内返回二进制全文；**默认跳过清单见 §7.4**。

### 7.2 守门

- 高危改写仍遵从 [`docs/AGENT_RUNTIME_BOUNDARY.md`](../AGENT_RUNTIME_BOUNDARY.md)。

### 7.3 合并前建议命令

在仓库根（工作区即为 aranea-agents）执行：

```bash
go test ./internal/tools/...
go test ./internal/agent/...
go test ./internal/team/...
go test ./internal/biz/...
```

若改了前端 Agent 工具矩阵展示，再在 `web/` 跑对应 lint/build（以 CI 为准）。

### 7.4 WalkDir/`rg` 共用默认跳过（建议常量，可配置）

减少无意义扫描与误读二进制。**至少**在目录遍历与 `rg` 参数中一致地排除：

- `.git`、`node_modules`、`vendor`、`dist`、`build`、`.cursor` 等（路径段匹配）
- 按后缀：`.exe`、`.png`、`.zip`……（任选一层 MIME/魔数判定）
- `testdata`、`*.min.js`、常见生成物是否跳过由产品权衡（须在工具描述或提示中写明，以免「搜不到fixtures」误判为 bug）。

**原则**：WalkDir 回退路径与 **embed `rg`** 路径必须 **同源 blocklist**，避免「一种方式能搜到、另一种漏」的分叉。

---

## 8. 对常见论断的辨析（精简）

| 论断 | 判断 |
|------|------|
| 效率差距主要来自上下文工程 | **成立**。 |
| 商业 IDE 「只靠向量」 | **不成立**——多为字面/符号/Git +（可选）向量。 |
| 本项目应先上 Milvus | **通常不应优先**——先补 **P0-WS**。 |
| RAG 解决「写需求」 | **条件成立**，需有可检索文档与设计输入。 |
| 双 Agent 银弹 | **不成立**，需先有高信号观测（搜索/符号）。 |

---

## 9. 非目标

- 不复制任一闭源 IDE 的内部架构。  
- 首阶段不要求分布式向量集群。  
- 不把产品设计/澄清责任全部推给检索层。

---

## 10. 文档维护

新增/改名 `tool_key` 时：**同步** `data seed`、`biz profiles`、`registry`、`（可选）native_tools`、`前端展示`，并在 PR 描述中写明 **默认 Profile 是否挂载**。

**版本**：本文为实施手册；演进以 Git 为准。

**最近一次复盘补充**：§1.5「Cursor 级」边界坦诚化；§5 P0-WS 明确 **rg 优先**与 Latency 验收；新增 **P1-BGIDX**；§7.4 默认跳过路径。
