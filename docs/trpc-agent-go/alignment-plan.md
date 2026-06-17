# trpc-agent-go 框架对齐总计划

> 基于 17 份模块对齐分析文档综合制定，覆盖全部模块的对齐项、依赖关系、阶段划分和收益量化。

---

## 一、全局概览

### 1.1 模块对齐度矩阵

| 模块 | 对齐度 | P1 项 | P2 项 | P3 项 | P4 项 | 总对齐项 |
|------|--------|-------|-------|-------|-------|---------|
| Event | ★★☆☆☆→★★★★★ | 3✅ | 2✅ | 2 | 0 | 7 |
| Prompt | ☆☆☆☆☆ | 1 | 2 | 1 | 0 | 4 |
| Knowledge | ★☆☆☆☆ | 2 | 2 | 6 | 0 | 10 |
| Team | ★☆☆☆☆ | 0 | 3 | 5 | 0 | 8 |
| Evaluation | ★★★☆☆→★★★★☆ | 2✅ | 2 | 3 | 0 | 7 |
| Tool | ★★★★☆→★★★★★ | 1✅ | 2 | 2 | 0 | 5 |
| Session | ★★★☆☆→★★★★☆ | 0 | 4(4✅) | 4 | 0 | 8 |
| Memory | ★★★☆☆→★★★★☆ | 0 | 2 | 6 | 0 | 8 |
| Server | ★★☆☆☆ | 0 | 2 | 3 | 0 | 5 |
| Model | ★★★★☆→★★★★★ | 0 | 3(1✅) | 7 | 0 | 10 |
| Agent | ★★★★☆ | 0 | 0 | 6 | 2 | 8 |
| Runner | ★★★★☆ | 0 | 0 | 6 | 0 | 6 |
| Skill | ★★★★☆→★★★★★ | 0 | 2 | 5(1✅) | 0 | 7 |
| Extended | ★★☆☆☆ | 1 | 1 | 1 | 3 | 6 |
| **合计** | — | **9(6✅)** | **27(3✅)** | **54(1✅)** | **5** | **99(10✅)** |

### 1.2 对齐类型分布

| 类型 | 数量 | 占比 |
|------|------|------|
| 启用框架功能 | 39 | 39% |
| 贡献回框架 | 30 | 30% |
| 替换自建实现 | 16 | 16% |
| 新增适配层 | 14 | 14% |

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
| P1-5 | Knowledge | 实现 VectorStore 适配器 | 新增适配层 | 对接框架 Knowledge 接口，统一向量搜索 |
| P1-6 | Knowledge | 实现 Embedder 适配器 | 新增适配层 | 对接框架 Embedder 接口，统一嵌入生成 |
| P1-7 | Evaluation | 启用框架 LLM Judge | 启用框架功能 | 替换自建 LLM-as-Judge，减少维护 | ✅ 已完成 |
| P1-8 | Evaluation | 启用 Callbacks | 启用框架功能 | 评估流程获得框架 Callback 能力 | ✅ 已完成 |
| P1-9 | Prompt | 启用 PromptIter 替换 PromptRefiner | 启用框架功能 | 替换自建 PromptRefiner，获得框架迭代优化能力 |

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
| P2-7 | Memory | L2/L3 适配 memory.Service 接口 | 新增适配层 | P1-1 EventBus | 记忆层对接框架接口 | ✅ 已验证（已有实现） |
| P2-8 | Memory | Extractor Chain 适配 MemoryExtractor | 新增适配层 | P2-7 | 提取器链对接框架接口 | ✅ 已完成 |
| P2-9 | Tool | DeferredCallableTool 实现 DeferredTool | 替换自建实现 | P1-4 ToolPipe | 延迟加载工具对接框架接口 |
| P2-10 | Tool | 贡献 Circuit Breaker | 贡献回框架 | 无 | 框架获得工具级熔断能力 |
| P2-11 | Tool | 贡献 Confirmation Gate | 贡献回框架 | 无 | 框架获得工具确认门控能力 |
| P2-12 | Tool | ToolResultGate + ToolPipe 协调 | 新增适配层 | P1-4 ToolPipe | 大结果截断与过滤协调 |

