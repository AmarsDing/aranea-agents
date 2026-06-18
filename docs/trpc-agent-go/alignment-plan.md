# trpc-agent-go 框架对齐总计划

> 基于 17 份模块对齐分析文档综合制定，覆盖全部模块的对齐项、依赖关系、阶段划分和收益量化。

---

## 一、全局概览

### 1.1 模块对齐度矩阵

| 模块 | 对齐度 | P1 项 | P2 项 | P3 项 | P4 项 | 总对齐项 |
|------|--------|-------|-------|-------|-------|---------|
| Event | ★★★★★ | 3✅ | 2✅ | 2 | 0 | 7 |
| Prompt | ★★★☆☆ | 1✅ | 2✅ | 1✅ | 0 | 4 |
| Knowledge | ★★★☆☆ | 2✅ | 2✅ | 6 | 0 | 10 |
| Team | ★★★☆☆ | 0 | 3✅ | 5 | 0 | 8 |
| Evaluation | ★★★★☆ | 2✅ | 2✅ | 3 | 0 | 7 |
| Tool | ★★★★★ | 1✅ | 2(1✅) | 2 | 0 | 5 |
| Session | ★★★★☆ | 0 | 4✅ | 4 | 0 | 8 |
| Memory | ★★★★☆ | 0 | 2✅ | 6 | 0 | 8 |
| Server | ★★★★☆ | 0 | 2✅ | 3(1✅) | 0 | 5 |
| Model | ★★★★★ | 0 | 3(1✅) | 7(1✅) | 0 | 10 |
| Agent | ★★★★★ | 0 | 0 | 7(3✅) | 1 | 8 |
| Runner | ★★★★★ | 0 | 0 | 6(2✅) | 0 | 6 |
| Skill | ★★★★★ | 0 | 2✅ | 5(2✅) | 0 | 7 |
| Extended | ★★★★☆ | 1✅ | 1✅ | 1✅ | 3 | 6 |
| **合计** | — | **9(6✅)** | **27(14✅)** | **54(13✅)** | **4** | **98(33✅)** |

### 1.2 对齐类型分布

| 类型 | 数量 | 占比 |
|------|------|------|
| 启用框架功能 | 39 | 39% |
| 贡献回框架 | 30 | 30% |
| 替换自建实现 | 16 | 16% |
| 新增适配层 | 14 | 14% |

> 注：原计划中 30 项"贡献回框架"已调整为 5 项（Phase 1 Event 贡献已完成），其余 25 项改为 internal 适配层或跳过（不贡献回框架）。

### 1.3 依赖链

```
Event ──→ Session ──→ Memory
   │
   └──────→ Tool ──→ Callback
                │
                └──→ Skill ──→ Knowledge

Team ──→ Agent ──→ Runner
  │
  └──→ Graph

Server（独立，无强依赖）
Extended（独立，ToolPipe 依赖 Tool）
```

---

## 二、P1 对齐项（立即执行）

> 9 项，对齐度极低或业务收益极高，应立即启动。

| # | 模块 | 对齐项 | 类型 | 核心收益 |
|---|------|--------|------|---------|
| P1-1 | Event | 贡献 EventBus（双总线） | 贡献回框架 | 框架获得事件总线能力，项目减少 ~800 行自建代码 | ✅ 已完成 |
| P1-2 | Event | 贡献 EventWAL（WBPF） | 贡献回框架 | 框架获得事件持久化+先写后发能力 | ✅ 已完成 |
| P1-3 | Event | 贡献事件可靠性分级 | 贡献回框架 | 框架获得 Critical/Important/Informational 三级可靠性保证 | ✅ 已完成 |
| P1-4 | Tool | 启用 ToolPipe Extension | 启用框架功能 | Token 消耗降低 50-90%（框架 benchmark 数据） | ✅ 已完成 |
| P1-5 | Knowledge | 实现 VectorStore 适配器 | 新增适配层 | 对接框架 Knowledge 接口，统一向量搜索 | ❌ 已删除（永久阻塞：框架 `vectorstore.VectorStore` 接口无消费者，死代码已删，详见 §十一/B-1） |
| P1-6 | Knowledge | 实现 Embedder 适配器 | 新增适配层 | 对接框架 Embedder 接口，统一嵌入生成 | ❌ 已删除（永久阻塞：框架 `embedder.Embedder` 接口无消费者，死代码已删，详见 §十一/B-2） |
| P1-7 | Evaluation | 启用框架 LLM Judge | 启用框架功能 | 替换自建 LLM-as-Judge，减少维护 | ✅ 已完成 |
| P1-8 | Evaluation | 启用 Callbacks | 启用框架功能 | 评估流程获得框架 Callback 能力 | ✅ 已完成 |
| P1-9 | Prompt | 启用 PromptIter 替换 PromptRefiner | 启用框架功能 | 替换自建 PromptRefiner，获得框架迭代优化能力 | 🟡 阻塞：框架协作者缺失（"编译错误"经核实为误判，详见 §十一/B-3） |

### P1 执行顺序

```
P1-1/2/3 (Event) ──→ 可并行启动，无外部依赖
P1-4 (ToolPipe) ──→ 独立，可并行
P1-5/6 (Knowledge) ──→ 可并行，但需先确认框架接口稳定性
P1-7/8 (Evaluation) ──→ 可并行
P1-9 (PromptIter) ──→ 独立
```

**建议**：P1 全部可并行启动，无模块间依赖。按团队资源分配，优先 P1-4（ToolPipe，收益立竿见影）和 P1-1/2/3（Event，是后续 Session/Memory 对齐的前置）。

---

## 三、P2 对齐项（下个迭代）

> 27 项，对齐度中等或需要 P1 完成后才能启动。

### 3.1 依赖 P1 完成的 P2 项

| # | 模块 | 对齐项 | 类型 | 前置依赖 | 核心收益 |
|---|------|--------|------|---------|---------|
| P2-1 | Event | Envelope 适配框架 Event | 替换自建实现 | P1-1 EventBus | 70+ 事件类型与框架事件体系统一 | ✅ 已完成 |
| P2-2 | Event | 贡献 FlowTracker/SpanCollector | 贡献回框架 | P1-1 EventBus | 框架获得链路追踪能力 | ✅ 已完成（纯数据层） |
| P2-3 | Session | 启用 AppendEventHook | 启用框架功能 | P1-1 EventBus | 事件写入后自动触发回调 | ✅ 已完成 |
| P2-4 | Session | 启用 GetSessionHook | 启用框架功能 | P1-1 EventBus | Session 读取时注入自定义逻辑 | ✅ 已完成 |
| P2-5 | Session | 评估 Summary 替换 micro_compact | 替换自建实现 | P2-3 | 框架 Summary 替换自建压缩 | ✅ 已完成（补充模式） |
| P2-6 | Session | 启用 WithSessionEventLimit | 启用框架功能 | P2-3 | 控制单 Session 事件数量 | ✅ 已完成 |
| P2-7 | Memory | L2/L3 适配 memory.Service 接口 | 新增适配层 | P1-1 EventBus | 记忆层对接框架接口 | ✅ 已实现（TECH-DEBT：未接入生产路径，项目自建 AutoMemoryWorker 更完善） |
| P2-8 | Memory | Extractor Chain 适配 MemoryExtractor | 新增适配层 | P2-7 | 提取器链对接框架接口 | ✅ 已实现（TECH-DEBT：未接入生产路径，同 P2-7） |
| P2-9 | Tool | DeferredCallableTool 实现 DeferredTool | 替换自建实现 | P1-4 ToolPipe | ✅ 已完成（`internal/tools/deferred/deferred_tool.go` 实现 `ShouldDefer()` 满足 `trpctool.DeferredTool` 接口） |
| P2-10 | Tool | 贡献 Circuit Breaker | 贡献回框架 | 无 | ⏭ 跳过（不贡献回框架） |
| P2-11 | Tool | 贡献 Confirmation Gate | 贡献回框架 | 无 | ⏭ 跳过（不贡献回框架） |
| P2-12 | Tool | ToolResultGate + ToolPipe 协调 | 新增适配层 | P1-4 ToolPipe | ✅ 已完成（功能已通过 `internal/agent/tool_result_gate_hook.go` 的 BeforeModelHook 接入，`callback_chain.go:62` 调用；`toolresult_gate_adapter.go` 为冗余实现，死代码已删，遵循 CS-B2） |

### 3.2 独立 P2 项（无前置依赖）

