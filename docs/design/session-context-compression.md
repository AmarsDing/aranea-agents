# 会话上下文压缩（Session Context Compression）

> **文档地位**：专题设计，细化「长会话如何用 LLM 生成可复用的压缩摘要，并在后续轮次仅向模型注入摘要 + 近期原文以降低 token」的行为契约与实现落点。  
> **对齐**：与 [`docs/需求/12 memory-L0-sensory.md`](../需求/12 memory-L0-sensory.md)（L0 装配、`SummaryService`、`session_summaries`）、[`docs/需求/10 session.md`](../需求/10 session.md)、[`docs/需求/14 memory-L2-episodic.md`](../需求/14 memory-L2-episodic.md) 一致；本文聚焦**压缩触发、摘要形态、装配顺序与 ADK 持久态**，不重复 L1～L4 全栈叙述。

---

## 1. 问题与目标

### 1.1 问题

单会话消息与 ADK 事件随轮次增长，反复把**完整历史**送入模型会导致：

- prompt token 线性增长，成本与延迟上升；
- 更易触碰上下文上限，尾部消息被粗暴截断时丢失关键约束。

### 1.2 目标

在**同一会话（session）**内：

1. 当历史达到一定规模（见 §3），将**一段可追溯的对话区间**交给「压缩模型」梳理为结构化摘要，要求**不遗漏对后续决策关键的事实**（用户目标、约束、已确认结论、待办、工具结果中的关键数据等）。
2. 后续用户继续对话时，模型侧上下文改为：**压缩摘要（持久视图）+ 最近若干轮原文（滑动窗口）+ 本轮输入**，而不是每次都附带已被摘要覆盖的全部原文。
3. **账本不变**：`messages`（及必要的 trace）仍保留完整原文，便于审计、回放与重新摘要；压缩改变的是**送入模型的装配结果**，默认不物理删除历史消息。

### 1.3 非目标

- 不替代 L3/L4 长期记忆检索；摘要可与 L3 并存，但职责不同（会话内连贯 vs 跨会话知识）。
- 不把「压缩」作为唯一降耗手段：滑动窗口、工具结果裁剪等仍按 L0 策略并行存在。
- 首版不要求用户编辑摘要正文（可作为后续增强）。

---

## 2. 核心概念

| 术语 | 含义 |
|------|------|
| **滚动摘要（Rolling Summary）** | 覆盖区间 `[from_turn, to_turn]` 的 Markdown（或其它约定格式）文本，存于 `session_summaries`，表示「该区间内对话对后续的等价浓缩」。 |
| **当前有效摘要（Active Summary）** | 某 session 在装配时刻用于头部的摘要集合：可按时间或 `to_turn` 合并多条记录，或维护一条「最新全量滚动摘要」指针（实现二选一，见 §5.2）。 |
| **滑动窗口（Tail）** | 摘要区间之后、尚未被摘要覆盖的最近 K 轮或最近 T tokens 的**原始**对话（user / assistant / tool 按产品策略取舍）。 |
| **压缩模型（Compressor Model）** | 执行摘要生成的 LLM 调用；可与对话模型同厂商或降级为更小规格，须可配置。 |

---

## 3. 触发条件（何时压缩）

建议**可组合**配置（Agent / Team / Session 级覆盖），默认启用「soft + hard」双层：

| 策略 | 条件示例 | 说明 |
|------|-----------|------|
| **比例触发** | `context_used_ratio ≥ summary_threshold` | 与现有 `sessions.context_*` 字段对齐（见 L0 文档）。 |
| **轮次触发** | 自上次摘要以来新增 `Δturn ≥ compress_every_n_turns` | 防止窗口很大但比例尚未告警时长对话不摘要。 |
| **Token 估算触发** | 未摘要前缀估算 token ≥ `compress_prefix_token_budget` | 与滑动窗口预算联动。 |
| **手动触发** | UI「生成会话摘要」或 API | 便于调试与关键节点强制固化。 |

