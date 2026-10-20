# ADR-86：Token 计费口径归一化（Anthropic 三分量）与缓存启用默认值

> 状态：已采纳（2026-10-20） ｜ 决策源：chat token/缓存优化分析（`docs/reports/2026-10-20-analysis-chat-token-cache-optimization.md` §3 F3/F7） ｜ 关联：`internal/provider/usage_normalize.go`、`internal/provider/catalog.go`、`internal/service/chat_orchestrator_turn_phases.go`、29-token 三件套（[`29-token.md`](./29-token.md) / [`29-token.design.md`](./29-token.design.md) §4.5/§十三 / [`29-token.development.md`](./29-token.development.md)）

## 背景

厂商对 prompt 缓存的 usage 回报口径分裂为两族：

- **OpenAI/DeepSeek（二元）**：`prompt_tokens` 已含命中部分；细分为 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`（DeepSeek）或 `prompt_tokens_details.cached_tokens`（OpenAI）。`cached ⊂ prompt`。
- **Anthropic/Bedrock（三分量）**：`input_tokens` **不含**缓存部分，另有 `cache_read_input_tokens` / `cache_creation_input_tokens`。计费输入 = 三分量之和；写时价 1.25×，读时价 0.1×。

修复前系统存在两个确定性亏/错算（分析 §3 F3/F7）：

1. **F7 计费双计**：`ApplyTokenUsageCosts` 公式 `input_cost = (input − cached) × in_price` 在三元口径下把 **write 分量**又按 input 原价计一遍——Anthropic 模型开启缓存后每个未命中 turn 被多收约 25% 输入价。
2. **F3 缓存从未启用**：`cache_system_prompt/tools/messages` 三项默认全 false，Anthropic 长 prompt 模型每轮全价付费（缓存读写差价 ~12.5 倍），且产生不了任何 cache_read/write 观测数据。
3. **F1/F2 口径漏报**：usage 抽取只读 `PromptTokens`/`CompletionTokens`/`CachedTokens` 三个字段，`cache_creation_tokens` 与 `reasoning_tokens`（推理模型输出内细分）在归一化与入库链路中整体丢失。

## 决策

1. **归一化前置到唯一抽取点**：新增 `internal/provider.NormalizeUsageTokens(*trpcmodel.Usage) (billingInput, output, cached, cacheWrite, reasoning)`，把厂商口径差异收敛为统一五元组：
   - `billingInput = PromptTokens + CacheReadTokens + CacheCreationTokens`（OpenAI/DeepSeek 退化为 `PromptTokens`，零成本）；
   - `cached = CachedTokens`（= Anthropic read 分量）；`cacheWrite = CacheCreationTokens`；`reasoning = CompletionTokensDetails.ReasoningTokens`。
   - 不变量：`cached ⊂ billingInput`、`cacheWrite ⊂ billingInput`、`reasoning ⊂ output`。
   主链路（stream_consumer 两处抽取）、team 链（成员行 + anchor 余量 + HITL/finalize）、辅助链（llmcompat 三处 + subagent/evolution/memory_extract/title 直读点）全部改走该函数。放置 `internal/provider`（biz 不得 import trpcmodel；tools/skill 只依赖 biz+provider）。
2. **计费公式修正（F7）**：`ApplyTokenUsageCosts` 引入 `writeBillable = clamp(CacheWriteTokens, 0, InputTokens−cachedBillable)`，`InputCost = cost(InputTokens−cachedBillable−writeBillable)`，`CacheWriteCost` 只按 writeBillable 计。clamp 保护历史脏数据（write 超界时不产生负 input 成本）。`QuoteTokenUsageCostMicroUSD` 同步加 cacheWriteTok 形参（biz port 签名扩展，AS-STA-01 Internal 演进）。
3. **Anthropic 缓存默认开启（F3）**：`catalogConfigToConfig` 对 `provider_type=="anthropic"` 的行，未显式配置时 `cache_system_prompt/cache_tools/cache_messages` 默认 **true**；显式 false 逐键关闭（如 Claude 2.x 端点拒绝 cache_control 的老模型）。非 anthropic 行不受默认值影响（不向 OpenAI 兼容网关泄漏 anthropic 语义）。
4. **OpenAI prompt_cache_key 锚定 session（F5）**：`provider_type=="openai"`（真官方行）的 turn 注入 `agent.WithModelRequestExtraFields({"prompt_cache_key": session_id})`，同 session 多轮路由同一缓存分片、拉长缓存驻留。经框架 openai builder 的 `WithJSONSet` 透传（framework READ-ONLY，红线 #27）。catalog 解析失败 → 静默降级为不带 key（自动缓存不受影响）；非官方行（兼容网关可能 400 拒绝未知字段）不注入。
5. **辅助链近似（记录性说明）**：llmcompat 抽取面只导出 (prompt, completion, cached) 三元组——write 分量留在 prompt 内按 input 价计。当前 cache flags 默认开启仅限 LLM 主链路不经过的字典/意图等短 prompt 辅助路径，write 实际恒为 0，近似无代价；若未来辅助链走 Anthropic 长 prompt，再扩展抽取面（F7 公式已防护双计）。

## 后果

- 正面：Anthropic 模型输入计费与官方账单一致（消除 ~25% 双计）；长系统 prompt/工具声明开启缓存后读价降 90%；OpenAI 行 prompt_cache_key 提升缓存亲和；usage 事件新增 `cache_write_tokens`/`reasoning_tokens` 落库，观测面补齐。
- 代价：Anthropic row 首次写缓存按 1.25× 计费（官方价目，换后续读价 0.1×）；catalog 缓存 flags 语义从「默认关」变「anthropic 默认开」——监控面板的 caps.Cache 声明随之变准。
- 兼容：`TurnUsageInput`/`AuxLLMUsageInput` 仅加字段；`TokenUsageEvent` 列 `cache_write_tokens`/`reasoning_tokens` 已存在（29-token 迁移期建列）；OpenAI/DeepSeek 行为零变化（归一化 no-op）。
- 数据迁移：无。历史行 keep 原值（公式修正只影响新写入成本计算）。

## 替代方案（已否决）

| 方案 | 否决理由 |
| --- | --- |
| 归一化放 biz/usage（而非 internal/provider） | biz 层不得 import trpcmodel（分层红线）；tools/subagent、skill/evolution 只依赖 biz+provider，放 biz 会成环 |
| 各抽取点各自 pick 字段（共享小 helper 在 agent 包） | 主/辅/team 三条链分散复制判断公式，口径易再分裂（本次 F7 正是分散付出的代价） |
| 缓存启用做成全局开关（env/feature flag）默认开 | 行级 `cache_*` 配置项已存在，粒度已够；再叠全局开关增加组合矩阵与排障面 |
| prompt_cache_key 对所有 openai-builder 变体启用 | 兼容网关（one-api/vllm strict 模式）可能拒绝未知 body 字段 400；官方行才承诺容忍/利用该字段 |
| 给 llmcompat 辅助链也穿透 write/reasoning | 当前辅助路径不产生 write（F3 修复前无缓存启用），近似零代价；扩展抽取面属于可待收益，不挤占本次闭环 |
