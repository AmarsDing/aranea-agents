# ADR-C: 模型级联路由策略与总成本核算口径（P2-1 / P2-2）

## 状态：已接受（2026-08-14，随 Batch-3 落地）

## 背景

Team 编排中所有成员（leader/planner/executor）默认共用同一高档模型。调研（Ensemble QSP 实证、阿里 HiClaw 网关、AgentScope 路由）表明：

1. 规划/聚合类调用（synthesizer、意图锚点）对模型能力敏感，需要高档模型；
2. 成员执行类调用（tool-use worker）对能力要求低，成本档模型即可胜任；
3. 但低成本模型的输入 token 可放大 5.5 倍（Ensemble QSP 实证），仅按单价核算会低估真实成本；
4. 旁路 LLM 调用（PromptRefiner、意图识别等经 `DynamicLLMCaller` 的路径）没有 HA 包装，单次 4xx/5xx/超时即整轮失败。

## 决策

### P2-1 模型级联路由（Leader/Worker 分档）

1. **配置入口**：`team.Definition.model_cascade{member_provider, member_model}`。`member_model` 为空 = auto（目录内最便宜的 ToolCall 可用模型）；显式 `member_model` 不配 `member_provider` 视为配置无效，保持 base 并 Warn。
2. **分档边界**：leader 档 = synthesizer + 意图锚点（按 agent key 匹配 `Invocation.AgentName`），保持构建期配置的高档 base 模型；其余成员经 run 级 `ModelSelector`（`CascadeModelSelector`）路由到成本档。
3. **no-op 语义**：级联目标 == 成员当前 base 时不替换（auto 模式不排除 base——若 base 已是最便宜档，排除会把成员升级到次便宜模型，违背降本语义）。
4. **优先级**：run 级 selector 优先于成员构建级 selector——级联是团队显式管理策略，覆盖成员自身的模型路由插件。
5. **总成本核算口径**：每次级联路由决策发射 FlowLog `team.model_cascade.route`（字段 `agent_key`/`provider`/`target_model`/`base_model`），run 维度可聚合分档 token 用量；配合既有 usage 记账按总 token 成本核算，而非仅单价。

### P2-2 模型 fallback 降级（仅一次）

1. `DynamicLLMCaller.Call` 主模型调用失败（任一侧信道错误）时，在目录内选**同 provider、enabled、非主模型**的首个候选降级重试**一次**；候选自身失败不再递归（避免隐性级联放大延迟与成本）。
2. 调用方 ctx 已取消/超时不降级（用户已放弃，再调一次是浪费）。
3. 降级发射 FlowLog K3 `chat.llm.fallback`（warn，字段 `provider`/`primary_model`/`fallback_model`）。

## 后果

**正面**：
- 成员执行调用成本下降（成本档单价 + 目录 auto 选择），leader 关键路径质量不变；
- 旁路调用单点故障消除，整轮成功率提升；
- 分档路由与降级均有 FlowLog 审计轨迹，可按 run 聚合真实总成本，为后续上限熔断提供数据。

**负面 / 风险**：
- token 放大风险仍在（计划 §风险 2）：本 ADR 只提供**计量口径**，上限熔断待后续批次基于用量数据定阈值；
- run 级 selector 覆盖成员构建级 selector——成员自定义模型路由插件在团队 run 内失效（有意为之，文档已注明）；
- fallback 候选选择是目录序首个，未按能力/成本排序（旁路调用对质量不敏感，简单策略足够）。

## 替代方案

1. **构建期分档**（每成员静态配置不同模型）：被否决——成员复用于多团队时需重复配置，且无法按团队策略统一切换；run 级 selector 单点控制更优。
2. **auto 模式排除 base**：被否决——base 已是最便宜档时排除会导致成员升级到次便宜模型，违背降本语义（测试 `TestCascadeModelSelector_AutoExcludesInvocationBase` 锁定该行为）。
3. **fallback 递归降级 / 跨 provider 降级**：被否决——多次降级隐性放大延迟与成本；跨 provider 的 API 兼容性（tool schema、thinking 字段）不可控，同 provider 内降级是安全边界。
4. **usage 表新增分档字段**：暂缓——FlowLog 聚合已满足当前核算口径，避免 Schema 变更；后续如需 SQL 直查再评估。

## 落地锚点

- `internal/agent/model_selector.go` — `CascadeModelSelector` / `CheapestCapableModel`
- `internal/team/definition.go` — `ModelCascadeDef`；`internal/team/team_cascade.go` — leader key 解析 + run option
- `internal/team/runner_team_trpc.go` — runOpts 接线
- `internal/agent/llm_caller_impl.go` — `Call` fallback / `fallbackCandidate` / `decryptedCatalogConfig`
- FlowLog step：`team.model_cascade.route`、`chat.llm.fallback`（`internal/event/flow_log.go` + `docs/development/52-flow-logger.design.md` §5.1）
- 测试：`model_selector_cascade_test.go`、`team_cascade_test.go`、`llm_caller_fallback_test.go`