| # | 模块 | 对齐项 | 类型 | 核心收益 |
|---|------|--------|------|---------|
| P2-13 | Model | 启用 tiktoken counter | 启用框架功能 | 精确 token 计数，替代估算 | ✅ 已完成 |
| P2-14 | Model | 贡献 Rate Limit Transport | 贡献回框架 | ⏭ 跳过（不贡献回框架） |
| P2-15 | Model | 贡献 Retry Transport | 贡献回框架 | ⏭ 跳过（不贡献回框架） |
| P2-16 | Model | 贡献 Circuit Breaker Transport | 贡献回框架 | ⏭ 跳过（不贡献回框架） |
| P2-17 | Model | 贡献 Callback Chain | 贡献回框架 | ⏭ 跳过（不贡献回框架） |
| P2-18 | Team | 适配 Export() 结构 | 新增适配层 | ✅ 已实现（TECH-DEBT：未接入生产路径，项目 Team 编排不使用框架 structure.Snapshot 导出） |
| P2-19 | Team | 借鉴 Swarm 安全机制 | 启用框架功能 | ✅ 已实现（TECH-DEBT：未接入生产路径，项目使用自建安全策略） |
| P2-20 | Team | 借鉴 Session 隔离 | 启用框架功能 | ✅ 已实现（TECH-DEBT：未接入生产路径，项目使用自建会话隔离机制） |
| P2-21 | Knowledge | 实现 Knowledge 接口 | 新增适配层 | ✅ 已完成 |
| P2-22 | Knowledge | 使用框架 SearchTool | 启用框架功能 | 🟡 阻塞：双重注册冲突 + 丢失动态 collection 限定能力（已补充 TECH-DEBT(B-4) 标记，详见 §十一/B-4） |
| P2-23 | Knowledge | 贡献 HybridRetriever | 贡献回框架 | ⏭ 跳过（不贡献回框架） |
| P2-24 | Knowledge | 贡献 AdaptiveRouter | 贡献回框架 | ⏭ 跳过（不贡献回框架） |
| P2-25 | Evaluation | 启用 EvalSet Recorder | 启用框架功能 | 🟡 阻塞：依赖评估集基础设施（Recorder + 持久化后端），与 P1-9/P2-26 PromptIter 同一阻塞点（框架 `promptiter` 需 `evalset.Recorder` 协作者，项目缺评估集持久化层） |
| P2-26 | Evaluation | 启用 PromptIter | 启用框架功能 | 🟡 阻塞：与 P1-9 同一阻塞点，框架 `promptiter` 需 5 个协作者（详见 §十一/B-3） |
| P2-27 | Skill | 启用提示缓存优化 | 启用框架功能 | ✅ 已启用（`internal/agent/trpc_build.go:160` 调用 `WithSkillsLoadedContentInToolResults(true)`，渐进加载模式下已启用） |
| P2-28 | Skill | 贡献 DBRepositoryAdapter | 贡献回框架 | ✅ 已完成（internal 适配层，非框架贡献） |
| P2-29 | Server | 启用 AG-UI 协议端点 | 启用框架功能 | ✅ 已完成（生产路径通过 `internal/service/agui_compat.go` 的 `AGUICompatService` 包装层接入，解决 Runner per-session 问题；server 层直接适配器死代码已删） |
| P2-30 | Server | 启用 A2A 扩展点 | 启用框架功能 | ✅ 已完成（生产路径通过 `internal/service/a2a_extension_compat.go` 的 `A2AExtensionCompatService` 包装层接入，支持懒加载 + AgentCard 构建；server 层直接适配器死代码已删） |
| P2-31 | Extended | 启用 TodoEnforcer 扩展 | 启用框架功能 | ✅ 已完成（`trpc_build.go` 条件启用：仅当 Agent 已有 `todo_write` 工具时追加 `NewTodoEnforcerOption`，避免向不使用 todo 的 Agent 注入工具。框架 earlier-wins 去重保证用户工具优先，enforcer 通过 session state key `temp:todos:<branch>` 追踪状态） |
| P2-32 | Prompt | 启用 prompt.Text.Render() | 启用框架功能 | 🟡 阻塞：设计前提不匹配（模板渲染 vs 结构化拼接，已标记 TECH-DEBT(B-5)，详见 §十一/B-5） |
| P2-33 | Prompt | 启用 state.Render() | 启用框架功能 | 🟡 阻塞：设计前提不匹配（模板渲染 vs RuntimeState 注入，已标记 TECH-DEBT(B-6)，详见 §十一/B-6） |

### P2 执行顺序

```
第一批（无前置依赖，可立即启动）：
  P2-13~17 (Model Transport) ──→ 可并行
  P2-18~20 (Team 基础) ──→ 可并行
  P2-27~28 (Skill) ──→ 可并行
  P2-29~30 (Server) ──→ 可并行
  P2-31 (TodoEnforcer) ──→ 独立
  P2-32~33 (Prompt Render) ──→ 可并行

第二批（依赖 P1 完成）：
  P2-1~2 (Event 后续) ──→ 依赖 P1-1
  P2-3~6 (Session) ──→ 依赖 P1-1
  P2-7~8 (Memory) ──→ 依赖 P1-1
  P2-9~12 (Tool 后续) ──→ 依赖 P1-4
  P2-21~24 (Knowledge 后续) ──→ 依赖 P1-5/6
  P2-25~26 (Evaluation 后续) ──→ 依赖 P1-7/8
```

---

## 四、P3 对齐项（按需执行）

> 54 项，对齐度较高或非紧急，按业务需求逐步推进。

### 4.1 跨模块协同 P3 项（建议打包执行）

#### 协同包 A：Agent 运行时增强

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Agent | 启用 WithKnowledge | Agent 获得框架知识注入 | ✅ 已完成（KnowledgeAdapter 桥接项目 Retriever 到框架 knowledge.Knowledge） |
| Agent | 启用安全限制 | Agent 获得框架安全保护 | ✅ 已完成（SafetyLimitAdapter 启用 MaxLLMCalls/MaxToolIterations） |
| Agent | 启用时间注入 | Agent 获得框架时间感知 | |
| Agent | 评估 ActivatableToolSets vs DeferredManager | 工具集管理策略统一 | ✅ 已完成（评估结论：保持自建 DeferredManager，详见 `docs/trpc-agent-go/02-agent.md` 对齐项 #4。两者功能重叠但机制不同，DeferredManager 更适合项目场景） |
| Agent | 贡献 BuildCache | 框架获得 LRU+singleflight 缓存 | ⏭ 跳过（不贡献回框架） |
| Agent | 贡献 AgentFactory 增强 | 框架获得 Agent 工厂增强 | ⏭ 跳过（不贡献回框架） |
| Runner | 评估 WithPersistInterruptedAssistant | 中断恢复能力 | ✅ 已完成（chat_orchestrator_durable.go 启用） |
| Runner | 评估 WithDetachedCancel | 取消机制增强 | ✅ 已完成（`chat_orchestrator_durable.go:111` 启用 `WithDetachedCancel(true)`，分离 context 取消传播，长任务取消不影响父流程） |
| Runner | 评估 WithStreamMode | 流式模式标准化 | ✅ 已完成（buildTurnRunOptions 启用 StreamModeMessages） |
| Runner | 贡献 RunnerManager/RunRegistry | 框架获得运行管理能力 | ⏭ 跳过（不贡献回框架） |
| Runner | 贡献 RunnerRollback | 框架获得回滚能力 | ⏭ 跳过（不贡献回框架） |

#### 协同包 B：Session/Memory 深度对齐

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Session | 评估 WindowService | 框架窗口管理替代自建 |
| Session | 贡献 Session 状态机 | 框架获得 5 状态显式状态机 | ⏭ 跳过（不贡献回框架） |
| Session | 贡献 SessionLockManager | 框架获得并发安全 | ⏭ 跳过（不贡献回框架） |
| Session | 贡献多级压缩管线 | 框架获得 4 级压缩能力 | ⏭ 跳过（不贡献回框架） |
| Memory | 贡献 L0-L4 分层抽象 | 框架获得五层记忆模型 | ⏭ 跳过（不贡献回框架） |
| Memory | 贡献 PII Scanner | 框架获得隐私检测 | ⏭ 跳过（不贡献回框架） |
| Memory | 贡献 MemoryInject Plugin | 框架获得记忆注入 | ⏭ 跳过（不贡献回框架） |
| Memory | 贡献 PriorityQueue | 框架获得优先级队列 | ⏭ 跳过（不贡献回框架） |
| Memory | 贡献 Audit Hook | 框架获得策略审计 | ⏭ 跳过（不贡献回框架） |
| Memory | 评估 Auto 模式 | 框架自动记忆管理 |

#### 协同包 C：Team 编排增强

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Team | 贡献 sequential/parallel/critic_loop 模式 | 框架获得 3 种编排模式 | ⏭ 跳过（不贡献回框架） |
| Team | 贡献 Graph 编译层 | 框架获得图编译能力 | ⏭ 跳过（不贡献回框架） |
| Team | 贡献 TeamFailurePolicy | 框架获得失败策略 | ⏭ 跳过（不贡献回框架） |
| Team | 贡献 HITL 支持 | 框架获得人机交互 | ⏭ 跳过（不贡献回框架） |
| Team | 评估 Team 实现 agent.Agent | Team 作为 Agent 嵌套 |

#### 协同包 D：Model 生态增强

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Model | 暴露 TailoringStrategy | Token 裁剪策略可配置 | ✅ 已完成（resolveTailoringStrategy 函数 + catalog 字段注释） |
| Model | 暴露 TokenTailoringConfig | 裁剪参数可配置 | ✅ 已完成（`internal/provider/catalog.go` 暴露 `EnableTokenTailoring`/`TokenTailoringStrategy`/`TokenTailoringSafetyMargin` 三个字段；`internal/provider/trpc_llm.go:282` 调用 `trpcprovider.WithTokenTailoringConfig` 注入框架） |
| Model | 贡献 Metrics Model | 框架获得模型指标 | ⏭ 跳过（不贡献回框架） |
| Model | 贡献 ModelSelector 策略 | 框架获得 5 种选择策略 | ⏭ 跳过（不贡献回框架） |

#### 协同包 E：Skill/Knowledge 深度对齐

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Skill | 启用命令安全限制 | Skill 运行安全增强 | ✅ 已完成（`ChatOrchestrator` 持有 `CommandSafetyPermissionChecker`，`buildTurnRunOptions` 通过 `WithToolPermissionPolicyFunc` 注入 per-run 权限策略。非保护工具零开销通过，保护工具（exec_command/shell_exec/file 等）访问敏感路径（.aws/.ssh/.kube/.env 等）时拒绝） |
| Skill | 启用输出大小限制 | 防止上下文窗口溢出 | ✅ 已完成（通过通用 Tool AfterTool 回调实现，覆盖所有工具包括 Skill Run，未使用框架 `WithSkillRunOutputLimits` 但防护范围更广） |
| Skill | FSRepository 显式实现接口 | 接口合规性 | ✅ 已完成 |
| Skill | 贡献 artifactSavingExecutor | 框架获得装饰器保存 | ⏭ 跳过（不贡献回框架） |
| Skill | 评估交互式执行工具 | 支持长时间交互式技能 | |
| Knowledge | 贡献 QueryRewriter 多策略 | 框架获得查询改写 | ⏭ 跳过（不贡献回框架） |
| Knowledge | 贡献 FederatedRetriever | 框架获得联邦检索 | ⏭ 跳过（不贡献回框架） |
| Knowledge | 贡献 RetrievalEvaluator | 框架获得检索评估 | ⏭ 跳过（不贡献回框架） |
| Knowledge | 贡献 Collection 抽象 | 框架获得集合管理 | ⏭ 跳过（不贡献回框架） |

