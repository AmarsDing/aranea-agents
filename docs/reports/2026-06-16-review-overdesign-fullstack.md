# Review: Overdesign Audit — Full-Stack Pass-Through & Premature Abstraction

> **日期**：2026-06-16
> **版本**：v1.1（含验证修正与修复记录）
> **范围**：前端（Chat/Agent/Session/共享层/Router）+ 后端（Chat/Agent/Skill/Tool/Event/Wire）

---

## 摘要

对 Aranea-Agents 系统进行全栈过度设计排查，覆盖前端 UI/Store/Composable/组件 和后端 Service/Biz/Data/Event/Wire 的逐函数审查。识别出 **20 个前端问题** 和 **17 个后端问题**，核心根因为三大模式：**透传层堆叠**（简单功能经 6-8 层零逻辑转发）、**单实现接口爆炸**（365 个 biz 接口绝大多数仅 1 个实现）、**过早抽象**（Composable/Store/类型文件以"可能复用"为假设拆分，实际仅 1 个消费者）。按收益/风险/工作量三维评审，筛选出 Top 5 修复项，预计 1.5-2 天可消除 ~700 行透传/重复代码 + 10 个冗余文件 + 1 个无价值 Store + 10 个单实现接口。

---

## 一、评审方法

| 维度 | 权重 | 说明 |
|------|------|------|
| 收益 | 40% | 减少多少代码/复杂度？对日常开发体验改善多大？ |
| 风险 | 30% | 改动是否安全？能否增量推进？回归风险多高？ |
| 工作量 | 30% | 改动涉及多少文件？是否需要跨层协调？ |

---

## 二、过度设计三大根因

| 根因 | 表现 | 影响 |
|------|------|------|
| **透传层堆叠** | 简单功能经过 6-8 层调用才到达真正实现 | 每层零逻辑，纯转发，增加阅读和维护成本 |
| **单实现接口爆炸** | 365 个 biz 接口绝大多数只有 1 个实现 | 接口组合金字塔，4 层嵌套只为最终得到 1 个实现 |
| **过早抽象** | Composable/Store/类型文件以"可能复用"为假设拆分，实际仅 1 个消费者 | 文件数膨胀，导航成本远大于维护收益 |

---

## 三、前端问题清单

### P0 — 严重

| # | 问题 | 位置 | 量化 | 建议 |
|---|------|------|------|------|
| FE-1 | useChatWorkspace 是 God Composable | `web/src/features/chat/composables/useChatWorkspace.ts` | 1144 行，返回 50+ 属性，编排 15+ 子 composable | 按 Tab/面板拆为 3-4 个独立页面 composable |
| FE-2 | runtimeStore 是纯透传层 | `web/src/stores/chat/runtimeStore.ts` | 12 个 action 全部 `return apiFunction()`，零状态逻辑 | 删除 Store，调用方直接用 API 函数；wsConnected 状态移到 streamManager |
| FE-3 | 发送消息 8 层调用链 | Composer → ComposerActions → Sender → sendAgentMessage → sendAgentUserContent → sendUserContent → strategy → streamManager | 8 层中 3 层零逻辑透传 | 合并 ComposerActions 到 Sender，删除 sendAgentMessage/sendAgentUserContent 中间层 |
| FE-4 | Agent 模块 9 个 composable 仅 1 消费者 | useAgentA2AEndpointTab, useAgentA2AProxyTab, useAgentAvatarIcon, useAgentChannelRefs, useAgentEvolutionPanel, useAgentEvolutionSettings, useAgentHooksPanel, useAgentModelValidation, useAgentToolOverrides | 21 个 composable 中 43% 仅 1 消费者 | 合并回消费者组件或父 composable |
| FE-5 | Agent Detail Store 12/14 action 为 API 透传 | `web/src/stores/agents/detail.ts` | 86% 的 action 无状态逻辑 | 仅保留有状态逻辑的 action，其余由 composable 直接调 API |

### P1 — 中等

