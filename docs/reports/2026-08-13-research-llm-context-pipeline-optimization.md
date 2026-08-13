# LLM 请求全链路综合分析与优化方案（含前沿对标调研）

> 调研日期：2026-08-13。范围：指令输入 → 意图识别 → 工具链加载 → 知识库加载 → 记忆加载 → 主 LLM 响应。
> 方法：代码证据（4 路并行调研 + 关键文件精读）× 前沿对标（Hermes / 上下文工程 / 缓存经济学 / Tool RAG / 记忆系统 / RAG 命中率 / 推理成本）。

---

## 1. 现状链路全景（代码证据）

```
用户消息
  │
  ├─ ① 意图识别（额外 1 次 LLM 调用，串行阻塞主链路）
  │     internal/service/chat_orchestrator_turn_phases.go:141 → intent.RunForAgent
  │     · 小模型（catalog 配置，thinking_disabled），带近期历史
  │     · 超时 → skipped_llm 降级；语音链路有 L2 投机 intent 复用
  │
  ├─ ② System Prompt 组装（静态层，前缀稳定化 ✅）
  │     internal/agent/prompt.go:32 BuildSystemPrompt
  │     = role_responsibility + industry_context + agent_description
  │       + prompt files + memory_self_marking（~120 行指令，常驻 ~400-500 token）
  │
  ├─ ③ 动态 cue 注入（8 处 BeforeModel hook，insertAfterLastSystem + 尾部 append）
  │     · StaticRuntimeCapabilityCue / DynamicRuntimeCapabilityCue（runtime_cue_inject.go:39,69）
  │     · 记忆 cue：L1 工作记忆（l1_prompt.go，L1BudgetTokens 过滤）
  │                L4 知识图谱（l4_prompt.go，mention 排序 + 置信度门 + maxPaths 截断）
  │     · 知识 cue：仅 collection 目录摘要（knowledge_inject.go，≤10 库、1500 字符封顶）
  │     · reply reminder / skill guidance / context compression marker
  │     · intent_reorder_inject.go 把 intent system 消息剪切 append 到消息末尾
  │
  ├─ ④ 工具 schema 装配
  │     tool_build_catalog.go + tool_assembly.go：按 profile/allow 静态过滤后【全量注入】
  │     无按 query 相关性预选；MCP 工具挂载进同一集合
  │
  ├─ ⑤ 知识检索（Just-in-Time，非自动注入 ✅）
  │     HybridRetriever：BM25(word_similarity + %>) + pgvector，RRF 融合，
  │     embedding 缺失时 sparse 降级；经 knowledge 工具按需调用
  │
  └─ ⑥ 主 LLM 流式响应
        缓存：依赖 provider 自动前缀缓存（DeepSeek 系 prompt_cache_hit_tokens
        → usage_tap_transport.go 归一化为 cached_tokens），chat_turn_metrics.go:71 记录
```

### 1.1 每轮 LLM 调用清单（文本 chat 路径）

| # | 调用 | 模型 | 串行？ | 备注 |
|---|------|------|--------|------|
| 1 | 意图识别 | 小模型 | ✅ 阻塞主链路 | 产物 = intent artifact，目前主要喂澄清门/prompt 注入 |
| 2 | 主 LLM | 主模型 | — | 携带全量工具 schema + 各层 cue + 历史 |
| 3 | 记忆提取/合并（异步） | 小模型 | 否（sleep-time） | memory_llm_extractor / consolidator |
| 4 | 会话压缩（条件触发） | 小模型 | 否 | compressor.go，版本 CAS 守卫 |

---

## 2. 对标前沿：方向正确的部分

| 业界共识 | 我们的实现 | 判定 |
|---|---|---|
| 前缀稳定化吃满 prompt cache | 三层前缀 + insertAfterLastSystem + intent 尾部搬移 | ✅ |
| Just-in-time 检索（不预注入 chunk） | 知识库走工具按需检索，cue 只注入 1500 字符目录 | ✅ |
| 记忆注入需预算封顶 + 相关性门 | L1 budget tokens、L4 置信度门 + maxPaths + MMR | ✅ 有，但分散 |
| 混合检索 BM25+向量+RRF | HybridRetriever 完全一致 | ✅ |
| Hermes stacked system messages | 三层前缀设计同构 | ✅ |
| 思考模式按场景关闭 | catalog thinking_disabled + 语音 fastpath | ✅ |

