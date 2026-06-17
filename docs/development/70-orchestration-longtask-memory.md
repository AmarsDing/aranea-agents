# 编排引擎 + 24h 长任务 + 领先记忆综合升级

> 模块编号：70
> 关联调研：`docs/reports/2026-06-17-research-orchestration-longtask-memory-upgrade.md`
> 关联设计：`70-orchestration-longtask-memory.design.md`
> 关联开发计划：`70-orchestration-longtask-memory.development.md`

---

## 一、项目愿景

将 Aranea-Agents 升级为支持 24 小时长时间任务执行、任务并行与批量工具调用媲美 Cursor、客户体验绝对友好、记忆系统领先的综合智能体编排平台。核心要求：用户发送指令有任务规划；根据任务属性调用或动态创建 Agent/Team；创建过程可观测；能根据任务使用 Graph 自动编排。

---

## 二、用户故事

### US-1：24h 长任务执行

**作为**一名研究员，**我希望**能向系统提交一个需要长时间运行的研究任务（如"分析 100 篇论文并生成综述"），**以便**系统在后台持续执行，即使关闭浏览器或进程重启也能恢复，最终交付完整结果。

**验收标准**：
- 用户从聊天 UI 直接发起长任务，无需切换到其他入口
- 任务执行期间，前端持续显示进度（步骤、心跳、ETA）
- 进程重启后，未完成的任务能从最近的 checkpoint 恢复
- 任务可被用户中途取消
- 任务结果完整交付，无数据丢失

### US-2：Cursor 级并行批量工具调用

**作为**一名开发者，**我希望**系统能并行执行多个工具调用（如同时编辑 5 个文件），**以便**大幅缩短任务完成时间，体验媲美 Cursor。

**验收标准**：
- 无数据依赖的工具调用自动并行执行
- 文件操作类工具有独立工作区隔离（worktree），互不干扰
- DB 操作类工具有事务保护，失败可回滚
- 并行工具的进度和结果实时流式回传
- 5 文件并行编辑延迟 < 现有串行的 40%

### US-3：绝对友好的客户体验

**作为**一名用户，**我希望**在任务执行的每个阶段都能获得清晰的反馈和可控的操作，**以便**即使任务复杂也能保持信心和掌控感。

**验收标准**：
- 错误发生时有内联重试按钮，且重试动作与错误码联动
- WS 断连 30 秒内检测并提示，提供恢复按钮
- 长任务有进度条、步骤列表、心跳指示
- 所有 UI 文案支持中英文（i18n 覆盖率 100%）
- 移动端三栏布局有合理的折叠策略

### US-4：领先的记忆系统

**作为**一名长期用户，**我希望**系统能记住我的偏好、历史交互、关键事实，并在合适时机主动召回，**以便**减少重复说明，获得个性化服务。

**验收标准**：
- 系统在对话中主动召回相关记忆，无需显式查询
- 记忆冲突时不删除旧记忆，而是标记失效，支持历史重建
- 长任务产生的记忆不爆炸（24h 任务记忆条数 <1000）
- 记忆按 Ebbinghaus 曲线智能衰减，低频访问记忆自动降权
- 用户可查看、编辑、删除被召回的记忆

### US-5：强制任务规划

**作为**一名用户，**我希望**系统在接收我的指令后，能自动识别任务复杂度并规划执行策略，**以便**复杂任务被合理分解和编排，简单任务快速响应。

**验收标准**：
- Simple 任务直接回答（<2s），不走规划
- Moderate/Complex 任务强制走规划路径，不依赖 Agent 自主决定
- 规划过程可观测，前端展示规划时间线（意图识别→复杂度评估→任务分解→策略决策）
- 规划结果包含子任务 DAG、Agent 分配、执行策略

### US-6：动态 Agent 创建

**作为**一名用户，**我希望**当系统中没有匹配的 Agent 时，系统能自动创建新 Agent 来处理我的任务，**以便**任何任务都能找到合适的执行者。

**验收标准**：
- 4 层匹配（Performance/Exact/Semantic/LLM 冷启动）失败时，自动触发 AgentFactory
- AgentFactory 用 LLM 生成 Agent 定义（含 Key/名称/描述/Provider/Model/Tools/Skills/Prompt）
- 创建过程无需人工审核，自动持久化到 DB
- 创建的 Agent 标记来源为"系统创建"，可在 Admin UI 区分
- 动态创建的 Agent 可被后续任务复用
- 创建过程发布事件，前端可见"系统创建了新 Agent [name]"

### US-7：自主 Graph 编排

**作为**一名用户，**我希望**系统能根据我的任务描述自动生成执行 Graph，并在执行过程中根据情况动态调整，**以便**任务以最优拓扑执行，失败时能自动恢复。