| # | 问题 | 位置 | 量化 | 建议 |
|---|------|------|------|------|
| FE-6 | sessionStore Agent/Team 对称重复 | `web/src/stores/chat/sessionStore.ts` | 8 个方法（4 对）逻辑完全对称 | 参数化为 4 个方法，接受 `entityKind` |
| FE-7 | Session 双 Store 重复逻辑 | `web/src/stores/session/` + `web/src/stores/chat/sessionStore.ts` | 两者都调 createSession/deleteSession/archiveSession 等 | 提取共享逻辑到 composable 或基础 Store |
| FE-8 | Chat 70 个根 TS 文件粒度过细 | `web/src/features/chat/*.ts` | 平均不足 50 行/文件 | 按子域合并：消息处理→messageUtils.ts，流处理→streamUtils.ts |
| FE-9 | useChatInboundSync 认知复杂度过高 | `web/src/features/chat/composables/useChatInboundSync.ts` | 473 行，7 层嵌套条件 | 按信封类型拆为 handler map 模式 |
| FE-10 | 3 个 EventFilter 文件 | `eventFilter.ts` + `useEventFilter.ts` + `composables/useEventFilter.ts` | 3 个文件做同一件事 | 保留 1 个，删除其余 2 个 |
| FE-11 | Agent 运行时配置 4 文件拆分 | `agentRuntimeConfig*.ts` (4 文件) | 处理同一对象的读写 | 合并为 1 个文件 |
| FE-12 | 约 7 个极薄透传 Store | agentsCatalog/avatarCatalog/heartbeat/event/learningLoop/skillIntelligence/skillEvolution | 每个 Store 仅 1-3 个透传 action | 改为 composable 直接调 API |
| FE-13 | useChatSender 5 种超时管理 | `web/src/features/chat/composables/useChatSender.ts` | 5 种独立超时 + 各自 clear 函数 | 统一为 TimeoutManager 或合并为 2-3 种 |

### P2 — 轻微

| # | 问题 | 位置 | 量化 | 建议 |
|---|------|------|------|------|
| FE-14 | 3 个死组件 | ChatSkillHintBar/ChatSkillCatalogStrip/ChatMentionPopup | 0 引用 | 删除 |
| FE-15 | 1 个死 composable | useToolDisplayMode | 0 消费者 | 删除 |
| FE-16 | 1 个废弃 API 函数 | enqueueUserMessage | 已标记 @deprecated | 删除并更新调用方 |
| FE-17 | 2 个纯 re-export barrel 文件 | envelope.ts / dispatcher.ts | 仅 re-export | 删除，更新 import 路径 |
| FE-18 | A2UI 8 文件仅服务 2 组件 | `a2ui*.ts` + `a2ui/` | 28 computed 属性的 composable | 简化为 2-3 个文件 |
| FE-19 | 6 个类型文件可合并 | activityTypes/timelineTypes/agentTreeTypes 等 | 每个文件极小 | 合并为 1-2 个类型文件 |
| FE-20 | useRunStatus 与 useChatRunStatus 重叠 | `composables/useRunStatus.ts` | 遗留代码 | 标记 deprecated 并迁移 |

---

## 四、后端问题清单

### P0 — 严重

| # | 问题 | 位置 | 量化 | 建议 |
|---|------|------|------|------|
| BE-1 | Service 层 23 个单实现接口 | `chat_orch_run_status.go`, `chat_orch_pending_queue.go`, `chat_orch_await.go` 等 | 23 个接口，每个仅 1 个实现，~200 行纯接口定义 | 删除接口，子管理器直接用 struct；需要 mock 时在测试中生成 |
| BE-2 | ChatService 20+ 个纯转发方法 | `internal/service/chat.go` + `run_status_store.go` + `chat_await_resume.go` + `chat_await_route.go` | 4 个文件共 ~120 行纯 `s.orch.xxx()` 转发 | ChatOrchestrator 直接实现 biz Gateway 接口，消除转发层 |
| BE-3 | Skill Evolution 10 接口 4 层组合金字塔 | `internal/biz/skill/skill_evolution_unified.go` | CheckReader→QueryReader→Reader→Store→Bridge，4 层组合仅 1 实现 | 直接使用 EvolutionStoreBridge 复合接口，删除中间层 |
| BE-4 | ChatUsecase 与 Service 子管理器职责重叠 | ChatUsecase vs chatAwaitCoordinator/chatPendingQueueManager/chatRunStatusTracker | 同一职责在 biz 和 service 两层各实现一次 | Service 子管理器直接内联到 ChatOrchestrator，消除与 ChatUsecase 的重复 |

