# Chat 链路 Token 消耗与缓存命中率深度分析与优化方案

> 日期：2026-10-20 | 类型：分析 + 实施方案 | 触发：用户反馈「token 消耗的标签似乎不符合逻辑，缓存命中率是否最优」
> 证据等级约定：【实证】= 本仓代码 file:line 核实；【文档】= 官方文档（训练数据，本环境外网受限无法在线复核）；【推断】= 由实证推导。

---

## 一、结论摘要（TL;DR）

本项目的 token 计费/缓存观测体系**架构成熟**（usage 事件单一事实源、P50 告警、上下文预算台账、前缀稳定契约测试），但存在 **5 个确凿的正确性/口径缺陷**与 **3 个未兑现的优化机会**：

| # | 发现 | 等级 | 处置 |
|---|------|------|------|
| F1 | Anthropic 缓存三分量（read/write/fresh）在内部提取层被压扁成二元 (prompt, cached)，cache_write 与 reasoning 被丢弃；计费公式会从 input 中二次扣减——**一旦开启 Anthropic cache_control 立即错误计费** | 【实证】critical | P0 |
| F2 | 辅助 LLM 调用链（intent/title/memory/planner/subagent，直连 `llmcompat`）不解析 `prompt_cache_hit_tokens`/`cached_tokens` → aux 行 cached 恒 0，且 aux 行参与命中率聚合 → **DeepSeek 命中率被系统性低估** | 【实证】high | P0 |
| F3 | Anthropic 的 `cache_control` 断点框架已支持（`WithCacheSystemPrompt/Tools/Messages`），但全仓无任何 catalog 种子/配置开启 → **Anthropic 系 provider 缓存命中率结构性为 0**；设计文档 §9.1 已将「显式断点」定为「阶段 1 再评估」，至今未落地 | 【实证】high | P1 |
| F4 | OpenAI 请求的 `prompt_cache_key`（会话级缓存亲和，官方推荐）从未传递 → shard 亲和缺失、命中率低于可达到水平 | 【实证】medium | P1 |
| F5 | 「缓存命中率」标签口径不一：Overview 卡片是 **P50 中位数**，TeamRunsDialog 是 **加权账单口径**，同名不同义；UsageEventsPage「来源」列裸英文枚举；chat 头 `in`（会话累计账单输入，含折价缓存读部分）与 `ctx`（当前上下文占用）并排无说明 | 【实证】medium | P0 |
| F6 | `usage_kind` 空值兜底写成 "chat"（`usage.go:1048`），意图/未知来源行可能被标成聊天 | 【实证】low | P0 |
| F7 | 计费公式 `input_cost = (input − cached_read) × 单价` 未扣减 `cache_write_tokens` → cache_write 会被同时按全价输入和 1.25× 写价**双计**（当前因 F1 恒 0 未暴露） | 【实证】critical-with-F1 | P0 |
| F8 | 团队/消息级 `biz.TokenUsage`（plan_step.go:162）只有 prompt/completion/total 三字段，cached/reasoning 无法到前端 | 【实证】medium | P2 |

---

## 二、现状链路（实证）

```
LLM 流 (usage: prompt/completion/cached[_read/_creation])
  ├─ trpc-agent-go openai.go ── Usage.PromptTokensDetails.CachedTokens（OpenAI/DeepSeek 语义：cached ⊂ prompt）
  ├─ trpc-agent-go anthropic.go ─ CachedTokens=cache_read + CacheReadTokens + CacheCreationTokens
  │                                （Anthropic 语义：input_tokens 不含 read/write，三分量独立） [实证 anthropic.go:617-627]
  ├─ 直连辅助调用 llmcompat.CallOpenAICompatChat ── 只解析 prompt/completion，cached 丢 [实证 llmcompat.go:162]
  └─ 传输层 usageTapTransport ── 仅仿 OpenAI 路径：prompt_cache_hit_tokens → prompt_tokens_details.cached_tokens
                                   [实证 usage_tap_transport.go]，llmcompat 路径不吃此 tap

→ internal/agent/stream_consumer.go:422,452 仅读 PromptTokens / CachedTokens（read/write 丢弃）
→ TurnUsageParams{PromptTok, CompletionTok, CachedTok} → chat_turn_metrics → biz.RecordTurnUsage
→ model_token_usage_events（cached_input_tokens ✓ / cache_write_tokens 恒 0 / reasoning_tokens 恒 0）
→ 聚合：CacheHitRatioStats P50/加权（usage_cache_hit.go；billable 排除 team_turn，含 aux_*）
→ 计费：ApplyTokenUsageCosts（usage.go:1227-1262）input_cost=(input−cached_read)×in_price + write×write_price + …
```