**验收标准**：
- NL2Graph：从自然语言任务描述生成 Graph 拓扑
- Graph 编译后，运行时可根据执行情况动态调整（拓扑演化）
- 节点失败时触发运行时重规划（retry/reroute/insert_fallback/rebuild_subgraph）
- 重规划过程可观测，前端可见 Graph 拓扑变化
- 执行历史反馈到 Graph 模板优化

### US-8：全链路可观测

**作为**一名管理员/开发者，**我希望**能查看任务从规划到交付的完整时间线，**以便**定位性能瓶颈、排查失败原因、优化系统。

**验收标准**：
- 编排时间线视图展示 Plan→Allocate→Orchestrate→Delivery 全阶段
- 分布式 Trace 跨 Spirit→Team→Graph 边界传播
- 统一仪表盘聚合 events/metrics/traces/logs
- SpiritStatusBar 显示当前编排阶段
- 可回放任意历史任务的完整事件流

---

## 三、功能需求清单

### FR-1：Postgres 全量迁移

| # | 需求 | 优先级 |
|---|------|--------|
| FR-1.1 | EventStore/WAL/Checkpoint/Run/SessionRun/TeamRun/GraphExecution 迁移到 Postgres | P0 |
| FR-1.2 | Memory（L2/L3/L4）+ Usage 迁移到 Postgres | P1 |
| FR-1.3 | 其余 70+ 表完成迁移，下线 SQLite | P2 |
| FR-1.4 | 补齐 FK 约束（关键实体间引用完整性） | P0 |
| FR-1.5 | 补齐唯一约束（SessionRun/TeamRun 的"一 X 多活跃 Run"） | P0 |
| FR-1.6 | Postgres 连接池支持高并发读写 | P0 |
| FR-1.7 | 数据库错误翻译适配 Postgres 错误码 | P0 |