### P1 — 中等

| # | 问题 | 位置 | 量化 | 建议 |
|---|------|------|------|------|
| BE-5 | AgentRuntimeSettings 140 字段 + 320 行映射 | `service/agent.go` 中 toProtoRuntime/fromProtoRuntime | 320 行零业务逻辑的纯字段搬运 | 用代码生成替代手写映射，或统一 Proto/Go 类型语义 |
| BE-6 | Tool 模块 14 接口单实现 | `internal/biz/tool/` | 6 个接口无独立消费者 | 合并为 2-3 个功能接口 |
| BE-7 | Skill 6 层读接口嵌套 | `internal/biz/skill/` | SkillReader→3 子接口→SkillRepo，仅 1 实现 | 直接使用 SkillRepo 复合接口 |
| BE-8 | 67+ 事件类型平铺 | `internal/event/contract/envelope.go` | 多数事件仅 1 对生产者-消费者 | 按域分组，减少全局常量 |
| BE-9 | provideChatServiceDeps 30+ 参数 | `cmd/admin/wire.go` | 上帝函数，依赖堆积 | ChatService 拆分为更小的服务单元 |
| BE-10 | wireOut 46 字段含 ~25 个后台 Job | `cmd/admin/wire.go` | Job 生命周期管理分散 | 引入 JobRegistry，Wire 只注入 1 个 Registry |
| BE-11 | 5 个 Adapter 仅做类型转换 | `chat_run_gateway.go`, `chat_native.go` | ~150 行字段名映射 | 统一 runtime/biz 类型定义，消除 Adapter |
| BE-12 | agent_ports.go 9 个单方法接口 | `internal/biz/agent/agent_ports.go` | 多数仅 1 实现 | 合并为 3-4 个功能接口 |
| BE-13 | 2 个重复 WebResearch 适配器 | `wire.go` 中 webResearchReadinessAdapter + bizWebResearchReadinessAdapter | 同一逻辑两个版本 | 统一为 1 个接口定义 |

### P2 — 轻微

| # | 问题 | 位置 | 量化 | 建议 |
|---|------|------|------|------|
| BE-14 | event 包 re-export 层 | `event/envelope.go` | ~90 行纯类型别名 | 新代码统一 import contract，逐步删除 re-export |
| BE-15 | 双 Bus 架构 | `event.Infra` (SessionBus + MonitorBus) | 可用单 Bus + 优先级替代 | 评估合并可行性 |
| BE-16 | logpipeline 4 层 Sink 抽象 | `pkg/logpipeline/` | Pipeline→SinkGroup→Sink→SinkFactory | 功能完整但偏复杂，暂不建议改 |
| BE-17 | DurableResumeGateway 仅 1 方法 | `internal/biz/turn_gateway.go` | 可合并到 TurnControlGateway | 合并 |

---

## 五、典型过度设计案例

### 案例 1：发送一条消息的 8 层调用链

```
用户点击发送
  → ChatComposer.onSend()
    → useChatComposerActions.onSend()          ← 零逻辑，委托 sender
      → useChatSender.onSend()                 ← 判断 entityKind
        → sendAgentMessage()                   ← 零逻辑，委托 sendAgentUserContent
          → sendAgentUserContent()             ← 零逻辑，委托 sendUserContent
            → sendUserContent()                ← 真正的逻辑
              → strategy.ensureSession()
              → strategy.buildWsPayload()
              → deps.sendChatViaWs()           ← 又一层委托
                → streamManager.sendChatViaWs()
                  → stream.transport.send()     ← 终点
```

