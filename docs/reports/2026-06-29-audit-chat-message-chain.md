# Chat 全链路审计报告：前端指令 → 后端响应 → LLM 回复 → 前端展示

> **类型**：架构/代码审计  
> **范围**：前端 Chat UI 发送指令至后端响应并展示的完整链路  
> **关联文档**：
> - [1-chat.design.md](../development/1-chat.design.md)
> - [1-chat.development.md](../development/1-chat.development.md)
> - [2026-06-18-review-full-message-chain-and-solutions.md](./2026-06-18-review-full-message-chain-and-solutions.md)

---

## 1. 执行摘要

| 维度 | 评估 |
|------|------|
| 架构合理性 | 中等偏差。Activity-First 架构方向正确，但 `ChatOrchestrator` 抽象过度、复合接口臃肿，单一职责被稀释。 |
| 逻辑正确性 | 存在多处硬错误：超时硬失败、竞态条件、依赖未注入、状态事件 `kind` 错误，直接影响用户体验。 |
| 过度设计 | 明显。前端去重补偿、`ChatOrchestrator` 多层 phase 抽象、复合 Repo 接口均超出当前需求。 |
| 设计一致性 | 较差。规范要求 No-Timeout，但代码仍有 first-byte/turn 硬超时；事件可靠性分级与实现未完全对齐。 |

**结论**：必须先修复 P0 级功能缺陷，再逐步偿还 P1/P2 级设计债务。

---

## 2. 审计范围

- **前端链路**：`useChatSender` → `ActivityStream` / `TeamCard` / `AgentCard`
- **后端链路**：`ChatOrchestrator` → `ActivityProjector` → `activityEventSequencer` → `RunRegistry`
- **跨层契约**：Activity 事件定义、事件可靠性分级、状态机规范

---

## 3. 问题清单与改进方案

### 3.1 P0 — 立即修复（功能错误）

| # | 问题 | 位置 | 影响 | 改进方案 |
|---|------|------|------|---------|
| P0-1 | **First-byte timeout 仍为硬失败** | `internal/agent/choice_stream.go`、[`internal/service/chat_orchestrator_turn_phases.go`](../development/1-chat.design.md) | 首 token 超过 30s 即报错中断，违反 No-Timeout 原则 | 改为通知-only；在 `assembleTurnResult` 中删除 `DeadlineExceeded → TurnError`，仅触发用户提示与 metrics |
| P0-2 | **Turn timeout 仍为硬失败** | `internal/service/chat_orchestrator_turn_phases.go:assembleTurnResult` | 10min 强制中断长对话 | 同 P0-1，改为通知或软降级 |
| P0-3 | **ToolCategorizer 未注入生产路径** | `internal/agent/activity_projector.go` | 所有 tool 被归为 `other`，前端无法按类别渲染 | 在 `BuildLLMAgent` / Runner 装配点注入；补充构造依赖 |
| P0-4 | **RunRegistry.Cancel 存在竞态** | `internal/runtime/run_registry.go` | 并发 cancel 与 run 完成重叠时可能重复回调/空指针 | 用 `sync.Mutex` + 完成标记保护 cancel 路径；cancel 后 double-check run 仍活跃 |
| P0-5 | **系统状态事件被错误标记为 `ActivityKindSession`** | `internal/service/run_status_publish.go` | 产生空白 `AgentCard` 标题为"成员"、取消按钮无效 | 改为 `ActivityKindNotice`；或为系统 `agentKey` 填充 `AgentName` 并隐藏操作按钮 |
| P0-6 | **TeamCard 成员名显示原始 `agent_key`** | `web/src/components/chat/TeamCard.vue` | 成员名不易识别 | 通过 `AgentStore` 查询 `display_name`；回退到 `agent_key` 时做可读化 |
| P0-7 | **AgentCard 标题显示"成员"** | `web/src/components/chat/AgentCard.vue` | 系统事件卡片无意义 | 对系统 `agentKey` 使用 `AgentName` 字段；无名字时隐藏卡片或显示"系统状态" |
| P0-8 | **取消按钮对系统/已完成卡片仍显示且无效** | `AgentCard.vue`、`TeamCard.vue` | 用户点击无效按钮 | 可见性按 `activity.kind !== notice` + `run.status === running` 控制 |

### 3.2 P1 — 本周内修复（体验问题）