### 3.2 独立 P2 项（无前置依赖）

| # | 模块 | 对齐项 | 类型 | 核心收益 |
|---|------|--------|------|---------|
| P2-13 | Model | 启用 tiktoken counter | 启用框架功能 | 精确 token 计数，替代估算 | ✅ 已完成 |
| P2-14 | Model | 贡献 Rate Limit Transport | 贡献回框架 | 框架获得模型级限流 |
| P2-15 | Model | 贡献 Retry Transport | 贡献回框架 | 框架获得模型级重试 |
| P2-16 | Model | 贡献 Circuit Breaker Transport | 贡献回框架 | 框架获得模型级熔断 |
| P2-17 | Model | 贡献 Callback Chain | 贡献回框架 | 框架获得模型调用链能力 |
| P2-18 | Team | 适配 Export() 结构 | 新增适配层 | Team 定义与框架导出格式对齐 |
| P2-19 | Team | 借鉴 Swarm 安全机制 | 启用框架功能 | 自适应模式获得安全保护 |
| P2-20 | Team | 借鉴 Session 隔离 | 启用框架功能 | Team 成员间 Session 隔离 |
| P2-21 | Knowledge | 实现 Knowledge 接口 | 新增适配层 | 完整对接框架 Knowledge 体系 |
| P2-22 | Knowledge | 使用框架 SearchTool | 启用框架功能 | 替换自建 knowledge_search 工具 |
| P2-23 | Knowledge | 贡献 HybridRetriever | 贡献回框架 | 框架获得混合检索能力 |
| P2-24 | Knowledge | 贡献 AdaptiveRouter | 贡献回框架 | 框架获得自适应路由能力 |
| P2-25 | Evaluation | 启用 EvalSet Recorder | 启用框架功能 | 评估集持久化走框架 |
| P2-26 | Evaluation | 启用 PromptIter | 启用框架功能 | 评估流程获得 Prompt 迭代能力 |
| P2-27 | Skill | 启用提示缓存优化 | 启用框架功能 | Token 消耗减少 10-30% |
| P2-28 | Skill | 贡献 DBRepositoryAdapter | 贡献回框架 | 框架获得 DB 后端 Skill 仓库 |
| P2-29 | Server | 启用 AG-UI 协议端点 | 启用框架功能 | CopilotKit 生态兼容 |
| P2-30 | Server | 启用 A2A 扩展点 | 启用框架功能 | A2A 消息审计/过滤/错误定制 |
| P2-31 | Extended | 启用 TodoEnforcer 扩展 | 启用框架功能 | Agent 行为一致性提升 |
| P2-32 | Prompt | 启用 prompt.Text.Render() | 启用框架功能 | 替换硬拼接，标准化 Prompt 组装 |
| P2-33 | Prompt | 启用 state.Render() | 启用框架功能 | 替换手动状态注入 |

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
| Agent | 启用 WithKnowledge | Agent 获得框架知识注入 |
| Agent | 启用安全限制 | Agent 获得框架安全保护 |
| Agent | 启用时间注入 | Agent 获得框架时间感知 |
| Agent | 评估 ActivatableToolSets vs DeferredManager | 工具集管理策略统一 |
| Agent | 贡献 BuildCache | 框架获得 LRU+singleflight 缓存 |
| Agent | 贡献 AgentFactory 增强 | 框架获得 Agent 工厂增强 |
| Runner | 评估 WithPersistInterruptedAssistant | 中断恢复能力 |
| Runner | 评估 WithDetachedCancel | 取消机制增强 |
| Runner | 评估 WithStreamMode | 流式模式标准化 |
| Runner | 贡献 RunnerManager/RunRegistry | 框架获得运行管理能力 |
| Runner | 贡献 RunnerRollback | 框架获得回滚能力 |