8 层中 3 层是零逻辑透传（ComposerActions、sendAgentMessage、sendAgentUserContent），可压缩为 4 层。

### 案例 2：Skill Evolution 的 4 层接口组合金字塔

```
UnifiedEvolutionCheckReader (3 methods)
UnifiedEvolutionQueryReader (2 methods)
    └── UnifiedEvolutionReader = CheckReader + QueryReader (5 methods)
UnifiedEvolutionMutationWriter (3 methods)
UnifiedEvolutionExpirationWriter (2 methods)
    └── UnifiedEvolutionWriter = MutationWriter + ExpirationWriter (5 methods)
        └── UnifiedEvolutionStore = Reader + Writer (10 methods)
            └── EvolutionStoreBridge = Store + LegacyStore (15+ methods)
                └── 唯一实现: data.EvolutionStoreBridge
```

4 层组合，每层仅 1 个实现，中间层无独立消费者。直接定义 `EvolutionStoreBridge` 即可。

### 案例 3：后端 ChatService 的纯转发文件

- `internal/service/run_status_store.go`（71 行，9 个方法）
- `internal/service/chat_await_resume.go`（27 行，6 个方法）
- `internal/service/chat_await_route.go`（16 行，3 个方法）

三个文件共 114 行，每个方法都是 `s.orch.xxx()` 的单行转发。

---

## 六、不过度设计的部分（值得保留）

| 模块 | 理由 |
|------|------|
| SessionRun 状态机 | 5 种状态 + CAS 转换，有真实并发安全需求 |
| Durable Resume 机制 | checkpoint + claim + resume，跨进程恢复的真实需求 |
| Await Channel 机制 | goroutine 协调的真实并发控制需求 |
| Turn Pipeline（admission → persist → execute） | 真实的排队和流控需求 |
| Router 守卫 | 仅 1 个 beforeEach，逻辑简洁 |
| 布局组件 | MainLayout + BlankLayout，职责清晰 |
| Adapter 层（RuntimeLogAdapter） | 107 行，唯一必要的桥接 |
| Session 窄接口拆分 | 子接口方法数合理，不同消费者确实只需依赖子集 |

---

## 七、量化总览

| 指标 | 当前值 | 合理上限 | 超标倍数 |
|------|--------|---------|---------|
| 单 composable 行数 | 1144 | 300 | 3.8x |
| 发送消息调用层数 | 8 | 4 | 2x |
| Service 层单实现接口数 | 23 | 0（非多态场景） | 23x |
| Skill Evolution 接口组合层数 | 4 | 1-2 | 2-4x |
| Chat 根 TS 文件数 | 70 | 20-30 | 2-3x |
| biz 层接口总数 | 365 | ~100（按单实现合并后） | 3.6x |
| 透传 Store 数 | 8 | 0 | 8x |
| wireOut 字段数 | 46 | 15-20 | 2-3x |
| 后台 Job 数 | ~25 | 统一 Registry 管理 | 分散管理 |

---

## 八、Top 5 修复项（按收益/风险/工作量综合排序）

| 排名 | 编号 | 问题 | 预估收益 | 预估工作量 | 风险 |
|------|------|------|---------|-----------|------|
| 1 | FE-2 | 删除 runtimeStore | 消除 1 个无价值透传层 | 1-2h | 极低 |
| 2 | FE-14/15/16/17 | 清理死代码（3 死组件 + 1 死 composable + 1 废弃 API + 2 re-export） | 删除 7 个无用文件 | 30min | 零 |
| 3 | FE-6 | sessionStore Agent/Team 参数化 | 减少 ~200 行重复 | 2-3h | 低 |
| 4 | BE-2 | 消除 ChatService 纯转发方法 | 删除 3 文件 114 行转发 | 半天 | 中 |
| 5 | BE-3 | 扁平化 Skill Evolution 接口金字塔 | 10 接口 → 1，删除 ~200 行 | 半天 | 中 |