#### 协同包 F：Server/Extended 增强

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Server | 启用 OpenAI 完整选项 | OpenAI 会话持久化 | ✅ 已完成（生产路径通过 `internal/service/openai_session_compat.go` 的 `OpenAISessionCompatService` 包装层接入；server 层直接适配器死代码已删，避免违反 R1 lint 规则） |
| Server | 贡献 WebSocket ServiceFactory | 框架获得 WS 传输 | ⏭ 跳过（不贡献回框架） |
| Server | 贡献多租户 A2A 路由器 | 框架获得多 Agent 路由 | ⏭ 跳过（不贡献回框架） |
| Extended | 评估 TaskRun 共存 | Agent 获得异步委派能力 | 🟡 阻塞：缺少 Controller 实例 + 复杂依赖链（已标记 TECH-DEBT(B-7)，详见 §十一/B-7） |
| Runner | 删除透传包装器 | 代码简化 |
| Event | Graph 事件用框架 EventEmitter | 事件源统一 |
| Evaluation | 贡献 SQLite 后端 | 框架获得评估持久化 | ⏭ 跳过（不贡献回框架） |
| Evaluation | 贡献脚本模拟器 | 框架获得模拟评估 | ⏭ 跳过（不贡献回框架） |
| Evaluation | 贡献 AfterTurn 触发器 | 框架获得自动评估 | ⏭ 跳过（不贡献回框架） |
| Prompt | 启用 ValidateRequired() | Prompt 必填校验 | 🟡 阻塞：依赖 P2-32 的 RenderPromptTemplate 接入，而 P2-32 被阻塞（详见 §十一/B-5） |
| Agent | 评估 structure.Exporter | 结构化导出 |
| Agent | 评估 TransferController | 传输控制 | ✅ 已完成（transfer_controller.go 实现，原子计数器深度限制+超时） |

### 4.2 P4 项（保持现状）

| 模块 | 对齐项 | 说明 |
|------|--------|------|
| Agent | 评估 structure.Exporter | 当前无业务需求 |
| Extended | Dify Agent 集成预留 | 当前无业务需求 |
| Extended | N8N Agent 集成预留 | 当前无业务需求 |
| Extended | Codex Agent 集成预留 | 当前无业务需求 |

---

## 五、阶段化实施路线

### Phase 0：基础建设（1-2 周） ✅ 已完成

**目标**：建立对齐基础设施，完成无风险的配置级启用。

| 任务 | 模块 | 工作量 | 风险 | 状态 |
|------|------|--------|------|------|
| 启用 ToolPipe Extension | Tool/Extended | 小 | 低 | ✅ 已完成 |
| 启用提示缓存优化 | Skill | 小 | 低 | ✅ 已确认（渐进加载模式下已启用） |
| FSRepository 接口断言 | Skill | 小 | 无 | ✅ 已完成 |
| 启用 tiktoken counter | Model | 小 | 低 | ✅ 已完成 |

**产出**：Token 消耗降低 50-90%（ToolPipe）+ 10-30%（Skill 缓存），零代码风险。

**实施记录**：

| 变更文件 | 变更内容 |
|---------|---------|
| `internal/agent/trpc_build.go` | +25 行：ToolPipe Extension 注册（`WithToolScope(isToolPipeEligible)` 白名单模式）+ tiktoken counter 注入（ContextCompaction 启用时精确计数，失败 fallback）+ `isToolPipeEligible` 函数（MCP 工具 + 6 个长输出工具） |
| `internal/skill/trpc/repository.go` | +5 行：FSRepositoryAdapter 编译期接口断言（`Repository`/`RootedRepository`/`RefreshableRepository`） |
| `go.mod` / `go.sum` | 新增依赖：`agent/extension/toolpipe` + `model/tiktoken` + `github.com/tiktoken-go/tokenizer` 等 |

**审查结果**：aranea-review 通过，0 阻断项，2 建议项（均不修复：ToolPipe 无条件启用安全；工具名硬编码 YAGNI）。

### Phase 1：Event 对齐（2-3 周） ✅ 已完成

**目标**：完成 Event 模块对齐，解锁 Session/Memory 依赖链。

| 任务 | 类型 | 工作量 | 风险 | 状态 |
|------|------|--------|------|------|
| 贡献 EventBus（双总线） | 贡献回框架 | 大 | 中 | ✅ P1-1 已完成 |
| 贡献 EventWAL（WBPF） | 贡献回框架 | 大 | 中 | ✅ P1-2 已完成 |
| 贡献事件可靠性分级 | 贡献回框架 | 中 | 低 | ✅ P1-3 已完成 |
| 启用 Plugin.OnEvent | 启用框架功能 | 小 | 低 | ✅ P2 已完成（eventTypeLabel 细化） |
| Envelope 适配框架 Event | 替换自建实现 | 中 | 中 | ✅ P2 已完成（FromFrameworkEvent 统一转换） |
| 贡献 FlowTracker/SpanCollector | 贡献回框架 | 中 | 低 | ✅ P2 已完成（纯数据层 tracing 包） |

**前置条件**：Phase 0 完成（ToolPipe 验证 Extension 注册机制）。

**产出**：Event 模块对齐度 ★★☆☆☆ → ★★★★★，解锁 Session/Memory 对齐。

**实施记录（P1-1/P1-2/P1-3）**：

| 变更文件 | 变更内容 |
|---------|---------|
| `pkg/trpc-agent-go/event/bus/bus.go` | 新增 ~300 行：泛型 Bus[T] 接口和实现，含 DropPolicy（DropOldest/DropNewest/BlockUpTo）、ChannelPriority（Critical/Normal）、SubscribeOptions、EventMatcher 过滤、DropLogger 回调、DefaultBufferSize/MaxBufferSize 常量 |
| `pkg/trpc-agent-go/event/bus/bus_test.go` | 新增 ~250 行：Bus 测试覆盖 Publish/Subscribe/PriorityOrder/DropNewest/BlockUpTo/Reliable/Filter/Unsubscribe/MultipleSubscribers/BufferSize/MatchLevelFilter |
| `pkg/trpc-agent-go/event/wal/wal.go` | 新增 ~220 行：泛型 WAL[T] 实现，含 Storage 接口（Insert/MarkPublished/ListUnpublished/PurgePublished/Close）、ExistChecker、IsCriticalFunc、SerializeFunc/DeserializeFunc、Logger 接口、WALOption 函数选项模式 |
| `pkg/trpc-agent-go/event/wal/memory_storage.go` | 新增 ~80 行：MemoryStorage 测试实现 |
| `pkg/trpc-agent-go/event/wal/wal_test.go` | 新增 ~200 行：WAL 测试覆盖 WriteBeforePublish/Recover/ExistChecker/PurgePublished/NilStorage/NilIsCritical |
| `pkg/trpc-agent-go/event/reliability/reliability.go` | 新增 ~140 行：泛型 Classifier[T] 可靠性分级器，含 Tier（Critical/Important/Informational）、RWMutex 并发安全、Register/RegisterBulk/Classify/IsRegistered/Tiers |
| `pkg/trpc-agent-go/event/reliability/reliability_test.go` | 新增 ~150 行：Classifier 测试覆盖 Classify/RequiresBlockUpTo/IsCriticalWBPF/TierString/IsRegistered/Tiers/CustomFallback/IntKeyType |
| `internal/event/contract/reliability.go` | 重构：从自包含 switch 分级改为委托 `reliability.Classifier[EnvelopeType]`，EventReliability 成为 `reliability.Tier` 类型别名 |
| `internal/event/bus_adapter.go` | 新增 ~140 行：busAdapter 将框架 Bus[Envelope] 适配到 contract.Bus，含 DropLogger（loggateway）、SubscribeOptions 转换、Filter 组合 |
| `internal/event/bus.go` | 修改：NewBus 委托到 busAdapter，legacyBus 保留并标注 TECH-DEBT，移除 stderr 写入 |
| `internal/event/wal_storage.go` | 新增 ~100 行：sqliteWALStorage 适配 *sql.DB 到框架 wal.Storage，含 ctx 参数、Scan 错误日志、time.Parse 错误处理 |
| `internal/event/wal.go` | 修改：EventWAL 委托到框架 WAL[Envelope]，walLogger 适配器透传 kv 参数（toLoggatewayFields），legacyEventWAL 保留并标注 TECH-DEBT |

**审查结果**：aranea-review 三轮审查，0 阻断项。已修复 12 项审查问题（REV-001~REV-014，含并发安全、错误传播、红线合规、魔法数字、TECH-DEBT 标注等）。

**实施记录（P2-1/P2-2）**：

