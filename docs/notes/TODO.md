# Chat Performance TODO

## P1: plan_and_execute 静默窗口 — 23秒无反馈

**问题**：Spirit Agent 调用 `plan_and_execute` 后，`decomposeTask()` 同步调用 LLM 分解任务（60s超时），期间无任何 Envelope 事件推送到前端，用户看到"正在处理任务…"后长时间无更新。

**时间线**：`butler.orchestration.started` → [23s 静默] → `spirit_plan_created`

**方案 A（推荐，最小改动）**：在 `decomposeTask()` 调用前发布 `spirit_planner_decomposing` Envelope，前端显示"正在分解任务…"
- 文件：`internal/agent/task_planner_impl.go` Plan() 方法，在 decomposeTask() 前发布事件
- 文件：`internal/event/contract/envelope.go` 新增 `EnvelopeTypeSpiritPlannerDecomposing`
- 文件：`web/src/features/spirit/observabilityConstants.ts` ORCHESTRATION_LOADING_MAP 增加条目
- 文件：`web/src/realtime/envelope.ts` 增加类型

**方案 B（深度优化）**：decomposeTask 流式化，子任务逐个产出时发布 `spirit_subtask_created` 事件，前端实时展示子任务列表填充。

**方案 C（架构变更）**：plan_and_execute 异步化，工具调用立即返回，Spirit LLM 先输出过渡文本，编排结果通过事件推送获取。

---

## P2: Agent 构建缓存命中率低 — 208ms/次

**问题**：`BuildTRPCAgentCached` 缓存 key 包含 `Agent.UpdatedAt`，任何 Agent 记录 UPDATE（含运行时统计字段如 `context_used_ratio`）都导致缓存失效，实际命中率远低于预期。

**根因**：
1. `UpdatedAt` 变化 ≠ 配置变化，运行时字段更新也会使缓存失效
2. `ToolVersionHash`/`SkillVersionHash`/`MCPVersionHash` 始终为空，工具配置变化无法自动失效
3. `get` 命中时不刷新 TTL，热 Agent 10分钟后仍过期

**方案 1（推荐，最小改动）**：Cache Key 用 `ConfigHash` 替代 `AgentUpdated`
- `ConfigHash` = SHA256(Settings JSON + ConfigJSON + SystemPrompt + ToolsProfile)
- 只有真正的配置变更才改变 hash，运行时字段更新不影响
- 文件：`internal/agent/cache.go` BuildCacheKey() 修改 fingerprint 结构体

**方案 2**：`get` 命中时刷新 TTL，保持热 Agent 常驻缓存
- 文件：`internal/agent/cache.go` get() 方法，命中后更新 expiresAt

**方案 3**：填充 ToolVersionHash/SkillVersionHash/MCPVersionHash
- 在 `chat_orchestrator_turn.go` 构建 TRPCBuilderDeps 时计算并赋值
- 工具配置变化时缓存 key 自动变化

**方案 4**：增加缓存命中率 Prometheus 监控
- 按 Agent ID 维度记录 hit/miss，便于排查频繁失效的 Agent

---

## P3: TTFT 精确计时 — 已完成

**问题**：日志缺少从事件流消费开始到首 token 的精确计时，无法区分"LLM 延迟"和"工具执行延迟"。

**已完成**：`internal/agent/stream_consumer.go` markFirstByte() 中添加 TTFT 日志
- 输出 `duration_ms`、`ttft_ms`、`first_byte_type`（tool_call/text_delta/runner_completion）、`author`
- StepID: `stream.first_byte`

---

## P4: ChatExecutionCard 独立折叠增强（OBS-08 / P1.5）

**问题**：工具卡片运行时无实时耗时（长任务无法判断是否卡死）、折叠态摘要信息不足（后端不总是提供 summary）、全局展开/折叠仅作用于 TurnBlock 不作用于 ChatExecutionCard、Spirit 模式折叠体验割裂。

**设计**：`docs/development/59-chat-ui-optimization.design.md` §6.4.7（OBS-08） | 详细参考：`docs/reports/2026-06-09-proposal-chat-execution-card-folding.md`

### SP-FE-27: 5s Elapsed Timer

**问题**：工具运行时只显示 spinner，≥5s 后用户无法判断是否卡死。

**方案**：ChatExecutionCard 内部新增 elapsed timer：
- `started_at` → `occurred_at` → `Date.now()` 三级降级获取起始时间
- running ≥5s 显示实时计时器（`5s` → `1m 12s`），≥60s 变 `var(--color-warning)` 警告色
- `onBeforeUnmount` 清理 `setInterval`
- 文件：`web/src/components/chat/ChatExecutionCard.vue`