**Top 5 合计**：约 1.5-2 天工作量，可消除 ~700 行透传/重复代码 + 10 个冗余文件 + 1 个无价值 Store + 10 个单实现接口。

---

## 九、第二梯队修复项（高收益但需谨慎）

| 排名 | 编号 | 问题 | 说明 |
|------|------|------|------|
| 6 | BE-1 | 删除 Service 层 23 个单实现接口 | 需更新所有消费者和 mock 测试，1-2 天 |
| 7 | FE-4 | 合并 Agent 单消费者 composable | 9 个 composable 合并回消费者，改 ~15 文件，半天 |
| 8 | FE-7 | Session 双 Store 重复逻辑 | 提取共享逻辑到 composable 或基础 Store |
| 9 | BE-4 | ChatUsecase 与 Service 子管理器重叠 | 涉及 biz/service 两层职责重新划分，影响面广 |
| 10 | BE-10 | JobRegistry 统一后台 Job | 架构改进但需改动 Wire 全局结构 |

---

## 十、第三梯队（架构级改进，需专项规划）

| 编号 | 问题 | 说明 |
|------|------|------|
| FE-1 | useChatWorkspace God Composable | 收益最高但风险也最高，1144 行重构需完整测试覆盖 |
| FE-3 | 发送消息 8 层调用链扁平化 | 需重新设计 Sender 策略模式 |
| BE-9 | provideChatServiceDeps 30+ 参数 | ChatService 需拆分为更小服务单元 |
| BE-5 | AgentRuntimeSettings 140 字段 + 320 行映射 | 需代码生成方案或 Proto 类型统一 |
| BE-8 | 67+ 事件类型平铺 | 需按域重新组织事件契约 |

---

## 十一、建议的"零透传"原则

以上问题的根因不是某一层的局部失误，而是全栈透传模式的系统性蔓延。建议建立以下原则作为后续代码审查的判断标准：

> **每一层转发必须添加至少一行有意义的逻辑，否则直接让调用方持有下一层的引用。**

具体落地：
1. Store action 必须管理至少 1 个响应式状态，否则调用方直接用 API 函数
2. Service 方法必须包含至少 1 行业务逻辑（校验/转换/编排），否则 Orchestrator 直接实现 biz 接口
3. Composable 必须被至少 2 个消费者使用才独立提取，否则内联到消费者
4. biz 接口必须有至少 2 个实现（或明确的 mock 需求）才独立定义，否则使用复合接口

---

## 十二、验证修正与修复记录（v1.1 — 2026-06-16）

### 验证修正

对报告 37 个问题逐项验证后，以下数据需修正：

| 编号 | 原声称 | 实际值 | 修正说明 |
|------|--------|--------|----------|
| FE-1 | 50+ 属性、15+ 子 composable | **140 个叶属性、21 个子 composable** | 问题比声称更严重 |
| FE-2 | 12 action 全透传、零状态逻辑 | **15 函数中 12 透传 + 3 个 wsConnected 状态管理** | "零状态逻辑"不准确 |
| FE-3 | 8 层调用链 | **7 层，2 层纯透传 + 1 层极薄包装** | 程度略轻 |
| FE-4 | 43% 仅 1 消费者 | **80% 仅 1 消费者（16/20）** | 问题比声称严重得多 |
| FE-5 | 12/14 透传 (86%) | **11/14 透传 (78.6%)** | 略低但核心判断成立 |
| FE-8 | 70 个根 TS 文件、平均不足 50 行 | **60 个文件、平均约 110 行** | 粒度偏细但程度被夸大 |
| FE-12 | 约 7 个极薄透传 Store | **仅 2 个**（agentsCatalog、event） | 严重夸大 |
| FE-15 | useToolDisplayMode 死 composable | **文件不存在** | 已被删除或从未存在 |
| BE-1 | 23 个单实现接口 | **35 个接口，29 个仅 1 实现（83%）** | 比声称更严重 |
| BE-3 | 10 接口 | **8 接口** | 略低但 4 层金字塔确认 |
| BE-7 | 6 层读接口嵌套 | **3 层** | 层数被夸大 |
| BE-9 | 30+ 参数 | **40 参数** | 比声称更严重 |
| BE-10 | 46 字段含 ~25 Job | **47 字段含 ~35 Job** | 比声称更严重 |
| BE-11 | 5 个 Adapter | **22 个 Adapter/Gateway** | 严重低估 |
| BE-12 | 9 个单方法接口 | **9 接口中 8 单方法 + 1 双方法** | 微调 |

