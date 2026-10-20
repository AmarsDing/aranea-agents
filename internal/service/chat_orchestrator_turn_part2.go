package service

// runSingleAgentViaTRPC runs a single agent turn via the trpc-agent-go framework.

// P1-7: Start the Wire-injected run heartbeat emitter so the frontend
// can detect stale runs within 30s. The stop function is invoked in the
// defer below to avoid goroutine leaks. When the emitter is not wired
// (nil), heartbeats are skipped and stale detection degrades gracefully.
//
// The progress closure reports the current turn phase (building/executing/
// persisting) via an atomic.Value so the heartbeat goroutine can read it
// concurrently without locks.

// ── PROACTIVE RECALL (P3-11) ──
// Surface relevant memories based on conversation context before building
// the runner. The hits are stored in ctx so the MemoryInject before-model
// hook can merge them with RecallComposite results. Failures are non-fatal:
// only a warning is logged and the turn continues without proactive hits.
//
// Voice Fast-Path（2026-08-09）：recall 从串行段移入 errgroup 与 BUILD/INTENT
// 三方并行——真机实测串行 recall 0.3-3.3s 且语音轮次 hits 恒为 0，纯开销。
// WithProactiveHits 在 eg.Wait() 后注入 ctx：MemoryInject before-model 钩子在
// 请求时才读取，语义不变；goroutine→Wait 的 happens-before 由 errgroup 保证。

// Session orchestration phase is resolved from persisted teams before the
// Spirit LLM runs. Idle still uses ForcePlanning; Ready/Orchestrating/
// Interrupted keep the current DAG unless the user asked for a new task.

// shouldSkipIntentPass reports whether this turn should skip the extra intent
// LLM: DirectReply patterns (介绍自己 / 不要调用工具 / 请记住 / 现在几点),
// or QuickAssess==simple when the message is not an underspecified task
// (帮我做个应用 still needs the clarification gate).
//
// 第二返回值携带 QuickAssess 等级（未评估时为空串）：C2 方案 D 分级时限用
// 其区分 intent 收口策略——complex 全等至保险丝，moderate 走 rendezvous。
// 第三返回值是 skip 判定的机器可读理由（包B B4 路由语料）：direct_reply /
// underspecified_task / agent_skip_disabled / task_action_signal /
// planner_unavailable / planner_error / not_simple / confident_simple /
// low_confidence_simple。
//
// 包B（session-eval-20260825 B1）两道闸：
//  1. agent 维度闸：ag.Settings.IntentSkipEnabled=false（管理层 agent：3 GM +
//     部门主管 + spirit，SQL 置 0）时禁用 QuickAssess==simple skip——任务型
//     消息被误判 simple 导致 intent pass 跳过、组织路由失效（P-INTENT-SKIP）。
//     DirectReply 是确定性模式匹配（非误判源），不受此闸影响。
//  2. R4 低置信不 skip：QuickAssess score 贴近 0.3 阈值的边界 simple 说明
//     评分器对「简单」不确信——走完整 intent pass（宁重勿轻，RouteLLM
//     非对称代价：任务型误判为简单 ≫ 闲聊误跑 intent pass）。仅高置信
//     simple（score < intentSkipConfidentSimpleMaxScore）允许 skip。
//
// ADR-79-V V2（2026-08-26）第三道闸：任务信号排除。分类器输出只可用于
// 增加义务、不可用于免除义务——凡含任务动作词的轮次，即使评分器误判
// confident simple 也不得 skip（与 QuickAssess 内部 taskRequestWords
// 升级互为纵深：那里是第一道，这里兜底评分器漏判）。

// intentSkipConfidentSimpleMaxScore R4 低置信闸（包B B1）：QuickAssess 判
// simple 的阈值为 score<0.3，但 [0.2,0.3) 属贴线边界——评分器对「简单」
// 不确信，不允许 skip intent pass。0.2 = 阈值 0.3 内收 0.1 置信边距。

// intentKindOf 安全取 artifact 的 IntentKind（nil → ""），包B B4 路由语料用。

// intentRendezvousWindow C2 方案 D：moderate 非欠规格轮次在 BUILD 收口后
// 再等 intent 的窗口。Intent Pass 实测 0.5–2.3s，500ms 会合几乎必然
// raced_out（产物丢弃、2.3s 白烧）。对齐保险丝 2.5s，让中档有机会吃到
// refined goal / 澄清门。超时 cancel intent，按既有无意图路径降级。

// awaitIntentArtifact C2 方案 D 分级收口：fullWait（欠规格/complex）全等至
// intentCtx 自带的 2.5s 保险丝；否则限 rendezvous 窗口。artifact 只经
// channel 传递（容量 1，goroutine 永不阻塞退出），主流程无共享变量竞争。
// outcome 取值：completed / fused（保险丝熔断）/ raced_out（窗口未命中）。

// shouldRunProactiveRecall 判定本 turn 是否执行主动召回（P3-11）。
//
// 语音轮次（input.Voice != nil）跳过：真机实测语音轮次 recall hits 恒为 0
// （语音输入多为短口语句，实体提及稀少），而 recall 含 query embedding +
// 向量检索，耗时 0.3-3.3s；虽然 recall 与 BUILD/Intent Pass 同 errgroup
// 并行，eg.Wait() 以最慢 goroutine 收口，零产出召回会成为关键路径纯开销，
// 直接击穿 ≤2s 停口到首音预算（2026-08-11）。
//
// 文字 DirectReply / 短闲聊同样跳过：中文分词几乎总会抽出「实体」，不能
// 靠 extractMentionedEntities 判断；L3 硬注入仍覆盖工号召回。
// Stability:internal

// runProactiveRecall triggers proactive memory recall at turn start to surface
// relevant memories based on the conversation context (P3-11). The returned
// hits are later merged with RecallComposite results by the MemoryInject
// before-model hook.
//
// Failures are non-fatal: only a warning is logged and nil is returned so the
// turn continues without proactive hits. The method is a no-op when memory is
// disabled, the composite recaller is not wired, or the recaller does not
// implement biz.ProactiveRecaller.

// extractMentionedEntities extracts candidate entity keywords from the user
// message for proactive recall. This is a simple tokenisation (YAGNI: no NLP,
// no stemming) that splits on whitespace and common punctuation (ASCII + CJK),
// filters out very short tokens, and caps the result to avoid excessive queries.
