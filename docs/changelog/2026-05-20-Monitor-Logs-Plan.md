# Monitor Logs 全链路实施计划

> 日期: 2026-05-20
> 目标: 在系统执行的每个关键环节，输出结构化日志（进入什么、怎么执行的、执行有没有成功），通过 FlowLogger → Event Bus → WebSocket 实时推送到前端

---

## 一、系统全链路执行环节总览

```
用户请求 → HTTP/gRPC入口 → 中间件鉴权 → ChatService → Agent构建 → Plugin加载
    → IntentPass意图识别 → LLM模型调用 → 事件流消费 → Tool调用 → Plugin Hook回调
    → 事件总线分发 → WebSocket推送 → 消息持久化 → 响应返回
```

---

## 二、各层埋点详细计划

### Layer 1: 入口层 (HTTP/gRPC → Middleware)

| # | step 名称 | 输入 | 执行过程 | 成功/失败 | 优先级 | 当前状态 |
|---|-----------|------|----------|-----------|--------|----------|
| 1.1 | `http.request_received` | method, path, session_id | Kratos HTTP路由匹配 | - | P2 | ❌ 缺失 |
| 1.2 | `grpc.request_received` | method, session_id | Kratos gRPC路由匹配 | - | P2 | ❌ 缺失 |
| 1.3 | `ws.connect` | session_id, user_id, channels | WS握手+鉴权+订阅 | `ws.auth_ok` / `ws.auth_failed` | P2 | ⚠️ 有slog |
| 1.4 | `middleware.auth` | token, user_id | JWT验证 | `auth_ok` / `auth_failed` | P2 | ❌ 缺失 |
| 1.5 | `middleware.rate_limit` | session_id, user_id | 限流检查 | `rate_limit_ok` / `rate_limit_blocked` | P2 | ❌ 缺失 |

### Layer 2: Service 层 (Chat Turn 主链路)

| # | step 名称 | 输入 | 执行过程 | 成功/失败 | 优先级 | 当前状态 |
|---|-----------|------|----------|-----------|--------|----------|
| 2.1 | `chat.turn_execute` | session_id, run_id, agent_key | 创建FlowLogger+超时上下文 | LogStart | P0 | ✅ 已有 |
| 2.2 | `chat.agent_build` | agent_id, provider, model | BuildTRPCLLMAgentCached | LogDone / LogError | P0 | ✅ 已有 |
| 2.3 | `chat.plugins_load` | plugin_count | RunnerPluginsForAgent | LogDone | P0 | ✅ 已有 |
| 2.4 | `chat.user_msg_persist` | session_id, content_len | AppendChatMessage | LogDone | P0 | ✅ 已有 |
| 2.5 | `chat.intent_pass` | provider, model, content_len | intent.Run | LogDone / LogSkip | P0 | ❌ 缺失 |
| 2.6 | `chat.llm_call` | run_id, provider, model | RunTRPCUserTurnMsg | LogStart / LogDone / LogError | P0 | ✅ 已有 |
| 2.7 | `chat.first_byte_timeout` | timeout | 30s无首字节 | LogError | P0 | ✅ 已有 |
| 2.8 | `chat.stream_consume` | reply_len, has_error, prompt_tok, completion_tok | ConsumeEventStreamWithFirstByte | LogDone | P0 | ✅ 已有 |
| 2.9 | `chat.turn_timeout` | timeout | 5min总超时 | LogError | P0 | ✅ 已有 |
| 2.10 | `chat.empty_reply` | has_error, last_error | Agent无输出 | LogError | P0 | ✅ 已有 |
| 2.11 | `chat.assistant_msg_persist` | reply_len | AppendChatMessage | LogDone | P0 | ✅ 已有 |
| 2.12 | `chat.turn_execute` | run_id, reply_len, prompt_tok, completion_tok | Turn完成 | LogDone | P0 | ✅ 已有 |
| 2.13 | `chat.runner_create` | agent_key, plugin_count | NewTurnRunner | LogDone / LogError | P0 | ❌ 缺失 |
| 2.14 | `chat.pending_dequeue` | session_id | processPendingQueue | LogDone / LogError | P1 | ❌ 缺失 |

### Layer 3: Agent 层 (构建 + Runner + Tool)

