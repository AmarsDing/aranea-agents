# Graph 工作流 — 开发计划

> **版本**：2026-05-23 | **状态**：✅ 后端核心 + Phase A/B/C/D 已落地
> **需求**：[36 graph-workflow.md](./36%20graph-workflow.md) · **设计**：[36 graph-workflow.design.md](./36%20graph-workflow.design.md) · **外部参考**：[36-graph-external-reference-playbook.md](./36-graph-external-reference-playbook.md)（Flowise + AgentCoord，其他机器无外部源码时可读）
> **近期变更**：[changelog/2026-05-23-Graph-Frontend-Phase-D.md](../changelog/2026-05-23-Graph-Frontend-Phase-D.md)

---

## 1. 模块定位

Graph 工作流：基于 trpc-agent-go `graph` 包的确定性工作流引擎。核心命题是"Agent 节点内部自主推理，节点间流转由确定性图规则控制"。

**代码锚点**：
- `internal/graph/trpc/` — 图引擎核心（Builder/Registry/Checkpoint/EventBridge/Validator/Templates）
- `internal/graph/adapter/` — 运行时适配器（GraphBuilderFactory）
- `internal/biz/graph.go` + `task.go` — 业务层（GraphUsecase + TaskUsecase）
- `internal/data/graph.go` + `task.go` — 数据层（GraphRepo + TaskRepo）
- `internal/service/graph.go` — 服务层（28 RPC 方法）
- `api/kratos/graph/v1/graph.proto` — Proto 定义
- `web/src/features/graph/` — 前端类型、API、runtime/editor composable
- `web/src/features/graph/runtime/` — WS 执行投影（`useGraphExecutionStream`）
- `web/src/features/graph/editor/` — 布局持久化（`graphLayout.ts`）
- `web/src/components/graph/` — 前端组件（Vue Flow + Run/HITL/Validation/Template）

---

## 2. 现状评估

### 2.1 已实现能力

| 项 | 状态 | 证据 |
|----|------|------|
| GraphDefinition CRUD | ✅ | `GraphUsecase` Create/Get/Update/Delete/List |
| 图执行引擎（BSP/DAG） | ✅ | `BuildStateGraph` + `GraphAgent` + `Executor` |
| Function 节点 | ✅ | `AddNode` + `NodeFunc` + Registry |
| Agent 节点（基础） | ✅ | `AddAgentNode` + Agent 解析 |
| Router 节点 | ✅ | `AddConditionalEdges` + CondFuncRef + PathMap |
| Join 节点 | ✅ | BSP/DAG 引擎自动处理 |
| State Schema + Reducer | ✅ | 4 种 Reducer（Default/Append/Cover/Merge） |
| 条件路由 | ✅ | `ConditionalEdgeDef` + Registry 解析 |
| HITL 中断/恢复 | ✅ | `InterruptBefore/After` + `ResumeExecution` |
| Checkpoint + TimeTravel | ✅ | InMemory + SQLite Saver + History/GetState/EditState |
| 事件桥接 | ✅ | 9 种 ObjectType → EnvelopeType 映射 |
| 设计时校验 | ✅ | `validator.go` 拓扑/Agent引用/StateSchema/循环 |
| 设计模式模板 | ✅ | 6 种内置模板 + `CreateGraphFromTemplate` |
| 任务系统 | ✅ | `TaskUsecase` + 状态机 + Claim/Submit/Heartbeat/Review/Timeout |
| 可视化 | ✅ | DOT 解析 + 结构化 JSON + `VisualizeGraph` API |
| 前端编辑器 | ✅ | Vue Flow + 模板/校验/布局持久化 |
| 前端运行监控 | ✅ | WS 实时节点态 + ExecutionSummary + HITL 对话框 |
| 前端类型与 API | ✅ | `types.ts` + `api.ts` + `stores/graph` |
| Ent Schema | ✅ | 7 张表（definition/execution/task/comment/log/run/event） |
| Proto + Service | ✅ | 28 个 RPC 端点 |

### 2.2 差距清单（2026-05-23 校正）