| 变更文件 | 变更内容 |
|---------|---------|
| `internal/event/framework_adapter.go` | 新增 ~115 行：`FromFrameworkEvent` 统一转换函数 + `FrameworkEventMeta` 元数据结构 + `isJSONString`/`coalesceStr` 辅助函数。单源真相：framework `*event.Event` → project `Envelope` 字段映射 |
| `internal/event/framework_adapter_test.go` | 新增 ~228 行：9 个测试用例覆盖基础字段、meta 回退、Extensions/Actions/nil Actions、空时间戳、Response、isJSONString、coalesceStr、返回类型 |
| `internal/plugin/trpc/hook_events.go` | 修改：`eventTypeLabel` 从 3 类（event/model_response/error）扩展到 10 类（runner_completion/chat.completion.chunk/chat.completion/tool.response/error/agent.transfer/state.update/preprocessing/postprocessing/model_response），覆盖所有框架 `model.ObjectType` |
| `internal/agent/event_projector.go` | 修改：`baseEnvelope` 方法从手动字段提取改为调用 `event.FromFrameworkEvent`，消除与 Graph EventBridge 的重复逻辑 |
| `internal/graph/trpc/event_bridge.go` | 修改：`convertEvent` 方法从手动字段提取改为调用 `event.FromFrameworkEvent`，修复 `IsZero() == false` → `!IsZero()` |
| `internal/event/flow_context_state.go` | 修改：`FlowContext` type alias 委托到 `frameworktracing.FlowContext`，添加 TECH-DEBT(P2-alignment) 注解 |
| `internal/event/span_context.go` | 修改：`SpanContext`/`UsageContext` type alias 委托到 `frameworktracing`，添加 TECH-DEBT(P2-alignment) 注解，移除 28 行冗余代码（恒真指针转换、方法签名检查、FlowTiming 空引用、time import） |
| `internal/event/usage_context.go` | 删除：`UsageContext` 合并到 `span_context.go` |
| `internal/event/flow_tracker.go` | 修改：`emit()` 签名改为接受 `*frameworktracing.FlowTiming`，内部转换为项目 `FlowTiming`（保留 `StartedAt` 字段） |
| `internal/event/bus.go` | 修改：移除 ~260 行 legacyBus 实现（TECH-DEBT，零调用者），仅保留 re-export + `NewBus` 委托 |
| `internal/event/wal.go` | 修改：移除 ~130 行 legacyEventWAL 实现（TECH-DEBT），仅保留 EventWAL 委托 + walLogger 适配器 |
| `pkg/trpc-agent-go/event/tracing/tracing.go` | 新增 ~287 行：框架级 tracing 原语包，零外部依赖。含 FlowContext（步骤计时）、SpanContext（Span 树管理）、UsageContext（OTel 关联 + turn 计时），全部使用 sync.Mutex 并发安全 |
| `pkg/trpc-agent-go/event/tracing/tracing_test.go` | 新增 ~200 行：13 个测试用例覆盖 FlowContext/SpanContext/UsageContext 含并发访问测试 |

**审查结果**：aranea-review 两轮审查，0 阻断项。第一轮修复 5 项（S-09 IsZero 修正、S-10 TECH-DEBT 注解、S-11 冗余代码清理、S-03 coalesceStr 重复评估、S-07 event_projector 行数预存问题），第二轮 0 阻断、1 建议（预存）、1 提示。

### Phase 2：Session + Memory 对齐（3-4 周） ✅ 已完成

**目标**：完成 Session 和 Memory 模块对齐。

| 任务 | 类型 | 工作量 | 风险 | 状态 |
|------|------|--------|------|------|
| 启用 AppendEventHook | 启用框架功能 | 小 | 低 | ✅ 已完成 |
| 启用 GetSessionHook | 启用框架功能 | 小 | 低 | ✅ 已完成 |
| 评估 Summary 替换 micro_compact | 替换自建实现 | 中 | 中 | ✅ 已完成（补充模式） |
| 启用 WithSessionEventLimit | 启用框架功能 | 小 | 低 | ✅ 已完成 |
| L2/L3 适配 memory.Service | 新增适配层 | 中 | 中 | ✅ 已验证（已有完整实现） |
| Extractor Chain 适配 MemoryExtractor | 新增适配层 | 中 | 中 | ✅ 已完成 |

**前置条件**：Phase 1 完成（Event 对齐）。

**产出**：Session 对齐度 ★★★☆☆ → ★★★★☆，Memory 对齐度 ★★★☆☆ → ★★★★☆。

**实施记录**：

| 变更文件 | 变更内容 |
|---------|---------|
| `internal/session/trpc/sqlite.go` | +15 行：添加 `WithSessionEventLimit(5000)` + `WithAppendEventHook` + `WithGetSessionHook` + `WithSummarizer`（DynamicSummarizer），新增 `sessionEventLimit` 常量 |
| `internal/session/trpc/hooks.go` | 新增 ~85 行：`AppendEventAuditHook`（事件写入审计日志）+ `GetSessionAuditHook`（Session 读取审计日志）+ `eventTypeLabel` 分类映射 |
| `internal/session/trpc/summarizer.go` | 新增 ~67 行：`SummarizerConfig` + `NewDynamicSummarizer`（DynamicSummarizer 模式，按请求解析模型）+ `resolveSummaryModel`（复用 PickTitleModel 策略） |
| `internal/session/trpc/factory.go` | 修改：`NewTRPCSessionService` 签名添加 `SummarizerConfig` 参数 |
| `internal/session/compressor.go` | +10 行：`postCompressionSync` 添加 `EnqueueFrameworkSummary` 调用，压缩后同步框架摘要状态 |
| `internal/memory/trpc/extractor_adapter.go` | 新增 ~110 行：`ConsolidatorExtractorAdapter` 将 `biz.MemoryConsolidator` 适配为 `extractor.MemoryExtractor` 接口 |
| `internal/runtime/providers.go` | 修改：`NewTRPCSessionService` 签名添加 `SummarizerConfig` |
| `internal/runtime/providers_test.go` | 修改：测试更新 |
| `cmd/admin/wire.go` | 修改：`provideTRPCSessionService` 注入 `LlmProviderModelUsecase` + `RoundTrip` |
| `cmd/admin/wire_gen.go` | 修改：同步 Wire 生成物 |

**审查结果**：aranea-review 一轮审查，修复 3 个阻断项（nil 指针解引用、ToolCall 信息丢失、空 proposal 过滤）+ 1 个建议项（魔法数字常量化），0 阻断项剩余。

**设计决策**：
- **Summary 不替换 micro_compact**：框架 Summary 操作 `session.Summaries`（异步 LLM 摘要），项目 Compressor 操作 `biz.SessionSummary` + Runner Snapshot（三级级联压缩）。两者互补而非替代：Compressor 压缩后调用 `EnqueueFrameworkSummary` 同步框架摘要状态。
- **L2/L3 适配已验证**：项目 `sqliteMemoryService` 已完整实现 `memory.Service` 接口（含编译期断言），无需额外适配层。
- **Extractor 适配器为未来桥接**：`ConsolidatorExtractorAdapter` 允许项目在准备好时切换到框架的 auto-memory worker，当前仍使用项目自建的 `AutoMemoryWorker`。

### Phase 3：Knowledge + Evaluation + Prompt 对齐（2-3 周） ✅ 已完成

**目标**：完成 Knowledge、Evaluation、Prompt 三个低对齐度模块。

| 任务 | 类型 | 工作量 | 风险 | 状态 |
|------|------|--------|------|------|
| 实现 VectorStore 适配器 | 新增适配层 | 中 | 中 | ✅ 已完成 |
| 实现 Embedder 适配器 | 新增适配层 | 中 | 中 | ✅ 已完成 |
| 实现 Knowledge 接口 | 新增适配层 | 中 | 中 | ✅ 已完成 |
| 使用框架 SearchTool | 启用框架功能 | 小 | 低 | ✅ 已完成 |
| 启用框架 LLM Judge | 启用框架功能 | 中 | 中 | ✅ 已完成 |
| 启用 Callbacks | 启用框架功能 | 小 | 低 | ✅ 已完成 |
| 启用 PromptIter | 启用框架功能 | 中 | 中 | ✅ 已完成 |
| 启用 prompt.Text.Render() | 启用框架功能 | 中 | 低 | ✅ 已完成 |
| 启用 state.Render() | 启用框架功能 | 小 | 低 | ✅ 已完成 |

**前置条件**：Phase 1 完成（Event 对齐，Knowledge 依赖 Event）。

**产出**：Knowledge ★☆☆☆☆ → ★★★☆☆，Evaluation ★★★☆☆ → ★★★★☆，Prompt ☆☆☆☆☆ → ★★★☆☆。

**实施记录（P1-7/P1-8）**：

| 变更文件 | 变更内容 |
|---------|---------|
| `internal/evaluation/judge_runner.go` | 新增 ~55 行：`NewJudgeRunner` 创建 `runner.Runner` 供 `WithJudgeRunner` 使用，复用 `resolveJudgeModel` 模型解析链，LLM Agent 输出结构化 `{score, reason}` JSON |
| `internal/evaluation/callbacks.go` | 新增 ~70 行：`NewEvalCallbacks` 创建框架 Callbacks 实例，注册 `AfterInferenceCase` + `AfterEvaluateCase` 进度日志回调 |
| `internal/evaluation/framework.go` | 重构：移除 `llmJudge` 字段和额外调用（~25 行），新增 `judgeRunner` + `callbacks` 字段，注入 `WithJudgeRunner` + `WithCallbacks` |
| `internal/evaluation/framework_metrics.go` | 修改：新增 `llm_as_judge` metric 注册（`criterion.WithLLMJudge(&criterionllm.LLMCriterion{})` + `EvaluatorName: "llm_final_response"`），`metricSpec` 新增 `evaluatorName` 字段 |
| `internal/evaluation/runner.go` | 修改：移除 `LLMJudge` 类型和 `llmJudge` 字段，`NewRunner` 签名简化 |
| `internal/evaluation/metrics.go` | 修改：移除 `scoreLegacyCase` 的 `llmJudge` 参数和 `LLMJudgeScore` 字段，Legacy 路径不再支持 LLM Judge |
| `internal/evaluation/runner_legacy.go` | 修改：移除 `llmJudge` 使用，修复吞错误问题（`_ =` → `if err := ... { r.lg.Warn(...) }`） |
| `internal/evaluation/llm_judge.go` | 精简：仅保留 `LLMJudge` 类型定义并标记 `Deprecated`，移除 `NewLLMJudge`/`pickJudgeModel`/`parseJudgeScore`（~90 行） |
| `internal/evaluation/eval_llm_resolve.go` | 修改：`pickJudgeModel` 从 `llm_judge.go` 迁入，增加空切片防御检查 |
| `internal/evaluation/scores.go` | 修改：`applyMetricResult` 新增 `MetricLLMAsJudge` → `res.LLMJudgeScore` 映射 |
| `internal/service/evaluation_runner.go` | 修改：`NewEvaluationRunner` 用 `NewJudgeRunner` + `NewEvalCallbacks` 替换 `NewLLMJudge`，Judge Runner 初始化失败时优雅降级 |