| # | step 名称 | 输入 | 执行过程 | 成功/失败 | 优先级 | 当前状态 |
|---|-----------|------|----------|-----------|--------|----------|
| 3.1 | `agent.cache_hit` | agent_key, cache_key | BuildTRPCLLMAgentCached | hit / miss | P0 | ❌ 缺失 |
| 3.2 | `agent.db_resolve` | agent_key | resolveBizAgentByKey | ok / not_found | P0 | ❌ 缺失 |
| 3.3 | `agent.model_select` | provider, model, selectors | ChainedModelSelector | selected / fallback | P1 | ❌ 缺失 |
| 3.4 | `agent.tool_build` | agent_id, tool_count | buildToolsetsForAgent | ok / error | P1 | ❌ 缺失 |
| 3.5 | `agent.skill_build` | agent_id | buildSkillDeps | ok / error | P1 | ❌ 缺失 |
| 3.6 | `agent.memory_inject` | memory_enabled, has_service | 检查+注入 | ok / warning | P1 | ❌ 缺失 |
| 3.7 | `agent.callback_chain` | agent_id, hook_count | buildCallbackChainOptions | ok | P1 | ❌ 缺失 |
| 3.8 | `agent.runner_run` | session_id, user_id | RunTRPCUserTurnMsg | ok / error | P0 | ❌ 缺失 |

### Layer 4: Plugin 层 (Hook 回调链 + 插件执行)

| # | step 名称 | 输入 | 执行过程 | 成功/失败 | 优先级 | 当前状态 |
|---|-----------|------|----------|-----------|--------|----------|
| 4.1 | `hook.before_agent` | hook_key, agent_id | executeHookAction | ok / blocked / error | P1 | ⚠️ 有metrics |
| 4.2 | `hook.after_agent` | hook_key, agent_id | executeHookAction | ok / error | P1 | ⚠️ 有metrics |
| 4.3 | `hook.before_model` | hook_key, agent_id | executeHookAction+ModifyPatch | ok / blocked | P1 | ⚠️ 有metrics |
| 4.4 | `hook.after_model` | hook_key, agent_id | executeHookAction | ok / error | P1 | ⚠️ 有metrics |
| 4.5 | `hook.before_tool` | hook_key, tool_name | executeHookAction+条件匹配 | ok / blocked | P1 | ⚠️ 有metrics |
| 4.6 | `hook.after_tool` | hook_key, tool_name | executeHookAction | ok / error | P1 | ⚠️ 有metrics |
| 4.7 | `hook.on_event` | hook_key, event_type | dispatchHookOnEvent | ok / error | P1 | ⚠️ 有metrics |
| 4.8 | `plugin.audit_log` | agent_id, step | AuditLogPlugin回调 | ok / error | P1 | ❌ 缺失 |
| 4.9 | `plugin.skill_tracker` | skill_id | SkillUsageTrackerPlugin | ok / error | P1 | ❌ 缺失 |
| 4.10 | `plugin.retry_reflect` | tool_name, retry_count | RetryAndReflectPlugin | ok / error | P1 | ❌ 缺失 |
| 4.11 | `plugin.cost_guard` | budget, used | CostGuardPlugin | ok / blocked | P1 | ❌ 缺失 |
| 4.12 | `plugin.model_router` | from_model, to_model | ModelRouterPlugin | ok / error | P1 | ❌ 缺失 |
| 4.13 | `plugin.permission_guard` | tool_name | PermissionGuardPlugin | ok / blocked | P1 | ❌ 缺失 |
| 4.14 | `plugin.output_policy` | content_len | OutputPolicyPlugin | ok / blocked | P1 | ❌ 缺失 |
| 4.15 | `plugin.confirmation_guard` | tool_name | ConfirmationGuardPlugin | ok / blocked | P1 | ❌ 缺失 |
| 4.16 | `plugin.sensitive_mask` | field_count | SensitiveDataMaskPlugin | ok / error | P1 | ❌ 缺失 |

### Layer 5: Event 层 (Bus + WebSocket)