## 三、外部 harness 调研结论【文档】

（本环境外网受限，以下基于官方文档内容与 trpc-agent-go `examples/promptcache`、 `agent/codex/transcript.go` 代码证据交叉验证）

1. **DeepSeek 上下文缓存**：服务端自动开启、磁盘 KV 缓存；命中价 ≈ 未命中价 1/10；按**精确前缀**分块命中——稳定前缀越长越省，前缀任意字节变动即击穿其后全部缓存。响应报 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`（**hit + miss = prompt_tokens**，cached ⊂ prompt）。官方建议：系统/工具/长期记忆放前缀、动态信息放尾部、历史仅追加不重写。
2. **OpenAI prompt caching**：自动生效（≥1024 tokens，128 token 块粒度，命中折半）；`prompt_cache_key`（替代旧 `user` 字段）将会话路由到同一缓存分片、显著提升长会话命中率；报 `usage.prompt_tokens_details.cached_tokens`。
3. **Anthropic prompt caching**：显式 `cache_control: {type: "ephemeral"}` 断点（≤4 个/请求），写 1.25×、读 0.1×，TTL 5m（可 1h）；`input_tokens` 不含 `cache_read_input_tokens`/`cache_creation_input_tokens`。trpc-agent-go 框架已在 `anthropic.go:344-530` 实现 system/tools/messages 三断点，本仓 provider 装配已全套接线（`trpc_llm.go:456-466`），只差**开启**。
4. **Codex harness（openai/codex）**：指令/环境上下文/历史确定性排序 + **thread 级 prompt_cache_key** + /compact 压缩只重写尾部（压缩轮接受击穿、随后回温）；`turn.completed` 上报 `cached_input_tokens` ⊂ `input_tokens`（本仓 `transcript.go:88-93,731-741` 已正确解析）。

**对项目的启示**：前缀稳定纪律（dynamic cue 尾部、`prefix_contract_test.go` 字节级回归）我们已经做了，且方向与 DeepSeek/Codex 完全一致；未做的是「显式缓存开关」「会话亲和 key」「口径一致的命中率标签」。

## 四、标签不合逻辑清单（用户反馈核实）

| 位置 | 现状 | 问题 | 修法 |
|---|---|---|---|
| 聊天头 composer `in X · out Y · Σ Z`（composerUsageMetrics.ts:26-49） | in=会话累计**账单**输入（含折价缓存读），ctx=当前轮上下文占用 | 两个口径并排无说明，用户看到 `in 120k · ctx 8k` 误以为异常 | 文案/悬停注明口径 |
| TeamRunsDialog `缓存命中率 xx%`（TeamRunsDialog.vue:68） | 加权账单口径 sum(cached)/sum(prompt) | 与 Overview「P50 命中率」同名不同义 | 标签改为「缓存命中占比（账单口径）」 |
| UsageEventsPage「来源」列（usageTableUi.ts:18） | 裸枚举 `chat_turn/aux_intent/…` | 非技术用户无法解读；aux 与 chat 混淆观感 | i18n 映射表 |
| `usage.go:1048` 空 kind 兜底 `"chat"` | 未知调用混入聊天口径 | 与 KindChatTurn 语义冲突 | 兜底改 `"unknown"` |
| 消息/团队 detail 无 cached/reasoning 展示 | biz.TokenUsage 三字段截断 | 「输入 X」看不到缓存折价部分 | P2 扩展契约 |

## 五、优化方案与实施清单

### P0 — 口径正确性（本次实施）

1. **llmcompat 解析 cached**：`CallOpenAICompatChat(Stream)` 响应解析补 `prompt_cache_hit_tokens`（→cached）与 `prompt_tokens_details.cached_tokens`；AuxLLMUsageInput 已有 CachedTok 字段，调用点透传。【internal/agent/llmcompat/llmcompat.go】
2. **Anthropic 三分量归一化落账**：stream_consumer/turn_helpers 提取层：
   - `input := PromptTokens + CacheReadTokens + CacheCreationTokens`（Anthropic/Bedrock 显式缓存语义）
   - `cached := CacheReadTokens`（=OpenAI 的 CachedTokens）、`cacheWrite := CacheCreationTokens`
   - 贯通 TurnUsageParams/TurnUsageInput/RecordTurnUsage → `cache_write_tokens` 列
   - 【实证】CacheRead/CacheCreation 仅 anthropic/bedrock 设置，OpenAI/DeepSeek 语义不受影响（read=0 → input 不变）
3. **计费公式修正**：`input_cost=(input − read − write)×in_price`（clamp ≥0），write 单独按 write 价。【usage.go applyTokenUsageCosts】
4. **标签口径**：usage_kind 空兜底→"unknown"；UsageEventsPage 来源列 i18n；TeamRunsDialog 标签注口径；composer `in` 悬停注明。
5. **回归测试**：usage 计费单测（含 write 双计回归）、llmcompat 解析单测、normalize 单测。

### P1 — 命中率实质提升（本次实施）

6. **启用 Anthropic cache_control**：catalog resolve 层对 `provider_type=anthropic` 且未显式配置 cache 标志位的行默认 `system_prompt+tools+messages=true`（保留配置显式覆盖/关闭能力）。ADR 记录成本前提（写 1.25×、短 prompt 不写入由 provider 侧最小阈值保护）。【internal/provider/catalog.go】【docs ADR】
7. **OpenAI/DeepSeek `prompt_cache_key` 会话亲和**：chat turn RunOptions 注入 `WithModelRequestExtraFields({"prompt_cache_key": session_id})`；仅 OpenAI 系 variant 生效（框架 extraFields 透传实测于 openai_test.go:6181-6225）。DeepSeek 同字段无害透传（官方无此参数，兼容代理可吸收）——**审慎起见仅对 variant=openai 注入**。
   注：team runner 已有 `WithModelRequestExtraFields` 用法先例（team_eval_profile.go）。

### P2 — 展示层契约（可随本批，工作量为主）

8. `biz.TokenUsage`（plan_step）补 `CachedTokens/CacheWriteTokens/ReasoningTokens` → ws wire → TS 类型 → TeamRunsDialog/Chat 明细展示。
9. UsageEventsPage 增加「缓存读/写」列（列宽允许时）或在明细行展开。

## 六、风险与回退

- **R1 计费口径变更**：P0-2/P0-3 改变 Anthropic 行 input_tokens 存储语义（旧行继续按含 cached 解读）。Anthropic cache 此前恒关，历史行 cached=0/write=0，**语义等价无迁移**；DeepSeek/OpenAI 行为不变。
- **R2 cache_write 成本上行**：开启 cache_control 后写价 1.25×；仅 >1024(模型最小） tokens 的前缀才写入，监控 `cache_write_tokens`/`cache_write_cost` 列（已存在），告警：`llm.cache_hit_ratio_low` 同机制可后补 `cache_write 占比` 观察。
- **R3 prompt_cache_key 透传副作用**：非 OpenAI endpoint 收到未知字段通常忽略；仅对 variant=openai 开启，风险最小化。
- **回退**：所有开关均配置可关（catalog cache flags / 无强制硬编码），费式修正为纯数**学修正**不回退语义。

## 七、验证计划

1. `go build ./...` 全绿（受影响包 + cmd/admin）。
2. 定向单测：`internal/biz/usage`（计费公式）、`internal/agent/llmcompat`、`internal/agent`（usage 提取）、`internal/provider`（options 默认）、`internal/service`（record 透传）。
3. 前端 `pnpm --dir web test`（usageTableUi/i18n 相关 spec）+ 类型检查。
4. 文档同步（DOC-SYNC）：29-token.design.md §4.5 公式、§十三 落地章节、65 交叉参考 §1.3 usage 行、报告即本文。

## 八、实施状态收尾（2026-10-20 当日闭环）

> 决策记录：[`docs/development/86-adr-usage-normalization-cache-defaults.md`](../development/86-adr-usage-normalization-cache-defaults.md)；设计同步：29-token.design.md §4.5/§4.6/§5.2/§十三；进度：29-token.development.md §19。

| 计划项 | 状态 | 交付与偏差说明 |
| --- | --- | --- |
| P0-1 llmcompat 解析 cached | ✅ | `llmcompat.go` 三处抽取改走 `NormalizeUsageTokens`（DeepSeek 命中值已由 `usage_tap_transport` 在传输层归一为 `cached_tokens`，llmcompat 不再重复解析原始字段——与计划等价且少一层） |
| P0-2 Anthropic 三分量归一化落账 | ✅ | `internal/provider.NormalizeUsageTokens`（新增唯一收敛点）；主链（`stream_consumer` 两处 + `accumulateStreamUsage` 8 参）、team 链（`recordMemberUsage` 14 参 + anchor/HITL/finalize）、辅助链（llmcompat 三处 + subagent/evolution/memory_extract/title 直读点）全部改走；`cache_write_tokens`/`reasoning_tokens` 全链落库 |
| P0-3 计费公式修正（双计） | ✅ | `ApplyTokenUsageCosts`：`writeBillable = clamp(write, 0, input−cachedBillable)`，`InputCost = cost(input−cached−write)`；clamp 保护历史脏数据；`QuoteTokenUsageCostMicroUSD` 加 cacheWriteTok 形参 |
| P0-4 标签口径 | ✅ | 兜底改 `unknown`（`usage.KindUnknown` + 单点归一化漏斗；ent 默认值保留不可达）；chat_ingress 显式 `chat_turn`；来源列 i18n（`usageKinds.*`，未命中回退原始值，遗留 `chat` 与 `chat_turn` 同标签「对话」）；TeamRunsDialog 改「缓存命中占比（账单口径）」；composer chips 结构化 `{text,tip}`，tooltip 区分 `ctx` 窗口快照 vs Σ 账单累计 |
| P0-5 回归测试 | ✅ | provider 归一化 3 例、catalog 缓存默认值 4 例、`IsGenuineOpenAIProvider` 5 例、`accumulateStreamUsage` cacheWrite/reasoning 回归（同轮重发 keep-max、跨轮累加）、biz/usage F7 clamp 与 KindUnknown 断言、team/subagent 签名迁移（13+10+4 处）全绿 |
| P1-6 Anthropic cache 默认开启 | ✅ | `catalog.go`：anthropic 行 `cache_system_prompt/tools/messages` 默认 true，显式 false 逐键优先；非 anthropic 行不受影响；成本前提与拒绝方案记入 ADR-86 决策 3 |
| P1-7 prompt_cache_key 会话亲和 | ✅ | `buildTurnRunOptions` 仅对 `IsGenuineOpenAIProvider`（catalog `provider_type=="openai"` 真官方行）注入 `prompt_cache_key=session_id`（框架 `WithJSONSet` 透传，未破红线 #27）；catalog 失败静默降级；**偏差**：DeepSeek 按原计划「审慎起见」未注入（计划文本已标注该保守项） |
| P2-8 biz.TokenUsage wire 穿透 | ⏳ 暂缓 | proto 再生成与并行会话 knowledge pb 脏改动冲突、工具链成本高；`cache_write_tokens` 已落库/计费，仅 proto/WS 下发面缺失；proto 下一空闲号 54，计入 29-token.development.md §2.3/§19.4 遗留 |
| P2-9 明细页缓存列 | ✅ 部分 | 已加「缓存命中」「推理」两列（proto 字段 23/24 与 TS parser 现成，纯前端接线）；「缓存写」列依赖 P2-8 的 proto 字段，同批暂缓 |

**验证结果**：`go vet` 受影响包全绿；provider/biz-usage/agent/team/service/tools-subagent/skill-evolution/compress 测试全绿（`internal/biz` 仅余环境依赖失败 `TestMigrateLegacyEmbeddedGraphs_PG`，需本地 PG，与本批无关）；前端 `vue-tsc` 本批文件零新增错误（存量错误比对 HEAD 确认非本批引入）；usage 特性 vitest 21 例全绿。运行时验证（真实对话观察 `cache_read/cache_write` 非零、`llm.cache_hit_ratio_low` 告警水位）待下次重启后端后进生产观察。