| # | 差距 | 优先级 | 状态 | 说明 |
|---|------|--------|------|------|
| G1 | LLM 节点接线 | P1 | ✅ | `internal/graph/trpc/node_wiring.go` |
| G2 | Tool 节点接线 | P1 | ✅ | `AddToolsNode` |
| G3 | Agent InputMapper/OutputMapper | P1 | ✅ | Phase D：Proto + mapper 接线 |
| G4 | Command.GoTo / WithEndsMap | P1 | ✅ | `nodeOptions()` Destinations |
| G5 | ExecutionSummary WS | P1 | ✅ | `graph_execution_done` metadata |
| G6 | 前端执行监控增强 | P1 | ✅ | Phase A：`useGraphExecutionStream` + `GraphRunSidebar` |
| G7 | 前端模板选择面板 | P1 | ✅ | Phase B：`GraphTemplatePicker` |
| G8 | 前端校验结果面板 | P1 | ✅ | Phase B：`GraphValidationPanel` + 保存后 validate |
| G9 | RetryPolicy / CachePolicy | P2 | ✅ | Phase D：Proto + `WithRetryPolicy` / `WithNodeCachePolicy` |
| G10 | 用户自定义模板 | P2 | ✅ | Phase D：`SaveGraphAsTemplate` + metadata |
| G11 | Graph 版本管理 | P2 | ✅ | Phase D：metadata 快照 + 回滚 UI |
| G12 | 导入/导出 | P2 | ✅ | Phase D：`ExportGraph` / `ImportGraph` + 编辑器菜单 |
| G13 | Webhook 通知 | P2 | ✅ | `graph.task.status` via GraphTaskRuntime |
| G14 | 熔断策略 | P2 | 📋 | |
| G15 | 节点布局持久化 | P1 | ✅ | `metadata.layout` |
| G16 | Checkpoint/TimeTravel UI | P1 | ✅ | Phase C：`GraphCheckpointPanel` + `useGraphTimeTravel` |
| G17 | Task 看板 UI | P1 | ✅ | Phase C：`GraphTaskKanban` + `GraphTaskDetailDrawer` + WS 投影 |
| G18 | Graph 运行时 CreateTask | P1 | 🚧 | M54 HK-RT-01 |
| G19 | TaskDispatcher | P1 | 🚧 | M54 HK-RT-02 |
| G20 | kanban_* Agent tools | P1 | 🚧 | M54 HK-TOOLS-01 |
| G21 | Unblock + 超时 reclaim | P1 | 🚧 | M54 HK-RT-04/05 |
| G22 | 任务依赖 graph_task_links | P2 | 🚧 | M54 HK-DEP-01 |

---

## 3. 开发阶段（路线图）

> **Phase A/B/C/D** ✅ 2026-05-23 见 [Phase A/B](../changelog/2026-05-23-Graph-Frontend-Phase-A-B.md) · [Phase C](../changelog/2026-05-23-Graph-Frontend-Phase-C.md) · [Phase D](../changelog/2026-05-23-Graph-Frontend-Phase-D.md)

### 阶段一（已完成）：P1 前端运行态 + 设计态闭环

#### Phase A — 运行态（✅）

- `features/graph/runtime/useGraphExecutionStream.ts`
- `GraphRunSidebar` / `GraphHitlDialog`
- `useGraphRunPage` WS 接线

#### Phase B — 设计态（✅）

- `GraphTemplatePicker` / `GraphValidationPanel`
- `metadata.layout` 布局持久化
- Tools catalog 加载

### 阶段二（已完成）：Phase C — 人机协同与任务态

#### 步骤 C-1：Checkpoint 时间线（✅）

**任务**：`GraphCheckpointPanel` + `listCheckpoints` API

#### 步骤 C-2：TimeTravel + EditState（✅）

**任务**：`useGraphTimeTravel` composable + `GraphTimeTravelPanel`

#### 步骤 C-3~5：Task 看板 + WS 投影（✅）

**任务**：`GraphTaskKanban` / `GraphTaskDetailDrawer`；接 `graph_task_status`；`GraphRunInspector` 三 Tab

### 阶段三（已完成）：Phase D — 契约与后端补全

- Proto NodeDef 扩展（retry / cache / mapper）✅
- 导入导出、版本管理、用户模板 ✅
- 前端属性面板 + 编辑器 ⋮ 菜单 ✅

### 后续 backlog

- Webhook 通知（G13）
- 熔断策略（G14，Proto 已定义 `CircuitBreakerPolicy`）

---

## 4. 历史开发阶段（归档）

### 阶段一（后端）：P1 节点类型补全

**任务**：
1. 修改 `internal/graph/trpc/builder.go`：在 `BuildStateGraphWithAgents` 中增加 LLM 节点分支
2. 解析 NodeDef 的 instruction/model/tool_names/user_input_key 字段
3. 调用 `sg.AddLLMNode(nodeID, instruction, model, opts...)` 接线
4. 处理 WithLLMNodeTools、WithLLMNodeUserInputKey 选项

**涉及文件**：
- `internal/graph/trpc/builder.go` — 增加 buildLLMNode 分支

**验证**：
- 创建包含 LLM 节点的 Graph 定义，执行后验证 LLM 调用正常
- 验证 instruction/model/tools 配置正确传递

#### 步骤 2：Tool 节点接线

**任务**：
1. 修改 `builder.go`：增加 Tool 节点分支
2. 解析 NodeDef 的 tool_names/enable_parallel_tools 字段
3. 调用 `sg.AddToolNode(nodeID, toolNames, opts...)` 接线