**结论一：架构方向没有大错，问题集中在"价值闭环"和"度量缺失"。**

---

## 3. 问题清单（按 ROI 排序）

### P0-1：意图识别是"高成本低复用"资产 ⭐ 最核心问题

- 证据：chat_orchestrator_turn_phases.go:141 每轮一次小模型调用、串行阻塞主链路；产物目前主要服务澄清门与 prompt 注入。
- 对标：业界共识（RouteLLM / Hermes Agent）——一次便宜的意图调用必须驱动全链路装配（工具预选、知识开关、记忆开关、模型路由四合一）。只用于澄清 = 付了钱只用一半。
- 影响：工具/记忆/知识各自为政，主模型每轮背着全量 schema。

### P0-2：工具 schema 全量注入，无相关性预选

- 证据：tool_assembly.go 按 profile/allow 静态过滤后全量注入；无 Tool RAG、无 schema 压缩。
- 对标：Hermes Agent 实测 30+ 工具全量注入 ≈ 14K token/轮，混合检索预选 top-8 ≈ 1.4K（-90%，零额外 LLM 往返）；RAG-MCP 报告工具选择准确率 13.6%→43.1%。
- 附带风险：工具列表随 profile/MCP 上下线变动时字节级击穿前缀缓存。

### P0-3：缓存命中率只记录、不告警、无回归测试

- 证据：cached_tokens 已采集（chat_turn_metrics.go:71、usage_tap_transport.go），但无命中率告警/CI 断言；Anthropic 系无显式 cache_control 断点。
- 对标：Claude Code 将缓存命中率下降列为生产事故级告警；coder/mux 分层断点把命中率 40%→95%、单请求成本 -76%。前缀稳定化做了但无人盯是否真在命中——一次引入变动字节（时间戳/字段序）即成本静默翻倍。

### P1-4：记忆注入预算分散，无单轮总预算；双轨写入冗余

- 证据：L1（L1BudgetTokens）、L4（maxPaths/confidence）、composite recall 各有独立限制，无统一"每轮记忆注入 ≤ X token"总量闸门；prompt.go:83 memory_self_marking 指令 ~400-500 token 常驻每个记忆开启的 agent，与 sleep-time LLM extractor 功能重叠（双轨写入）。
- 对标：Mem0/MIRIX 冠军配置单轮记忆注入硬上限 ≤7K token；MemoryOS"热度×时间衰减"淘汰可直接借鉴。

### P1-5：命中率度量未闭环

- 证据：retrieval_evaluator.go 存在（recall@k），但未接入真实流量反馈；无"注入 token vs 任务成功率"监控——无法感知 context rot（Chroma 实测所有模型随注入变长性能衰减；RULER 显示有效上下文常只有标称的 1/4-1/2）。
- 对标：金标集 50-100 条 + 真实流量小网格复测是业界标配。

### P2-6：检索质量还有一代差

- Contextual Retrieval（离线 chunk 上下文前缀，失败率 -49%，叠加 rerank -67%）、HyPE（索引时预生成假设问题，precision +42pp，零运行时开销）均未采用；rerank 逻辑在库但策略不明。
- 工具结果在历史中累积，无 tool-result 清理/截断机制（工具结果是 agent 场景最大单一预算项，可达 50K+ token）。

---

## 4. 优化方案（三阶段）

### 阶段 0：度量先行（纯工程，零 LLM 成本）

> **落地状态（2026-08-13）**：✅ 已完成——缓存命中率聚合查询 + `llm.cache_hit_ratio_low` 告警 + `chat.context_budget` 台账（含运行时验证：tools_schema 占输入 93%，已量化为阶段 1 基线）。详见 `docs/development/29-token.development.md` §13。

1. **缓存命中率监控 + 告警**：按 session/agent 聚合 cached_tokens/prompt_tokens 比率，跌破阈值告警；CI 加"双请求缓存击穿回归测试"（第二次请求 cached_tokens ≥ 阈值）。
2. **上下文预算台账**：每轮记录静态前缀/工具 schema/记忆各层/知识/历史五类 token 分量日志（复用 L1 已有 token_estimate 基础设施）——后续所有优化的验收基准。

### 阶段 1：意图驱动装配（核心收益）

