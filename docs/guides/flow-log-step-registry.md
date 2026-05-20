# FlowLog 步骤注册表

> **真相源**：与 [52-flow-logger.design.md](../需求/52-flow-logger.design.md) §5.1 同步。新增/重命名步骤须更新本表。  
> **迁移**：SlogBridge 已移除，系统域步骤见 [changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md)。  
> **命名**：`{domain}.{subsystem}.{action}`，全小写，点分。

---

## Chat（`domain=chat`）

| step_id | phase 典型 | severity（成功/失败） | title（用户可见） | 说明 |
|---------|------------|----------------------|-------------------|------|
| `chat.receive` | start | info / — | 收到消息 | 入口 |
| `chat.session_fetch` | done/error | ok / error | 加载会话 | |
| `chat.agent_hydrate` | done/error | ok / error | 加载 Agent 配置 | |
| `chat.turn.enter` | start | info / — | 开始处理对话 | |
| `chat.agent.build` | done/error | ok / error | 构建 Agent | |
| `chat.plugins_load` | done | ok / — | 加载插件 | |
| `chat.user_msg_persist` | done/error | ok / error | 保存用户消息 | |
| `chat.intent.pass` | done/skip/error | ok / info / warn | 意图识别 | |
| `chat.llm.invoke` | start/done/error | info / ok / error | 调用语言模型 | 原 `chat.llm_call` |
| `chat.stream.consume` | done/error | ok / error | 处理模型输出 | |
| `chat.assistant_msg_persist` | done/error | ok / error | 保存助手回复 | |
| `chat.turn.execute` | done | ok / — | 对话轮次完成 | |
| `chat.turn.timeout` | error | critical | 对话超时 | |
| `chat.turn.empty_reply` | error | critical | 未收到模型回复 | |
| `chat.first_byte_timeout` | error | critical | 模型响应过慢 | |
| `chat.usage_record` | error | error | 用量记录失败 | Token/Span 落库 |

---

## Team（`domain=team`）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `team.run.start` | ok / error | 开始团队协作 |
| `team.run.build` | ok / error | 构建团队 Agent |
| `team.run.execute` | ok / critical | 执行团队任务 |
| `team.run.finish` | ok / error | 团队任务结束 |
| `team.intent.pass` | ok / warn | 团队意图识别 |

---

## Knowledge（`domain=knowledge`）

| step_id | severity | title |
|---------|----------|-------|
| `knowledge.search` | ok / error | 知识库检索 |
| `knowledge.rerank` | ok / warn | 结果重排 |
| `knowledge.rerank.fallback` | warn | 重排失败，使用向量排序 |

---

## Plugin / System（`domain=plugin` / `system`）

| step_id | severity | title |
|---------|----------|-------|
| `plugin.cost_guard.block` | warn | 费用保护拦截 |
| `plugin.model_router.route` | info | 模型路由 |
| `event_bus.runner.completion` | ok / error | Runner 完成处理 |
| `event_bus.usage.record` | error | 用量事件写入失败 |
| `event_bus.state.persist` | error | 会话状态保存失败 |
| `event_bus.state.apply` | error | 会话状态应用失败 |
| `event_bus.monitor.persist` | error | 监控事件持久化失败 |
| `system.bus.drop` | warn | 事件总线丢弃消息 |
| `system.ws.*` | warn | WebSocket 连接/读写/解析 |
| `system.cron.*` | warn/error | 定时任务死信/重试/panic/跳过 |
| `system.telemetry.*` | info/warn/error | OTel 初始化 |
| `system.agent.*` | info/warn/error | Agent 构建/缓存/DB 解析 |
| `system.provider.*` | info/error | 模型目录与预检 |
| `system.plugin.*` / `system.hook.*` | warn | 插件种子与 Hook 重载 |
| `system.auto_memory.*` | warn/info | 自动记忆提取 |
| `system.monitor.alert_*` | warn | 告警 Webhook/通道 |
| `chat.intent.merge_fail` | warn | 意图结果合并失败 |
| `chat.usage_record_fail` | warn | 用量/轮次记录失败 |
| `team.intent_*` | warn | 团队意图锚点/合并 |

---

## 别名（v1 → v2，兼容 1 版本）

| v1 `flow_step` | v2 `step_id` |
|----------------|--------------|
| `chat.llm_call` | `chat.llm.invoke` |
| `chat.turn_execute` | `chat.turn.execute` |