| # | step 名称 | 输入 | 执行过程 | 成功/失败 | 优先级 | 当前状态 |
|---|-----------|------|----------|-----------|--------|----------|
| 5.1 | `bus.publish` | envelope_type, channel | Publish到订阅者 | ok / dropped | P1 | ⚠️ 有metrics |
| 5.2 | `bus.subscribe` | session_id, priority, buffer_size | 注册订阅者 | ok | P2 | ❌ 缺失 |
| 5.3 | `bus.drop` | envelope_type, policy | 缓冲区满丢弃 | drop_oldest/drop_newest | P1 | ⚠️ 有metrics |
| 5.4 | `event.project` | event_type, author | trpc事件→Envelope | ok | P2 | ❌ 缺失 |
| 5.5 | `ws.event_pump` | session_id, envelope | eventPump→send channel | ok / buffer_full | P2 | ❌ 缺失 |
| 5.6 | `ws.replay` | session_id, count | replayEvents | ok | P2 | ❌ 缺失 |
| 5.7 | `ws.upstream` | type(ping/subscribe/cancel/send) | handleUpstream | ok / error | P2 | ❌ 缺失 |

### Layer 6: 基础设施层 (DB + LLM Provider + Memory + Session)

| # | step 名称 | 输入 | 执行过程 | 成功/失败 | 优先级 | 当前状态 |
|---|-----------|------|----------|-----------|--------|----------|
| 6.1 | `provider.catalog_lookup` | provider, model | GetByProviderAndModel | ok / error | P1 | ❌ 缺失 |
| 6.2 | `provider.preflight_probe` | base_url | HTTP HEAD探测 | ok / unreachable | P1 | ❌ 缺失 |
| 6.3 | `provider.model_build` | provider_type, model_api | trpcprovider.Model | ok / error | P1 | ❌ 缺失 |
| 6.4 | `provider.ha_failover` | primary_error | failover切换 | switched / all_failed | P2 | ❌ 缺失 |
| 6.5 | `session.create` | session_id, agent_id | Sessions.Create | ok / error | P2 | ❌ 缺失 |
| 6.6 | `session.append_message` | session_id, role | AppendChatMessage | ok / error | P2 | ❌ 缺失 |
| 6.7 | `session.create_turn` | session_id, tokens | CreateTurn | ok / error | P2 | ❌ 缺失 |
| 6.8 | `memory.l0_assembly` | session_id, agent_id | 上下文窗口组装 | ok / truncated | P2 | ❌ 缺失 |
| 6.9 | `memory.entity_crud` | entity_type, operation | 实体增删改查 | ok / error | P2 | ❌ 缺失 |
| 6.10 | `channel.webhook_send` | channel_id, platform | 企微/钉钉/飞书推送 | ok / error | P2 | ❌ 缺失 |

---

## 三、覆盖率统计

| 层 | 总埋点数 | 已有 | 缺失 | 覆盖率 |
|----|---------|------|------|--------|
| 入口层 | 5 | 0 | 5 | 0% |
| Service层 | 14 | 11 | 3 | 79% |
| Agent层 | 8 | 0 | 8 | 0% |
| Plugin层 | 16 | 0 | 16 | 0% |
| Event层 | 7 | 0 | 7 | 0% |
| 基础设施层 | 10 | 0 | 10 | 0% |
| **合计** | **60** | **11** | **49** | **18%** |

---

## 四、实施阶段计划

### Phase 1 (P0): 核心链路补全 — Service + Agent 层

目标: Chat Turn 主链路 + Agent 构建链路达到 90%+ 覆盖率

| # | 任务 | 涉及文件 | 改动说明 |
|---|------|----------|----------|
| 1.1 | Intent Pass 埋点 | `service/trpc_turn.go` | 在 `intent.Run` 前后添加 `flow.LogStart/LogDone/LogSkip` |
| 1.2 | Pending 队列埋点 | `service/trpc_turn.go` | `processPendingQueue` 添加 `flow.LogStart/LogDone/LogError` |
| 1.3 | Agent 缓存命中/未命中 + DB解析 | `agent/cache.go`, `agent/factory.go` | 添加 slog 结构化日志（无 FlowLogger 上下文，用 slog） |
| 1.4 | Runner 创建埋点 | `service/trpc_turn.go` | `NewTurnRunner` 添加 `flow.LogStart/LogDone/LogError` |
| 1.5 | Tool/Skill 构建日志 | `agent/trpc_build.go` | `buildToolsetsForAgent`/`buildSkillDeps` 添加 slog |

### Phase 2 (P1): Plugin 层 + Event 层

目标: Plugin Hook 执行链路和 Event Bus 分发可观测