**涉及文件**：
- `internal/graph/trpc/builder.go` — 增加 buildToolNode 分支

**验证**：
- 创建包含 Tool 节点的 Graph 定义，执行后验证工具调用正常

#### 步骤 3：Agent InputMapper/OutputMapper 接线

**任务**：
1. 修改 `builder.go`：在 Agent 节点构建中解析 input_mapper/output_mapper JSON
2. 传入 `WithSubgraphInputMapper` / `WithSubgraphOutputMapper` 选项
3. 传入 `WithSubgraphIsolatedMessages` / `WithSubgraphInputFromLastResponse` 选项

**涉及文件**：
- `internal/graph/trpc/builder.go` — 修改 buildAgentNode 分支

**验证**：
- 创建包含 Agent 节点的 Graph，配置 InputMapper/OutputMapper
- 执行后验证状态映射正确

#### 步骤 4：Command.GoTo / WithEndsMap 接线

**任务**：
1. 修改 `builder.go`：解析 NodeDef.Destinations 字段
2. 当 Destinations 非空时，传入 `WithEndsMap` 选项
3. Command.GoTo 事件通过 EventBridge 已有 `graph_node_custom` 映射

**涉及文件**：
- `internal/graph/trpc/builder.go` — 增加 WithEndsMap 接线

**验证**：
- 创建包含动态路由的 Graph，验证 Command.GoTo 事件推送

#### 步骤 5：ExecutionSummary Proto + 推送

**任务**：
1. Proto 新增 `ExecutionSummary` / `NodeExecutionSummary` 消息
2. 修改 `event_bridge.go`：在 `graph_execution_done` 事件中填充 ExecutionSummary
3. 前端 `types.ts` 新增 ExecutionSummary 类型
4. 前端 `api.ts` 新增获取 ExecutionSummary 方法

**涉及文件**：
- `api/kratos/graph/v1/graph.proto` — 新增消息
- `internal/graph/trpc/event_bridge.go` — 填充摘要
- `web/src/features/graph/types.ts` — 新增类型
- `web/src/features/graph/api.ts` — 新增方法

**验证**：
- Graph 执行完成后，WS 推送包含 ExecutionSummary 的事件

#### 步骤 6：前端执行监控增强

**任务**：
1. 修改 `GraphRunPage.vue`：监听 WS graph 事件，实时更新节点状态
2. 节点状态样式：运行中（脉冲动画）、完成（绿色勾）、失败（红色叉）、中断（黄色暂停）
3. 执行摘要展示组件

**涉及文件**：
- `web/src/pages/GraphRunPage.vue` — 增强执行监控
- `web/src/components/graph/GraphFlowNode.vue` — 增加状态样式
- `web/src/components/graph/GraphFlowDiamond.vue` — 增加状态样式

**验证**：
- 执行 Graph 时，前端节点实时显示执行状态
- 执行完成后展示摘要信息

#### 步骤 7：前端模板选择面板

**任务**：
1. 完善 `GraphNodePalette.vue`：增加模板选择区域
2. 调用 `listGraphTemplates` API 展示模板列表
3. 选择模板后调用 `createGraphFromTemplate` 创建 Graph

**涉及文件**：
- `web/src/components/graph/GraphNodePalette.vue` — 增加模板区域

**验证**：
- 从模板创建 Graph 正常工作

#### 步骤 8：前端校验结果面板

**任务**：
1. 修改 `GraphPropertyPanel.vue`：增加校验结果展示区域
2. 保存 Graph 时调用 `validateGraph` API
3. 展示 errors/warnings 列表，点击跳转到对应节点

**涉及文件**：
- `web/src/components/graph/GraphPropertyPanel.vue` — 增加校验区域

**验证**：
- 保存无效 Graph 时，前端展示校验错误

---

### 阶段二：P2 增强能力

> 目标：补全重试/缓存策略、资产复用、Webhook 通知等增强能力。

#### 步骤 9：RetryPolicy / CachePolicy

**任务**：
1. Proto NodeDef 新增 `retry_policy` / `cache_policy` 字段
2. 前端 `types.ts` 新增对应类型
3. 修改 `builder.go`：解析并传入 `WithRetryPolicy` / `WithNodeCachePolicy`
4. 前端属性面板增加重试/缓存配置

**涉及文件**：
- `api/kratos/graph/v1/graph.proto`
- `internal/graph/trpc/builder.go`
- `web/src/features/graph/types.ts`
- `web/src/components/graph/GraphPropertyPanel.vue`

**验证**：
- 配置重试策略后，失败节点自动重试

#### 步骤 10：用户自定义模板