#### 协同包 B：Session/Memory 深度对齐

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Session | 评估 WindowService | 框架窗口管理替代自建 |
| Session | 贡献 Session 状态机 | 框架获得 5 状态显式状态机 |
| Session | 贡献 SessionLockManager | 框架获得并发安全 |
| Session | 贡献多级压缩管线 | 框架获得 4 级压缩能力 |
| Memory | 贡献 L0-L4 分层抽象 | 框架获得五层记忆模型 |
| Memory | 贡献 PII Scanner | 框架获得隐私检测 |
| Memory | 贡献 MemoryInject Plugin | 框架获得记忆注入 |
| Memory | 贡献 PriorityQueue | 框架获得优先级队列 |
| Memory | 贡献 Audit Hook | 框架获得策略审计 |
| Memory | 评估 Auto 模式 | 框架自动记忆管理 |

#### 协同包 C：Team 编排增强

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Team | 贡献 sequential/parallel/critic_loop 模式 | 框架获得 3 种编排模式 |
| Team | 贡献 Graph 编译层 | 框架获得图编译能力 |
| Team | 贡献 TeamFailurePolicy | 框架获得失败策略 |
| Team | 贡献 HITL 支持 | 框架获得人机交互 |
| Team | 评估 Team 实现 agent.Agent | Team 作为 Agent 嵌套 |

#### 协同包 D：Model 生态增强

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Model | 暴露 TailoringStrategy | Token 裁剪策略可配置 |
| Model | 暴露 TokenTailoringConfig | 裁剪参数可配置 |
| Model | 贡献 Metrics Model | 框架获得模型指标 |
| Model | 贡献 ModelSelector 策略 | 框架获得 5 种选择策略 |

#### 协同包 E：Skill/Knowledge 深度对齐

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Skill | 启用命令安全限制 | Skill 运行安全增强 |
| Skill | 启用输出大小限制 | 防止上下文窗口溢出 |
| Skill | FSRepository 显式实现接口 | 接口合规性 | ✅ 已完成 |
| Skill | 贡献 artifactSavingExecutor | 框架获得装饰器保存 |
| Skill | 评估交互式执行工具 | 支持长时间交互式技能 |
| Knowledge | 贡献 QueryRewriter 多策略 | 框架获得查询改写 |
| Knowledge | 贡献 FederatedRetriever | 框架获得联邦检索 |
| Knowledge | 贡献 RetrievalEvaluator | 框架获得检索评估 |
| Knowledge | 贡献 Collection 抽象 | 框架获得集合管理 |

#### 协同包 F：Server/Extended 增强

| 模块 | 对齐项 | 协同收益 |
|------|--------|---------|
| Server | 启用 OpenAI 完整选项 | OpenAI 会话持久化 |
| Server | 贡献 WebSocket ServiceFactory | 框架获得 WS 传输 |
| Server | 贡献多租户 A2A 路由器 | 框架获得多 Agent 路由 |
| Extended | 评估 TaskRun 共存 | Agent 获得异步委派能力 |
| Runner | 删除透传包装器 | 代码简化 |
| Event | Graph 事件用框架 EventEmitter | 事件源统一 |
| Evaluation | 贡献 SQLite 后端 | 框架获得评估持久化 |
| Evaluation | 贡献脚本模拟器 | 框架获得模拟评估 |
| Evaluation | 贡献 AfterTurn 触发器 | 框架获得自动评估 |
| Prompt | 启用 ValidateRequired() | Prompt 必填校验 |
| Agent | 评估 structure.Exporter | 结构化导出 |
| Agent | 评估 TransferController | 传输控制 |

### 4.2 P4 项（保持现状）

| 模块 | 对齐项 | 说明 |
|------|--------|------|
| Agent | 评估 structure.Exporter | 当前无业务需求 |
| Agent | 评估 TransferController | 当前无业务需求 |
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

### Phase 1：Event 对齐（2-3 周） 🔄 进行中

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
| 启用 WithKnowledge | 启用框架功能 | 小 | 低 | ✅ 已完成（TECH-DEBT：当前返回 nil，待迁移） |
| 启用安全限制 | 启用框架功能 | 小 | 低 | ✅ 已完成（TECH-DEBT：当前返回 nil，待字段添加） |
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
| `internal/agent/knowledge_safety_adapter.go` | 新增 ~67 行：KnowledgeAdapter/SafetyLimitAdapter/FrameworkKnowledge，TECH-DEBT 标注待激活 |

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

---

## 六、收益量化总表

### 6.1 代码减少预估