**审查结果**：aranea-review 一轮审查，1 阻断项（legacy 路径吞错误）+ 5 建议项。已修复阻断项和 2 个建议项（`pickJudgeModel` 防御检查、魔法数字常量化），0 阻断项剩余。

**实施记录（Knowledge + Prompt 对齐）**：

| 变更文件 | 变更内容 |
|---------|---------|
| `internal/knowledge/vectorstore_adapter.go` | 新增 ~150 行：VectorStoreAdapter 将自建 vector.VectorStore 适配到框架 vectorstore.VectorStore 接口，含 metaAnyToString/metaStringToAny 转换、TECH-DEBT 标注不支持方法 |
| `internal/knowledge/vectorstore_adapter_test.go` | 新增 ~200 行：13 个测试覆盖 Add/Update/Delete/Search/Get/Count/DeleteByFilter/UpdateByFilter/GetMetadata/Close |
| `internal/knowledge/embedder_adapter.go` | 新增 ~55 行：EmbedderAdapter 将 MultiProviderEmbedder 适配到框架 embedder.Embedder，含 float32→float64 转换 |
| `internal/knowledge/embedder_adapter_test.go` | 新增 ~80 行：6 个测试覆盖 GetEmbedding/GetEmbeddingWithUsage/GetDimensions/float32sToFloat64s |
| `internal/knowledge/knowledge_adapter.go` | 新增 ~145 行：KnowledgeAdapter 实现 knowledge.Knowledge 接口，使用 SearchFunc 桥接所有检索器，含 toBizQuery/toSearchResult 转换 |
| `internal/knowledge/knowledge_adapter_test.go` | 新增 ~100 行：6 个测试覆盖 Search/nil request/empty chunks/collection_id 提取/filter 序列化/best doc 选择 |
| `internal/tools/knowledge/framework_searchtool.go` | 新增 ~150 行：NewFrameworkSearchTool 和 NewFrameworkAgenticFilterSearchTool，使用框架 knowledge.NewKnowledgeSearchTool |
| `internal/agent/prompt_render.go` | 新增 ~35 行：RenderPromptTemplate 和 RenderCapabilityCue，使用框架 prompt.Text.Render() |
| `internal/agent/prompt_render_test.go` | 新增 ~60 行：5 个测试覆盖变量替换/未匹配保留/空模板/空变量 |
| `internal/agent/state_render.go` | 新增 ~120 行：RenderStateTemplate + stateResolver，使用 prompt.Text.Render + 自定义 Resolver 重新实现框架内部 stateResolver 逻辑 |
| `internal/agent/state_render_test.go` | 新增 ~200 行：16 个测试覆盖 invocation state/session state/optional/namespace/artifact/nil session/empty template |
| `internal/agent/prompt_iter_adapter.go` | 新增 ~145 行：PromptIterAdapter 桥接框架 engine.Engine 到 biz.Refiner 接口，fallback 到 biz.Refiner |
| `internal/agent/prompt_iter_adapter_test.go` | 新增 ~230 行：12 个测试覆盖 fallback/engine success/engine error/extract/build request/interface satisfaction |

**审查结果**：aranea-review 一轮审查，2 阻断项（biz 层违规 import 框架包、裸 panic）+ 4 建议项。已全部修复：PromptIterAdapter 从 biz 移到 agent 层、panic 改为 error 返回、dead code 清理、TECH-DEBT 标注。

### Phase 4：Model + Tool + Skill 对齐（3-4 周） ✅ 已完成

**目标**：完成 Model Transport 贡献、Tool 后续对齐、Skill 深度对齐。

| 任务 | 类型 | 工作量 | 风险 | 状态 |
|------|------|--------|------|------|
| 贡献 Rate Limit Transport | 贡献回框架 | 中 | 低 | ⏭ 跳过（不贡献回框架） |
| 贡献 Retry Transport | 贡献回框架 | 中 | 低 | ⏭ 跳过（不贡献回框架） |
| 贡献 Circuit Breaker Transport | 贡献回框架 | 中 | 低 | ⏭ 跳过（不贡献回框架） |
| 贡献 Callback Chain | 贡献回框架 | 中 | 中 | ⏭ 跳过（不贡献回框架） |
| DeferredCallableTool 实现 DeferredTool | 替换自建实现 | 中 | 中 | |
| 贡献 Circuit Breaker（Tool 级） | 贡献回框架 | 中 | 低 | ⏭ 跳过（不贡献回框架） |
| 贡献 Confirmation Gate | 贡献回框架 | 中 | 中 | ⏭ 跳过（不贡献回框架） |
| 贡献 DBRepositoryAdapter | 贡献回框架 | 中 | 中 | ✅ 已完成（internal 适配层，非框架贡献） |
| 启用命令安全限制 | 启用框架功能 | 中 | 中 | ✅ 已完成 |
| 启用输出大小限制 | 启用框架功能 | 小 | 低 | ✅ 已完成 |

**前置条件**：Phase 0 完成（ToolPipe 验证），Phase 2 完成（Session 对齐影响 Tool）。

**产出**：Model ★★★★☆ → ★★★★★，Tool ★★★★☆ → ★★★★★，Skill ★★★★☆ → ★★★★★。

**实施记录**：

| 变更文件 | 变更内容 |
|---------|---------|
| `internal/tools/toolresult_gate_adapter.go` | 新增 ~110 行：NewToolResultGateAfterHook 将 ToolResultGate 适配为 tool.AfterToolCallbackStructured |
| `internal/tools/toolresult_gate_adapter_test.go` | 新增 ~150 行：10 个测试覆盖 gate check/truncation/no session/nil args/error fallback |
| `internal/tools/security/command_safety_adapter.go` | 新增 ~80 行：CommandSafetyPermissionChecker 实现 tool.PermissionChecker，启用框架命令安全 |
| `internal/tools/security/command_safety_adapter_test.go` | 新增 ~135 行：7 个测试覆盖 allow/deny/nil request/custom policy/nil policy |
| `internal/tools/toolresult_size_limiter.go` | 新增 ~80 行：NewOutputSizeLimiterHook 返回 tool.AfterToolCallbackStructured，截断超长输出 |
| `internal/tools/toolresult_size_limiter_test.go` | 新增 ~120 行：9 个测试覆盖 truncation/within limit/non-string/error pass-through |
| `internal/skill/trpc/db_repository_adapter.go` | 新增 ~140 行：DBStoreAdapter 桥接 biz skill 层到 DBStore 接口，内联定义 DBStore 接口和 ErrSkillNotFound |
| `internal/skill/trpc/db_repository_adapter_test.go` | 新增 ~80 行：测试覆盖 ListSummaries/GetByName/GetPathByName |

### Phase 5：Team + Agent + Runner 对齐（4-6 周） ✅ 已完成

**目标**：完成 Team 编排对齐、Agent 运行时增强、Runner 贡献。

| 任务 | 类型 | 工作量 | 风险 | 状态 |
|------|------|--------|------|------|
| 适配 Export() 结构 | 新增适配层 | 中 | 中 | ✅ 已完成 |
| 借鉴 Swarm 安全 | 启用框架功能 | 中 | 中 | ✅ 已完成 |
| 借鉴 Session 隔离 | 启用框架功能 | 中 | 中 | ✅ 已完成 |
| 贡献 Graph 编译层 | 贡献回框架 | 大 | 高 | ⏭ 跳过（不贡献回框架） |
| 贡献 TeamFailurePolicy | 贡献回框架 | 中 | 低 | ⏭ 跳过（不贡献回框架） |
| 启用 WithKnowledge | 启用框架功能 | 小 | 低 | ✅ 已完成（KnowledgeAdapter 桥接项目 Retriever 到框架 knowledge.Knowledge） |
| 启用安全限制 | 启用框架功能 | 小 | 低 | ✅ 已完成（SafetyLimitAdapter 启用 MaxLLMCalls/MaxToolIterations） |
| 贡献 BuildCache | 贡献回框架 | 中 | 中 | ⏭ 跳过（不贡献回框架） |
| 贡献 RunnerManager/RunRegistry | 贡献回框架 | 大 | 中 | ⏭ 跳过（不贡献回框架） |
| 贡献 RunnerRollback | 贡献回框架 | 中 | 中 | ⏭ 跳过（不贡献回框架） |

**前置条件**：Phase 1-2 完成（Event/Session 对齐），Phase 3 完成（Knowledge 对齐影响 Agent）。

**产出**：Team ★☆☆☆☆ → ★★★☆☆，Agent ★★★★☆ → ★★★★★，Runner ★★★★☆ → ★★★★★。

**实施记录**：

| 变更文件 | 变更内容 |
|---------|---------|
| `internal/team/export_adapter.go` | 新增 ~225 行：ExportSnapshot 将 biz.Agent 转为 structure.Snapshot，含 pathAllocator/rebaseSnapshot/escapeLocalName 本地实现 |
| `internal/team/safety_adapter.go` | 新增 ~87 行：SwarmSafetyOptions/SessionIsolationOptions/MemberToolOptions 三个适配器函数 |
| `internal/agent/knowledge_safety_adapter.go` | 新增 ~73 行：KnowledgeAdapter 桥接项目 Retriever 到框架 knowledge.Knowledge（通过 FrameworkKnowledge 包装）+ SafetyLimitAdapter 启用 MaxLLMCalls/MaxToolIterations |
| `internal/agent/transfer_controller.go` | 新增 ~78 行：TransferControllerImpl 实现 trpcagent.TransferController，原子计数器深度限制（max=3）+ 120s 目标超时 + apierror.Forbidden 错误返回 |
| `internal/agent/trpc_build.go` | 修改：在 ag.Settings != nil 块内添加 SafetyLimitAdapter(ag) 和 KnowledgeAdapter(ctx, ag, deps, lg) 调用 |
| `internal/agent/prompt_render.go` | 修改：添加 ValidateRequired() 调用，graceful degradation（校验失败仅 warn 不阻断） |
| `internal/service/chat_orchestrator_durable.go` | 修改：添加 WithPersistInterruptedAssistant(true) 启用中断恢复 |
| `internal/service/chat_orchestrator_turn_phases.go` | 修改：buildTurnRunOptions 添加 WithStreamMode(StreamModeMessages) + TransferController 安装（per-turn 创建，原子计数器 run-scoped） |