**任务**：
1. Proto 新增 `SaveGraphAsTemplate` RPC
2. Biz 层实现：将 Graph 定义转换为模板并持久化
3. Data 层：新增 `graph_template` 表或复用 `graph_definitions` + 标记
4. 前端：编辑器中"保存为模板"按钮

**涉及文件**：
- `api/kratos/graph/v1/graph.proto`
- `internal/biz/graph.go`
- `internal/data/graph.go`
- `web/src/pages/GraphEditorPage.vue`

**验证**：
- 将已有 Graph 保存为模板，从模板创建新 Graph

#### 步骤 11：Graph 版本管理

**任务**：
1. `graph_definitions` 表新增 `version` / `previous_version_id` 字段
2. UpdateGraph 时创建新版本而非覆盖
3. 新增 `ListGraphVersions` / `RollbackGraphVersion` API
4. 前端版本历史面板

**涉及文件**：
- `internal/data/ent/schema/graph_definition.go`
- `internal/biz/graph.go`
- `internal/service/graph.go`
- `api/kratos/graph/v1/graph.proto`

**验证**：
- 更新 Graph 后可查看历史版本并回滚

#### 步骤 12：导入/导出

**任务**：
1. Proto 新增 `ExportGraph` / `ImportGraph` RPC
2. Biz 层实现：Graph 定义序列化/反序列化为 JSON
3. 前端：导入/导出按钮

**涉及文件**：
- `api/kratos/graph/v1/graph.proto`
- `internal/biz/graph.go`
- `internal/service/graph.go`

**验证**：
- 导出 Graph 为 JSON，从 JSON 导入创建新 Graph

#### 步骤 13：Webhook 通知

**任务**：
1. Proto 新增 `WebhookConfig` 消息和 `ConfigureWebhook` RPC
2. 节点执行完成/失败时触发 Webhook
3. 前端：节点属性面板增加 Webhook 配置

**涉及文件**：
- `api/kratos/graph/v1/graph.proto`
- `internal/biz/graph.go`
- `internal/graph/trpc/event_bridge.go`

**验证**：
- 节点执行完成后 Webhook 正确触发

---

### 阶段三：P2 高级能力

#### 步骤 14：熔断策略

**任务**：
1. 实现 `CircuitBreakerPolicy`（Proto 已定义）
2. 连续失败达到阈值时暂停工作流分支
3. 可选执行补偿节点

**涉及文件**：
- `internal/graph/trpc/builder.go`
- `internal/biz/graph.go`

**验证**：
- 连续失败后熔断生效

---

## 4. 验收标准

### 阶段一验收

- [ ] LLM 节点可在画布上创建，执行时正确调用 LLM
- [ ] Tool 节点可在画布上创建，执行时正确调用工具
- [ ] Agent 节点 InputMapper/OutputMapper 正确映射状态
- [ ] Command.GoTo 动态路由事件通过 WS 推送
- [ ] ExecutionSummary 在执行完成后推送
- [ ] 前端节点实时显示执行状态
- [ ] 前端模板选择面板可用
- [ ] 前端校验结果面板可用

### 阶段二验收

- [ ] 节点重试策略配置后自动重试
- [ ] 用户可将 Graph 保存为自定义模板
- [ ] Graph 版本管理支持查看历史和回滚
- [ ] Graph 定义可导入导出
- [ ] Webhook 通知正确触发

### 全局验收

- [ ] `go test ./internal/graph/...` 通过
- [ ] `go test ./internal/biz/...` 通过（Graph + Task 相关）
- [ ] 前端 `npm run build` 无类型错误

---

## 5. 依赖与风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| LLM/Tool 节点框架 API 不稳定 | builder 接线可能需要调整 | 先验证 trpc-agent-go AddLLMNode/AddToolNode API 稳定性 |
| Agent InputMapper/OutputMapper 映射逻辑复杂 | 状态映射可能出错 | 编写单元测试覆盖常见映射场景 |
| 前端执行监控 WS 事件量大 | 高频更新可能影响性能 | 节流更新，批量处理事件 |
| Checkpoint SQLite 持久化并发 | 高并发写入可能冲突 | SQLite WAL 模式 + 写入队列 |
| 版本管理数据迁移 | 现有 Graph 需要版本化 | 默认 version=1，渐进迁移 |

### 模块协调

| 关联模块 | 协调点 |
|----------|--------|
| Agent | Graph Agent 节点引用 Agent 目录，需确保 Agent 存在性校验 |
| Team | Graph 节点可以是 Team（子图），需确保 Team 可作为 Agent 注册 |
| Chat | LLM 节点调用模型服务，需确保模型服务可用 |
| EventBus | 事件桥接依赖 EventBus，需确保事件类型注册 |
| WebSocket | 执行监控依赖 WS 推送，需确保 graph channel 可用 |