3. **意图产物四合一升级**：intent pass 一次输出 `{意图, 工具 topK, 是否检索知识库, 是否召回记忆, 难度分}`。
4. **Tool RAG 预选**：复用知识库既有 BM25+pgvector+RRF 基础设施对工具描述建索引；常驻 3-5 个核心工具 + 意图预选 top-8。预期 schema token -80~90%，工具选择准确率同步提升。
5. **级联路由**：难度分 < 阈值 → 小模型直答；不确定 → 主模型兜底。RouteLLM 数据：60-80% 流量可降级，成本 -40~85% 且质量保留 95%。
6. **记忆统一预算**：单轮记忆注入总量硬上限（建议 7K token），L1/L4/composite 共享额度；self-marking 指令精简至 ~150 token；写入侧保留单轨（LLM extractor 为主，fact tags 为补充）。

### 阶段 2：质量与护栏

7. **Contextual Retrieval / HyPE 离线增强**（离线一次性成本，预期检索失败率 -49%）；reranker 按实测 ROI 决定。
8. **工具结果清理**：旧轮工具结果截断/清理；注入总量接近 32K（RULER 有效上下文拐点）触发 compaction。
9. **金标评测闭环**：50-100 条真实流量金标集（含多跳/时序/知识更新），对「记忆策略×检索策略×注入预算」小网格复测后定配置。

### 4.1 收益估算汇总

| 优化项 | Token 影响 | 命中率影响 |
|---|---|---|
| Tool RAG 预选 | schema -80~90%（典型省 10K+/轮） | 工具选择准确率 ×3 |
| 缓存命中率护栏 | 防静默翻倍（命中读价 0.1-0.25×） | — |
| 级联路由 | 整体成本 -40~85% | 质量保留 90-95% |
| Contextual Retrieval | 离线一次性成本 | 检索失败率 -49~-67% |
| 记忆统一预算 | 注入封顶 ≤7K/轮 | 防膨胀衰减 |

---

## 5. 前沿调研摘要（来源与关键数据）

### Hermes（Nous Research）

- Hermes 4 技术报告（arXiv:2508.18255，2025-08）：14B/70B/405B 三档，hybrid reasoning（`<think>` 切换），后训练 500 万样本/190 亿 token。工具契约：工具定义在 system 块 `<tools>` 中，模型输出 `<tool_call>{...}</tool_call>`，工具结果以 tool role + `<tool_response>` 返回；vLLM 提供 `--tool-call-parser hermes`。
- 长度控制微调：30K 处插入 `</think>` 且只对终止 token 计梯度，超长生成率降 ≥98.9%，AIME'25 仅 -3.9% 相对。启示：推理预算是可训练/可工程化的产品变量。
- 支持 stacked system messages（persona + tools + reasoning policy 分层），与我们三层前缀同构。
- 2026 动态：重心转向 Hermes Agent（开源自主 agent，持久记忆、自动技能创建、并行子智能体）。其社区 issue 实测 30+ 工具全量注入 ≈ 14K token/调用，推荐 BM25+embedding+RRF 预选 top-8 ≈ 1.4K token，无额外 LLM 往返。
- 来源：https://arxiv.org/abs/2508.18255 、https://github.com/NousResearch/hermes-agent/issues/13332

### 上下文工程

- Anthropic《Effective context engineering for AI agents》（2025-09）：context 是有限注意力预算，目标是"最小高信号 token 集合"；长程三手段 = compaction / 结构化笔记 / sub-agent 隔离；推荐 just-in-time retrieval（cursor 实测动态加载 MCP 工具 -46.9% token）。
- Context Rot（Chroma，2025-07）：18 个前沿模型全部随输入变长性能下降；NoLiMa：12 个模型中 10 个在 32K 时跌破短上下文成绩 50%；RULER：标称 128K 模型有效上下文常只有 32K-64K。窗口大小不是目标，信噪比才是。
- 两段式装配共识：静态层在前且跨请求稳定（吃满 prefix cache），动态层放最后；高信号内容放开头或结尾，永不埋中间。
- 来源：https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents 、https://research.trychroma.com/context-rot

### 缓存经济学