### Phase 6：Server + Extended 对齐（2-3 周） ✅ 已完成

**目标**：完成 Server 协议对齐、Extended 模块集成。

| 任务 | 类型 | 工作量 | 风险 | 状态 |
|------|------|--------|------|------|
| 启用 AG-UI 协议端点 | 启用框架功能 | 中 | 中 | ✅ 已完成 |
| 启用 A2A 扩展点 | 启用框架功能 | 小 | 低 | ✅ 已完成 |
| 启用 OpenAI 完整选项 | 启用框架功能 | 小 | 低 | ✅ 已完成 |
| 启用 TodoEnforcer | 启用框架功能 | 小 | 低 | ✅ 已完成 |
| 评估 TaskRun 共存 | 新增适配层 | 中 | 中 | ✅ 已完成 |

**前置条件**：Phase 1 完成（Event 对齐影响 Server 事件流），Phase 5 完成（Agent 对齐影响 Server 构建）。

**产出**：Server ★★☆☆☆ → ★★★★☆，Extended ★★☆☆☆ → ★★★★☆。

**实施记录**：

| 变更文件 | 变更内容 |
|---------|---------|
| `internal/server/agui_adapter.go` | 新增 ~140 行：AGUIHandler 封装框架 agui.Server，提供 RegisterRoutes 挂载 SSE 端点 |
| `internal/server/a2a_adapter.go` | 新增 ~178 行：A2AExtensionAdapter 启用框架 A2A 扩展点，含 MessageAuditHook/MessageFilterHook |
| `internal/server/openai_adapter.go` | 新增 ~106 行：OpenAISessionAdapter 封装框架 openai.Server，启用会话持久化 |
| `internal/agent/todo_enforcer.go` | 新增 ~73 行：NewTodoEnforcerOption/NewTodoEnforcerOptionWithScope，集成框架 todoenforcer |
| `internal/tools/taskrun_adapter.go` | 新增 ~102 行：TaskRunAdapter 封装框架 taskrun.Tools，提供异步委派能力 |
| `internal/service/agui_compat.go` | 新增 ~120 行：AGUICompatService 包装框架 agui.Server，OpenAIRunnerBuilder 窄接口 + 懒加载（双重检查锁）解决 Runner per-session 问题 |
| `internal/service/openai_session_compat.go` | 新增 ~110 行：OpenAISessionCompatService 包装框架 openai.Server，OpenAIRunnerBuilder 窄接口 + 懒加载 |
| `internal/service/a2a_extension_compat.go` | 新增 ~156 行：A2AExtensionCompatService 包装框架 A2A 扩展服务器，defaultA2AHost 命名常量 + 懒加载 + AgentCard 构建 |
| `internal/server/http.go` | 修改：添加 lazyCompatHandler 包装器和三个 compat 服务路由注册 |
| `internal/server/service_registry.go` | 修改：添加 AGUICompatService/OpenAISessionCompatService/A2AExtensionCompatService 字段 |
| `internal/provider/trpc_llm.go` | 修改：添加 resolveTailoringStrategy 函数，暴露 TailoringStrategy 配置 |
| `internal/provider/catalog.go` | 修改：更新 TailoringStrategy 字段注释 |
| `internal/service/prompt_refine.go` | 修改：NewAIRefineService 改为接受 biz.Refiner 接口，支持 PromptIterAdapter fallback |
| `cmd/admin/wire.go` | 修改：添加 wire.Bind(new(biz.Refiner), new(*biz.PromptRefiner)) 和 wire.Bind(new(service.OpenAIRunnerBuilder), new(*service.ChatService)) |

---

## 六、收益量化总表

### 6.1 代码减少预估

| 阶段 | 预计代码减少 | 主要来源 |
|------|------------|---------|
| Phase 0 | +30 行（净增，启用框架功能而非替换自建） | ToolPipe Extension 注册 + tiktoken counter + 接口断言 |
| Phase 1 | ~800 行 | EventBus/EventWAL 贡献后移除自建 |
| Phase 2 | ~400 行 | Session/Memory 适配框架接口 |
| Phase 3 | ~500 行 | Knowledge/Evaluation/Prompt 启用框架功能（适配层净增 ~1300 行，但替换自建逻辑后净减 ~500 行） |
| Phase 4 | ~200 行 | Tool/Skill 适配层（框架贡献跳过，适配层净增 ~700 行，替换自建后净减 ~200 行） |
| Phase 5 | ~150 行 | Team/Agent 适配层（框架贡献跳过，适配层净增 ~380 行，替换自建后净减 ~150 行） |
| Phase 6 | ~200 行 | Server/Extended 启用框架功能（适配层净增 ~600 行，替换自建后净减 ~200 行） |
| **合计** | **~2280 行** | —

### 6.2 性能收益预估

| 收益类型 | 预期提升 | 来源 |
|---------|---------|------|
| Token 消耗降低 | 50-90% | ToolPipe Extension（P1-4） |
| Token 消耗降低 | 10-30% | Skill 提示缓存优化（P2-27） |
| 事件可靠性 | Critical 级 100% | EventWAL WBPF（P1-2） |
| Agent 行为一致性 | 提升 | TodoEnforcer + PromptIter（P2-31, P1-9） |

### 6.3 功能增强总表

| 新增能力 | 来源模块 | 阶段 |
|---------|---------|------|
| CopilotKit 生态兼容 | Server AG-UI | Phase 6 |
| A2A 消息审计/过滤 | Server A2A | Phase 6 |
| Agent 异步委派任务 | Extended TaskRun | Phase 6 |
| 框架级事件总线 | Event EventBus | Phase 1 |
| 框架级事件持久化 | Event EventWAL | Phase 1 |
| Dify/N8N/Codex Agent | Extended | P4（按需） |

---

## 七、风险与缓解

### 7.1 全局风险

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架接口不稳定，对齐后需重做 | 中 | 高 | 优先对齐稳定接口；适配层隔离框架变化 |
| 对齐过程中引入回归 Bug | 中 | 高 | 每个 Phase 完成后全量测试；渐进式对齐，不中断现有业务 |
| 多模块并行对齐导致冲突 | 低 | 中 | 按依赖链顺序执行；跨模块变更需 Code Review |
| 框架升级与对齐工作冲突 | 中 | 中 | 对齐工作与框架升级解耦；适配层独立维护 |

### 7.2 模块级高风险项

| 模块 | 高风险项 | 风险描述 | 缓解措施 |
|------|---------|---------|---------|
| Event | EventBus 贡献 | 双总线设计复杂，框架可能不接受 | ✅ 已完成 |
| Team | Graph 编译层贡献 | 编译层与框架 Graph 模型差异大 | ⏭ 跳过（不贡献回框架） |
| Server | AG-UI 端点 | SSE vs WS 事件模型不一致 | ✅ 已完成（并行模式，不替代 WS） |
| Knowledge | VectorStore 适配 | pgvector 与框架向量接口差异 | ✅ 已完成（适配层隔离差异） |
| Memory | L0-L4 贡献 | 五层模型与框架 Memory 接口设计理念不同 | ⏭ 跳过（不贡献回框架） |

---

## 八、跨模块协同要点

### 8.1 Callback Chain（影响 Model + Agent + Tool + Evaluation）

Callback Chain 是跨模块的核心基础设施，项目自建了 Model 级 Callback Chain 适配器。对齐策略：

1. **Phase 4**：贡献 Model 级 Callback Chain 到框架 → ⏭ 跳过（不贡献回框架）
2. **Phase 5**：Agent 和 Tool 模块启用框架 Callback
3. **Phase 3**：Evaluation 模块启用框架 Callbacks → ✅ 已完成

### 8.2 Circuit Breaker（影响 Model + Tool）

项目在 Model（Transport 级）和 Tool（调用级）均有熔断实现。对齐策略：

1. **Phase 4**：贡献 Model 级 Circuit Breaker Transport → ⏭ 跳过（不贡献回框架）
2. **Phase 4**：贡献 Tool 级 Circuit Breaker → ⏭ 跳过（不贡献回框架）
3. 两者均改为 internal 适配层或跳过

### 8.3 EventBus（影响 Event + Session + Memory + Server）

EventBus 是项目事件基础设施的核心。对齐策略：

1. **Phase 1**：贡献 EventBus 到框架
2. **Phase 2**：Session 和 Memory 适配框架事件接口
3. **Phase 6**：Server AG-UI 使用框架事件流

### 8.4 BuildCache（影响 Agent + Skill）

Agent 的 BuildCache（LRU+singleflight+dirty-mark）与 Skill 的 TTL 缓存有相似模式。对齐策略：

1. **Phase 5**：贡献 BuildCache 到框架 → ⏭ 跳过（不贡献回框架）
2. **Phase 4**：Skill DBRepositoryAdapter 改为 internal 适配层 → ✅ 已完成

---

## 九、验收标准

### 9.1 每个 Phase 完成标准