| 阶段 | 预计代码减少 | 主要来源 |
|------|------------|---------|
| Phase 0 | +30 行（净增，启用框架功能而非替换自建） | ToolPipe Extension 注册 + tiktoken counter + 接口断言 |
| Phase 1 | ~800 行 | EventBus/EventWAL 贡献后移除自建 |
| Phase 2 | ~400 行 | Session/Memory 适配框架接口 |
| Phase 3 | ~600 行 | Knowledge/Evaluation/Prompt 启用框架功能 |
| Phase 4 | ~500 行 | Model Transport/Tool/Skill 贡献 |
| Phase 5 | ~1000 行 | Team/Agent/Runner 贡献 |
| Phase 6 | ~300 行 | Server/Extended 启用框架功能 |
| Phase 7 | ~2000 行 | 深度贡献后移除自建代码 |
| **合计** | **~5650 行** | — |

### 6.2 性能收益预估

| 收益类型 | 预期提升 | 来源 |
|---------|---------|------|
| Token 消耗降低 | 50-90% | ToolPipe Extension（P1-4） |
| Token 消耗降低 | 10-30% | Skill 提示缓存优化（P2-27） |
| 事件可靠性 | Critical 级 100% | EventWAL WBPF（P1-2） |
| 模型调用可靠性 | 提升 | Rate Limit + Retry + CB Transport（P2-14/15/16） |
| Agent 行为一致性 | 提升 | TodoEnforcer + PromptIter（P2-31, P1-9） |

### 6.3 功能增强总表

| 新增能力 | 来源模块 | 阶段 |
|---------|---------|------|
| CopilotKit 生态兼容 | Server AG-UI | Phase 6 |
| A2A 消息审计/过滤 | Server A2A | Phase 6 |
| Agent 异步委派任务 | Extended TaskRun | Phase 6 |
| 交互式技能执行 | Skill skill_exec | Phase 7 |
| Dify/N8N/Codex Agent | Extended | P4（按需） |
| 框架级事件总线 | Event EventBus | Phase 1 |
| 框架级事件持久化 | Event EventWAL | Phase 1 |
| 框架级混合检索 | Knowledge HybridRetriever | Phase 7 |
| 框架级五层记忆 | Memory L0-L4 | Phase 7 |
| 框架级图编译 | Team Graph | Phase 7 |

---

## 七、风险与缓解

### 7.1 全局风险

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 框架接口不稳定，对齐后需重做 | 中 | 高 | 优先对齐稳定接口；贡献回框架的代码由框架维护者审核 |
| 贡献回框架被拒绝 | 中 | 中 | 保持自建实现作为回退；先在项目内验证再贡献 |
| 对齐过程中引入回归 Bug | 中 | 高 | 每个 Phase 完成后全量测试；渐进式对齐，不中断现有业务 |
| 多模块并行对齐导致冲突 | 低 | 中 | 按依赖链顺序执行；跨模块变更需 Code Review |
| 框架升级与对齐工作冲突 | 中 | 中 | 对齐工作与框架升级解耦；贡献回框架的代码独立提交 |

### 7.2 模块级高风险项

| 模块 | 高风险项 | 风险描述 | 缓解措施 |
|------|---------|---------|---------|
| Event | EventBus 贡献 | 双总线设计复杂，框架可能不接受 | 先在项目内重构为框架可接受的形式 |
| Team | Graph 编译层贡献 | 编译层与框架 Graph 模型差异大 | 先适配 Export()，逐步贡献 |
| Server | AG-UI 端点 | SSE vs WS 事件模型不一致 | 并行模式，不替代 WS |
| Knowledge | VectorStore 适配 | pgvector 与框架向量接口差异 | 定义适配层隔离差异 |
| Memory | L0-L4 贡献 | 五层模型与框架 Memory 接口设计理念不同 | 先适配 L2/L3，L0/L1/L4 后续贡献 |

---

## 八、跨模块协同要点

### 8.1 Callback Chain（影响 Model + Agent + Tool + Evaluation）

Callback Chain 是跨模块的核心基础设施，项目自建了 Model 级 Callback Chain 适配器。对齐策略：