- 命中条件三家一致：字节级精确前缀匹配；Anthropic 缓存引用顺序固定 tools → system → messages；OpenAI ≥1024 token 起、128 token 增量命中。
- 成本：Anthropic 读 0.1×；OpenAI 读 0.25-0.5×；Gemini implicit caching 折扣 ~75%。
- coder/mux 多层断点策略（system+tools 1h TTL、会话 5m）命中率 40%→95%、单请求成本 -76%；阿里云 KVCache in the Wild：to-B 场景 97% 命中来自 system prompt 共享；Claude Code 将命中率下降列为事故级告警。
- 来源：https://console.anthropic.com/docs/en/build-with-claude/prompt-caching 、https://platform.openai.com/docs/guides/prompt-caching 、https://github.com/coder/mux/pull/561

### Tool RAG

- RAG-MCP（arXiv:2505.03275）：prompt token -50%+，工具选择准确率 13.62%→43.13%。
- ToolRAG（arXiv:2509.20386）：3× 选择准确率 + ~50% token 削减；混合检索 118 条查询 100% 命中、37ms、零 LLM 成本。
- TSCG（arXiv:2605.26165）：schema 压缩 44-50% token；典型工具 schema 300-500 token/个。
- 适用边界：工具 ≤5 个、任务固定时不必做；10+ 工具且任务多变时收益最大。
- 来源：https://arxiv.org/pdf/2505.03275 、https://arxiv.org/pdf/2509.20386 、https://arxiv.org/pdf/2605.26165

### 记忆系统

- 2026 格局：图优先（Zep/Graphiti）、向量+抽取（Mem0）、文件系统优先（Letta Context Repos / Anthropic memory tool）、研究级（MemoryOS/MIRIX）。
- 冠军配置共识：单轮检索注入 ≤7K token；写入侧单遍 LLM 抽取事实而非存原文；检索式记忆明确优于全量注入（MIRIX 存储 -99.9%，单跳准确率差距已很小）。
- MemoryOS（arXiv:2506.06326）：STM→MTM→LPM 三层 + 热度分（访问次数×交互深度×时间衰减）晋升/淘汰，LoCoMo F1 +49.11%。
- 基准自报偏差警告：各家分数依赖自家 pipeline 配置，选型须以自有流量复测为准。
- 来源：https://arxiv.org/pdf/2506.06326v1 、https://arxiv.org/html/2507.07957v1/ 、https://mem0.ai/blog/benchmarked-openai-memory-vs-langmem-vs-memgpt-vs-mem0-for-long-term-memory-here-s-how-they-stacked-up

### RAG 命中率

- Contextual Retrieval（Anthropic）：chunk 前置 LLM 生成上下文，top-20 失败率 5.7%→2.9%（-49%），叠加 rerank -67%。
- HyPE（IEEE Access 2025）：索引时预生成假设问题，question-question 匹配，零运行时开销，precision +42pp。
- HyDE 非对称结论：收益依赖数据规模与任务，低延迟/单会话路径应关闭。
- 混合检索 + RRF 是 2025-2026 所有 SOTA pipeline 的事实标准；reranker 边际收益须实测（EncouRAGe：Hybrid BM25 已是最稳基线）。
- Anthropic 建议：知识库 <20 万 token 时直接全量进 prompt + caching，比 RAG 更简单更准。
- 来源：https://www.anthropic.com/news/contextual-retrieval 、https://arxiv.org/pdf/2511.04696v1

### 推理成本优化

- RouteLLM（ICLR 2025）：95% GPT-4 质量只需 26% 调用量，成本 -75~85%；生产区间 -40~85% 成本、保留 90-95% 质量。
- 三层混合路由：规则拦截高频简单意图 → 轻量模型打难度分 → 大模型兜底；评分器优于分类器（新增模型只调阈值）。
- 级联路由：便宜模型先答、置信度不足再升级；70% 流量过阈时净省 ~60%。
- 意图调用值得的判定标准：产物被多处复用（工具预选/记忆范围/路由/知识开关）则 ROI 极高；只用于路由本身则规则已够。
- 来源：https://github.com/lm-sys/RouteLLM

---

## 6. 核心结论

链路骨架（前缀稳定化、JIT 检索、混合检索、分层记忆）方向正确；最大缺口是 **意图识别产出的复用率** 与 **缓存/预算的可观测性**。阶段 0+1 即可拿到 60% 以上总收益，且全部复用既有基础设施，无新框架引入。