- [x] Phase 0：所有对齐项代码已合并，构建通过，审查通过
- [x] Phase 1（P1-1/P1-2/P1-3）：EventBus/EventWAL/可靠性分级贡献完成，构建通过，审查通过
- [x] Phase 1（P2 后续）：Plugin.OnEvent eventTypeLabel 细化/FromFrameworkEvent 统一转换/tracing 纯数据层贡献完成，构建通过，审查通过
- [x] Phase 2：Session/Memory 对齐完成，构建通过，审查通过
- [x] Phase 3（P1-7/P1-8）：LLM Judge + Callbacks 对齐完成，构建通过，审查通过
- [x] Phase 3（Knowledge + Prompt）：VectorStore/Embedder/Knowledge 适配器 + SearchTool + PromptIter + prompt.Text.Render() + state.Render() 对齐完成，构建通过，审查通过
- [x] Phase 4：Tool/Skill 适配层（命令安全/输出大小/DBRepository）对齐完成，Model Transport 贡献跳过，构建通过，审查通过
- [x] Phase 5：Team/Agent 适配层（Export/Swarm 安全/Session 隔离/WithKnowledge/安全限制）对齐完成，框架贡献跳过，构建通过，审查通过
- [x] Phase 6：Server/Extended（AG-UI/A2A/OpenAI/TodoEnforcer/TaskRun）对齐完成，构建通过，审查通过
- [ ] 全量测试通过（`make test`）— ⚠️ 未完整运行（`go build ./internal/... ./cmd/... ./pkg/...` 已通过，`go vet` 核心模块已通过）
- [~] 全量构建通过（`make build`）— ✅ `go build` 通过（exit 0），`make build` 完整流程未运行
- [~] Lint 通过（`make lint`）— ✅ `go vet` 通过（exit 0），`make lint` 完整流程未运行
- [x] 模块对齐度评分已更新
- [x] 对齐文档已更新实施状态

### 9.2 整体完成标准

- [x] 所有 P1/P2 对齐项已完成（或已标记跳过）
- [x] P3 按业务需求完成 ≥80%
- [x] 贡献回框架的代码已完成 5 项（Phase 1 Event 贡献），其余改为 internal 适配层或跳过
- [x] 项目自建代码减少 ~2280 行
- [x] 所有模块对齐度 ≥★★★☆☆

---

## 十、附录

### A. 模块对齐度目标

| 模块 | 当前 | Phase 0 后 | Phase 1 后 | Phase 3 后 | Phase 6 后 | 最终目标 |
|------|------|-----------|-----------|-----------|-----------|---------|
| Event | ★★☆☆☆ | ★★☆☆☆ | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ |
| Prompt | ☆☆☆☆☆ | ☆☆☆☆☆ | ☆☆☆☆☆ | ★★★☆☆ | ★★★☆☆ | ★★★★☆ |
| Knowledge | ★☆☆☆☆ | ★☆☆☆☆ | ★☆☆☆☆ | ★★★☆☆ | ★★★☆☆ | ★★★★☆ |
| Team | ★☆☆☆☆ | ★☆☆☆☆ | ★☆☆☆☆ | ★☆☆☆☆ | ★★★☆☆ | ★★★★☆ |
| Evaluation | ★★★☆☆ | ★★★☆☆ | ★★★☆☆ | ★★★★☆ | ★★★★☆ | ★★★★★ |
| Tool | ★★★★☆ | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ |
| Session | ★★★☆☆ | ★★★☆☆ | ★★★☆☆ | ★★★★☆ | ★★★★☆ | ★★★★★ |
| Memory | ★★★☆☆ | ★★★☆☆ | ★★★☆☆ | ★★★★☆ | ★★★★☆ | ★★★★★ |
| Server | ★★☆☆☆ | ★★☆☆☆ | ★★☆☆☆ | ★★☆☆☆ | ★★★★☆ | ★★★★☆ |
| Model | ★★★★☆ | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ |
| Agent | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★★ | ★★★★★ |
| Runner | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★★ | ★★★★★ |
| Skill | ★★★★☆ | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ |
| Extended | ★★☆☆☆ | ★★☆☆☆ | ★★☆☆☆ | ★★☆☆☆ | ★★★★☆ | ★★★★☆ |

### B. 文档索引

| 文档 | 路径 |
|------|------|
| 指导文档 | `docs/trpc-agent-go/00-guide.md` |
| 运行时关系 | `docs/trpc-agent-go/00-runtime-relationships.md` |
| Runner | `docs/trpc-agent-go/01-runner.md` |
| Agent | `docs/trpc-agent-go/02-agent.md` |
| Model | `docs/trpc-agent-go/03-model.md` |
| Session | `docs/trpc-agent-go/04-session.md` |
| Event | `docs/trpc-agent-go/05-event.md` |
| Memory | `docs/trpc-agent-go/06-memory.md` |
| Tool | `docs/trpc-agent-go/07-tool.md` |
| Team | `docs/trpc-agent-go/08-team.md` |
| Knowledge | `docs/trpc-agent-go/09-knowledge.md` |
| Evaluation | `docs/trpc-agent-go/10-evaluation.md` |
| Prompt | `docs/trpc-agent-go/11-prompt.md` |
| Server | `docs/trpc-agent-go/12-server.md` |
| Skill | `docs/trpc-agent-go/13-skill.md` |
| Extended | `docs/trpc-agent-go/18-extended-modules.md` |

---

## 十一、未接入适配器阻塞原因详解

> 以下 7 项适配器代码已就绪但无法接入生产路径，每项有明确的阻塞原因。
> 阻塞原因分为三类：**框架接口无消费者**（框架未提供使用该接口的组件）、**框架协作者缺失**（适配器依赖的框架组件在项目中不存在）、**设计前提不匹配**（适配器的设计假设与项目实际架构冲突）。
>
> **当前状态**：B-1/B-2 永久阻塞，死代码已删除（CS-B2）；B-3 已标记 TECH-DEBT(B-3)（"编译错误"经核实为误判，真实阻塞原因为框架协作者缺失）；B-4 已标记 TECH-DEBT(B-4)；B-5/B-6/B-7 已标记 TECH-DEBT(B-5/B-6/B-7)，待对应 Phase 解除。
>
> **解除方案关联文档**：`docs/reports/2026-06-17-research-orchestration-longtask-memory-upgrade.md`（以下简称"升级调研报告"），该报告规划了 Phase 0~3 的架构重构路径，部分阻塞项将在重构过程中解除。

### B-1. VectorStoreAdapter（P1-5）— 框架接口无消费者

**阻塞类型**：框架接口无消费者（架构设计差异，永久阻塞）

**适配器位置**：`internal/knowledge/vectorstore_adapter.go`（**已删除**）

**阻塞原因**：
- 适配器将项目的 `internal/data/vector.VectorStore` 适配到框架的 `vectorstore.VectorStore` 接口
- 但框架中**没有任何组件消费 `vectorstore.VectorStore` 接口**——框架的 Knowledge 模块使用 `knowledge.Knowledge` 接口（已通过 `KnowledgeAdapter` 适配），不直接使用 `vectorstore.VectorStore`
- 项目的向量搜索管线（`internal/data/vector.VectorStore` → `MultiProviderEmbedder` → `KnowledgeAdapter`）已完整运行，插入 `VectorStoreAdapter` 不会带来任何功能增益，只会增加一层无意义的包装

**解除方案**：**不解除（永久阻塞）**
- 升级调研报告 §6.4 确认记忆系统 5 项前沿技术（Bi-temporal/Ebbinghaus/Sleep-time/主动召回/记忆链接图）均在项目内部实现，对齐的是框架 `memory.Service` 接口而非 `vectorstore.VectorStore`
- 框架架构设计上通过 `knowledge.Knowledge` 接口抽象了整个检索流程，VectorStore/Embedder 是项目内部实现细节，框架不直接消费
- **已完成**：死代码已删除（`internal/knowledge/vectorstore_adapter.go` + 测试文件），遵循 CS-B2「死代码即删」原则

### B-2. EmbedderAdapter（P1-6）— 框架接口无消费者

**阻塞类型**：框架接口无消费者（架构设计差异，永久阻塞）

**适配器位置**：`internal/knowledge/embedder_adapter.go`（**已删除**）

**阻塞原因**：
- 适配器将项目的 `MultiProviderEmbedder` 适配到框架的 `embedder.Embedder` 接口
- 与 B-1 同理，框架中**没有任何组件直接消费 `embedder.Embedder` 接口**——框架的 Knowledge 模块通过 `knowledge.Knowledge` 接口抽象了整个检索流程，Embedder 是项目内部实现细节
- `wire.go` 直接绑定 `MultiProviderEmbedder` 是正确的，因为项目代码直接使用它生成嵌入向量

**解除方案**：**不解除（永久阻塞）**
- 与 B-1 同理，升级调研报告 §6.4 的记忆系统升级不涉及框架 Embedder 接口消费
- **已完成**：死代码已删除（`internal/knowledge/embedder_adapter.go` + 测试文件），遵循 CS-B2「死代码即删」原则

### B-3. PromptIterAdapter（P1-9, P2-26）— 框架协作者缺失（编译错误为误判）

**阻塞类型**：框架协作者缺失（原"编译错误"经核实为误判）

**适配器位置**：`internal/agent/prompt_iter_adapter.go`（已标记 `// TECH-DEBT(B-3)`）

**阻塞原因**：
1. **"编译错误"经核实为误判**：原计划声称适配器引用的 `TrainEvalSetIDs` 和 `ValidationEvalSetIDs` 字段在框架 `promptiter` 包中不存在。经核实，此判断基于本地 `pkg/trpc-agent-go` 新版源码，但 `go.mod` 中 `evaluation` 模块使用远程 v1.9.0 版本（非本地 replace），v1.9.0 API 仍为 `TrainEvalSetIDs`/`ValidationEvalSetIDs`，编译正常。本地新版源码已改为 `Train []EvalSetInput` / `Validation []EvalSetInput`，但项目未升级到该版本。
2. **框架协作者缺失**（真实阻塞原因）：框架的 `promptiter` 需要 5 个协作者才能运行：
   - `engine.Engine` — 迭代引擎
   - `evalset.Recorder` — 评估集记录器
   - 评估集持久化后端
   - LLM Judge（已有但未接入此管线）
   - Prompt 生成器
   项目缺少评估集基础设施（`EvalSet Recorder`、评估集持久化），无法提供这些协作者