> 实现细节（连接池参数、错误码映射、迁移 SQL）详见 [70-orchestration-longtask-memory.design.md §二](./70-orchestration-longtask-memory.design.md#二postgres-全量迁移设计)

### FR-2：统一执行引擎（基于 trpc-agent-go 增强）

| # | 需求 | 优先级 |
|---|------|--------|
| FR-2.1 | 后台任务事件可被外部消费（taskrun 事件透传） | P0 |
| FR-2.2 | 跨进程事件流：事件总线增加 Postgres 持久层 | P0 |
| FR-2.3 | 任务级心跳：执行引擎周期发布心跳事件 | P0 |
| FR-2.4 | 崩溃恢复：所有 Run 强制启用 CheckpointSaver（Postgres） | P0 |
| FR-2.5 | RecoveryWorker：进程重启时扫描未完成 Run，从 checkpoint 恢复 | P0 |
| FR-2.6 | Critical 事件写入失败时不发布（WBPF 语义修复） | P0 |
| FR-2.7 | GraphExecution 状态变更经状态机校验（禁止直接赋值） | P0 |
| FR-2.8 | 短/长任务无感切换：前端检测运行 >2min 自动展开任务面板 | P1 |

> 实现细节（接口签名、状态机定义、WBPF 流程）详见 [70-orchestration-longtask-memory.design.md §三](./70-orchestration-longtask-memory.design.md#三统一执行引擎设计)

### FR-3：强制任务规划（Layer 1）

| # | 需求 | 优先级 |
|---|------|--------|
| FR-3.1 | Intent Pass 默认开启 | P0 |
| FR-3.2 | 预规划门控：复杂度 ≥ Moderate 强制走规划路径 | P0 |
| FR-3.3 | 规划时间线事件（planning_phase: start/progress/done） | P0 |
| FR-3.4 | 前端展示规划时间线 | P1 |

### FR-4：动态 Agent 供给（Layer 2）

| # | 需求 | 优先级 |
|---|------|--------|
| FR-4.1 | 语义匹配升级为向量 embedding（替换关键词匹配） | P0 |
| FR-4.2 | AgentFactory：LLM 生成 Agent 定义，无需人工审核 | P0 |
| FR-4.3 | 动态创建的 Agent 持久化到 DB，标记来源 | P0 |
| FR-4.4 | Agent 创建事件发布 | P0 |
| FR-4.5 | Capacity 负载均衡接入（解决 DEV-03） | P1 |
| FR-4.6 | 前端展示"系统创建了新 Agent"通知 | P1 |

> 实现细节（AgentFactory 接口、向量匹配流程）详见 [70-orchestration-longtask-memory.design.md §五](./70-orchestration-longtask-memory.design.md#五layer-2动态-agent-供给设计)

### FR-5：自主 Graph 编排（Layer 3）

| # | 需求 | 优先级 |
|---|------|--------|
| FR-5.1 | NL2Graph：自然语言任务描述 → GraphBuildConfig | P1 |
| FR-5.2 | RuntimeReplanner：节点失败触发重规划（retry/reroute/insert_fallback/rebuild_subgraph） | P1 |
| FR-5.3 | Graph 拓扑演化：运行时动态添加 transfer 边 | P2 |
| FR-5.4 | Graph 学习：执行历史反馈到模板优化 | P2 |
| FR-5.5 | 重规划过程可观测，前端可见拓扑变化 | P1 |

### FR-6：全链路可观测（Layer 4）

| # | 需求 | 优先级 |
|---|------|--------|
| FR-6.1 | 编排时间线视图（Plan→Allocate→Orchestrate→Delivery） | P1 |
| FR-6.2 | 跨边界 Trace 传播（Spirit→Team→Graph） | P1 |
| FR-6.3 | Spirit 编排阶段 Metrics（耗时直方图） | P1 |
| FR-6.4 | 统一编排仪表盘（events+metrics+traces+logs） | P2 |
| FR-6.5 | SpiritStatusBar 显示当前编排阶段 | P1 |

### FR-7：Cursor 级并行工具执行

| # | 需求 | 优先级 |
|---|------|--------|
| FR-7.1 | DependencyAnalyzer：分析 tool calls 数据依赖，构建 DAG | P1 |
| FR-7.2 | WorktreeIsolator：文件操作类工具分配 Git worktree | P1 |
| FR-7.3 | TransactionSandbox：DB 操作类工具包裹 Postgres 事务 | P1 |
| FR-7.4 | ParallelExecutor：worker pool 并行执行，流式回传事件 | P1 |
| FR-7.5 | Team 并行组装优化（errgroup） | P2 |

### FR-8：领先记忆系统（5 项前沿技术）

| # | 需求 | 优先级 |
|---|------|--------|
| FR-8.1 | Bi-temporal 失效标记：Memory 支持双时序（有效区间），冲突时不删除 | P1 |
| FR-8.2 | Ebbinghaus 衰减评分：后台 worker 周期计算衰减因子 | P1 |
| FR-8.3 | Sleep-time Agent 异步整理：后台合并/反思/更新 core memory | P2 |
| FR-8.4 | 主动召回触发器：基于对话上下文自发检索关联记忆 | P2 |
| FR-8.5 | 记忆链接图 Evolution：记忆条目增加链接/关键词/标签 | P2 |
| FR-8.6 | mid-run 增量记忆提取（扩展记忆提取任务触发点） | P1 |

> 实现细节（Memory 结构扩展、衰减公式、召回接口）详见 [70-orchestration-longtask-memory.design.md §九](./70-orchestration-longtask-memory.design.md#九领先记忆系统设计)

### FR-9：体验痛点修复

| # | 需求 | 优先级 |
|---|------|--------|
| FR-9.1 | ErrorBlock 增加重试/切换模型/重新表述按钮，与 errorCodeHints 联动 | P0 |
| FR-9.2 | errorCodeHints 扩展到覆盖所有 apierror 码 | P0 |
| FR-9.3 | WS 断连快速检测（心跳 30s 内） | P0 |
| FR-9.4 | i18n 全覆盖，CI 禁止新增硬编码 | P1 |
| FR-9.5 | 移动端三栏折叠策略 | P1 |
| FR-9.6 | HTTP 错误消息后端地址可配置 | P1 |
| FR-9.7 | 长任务进度可视化（进度条+步骤+ETA） | P1 |
| FR-9.8 | 记忆透明度（侧边栏展示召回记忆） | P2 |

---

## 四、非功能需求

### NFR-1：性能

| # | 需求 | 指标 |
|---|------|------|
| NFR-1.1 | Postgres 写并发 | ≥16 |
| NFR-1.2 | Postgres 读并发 | ≥32 |
| NFR-1.3 | 5 文件并行编辑延迟 | < 现有串行 40% |
| NFR-1.4 | Simple 任务响应 | <2s |
| NFR-1.5 | 24h 任务记忆条数 | <1000 |

### NFR-2：可靠性

| # | 需求 | 指标 |
|---|------|------|
| NFR-2.1 | 24h 任务崩溃恢复 | 进程重启后从 checkpoint 恢复 |
| NFR-2.2 | Critical 事件不丢失 | WAL 失败时不发布 |
| NFR-2.3 | 状态机合法性 | 非法状态转换被拒绝 |

### NFR-3：可观测性

| # | 需求 | 指标 |
|---|------|------|
| NFR-3.1 | 编排时间线 | Plan→Allocate→Orchestrate→Delivery 全阶段可见 |
| NFR-3.2 | Trace 跨边界 | Spirit→Team→Graph 传播 |
| NFR-3.3 | i18n 覆盖率 | 100% |

### NFR-4：可扩展性

| # | 需求 | 指标 |
|---|------|------|
| NFR-4.1 | 四层增强预留扩展点 | 新增规划策略/Agent 类型/Graph 模式/可观测维度不需重构 |
| NFR-4.2 | 记忆系统 5 项技术可独立开关 | 通过 SearchOption/Config 控制 |

### NFR-5：兼容性

| # | 需求 | 指标 |
|---|------|------|
| NFR-5.1 | 对齐 trpc-agent-go 框架接口 | 不破坏框架 Service 接口契约 |
| NFR-5.2 | 现有 API 契约不变 | 不破坏前端兼容 |

---

## 五、交互规格（用户视角）

### IR-1：用户发起长任务

1. 用户在聊天框输入长任务指令（如"分析 100 篇论文并生成综述"）
2. 系统立即返回"正在规划任务..."，展示规划时间线
3. 规划完成后，展示任务分解（子任务 DAG）、Agent 分配、预计耗时
4. 用户确认后（Simple 任务自动确认，Complex 任务可选确认），任务开始执行
5. 前端展示任务面板：进度条、步骤列表、心跳指示、ETA
6. 用户可随时取消任务
7. 任务完成后，结果以消息形式交付

### IR-2：用户发起批量工具调用

1. 用户输入"同时修改 5 个文件的接口定义"
2. 系统识别为批量操作，触发 ParallelToolExecutor
3. 前端展示 5 个并行工具调用卡片，每个独立进度
4. 工具完成后，结果聚合展示
5. 若某工具失败，提示"已回滚 N 个关联操作"

### IR-3：系统动态创建 Agent

1. 用户输入一个特殊领域任务（如"生成量子电路仿真代码"）
2. 系统规划后，4 层匹配无合适 Agent
3. 前端展示"系统正在创建专用 Agent..."
4. AgentFactory 用 LLM 生成 Agent 定义
5. 前端展示"已创建 Agent [量子电路工程师]，开始执行任务"
6. 新 Agent 加入 catalog，后续可复用

### IR-4：Graph 运行时重规划

1. 任务执行中，某节点失败
2. 系统触发 RuntimeReplanner
3. 前端展示"节点 [X] 失败，正在重新规划..."
4. Graph 拓扑动态调整（插入 fallback 节点/重路由）
5. 前端 Graph 可视化展示拓扑变化
6. 任务继续执行

### IR-5：记忆主动召回

1. 用户输入"帮我再订一次上次那家餐厅"
2. 系统主动召回记忆，识别"上次那家餐厅"=用户 2 周前提到的"XX 餐厅"
3. 侧边栏展示"召回的记忆：用户偏好 XX 餐厅"
4. 系统基于记忆执行任务

---

## 六、验收标准（整体）

| # | 验收项 | 验证方式 |
|---|--------|---------|
| AC-1 | 24h 长任务 | 模拟 24h 任务，进程中途重启能从 checkpoint 恢复 |
| AC-2 | Cursor 级并行 | 5 文件并行编辑延迟 < 现有串行 40%；worktree 隔离验证 |
| AC-3 | 极致体验 | 7 痛点全部修复；i18n 覆盖率 100% |
| AC-4 | 领先记忆 | LoCoMo 基准 >85；24h 任务记忆条数 <1000；主动召回准确率 >80% |
| AC-5 | 强制规划 | Simple <2s；Moderate/Complex 强制规划，时间线可见 |
| AC-6 | 动态 Agent 创建 | 无匹配时自动创建，可观测，可复用 |
| AC-7 | 自主 Graph 编排 | NL2Graph 生成有效拓扑；失败触发重规划；拓扑演化有记录 |
| AC-8 | 全链路可观测 | 编排时间线全阶段；Trace 跨边界；仪表盘聚合 |

---

## 七、范围边界

### 包含

- Postgres 全量迁移
- 统一执行引擎（基于 trpc-agent-go 增强）
- 四层编排增强（强制规划 + 动态 Agent + 自主 Graph + 全链路可观测）
- Cursor 级并行工具执行
- 5 项记忆前沿技术集成
- 7 体验痛点修复

### 不包含

- 多租户改造（单机/团队场景为主）
- 移动端原生 App（响应式 Web 为主）
- 联邦 A2A 网络的运行时动态发现（保持预注册）
- Subagent 多级嵌套（保持 MaxGenerationDepth=1）