### SP-FE-28: 折叠态摘要兜底

**问题**：`event.summary` 为空时折叠态无摘要文字。

**方案**：前端根据 `tool_name` + `arguments` 生成摘要：
- `file_edit`/`file_write` → `修改 {filename}`
- `file_read` → `读取 {filename}`
- `grep`/`search_files` → `搜索 "{pattern}"`
- `bash` → `> {command}`
- 文件：`web/src/components/chat/ChatExecutionCard.vue`

### SP-FE-29: ToolStrip 摘要增强

**问题**：ToolStrip 折叠态只显示 `"N tools · Xs"`，不区分工具类型。

**方案**：统计工具类型分布，显示 `"3 file_read · 2.5s"` 或 `"2 grep + 1 bash · 2.5s · 1 failed"`。
- 需 import `toolEventFromMessage`
- 文件：`web/src/components/chat/ToolStrip.vue`

### SP-FE-30: Provide/Inject 全局控制

**问题**：全局"展开全部/折叠全部"仅作用于 TurnBlock，不作用于 ChatExecutionCard；Spirit 模式（TaskExecutionPanel）中的 ChatExecutionCard 不响应全局控制。

**方案**：ChatMessagePanel provide `ExecutionCollapseControl`（`InjectionKey` + `Symbol` 类型安全），ChatExecutionCard inject 响应：
- `expandAllSignal`：递增计数器，watch 后 `expanded=true` + `userManuallyExpanded=true`
- `collapseAllSignal`：递增计数器，watch 后 `status !== 'running'` 时 `expanded=false`
- 全局按钮 `expandAll()`/`collapseAll()` 同时操作 TurnBlock 级 + ChatExecutionCard 级
- inject fallback 为 `null`，无 provide 时行为完全不变
- 文件：`web/src/features/chat/types.ts`（新增接口 + InjectionKey）、`ChatMessagePanel.vue`（provide）、`ChatExecutionCard.vue`（inject + watch）

### SP-FE-31: ToolUseEvent.expanded 死代码清理

**问题**：`ToolUseEvent.expanded` 字段存在于类型定义中但从未被消费，与新增 inject 控制混淆。

**方案**：从 `ToolUseEvent` 类型中移除 `expanded?: boolean` 字段。
- 文件：`web/src/features/chat/types.ts`

### 推迟到后续迭代

- ToolStrip `<details>` → `q-expansion-item` 统一折叠动画
- `aria-expanded`/`aria-controls` 无障碍属性
- 虚拟滚动兼容验证

---

## P5: 进化闭环（P2 阶段，需后端配合）

### SP-EVO-01: Session 执行轨迹 → 技能管家分析输入

**问题**：Team Session 执行数据无法被技能管家消费，技能管家无法基于历史执行优化 Agent 能力。

**方案**：在 Team Session 完成时，将执行轨迹（工具调用序列 + 结果摘要 + 耗时）写入 L3/L4 记忆层，技能管家 dream_cycle 可读取。
- 文件：`internal/biz/spirit_team_usecase.go` CheckAllTeamsCompleted 后触发
- 文件：`internal/memory/` L3/L4 写入接口

### SP-EVO-02: Session 执行轨迹 → 记忆管家分析输入

**问题**：记忆管家 dream_cycle 无法消费 Team Session 数据。

**方案**：同 SP-EVO-01，写入路径复用，dream_cycle 读取时按 session_type=team 过滤。

### SP-EVO-03: 编排效率分析 + DQ Score 输出

**问题**：DQ Score 仅在内存中计算，未持久化，无法用于历史编排效率分析。

**方案**：DQ Score 写入 Session 元数据（`options_json.dq_score`），新增 `/v1/spirit/{id}/analytics` API 返回历史编排效率统计。
- 文件：`internal/biz/dq_score.go` 持久化逻辑
- 文件：`api/kratos/session/v1/session.proto` 新增 RPC

### SP-EVO-04: Agent 能力画像 → 团队组建优化

**问题**：assemble_team 选择 Agent 时无历史表现参考，仅基于静态配置匹配。

**方案**：Agent 能力画像（成功率 + 平均耗时 + 擅长任务类型）写入 Agent 元数据，assemble_team 优先选择历史表现好的 Agent。
- 文件：`internal/agent/profile.go` 能力画像计算
- 文件：`internal/tools/orchestrator/assemble_team.go` 读取画像排序