**防抖**：同一 session 在短时间窗口内（如 5～10 分钟）最多触发 N 次摘要任务（L0 文档 N4 建议 24h ≤5 次可按产品收紧）。

**并发**：若上一轮摘要尚未完成，后续触发应合并区间或排队，避免交错写入两条重叠 `from_turn/to_turn`（见 §7）。

---

## 4. 压缩任务的输入与输出

### 4.1 输入（发给压缩模型）

对选定区间，序列化**可追溯**的对话，建议包含：

- **角色与时间序**：user / assistant（及必要的 system 片段若参与事实）。
- **工具调用**：工具名、参数摘要、**结果摘要或截断后的关键字段**（全文过长时必须结构化截取，避免压缩调用本身爆窗）。
- **锚点**：`session_id`、区间 `[from_turn, to_turn]`、上轮摘要（若有）作为「禁止丢失」的约束输入。

可选：**结构化中间表示**（JSON）再交给模型生成 Markdown，便于后续机器合并。

### 4.2 输出（摘要_schema）

推荐固定章节，便于装配与人工扫读：

1. **用户意图与目标**  
2. **已确认事实 / 结论**（含数字、版本、路径、API 名等硬信息）  
3. **约束与偏好**（语言、风格、禁止项）  
4. **未完成事项 / 待澄清问题**  
5. **重要工具结果摘录**（表格或列表）  
6. **术语与别名**

输出写入 `session_summaries.summary_markdown`，并填写 `from_turn`、`to_turn`、`token_estimate`、`created_at`。可同时更新 `sessions.summary` 为「列表页 / 会话卡片用的一句话摘要」（与表字段已有语义对齐）。

### 4.3 提示词原则（对压缩模型）

- 明确：**后续对话仅能看到本摘要 + 最近几轮**，要求在无损前提下最大化密度。  
- 要求：**不得编造**；不确定处写入「待澄清」。  
- 要求：**保留可执行细节**（命令、配置键、错误码、文件路径）。  
- 可选：输出末尾附「置信度 / 遗漏风险提示」供日志使用（不默认展示给终端用户）。

---

## 5. 后续轮次如何装配上下文

### 5.1 装配顺序（与 L0 一致）

在单次模型调用前，L0 装配器按段拼装（参见 `12 memory-L0-sensory.md` §5），会话压缩对应 **`summary` 段**：

1. 系统 / 开发者固定段（SOUL、策略等）  
2. **`session_summaries` 合并摘要**（标记 `source: session_summaries:<id>`）  
3. L1 工作记忆字段（若有）  
4. **滑动窗口内原始 messages**（摘要区间之后）  
5. L3/L4 检索段（若有）  
6. 本轮 user 输入  

要点：**被摘要覆盖的旧消息不再重复进入 prompt**，但仍可从 DB 读取用于 UI 与合规。

### 5.2 多条 `session_summaries` 的合并策略

两种实现路径，二期可演进：

- **A. 区间链式**：保留多条记录，装配时按 `from_turn` 排序拼接为一块「历史摘要」文本（简单，文本略冗余）。  
- **B. 单条滚动**：每次压缩后生成新摘要时把「旧摘要 + 新区间对话」一并输入，产出**一条**覆盖 `[0, current_to]` 的新摘要并 supersede 旧指针（token 更省，单次成本高）。

首版建议 **A**，逻辑清晰且易于回放；并在 metadata 中记录 `supersedes_id` 以备迁移到 B。

---

## 6. 与 ADK 会话持久态的关系

当前 Runner 侧会话状态保存在 `sessions.adk_snapshot_json`（`BizSessionService`：`Get` 支持 `NumRecentEvents` / `After` 等裁剪）。会话压缩与 ADK 有两种协同方式：