| # | 任务 | 涉及文件 | 改动说明 |
|---|------|----------|----------|
| 2.1 | Hook 执行 FlowLogger | `plugin/trpc/hook_callbacks.go` | `executeHookAction` 中通过 PluginSafeLogger 发布 monitor 事件 |
| 2.2 | 内置插件执行日志 | `plugin/trpc/audit.go`, `skill_tracker.go`, `retry_reflect.go` 等 | 每个插件回调添加 PluginSafeLogger monitor 事件 |
| 2.3 | Bus Publish/Drop 日志 | `event/bus.go` | `deliverToSubscriber` drop 时发布 slog（防循环，不走 FlowLogger） |
| 2.4 | EventProjector + WS 推送日志 | `agent/event_projector.go`, `server/ws.go` | 关键投影和推送节点添加 slog |

### Phase 3 (P2): 基础设施层 + 入口层

目标: DB、LLM Provider、Memory、Session 操作可追踪

| # | 任务 | 涉及文件 | 改动说明 |
|---|------|----------|----------|
| 3.1 | LLM Provider 调用链路 | `provider/trpc_llm.go` | Catalog查询、Preflight探测、Model构造添加 slog |
| 3.2 | Session CRUD 日志 | `service/session.go` | 关键操作添加 slog |
| 3.3 | Memory 操作日志 | `memory/trpc/sqlite_adapter.go` | L0/L1 操作添加 slog |
| 3.4 | HTTP/gRPC 入口中间件 | `server/http.go`, `server/grpc.go` | 请求接收+鉴权添加 slog |
| 3.5 | Channel Webhook 日志 | `channel/*/webhook.go` | 推送结果添加 slog |

---

## 五、技术约束

1. **防循环死锁**: FlowLogger 发布到 Bus → Bus 分发给 WS → 不能再触发 FlowLogger。FlowLogger 事件标记 `channel=monitor`，Bus 内部对 `monitor` channel 事件不再触发 FlowLogger
2. **PluginSafeLogger**: Plugin 回调内必须使用 `PluginSafeLogger`（写 stderr + 异步 Publish），**禁止 slog**（SlogBridge 已删除；业务日志用 FlowLog）
3. **性能开销**: FlowLogger 每步都有 `os.Stderr` 写入 + Bus Publish，高频场景（如 text_delta）需跳过，只在关键节点（start/done/error）埋点
4. **safego.Go**: 所有异步 Publish 必须使用 `safego.Go`，禁止裸 `go func()`
5. **EnvelopeTypeLog**: 所有 FlowLogger 日志统一使用 `EnvelopeTypeLog` + `channel=monitor`，前端通过 WS `log_enabled=1` 参数控制是否接收
6. **Buffer.Append**: FlowLogger 的 `log()` 方法已调用 `buffer.Append(env)`，支持 WS 断线重连后的历史回放
7. **Agent 层系统域打点**: Agent 构建（cache/factory/trpc_build）使用 `CtxFlowLog*` / `system.agent.*` step_id，经 `SysLog` 或上下文 Emitter 进入 `flow_log`（2026-05-20 已迁移）
8. **Provider 层同理**: LLM Provider 调用链路无 FlowLogger，使用 slog 结构化日志

---

## 六、日志输出格式规范

### FlowLogger 格式（Service 层，有 session 上下文）
```
[flow] chat.intent_pass.start: 意图识别开始 provider=openai model=gpt-4o
[flow] chat.intent_pass.done: 意图识别完成 (120ms) intent_kind=question refined_goal_len=45
[flow] chat.intent_pass.skip: 意图识别已禁用
[flow] chat.intent_pass.error: 意图识别失败 (50ms) error=llm_timeout
```

### slog 格式（Agent/Plugin/Provider 层，无 session 上下文）
```
level=INFO msg="agent.cache_hit" agent_key=xxx cache_key=sha256:...
level=WARN msg="agent.memory_inject" agent_id=xxx memory_enabled=true has_service=false
level=ERROR msg="provider.preflight_probe_failed" url=https://api.openai.com error=connection_refused
```

### Envelope 元数据规范
```json
{
  "type": "log",
  "channel": "monitor",
  "session_id": "sess-xxx",
  "metadata": {
    "flow_step": "chat.intent_pass",
    "flow_phase": "done",
    "agent_key": "my-agent",
    "duration_ms": 120,
    "provider": "openai",
    "model": "gpt-4o"
  }
}
```