### 反驳项

| 编号 | 问题 | 判定理由 |
|------|------|---------|
| FE-11 | Agent 运行时配置 4 文件拆分 | **合理拆分**——类型/填充/序列化/子域各 131-246 行，合并超 500 行限制 |
| BE-17 | DurableResumeGateway 仅 1 方法 | **Go 窄接口惯用模式**，非过度设计 |

### 已修复问题

| 编号 | 问题 | 修复内容 | 状态 |
|------|------|---------|------|
| FE-14 | 3 个死组件 | 删除 ChatSkillHintBar.vue、ChatSkillCatalogStrip.vue、ChatMentionPopup.vue | ✅ 已修复 |
| FE-15 | useToolDisplayMode 死 composable | 文件不存在，无需操作 | ✅ 无需操作 |
| FE-16 | enqueueUserMessage 废弃函数 | 从 api.ts 删除函数定义 | ✅ 已修复 |
| FE-17 | 2 个 re-export barrel 文件 | 删除 envelope.ts、dispatcher.ts，更新 24 处 import 指向 realtime/ 目录 | ✅ 已修复 |
| FE-2 | runtimeStore 纯透传层 | 删除 Store，wsConnected 迁移到 useChatStreamManager，11 个 API 透传改为消费者直接调 API | ✅ 已修复 |
| FE-12 | agentsCatalog 极薄 Store | 删除 Store，2 个消费者改为直接调 listAgents API | ✅ 已修复 |
| FE-12 | event 极薄 Store | 删除 Store，1 个消费者改为直接调 listSessionEvents API | ✅ 已修复 |
| FE-4 | useAgentChannelRefs 单消费者 composable | 内联到 AgentChannelRefsSection.vue（86 行） | ✅ 已修复 |
| BE-2 | ChatService 纯转发文件 | 删除 chat_await_resume.go、chat_await_route.go，errResumeInFlight 迁移到 chat_orch_await.go | ✅ 已修复 |
| BE-13 | 重复 WebResearch 适配器 | biz.WebResearchPlatformFields/WebResearchReadinessChecker 改为 tool 包类型别名，删除 wire.go 重复适配器 | ✅ 已修复 |

### 审查修复（aranea-review）

| 审查项 | 问题描述 | 修复内容 |
|--------|---------|---------|
| BLK-01 | 删除 barrel 文件后 19 个文件 import 未更新 | 更新所有 import 指向 realtime/ 目录 |
| ADV-01 | tool_reexport.go 大规模 re-export 无设计说明 | 添加文件头注释说明向后兼容意图 + TECH-DEBT 标注 |
| ADV-03 | webResearchPlatformFields 重复代码 | 统一到 tool 包，biz 层通过 re-export 调用 |
| ADV-04 | composable 直接调 API 未标 TECH-DEBT | useChatSender/useFollowUpQueue/useChatEventInspector 添加 TECH-DEBT(FL5) 标注 |
| ADV-05 | useChatWorkspace 直接调 API 未标 TECH-DEBT | 添加 TECH-DEBT(FL5) 标注 |

### 修复量化