| 方案 | 做法 | 优点 | 风险 |
|------|------|------|------|
| **装配层优先（推荐首版）** | 不改变 `adk_snapshot_json` 内全量事件；仅在构造发往 LLM 的 `Contents` / messages 时应用摘要 + tail。 | 实现集中、可逆、与现有 messages 账本一致。 | ADK 内部若独立推算上下文，需确认走同一装配入口。 |
| **快照裁剪（可选）** | 在摘要固化后，对 snapshot 中早于 `to_turn` 的模型事件做归档或删除，仅保留 state delta 与摘要引用。 | 持久态更小。 | 回放 Runner 历史不完整，需额外归档存储。 |

**结论**：首版采用 **装配层优先**；若未来 token 压力来自 snapshot 同步路径，再评估「快照裁剪 + 归档」。

---

## 7. 一致性、失败与重试

- **幂等键**：`(session_id, from_turn, to_turn, prompt_hash)`；重复任务返回已有记录。  
- **失败**：摘要失败时不阻断用户发消息；回退为「仅滑动窗口截断」并打 `context_status`/告警日志。  
- **部分完成**：若压缩调用超时，可写入 `session_summaries` 状态为 `partial`（若扩展状态字段）或使用 `warning_codes_json` 记录。  
- **观测**：写入 `memory_l0_assembly_snapshots`（或等价 span）中的 `summarized_turn_from/to`、`summary_token_estimate`、`segments_json` 段来源，满足 L0「可溯源」要求。

---

## 8. Team 会话

- **隔离**：每个子 Agent 应有独立的摘要区间与 `session_summaries` 维度（或通过 `agent_id` / `run` 维度区分），避免 Host 与子 Agent 上下文串扰（对齐 L0 F10）。  
- **Host 摘要**：可仅摘要「路由级」对话；专家会话单独滚动摘要。

---

## 9. API / 配置 / UX（摘要）

| 层次 | 建议 |
|------|------|
| **配置** | 扩展 `agent_runtime_settings`：`summary_threshold`、`compress_every_n_turns`、`recent_window_turns`、`recent_window_tokens`、`compressor_model_profile`；另设 **`l0_compress_provider` / `l0_compress_model`**（可选）：指定专用压缩调用（OpenAI 兼容），缺省时回落为当前会话/ Agent 的对话 provider与 model。压缩调用封装在 **`internal/compress`**（Catalog + HTTP client + `chatagent.CallOpenAICompatChat`），与会话编排 `session_compress` 解耦。 |
| **API** | `SummaryService.SummarizeRange(session_id, from, to)`（内部 HTTP/gRPC 由 `api/**/*.proto` 落地时遵循平台规范）。 |
| **UI** | 会话详情展示「已摘要至第 N 轮」标签；可选展示摘要正文（只读）；手动触发按钮。 |

---

## 10. 风险与缓解

| 风险 | 缓解 |
|------|------|
| 摘要丢失关键约束 | 结构化输出章节 + 保守触发阈值 + 尾部保留足够轮次 |
| 压缩调用成本高 | 更小模型、更长触发间隔、批量区间合并 |
| 与工具链语义漂移 | 摘要中显式保留工具名与结论绑定 |
| 合规与审计 | 原文永远在 `messages`，摘要仅派生视图 |

---

## 11. 落地里程碑（建议）

1. **M1**：实现 `SummarizeRange`，写入 `session_summaries`；L0 装配器读取并注入 summary 段；集成测试覆盖「摘要 + tail」token 低于「全量」。  
2. **M2**：与 `context_used_ratio` / 轮次策略联动自动触发；观测写入 assembly snapshot。  
3. **M3**：Team 维度与手动触发 / UI；可选迁移到单条滚动摘要策略 B。

---

## 12. 参考代码与数据落点（索引）

- 会话表：`internal/data/ent/schema/session.go`（`summary`、`context_*`、`adk_snapshot_json`）。  
- ADK 会话服务：`internal/agent/adksvc/session_service.go`（`Get` 裁剪、`AppendEvent`）。  
- 压缩 LLM：`internal/compress`（与编排 `internal/service/session_compress.go`）。  
- 摘要表 DDL：`internal/data/sessionmemory/memory_chain.sql`（`session_summaries`、`memory_l0_assembly_snapshots`）。