1. **Phase 4**：贡献 Model 级 Callback Chain 到框架
2. **Phase 5**：Agent 和 Tool 模块启用框架 Callback
3. **Phase 3**：Evaluation 模块启用框架 Callbacks

### 8.2 Circuit Breaker（影响 Model + Tool）

项目在 Model（Transport 级）和 Tool（调用级）均有熔断实现。对齐策略：

1. **Phase 4**：贡献 Model 级 Circuit Breaker Transport
2. **Phase 4**：贡献 Tool 级 Circuit Breaker
3. 两者独立贡献，框架可分别采纳

### 8.3 EventBus（影响 Event + Session + Memory + Server）

EventBus 是项目事件基础设施的核心。对齐策略：

1. **Phase 1**：贡献 EventBus 到框架
2. **Phase 2**：Session 和 Memory 适配框架事件接口
3. **Phase 6**：Server AG-UI 使用框架事件流

### 8.4 BuildCache（影响 Agent + Skill）

Agent 的 BuildCache（LRU+singleflight+dirty-mark）与 Skill 的 TTL 缓存有相似模式。对齐策略：

1. **Phase 5**：贡献 BuildCache 到框架
2. **Phase 4**：Skill DBRepositoryAdapter 贡献时复用缓存模式

---

## 九、验收标准

### 9.1 每个 Phase 完成标准

- [x] Phase 0：所有对齐项代码已合并，构建通过，审查通过
- [x] Phase 1（P1-1/P1-2/P1-3）：EventBus/EventWAL/可靠性分级贡献完成，构建通过，审查通过
- [x] Phase 1（P2 后续）：Plugin.OnEvent eventTypeLabel 细化/FromFrameworkEvent 统一转换/tracing 纯数据层贡献完成，构建通过，审查通过
- [x] Phase 3（P1-7/P1-8）：LLM Judge + Callbacks 对齐完成，构建通过，审查通过
- [ ] Phase 1~7：所有该 Phase 的对齐项代码已合并
- [ ] 全量测试通过（`make test`）
- [ ] 全量构建通过（`make build`）
- [ ] Lint 通过（`make lint`）
- [ ] 模块对齐度评分已更新
- [ ] 对齐文档已更新实施状态

### 9.2 整体完成标准

- [ ] 所有 P1/P2 对齐项已完成
- [ ] P3 按业务需求完成 ≥80%
- [ ] 贡献回框架的代码 ≥50% 被框架接受
- [ ] 项目自建代码减少 ≥5000 行
- [ ] 所有模块对齐度 ≥★★★☆☆

---

## 十、附录

### A. 模块对齐度目标

| 模块 | 当前 | Phase 0 后 | Phase 1 后 | Phase 3 后 | 最终目标 |
|------|------|-----------|-----------|-----------|---------|
| Event | ★★☆☆☆ | ★★☆☆☆ | ★★★★★ | ★★★★★ | ★★★★★ |
| Prompt | ☆☆☆☆☆ | ☆☆☆☆☆ | ☆☆☆☆☆ | ★★★☆☆ | ★★★★☆ |
| Knowledge | ★☆☆☆☆ | ★☆☆☆☆ | ★☆☆☆☆ | ★★★☆☆ | ★★★★☆ |
| Team | ★☆☆☆☆ | ★☆☆☆☆ | ★☆☆☆☆ | ★☆☆☆☆ | ★★★★☆ |
| Evaluation | ★★★☆☆ | ★★★☆☆ | ★★★☆☆ | ★★★★☆ | ★★★★★ |
| Tool | ★★★★☆ | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ |
| Session | ★★★☆☆ | ★★★☆☆ | ★★★☆☆ | ★★★★☆ | ★★★★★ |
| Memory | ★★★☆☆ | ★★★☆☆ | ★★★☆☆ | ★★★★☆ | ★★★★★ |
| Server | ★★☆☆☆ | ★★☆☆☆ | ★★☆☆☆ | ★★☆☆☆ | ★★★★☆ |
| Model | ★★★★☆ | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ |
| Agent | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★★ |
| Runner | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★★ |
| Skill | ★★★★☆ | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ |
| Extended | ★★☆☆☆ | ★★☆☆☆ | ★★☆☆☆ | ★★☆☆☆ | ★★★★☆ |

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