| # | 问题 | 位置 | 改进方案 |
|---|------|------|---------|
| P1-1 | **异步 persist 与前端展示顺序可能错乱** | `internal/agent/activity_event_sequencer.go` | 保证 publish 与 persist 同序；对同一 `activityId` 的更新使用 Upsert 并带单调 version |
| P1-2 | **自动滚动失效** | `ActivityStream.vue` / `useChatScroll.ts` | 用 `requestAnimationFrame` 节流；在 activity 列表变化/新增 final reply 时触发 |
| P1-3 | **中间回复顺序混乱** | `ActivityStream.vue` | 按 `activity.timestamp` 稳定排序，同一毫秒按 kind 优先级 |
| P1-4 | **进度查询消息刷屏** | `useChatSender.ts` / 后端 `check_progress` tool | 将 `check_progress` 标记为 silent tool，不在 UI 渲染；或合并为单个 `notice` |
| P1-5 | **空 thinking block 过多** | `ActivityStream.vue` | 过滤掉 content 为空且已完成的 thinking；或默认折叠 |
| P1-6 | **前端仍在对 `team_stage`/`plan` 做防御性去重** | `web/src/features/chat/composables/useActivityTimeline.ts` | 后端应保证唯一 `activityId`；前端删除去重逻辑，改为按 id 索引 |

### 3.3 P2 — 本月内偿还（设计债务）

| # | 问题 | 位置 | 改进方案 |
|---|------|------|---------|
| P2-1 | `ChatOrchestrator` 过度分层 | `internal/service/chat_orchestrator_turn.go`、`chat_orchestrator_turn_phases.go` | 合并为 `TurnPipeline` 单一结构体，或按阶段拆为内聚小对象，减少跨文件跳转 |
| P2-2 | 复合接口滥用 | `internal/biz/activity.go` 等 | `ActivityRepo`/`SessionRepo` 等方法数远超 5；拆为 `ActivityReader`/`ActivityWriter`/`ActivityUpserter`，Wire 按需绑定 |
| P2-3 | `activityProjector` 维护大量 map 状态 | `internal/agent/activity_projector.go` | 按 concern 拆为 `turnState`、`toolState`、`reasoningState` 子结构体 |
| P2-4 | `memberToolCalls` 计数与展示逻辑耦合 | `internal/agent/activity_projector.go` | 由前端根据 activity 列表自行聚合，或显式输出 `sub_task_board` activity |
| P2-5 | 状态机未显式化 | Run / TeamRun / GraphExecution | 按 AS-FSM-01 要求，定义显式状态机文件 `*_state_machine.go` |
| P2-6 | 事件可靠性配置与代码分级不匹配 | AS-EVT-01 vs `activity_event_sequencer.go` | 同步配置、注释、字段命名 |
| P2-7 | `durableResumeTurnCtx` 与 `turnAdmissionResult` 重复包装 | `chat_orchestrator_turn_phases.go` | 合并为单一上下文对象 |
| P2-8 | `useChatSender` 职责过重 | `web/src/features/chat/composables/useChatSender.ts` | 拆分 stall 检测、重连、消息发送为独立 composable |
| P2-9 | 聊天消息分组仍可能读取 `turn_index` | 全局搜索 | 按红线 #14，仅使用 `role=user` 边界 + 时间顺序分组 |

---

## 4. 跨层 / 架构问题

| # | 问题 | 说明 | 改进方案 |
|---|------|------|---------|
| A-1 | **No-Timeout 规范与实现不一致** | 规范要求无硬超时，但 orchestrator/agent 仍有 | 全面审计 `context.WithTimeout` 在 chat 链路中的使用，全部改为通知或软降级 |
| A-2 | **Activity 真源未统一** | 后端 projector 输出 activity，但前端又基于消息重建 turn | 逐步以 activity 为唯一渲染真源，消息仅作为 fallback |
| A-3 | **事件可靠性配置与代码分级不匹配** | AS-EVT-01 已降级为 2 级，但代码仍保留 3 级痕迹 | 同步配置、注释、字段命名 |
| A-4 | **文档同步债务** | `1-chat.design.md` / `1-chat.development.md` | 本次改动涉及 RPC/行为/activity kind，需同步更新三件套 |

---

## 5. 验收标准

1. **P0 完成后**：
   - 首 token 30s 未返回不再报错，仅提示用户
   - turn 10min 不再硬中断
   - tool 分类正常显示（coding/search 等）
   - 取消按钮仅在运行中且非系统事件时显示
   - TeamCard/AgentCard 显示可识别名称
2. **P1 完成后**：
   - 长对话自动滚动到底部
   - thinking/action/reply 顺序与 LLM 输出一致
   - `check_progress` 不再刷屏
3. **P2 完成后**：
   - `ActivityRepo` 窄接口拆分并通过 Wire 绑定
   - Run/TeamRun 显式状态机存在并覆盖所有转换
   - `ChatOrchestrator` 单方法行数 ≤80、struct 字段 ≤15

---

## 6. 验证命令

```bash
# 后端
make api && make wire && make build && go test ./internal/agent/... ./internal/service/... ./internal/runtime/... -count=1

# 前端
cd web && pnpm lint && pnpm test && pnpm build
```