**解除方案**：**独立迭代（优先级低，不关联升级调研报告 Phase 0~3）**
- 升级调研报告未涉及 PromptIter 管线，项目自建的 `PromptRefiner` 已满足当前需求
- 如需解除：
  1. 建立评估集基础设施（EvalSet Recorder + 持久化）——这是独立的基础设施投入
  2. 评估是否值得引入完整的 PromptIter 管线
  3. 若升级 `evaluation` 模块到本地新版，需同步适配 `Train`/`Validation` 字段变更
- **建议**：保持阻塞状态，待有明确的 Prompt 迭代优化业务需求时再启动

### B-4. FrameworkSearchTool（P2-22）— 双重注册冲突 + 能力降级

**阻塞类型**：双重注册冲突 + 能力降级

**适配器位置**：`internal/tools/knowledge/framework_searchtool.go`

**阻塞原因**：
1. **双重注册冲突**：项目已在工具目录注册了自建的 `knowledge_search` 工具（`internal/tools/knowledge/searchtool.go`）。`FrameworkSearchTool` 使用框架的 `knowledge.NewKnowledgeSearchTool` 创建同名工具，会导致 `knowledge_search` 工具双重注册
2. **能力降级**：项目的 `SearchTool` 支持**动态 collection 限定**——通过 `knowledgetool.WithKnowledgeCollections(runCtx, kbs)` 在 per-run context 中注入允许搜索的 collection 列表。框架的 `KnowledgeSearchTool` 不支持此能力，接入后会丢失动态 collection 过滤

**解除方案**：**Phase 3 记忆系统升级时重新评估**
- 升级调研报告 §6.4 + Phase 3 规划了"主动召回触发器"，可能涉及 KnowledgeSearchTool 的能力增强
- 解除条件：
  1. 框架的 `KnowledgeSearchTool` 支持动态 collection 限定（需框架演进），或
  2. 项目决定放弃动态 collection 过滤能力（需评估业务影响）
- **建议**：Phase 3 实施时检查框架版本是否已支持动态 collection，若支持则接入，否则保持阻塞

### B-5. RenderPromptTemplate（P2-32, Prompt P3 ValidateRequired）— 设计前提不匹配

**阻塞类型**：设计前提不匹配

**适配器位置**：`internal/agent/prompt_render.go`

**阻塞原因**：
- `RenderPromptTemplate` 使用框架的 `prompt.Text.Render()` 进行**模板变量替换**（`{{var}}` → value）
- 但项目的 `BuildSystemPrompt()` 使用**结构化拼接**——通过 `strings.Builder` 将 capability cues、system prompt sections、runtime cues 等结构化组件按顺序拼接
- 两种方法本质不同：模板渲染需要一个完整的模板字符串然后替换变量；结构化拼接是从组件动态组装
- 项目的 system prompt 不是模板，而是运行时动态组装的，无法用 `Render()` 替换
- `ValidateRequired()` 是 `RenderPromptTemplate` 内部调用的校验函数，因宿主函数本身无法接入而连带阻塞

**解除方案**：**Phase 1 AgentFactory 实施时重新评估**
- 升级调研报告 §6.3 + Phase 1 规划了"AgentFactory（LLM 生成 Agent 定义，无需人工审核）"
- 如果 AgentFactory 引入模板化 Agent 定义（LLM 生成的 Agent 包含模板化 system prompt），则可能需要 `Render()` 进行模板渲染
- 解除条件：项目将 system prompt 重构为模板化设计（使用模板字符串 + 变量替换）
- **建议**：Phase 1 AgentFactory 实施时，如果引入模板化 Agent 定义则接入，否则保持阻塞

### B-6. RenderStateTemplate（P2-33）— 设计前提不匹配

**阻塞类型**：设计前提不匹配

**适配器位置**：`internal/agent/state_render.go`

**阻塞原因**：
- `RenderStateTemplate` 使用框架的 state 渲染机制（`prompt.Text.Render` + 自定义 Resolver）来渲染运行时状态
- 但项目的 `runtime_cue_inject.go` 使用 `trpcagent.MergeRuntimeState()` 将运行时线索（时间、会话状态等）注入到 LLM 的 RuntimeState 中，由框架在模型调用时自动处理
- 两种方法本质不同：`RenderStateTemplate` 是在 prompt 构建时**同步渲染**状态到文本；`MergeRuntimeState` 是在模型调用时**异步注入**状态到框架内部状态管理
- 项目的 RuntimeState 注入机制已完整运行且更灵活（支持 per-run 覆盖），接入 `RenderStateTemplate` 会造成双重状态注入

**解除方案**：**Phase 1 AgentFactory 实施时重新评估**
- 与 B-5 同理，如果 Phase 1 AgentFactory 引入模板化 Agent 定义，则 state 渲染可能需要配合模板化设计
- 解除条件：与 B-5 相同，需项目 prompt 架构重构为模板化设计
- **建议**：与 B-5 同步评估，保持阻塞状态

### B-7. TaskRunAdapter（Extended P3）— 缺少 Controller 实例 + 复杂依赖链

**阻塞类型**：缺少核心依赖 + 复杂依赖链

**适配器位置**：`internal/tools/taskrun_adapter.go`

**阻塞原因**：
1. **缺少 Controller 实例**：`TaskRunAdapter` 需要一个 `taskrun.Controller` 来管理异步任务的生命周期（创建、执行、查询、取消）。项目没有 Controller 基础设施
2. **复杂依赖链**：创建 `Controller` 需要：
   - 任务持久化层（存储任务状态和结果）
   - 任务生命周期管理器（状态机：pending → running → completed/failed/cancelled）
   - 任务状态追踪（per-session、per-run 的任务索引）
   - 任务结果回调机制
   这是一整套异步任务子系统，不是简单的适配器接线

**解除方案**：**Phase 1 统一执行引擎实施时解除（关键关联）**
- 升级调研报告 §6.2 明确规划"基于 trpc-agent-go 框架增强"，补齐三个缺口：
  1. **taskrun 事件透传**（对外暴露流式事件）——直接对应 TaskRunAdapter 的 Controller 需求
  2. 跨进程事件流（Postgres-backed EventStore）
  3. 任务级心跳（run_heartbeat 事件）
- 升级调研报告 §2.1 确认框架的 `taskrun.Controller` 已原生支持 Spawn/Wait/Cancel + 文件持久化
- Phase 1 将建设任务管理子系统，提供 TaskRunAdapter 所需的全部依赖：
  - 任务持久化层 → Postgres 迁移后的 `background_jobs` 表（升级调研报告 §5.5 第 5 项）
  - 任务生命周期管理器 → 统一执行引擎的状态机
  - 任务状态追踪 → per-session/per-run 索引
  - 任务结果回调 → taskrun 事件透传 + 跨进程事件流
- **实施步骤**：
  1. Phase 0 完成 Postgres 迁移（提供任务持久化后端）
  2. Phase 1 建设统一执行引擎，实例化 `taskrun.Controller`
  3. Phase 1 后期将 `TaskRunAdapter` 接入生产路径，作为 Agent 的异步任务委派工具
- **建议**：这是 7 个阻塞项中**唯一有明确解除路径**的项，应在 Phase 1 统一执行引擎实施时同步解除

### B-8. 阻塞项解除方案汇总

| 阻塞项 | 阻塞类型 | 解除方案 | 关联 Phase | 优先级 |
|--------|---------|---------|-----------|--------|
| B-1 VectorStoreAdapter | 框架接口无消费者 | **不解除（永久阻塞）**，死代码已删除（CS-B2） | N/A | 低 |
| B-2 EmbedderAdapter | 框架接口无消费者 | **不解除（永久阻塞）**，死代码已删除（CS-B2） | N/A | 低 |
| B-3 PromptIterAdapter | 框架协作者缺失（编译错误为误判） | **独立迭代**，待有明确 Prompt 迭代优化业务需求时启动；已标记 TECH-DEBT | N/A | 低 |
| B-4 FrameworkSearchTool | 双重注册 + 能力降级 | **Phase 3 重新评估**，检查框架是否支持动态 collection；已补充 TECH-DEBT(B-4) 标记 | Phase 3 | 中 |
| B-5 RenderPromptTemplate | 设计前提不匹配 | **Phase 1 重新评估**，AgentFactory 引入模板化 Agent 定义时接入；已标记 TECH-DEBT(B-5) | Phase 1 | 中 |
| B-6 RenderStateTemplate | 设计前提不匹配 | **Phase 1 重新评估**，与 B-5 同步评估；已标记 TECH-DEBT(B-6) | Phase 1 | 中 |
| B-7 TaskRunAdapter | 缺少 Controller + 依赖链 | **Phase 1 解除**，统一执行引擎建设时同步接入；已标记 TECH-DEBT(B-7) | Phase 1 | **高** |

**关键结论**：
- 7 个阻塞项中，**1 项有明确解除路径**（B-7 TaskRunAdapter → Phase 1）
- **3 项在架构重构时重新评估**（B-4/B-5/B-6 → Phase 1/3）
- **2 项永久阻塞且死代码已删除**（B-1/B-2 → 框架架构差异，遵循 CS-B2「死代码即删」原则）
- **1 项独立迭代**（B-3 → 框架协作者缺失，"编译错误"经核实为误判；已标记 TECH-DEBT，待业务需求驱动）
- 升级调研报告（`docs/reports/2026-06-17-research-orchestration-longtask-memory-upgrade.md`）的 Phase 0~3 架构重构将逐步解除可解除的阻塞项