| 指标 | 修复前 | 修复后 | 变化 |
|------|--------|--------|------|
| 删除的死代码文件 | — | 8 个（3 组件 + 2 barrel + 1 Store + 2 Go 文件） | -8 文件 |
| 删除的 Store | — | 3 个（runtimeStore + agentsCatalog + event） | -3 Store |
| 消除的透传 action | — | 26 个（15 runtimeStore + 1 agentsCatalog + 1 event + 9 Go 转发方法） | -26 透传 |
| 消除的重复代码 | — | ~120 行（2 个 WebResearch 适配器 + 2 个转换函数 + 2 个 Go 转发文件） | -120 行 |
| 内联的 composable | — | 1 个（useAgentChannelRefs） | -1 文件 |
| 添加的 TECH-DEBT 标注 | — | 5 处 | +5 标注 |

### 未修复项（需后续迭代）

| 编号 | 问题 | 原因 |
|------|------|------|
| FE-1 | useChatWorkspace God Composable (1125 行) | 风险最高，需完整测试覆盖后专项重构 |
| FE-3 | 发送消息 7 层调用链 | 需重新设计 Sender 策略模式 |
| FE-4 | Agent 15 个单消费者 composable | 多数 >50 行或消费者已超 200 行，合并成本高 |
| FE-5 | Agent Detail Store 11/14 透传 | 需评估哪些 action 应迁入 composable |
| FE-6 | sessionStore Agent/Team 对称重复 | 数据容器差异使参数化需额外抽象 |
| BE-1 | Service 层 29 个单实现接口 | 需更新所有消费者和 mock 测试，1-2 天 |
| BE-3 | Skill Evolution 4 层接口金字塔 | 需更新 Wire 绑定和所有消费者 |
| BE-4 | ChatUsecase 与 Service 子管理器重叠 | 涉及 biz/service 两层职责重新划分 |
| BE-9 | provideChatServiceDeps 40 参数 | ChatService 需拆分为更小服务单元 |

### 12.7 Phase 1 实施记录（已完成）

**日期**：2026-06-16

| 修复项 | 文件 | 变更 | 状态 |
|--------|------|------|------|
| P1.1 | `streamHandlers.ts` | 删除 6 个 Legacy handler + legacyUnsubs/activateAFMode/afMode/streamRowId/snapshotStreamingMessage/snapshotCounter/patchStreamingEnvelope/patchMessages/patch；删除 onStreamingPatch/streamIdPrefix 接口属性；删除 if (!afMode) return 守卫；添加 runner_completion/error 兜底 handler | ✅ |
| P1.2 | `mergeSessionMessages.ts` | 删除 isStreamingPlaceholder/findServerMatchForStreaming/buildServerAssistantContentMap + 合并循环 Legacy 分支 | ✅ |
| P1.3 | `messageOrigin.ts` | 删除 ws-stream-/ws-team-stream-/ws-snap- 前缀分支 | ✅ |
| P1.3 | `chatStreamingSnapshots.ts` | 删除 applyStreamingSnapshotToSession 函数 | ✅ |
| P1.3 | `channelFocusLoad.ts` | 删除 applyStreamingSnapshotToSession 导入和调用 | ✅ |
| P1.4 | `streamContentPatch.ts` | 删除 isReasoningAsDisplay 函数和 reasoning_as_display 字段 | ✅ |
| P1.1 | `useChatStreamManager.ts` | 删除 onStreamingPatch 回调和 streamIdPrefix 属性 | ✅ |
| P1.1 | `streamHandlers.spec.ts` | 删除 5 个 Legacy handler 测试用例 | ✅ |

**审查结果**：0 个阻断；4 个建议中 S-01/S-02（error/runner_completion 兜底 handler）已修复，S-03（mergeSessionMessages 清理不完整）已修复，S-04（测试文件同步）为低优先级；1 个提示（inbound sync 兼容层注释）记录备忘。

**净减代码**：约 500 行（streamHandlers.ts ~300 行 + mergeSessionMessages.ts ~70 行 + 其他文件 ~130 行）
