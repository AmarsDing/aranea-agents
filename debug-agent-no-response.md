# Debug Session: agent-no-response

**Status:** [OPEN]
**Started:** 2026-05-20
**Symptom:** 发送消息给 agent 后无任何回复（HTTP 返回 200 但无内容，或请求挂起）

## Hypotheses

| # | Hypothesis | Status |
|---|-----------|--------|
| H1 | LLM Provider 配置缺失/无效，LLM 调用失败但错误被静默吞掉 | PENDING |
| H2 | Event Bus 为 nil 导致异常流程 | PENDING |
| H3 | runNativeAgentTurn 被 HasActive 检查阻断，消息入队未执行 | PENDING |
| H4 | SQLite 中 Session/Agent 数据不完整 | PENDING |
| H5 | Runner 内部 LLM 客户端创建失败，事件流立即关闭 | PENDING |

## Instrumentation Points

| Point | File | Line | Purpose |
|-------|------|------|---------|
| P1 | internal/service/trpc_turn.go | runSingleAgentViaTRPC entry | Confirm function is reached |
| P2 | internal/service/trpc_turn.go | after ConsumeEventStream | Check result.Reply, result.HasError, result.LastError |
| P3 | internal/service/chat_native.go | runNativeAgentTurn | Check session fetch, agent hydration, HasActive |
| P4 | internal/service/chat_native.go | nativeSendChatMessage | Check final response |

## Log References